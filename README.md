# go-deps-view

View a graphical representation of a Go module's package dependencies in the
browser: the entrypoint at the top, packages grouped into nested folder boxes,
arrows following the imports, and the external libraries off to the side.

It shells out to `go list -deps -json ./...` in the target module and serves the
graph to a small, dependency-free browser viewer that is embedded in the binary.
All grouping and layout is derived from the package paths and their imports,
optionally steered by a [layers config](#layers-config).

![go-deps-view rendering its own package dependency graph](docs/screenshot.png)

> **Requires the Go toolchain on `PATH`.** The graph is built by running
> `go list` against the target module on each request.

## Install

```sh
go install github.com/candy-tools/go-deps-view@latest
```

Tagged releases also publish prebuilt binaries for Linux and macOS (amd64 / arm64):

- **Archives** — download a `go-deps-view_<OS>_<arch>.tar.gz` from the
  [releases page](https://github.com/candy-tools/go-deps-view/releases).
- **Homebrew (macOS)** — `brew install --cask candy-tools/tap/go-deps-view`.
- **Debian/Ubuntu** — download the `.deb` from the releases page and
  `sudo dpkg -i go-deps-view_*.deb`.

## Usage

Run it against the current module, or point it at another with `-dir`:

```sh
go-deps-view                       # scan ., serve on http://localhost:8099
go-deps-view -dir /path/to/module  # scan another module
go-deps-view -addr :9000           # different port
go-deps-view -exclude /testdata,/mocks
go-deps-view -json                 # print the graph JSON and exit (no server)
go-deps-view -version              # print version information and exit
```

From a checkout you can run it straight from source with `go run .` in place of
the installed binary. Stop the server with `Ctrl-C`.

## Layers config

By default whatever imports the most ends up on top, which isn't always the real
entrypoint: a folder of examples imports everything, for instance. To define the
layers yourself, add a YAML file named `.go-deps-view.config` to the directory
being scanned:

```yaml
# Layers, top first.
layers:
  - name: Entry point
    description: What consumers import first
    entrypoint: true
    packages: [search]

  - name: Features
    packages: [tagging]

  - name: Core
    packages: ["*"]   # everything not listed; the quotes are required

  - name: Support
    description: Examples and test helpers
    packages: [zarf, langaitest]
```

| Key | Required | Meaning |
|---|---|---|
| `packages` | yes | Folder or package paths relative to the scanned directory. A folder covers everything inside it, `.` is the root package, and `"*"` is everything not listed. |
| `name` | no | Label on the layer's divider. |
| `description` | no | Text shown after the name on the divider. |
| `entrypoint` | no | Gives the layer's packages the accent border that programs have. A folder entry marks every package inside it. |

- Without a `"*"` entry, unlisted packages go below all the layers.
- Inside a layer, importers still sit above what they import. An import that goes
  against the configured order is drawn as an arrow pointing up.
- Layers order items that share a parent folder, and a folder box is never split.
  Listing `tagging/storetest` positions it inside the `tagging` box, relative to its
  siblings there. Only layers holding top-level items get a band and a divider.
- Layers show as alternating background bands split by a solid line. They mark the
  configured layout, so they stay put when you drag packages.
- Each band has a **hide** / **show** button that toggles every package in its layer,
  including nested packages listed under that layer. A layer listing only nested paths
  has no band, so it has no button.
- A misspelled key or invalid YAML shows as an error on the page. An entry that
  matches no package only prints a warning on stderr.

The file also holds notes about individual packages and libraries, under the
top-level `info` key:

```yaml
info:
  internal/graph: Builds the graph from `go list`. Knows nothing about HTTP.
  .: The root package.
  go.yaml.in/yaml/v3: |
    Parses this config file.

    Blank lines and line breaks are kept.
```

Each key is a package path relative to the scanned directory, or a library's module
path. A key must name one package, so a folder without a package of its own doesn't
count. Keys that match nothing print a warning on stderr, like layer entries do.

The file is read on every request, so reload the page to see an edit. To remove
packages from the graph altogether, use `-exclude` instead.

## What you see

- **Folder boxes** — the viewer builds a folder tree from the package paths and
  draws each folder as a nested box (`./internal/model/foo` → a `foo` node in a
  `model` box in an `internal` box). A folder holding just one package collapses
  into a label such as `internal/store`.
- **Layers** — an importer sits above what it imports, so programs end up at the
  top and foundation packages at the bottom, unless a
  [layers config](#layers-config) says otherwise. Programs (`package main`) get an
  accent border.
- **Import arrows** connect individual packages and route around the packages in
  their way, avoiding unrelated folders where they can. Edges crossing a top-level
  folder use a distinct colour.
- **Transitive imports** — an import already implied by a longer path is hidden
  until you hover one of its packages. Turn on the **transitive imports** checkbox
  to see them all, drawn dashed.
- **Libraries** — external dependency modules imported directly by the project
  sit in a box on the right, toggled by the **external libraries** checkbox
  (standard-library imports are always excluded). Their arrows appear on hover.
- **Click to cycle** — clicking a package or library rotates it through highlighted
  (the hover look stays on: all its arrows lit and unrelated nodes dimmed, with the
  box itself amber), hidden (greyed out with no arrows), and back to normal. Several
  nodes can be highlighted at once. **show all** brings back everything hidden. **hide
  all** only affects packages and clears their highlights.
- **Info button** — a package or library with an [`info` text](#layers-config) has
  an **i** button that shows the text in a dialog.
- Hover a node to highlight its neighbours and the edges touching it. Arrows to what
  it imports turn blue and arrows from whatever imports it turn coral, as the key in
  the header shows. A library is only ever imported, so its arrows are all coral.
- **Drag to rearrange** — drag a package or library to move it on its own, or drag
  a box to move everything inside it together. Boxes re-wrap around their contents
  and arrows follow. The arrangement lasts until the page is reloaded.

## JSON output (`-json`)

```json
{
  "module": "example.com/mod",
  "nodes": [
    { "id": ".",         "pkg": "example.com/mod",         "name": "main", "info": "" },
    { "id": "./app/cmd", "pkg": "example.com/mod/app/cmd", "name": "cmd",  "info": "Flag parsing." }
  ],
  "edges":    [{ "from": ".", "to": "./app/cmd" }],
  "libs":     [{ "id": "github.com/gorilla/mux", "label": "gorilla/mux", "info": "" }],
  "libEdges": [{ "from": ".", "to": "github.com/gorilla/mux" }],
  "layers": [
    { "name": "Entry", "description": "", "entrypoint": true, "packages": ["./app/cmd"] },
    { "name": "", "description": "", "entrypoint": false, "packages": ["*"] }
  ]
}
```

- `nodes` — in-module packages; `id` is the module-relative path (`.` is the module
  root), `name` the package clause name (`main` for a program), and `info` the
  configured text, empty when there is none.
- `edges` — `from imports to` (project package → project package).
- `libs` — external dependency modules; `id` is the module path, `label` the
  shortened display name, and `info` the configured text, empty when there is none.
- `libEdges` — `package imports library`.
- `layers` — the configured layers, top first; `packages` entries are in `id` form,
  plus `*`.

Empty collections are emitted as `[]` (never `null`), and standard-library
imports are excluded.

## Project layout

- `main.go` — thin entrypoint.
- `app/cmd` — flag parsing (`-dir`, `-addr`, `-exclude`, `-json`, `-version`) and dispatch.
- `app/server` — the HTTP handlers and the embedded, dependency-free viewer (`index.html`).
- `internal/graph` — the graph builder (shells out to `go list`) and the JSON model.
- `app/metainfo` — build/version metadata stamped in at release time.

## Development

```sh
make verify   # test + license-check + lint + benchmark + coverage (the full gate)
make run      # build + serve on http://localhost:8099 (DIR=/path to scan elsewhere)
make build    # goreleaser snapshot build → ./dist
```

Agent-facing notes live in [`docs/agents/`](docs/agents/) and [`AGENTS.md`](AGENTS.md);
releases are cut with `make tag version="vX.Y.Z"` (see
[`docs/agents/releasing.md`](docs/agents/releasing.md)).
