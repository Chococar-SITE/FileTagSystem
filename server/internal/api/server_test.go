package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/chococar-site/filetagsystem/server/internal/api"
	"github.com/chococar-site/filetagsystem/server/internal/config"
	"github.com/chococar-site/filetagsystem/server/internal/crypto"
	"github.com/chococar-site/filetagsystem/server/internal/db"
)

const adminPass = "admin-pass-123"

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	root := t.TempDir()
	mustMk(t, filepath.Join(root, "GameA"))
	mustWrite(t, filepath.Join(root, "GameA", "readme.txt"), "hello world")

	key, _ := crypto.GenerateKey()
	kr, _ := crypto.NewKeyRing(key)
	cfg := &config.Config{
		AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour,
		LoginMaxFails: 5, LoginLockout: time.Minute,
		ScannerBin: "filetag-scanner", TextPreviewCap: 256 * 1024,
	}
	srv := api.NewServer(cfg, d, kr, []byte("0123456789abcdef0123456789abcdef"))
	if _, err := srv.Bootstrap(context.Background(), "admin", adminPass); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, root
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func do(t *testing.T, c *http.Client, method, url string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func TestEndToEndFlow(t *testing.T) {
	ts, root := newTestServer(t)
	c := newClient(t)
	base := ts.URL

	// Unauthenticated calls are rejected.
	if code, _ := do(t, c, "GET", base+"/api/auth/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("me before login = %d, want 401", code)
	}

	// Login as admin → cookies stored in the jar.
	code, body := do(t, c, "POST", base+"/api/auth/login", map[string]string{"username": "admin", "password": adminPass})
	if code != http.StatusOK {
		t.Fatalf("login = %d (%s)", code, body)
	}
	if code, _ := do(t, c, "GET", base+"/api/auth/me", nil); code != http.StatusOK {
		t.Fatalf("me after login = %d", code)
	}

	// Create a storage over the seeded directory.
	code, body = do(t, c, "POST", base+"/api/storages", map[string]string{"name": "main", "type": "local", "root_path": root})
	if code != http.StatusCreated {
		t.Fatalf("create storage = %d (%s)", code, body)
	}
	sid := int64(jsonNum(t, body, "id"))

	// Live listing shows GameA/ (not yet materialized → id null).
	code, body = do(t, c, "GET", base+"/api/files?storage_id="+itoa(sid)+"&path=", nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d (%s)", code, body)
	}
	var listing struct {
		Entries []struct {
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
			ID    *int64 `json:"id"`
		} `json:"entries"`
	}
	_ = json.Unmarshal(body, &listing)
	if len(listing.Entries) != 1 || listing.Entries[0].Path != "GameA/" || listing.Entries[0].ID != nil {
		t.Fatalf("listing = %+v", listing.Entries)
	}

	// Define a field type + value.
	code, body = do(t, c, "POST", base+"/api/field-types", map[string]any{"name": "engine", "allow_multi": false})
	if code != http.StatusCreated {
		t.Fatalf("create field-type = %d (%s)", code, body)
	}
	ftID := int64(jsonNum(t, body, "id"))
	code, body = do(t, c, "POST", base+"/api/field-values", map[string]any{"field_type_id": ftID, "value": "EngineA"})
	if code != http.StatusCreated {
		t.Fatalf("create field-value = %d (%s)", code, body)
	}
	fvID := int64(jsonNum(t, body, "id"))

	// Tag GameA/ by path (lazy materialize).
	code, body = do(t, c, "POST", base+"/api/storages/"+itoa(sid)+"/tags",
		map[string]any{"path": "GameA/", "field_type_id": ftID, "field_value_id": fvID})
	if code != http.StatusOK {
		t.Fatalf("tag by path = %d (%s)", code, body)
	}
	fileID := int64(jsonNum(t, body, "file_id"))

	// A child file inherits the tag via path (not directly tagged, not materialized).
	code, body = do(t, c, "POST", base+"/api/storages/"+itoa(sid)+"/tags",
		map[string]any{"path": "GameA/readme.txt", "field_type_id": ftID, "field_value_id": fvID})
	if code != http.StatusOK {
		t.Fatalf("tag file = %d (%s)", code, body)
	}
	readmeID := int64(jsonNum(t, body, "file_id"))

	// Read effective fields on the folder (direct).
	code, body = do(t, c, "GET", base+"/api/files/"+itoa(fileID)+"/fields", nil)
	if code != http.StatusOK {
		t.Fatalf("fields = %d (%s)", code, body)
	}
	var fields []struct {
		Value     string `json:"value"`
		Inherited bool   `json:"inherited"`
	}
	_ = json.Unmarshal(body, &fields)
	if len(fields) != 1 || fields[0].Value != "EngineA" || fields[0].Inherited {
		t.Fatalf("fields = %+v", fields)
	}

	// Search finds the tagged folder.
	code, body = do(t, c, "GET", base+"/api/search?filters[0][field_type_id]="+itoa(ftID)+"&filters[0][field_value_id]="+itoa(fvID), nil)
	if code != http.StatusOK {
		t.Fatalf("search = %d (%s)", code, body)
	}
	var sr struct {
		Results []struct {
			Path string `json:"path"`
		} `json:"results"`
	}
	_ = json.Unmarshal(body, &sr)
	if len(sr.Results) != 2 {
		t.Fatalf("search results = %+v, want 2", sr.Results)
	}

	// Preview + raw of the text file.
	code, body = do(t, c, "GET", base+"/api/files/"+itoa(readmeID)+"/preview", nil)
	if code != http.StatusOK || jsonStr(t, body, "kind") != "text" {
		t.Fatalf("preview = %d (%s)", code, body)
	}
	code, raw := do(t, c, "GET", base+"/api/files/"+itoa(readmeID)+"/raw", nil)
	if code != http.StatusOK || string(raw) != "hello world" {
		t.Fatalf("raw = %d (%q)", code, raw)
	}
}

func TestNonAdminForbidden(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := newClient(t)
	base := ts.URL
	do(t, admin, "POST", base+"/api/auth/login", map[string]string{"username": "admin", "password": adminPass})

	// Admin creates a plain user.
	code, _ := do(t, admin, "POST", base+"/api/users", map[string]any{"username": "bob", "email": "b@x.io", "password": "bobpass12"})
	if code != http.StatusCreated {
		t.Fatalf("create user = %d", code)
	}

	// Bob logs in (separate jar) and is forbidden from privileged actions.
	bob := newClient(t)
	if code, _ := do(t, bob, "POST", base+"/api/auth/login", map[string]string{"username": "bob", "password": "bobpass12"}); code != http.StatusOK {
		t.Fatalf("bob login = %d", code)
	}
	if code, _ := do(t, bob, "POST", base+"/api/storages", map[string]string{"name": "x", "type": "local", "root_path": "/tmp"}); code != http.StatusForbidden {
		t.Fatalf("bob create storage = %d, want 403", code)
	}
	if code, _ := do(t, bob, "GET", base+"/api/users", nil); code != http.StatusForbidden {
		t.Fatalf("bob list users = %d, want 403", code)
	}
}

// --- helpers -----------------------------------------------------------------

func mustMk(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func jsonNum(t *testing.T, body []byte, key string) float64 {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("json: %v (%s)", err, body)
	}
	n, ok := m[key].(float64)
	if !ok {
		t.Fatalf("key %q not a number in %s", key, body)
	}
	return n
}

func jsonStr(t *testing.T, body []byte, key string) string {
	t.Helper()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	s, _ := m[key].(string)
	return s
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
