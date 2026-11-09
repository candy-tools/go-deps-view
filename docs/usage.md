# Using go-deps-view

## Flags

Run it against the current module, or point it at another with `-dir`:

```sh
go-deps-view                       # scan ., serve on http://localhost:8099
go-deps-view -dir /path/to/module  # scan another module
go-deps-view -addr :9000           # different port
go-deps-view -exclude /testdata,/mocks
go-deps-view -json                 # print the graph JSON and exit (no server)
go-deps-view -init                 # write a commented config template and exit
go-deps-view -version              # print version information and exit
```

Stop the server with `Ctrl-C`. The graph is built by running `go list` against the
target module on each request, so the Go toolchain must be on `PATH`.

## What you see

- **Folder boxes** — the viewer builds a folder tree from the package paths and
  draws each folder as a nested box (`./internal/model/foo` → a `foo` node in a
  `model` box in an `internal` box). A folder holding just one package collapses
  into a label such as `internal/store`.
- **Layers** — an importer sits above what it imports, so programs end up at the
  top and foundation packages at the bottom, unless a
  [layers config](config.md) says otherwise. Programs (`package main`) get an
  accent border.
- **Import arrows** connect individual packages and route around the packages in
  their way, avoiding unrelated folders where they can. Edges crossing a top-level
  folder use a distinct colour.
- **Transitive imports** — an import already implied by a longer path is hidden
  until you hover one of its packages. Turn on the **transitive imports** checkbox
  to see them all, drawn dashed.
- **Libraries** — external dependency modules imported directly by the project
  sit on the right, toggled by the **external libraries** checkbox (standard-library
  imports are always excluded). With a [layers config](config.md) a vertical line
  cuts them off from the bands into a column at the window's right edge, only as wide
  as they need, since they belong to no layer; without one they sit in a box. Each sits
  level with the packages importing it, and its arrows appear on hover.
- **Support packages** — with a [`support` config](config.md#support-packages), the
  **support packages** checkbox shows or removes the packages and libraries flagged there.
- **Click to cycle** — clicking a package or library rotates it through highlighted
  (the hover look stays on: all its arrows lit and unrelated nodes dimmed, with the
  box itself amber), hidden (greyed out with no arrows), and back to normal. Several
  nodes can be highlighted at once. **show all** brings back everything hidden. **hide
  all** only affects packages and clears their highlights.
- **Info button** — a package with a README in its folder has an **i** button that
  shows the README, rendered as markdown, in a dialog.
- **Loupe** — the magnifying glass on a package expands it to list its exported
  symbols, and imports of it split into one arrow per symbol the importer references.
  Hover a symbol to see only who references it, or hover a package to see which symbols
  it references. The other packages move aside but keep their order. The symbols come
  from type checking the module, which starts in the background once the graph shows,
  so an expand right after loading can take a moment. The result lasts until you reload.
  - A long signature is cut off; its tooltip has the rest.
  - Arrows to an expanded package show even if the import is transitive, and stay dashed.
  - A blank import references no symbol, so it stays a single arrow to the package.
  - Only references from packages that import the symbol's package directly count. A
    symbol with no arrows may still be reached through a value from another package, or
    from tests, so it isn't necessarily unused.
- **Interfaces** — Go types satisfy interfaces implicitly, so an import arrow can't say
  whether a package consumes an interface or provides an implementation. The loupe's
  data says it: a package implementing an expanded package's interfaces gets violet
  arrows with hollow heads, arriving on the package's right side, one per interface
  it implements. That holds even when the implementer never imports the package. An
  interface row shows how many types implement it, and hovering it lights up its
  users and, in violet, its implementers, beside a card listing both. An import arrow
  from an implementer to a collapsed package is violet too, a moment after the graph
  shows, once the type check is done.
  - Interfaces without methods, constraints and generic types are left out.
- Hover a node to highlight its neighbours and the edges touching it. Arrows to what
  it imports turn blue and arrows from whatever imports it turn coral, as the key in
  the header shows. A library is only ever imported, so its arrows are all coral.
- **Drag to rearrange** — drag a package or library to move it on its own, or drag
  a box to move everything inside it together. Boxes re-wrap around their contents
  and arrows follow. The libraries' column is not a box, so it stays put and its
  libraries move one at a time. The arrangement lasts until the page is reloaded.
