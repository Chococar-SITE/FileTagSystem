// Package storage abstracts file sources behind one interface (§2.2). Every
// method that takes a path resolves and validates it against the provider root
// (§7.3); callers must STILL enforce authorization (§6.3) on top of this.
package storage

import (
	"errors"
	"io"
	"time"
)

var (
	// ErrPathEscape means the requested path resolves outside the provider root.
	ErrPathEscape = errors.New("storage: path escapes root")
	// ErrNotExist means the resolved path does not exist.
	ErrNotExist = errors.New("storage: not found")
	// ErrIsDir is returned when opening a directory as a file.
	ErrIsDir = errors.New("storage: is a directory")
)

// FileInfo describes a directory entry. Path is relative to the provider root,
// forward-slash, with a trailing '/' for directories (§4.0).
type FileInfo struct {
	Path     string    `json:"path"`
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size_bytes"`
	Modified time.Time `json:"modified_at_fs"`
}

// Provider is the storage abstraction. Implementations must apply path-traversal
// protection on every path argument.
type Provider interface {
	Type() string
	Root() string
	// ListDir lists immediate children of a directory (live, like Alist).
	ListDir(rel string) ([]FileInfo, error)
	// Stat returns metadata for a single entry.
	Stat(rel string) (FileInfo, error)
	// Open returns a seekable reader for a file (enables HTTP Range, §9.4).
	Open(rel string) (io.ReadSeekCloser, FileInfo, error)
	// Link returns (url, true) for sources that redirect (cloud 302), or
	// ("", false) for sources that must be proxied (local).
	Link(rel string) (string, bool, error)
}
