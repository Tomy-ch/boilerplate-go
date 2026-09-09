---
name: scaffold-test
description: >-
  Generate a Go unit test file for an existing function / method in this repository, following the canonical test pattern and the viewpoints declared by `docs/testing-conventions.md` plus the nearest ancestor README's Test Strategy section. One `TestXxx` per function or method — strictly 1:1, no bundling (getters / accessors included), mechanically enforced by `internal/architest`. Use it whenever a new or changed subject needs unit tests, or someone asks 「テストを書いて」「テストを追加して」. Standalone-callable and chainable from `scaffold-domain` / `scaffold-usecase` / `scaffold-controller` / `scaffold-infra-db`. Read-only on implementation code — never edits the subject under test. Do NOT use it to review existing tests (`test-review`) or to write HTTP-boundary integration tests (`scaffold-integration-test`).
---

# Scaffold Test

Generate a Go unit test file for an existing function or method, applying the canonical pattern abstractly extracted from `internal/domain/<aggregate>/<aggregate>_domain_test.go` (parallel + nested `t.Run` + Japanese case names + 正常系 / 異常系 outer grouping, table-driven for-loops forbidden — always sequential `t.Run`).

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Adding a unit test for a function or method whose implementation already exists and compiles.
- Filling a coverage gap surfaced by `make go-test` (a package below the 90 % threshold).
- After hand-editing a function whose behavior changed and the existing test no longer reflects intent.
- As a chained step from `scaffold-domain` / `scaffold-usecase` / `scaffold-controller` / `scaffold-infra-db` when those skills want test generation factored out (the parent passes target + viewpoints; this skill skips its own resolution + perspective subagent).

Do NOT use this skill for:

- **HTTP-boundary integration tests** under `internal/integration/` — use `scaffold-integration-test` instead. This skill produces same-package unit tests.
- Rewriting an entire existing test file — this skill writes new tests; for incremental edits, edit by hand.
- Generating implementation code (writes test files only; never edits the subject under test).
- Writing tests against generated files (`*.gen.go`, `*_mock.go`, `*.sql.go`).

## What This Skill Reads / Writes

**Reads (always)**:

- `docs/testing-conventions.md` — the project-wide testing conventions (parallel mandate, naming, require vs assert, generated-mock policy, architectural rules) **and its section 10 semantic quality bar / anti-patterns** — the SSOT the generated tests must satisfy (each case asserts its branch's distinctive outcome; none of the listed anti-patterns is emitted). `test-review` reads the same section to review against it, so generator and reviewer stay symmetric — no viewpoint or anti-pattern list is duplicated into this skill.
- The target source file — to extract the function/method signature, parameters, return types, error sentinels, and any in-package helpers.
- The nearest layer README, resolved by **walking up from the target file to the closest ancestor `README.md` that actually carries a Test Strategy section (the heading wording varies across READMEs — `Test Strategy`, `Test strategy`, `Testing strategy`, `Testing Strategy`, `Testing Policy` — so match on meaning: a section that IS the layer's test strategy counts no matter what it is called, and renaming one to fit this rule is the wrong fix when other docs link to it by name)**. Also read any nearer README that lacks the section — it still owns that package's naming, helper, and invariant conventions even when the viewpoints come from an ancestor. Resolve by walking, not by lookup: the list below is a snapshot of where the walk currently lands, and a layer absent from it is a layer to walk to, never a layer to skip.
  - `internal/domain/README.md` for `internal/domain/**`
  - `internal/usecase/README.md` (+ `internal/usecase/boundary/README.md`) for `internal/usecase/**`
  - `internal/controller/README.md` — the controller-layer baseline, scoped per driver: HTTP handlers (+ `internal/controller/handler/README.md`) for `internal/controller/handler/**`, loop-driven controllers for `internal/controller/outbox/**` and `internal/controller/worker/**`
  - `internal/controller/httpstack/README.md` for `internal/controller/httpstack/**` — every middleware sub-package resolves to this parent
  - `internal/controller/server/README.md` for `internal/controller/server/**`
  - `internal/infrastructure/README.md` (+ `internal/infrastructure/rdb/README.md`) for `internal/infrastructure/**`
  - `internal/di/README.md` for `internal/di/**`, superseded by the nearer `internal/di/module/README.md` or `internal/di/server/hook/README.md` when the target sits under one of those
  - `internal/apperror/` / `internal/cli/` / `internal/config/` / `internal/logging/` / `internal/observability/` / `internal/system/` — the cross-cutting substrate packages each carry their own section at their package root
  - `pkg/README.md` (+ nearest sub-`pkg/<name>/README.md`) for `pkg/**`
- Sibling test files in the **same package** — secondary structural template for imports, helper style (e.g. `newValidUser(t)`), assertion phrasing, fixture conventions. README wins on any conflict.
- Existing mocks under `<package>/mock/*_mock.go` when the target depends on injected interfaces — to wire them up without writing custom mocks (per `docs/testing-conventions.md`).

**Writes (with confirmation)**:

- One test file alongside the subject, named `<subject>_test.go` (where `<subject>` is the source file basename without `.go`). If the file already exists, the skill appends new top-level `TestXxx` functions rather than rewriting it — and asks first.

**Triggers (via `make`)**:

- `make go-fix` — auto-format the generated test file.
- `make go-test` — run the produced tests + verify the package's coverage did not regress.

**Never touches**:

- The subject source file (`<subject>.go`).
- Generated artifacts (`**/*.gen.go`, `*_mock.go`, `*.sql.go`).
- Mocks under `*/mock/` directories (consumed as-is).
- Other packages' test files.

## First Step: Resolve Target

Unless invoked with target context from a chained scaffold-* skill, the first action is `ask the user explicitly`:

- Question: 「テストを書きたい対象を指定してください」
- Options (single-select):
  - 「対象ファイル全体」 — free-text path; the skill enumerates every exported top-level function and method in that file and generates one `TestXxx` per subject.
  - 「ファイル内の特定関数 / メソッドのみ」 — free-text `<file>:<symbol>`; the skill generates one `TestXxx` for that single subject.
  - 「キャンセル」.

After resolution, detect the layer by applying the walk-up rule above to the file path (do not pattern-match against a fixed set of layer prefixes) and store the resolved README(s) for downstream steps.

If the target file does not exist, abort and ask the user to confirm the path.

## Step 1. Read Layer Context

1. Read the layer README(s) listed above. The README is the canonical source for naming, helper style, and any layer-specific testing conventions (e.g. domain `pkg/ptr.Copy` immutability checks, controller `testecho` usage, infra `pgerror.NormalizeError` assertions).
2. Read every `*_test.go` file in the target package. Extract:
   - Top-level test helper signatures (e.g. `newValidUser(t *testing.T) (*User, time.Time)`).
   - Fixture variables conventionally declared at the top of `TestXxx` functions.
   - Assertion style and import set actually in use.
3. Read `docs/testing-conventions.md` once and treat it as load-bearing for: parallel mandate, naming, require vs assert, generated-mock policy, architectural test rules.

If any conflict arises between sibling tests and the layer README, the README wins.

## Step 2. Test-Perspective Subagent

Spawn a subagent (`subagent_type: general-purpose`) to enumerate the test viewpoints for **this specific subject** before any code is generated. This step is mandatory and never inlined into the main loop — the perspective must be derived from the layer README's Test Strategy section + the target signature, not pattern-matched from the skill's own memory.

This skill does NOT carry a hardcoded viewpoint seed list per layer. Viewpoints are the layer README's responsibility (canonical), and this skill defers to whatever the README currently documents. As READMEs evolve, the viewpoint set evolves automatically — no skill edit required.

Prompt content (Japanese):

- The target subject's signature + Doc comment.
- The **full text** of the layer README's `Test Strategy` / `Testing strategy` section (whichever heading exists) — captured verbatim from Step 1, including every sub-section heading. The subagent's job is to map those headings to concrete viewpoints for this subject. Reference points already present in the current READMEs:
  - `internal/domain/README.md` → `## Testing strategy` (Getter contract / Immutable guarantee / Domain behavior / Error classification / Test design policy / Test Fixture / Invariant preservation)
  - `internal/usecase/README.md` → `## Testing Strategy` (Test dependencies / Testing goals / Test targets / Test structure / What not to test)
  - `internal/controller/handler/README.md` → `## Test Strategy` (Test Dependencies / Test Targets / Test Structure / Router Test / Handler Test / Error Test / Thin Controller Test Scope / Observability Test / Test Policy / Not Covered in Controller Tests / Test Kit testkit / testassert / testauth / testecho / testspan)
  - `internal/controller/httpstack/README.md` → `## Test Strategy` (Real vs mocked / Viewpoints every middleware covers / Viewpoints for `Before` / `After` hooks) — middleware is tested as an isolated unit; ops-path exclusion, `server.ResponseOf` nil degradation, and hook firing / repeat-firing / non-firing are the recurring viewpoints
  - `internal/controller/server/README.md` → `## Test Strategy` (Echo context utilities / Server construction)
  - `internal/infrastructure/README.md` + `internal/infrastructure/rdb/README.md` → `## Test Strategy` / `### 7. Test Strategy (Integration-based)`
  - `internal/di/README.md` → `## Test Strategy` (the DI-layer baseline: graph validity vs provider bodies vs lifecycle hooks vs environment-gated wiring), with `internal/di/module/README.md` and `internal/di/server/hook/README.md` carrying the detail for their sub-trees — the latter names the three HTTP-hook paths (bind failure / graceful shutdown / log-only `Serve` exit)
  - `pkg/README.md` → **intentionally has no Test Strategy section**. `pkg/` is framework-agnostic pure utilities (per `docs/testing-conventions.md`), and the test viewpoints reduce to the standard Go pattern — input-output verification, edge / boundary values, nil / zero handling — which is well-covered by sibling tests (the existing `pkg/datetime`, `pkg/envutil`, `pkg/ptr`, `pkg/uuid`, `pkg/xerrors`, etc. tests demonstrate the pattern). The subagent derives viewpoints from sibling tests + `docs/testing-conventions.md` here and does NOT surface a gap warning — this is the layer's normal mode, not a documentation hole. Any per-package sub-`pkg/<name>/README.md` should still be consulted for package-specific invariants.
  These cross-references are descriptive (current state of the READMEs at the time this skill was written) and are NOT a hard map — when the READMEs change, the subagent reads the up-to-date headings and adapts. If a heading is renamed, removed, or added, the subagent uses what is actually in the README on the day it runs.
- Sibling test patterns observed in Step 1 (as secondary reference).
- `docs/testing-conventions.md` (as the project-wide baseline).

Expected return: a structured list of `TestXxx → t.Run(正常系) → t.Run(case)` / `t.Run(異常系) → t.Run(case)` paths the skill should produce, with each case annotated by which README sub-section (or sibling test pattern) it traces back to.

Fallback behavior:

- **Layer is `pkg/**`** — the README intentionally has no Test Strategy section because the test viewpoints are the standard Go pattern (input-output, edge cases, nil / zero handling). The subagent derives viewpoints from sibling tests + per-package sub-`pkg/<name>/README.md` (if present) + `docs/testing-conventions.md` and the skill does NOT surface a warning. This is the layer's normal mode.
- **Target is anywhere under `internal/**` and the walk up reaches the repository root without finding a Test Strategy section** — surface the gap to the user (`「<walked path> のいずれにも Test Strategy 節がないため、sibling テストパターン + docs/testing-conventions.md からフォールバックで観点を導出しています。README を補完する余地があります」`). **Every `internal/**` layer is expected to have one**, with `pkg/**` as the single documented exemption — do not narrow this to a list of layers that happen to have one today. That narrowing is exactly what let whole layers (middleware / server lifecycle / DI wiring) sit without a comparison baseline while the check reported nothing: an unlisted layer looked exempt instead of undocumented. Name the READMEs the walk passed through, so the user can see where the section belongs.
- If the subagent returns no viewpoints at all (regardless of layer), fall back to a minimal default (one 正常系 success + one 異常系 catchall) and warn the user.

## Step 3. Plan the Test Structure

Apply these hard rules to map viewpoints to a concrete test file outline:

1. **One `TestXxx` per function or method.** A function `Foo` gets `func TestFoo(t *testing.T)`. A method `(*User).UpdateProfile` gets `func TestUser_UpdateProfile(t *testing.T)`. Multiple `TestXxx` for the same subject are never produced.
   - **Reverse direction — every public function / method keeps its own `TestXxx`, and the 1:1 mapping outranks weak-test avoidance.** Do NOT delete or drop a subject's dedicated `TestXxx` merely because its only honest assertion is currently thin (e.g. `assert.NotNil(NewClock())` for a trivial constructor). Keep the 1:1 slot so a future meaningful test has a home. A public subject with no `TestXxx` of its own is a 1:1 violation even when its behavior is transitively exercised by another subject's test.
   - **Never fold one subject's verification into another subject's `TestXxx`** — it muddies the responsibility of the test it is folded into. Asserting `NewClock`'s contract inside `TestClockNow` is folding; the constructor's own assertion belongs in `TestNewClock`. A method test MAY *call* the constructor as a fixture to obtain the SUT — that is not folding (folding is adding a separate assertion about the constructor itself into the method's test).
2. **Never bundle multiple subjects into one `TestXxx` — strict 1:1, no exception.** Do NOT produce a unified `TestEntity_Accessors` / `*_Getters` covering multiple getters; each getter / accessor gets its own `TestXxx`. There is no `ask the user explicitly` bundling path and no rationale-comment exemption. The *only* exemption is a subject that is **unverifiable and therefore unreachable** (e.g. a helper whose failure path calls `tb.Fatalf`, which terminates the calling test): it STILL declares its convention-named `TestXxx` and calls `t.Skip("<why it cannot be verified>")` — there is no allowlist; the reason lives in the `t.Skip` string. **"Covered by another test" is NOT an exemption**: it makes the subject depend on another test's implementation and stays green after that test shrinks, so a testable subject gets a real test even when a caller / integration / DI-graph test already exercises it. This mirrors `docs/testing-conventions.md` §1, mechanically enforced by `internal/architest` (`TestUnitTestMappingCompleteness` for the 1:1 slot, `TestSkipReasonDoesNotNameCoveringTest` for the skip reason).
3. **Outermost two `t.Run` groups MUST be the literal strings `正常系` and `異常系` — nothing else.**
   - Use `t.Run("正常系", ...)` and `t.Run("異常系", ...)` exactly. The group names are the literal two characters, not a prefix.
   - **Forbidden pattern**: `t.Run("正常系_ユーザーが存在する場合", ...)` at the top level. The `正常系_` / `異常系_` prefix on individual case names is explicitly NOT the project convention — it conflates the group axis (正常系 / 異常系) with the case description axis (what this specific case does).
   - **Correct pattern**:

     ```go
     t.Run("正常系", func(t *testing.T) {
         t.Parallel()
         t.Run("ユーザーが存在する場合エンティティを返す", func(t *testing.T) { ... })
         t.Run("ユーザーが論理削除済みの場合は除外する", func(t *testing.T) { ... })
     })
     t.Run("異常系", func(t *testing.T) {
         t.Parallel()
         t.Run("IDがゼロ値の場合エラーを返す", func(t *testing.T) { ... })
     })
     ```

   - Both groups call `t.Parallel()` immediately inside. Nested sub-groups for finer categorization (e.g. `t.Run("firstNameが範囲外の場合、エラーを返す", ...)`) are encouraged when readable, and live INSIDE the relevant 正常系 / 異常系 group.
   - Each `TestXxx` has at most one `正常系` block and at most one `異常系` block. If only happy cases exist, omit the `異常系` block (and vice versa); do NOT create an empty group.
4. **Every `t.Run` calls `t.Parallel()` as its first statement.** Exception: a block that mutates a pointer shared with another sibling block keeps its outer `t.Run` serial; the comment above the block MUST explain why (`-race` would catch the violation). Inner cases inside that serial block still call `t.Parallel()`.
5. **Table-driven `for`-loop tests are forbidden — always use sequential `t.Run` siblings.** Each case is its own named `t.Run`, as in `user_domain_test.go`. Do NOT loop over a slice of `(input, expected)` structs with `for _, tc := range cases`. Writing each case out separately makes failures name the exact case, lets each case call `t.Parallel()`, and avoids a shared loop body coupling the cases together. This holds even for a long list of near-identical getter / boundary assertions (accept the repetition; do not collapse into a table). There is no per-case exception — do not ask; write sequential `t.Run`.
6. **Case names are Japanese, and sub-case names carry NO `正常系_` / `異常系_` prefix.** Outermost groups: literally `正常系` / `異常系`. Sub-cases inside those groups: free-form Japanese sentence describing the input class and the expected outcome (`「<input class>の場合、<outcome>」`). The case name reads as a complete sentence. Since the sub-case already lives under a 正常系 / 異常系 group, adding `正常系_` / `異常系_` to the case name itself is redundant and forbidden — it produces `正常系 > 正常系_xxx` paths in `go test` output, which is double-labelling. Strip the prefix from the case description.
7. **`require` vs `assert` per `docs/testing-conventions.md`**:
   - `require.NoError` / `require.Error` / `require.ErrorIs` / `require.ErrorContains` — every error-related assertion (the testifylint `require-error` rule rejects `assert.ErrorIs`).
   - `require.Not<Nil>` only when guarding a subsequent dereference.
   - `assert.Equal` / `assert.Len` / `assert.Contains` / `assert.True` / `assert.False` / `assert.Empty` for terminal value verification — a failure here should not stop the test.
8. **Mocks always come from `<package>/mock/`** (generated via `go.uber.org/mock` + `make gen-api`). Custom hand-written mocks are forbidden by `docs/testing-conventions.md`.
9. **No DB / HTTP / external IO** inside unit tests in domain / usecase / controller layers (per `docs/testing-conventions.md` architectural rules). Infra tests against the real DB stay in this skill's scope only when the package already has a sibling test that establishes the convention — otherwise refer the user to `scaffold-integration-test`.

## Step 4. Confirm Plan

Display, in Japanese:

- Target file path.
- Detected layer.
- Each `TestXxx` to be generated, with its 正常系 / 異常系 sub-case list.
- A short rationale tying each case to the viewpoint that produced it.
- The first ~20 lines of the proposed test file as a preview.
- Any subject emitted as a skip-test (`TestXxx` + `t.Skip("<why it cannot be verified>")`) because it is unverifiable, stating why verification is impossible.

Then `ask the user explicitly`:

- Question: 「以下の構成でテストを生成しますか？」
- Options: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」.

## Step 5. Write the Test File

Generate the test file applying the abstract pattern from `internal/domain/<aggregate>/<aggregate>_domain_test.go`:

```go
func Test<Subject>(t *testing.T) {
    t.Parallel()

    // Common fixture setup shared across subcases. Pin time to a fixed
    // baseTime so subtests stay deterministic.
    <fixture variables>

    t.Run("正常系", func(t *testing.T) {
        t.Parallel()

        t.Run("<日本語の入力クラス>の場合、<期待される結果>", func(t *testing.T) {
            t.Parallel()
            actual, err := Subject(<args>)
            require.NoError(t, err)
            assert.Equal(t, <expected>, actual.<field>)
        })
        // ... additional happy cases
    })

    t.Run("異常系", func(t *testing.T) {
        t.Parallel()

        t.Run("<日本語の入力クラス>の場合、エラーを返す", func(t *testing.T) {
            t.Parallel()
            actual, err := Subject(<invalid args>)
            assert.Nil(t, actual)
            require.ErrorIs(t, err, <ErrSentinel>)
        })
        // ... additional error cases, optionally nested for finer grouping
    })
}
```

Additional generation rules:

- **Satisfy the semantic quality bar (`docs/testing-conventions.md` section 10).** Each generated case must assert its branch's *distinctive* outcome — error branches via `require.ErrorIs` on the specific sentinel, success / state-mutating branches on the resulting value / field, boundary cases on both sides — never a vacuous `require.NoError` / `assert.NotNil`-only body that merely proves the branch executed. Do not emit any section-10 anti-pattern (weak assertion, name over-promising the assertion, brittle internals coupling, over-mocking, time-literal pinning leak, redundant comments). This is the same section `test-review` reviews against — do NOT restate the list here, read it at runtime.
- **Test helpers** declared in the same package (e.g. `newValidUser(t)`) are reused as-is. If a helper does not yet exist but the same fixture would be repeated 3+ times across the produced tests, generate an unexported `t.Helper()`-tagged helper at the bottom of the test file.
- **`uuid.NewTestFromSalt(t, "<salt>")`** is the canonical way to obtain a deterministic UUID inside tests (per `pkg/uuid`).
- **`ptr.To(...)` / `ptr.Copy(...)`** are used for nullable pointer fields per the domain README's "Handling time and ID" / "Invariants" sections.
- **Mock setup**: when the target consumes injected interfaces, instantiate the corresponding `mock.NewMock<Interface>(t)` and wire `.EXPECT().<Method>(...).Return(...)` calls. The order and argument matchers should mirror the target's call sequence.
- **Imports**: assemble the import block exactly as the same-package sibling tests do. Do not add unused imports.

If the test file already exists, do not rewrite it — append the new `TestXxx` functions at the bottom, after any existing helper declarations. Confirm appending via `ask the user explicitly` first.

## Step 6. Verify

Run, in order:

1. `make go-fix` — formats the new test file. If any non-target file is reformatted, surface the diff to the user.
2. `make go-test` — confirms the new tests pass and that the package's coverage stays at or above its prior level (and above the 90 % project threshold for new / modified packages, per `docs/testing-conventions.md`).
3. **Mutation-check the regression-critical cases.** For any case written to lock in a specific behavior or guard a known / just-fixed bug (not generic coverage cases), prove the test actually catches the regression: temporarily inject the regression into the **subject** (flip the condition, drop the guard, swap the field / arg), re-run just that test, confirm it **FAILs**, then revert the mutation. A test that still passes under the mutation protects nothing — strengthen the assertion until it fails. This is the difference between a real regression test and a tautology; do it on the regression-critical cases, not every case.

If `make go-test` fails:

- Leave the produced test file in place so the user can inspect it.
- Surface the failing test name + assertion message.
- Do NOT auto-rollback — the user decides whether to amend the test or the subject.

If coverage regressed below 90 % despite tests passing, surface the gap and propose the missing viewpoint(s) for a follow-up invocation.

## Chainability

When invoked from `scaffold-domain`, `scaffold-usecase`, `scaffold-controller`, or `scaffold-infra-db`, the parent passes a context payload containing at minimum:

- `target_file` — absolute path to the source file under test.
- `target_subjects` — list of `<func or method name>` strings the parent wants tests for.
- `layer` — pre-resolved layer key (`domain` / `usecase` / `controller` / `infra` / `pkg`).
- `viewpoints` — the parent's already-derived test-perspective output, when available.

In chained mode, this skill skips:

- First Step (target resolution).
- Step 2 (test-perspective subagent), when `viewpoints` is non-empty.
- Step 4's `ask the user explicitly` confirmation (the parent already obtained per-feature approval) — but a one-line summary is still printed for the audit trail.

The strict 1:1 rule (no bundling; skip-test exemption only for an unverifiable subject) holds in chained mode too — there is no bundling confirmation to obtain, since bundling is never allowed. (Table-driven tests likewise have no exception path: they are forbidden — always sequential `t.Run`.)

## Constraints (Summary)

- ❌ Add restate-the-code or *why*-narration comments to the generated test — keep test comments minimal (behavior only). Case intent is carried by the Japanese `t.Run` name, not by inline comments; the only required non-godoc comment is the `-race` serial-block exception rationale. When a comment does earn its place, keep it to one line — a multi-line explanation of a fixture or an idiomatic assertion costs more to read than it returns.
- ❌ Multiple `TestXxx` for the same function / method.
- ❌ Bundling multiple subjects into one `TestXxx` (strict 1:1, no exception; getters / accessors included). Each subject gets its own named `TestXxx` — never a bundle.
- ❌ `t.Skip` justified by another test covering the subject (it makes one test depend on another's implementation). Skip only what is unverifiable, and write *why* it cannot be verified.
- ❌ Table-driven `for`-loop tests (forbidden — always write sequential `t.Run` siblings, one per case).
- ❌ Custom hand-written mocks (use `<package>/mock/*_mock.go`).
- ❌ Editing the subject source file.
- ❌ Editing generated artifacts (`*.gen.go`, `*_mock.go`, `*.sql.go`).
- ❌ `assert.ErrorIs` / `assert.NoError` (testifylint `require-error` rule).
- ❌ Skipping `t.Parallel()` without a documented `-race` reason.
- ❌ Skipping `t.Run` for any subcase.
- ❌ English case names (Japanese per `docs/testing-conventions.md`).
- ❌ Outer `t.Run("正常系_xxx", ...)` / `t.Run("異常系_xxx", ...)` prefix form. Use literal `正常系` / `異常系` as the outer group name and put the case description in the inner `t.Run`.
- ❌ Sub-case names that include the `正常系_` / `異常系_` prefix (they live under the group already).
- ✅ `t.Parallel()` at every nesting level (with documented exceptions).
- ✅ `t.Run` per subcase.
- ✅ Japanese case names; outermost groups are the literal strings `正常系` / `異常系`; inner sub-case names are free-form Japanese sentences without `正常系_` / `異常系_` prefix.
- ✅ `require` for errors, `assert` for terminal values.
- ✅ Generated mocks from `*/mock/` only.
- ✅ Deterministic fixtures (fixed `baseTime`, `uuid.NewTestFromSalt(t, ...)`).
- ✅ `t.Helper()` on helper functions in the produced test file.
- ✅ Satisfy the section-10 semantic quality bar (each case asserts its branch's distinctive outcome; no anti-pattern emitted) — read from `docs/testing-conventions.md`, not restated here.
- ✅ Mutation-check regression-critical cases (inject the regression into the subject, confirm the test FAILs, revert) — a case that stays green under the mutation is a tautology, not a guard.

## Checklist

Before reporting completion, confirm:

- [ ] Target file + layer were resolved (standalone) or accepted from the parent scaffold-* skill (chained).
- [ ] Layer README + `docs/testing-conventions.md` + sibling tests were read in Step 1.
- [ ] Test-perspective subagent ran in Step 2 (or `viewpoints` was supplied by the parent).
- [ ] Each produced `TestXxx` matches exactly one subject (strict 1:1, no bundling); only an unverifiable subject is emitted as a named `TestXxx` + `t.Skip("<why it cannot be verified>")`, and no skip reason names another test.
- [ ] Outermost `t.Run` group names are the literal strings `正常系` / `異常系`, NOT the `正常系_xxx` / `異常系_xxx` prefix form. Inner sub-case names contain no `正常系_` / `異常系_` prefix.
- [ ] Every `t.Run` has `t.Parallel()` as its first statement, or carries a comment explaining the `-race` exception.
- [ ] No `for`-loop tables produced (forbidden — always sequential `t.Run` siblings, no exception).
- [ ] All error assertions use `require.*`, all terminal value checks use `assert.*`.
- [ ] Mocks come from `<package>/mock/`; no custom mocks added.
- [ ] The subject source file was not edited.
- [ ] `make go-fix` + `make go-test` passed; coverage did not regress.
