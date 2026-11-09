# JSON output

## Graph (`-json`)

`go-deps-view -json` prints the graph and exits; the server serves the same data to
the viewer.

```json
{
  "module": "example.com/mod",
  "nodes": [
    { "id": ".",         "pkg": "example.com/mod",         "name": "main", "readme": true,  "hidden": false, "support": false },
    { "id": "./app/cmd", "pkg": "example.com/mod/app/cmd", "name": "cmd",  "readme": false, "hidden": false, "support": false }
  ],
  "edges":    [{ "from": ".", "to": "./app/cmd" }],
  "libs":     [{ "id": "github.com/gorilla/mux", "label": "gorilla/mux", "hidden": true, "support": false }],
  "libEdges": [{ "from": ".", "to": "github.com/gorilla/mux" }],
  "layers": [
    { "name": "Entry", "description": "", "entrypoint": true, "packages": ["./app/cmd"] },
    { "name": "", "description": "", "entrypoint": false, "packages": ["*"] }
  ]
}
```

- `nodes` — in-module packages; `id` is the module-relative path (`.` is the module
  root), `name` the package clause name (`main` for a program), `readme` whether its
  folder has a README, `hidden` whether the config starts it toggled off, and `support`
  whether the config flags it as support code.
- `edges` — `from imports to` (project package → project package).
- `libs` — external dependency modules; `id` is the module path, `label` the
  shortened display name, and `hidden` and `support` as for nodes.
- `libEdges` — `package imports library`.
- `layers` — the configured layers, top first; `packages` entries are in `id` form,
  plus `*`.

Empty collections are emitted as `[]` (never `null`), and standard-library
imports are excluded.

## Symbols (`/symbols.json`)

While the server runs, `/symbols.json` serves the data behind the loupe:

```json
{
  "packages": [
    { "id": "./store", "symbols": [
      { "name": "New",        "signature": "func New(c Config) *Store" },
      { "name": "Store",      "signature": "type Store struct" },
      { "name": "Store.Get",  "signature": "func (*Store).Get(id string) (model.Item, error)" }
    ] }
  ],
  "uses": [{ "from": "./server", "to": "./store", "symbol": "Store.Get" }],
  "implements": [{ "from": "./store/pg", "type": "Store", "to": "./store", "interface": "Backend" }]
}
```
