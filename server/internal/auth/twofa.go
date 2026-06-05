package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/pquerna/otp/totp"
)

// Setup2FA generates a TOTP secret for the user (stored encrypted, §7.2) and
// returns the secret + otpauth:// URL for the authenticator QR code. 2FA is not
// enabled until Enable2FA succeeds with a valid code.
func (s *Service) Setup2FA(ctx context.Context, userID int64, account string) (secret, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.issuerName, AccountName: account})
	if err != nil {
		return "", "", err
	}
	enc, err := s.keys.EncryptString(key.Secret())
	if err != nil {
		return "", "", err
	}
	if _, err := s.db.Write.ExecContext(ctx,
		`INSERT INTO user_2fa(user_id, totp_secret, is_enabled) VALUES (?,?,0)
		 ON CONFLICT(user_id) DO UPDATE SET totp_secret=excluded.totp_secret, is_enabled=0`,
		userID, enc); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) loadSecret(ctx context.Context, userID int64) (secret string, enabled bool, err error) {
	var enc sql.NullString
	err = s.db.Read.QueryRowContext(ctx,
		`SELECT totp_secret, is_enabled FROM user_2fa WHERE user_id=?`, userID).Scan(&enc, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil || !enc.Valid {
		return "", enabled, err
	}
	plain, err := s.keys.Decrypt(enc.String)
	return string(plain), enabled, err
}

// TwoFAEnabled reports whether the user has 2FA active.
func (s *Service) TwoFAEnabled(ctx context.Context, userID int64) (bool, error) {
	_, enabled, err := s.loadSecret(ctx, userID)
	return enabled, err
}

// Enable2FA verifies a code against the pending secret, activates 2FA, and
// returns one-time backup codes (shown once; stored hashed).
func (s *Service) Enable2FA(ctx context.Context, userID int64, code string) ([]string, error) {
	secret, _, err := s.loadSecret(ctx, userID)
	if err != nil {
		return nil, err
	}
	if secret == "" {
		return nil, errors.New("auth: run 2FA setup first")
	}
	if !totp.Validate(code, secret) {
		return nil, ErrInvalidCredentials
	}
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE user_2fa SET is_enabled=1 WHERE user_id=?`, userID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_backup_codes WHERE user_id=?`, userID); err != nil {
		return nil, err
	}
	codes := make([]string, 10)
	for i := range codes {
		c, err := randomBackupCode()
		if err != nil {
			return nil, err
		}
		codes[i] = c
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_backup_codes(user_id, code_hash) VALUES (?,?)`, userID, hashToken(c)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return codes, nil
}

// Disable2FA turns off 2FA and removes secrets + backup codes.
func (s *Service) Disable2FA(ctx context.Context, userID int64) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_2fa WHERE user_id=?`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_backup_codes WHERE user_id=?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// VerifyTwoFA accepts a TOTP code or an unused backup code.
func (s *Service) VerifyTwoFA(ctx context.Context, userID int64, code string) (bool, error) {
	secret, enabled, err := s.loadSecret(ctx, userID)
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, nil
	}
	if totp.Validate(code, secret) {
		return true, nil
	}
	return s.consumeBackupCode(ctx, userID, code)
}

func (s *Service) consumeBackupCode(ctx context.Context, userID int64, code string) (bool, error) {
	res, err := s.db.Write.ExecContext(ctx,
		`UPDATE user_backup_codes SET used_at=CURRENT_TIMESTAMP
		 WHERE user_id=? AND code_hash=? AND used_at IS NULL`, userID, hashToken(code))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func randomBackupCode() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// 10 hex chars, grouped for readability: xxxxx-xxxxx
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s", h[:5], h[5:]), nil
}
