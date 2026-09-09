package graph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAssemble(t *testing.T) {
	const mod = "example.com/mod"
	imports := map[string][]string{
		".":         {"./app/cmd"},
		"./app/cmd": {},
		"./z":       {"./app/cmd"},
	}

	g := assemble(mod, imports)

	if g.Module != mod {
		t.Errorf("Module = %q, want %q", g.Module, mod)
	}
	wantNodes := []Node{
		{ID: ".", Pkg: "example.com/mod"},
		{ID: "./app/cmd", Pkg: "example.com/mod/app/cmd"},
		{ID: "./z", Pkg: "example.com/mod/z"},
	}
	if !reflect.DeepEqual(g.Nodes, wantNodes) {
		t.Errorf("Nodes = %+v, want %+v", g.Nodes, wantNodes)
	}
	wantEdges := []Edge{
		{From: ".", To: "./app/cmd"},
		{From: "./z", To: "./app/cmd"},
	}
	if !reflect.DeepEqual(g.Edges, wantEdges) {
		t.Errorf("Edges = %+v, want %+v", g.Edges, wantEdges)
	}
}

func TestBuild(t *testing.T) {
	mod := writeFixtureModule(t)

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if g.Module != "example.com/fixture" {
		t.Errorf("Module = %q, want example.com/fixture", g.Module)
	}
	// "." (main) and "./mocks" and "./sub" are in-module; extlib and fmt are not.
	wantNodes := []Node{
		{ID: ".", Pkg: "example.com/fixture"},
		{ID: "./mocks", Pkg: "example.com/fixture/mocks"},
		{ID: "./sub", Pkg: "example.com/fixture/sub"},
	}
	if !reflect.DeepEqual(g.Nodes, wantNodes) {
		t.Errorf("Nodes = %+v, want %+v", g.Nodes, wantNodes)
	}
	// main imports sub (in-module); it does not import mocks; sub imports nothing.
	wantEdges := []Edge{{From: ".", To: "./sub"}}
	if !reflect.DeepEqual(g.Edges, wantEdges) {
		t.Errorf("Edges = %+v, want %+v", g.Edges, wantEdges)
	}
	// The external module is a lib; the standard library (fmt) is excluded.
	wantLibs := []Lib{{ID: "example.com/extlib", Label: "example.com/extlib"}}
	if !reflect.DeepEqual(g.Libs, wantLibs) {
		t.Errorf("Libs = %+v, want %+v", g.Libs, wantLibs)
	}
	wantLibEdges := []Edge{{From: ".", To: "example.com/extlib"}}
	if !reflect.DeepEqual(g.LibEdges, wantLibEdges) {
		t.Errorf("LibEdges = %+v, want %+v", g.LibEdges, wantLibEdges)
	}
}

func TestBuildExcludesDropNodes(t *testing.T) {
	mod := writeFixtureModule(t)

	g, err := Build(mod, []string{"/mocks"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, n := range g.Nodes {
		if n.ID == "./mocks" {
			t.Fatalf("excluded node ./mocks still present: %+v", g.Nodes)
		}
	}
	// The external lib is unaffected by the exclude.
	wantLibs := []Lib{{ID: "example.com/extlib", Label: "example.com/extlib"}}
	if !reflect.DeepEqual(g.Libs, wantLibs) {
		t.Errorf("Libs = %+v, want %+v", g.Libs, wantLibs)
	}
}

func TestBuildEmptyCollectionsAreNonNil(t *testing.T) {
	mod := writeMinimalModule(t)

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// A lone package importing only the standard library has no edges or libs,
	// but the slices must marshal as [] rather than null for the viewer.
	if g.Nodes == nil || g.Edges == nil || g.Libs == nil || g.LibEdges == nil {
		t.Fatalf("collections must be non-nil: %+v", g)
	}
	if len(g.Libs) != 0 || len(g.Edges) != 0 || len(g.LibEdges) != 0 {
		t.Errorf("expected empty edges/libs, got %+v", g)
	}
}

func TestBuildBadDir(t *testing.T) {
	if _, err := Build(t.TempDir(), nil); err == nil {
		t.Fatal("expected an error building a non-module directory")
	}
}

// writeFixtureModule writes a two-module fixture under a temp dir and returns
// the main module's directory. The main module (example.com/fixture) has main,
// sub, and mocks packages; main imports sub plus a package from a second local
// module (example.com/extlib) wired in via a filesystem replace, so Build's
// external-library resolution runs without any network access.
func writeFixtureModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	modDir := filepath.Join(root, "mod")
	extDir := filepath.Join(root, "extlib")

	write(t, filepath.Join(extDir, "go.mod"), "module example.com/extlib\n\ngo 1.25\n")
	write(t, filepath.Join(extDir, "pkg", "pkg.go"), "package pkg\n")

	write(t, filepath.Join(modDir, "go.mod"),
		"module example.com/fixture\n\ngo 1.25\n\nrequire example.com/extlib v0.0.0\n\nreplace example.com/extlib => ../extlib\n")
	write(t, filepath.Join(modDir, "main.go"),
		"package main\n\nimport (\n\t\"fmt\"\n\n\t_ \"example.com/extlib/pkg\"\n\t_ \"example.com/fixture/sub\"\n)\n\nfunc main() { fmt.Println(\"x\") }\n")
	write(t, filepath.Join(modDir, "sub", "sub.go"), "package sub\n")
	write(t, filepath.Join(modDir, "mocks", "mocks.go"), "package mocks\n")

	return modDir
}

// writeMinimalModule writes a single-package module importing only the standard
// library and returns its directory.
func writeMinimalModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/solo\n\ngo 1.25\n")
	write(t, filepath.Join(dir, "main.go"),
		"package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"x\") }\n")
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
