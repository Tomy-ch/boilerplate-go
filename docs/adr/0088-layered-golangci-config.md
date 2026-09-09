---
status: accepted
date: 2026-07-04
deciders: [maintainers]
tags: [ci, lint]
---

# ADR-0088: Layered golangci configs, split by who runs them and how often

## Status

accepted

## Context

golangci-lint picks up `.golangci.yaml` automatically when no `--config` flag is given, so whoever
does not pass one — an editor integration, a bare CLI run, a contributor's tooling we do not
control — gets whatever that file happens to hold. The authoritative rule set is substantially
stricter than what a per-save pass can afford: over 50 linters against roughly 20, the full set of
`depguard` rules that guard layer boundaries, and the `forbidigo` patterns. That analysis takes
minutes — measured at 2.5 on an idle developer machine and 17.5 when parallel worktrees have
saturated the host — against a 30-second budget for the minimal set.

A second consumer has appeared since: an AI agent working in the repository, which runs a lint pass
at a checkpoint rather than on every save. Its lifecycle matches neither of the other two. The
editor's pass is per-save and scoped to one package, so it must stay under a second and may only
report what the author can act on in the file in front of them. The agent's pass is deliberate and
repository-wide, so it can afford seconds and wants breadth. Serving both from one file forces a
choice that is wrong for one of them, and the failure is silent: strengthening the shared file for
the checkpoint pass also changes what fires on every save.

What separates the two non-authoritative passes from the authority is not linter speed. It is that
the full gate always runs before merge, so an early pass only earns its place by catching what is
**expensive to fix late** — a violation whose repair reaches other files. A layer boundary crossed
in the wrong direction means moving a package or inverting a dependency; a forbidden `time.Now`
means swapping one call for an already-injected `clock.Clock`. Both fail CI, but only the first
costs anything to discover there.

## Decision

Maintain three golangci-lint configurations, each named for the pass that runs it:

- `.golangci.yaml` — the authoritative gate, run by `make go-lint` and `make go-fix` and by CI. It carries
  the complete set including every `depguard` rule and the `forbidigo` patterns. It holds the
  implicit-discovery name deliberately: a tool that passes no `--config` then gets the **strictest**
  set rather than the weakest, so an unconfigured run cannot report clean on something the gate
  would reject.
- `.golangci-min.yaml` — the per-save pass, which `.vscode/settings.json` points at explicitly via
  `go.lintFlags`. A curated minimal set with a fixed 30-second timeout, because a per-save pass that
  outlives the next keystroke is worse than no pass. **It is not a gate**, and nothing may be added
  to it for the sake of enforcement.
- `.golangci-fast.yaml` — the checkpoint pass, run by `make go-lint-fast`. Carries the `depguard`
  rules whose violation propagates: layer boundaries, dependency direction, and the independence of
  the shared packages. It is not a gate either; it is the cheapest place to learn that a change is
  built in the wrong shape.
Every entry point passes `--config` explicitly anyway, so discovery is a safety net rather than the
mechanism. Any rule that must be enforced belongs in `.golangci.yaml`, whether or not it also appears
elsewhere. The three are ordered by containment:

```txt
.golangci-min.yaml  ⊆  .golangci-fast.yaml  ⊆  .golangci.yaml
```

Each is curated for its own pass rather than derived from the next, so nothing propagates
automatically — but the ordering must hold, in two senses. A linter or `depguard` rule present in a
narrower file is present in every wider one, and a rule that appears in more than one file is never
**stricter** in the narrower: the earlier pass may omit a `deny` entry, never add one the gate lacks.
An early pass that reports something the gate would accept would be teaching a rule that does not
exist.

One declared exception: a linter that reports **intentional, temporary state** rather than a defect
belongs in the early passes and must stay out of the gate. `godox` is the only member today — it
surfaces the `// TODO:` hand-off comments `arch-check` writes for a human to resolve, so making it a
gate would fail CI on a workflow this repository ships. Adding to this exception requires the same
reasoning: the linter must be a hint whose findings are not defects.

Only the editor config carries a fixed timeout. A repository-wide pass grows with the repository, so
a budget written into the config would go stale; the cutoff belongs to whichever entry point runs it
— `GOLANGCI_LINT_TIMEOUT` in the makefile locally, `timeout-minutes` on the `go-lint` job in CI.

## Consequences

### Positive Consequences

- Each pass is tuned for its own cost ceiling: the editor stays responsive, the checkpoint pass stays
  worth running, and CI enforcement is unambiguous.
- Strengthening the checkpoint pass no longer changes what fires on every save.
- Layer-boundary rules reach the agent and the author before CI, which is where they are cheap.

### Negative Consequences

- Three files to maintain, and the containment ordering is not enforced by anything today: the early
  files have drifted from the gate before, silently, in the body of a rule they shared. Until a check
  asserts the ordering, the drift is found by reading.
- A rule added to the full gate is not visible in the editor or the checkpoint pass unless it is
  added there deliberately. That is the intended cost of curation, not an oversight to fix by
  syncing everything.

## Alternatives Considered

### Single shared config

One `.golangci.yaml` used everywhere. Simple to maintain but forces a choice: either use the full
slow suite in editors and at every checkpoint, or weaken CI to the fast subset. Neither is
acceptable.

### Two configs, editor and gate

The arrangement this record originally described. It was sufficient while the only non-CI consumer
was an editor. It stopped being sufficient once a checkpoint pass existed, because the two share
neither a cost ceiling nor a definition of a useful finding, and the shared file silently served the
one that was edited most recently.

### golangci-lint `--fast` flag

The `--fast` flag selects a subset of linters. Does not give the fine-grained control needed to
enforce the project-specific `depguard` rules selectively, and the flag's semantics have changed
across golangci-lint versions.

## Notes

- Source: `.golangci-min.yaml` (editor), `.golangci-fast.yaml` (checkpoint), `.golangci.yaml`
  (gate), and the `lint` / `fix` targets in `.makefiles/go/golangci-lint.mk`.
- The `depguard` rules in `.golangci.yaml` are the machine-enforced expression of the layer
  dependency rules documented in [`docs/rules.md`](../rules.md).
- Related: [ADR-0002](0002-onion-architecture.md) — the layer boundaries that `depguard` enforces.
