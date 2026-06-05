package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/crypto"
	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/pquerna/otp/totp"
)

func newService(t *testing.T, maxFails int) *Service {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	key, _ := crypto.GenerateKey()
	kr, _ := crypto.NewKeyRing(key)
	jwtKey := []byte("0123456789abcdef0123456789abcdef")
	return NewService(d, kr, jwtKey, time.Hour, 24*time.Hour, maxFails, time.Minute)
}

func TestLoginSuccessAndLockout(t *testing.T) {
	ctx := context.Background()
	s := newService(t, 2)
	if _, err := s.CreateUser(ctx, "alice", "a@x.io", "s3cret!!"); err != nil {
		t.Fatal(err)
	}

	res, err := s.Login(ctx, "alice", "s3cret!!", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if res.Access == "" || res.Refresh == "" || res.TwoFARequired {
		t.Fatalf("login result = %+v", res)
	}

	// Wrong password twice locks the account (maxFails=2).
	for n := 0; n < 2; n++ {
		if _, err := s.Login(ctx, "alice", "bad", "9.9.9.9"); err != ErrInvalidCredentials {
			t.Fatalf("attempt %d: %v", n, err)
		}
	}
	if _, err := s.Login(ctx, "alice", "s3cret!!", "9.9.9.9"); err != ErrLocked {
		t.Fatalf("after lockout = %v, want ErrLocked", err)
	}
}

func TestUnknownUserIsGenericError(t *testing.T) {
	s := newService(t, 5)
	if _, err := s.Login(context.Background(), "ghost", "x", "1.1.1.1"); err != ErrInvalidCredentials {
		t.Fatalf("unknown user = %v, want generic ErrInvalidCredentials", err)
	}
}

func TestRefreshRotationAndReplay(t *testing.T) {
	ctx := context.Background()
	s := newService(t, 5)
	uid, _ := s.CreateUser(ctx, "bob", "b@x.io", "pw")
	_, refresh1, err := s.IssueTokens(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}

	// Rotate: refresh1 → refresh2.
	_, refresh2, gotUID, err := s.Refresh(ctx, refresh1)
	if err != nil || gotUID != uid || refresh2 == refresh1 {
		t.Fatalf("rotate: uid=%d err=%v", gotUID, err)
	}

	// Replaying the old (revoked) token is detected and revokes the family.
	if _, _, _, err := s.Refresh(ctx, refresh1); err != ErrReplay {
		t.Fatalf("replay = %v, want ErrReplay", err)
	}
	// refresh2 is now revoked too (whole family killed).
	if _, _, _, err := s.Refresh(ctx, refresh2); err != ErrReplay {
		t.Fatalf("post-replay refresh2 = %v, want ErrReplay", err)
	}
}

func TestLogoutRevokes(t *testing.T) {
	ctx := context.Background()
	s := newService(t, 5)
	uid, _ := s.CreateUser(ctx, "carol", "c@x.io", "pw")
	_, refresh, _ := s.IssueTokens(ctx, uid)
	if err := s.Logout(ctx, refresh); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Refresh(ctx, refresh); err != ErrReplay && err != ErrInvalidRefresh {
		t.Fatalf("refresh after logout = %v", err)
	}
}

func TestTwoFAFlow(t *testing.T) {
	ctx := context.Background()
	s := newService(t, 5)
	uid, _ := s.CreateUser(ctx, "dave", "d@x.io", "pw")

	secret, url, err := s.Setup2FA(ctx, uid, "dave")
	if err != nil || secret == "" || url == "" {
		t.Fatalf("setup: %v", err)
	}
	// Not yet enabled → login returns tokens directly.
	res, _ := s.Login(ctx, "dave", "pw", "1.1.1.1")
	if res.TwoFARequired {
		t.Fatal("2FA should not be required before Enable")
	}

	code, _ := totp.GenerateCode(secret, time.Now())
	backups, err := s.Enable2FA(ctx, uid, code)
	if err != nil || len(backups) == 0 {
		t.Fatalf("enable: %v", err)
	}

	// Now login requires 2FA.
	res, err = s.Login(ctx, "dave", "pw", "1.1.1.1")
	if err != nil || !res.TwoFARequired || res.PendingToken == "" {
		t.Fatalf("login after enable = %+v err=%v", res, err)
	}
	// Complete with a fresh TOTP code.
	code, _ = totp.GenerateCode(secret, time.Now())
	done, err := s.CompleteTwoFA(ctx, res.PendingToken, code)
	if err != nil || done.Access == "" {
		t.Fatalf("complete 2FA: %v", err)
	}

	// A backup code works once, then is consumed.
	ok, err := s.VerifyTwoFA(ctx, uid, backups[0])
	if err != nil || !ok {
		t.Fatalf("backup code verify: %v", err)
	}
	if ok, _ := s.VerifyTwoFA(ctx, uid, backups[0]); ok {
		t.Fatal("backup code should be single-use")
	}
}
