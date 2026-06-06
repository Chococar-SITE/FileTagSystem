package crypto

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	kr, err := NewKeyRing(mustKey(t))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("totp-secret-üñîçødé-🔐")
	blob, err := kr.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if blob == string(plain) || !bytes.Contains([]byte(blob), []byte(":")) {
		t.Fatalf("blob looks wrong: %q", blob)
	}
	got, err := kr.Decrypt(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round-trip mismatch: %q != %q", got, plain)
	}
}

func TestEncryptIsNondeterministic(t *testing.T) {
	kr, _ := NewKeyRing(mustKey(t))
	a, _ := kr.EncryptString("same")
	b, _ := kr.EncryptString("same")
	if a == b {
		t.Fatal("two encryptions of the same plaintext must differ (random nonce)")
	}
}

func TestWrongKeyFails(t *testing.T) {
	kr1, _ := NewKeyRing(mustKey(t))
	kr2, _ := NewKeyRing(mustKey(t))
	blob, _ := kr1.EncryptString("secret")
	if _, err := kr2.Decrypt(blob); err == nil {
		t.Fatal("decrypt with a ring lacking the key must fail")
	}
}

func TestKeyRotation(t *testing.T) {
	oldKey := mustKey(t)
	krOld, _ := NewKeyRing(oldKey)
	blob, _ := krOld.EncryptString("legacy")

	// New ring: fresh active key, old key retired so legacy blobs still decrypt.
	newKey := mustKey(t)
	krNew, err := NewKeyRing(newKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := krNew.Decrypt(blob)
	if err != nil {
		t.Fatalf("retired key should decrypt legacy blob: %v", err)
	}
	if string(got) != "legacy" {
		t.Fatalf("got %q", got)
	}
	if !krNew.NeedsReEncrypt(blob) {
		t.Fatal("legacy blob should be flagged for re-encryption")
	}
	fresh, _ := krNew.EncryptString("x")
	if krNew.NeedsReEncrypt(fresh) {
		t.Fatal("blob under active key must not need re-encryption")
	}
}

func TestParseKeyValidation(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(make([]byte, KeySize))
	if _, err := ParseKey(good); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if _, err := ParseKey("not-base64!!"); err == nil {
		t.Fatal("invalid base64 accepted")
	}
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if _, err := ParseKey(short); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestDecryptMalformed(t *testing.T) {
	kr, _ := NewKeyRing(mustKey(t))
	if _, err := kr.Decrypt("no-colon-here"); err != ErrMalformed {
		t.Fatalf("want ErrMalformed, got %v", err)
	}
}

func TestLoadOrCreateMasterKey(t *testing.T) {
	// From env.
	envKey := base64.StdEncoding.EncodeToString(make([]byte, KeySize))
	k, err := LoadOrCreateMasterKey(envKey, "")
	if err != nil || len(k) != KeySize {
		t.Fatalf("env path: %v", err)
	}

	// Generate + persist, then reload.
	dir := t.TempDir()
	kf := filepath.Join(dir, "sub", "master.key")
	k1, err := LoadOrCreateMasterKey("", kf)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	info, err := os.Stat(kf)
	if err != nil {
		t.Fatalf("keyfile not written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("keyfile perm = %v, want 0600", info.Mode().Perm())
	}
	k2, err := LoadOrCreateMasterKey("", kf)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("reloaded key differs from persisted key")
	}
}
