package api

import (
	"fmt"
	"net/http"

	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/search"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, p principal) {
	q := r.URL.Query()

	// Permission scope: a search must be bounded by something the caller can
	// read — a specific storage, or system-wide read.
	var storagePtr *int64
	if sid, ok := parseID2(q.Get("storage_id")); ok {
		if !s.can(r.Context(), p, sid, "", models.ActionRead) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		storagePtr = &sid
	} else if !s.canSystem(r.Context(), p, models.ActionRead) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var filters []search.Filter
	for i := 0; ; i++ {
		ftKey := fmt.Sprintf("filters[%d][field_type_id]", i)
		fvKey := fmt.Sprintf("filters[%d][field_value_id]", i)
		stKey := fmt.Sprintf("filters[%d][strict]", i)
		ft, okFT := parseID2(q.Get(ftKey))
		fv, okFV := parseID2(q.Get(fvKey))
		if !okFT || !okFV {
			break
		}
		filters = append(filters, search.Filter{
			FieldTypeID:  ft,
			FieldValueID: fv,
			Strict:       q.Get(stKey) == "true" || q.Get(stKey) == "1",
		})
	}
	if len(filters) == 0 {
		writeError(w, http.StatusBadRequest, "at least one filter is required")
		return
	}

	limit := 50
	if v, ok := parseID2(q.Get("limit")); ok {
		limit = int(v)
	}
	var cursor int64
	if v, ok := parseID2(q.Get("cursor")); ok {
		cursor = v
	}

	results, next, err := s.search.Search(r.Context(), storagePtr, filters, limit, cursor)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results":     ensureSlice(results),
		"next_cursor": next,
	})
}
