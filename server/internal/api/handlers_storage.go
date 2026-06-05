package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/models"
)

func (s *Server) handleListStorages(w http.ResponseWriter, r *http.Request, p principal) {
	all, err := s.cat.ListStorages(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	visible := make([]models.StorageProvider, 0, len(all))
	for _, st := range all {
		if s.can(r.Context(), p, st.ID, "", models.ActionRead) {
			visible = append(visible, st)
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

func (s *Server) handleCreateStorage(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.canSystem(r.Context(), p, models.ActionManage) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		RootPath string `json:"root_path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Type == "" {
		req.Type = "local"
	}
	if req.Type != "local" {
		writeError(w, http.StatusBadRequest, "unsupported storage type")
		return
	}
	if req.Name == "" || req.RootPath == "" {
		writeError(w, http.StatusBadRequest, "name and root_path are required")
		return
	}
	id, err := s.cat.CreateStorage(r.Context(), req.Name, req.Type, req.RootPath)
	if err != nil {
		serverError(w, err)
		return
	}
	st, _ := s.cat.GetStorage(r.Context(), id)
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) handleUpdateStorage(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionManage) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	cur, err := s.cat.GetStorage(r.Context(), id)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	var req struct {
		Name     *string `json:"name"`
		RootPath *string `json:"root_path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name, root := cur.Name, cur.RootPath
	if req.Name != nil {
		name = *req.Name
	}
	if req.RootPath != nil {
		root = *req.RootPath
	}
	if err := s.cat.UpdateStorage(r.Context(), id, name, root); err != nil {
		serverError(w, err)
		return
	}
	st, _ := s.cat.GetStorage(r.Context(), id)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleDeleteStorage(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionAdmin) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.cat.DeleteStorage(r.Context(), id); err != nil {
		notFoundOr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionManage) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	st, err := s.cat.GetStorage(r.Context(), id)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	bin, root := s.cfg.ScannerBin, st.RootPath
	err = s.jobs.Start(id, func(ctx context.Context, progress func(int64)) (int64, error) {
		return s.ingest.RunScanner(ctx, bin, root, id, "", progress)
	})
	if err != nil {
		writeError(w, http.StatusConflict, "a scan is already running")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionRead) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	st, ok := s.jobs.Status(id)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"state": "idle", "processed": 0})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleScanCancel(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionManage) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if !s.jobs.Cancel(id) {
		writeError(w, http.StatusBadRequest, "no scan running")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTagByPath applies a tag to a path, lazily materializing the row (§5.9).
func (s *Server) handleTagByPath(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Path         string `json:"path"`
		FieldTypeID  int64  `json:"field_type_id"`
		FieldValueID int64  `json:"field_value_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.can(r.Context(), p, id, req.Path, models.ActionWriteMeta) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	prov, _, err := s.cat.Provider(r.Context(), id)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	fi, err := prov.Stat(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "path not found")
		return
	}
	fileID, err := s.tags.ApplyByPath(r.Context(), id, fi, req.FieldTypeID, req.FieldValueID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file_id": fileID})
}

// notFoundOr maps catalog.ErrNotFound to 404, else 500.
func notFoundOr(w http.ResponseWriter, err error) {
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	serverError(w, err)
}
