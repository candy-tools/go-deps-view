package graph

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// readmeNames are the README file names looked for in a package's folder, in order of
// preference. Case is ignored.
var readmeNames = []string{"readme.md", "readme.markdown", "readme"}

// readmeFile returns the name of the README in the folder dir of fsys, or "" when
// there is none or the folder can't be read.
func readmeFile(fsys fs.FS, dir string) string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return ""
	}
	for _, want := range readmeNames {
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), want) {
				return e.Name()
			}
		}
	}
	return ""
}

// Readme renders the README in the folder of the package id (a node ID) of the module
// holding dir as HTML. Like on a code host, raw HTML and dangerous links in it are
// dropped. A package without a README is an error matching fs.ErrNotExist, and so is
// the root package: the README at the module root describes the whole project.
func Readme(dir, id string) ([]byte, error) {
	rel := path.Clean(id)
	if !fs.ValidPath(rel) { // also refuses "..", so the id can't leave the module
		return nil, fmt.Errorf("readme for %q: %w", id, fs.ErrInvalid)
	}
	if rel == "." {
		return nil, fmt.Errorf("readme for %q: the module root's README is the project's: %w", id, fs.ErrNotExist)
	}
	modDir, err := moduleDir(dir)
	if err != nil {
		return nil, err
	}
	// Reading through a root also stops a symlink from reaching outside the module.
	root, err := os.OpenRoot(modDir)
	if err != nil {
		return nil, fmt.Errorf("reading readme: %w", err)
	}
	defer func() { _ = root.Close() }()
	name := readmeFile(root.FS(), rel)
	if name == "" {
		return nil, fmt.Errorf("readme for %q: %w", id, fs.ErrNotExist)
	}
	src, err := root.ReadFile(path.Join(rel, name))
	if err != nil {
		return nil, fmt.Errorf("reading readme: %w", err)
	}
	var out bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM)).Convert(src, &out); err != nil {
		return nil, fmt.Errorf("rendering readme: %w", err)
	}
	return out.Bytes(), nil
}
