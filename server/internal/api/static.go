package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// registerStatic serves the built frontend (web/dist) when present, enabling a
// single-binary deployment. API routes take precedence; unknown non-API paths
// fall back to index.html for client-side routing.
func (s *Server) registerStatic(m *http.ServeMux) {
	dir := os.Getenv("WEB_DIST")
	if dir == "" {
		dir = "web/dist"
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return // no build present; API-only
	}
	fileServer := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")

	m.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if r.URL.Path != "/" {
			candidate := filepath.Join(dir, filepath.Clean(r.URL.Path))
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, index)
	})
}
