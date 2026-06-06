package auth

import (
	"context"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// TestTwoFARateLimited verifies that repeated wrong 2FA codes lock out further
// attempts, preventing brute-force of the 6-digit code (§7.4).
func TestTwoFARateLimited(t *testing.T) {
	ctx := context.Background()
	s := newService(t, 3) // lock after 3 failures

	uid, _ := s.CreateUser(ctx, "u", "u@x.io", "pw")
	secret, _, err := s.Setup2FA(ctx, uid, "u")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	if _, err := s.Enable2FA(ctx, uid, code); err != nil {
		t.Fatal(err)
	}

	res, err := s.Login(ctx, "u", "pw", "1.1.1.1")
	if err != nil || !res.TwoFARequired {
		t.Fatalf("login should require 2FA: %v", err)
	}
	pending := res.PendingToken

	// Three wrong codes are rejected as invalid...
	for i := 0; i < 3; i++ {
		if _, err := s.CompleteTwoFA(ctx, pending, "000000"); err != ErrInvalidCredentials {
			t.Fatalf("wrong-code attempt %d = %v, want ErrInvalidCredentials", i, err)
		}
	}
	// ...then the account is locked, so even a (would-be) valid code is refused.
	if _, err := s.CompleteTwoFA(ctx, pending, "000000"); err != ErrLocked {
		t.Fatalf("after lockout = %v, want ErrLocked", err)
	}
}
