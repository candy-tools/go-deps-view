# Configuration

## Layers

By default whatever imports the most ends up on top, which isn't always the real
entrypoint: a folder of examples imports everything, for instance. To define the
layers yourself, add a YAML file named `.go-deps-view.config` to the directory
being scanned. `go-deps-view -init` creates one holding only comments: a guide to
filling it in, written so an agent can follow it, and an example. It never overwrites
an existing file.

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
| `name` | no | The layer's label in the gutter left of the graph. |
| `description` | no | Text shown under the name, wrapped to fit the gutter. |
| `entrypoint` | no | Gives the layer's packages the accent border that programs have. A folder entry marks every package inside it. |

- Without a `"*"` entry, unlisted packages go below all the layers.
- Inside a layer, importers still sit above what they import. An import that goes
  against the configured order is drawn as an arrow pointing up.
- Layers order items that share a parent folder, and
  [a folder box is never split](#flatten). Listing `tagging/storetest` positions it
  inside the `tagging` box, relative to its siblings there. Only layers holding
  top-level items get a band, or, when everything sits in one folder, items directly
  inside it. A layer that gets no band prints a warning saying which folders to
  flatten.
- An entry listed in more than one layer belongs to the first, and prints a warning.
- Layers show as alternating background bands split by a solid line, with their text
  in a gutter left of the graph. A band grows if its text needs the room. Bands mark
  the configured layout, so they stay put when you drag packages, and packages can't
  be dragged over the gutter.
- Each layer's gutter text has a **hide** / **show** button that toggles every package
  in the layer, including nested packages listed under it. A layer listing only nested
  paths has no band, so it has no button.
- Invalid YAML shows as an error on the page. Unknown keys are ignored, so a
  misspelled one silently does nothing. An entry that matches no package only prints
  a warning on stderr.

The file is read on every request, so reload the page to see an edit. To remove
packages from the graph altogether, use `-exclude` instead.

## Flatten

Bands are made of the top-level items. A module that keeps everything under one
folder, as libraries often do (`gormdb/...`, `pkg/...`), has a single top-level box,
so there the bands are made of what that folder holds, and its box runs through them.

Below that level, layers never split a folder box: packages inside a folder stay in
its box, in its band. The top-level `flatten` key dissolves folder boxes: a listed
folder's own package and its subfolders move up into the parent folder, as if the box
weren't there. Take `github.com/go-bumbu/db`:

```
gormdb                      core: Cfg, Open, driver interfaces
gormdb/sqlite               driver
gormdb/sqlitecgo            driver
gormdb/postgres             driver
gormdb/internal/sqliteerr   helper shared by the SQLite drivers
gormdb/internal/sqlitetest  shared test suite (support)
gormdb/gormdbtest           test helper (support)
```

```yaml
flatten: [gormdb/internal]
layers:
  - name: Drivers
    packages: [gormdb/sqlite, gormdb/sqlitecgo, gormdb/postgres]
  - name: Core
    packages: [gormdb, "*"]
  - name: Internal
    packages: [gormdb/internal/sqliteerr]
  - name: Test helpers
    packages: [gormdb/gormdbtest, gormdb/internal/sqlitetest]
support: [gormdb/gormdbtest, gormdb/internal/sqlitetest]
```

Everything sits in `gormdb`, so its box spans the four bands. Flattening
`gormdb/internal` lets `sqliteerr` and `sqlitetest` sit in different layers. Without
it, `go-deps-view` warns that Internal gets no band. To drop the `gormdb` box as
well, flatten it too: `flatten: [gormdb, gormdb/internal]`.

- Entries are folder paths, like layer entries. `.` and `"*"` are not allowed, and an
  entry naming no folder, or a package without subfolders, prints a warning.
- Nested entries dissolve bottom-up, whatever order they are listed in.
- A moved item's label is its path relative to the nearest box still drawn:
  `gormdb/sqlite` shows as `sqlite`, `gormdb/internal/sqliteerr` as
  `internal/sqliteerr`. A top-level item keeps its plain name, unless a sibling
  shares it, and then both show their full path.
- Layers, `hidden` and `support` keep naming packages by their real paths.

## Hidden packages

The top-level `hidden` key lists what starts toggled off, as if clicked until hidden:

```yaml
hidden:
  - internal/mocks              # a package
  - examples                    # a folder covers everything inside it
  - github.com/stretchr/testify # a library, by its module path
```

**show all** and clicking bring them back until the page is reloaded. Entries that
match nothing print a warning on stderr too.

## Support packages

The top-level `support` key flags code that only exists to support or verify the rest,
like a `cachetest` package that checks implementations of an interface, test helpers,
fakes or examples. It takes the same entries as `hidden`:

```yaml
support:
  - internal/cache/cachetest  # a package
  - internal/testutil         # a folder covers everything inside it
```

Unchecking the **support packages** checkbox takes them out of the graph, along with
their arrows and the libraries only they import, and lays out the rest without them.

## Package notes

Notes about a package don't go in the config file: write a `README.md` (or
`README.markdown` or `README`, in any case) in the package's folder, and the package
gets an **i** button that shows it, rendered as markdown. Raw HTML in it is left out.
The README at the module root describes the project, so the root package never gets
the button. An `info` key left in an older config is ignored.
