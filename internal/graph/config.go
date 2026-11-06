package graph

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	// configFile is read from the scanned directory. It is YAML holding the layers,
	// top first, and the info texts. Applying the layers is the viewer's job; the
	// graph only carries them.
	configFile = ".go-deps-view.config"
	// restEntry is the package entry standing for everything no other entry names.
	restEntry = "*"
)

// Layer is one configured top-to-bottom layer of the graph.
type Layer struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	// Entrypoint marks the layer's packages as entry points, like a program.
	Entrypoint bool `json:"entrypoint" yaml:"entrypoint"`
	// Packages holds node IDs of packages or folders, or "*" for everything not listed.
	Packages []string `json:"packages" yaml:"packages"`
}

// config is the decoded config file.
type config struct {
	Layers []Layer `yaml:"layers"`
	// Info maps a package path or a library's module path to its info text.
	Info map[string]string `yaml:"info"`
}

// applyConfig sets g.Layers and the info texts from dir's config file, if it has
// one. Entries that match nothing are probably typos, but they can also be packages
// dropped by -exclude, so they are reported without failing the build.
func applyConfig(g *Graph, dir string) error {
	data, err := fs.ReadFile(os.DirFS(dir), configFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return fmt.Errorf("%s: %w", configFile, err)
	}
	g.Layers = cfg.Layers
	for _, entry := range unmatchedEntries(g.Layers, g.Nodes) {
		fmt.Fprintf(os.Stderr, "go-deps-view: %s: %q matches no package\n", configFile, entry)
	}
	for _, key := range applyInfo(g, cfg.Info) {
		fmt.Fprintf(os.Stderr, "go-deps-view: %s: info for %q matches no package or library\n", configFile, key)
	}
	return nil
}

// parseConfig decodes the config strictly, so a misspelled key is an error rather
// than a setting that silently does nothing, and normalises the package entries.
func parseConfig(data []byte) (config, error) {
	var cfg config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) { // EOF: an empty file
		return config{}, err
	}
	for i, layer := range cfg.Layers {
		if len(layer.Packages) == 0 {
			return config{}, fmt.Errorf("layer %d (%q) lists no packages", i+1, layer.Name)
		}
		for j, entry := range layer.Packages {
			cfg.Layers[i].Packages[j] = packageEntry(entry)
		}
	}
	return cfg, nil
}

// applyInfo sets the info text on the package or library each key names and returns
// the keys naming neither, sorted. A key is a library's module path or a package
// path; a folder is not enough, as the text describes one package.
func applyInfo(g *Graph, info map[string]string) []string {
	targets := map[string]*string{}
	for i := range g.Nodes {
		targets[g.Nodes[i].ID] = &g.Nodes[i].Info
	}
	for i := range g.Libs {
		targets[g.Libs[i].ID] = &g.Libs[i].Info
	}
	var unmatched []string
	for _, key := range slices.Sorted(maps.Keys(info)) {
		target, ok := targets[key]
		if !ok {
			target, ok = targets[packageEntry(key)]
		}
		if !ok {
			unmatched = append(unmatched, key)
			continue
		}
		*target = info[key]
	}
	return unmatched
}

// packageEntry converts a path as written in the config to the form of a node ID.
func packageEntry(s string) string {
	s = path.Clean(s)
	if s == restEntry || s == "." {
		return s
	}
	return "./" + s
}

// unmatchedEntries returns the package entries that name no package.
func unmatchedEntries(layers []Layer, nodes []Node) []string {
	var out []string
	for _, layer := range layers {
		for _, entry := range layer.Packages {
			matches := func(n Node) bool { return entryMatches(entry, n.ID) }
			if entry != restEntry && !slices.ContainsFunc(nodes, matches) {
				out = append(out, entry)
			}
		}
	}
	return out
}

// entryMatches reports whether entry names the package id or a folder holding
// it. "." names only the root package, not the whole module.
func entryMatches(entry, id string) bool {
	return entry == id || (entry != "." && strings.HasPrefix(id, entry+"/"))
}
