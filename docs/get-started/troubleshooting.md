# Troubleshooting

Failures you are likely to meet while setting the repository up and running it locally, indexed by
**the symptom you actually see**. Each entry names the cause and hands off to the document that
owns the mechanism — the explanations live there, not here.

A failure that is *expected* is worth recognising as such: several of the entries below are the
environment doing its job (a fail-closed provider, a gate that defers to CI), and reading them as
bugs costs more time than the failure itself.

## Setup

### A pinned tool is "not found" although `mise` is installed

```txt
make: golangci-lint: No such file or directory
```

`mise` was installed but never **activated in your shell**. Every Make target resolves its tools
through mise's shims, and the shims reach `PATH` only through activation — installing mise is not
enough. Activation, and how to verify it, is Phase 1 of
[setup-repository.md](setup-repository.md).

### Git hooks never run

`lefthook` is installed by `make install-tools`, but the hooks are wired into `.git/` by a separate
step: `make activate-tools`. Until it runs, commits and pushes bypass every local gate — and the
first thing that notices is CI.

### A Docker image build fails with `403 Forbidden`

The tool images resolve their tools with `mise`, which reads the GitHub Releases API. Unauthenticated
calls are capped well below what a single build needs, and each retry consumes the fresh quota. Make
hands the build a token when it can find one (`GITHUB_TOKEN`, otherwise `gh auth token`), so the fix
is to make one available — `gh auth login` is enough. Details:
[local-environment.md](../maintenance/local-environment.md).

## Running locally

### The application exits during startup with `no authorizer configured for environment`

Expected, and not a bug — the message also appears as `no authenticator configured for environment`.
The authentication and authorization providers are wired **only** for `local` / `ci` / `test`; for
`development` / `staging` / `production` they are fail-closed, so the process refuses to start until
real implementations are wired. This is the forcing function that keeps a signature-skipping
authenticator or an allow-all authorizer out of a real environment. Implementing both is Phase 11 of
[setup-repository.md](setup-repository.md); the design is in [auth.md](../design/auth.md).

### A host port is already allocated

The `database` and observability services are **shared by every checkout** — a single instance, not
one per working tree — so a port already in use usually means another checkout brought the shared
infra up rather than that something is broken. Which ports are fixed, which are per-slot
(`8080+N` and friends), and why they sit where they do:
[local-environment.md](../maintenance/local-environment.md). For running several worktrees at once,
lease a slot rather than starting a second stack:
[db-worktree-pool.md](../maintenance/db-worktree-pool.md).

### Generated files became root-owned and `git` can no longer touch them

Code generation and lint run inside tool-runner containers with the repository bind-mounted, and the
process inside runs as root. Recovery commands are in the `repo-ops` skill; the mechanism is
described in [local-environment.md](../maintenance/local-environment.md).

## Database and tests

### Tests fail on missing tables or missing rows

The test suite assumes `make db-init` has run — it migrates **and seeds** both the local and test
databases. A bare migration target brings the schema up without the seed data, which fails later and
further away, in a test that looks unrelated to the DB.

### `make gen-query` fails with `connection refused`

`gen-query` dumps the **live** schema with `pg_dump` before generating, so the database container has
to be running. Start it (`docker compose up -d database`, or `make serve`) and re-run.

### `CREATE DATABASE` fails with a collation version mismatch

```txt
ERROR: template database "template1" has a collation version mismatch (SQLSTATE XX000)
```

The `database` image's base OS changed while the data volume survived. Only the paths that *create* a
database fail; connecting to an existing one merely warns. Re-creating the databases is not a
workaround — the fix is a one-time reindex and collation refresh across the shared instance, and the
command, plus why the shared instance makes this bite twice, is in
[local-environment.md](../maintenance/local-environment.md).

## Gates and generated artifacts

### CI fails on a generated artifact, but the code compiles locally

The generated artifacts are committed, and CI regenerates them and fails on any diff — so a change to
the OpenAPI document or the SQL files that was not followed by `make gen` (or the narrower `make
gen-api` / `make gen-query`) is caught there rather than at build time. Regenerate, commit the
result, and never hand-edit a generated file.

### The editor's lint findings differ from `make go-lint`

By design. `golangci-lint` picks up `.golangci.yaml` implicitly, which is a deliberately minimal set
tuned for editor responsiveness; the authoritative gate is `.golangci.yaml`, which `make go-lint`
and `make go-fix` pass explicitly and which carries the depguard layer rules. An editor that stays quiet
is not evidence. Rationale:
[ADR-0088 (layered-golangci-config)](../adr/0088-layered-golangci-config.md).

### Local gates seem to have stopped running

They were deferred, not lost. How much runs locally is sized from the number of open worktrees, and
past a threshold the heavy gates are left to CI, which re-runs them identically. `make load-status`
prints the resolved band and what each tool will receive; the bands themselves are documented in
[.makefiles/README.md](../../.makefiles/README.md).

### A `vendor/` inconsistency after switching branches

`vendor/` is gitignored, so the checkout that breaks is the one that merely *received* someone else's
`go.mod` change. The `post-checkout` / `post-merge` hooks run `make vendor-sync` for exactly this
reason — run it by hand when the hooks were not active (see *Git hooks never run* above).

## Related documents

- [setup-repository.md](setup-repository.md) — the phased setup this page assumes you are following
- [local-environment.md](../maintenance/local-environment.md) — containers, ports, tool-runners
- [db-worktree-pool.md](../maintenance/db-worktree-pool.md) — running several worktrees against the shared database
- [.makefiles/README.md](../../.makefiles/README.md) — every target, grouped by area
