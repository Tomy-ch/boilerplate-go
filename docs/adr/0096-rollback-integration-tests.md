---
status: accepted
date: 2026-07-04
deciders: [maintainers]
tags: [testing]
---

# ADR-0096: Run infrastructure integration tests against a real DB with sentinel-error rollback

## Status

accepted

## Context

Repository (infrastructure) tests must verify behavior against real SQL and real PostgreSQL
semantics — uniqueness constraints, SQLSTATE error codes, transaction isolation, and query
shape all behave differently in an in-memory mock than against a real database. At the same
time, tests must not leave state in the database between runs, must support parallel
execution without interference, and must not require heavy setup or teardown per test.

Three candidate strategies exist:

1. Mock the database driver (sqlmock or interface mock) — fast, but does not exercise real SQL.
2. Full database reset between tests — correct, but too slow for a test suite.
3. Run each test inside a transaction and roll it back unconditionally — real DB, zero cleanup cost.

The project's onion architecture (see [ADR-0002](0002-onion-architecture.md)) enforces that
infrastructure is replaceable, but Repository tests must still target the real implementation
to be meaningful.

## Decision

All infrastructure integration tests run against a real PostgreSQL instance. Each test body
that mutates state is wrapped in a transaction that is always rolled back via a sentinel
error (`errRollbackForTest`). No database mocking is used in Repository tests.

The `testkit` package (`internal/infrastructure/rdb/testkit/`) implements this pattern:

- `NewTestDB` — returns a shared singleton `driver.DatabaseDriver` (single connection pool
  per process, initialized with `sync.Once`).
- `NewTestTransactionRunner` — constructs a `TransactionRunner` backed by the production
  `tx.Manager`.
- `WithinTx(fn func(ctx context.Context))` — begins a real transaction, executes `fn`, then
  unconditionally returns `errRollbackForTest` to the transaction manager, which triggers a
  rollback. The sentinel error is caught and treated as success; any other error fails the
  test.

Parallel execution (`t.Parallel()`) is supported at the test level. Transactions are
serialized internally via `txLock sync.Mutex` to prevent concurrent transaction conflicts on
the shared DB connection.

## Consequences

### Positive Consequences

- Tests verify real SQL semantics: query shape, SQLSTATE error codes, constraint behavior,
  and transaction isolation are all exercised against the actual PostgreSQL schema.
- No cleanup between tests — rollback restores the DB state automatically.
- Parallel test scheduling is safe: `t.Parallel()` can be used without data interference.
- Minimal per-test setup: wrap with `WithinTx`, assert with `require`/`assert` inside the
  closure.

### Negative Consequences

- A running PostgreSQL instance is required for both local development and CI. This adds an
  infrastructure dependency to the test environment (`make db-init` must run before `make go-test`).
- `WithinTx` cannot be used for tests that intentionally verify committed state (e.g.,
  background jobs that read data written by a separate transaction). Such tests must manage
  their own teardown.
- Transactions are serialized by the mutex, so DB-level concurrency between simultaneous
  tests is not achievable via this helper.

## Alternatives Considered

### Database driver mocking (sqlmock / interface mocks)

Fast and hermetic, but does not exercise real SQL. Query shape mistakes, SQLSTATE-specific
error branches, and constraint violations are invisible to mock-based tests. Rejected because
the primary goal is to verify real Repository behavior against real PostgreSQL semantics.

### Full database reset between tests

Correct in terms of isolation, but prohibitively slow for a test suite with many Repository
tests. A fixture-based or truncate-based teardown also requires coordinated ordering when
tests run in parallel.

### In-memory database (SQLite)

Avoids a PostgreSQL dependency but introduces schema drift (PostgreSQL-specific types,
functions, and constraints are not available) and obscures SQLSTATE-specific error branches
that `pgerror.NormalizeError` handles. Rejected for incompatibility with the production
schema.

## Notes

- Source: `internal/infrastructure/rdb/testkit/README.md`,
  `internal/infrastructure/rdb/testkit/test_kit.go`.
- The rollback mechanism relies on `tx.Manager.Do` treating a non-nil return from its
  callback as an error that triggers rollback. `errRollbackForTest` is a private sentinel
  recognized and suppressed by `WithinTx`; it is never surfaced to the test as a failure.
- Tests asserting on values produced inside `fn` must use `require`/`assert` inside the
  closure, because `WithinTx` does not propagate `fn`'s return value.
- Infra test coverage target: ≥ 85% (per [`.claude/skills/scaffold-infra-db/SKILL.md`](../../.claude/skills/scaffold-infra-db/SKILL.md); the repo-wide bar in [`docs/rules.md`](../rules.md) is > 90%).
