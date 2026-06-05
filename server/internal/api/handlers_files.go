package api

import (
	"net/http"
	"path"
	"strings"

	"github.com/chococar-site/filetagsystem/server/internal/audit"
	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/storage"
)

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request, p principal) {
	sid, ok := parseID2(r.URL.Query().Get("storage_id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "storage_id required")
		return
	}
	rel := r.URL.Query().Get("path")
	if !s.can(r.Context(), p, sid, normalizeDir(rel), models.ActionRead) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	kids, err := s.cat.ListChildren(r.Context(), sid, rel)
	if err != nil {
		if err == storage.ErrPathEscape {
			writeError(w, http.StatusBadRequest, "invalid path")
			return
		}
		if err == storage.ErrNotExist {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": rel, "entries": kids})
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForRead(w, r, p)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleGetFields(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForRead(w, r, p)
	if !ok {
		return
	}
	fields, err := s.tags.EffectiveFields(r.Context(), f.StorageID, f.Path)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(fields))
}

func (s *Server) handleApplyField(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForWrite(w, r, p)
	if !ok {
		return
	}
	var req struct {
		FieldTypeID  int64 `json:"field_type_id"`
		FieldValueID int64 `json:"field_value_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.tags.ApplyByFileID(r.Context(), f.ID, req.FieldTypeID, req.FieldValueID); err != nil {
		serverError(w, err)
		return
	}
	s.audit.Log(r.Context(), ref(p.UserID), audit.ActionTagApply, "file", ref(f.ID),
		map[string]any{"field_type_id": req.FieldTypeID, "field_value_id": req.FieldValueID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveField(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForWrite(w, r, p)
	if !ok {
		return
	}
	ftid, ok := parseID(r, "ftid")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad field type id")
		return
	}
	if err := s.tags.RemoveField(r.Context(), f.ID, ftid); err != nil {
		serverError(w, err)
		return
	}
	s.audit.Log(r.Context(), ref(p.UserID), audit.ActionTagRemove, "file", ref(f.ID), map[string]any{"field_type_id": ftid})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetThumbnail(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForWrite(w, r, p)
	if !ok {
		return
	}
	var req struct {
		ThumbnailPath string `json:"thumbnail_path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.cat.SetThumbnailByID(r.Context(), f.ID, req.ThumbnailPath); err != nil {
		notFoundOr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForRead(w, r, p)
	if !ok {
		return
	}
	k := kindForPath(f.Path)
	var size int64
	if f.SizeBytes != nil {
		size = *f.SizeBytes
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind": k.Kind, "mime": k.MIME, "size": size, "streamable": k.Streamable, "truncated": false,
	})
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForRead(w, r, p)
	if !ok {
		return
	}
	prov, _, err := s.cat.Provider(r.Context(), f.StorageID)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	// Cloud providers redirect; local proxies the bytes.
	if url, redirect, err := prov.Link(f.Path); err == nil && redirect {
		http.Redirect(w, r, url, http.StatusFound)
		return
	}
	rc, fi, err := prov.Open(f.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	defer func() { _ = rc.Close() }()

	// Untrusted content hardening (§9.5): neutralize executable types, sandbox.
	w.Header().Set("Content-Type", safeContentType(f.Path))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; frame-ancestors 'none'")
	w.Header().Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(path.Base(f.Path))+"\"")
	http.ServeContent(w, r, path.Base(f.Path), fi.Modified, rc) // handles Range/206
}

// --- helpers -----------------------------------------------------------------

// fileForRead loads the file by id and checks read permission.
func (s *Server) fileForRead(w http.ResponseWriter, r *http.Request, p principal) (models.File, bool) {
	return s.fileForAction(w, r, p, models.ActionRead)
}

// fileForWrite loads the file and checks write_meta permission.
func (s *Server) fileForWrite(w http.ResponseWriter, r *http.Request, p principal) (models.File, bool) {
	return s.fileForAction(w, r, p, models.ActionWriteMeta)
}

func (s *Server) fileForAction(w http.ResponseWriter, r *http.Request, p principal, action models.Action) (models.File, bool) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return models.File{}, false
	}
	f, err := s.cat.GetFile(r.Context(), id)
	if err != nil {
		notFoundOr(w, err)
		return models.File{}, false
	}
	if !s.can(r.Context(), p, f.StorageID, f.Path, action) {
		writeError(w, http.StatusForbidden, "forbidden")
		return models.File{}, false
	}
	return f, true
}

func parseID2(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, true
}

// normalizeDir adds a trailing slash for non-empty directory paths so permission
// prefix matching behaves (a listing target is always a directory).
func normalizeDir(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

func sanitizeFilename(name string) string {
	// strip CR/LF/quotes to prevent header injection (§9.5)
	r := strings.NewReplacer("\r", "", "\n", "", "\"", "", "\\", "")
	return r.Replace(name)
}

// ensureSlice guarantees a non-nil JSON array for effective fields.
func ensureSlice[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
