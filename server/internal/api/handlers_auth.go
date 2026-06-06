package api

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/audit"
	"github.com/chococar-site/filetagsystem/server/internal/auth"
)

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.auth.Login(r.Context(), req.Username, req.Password, clientIP(r))
	if err != nil {
		switch err {
		case auth.ErrInvalidCredentials:
			s.audit.Log(r.Context(), nil, audit.ActionLoginFail, "", nil, map[string]any{"username": req.Username, "ip": clientIP(r)})
			writeError(w, http.StatusUnauthorized, "invalid credentials")
		case auth.ErrLocked:
			writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		default:
			serverError(w, err)
		}
		return
	}
	s.audit.Log(r.Context(), ref(res.User.ID), audit.ActionLogin, "", nil, map[string]any{"ip": clientIP(r), "twofa": res.TwoFARequired})
	if res.TwoFARequired {
		setCookie(w, r, cookiePending, res.PendingToken, "/api/auth", 5*time.Minute)
		writeJSON(w, http.StatusOK, map[string]any{"twofa_required": true})
		return
	}
	s.setAuthCookies(w, r, res.Access, res.Refresh)
	writeJSON(w, http.StatusOK, map[string]any{"user": res.User})
}

func (s *Server) handle2FALogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	c, err := r.Cookie(cookiePending)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no pending 2FA")
		return
	}
	res, err := s.auth.CompleteTwoFA(r.Context(), c.Value, req.Code)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}
	clearCookie(w, cookiePending, "/api/auth")
	s.setAuthCookies(w, r, res.Access, res.Refresh)
	writeJSON(w, http.StatusOK, map[string]any{"user": res.User})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieRefresh); err == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	s.clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieRefresh)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no refresh token")
		return
	}
	access, refresh, _, err := s.auth.Refresh(r.Context(), c.Value)
	if err != nil {
		s.clearAuthCookies(w)
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	s.setAuthCookies(w, r, access, refresh)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, p principal) {
	u, err := s.auth.GetUser(r.Context(), p.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handle2FASetup(w http.ResponseWriter, r *http.Request, p principal) {
	u, err := s.auth.GetUser(r.Context(), p.UserID)
	if err != nil {
		serverError(w, err)
		return
	}
	secret, url, err := s.auth.Setup2FA(r.Context(), p.UserID, u.Username)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": secret, "otpauth_url": url})
}

func (s *Server) handle2FAEnable(w http.ResponseWriter, r *http.Request, p principal) {
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	codes, err := s.auth.Enable2FA(r.Context(), p.UserID, req.Code)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

func (s *Server) handle2FADisable(w http.ResponseWriter, r *http.Request, p principal) {
	if err := s.auth.Disable2FA(r.Context(), p.UserID); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
