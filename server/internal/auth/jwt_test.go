package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTRoundTrip(t *testing.T) {
	i := NewJWTIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Hour)
	tok, err := i.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := i.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 {
		t.Fatalf("uid = %d", claims.UserID)
	}
}

func TestJWTRejectsAlgNone(t *testing.T) {
	i := NewJWTIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Hour)
	// Forge an unsigned token (alg=none) — must be rejected by the locked verifier.
	forged := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
		UserID:  1,
		Purpose: purposeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	s, err := forged.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.Verify(s); err == nil {
		t.Fatal("alg=none token must be rejected")
	}
}

func TestJWTRejectsTamperAndWrongPurpose(t *testing.T) {
	i := NewJWTIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Hour)
	tok, _ := i.Issue(7)
	if _, err := i.Verify(tok + "x"); err == nil {
		t.Fatal("tampered token must fail")
	}
	// A pending token must not pass as an access token.
	pending, _ := i.IssuePending(7)
	if _, err := i.Verify(pending); err != ErrWrongPurpose {
		t.Fatalf("pending-as-access = %v, want ErrWrongPurpose", err)
	}
}

func TestJWTExpiry(t *testing.T) {
	i := NewJWTIssuer([]byte("0123456789abcdef0123456789abcdef"), -time.Second)
	tok, _ := i.Issue(1)
	if _, err := i.Verify(tok); err == nil {
		t.Fatal("expired token must fail")
	}
}
