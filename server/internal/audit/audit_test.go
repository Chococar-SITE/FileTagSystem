package audit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chococar-site/filetagsystem/server/internal/db"
)

func TestLogWritesEntries(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	l := New(d)

	// audit_log.user_id references users(id), so create a real user.
	res, err := d.Write.ExecContext(ctx, `INSERT INTO users(username,email,password_hash) VALUES ('u','u@e.io','x')`)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	fileID := int64(9)
	l.Log(ctx, &uid, ActionLogin, "", nil, map[string]any{"ip": "1.2.3.4"})
	l.Log(ctx, nil, ActionTagApply, "file", &fileID, nil)

	var n int
	if err := d.Read.QueryRow(`SELECT count(*) FROM audit_log`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit count = %d (err %v), want 2", n, err)
	}

	var action, detail string
	var userID *int64
	if err := d.Read.QueryRow(`SELECT user_id, action, detail FROM audit_log ORDER BY id LIMIT 1`).
		Scan(&userID, &action, &detail); err != nil {
		t.Fatal(err)
	}
	if action != ActionLogin || userID == nil || *userID != uid {
		t.Fatalf("first entry action=%q user=%v", action, userID)
	}
	if !strings.Contains(detail, "1.2.3.4") {
		t.Fatalf("detail missing ip: %q", detail)
	}

	// user_id is nullable (the second entry).
	var nilUser *int64
	if err := d.Read.QueryRow(`SELECT user_id FROM audit_log WHERE action=?`, ActionTagApply).Scan(&nilUser); err != nil {
		t.Fatal(err)
	}
	if nilUser != nil {
		t.Fatalf("expected NULL user_id, got %v", *nilUser)
	}
}
