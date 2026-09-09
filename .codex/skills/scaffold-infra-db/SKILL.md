---
name: scaffold-infra-db
description: >-
  Implement the infrastructure (RDB) layer Repository for one feature, derived from the domain Repository interface and the sqlc-generated functions (lean A — no spec file), and register its provider in the DI module. Any method it cannot map to a sqlc function is left as a TODO plus a report. Prerequisites: the domain Repository interface exists and `make gen-query` has run — it does NOT generate SQL or run `make gen-query` itself. Use it once the domain layer for the feature is in place. Standalone-callable; the second scaffold step when chained from `scaffold-endpoint`. Do NOT use it to implement any other layer (`scaffold-domain` / `scaffold-usecase` / `scaffold-controller`).
---

# Scaffold Infra DB

Generate the infrastructure (RDB) layer Repository for a feature. **lean A: no spec file** — Repository is derived from domain Repository IF + sqlc gen functions via naming convention.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- After `scaffold-domain` has created the Repository interface AND `make gen-query` has produced the sqlc gen functions.
- As the second step of `scaffold-endpoint` (auto-chained after `scaffold-domain`).
- Standalone when only the infra layer needs scaffolding.

Do NOT use for:

- Generating SQL or running `make gen-query` (out of scope; precondition).
- Adding a single new method to an existing Repository impl — edit by hand.
- Implementing non-DB infrastructure.

## What This Skill Reads / Writes

**Reads (always)**:

- `internal/domain/<aggregate>/<aggregate>_repository.go` — target interface to implement (method list + signatures)
- `internal/infrastructure/rdb/sqlc/gen/*.gen.go` — available sqlc functions (for mapping derivation)
- `internal/infrastructure/rdb/README.md` — naming convention + impl rules
- `internal/infrastructure/README.md` — infra-layer convention
- `internal/infrastructure/rdb/pgerror/README.md` — error normalization rules (SQLSTATE mapping, single-normalization-point principle)
- `internal/infrastructure/rdb/repository/<sibling>/<sibling>_repository.go` — **de facto reference implementation** (infra READMEs describe principles textually but do not carry full impl snippets, so sibling code is the closest reference; still, if it drifts from README rules the README wins)
- `internal/di/module/persistence.go` — DI registration target

**Writes (with confirmation)**:

- `internal/infrastructure/rdb/repository/<aggregate>/<aggregate>_repository.go`
- `internal/infrastructure/rdb/repository/<aggregate>/<aggregate>_repository_test.go`
- `internal/di/module/persistence.go` (append `fx.Provide(<aggregate>.New)`)

**Triggers (via `make`)**:

- `make go-fix` + `make go-test` — final verification

**Never touches**:

- SQL files or sqlc gen output (read-only references)
- Other aggregates' repository directories
- domain layer

## Preconditions

The skill verifies before any write:

1. `internal/domain/<aggregate>/<aggregate>_repository.go` exists with the Repository interface.
2. `internal/infrastructure/rdb/sqlc/gen/` exists with at least some sqlc gen functions.
3. `internal/infrastructure/rdb/repository/<aggregate>/` does NOT already exist (abort if it does).

If any precondition fails, abort with corrective guidance (`/scaffold-domain`, `make gen-query`, manual cleanup).

> **Environment note:** `make gen-query` dumps the live DB schema via `pg_dump`, so the database must be running first. Use the dedicated make targets — **not raw `docker compose`** — to prepare it: `make serve` (starts the development profile incl. the `database` service) → **`make db-init`** → `make gen-query`. `make db-init` migrates **and seeds** both the local and test DBs in one shot; the integration-style tests this skill writes (`make go-test`) also need the running, **seeded** test DB, so `db-init` (not a bare `db-*-migrate-up`) is the correct setup.
>
> **Toolchain note:** if the final `make go-fix` / `make go-test` (or `make go-lint`) fails on a tool version mismatch (e.g. `golangci-lint` v1-vs-v2 config error), realign the local toolchain with `make install-tools` (`make sync-versions` first if `mise.toml` changed) rather than hand-editing `PATH` — then re-run.

## First Step: Resolve Identity

This skill **MUST call `ask the user explicitly` immediately after invocation** (unless invoked from `scaffold-endpoint` with the aggregate name in context):

- 質問: 「対象 aggregate 名 (PascalCase, e.g., `User`)」 — resolves to `internal/domain/<aggregate-lower>/` + `internal/infrastructure/rdb/repository/<aggregate-lower>/`

## Step 1. Read Inputs

1. Read `internal/domain/<aggregate>/<aggregate>_repository.go` and extract Repository interface method list (each with signature).
2. Read `internal/infrastructure/rdb/sqlc/gen/*.gen.go` and extract sqlc-generated function list (the `Queries` methods).
3. Read `internal/infrastructure/rdb/README.md` for the naming convention (how Repository method names typically map to sqlc gen function names) and impl rules.
4. Read `internal/infrastructure/README.md` for layer rules.
5. Read `internal/infrastructure/rdb/pgerror/README.md` for SQLSTATE → apperror mapping and the single-normalization-point principle (every sqlc call's error MUST flow through `pgerror.NormalizeError`).
6. Read 1 sibling repository (`internal/infrastructure/rdb/repository/<sibling>/<sibling>_repository.go` etc.) as **concrete reference** — tracer wiring, `gen.New(driver.New(ctx, r.db))` usage, pgerror normalization placement, conversion helper pattern. The infra READMEs do not carry a full code snippet, so sibling code is the closest concrete reference; on any conflict the READMEs win.

## Step 2. Derive Mapping (lean A core)

For each Repository method, find the corresponding sqlc gen function(s) via name-match heuristic:

- Exact match: `Repository.Save` ↔ `Queries.SaveUser` ↔ ✓ mapped
- Stem match: `FindByActive` ↔ `Queries.ListUsers` / `ListActiveUsers` / `ListDeletedUsers` (multi-target for switch dispatch) → ✓ mapped (multi)
- Aggregate-aware: `Repository.Find` ↔ `Queries.GetUser` → ✓ mapped (synonyms)

For each Repository method WITHOUT a derivable sqlc gen match:

- Do NOT halt. **Generate the method stub with a TODO** explaining what's missing:

  ```go
  func (r *repository) CountByActive(ctx context.Context, status user.ActiveStatus) (int, error) {
      ctx, endSpan := r.tracer.Start(ctx)
      defer endSpan()

      // TODO: CountByActive に対応する sqlc gen 関数が見当たりません。
      // 解決方法:
      //   1. database/dml/repository/<aggregate>/*.sql に CountByActive query を追加
      //   2. make gen-query を実行
      //   3. 本 TODO を消して sqlc gen を呼ぶ実装に置き換え
      return 0, errors.New("not implemented")
  }
  ```

- Surface in final report which methods got TODO stubs.

This differs from scaffold-controller (which halts) because partial Repository implementation can still compile + run for the methods that ARE mapped.

## Step 3. Test-Perspective Subagent

Invoke the Codex delegation to enumerate infra-layer test viewpoints BEFORE writing code.

- `subagent_type: general-purpose`
- Prompt (Japanese): pass the derived mapping + `internal/infrastructure/rdb/README.md` `Test Strategy` + expected infra viewpoints:
  - real DB + rollback isolation per test
  - sqlc gen wrap correctness (param mapping)
  - pgerror normalization paths (unique violation, no rows, connection error)
  - observability span emission
  - parallel execution safety (Tx serialization)
- Output: structured list of test cases per method.

## Step 4. Plan and Confirm

Display a Japanese summary:

- Files to be created + DI module update
- For each Repository method: signature + mapped sqlc gen function(s) (or TODO marker if unmapped)
- Test method list from subagent

Ask:

- 「以下の構成で infra-db 層を生成しますか？」
- Options: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 5. Write Files

Order:

1. `<aggregate>_repository.go` — main implementation
2. `<aggregate>_repository_test.go` — testkit-backed integration tests
3. Update `internal/di/module/persistence.go` — append `<aggregate>.New` under `repository` module

Implementation file conventions:

- `package <aggregate>`
- `type repository struct { db driver.DatabaseDriver; tracer observability.LayerTracer }`
- `func New(db driver.DatabaseDriver, tf observability.TracerFactory) <domain>.Repository { return &repository{db: db, tracer: tf.Infra()} }`
- Each method (mapped):
  - `ctx, endSpan := r.tracer.Start(ctx); defer endSpan()`
  - `db := gen.New(driver.New(ctx, r.db))` (SQL logging / tracing is applied at the driver connection level; no per-repository wrapper)
  - Map domain params → sqlc params via heuristic (1:1 by name, types adjusted)
  - Call `db.<SqlcFunc>(ctx, params)`
  - Convert sqlc rows → domain entity per sibling pattern
  - Wrap every sqlc-call error with `pgerror.NormalizeError(err)` (single-normalization principle — see Step 1 / `pgerror/README.md`)
- Each method (unmapped): TODO stub as in Step 2
- Complex helpers (multi-row → slice): follow sibling pattern (helper function in same file)

Test file conventions:

- Use `testkit` for real DB + rollback
- Japanese subtest names
- Tests only for mapped methods; unmapped TODO methods get a skipped test stub

## Step 6. Verify

```sh
make go-fix
make go-test
```

Confirm Repository package coverage. Infra layer targets ≥85%. On failure: TODO + FB; no auto-rollback.

> **DI verification (runtime):** `go build` / `make go-test` do NOT construct the Fx graph — a missing provider, an unregistered `New`, or a mismatched constructor signature only fails at **app startup**, not at compile/test time. After the DI registration (`fx.Provide(<aggregate>.New)`), confirm the app actually boots: with `make serve` running, `air` rebuilds on save — verify the `api_server` logs reach `[Fx] RUNNING` ("http server started") with no Fx `provide` / `invoke` errors. Fresh-env caveat: the container builds in **vendor mode**, so run `make tidy-lib` (generates `vendor/`) first — otherwise it fails with `inconsistent vendoring` before Fx even runs.

## Step 7. Closing

```text
<Aggregate> infra-db 層を生成しました。<N> ファイル作成 + DI 1 行追加。
  mapped: <X> methods (sqlc gen 経由)
  unmapped: <Y> methods (TODO stub、SQL 追加 + make gen-query 後に再 scaffold or 手動実装)
make go-test OK、coverage <Z>%。
次は scaffold-usecase で application service、または scaffold-endpoint で残層を続行できます。
```

Do NOT commit.

## AI Modification Scope

Per the "Exception: Skill Execution" clause:

- Write scope: `internal/infrastructure/rdb/repository/<aggregate>/` (new directory) + `internal/di/module/persistence.go` (1 entry append).
- Aborts if the aggregate directory already exists.

Remains protected:

- SQL / sqlc gen output (read-only references)
- Other repository / query_service / system_query directories
- domain layer

## Constraints

- ❌ Add comments that restate the code or explain *why* a choice was made — keep code comments minimal (behavior / contract only); rationale belongs in the commit message / README, not the code. One-line declaration godoc stays (even on unexported symbols). **Volume counts too**: what this skill scaffolds is idiomatic by construction, so a multi-line explanation of a constructor / Params struct / row-to-entity mapping / handler template is noise. State the contract in one line and stop; never restate a repo-wide rule `docs/rules.md` already carries. Suppression, not elimination — a genuinely non-obvious Why still stays.
- ❌ Invent business logic in Repository (data orchestration only)
- ❌ Generate SQL or run `make gen-query` (out of scope)
- ❌ Hand-edit sqlc generated files
- ❌ Skip the test-perspective subagent
- ❌ Skip the identity-confirmation `ask the user explicitly`
- ❌ Overwrite an existing aggregate repository directory
- ❌ Touch other aggregates' repositories
- ❌ Auto-rollback on failure (TODO + FB instead)
- ❌ Auto-generate dummy sqlc functions for unmapped methods (TODO stub + hand-off)
- ✅ Japanese user-facing output + Japanese test case names
- ✅ Use existing `repository/<sibling>/` as concrete reference (infra READMEs do not include a full impl snippet); READMEs still win on any rule conflict
- ✅ Mapping derivation strictly via naming convention + sibling pattern
- ✅ Update DI registration in same skill run
- ✅ Partial implementation OK — mapped methods generate, unmapped methods get TODO stubs

## Checklist

- [ ] Identity (aggregate name) confirmed
- [ ] Preconditions verified (domain IF exists, sqlc gen exists, target directory does NOT exist)
- [ ] Domain Repository IF read for authoritative signatures
- [ ] sqlc gen function list extracted
- [ ] `internal/infrastructure/rdb/README.md` + `pgerror/README.md` + sibling repo read (READMEs canonical, sibling concrete reference)
- [ ] Mapping derivation done (mapped + unmapped lists ready)
- [ ] Test-perspective subagent invoked
- [ ] Plan displayed (mapped + unmapped clearly distinguished) and confirmed
- [ ] Implementation file written; mapped methods call sqlc gen, unmapped methods have TODO stub
- [ ] Test file written; tests only for mapped methods
- [ ] `internal/di/module/persistence.go` updated with new `fx.Provide`
- [ ] `make go-fix` + `make go-test` run; coverage reported (or failure surfaced)
- [ ] Final summary clearly lists mapped count + unmapped count + next-step guidance
- [ ] No commits / pushes
