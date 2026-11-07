package graph

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
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
		{"invalid yaml", "layers: [\n", "yaml"},
		{"unquoted wildcard", "layers:\n  - packages: [*]\n", "yaml"},
		{"layer without packages", "layers:\n  - name: Entry\n", "lists no packages"},
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

func TestParseConfigIgnoresUnknownKeys(t *testing.T) {
	in := "info:\n  search: Searches.\nlevels: []\nlayers:\n  - nmae: Entry\n    packages: [search]\nhidden: [search]\n"

	got, err := parseConfig([]byte(in))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	want := config{Layers: []Layer{{Packages: []string{"./search"}}}, Hidden: []string{"search"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseConfig = %+v, want %+v", got, want)
	}
}

func TestApplyHidden(t *testing.T) {
	cases := []struct {
		name          string
		entries       []string
		wantNodes     []string
		wantLibs      []string
		wantUnmatched []string
	}{
		{name: "nothing listed"},
		{name: "a package", entries: []string{"other"}, wantNodes: []string{"./other"}},
		{name: "written like a node id", entries: []string{"./other"}, wantNodes: []string{"./other"}},
		{
			name:      "a folder covers what is inside it, not a sibling sharing its prefix",
			entries:   []string{"sub"},
			wantNodes: []string{"./sub/deep/x", "./sub/inner"},
		},
		{name: "the root is only the root package", entries: []string{"."}, wantNodes: []string{"."}},
		{name: "a library", entries: []string{"example.com/extlib"}, wantLibs: []string{"example.com/extlib"}},
		{
			name:          "a library must be named in full",
			entries:       []string{"example.com"},
			wantUnmatched: []string{"example.com"},
		},
		{
			name:          "unmatched entries keep their order and the rest still apply",
			entries:       []string{"typo", "other", "su"},
			wantNodes:     []string{"./other"},
			wantUnmatched: []string{"typo", "su"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &Graph{
				Nodes: []Node{{ID: "."}, {ID: "./other"}, {ID: "./sub/deep/x"}, {ID: "./sub/inner"}, {ID: "./subway"}},
				Libs:  []Lib{{ID: "example.com/extlib"}, {ID: "example.com/unused"}},
			}

			unmatched := applyHidden(g, tc.entries)

			var gotNodes, gotLibs []string
			for _, n := range g.Nodes {
				if n.Hidden {
					gotNodes = append(gotNodes, n.ID)
				}
			}
			for _, l := range g.Libs {
				if l.Hidden {
					gotLibs = append(gotLibs, l.ID)
				}
			}
			if !reflect.DeepEqual(gotNodes, tc.wantNodes) {
				t.Errorf("hidden nodes = %v, want %v", gotNodes, tc.wantNodes)
			}
			if !reflect.DeepEqual(gotLibs, tc.wantLibs) {
				t.Errorf("hidden libs = %v, want %v", gotLibs, tc.wantLibs)
			}
			if !reflect.DeepEqual(unmatched, tc.wantUnmatched) {
				t.Errorf("unmatched = %v, want %v", unmatched, tc.wantUnmatched)
			}
		})
	}
}

// Support entries resolve like hidden ones, which TestApplyHidden covers in full, but
// set their own flag and leave the hidden one alone.
func TestApplySupport(t *testing.T) {
	g := &Graph{
		Nodes: []Node{{ID: "."}, {ID: "./cache"}, {ID: "./cache/cachetest"}, {ID: "./cachetestx"}},
		Libs:  []Lib{{ID: "example.com/testlib"}, {ID: "example.com/extlib"}},
	}

	unmatched := applySupport(g, []string{"cache/cachetest", "example.com/testlib", "typo"})

	var gotNodes, gotLibs []string
	for _, n := range g.Nodes {
		if n.Support {
			gotNodes = append(gotNodes, n.ID)
		}
		if n.Hidden {
			t.Errorf("node %q hidden, want only the support flag set", n.ID)
		}
	}
	for _, l := range g.Libs {
		if l.Support {
			gotLibs = append(gotLibs, l.ID)
		}
		if l.Hidden {
			t.Errorf("lib %q hidden, want only the support flag set", l.ID)
		}
	}
	if want := []string{"./cache/cachetest"}; !reflect.DeepEqual(gotNodes, want) {
		t.Errorf("support nodes = %v, want %v", gotNodes, want)
	}
	if want := []string{"example.com/testlib"}; !reflect.DeepEqual(gotLibs, want) {
		t.Errorf("support libs = %v, want %v", gotLibs, want)
	}
	if want := []string{"typo"}; !reflect.DeepEqual(unmatched, want) {
		t.Errorf("unmatched = %v, want %v", unmatched, want)
	}
}

func TestBuildEmitsSupport(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(mod, configFile), "support: [mocks, example.com/extlib]\n")

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Nodes []struct {
			ID      string `json:"id"`
			Support bool   `json:"support"`
		} `json:"nodes"`
		Libs []struct {
			Support bool `json:"support"`
		} `json:"libs"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	nodes := map[string]bool{}
	for _, n := range got.Nodes {
		nodes[n.ID] = n.Support
	}
	if want := map[string]bool{".": false, "./mocks": true, "./sub": false}; !reflect.DeepEqual(nodes, want) {
		t.Errorf("node support = %v, want %v", nodes, want)
	}
	if len(got.Libs) != 1 || !got.Libs[0].Support {
		t.Errorf("libs = %+v, want the one library flagged support", got.Libs)
	}
}

// Decodes the JSON rather than reading the structs, as the viewer only sees that.
func TestBuildEmitsHidden(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(mod, configFile), "hidden: [sub, example.com/extlib]\n")

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Nodes []struct {
			ID     string `json:"id"`
			Hidden bool   `json:"hidden"`
		} `json:"nodes"`
		Libs []struct {
			ID     string `json:"id"`
			Hidden bool   `json:"hidden"`
		} `json:"libs"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	nodes := map[string]bool{}
	for _, n := range got.Nodes {
		nodes[n.ID] = n.Hidden
	}
	if want := map[string]bool{".": false, "./mocks": false, "./sub": true}; !reflect.DeepEqual(nodes, want) {
		t.Errorf("node hidden = %v, want %v", nodes, want)
	}
	if len(got.Libs) != 1 || !got.Libs[0].Hidden {
		t.Errorf("libs = %+v, want the one library hidden", got.Libs)
	}
}

func TestInitConfigCreatesAFileThatChangesNothing(t *testing.T) {
	mod := writeFixtureModule(t)

	path, err := InitConfig(mod)
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}
	if want := filepath.Join(mod, configFile); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build with the new config: %v", err)
	}
	if len(g.Layers) != 0 {
		t.Errorf("Layers = %+v, want none until the file is filled in", g.Layers)
	}
	for _, n := range g.Nodes {
		if n.Hidden || n.Support {
			t.Errorf("node %+v, want no settings until the file is filled in", n)
		}
	}
}

func TestInitConfigKeepsAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	const mine = "layers:\n  - packages: [mine]\n"
	write(t, filepath.Join(dir, configFile), mine)

	_, err := InitConfig(dir)

	if !errors.Is(err, fs.ErrExist) {
		t.Errorf("error = %v, want fs.ErrExist", err)
	}
	got, readErr := os.ReadFile(filepath.Join(dir, configFile))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != mine {
		t.Errorf("file = %q, want it untouched", got)
	}
}

func TestInitConfigMissingDirectory(t *testing.T) {
	if _, err := InitConfig(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

// The file's commented-out example is what an agent copies, so uncommented it has
// to parse, and show every section. A real config may hold unknown keys, but the
// example may not, as there one would be a typo that silently does nothing.
func TestConfigTemplateExampleIsValid(t *testing.T) {
	_, example, found := strings.Cut(configTemplate, exampleMarker+"\n")
	if !found {
		t.Fatalf("template has no %q line", exampleMarker)
	}
	var uncommented strings.Builder
	for line := range strings.Lines(example) {
		uncommented.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "#"), " "))
	}

	cfg, err := parseConfig([]byte(uncommented.String()))
	if err != nil {
		t.Fatalf("uncommented example: %v\n%s", err, uncommented.String())
	}
	strict := yaml.NewDecoder(strings.NewReader(uncommented.String()))
	strict.KnownFields(true)
	if err := strict.Decode(&config{}); err != nil {
		t.Errorf("uncommented example has an unknown key: %v", err)
	}
	if len(cfg.Layers) == 0 || len(cfg.Hidden) == 0 || len(cfg.Support) == 0 {
		t.Errorf("example = %+v, want layers, hidden and support all shown", cfg)
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
	write(t, filepath.Join(mod, configFile), "layers: [\n")

	_, err := Build(mod, nil)
	if err == nil || !strings.Contains(err.Error(), configFile) {
		t.Fatalf("error = %v, want it to name %s", err, configFile)
	}
}
