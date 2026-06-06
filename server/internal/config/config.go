// Package config loads runtime configuration from environment variables.
// Secrets are NEVER read from files in the repo — only env or an out-of-tree
// keyfile (§7.2).
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// OAuth holds a provider's client credentials and redirect.
type OAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Configured reports whether the provider has credentials.
func (o OAuth) Configured() bool { return o.ClientID != "" && o.ClientSecret != "" }

// Config is the fully resolved application configuration.
type Config struct {
	HTTPAddr string
	DataDir  string
	DBPath   string

	MasterKeyB64 string // APP_MASTER_KEY (base64); may be empty (keyfile used)
	KeyfilePath  string // out-of-tree keyfile fallback

	AccessTTL  time.Duration
	RefreshTTL time.Duration

	GitHub OAuth
	Google OAuth

	ScannerBin string // path/name of the Rust scanner binary

	// Security knobs.
	LoginMaxFails  int
	LoginLockout   time.Duration
	TextPreviewCap int64 // max bytes streamed for text preview

	// TrustProxy: when true, the client IP is taken from X-Forwarded-For (set
	// this only behind a trusted reverse proxy). When false (default), the TCP
	// peer address is used so clients cannot spoof their IP to evade per-IP
	// rate limiting or forge audit entries.
	TrustProxy bool

	// General per-client API throttle (§7.4).
	APIRatePerSec float64
	APIRateBurst  float64
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() *Config {
	dataDir := getenv("DATA_DIR", "./data")
	c := &Config{
		HTTPAddr:     getenv("HTTP_ADDR", ":8080"),
		DataDir:      dataDir,
		DBPath:       getenv("DB_PATH", filepath.Join(dataDir, "filetag.db")),
		MasterKeyB64: os.Getenv("APP_MASTER_KEY"),
		// Keyfile defaults OUTSIDE the data dir (a sibling of CWD), per §7.2.
		KeyfilePath:    getenv("APP_KEYFILE", "filetag-master.key"),
		AccessTTL:      getdur("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:     getdur("REFRESH_TTL", 7*24*time.Hour),
		ScannerBin:     getenv("SCANNER_BIN", "filetag-scanner"),
		LoginMaxFails:  getint("LOGIN_MAX_FAILS", 5),
		LoginLockout:   getdur("LOGIN_LOCKOUT", 15*time.Minute),
		TextPreviewCap: int64(getint("TEXT_PREVIEW_CAP", 256*1024)),
		TrustProxy:     getbool("TRUST_PROXY", false),
		APIRatePerSec:  getfloat("API_RATE_PER_SEC", 20),
		APIRateBurst:   getfloat("API_RATE_BURST", 40),
		GitHub: OAuth{
			ClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GITHUB_OAUTH_REDIRECT_URL"),
		},
		Google: OAuth{
			ClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
			ClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
		},
	}
	return c
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getint(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getdur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getbool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getfloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
