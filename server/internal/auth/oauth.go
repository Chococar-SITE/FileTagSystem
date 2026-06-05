package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrOAuthEmailUnverified means the provider account has no verified email, so
// it cannot be safely linked or used to create an account (§6.4).
var ErrOAuthEmailUnverified = errors.New("auth: oauth email not verified")

// ErrOAuthProvider means the named provider is not configured.
var ErrOAuthProvider = errors.New("auth: oauth provider not configured")

// OAuthUser is the normalized identity returned by a provider.
type OAuthUser struct {
	ProviderID    string
	Email         string
	EmailVerified bool
}

// OAuthProviderConfig configures one provider. Endpoint URLs are injectable so
// the flow can be tested against a mock server.
type OAuthProviderConfig struct {
	Kind         string // "google" | "github"
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserURL      string
	EmailsURL    string // github only
	RedirectURL  string
	Scopes       []string
	HTTPClient   *http.Client
}

// RegisterOAuthProvider adds/replaces a provider by name.
func (s *Service) RegisterOAuthProvider(name string, cfg OAuthProviderConfig) {
	if s.oauth == nil {
		s.oauth = map[string]OAuthProviderConfig{}
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	s.oauth[name] = cfg
}

// OAuthConfigured reports whether a provider is available.
func (s *Service) OAuthConfigured(name string) bool {
	_, ok := s.oauth[name]
	return ok
}

// NewState returns a random anti-CSRF state value (§6.4).
func NewState() (string, error) { return randomURL(24) }

// PKCE is a code verifier and its S256 challenge (§6.4).
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a PKCE pair.
func NewPKCE() (PKCE, error) {
	v, err := randomURL(32)
	if err != nil {
		return PKCE{}, err
	}
	sum := sha256.Sum256([]byte(v))
	return PKCE{Verifier: v, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

func randomURL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthCodeURL builds the provider authorize URL with state + PKCE challenge.
func (s *Service) AuthCodeURL(name, state, challenge string) (string, error) {
	p, ok := s.oauth[name]
	if !ok {
		return "", ErrOAuthProvider
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", p.RedirectURL)
	q.Set("scope", strings.Join(p.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	sep := "?"
	if strings.Contains(p.AuthURL, "?") {
		sep = "&"
	}
	return p.AuthURL + sep + q.Encode(), nil
}

// ExchangeAndFetch swaps the auth code for a token (with the PKCE verifier) and
// returns the normalized provider user.
func (s *Service) ExchangeAndFetch(ctx context.Context, name, code, verifier string) (OAuthUser, error) {
	p, ok := s.oauth[name]
	if !ok {
		return OAuthUser{}, ErrOAuthProvider
	}
	token, err := s.exchange(ctx, p, code, verifier)
	if err != nil {
		return OAuthUser{}, err
	}
	switch p.Kind {
	case "github":
		return s.fetchGitHubUser(ctx, p, token)
	default:
		return s.fetchOIDCUser(ctx, p, token)
	}
}

func (s *Service) exchange(ctx context.Context, p OAuthProviderConfig, code, verifier string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.RedirectURL)
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth: oauth token exchange failed: %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("auth: oauth token response missing access_token")
	}
	return tok.AccessToken, nil
}

func (s *Service) fetchOIDCUser(ctx context.Context, p OAuthProviderConfig, token string) (OAuthUser, error) {
	body, err := s.getBearer(ctx, p, p.UserURL, token)
	if err != nil {
		return OAuthUser{}, err
	}
	var u struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return OAuthUser{}, err
	}
	return OAuthUser{ProviderID: u.Sub, Email: u.Email, EmailVerified: truthy(u.EmailVerified)}, nil
}

func (s *Service) fetchGitHubUser(ctx context.Context, p OAuthProviderConfig, token string) (OAuthUser, error) {
	body, err := s.getBearer(ctx, p, p.UserURL, token)
	if err != nil {
		return OAuthUser{}, err
	}
	var u struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &u); err != nil || u.ID == 0 {
		return OAuthUser{}, errors.New("auth: github user response invalid")
	}
	out := OAuthUser{ProviderID: fmt.Sprintf("%d", u.ID)}
	if p.EmailsURL != "" {
		eb, err := s.getBearer(ctx, p, p.EmailsURL, token)
		if err != nil {
			return OAuthUser{}, err
		}
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		_ = json.Unmarshal(eb, &emails)
		for _, e := range emails {
			if e.Primary && e.Verified {
				out.Email = e.Email
				out.EmailVerified = true
				break
			}
		}
	}
	return out, nil
}

func (s *Service) getBearer(ctx context.Context, p OAuthProviderConfig, target, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: oauth userinfo failed: %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	}
	return false
}

// LoginWithOAuth resolves the provider identity to a local user, creating or
// linking only when the email is verified (§6.4), then completes login (which
// still honors 2FA).
func (s *Service) LoginWithOAuth(ctx context.Context, provider string, ou OAuthUser) (LoginResult, error) {
	// 1. Already linked?
	var uid int64
	err := s.db.Read.QueryRowContext(ctx,
		`SELECT user_id FROM user_oauth WHERE provider=? AND provider_id=?`, provider, ou.ProviderID).Scan(&uid)
	if err == nil {
		return s.finishLogin(ctx, uid)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return LoginResult{}, err
	}

	// 2. Not linked → require a verified email to proceed.
	if !ou.EmailVerified || ou.Email == "" {
		return LoginResult{}, ErrOAuthEmailUnverified
	}

	// 3. Link to an existing user with the same (verified) email, else create.
	var existing int64
	err = s.db.Read.QueryRowContext(ctx, `SELECT id FROM users WHERE email=?`, ou.Email).Scan(&existing)
	switch {
	case err == nil:
		if _, e := s.db.Write.ExecContext(ctx,
			`INSERT INTO user_oauth(user_id, provider, provider_id) VALUES (?,?,?)`, existing, provider, ou.ProviderID); e != nil {
			return LoginResult{}, e
		}
		return s.finishLogin(ctx, existing)
	case errors.Is(err, sql.ErrNoRows):
		newID, e := s.createOAuthUser(ctx, provider, ou)
		if e != nil {
			return LoginResult{}, e
		}
		return s.finishLogin(ctx, newID)
	default:
		return LoginResult{}, err
	}
}

func (s *Service) createOAuthUser(ctx context.Context, provider string, ou OAuthUser) (int64, error) {
	username := deriveUsername(ou.Email)
	randPw, err := randomURL(24)
	if err != nil {
		return 0, err
	}
	hash, err := HashPassword(randPw) // unusable password; OAuth-only account
	if err != nil {
		return 0, err
	}
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Resolve a unique username.
	base := username
	for i := 0; ; i++ {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE username=?`, username).Scan(&n); err != nil {
			return 0, err
		}
		if n == 0 {
			break
		}
		username = fmt.Sprintf("%s%d", base, i+1)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO users(username, email, password_hash) VALUES (?,?,?)`, username, ou.Email, hash)
	if err != nil {
		return 0, err
	}
	uid, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_oauth(user_id, provider, provider_id) VALUES (?,?,?)`, uid, provider, ou.ProviderID); err != nil {
		return 0, err
	}
	return uid, tx.Commit()
}

func deriveUsername(email string) string {
	local := email
	if i := strings.IndexByte(email, '@'); i >= 0 {
		local = email[:i]
	}
	local = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			return r
		}
		return -1
	}, local)
	if local == "" {
		local = "user"
	}
	return local
}
