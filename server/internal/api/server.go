// Package api exposes the HTTP REST API (§8). Every mutating/reading handler is
// permission-aware (§6.3); errors are generic to the client (§7.2).
package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/chococar-site/filetagsystem/server/internal/audit"
	"github.com/chococar-site/filetagsystem/server/internal/auth"
	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/config"
	"github.com/chococar-site/filetagsystem/server/internal/crypto"
	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/ingest"
	"github.com/chococar-site/filetagsystem/server/internal/search"
	"github.com/chococar-site/filetagsystem/server/internal/tags"
)

// Server holds the application services and the HTTP router.
type Server struct {
	cfg    *config.Config
	db     *db.DB
	cat    *catalog.Store
	tags   *tags.Store
	search *search.Store
	auth   *auth.Service
	ingest *ingest.Ingester
	jobs   *ingest.JobManager
	audit  *audit.Logger
	mux    *http.ServeMux
}

// ref returns a pointer to v (handy for nullable audit ids).
func ref[T any](v T) *T { return &v }

// NewServer constructs the server and registers routes.
func NewServer(cfg *config.Config, d *db.DB, keys *crypto.KeyRing, jwtKey []byte) *Server {
	s := &Server{
		cfg:    cfg,
		db:     d,
		cat:    catalog.New(d),
		tags:   tags.New(d),
		search: search.New(d),
		auth:   auth.NewService(d, keys, jwtKey, cfg.AccessTTL, cfg.RefreshTTL, cfg.LoginMaxFails, cfg.LoginLockout),
		ingest: ingest.New(d),
		jobs:   ingest.NewJobManager(),
		audit:  audit.New(d),
		mux:    http.NewServeMux(),
	}
	s.registerOAuth()
	s.routes()
	return s
}

// registerOAuth wires the GitHub/Google providers when configured (§6.1).
func (s *Server) registerOAuth() {
	if s.cfg.GitHub.Configured() {
		s.auth.RegisterOAuthProvider("github", auth.OAuthProviderConfig{
			Kind:         "github",
			ClientID:     s.cfg.GitHub.ClientID,
			ClientSecret: s.cfg.GitHub.ClientSecret,
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserURL:      "https://api.github.com/user",
			EmailsURL:    "https://api.github.com/user/emails",
			RedirectURL:  s.cfg.GitHub.RedirectURL,
			Scopes:       []string{"read:user", "user:email"},
		})
	}
	if s.cfg.Google.Configured() {
		s.auth.RegisterOAuthProvider("google", auth.OAuthProviderConfig{
			Kind:         "google",
			ClientID:     s.cfg.Google.ClientID,
			ClientSecret: s.cfg.Google.ClientSecret,
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			UserURL:      "https://openidconnect.googleapis.com/v1/userinfo",
			RedirectURL:  s.cfg.Google.RedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
		})
	}
}

// Handler returns the HTTP handler with global middleware applied.
func (s *Server) Handler() http.Handler { return withGlobal(s.mux) }

// Auth exposes the auth service (used by main for bootstrap).
func (s *Server) Auth() *auth.Service { return s.auth }

func (s *Server) routes() {
	m := s.mux

	// Auth
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/2fa/verify", s.handle2FALogin)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("POST /api/auth/refresh", s.handleRefresh)
	m.HandleFunc("GET /api/auth/me", s.authed(s.handleMe))
	m.HandleFunc("POST /api/auth/2fa/setup", s.authed(s.handle2FASetup))
	m.HandleFunc("POST /api/auth/2fa/enable", s.authed(s.handle2FAEnable))
	m.HandleFunc("POST /api/auth/2fa/disable", s.authed(s.handle2FADisable))
	m.HandleFunc("GET /api/auth/providers", s.handleOAuthProviders)
	m.HandleFunc("GET /api/auth/oauth/{provider}", s.handleOAuthStart)
	m.HandleFunc("GET /api/auth/oauth/{provider}/callback", s.handleOAuthCallback)

	// Storages
	m.HandleFunc("GET /api/storages", s.authed(s.handleListStorages))
	m.HandleFunc("POST /api/storages", s.authed(s.handleCreateStorage))
	m.HandleFunc("PUT /api/storages/{id}", s.authed(s.handleUpdateStorage))
	m.HandleFunc("DELETE /api/storages/{id}", s.authed(s.handleDeleteStorage))
	m.HandleFunc("POST /api/storages/{id}/scan", s.authed(s.handleScan))
	m.HandleFunc("GET /api/storages/{id}/status", s.authed(s.handleScanStatus))
	m.HandleFunc("POST /api/storages/{id}/scan/cancel", s.authed(s.handleScanCancel))
	m.HandleFunc("POST /api/storages/{id}/tags", s.authed(s.handleTagByPath))
	m.HandleFunc("PUT /api/storages/{id}/root", s.authed(s.handleStorageRoot))
	m.HandleFunc("POST /api/storages/{id}/verify", s.authed(s.handleVerify))
	m.HandleFunc("GET /api/storages/{id}/missing", s.authed(s.handleMissing))

	// Files
	m.HandleFunc("GET /api/files", s.authed(s.handleListFiles))
	m.HandleFunc("GET /api/files/{id}", s.authed(s.handleGetFile))
	m.HandleFunc("GET /api/files/{id}/fields", s.authed(s.handleGetFields))
	m.HandleFunc("POST /api/files/{id}/fields", s.authed(s.handleApplyField))
	m.HandleFunc("DELETE /api/files/{id}/fields/{ftid}", s.authed(s.handleRemoveField))
	m.HandleFunc("GET /api/files/{id}/thumbnail", s.authed(s.handleThumbnail))
	m.HandleFunc("PUT /api/files/{id}/thumbnail", s.authed(s.handleSetThumbnail))
	m.HandleFunc("GET /api/files/{id}/preview", s.authed(s.handlePreview))
	m.HandleFunc("GET /api/files/{id}/raw", s.authed(s.handleRaw))
	m.HandleFunc("GET /api/files/{id}/archive", s.authed(s.handleArchive))

	// Field types / values / aliases
	m.HandleFunc("GET /api/field-types", s.authed(s.handleListFieldTypes))
	m.HandleFunc("POST /api/field-types", s.authed(s.handleCreateFieldType))
	m.HandleFunc("PUT /api/field-types/{id}", s.authed(s.handleUpdateFieldType))
	m.HandleFunc("DELETE /api/field-types/{id}", s.authed(s.handleDeleteFieldType))
	m.HandleFunc("GET /api/field-types/{id}/values", s.authed(s.handleListRootValues))
	m.HandleFunc("GET /api/field-values/{id}/children", s.authed(s.handleListChildValues))
	m.HandleFunc("POST /api/field-values", s.authed(s.handleCreateFieldValue))
	m.HandleFunc("PUT /api/field-values/{id}", s.authed(s.handleUpdateFieldValue))
	m.HandleFunc("DELETE /api/field-values/{id}", s.authed(s.handleDeleteFieldValue))
	m.HandleFunc("GET /api/field-values/search", s.authed(s.handleSearchValues))
	m.HandleFunc("GET /api/field-values/{id}/aliases", s.authed(s.handleListAliases))
	m.HandleFunc("POST /api/field-values/{id}/aliases", s.authed(s.handleAddAlias))
	m.HandleFunc("DELETE /api/field-values/{id}/aliases/{aid}", s.authed(s.handleDeleteAlias))
	m.HandleFunc("POST /api/field-values/{id}/merge", s.authed(s.handleMergeValue))
	m.HandleFunc("GET /api/field-values/{id}/usage", s.authed(s.handleValueUsage))
	m.HandleFunc("GET /api/field-types/{id}/unused", s.authed(s.handleUnusedValues))

	// Tag application (batch)
	m.HandleFunc("POST /api/tags/batch", s.authed(s.handleBatchTags))

	// Search
	m.HandleFunc("GET /api/search", s.authed(s.handleSearch))

	// Users / groups / permissions
	m.HandleFunc("GET /api/users", s.authed(s.handleListUsers))
	m.HandleFunc("POST /api/users", s.authed(s.handleCreateUser))
	m.HandleFunc("DELETE /api/users/{id}", s.authed(s.handleDeleteUser))
	m.HandleFunc("GET /api/groups", s.authed(s.handleListGroups))
	m.HandleFunc("POST /api/groups", s.authed(s.handleCreateGroup))
	m.HandleFunc("DELETE /api/groups/{id}", s.authed(s.handleDeleteGroup))
	m.HandleFunc("POST /api/groups/{id}/members", s.authed(s.handleAddMember))
	m.HandleFunc("DELETE /api/groups/{id}/members/{uid}", s.authed(s.handleRemoveMember))
	m.HandleFunc("POST /api/permissions", s.authed(s.handleGrantPermission))
	m.HandleFunc("DELETE /api/permissions/{id}", s.authed(s.handleDeletePermission))
	m.HandleFunc("GET /api/storages/{id}/permissions", s.authed(s.handleStoragePermissions))
	m.HandleFunc("GET /api/permissions/effective", s.authed(s.handleEffective))
	m.HandleFunc("GET /api/audit", s.authed(s.handleListAudit))

	// Health
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// SPA / static assets (optional; serves the built frontend if present).
	s.registerStatic(m)
}

// Bootstrap seeds an initial admin (system admin permission) when no users
// exist, so a fresh install is usable. Credentials come from env.
func (s *Server) Bootstrap(ctx context.Context, username, password string) (bool, error) {
	var n int
	if err := s.db.Read.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	id, err := s.auth.CreateUser(ctx, username, username+"@local", password)
	if err != nil {
		return false, err
	}
	if _, err := grantSystemAdmin(ctx, s.db, id); err != nil {
		return false, err
	}
	return true, nil
}

// parseID reads an int64 path value.
func parseID(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
