package graph

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadmeFile(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{name: "none", files: []string{"x.go", "notes.md"}},
		{name: "markdown", files: []string{"x.go", "README.md"}, want: "README.md"},
		{name: "case is ignored", files: []string{"Readme.MD"}, want: "Readme.MD"},
		{name: "plain", files: []string{"README"}, want: "README"},
		{name: "markdown is preferred", files: []string{"README", "readme.markdown", "README.md"}, want: "README.md"},
		{name: "a folder doesn't count", files: []string{"README.md/x.go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				write(t, filepath.Join(dir, f), "x")
			}
			if got := readmeFile(os.DirFS(dir), "."); got != tc.want {
				t.Errorf("readmeFile = %q, want %q", got, tc.want)
			}
		})
	}
	if got := readmeFile(os.DirFS(t.TempDir()), "missing"); got != "" {
		t.Errorf("readmeFile of a missing folder = %q, want none", got)
	}
}

func TestBuildFlagsReadmes(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(mod, "sub", "README.md"), "# sub\n")
	write(t, filepath.Join(mod, "README.md"), "# The project\n") // not the root package's

	g, err := Build(mod, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, n := range g.Nodes {
		if want := n.ID == "./sub"; n.Readme != want {
			t.Errorf("node %q readme = %v, want %v", n.ID, n.Readme, want)
		}
	}
}

func TestReadme(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(mod, "sub", "README.md"),
		"# Sub\n\nDoes **things**.\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")

	got, err := Readme(mod, "./sub")
	if err != nil {
		t.Fatalf("Readme: %v", err)
	}
	html := string(got)
	for _, want := range []string{"<h1>Sub</h1>", "<strong>things</strong>", "<table>"} {
		if !strings.Contains(html, want) {
			t.Errorf("html = %q, want it to contain %q", html, want)
		}
	}
	for _, unwanted := range []string{"<script", "javascript:"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("html = %q, want no %q", html, unwanted)
		}
	}
}

func TestReadmeErrors(t *testing.T) {
	mod := writeFixtureModule(t)
	write(t, filepath.Join(filepath.Dir(mod), "README.md"), "outside the module\n")

	if _, err := Readme(mod, "./mocks"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no README: error = %v, want fs.ErrNotExist", err)
	}
	write(t, filepath.Join(mod, "README.md"), "# The project\n")
	for _, id := range []string{".", "./", "sub/.."} {
		if _, err := Readme(mod, id); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the module root as %q: error = %v, want fs.ErrNotExist", id, err)
		}
	}
	if _, err := Readme(mod, ".."); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("leaving the module: error = %v, want fs.ErrInvalid", err)
	}
	if err := os.Symlink(filepath.Join(filepath.Dir(mod), "README.md"), filepath.Join(mod, "sub", "README.md")); err != nil {
		t.Fatal(err)
	}
	if html, err := Readme(mod, "./sub"); err == nil {
		t.Errorf("symlink leaving the module: got %q, want an error", html)
	}
	// Not ErrNotExist: the lookup must not fall back to the working directory.
	if _, err := Readme(t.TempDir(), "./sub"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("not a module: error = %v, want a module error", err)
	}
}
