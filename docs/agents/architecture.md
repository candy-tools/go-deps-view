# Architecture — layering, packages, the graph model

The user-facing purpose lives in [`../../AGENTS.md`](../../AGENTS.md). This file
distills the design decisions so they can be consulted without re-reading the code.

Related agent docs: [testing.md](testing.md), [releasing.md](releasing.md).

## Layering

```
main.go  (thin entrypoint)
   │
app/cmd            ← flag parsing, -json / -init / -version, dispatch
   ├── app/server  ← http handlers + embedded index.html viewer
   │      │
   │   internal/graph   ← `go list` + go/packages, the graph and symbols models (pure, no HTTP)
   └── app/metainfo     ← linker-stamped version vars
```

- **`internal/graph` is the only place the graph is built.** `Build(dir, exclude)`
  shells out to `go list -deps -json ./...`, resolves in-module packages to nodes and
  edges and external imports to library modules, and returns a `*Graph`. It knows
  nothing about HTTP or flags, so it is unit-tested directly against fixture modules.
- **`internal/graph` also builds the symbol detail.** `BuildSymbols(dir, exclude)` type
  checks the module with `golang.org/x/tools/go/packages`. It is separate from `Build`
  because that takes far longer than `go list`, and most page loads never need it.
- **`app/server` owns the viewer.** `index.html` is colocated here and embedded via
  `//go:embed` so the released binary is self-contained. `Handler(dir, exclude)` serves
  the page at `/`, the graph at `/graph.json`, the symbols at `/symbols.json` (both
  rebuilt per request) and a package's rendered README at `/readme?id=`; `Serve` wraps it in an `http.Server` with a read-header timeout.
- **`app/cmd` is the only frontend.** It parses flags and dispatches to the JSON dump,
  the config template, the version print, or the server. The server entrypoint is an injectable package var
  (`serveFn`) so the dispatch is testable without binding a port.
- **`app/metainfo` holds linker-stamped vars only** (`Version`, `BuildTime`, `ShaVer`)
  and is excluded from the coverage gate.

## The graph model (JSON contract)

`Build` returns, and `/graph.json` / `-json` emit:

| Field | Meaning |
|---|---|
| `module` | the scanned module path |
| `nodes` | in-module packages; `id` is the module-relative path (`.` is the root), `name` the package clause name (`main` marks a program), `readme` whether its folder has a README, `hidden` whether the config starts it toggled off, `support` whether the config flags it as support code |
| `edges` | `from imports to` (project package → project package) |
| `libs` | external dependency modules; `id` is the module path, `label` is shortened, `hidden` and `support` as for nodes |
| `libEdges` | `package imports library` |
| `layers` | configured layers, top first: `name`, `description`, `entrypoint`, and `packages` (node ids or folder paths in id form, or `*`) |

- **Layers config:** `Build` reads the YAML file `.go-deps-view.config` from the scanned
  directory on every call (`internal/graph/config.go`). The Go side only parses and passes
  the layers along. A missing file means `[]`. An unreadable or invalid file fails the
  build. Unknown keys are ignored, so a config written for another version (like one
  still holding the dropped `info` key) keeps working; the price is that a misspelled
  key silently does nothing. An entry matching no
  package only warns on stderr, because `-exclude` can legitimately cause it.
- **READMEs, not config:** a package's notes live in a README in its folder
  (`readme.go`; `README.md`, `README.markdown` or `README`, any case), not in the config,
  which dropped its `info` key and now ignores it. `Build` only sets the node's `readme`
  flag from `go list`'s `Dir`, keeping `/graph.json` small. The viewer gives such a node
  an `i` button and widens it by `INFO_W`; the button opens a native `<dialog>` and
  fetches `/readme?id=<node id>`, which `Readme` renders with goldmark (GFM) on the
  server. goldmark's safe default drops raw HTML and dangerous links, which is what lets
  the viewer insert the result as HTML. The id is cleaned and must be an `fs.ValidPath`,
  so it can't leave the module folder; outside a module `moduleDir` errors rather than
  falling back to the working directory. The module root's README is the project's,
  not the root package's, so both `Build` and `Readme` skip it. Libraries have no
  README button.
- **Hidden config:** the same file's `hidden` list is resolved by `applyHidden` to a
  `hidden` flag on each node and lib: package entries match like layer entries, so a
  folder covers its packages, and libraries need the exact module path. Unlike layers it
  needs no closest-path lookup, so the viewer only has to read the flags. It does that
  once per page load, seeding the same set the toggles use; a re-render must not reapply
  them, or "show all" wouldn't survive toggling the libraries.
- **Support config:** the same file's `support` list marks code that only supports or
  verifies the rest (test helpers like a `cachetest` package, fakes, examples). It
  resolves exactly like `hidden` (both go through `flagEntries`) to a `support` flag.
  The viewer's "support packages" checkbox, shown only when something is flagged, swaps
  in a view of the graph without those packages, the imports touching them, and the
  libraries only they import (`withoutSupport`). Unlike the libraries checkbox this
  changes which packages are laid out, so each view has its own cached layout and its
  own copies of the nodes and edges, as a layout stores its results on them; switching
  re-sizes the packages for the current expanded set and reflows (`showView`).
- **Config template:** `InitConfig` (`-init`) writes the embedded
  `internal/graph/config_template.yaml` and refuses to replace an existing file. It is
  all comments, so it changes nothing until filled in. A new config key belongs in its
  guide and its example; a test uncomments the example and decodes it strictly, so a
  typo there can't hide behind the lenient parsing.
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

## The symbols model (JSON contract)

`BuildSymbols` returns, and `/symbols.json` emits:

| Field | Meaning |
|---|---|
| `packages` | every in-module package: `id` (the node id) and `symbols`, each with a `name` (`Build`, or `Graph.Sorted` for a method) and a display `signature` |
| `uses` | `from` references `symbol` in `to` |

- **A use refines an import edge.** It only counts if `from` imports `to` directly, so
  each one has a matching `edges` entry. Calling a method on a value obtained from a third
  package, without importing the method's package, is not a use. So a symbol with no uses
  isn't necessarily dead code.
- **What resolves to what:** objects are matched through `TypesInfo.Uses`, not by
  syntax, so a method call on a value counts. A struct field counts as its type, a method
  of an instantiated generic as the generic's, and an interface's method as the interface
  declaring it, which may be an embedded one. Test files aren't loaded, as in `Build`.
- **Not listed:** exported methods of unexported types, and libraries' symbols.
- **Type errors don't fail the build,** because `Build` succeeds on such a module too: the
  errors go to stderr and the resolved part is returned. A directory with no packages fails.
- **Pinned dependency:** `x/tools` is held at the last release supporting Go 1.25, because
  newer ones raise this module's `go` line.

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
   by a solid line in extra space (`LAYER_GAP`) added at the boundary. Bands sit at home
   positions and ignore drags. The svg has no `viewBox` and is at least as big as its
   container, so percentage-sized bands reach the window's edges on a small graph. A
   layer's text sits in a gutter (`GUTTER_W`) left of the graph, where arrows don't run
   through it. `placeRows` makes a band at least as tall as its wrapped text, and drags
   stop at the gutter. The gutter only exists when layers are configured. A band's
   hide/show button toggles its layer's packages by the closest-path lookup, not by what
   sits in the band, so a nested package follows the layer it is listed under. Libraries
   belong to no layer, so with layers a vertical line (`libCutX`) ends the bands and the
   libraries get a column past it, as narrow as its longest label allows. It sits at the
   window's right edge, or just past the graph if that's wider, so it is the one part of
   the render that depends on the window: a resize re-renders, and `#wrap` reserves its
   scrollbar's width so the size is known before the graph is drawn. Like the bands it
   ignores drags, so a dragged package can cross it. It goes away with the libraries
   checkbox, and without layers the libraries keep their box.
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
9. **Expanding (`toggleExpanded`, `layout.reflow`):** a package's loupe fetches
   `/symbols.json` once per page load, then grows the node to hold a row per symbol.
   `reflow` re-derives the home spots from the new sizes without repeating the ordering
   search: a row becomes as tall as its tallest package, and the columns are re-relaxed.
   So nothing changes rows or order, and collapsing restores the exact earlier layout.
   Node height is `nd._h` everywhere, never `NODE_H`, except for the header row inside a
   node, which is what library arrows and the buttons line up with.
10. **Symbol arrows:** an import of an expanded package becomes a trunk, routed like any
    import but ending beside the package's corner on the importer's side, plus one branch
    per referenced symbol. Sharing the trunk keeps a package using ten symbols from
    drawing ten full-length arrows. Each trunk has its own lane, the one arriving furthest
    out on the outside. `sizeNode` reserves the lanes' room (`_laneRoom`) on both sides, since
    drags can change which side an importer is on, and `nodeRect` counts it, so folder
    boxes wrap the lanes and passing arrows keep out. Trunks heading for one package's
    lanes turn at staggered heights, as do any arrows sharing a gap (`bendY`), and importers
    sharing a row land side by side, so no arrow runs along another. These arrows show even when the import is `implied`, since a
    list of users with holes would mislead, and stay dashed. An import referencing
    no symbol, such as a blank one, stays a plain arrow. Only the arrows into a package
    split: the ones leaving an expanded package still start at its edge, because a use is
    per package, not per function.
