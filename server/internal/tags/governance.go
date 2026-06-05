package tags

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/storage"
)

// ErrHasChildren is returned when merging a value that still has children.
var ErrHasChildren = errors.New("tags: cannot merge a value that has children")

// FieldRef names a (field type, value) pair to apply.
type FieldRef struct {
	FieldTypeID  int64 `json:"field_type_id"`
	FieldValueID int64 `json:"field_value_id"`
}

// BatchOp is one target of a batch tag operation.
type BatchOp struct {
	StorageID int64
	FI        storage.FileInfo
	Apply     []FieldRef
	Remove    []int64 // field type ids to clear
}

// Batch applies/removes tags across many targets in a single transaction
// (§5.11), lazily materializing each target.
func (s *Store) Batch(ctx context.Context, ops []BatchOp) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, op := range ops {
		fileID, err := catalog.MaterializeTx(ctx, tx, op.StorageID, op.FI)
		if err != nil {
			return err
		}
		for _, ft := range op.Remove {
			if _, err := tx.ExecContext(ctx, `DELETE FROM file_fields WHERE file_id=? AND field_type_id=?`, fileID, ft); err != nil {
				return err
			}
		}
		for _, ap := range op.Apply {
			if err := s.applyTx(ctx, tx, fileID, ap.FieldTypeID, ap.FieldValueID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// MergeFieldValue folds value id into targetID: all applications of id are
// repointed to targetID (deduped), id's aliases (plus its own label) become
// aliases of targetID, then id is removed (§5.11). id must be a leaf.
func (s *Store) MergeFieldValue(ctx context.Context, id, targetID int64) error {
	if id == targetID {
		return errors.New("tags: cannot merge a value into itself")
	}
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var srcType int64
	if err := tx.QueryRowContext(ctx, `SELECT field_type_id FROM field_values WHERE id=?`, id).Scan(&srcType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var dstType int64
	if err := tx.QueryRowContext(ctx, `SELECT field_type_id FROM field_values WHERE id=?`, targetID).Scan(&dstType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if srcType != dstType {
		return ErrFieldTypeMismatch
	}
	var kids int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM field_values WHERE parent_id=?`, id).Scan(&kids); err != nil {
		return err
	}
	if kids > 0 {
		return ErrHasChildren
	}

	// Repoint applications (skip rows that would duplicate), drop leftovers.
	if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE file_fields SET field_value_id=? WHERE field_value_id=?`, targetID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_fields WHERE field_value_id=?`, id); err != nil {
		return err
	}
	// Move aliases and add the merged value's own label as an alias.
	if _, err := tx.ExecContext(ctx, `UPDATE field_value_aliases SET field_value_id=? WHERE field_value_id=?`, targetID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO field_value_aliases(field_value_id, alias) SELECT ?, value FROM field_values WHERE id=?`, targetID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM field_values WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ValueUsage returns how many files apply the value directly and across its
// whole subtree (§8.4).
func (s *Store) ValueUsage(ctx context.Context, id int64) (direct, subtree int, err error) {
	path, err := s.valuePath(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	if err = s.db.Read.QueryRowContext(ctx, `SELECT count(*) FROM file_fields WHERE field_value_id=?`, id).Scan(&direct); err != nil {
		return 0, 0, err
	}
	err = s.db.Read.QueryRowContext(ctx,
		`SELECT count(*) FROM file_fields WHERE field_value_id IN (SELECT id FROM field_values WHERE path LIKE ? || '%')`, path).
		Scan(&subtree)
	return direct, subtree, err
}

// UnusedValues lists values of a field type that no file applies directly (§8.4).
func (s *Store) UnusedValues(ctx context.Context, fieldTypeID int64) ([]models.FieldValue, error) {
	return s.queryValues(ctx,
		`SELECT id, field_type_id, parent_id, value, path FROM field_values
		 WHERE field_type_id=? AND id NOT IN (SELECT DISTINCT field_value_id FROM file_fields)
		 ORDER BY value`, fieldTypeID)
}
