// Package tags implements the user-defined tagging system (§5): field types,
// field-value trees stored as Materialized Paths (§5.4), aliases, tag
// application with lazy materialize (§5.9), and path-based inheritance (§4.2).
package tags

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/models"
)

var (
	// ErrNotFound is returned when a field type/value is missing.
	ErrNotFound = errors.New("tags: not found")
	// ErrCycle is returned when a move would put a node inside its own subtree.
	ErrCycle = errors.New("tags: cannot move a node into its own subtree")
	// ErrFieldTypeMismatch is returned when moving across field types.
	ErrFieldTypeMismatch = errors.New("tags: parent is in a different field type")
)

// Store is the tag repository.
type Store struct {
	db *db.DB
}

// New builds a Store.
func New(d *db.DB) *Store { return &Store{db: d} }

// --- field types -------------------------------------------------------------

// CreateFieldType creates a tag dimension.
func (s *Store) CreateFieldType(ctx context.Context, name string, allowMulti bool) (int64, error) {
	res, err := s.db.Write.ExecContext(ctx,
		`INSERT INTO field_types(name, allow_multi) VALUES (?,?)`, name, allowMulti)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListFieldTypes returns all field types.
func (s *Store) ListFieldTypes(ctx context.Context) ([]models.FieldType, error) {
	rows, err := s.db.Read.QueryContext(ctx, `SELECT id, name, allow_multi FROM field_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.FieldType
	for rows.Next() {
		var ft models.FieldType
		if err := rows.Scan(&ft.ID, &ft.Name, &ft.AllowMulti); err != nil {
			return nil, err
		}
		out = append(out, ft)
	}
	return out, rows.Err()
}

// GetFieldType returns one field type.
func (s *Store) GetFieldType(ctx context.Context, id int64) (models.FieldType, error) {
	var ft models.FieldType
	err := s.db.Read.QueryRowContext(ctx, `SELECT id, name, allow_multi FROM field_types WHERE id=?`, id).
		Scan(&ft.ID, &ft.Name, &ft.AllowMulti)
	if errors.Is(err, sql.ErrNoRows) {
		return ft, ErrNotFound
	}
	return ft, err
}

// UpdateFieldType renames a field type and/or toggles allow_multi. When turning
// allow_multi OFF, it verifies no file already has multiple values of this type.
func (s *Store) UpdateFieldType(ctx context.Context, id int64, name string, allowMulti bool) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if !allowMulti {
		var dupes int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM (
				SELECT file_id FROM file_fields WHERE field_type_id=? GROUP BY file_id HAVING count(*) > 1
			)`, id).Scan(&dupes); err != nil {
			return err
		}
		if dupes > 0 {
			return errors.New("tags: cannot set allow_multi=false while files have multiple values")
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE field_types SET name=?, allow_multi=? WHERE id=?`, name, allowMulti, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// DeleteFieldType removes a field type; values and applications cascade (§4.3).
func (s *Store) DeleteFieldType(ctx context.Context, id int64) error {
	res, err := s.db.Write.ExecContext(ctx, `DELETE FROM field_types WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- field values (materialized path) ----------------------------------------

// CreateFieldValue inserts a value under parentID (nil = root) and assigns its
// materialized path = parentPath + newID + "/".
func (s *Store) CreateFieldValue(ctx context.Context, ftID int64, parentID *int64, value string) (models.FieldValue, error) {
	var fv models.FieldValue
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return fv, err
	}
	defer func() { _ = tx.Rollback() }()

	parentPath := "/"
	if parentID != nil {
		var pft int64
		err := tx.QueryRowContext(ctx, `SELECT path, field_type_id FROM field_values WHERE id=?`, *parentID).
			Scan(&parentPath, &pft)
		if errors.Is(err, sql.ErrNoRows) {
			return fv, ErrNotFound
		}
		if err != nil {
			return fv, err
		}
		if pft != ftID {
			return fv, ErrFieldTypeMismatch
		}
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO field_values(field_type_id, parent_id, value, path) VALUES (?,?,?,'')`, ftID, parentID, value)
	if err != nil {
		return fv, err
	}
	id, _ := res.LastInsertId()
	path := parentPath + strconv.FormatInt(id, 10) + "/"
	if _, err := tx.ExecContext(ctx, `UPDATE field_values SET path=? WHERE id=?`, path, id); err != nil {
		return fv, err
	}
	if err := tx.Commit(); err != nil {
		return fv, err
	}
	return models.FieldValue{ID: id, FieldTypeID: ftID, ParentID: parentID, Value: value, Path: path}, nil
}

// ListRootValues lists the top-level values of a field type.
func (s *Store) ListRootValues(ctx context.Context, ftID int64) ([]models.FieldValue, error) {
	return s.queryValues(ctx, `SELECT id, field_type_id, parent_id, value, path FROM field_values
		WHERE field_type_id=? AND parent_id IS NULL ORDER BY value`, ftID)
}

// ListChildValues lists the direct children of a value.
func (s *Store) ListChildValues(ctx context.Context, parentID int64) ([]models.FieldValue, error) {
	return s.queryValues(ctx, `SELECT id, field_type_id, parent_id, value, path FROM field_values
		WHERE parent_id=? ORDER BY value`, parentID)
}

// GetFieldValue returns one value.
func (s *Store) GetFieldValue(ctx context.Context, id int64) (models.FieldValue, error) {
	vs, err := s.queryValues(ctx, `SELECT id, field_type_id, parent_id, value, path FROM field_values WHERE id=?`, id)
	if err != nil {
		return models.FieldValue{}, err
	}
	if len(vs) == 0 {
		return models.FieldValue{}, ErrNotFound
	}
	return vs[0], nil
}

// RenameFieldValue changes a value's label (its tree position is unchanged).
func (s *Store) RenameFieldValue(ctx context.Context, id int64, value string) error {
	res, err := s.db.Write.ExecContext(ctx, `UPDATE field_values SET value=? WHERE id=?`, value, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveFieldValue reparents a value, rewriting the whole subtree's paths in one
// UPDATE (§5.4) after a cycle check.
func (s *Store) MoveFieldValue(ctx context.Context, id int64, newParentID *int64) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var oldPath string
	var ftID int64
	err = tx.QueryRowContext(ctx, `SELECT path, field_type_id FROM field_values WHERE id=?`, id).Scan(&oldPath, &ftID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	newParentPath := "/"
	if newParentID != nil {
		var pft int64
		err := tx.QueryRowContext(ctx, `SELECT path, field_type_id FROM field_values WHERE id=?`, *newParentID).
			Scan(&newParentPath, &pft)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if pft != ftID {
			return ErrFieldTypeMismatch
		}
	}
	newPath := newParentPath + strconv.FormatInt(id, 10) + "/"
	if newPath == oldPath {
		return tx.Commit() // no-op
	}
	// New parent must not be the node itself or any descendant.
	if strings.HasPrefix(newParentPath, oldPath) {
		return ErrCycle
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE field_values SET path = ? || substr(path, ?) WHERE path LIKE ? || '%'`,
		newPath, len(oldPath)+1, oldPath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE field_values SET parent_id=? WHERE id=?`, newParentID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteFieldValue removes a value and its whole subtree via path prefix (§4.3);
// aliases and file_fields cascade.
func (s *Store) DeleteFieldValue(ctx context.Context, id int64) error {
	path, err := s.valuePath(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.db.Write.ExecContext(ctx, `DELETE FROM field_values WHERE path LIKE ? || '%'`, path)
	return err
}

// DeleteInfluence reports how many values and how many files would be affected
// by deleting a value's subtree — the dry-run for destructive deletes (§5.11).
func (s *Store) DeleteInfluence(ctx context.Context, id int64) (values, files int, err error) {
	path, err := s.valuePath(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	if err = s.db.Read.QueryRowContext(ctx,
		`SELECT count(*) FROM field_values WHERE path LIKE ? || '%'`, path).Scan(&values); err != nil {
		return 0, 0, err
	}
	err = s.db.Read.QueryRowContext(ctx,
		`SELECT count(DISTINCT file_id) FROM file_fields
		 WHERE field_value_id IN (SELECT id FROM field_values WHERE path LIKE ? || '%')`, path).Scan(&files)
	return values, files, err
}

func (s *Store) valuePath(ctx context.Context, id int64) (string, error) {
	var path string
	err := s.db.Read.QueryRowContext(ctx, `SELECT path FROM field_values WHERE id=?`, id).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return path, err
}

// --- aliases -----------------------------------------------------------------

// AddAlias adds an alias to a value.
func (s *Store) AddAlias(ctx context.Context, fvID int64, alias string) (int64, error) {
	res, err := s.db.Write.ExecContext(ctx,
		`INSERT INTO field_value_aliases(field_value_id, alias) VALUES (?,?)`, fvID, alias)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListAliases lists a value's aliases.
func (s *Store) ListAliases(ctx context.Context, fvID int64) ([]string, error) {
	rows, err := s.db.Read.QueryContext(ctx,
		`SELECT alias FROM field_value_aliases WHERE field_value_id=? ORDER BY alias`, fvID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAlias removes an alias by id.
func (s *Store) DeleteAlias(ctx context.Context, aliasID int64) error {
	_, err := s.db.Write.ExecContext(ctx, `DELETE FROM field_value_aliases WHERE id=?`, aliasID)
	return err
}

func (s *Store) queryValues(ctx context.Context, q string, args ...any) ([]models.FieldValue, error) {
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
