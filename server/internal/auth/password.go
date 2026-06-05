// Package auth handles authentication (§6): password and OAuth login, 2FA,
// algorithm-locked JWTs, and refresh-token rotation with replay detection.
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of pw.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// VerifyPassword reports whether pw matches the bcrypt hash.
func VerifyPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
