---
status: accepted
date: 2026-07-04
deciders: [maintainers]
tags: [ci, tooling]
---

# ADR-0089: Local git hooks duplicate the CI contract (local == CI, glob-scoped, bypass-then-verify-once)

## Status

accepted

## Context

Without local hooks, developers only learn about CI failures after pushing — the feedback
loop is minutes long. Repeating the full CI suite on every commit attempt, on the other
hand, would be prohibitively slow and frustrating: running tests, linters, and secret
scans against every staged change regardless of which files changed makes committing
feel expensive.

The project also uses a structured multi-commit workflow (scoped commits via the `commit`
skill), which requires suppressing hooks during the split and running verification exactly
once at the end — meaning the hook system must be bypass-able in a controlled way while
still producing a single, definitive verification pass.

## Decision

Use lefthook (`.lefthook.yaml`) to define local git hooks that mirror the CI contract,
with two design constraints:

**1. Glob-scoped execution.** Each hook command declares a `glob:` (or `glob` list) that
limits it to the file types it is relevant for. A commit that touches only `.md` files
does not run Go lint or tests; a commit that touches only `.go` files skips SQL and
Markdown lint. Commands without a glob run unconditionally (e.g. commitlint on
commit-msg).

**2. Bypass-then-verify-once.** The `commit` skill commits with `--no-verify` during the
split phase to avoid repeated hook overhead on each partial commit, then executes the
lefthook-defined commands directly — plus `make go-fix` — as a final verification gate.
This guarantees CI parity without redundant per-commit overhead.

The hook stages and their commands are:

The stages and their commands are below. The names are the lefthook command names, which is
what `--command` selects — they are not always the `make` target the command runs (`go-lint`
runs `make go-lint`, `go-test` runs `make go-test-cached`).

- `pre-commit` (parallel, 13): `go-lint` (`.go`), `go-test` (`.go`),
  `go-test-scripts` (`scripts/**/*.go`), `sql-lint` (`.sql`), `md-lint` (`.md`),
  `actions-lint` (workflow YAML + action YAML + the node lint scripts),
  `actions-zizmor` (workflow YAML + action YAML + `zizmor.yml`), `oapi-lint` (`openapi/**`),
  `docker-lint` (Dockerfiles),
  `pin-actions` (workflow YAML + action YAML + `actions-pin.toml`),
  `pin-images` (Dockerfiles + compose + `images-pin.toml`),
  `migration-check-version` (`.sql`), `migration-check-gap` (`.sql`).
- `commit-msg`: `commitlint`.
- `pre-push` (parallel, 5): `secret-scan`, `test` (full, no cache, `.go`),
  `go-test-scripts` (`scripts/**/*.go`), `gen-go-check` (generated artifact drift),
  `tidy-check` (`go.mod` / `go.sum`).

## Consequences

### Positive Consequences

- CI failures are surfaced locally before a push, reducing round-trip time.
- Glob scoping keeps individual commits fast: only the relevant checks run.
- The bypass-then-verify-once pattern gives the multi-commit workflow safe hook avoidance
  without silently dropping the verification requirement.
- The hook set covers the full CI contract: lint, tests, secret scan, generated-artifact
  drift, migration sequencing, action pinning, and commit message format.

### Negative Consequences

- Developers can bypass hooks manually (`git commit --no-verify`). This is intentional
  for the structured workflow but relies on discipline to run the final verification, and
  an edit made through the GitHub web UI never meets a hook at all. A gate that exists
  only as a hook is therefore optional in practice, so each one needs a CI counterpart to
  be binding — `md-lint.yaml` is that counterpart for the Markdown checks.
- The final verification is `lefthook run pre-commit`, so it reaches no `commit-msg`
  command: a stage runs only the commands defined under its own name. Commit messages
  written during a split are therefore the one gate the pattern cannot restore, and
  `commitlint.yaml` checks the PR's commit range in CI instead. It is the only gate here
  with no hook counterpart, because a range needs a base to compare against and a base
  exists only once a pull request does.
- Glob matching is file-extension based; a change to a build script that does not touch
  `.go` files still requires Go tests indirectly, but those tests are not triggered by
  the pre-commit glob.
- Pre-push runs the full non-cached test suite and generated-artifact checks, which can
  be slow on large changesets.

## Alternatives Considered

### No local hooks; rely entirely on CI

Simplest setup. Feedback loop is slow (minutes per push), and mistakes accumulate on the
remote branch before detection.

### Run all checks unconditionally on every commit

Maximally safe but prohibitively slow. A Markdown edit should not block on a full Go
test run.

## Notes

- Source: `.lefthook.yaml`.
- A hook command shares its gate definition with CI through a `make` target rather than
  restating the tool invocation, so the two cannot drift. `actions-zizmor` runs the offline
  audits only; the online ones need a GitHub token and stay in CI, which makes the hook a
  fast subset rather than an exact copy.
- The bypass-then-verify-once pattern is described in `CLAUDE.md` under "Commit / PR
  execution".
- Migration gap and version checks enforce the rules documented in
  [`docs/rules.md`](../rules.md).
