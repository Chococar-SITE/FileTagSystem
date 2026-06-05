package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/chococar-site/filetagsystem/server/internal/audit"
	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/perm"
)

// grantSystemAdmin gives a user system-wide admin (bootstrap/seed only).
func grantSystemAdmin(ctx context.Context, d *db.DB, userID int64) (int64, error) {
	return perm.GrantRaw(ctx, d, models.Permission{
		PrincipalType: models.PrincipalUser,
		PrincipalID:   userID,
		ResourceType:  models.ResourceSystem,
		Grant:         models.PermSet{Admin: true},
	})
}

func (s *Server) requireSystemAdmin(w http.ResponseWriter, r *http.Request, p principal) bool {
	if !s.canSystem(r.Context(), p, models.ActionAdmin) {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// --- users -------------------------------------------------------------------

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	us, err := s.auth.ListUsers(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(us))
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	id, err := s.auth.CreateUser(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not create user (duplicate?)")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if id == p.UserID {
		writeError(w, http.StatusBadRequest, "cannot delete yourself")
		return
	}
	if err := s.auth.DeleteUser(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	s.audit.Log(r.Context(), ref(p.UserID), audit.ActionUserDelete, "user", ref(id), nil)
	w.WriteHeader(http.StatusNoContent)
}

// --- groups ------------------------------------------------------------------

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	gs, err := s.auth.ListGroups(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(gs))
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	id, err := s.auth.CreateGroup(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not create group (duplicate?)")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.auth.DeleteGroup(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	gid, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.auth.AddMember(r.Context(), gid, req.UserID); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	gid, ok := parseID(r, "id")
	uid, ok2 := parseID(r, "uid")
	if !ok || !ok2 {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.auth.RemoveMember(r.Context(), gid, uid); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- permissions -------------------------------------------------------------

func (s *Server) handleGrantPermission(w http.ResponseWriter, r *http.Request, p principal) {
	var req struct {
		PrincipalType string         `json:"principal_type"`
		PrincipalID   int64          `json:"principal_id"`
		ResourceType  string         `json:"resource_type"`
		ResourceID    *int64         `json:"resource_id"`
		Grant         models.PermSet `json:"grant"`
		IsDeny        bool           `json:"is_deny"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	permission := models.Permission{
		PrincipalType: models.PrincipalType(req.PrincipalType),
		PrincipalID:   req.PrincipalID,
		ResourceType:  models.ResourceType(req.ResourceType),
		ResourceID:    req.ResourceID,
		Grant:         req.Grant,
		IsDeny:        req.IsDeny,
	}
	id, err := perm.Grant(r.Context(), s.db, p.UserID, p.Groups, permission)
	switch {
	case errors.Is(err, perm.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, perm.ErrOverGrant):
		writeError(w, http.StatusForbidden, "cannot grant permissions beyond your own")
	case errors.Is(err, perm.ErrBadResource):
		writeError(w, http.StatusBadRequest, "invalid resource")
	case err != nil:
		serverError(w, err)
	default:
		s.audit.Log(r.Context(), ref(p.UserID), audit.ActionPermGrant, string(permission.ResourceType), permission.ResourceID,
			map[string]any{"principal_type": req.PrincipalType, "principal_id": req.PrincipalID, "is_deny": req.IsDeny})
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

func (s *Server) handleDeletePermission(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := perm.DeletePermission(r.Context(), s.db, id); err != nil {
		serverError(w, err)
		return
	}
	s.audit.Log(r.Context(), ref(p.UserID), audit.ActionPermRevoke, "permission", ref(id), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStoragePermissions(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionAdmin) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	rows, err := perm.ListResourcePermissions(r.Context(), s.db, models.ResourceStorage, &id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(rows))
}

// handleListAudit returns recent audit entries for admins (§7.5).
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	limit := 100
	if v, ok := parseID2(r.URL.Query().Get("limit")); ok {
		limit = int(v)
	}
	entries, err := s.audit.List(r.Context(), limit)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(entries))
}

// handleEffective explains a user's effective permissions on a resource (§8.8).
func (s *Server) handleEffective(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireSystemAdmin(w, r, p) {
		return
	}
	q := r.URL.Query()
	uid, ok := parseID2(q.Get("user_id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	rt := models.ResourceType(q.Get("resource_type"))
	var sid int64
	var path string
	switch rt {
	case models.ResourceSystem:
	case models.ResourceStorage:
		if v, ok := parseID2(q.Get("resource_id")); ok {
			sid = v
		}
	case models.ResourceFile:
		if v, ok := parseID2(q.Get("resource_id")); ok {
			f, err := s.cat.GetFile(r.Context(), v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "file not found")
				return
			}
			sid, path = f.StorageID, f.Path
		}
	default:
		writeError(w, http.StatusBadRequest, "bad resource_type")
		return
	}
	groups, err := s.auth.UserGroupIDs(r.Context(), uid)
	if err != nil {
		serverError(w, err)
		return
	}
	set, err := perm.ResolveEffective(r.Context(), s.db.Read, uid, groups, sid, path)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"effective": set})
}
