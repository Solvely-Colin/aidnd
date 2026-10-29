// Package web serves the single-file /dev/ testing table.
package web

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var page []byte

// Handler serves the testing table at its root.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(page)
	})
}
