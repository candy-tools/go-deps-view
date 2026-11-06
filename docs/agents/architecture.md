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
| `nodes` | in-module packages; `id` is the module-relative path (`.` is the root), `name` the package clause name (`main` marks a program), `info` the configured text or empty |
| `edges` | `from imports to` (project package → project package) |
| `libs` | external dependency modules; `id` is the module path, `label` is shortened, `info` the configured text or empty |
| `libEdges` | `package imports library` |
| `layers` | configured layers, top first: `name`, `description`, `entrypoint`, and `packages` (node ids or folder paths in id form, or `*`) |

- **Layers config:** `Build` reads the YAML file `.go-deps-view.config` from the scanned
  directory on every call (`internal/graph/config.go`). The Go side only parses and passes
  the layers along. A missing file means `[]`. An unreadable or invalid file fails the
  build, and decoding is strict so a misspelled key is an error. An entry matching no
  package only warns on stderr, because `-exclude` can legitimately cause it.
- **Info config:** the same file's `info` map holds a text per package or library, set
  on the matching node or lib by `applyInfo`. A key must name one exact package or
  library, never a folder. The viewer gives a node with text an `i` button that opens
  a native `<dialog>`, and widens that node by `INFO_W` to fit it, so a config without
  `info` lays out unchanged.
- **Build errors reach the page:** the viewer shows the `/graph.json` error body, so a
  config mistake is visible without looking at the server's output.
- **In-module vs external:** an import is in-module when it has the module path as a
  prefix and matches none of the `-exclude` substrings; everything else non-stdlib is a
  library, resolved to its `Module.Path`. Standard-library imports are dropped.
- **Excludes** are plain substring matches against the import path (`/testdata`,
  `/mocks`, …). An excluded in-module package is dropped as a node; if something still
  imports it, it then falls through to library resolution (inherited behavior, covered
  by a test).
- **Stable output:** nodes, edges, lib ids, and lib edges are all sorted, and empty
  collections marshal as `[]` rather than `null`.

## The viewer's layout

`layoutPackages` in `index.html` computes each package's home position once per graph;
drags are offsets on top of it. Folder containment is a hard guarantee. "Importer above
imported" is too, except where configured layers explicitly override it. Everything
else is optimised around them.

1. **Backbone:** an import reachable through a longer path is `implied`. It neither
   shapes the layout nor shows by default, which removes most of the visual noise.
2. **Rows:** inside each folder, items are layered by the imports between them, with
   dependency-free items at the bottom. A folder stacks as one block, which keeps large
   modules compact. Global rows were tried and rejected: tall folders end up side by
   side, which doubled the width of a 63-package module. Configured layers work inside
   this scheme: an item takes the layer of its closest listed path, siblings stack by
   layer, and only imports between siblings in the same layer order them. That is why
   layers can't split a folder, and why a config-free graph lays out unchanged. A package
   is an entry point if its layer has `entrypoint` set, using that same closest-path
   lookup. Layers with top-level items get a band, tinted on every other layer, topped
   by a solid line and label in extra space (`LAYER_GAP`) added at the boundary. Bands sit
   at home positions and ignore drags. The svg has no `viewBox` and is at least as big as
   its container, so percentage-sized bands reach the window's edges on a small graph.
   A band's hide/show button toggles its layer's packages by the closest-path lookup, not
   by what sits in the band, so a nested package follows the layer it is listed under.
3. **Folder cycles:** Go forbids package cycles but not folders importing each other.
   Such items (a strongly connected component) can't stack, so they share a band, side
   by side, with their packages interleaved by the package-level imports.
4. **Wrapping:** a level wider than the target wraps onto more rows, which is safe
   because nothing in a level imports anything else in it.
5. **Ordering:** seeded multi-start barycentre plus adjacent swaps, scored by crossings
   of the implied-free straight-line edges. Items only permute inside their parent, so
   boxes can't split or overlap. The budget derives from graph size, never a clock, so
   a module always lays out the same way. The seed swings results noticeably (99–140
   crossings on one module), so don't judge a change from one module's count.
6. **Columns:** siblings need separating only if their row ranges overlap. Compact
   against those constraints, then pull each item toward its neighbours' median.
7. **Routing (`routeEdges`):** works from current positions, so it also runs during
   drags. Arrows pick a gap in each band they cross, avoiding unrelated folders, then
   drop any waypoint a clear curve can skip. Hidden arrows are routed too, so revealing
   them moves nothing. Routes always run from the upper package to the lower one, and
   the path is reversed for an import pointing up (from a layer or a drag), so its
   arrowhead lands on the bottom of the imported package.
8. **Highlighting (`highlight`):** a lit node's arrows are coloured by direction, for
   packages and libraries alike: blue when leaving it, coral (`.edge.hi.in`) when reaching
   it. An arrow between two lit nodes counts as leaving.
