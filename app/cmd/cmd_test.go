package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/go-deps-view/internal/graph"
)

func TestSplitExclude(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "/testdata", []string{"/testdata"}},
		{"multi with surrounding spaces", " /testdata , /mocks ", []string{"/testdata", "/mocks"}},
		{"drops empty fields", "/a,,/b,", []string{"/a", "/b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := splitExclude(tc.in); !equalStrings(got, tc.want) {
				t.Fatalf("splitExclude(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if code := Run([]string{"-version"}, &out, io.Discard); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	for _, want := range []string{"Version:", "Build date:", "Commit sha:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("version output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunJSON(t *testing.T) {
	var out bytes.Buffer
	if code := Run([]string{"-json", "-dir", minimalModule(t)}, &out, io.Discard); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	var g graph.Graph
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("decoding graph: %v", err)
	}
	if g.Module != "example.com/solo" {
		t.Errorf("Module = %q, want example.com/solo", g.Module)
	}
}

func TestRunJSONError(t *testing.T) {
	var errBuf bytes.Buffer
	if code := Run([]string{"-json", "-dir", t.TempDir()}, io.Discard, &errBuf); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if errBuf.Len() == 0 {
		t.Error("expected an error message on stderr")
	}
}

func TestRunServeDispatch(t *testing.T) {
	var gotAddr, gotDir string
	var gotExclude []string
	restore := stubServe(func(addr, dir string, exclude []string) error {
		gotAddr, gotDir, gotExclude = addr, dir, exclude
		return nil
	})
	defer restore()

	code := Run([]string{"-addr", ":9999", "-dir", "/somewhere", "-exclude", "/a,/b"}, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if gotAddr != ":9999" || gotDir != "/somewhere" {
		t.Errorf("serve got addr=%q dir=%q, want :9999 /somewhere", gotAddr, gotDir)
	}
	if !equalStrings(gotExclude, []string{"/a", "/b"}) {
		t.Errorf("serve got exclude=%v, want [/a /b]", gotExclude)
	}
}

func TestRunServeError(t *testing.T) {
	restore := stubServe(func(string, string, []string) error {
		return fmt.Errorf("boom")
	})
	defer restore()

	var errBuf bytes.Buffer
	if code := Run(nil, io.Discard, &errBuf); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "boom") {
		t.Errorf("stderr = %q, want it to contain boom", errBuf.String())
	}
}

func TestRunBadFlag(t *testing.T) {
	if code := Run([]string{"-nope"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func stubServe(fn func(addr, dir string, exclude []string) error) (restore func()) {
	orig := serveFn
	serveFn = fn
	return func() { serveFn = orig }
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

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
