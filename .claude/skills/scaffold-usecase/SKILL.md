---
name: scaffold-usecase
description: >-
  Implement the usecase (Application Service) layer for one feature, driven by `docs/spec/usecase/<pkgpath>.md` (`<pkgpath>` = the package path under `internal/usecase/`), and register its provider in the DI module. The spec is the single source of truth — it invents nothing the spec does not declare, and on failure leaves TODOs plus a summary rather than rolling back. Prerequisites: the domain layer's Repository interface + entity exist, and the boundary interfaces the spec depends on (clock, tx, security, ...) are defined under `internal/usecase/boundary/`. Use it once the domain layer for the feature is in place. Standalone-callable; the third scaffold step when chained from `scaffold-endpoint`. Do NOT use it to implement any other layer (`scaffold-domain` / `scaffold-infra-db` / `scaffold-controller`) or to write the spec itself (`new-spec-usecase`).
---

# Scaffold Usecase

Generate the application service (usecase) layer for a feature based on `docs/spec/usecase/<pkgpath>.md`. Produces the interface, struct, constructor, methods with workflow body, DTOs, tests, and DI registration in a single pass.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- After `scaffold-domain` (entity + Repository IF) and `scaffold-infra-db` (Repository impl) have been completed.
- As the third step of `scaffold-endpoint` (auto-chained).
- Standalone when only the usecase layer needs scaffolding (e.g., domain/infra already exist).

Do NOT use this skill for:

- Modifying an existing usecase package (skill assumes a fresh package).
- Adding a single new method to an existing usecase — edit by hand.
- Creating new boundary interfaces (those are bootstrapped manually under `internal/usecase/boundary/`).

## What This Skill Reads / Writes

**Reads (always)**:

- `docs/spec/usecase/<pkgpath>.md` — single source of truth for what gets generated.
- `internal/usecase/README.md` — layer convention (Application Service Pattern, Tx boundary, Application Policy, etc.).
- `internal/usecase/boundary/README.md` — boundary conventions.
- `internal/usecase/<sibling>/<sibling>_usecase.go` — **secondary** reference (README has the canonical Implementation Example; existing code is observed but not authoritative).
- `internal/domain/<aggregate>/<aggregate>_repository.go` + entity — to validate `calls:` references.
- `internal/usecase/boundary/<name>/` — to validate spec's `Dependencies` boundaries exist.
- `internal/di/module/usecase.go` — DI registration target.

**Writes (with confirmation)**:

- `internal/usecase/<package>/<package>_usecase.go` — Usecase interface (with `//go:generate mockgen`) + struct + constructor + methods + DTOs
- `internal/usecase/<package>/<package>_usecase_test.go` — gomock-based tests
- `internal/di/module/usecase.go` — append `<package>.New` under `UsecaseModule`'s `fx.Provide(...)`

**Triggers (via `make`)**:

- `make gen-api` — regenerates Usecase mock under `internal/usecase/<package>/mock/`
- `make go-fix` + `make go-test` — final verification

**Never touches**:

- domain / infra layers.
- boundary interface definitions.
- `docs/spec/` files.
- Generated mock files (hand-edit forbidden).

## Preconditions

The skill verifies before any write:

1. `docs/spec/usecase/<pkgpath>.md` exists and parses.
2. For every `calls:` reference in spec:
   - `<aggregate>.Repository.<Method>` → exists in domain Repository IF.
   - `<aggregate>.<BehaviorMethod>` or `<aggregate>.New` → exists in domain.
   - `<boundary>.<Method>` → boundary type exists under `internal/usecase/boundary/`.
3. `internal/usecase/<package>/` does NOT already exist (abort if it does).

If any precondition fails, abort with a clear corrective pointer (`/scaffold-domain`, manual boundary creation, manual cleanup).

## First Step: Resolve Spec Path

This skill **MUST call `AskUserQuestion` immediately after invocation** (unless invoked from `scaffold-endpoint` with the package path in context):

- Question: 「対象 usecase パッケージパス（`internal/usecase/` 以下。例: `address`, `product/ranking`, `user/search`）」
- Free-text. Resolves to `docs/spec/usecase/<pkgpath>.md` — the spec tree mirrors the package tree, so the target package and the spec path are the same answer.

Standalone alternative: `--spec=<path>`.

## Step 1. Read Spec + Reference Context

1. Parse the usecase spec's YAML into inventory:
   - `interface`: package, name, methods (name + signature)
   - `dtos`: list of (name, fields)
   - `dependencies`: list of (name, type) — boundaries + Repository IFs
   - `workflow`: per method `(tx_required, steps, calls, errors)`
2. Read `internal/usecase/README.md` as the **authoritative source** for layer rules + reference implementation (especially "Application Service Design Policy", "Time Handling Policy", "Boundary Concept", "Allowed dependencies", "Forbidden dependencies", "Observability (Tracing)", and the "Implementation Example" section at the bottom which carries the canonical struct / constructor / per-method template).
3. Read `internal/usecase/boundary/README.md` for boundary conventions.
4. Existing usecase packages (e.g., `internal/usecase/<sibling>/<sibling>_usecase.go`) are a **secondary** reference only — observe but never silently follow if they diverge from the README's reference implementation; README wins.
5. Verify each `calls:` reference against the actual source (domain Repository IF, domain entity factory/methods, boundary types).

## Step 2. Test-Perspective Subagent

Invoke the Agent tool to enumerate usecase-layer test viewpoints BEFORE writing code.

- `subagent_type: general-purpose`
- Prompt (Japanese): pass the parsed spec inventory + `internal/usecase/README.md`'s "Testing Strategy" section + expected usecase viewpoints:
  - workflow call ordering (domain → repo → boundary in correct order)
  - mock strategy: Repository / Boundary mocked; Domain entity used as real
  - transaction-boundary correctness (`tx.Manager.Do` wrapping)
  - error propagation (boundary error → returned, Repository error → mapped to apperror)
  - DTO composition (final return value structure)
  - empty / zero-result handling
- Expected output: per-method test case list.

Fall back to a minimal default with a warning if the subagent returns nothing.

## Step 3. Plan and Confirm

Display a Japanese summary:

- Files to be created + DI update
- For each method: signature + workflow synopsis (calls, tx_required)
- Test method list from subagent

Ask:

- 「以下の構成で usecase 層を生成しますか？」
- Options: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 4. Write Files

Order:

1. `<package>_usecase.go` — interface (with `//go:generate mockgen`) + DTOs + struct + constructor + methods
2. `<package>_usecase_test.go` — gomock tests using subagent viewpoints
3. `internal/di/module/usecase.go` — append `<package>.New` under the `usecase` `fx.Provide(...)` list

Implementation file conventions:

- `package <package>` (lowercase aggregate name)
- `//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE` at top (repo-wide standard directive — identical in every interface file)
- `type Usecase interface { ... }` from spec's Interface
- DTOs as `type XxxDTO struct { ... }` from spec's DTOs
- `type usecase struct { tracer observability.LayerTracer; <deps...> }` from spec's Dependencies
- `func New(tf observability.TracerFactory, <deps...>) Usecase { return &usecase{tracer: tf.Usecase(), <init...>} }`
- Each method body:
  - Open the tracer span at the top (`u.tracer.Start(ctx)` / `defer endSpan()`), per usecase README "Observability (Tracing)"
  - If `tx_required: true`, wrap the body in `u.txm.Do(ctx, func(ctx context.Context) error { ... })`
  - Implement Workflow steps in order, calling the declared `calls:` entries
  - Apply error mapping per spec
  - Return the DTO

Test file conventions:

- gomock controller per subtest
- Mock Repository + Boundaries; use real Domain entity (per project convention)
- Japanese subtest names
- Use the subagent's viewpoint list as test case map

## Step 5. Regenerate Mocks

```sh
make gen-api
```

Processes the `//go:generate mockgen` directive in the new usecase file and produces `internal/usecase/<package>/mock/mock_<package>_usecase.go.gen.go`. Verify the mock file exists.

## Step 6. Verify

```sh
make go-fix
make go-test
```

Confirm `internal/usecase/<package>` coverage line. Target 100% per project convention. If below, identify untested error / branch paths and recommend test additions.

On failure: TODO + FB summary; no auto-rollback.

> **DI verification (runtime):** `go build` / `make go-test` do NOT construct the Fx graph — a missing provider, an unregistered `New`, or a mismatched constructor signature only fails at **app startup**, not at compile/test time. After the DI registration (`fx.Provide(<package>.New)`), confirm the app actually boots: with `make serve` running, `air` rebuilds on save — verify the `api_server` logs reach `[Fx] RUNNING` ("http server started") with no Fx `provide` / `invoke` errors. Fresh-env caveat: the container builds in **vendor mode**, so run `make tidy-lib` (generates `vendor/`) first — otherwise it fails with `inconsistent vendoring` before Fx even runs.

## Step 7. Closing

```text
<Package> usecase 層を生成しました。<N> ファイル作成 + DI 1 行追加、make go-test OK、coverage <X>%。
次は scaffold-controller で handler、または scaffold-endpoint で残層を続行できます。
```

Do NOT commit. Do NOT trigger the next scaffold skill.

## AI Modification Scope

Per the "Exception: Skill Execution" clause:

- Write scope: `internal/usecase/<package>/` (new directory) + `internal/di/module/usecase.go` (append 1 entry).
- Aborts if the package directory already exists.

Remains protected:

- domain / infra layers.
- boundary interface files (read-only).
- spec file.
- Mock files (regenerated via `make gen-api`).

## Constraints

- ❌ Add comments that restate the code or explain *why* a choice was made — keep code comments minimal (behavior / contract only); rationale belongs in the commit message / README, not the code. One-line declaration godoc stays (even on unexported symbols). **Volume counts too**: what this skill scaffolds is idiomatic by construction, so a multi-line explanation of a constructor / Params struct / row-to-entity mapping / handler template is noise. State the contract in one line and stop; never restate a repo-wide rule `docs/rules.md` already carries. Suppression, not elimination — a genuinely non-obvious Why still stays.
- ❌ Invent methods, DTOs, dependencies, or workflow steps not in the spec
- ❌ Implement business rules (those belong in domain entity)
- ❌ Access infrastructure directly (only via Repository / Boundary interfaces)
- ❌ Use `time.Now()` directly — go through the `clock.Clock` boundary (canonical detail: usecase README "Time Handling Policy")
- ❌ Skip the test-perspective subagent (Step 2)
- ❌ Skip the spec-confirmation `AskUserQuestion`
- ❌ Overwrite an existing usecase package
- ❌ Hand-edit the mock file
- ❌ Auto-rollback on failure (TODO + FB)
- ✅ Japanese user-facing output and Japanese test case names
- ✅ Use `internal/usecase/README.md`'s Implementation Example as the canonical structural template; existing packages are secondary reference only
- ✅ Validate every `calls:` reference exists before writing
- ✅ Update DI registration in the same skill run

## Checklist

Before reporting completion, confirm:

- [ ] Spec path resolved and file exists
- [ ] Preconditions verified (domain references, boundary references, target directory does not exist)
- [ ] Spec YAML parsed successfully
- [ ] `internal/usecase/README.md` (incl. Implementation Example) + `boundary/README.md` read as canonical template; sibling package observed as secondary
- [ ] Test-perspective subagent invoked; viewpoints captured before any write
- [ ] Plan displayed and confirmed via `AskUserQuestion`
- [ ] Implementation file written with interface / DTOs / struct / constructor / methods
- [ ] Test file written with gomock setup + subagent viewpoints
- [ ] `internal/di/module/usecase.go` updated with new `fx.Provide`
- [ ] `make gen-api` executed; mock file present
- [ ] `make go-fix` + `make go-test` run; coverage reported (or failure surfaced with TODO + FB)
- [ ] No commits / pushes
- [ ] Final summary in Japanese
