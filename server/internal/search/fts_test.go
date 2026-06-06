package search

import "testing"

func TestSearchValuesFTS(t *testing.T) {
	e := setup(t)
	work, _ := e.tg.CreateFieldType(e.ctx, "work", false)
	ark, _ := e.tg.CreateFieldValue(e.ctx, work, nil, "明日方舟")
	char, _ := e.tg.CreateFieldType(e.ctx, "char", false)
	_, _ = e.tg.CreateFieldValue(e.ctx, char, nil, "鋼鐵雄心")
	aliasID, _ := e.tg.AddAlias(e.ctx, ark.ID, "Arknights")

	expectOne := func(label, kw string, ft *int64, wantID int64) {
		t.Helper()
		hits, err := e.se.SearchValues(e.ctx, kw, ft, 50)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if len(hits) != 1 || hits[0].ID != wantID {
			t.Fatalf("%s: got %+v, want single id=%d", label, hits, wantID)
		}
	}
	expectNone := func(label, kw string, ft *int64) {
		t.Helper()
		hits, err := e.se.SearchValues(e.ctx, kw, ft, 50)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if len(hits) != 0 {
			t.Fatalf("%s: got %+v, want none", label, hits)
		}
	}

	// 3-char CJK substring → FTS trigram index.
	expectOne("cjk substring (fts)", "明日方", nil, ark.ID)
	// Latin alias substring, case-insensitive (>=3 chars → fts).
	expectOne("alias substring (fts)", "rknigh", nil, ark.ID)
	// Field-type filter excludes the match.
	expectNone("field-type filter", "明日方", &char)
	// 2-char query falls back to LIKE and still works.
	expectOne("short query fallback (like)", "方舟", nil, ark.ID)

	// Rename rebuilds the index (trigger), keeping aliases too.
	if err := e.tg.RenameFieldValue(e.ctx, ark.ID, "明日方舟MZ"); err != nil {
		t.Fatal(err)
	}
	expectOne("after rename, new term", "方舟MZ", nil, ark.ID)
	expectOne("alias survives rename", "Arknights", nil, ark.ID)

	// Deleting an alias removes it from the index.
	if err := e.tg.DeleteAlias(e.ctx, aliasID); err != nil {
		t.Fatal(err)
	}
	expectNone("alias removed from fts", "Arknights", nil)
	expectOne("value still indexed", "明日方", nil, ark.ID)

	// Deleting the value removes it from the index.
	if err := e.tg.DeleteFieldValue(e.ctx, ark.ID); err != nil {
		t.Fatal(err)
	}
	expectNone("deleted value gone from fts", "明日方", nil)
}
