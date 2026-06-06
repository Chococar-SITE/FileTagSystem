package db

import (
	"context"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestMigrateAndPragmas(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()

	v, err := SchemaVersion(ctx, d.Write)
	if err != nil {
		t.Fatal(err)
	}
	if v < 1 {
		t.Fatalf("schema version = %d, want >= 1", v)
	}

	// foreign_keys must be ON for BOTH pools (per-connection PRAGMA).
	var fk int
	if err := d.Write.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("write foreign_keys = %d (err %v), want 1", fk, err)
	}
	if err := d.Read.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("read foreign_keys = %d (err %v), want 1", fk, err)
	}

	var jm string
	if err := d.Write.QueryRow("PRAGMA journal_mode").Scan(&jm); err != nil || jm != "wal" {
		t.Fatalf("journal_mode = %q (err %v), want wal", jm, err)
	}

	// The last table in the schema must exist — proves multi-statement migration ran fully.
	var n int
	if err := d.Write.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='audit_log'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit_log table missing (n=%d err=%v)", n, err)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	v1, _ := SchemaVersion(ctx, d.Write)
	if err := Migrate(ctx, d.Write); err != nil {
		t.Fatal(err)
	}
	v2, _ := SchemaVersion(ctx, d.Write)
	if v1 != v2 {
		t.Fatalf("re-migrate changed version %d -> %d", v1, v2)
	}
}

func TestForeignKeyEnforcementAndCascade(t *testing.T) {
	d := openTest(t)
	w := d.Write

	// FK enforced: a file referencing a missing storage must be rejected.
	if _, err := w.Exec(`INSERT INTO files(storage_id, path, is_dir) VALUES (999, 'x/', 1)`); err == nil {
		t.Fatal("insert with dangling storage_id should fail (foreign_keys ON)")
	}

	// Build a small graph and verify ON DELETE CASCADE reaches file_fields.
	res, err := w.Exec(`INSERT INTO storage_providers(name, type, root_path) VALUES ('s','local','/r')`)
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := res.LastInsertId()
	res, err = w.Exec(`INSERT INTO files(storage_id, path, is_dir) VALUES (?, 'a/', 1)`, sid)
	if err != nil {
		t.Fatal(err)
	}
	fid, _ := res.LastInsertId()
	res, _ = w.Exec(`INSERT INTO field_types(name) VALUES ('engine')`)
	ftid, _ := res.LastInsertId()
	res, _ = w.Exec(`INSERT INTO field_values(field_type_id, value, path) VALUES (?, 'A', '/1/')`, ftid)
	fvid, _ := res.LastInsertId()
	if _, err := w.Exec(`INSERT INTO file_fields(file_id, field_type_id, field_value_id) VALUES (?,?,?)`, fid, ftid, fvid); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Exec(`DELETE FROM storage_providers WHERE id = ?`, sid); err != nil {
		t.Fatal(err)
	}
	var cnt int
	if err := w.QueryRow(`SELECT count(*) FROM files WHERE storage_id = ?`, sid).Scan(&cnt); err != nil || cnt != 0 {
		t.Fatalf("files not cascaded (cnt=%d err=%v)", cnt, err)
	}
	if err := w.QueryRow(`SELECT count(*) FROM file_fields WHERE file_id = ?`, fid).Scan(&cnt); err != nil || cnt != 0 {
		t.Fatalf("file_fields not cascaded (cnt=%d err=%v)", cnt, err)
	}
}
