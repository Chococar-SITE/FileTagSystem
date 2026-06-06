// Command server is the File Tag Management System API server.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/api"
	"github.com/chococar-site/filetagsystem/server/internal/config"
	"github.com/chococar-site/filetagsystem/server/internal/crypto"
	"github.com/chococar-site/filetagsystem/server/internal/db"
)

func main() {
	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// Master key: env or out-of-tree keyfile (§7.2).
	masterKey, err := crypto.LoadOrCreateMasterKey(cfg.MasterKeyB64, cfg.KeyfilePath)
	if err != nil {
		log.Fatalf("master key: %v", err)
	}
	keyring, err := crypto.NewKeyRing(masterKey)
	if err != nil {
		log.Fatalf("keyring: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() { _ = database.Close() }()

	// JWT signing key: dedicated env, else derived keyfile beside the master key.
	jwtKey, err := loadJWTKey(cfg)
	if err != nil {
		log.Fatalf("jwt key: %v", err)
	}

	srv := api.NewServer(cfg, database, keyring, jwtKey)

	// Bootstrap an initial admin if the install is empty.
	adminUser := getenv("ADMIN_USERNAME", "admin")
	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminPass == "" {
		adminPass = randomString(12)
	}
	if created, err := srv.Bootstrap(context.Background(), adminUser, adminPass); err != nil {
		log.Fatalf("bootstrap: %v", err)
	} else if created {
		log.Printf("bootstrap: created admin user %q with password %q — CHANGE THIS IMMEDIATELY", adminUser, adminPass)
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s (data dir %s)", cfg.HTTPAddr, cfg.DataDir)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// loadJWTKey resolves the JWT signing key from APP_JWT_KEY (base64) or a keyfile
// next to the data dir, generating one on first run.
func loadJWTKey(cfg *config.Config) ([]byte, error) {
	if v := os.Getenv("APP_JWT_KEY"); v != "" {
		return []byte(v), nil
	}
	path := filepath.Join(filepath.Dir(cfg.KeyfilePath), "filetag-jwt.key")
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	}
	key := []byte(randomString(48))
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)[:n]
}
