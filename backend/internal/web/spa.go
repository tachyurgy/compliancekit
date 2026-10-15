// Package web serves the built React single-page app from disk so one Go
// process can host both the API and the UI on a single origin.
package web

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// SPA returns a handler that serves files under dir and falls back to
// index.html for client-side routes. Vite's hashed assets get a long cache
// lifetime; index.html is always revalidated so a deploy shows up at once.
// Requests under /api/ never fall back: they get a JSON 404.
func SPA(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"not found"}}`))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		if clean != "/" {
			if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(clean))); err == nil && !st.IsDir() {
				if strings.HasPrefix(clean, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fs.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, index)
	})
}
