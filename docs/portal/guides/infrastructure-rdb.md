# RDB Infrastructure Guide (`internal/infrastructure/rdb`)

## Role

`internal/infrastructure/rdb` is an **Infrastructure subsystem for using RDB (PostgreSQL)**.

This directory has the following responsibilities.

- Connection management to PostgreSQL
- SQL execution (sqlc)
- Repository / QueryService implementation
- SQL execution logging and tracing (Observability)
- PostgreSQL error normalization
- Conversion between DB nullable types and Go types

Domain / Usecase can use **data persistence and querying without being aware of RDB implementation details**.

## RDB Architecture

This directory is composed of the following layered structure.

```mermaid
flowchart TB
    Usecase --> Repo["Repository / QueryService"] --> Driver["driver (+ pgx query tracer)"] --> DB["PostgreSQL"]
```

The responsibilities of each layer are as follows.

|Layer|Role|
|---|---|
|Repository|Aggregate persistence (implementation of Domain Repository Interface)|
|QueryService|Provides search-specific queries (implementation of Usecase Interface)|
|CommandService|Multi-aggregate atomic writes (write-side counterpart of QueryService)|
|driver|DB connection / transaction management, and SQL logging / tracing via a pgx query tracer|
|PostgreSQL|Actual DB|

The following exist as supporting components.

|Component|Role|
|---|---|
|sqlc|Type-safe query execution code generated from SQL|
|pgerror|PostgreSQL error → application error conversion|
|metrics|Connection pool + query duration/error statistics as Prometheus metrics|
|system_cqrs|System operational queries (health check, etc.)|
|testkit|RDB test utilities (real DB + rollback)|

## Directory Structure

The data-access directories mirror the DML categories under `database/dml/`, one for one —
`repository/`, `query_service/`, `command_service/`, `system_cqrs/` — because a category that
exists in SQL and not in Go (or the reverse) is a category nobody decided on.

The rest are what those four need: `driver/` for connections, transactions and the pgx tracer,
`sqlc/` for generated code, `pgerror/` for normalizing PostgreSQL errors, `metrics/` for pool and
query instrumentation, and `testkit/` for the test harness.

## Repository

Repository is the layer that **implements the Domain Repository Interface**.

Main responsibilities

- sqlc query execution
- Row → Domain entity conversion
- DB error normalization

Important: **Repository does not contain business logic**

See details below.

[repository directory README](repository/README.md)

## QueryService

QueryService is the layer that **provides read-only queries such as search and listing**.

While Repository handles Aggregate persistence,  
QueryService is specialized for search use cases.

Main responsibilities

- Execute search queries
- Convert Row → Domain / DTO
- Normalize DB errors

Important: **Search must be implemented in QueryService, not Repository**

See details below.

[query_service directory README](query_service/README.md)

## sqlc

`sqlc` is a tool that generates Go code from SQL.

In this directory:

- sqlc generated code
- LIKE search helper

are provided.

Generated code is placed at:

`internal/infrastructure/rdb/sqlc/gen`

See details below.

[sqlc directory README](sqlc/README.md)

## driver

`driver` is the **lowest layer that provides DB connection and transaction management**.

Main functions

- DatabaseDriver abstraction
- DB connection pool management
- Transaction management (tx.Manager)
- DBTX interface for sqlc
- SQL logging / tracing via a pgx query tracer (see below)

Important: **Transaction boundaries are managed by the Usecase layer**

See details below.

[driver directory README](driver/README.md)

## SQL Logging / Tracing (pgx query tracer)

SQL logging and tracing are wired at the **pgx connection level** (`ConnConfig.Tracer`) by
`driver.NewQueryTracer`, not in a separate wrapper layer. Repository / QueryService talk to the
driver directly via `driver.New(ctx, db)`; instrumentation is transparent and also covers
transaction-bound queries.

```mermaid
flowchart TB
    Repo["Repository / QueryService"] --> Driver["driver (ConnConfig.Tracer)"] --> DB["PostgreSQL"]
```

The query tracer embeds `otelpgx` for OpenTelemetry spans (with semconv DB attributes) and adds
query logs: **success at Info (with latency), slow queries at Warn, and failures at Error**.

Main functions

- OpenTelemetry span per query (via `otelpgx`, including batch / copy)
- Info log on successful completion (with latency)
- Error log on query failure (`span.RecordError` + log)
- Slow query warning log (threshold: `DB_SLOW_QUERY_WARN_THRESHOLD`)
- Query argument masking (`OBS_MASKED_DB_QUERY_ARGS`)

## PostgreSQL Error Normalization

`pgerror` is a **layer that converts PostgreSQL-specific errors into application errors**.

Repository / QueryService use:

```go
pgerror.NormalizeError(err)
```

to normalize DB errors.

Main conversions

```mermaid
flowchart TB
    A["pgx.ErrNoRows"] -->|→| B["ErrNotFound"]
    C["unique violation"] -->|→| D["ErrConflict"]
    E["connection error"] -->|→| F["ErrUnavailable"]
    G["others"] -->|→| H["ErrInternal"]
```

See details below.

[pgerror directory README](pgerror/README.md)

## metrics

`metrics` is a package that **exposes pgxpool connection pool statistics and per-query duration / error statistics as Prometheus metrics**.

Provides Gauge (connection counts) and Counter (acquire counts, destroy counts, etc.).

See details below.

[metrics directory README](metrics/README.md)

## system_cqrs

`system_cqrs` is a layer that provides **system-operational DB queries** (health check, etc.).

Unlike Repository / QueryService, it handles operational and monitoring queries that do not belong to the business domain.

See details below.

[system_cqrs directory README](system_cqrs/README.md)

## command_service

`command_service` implements a **CommandService** — the write-side counterpart of QueryService
(interface in the Usecase layer alongside QueryService, implementation here). It is reserved for multi-aggregate writes
that require single-transaction atomicity (see [ADR-0032 (lightweight-cqrs)](../../../docs/adr/0032-lightweight-cqrs.md)
/ [ADR-0034 (commandservice-atomicity-criterion)](../../../docs/adr/0034-commandservice-atomicity-criterion.md)),
and the write ordering follows
[ADR-0036 (ordered-pessimistic-row-locks)](../../../docs/adr/0036-ordered-pessimistic-row-locks.md).

<!-- sample-api:replace-begin -->
The category carries one implementation, `command_service/product`, which issues replacement coupons
when a product is discontinued. A write whose target rows can be named by identity is lockable and
therefore decomposes into a usecase composed of Repository calls, which is why the purchase flow does
not use one; the recipients here are defined by a predicate over cart rows, cannot be enumerated
before the write, and have no upper bound, so the write is a set operation rather than a sequence of
per-aggregate saves. It is the worked instance of branch 3 in
[ADR-0034 (commandservice-atomicity-criterion)](../../../docs/adr/0034-commandservice-atomicity-criterion.md).

Its parameters are the issuing conditions rather than a decided aggregate, because the aggregates
cannot exist before the recipients are read. The rule the shape rule protects is kept all the same:
the method reads the recipients and then builds every row through the Domain constructor, so no row
reaches the database without satisfying the aggregate's invariants. Round trips stay at two and do
not grow with the number of coupons.
<!-- sample-api:replace-with -->
<!-- = The category carries no implementation until a write meets the criterion. -->
<!-- sample-api:replace-end -->

A CommandService executes writes on the transaction supplied via the `ctx` (it never opens its own —
the Usecase owns the boundary, nested under `idempotency.Run`) and does **not** emit outbox events
(that is a Usecase responsibility, `system_cqrs` category). Its methods take the decided Domain
aggregate and normalize every sqlc error with `pgerror.NormalizeError`.

**One exception to "take the decided aggregate", and only this one.** When the rows to write are
defined by a predicate with no upper bound, the aggregates cannot exist before the write reads which
ones to build — so the method takes the issuing conditions instead, and builds every aggregate
through its Domain constructor after reading them. What the shape rule protects is that no row
reaches the database unvalidated, and that is what the constructor call preserves; taking the
aggregate is the usual way to get it, not the property itself. A method that could name its rows by
identity does not qualify — it decomposes into Repository calls and never reaches this package.

**What may live here.** Only a write that cannot be expressed as loading an aggregate and saving it:
a relative update, a set-based operation, or one that obtains atomicity without taking a lock.
Anything that can be read-modify-saved belongs on the Repository. Without that line this package
becomes "where I put SQL I want to write directly".

**Conditions are derived, never authored.** A guard enforced in SQL here must restate a domain
invariant that already exists — a guard in a relative-update statement restates the condition the
domain has already checked, and returns that same domain sentinel. It is downstream: a change to the
domain rule obliges a change here, never the reverse. Two independently written copies of one rule
diverge silently the first time only one of them moves. See
[ADR-0032 (lightweight-cqrs)](../../../docs/adr/0032-lightweight-cqrs.md) § Derivation.

## testkit

`testkit` is a **utility for testing using RDB**.

Main functions

- Test DB initialization
- Shared test DB driver provision
- Transaction-based testing (automatic rollback)

Test characteristics

- real DB
- transaction rollback
- parallel execution (Tx is serialized)

See details below.

[testkit directory README](testkit/README.md)

## Design Principles

This RDB subsystem is based on the following design principles.

### 1. Hiding DB Implementation

Domain / Usecase do not depend on:

- sql
- pgx
- sql.DB

### 2. Separation of Responsibility (Repository / QueryService)

- Write / persistence → Repository
- Search / read       → QueryService

### 3. Centralized Transaction Boundary Management

Usecase manages Tx, and Infrastructure does not start Tx.

### 4. DB Error Normalization

PostgreSQL-specific errors are converted into application-wide errors by `pgerror`.

### 5. SQL Type Safety

All SQL execution is performed through `sqlc`.

Exception: PostgreSQL session-configuration commands that `sqlc` cannot model (e.g. `SET LOCAL lock_timeout` issued before a row-locking query) may be run via a direct `Exec`. These are confined to `system_cqrs` and their errors still flow through `pgerror.NormalizeError`.

### 6. Observability

SQL execution tracing is provided by a pgx query tracer wired at the driver connection level
(`otelpgx` spans), with logs on successful completion (Info, with latency), slow queries (Warn),
and failures (Error).

### 7. Test Strategy (Integration-based)

Repository / QueryService / CommandService tests are executed with:

real DB + rollback

This is safely implemented using `testkit`.

Each Repository / QueryService test verifies:

- SQL execution paths — every query / branch the method dispatches
- `pgerror.NormalizeError` application on every sqlc return (raw `pg` / connection errors → `apperror`)
- row → entity conversion (column → field mapping, NULL handling)

A CommandService test additionally verifies:

<!-- sample-api:replace-begin -->
- the atomic write effect on every touched table (e.g. stock decremented, purchase / detail rows
  inserted, `status_id` resolved from a business code) via post-write `SELECT`
<!-- sample-api:replace-with -->
<!-- = - the atomic write effect on every touched table (including any id resolved from a business code) -->
<!-- =   via post-write `SELECT` -->
<!-- sample-api:replace-end -->
- the fail-closed guard (e.g. defensive `WHERE quantity >= :qty` → 0 rows → `ErrConflict`) using a
  stale lock value so the domain check passes but the DB predicate rejects
- constraint-violation normalization (`pgerror.NormalizeError`: FK `23503` → `ErrInvalidArgument`,
  unique `23505` → `ErrConflict`)

Concurrency / lock contention cannot be reproduced with the default `testkit` helper: its `WithinTx` serializes transactions (a single tx rolled back at the end). A branch that only fires under genuinely concurrent connections — e.g. `Claim`'s `lock_timeout` `55P03` (lock_not_available) — needs a dedicated integration test that runs two independent `TransactionManager.Do` calls (two connections / transactions) so one holds the row lock while the other times out.

#### Coverage exceptions (infallible defensive branches)

Domain values are narrowed to the sqlc column width through `pkg/safecast`, so each write site
carries an `if err != nil` that a caller cannot reach once the domain guarantees the range. The
range check itself lives in `pkg/safecast` and is fully branch-tested there; what remains at the
call site is error plumbing, and per
[`testing-conventions.md` §9](../../../docs/testing-conventions.md) it is recorded here rather
than covered with a contrived test.

<!-- sample-api:replace-begin -->
|File|Function|Uncovered branch|Why unreachable|
|---|---|---|---|
|`repository/product/product_repository.go`|`Create`|`safecast.IntToInt32(p.Quantity())` error|`product` validates `quantity` into `[0, math.MaxInt32]`|
|`repository/product/product_repository.go`|`Create`|`safecast.IntPtrToInt32Ptr(p.StockWarningThreshold())` error|`product` validates the threshold into `[0, math.MaxInt32]`|
|`repository/product/product_repository.go`|`Update`|`safecast.IntToInt32(p.Quantity())` error|同上|
|`repository/product/product_repository.go`|`Update`|`safecast.IntPtrToInt32Ptr(p.StockWarningThreshold())` error|同上|
|`repository/product/product_repository.go`|`UpdateStock`|`safecast.IntToInt32(p.Quantity())` error|同上|
|`repository/product/product_repository.go`|`insertImages`|`safecast.IntToInt16(img.DisplaySort())` error|`product` validates `displaySort` into `[1, math.MaxInt16]`|
|`repository/product/product_repository.go`|`syncImages`|`safecast.IntToInt16(img.DisplaySort())` error|同上|
|`command_service/product/product_discontinue_command_service.go`|`IssueDiscontinuationCoupons`|`uuid.New()` error|`crypto/rand` failure only|
|`command_service/product/product_discontinue_command_service.go`|`IssueDiscontinuationCoupons`|`safecast.IntToInt16(...Kind().Code())` error|`DiscountKind` / `ScopeKind` are closed sets whose codes are single digits|
|`command_service/product/product_discontinue_command_service.go`|`IssueDiscontinuationCoupons`|FK 23503 normalization|recipients come from an inner join on `users`, so the reference cannot be missing within the same transaction|
<!-- sample-api:replace-with -->
<!-- = |File|Function|Uncovered branch|Why unreachable| -->
<!-- = |---|---|---|---| -->
<!-- = |(none recorded yet)|||| -->
<!-- sample-api:replace-end -->

A `version` conversion at such a call site is **not** exempt: the domain only requires
`version >= 1`, so an out-of-range version is reachable and is covered by a test. The same applies
<!-- sample-api:replace-begin -->
to the purchase `statusCode` / detail-quantity conversions.
<!-- sample-api:replace-with -->
<!-- = to any other conversion whose domain type admits a narrower range than the column. -->
<!-- sample-api:replace-end -->
