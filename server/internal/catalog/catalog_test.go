package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/storage"
)

func setup(t *testing.T) (*Store, int64, string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Projects", "GameA"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Projects", "GameA", "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := New(d)
	sid, err := st.CreateStorage(context.Background(), "main", "local", root)
	if err != nil {
		t.Fatal(err)
	}
	return st, sid, root
}

func TestListChildrenMaterializeRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, sid, _ := setup(t)

	// Nothing remembered yet: all IDs nil.
	kids, err := st.ListChildren(ctx, sid, "Projects")
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 1 || kids[0].Path != "Projects/GameA/" {
		t.Fatalf("children = %+v", kids)
	}
	if kids[0].ID != nil {
		t.Fatal("unmaterialized child should have nil ID")
	}

	// Materialize the folder, then it should carry an ID on next listing.
	fi := kids[0].FileInfo
	id, err := st.Materialize(ctx, sid, fi)
	if err != nil || id == 0 {
		t.Fatalf("materialize: id=%d err=%v", id, err)
	}
	// Idempotent: same row.
	id2, err := st.Materialize(ctx, sid, fi)
	if err != nil || id2 != id {
		t.Fatalf("materialize not idempotent: %d vs %d (%v)", id, id2, err)
	}

	kids, _ = st.ListChildren(ctx, sid, "Projects")
	if kids[0].ID == nil || *kids[0].ID != id {
		t.Fatalf("materialized child missing ID: %+v", kids[0])
	}

	f, err := st.GetFileByPath(ctx, sid, "Projects/GameA/")
	if err != nil || !f.IsDir {
		t.Fatalf("GetFileByPath: %+v err=%v", f, err)
	}
}

func TestSetThumbnailMaterializes(t *testing.T) {
	ctx := context.Background()
	st, sid, _ := setup(t)
	fi := storage.FileInfo{Path: "Projects/GameA/readme.txt", IsDir: false, Size: 2}
	id, err := st.SetThumbnail(ctx, sid, fi, "thumbs/abc.webp")
	if err != nil || id == 0 {
		t.Fatalf("SetThumbnail: %v", err)
	}
	f, err := st.GetFile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if f.ThumbnailPath == nil || *f.ThumbnailPath != "thumbs/abc.webp" {
		t.Fatalf("thumbnail not set: %+v", f)
	}
}

func TestDeleteStorageCleansPermissions(t *testing.T) {
	ctx := context.Background()
	st, sid, _ := setup(t)
	fi := storage.FileInfo{Path: "Projects/GameA/", IsDir: true}
	fid, _ := st.Materialize(ctx, sid, fi)

	// Add multi-type permission rows that have no FK to clean up.
	if _, err := st.db.Write.ExecContext(ctx,
		`INSERT INTO permissions(principal_type,principal_id,resource_type,resource_id,can_read) VALUES ('user',1,'storage',?,1)`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Write.ExecContext(ctx,
		`INSERT INTO permissions(principal_type,principal_id,resource_type,resource_id,can_read) VALUES ('user',1,'file',?,1)`, fid); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteStorage(ctx, sid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.db.Read.QueryRowContext(ctx, `SELECT count(*) FROM permissions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("permissions not cleaned: %d remain", n)
	}
	if _, err := st.GetStorage(ctx, sid); err != ErrNotFound {
		t.Fatalf("storage still present: %v", err)
	}
}
