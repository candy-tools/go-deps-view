package graph

import (
	"bytes"
	_ "embed" // for configTemplate
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	// configFile is read from the scanned directory. It is YAML holding the layers,
	// top first, and which packages start hidden or are support code. Applying the
	// layers is the viewer's job; the graph only carries them.
	configFile = ".go-deps-view.config"
	// restEntry is the package entry standing for everything no other entry names.
	restEntry = "*"
	// exampleMarker is the template's line above its commented-out example config.
	exampleMarker = "# Example, to adapt and uncomment:"
)

// configTemplate is what InitConfig writes: a guide to filling in the config.
//
//go:embed config_template.yaml
var configTemplate string

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
	// Hidden lists the packages, folders and library module paths that start toggled off.
	Hidden []string `yaml:"hidden"`
	// Support lists the packages, folders and library module paths that only exist to
	// support or verify the rest, like test helpers, which the viewer can leave out.
	Support []string `yaml:"support"`
}

// InitConfig writes a config template into dir and returns its path. The template
// only holds comments, so it changes nothing until it's filled in. An existing file
// is kept and the error matches fs.ErrExist.
func InitConfig(dir string) (string, error) {
	path := filepath.Join(dir, configFile)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("creating config: %w", err)
	}
	defer func() { _ = root.Close() }()

	f, err := root.OpenFile(configFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("creating config: %w", err)
	}
	_, err = f.WriteString(configTemplate)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("writing config: %w", err)
	}
	return path, nil
}

// applyConfig sets g.Layers and the hidden and support flags from dir's config file,
// if it has one. Entries that match nothing are probably typos, but they can also be packages
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
	for _, entry := range applyHidden(g, cfg.Hidden) {
		fmt.Fprintf(os.Stderr, "go-deps-view: %s: hidden entry %q matches no package or library\n", configFile, entry)
	}
	for _, entry := range applySupport(g, cfg.Support) {
		fmt.Fprintf(os.Stderr, "go-deps-view: %s: support entry %q matches no package or library\n", configFile, entry)
	}
	return nil
}

// parseConfig decodes the config and normalises the package entries. Unknown keys
// are ignored, so a config written for another version, like one still holding the
// dropped `info` key, keeps working.
func parseConfig(data []byte) (config, error) {
	var cfg config
	dec := yaml.NewDecoder(bytes.NewReader(data))
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

// applyHidden flags what each entry names as hidden and returns the entries naming
// nothing.
func applyHidden(g *Graph, entries []string) []string {
	return flagEntries(g, entries, func(n *Node) *bool { return &n.Hidden }, func(l *Lib) *bool { return &l.Hidden })
}

// applySupport flags what each entry names as support and returns the entries naming
// nothing.
func applySupport(g *Graph, entries []string) []string {
	return flagEntries(g, entries, func(n *Node) *bool { return &n.Support }, func(l *Lib) *bool { return &l.Support })
}

// flagEntries sets the flag of what each entry names and returns the entries naming
// nothing. A package entry works as in a layer, so a folder covers the packages
// inside it; a library takes its exact module path.
func flagEntries(g *Graph, entries []string, nodeFlag func(*Node) *bool, libFlag func(*Lib) *bool) []string {
	var unmatched []string
	for _, entry := range entries {
		found := false
		id := packageEntry(entry)
		for i := range g.Nodes {
			if entryMatches(id, g.Nodes[i].ID) {
				*nodeFlag(&g.Nodes[i]) = true
				found = true
			}
		}
		for i := range g.Libs {
			if g.Libs[i].ID == entry {
				*libFlag(&g.Libs[i]) = true
				found = true
			}
		}
		if !found {
			unmatched = append(unmatched, entry)
		}
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
