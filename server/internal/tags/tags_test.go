package tags

import (
	"context"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/storage"
)

func newStore(t *testing.T) (*Store, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return New(d), d
}

func TestFieldValueMaterializedPath(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	ft, _ := s.CreateFieldType(ctx, "engine", false)

	root, err := s.CreateFieldValue(ctx, ft, nil, "EngineA")
	if err != nil {
		t.Fatal(err)
	}
	if root.Path != "/"+itoa(root.ID)+"/" {
		t.Fatalf("root path = %q", root.Path)
	}
	child, err := s.CreateFieldValue(ctx, ft, &root.ID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	want := root.Path + itoa(child.ID) + "/"
	if child.Path != want {
		t.Fatalf("child path = %q, want %q", child.Path, want)
	}
}

func TestMoveAndCycleDetection(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	ft, _ := s.CreateFieldType(ctx, "t", false)
	a, _ := s.CreateFieldValue(ctx, ft, nil, "A")
	b, _ := s.CreateFieldValue(ctx, ft, &a.ID, "B")
	c, _ := s.CreateFieldValue(ctx, ft, &b.ID, "C")

	// Cannot move A under its descendant C.
	if err := s.MoveFieldValue(ctx, a.ID, &c.ID); err != ErrCycle {
		t.Fatalf("move into subtree = %v, want ErrCycle", err)
	}
	// Cannot move into itself.
	if err := s.MoveFieldValue(ctx, b.ID, &b.ID); err != ErrCycle {
		t.Fatalf("move into self = %v, want ErrCycle", err)
	}
	// Move B (and C) to root; subtree paths rewrite.
	if err := s.MoveFieldValue(ctx, b.ID, nil); err != nil {
		t.Fatal(err)
	}
	nb, _ := s.GetFieldValue(ctx, b.ID)
	nc, _ := s.GetFieldValue(ctx, c.ID)
	if nb.Path != "/"+itoa(b.ID)+"/" {
		t.Fatalf("B path after move = %q", nb.Path)
	}
	if nc.Path != nb.Path+itoa(c.ID)+"/" {
		t.Fatalf("C path after move = %q (B=%q)", nc.Path, nb.Path)
	}
}

func TestDeleteSubtreeAndInfluence(t *testing.T) {
	ctx := context.Background()
	s, d := newStore(t)
	ft, _ := s.CreateFieldType(ctx, "t", true)
	a, _ := s.CreateFieldValue(ctx, ft, nil, "A")
	b, _ := s.CreateFieldValue(ctx, ft, &a.ID, "B")

	// Two files tagged with B → deleting A's subtree affects both values and files.
	mkFile := func(p string) int64 {
		id, err := newCatalog(d).Materialize(ctx, mustStorage(t, d), storage.FileInfo{Path: p, IsDir: true})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	f1 := mkFile("x/")
	f2 := mkFile("y/")
	if err := s.ApplyByFileID(ctx, f1, ft, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyByFileID(ctx, f2, ft, b.ID); err != nil {
		t.Fatal(err)
	}

	vals, files, err := s.DeleteInfluence(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if vals != 2 || files != 2 {
		t.Fatalf("influence vals=%d files=%d, want 2/2", vals, files)
	}
	if err := s.DeleteFieldValue(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.Read.QueryRowContext(ctx, `SELECT count(*) FROM field_values`).Scan(&n)
	if n != 0 {
		t.Fatalf("subtree not deleted: %d values remain", n)
	}
	_ = d.Read.QueryRowContext(ctx, `SELECT count(*) FROM file_fields`).Scan(&n)
	if n != 0 {
		t.Fatalf("file_fields not cascaded: %d remain", n)
	}
}

func TestSingleValueReplaces(t *testing.T) {
	ctx := context.Background()
	s, d := newStore(t)
	ft, _ := s.CreateFieldType(ctx, "engine", false) // single-value
	v1, _ := s.CreateFieldValue(ctx, ft, nil, "A")
	v2, _ := s.CreateFieldValue(ctx, ft, nil, "B")
	fid, _ := newCatalog(d).Materialize(ctx, mustStorage(t, d), storage.FileInfo{Path: "f/", IsDir: true})

	if err := s.ApplyByFileID(ctx, fid, ft, v1.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyByFileID(ctx, fid, ft, v2.ID); err != nil {
		t.Fatal(err)
	}
	var cnt int
	var got int64
	_ = d.Read.QueryRowContext(ctx, `SELECT count(*), max(field_value_id) FROM file_fields WHERE file_id=? AND field_type_id=?`, fid, ft).Scan(&cnt, &got)
	if cnt != 1 || got != v2.ID {
		t.Fatalf("single-value field has cnt=%d value=%d, want 1/%d", cnt, got, v2.ID)
	}
}

func TestPathInheritanceAndOverride(t *testing.T) {
	ctx := context.Background()
	s, d := newStore(t)
	sid := mustStorage(t, d)
	ft, _ := s.CreateFieldType(ctx, "engine", false)
	ea, _ := s.CreateFieldValue(ctx, ft, nil, "EngineA")
	eb, _ := s.CreateFieldValue(ctx, ft, nil, "EngineB")

	// Tag is applied on the parent only; the child is NOT materialized.
	if _, err := s.ApplyByPath(ctx, sid, storage.FileInfo{Path: "Proj/", IsDir: true}, ft, ea.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.EffectiveFields(ctx, sid, "Proj/Sub/x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Value != "EngineA" || !got[0].Inherited || got[0].SourcePath != "Proj/" {
		t.Fatalf("inherited field = %+v", got)
	}

	// Override on the subfolder: nearest (EngineB) wins for descendants.
	if _, err := s.ApplyByPath(ctx, sid, storage.FileInfo{Path: "Proj/Sub/", IsDir: true}, ft, eb.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.EffectiveFields(ctx, sid, "Proj/Sub/x.txt")
	if len(got) != 1 || got[0].Value != "EngineB" {
		t.Fatalf("override field = %+v, want EngineB", got)
	}

	// At the source folder itself the tag is direct, not inherited.
	got, _ = s.EffectiveFields(ctx, sid, "Proj/")
	if len(got) != 1 || got[0].Inherited {
		t.Fatalf("direct field = %+v, want not inherited", got)
	}
}

func TestBreadcrumb(t *testing.T) {
	ctx := context.Background()
	s, d := newStore(t)
	sid := mustStorage(t, d)
	ft, _ := s.CreateFieldType(ctx, "work", false)
	mobile, _ := s.CreateFieldValue(ctx, ft, nil, "手機遊戲")
	ark, _ := s.CreateFieldValue(ctx, ft, &mobile.ID, "明日方舟")
	if _, err := s.ApplyByPath(ctx, sid, storage.FileInfo{Path: "Ark/", IsDir: true}, ft, ark.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.EffectiveFields(ctx, sid, "Ark/")
	if len(got) != 1 || !reflect.DeepEqual(got[0].Breadcrumb, []string{"手機遊戲", "明日方舟"}) {
		t.Fatalf("breadcrumb = %+v", got[0].Breadcrumb)
	}
}

func TestAncestorPaths(t *testing.T) {
	got := AncestorPaths("A/B/C/")
	want := []string{"A/B/C/", "A/B/", "A/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dir ancestors = %v, want %v", got, want)
	}
	got = AncestorPaths("A/B/x.txt")
	want = []string{"A/B/x.txt", "A/B/", "A/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("file ancestors = %v, want %v", got, want)
	}
	if AncestorPaths("") != nil {
		t.Fatal("empty path should yield nil")
	}
}

// --- helpers -----------------------------------------------------------------

func newCatalog(d *db.DB) *catalog.Store { return catalog.New(d) }

func mustStorage(t *testing.T, d *db.DB) int64 {
	t.Helper()
	id, err := catalog.New(d).CreateStorage(context.Background(), "s", "local", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
