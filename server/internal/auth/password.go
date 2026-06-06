// Package auth handles authentication (§6): password and OAuth login, 2FA,
// algorithm-locked JWTs, and refresh-token rotation with replay detection.
package auth

import "golang.org/x/crypto/bcrypt"

// dummyHash is a valid bcrypt hash (DefaultCost) compared against when a login
// username does not exist, so the request still spends the bcrypt time and the
// response timing can't be used to enumerate accounts (§7.4).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equalization-placeholder"), bcrypt.DefaultCost)

// HashPassword returns a bcrypt hash of pw.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// VerifyPassword reports whether pw matches the bcrypt hash.
func VerifyPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
