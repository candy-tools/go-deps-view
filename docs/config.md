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
- Layers order items that share a parent folder, and a folder box is never split.
  Listing `tagging/storetest` positions it inside the `tagging` box, relative to its
  siblings there. Only layers holding top-level items get a band.
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
