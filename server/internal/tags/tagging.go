package tags

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"

	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/storage"
)

// ApplyByFileID applies a value to an already-materialized file. For a single-
// value field (allow_multi=false) it first clears the existing value (§4.1).
func (s *Store) ApplyByFileID(ctx context.Context, fileID, ftID, fvID int64) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.applyTx(ctx, tx, fileID, ftID, fvID); err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyByPath applies a value to a file identified by path, lazily materializing
// its files row first (§5.9). fi is the live metadata from the provider.
func (s *Store) ApplyByPath(ctx context.Context, storageID int64, fi storage.FileInfo, ftID, fvID int64) (int64, error) {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	fileID, err := catalog.MaterializeTx(ctx, tx, storageID, fi)
	if err != nil {
		return 0, err
	}
	if err := s.applyTx(ctx, tx, fileID, ftID, fvID); err != nil {
		return 0, err
	}
	return fileID, tx.Commit()
}

func (s *Store) applyTx(ctx context.Context, tx *sql.Tx, fileID, ftID, fvID int64) error {
	var allowMulti bool
	if err := tx.QueryRowContext(ctx, `SELECT allow_multi FROM field_types WHERE id=?`, ftID).Scan(&allowMulti); err != nil {
		return err
	}
	if !allowMulti {
		if _, err := tx.ExecContext(ctx, `DELETE FROM file_fields WHERE file_id=? AND field_type_id=?`, fileID, ftID); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO file_fields(file_id, field_type_id, field_value_id) VALUES (?,?,?)
		 ON CONFLICT DO NOTHING`, fileID, ftID, fvID)
	return err
}

// RemoveField removes every value of one field type from a file (§8.6).
func (s *Store) RemoveField(ctx context.Context, fileID, ftID int64) error {
	_, err := s.db.Write.ExecContext(ctx, `DELETE FROM file_fields WHERE file_id=? AND field_type_id=?`, fileID, ftID)
	return err
}

// EffectiveField is one field type's value for a file: either applied directly
// or inherited from the nearest ancestor (§4.2, §5.6).
type EffectiveField struct {
	FieldTypeID   int64    `json:"field_type_id"`
	FieldTypeName string   `json:"field_type_name"`
	ValueID       int64    `json:"value_id"`
	Value         string   `json:"value"`
	SourcePath    string   `json:"source_path"`
	Inherited     bool     `json:"inherited"`
	Breadcrumb    []string `json:"breadcrumb"`
}

// EffectiveFields resolves a file's tags: for every field type it returns the
// nearest value found along the path's ancestors (longest path = nearest),
// marking whether it was inherited. Uses one batched query (§5.6).
func (s *Store) EffectiveFields(ctx context.Context, storageID int64, path string) ([]EffectiveField, error) {
	ancestors := AncestorPaths(path)
	if len(ancestors) == 0 {
		return nil, nil
	}
	ph := make([]string, len(ancestors))
	args := make([]any, 0, len(ancestors)+1)
	args = append(args, storageID)
	for i, a := range ancestors {
		ph[i] = "?"
		args = append(args, a)
	}
	q := `SELECT ff.field_type_id, ft.name, ff.field_value_id, fv.value, fv.path, f.path
		FROM file_fields ff
		JOIN files f ON f.id = ff.file_id
		JOIN field_values fv ON fv.id = ff.field_value_id
		JOIN field_types ft ON ft.id = ff.field_type_id
		WHERE f.storage_id=? AND f.path IN (` + strings.Join(ph, ",") + `)
		ORDER BY length(f.path) DESC`
	rows, err := s.db.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	seen := map[int64]bool{}
	var out []EffectiveField
	valuePaths := map[int64]string{} // field_value materialized path, for breadcrumb
	for rows.Next() {
		var ef EffectiveField
		var fvPath, srcPath string
		if err := rows.Scan(&ef.FieldTypeID, &ef.FieldTypeName, &ef.ValueID, &ef.Value, &fvPath, &srcPath); err != nil {
			return nil, err
		}
		if seen[ef.FieldTypeID] {
			continue // a nearer value already won for this field type
		}
		seen[ef.FieldTypeID] = true
		ef.SourcePath = srcPath
		ef.Inherited = srcPath != path
		valuePaths[ef.ValueID] = fvPath
		out = append(out, ef)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillBreadcrumbs(ctx, out, valuePaths); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FieldTypeID < out[j].FieldTypeID })
	return out, nil
}

// fillBreadcrumbs resolves each effective value's root→node label chain (§5.6)
// from its materialized path, in a single query over all referenced ids.
func (s *Store) fillBreadcrumbs(ctx context.Context, fields []EffectiveField, valuePaths map[int64]string) error {
	idSet := map[int64]bool{}
	for _, p := range valuePaths {
		for _, id := range parsePathIDs(p) {
			idSet[id] = true
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]any, 0, len(idSet))
	ph := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
		ph = append(ph, "?")
	}
	rows, err := s.db.Read.QueryContext(ctx,
		`SELECT id, value FROM field_values WHERE id IN (`+strings.Join(ph, ",")+`)`, ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	label := map[int64]string{}
	for rows.Next() {
		var id int64
		var val string
		if err := rows.Scan(&id, &val); err != nil {
			return err
		}
		label[id] = val
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range fields {
		crumb := []string{}
		for _, id := range parsePathIDs(valuePaths[fields[i].ValueID]) {
			if l, ok := label[id]; ok {
				crumb = append(crumb, l)
			}
		}
		fields[i].Breadcrumb = crumb
	}
	return nil
}

// AncestorPaths returns the target path plus all of its ancestor directory paths
// (each with a trailing slash), nearest first.
func AncestorPaths(path string) []string {
	if path == "" {
		return nil
	}
	out := []string{path}
	trimmed := strings.TrimSuffix(path, "/")
	for {
		idx := strings.LastIndex(trimmed, "/")
		if idx < 0 {
			break
		}
		dir := trimmed[:idx+1] // include trailing slash
		if dir != path {
			out = append(out, dir)
		}
		trimmed = trimmed[:idx]
	}
	return out
}

// parsePathIDs turns "/1/3/5/" into [1,3,5] (root → leaf).
func parsePathIDs(p string) []int64 {
	var ids []int64
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg == "" {
			continue
		}
		if n, err := strconv.ParseInt(seg, 10, 64); err == nil {
			ids = append(ids, n)
		}
	}
	return ids
}
