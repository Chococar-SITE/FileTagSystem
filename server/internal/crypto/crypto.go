// Package crypto provides AES-256-GCM encryption for sensitive at-rest fields
// (§7.2) and master-key resolution. Ciphertext is stored as "keyid:base64(nonce‖ct)"
// so keys can be rotated: old keys stay in the ring to decrypt legacy blobs, which
// are lazily re-encrypted under the active key when read.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// KeySize is the required master-key length (AES-256).
const KeySize = 32

var (
	// ErrUnknownKey means the blob's key id is not present in the ring.
	ErrUnknownKey = errors.New("crypto: unknown key id")
	// ErrMalformed means the blob is not "keyid:base64".
	ErrMalformed = errors.New("crypto: malformed ciphertext blob")
)

// GenerateKey returns a cryptographically random 32-byte key.
func GenerateKey() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return nil, err
	}
	return k, nil
}

// ParseKey decodes a base64 master key and validates its length.
func ParseKey(b64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("crypto: decode key: %w", err)
	}
	if len(raw) != KeySize {
		return nil, fmt.Errorf("crypto: key must be %d bytes, got %d", KeySize, len(raw))
	}
	return raw, nil
}

// keyID is a short, stable fingerprint of a key, used as the blob prefix so the
// right key is selected on decrypt without manual version bookkeeping.
func keyID(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:4])
}

// KeyRing holds the active key plus any retired keys for decryption.
type KeyRing struct {
	keys     map[string][]byte
	activeID string
}

// NewKeyRing builds a ring with one active key and zero or more retired keys.
func NewKeyRing(active []byte, retired ...[]byte) (*KeyRing, error) {
	if len(active) != KeySize {
		return nil, fmt.Errorf("crypto: active key must be %d bytes", KeySize)
	}
	kr := &KeyRing{keys: make(map[string][]byte)}
	kr.activeID = keyID(active)
	kr.keys[kr.activeID] = append([]byte(nil), active...)
	for _, k := range retired {
		if len(k) == KeySize {
			kr.keys[keyID(k)] = append([]byte(nil), k...)
		}
	}
	return kr, nil
}

// ActiveID returns the fingerprint of the active key.
func (kr *KeyRing) ActiveID() string { return kr.activeID }

// Encrypt seals plaintext under the active key, returning "keyid:base64(nonce‖ct)".
func (kr *KeyRing) Encrypt(plain []byte) (string, error) {
	gcm, err := newGCM(kr.keys[kr.activeID])
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return kr.activeID + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// EncryptString is a convenience wrapper over Encrypt.
func (kr *KeyRing) EncryptString(s string) (string, error) { return kr.Encrypt([]byte(s)) }

// Decrypt opens a blob produced by Encrypt, selecting the key by its id prefix.
func (kr *KeyRing) Decrypt(blob string) ([]byte, error) {
	id, b64, ok := strings.Cut(blob, ":")
	if !ok {
		return nil, ErrMalformed
	}
	key, ok := kr.keys[id]
	if !ok {
		return nil, ErrUnknownKey
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("crypto: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return nil, ErrMalformed
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// NeedsReEncrypt reports whether a blob was sealed under a non-active key and
// should be lazily re-encrypted on next write.
func (kr *KeyRing) NeedsReEncrypt(blob string) bool {
	id, _, ok := strings.Cut(blob, ":")
	return !ok || id != kr.activeID
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// LoadOrCreateMasterKey resolves the master key. Priority: APP_MASTER_KEY env
// (base64) → existing keyfile → generate a new key and persist it (0600) at
// keyfilePath, which the operator should place OUTSIDE the data directory (§7.2).
func LoadOrCreateMasterKey(envVal, keyfilePath string) ([]byte, error) {
	if strings.TrimSpace(envVal) != "" {
		return ParseKey(envVal)
	}
	if keyfilePath == "" {
		return nil, errors.New("crypto: APP_MASTER_KEY unset and no keyfile path configured")
	}
	if data, err := os.ReadFile(keyfilePath); err == nil {
		return ParseKey(string(data))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key, err := GenerateKey()
	if err != nil {
		return nil, err
	}
	if dir := filepath.Dir(keyfilePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	enc := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(keyfilePath, []byte(enc), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}
