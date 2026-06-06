package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPKCEChallenge(t *testing.T) {
	p, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if p.Challenge != want {
		t.Fatalf("challenge = %q, want %q", p.Challenge, want)
	}
}

func TestAuthCodeURL(t *testing.T) {
	s := newService(t, 5)
	s.RegisterOAuthProvider("google", OAuthProviderConfig{
		Kind: "google", ClientID: "cid", ClientSecret: "sec",
		AuthURL: "https://accounts.example/auth", RedirectURL: "https://app/cb",
		Scopes: []string{"openid", "email"},
	})
	raw, err := s.AuthCodeURL("google", "state123", "chal123")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	if q.Get("state") != "state123" || q.Get("code_challenge") != "chal123" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("auth url query = %v", q)
	}
	if q.Get("client_id") != "cid" || q.Get("redirect_uri") != "https://app/cb" {
		t.Fatalf("auth url client = %v", q)
	}
}

func TestExchangeAndFetchOIDC(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("code") != "the-code" || r.FormValue("code_verifier") != "the-verifier" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-xyz"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-xyz" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "p1", "email": "a@e.io", "email_verified": true})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	s := newService(t, 5)
	s.RegisterOAuthProvider("google", OAuthProviderConfig{
		Kind: "google", ClientID: "id", ClientSecret: "sec",
		TokenURL: ts.URL + "/token", UserURL: ts.URL + "/userinfo", RedirectURL: "https://app/cb",
	})
	ou, err := s.ExchangeAndFetch(context.Background(), "google", "the-code", "the-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if ou.ProviderID != "p1" || ou.Email != "a@e.io" || !ou.EmailVerified {
		t.Fatalf("oauth user = %+v", ou)
	}
}

func TestLoginWithOAuthLinkCreateReject(t *testing.T) {
	ctx := context.Background()
	s := newService(t, 5)

	// Pre-existing local user with a matching email.
	if _, err := s.CreateUser(ctx, "alice", "alice@e.io", "pw"); err != nil {
		t.Fatal(err)
	}

	// Verified email matching an existing user → link + login.
	res, err := s.LoginWithOAuth(ctx, "google", OAuthUser{ProviderID: "g-1", Email: "alice@e.io", EmailVerified: true})
	if err != nil || res.Access == "" {
		t.Fatalf("link existing: %v", err)
	}
	// Same provider id again → logs in via the link (no duplicate user).
	if _, err := s.LoginWithOAuth(ctx, "google", OAuthUser{ProviderID: "g-1", Email: "alice@e.io", EmailVerified: true}); err != nil {
		t.Fatalf("login via link: %v", err)
	}

	// Verified email, no existing user → create a new account.
	res, err = s.LoginWithOAuth(ctx, "google", OAuthUser{ProviderID: "g-2", Email: "bob@e.io", EmailVerified: true})
	if err != nil || res.Access == "" {
		t.Fatalf("create new: %v", err)
	}
	if res.User.Email != "bob@e.io" {
		t.Fatalf("new user email = %q", res.User.Email)
	}

	// Unverified email → reject (no linking/creation).
	if _, err := s.LoginWithOAuth(ctx, "google", OAuthUser{ProviderID: "g-3", Email: "eve@e.io", EmailVerified: false}); err != ErrOAuthEmailUnverified {
		t.Fatalf("unverified = %v, want ErrOAuthEmailUnverified", err)
	}

	// Exactly 2 users should exist (alice + bob).
	us, _ := s.ListUsers(ctx)
	if len(us) != 2 {
		t.Fatalf("user count = %d, want 2", len(us))
	}
}

func TestDeriveUsername(t *testing.T) {
	cases := map[string]string{
		"alice@example.com": "alice",
		"a.b-c_d@x.io":      "a.b-c_d",
		"@weird":            "user",
	}
	for in, want := range cases {
		if got := deriveUsername(in); got != want {
			t.Errorf("deriveUsername(%q) = %q, want %q", in, got, want)
		}
	}
}
