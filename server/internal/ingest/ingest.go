// Package ingest consumes the scanner's NDJSON (§3.4) as the sole SQLite writer
// (§7.1): batched UPSERTs into files, then a missing-sweep scoped to the scanned
// path prefix so re-scanning a subtree never mis-flags rows outside it.
package ingest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/db"
)

// Record is one line of scanner NDJSON.
type Record struct {
	Path         string  `json:"path"`
	IsDir        bool    `json:"is_dir"`
	SizeBytes    *int64  `json:"size_bytes"`
	ModifiedAtFS *string `json:"modified_at_fs"`
	CreatedAtFS  *string `json:"created_at_fs"`
}

// Ingester writes scan results into the database.
type Ingester struct {
	db        *db.DB
	BatchSize int
}

// New builds an Ingester.
func New(d *db.DB) *Ingester { return &Ingester{db: d, BatchSize: 1000} }

// Ingest reads NDJSON from r and upserts files for storageID. pathPrefix is
// prepended to each record's (root-relative) path so a subtree scan maps back to
// storage-root-relative paths; it is also the sweep scope. Pass "" for a full
// scan. Returns the number of records processed.
func (in *Ingester) Ingest(ctx context.Context, storageID int64, pathPrefix string, r io.Reader, onProgress func(int64)) (int64, error) {
	scanStart := time.Now().UTC().Format(time.RFC3339Nano)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // tolerate long path lines

	var processed int64
	batch := in.BatchSize
	if batch <= 0 {
		batch = 1000
	}

	tx, err := in.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	inBatch := 0
	const upsert = `INSERT INTO files(storage_id, path, is_dir, size_bytes, modified_at_fs, created_at_fs, last_scanned, path_status)
		VALUES (?,?,?,?,?,?,?, 'ok')
		ON CONFLICT(storage_id, path) DO UPDATE SET
			is_dir=excluded.is_dir, size_bytes=excluded.size_bytes,
			modified_at_fs=excluded.modified_at_fs, created_at_fs=excluded.created_at_fs,
			last_scanned=excluded.last_scanned, path_status='ok'`

	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			_ = tx.Rollback()
			return processed, err
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			_ = tx.Rollback()
			return processed, fmt.Errorf("ingest: bad NDJSON: %w", err)
		}
		if _, err := tx.ExecContext(ctx, upsert,
			storageID, pathPrefix+rec.Path, rec.IsDir, rec.SizeBytes, rec.ModifiedAtFS, rec.CreatedAtFS, scanStart); err != nil {
			_ = tx.Rollback()
			return processed, err
		}
		processed++
		inBatch++
		if inBatch >= batch {
			if err := tx.Commit(); err != nil {
				return processed, err
			}
			if onProgress != nil {
				onProgress(processed)
			}
			if tx, err = in.db.Write.BeginTx(ctx, nil); err != nil {
				return processed, err
			}
			inBatch = 0
		}
	}
	if err := sc.Err(); err != nil {
		_ = tx.Rollback()
		return processed, err
	}
	if err := tx.Commit(); err != nil {
		return processed, err
	}

	// Missing-sweep, scoped to the scan prefix (§3.4). Rows never scanned
	// (last_scanned NULL, e.g. tag-only) are left untouched.
	if _, err := in.db.Write.ExecContext(ctx,
		`UPDATE files SET path_status='missing'
		 WHERE storage_id=? AND last_scanned IS NOT NULL AND last_scanned < ? AND path LIKE ? || '%'`,
		storageID, scanStart, pathPrefix); err != nil {
		return processed, err
	}
	if onProgress != nil {
		onProgress(processed)
	}
	return processed, nil
}

// RunScanner executes the scanner binary on absRoot and ingests its output.
func (in *Ingester) RunScanner(ctx context.Context, scannerBin, absRoot string, storageID int64, pathPrefix string, onProgress func(int64)) (int64, error) {
	cmd := exec.CommandContext(ctx, scannerBin, "scan", "--root", absRoot)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("ingest: start scanner: %w", err)
	}
	processed, ingestErr := in.Ingest(ctx, storageID, pathPrefix, stdout, onProgress)
	waitErr := cmd.Wait()
	if ingestErr != nil {
		return processed, ingestErr
	}
	if waitErr != nil {
		return processed, fmt.Errorf("ingest: scanner exited: %w", waitErr)
	}
	return processed, nil
}
