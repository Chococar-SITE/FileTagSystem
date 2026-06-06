package api

import (
	"errors"
	"net/http"

	"github.com/chococar-site/filetagsystem/server/internal/audit"
	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/tags"
)

// handleBatchTags applies/removes tags across many targets (§5.11, §8.6).
func (s *Server) handleBatchTags(w http.ResponseWriter, r *http.Request, p principal) {
	var req struct {
		Targets []struct {
			StorageID int64  `json:"storage_id"`
			Path      string `json:"path"`
		} `json:"targets"`
		Apply  []tags.FieldRef `json:"apply"`
		Remove []int64         `json:"remove"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Targets) == 0 {
		writeError(w, http.StatusBadRequest, "no targets")
		return
	}
	ops := make([]tags.BatchOp, 0, len(req.Targets))
	for _, t := range req.Targets {
		if !s.can(r.Context(), p, t.StorageID, normalizeDir(t.Path), models.ActionWriteMeta) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		prov, _, err := s.cat.Provider(r.Context(), t.StorageID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid storage")
			return
		}
		fi, err := prov.Stat(t.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "path not found: "+t.Path)
			return
		}
		ops = append(ops, tags.BatchOp{StorageID: t.StorageID, FI: fi, Apply: req.Apply, Remove: req.Remove})
	}
	if err := s.tags.Batch(r.Context(), ops); err != nil {
		serverError(w, err)
		return
	}
	s.audit.Log(r.Context(), ref(p.UserID), audit.ActionTagApply, "", nil,
		map[string]any{"batch": true, "targets": len(ops), "apply": len(req.Apply), "remove": len(req.Remove)})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMergeValue(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		TargetID int64 `json:"target_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	err := s.tags.MergeFieldValue(r.Context(), id, req.TargetID)
	switch {
	case errors.Is(err, tags.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, tags.ErrHasChildren) || errors.Is(err, tags.ErrFieldTypeMismatch):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		serverError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleValueUsage(w http.ResponseWriter, r *http.Request, _ principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	direct, subtree, err := s.tags.ValueUsage(r.Context(), id)
	if err != nil {
		notFoundOrTag(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"direct": direct, "subtree": subtree})
}

func (s *Server) handleUnusedValues(w http.ResponseWriter, r *http.Request, _ principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	vs, err := s.tags.UnusedValues(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(vs))
}

// --- storage root move / verify / missing (§5.8) -----------------------------

func (s *Server) handleStorageRoot(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionManage) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req struct {
		RootPath string `json:"root_path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RootPath == "" {
		writeError(w, http.StatusBadRequest, "root_path required")
		return
	}
	if err := s.cat.UpdateStorageRoot(r.Context(), id, req.RootPath); err != nil {
		notFoundOr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionManage) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.triggerScan(r.Context(), id); err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusConflict, "a scan is already running")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (s *Server) handleMissing(w http.ResponseWriter, r *http.Request, p principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.can(r.Context(), p, id, "", models.ActionRead) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	files, err := s.cat.ListMissing(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(files))
}
