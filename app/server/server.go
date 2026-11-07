// Package server serves the dependency-graph viewer: the embedded HTML page
// plus the graph JSON, which is rebuilt from the target module on each request.
package server

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/candy-tools/go-deps-view/internal/graph"
)

//go:embed index.html
var assets embed.FS

// Handler returns the HTTP handler that serves the viewer page, the graph JSON, the
// symbols JSON and the packages' READMEs for the module in dir, applying the given
// import-path excludes. All of them are rebuilt from the module on each request.
func Handler(dir string, exclude []string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		page, err := assets.ReadFile("index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})

	mux.HandleFunc("/graph.json", func(w http.ResponseWriter, r *http.Request) {
		g, err := graph.Build(dir, exclude)
		writeJSON(w, g, err)
	})

	// Separate from /graph.json because it type checks the module: the viewer only
	// asks for it when a package is first expanded.
	mux.HandleFunc("/symbols.json", func(w http.ResponseWriter, r *http.Request) {
		s, err := graph.BuildSymbols(dir, exclude)
		writeJSON(w, s, err)
	})

	// A package's README as an HTML fragment, for the dialog behind its info button.
	mux.HandleFunc("/readme", func(w http.ResponseWriter, r *http.Request) {
		html, err := graph.Readme(dir, r.URL.Query().Get("id"))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, fs.ErrInvalid):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(html) //nolint:gosec // G705: graph.Readme's renderer drops raw HTML and dangerous links
		}
	})

	return mux
}

// writeJSON writes v as the response, or err as a 500 with err's text as the body,
// which the viewer shows on the page.
func writeJSON(w http.ResponseWriter, v any, err error) {
	var data []byte
	if err == nil {
		data, err = json.Marshal(v)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// Serve builds the handler for dir and serves it on addr until the process is
// stopped.
func Serve(addr, dir string, exclude []string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(dir, exclude),
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("go-deps-view serving on http://localhost%s\n", addr)
	return srv.ListenAndServe()
}
