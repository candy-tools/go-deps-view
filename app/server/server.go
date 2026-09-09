// Package server serves the dependency-graph viewer: the embedded HTML page
// plus the graph JSON, which is rebuilt from the target module on each request.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/candy-tools/go-deps-view/internal/graph"
)

//go:embed index.html
var assets embed.FS

// Handler returns the HTTP handler that serves the viewer page and the graph
// JSON for the module in dir, applying the given import-path excludes. The graph
// is rebuilt from the module on each /graph.json request.
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
		var data []byte
		if err == nil {
			data, err = json.Marshal(g)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})

	return mux
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
