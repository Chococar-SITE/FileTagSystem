package auth

import (
	"context"

	"github.com/chococar-site/filetagsystem/server/internal/models"
)

// LoginResult is the outcome of a login attempt.
type LoginResult struct {
	User          models.User
	TwoFARequired bool
	PendingToken  string // set when TwoFARequired (carry to 2FA verify)
	Access        string // set when fully authenticated
	Refresh       string
}

// Login verifies username/password with per-account and per-IP throttling
// (§7.4). Failures are generic to avoid account enumeration. If 2FA is enabled,
// it returns TwoFARequired with a short-lived pending token instead of tokens.
func (s *Service) Login(ctx context.Context, username, password, ip string) (LoginResult, error) {
	ukey, ikey := "u:"+username, "ip:"+ip
	if !s.limiter.Allowed(ukey) || !s.limiter.Allowed(ikey) {
		return LoginResult{}, ErrLocked
	}
	id, hash, active, err := s.credentials(ctx, username)
	// Always run bcrypt — even for a missing user — so the response time does not
	// reveal whether the account exists (account enumeration, §7.4).
	if err != nil {
		hash = string(dummyHash)
	}
	passwordOK := VerifyPassword(hash, password)
	if err != nil || !active || !passwordOK {
		s.limiter.Fail(ukey)
		s.limiter.Fail(ikey)
		return LoginResult{}, ErrInvalidCredentials
	}
	s.limiter.Reset(ukey)
	return s.finishLogin(ctx, id)
}

// finishLogin completes authentication for a verified user: it returns a
// pending result if 2FA is enabled, otherwise issues tokens. Shared by password
// and OAuth login.
func (s *Service) finishLogin(ctx context.Context, userID int64) (LoginResult, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return LoginResult{}, err
	}
	enabled, err := s.TwoFAEnabled(ctx, userID)
	if err != nil {
		return LoginResult{}, err
	}
	if enabled {
		pending, err := s.jwt.IssuePending(userID)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{User: u, TwoFARequired: true, PendingToken: pending}, nil
	}
	access, refresh, err := s.IssueTokens(ctx, userID)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: u, Access: access, Refresh: refresh}, nil
}

// CompleteTwoFA verifies the 2FA code for a pending login and issues tokens.
func (s *Service) CompleteTwoFA(ctx context.Context, pendingToken, code string) (LoginResult, error) {
	claims, err := s.jwt.VerifyPending(pendingToken)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	ok, err := s.VerifyTwoFA(ctx, claims.UserID, code)
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		return LoginResult{}, ErrInvalidCredentials
	}
	u, err := s.GetUser(ctx, claims.UserID)
	if err != nil {
		return LoginResult{}, err
	}
	access, refresh, err := s.IssueTokens(ctx, claims.UserID)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: u, Access: access, Refresh: refresh}, nil
}
