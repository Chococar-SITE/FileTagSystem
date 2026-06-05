package storage

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func newTree(t *testing.T) (*LocalProvider, string) {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "Projects", "GameA"), 0o755))
	must(os.WriteFile(filepath.Join(root, "Projects", "GameA", "readme.txt"), []byte("hello!!"), 0o644))
	must(os.WriteFile(filepath.Join(root, "notes.txt"), []byte("note"), 0o644))
	p, err := NewLocalProvider(root)
	must(err)
	return p, root
}

func TestResolveWithinRoot(t *testing.T) {
	p, _ := newTree(t)
	if _, err := p.resolve("Projects/GameA/readme.txt"); err != nil {
		t.Fatalf("legit path rejected: %v", err)
	}
	if _, err := p.resolve(""); err != nil {
		t.Fatalf("root rejected: %v", err)
	}
}

func TestResolveNeutralizesTraversal(t *testing.T) {
	p, _ := newTree(t)
	// Per §7.3 the resolver NEUTRALIZES "../" (leading slash + Clean), so the
	// result is always contained within root rather than erroring.
	for _, in := range []string{
		"../../etc/passwd",
		"Projects/../../../etc/passwd",
		"..",
		"a/../../b",
	} {
		abs, err := p.resolve(in)
		if err != nil {
			t.Errorf("resolve(%q) errored: %v", in, err)
			continue
		}
		if !withinRoot(p.root, abs) {
			t.Errorf("resolve(%q) escaped root: %s", in, abs)
		}
	}
}

func TestResolveAbsoluteIsContained(t *testing.T) {
	p, _ := newTree(t)
	// A leading slash is treated as relative to root, not the FS root.
	abs, err := p.resolve("/etc/passwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !withinRoot(p.root, abs) {
		t.Fatalf("absolute-looking path escaped: %s", abs)
	}
}

func TestResolveRejectsNULAndControl(t *testing.T) {
	p, _ := newTree(t)
	if _, err := p.resolve("a\x00b"); err != ErrPathEscape {
		t.Errorf("NUL not rejected: %v", err)
	}
	if _, err := p.resolve("a\nb"); err != ErrPathEscape {
		t.Errorf("control char not rejected: %v", err)
	}
}

func TestResolveBlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	p, root := newTree(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.resolve("escape/secret.txt"); err != ErrPathEscape {
		t.Fatalf("symlink escape not blocked: %v", err)
	}
}

func TestWithinRootSiblingPrefix(t *testing.T) {
	// "/data/root-evil" must NOT be considered inside "/data/root".
	if withinRoot("/data/root", "/data/root-evil") {
		t.Fatal("sibling-prefix path wrongly considered within root")
	}
	if !withinRoot("/data/root", "/data/root/a") {
		t.Fatal("real child wrongly rejected")
	}
	if !withinRoot("/data/root", "/data/root") {
		t.Fatal("root itself should be within root")
	}
}

func TestStatAndListDir(t *testing.T) {
	p, _ := newTree(t)

	dir, err := p.Stat("Projects/GameA")
	if err != nil {
		t.Fatal(err)
	}
	if !dir.IsDir || dir.Path != "Projects/GameA/" {
		t.Fatalf("dir stat = %+v; want trailing-slash dir", dir)
	}
	if dir.Size != 0 {
		t.Fatalf("dir size = %d, want 0", dir.Size)
	}

	entries, err := p.ListDir("Projects/GameA")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "Projects/GameA/readme.txt" {
		t.Fatalf("listing = %+v", entries)
	}
	if entries[0].IsDir || entries[0].Size != int64(len("hello!!")) {
		t.Fatalf("file entry wrong: %+v", entries[0])
	}

	// Root listing: dirs first, correct relative paths.
	rootEntries, err := p.ListDir("")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootEntries) != 2 || !rootEntries[0].IsDir || rootEntries[0].Path != "Projects/" {
		t.Fatalf("root listing = %+v", rootEntries)
	}
	if rootEntries[1].Path != "notes.txt" {
		t.Fatalf("root file entry = %+v", rootEntries[1])
	}
}

func TestOpenReadsFileAndRejectsDir(t *testing.T) {
	p, _ := newTree(t)
	r, info, err := p.Open("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	b, _ := io.ReadAll(r)
	if string(b) != "note" || info.Size != 4 {
		t.Fatalf("read %q size %d", b, info.Size)
	}
	if _, _, err := p.Open("Projects"); err != ErrIsDir {
		t.Fatalf("opening dir = %v, want ErrIsDir", err)
	}
	if _, _, err := p.Open("nope.txt"); err != ErrNotExist {
		t.Fatalf("opening missing = %v, want ErrNotExist", err)
	}
}
