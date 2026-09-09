// Command go-deps-view serves a browser visualization of a Go module's package
// dependency graph.
//
// It shells out to `go list -deps -json ./...` in the target module, emits the
// in-module packages and their import edges plus the external library modules
// they import directly, and serves it to the browser, where folder nesting and
// layering are derived from the package paths. The graph is built on the fly.
//
// Usage:
//
//	go-deps-view                       # scan the current dir, serve on :8099
//	go-deps-view -dir /path/to/module  # scan another module
//	go-deps-view -addr :9000
//	go-deps-view -exclude /testdata,/mocks
//	go-deps-view -json                 # print the graph JSON and exit (no server)
//	go-deps-view -version              # print version information and exit
package main

import "github.com/candy-tools/go-deps-view/app/cmd"

func main() {
	cmd.Execute()
}
