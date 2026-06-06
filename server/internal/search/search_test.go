package search

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/db"
	"github.com/chococar-site/filetagsystem/server/internal/storage"
	"github.com/chococar-site/filetagsystem/server/internal/tags"
)

type env struct {
	ctx context.Context
	sid int64
	tg  *tags.Store
	se  *Store
}

func setup(t *testing.T) *env {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	sid, err := catalog.New(d).CreateStorage(context.Background(), "s", "local", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &env{ctx: context.Background(), sid: sid, tg: tags.New(d), se: New(d)}
}

func (e *env) tag(t *testing.T, path string, ft, fv int64) {
	t.Helper()
	if _, err := e.tg.ApplyByPath(e.ctx, e.sid, storage.FileInfo{Path: path, IsDir: true}, ft, fv); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFuzzyStrictAndIntersection(t *testing.T) {
	e := setup(t)
	work, _ := e.tg.CreateFieldType(e.ctx, "work", false)
	mobile, _ := e.tg.CreateFieldValue(e.ctx, work, nil, "手機遊戲")
	ark, _ := e.tg.CreateFieldValue(e.ctx, work, &mobile.ID, "明日方舟")
	char, _ := e.tg.CreateFieldType(e.ctx, "char", true)
	kal, _ := e.tg.CreateFieldValue(e.ctx, char, nil, "凱爾希")

	e.tag(t, "ArkFolder/", work, ark.ID)
	e.tag(t, "ArkFolder/", char, kal.ID)
	e.tag(t, "OtherKal/", char, kal.ID)
	e.tag(t, "MobileGen/", work, mobile.ID)

	// Fuzzy work=手機遊戲 includes the descendant 明日方舟.
	res, _, err := e.se.Search(e.ctx, nil, []Filter{{FieldTypeID: work, FieldValueID: mobile.ID}}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("fuzzy work: got %d, want 2 (%+v)", len(res), res)
	}

	// Strict work=手機遊戲 matches only the exact node.
	res, _, _ = e.se.Search(e.ctx, nil, []Filter{{FieldTypeID: work, FieldValueID: mobile.ID, Strict: true}}, 50, 0)
	if len(res) != 1 || res[0].Path != "MobileGen/" {
		t.Fatalf("strict work: %+v", res)
	}

	// char=凱爾希 across works.
	res, _, _ = e.se.Search(e.ctx, nil, []Filter{{FieldTypeID: char, FieldValueID: kal.ID}}, 50, 0)
	if len(res) != 2 {
		t.Fatalf("char: got %d, want 2", len(res))
	}

	// AND: work=明日方舟 AND char=凱爾希 → only ArkFolder.
	res, _, _ = e.se.Search(e.ctx, nil, []Filter{
		{FieldTypeID: work, FieldValueID: ark.ID, Strict: true},
		{FieldTypeID: char, FieldValueID: kal.ID},
	}, 50, 0)
	if len(res) != 1 || res[0].Path != "ArkFolder/" {
		t.Fatalf("AND: %+v", res)
	}
}

func TestSearchPagination(t *testing.T) {
	e := setup(t)
	ft, _ := e.tg.CreateFieldType(e.ctx, "t", true)
	v, _ := e.tg.CreateFieldValue(e.ctx, ft, nil, "X")
	for _, p := range []string{"a/", "b/", "c/", "d/", "e/"} {
		e.tag(t, p, ft, v.ID)
	}
	res, next, err := e.se.Search(e.ctx, nil, []Filter{{FieldTypeID: ft, FieldValueID: v.ID}}, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || next == 0 {
		t.Fatalf("page 1: got %d next=%d", len(res), next)
	}
	res2, next2, _ := e.se.Search(e.ctx, nil, []Filter{{FieldTypeID: ft, FieldValueID: v.ID}}, 2, next)
	if len(res2) != 2 || next2 == 0 {
		t.Fatalf("page 2: got %d next=%d", len(res2), next2)
	}
	if res2[0].ID <= res[1].ID {
		t.Fatal("pagination did not advance")
	}
	res3, next3, _ := e.se.Search(e.ctx, nil, []Filter{{FieldTypeID: ft, FieldValueID: v.ID}}, 2, next2)
	if len(res3) != 1 || next3 != 0 {
		t.Fatalf("last page: got %d next=%d, want 1/0", len(res3), next3)
	}
}

func TestSearchValuesWithAlias(t *testing.T) {
	e := setup(t)
	work, _ := e.tg.CreateFieldType(e.ctx, "work", false)
	mobile, _ := e.tg.CreateFieldValue(e.ctx, work, nil, "手機遊戲")
	ark, _ := e.tg.CreateFieldValue(e.ctx, work, &mobile.ID, "明日方舟")
	if _, err := e.tg.AddAlias(e.ctx, ark.ID, "Arknights"); err != nil {
		t.Fatal(err)
	}

	hits, err := e.se.SearchValues(e.ctx, "方舟", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != ark.ID {
		t.Fatalf("value search: %+v", hits)
	}
	// Match via alias.
	hits, _ = e.se.SearchValues(e.ctx, "Arknights", nil, 50)
	if len(hits) != 1 || hits[0].ID != ark.ID {
		t.Fatalf("alias search: %+v", hits)
	}
	if _, _, err := e.se.Search(e.ctx, nil, nil, 50, 0); err != ErrNoFilters {
		t.Fatalf("no-filter search = %v, want ErrNoFilters", err)
	}
}
