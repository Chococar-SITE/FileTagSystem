package api

import (
	"path/filepath"
	"strings"
)

// previewKind classifies a file for the frontend viewer (§9.2).
type previewKind struct {
	Kind       string
	MIME       string
	Streamable bool // supports HTTP Range (audio/video)
}

var extKinds = map[string]previewKind{
	// images
	".jpg":  {"image", "image/jpeg", false},
	".jpeg": {"image", "image/jpeg", false},
	".png":  {"image", "image/png", false},
	".gif":  {"image", "image/gif", false},
	".webp": {"image", "image/webp", false},
	".avif": {"image", "image/avif", false},
	".bmp":  {"image", "image/bmp", false},
	// video
	".mp4":  {"video", "video/mp4", true},
	".webm": {"video", "video/webm", true},
	".mov":  {"video", "video/quicktime", true},
	".m4v":  {"video", "video/x-m4v", true},
	// audio
	".mp3":  {"audio", "audio/mpeg", true},
	".aac":  {"audio", "audio/aac", true},
	".m4a":  {"audio", "audio/mp4", true},
	".ogg":  {"audio", "audio/ogg", true},
	".opus": {"audio", "audio/opus", true},
	".wav":  {"audio", "audio/wav", true},
	".flac": {"audio", "audio/flac", true},
	// text
	".txt":  {"text", "text/plain; charset=utf-8", false},
	".md":   {"text", "text/plain; charset=utf-8", false},
	".csv":  {"text", "text/plain; charset=utf-8", false},
	".log":  {"text", "text/plain; charset=utf-8", false},
	".json": {"text", "text/plain; charset=utf-8", false},
	".yaml": {"text", "text/plain; charset=utf-8", false},
	".yml":  {"text", "text/plain; charset=utf-8", false},
	".go":   {"text", "text/plain; charset=utf-8", false},
	".js":   {"text", "text/plain; charset=utf-8", false},
	".ts":   {"text", "text/plain; charset=utf-8", false},
	".rs":   {"text", "text/plain; charset=utf-8", false},
	".py":   {"text", "text/plain; charset=utf-8", false},
	// office
	".docx": {"office", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", false},
	".xlsx": {"office", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", false},
	".pptx": {"office", "application/vnd.openxmlformats-officedocument.presentationml.presentation", false},
	".odt":  {"office", "application/vnd.oasis.opendocument.text", false},
	".ods":  {"office", "application/vnd.oasis.opendocument.spreadsheet", false},
	".odp":  {"office", "application/vnd.oasis.opendocument.presentation", false},
	// pdf
	".pdf": {"pdf", "application/pdf", false},
	// archives
	".zip": {"archive", "application/zip", false},
	".7z":  {"archive", "application/x-7z-compressed", false},
	".rar": {"archive", "application/vnd.rar", false},
	".tar": {"archive", "application/x-tar", false},
	".gz":  {"archive", "application/gzip", false},
	".tgz": {"archive", "application/gzip", false},
}

func kindForPath(p string) previewKind {
	ext := strings.ToLower(filepath.Ext(p))
	if k, ok := extKinds[ext]; ok {
		return k
	}
	return previewKind{Kind: "unknown", MIME: "application/octet-stream", Streamable: false}
}

// safeContentType neutralizes types the browser could execute in our origin
// (HTML/SVG → text/plain), defending the raw endpoint against stored XSS (§9.5).
func safeContentType(p string) string {
	k := kindForPath(p)
	ext := strings.ToLower(filepath.Ext(p))
	if ext == ".html" || ext == ".htm" || ext == ".svg" || ext == ".xml" || ext == ".xhtml" {
		return "text/plain; charset=utf-8"
	}
	return k.MIME
}
