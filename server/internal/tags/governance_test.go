package tags

import (
	"context"
	"testing"

	"github.com/chococar-site/filetagsystem/server/internal/storage"
)

func TestBatchApplyAndRemove(t *testing.T) {
	ctx := context.Background()
	s, d := newStore(t)
	sid := mustStorage(t, d)
	ft, _ := s.CreateFieldType(ctx, "t", true)
	v1, _ := s.CreateFieldValue(ctx, ft, nil, "A")
	v2, _ := s.CreateFieldValue(ctx, ft, nil, "B")

	ops := []BatchOp{
		{StorageID: sid, FI: storage.FileInfo{Path: "x/", IsDir: true}, Apply: []FieldRef{{ft, v1.ID}, {ft, v2.ID}}},
		{StorageID: sid, FI: storage.FileInfo{Path: "y/", IsDir: true}, Apply: []FieldRef{{ft, v1.ID}}},
	}
	if err := s.Batch(ctx, ops); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.Read.QueryRowContext(ctx, `SELECT count(*) FROM file_fields`).Scan(&n)
	if n != 3 {
		t.Fatalf("after apply, file_fields = %d, want 3", n)
	}

	// Remove field type t from x/ → drops its two values.
	if err := s.Batch(ctx, []BatchOp{{StorageID: sid, FI: storage.FileInfo{Path: "x/", IsDir: true}, Remove: []int64{ft}}}); err != nil {
		t.Fatal(err)
	}
	_ = d.Read.QueryRowContext(ctx, `SELECT count(*) FROM file_fields`).Scan(&n)
	if n != 1 {
		t.Fatalf("after remove, file_fields = %d, want 1", n)
	}
}

func TestMergeAndUsageAndUnused(t *testing.T) {
	ctx := context.Background()
	s, d := newStore(t)
	sid := mustStorage(t, d)
	ft, _ := s.CreateFieldType(ctx, "t", true)
	a, _ := s.CreateFieldValue(ctx, ft, nil, "Apple")
	b, _ := s.CreateFieldValue(ctx, ft, nil, "Banana")
	unused, _ := s.CreateFieldValue(ctx, ft, nil, "Cherry")

	// Apply A to x/ and y/, B to y/.
	if err := s.Batch(ctx, []BatchOp{
		{StorageID: sid, FI: storage.FileInfo{Path: "x/", IsDir: true}, Apply: []FieldRef{{ft, a.ID}}},
		{StorageID: sid, FI: storage.FileInfo{Path: "y/", IsDir: true}, Apply: []FieldRef{{ft, a.ID}, {ft, b.ID}}},
	}); err != nil {
		t.Fatal(err)
	}

	direct, _, err := s.ValueUsage(ctx, a.ID)
	if err != nil || direct != 2 {
		t.Fatalf("A usage direct=%d err=%v, want 2", direct, err)
	}

	// Merge A into B: y/ already has B (dedup), x/ moves to B → B used by x/ and y/.
	if err := s.MergeFieldValue(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFieldValue(ctx, a.ID); err != ErrNotFound {
		t.Fatalf("A should be gone: %v", err)
	}
	directB, _, _ := s.ValueUsage(ctx, b.ID)
	if directB != 2 {
		t.Fatalf("B usage after merge = %d, want 2", directB)
	}
	// A's label became an alias of B.
	aliases, _ := s.ListAliases(ctx, b.ID)
	found := false
	for _, al := range aliases {
		if al == "Apple" {
			found = true
		}
	}
	if !found {
		t.Fatalf("merged label not added as alias: %v", aliases)
	}

	// Unused values: Cherry only (A merged away, B used).
	un, _ := s.UnusedValues(ctx, ft)
	if len(un) != 1 || un[0].ID != unused.ID {
		t.Fatalf("unused = %+v, want [Cherry]", un)
	}
}

func TestMergeRejectsNodeWithChildren(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	ft, _ := s.CreateFieldType(ctx, "t", false)
	parent, _ := s.CreateFieldValue(ctx, ft, nil, "P")
	_, _ = s.CreateFieldValue(ctx, ft, &parent.ID, "C")
	other, _ := s.CreateFieldValue(ctx, ft, nil, "O")
	if err := s.MergeFieldValue(ctx, parent.ID, other.ID); err != ErrHasChildren {
		t.Fatalf("merge node with children = %v, want ErrHasChildren", err)
	}
}
