---
name: scaffold-endpoint
description: >-
  End-to-end orchestrator that builds a complete onion-architecture endpoint (domain + infra-db + usecase + controller) for one feature — and, when you start from a rough idea instead of finished specs, first drives the feature-dev-style upstream design phases that turn that idea into the input artifacts the deterministic scaffold core needs. Two entry modes, auto-detected: (A) **idea-first** — Discovery + Clarifying Questions (ask the user explicitly) → parallel Codebase Exploration (Explore agents) → Architecture Design (Plan agent, constrained to the lean A / onion / OpenAPI-first / sqlc rails) → draft the OpenAPI YAML + SQL migration + the domain and usecase specs for user review, then run `make gen-api` / `make gen-query`; (B) **specs-ready** — jump straight to the core when `docs/spec/domain/<domainPkg>.md` + `docs/spec/usecase/<usecasePkg>.md` + OpenAPI + SQL already exist. The deterministic core is unchanged: `verify-spec` → `scaffold-domain` → `scaffold-infra-db` → `scaffold-usecase` → `scaffold-controller` → `make go-fix`/`make go-test` → runtime curl + o11y. Closes with a Quality Review that reuses the repo's own review skills (`impl-review` + `arch-check` + `test-review`) rather than a generic reviewer. Use when starting a new feature / endpoint end-to-end, when you have only an idea or a requirement and no specs yet, when you want the whole controller→usecase→domain→infra stack built consistently with one consolidated report, or when you want the upstream design phases (clarify → explore → design) before implementing. Do NOT use for modifying a single existing layer (run the specific `scaffold-<layer>` standalone), for a pure spec-template scaffold (`new-spec`), or for a review-only pass (`impl-review` / `arch-check` / `test-review`). Halts (never auto-rollbacks) on any failing phase; each layer keeps its own human-in-the-loop confirmation.
---

# Scaffold Endpoint

Top-level orchestrator that takes a feature from wherever you are — a rough idea or a finished spec set — to a reviewed, running onion-architecture endpoint. It grafts a **feature-dev-style set of upstream design phases** (clarify → explore → design → draft the inputs) onto an unchanged **deterministic spec-driven core** (verify → scaffold each layer → test → curl), and closes with a **quality review built from this repo's own review skills**.

The core's strength is that it is deterministic: specs + OpenAPI + SQL fully determine the generated code. These upstream phases exist only to *produce* those inputs when you don't have them yet — it never bypasses or weakens the core.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Starting a new feature / endpoint end-to-end — whether you have only an idea, or the 2 specs (domain + usecase) + OpenAPI YAML + SQL are already prepared.
- You want the upstream design phases (clarify ambiguities → explore existing patterns → weigh approaches) *before* any code is written.
- You want all layers built with the same conventions and one consolidated report, then reviewed by `impl-review` / `arch-check` / `test-review`.

Do NOT use this skill for:

- Modifying a single existing layer — run the specific layer skill (`scaffold-domain` / `-infra-db` / `-usecase` / `-controller`) standalone.
- Only scaffolding empty spec templates — that is `new-spec`.
- A review-only pass on existing code — run `impl-review` / `arch-check` / `test-review` directly.

## Two Entry Modes (auto-detected in Phase 0)

| Mode | Trigger | Upstream phases (1–4) | Core (Phases 5–7) |
| --- | --- | --- | --- |
| **A. idea-first** | User starts from an idea / requirement; the domain or the usecase spec is missing from its tree | **runs** — clarify → explore → design → draft inputs | runs |
| **B. specs-ready** | `docs/spec/domain/<domainPkg>.md` + `docs/spec/usecase/<usecasePkg>.md` + OpenAPI gen + sqlc gen already exist | **skipped** (fast path) | runs |

Never force the upstream phases on a user who already has valid inputs; detect and skip. Conversely, never skip straight to `verify-spec` when the specs don't exist yet — that is the gap the upstream phases fill.

## What This Skill Reads / Writes

**Reads (always)**:

- `docs/spec/domain/<domainPkg>.md` + `docs/spec/usecase/<usecasePkg>.md` — via child skills (lean A: 2 spec files only).
- OpenAPI gen + sqlc gen + domain Repository IF — derivation sources for controller / infra (no spec).
- `.codex/scaffold-spec/lifecycle.md` — for the canonical workflow / scaffold execution order.
- Layer `README.md` + `docs/` at runtime (the upstream phases read them as the source of truth for existing patterns; does not hardcode design rules that would drift).

**Writes**:

- **Mode B**: nothing directly. All writes happen inside the chained child skills, each within its own scope.
- **Mode A (upstream phases, Phase 4 only)**: *draft* input artifacts for user review — `openapi/**` (OpenAPI YAML), `database/migrations/**` (new files only) + `database/dml/**` (SQL), `docs/spec/domain/<domainPkg>.md` + `docs/spec/usecase/<usecasePkg>.md` (via `new-spec`, then filled). These are all inside the AI modification scope in `AGENTS.md`. These upstream phases never write Go source; that stays with the core's child skills.

## Preconditions for the core (Phases 5+)

These must be true before the **core** runs. In Mode A they are the *output* of the upstream phases (Phase 4); in Mode B the user has already satisfied them.

| # | Precondition | Verifier |
| --- | --- | --- |
| 1 | The domain spec and the usecase spec exist in their trees (lean A: 2 spec files only) | verify-spec |
| 2 | Spec format valid + cross-spec references consistent + naming convention satisfied | verify-spec |
| 3 | OpenAPI YAML written and `make gen-api` produced `internal/controller/handler/<path>/gen/` | scaffold-controller precondition |
| 4 | SQL files under `database/dml/...` written and `make gen-query` produced sqlc gen files | scaffold-infra-db precondition |
| 5 | DB running + migrated + seeded (DB schema matches the new SQL) | manual: start env with `make serve`, then `make db-init` |
| 6 | Boundary interfaces the usecase spec depends on exist under `internal/usecase/boundary/` | scaffold-usecase precondition |

If any precondition fails when the core starts, the relevant child skill will surface it and this skill aborts the chain.

> **Environment note (preconditions 3–5):** `make gen-query` dumps the live DB schema via `pg_dump`, so the database **must be running** — `make gen-query` (and `make go-test`) fail with `could not translate host name "database"` when it is not. Bring the environment up with the **dedicated make targets, not raw `docker compose`**: `make serve` (starts the development profile incl. the `database` service), then **`make db-init`** — which migrates **and seeds** both the local and test DBs (the test suite assumes seed data exists; piecemeal `db-*-migrate-up` alone is insufficient) — and only then `make gen-query` / `make gen-api`.
>
> **Toolchain note (final `make go-fix` / `make go-test`):** if `make go-fix` or `make go-lint` fails on a **tool version mismatch** (e.g. `golangci-lint` reporting "you are using a configuration file for golangci-lint v2 with golangci-lint v1"), do **not** work around it — align the local toolchain with `make install-tools` (installs the versions pinned in `mise.toml`; run `make sync-versions` first if `mise.toml` itself changed), then re-run. Do not hand-edit `PATH` or invoke version-specific binaries as a substitute.

---

## Phase 0. Confirm Feature + Detect Mode

**MUST call `ask the user explicitly` immediately after invocation**, one batched call:

- 「対象 feature 名 (kebab-case)」 — free-text. The bundle label used in the report; not a path.
- 「domain パッケージパス（`internal/domain/` 以下。例: `cart`, `product/category`）」 — free-text.
- 「usecase パッケージパス（`internal/usecase/` 以下。例: `address`, `product/ranking`, `user/search`）」 — free-text.

The two package paths are separate answers and routinely differ from each other and from the feature name: the spec tree is keyed by package path (`docs/spec/<layer>/<rest>.md` ⇔ `internal/<layer>/<rest>`), Go package names carry no hyphens, and a usecase package does not sit under the aggregate it depends on (`internal/usecase/user/search` → `internal/domain/user`). Default each to the feature name with `-` replaced by `/` and let the user correct it.

Then detect the entry mode — **two lookups, one per tree**:

- Check `docs/spec/domain/<domainPkg>.md` and `docs/spec/usecase/<usecasePkg>.md`. If **both** exist **and** the user's request reads as "scaffold from my ready specs", choose **Mode B** and go straight to Phase 5.
- Otherwise choose **Mode A** and run the upstream phases (Phases 1–4). Either file being absent is the normal idea-first start — do not treat it as an error.

If the mode is ambiguous (specs exist but the user is clearly still designing), ask which they want rather than guessing.

---

## Upstream design phases (Mode A only) — Phases 1–4

These upstream phases are grafted from the official `feature-dev` workflow, but wired to this repo's agents and constrained to its architecture. It reuses the existing **`Explore`** and **`Plan`** agents rather than introducing new ones.

### Phase 1. Discovery + Clarifying Questions

**Goal**: turn the idea into concrete, unambiguous requirements before any exploration or design.

1. Restate the feature request and the problem it solves; confirm scope boundaries with the user.
2. Identify the underspecified aspects that matter for an onion endpoint: the resource + its invariants, the operations (and their HTTP shape), error/edge cases, auth (`security:`) needs, persistence shape, idempotency / transaction needs, pagination, and backward-compatibility with existing endpoints.
3. Present the open questions with `ask the user explicitly` (group them; recommend a default per question). **Wait for answers** — the whole point of this phase is that nothing downstream is guessed. If the user says "whatever you think is best", record your recommendation and get explicit confirmation.

Output: a short requirements summary (kept in the run context; not a committed file) that Phases 2–4 build on.

### Phase 2. Codebase Exploration

**Goal**: ground the design in how this repo already does things, so the new feature integrates seamlessly instead of inventing a parallel pattern.

Launch 2–3 **`Explore`** agents in parallel (single message), each on a different aspect, e.g.:

- "Find the endpoint(s) most similar to `<feature>` and trace the full controller→usecase→domain→infra flow; return the 5–10 key files."
- "Map the domain + usecase conventions relevant to `<feature>` (entity/VO/Repository IF shape, boundary interfaces, DTO mapping); return key files."
- "Identify the OpenAPI + sqlc + DI wiring an endpoint like `<feature>` touches (spec layout, migration/dml layout, `internal/di/module/*`); return key files."

When the agents return, **read the key files they surface** to build first-hand context. Present a concise summary of the patterns found (with `file:line` references) before designing.

### Phase 3. Architecture Design

**Goal**: choose how this feature is built *within the repo's rails*, with the trade-offs made explicit.

Launch the **`Plan`** agent (1–3 focuses, e.g. minimal-change / clean-boundaries / pragmatic) to produce candidate approaches. **Constrain every approach to the fixed architecture** — the design space is *within* these rails, never around them:

- Onion layering: `controller → usecase → domain`; infrastructure implements domain interfaces; no layer bypass (enforced by depguard).
- lean A constitution: only the domain and usecase layers are spec-driven; controller is derived from OpenAPI gen, infra from sqlc gen.
- OpenAPI-first for the HTTP contract; sqlc for queries; no new frameworks or architectural patterns (per `AGENTS.md`).

So the approaches differ on *repo-legal* axes — e.g. where a computed value lives (domain method vs VO), repository vs query-service for a read path, synchronous write vs outbox, pagination style, how invariants are enforced — not on framework-level choices. Present each approach's trade-offs plus your recommendation, then **`ask the user explicitly` for which approach to use**.

### Phase 4. Draft the Input Artifacts

**Goal**: produce the core's preconditions as reviewable drafts (per the confirmed design), then hand off to generation.

Following the chosen approach:

1. Chain `new-spec` to scaffold the domain + usecase spec templates, then **fill them** with the designed content (entities/invariants/behaviors/VOs/Repository methods; usecase interface/DTOs/dependencies/workflow). `new-spec` only creates identity-level templates — the design content comes from Phases 1–3.
2. Draft the **OpenAPI YAML** under `openapi/**` (OpenAPI-first) and the **SQL** under `database/dml/**` + a **new** migration under `database/migrations/**` (new files only — never edit an existing migration).
3. **Present the drafts to the user for review** (`ask the user explicitly`: 「このドラフトで生成に進みますか？」 / 修正指摘 / キャンセル). These are drafts — the user is the author-of-record and must approve before generation.
4. After approval, run `make gen-api` + `make gen-query` (DB must be up — see the environment note) so the generated `gen/` + sqlc files exist for the core.

Do **not** hand-write or edit any generated file (`*.gen.go`, `*.sql.go`, `*_mock.go`, `openapi.gen.yaml`). If a draft can't be completed without a decision only the user can make, stop and ask rather than inventing business content.

When Phase 4 completes, the Phase-5+ preconditions hold and the flow continues into the (unchanged) core.

---

## Core (both modes) — Phases 5–7

### Phase 5. Verify Specs (auto-chain)

Invoke the `verify-spec` skill with the two package paths collected in Phase 0. If `verify-spec` reports `violations > 0`, abort the chain:

```text
scaffold can not safely proceed: verify-spec で <N> 件の違反が検出されました。
spec を修正してから再度 /scaffold-endpoint を実行してください。
```

If only warnings are reported, continue (warnings do not block).

### Phase 6. Chain Child Skills in Dependency Order

Invoke each child skill in turn, passing that layer's package path in context so each child resolves its spec path from it — the feature name does not determine a spec path:

1. **`scaffold-domain`** — entity + Repository IF + VOs + constants + errors + tests (+ `make gen-api` for mock).
2. **`scaffold-infra-db`** — Repository impl wrapping sqlc gen (requires `make gen-query` already run, verified internally).
3. **`scaffold-usecase`** — Application Service + DTOs + tests (+ `make gen-api` for Usecase mock).
4. **`scaffold-controller`** — Handler implementing `ServerInterface` + tests.

Between each child skill, propagate the success / failure status:

- **Child success** → proceed to next.
- **Child failure** → halt the chain, surface the child's FB summary, do NOT proceed.

Each child skill independently:

- Asks its own plan confirmation `ask the user explicitly` (the user approves each layer's plan separately to keep judgment per-layer)
- Invokes its own test-perspective subagent
- Runs `make gen-api` if needed
- Writes its own files
- Runs `make go-fix` + `make go-test` after its writes
- Surfaces TODO + FB on failure

If you want a "fully unattended" mode, the user can add `--auto-approve` (future flag) — but the default is to confirm each layer to keep human-in-the-loop on judgment-heavy steps.

### Phase 7. Integration Verification (make go-test + runtime curl + o11y)

After all 4 child skills succeed, run a final consolidated check:

```sh
make go-fix
make go-test
```

This confirms the cross-layer integration (handler → usecase → domain → infra) compiles and tests pass as a whole. Surface the per-package coverage line for the 4 packages this scaffold touched. If `make go-test` fails here (rare — child skills already ran their own), surface the failure with TODO + FB and stop.

Then run the **runtime verification (curl + o11y)**. `make go-test` uses **mocked usecases/repositories**, so it does NOT construct the real Fx graph, run the HTTP middleware (auth / OpenAPI validation), or touch the DB. A whole class of bugs only surfaces at runtime: a missing `security:` declaration (endpoint reachable without auth), an unregistered/mis-wired `BindHandler`, a DI provider mismatch, or a SQL filter that behaves differently against a real DB. **This is the correct place to curl** — all layers + DI now exist. Per-layer skills cannot do this (their lower layers / DI may be absent, so the app would not even boot).

Preconditions:

- `make serve` is running and the `api_server` log reached `[Fx] RUNNING` (the DI boot check from `scaffold-controller`).
- `make db-init` has run (seeds local + test DB — the canonical setup).

Steps:

1. **Pick or create a target in a known state.** If the operation needs an existing row, use a seeded id or create one first via the create endpoint. For credential/state-sensitive checks (e.g. password change), create a row whose plaintext/state you control (a `psql` insert with a known bcrypt hash is acceptable for local verification).
2. **curl the new endpoint(s)** (auth header for local: `Authorization: Bearer debug:<subject>`) and assert:
   - the route is reachable — a non-404-from-router response proves the handler is registered and DI is wired;
   - the happy path returns the expected status/body;
   - the key error paths: NotFound (404), validation (400/422), and — **if the operation declares `security:`** — no-token ⇒ 401 (verify the endpoint is actually protected);
   - the mutation **actually took effect** (re-read / re-auth with the new state), not merely a 2xx.
3. **Read the o11y logs once** for a single request: confirm the trace spans the full stack (controller → usecase → infra) and the emitted SQL is what you expect. After this one capture, further re-checks can rely on o11y instead of re-curling.

Shared-schema impact: if the change edits a **shared** OpenAPI component (e.g. a `components/schemas/*` or `components/requests/*` referenced by more than one operation), the runtime check must cover **every** endpoint that references it — not just the new/changed one. Grep the spec for `$ref`s to the edited file and curl each consumer. A shared-schema edit can silently break sibling endpoints the mocked tests never exercise (e.g. removing a property from a base used via `allOf`, or `additionalProperties: false` rejecting a sibling's property → `POST` create breaks while the new endpoint looks fine).

Destructive guard: if a curl mutates data and the only way to restore the prior state is `make db-init` (or equivalent), **confirm with the user before running it** (per `AGENTS.md`). Clean up any rows you created for verification.

If any check fails, surface TODO + FB and stop (do NOT commit).

---

## Review & closing (both modes) — Phases 8–9

### Phase 8. Quality Review (reuse the repo's review skills)

The scaffold + runtime check proves the feature is *built and boots*. This phase judges whether it is *good* — using the repo's own review skills rather than a generic reviewer, because they encode this codebase's rules and run reviewers on a different model than the implementer:

- **`impl-review`** — adversarial correctness / security / architecture / runtime-gap + comment quality on the new change.
- **`arch-check`** — layer-compliance audit across the touched layers (depguard-level boundaries, lean A conventions).
- **`test-review`** — quality of the generated tests (structural compliance + viewpoint coverage + semantic strength).

Run them scoped to this feature's change. Consolidate the findings, surface the highest-severity ones, and **`ask the user explicitly`**: 「指摘を今修正 / 後で / このまま進む」. Address per the user's choice. This phase is read-only detection; applying fixes is a deliberate, separately-confirmed step (these reviewers never auto-edit).

### Phase 9. Closing

Print a Japanese summary:

```text
scaffold-endpoint 完了（feature: <feature>, mode: <A/B>）。

  [Mode A のみ] ✓ 要件整理 / 探索 / 設計案選択 / 入力ドラフト作成 + gen
  ✓ verify-spec: violations 0
  ✓ scaffold-domain: <N> ファイル作成、coverage 100%
  ✓ scaffold-infra-db: <N> ファイル作成、coverage <X>%
  ✓ scaffold-usecase: <N> ファイル作成、coverage 100%
  ✓ scaffold-controller: <N> ファイル作成、coverage 100%
  ✓ make go-test: 全体 OK
  ✓ ランタイム動作確認: curl 到達 / 認証 / 主要異常系 / o11y トレース OK
  ✓ 品質レビュー: impl-review / arch-check / test-review 実施（指摘 <n> 件、対応方針: <...>）

次のアクション:
  - /commit で 変更をコミット
  - /submit-pr で PR 作成
```

If any phase failed, output the failure status table with the FB from the failing phase/child, and the user decides whether to fix forward.

Do NOT commit. Do NOT push.

## AI Modification Scope

- **Mode B**: this skill writes no files. All scope is delegated to child skills, each within their own constraints (see their SKILL.md).
- **Mode A**: additionally, the upstream phases draft input artifacts in Phase 4 — only under `openapi/**`, `database/dml/**`, `database/migrations/**` (new files only), and `docs/spec/domain/**` / `docs/spec/usecase/**` (all inside the `AGENTS.md` AI modification scope). It never writes Go source and never touches generated files.

## Constraints

- ❌ Force the upstream phases (Phases 1–4) when Mode B inputs already exist — detect and skip.
- ❌ Skip straight to `verify-spec` when the specs do not exist yet — the upstream phases must produce them first (Mode A).
- ❌ Let an architecture approach (Phase 3) step outside the onion / lean A / OpenAPI-first / sqlc rails — the design space is within them.
- ❌ Invent business content in Phase 4 — draft from the confirmed design; stop and ask on a genuine gap.
- ❌ Edit generated files (`*.gen.go`, `*.sql.go`, `*_mock.go`, `openapi.gen.yaml`) or an existing migration.
- ❌ Modify any Go source directly (delegate to child skills).
- ❌ Skip `verify-spec` (Phase 5) — it is the safety net for spec consistency.
- ❌ Proceed past a failing child skill — halt and surface FB.
- ❌ Auto-rollback files written by a successful earlier phase/child when a later one fails — the user decides.
- ❌ Skip the feature-confirmation `ask the user explicitly` (Phase 0).
- ✅ Japanese user-facing output.
- ✅ Reuse the existing `Explore` / `Plan` agents for the upstream phases (no new agent types).
- ✅ Run child skills in the documented dependency order (domain → infra-db → usecase → controller).
- ✅ Surface a consolidated final report covering every phase that ran + the final `make go-test`.
- ✅ Let each child skill ask its own confirmation per layer (human-in-the-loop on judgment-heavy steps).
- ✅ Run the runtime curl + o11y verification (Phase 7) — `make go-test` alone does not exercise DI / middleware / DB.
- ✅ Reuse `impl-review` / `arch-check` / `test-review` for the Quality Review (Phase 8), not a generic reviewer.
- ✅ Confirm with the user before any destructive curl whose only restore path is `make db-init`.

## Checklist

Before reporting completion, confirm:

- [ ] Feature name confirmed via `ask the user explicitly`; entry mode (A/B) detected (Phase 0)
- [ ] **Mode A**: requirements clarified (Phase 1) → codebase explored via `Explore` (Phase 2) → approach chosen via `Plan` within the rails (Phase 3) → input artifacts drafted, user-approved, and `make gen-api`/`gen-query` run (Phase 4)
- [ ] `verify-spec` ran; chain aborted if any violation (Phase 5)
- [ ] `scaffold-domain` / `-infra-db` / `-usecase` / `-controller` each ran successfully (or failed and chain halted) (Phase 6)
- [ ] Final `make go-fix` + `make go-test` run after all child skills; runtime curl / auth / key error paths / o11y trace confirmed (Phase 7)
- [ ] Quality Review ran (`impl-review` + `arch-check` + `test-review`); findings surfaced and fix decision taken (Phase 8)
- [ ] Consolidated Japanese summary with per-layer file counts and coverage (Phase 9)
- [ ] No commits / pushes
- [ ] On any failure, this skill did NOT auto-rollback already-written files
