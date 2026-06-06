// Package search implements tag search (§5.3–§5.7, §8.7): multiple AND filters,
// each fuzzy (value + descendants via Materialized Path) or strict (exact id),
// intersected; plus text lookup over values and aliases (§5.5).
package search

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/models"
)

// ErrNoFilters is returned when a search has no filters.
var ErrNoFilters = errors.New("search: at least one filter is required")

// Store is the search repository.
type Store struct {
	db *db.DB
}

// New builds a Store.
func New(d *db.DB) *Store { return &Store{db: d} }

// Filter is one AND condition.
type Filter struct {
	FieldTypeID  int64
	FieldValueID int64
	Strict       bool // exact node only (no descendants)
}

// ResultFile is a search hit (a node that directly carries a matching tag).
type ResultFile struct {
	ID            int64   `json:"id"`
	StorageID     int64   `json:"storage_id"`
	Path          string  `json:"path"`
	IsDir         bool    `json:"is_dir"`
	ThumbnailPath *string `json:"thumbnail_path,omitempty"`
}

// Search returns files matching ALL filters (§8.0 AND semantics), keyset-
// paginated by file id. storageID optionally restricts to one source.
func (s *Store) Search(ctx context.Context, storageID *int64, filters []Filter, limit int, cursor int64) (files []ResultFile, next int64, err error) {
	if len(filters) == 0 {
		return nil, 0, ErrNoFilters
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	// Expand each filter to its set of matching field_value ids.
	type expanded struct {
		ids []int64
	}
	sets := make([]expanded, 0, len(filters))
	for _, f := range filters {
		ids, err := s.valueIDs(ctx, f)
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			return nil, 0, nil // a filter matches nothing → empty result
		}
		sets = append(sets, expanded{ids: ids})
	}
	// Most selective (fewest ids) first so INTERSECT converges early (§5.4 C).
	sort.SliceStable(sets, func(i, j int) bool { return len(sets[i].ids) < len(sets[j].ids) })

	var subqueries []string
	var args []any
	for _, set := range sets {
		ph := make([]string, len(set.ids))
		for i, id := range set.ids {
			ph[i] = "?"
			args = append(args, id)
		}
		subqueries = append(subqueries, fmt.Sprintf(
			"SELECT file_id FROM file_fields WHERE field_value_id IN (%s)", strings.Join(ph, ",")))
	}
	intersection := strings.Join(subqueries, "\nINTERSECT\n")

	var sb strings.Builder
	sb.WriteString(`SELECT f.id, f.storage_id, f.path, f.is_dir, f.thumbnail_path
FROM files f
WHERE f.id IN (` + intersection + `)`)
	if storageID != nil {
		sb.WriteString(" AND f.storage_id = ?")
		args = append(args, *storageID)
	}
	sb.WriteString(" AND f.id > ? ORDER BY f.id LIMIT ?")
	args = append(args, cursor, limit+1)

	rows, err := s.db.Read.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rf ResultFile
		if err := rows.Scan(&rf.ID, &rf.StorageID, &rf.Path, &rf.IsDir, &rf.ThumbnailPath); err != nil {
			return nil, 0, err
		}
		files = append(files, rf)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(files) > limit {
		next = files[limit-1].ID
		files = files[:limit]
	}
	return files, next, nil
}

// valueIDs expands a filter to its matching field_value ids: the exact node for
// strict, or the node plus all descendants (Materialized Path) for fuzzy.
func (s *Store) valueIDs(ctx context.Context, f Filter) ([]int64, error) {
	if f.Strict {
		return []int64{f.FieldValueID}, nil
	}
	var path string
	err := s.db.Read.QueryRowContext(ctx, `SELECT path FROM field_values WHERE id=?`, f.FieldValueID).Scan(&path)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Read.QueryContext(ctx,
		`SELECT id FROM field_values WHERE field_type_id=? AND path LIKE ? || '%'`, f.FieldTypeID, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SearchValues finds field values whose value OR any alias matches the keyword
// (§5.5). Queries of 3+ characters use the FTS5 trigram index; shorter queries
// fall back to a LIKE scan (the trigram tokenizer needs >= 3 characters).
func (s *Store) SearchValues(ctx context.Context, keyword string, fieldTypeID *int64, limit int) ([]models.FieldValue, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if strings.TrimSpace(keyword) == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(keyword) >= 3 {
		return s.searchValuesFTS(ctx, keyword, fieldTypeID, limit)
	}
	return s.searchValuesLike(ctx, keyword, fieldTypeID, limit)
}

// searchValuesFTS queries the FTS5 trigram index. The keyword is wrapped as a
// quoted phrase so FTS5 operators in user input can't alter the query.
func (s *Store) searchValuesFTS(ctx context.Context, keyword string, fieldTypeID *int64, limit int) ([]models.FieldValue, error) {
	match := `"` + strings.ReplaceAll(keyword, `"`, `""`) + `"`
	args := []any{match}
	q := `SELECT DISTINCT fv.id, fv.field_type_id, fv.parent_id, fv.value, fv.path
		FROM field_value_fts f
		JOIN field_values fv ON fv.id = f.rowid
		WHERE f.text MATCH ?`
	if fieldTypeID != nil {
		q += " AND fv.field_type_id = ?"
		args = append(args, *fieldTypeID)
	}
	q += " ORDER BY fv.value LIMIT ?"
	args = append(args, limit)
	return s.scanValues(ctx, q, args...)
}

// searchValuesLike is the substring fallback for queries shorter than 3 chars.
func (s *Store) searchValuesLike(ctx context.Context, keyword string, fieldTypeID *int64, limit int) ([]models.FieldValue, error) {
	like := "%" + escapeLike(keyword) + "%"
	args := []any{like, like}
	q := `SELECT DISTINCT fv.id, fv.field_type_id, fv.parent_id, fv.value, fv.path
		FROM field_values fv
		LEFT JOIN field_value_aliases a ON a.field_value_id = fv.id
		WHERE (fv.value LIKE ? ESCAPE '\' OR a.alias LIKE ? ESCAPE '\')`
	if fieldTypeID != nil {
		q += " AND fv.field_type_id = ?"
		args = append(args, *fieldTypeID)
	}
	q += " ORDER BY fv.value LIMIT ?"
	args = append(args, limit)
	return s.scanValues(ctx, q, args...)
}

func (s *Store) scanValues(ctx context.Context, q string, args ...any) ([]models.FieldValue, error) {
	rows, err := s.db.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.FieldValue
	for rows.Next() {
		var v models.FieldValue
		if err := rows.Scan(&v.ID, &v.FieldTypeID, &v.ParentID, &v.Value, &v.Path); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// escapeLike escapes LIKE wildcards in user input.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
