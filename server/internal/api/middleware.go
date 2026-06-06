package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/perm"
)

const (
	cookieAccess  = "ft_access"
	cookieRefresh = "ft_refresh"
	cookiePending = "ft_pending"
)

var errUnauthorized = errors.New("unauthorized")

// principal is the authenticated caller plus their group memberships.
type principal struct {
	UserID int64
	Groups []int64
}

// recoverMW converts panics into a generic 500 (no stack leaked, §7.2).
func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("api: panic: %v", rec)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets baseline hardening headers on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// withGlobal wraps the mux with the global middleware chain.
func withGlobal(h http.Handler) http.Handler {
	return recoverMW(securityHeaders(h))
}

// requireAuth validates the access-token cookie and loads the principal.
func (s *Server) requireAuth(r *http.Request) (principal, error) {
	c, err := r.Cookie(cookieAccess)
	if err != nil {
		return principal{}, errUnauthorized
	}
	claims, err := s.auth.JWT().Verify(c.Value)
	if err != nil {
		return principal{}, errUnauthorized
	}
	groups, err := s.auth.UserGroupIDs(r.Context(), claims.UserID)
	if err != nil {
		return principal{}, err
	}
	return principal{UserID: claims.UserID, Groups: groups}, nil
}

// authed adapts a principal-aware handler into an http.HandlerFunc, rejecting
// unauthenticated requests with 401.
func (s *Server) authed(fn func(http.ResponseWriter, *http.Request, principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.requireAuth(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		fn(w, r, p)
	}
}

// can resolves whether the principal may perform action on (storageID, path).
func (s *Server) can(ctx context.Context, p principal, storageID int64, path string, action models.Action) bool {
	ok, err := perm.Resolve(ctx, s.db.Read, p.UserID, p.Groups, storageID, path, action)
	if err != nil {
		log.Printf("api: permission resolve error: %v", err)
		return false
	}
	return ok
}

// canSystem checks a system-level action (vocabulary management, user admin).
func (s *Server) canSystem(ctx context.Context, p principal, action models.Action) bool {
	return s.can(ctx, p, 0, "", action)
}

// --- auth cookies ------------------------------------------------------------

// cookieSecure reports whether cookies should carry the Secure flag — true when
// served over TLS, so it adapts to local http dev vs. https production.
func cookieSecure(r *http.Request) bool { return r.TLS != nil }

func setCookie(w http.ResponseWriter, r *http.Request, name, value, path string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteStrictMode, // primary CSRF defense (§6.4)
		MaxAge:   int(maxAge.Seconds()),
	})
}

func (s *Server) setAuthCookies(w http.ResponseWriter, r *http.Request, access, refresh string) {
	setCookie(w, r, cookieAccess, access, "/", s.cfg.AccessTTL)
	setCookie(w, r, cookieRefresh, refresh, "/api/auth", s.cfg.RefreshTTL)
}

func clearCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, HttpOnly: true, MaxAge: -1})
}

func (s *Server) clearAuthCookies(w http.ResponseWriter) {
	clearCookie(w, cookieAccess, "/")
	clearCookie(w, cookieRefresh, "/api/auth")
	clearCookie(w, cookiePending, "/api/auth")
}
