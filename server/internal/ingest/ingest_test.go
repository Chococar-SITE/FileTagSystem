package ingest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/catalog"
	"github.com/chococar-site/filetagsystem/server/internal/db"
)

func setup(t *testing.T) (*Ingester, *db.DB, int64) {
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
	return New(d), d, sid
}

func countStatus(t *testing.T, d *db.DB, sid int64, status string) int {
	t.Helper()
	var n int
	if err := d.Read.QueryRow(`SELECT count(*) FROM files WHERE storage_id=? AND path_status=?`, sid, status).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIngestUpsertAndMissingSweep(t *testing.T) {
	ctx := context.Background()
	in, d, sid := setup(t)

	nd := `{"path":"a/","is_dir":true,"size_bytes":null,"modified_at_fs":"2026-01-02T03:04:05Z","created_at_fs":null}
{"path":"a/file.txt","is_dir":false,"size_bytes":12,"modified_at_fs":"2026-01-02T03:04:05Z","created_at_fs":null}
{"path":"b/","is_dir":true,"size_bytes":null,"modified_at_fs":null,"created_at_fs":null}
`
	n, err := in.Ingest(ctx, sid, "", strings.NewReader(nd), nil)
	if err != nil || n != 3 {
		t.Fatalf("first ingest n=%d err=%v", n, err)
	}
	if countStatus(t, d, sid, "ok") != 3 {
		t.Fatalf("want 3 ok rows")
	}
	// Verify a file's size and a dir's null size came through.
	var size *int64
	_ = d.Read.QueryRow(`SELECT size_bytes FROM files WHERE storage_id=? AND path='a/file.txt'`, sid).Scan(&size)
	if size == nil || *size != 12 {
		t.Fatalf("file size = %v, want 12", size)
	}

	time.Sleep(2 * time.Millisecond) // ensure a strictly later scan timestamp

	// Re-scan without b/ → b/ becomes missing; a/* stay ok.
	nd2 := `{"path":"a/","is_dir":true,"size_bytes":null,"modified_at_fs":"2026-01-03T00:00:00Z","created_at_fs":null}
{"path":"a/file.txt","is_dir":false,"size_bytes":99,"modified_at_fs":"2026-01-03T00:00:00Z","created_at_fs":null}
`
	if _, err := in.Ingest(ctx, sid, "", strings.NewReader(nd2), nil); err != nil {
		t.Fatal(err)
	}
	if got := countStatus(t, d, sid, "missing"); got != 1 {
		t.Fatalf("missing rows = %d, want 1", got)
	}
	if got := countStatus(t, d, sid, "ok"); got != 2 {
		t.Fatalf("ok rows = %d, want 2", got)
	}
	var status string
	_ = d.Read.QueryRow(`SELECT path_status FROM files WHERE storage_id=? AND path='b/'`, sid).Scan(&status)
	if status != "missing" {
		t.Fatalf("b/ status = %q, want missing", status)
	}
}

func TestIngestPrefixScoping(t *testing.T) {
	ctx := context.Background()
	in, d, sid := setup(t)
	nd := `{"path":"x/","is_dir":true,"size_bytes":null,"modified_at_fs":null,"created_at_fs":null}
{"path":"y.txt","is_dir":false,"size_bytes":3,"modified_at_fs":null,"created_at_fs":null}
`
	if _, err := in.Ingest(ctx, sid, "Sub/", strings.NewReader(nd), nil); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.Read.QueryRow(`SELECT count(*) FROM files WHERE storage_id=? AND path IN ('Sub/x/','Sub/y.txt')`, sid).Scan(&n)
	if n != 2 {
		t.Fatalf("prefixed paths = %d, want 2", n)
	}
}

func TestJobManager(t *testing.T) {
	jm := NewJobManager()
	done := make(chan struct{})
	err := jm.Start(1, func(ctx context.Context, progress func(int64)) (int64, error) {
		progress(5)
		close(done)
		return 5, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-done
	// Allow finish() to run.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if st, ok := jm.Status(1); ok && st.State == StateDone {
			if st.Processed != 5 {
				t.Fatalf("processed = %d", st.Processed)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not reach done state")
}
