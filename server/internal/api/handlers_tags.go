package api

import (
	"errors"
	"net/http"

	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/tags"
)

// requireVocabAdmin gates tag-vocabulary mutations behind system-level manage.
func (s *Server) requireVocabAdmin(w http.ResponseWriter, r *http.Request, p principal) bool {
	if !s.canSystem(r.Context(), p, models.ActionManage) {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func (s *Server) handleListFieldTypes(w http.ResponseWriter, r *http.Request, _ principal) {
	fts, err := s.tags.ListFieldTypes(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(fts))
}

func (s *Server) handleCreateFieldType(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	var req struct {
		Name       string `json:"name"`
		AllowMulti bool   `json:"allow_multi"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	id, err := s.tags.CreateFieldType(r.Context(), req.Name, req.AllowMulti)
	if err != nil {
		serverError(w, err)
		return
	}
	ft, _ := s.tags.GetFieldType(r.Context(), id)
	writeJSON(w, http.StatusCreated, ft)
}

func (s *Server) handleUpdateFieldType(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Name       string `json:"name"`
		AllowMulti bool   `json:"allow_multi"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.tags.UpdateFieldType(r.Context(), id, req.Name, req.AllowMulti); err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteFieldType(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.tags.DeleteFieldType(r.Context(), id); err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRootValues(w http.ResponseWriter, r *http.Request, _ principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	vs, err := s.tags.ListRootValues(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(vs))
}

func (s *Server) handleListChildValues(w http.ResponseWriter, r *http.Request, _ principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	vs, err := s.tags.ListChildValues(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(vs))
}

func (s *Server) handleCreateFieldValue(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	var req struct {
		FieldTypeID int64  `json:"field_type_id"`
		ParentID    *int64 `json:"parent_id"`
		Value       string `json:"value"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value required")
		return
	}
	fv, err := s.tags.CreateFieldValue(r.Context(), req.FieldTypeID, req.ParentID, req.Value)
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) || errors.Is(err, tags.ErrFieldTypeMismatch) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, fv)
}

func (s *Server) handleUpdateFieldValue(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Value    *string `json:"value"`
		Reparent bool    `json:"reparent"`
		ParentID *int64  `json:"parent_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Value != nil {
		if err := s.tags.RenameFieldValue(r.Context(), id, *req.Value); err != nil {
			notFoundOrTag(w, err)
			return
		}
	}
	if req.Reparent {
		if err := s.tags.MoveFieldValue(r.Context(), id, req.ParentID); err != nil {
			if errors.Is(err, tags.ErrCycle) || errors.Is(err, tags.ErrFieldTypeMismatch) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			notFoundOrTag(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteFieldValue(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if r.URL.Query().Get("dry_run") == "1" {
		values, files, err := s.tags.DeleteInfluence(r.Context(), id)
		if err != nil {
			notFoundOrTag(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"values": values, "files": files})
		return
	}
	if err := s.tags.DeleteFieldValue(r.Context(), id); err != nil {
		notFoundOrTag(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSearchValues(w http.ResponseWriter, r *http.Request, _ principal) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	var ftPtr *int64
	if v, ok := parseID2(r.URL.Query().Get("field_type_id")); ok {
		ftPtr = &v
	}
	vs, err := s.search.SearchValues(r.Context(), q, ftPtr, 50)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(vs))
}

func (s *Server) handleListAliases(w http.ResponseWriter, r *http.Request, _ principal) {
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	al, err := s.tags.ListAliases(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ensureSlice(al))
}

func (s *Server) handleAddAlias(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	id, ok := parseID(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Alias string `json:"alias"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	aid, err := s.tags.AddAlias(r.Context(), id, req.Alias)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": aid})
}

func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request, p principal) {
	if !s.requireVocabAdmin(w, r, p) {
		return
	}
	aid, ok := parseID(r, "aid")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.tags.DeleteAlias(r.Context(), aid); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func notFoundOrTag(w http.ResponseWriter, err error) {
	if errors.Is(err, tags.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	serverError(w, err)
}
