# Tutorial: Build the User Feature From Zero

This tutorial walks you through building a complete onion-architecture feature — the
sample **User** API — starting from an empty slate. It is the *worked example* companion
to the reference docs: where [`architecture.md`](../architecture.md) and
[`rules.md`](../rules.md) describe the rules, this document threads a single feature
through every layer in dependency order and explains **why each step happens when it
does**.

It is deliberately reproducible on a real checkout: the repository ships a script that
deletes the entire User feature, so you can return to zero and rebuild it yourself.

## Who this is for

- Newcomers who want one continuous path through the codebase instead of layer-by-layer
  reference docs.
- Anyone who needs to see **what the scaffold skills actually do** — this document is the
  ground truth they encode, and the place to look when a generated result has to be judged.
- Anyone operating without the `.claude/` scaffold skills, who therefore performs the
  procedure by hand.

> The standard path is not to type this out. The automated equivalent of the whole tutorial is
> `/scaffold-endpoint user` (see [Where to go next](#where-to-go-next)); read this once so you
> can tell whether what it produced is right.

## What you will build

The full-stack User sample: an aggregate with invariants, a repository + read-optimized
query service, an application service, four HTTP endpoints, a CLI job, and the HTTP-boundary
integration tests — plus the OpenAPI and SQL **contracts** they are generated from.

## The constitution: lean A

This repository follows a **lean A** spec policy. Only two layers are spec-driven:

|Layer|Source of truth|
|---|---|
|domain|`docs/spec/domain/<package path>.md`|
|usecase|`docs/spec/usecase/<package path>.md`|
|infrastructure|**derived** from the domain Repository interface + sqlc gen (no spec file)|
|controller|**derived** from the OpenAPI-generated `ServerInterface` + usecase interface (no spec file)|

So you write specs for the *inner* two layers and let the *outer* two layers follow from
generated contracts. Keep this in mind — it explains why Step 1 only produces two spec files.

## Dependency order

Dependencies always point inward, but you *build* contracts-first so generators have
something to consume:

```text
spec (domain + usecase)
   └─ contracts:  OpenAPI  +  DB migration  +  DML SQL
        └─ generate:  make gen-api   make gen-query
             └─ domain  →  infrastructure  →  usecase  →  controller
                  └─ DI wiring  →  integration tests  →  verify
```

Each step below states its **goal**, the **files it touches**, the **convention that makes
it correct** (the "why"), and the **command that proves it**.

---

## Prerequisites

- Toolchain bootstrapped (`mise`), Docker available — the code generators and the DB run in
  containers. See the root [`README.md`](../../README.md) Quick Start.
- The DB container (`database`) running. `make gen-query` dumps the **live** schema, and the
  infra/integration tests need a real database.
- Do this on a **scratch branch** so the reference implementation is one `git restore` away:

```bash
git switch -c tutorial/build-user
```

---

## Step 0 — Reset to zero

The repository declares every file that constitutes the sample in
[`scripts/setup/remove-sample-api/sample-manifest.ts`](../../scripts/setup/remove-sample-api/sample-manifest.ts). One command
deletes them and strips the `sample-api` marker blocks out of the shared DI modules and
`openapi.yaml`, then regenerates and verifies the now-smaller tree:

```bash
# Preview first — lists every path that would be deleted, writes nothing.
DRY_RUN=1 make setup-remove-sample-api

# Actually reset. Runs gen-api → gen-query → fix → lint afterwards.
make setup-remove-sample-api
```

**What it removes** (the manifest is your table of contents for the rest of this tutorial):
`internal/domain/user`, `internal/usecase/user`, the infra package under
`repository/user`, `internal/controller/handler/v1/users`,
`internal/controller/job/usercount`, the three `internal/integration/v1_users*_test.go`
files, the User OpenAPI paths/components, the User DML + migrations + seed, and
`docs/spec/{domain,usecase}/user.md`.

> **Caveat:** the script also removes the `product` / `order` DB stubs (they share the
> "sample" manifest). That is fine for a User-only rebuild — they are unused migrations — but
> be aware the reset is "all samples," not "user only."

Because `gen-query` dumps the live schema, drop the now-removed `users` table from your dev
and test databases so it does not linger in generated models, then regenerate:

```bash
make db-init-local db-init-test
make gen-query
```

At this point the tree compiles and lints **without** the User feature. That is your zero
state. Everything below rebuilds it.

---

## Step 1 — Specs (the inner contract)

**Goal:** declare the domain aggregate and the application service in the two lean-A spec
files. These are the human-authored intent the inner layers will implement.

**Files:**

- `docs/spec/domain/user.md` — Overview / Entity / Cross-field Invariants / Behavior
  Methods / Value Objects / Repository Methods.
- `docs/spec/usecase/user.md` — Overview / Interface / DTOs / Dependencies / Workflow.
- `docs/spec/usecase/user/search.md` — the read-only keyword-search service (its own spec
  because it is a separate query-side use case).

**Why first:** in lean A the domain and usecase implementations are validated against these
specs (`/verify-spec`), and the spec's Entity field table is the soft contract checked
against the SQL migration. Writing them first gives every later step a target. The Entity
section, for example, fixes that `prefectureID` is held as an ID reference only. The `User`
aggregate holds no credential of its own: authentication is delegated to an external OIDC
provider, and the mapping from a token's `(issuer, subject)` to an internal user is owned by
the separate `user_identities` table (see [`docs/design/auth.md`](../design/auth.md)).

**Verify:** none yet (Markdown). If you have the skills, `/verify-spec user` checks format +
cross-references.

---

## Step 2 — Contracts (OpenAPI + database)

**Goal:** define the outer contracts that the generators turn into Go. Nothing in
`internal/` is written by hand for these — you write YAML and SQL.

### 2a. OpenAPI

**Files:**

- `openapi/paths/v1/users.yaml` (list + create), `openapi/paths/v1/users/userId.yaml`
  (get/put/patch/delete), `openapi/paths/v1/users/me.yaml` (authenticated self-retrieval),
  `openapi/paths/v1/users/search.yaml`, `openapi/paths/v1/users/feed.yaml`.
- `openapi/components/schemas/{UserBaseInputRequest,UserResponse}.yaml`, plus the
  `requests/`, `responses/`, and `parameters/` fragments for users and search.
- `openapi/openapi.yaml` — the root document that `$ref`s the above. The sample entries here
  live inside `# sample-api:begin … # sample-api:end` marker blocks under `paths`,
  `components.parameters`, and `components.schemas`.

**Why:** **OpenAPI-first is non-negotiable** (`rules.md`). The handler interface, request,
and response types are *generated* from this; you may not hand-write a handler for an
endpoint that has no contract. Each `operationId` (`GetUsers`, `PostUsers`, `GetUsersSearch`,
…) becomes one method on the generated `ServerInterface`.

### 2b. Database

**Files:**

- `database/migrations/000004_create_users.up.sql` / `.down.sql` — the `users` table
  (`id` UUID PK, `email` UNIQUE, `prefecture_id` FK, address columns, `created_at` /
  `updated_at` / `deleted_at` for soft delete).
- `database/migrations/000017_add_users_table_search_text_column.up.sql` / `.down.sql` — a
  `GENERATED ALWAYS` `search_text` column + a GIN trigram index for keyword search.
- `database/dml/repository/user/*.sql` — the aggregate's CRUD queries
  (`insert_user`, `select_user_by_id`, `select_users`, `update_user`, `count_user`) and its
  keyword search (`select_users_by_keyword`, `count_users_by_keyword`). User keeps both on the
  Repository rather than splitting a QueryService — the read side returns entities, not a
  separate projection.
- `database/seed/000001_users.sql` — sample rows (including a soft-deleted one).

**Why:** **migrations are append-only** — never edit an existing migration; add a new
numbered `NNNNNN_name.up.sql` / `.down.sql` pair. The DML files are the input to sqlc; they
are split by responsibility (repository = aggregate persistence, query_service =
read/projection) because that split is mirrored all the way up the stack.

**Verify:** apply the migrations to your dev/test DBs:

```bash
make db-local-migrate-up
make db-test-migrate-up
```

---

## Step 3 — Generate

**Goal:** turn the contracts into Go. This is the boundary between "what you write" and
"what you must never edit by hand."

```bash
make gen-api     # bundles OpenAPI → openapi.gen.yaml, runs oapi-codegen + mockgen
make gen-query   # dumps schema → merges DML → sqlc generate → fmt
```

**What appears (generated — do not edit):**

- `internal/controller/handler/v1/users/gen/server.gen.go` + `type.gen.go` (and the same
  under `detail/gen`, `search/gen`) — the `ServerInterface` and request/response types.
- `internal/infrastructure/rdb/sqlc/gen/user_repository.gen.sql.go` — type-safe query methods.
- `*_mock.go` for any interface carrying a `//go:generate mockgen` directive (added in the
  next steps; re-run `make gen-api` after you declare them).

**Why:** `**/*.gen.go`, `*.sql.go`, `*_mock.go`, and `openapi.gen.yaml` are generated and
**protected**. You change behavior by changing the *contract* and regenerating, never by
editing the output.

---

## Step 4 — Domain layer

**Goal:** implement the aggregate from `docs/spec/domain/user.md`: the entity, its invariants, behavior
methods, value objects, sentinel errors, constants, and the Repository interface.

**Files (in `internal/domain/user/`):**

- `user_domain.go` — the `User` struct + `New(...)` constructor + getters + behavior methods.
- `constant.go` — `min*/max*` length bounds derived from the spec's field constraints.
- `error.go` — `ErrInvalid<Field>` sentinels + `ErrAlreadyDeleted` etc.
- `email.go` / `postal_code.go` — value objects (`Email` / `PostalCode`) that validate their
  raw string in a factory (`NewEmail` / `NewPostalCode`) and expose it via `Value()`, so an
  invalid form can never be constructed.
- `user_repository.go` — the Repository interface, carrying a `//go:generate mockgen`
  directive (→ `mock/`).
- `*_test.go` — invariants, behavior methods, VO boundaries.

**The conventions that make it correct** (`internal/domain/README.md`):

- **Fields are unexported, exposed via getters.** Outside code cannot bypass an invariant.
- **Pointer getters/setters copy with `ptr.Copy`** so internal state never leaks by
  reference.
- **Validation failures wrap a named sentinel** with `xerrors.Wrap(ErrInvalidEmail, msg)`.
- **The layer is pure:** no `time.Now()`, no `uuid.New()`, no I/O, no context in domain
  logic. Time and IDs arrive as constructor arguments.

The constructor is the shape every aggregate follows — validate, then build:

```go
// internal/domain/user/user_domain.go  (excerpt — see the file for the full body)
func New(id uuid.UUID, firstName /* … */ string, /* … */ ) (*User, error) {
 if id.IsNil() {
  return nil, xerrors.Wrap(ErrInvalidID, "id is required")
 }
 if err := validateProfileFields(firstName /* … */); err != nil {
  return nil, err
 }
 // … updatedAt/deletedAt ordering checks …
 return &User{id: id, firstName: firstName /* … */, building: ptr.Copy(building)}, nil
}
```

Mutations are methods that re-check invariants (e.g. `UpdateProfile`, `MarkAsDeleted` — each
calls `ensureNotDeleted` first). Read the real file: it is the canonical template for any
future aggregate.

**Verify:**

```bash
make gen-api                        # regenerate the repository mock
go test ./internal/domain/user/...
```

---

## Step 5 — Infrastructure layer

**Goal:** implement the domain Repository interface (and the usecase's QueryService
interface) by wrapping the sqlc-generated functions. **No spec file** — this layer is derived
from the interface + sqlc gen.

**Files:**

- `internal/infrastructure/rdb/repository/user/user_repository.go` — implements the domain
  `user.Repository` (Create / FindByID / Update / active listing / count).
- `*_test.go` — integration tests against a **real DB** with transaction rollback (via the
  rdb `testkit`).

**The conventions that make it correct** (`internal/infrastructure/rdb/README.md`,
`pgerror/README.md`):

- **Every sqlc return is normalized through `pgerror.NormalizeError`** so PostgreSQL
  SQLSTATEs become `apperror` values (`pgx.ErrNoRows → ErrNotFound`, unique violation →
  `ErrConflict`). Outer layers never see driver-specific errors.
- **Each method opens a tracer span** (`r.tracer.Start(ctx)` / `defer endSpan()`).
- **sqlc types never leak.** Rows are converted to domain entities (repository) or DTOs
  (query service) before returning. The query service may skip the domain and project
  straight to a DTO.

The repository method shape — span, sqlc call, normalize, convert:

```go
// shape only — see user_repository.go for the real body
func (r *repository) Create(ctx context.Context, u *user.User) error {
 ctx, endSpan := r.tracer.Start(ctx)
 defer endSpan()
 db := gen.New(driver.New(ctx, r.db))
 if err := db.CreateUser(ctx, toCreateParams(u)); err != nil {
  return pgerror.NormalizeError(err)
 }
 return nil
}
```

Register the implementations with `fx.Provide` in
`internal/di/module/infrastructure.go` (a `sample-api` marker block — Step 8).

**Verify:** needs the test DB migrated (Step 2b):

```bash
go test ./internal/infrastructure/rdb/repository/user/...
```

---

## Step 6 — Usecase layer

**Goal:** implement the application service from `docs/spec/usecase/user.md`: orchestrate domain + repository +
boundaries, and return DTOs. No business *rules* are invented here — this layer coordinates.

**Files (in `internal/usecase/user/`):**

- `user_usecase.go` — the `Usecase` interface + `usecase` struct + `New` constructor.
- `search/user_search_usecase.go` + `search/query/…` — the keyword-search service and its
  QueryService interface.
- `mock/…gen.go` — generated mocks for the usecase interfaces (for controller/integration
  tests).
- `*_test.go` — table-driven, domain repositories mocked.

**The conventions that make it correct** (`internal/usecase/README.md`,
`boundary/README.md`):

- **Return DTOs, never domain entities.** Map `*user.User` → `UserView` before returning.
- **Time and transactions come through boundaries**, not the stdlib: `u.clock.Now()` (not
  `time.Now()`) for the current time, `u.txm.Do(ctx, fn)` for transaction boundaries. Every
  non-deterministic or external dependency arrives as a `boundary/` interface. Determinism and
  testability depend on this.
- **The usecase owns the transaction boundary**; the domain knows nothing about `tx`.
- **Orchestrate, don't re-implement rules.** Calling a domain behavior method is fine;
  encoding a new invariant here is not — that belongs in the domain.

Shape of a write use case — obtain time from the clock boundary, then run inside a transaction:

```go
// shape only — see user_usecase.go for the real body
func (u *usecase) CreateUser(ctx context.Context, dto *CreateParamsDTO) (UserView, error) {
 ctx, endSpan := u.tracer.Start(ctx); defer endSpan()
 now := u.clock.Now()                                   // boundary, not time.Now()
 // … u.txm.Do(ctx, func(ctx) { user.New(...now...); u.repo.Create(...) }) …
 return toUserView(entity /* … */), nil                 // DTO out
}
```

**Verify:**

```bash
make gen-api                          # regenerate usecase mocks
go test ./internal/usecase/user/...
```

---

## Step 7 — Controller layer

**Goal:** implement the OpenAPI-generated `ServerInterface` — one handler method per
`operationId` — plus the CLI job. **No spec file** — derived from the generated interface +
the usecase interface.

**Files:**

- `internal/controller/handler/v1/users/v1_users_handler.go` (+ `detail/`, `search/`
  subpackages mirroring the URL structure) — the `server` struct, the `BindHandler`
  constructor, the handler methods, and the presenter functions (`toUserResponse`, …).
- `internal/controller/job/usercount/user_count_job.go` — a CLI batch job (not HTTP): parses
  flags, calls the usecase, logs. No `os.Exit` (the runner owns process exit).
- `*_test.go` — usecase mocked.

**The conventions that make it correct** (`internal/controller/handler/README.md`):

- **`BindHandler(echo, tracerFactory, usecase)`** builds the `server`, then registers via
  `gen.NewStrictHandler` + `gen.RegisterHandlers`. This is the canonical reference snippet in
  the handler README.
- **One method per `operationId`, name-matched 1:1** with the generated `ServerInterface`.
- **The handler body is a pure template:** start span → parse request → call **one** usecase
  method → convert the DTO to the OpenAPI response type → return. No business logic, no infra
  access, no manual status codes (apperror → HTTP status is automatic).
- **HTTP vocabulary stays in the controller.** Convert OpenAPI types to domain types via
  `internal/controller/conv` (e.g. `conv.UUID`) so `http.*` / `openapi_types.*` never reach
  the usecase.

```go
// shape only — see v1_users_handler.go for the real body
func (s *server) GetUsers(ctx context.Context, req gen.GetUsersRequestObject) (gen.GetUsersResponseObject, error) {
 ctx, endSpan := s.tracer.Start(ctx); defer endSpan()
 page, err := paging.NewPageFrom1Based(req.Params.Page, req.Params.PerPage)
 // … list, err := s.uc.<ListMethod>(ctx, …) …
 return gen.GetUsers200JSONResponse(/* mapped DTOs */), nil
}
```

**Verify:**

```bash
go test ./internal/controller/handler/v1/users/...
```

---

## Step 8 — Wire dependency injection

**Goal:** register the new providers/handlers in the Fx modules. These are the shared
`MARKER_FILES` the reset script strips — so re-adding the wiring inside `sample-api` marker
blocks keeps the feature self-contained and removable.

**Files (each edit lives inside `// sample-api:begin … // sample-api:end`):**

- `internal/di/module/controller.go` — `fx.Invoke(users.BindHandler, detail.BindHandler,
  search.BindHandler)`.
- `internal/di/module/usecase.go` — `fx.Provide` the usecase constructors.
- `internal/di/module/infrastructure.go` — `fx.Provide` the repository + query service.
- `internal/di/module/job.go` — register the `usercount` job.

```go
// internal/di/module/controller.go
fx.Invoke(
 health.BindHandler,
 // … core handlers …
 // sample-api:begin
 users.BindHandler,
 detail.BindHandler,
 search.BindHandler,
 // sample-api:end
),
```

**Why:** DI is the only place these layers meet; there is **no business logic here**. The
marker comments are what let `make setup-remove-sample-api` cleanly excise the sample again.

---

## Step 9 — Integration tests

**Goal:** verify the HTTP boundary end-to-end with the usecase mocked — Router → Middleware →
Handler → Presenter.

**Files:** `internal/integration/v1_users_test.go`,
`v1_users_detail_test.go`, `v1_users_search_test.go`.

**Why these are different from Step 5's tests:** they boot a real Echo server via `httptest`
but **mock the usecase** (`internal/integration/README.md`). They are not DB tests — they
prove the HTTP wiring (status codes, JSON shape, path/param parsing, auth header handling),
which mocked unit tests of the handler cannot fully cover. One subtest per `operationId`.

**Verify:**

```bash
go test ./internal/integration/...
```

---

## Step 10 — The event-driven side (optional)

**Goal:** see the other entry point into the usecase layer. Everything so far was driven by an HTTP
request; withdrawal additionally emits a domain event that a **worker** consumes.

Nothing you built in Steps 1–9 changes for this. The withdrawal usecase already writes
`user.withdrawn.v1` to the outbox **inside the same transaction** as the deletion, so the event
cannot be lost, nor emitted for a withdrawal that rolled back. From there:

|#|Piece|File|
|---|---|---|
|①|business `Handler` — decode, classify, call the usecase|`internal/controller/worker/withdrawalarchive/withdrawal_archive_handler.go`|
|②|`Consumer` / `FailureHandler` adapter|`internal/infrastructure/queue/sqs` (constructed in DI, not here)|
|③|`Worker` — bundles name / consumer / handler / failure handler|`internal/controller/worker/withdrawalarchive/withdrawal_archive_worker.go`|
|④|registration|`internal/di/module/worker.go`, inside `sample-api` markers|
|⑤|broker client + adapter config|`internal/di/module/withdrawalarchive.go`|
|⑥|env|`CONSUMER_QUEUE_*` in `env/.env`|

**Why the adapter is built in DI rather than in the worker package:** the controller layer must not
import infrastructure (depguard enforces it), so the worker receives an already-constructed
`Consumer` and never learns which broker it is reading from. Same rule that keeps the HTTP handler
away from a repository.

**Two things worth opening the code for:**

- **Idempotency.** Delivery is at-least-once, so this handler will sometimes run twice for one
  withdrawal. Rather than detect the repeat, the *operation* is made idempotent: the object key is
  derived from the user ID alone and the body is the payload unmodified, so a second run writes
  identical bytes. That is stronger than propagating an idempotency key, which only works if the
  downstream honours it.
- **Selection.** One queue carries every event the outbox emits, so the handler checks the
  `event_type` attribute first and acks anything else without processing it. Treating a foreign event
  as a failure would route every purchase event to the DLQ.

**Verify** — three terminals (`make serve`, `make outbox-relay`, `make worker
NAME=withdrawal-archive`), then withdraw a user. The full recipe, with what to watch at each hop, is
in
[`internal/controller/worker/withdrawalarchive/README.md`](../../internal/controller/worker/withdrawalarchive/README.md).

---

## Step 11 — Full verification

**Goal:** prove the rebuilt feature is correct, formatted, and meets the coverage gate.

```bash
make go-fix    # gofmt + golangci-lint --fix
make go-lint   # golangci-lint (full config)
make go-test   # all tests, no cache; needs the test DB migrated
```

**Coverage:** `make go-test` must not drop coverage; new/modified packages must exceed **90%**
(`make cover-gate` enforces the floor in CI). Generated packages (`gen`, `cmd`, `mock`,
`apperror`, `scripts`) are excluded from the calculation.

If a package is short of 90%, add the missing branch tests before moving on — this is part of
the Definition of Done, not an optional polish.

---

## Recap

|Step|Layer|Key files|The "why" in one line|Verify|
|---|---|---|---|---|
|0|reset|`make setup-remove-sample-api`|The manifest is the feature's table of contents|tree compiles without User|
|1|spec|`docs/spec/{domain,usecase}/user.md`|lean A: only the inner two layers are spec-driven|`/verify-spec user`|
|2|contracts|`openapi/**`, `database/migrations/**`, `database/dml/**`|OpenAPI-first + append-only migrations|`make db-*-migrate-up`|
|3|generate|`make gen-api` / `make gen-query`|Change the contract, never the generated output|files appear under `gen/`|
|4|domain|`internal/domain/user/**`|Unexported fields + `ptr.Copy` + sentinel errors + purity|`go test ./internal/domain/user/...`|
|5|infra|`internal/infrastructure/rdb/repository/user/**`|`pgerror.NormalizeError` + tracer span + no type leak|`go test ./…/user/...` (DB)|
|6|usecase|`internal/usecase/user/**`|DTOs out; time/tx via boundaries; orchestrate only|`go test ./internal/usecase/user/...`|
|7|controller|`internal/controller/handler/v1/users/**`, `job/usercount/**`|One method per operationId; handler is a pure template|`go test ./…/users/...`|
|8|DI|`internal/di/module/*.go`|The only place layers meet; marker blocks keep it removable|`make go-lint`|
|9|integration|`internal/integration/v1_users*_test.go`|HTTP boundary with usecase mocked|`go test ./internal/integration/...`|
|9.5|worker|`internal/controller/worker/withdrawalarchive/**`|A second entry point into the usecase layer; the operation itself is idempotent|`make worker NAME=withdrawal-archive`|
|10|verify|`make go-fix` / `make go-lint` / `make go-test`|90% floor is part of Done|`make cover-gate`|

---

## Where to go next

- **Automate it.** With the `.claude/` skills, this entire flow is
  `/scaffold-endpoint user` (which chains `verify-spec` → `scaffold-domain` →
  `scaffold-infra-db` → `scaffold-usecase` → `scaffold-controller` →
  `scaffold-integration-test`). This tutorial is the manual ground truth those skills encode —
  read it once, then let the skills do the typing.
- **Check for drift.** After any multi-layer change, `/back-prop` and `/arch-check` confirm
  the READMEs and code still agree.
- **Add a second feature.** `product` and `order` already ship as DB-only stubs (see the
  `sample-manifest.ts` manifest); promoting one to a full stack is the natural next exercise.

## Maintenance note

This tutorial references the **real** User implementation rather than transcribing it, so the
canonical files stay the single source of truth. If you rename a layer file or change a
convention, the drift surfaces here too — run `/back-prop` to catch it.
