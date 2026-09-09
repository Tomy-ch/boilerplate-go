---
name: scaffold-integration-test
description: >-
  Generate HTTP-boundary integration tests for one feature under `internal/integration/`. These are NOT DB / usecase integration tests — they boot an Echo server via `httptest` and verify only the HTTP path (Router → Middleware → Handler → Presenter) with the usecase MOCKED, one subtest per operationId. Prerequisite: the feature's controller handler exists and compiles, and the usecase mock exists. Use it when a feature's HTTP boundary needs coverage above the handler unit test. Standalone-callable; chained from `scaffold-controller`, so `scaffold-endpoint` gets it transitively. Do NOT use it for unit tests of a single subject (`scaffold-test`) or for DB-backed verification.
---

# Scaffold Integration Test

Generate the HTTP-boundary integration test for a feature under `internal/integration/`.

This layer is **not** about DB or usecase internals — it boots a real Echo server (`httptest`) and asserts the behavior of the **whole HTTP path** (Router → Middleware → Handler → Presenter) with the **usecase mocked**. See `internal/integration/README.md` for the canonical test strategy and scope.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Immediately after a feature's controller handler is implemented and compiles.
- As the final step of `scaffold-controller` (auto-chained), so a feature gets its HTTP-boundary test right after the handler exists.
- Standalone when only the integration test for an existing handler needs scaffolding.

Do NOT use for:

- DB / Repository / usecase-logic verification — those are unit tests (domain/usecase) and the repository's own `make go-test` (real DB). Integration here stops at the HTTP boundary.
- Modifying the handler, usecase, or generated files.
- Adding a single case to an existing `<feature>_test.go` — edit by hand.

## What This Skill Reads / Writes

**Reads (always, as source of truth)**:

- `internal/integration/README.md` — test strategy + scope (what is and is NOT verified here)
- `internal/integration/helper_test.go` — the package's test helpers, by role: HTTP server bootstrap, request execution, JSON-response assertion, auth-header injection. **Read their exact names + signatures from this file at runtime — the skill hardcodes none of them.** Use whatever helpers exist there; do not reinvent HTTP plumbing.
- `internal/integration/<sibling>_test.go` (e.g. `v1_users_test.go`) — structural template for the package's idioms
- `internal/controller/handler/<path>/<feature>_handler.go` — the `BindHandler` signature + handler package name to import
- `internal/controller/handler/<path>/gen/server.gen.go` — operationId list + HTTP method/path (from the route comments) + request/response types
- the usecase mock package (e.g. `internal/usecase/<pkg>/mock`) — the mock + `EXPECT()` method names for the mapped usecase methods

**Writes (with confirmation)**:

- `internal/integration/<feature>_test.go` — one top-level `Test<Feature>_Integration` func with one subtest per operationId

**Triggers (via `make`)**:

- `make go-fix` + `make go-test` — final verification

**Never touches**:

- Handler / usecase / domain / infra code
- Generated `*.gen.go`
- `internal/integration/helper_test.go` (read-only; if a needed helper is missing, surface it — do not edit the helper here)

## Preconditions

The skill verifies before any write:

1. `internal/integration/` exists with `helper_test.go` (the helper API is present).
2. The target handler package exists and exposes `BindHandler` (controller scaffolding done + compiling).
3. The usecase mock package for the feature exists (generated).
4. `internal/integration/<feature>_test.go` does NOT already exist (abort if it does — hand back to manual editing).

If any precondition fails, abort with corrective guidance (`/scaffold-controller`, `make gen-api` for the mock, manual cleanup).

## First Step: Resolve Identity

This skill **MUST call `AskUserQuestion` immediately after invocation** (unless invoked from `scaffold-controller` with feature + handler path + usecase package already in context):

1. 質問: 「対象 feature 名 (kebab-case) / 出力ファイル名 (`internal/integration/<feature>_test.go`)」 — free-text
2. 質問: 「handler package パス (`internal/controller/handler/` 配下)」 — free-text, validate dir exists
3. 質問: 「対応する usecase package 名 (mock の所在)」 — free-text, validate `internal/usecase/<name>/mock` exists

## Step 1. Read Inputs

1. Read `internal/integration/README.md` — re-confirm the scope (HTTP boundary only, usecase mocked, no DB).
2. Read `internal/integration/helper_test.go` — enumerate the available helpers and their signatures (do not hardcode; read them this run).
3. Read 1 sibling `<sibling>_test.go` as the structural template (imports, `echo.New()`, mock wiring, `StartServer`, assertions, `MakeAvailableUserID` for auth).
4. Read the handler `gen/server.gen.go` — for each operationId: HTTP method + path (from route comments), request type, response type + the 2xx response constructor.
5. Read the handler `<feature>_handler.go` — `BindHandler` signature + which operations require auth (`ctxhelper.GetAuthn` usage).
6. Read the usecase mock package — the mock constructor + `EXPECT()` method names to stub for each operation.

## Step 2. Derive Per-Operation Test Plan

For each operationId in the handler `gen`:

- HTTP method + path (e.g. `GET /v1/<resources>/{<id>}`)
- the usecase method the handler calls (read from `<feature>_handler.go`) → the mock `EXPECT()` to set
- whether the operation requires authentication → if so, use `MakeAvailableUserID`
- happy-path expectation (status + minimal JSON body shape) and, where cheap, one representative error path (e.g. usecase returns `apperror.ErrNotFound` → expect 404) to confirm the errorhandler middleware mapping on the HTTP path

Keep it at the HTTP boundary: assert status codes and response serialization, NOT business outcomes (the usecase is mocked).

## Step 3. Test-Perspective Subagent

Invoke the Agent tool (`subagent_type: general-purpose`) to enumerate HTTP-boundary viewpoints BEFORE writing, passing `internal/integration/README.md` + the per-operation plan. Expected viewpoints:

- routing reachability (method + path resolves to the handler)
- request deserialization (path param / body) and 2xx response serialization shape
- apperror → HTTP status mapping along the real middleware stack (e.g. NotFound → 404, validation → 422/400)
- auth-required operations: with and (where meaningful) without `MakeAvailableUserID`
- middleware effects that only manifest on the full HTTP path (request id, force-json, recovery) when relevant

Output: per-operation subtest list (happy path + the error/auth paths worth asserting at this layer).

## Step 4. Plan and Confirm

Display a Japanese summary: output file, per-operation subtests (method + path + mocked usecase method + auth note + expected status), then ask:

- 「以下の構成で integration テストを生成しますか？」
- Options: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 5. Write the Test File

Write `internal/integration/<feature>_test.go`:

- `package integration`
- One `func Test<Feature>_Integration(t *testing.T)` with `t.Parallel()`
- One `t.Run("<日本語の振る舞い説明>", ...)` per planned subtest. Build each by **mirroring the sibling `<feature>_test.go` and the helpers read in Step 1** — do not assume helper names; use whatever `helper_test.go` actually exposes. The shape per subtest:
  - boot a fresh Echo instance + a no-op tracer factory + a `gomock` controller
  - construct the usecase mock and set its `EXPECT()` for the mapped method (return canned DTO / error)
  - bind the handler via the package's `BindHandler`
  - for auth-required ops: obtain auth headers via the auth-header helper
  - drive the endpoint (method + path + body + headers) through the request helper, capture the response
  - assert the status code; on happy path build the expected `gen` response struct and compare via the JSON-assert helper
- Japanese subtest names; mirror the sibling file's idioms exactly (the actual helper names, import aliases like `mock_<pkg>`, tracer-factory constructor). The sibling test + `helper_test.go` are the source of truth for every helper call — the steps above describe roles, not fixed identifiers.

## Step 6. Verify

```sh
make go-fix
make go-test
```

> **Environment note:** `make go-test` requires the dev environment to be running for the DB-backed suites in the same run; bring it up with the dedicated make targets (`make serve` → **`make db-init`**, which migrates **and seeds** both local & test DBs — seed data is assumed by the suite), **not raw `docker compose`** and not a bare `db-*-migrate-up`. If `make go-fix` / `make go-test` fails on a tool version mismatch (e.g. `golangci-lint` v1/v2 config error), realign with `make install-tools` (`make sync-versions` first if `mise.toml` changed) rather than hand-editing `PATH`.

On failure: surface the failing test output + leave a `// TODO:` at the problem case + FB summary. No auto-rollback.

## Step 7. Closing

```text
<Feature> の HTTP 境界 integration テストを internal/integration/<feature>_test.go に生成しました。
<N> 件の subtest、make go-test OK。
```

Do NOT commit.

## AI Modification Scope

Per the "Exception: Skill Execution" clause:

- Write scope: `internal/integration/<feature>_test.go` only.
- Aborts if that file already exists.

Remains protected:

- `internal/integration/helper_test.go` (read-only).
- Handler / usecase / domain / infra code and generated files.

## Constraints

- ❌ Verify DB / Repository / usecase business logic here (HTTP boundary only — usecase is mocked)
- ❌ Edit `helper_test.go` or reinvent HTTP plumbing (use the existing helpers)
- ❌ Modify handler / usecase / generated files
- ❌ Overwrite an existing `<feature>_test.go`
- ❌ Skip the test-perspective subagent or the confirmation `AskUserQuestion`
- ❌ Auto-rollback on failure (TODO + FB)
- ✅ Japanese user-facing output and Japanese subtest names
- ✅ `t.Parallel()`, generated mocks, existing helpers
- ✅ Mirror a sibling `<feature>_test.go` as the structural template
- ✅ One subtest per operationId; assert status + response serialization

## Checklist

- [ ] Identity confirmed (feature / handler path / usecase package) or received from `scaffold-controller`
- [ ] Preconditions verified (helper_test.go present, BindHandler exists, mock exists, target file absent)
- [ ] `internal/integration/README.md` + `helper_test.go` + sibling test read this run
- [ ] handler `gen` operationIds + method/path + mapped usecase methods + auth flags derived
- [ ] Test-perspective subagent invoked; viewpoints captured before any write
- [ ] Plan displayed and confirmed via `AskUserQuestion`
- [ ] `internal/integration/<feature>_test.go` written using existing helpers, Japanese subtest names, `t.Parallel()`
- [ ] `make go-fix` + `make go-test` run (or failure surfaced with TODO + FB)
- [ ] No commits / pushes
- [ ] Final summary in Japanese
