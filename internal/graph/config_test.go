package graph

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseLayers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Layer
	}{
		{"empty", "", nil},
		{"only comments", "# nothing yet\n", nil},
		{"no layers", "layers: []\n", []Layer{}},
		{
			"all fields",
			"layers:\n  - name: Entry\n    description: Start here\n    entrypoint: true\n    packages: [search, tagging]\n",
			[]Layer{{Name: "Entry", Description: "Start here", Entrypoint: true, Packages: []string{"./search", "./tagging"}}},
		},
		{
			"keeps order, the wildcard and the root package",
			"layers:\n  - packages: [\"*\"]\n  - packages: [.]\n",
			[]Layer{{Packages: []string{"*"}}, {Packages: []string{"."}}},
		},
		{
			"normalises paths",
			"layers:\n  - packages: [./search, vector/, a//b]\n",
			[]Layer{{Packages: []string{"./search", "./vector", "./a/b"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseConfig([]byte(tc.in))
			if err != nil {
				t.Fatalf("parseConfig(%q): %v", tc.in, err)
			}
			if !reflect.DeepEqual(got.Layers, tc.want) {
				t.Errorf("parseConfig(%q).Layers = %+v, want %+v", tc.in, got.Layers, tc.want)
			}
		})
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"misspelled key", "layers:\n  - nmae: Entry\n    packages: [search]\n", "nmae"},
		{"unknown top-level key", "levels: []\n", "levels"},
		{"invalid yaml", "layers: [\n", "yaml"},
		{"unquoted wildcard", "layers:\n  - packages: [*]\n", "yaml"},
		{"layer without packages", "layers:\n  - name: Entry\n", "lists no packages"},
		{"info that isn't text", "info:\n  search: [a, b]\n", "yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parseConfig(%q) error = %v, want it to mention %q", tc.in, err, tc.want)
			}
		})
	}
}

func TestApplyInfo(t *testing.T) {
	g := &Graph{
		Nodes: []Node{{ID: "."}, {ID: "./sub/inner"}, {ID: "./other"}},
		Libs:  []Lib{{ID: "example.com/extlib"}, {ID: "example.com/unused"}},
	}

	unmatched := applyInfo(g, map[string]string{
		".":                  "root",
		"sub/inner":          "written like a layer entry",
		"./other":            "written like a node id",
		"example.com/extlib": "a library",
		"sub":                "a folder is not a package",
		"typo":               "no such package",
	})

	wantNodes := []Node{
		{ID: ".", Info: "root"},
		{ID: "./sub/inner", Info: "written like a layer entry"},
		{ID: "./other", Info: "written like a node id"},
	}
	if !reflect.DeepEqual(g.Nodes, wantNodes) {
		t.Errorf("Nodes = %+v, want %+v", g.Nodes, wantNodes)
	}
	wantLibs := []Lib{{ID: "example.com/extlib", Info: "a library"}, {ID: "example.com/unused"}}
	if !reflect.DeepEqual(g.Libs, wantLibs) {
		t.Errorf("Libs = %+v, want %+v", g.Libs, wantLibs)
	}
	// Sorted, so the warnings come out in a stable order.
	if want := []string{"sub", "typo"}; !reflect.DeepEqual(unmatched, want) {
		t.Errorf("unmatched = %v, want %v", unmatched, want)
	}
}

func TestBuildReadsInfo(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(mod, configFile),
		"info:\n  sub: |\n    First line.\n    Second line.\n  example.com/extlib: The library.\n")

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	info := map[string]string{}
	for _, n := range g.Nodes {
		info[n.ID] = n.Info
	}
	want := map[string]string{".": "", "./mocks": "", "./sub": "First line.\nSecond line.\n"}
	if !reflect.DeepEqual(info, want) {
		t.Errorf("node info = %q, want %q", info, want)
	}
	if len(g.Libs) != 1 || g.Libs[0].Info != "The library." {
		t.Errorf("Libs = %+v, want one with the configured info", g.Libs)
	}
}

func TestUnmatchedEntries(t *testing.T) {
	nodes := []Node{{ID: "./sub/inner"}, {ID: "./other"}}
	layers := []Layer{
		{Packages: []string{"./sub", "./other", "*"}},
		{Packages: []string{"./sub/inner", "./typo", "./su", "."}},
	}

	got := unmatchedEntries(layers, nodes)
	// A folder matches through its packages, a partial segment does not, and "."
	// names only the root package, which these nodes don't have.
	want := []string{"./typo", "./su", "."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unmatchedEntries = %v, want %v", got, want)
	}
}

func TestBuildReadsLayers(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(mod, configFile),
		"layers:\n  - name: Top\n    entrypoint: true\n    packages: [sub]\n  - packages: [\"*\"]\n  - packages: [mocks]\n")

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []Layer{
		{Name: "Top", Entrypoint: true, Packages: []string{"./sub"}},
		{Packages: []string{"*"}},
		{Packages: []string{"./mocks"}},
	}
	if !reflect.DeepEqual(g.Layers, want) {
		t.Errorf("Layers = %+v, want %+v", g.Layers, want)
	}
}

func TestBuildWithoutConfigHasEmptyLayers(t *testing.T) {
	g, err := Build(writeMinimalModule(t), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Must marshal as [] rather than null for the viewer.
	if g.Layers == nil || len(g.Layers) != 0 {
		t.Errorf("Layers = %#v, want an empty non-nil slice", g.Layers)
	}
}

func TestBuildUnreadableConfig(t *testing.T) {
	mod := writeMinimalModule(t)
	// A directory exists but can't be read as a file, unlike a missing config.
	if err := os.Mkdir(filepath.Join(mod, configFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(mod, nil); err == nil {
		t.Fatal("expected an error for an unreadable config")
	}
}

func TestBuildInvalidConfigNamesTheFile(t *testing.T) {
	mod := writeMinimalModule(t)
	write(t, filepath.Join(mod, configFile), "levels: []\n")

	_, err := Build(mod, nil)
	if err == nil || !strings.Contains(err.Error(), configFile) {
		t.Fatalf("error = %v, want it to name %s", err, configFile)
	}
}
