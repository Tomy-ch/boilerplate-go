---
status: accepted
date: 2026-07-04
deciders: [maintainers]
tags: [ci, testing]
---

# ADR-0090: Total coverage 90% is a CI hard gate, with an exception-governance path

## Status

accepted

## Context

Test coverage percentages are easy to game by writing trivially passing tests. Without a
hard numeric floor enforced in CI, coverage tends to erode over time: new code is merged
without tests, and the bar shifts downward by inertia. On the other hand, chasing 100%
coverage encourages writing contrived tests solely to colour lines — particularly for
unreachable defensive branches in infrastructure or write-once bootstrap code — which
increases maintenance burden without improving confidence.

The project needs a floor that is high enough to be meaningful but paired with a
sanctioned, auditable path for exempting genuinely unreachable branches so that the
number does not become a game.

## Decision

Set the total coverage threshold at **90%** and enforce it as a hard CI gate.

The gate is implemented in `.makefiles/go/test.mk`:

- `COVERAGE_THRESHOLD := 90` is the single source of truth for the floor.
- Excluded packages (`GO_TEST_EXCLUDE`) are `gen`, `cmd`, `mock`, `apperror`,
  and `scripts` — packages that are either generated, DI wiring, or utilities that
  cannot meaningfully be unit-tested in isolation.
- `make go-test-cover-ci` runs tests with `-coverpkg` set to the same filtered package list
  and writes `coverage.out`.
- `make cover-gate` reads `coverage.out`, extracts the total line from
  `go tool cover -func`, and exits non-zero if the total falls below the threshold
  (the `cover-gate` target).

In the CI workflow (`go-test.yaml`, the `Coverage gate` step), the gate runs after the coverage report
is uploaded to octocov:

```yaml
- name: Coverage gate
  run: make cover-gate
```

**Exception governance.** Branches that are structurally unreachable or cannot be
triggered deterministically (e.g. runtime-internal error paths, no-op provider failures)
may be formally exempted. Exemptions are recorded in the affected package's `README.md`
under a designated section (example: `internal/observability/README.md`),
require architect-level sign-off, and are governed by the rule that no contrived tests
or extra production code are added solely to reach them.

## Consequences

### Positive Consequences

- New code cannot be merged without tests; coverage erosion is blocked at the PR boundary.
- The threshold is a single constant — changing it is a one-line diff in the makefile,
  visible in review.
- The exception-governance path makes the hard gate sustainable: legitimate exemptions
  are approved and auditable rather than suppressed with `// nolint`.

### Negative Consequences

- 90% is a total figure; a single well-tested package can mask a poorly tested one. Per-
  package floors are not currently enforced.
- The exception-governance path requires manual record-keeping in README files; it is not
  machine-checked for completeness or freshness.
- Developers working on infrastructure bootstrap code must anticipate the approval
  overhead if they encounter genuinely unreachable branches.

## Alternatives Considered

### No coverage gate

Coverage would be tracked but not enforced. Historical evidence in similar projects shows
that advisory-only coverage declines steadily without a hard gate.

### 100% coverage requirement

Eliminates the exemption path by requiring all lines to be covered. In practice this
leads to test-quality degradation (contrived setups, mocked internals) to satisfy the
number. Rejected in favour of a realistic floor with a governed exception path.

### Per-package thresholds

More granular but significantly harder to configure and maintain. Deferred until there is
evidence that the total-line gate is insufficient.

## Notes

- Source: `.makefiles/go/test.mk` (`COVERAGE_THRESHOLD` / `GO_TEST_EXCLUDE` / the `cover-gate`
  target); `.github/workflows/go-test.yaml` (the `Coverage gate` step);
  `internal/observability/README.md` (the coverage-exception section).
- Coverage testing conventions (test structure, `require` vs `assert`, mocks) are in
  [`docs/testing-conventions.md`](../testing-conventions.md).
- The DoD requiring coverage is in [`docs/rules.md`](../rules.md).
