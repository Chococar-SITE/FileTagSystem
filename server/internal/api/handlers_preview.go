package api

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chococar-site/filetagsystem/server/internal/thumbnail"
)

const (
	thumbMaxDim     = 256
	thumbInputLimit = 50 << 20  // cap bytes read for thumbnailing
	archiveMaxEntry = 10000     // cap entries listed (zip-bomb defense, §9.5)
	archiveMaxSize  = 200 << 20 // refuse listing archives larger than this
)

// handleThumbnail serves a cached thumbnail, generating one on demand for images
// (§9.8). Non-images (or undecodable files) return 404 so the UI shows a
// placeholder.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForRead(w, r, p)
	if !ok {
		return
	}
	cacheRel := filepath.Join("thumbnails", strconv.FormatInt(f.StorageID, 10), strconv.FormatInt(f.ID, 10)+".png")
	cacheAbs := filepath.Join(s.cfg.DataDir, cacheRel)
	if data, err := os.ReadFile(cacheAbs); err == nil {
		serveThumb(w, data)
		return
	}
	if kindForPath(f.Path).Kind != "image" {
		writeError(w, http.StatusNotFound, "no thumbnail")
		return
	}
	prov, _, err := s.cat.Provider(r.Context(), f.StorageID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	rc, _, err := prov.Open(f.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, thumbInputLimit))
	if err != nil {
		serverError(w, err)
		return
	}
	thumb, err := thumbnail.Generate(data, thumbMaxDim)
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "cannot generate thumbnail")
		return
	}
	if err := os.MkdirAll(filepath.Dir(cacheAbs), 0o700); err == nil {
		if os.WriteFile(cacheAbs, thumb, 0o644) == nil {
			_ = s.cat.SetThumbnailByID(r.Context(), f.ID, cacheRel) // best-effort
		}
	}
	serveThumb(w, thumb)
}

func serveThumb(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

// archiveEntry is one item inside an archive.
type archiveEntry struct {
	Name       string `json:"name"`
	Size       uint64 `json:"size"`
	Compressed uint64 `json:"compressed"`
	IsDir      bool   `json:"is_dir"`
}

// handleArchive lists a zip's directory without extracting it (§9.3, §9.6).
func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request, p principal) {
	f, ok := s.fileForRead(w, r, p)
	if !ok {
		return
	}
	if strings.ToLower(filepath.Ext(f.Path)) != ".zip" {
		writeError(w, http.StatusBadRequest, "archive listing supports zip only")
		return
	}
	prov, _, err := s.cat.Provider(r.Context(), f.StorageID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	rc, fi, err := prov.Open(f.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	defer func() { _ = rc.Close() }()
	if fi.Size > archiveMaxSize {
		writeError(w, http.StatusRequestEntityTooLarge, "archive too large to list")
		return
	}

	var ra io.ReaderAt
	if r2, ok := rc.(io.ReaderAt); ok {
		ra = r2
	} else {
		ra = &seekerReaderAt{rs: rc}
	}
	zr, err := zip.NewReader(ra, fi.Size)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid zip")
		return
	}
	entries := make([]archiveEntry, 0, len(zr.File))
	for i, ze := range zr.File {
		if i >= archiveMaxEntry {
			break
		}
		// Names are display-only; normalize to neutralize zip-slip in any later
		// extraction (§9.5). We never extract here.
		name := strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(ze.Name, "\\", "/")), "/")
		entries = append(entries, archiveEntry{
			Name:       name,
			Size:       ze.UncompressedSize64,
			Compressed: ze.CompressedSize64,
			IsDir:      strings.HasSuffix(ze.Name, "/"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// seekerReaderAt adapts a ReadSeeker to io.ReaderAt for archive/zip. Not safe
// for concurrent use, which is fine for single-request listing.
type seekerReaderAt struct {
	rs io.ReadSeeker
}

func (s *seekerReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if _, err := s.rs.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(s.rs, p)
}
