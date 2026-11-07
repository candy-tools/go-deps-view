# go-deps-view — What this app is for

go-deps-view renders a Go module's package dependency graph in the browser: the
entrypoint at the top, packages grouped into nested folder boxes, arrows following
the imports, and the external libraries off to the side. The compacted
implementation decisions live in [docs/agents/](docs/agents/) — read the file that
matches your task:

| Task | Read |
|---|---|
| Layering, packages, the graph model and JSON contract | [docs/agents/architecture.md](docs/agents/architecture.md) |
| Writing/running tests, `make verify`, coverage, lint | [docs/agents/testing.md](docs/agents/testing.md) |
| Releases, builds, packaging (deb/cask), versioning | [docs/agents/releasing.md](docs/agents/releasing.md) |

## What it does

1. Shell out to `go list -deps -json ./...` in a target module directory.
2. Keep the in-module packages as graph **nodes** and their in-module imports as
   **edges**; resolve every other non-stdlib import to its **library** module and
   collapse it to one edge per (package, library). Standard-library imports are
   dropped.
3. Serve a single, dependency-free HTML viewer (embedded in the binary) plus the
   graph JSON, which is rebuilt from the module on every request. All folder nesting
   and layering is derived by the viewer from the package paths, and an optional
   `.go-deps-view.config` (YAML) in the scanned directory, bootstrapped by `-init`, can
   define named top-to-bottom layers, what starts hidden, and which support code (test
   helpers and the like) a checkbox can leave out. A package's README, if its folder
   has one, is served rendered as HTML for the dialog behind its info button.
4. On request, expand a package to show its exported symbols, with an arrow from each
   importing package to the symbols it references. That data comes from type checking
   the module, so it has its own endpoint, fetched the first time a package is expanded.

## Ground rules that follow

- The Go side only produces the graph model; it never lays anything out. Layout,
  folder boxes, layering, and arrow routing are entirely the viewer's job.
- The graph is built on the fly per request — there is no cache or persisted state.
- The tool shells out to the Go toolchain, so `go` must be on `PATH` at runtime.
- Collections in the JSON are always arrays, never `null`, so the viewer can iterate
  unconditionally.
