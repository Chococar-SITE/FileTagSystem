package storage

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LocalProvider serves files from a local directory subtree.
type LocalProvider struct {
	root     string // absolute, cleaned
	realRoot string // symlink-resolved root, for containment checks
}

// NewLocalProvider creates a provider rooted at root.
func NewLocalProvider(root string) (*LocalProvider, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		real = abs
	}
	return &LocalProvider{root: abs, realRoot: real}, nil
}

// Type implements Provider.
func (p *LocalProvider) Type() string { return "local" }

// Root implements Provider.
func (p *LocalProvider) Root() string { return p.root }

// resolve validates a relative path and returns the absolute on-disk path.
// Defense layers (§7.3): reject NUL/control chars and volume names, neutralize
// ".." with a leading slash + Clean, require lexical containment, and require
// symlink-resolved containment so an in-root symlink can't point out.
func (p *LocalProvider) resolve(rel string) (string, error) {
	if strings.IndexByte(rel, 0) >= 0 {
		return "", ErrPathEscape
	}
	for _, r := range rel {
		if r < 0x20 {
			return "", ErrPathEscape
		}
	}
	if filepath.VolumeName(rel) != "" { // e.g. "C:" on Windows
		return "", ErrPathEscape
	}

	clean := "/" + strings.TrimLeft(filepath.ToSlash(rel), "/")
	clean = filepath.ToSlash(filepath.Clean(clean)) // collapse ".." against the leading "/"
	abs := filepath.Join(p.root, filepath.FromSlash(clean))

	if !withinRoot(p.root, abs) {
		return "", ErrPathEscape
	}
	if realAbs, err := filepath.EvalSymlinks(abs); err == nil {
		if !withinRoot(p.realRoot, realAbs) {
			return "", ErrPathEscape
		}
	} else {
		// abs doesn't fully exist, so EvalSymlinks couldn't resolve it. Resolve the
		// deepest existing ancestor instead, so a symlinked parent directory can't
		// smuggle a (yet non-existent) path outside the root.
		for anc := filepath.Dir(abs); len(anc) >= len(p.root); {
			if real, e := filepath.EvalSymlinks(anc); e == nil {
				if !withinRoot(p.realRoot, real) {
					return "", ErrPathEscape
				}
				break
			}
			parent := filepath.Dir(anc)
			if parent == anc {
				break
			}
			anc = parent
		}
	}
	return abs, nil
}

// withinRoot reports whether target is root or a descendant of it. The trailing
// separator on both sides prevents "/root-evil" from matching "/root".
func withinRoot(root, target string) bool {
	sep := string(os.PathSeparator)
	rootSep := strings.TrimSuffix(root, sep) + sep
	return strings.HasPrefix(target+sep, rootSep)
}

// ListDir implements Provider.
func (p *LocalProvider) ListDir(rel string) ([]FileInfo, error) {
	abs, err := p.resolve(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotExist
		}
		return nil, err
	}
	base := strings.Trim(filepath.ToSlash(rel), "/")
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // entry vanished between ReadDir and Info; skip
		}
		childRel := e.Name()
		if base != "" {
			childRel = base + "/" + e.Name()
		}
		out = append(out, toFileInfo(childRel, info))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir // directories first
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// Stat implements Provider.
func (p *LocalProvider) Stat(rel string) (FileInfo, error) {
	abs, err := p.resolve(rel)
	if err != nil {
		return FileInfo{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return FileInfo{}, ErrNotExist
		}
		return FileInfo{}, err
	}
	return toFileInfo(rel, info), nil
}

// Open implements Provider.
func (p *LocalProvider) Open(rel string) (io.ReadSeekCloser, FileInfo, error) {
	abs, err := p.resolve(rel)
	if err != nil {
		return nil, FileInfo{}, err
	}
	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, FileInfo{}, ErrNotExist
		}
		return nil, FileInfo{}, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, FileInfo{}, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, FileInfo{}, ErrIsDir
	}
	return f, toFileInfo(rel, info), nil
}

// Link implements Provider: local files are proxied, never redirected.
func (p *LocalProvider) Link(rel string) (string, bool, error) {
	if _, err := p.resolve(rel); err != nil {
		return "", false, err
	}
	return "", false, nil
}

// toFileInfo normalizes a relative path (forward slashes, dir trailing slash)
// and builds a FileInfo.
func toFileInfo(rel string, info os.FileInfo) FileInfo {
	s := strings.Trim(filepath.ToSlash(rel), "/")
	if info.IsDir() && s != "" {
		s += "/"
	}
	size := info.Size()
	if info.IsDir() {
		size = 0
	}
	return FileInfo{Path: s, IsDir: info.IsDir(), Size: size, Modified: info.ModTime().UTC()}
}
