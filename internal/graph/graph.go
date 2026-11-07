// Package graph builds a Go module's package dependency graph by shelling out
// to `go list`. It emits the in-module packages and their import edges plus the
// external library modules they import directly; all folder nesting and layering
// is left to the viewer, which derives it from the package paths. Separately, it
// can type check the module to detail which exported symbols those imports use.
package graph

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Node is one in-module Go package. The viewer derives its folder-box nesting and
// layer from the ID and the edges; Name tells it which packages are programs.
type Node struct {
	ID     string `json:"id"`     // module-relative path, e.g. "./app/router" ("." is the module root)
	Pkg    string `json:"pkg"`    // full import path
	Name   string `json:"name"`   // package clause name, "main" for a program
	Readme  bool   `json:"readme"`  // its folder has a README, which Readme renders
	Hidden  bool   `json:"hidden"`  // configured to start toggled off in the viewer
	Support bool   `json:"support"` // configured as support code, like a test helper, the viewer can leave out
}

// Edge is a "From imports To" relationship between two in-module packages.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Lib is an external dependency module imported directly by the project. Libs
// live in their own box to the side; they are not layered.
type Lib struct {
	ID     string `json:"id"`     // module path, e.g. "github.com/gorilla/mux"
	Label  string `json:"label"`  // short display name
	Hidden  bool   `json:"hidden"`  // configured to start toggled off in the viewer
	Support bool   `json:"support"` // as for nodes
}

// Graph is the whole dependency graph: in-module packages and their import
// edges, plus the external libraries the project imports directly.
type Graph struct {
	Module   string  `json:"module"`
	Nodes    []Node  `json:"nodes"`
	Edges    []Edge  `json:"edges"`
	Libs     []Lib   `json:"libs"`     // external dependency modules
	LibEdges []Edge  `json:"libEdges"` // project package -> lib module
	Layers   []Layer `json:"layers"`   // configured layers, top first; empty when not configured
}

// goListPkg is the subset of `go list -deps -json` output that we use.
type goListPkg struct {
	ImportPath string
	Dir        string // the package's folder
	Name       string
	Standard   bool // true for standard-library packages
	Module     *struct {
		Path string
	}
	Imports []string
}

// Build runs `go list` in dir and assembles the dependency graph, including the
// external libraries the project packages import directly. Packages whose import
// path contains any of the exclude substrings are dropped. The settings in dir's
// config file, if any, are passed along for the viewer.
func Build(dir string, exclude []string) (*Graph, error) {
	module, err := modulePath(dir)
	if err != nil {
		return nil, err
	}

	pkgs, err := goList(dir)
	if err != nil {
		return nil, err
	}

	inModule := moduleMatcher(module, exclude)

	// Walk the project packages: internal imports become graph edges; external
	// (non-stdlib) imports are resolved to their library module and collapsed to
	// one edge per (package, library).
	imports := map[string][]string{}         // id -> local import ids
	names := map[string]string{}             // id -> package clause name
	readmes := map[string]bool{}             // ids whose folder has a README
	libSet := map[string]bool{}              // library module paths
	libEdges := map[string]map[string]bool{} // pkg id -> set of library ids
	for _, p := range pkgs {
		if !inModule(p.ImportPath) {
			continue
		}
		id := relID(module, p.ImportPath)
		names[id] = p.Name
		readmes[id] = id != "." && p.Dir != "" && readmeFile(os.DirFS(p.Dir), ".") != ""
		if _, ok := imports[id]; !ok {
			imports[id] = nil
		}
		for _, imp := range p.Imports {
			if inModule(imp) {
				imports[id] = append(imports[id], relID(module, imp))
				continue
			}
			dep, ok := pkgs[imp]
			if !ok || dep.Standard {
				continue // standard library or unresolved: not a "library"
			}
			libID := imp
			if dep.Module != nil && dep.Module.Path != "" {
				libID = dep.Module.Path
			}
			libSet[libID] = true
			if libEdges[id] == nil {
				libEdges[id] = map[string]bool{}
			}
			libEdges[id][libID] = true
		}
	}

	g := assemble(module, imports, names)
	for i := range g.Nodes {
		g.Nodes[i].Readme = readmes[g.Nodes[i].ID]
	}
	addLibs(g, libSet, libEdges)
	if err := applyConfig(g, dir); err != nil {
		return nil, err
	}
	ensureNonNil(g)
	return g, nil
}

// moduleMatcher returns the test for whether an import path counts as part of the
// module: it has the module path as a prefix and contains no exclude substring.
func moduleMatcher(module string, exclude []string) func(string) bool {
	return func(p string) bool {
		if !strings.HasPrefix(p, module) {
			return false
		}
		for _, ex := range exclude {
			if strings.Contains(p, ex) {
				return false
			}
		}
		return true
	}
}

// goList runs `go list -deps -json ./...` in dir and returns the packages keyed
// by import path.
func goList(dir string) (map[string]goListPkg, error) {
	cmd := exec.Command("go", "list", "-deps", "-json", "./...")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// -deps streams one JSON object per package, so decode until EOF.
	dec := json.NewDecoder(stdout)
	pkgs := map[string]goListPkg{}
	for {
		var p goListPkg
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		pkgs[p.ImportPath] = p
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("go list failed: %w", err)
	}
	return pkgs, nil
}

// addLibs appends the collected external libraries and their edges to g, sorted
// for stable output.
func addLibs(g *Graph, libSet map[string]bool, libEdges map[string]map[string]bool) {
	libIDs := make([]string, 0, len(libSet))
	for l := range libSet {
		libIDs = append(libIDs, l)
	}
	sort.Strings(libIDs)
	for _, l := range libIDs {
		g.Libs = append(g.Libs, Lib{ID: l, Label: libLabel(l)})
	}
	for from, set := range libEdges {
		for to := range set {
			g.LibEdges = append(g.LibEdges, Edge{From: from, To: to})
		}
	}
	sort.Slice(g.LibEdges, func(i, j int) bool {
		if g.LibEdges[i].From != g.LibEdges[j].From {
			return g.LibEdges[i].From < g.LibEdges[j].From
		}
		return g.LibEdges[i].To < g.LibEdges[j].To
	})
}

// ensureNonNil replaces nil collections with empty slices so the graph always
// marshals as JSON arrays rather than null.
func ensureNonNil(g *Graph) {
	if g.Nodes == nil {
		g.Nodes = []Node{}
	}
	if g.Edges == nil {
		g.Edges = []Edge{}
	}
	if g.Libs == nil {
		g.Libs = []Lib{}
	}
	if g.LibEdges == nil {
		g.LibEdges = []Edge{}
	}
	if g.Layers == nil {
		g.Layers = []Layer{}
	}
}

// modulePath reads the module path from go.mod via `go list -m`.
func modulePath(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading module path: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// moduleDir returns the root folder of the module holding dir.
func moduleDir(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading module folder: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" { // outside a module, go list -m answers with no folder
		return "", fmt.Errorf("reading module folder: %s is not in a module", dir)
	}
	return root, nil
}

// assemble turns the id->local-imports map into a flat graph of packages and
// edges, sorted for stable output. All grouping and layering is done by the
// viewer from the package paths.
func assemble(module string, imports map[string][]string, names map[string]string) *Graph {
	g := &Graph{Module: module}
	ids := make([]string, 0, len(imports))
	for id := range imports {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g.Nodes = append(g.Nodes, Node{ID: id, Pkg: fullPkg(module, id), Name: names[id]})
		deps := append([]string(nil), imports[id]...)
		sort.Strings(deps)
		for _, dep := range deps {
			g.Edges = append(g.Edges, Edge{From: id, To: dep})
		}
	}
	return g
}

// relID converts a full import path to a module-relative id ("." for the root).
func relID(module, importPath string) string {
	if importPath == module {
		return "."
	}
	return "." + strings.TrimPrefix(importPath, module)
}

// fullPkg is the inverse of relID.
func fullPkg(module, id string) string {
	if id == "." {
		return module
	}
	return module + strings.TrimPrefix(id, ".")
}

// libLabel shortens a module path for display: the common "github.com/" host is
// dropped, others (e.g. "gorm.io/gorm") are kept as-is.
func libLabel(modulePath string) string {
	return strings.TrimPrefix(modulePath, "github.com/")
}
