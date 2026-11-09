# Developing go-deps-view

## Overview

`main.go` is a thin entrypoint. `app/cmd` parses the flags and dispatches,
`app/server` holds the HTTP handlers and the embedded, dependency-free viewer
(`index.html`), and `internal/graph` builds the graph (by shelling out to `go list`)
and the symbols (by type checking the module). `app/metainfo` holds the version info
stamped in at release time.

Design decisions and agent notes live in [AGENTS.md](AGENTS.md) and
[docs/agents/](docs/agents/).

## Prerequisites

- Go 1.25 or newer (the floor in `go.mod`)
- [golangci-lint](https://github.com/golangci/golangci-lint) for `make lint`
- [go-licence-detector](https://github.com/elastic/go-licence-detector) for `make license-check`
- [GoReleaser](https://goreleaser.com) for `make build`
- `bc`, used by `make coverage`

## Common tasks

```sh
make help            # list all targets
make test            # fast go tests
make lint            # golangci-lint
make license-check   # check dependency licenses
make coverage        # per-package coverage gate (70%)
make verify          # the full check: test, license-check, lint, benchmark, coverage
```

Run `make verify` before opening a PR. See [docs/agents/testing.md](docs/agents/testing.md)
for the test strategy and linters.

## Run locally

```sh
make run                    # serve this repo's own graph on http://localhost:8099
make run DIR=/path/to/mod   # scan another module
```

`make run` is `go run .`, so any flag from [docs/usage.md](docs/usage.md) also works
with `go run . <flags>`.

## Build

```sh
make build   # GoReleaser snapshot build for the current OS/arch → ./dist
make clean   # remove ./dist
```

A plain `go build` also works but leaves the version info unstamped.

## Release

From a clean `main`:

```sh
make tag version="v1.2.3"
```

The tag triggers `.github/workflows/release.yml`, which runs GoReleaser to publish the
GitHub release (archives and `.deb`), the Homebrew cask in `candy-tools/homebrew-tap`,
and the `.deb` to the candy-tools APT repo. Releases are built with Go 1.27, not the
`go.mod` floor. Details are in [docs/agents/releasing.md](docs/agents/releasing.md).
