// Package catalog manages storage sources and the files table — the "remembered"
// rows. Listings stay live (via the storage provider); a files row exists only
// when an item is scanned, tagged, given a thumbnail, or assigned a permission
// (§2.3, §5.9). Go is the sole writer (§7.1).
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/models"
	"github.com/chococar-site/filetagsystem/server/internal/storage"
)

// ErrNotFound is returned when a storage or file row does not exist.
var ErrNotFound = errors.New("catalog: not found")

// Store is the files/storage repository.
type Store struct {
	db *db.DB
}

// New builds a Store over an open database.
func New(d *db.DB) *Store { return &Store{db: d} }

// --- storage sources ---------------------------------------------------------

// CreateStorage inserts a storage source and returns its id.
func (s *Store) CreateStorage(ctx context.Context, name, typ, root string) (int64, error) {
	res, err := s.db.Write.ExecContext(ctx,
		`INSERT INTO storage_providers(name, type, root_path) VALUES (?,?,?)`, name, typ, root)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListStorages returns all storage sources.
func (s *Store) ListStorages(ctx context.Context) ([]models.StorageProvider, error) {
	rows, err := s.db.Read.QueryContext(ctx,
		`SELECT id, name, type, root_path, created_at FROM storage_providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.StorageProvider
	for rows.Next() {
		var p models.StorageProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.RootPath, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetStorage returns one storage source.
func (s *Store) GetStorage(ctx context.Context, id int64) (models.StorageProvider, error) {
	var p models.StorageProvider
	err := s.db.Read.QueryRowContext(ctx,
		`SELECT id, name, type, root_path, created_at FROM storage_providers WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &p.Type, &p.RootPath, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// UpdateStorageRoot changes the root path (§5.8). Relative file paths are kept.
func (s *Store) UpdateStorageRoot(ctx context.Context, id int64, root string) error {
	res, err := s.db.Write.ExecContext(ctx,
		`UPDATE storage_providers SET root_path=? WHERE id=?`, root, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStorage updates the name and root path of a storage source.
func (s *Store) UpdateStorage(ctx context.Context, id int64, name, root string) error {
	res, err := s.db.Write.ExecContext(ctx,
		`UPDATE storage_providers SET name=?, root_path=? WHERE id=?`, name, root, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteStorage removes a storage source. The files cascade via FK, but
// multi-type permission rows (resource_type in 'storage'/'file') have no FK and
// must be cleaned in the same transaction (§4.3).
func (s *Store) DeleteStorage(ctx context.Context, id int64) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM permissions WHERE resource_type='file' AND resource_id IN (SELECT id FROM files WHERE storage_id=?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM permissions WHERE resource_type='storage' AND resource_id=?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM storage_providers WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// Provider builds a live storage.Provider for a source.
func (s *Store) Provider(ctx context.Context, storageID int64) (storage.Provider, models.StorageProvider, error) {
	st, err := s.GetStorage(ctx, storageID)
	if err != nil {
		return nil, st, err
	}
	p, err := buildProvider(st)
	return p, st, err
}

func buildProvider(st models.StorageProvider) (storage.Provider, error) {
	switch st.Type {
	case "local":
		return storage.NewLocalProvider(st.RootPath)
	default:
		return nil, fmt.Errorf("catalog: unsupported storage type %q", st.Type)
	}
}

// --- files -------------------------------------------------------------------

// Materialize ensures a files row exists for (storageID, path), returning its id
// (§5.9). Used before tagging / setting a thumbnail / assigning a permission.
func (s *Store) Materialize(ctx context.Context, storageID int64, fi storage.FileInfo) (int64, error) {
	return materializeTx(ctx, s.db.Write, storageID, fi)
}

// MaterializeTx upserts a files row using an existing transaction, so callers
// (e.g. tagging) can keep materialize + write atomic in one tx (§5.11).
func MaterializeTx(ctx context.Context, tx *sql.Tx, storageID int64, fi storage.FileInfo) (int64, error) {
	return materializeTx(ctx, tx, storageID, fi)
}

// materializeTx runs the upsert on any executor (DB or Tx).
func materializeTx(ctx context.Context, ex execer, storageID int64, fi storage.FileInfo) (int64, error) {
	var size any
	if !fi.IsDir {
		size = fi.Size
	}
	var modified any
	if !fi.Modified.IsZero() {
		modified = fi.Modified.UTC().Format(time.RFC3339Nano)
	}
	if _, err := ex.ExecContext(ctx,
		`INSERT INTO files(storage_id, path, is_dir, size_bytes, modified_at_fs)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT(storage_id, path) DO NOTHING`,
		storageID, fi.Path, fi.IsDir, size, modified); err != nil {
		return 0, err
	}
	var id int64
	err := ex.QueryRowContext(ctx,
		`SELECT id FROM files WHERE storage_id=? AND path=?`, storageID, fi.Path).Scan(&id)
	return id, err
}

// GetFile returns a files row by id.
func (s *Store) GetFile(ctx context.Context, id int64) (models.File, error) {
	return scanFile(s.db.Read.QueryRowContext(ctx, filesSelect+` WHERE id=?`, id))
}

// GetFileByPath returns a files row by (storageID, path), or ErrNotFound.
func (s *Store) GetFileByPath(ctx context.Context, storageID int64, path string) (models.File, error) {
	return scanFile(s.db.Read.QueryRowContext(ctx, filesSelect+` WHERE storage_id=? AND path=?`, storageID, path))
}

// SetThumbnail materializes the row (if needed) and records the thumbnail path.
func (s *Store) SetThumbnail(ctx context.Context, storageID int64, fi storage.FileInfo, thumbPath string) (int64, error) {
	id, err := s.Materialize(ctx, storageID, fi)
	if err != nil {
		return 0, err
	}
	_, err = s.db.Write.ExecContext(ctx, `UPDATE files SET thumbnail_path=? WHERE id=?`, thumbPath, id)
	return id, err
}

// SetThumbnailByID records a thumbnail path for an existing files row.
func (s *Store) SetThumbnailByID(ctx context.Context, fileID int64, thumbPath string) error {
	res, err := s.db.Write.ExecContext(ctx, `UPDATE files SET thumbnail_path=? WHERE id=?`, thumbPath, fileID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Child is a live directory entry augmented with its files row, if any.
type Child struct {
	storage.FileInfo
	ID            *int64  `json:"id"`
	ThumbnailPath *string `json:"thumbnail_path,omitempty"`
}

// ListChildren lists a directory live and LEFT-JOINs the existing files rows in
// a single batch query (§5.9, §6.3.6) — cost is independent of child count.
func (s *Store) ListChildren(ctx context.Context, storageID int64, relPath string) ([]Child, error) {
	p, _, err := s.Provider(ctx, storageID)
	if err != nil {
		return nil, err
	}
	entries, err := p.ListDir(relPath)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	known, err := s.filesByPaths(ctx, storageID, paths)
	if err != nil {
		return nil, err
	}
	out := make([]Child, 0, len(entries))
	for _, e := range entries {
		c := Child{FileInfo: e}
		if row, ok := known[e.Path]; ok {
			id := row.id
			c.ID = &id
			c.ThumbnailPath = row.thumb
		}
		out = append(out, c)
	}
	return out, nil
}

type knownRow struct {
	id    int64
	thumb *string
}

func (s *Store) filesByPaths(ctx context.Context, storageID int64, paths []string) (map[string]knownRow, error) {
	res := make(map[string]knownRow, len(paths))
	if len(paths) == 0 {
		return res, nil
	}
	ph := make([]string, len(paths))
	args := make([]any, 0, len(paths)+1)
	args = append(args, storageID)
	for i, p := range paths {
		ph[i] = "?"
		args = append(args, p)
	}
	q := fmt.Sprintf(`SELECT id, path, thumbnail_path FROM files WHERE storage_id=? AND path IN (%s)`, strings.Join(ph, ","))
	rows, err := s.db.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var path string
		var thumb *string
		if err := rows.Scan(&id, &path, &thumb); err != nil {
			return nil, err
		}
		res[path] = knownRow{id: id, thumb: thumb}
	}
	return res, rows.Err()
}

const filesSelect = `SELECT id, storage_id, path, is_dir, thumbnail_path, path_status,
	size_bytes, created_at_fs, modified_at_fs FROM files`

type rowScanner interface{ Scan(...any) error }

func scanFile(r rowScanner) (models.File, error) {
	var f models.File
	var createdAt, modifiedAt sql.NullString
	err := r.Scan(&f.ID, &f.StorageID, &f.Path, &f.IsDir, &f.ThumbnailPath, &f.PathStatus,
		&f.SizeBytes, &createdAt, &modifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	if err != nil {
		return f, err
	}
	f.CreatedAtFS = parseTime(createdAt)
	f.ModifiedAtFS = parseTime(modifiedAt)
	return f, nil
}

func parseTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s.String); err == nil {
			return &t
		}
	}
	return nil
}

// execer is satisfied by *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
