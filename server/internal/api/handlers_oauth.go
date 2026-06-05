package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/auth"
)

const (
	cookieOAuthState    = "ft_oauth_state"
	cookieOAuthVerifier = "ft_oauth_verifier"
)

// handleOAuthStart begins an OAuth login: it sets a state + PKCE verifier cookie
// and redirects to the provider (§6.4).
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !s.auth.OAuthConfigured(provider) {
		writeError(w, http.StatusNotFound, "provider not configured")
		return
	}
	state, err := auth.NewState()
	if err != nil {
		serverError(w, err)
		return
	}
	pkce, err := auth.NewPKCE()
	if err != nil {
		serverError(w, err)
		return
	}
	setCookie(w, r, cookieOAuthState, state, "/api/auth", 10*time.Minute)
	setCookie(w, r, cookieOAuthVerifier, pkce.Verifier, "/api/auth", 10*time.Minute)
	authURL, err := s.auth.AuthCodeURL(provider, state, pkce.Challenge)
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOAuthCallback verifies state, exchanges the code (with PKCE), resolves
// the identity to a user, and logs in.
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	q := r.URL.Query()
	code, state := q.Get("code"), q.Get("state")

	stateCookie, err1 := r.Cookie(cookieOAuthState)
	verifierCookie, err2 := r.Cookie(cookieOAuthVerifier)
	clearCookie(w, cookieOAuthState, "/api/auth")
	clearCookie(w, cookieOAuthVerifier, "/api/auth")
	if err1 != nil || err2 != nil || state == "" || code == "" || stateCookie.Value != state {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	ou, err := s.auth.ExchangeAndFetch(r.Context(), provider, code, verifierCookie.Value)
	if err != nil {
		if errors.Is(err, auth.ErrOAuthProvider) {
			writeError(w, http.StatusNotFound, "provider not configured")
			return
		}
		writeError(w, http.StatusBadGateway, "oauth exchange failed")
		return
	}
	res, err := s.auth.LoginWithOAuth(r.Context(), provider, ou)
	if err != nil {
		if errors.Is(err, auth.ErrOAuthEmailUnverified) {
			http.Redirect(w, r, "/login?error=email_unverified", http.StatusFound)
			return
		}
		serverError(w, err)
		return
	}
	if res.TwoFARequired {
		setCookie(w, r, cookiePending, res.PendingToken, "/api/auth", 5*time.Minute)
		http.Redirect(w, r, "/login?twofa=1", http.StatusFound)
		return
	}
	s.setAuthCookies(w, r, res.Access, res.Refresh)
	http.Redirect(w, r, "/", http.StatusFound)
}
