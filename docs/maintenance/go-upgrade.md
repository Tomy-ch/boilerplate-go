# Go Version Upgrade Procedure

This document explains the **procedure for updating the Go version** in this project.

Updating the Go version affects the following.

- Go toolchain
- Dependency packages
- Go tools
- Code generation
- CI
- Docker images

Therefore, update according to the following steps.

## 1. Check Release Notes

Go programming language

Check the Release Notes of the target version.

Main items to check

- Changes to the language spec
- Breaking changes in the standard library
- Changes to `go vet`
- Changes to the toolchain

Example

```text
<https://go.dev/doc/devel/release>
```

## 2. Update `mise.toml` (SSOT) and run sync

This project pins the Go version (along with all other tool versions) in `mise.toml` as the single source of truth. The Go runtime version specifically needs to be mirrored to several files for compatibility with `actions/setup-go` and the `golang:` Docker base image. The `make sync-versions` target handles this propagation.

```toml
# mise.toml
[tools]
go = "1.26.3"
# ...
```

Then run the sync target:

```sh
make sync-versions
```

This updates the following files automatically from `mise.toml`:

- `go.mod` — `go X.Y.Z` directive (read by `actions/setup-go` in CI workflows via `go-version-file: go.mod`)
- `docker/server/Dockerfile` — `FROM golang:X.Y.Z-alpine` lines (builder + tooling stages)
- `docker/tools/Dockerfile` — `FROM golang:X.Y.Z-alpine` lines (go_tools stage)

Commit the resulting changes together with the `mise.toml` bump.

## 3. Update Local Go Environment

This project **requires mise** as the version manager for tools, and the same mise installation can manage the Go runtime locally. After step 2, install the pinned Go runtime:

```sh
make go-update
```

Internally this runs `mise install go`, which reads the `go` value from `mise.toml`.

Verification

```sh
go version
```

### IDE / Editor Integration (VSCode + mise)

When VSCode is launched from Dock / Spotlight, the shell init (where mise is
activated) is not applied, so the Go extension may pick up a stale `go` binary
from the system `PATH`. Use one of the following to keep the editor in sync
with `mise.toml`:

1. **Install the [mise VSCode extension](https://marketplace.visualstudio.com/items?itemName=hverlin.mise-vscode) (recommended)** —
   activates the project's mise environment automatically inside VSCode.
   Already listed in `.vscode/extensions.json` as a recommended extension.
2. **Launch VSCode from a terminal where mise is active** —
   `code /path/to/repo` inherits the shell environment.
3. **Set `go.alternateTools.go` to the mise shim in your VSCode User Settings** —

   ```json
   "go.alternateTools": {
     "go": "${env:HOME}/.local/share/mise/shims/go"
   }
   ```

   Apply this in **User Settings**, not project `.vscode/settings.json`,
   to keep the project portable.

After applying any of the above, restart VSCode and confirm via
**Command Palette → Go: Locate Configured Go Tools** that the active Go binary
matches `mise current`.

## 4. CI uses `go.mod` automatically

GitHub Actions workflows use `actions/setup-go` with `go-version-file: go.mod`. Because step 2's `make sync-versions` already rewrote `go.mod`'s `go` directive from `mise.toml`, no manual workflow edit and no separate `go mod edit -go=...` is required.

## 5. Update dependencies and vendor

This project uses **Makefile task `tidy-lib`** for dependency management.

```sh
make tidy-lib
```

This task executes the following.

- `go mod tidy`
- `go mod vendor`

## 5.5. (Optional) Update Go module dependencies

A Go runtime upgrade is a natural point to also refresh the module dependencies. This step is optional — decide whether to update, and at which level:

- **Latest minor** — `go get -u ./...` updates all direct/indirect deps to the latest minor/patch within the same major.
- **Patch only** — `go get -u=patch ./...` stays within the current minor (safest).
- **Skip** — leave dependencies untouched (Go directive bump only).

`go get -u` never crosses a major version, so major upgrades remain a separate, deliberate task.

If updating:

```sh
go get -u ./...        # or: go get -u=patch ./...
make tidy-lib          # re-run go mod tidy + go mod vendor
```

Then review the `go.mod` diff: keep the `go` directive at the upgraded version and make sure no unintended `toolchain` line was added. The later rebuild / gen / test / lint steps verify the runtime bump and the dependency update together.

This repository has a thick test + lint suite (including real-DB infrastructure tests), so a green run gives high confidence for minor/patch updates — but it is not a guarantee. For runtime-sensitive core deps (DB driver, OpenTelemetry, web framework), skim their CHANGELOG even when green.

## 6. Reinstall Go tools

When the Go runtime is updated, Go tools built against the previous runtime should be rebuilt. Reinstall them via mise:

```sh
make install-tools
```

Main tools installed (versions pinned in `mise.toml`):

- gopls
- golangci-lint
- delve (dlv)
- lefthook
- gotests
- impl
- zizmor
- shellcheck

## 7. Docker images pick up the new Go base via sync

`docker/server/Dockerfile` and `docker/tools/Dockerfile` both use `FROM golang:X.Y.Z-alpine` as the base for stages that need the Go runtime. Step 2's `make sync-versions` already rewrote these `FROM` lines. No manual Dockerfile edit is needed for a Go bump.

The non-Go tools (air, dlv, golangci-lint, etc.) inside these Dockerfiles continue to be installed via `mise install <tool>`, so their versions are also driven by `mise.toml`.

## 7.5. Re-pin the base image digests

`make sync-versions` rewrites the `FROM golang:` **tag**, but the `@sha256:...` digest pinned beside it still points at the OLD Go image. Docker honors the digest, so a build would silently keep using the old image. Re-resolve the pins from the registry so the digest tracks the new tag:

```sh
make pin-images-resolve   # run `docker login` first if Docker Hub returns 429
make pin-images-apply
make pin-images-check
```

**A Go upgrade normally cannot complete this step on the day the release lands.** The new `golang:` tag has no prior lockfile entry, and the image is inside the `PIN_IMAGES_MIN_AGE_DAYS` cooldown (default 14 days). With no aged digest to fall back to, `pin-images-resolve` fails closed rather than adopting a fresh digest or dropping the pin to tag-only, and `pin-images-check` then rejects the surviving stale digest as `未登録`. The lockfile is `docker/images-pin.toml`; the targets and the cooldown knob are documented in `.makefiles/README.md`.

So the upgrade and its base-image pin are coupled: it lands cleanly once the new Go image has aged past the window, or when a human deliberately bootstraps it with `days=0`. That is a decision to raise, not one to take — and the tag/digest mismatch must not be left in the tree either way.

## 8. Rebuild Docker containers

Bumping the Go base image invalidates layers in the Dockerfile. Use the clean (`--no-cache --pull`) variants so the new `golang:` image is actually pulled.

Server containers

```sh
make serve-build-clean
```

Tool containers

```sh
make tool-runners-build-clean
```

## 9. Re-run code generation

Generated code may change due to Go version changes.

```sh
make gen
```

## 10. Run tests

```sh
make go-test
```

or

```sh
go test ./...
```

## 11. Run lint

```sh
make go-lint
```

## 12. Final check

Ensure that all of the following commands succeed.

```sh
make sync-versions
make pin-images-check
make tidy-lib
make install-tools
make gen
make go-test
make go-lint
make serve-build-clean
make tool-runners-build-clean
```

`make sync-versions` is repeated here so that any leftover drift between `mise.toml` and the files derived from it is caught before the upgrade is reported as done.

## Upgrade Checklist

When updating the Go version, check the following.

- [ ] Check Release Notes
- [ ] Update `mise.toml` (`go = "..."`)
- [ ] Run `make sync-versions` (regenerates `go.mod` go directive + Dockerfile FROM)
- [ ] Run `make go-update` (installs Go on host) and confirm `go version`
- [ ] Run `make tidy-lib`
- [ ] (Optional) Decide whether to update Go module dependencies; if yes, run `go get -u[=patch] ./...` + `make tidy-lib` (keep the `go` directive unchanged)
- [ ] Run `make install-tools`
- [ ] Re-pin the base image digests (`make pin-images-resolve` → `apply` → `check`); if `resolve` fails closed on the cooldown, raise the choice instead of forcing it
- [ ] Rebuild Docker containers (`make serve-build-clean`, `make tool-runners-build-clean`)
- [ ] Re-run code generation
- [ ] Run test
- [ ] Run lint
