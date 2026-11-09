package graph

import (
	"cmp"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// gormdbNodes is the go-bumbu/db case from the flatten spec: a library keeping
// every package under one folder.
func gormdbNodes() []Node {
	return nodes("./gormdb", "./gormdb/gormdbtest", "./gormdb/internal/sqliteerr",
		"./gormdb/internal/sqlitetest", "./gormdb/postgres", "./gormdb/sqlite", "./gormdb/sqlitecgo")
}

// gormdbLayers is the spec's acceptance config, parsed.
func gormdbLayers(t *testing.T) []Layer {
	t.Helper()
	cfg, err := parseConfig([]byte(`
layers:
  - name: Drivers
    packages: [gormdb/sqlite, gormdb/sqlitecgo, gormdb/postgres]
  - name: Core
    packages: [gormdb, "*"]
  - name: Internal
    packages: [gormdb/internal/sqliteerr]
  - name: Test helpers
    packages: [gormdb/gormdbtest, gormdb/internal/sqlitetest]
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Layers
}

func nodes(ids ...string) []Node {
	out := make([]Node, len(ids))
	for i, id := range ids {
		out[i] = Node{ID: id}
	}
	return out
}

func TestBandItems(t *testing.T) {
	cases := []struct {
		name    string
		nodes   []Node
		flatten []string
		want    map[string][]string // item path -> its packages
		in      string              // the folder holding them, if not the root
	}{
		{
			name:  "bands are made of what a lone folder holds",
			nodes: gormdbNodes(),
			want: map[string][]string{
				"./gormdb": {"./gormdb"}, "./gormdb/gormdbtest": {"./gormdb/gormdbtest"},
				"./gormdb/internal":  {"./gormdb/internal/sqliteerr", "./gormdb/internal/sqlitetest"},
				"./gormdb/postgres":  {"./gormdb/postgres"},
				"./gormdb/sqlite":    {"./gormdb/sqlite"},
				"./gormdb/sqlitecgo": {"./gormdb/sqlitecgo"},
			},
			in: "./gormdb",
		},
		{
			name:  "a lone folder without a package of its own",
			nodes: nodes("./pkg/a/x", "./pkg/a/y", "./pkg/b"),
			want:  map[string][]string{"./pkg/a": {"./pkg/a/x", "./pkg/a/y"}, "./pkg/b": {"./pkg/b"}},
			in:    "./pkg",
		},
		{
			name:  "a lone folder drawn as its only subfolder",
			nodes: nodes("./x/y/p", "./x/y/q"),
			want:  map[string][]string{"./x/y/p": {"./x/y/p"}, "./x/y/q": {"./x/y/q"}},
			in:    "./x/y",
		},
		{
			name:    "flattening the folder leaves its subfolders as boxes",
			nodes:   gormdbNodes(),
			flatten: []string{"./gormdb"},
			want: map[string][]string{
				"./gormdb": {"./gormdb"}, "./gormdb/gormdbtest": {"./gormdb/gormdbtest"},
				"./gormdb/internal":  {"./gormdb/internal/sqliteerr", "./gormdb/internal/sqlitetest"},
				"./gormdb/postgres":  {"./gormdb/postgres"},
				"./gormdb/sqlite":    {"./gormdb/sqlite"},
				"./gormdb/sqlitecgo": {"./gormdb/sqlitecgo"},
			},
		},
		{
			name:    "nested entries dissolve bottom-up, whatever their order",
			nodes:   nodes("./a/b/x", "./a/b/y", "./a/z"),
			flatten: []string{"./a", "./a/b"},
			want:    map[string][]string{"./a/b/x": {"./a/b/x"}, "./a/b/y": {"./a/b/y"}, "./a/z": {"./a/z"}},
		},
		{
			name:  "a folder holding only one subfolder is drawn as it, and the root package is .",
			nodes: nodes(".", "./internal/store/pg", "./internal/store/mem"),
			want:  map[string][]string{".": {"."}, "./internal/store": {"./internal/store/mem", "./internal/store/pg"}},
		},
		{
			name:    "a flattened child may share its name with a sibling",
			nodes:   nodes("./a/x/p", "./a/q", "./x"),
			flatten: []string{"./a"},
			want:    map[string][]string{"./a/q": {"./a/q"}, "./a/x/p": {"./a/x/p"}, "./x": {"./x"}},
		},
		{
			name:    "entries with nothing to dissolve change nothing",
			nodes:   nodes("./a", "./b/c", "./b/d"),
			flatten: []string{"./a", "./nope", "./b/c"},
			want:    map[string][]string{"./a": {"./a"}, "./b": {"./b/c", "./b/d"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string][]string{}
			items, in := bandItems(folderTree(tc.nodes, tc.flatten))
			for _, it := range items {
				got[it.path] = slices.Sorted(slices.Values(it.ids))
			}
			if want := cmp.Or(tc.in, "."); in.path != want {
				t.Errorf("held by %q, want %q", in.path, want)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("bandItems = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLayerIndex(t *testing.T) {
	idx := newLayerIndex([]Layer{
		{Packages: []string{"./a"}},
		{Packages: []string{"*", "./a/b"}},
		{Packages: []string{"./a", "*", "."}},
	})
	cases := map[string]int{
		"./a":     0, // listed twice: the first layer wins
		"./a/c/d": 0, // the closest listed path
		"./a/b/c": 1,
		"./z":     1, // the first "*" wins
		".":       2,
	}
	for p, want := range cases {
		if got := idx.of(p); got != want {
			t.Errorf("of(%q) = %d, want %d", p, got, want)
		}
	}
	if got := newLayerIndex([]Layer{{Packages: []string{"./a"}}}).of("./z"); got != 1 {
		t.Errorf("without a \"*\" layer, of(./z) = %d, want 1, below every layer", got)
	}
}

func TestBandWarnings(t *testing.T) {
	t.Run("the acceptance config gets every band", func(t *testing.T) {
		got := bandWarnings(gormdbLayers(t), []string{"./gormdb", "./gormdb/internal"}, gormdbNodes())
		if len(got) != 0 {
			t.Errorf("warnings = %q, want none", got)
		}
	})

	t.Run("keeping the lone folder's box, only the folder inside it needs flattening", func(t *testing.T) {
		if got := bandWarnings(gormdbLayers(t), []string{"./gormdb/internal"}, gormdbNodes()); len(got) != 0 {
			t.Errorf("warnings = %q, want none", got)
		}
	})

	t.Run("without flatten, only the folder inside the lone one is in the way", func(t *testing.T) {
		got := bandWarnings(gormdbLayers(t), nil, gormdbNodes())
		want := []string{
			`layer "Internal" gets no band: its packages sit inside folder "gormdb/internal", which layer "Core" owns (layers never split a folder box); add ` + "`flatten: [gormdb/internal]`" + ` to layer what is inside it`,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("warnings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("a collapsed folder chain names every folder in it", func(t *testing.T) {
		layers := []Layer{{Name: "A", Packages: []string{"*"}}, {Name: "B", Packages: []string{"./a/b/c"}}}
		got := bandWarnings(layers, nil, nodes("./a/b/c", "./a/b/d", "./e"))
		want := []string{
			`layer "B" gets no band: its packages sit inside folder "a/b", which layer "A" owns (layers never split a folder box); add ` + "`flatten: [a, a/b]`" + ` to layer what is inside it`,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("warnings = %q, want %q", got, want)
		}
	})

	t.Run("half flattened, only the folder still in the way is named", func(t *testing.T) {
		got := bandWarnings(gormdbLayers(t), []string{"./gormdb"}, gormdbNodes())
		want := []string{
			`layer "Internal" gets no band: its packages sit inside folder "gormdb/internal", which layer "Core" owns (layers never split a folder box); add ` + "`flatten: [gormdb/internal]`" + ` to layer what is inside it`,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("warnings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})

	t.Run("a folder no layer lists, and an unnamed layer", func(t *testing.T) {
		layers := []Layer{{Packages: []string{"./cmd"}}, {Packages: []string{"./lib/x"}}}
		got := bandWarnings(layers, nil, nodes("./cmd", "./lib/x", "./lib/y"))
		want := []string{
			`layer 2 gets no band: its packages sit inside folder "lib", which no layer lists (layers never split a folder box); add ` + "`flatten: [lib]`" + ` to layer what is inside it`,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("warnings = %q, want %q", got, want)
		}
	})

	t.Run("a layer that only lists what another claims first", func(t *testing.T) {
		layers := []Layer{{Name: "A", Packages: []string{"./a", "*"}}, {Name: "B", Packages: []string{"*"}}, {Name: "C", Packages: []string{"./a"}}}
		got := bandWarnings(layers, nil, nodes("./a", "./b"))
		want := []string{`layer "B" gets no band: no item at the band level falls to it`, `layer "C" gets no band: no item at the band level falls to it`}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("warnings = %q, want %q", got, want)
		}
	})

	t.Run("a layer listing only typos is left to the unmatched warning", func(t *testing.T) {
		layers := []Layer{{Packages: []string{"*"}}, {Packages: []string{"./typo"}}}
		if got := bandWarnings(layers, nil, nodes("./a")); len(got) != 0 {
			t.Errorf("warnings = %q, want none", got)
		}
	})

	t.Run("no layers", func(t *testing.T) {
		if got := bandWarnings(nil, nil, gormdbNodes()); got != nil {
			t.Errorf("warnings = %q, want none", got)
		}
	})
}

func TestFlattenWarnings(t *testing.T) {
	got := flattenWarnings([]string{"./gormdb", "./gormdb/sqlite", "./typo", "./gormdb/internal"}, gormdbNodes())
	want := []string{
		`flatten entry "gormdb/sqlite" is a package with no subfolders, so there is nothing to dissolve`,
		`flatten entry "typo" matches no folder`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("warnings = %q, want %q", got, want)
	}
}

func TestDuplicateWarnings(t *testing.T) {
	layers := []Layer{
		{Name: "Top", Packages: []string{"./a", "./a", "*"}}, // repeated in one layer: harmless
		{Packages: []string{"./a", "*", "./b"}},
	}
	got := duplicateWarnings(layers)
	want := []string{
		`"a" is listed in both layer "Top" and layer 2; the first one wins`,
		`"*" is listed in both layer "Top" and layer 2; the first one wins`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("warnings = %q, want %q", got, want)
	}
}

func TestParseFlatten(t *testing.T) {
	cfg, err := parseConfig([]byte("flatten: [gormdb, ./gormdb/internal/, a//b]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"./gormdb", "./gormdb/internal", "./a/b"}; !reflect.DeepEqual(cfg.Flatten, want) {
		t.Errorf("Flatten = %q, want %q", cfg.Flatten, want)
	}
}

func TestBuildEmitsFlatten(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(mod, configFile), "flatten: [sub]\n")

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if want := []string{"./sub"}; !reflect.DeepEqual(g.Flatten, want) {
		t.Errorf("Flatten = %q, want %q", g.Flatten, want)
	}
}
