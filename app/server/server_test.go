package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/go-deps-view/internal/graph"
)

func TestHandlerServesPage(t *testing.T) {
	h := Handler(minimalModule(t), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<title>go-deps-view</title>") {
		t.Error("page body did not contain the embedded viewer title")
	}
}

func TestHandlerServesGraphJSON(t *testing.T) {
	h := Handler(minimalModule(t), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/graph.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var g graph.Graph
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
		t.Fatalf("decoding graph: %v", err)
	}
	if g.Module != "example.com/solo" {
		t.Errorf("Module = %q, want example.com/solo", g.Module)
	}
}

func TestHandlerServesSymbolsJSON(t *testing.T) {
	dir := minimalModule(t)
	writeFile(t, filepath.Join(dir, "lib.go"), "package main\n\nfunc Exported() {}\n")
	h := Handler(dir, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/symbols.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var s graph.Symbols
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decoding symbols: %v", err)
	}
	if len(s.Packages) != 1 || len(s.Packages[0].Symbols) != 1 || s.Packages[0].Symbols[0].Name != "Exported" {
		t.Errorf("symbols = %+v, want one package exporting Exported", s)
	}
}

func TestHandlerSymbolsBuildError(t *testing.T) {
	h := Handler(t.TempDir(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/symbols.json", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestHandlerServesReadme(t *testing.T) {
	dir := minimalModule(t)
	writeFile(t, filepath.Join(dir, "README.md"), "# The project\n")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "sub", "sub.go"), "package sub\n")
	writeFile(t, filepath.Join(dir, "sub", "README.md"), "# Sub\n")
	h := Handler(dir, nil)
	cases := []struct {
		url  string
		code int
	}{
		{"/readme?id=./sub", http.StatusOK},
		{"/readme?id=.", http.StatusNotFound}, // the project's README, not the root package's
		{"/readme?id=./nope", http.StatusNotFound},
		{"/readme?id=..", http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if rec.Code != tc.code {
			t.Errorf("%s: status = %d, want %d", tc.url, rec.Code, tc.code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readme?id=./sub", nil))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<h1>Sub</h1>") {
		t.Errorf("body = %q, want the rendered README", rec.Body.String())
	}
}

func TestHandlerReadmeError(t *testing.T) {
	h := Handler(t.TempDir(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readme?id=./sub", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestHandlerNotFound(t *testing.T) {
	h := Handler(minimalModule(t), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerGraphBuildError(t *testing.T) {
	// A non-module directory makes graph.Build fail, so /graph.json is a 500.
	h := Handler(t.TempDir(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/graph.json", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func minimalModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/solo\n\ngo 1.25\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"x\") }\n")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
