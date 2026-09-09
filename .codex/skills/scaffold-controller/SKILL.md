---
name: scaffold-controller
description: >-
  Implement the controller (HTTP handler) layer for one feature, derived from OpenAPI gen + usecase Interface (lean A — no spec file). Reads the OpenAPI-generated `ServerInterface` at `internal/controller/handler/<path>/gen/server.gen.go` for operationIds, the target usecase Interface for available methods, and `internal/usecase/README.md` for naming convention. Derives the mapping operationId → usecase method via name-match heuristic. For any operationId without a derivable mapping, **halts with hand-off message** (user resolves: add usecase method, rename for convention, or accept hand-write). Reads `internal/controller/README.md` + `internal/controller/handler/README.md` (which carries the canonical reference snippet — `server` struct + `BindHandler` constructor + `gen.NewStrictHandler` + `gen.RegisterHandlers` pattern). Existing handler packages are secondary reference only; README wins on any drift. Invokes a test-perspective subagent (HTTP I/O conversion, validation paths, apperror→status mapping, middleware integration). Generates: `server` struct + `BindHandler(echo, tracerFactory, usecase)` constructor + one method per operationId matching `ServerInterface` (tracer span → request parse → usecase call → response conversion), plus a handler test file using `testkit/testecho` + `testkit/testassert`. Updates `internal/di/module/controller.go` with `fx.Invoke(<pkg>.BindHandler)`. Verifies with `make go-fix` + `make go-test`, then chains `scaffold-integration-test` as its final step to generate the feature's HTTP-boundary integration test under `internal/integration/`. Prerequisites: (1) OpenAPI YAML written and `make gen-api` run; (2) target usecase implemented + mock exists. Standalone-callable; chained from `scaffold-endpoint` runs as the fourth (last) scaffold step.
---

# Scaffold Controller

Generate the controller (HTTP handler) layer for a feature. **lean A: no spec file** — handler is derived from OpenAPI gen + usecase Interface via naming convention.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- After OpenAPI YAML is written and `make gen-api` has produced the handler interface.
- After the target usecase is implemented (mock available).
- As the fourth (last) step of `scaffold-endpoint` (auto-chained).
- Standalone when only the controller layer needs scaffolding.

Do NOT use for:

- Modifying OpenAPI YAML (human-written).
- Modifying an existing handler package (skill assumes a fresh package).
- Adding a single endpoint to an existing handler — edit by hand.
- Generating non-HTTP controllers (job CLI etc.).

## What This Skill Reads / Writes

**Reads (always)**:

- `internal/controller/handler/<path>/gen/server.gen.go` — generated `ServerInterface` (authoritative for operationId list + signatures)
- `internal/usecase/<package>/<package>_usecase.go` — target usecase Interface (available methods)
- `internal/usecase/README.md` — naming convention (verb-prefix, Usecase interface naming) for mapping derivation
- `internal/controller/README.md` — layer convention (handler must not contain business logic)
- `internal/controller/handler/README.md` — handler subdirectory conventions
- `internal/controller/handler/<sibling>/<sibling>_handler.go` — structural template
- `internal/di/module/controller.go` — DI registration target (uses `fx.Invoke(...BindHandler)` per README)

**Writes (with confirmation)**:

- `internal/controller/handler/<path>/<feature>_handler.go` — handler implementation
- `internal/controller/handler/<path>/<feature>_handler_test.go` — Echo-based test
- `internal/di/module/controller.go`: append `<pkg>.BindHandler` inside the existing `fx.Invoke(...)`

**Triggers (via `make`)**:

- `make go-fix` + `make go-test` — final verification

**Never touches**:

- OpenAPI YAML (read-only)
- Generated `server.gen.go` / `type.gen.go`
- usecase / domain / infra layers

## Preconditions

The skill verifies before any write:

1. `internal/controller/handler/<path>/gen/server.gen.go` exists (OpenAPI gen done).
2. Target usecase package exists at `internal/usecase/<package>/` with Interface + generated mock.
3. `internal/controller/handler/<path>/<feature>_handler.go` does NOT already exist (abort if it does).

If any precondition fails, abort with corrective guidance (`make gen-api`, `/scaffold-usecase`, manual cleanup).

## First Step: Resolve Identity

This skill **MUST call `ask the user explicitly` immediately after invocation** (unless invoked from `scaffold-endpoint` with feature + handler path + usecase package in context):

1. 質問: 「対象 feature 名 (kebab-case)」 — free-text
2. 質問: 「handler package パス (`internal/controller/handler/` 配下)」 — free-text, validate dir exists
3. 質問: 「対応する usecase package 名」 — free-text, validate `internal/usecase/<name>/` exists

## Step 1. Read Inputs

1. Read `internal/controller/handler/<path>/gen/server.gen.go` and extract `ServerInterface` method list (each with signature + request type + response type).
2. Read `internal/usecase/<package>/<package>_usecase.go` and extract the `Usecase` Interface method list (signatures).
3. Read `internal/usecase/README.md` and (if needed) 1〜2 sibling usecase packages for the naming convention (verb prefixes used in this codebase).
4. Read `internal/controller/README.md` + `internal/controller/handler/README.md` for layer rules.
5. Read 1 sibling handler (`internal/controller/handler/v1/<sibling>/v1_<sibling>_handler.go` etc.) as structural template. <!-- skill-lint-ignore -->

## Step 2. Derive Mapping (lean A core)

For each `ServerInterface` operationId, find the corresponding usecase method via name-match heuristic:

- Exact match: `operationId == usecaseMethod` (camelCase) → ✓ mapped
- Verb-aware match: `GetUsers` op vs `ListUsers` / `FindUsers` usecase method → ✓ mapped if convention recognizes the pair
- Plural / singular variants: `GetUser` ↔ `GetUserByID` → ✓ mapped via partial match

For each operationId WITHOUT a derivable mapping:

- **Halt** with `scaffold-controller can not safely proceed` and hand-off message:

  ```text
  以下の operationId は usecase method にマップできません:
    - DeleteUser → usecase に対応する Delete* メソッドなし

  解決方法:
    1. usecase Interface に対応 method を追加（推奨命名: DeleteUser）
    2. OpenAPI から operationId を削除
    3. handler を手で書く（scaffold 対象外）

  解決後に再度 scaffold-controller を実行してください。
  ```

- Do NOT auto-resolve. Do NOT skip. Do NOT generate handler stubs that fail compile.

Surface 1 mapping report (matched count / unmapped count) before Step 3.

## Step 3. Test-Perspective Subagent

Invoke the Codex delegation to enumerate controller-layer test viewpoints BEFORE writing code.

- `subagent_type: general-purpose`
- Prompt (Japanese): pass the derived mapping + `internal/controller/README.md` test guidance + expected controller viewpoints:
  - HTTP request / response shape (status code, body, headers)
  - validation (OpenAPI schema-level + handler-side extras if any)
  - usecase mocked, no business logic in handler
  - apperror → HTTP status mapping (verify errorhandler middleware covers it)
  - middleware integration if endpoint-specific middleware applies
  - authentication context handling for auth-required operations
- Output: per-operation test case list (happy path + error paths + auth paths).

## Step 4. Plan and Confirm

Display a Japanese summary:

- Files to be created + DI module update target
- For each operationId: signature (from generated interface) + mapped usecase method + auth/middleware notes (inferred from sibling)
- Test method list from subagent

Ask:

- 「以下の構成で controller 層を生成しますか？」
- Options: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 5. Write Files

Order:

1. `<feature>_handler.go` — handler struct + constructor + per-operation methods
2. `<feature>_handler_test.go` — `testkit/testecho` + `testkit/testassert` patterns
3. `internal/di/module/controller.go` — append `<pkg>.BindHandler` inside the existing `fx.Invoke(...)`

Implementation file conventions (per `internal/controller/handler/README.md` reference snippet — README is canonical):

- `package <handler-package>` (lowercase)
- `server` struct + `BindHandler(e, tf, uc)` constructor + `gen.RegisterHandlers(e, gen.NewStrictHandler(&server{...}, nil))` registration — follow the exact shape in the `internal/controller/handler/README.md` reference snippet (canonical; not restated here). Constructor name is `BindHandler`, NOT `New`.
- Each method:
  - Match the generated `ServerInterface` signature exactly
  - Open the tracer span at the top (`s.tracer.Start(ctx)` / `defer endSpan()`), per the README reference snippet
  - Extract request from generated request type
  - Call mapped usecase method (resolved in Step 2)
  - Convert DTO → generated response type (1:1 field map per README reference snippet; complex conversions left as TODO)
  - Return errors directly (errorhandler middleware translates apperror)
- For auth-required operations, use `ctxhelper.GetAuthn(ctx)` per README
- No business logic in the handler

Test file conventions:

- Use `testkit/testecho` + `testkit/testassert`
- Mock the usecase via its generated mock package
- Japanese subtest names

## Step 6. Verify

```sh
make go-fix
make go-test
```

Confirm handler package coverage. Handler tests target 100% (per project convention). On failure surface + TODO + FB.

> **DI verification (runtime):** `go build` / `make go-test` do NOT construct the Fx graph — a missing provider, an unregistered `BindHandler`, or a mismatched constructor signature only fails at **app startup**, not at compile/test time. After the DI registration (`fx.Invoke(<pkg>.BindHandler)`), confirm the app actually boots: with `make serve` running, `air` rebuilds on save — verify the `api_server` logs reach `[Fx] RUNNING` ("http server started") with no Fx `provide` / `invoke` errors. Fresh-env caveat: the container builds in **vendor mode**, so run `make tidy-lib` (generates `vendor/`) first — otherwise it fails with `inconsistent vendoring` before Fx even runs.

## Step 7. Chain Integration Test

Once the handler compiles and `make go-test` passes, invoke `scaffold-integration-test` (via the `Skill` tool) as the final step, passing the feature name + handler package path + usecase package in context so it skips its own identity `ask the user explicitly`. It generates the feature's HTTP-boundary test under `internal/integration/<feature>_test.go` (Router → Middleware → Handler → Presenter, usecase mocked).

- If `internal/integration/<feature>_test.go` already exists, `scaffold-integration-test` aborts (hand back to manual editing) — surface that and continue to closing.
- A failure inside `scaffold-integration-test` does NOT roll back the handler; surface its FB and let the user decide.
- When this skill is itself chained from `scaffold-endpoint`, the integration test is produced transitively here (no separate wiring needed in `scaffold-endpoint`).

## Step 8. Closing

```text
<Feature> controller 層を生成しました。<N> ファイル作成 + DI 1 行追加、make go-test OK、coverage <X>%。
HTTP 境界 integration テストも internal/integration/<feature>_test.go に生成しました（scaffold-integration-test）。
全層が揃いました — `make serve` + curl での実機ランタイム確認に進めます。
```

> **ランタイム curl 確認の位置づけ:** 認証（`security:`）・DI 配線・実 DB を通した curl + o11y の確認は、全層が揃う `scaffold-endpoint` の Phase 7（Integration Verification）が正式な実施場所。controller を**単独**で scaffold した場合も、下位層（usecase / domain / infra）と DI が既に存在していれば同様に curl 確認できる。下位層が未整備のうちは curl しても Fx が組み上がらず失敗するため、curl は全層が揃ってから行う。

Do NOT commit.

## AI Modification Scope

Per the "Exception: Skill Execution" clause:

- Write scope: `internal/controller/handler/<path>/<feature>_handler.go` + `_test.go` + 1 DI entry append.
- Aborts if the handler file already exists.

Remains protected:

- OpenAPI YAML.
- Generated `server.gen.go` / `type.gen.go`.
- usecase / domain / infra layers.

## Constraints

- ❌ Add comments that restate the code or explain *why* a choice was made — keep code comments minimal (behavior / contract only); rationale belongs in the commit message / README, not the code. One-line declaration godoc stays (even on unexported symbols). **Volume counts too**: what this skill scaffolds is idiomatic by construction, so a multi-line explanation of a constructor / Params struct / row-to-entity mapping / handler template is noise. State the contract in one line and stop; never restate a repo-wide rule `docs/rules.md` already carries. Suppression, not elimination — a genuinely non-obvious Why still stays.
- ❌ Contain business logic in the handler (must be in usecase or domain)
- ❌ Generate handler stubs for unmapped operationIds (halt with hand-off instead)
- ❌ Auto-resolve mapping gaps (e.g., create dummy usecase methods)
- ❌ Modify OpenAPI YAML, generated files, or usecase code
- ❌ Skip the test-perspective subagent
- ❌ Skip identity confirmation `ask the user explicitly`
- ❌ Overwrite an existing handler file
- ❌ Add custom error → status mapping when errorhandler middleware already covers it
- ❌ Auto-rollback on failure (TODO + FB)
- ✅ Japanese user-facing output and Japanese test case names
- ✅ Use existing handler as structural template
- ✅ Match generated `ServerInterface` exactly (signatures, request/response types)
- ✅ Update DI registration in same skill run
- ✅ Mapping derivation strictly via naming convention + sibling pattern (no AI guessing intent)

## Checklist

- [ ] Identity confirmed (feature + handler path + usecase package)
- [ ] Preconditions verified (gen exists, usecase exists, handler file does not exist)
- [ ] OpenAPI gen `ServerInterface` read + usecase Interface read
- [ ] `internal/usecase/README.md` + sibling pkg read for naming convention
- [ ] Mapping derivation done; **halted with hand-off if any operationId unmapped** (Step 2)
- [ ] Test-perspective subagent invoked; viewpoints captured before any write
- [ ] Plan displayed and confirmed via `ask the user explicitly`
- [ ] Implementation file written; method signatures match generated interface exactly
- [ ] Test file written with testkit/testecho + testkit/testassert patterns
- [ ] `internal/di/module/controller.go` updated with new `BindHandler` entry inside `fx.Invoke(...)`
- [ ] `make go-fix` + `make go-test` run; coverage reported (or failure surfaced with TODO + FB)
- [ ] `scaffold-integration-test` chained as the final step (its skip-if-exists / failure surfaced, no rollback)
- [ ] No commits / pushes
- [ ] Final summary in Japanese
