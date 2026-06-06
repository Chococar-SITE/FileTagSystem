package auth

import (
	"context"
	"strings"
	"testing"
)

// TestLoginTimingEqualization checks the account-enumeration hardening: bcrypt
// must run even for an unknown user, so dummyHash has to be a real bcrypt hash,
// and an unknown user must still yield the generic error.
func TestLoginTimingEqualization(t *testing.T) {
	if len(dummyHash) == 0 || !strings.HasPrefix(string(dummyHash), "$2") {
		t.Fatalf("dummyHash is not a valid bcrypt hash: %q", dummyHash)
	}
	if VerifyPassword(string(dummyHash), "anything") {
		t.Fatal("dummyHash must not match an arbitrary password")
	}

	s := newService(t, 5)
	if _, err := s.CreateUser(context.Background(), "real", "r@x.io", "pw"); err != nil {
		t.Fatal(err)
	}
	// Unknown user and known-user-wrong-password both return the same generic error.
	if _, err := s.Login(context.Background(), "ghost", "pw", "1.1.1.1"); err != ErrInvalidCredentials {
		t.Fatalf("unknown user = %v, want ErrInvalidCredentials", err)
	}
	if _, err := s.Login(context.Background(), "real", "wrong", "2.2.2.2"); err != ErrInvalidCredentials {
		t.Fatalf("wrong password = %v, want ErrInvalidCredentials", err)
	}
}
