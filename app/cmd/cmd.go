// Package cmd is the go-deps-view command-line entrypoint: it parses flags and
// either prints the graph JSON, prints version information, or serves the viewer.
package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/candy-tools/go-deps-view/app/metainfo"
	"github.com/candy-tools/go-deps-view/app/server"
	"github.com/candy-tools/go-deps-view/internal/graph"
)

// serveFn is the server entrypoint, indirected so tests can exercise the serve
// dispatch without binding a port.
var serveFn = server.Serve

// Execute runs the command and exits with its status code.
func Execute() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

// Run parses args and dispatches, returning the process exit code. All output
// goes to the provided streams so the command can be tested end to end.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("go-deps-view", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "module directory to scan")
	addr := fs.String("addr", ":8099", "listen address")
	exclude := fs.String("exclude", "", "comma-separated import-path substrings to drop (e.g. /testdata,/mocks)")
	dump := fs.Bool("json", false, "print the graph JSON to stdout and exit (no server)")
	version := fs.Bool("version", false, "print version information and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *version {
		// Writes to the injected stream; a failure to write version info to
		// stdout is not actionable, so the write error is intentionally dropped.
		_, _ = fmt.Fprintf(stdout, "Version:    %s\nBuild date: %s\nCommit sha: %s\n",
			metainfo.Version, metainfo.BuildTime, metainfo.ShaVer)
		return 0
	}

	ex := splitExclude(*exclude)

	if *dump {
		return dumpJSON(*dir, ex, stdout, stderr)
	}

	if err := serveFn(*addr, *dir, ex); err != nil {
		_, _ = fmt.Fprintln(stderr, "go-deps-view:", err)
		return 1
	}
	return 0
}

// dumpJSON builds the graph for dir and writes it as indented JSON to stdout.
func dumpJSON(dir string, exclude []string, stdout, stderr io.Writer) int {
	g, err := graph.Build(dir, exclude)
	if err == nil {
		var data []byte
		data, err = json.MarshalIndent(g, "", "  ")
		if err == nil {
			_, _ = fmt.Fprintln(stdout, string(data))
			return 0
		}
	}
	_, _ = fmt.Fprintln(stderr, "go-deps-view:", err)
	return 1
}

// splitExclude parses the comma-separated -exclude value into trimmed, non-empty
// substrings.
func splitExclude(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
