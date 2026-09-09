# Architecture — layering, packages, the graph model

The user-facing purpose lives in [`../../AGENTS.md`](../../AGENTS.md). This file
distills the design decisions so they can be consulted without re-reading the code.

Related agent docs: [testing.md](testing.md), [releasing.md](releasing.md).

## Layering

```
main.go  (thin entrypoint)
   │
app/cmd            ← flag parsing, -json / -version, dispatch
   ├── app/server  ← http handlers + embedded index.html viewer
   │      │
   │   internal/graph   ← `go list` + the graph model (pure, no HTTP)
   └── app/metainfo     ← linker-stamped version vars
```

- **`internal/graph` is the only place the graph is built.** `Build(dir, exclude)`
  shells out to `go list -deps -json ./...`, resolves in-module packages to nodes and
  edges and external imports to library modules, and returns a `*Graph`. It knows
  nothing about HTTP or flags, so it is unit-tested directly against fixture modules.
- **`app/server` owns the viewer.** `index.html` is colocated here and embedded via
  `//go:embed` so the released binary is self-contained. `Handler(dir, exclude)` serves
  the page at `/` and the graph at `/graph.json` (rebuilt per request); `Serve` wraps it
  in an `http.Server` with a read-header timeout.
- **`app/cmd` is the only frontend.** It parses flags and dispatches to the JSON dump,
  the version print, or the server. The server entrypoint is an injectable package var
  (`serveFn`) so the dispatch is testable without binding a port.
- **`app/metainfo` holds linker-stamped vars only** (`Version`, `BuildTime`, `ShaVer`)
  and is excluded from the coverage gate.

## The graph model (JSON contract)

`Build` returns, and `/graph.json` / `-json` emit:

| Field | Meaning |
|---|---|
| `module` | the scanned module path |
| `nodes` | in-module packages; `id` is the module-relative path (`.` is main) |
| `edges` | `from imports to` (project package → project package) |
| `libs` | external dependency modules; `id` is the module path, `label` is shortened |
| `libEdges` | `package imports library` |

- **In-module vs external:** an import is in-module when it has the module path as a
  prefix and matches none of the `-exclude` substrings; everything else non-stdlib is a
  library, resolved to its `Module.Path`. Standard-library imports are dropped.
- **Excludes** are plain substring matches against the import path (`/testdata`,
  `/mocks`, …). An excluded in-module package is dropped as a node; if something still
  imports it, it then falls through to library resolution (inherited behavior, covered
  by a test).
- **Stable output:** nodes, edges, lib ids, and lib edges are all sorted, and empty
  collections marshal as `[]` rather than `null`.
