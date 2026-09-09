# Testing — strategy, make targets, CI

## Strategy

- `internal/graph` is tested directly: the pure helpers table-test the id/label
  transforms, and `Build` runs against **fixture modules** written to a temp dir —
  including a second local module wired in through a filesystem `replace`, so the
  external-library resolution path is exercised without any network access.
- `app/server` is tested through `httptest`: the page, the graph JSON, a 404, and the
  build-error 500 path.
- `app/cmd` tests `Run(args, stdout, stderr) int` end to end (version, JSON dump, JSON
  error, bad flag) and the serve dispatch via the injectable `serveFn` seam, so no test
  binds a port.

## Make targets

| Target | What it runs |
|---|---|
| `make test` | `go test ./... -cover` — the fast suite |
| `make lint` | `golangci-lint run` |
| `make license-check` | `go-licence-detector` against `allowedLicenses.json` / `overrideLicenses.json` |
| `make benchmark` | `go test -bench=.` across all packages |
| `make coverage` | per-package coverage gate, threshold **70%** (`COVERAGE_THRESHOLD`) |
| `make verify` | all of the above, in that order — the full gate before calling work done |

- Coverage is gated on `./app/...` and `./internal/...`; `app/metainfo` is excluded
  (linker-stamped vars only) and the root `main` package carries no logic. If a package
  falls below threshold, write the missing tests — never lower the threshold.
- There is no `e2e` suite (the tool is small and shells out to `go list`); `make verify`
  is the whole gate.

## Linters (`.golangci.yaml`)

Standard set (errcheck, govet, ineffassign, staticcheck, unused) plus `nolintlint`,
`gocyclo` (min-complexity 20), `nestif` (min-complexity 5), `gosec`, `dupl`.

- Test files (`*_test.go`) are excluded from `nestif`, `dupl`, and `gosec`.
- `errcheck` checks writes to injected `io.Writer`s, so CLI output uses explicit
  `_, _ =` where a write error is deliberately ignored.
- `nolint` directives must name the specific linter and carry an explanation
  (`nolintlint` enforces both). Fix the code instead of silencing the tool.

## CI (`.github/workflows/`)

- `test.yml` — on push to main and on PRs: `make test`, `make coverage`.
- `golangci-lint.yml` / `license-check.yml` — lint and license gates.
- `release.yml` — see [releasing.md](releasing.md).
