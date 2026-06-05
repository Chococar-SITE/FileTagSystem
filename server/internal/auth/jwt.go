package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signingMethod is the ONLY accepted JWT algorithm. Locking it rejects
// `alg=none` and RS256→HS256 downgrade attacks (§6.4).
const signingMethod = "HS256"

// purposeAccess / purposePending distinguish a full access token from the
// short-lived token issued between password success and 2FA verification.
const (
	purposeAccess  = "access"
	purposePending = "2fa_pending"
)

// Claims are the JWT claims.
type Claims struct {
	UserID  int64  `json:"uid"`
	Purpose string `json:"pur"`
	jwt.RegisteredClaims
}

// JWTIssuer signs and verifies HS256 access tokens.
type JWTIssuer struct {
	key        []byte
	accessTTL  time.Duration
	pendingTTL time.Duration
}

// NewJWTIssuer builds an issuer. The key comes from secret management (§7.2),
// never hard-coded.
func NewJWTIssuer(key []byte, accessTTL time.Duration) *JWTIssuer {
	return &JWTIssuer{key: key, accessTTL: accessTTL, pendingTTL: 5 * time.Minute}
}

// Issue returns a signed access token for userID.
func (i *JWTIssuer) Issue(userID int64) (string, error) {
	return i.sign(userID, purposeAccess, i.accessTTL)
}

// IssuePending returns a short-lived token marking that the password step passed
// but 2FA is still required.
func (i *JWTIssuer) IssuePending(userID int64) (string, error) {
	return i.sign(userID, purposePending, i.pendingTTL)
}

func (i *JWTIssuer) sign(userID int64, purpose string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:  userID,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.key)
}

// ErrWrongPurpose is returned when a token's purpose does not match.
var ErrWrongPurpose = errors.New("auth: token has wrong purpose")

// Verify parses and validates an access token, enforcing the locked algorithm.
func (i *JWTIssuer) Verify(token string) (*Claims, error) {
	return i.verify(token, purposeAccess)
}

// VerifyPending validates a pending-2FA token.
func (i *JWTIssuer) VerifyPending(token string) (*Claims, error) {
	return i.verify(token, purposePending)
}

func (i *JWTIssuer) verify(token, purpose string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return i.key, nil
	}, jwt.WithValidMethods([]string{signingMethod}))
	if err != nil {
		return nil, err
	}
	if claims.Purpose != purpose {
		return nil, ErrWrongPurpose
	}
	return claims, nil
}
