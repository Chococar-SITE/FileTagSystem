package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

var (
	// ErrInvalidRefresh means the refresh token is unknown or expired.
	ErrInvalidRefresh = errors.New("auth: invalid refresh token")
	// ErrReplay means a revoked refresh token was reused (token theft, §6.4).
	ErrReplay = errors.New("auth: refresh token replay detected")
)

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func (s *Service) issueRefresh(ctx context.Context, ex execer, userID int64, family string) (string, error) {
	tok, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = ex.ExecContext(ctx,
		`INSERT INTO refresh_tokens(user_id, token_hash, family_id, expires_at) VALUES (?,?,?,?)`,
		userID, hashToken(tok), family, time.Now().Add(s.refreshTTL).UTC().Format(time.RFC3339))
	return tok, err
}

// IssueTokens issues a fresh access + refresh pair in a new token family.
func (s *Service) IssueTokens(ctx context.Context, userID int64) (access, refresh string, err error) {
	access, err = s.jwt.Issue(userID)
	if err != nil {
		return "", "", err
	}
	family, err := randomToken()
	if err != nil {
		return "", "", err
	}
	refresh, err = s.issueRefresh(ctx, s.db.Write, userID, family)
	return access, refresh, err
}

// Refresh validates and rotates a refresh token. Reusing a revoked token is
// treated as theft and revokes the whole family (§6.4).
func (s *Service) Refresh(ctx context.Context, refresh string) (access, newRefresh string, userID int64, err error) {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return "", "", 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	var family, expires string
	var revoked sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT id, user_id, family_id, revoked_at, expires_at FROM refresh_tokens WHERE token_hash=?`, hashToken(refresh)).
		Scan(&id, &userID, &family, &revoked, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, ErrInvalidRefresh
	}
	if err != nil {
		return "", "", 0, err
	}
	if revoked.Valid {
		if _, e := tx.ExecContext(ctx,
			`UPDATE refresh_tokens SET revoked_at=CURRENT_TIMESTAMP WHERE family_id=? AND revoked_at IS NULL`, family); e != nil {
			return "", "", 0, e
		}
		if e := tx.Commit(); e != nil {
			return "", "", 0, e
		}
		return "", "", 0, ErrReplay
	}
	if exp, perr := time.Parse(time.RFC3339, expires); perr == nil && time.Now().After(exp) {
		return "", "", 0, ErrInvalidRefresh
	}

	if _, err = tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=CURRENT_TIMESTAMP WHERE id=?`, id); err != nil {
		return "", "", 0, err
	}
	newRefresh, err = s.issueRefresh(ctx, tx, userID, family)
	if err != nil {
		return "", "", 0, err
	}
	access, err = s.jwt.Issue(userID)
	if err != nil {
		return "", "", 0, err
	}
	if err = tx.Commit(); err != nil {
		return "", "", 0, err
	}
	return access, newRefresh, userID, nil
}

// Logout revokes the entire family of the given refresh token.
func (s *Service) Logout(ctx context.Context, refresh string) error {
	_, err := s.db.Write.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at=CURRENT_TIMESTAMP
		 WHERE family_id=(SELECT family_id FROM refresh_tokens WHERE token_hash=?) AND revoked_at IS NULL`,
		hashToken(refresh))
	return err
}
