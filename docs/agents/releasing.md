# Releasing — tags, GoReleaser, packaging

## How a release happens

Push a semver tag from a clean `main` branch:

```bash
make tag version="v1.2.3"
```

`make tag` refuses unless you're on `main` with a clean working tree, then creates and
pushes the annotated `vX.Y.Z` tag (deleting any stale local/remote tag of the same name
first). The tag triggers `.github/workflows/release.yml`, which checks out with full
history (the changelog needs it) and runs `goreleaser release --clean`.

## What GoReleaser produces (`.goreleaser.yaml`)

- **Binaries:** linux + darwin, amd64 + arm64, `CGO_ENABLED=0` (pure Go — the tool
  shells out to `go list`, so nothing needs cgo), `-trimpath`, stripped.
- **Version stamping:** `-X` ldflags set `Version`, `BuildTime`, and `ShaVer` in
  `app/metainfo` — that package holds linker-stamped vars only and is excluded from
  coverage. A plain `go build` produces an unstamped binary (`-version` shows the dev
  defaults).
- **Archives:** `tar.gz` per OS/arch with `uname`-compatible names
  (`go-deps-view_Linux_x86_64.tar.gz`, …), bundling LICENSE and README.
- **`.deb`** (linux only) with a lintian override for the statically-linked binary. The
  Go toolchain is a runtime requirement (the tool shells out to `go list`) but is **not**
  declared as a hard package dependency — the audience already has Go installed.
- **Homebrew cask** published into the **candy-tools/homebrew-tap** repo at
  `Casks/go-deps-view.rb`. Pushing to that separate repo needs a cross-repo PAT
  (`HOMEBREW_TAP_GITHUB_TOKEN`); the default `GITHUB_TOKEN` only reaches this repo. The
  darwin binary isn't code-signed/notarized, so a post-install hook strips the
  `com.apple.quarantine` attribute to avoid a "go-deps-view is damaged" Gatekeeper error.
- **Changelog** excludes `docs:` and `test:` commits.

## Prerequisites (one-time, outside this repo)

- The **candy-tools/homebrew-tap** repository must exist (with a `Casks/` directory).
- A **`HOMEBREW_TAP_GITHUB_TOKEN`** secret (a PAT with write access to the tap repo) must
  be set on this repository's Actions secrets.

## License note

The repository ships **GPL-3.0** (`LICENSE`), and `.goreleaser.yaml` declares the same
SPDX id. (The sibling `dibs` repo carries the identical GPL-3.0 file but mislabels it
`AGPL-3.0` in its goreleaser metadata — not copied here.)

## Local builds

```bash
make build    # goreleaser snapshot build for the current OS/arch → ./dist
make clean    # remove ./dist
```

Snapshot versions are `{next-patch}-snapshot`; no tag or publish involved.
