package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestResolveBlocksSymlinkEscapeNonexistentFinal closes the symlinked-parent
// hardening: even when the FINAL path component does not exist (so EvalSymlinks
// can't resolve the full path), a symlinked parent directory pointing outside
// the root must be rejected.
func TestResolveBlocksSymlinkEscapeNonexistentFinal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	p, root := newTree(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.resolve("escape/does-not-exist.txt"); err != ErrPathEscape {
		t.Fatalf("symlinked-parent escape (non-existent final) = %v, want ErrPathEscape", err)
	}
}

// TestResolveAllowsNonexistentUnderRoot guards against a false positive: a normal
// not-yet-existing path under root must still resolve cleanly.
func TestResolveAllowsNonexistentUnderRoot(t *testing.T) {
	p, _ := newTree(t)
	if _, err := p.resolve("newdir/newfile.txt"); err != nil {
		t.Fatalf("legit non-existent path rejected: %v", err)
	}
}
