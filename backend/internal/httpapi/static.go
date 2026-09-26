package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spa serves the built frontend from dir: existing files as they are (hashed
// assets cached for a year) and index.html for any other path, so client-side
// routes survive a reload. It lets a single container serve UI and API, which
// is what free single-service hosts (Render, Koyeb, Cloud Run) need.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "no such API route")
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		clean := filepath.Clean("/" + r.URL.Path)
		if st, err := os.Stat(filepath.Join(dir, clean)); err == nil && !st.IsDir() {
			if strings.HasPrefix(clean, "/assets/") {
				h.Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		h.Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
