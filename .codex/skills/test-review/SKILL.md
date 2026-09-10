---
name: test-review
description: >-
  Independent quality review of Go test files (`*_test.go`) in this repository — structural compliance, coverage of the viewpoints the layer README's Test Strategy declares, semantic quality (weak assertions, brittle internals coupling, over-mocking), branch x meaning completeness read from the subject source, and public symbols with no test at all. Defaults to the changed test files (`git diff` HEAD-vs-working tree); branch-vs-base or specific paths are selectable at the start. Read-only — it never edits test files; the user decides what to fix and runs `scaffold-test` or hand-edits to apply. Use it after writing or changing tests, before a PR, or whenever someone asks 「テストをレビューして」「このテストで十分か」「テスト観点が足りているか」. Standalone-callable, and also the delegation target of `impl-review`, whose Step 5 chains here for the exhaustive audit while suppressing its own `test-gap` lens. Do NOT use it to review implementation code (`impl-review` / `arch-check`), to judge comments (`comment-sweep`), or to write tests (`scaffold-test`).
---

# Test Review

Adversarial, low-bias review of Go unit test files. Read-only — surfaces what looks broken / under-tested / over-tested, and the user decides how to act.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Before commit / PR, on the `*_test.go` files in the current change.
- After `scaffold-test` to get an independent second opinion on the generated tests.
- When coverage stays at 100 % but bug regressions still slip through (signal that test viewpoints are structurally compliant but semantically weak).
- As a standalone audit of a specific test package or file.

Do NOT use this skill for:

- Reviewing **implementation** code — use `code-review` / `impl-review` / `arch-check` for that.
- Reviewing **HTTP integration tests** under `internal/integration/` — those have their own conventions documented in `internal/integration/README.md` and are better reviewed against `scaffold-integration-test`'s rules; this skill focuses on same-package unit tests.
- Applying fixes — this skill never edits files. The user runs `scaffold-test` or hand-edits afterwards.

## What This Skill Reads / Writes

**Reads (always)**:

- `docs/testing-conventions.md` — the project-wide testing conventions, **including section 10 (Semantic quality bar / anti-patterns)** — the SSOT for Lens 3 and Lens 4 Axis B (意味網羅), shared with `scaffold-test`.
- `.codex/skills/scaffold-test/SKILL.md` — the canonical generation rules (parallel mandate, `t.Run` per subcase, 正常系 / 異常系 grouping, Japanese naming, require vs assert, mock policy, `for`-loop policy, one-`TestXxx`-per-subject policy). This skill reviews against those same rules — no duplication.
- The nearest layer README, resolved by **walking up from each target test file to the closest ancestor `README.md` that carries a Test Strategy section (the heading wording varies across READMEs — `Test Strategy`, `Test strategy`, `Testing strategy`, `Testing Strategy`, `Testing Policy` — so match on meaning: a section that IS the layer's test strategy counts no matter what it is called, and renaming one to fit this rule is the wrong fix when other docs link to it by name)** (the same rule `scaffold-test/SKILL.md` applies — keep the two in step). A nearer README without the section is still read for that package's own conventions. The list below is a snapshot of where the walk currently lands, not a closed map; a path that is not listed is walked, never treated as out of scope:
  - `internal/domain/README.md` (Testing strategy)
  - `internal/usecase/README.md` (Testing Strategy)
  - `internal/controller/handler/README.md` (Test Strategy) for handlers; `internal/controller/README.md` (Test Strategy, layer baseline) is what `internal/controller/outbox/**` / `internal/controller/worker/**` resolve to — read its loop-driven sub-section, not the HTTP one
  - `internal/controller/httpstack/README.md` (Test Strategy) — the resolution target for every middleware sub-package
  - `internal/controller/server/README.md` (Test Strategy)
  - `internal/infrastructure/README.md` + `internal/infrastructure/rdb/README.md` (Test Strategy)
  - `internal/di/README.md` (Test Strategy, layer baseline) — superseded by `internal/di/module/README.md` / `internal/di/server/hook/README.md` for targets under those
  - `internal/apperror/` / `internal/cli/` / `internal/config/` / `internal/logging/` / `internal/observability/` / `internal/system/` — cross-cutting substrate, each with its own section at the package root
  - For `pkg/**`, see `scaffold-test/SKILL.md` — `pkg/README.md` intentionally has no Test Strategy section; viewpoints come from sibling tests + per-package sub-`pkg/<name>/README.md`. **No gap warning for pkg.**
- The target `*_test.go` file(s).
- The corresponding subject source file(s) (`<subject>.go` paired with `<subject>_test.go`) — required for the two code-origin lenses (Lens 4 branch × meaning, Lens 5 subject-symbol completeness) to know what is and isn't tested.
- Sibling test files in the same package — secondary reference for established conventions.

**Writes**:

- Nothing. This skill is read-only. The final output is the Japanese report rendered into the conversation.

**Triggers**: none. No `make` invocation needed (does not run the tests; it reviews them).

**Never touches**:

- Test files (no auto-fix; that is `scaffold-test` or hand-edit territory).
- Subject source files.
- Generated artifacts (`*.gen.go`, `*_mock.go`, `*.sql.go`).
- `.codex/` files.

## First Step: Resolve Scope

**Skip this question entirely when a caller passed a `scope` payload** (see Chainability) — the file list is already resolved, and asking again would be a second dialog for a decision the caller has made.

`ask the user explicitly`:

- Question: 「test-review の対象スコープを指定してください」
- Options (single-select):
  - 「変更ファイル (HEAD-vs-working tree, 推奨)」 — `git diff --name-only` で `*_test.go` を抽出（このフローは `impl-review` / `code-review` と同じ振る舞い）。新規追加 (`git diff --name-only --diff-filter=A`) も含める。
  - 「ブランチ base 比較 (ベースブランチ以降の変更)」 — base からの分岐点を `git merge-base` で取り、その間に touched された `*_test.go`。 PR 単位で見たいとき。base は PR があればその `baseRefName`、無ければ `make base-branch`（`origin` の実状態から最新のリリースラインを解決する）。`gh repo view --json defaultBranchRef` は使わない — GitHub のデフォルトブランチはアクティブなリリースラインより遅れており、レビュー範囲が 1 世代分黙って広がる。
  - 「特定パス / パッケージ (free-text)」 — ユーザがパスを指定。 ファイルでもディレクトリでもよい。
  - 「キャンセル」.

After resolution, build the target list. If no `*_test.go` files are in scope, stop with a friendly message — no tests to review. **Standalone only**: under a caller payload an untested production file is the point rather than an empty scope, so the run continues (see Chainability).

For each target test file, also resolve its **subject source file** (same package, basename without `_test`). Required for the two code-origin lenses (Lens 4 / Lens 5).

## Step 1. Read Layer Context

For every target test file:

1. Resolve the layer README by walking up from the file path (the same walk `scaffold-test/SKILL.md` describes — not a fixed band lookup).
2. Read the layer README's `Test Strategy` / `Testing strategy` section (full text including every sub-section heading).
3. Read `docs/testing-conventions.md` once.
4. Read `.codex/skills/scaffold-test/SKILL.md` once — the canonical generation rules.
5. Read the subject source file (paired with the test file).
6. Read sibling `*_test.go` files in the same package for established conventions (helper signatures, fixture style, mock wiring).

If the walk reaches the repository root without finding a Test Strategy section and the target is under `internal/**`, note it for the report — it surfaces as a documentation gap, but does not block the review. **Every `internal/**` path is expected to resolve to one**, with `pkg/**` as the single documented exemption; never treat a layer as exempt merely because it is not named in the snapshot list above. Lens 2 (viewpoint coverage) has no comparison baseline in that state, so say so explicitly rather than letting the lens report nothing and read as a pass.

## Step 2. Fan Out Five Adversarial Reviewers

Spawn five `adversarial-reviewer` subagents (`subagent_type: adversarial-reviewer`) **in parallel**, each on `sonnet` by default (so reviewer ≠ an Opus implementer; the orchestrator may override the model to keep reviewer ≠ implementer).

Two of the five are **code-origin (subject-driven)** — they start from the subject source, not the test file, so a code element that has no test at all still enters their field of view: **Lens 5** (does a convention-named `TestXxx` exist for every subject symbol?) and **Lens 4** (within a tested function, is every branch reached and distinctly asserted?). The other three (Lens 1 / 2 / 3) start from the test file or the README. The code-origin pair is what catches reachable-but-untested code that a test-file-first read structurally cannot see.

Each subagent receives the same Step 1 context bundle (layer README, `docs/testing-conventions.md`, `scaffold-test/SKILL.md`, target test file, subject source file, sibling tests) but a different lens prompt:

### Lens 1: Structural Compliance

Audits mechanical rule adherence — these are the hard rules surfaced by `scaffold-test/SKILL.md`:

- `t.Parallel()` is the first statement in every `t.Run` (or the block has a comment-explained `-race` exception).
- Every subcase uses `t.Run` (no inline assertions outside a `t.Run`).
- Outermost `t.Run` groups in a `TestXxx` are the literal strings `正常系` and `異常系` (each `TestXxx` may have at most one of each; finer groupings sit INSIDE those two). The `正常系_xxx` / `異常系_xxx` prefix form on the outermost group is a violation — flag it. Sub-case names inside `正常系` / `異常系` groups must NOT carry the `正常系_` / `異常系_` prefix either, since the group already labels the axis.
- Case names are Japanese.
- Error assertions use `require.*` (testifylint `require-error`); terminal value assertions use `assert.*`. `require.NotNil` / `require.True` etc. are correct ONLY when they guard subsequent code that would panic / be meaningless otherwise (e.g. `require.NotNil(fn); fn(...)` or `require.NotNil(rec); _ = rec.Field`); when nothing uses the value afterwards the check is terminal and must be `assert.*` — flag a terminal `require.NotNil` / `require.True` / `require.Equal` as a violation.
- A subtest that drives a generated mock asserts via the mock controller: `EXPECT()` expectations — including a deliberate *no-EXPECT* (or `.Times(0)`) to assert a method is never called — ARE the assertion. Do NOT flag such a subtest as "assertion-less" merely because it has no `assert.*` / `require.*` line.
- **Table-driven `for`-loop tests are a violation** (`scaffold-test/SKILL.md` Rule 5) — a `for _, tc := range cases { t.Run(...) }` block over a slice of `(input, expected)` structs must be flagged, regardless of readability or `dupl` justification. The required form is sequential `t.Run` siblings, one per case (accept the repetition). This applies even to long getter / boundary lists.
- **1:1 mapping between subject functions and `TestXxx`.** The *subject* is the paired production source — the non-test, non-generated `.go` file that the binary is built from; `*.gen.go` / `*.sql.go` / `*_mock.go` and test-only helpers are out of scope (no hand-written `TestXxx` expected). Check BOTH directions:
  - *forward*: each `TestXxx` covers exactly one subject function / method — a `TestXxx` bundling multiple subjects (e.g. a unified `*_Accessors` / `*_Getters`) is a 1:1 violation, with no rationale-comment exemption. Decompose into one `TestXxx` per subject. The only exemption is a subject that is **unverifiable and therefore unreachable**: it still declares its convention-named `TestXxx` and calls `t.Skip("<why it cannot be verified>")` — flag a bundle, never accept it (per `docs/testing-conventions.md` §1, enforced by `internal/architest`).
  - *reverse*: each subject function / method maps to exactly one `TestXxx`. A subject split across multiple `TestXxx` (e.g. `TestFoo` + `TestFoo_Metrics` + `TestFoo_CloseError`, or a `Test_foo` / `TestFoo_foo` naming-variant pair) is a finding → consolidate into a single `TestXxx` whose `正常系` / `異常系` groups absorb the variants (Rule 7). A public subject function with NO `TestXxx` at all is **owned by Lens 5** (subject-symbol completeness) — this lens flags only the *shape* of the `TestXxx` that already exist (naming variants, bundling, split), not the absence of one; leave "symbol has zero test" to Lens 5 so the two do not double-report.
- Mocks come from `<package>/mock/*_mock.go` — no hand-written mocks.
- No imports of `internal/` from `pkg/**` test files; no infrastructure imports from `internal/domain/**` test files; etc. (architectural rules in `docs/testing-conventions.md`).

Output: a structured finding list with `file:line` references and the violated rule.

### Lens 2: Viewpoint Coverage

Compares the layer README's Test Strategy sub-sections to what the test file actually exercises:

- For each sub-section heading in the README's Test Strategy (`### Getter contract test` / `### Immutable guarantee test` / `### Invariant preservation test` / etc.), is there at least one `TestXxx → t.Run(case)` that maps to it?
- The layer README's Test Strategy section is the SSOT for that layer's per-layer viewpoint list (read in Step 1). Do NOT keep a hardcoded per-layer viewpoint list in this skill — it drifts from the READMEs. Apply whatever sub-sections that layer's README declares; when a README is missing a viewpoint the reviewer would expect, that absence is itself a documentation-gap finding (surfaced in 補遺), not a reason to hardcode the viewpoint here.
- If the walk finds no Test Strategy section anywhere above the target, use the sibling-test pattern as the comparison baseline instead — and say which case it is: under `pkg/**` that is the layer's normal mode (no gap), while under `internal/**` it is a documentation gap that leaves this lens without a baseline (report it in 補遺).

Output: a list of viewpoints the README declares but the test file does not exercise, with `file:section` references back into the README.

### Lens 3: Semantic Quality

Audits whether the assertions are actually meaningful, **against `docs/testing-conventions.md` section 10 (Semantic quality bar / anti-patterns) as the single source of truth**. That section is the SSOT shared with `scaffold-test` (the generator satisfies it; this lens flags violations of it) — read it at runtime and apply whatever it currently lists. Do NOT hardcode the anti-pattern catalogue here; it drifts from the doc. As of this writing §10 enumerates: weak assertions (with the trivial-constructor 1:1 *strengthen-in-place* exception — recommend a stronger assertion, never delete the dedicated `TestXxx` or fold it into another subject's test), name over-promising the assertion (with the branchless pass-through / wiring corollary — collapse redundant `NotNil` re-runs), brittle internals coupling, over-mocking, time-literal pinning leaks, `TestXxx` responsibility creep, helper duplication, and redundant comments. If §10 adds, removes, or refines an anti-pattern, follow the doc, not this paragraph.

Output: a list of findings with `file:line`, the §10 anti-pattern violated, and a one-sentence explanation of why the assertion is weak or brittle.

### Lens 4: Viewpoint Gap — Branch × Meaning Completeness (subject-driven)

Reads the subject source file itself and builds, **per function / method**, a two-axis completeness matrix. Code coverage ≠ meaningful coverage: a branch can be executed by a case that asserts nothing about what makes that branch distinct, and isolating that gap is the point of this lens. Run both axes for every subject — a branch is only "done" when it is both reached (Axis A) and distinctly asserted (Axis B).

**Division from Lens 5**: Lens 4 audits *within* a symbol that already has a `TestXxx` — which of its branches are reached and meaningfully asserted. "The symbol has no test at all" is **Lens 5's** finding, not Lens 4's; when Lens 5 has already flagged a zero-test symbol, do not also enumerate all of its uncovered branches here (that is one gap, not N). Lens 4's branch findings apply to symbols whose test exists but is incomplete.

**Axis A — branch enumeration (分岐網羅)**: every logical branch in the subject is reached by at least one case.

- Every conditional branch (positive / negative) has at least one `t.Run` case.
- Every error sentinel (`ErrInvalid*` / `apperror.*`) declared or returned is reached by at least one case.
- Every boundary value pair (min-1 / min / max / max+1) for a constrained field is exercised if the subject enforces it.
- Constructor / factory functions that defend against zero-value / nil input have a rejecting case.
- A branch reached only by *executing* a constructor / provider / factory body that the test's harness never runs is still uncovered — a graph- or wiring-validation harness that builds the dependency graph without executing the constructors does NOT cover those bodies; they need a direct unit test (call the function). The layer README's Test Strategy names the harness that applies.
- A `t.Skip` whose reason names another test as covering the subject is a **修正必須** violation, not a gap to weigh: the skip makes one test depend on another test's implementation and stays green after that test shrinks. Require a real test (`docs/testing-conventions.md` §1, enforced by `internal/architest` `TestSkipReasonDoesNotNameCoveringTest`).
- A `t.Skip` whose reason claims the branch "cannot be reproduced" is itself an Axis-A gap to challenge, not to accept: check the layer README's Test Strategy for an integration-style harness that reaches it (e.g. true concurrency / lock contention needs independent connections, not a serialized test-tx helper). Surface the skipped branch as 追加検討 with the concrete reproduction path.

A branch with NO covering case is a **分岐未カバー** finding → severity **追加検討** (proactive). Cite the subject `file:line` of the uncovered branch + a proposed `t.Run` case name. Attach a **criticality (1-10)** scored by *production impact* (a direction orthogonal to the lens-derived severity — 「追加検討」 says *what kind* of gap, criticality says *how bad if it breaks*) plus a one-line note of the regression that would ship if the branch stayed unverified, and order the 追加検討 findings by criticality descending so the user fixes the worst first: 9-10 データ破壊 / 認証・認可の穴 / 整合性違反 · 7-8 ユーザ影響のあるロジック誤り（誤った status / DTO マッピング）· 5-6 軽微な edge / boundary · 3-4 網羅性のための nice-to-have · 1-2 任意. Do NOT attach criticality to structural-compliance (修正必須) findings — those are always fix-now.

**Axis B — meaning coverage (意味網羅)**: each covered branch's case actually asserts that branch's *distinctive* outcome, not merely that it executed. This axis applies the **意味網羅 bar defined in `docs/testing-conventions.md` section 10** to the subject's branch set — §10 is the SSOT for what "distinctly asserted" means (shared with `scaffold-test`, which generates to satisfy it); the per-branch checks below are its concrete application.

- An error branch asserts the specific sentinel via `require.ErrorIs` — not just `require.Error`.
- A success branch asserts the resulting value / state that distinguishes it from the other branches — not just `require.NoError` / `assert.NotNil`.
- A state-mutating method's case asserts the post-mutation invariant / changed field — not just that the call returned.
- A pointer / reference-returning getter that the layer README marks as immutable has an immutability assertion (mutating the returned value must not affect the entity), not just a value-equality check.
- A boundary case asserts the differing outcome on each side of the boundary (accept vs reject), not just the accept side.

A branch that IS covered but whose case does not distinctly assert its outcome is a **分岐カバー済み・意味未検証** finding → severity **再考** (it passes and lifts coverage but reveals nothing). Tie the finding to the specific subject branch + the test case that nominally covers it.

Output: per subject, (1) uncovered branches with proposed `t.Run` case names (Axis A → 追加検討), and (2) covered-but-vacuously-asserted branches with the missing distinctive assertion (Axis B → 再考), each citing the subject branch `file:line` and the covering test case.

### Lens 5: Subject Symbol Completeness (code-origin)

Starts from the **subject source, not the test file** — the same origin as Lens 4, but at the coarser *symbol* granularity. Its single job is to answer, for the subject's complete public-symbol table, "does a test even exist for this?" A test-file-first read (Lens 1) can only judge the `TestXxx` it finds; a symbol with zero tests is invisible to it. This lens exists precisely because that blind spot is how reachable-but-untested code slips through — it enumerates the code first, then checks the tests against it, so absence is a positive finding rather than a silent nothing.

Procedure:

1. **Build the subject symbol table.** From the paired `<subject>.go` (non-generated, non-`*_test.go`), list every symbol that the layer's convention expects a `TestXxx` for: exported funcs / methods / constructors, and unexported package-level funcs that carry branching logic the layer tests directly (the layer README's Test Strategy + `docs/testing-conventions.md` §1 define the expectation; `*.gen.go` / `*.sql.go` / `*_mock.go` and test-only helpers are out of scope). Getters / accessors / provider funcs / env-gate helpers count — they are exactly the low-visibility symbols that get skipped.
2. **Match each symbol to a `TestXxx`.** A symbol is *satisfied* only when a convention-named `TestXxx` actually tests it. A `TestXxx` whose body is just `t.Skip` is satisfied ONLY when the reason states why the subject is unverifiable; a skip justified by another test covering the subject is **unsatisfied** per `docs/testing-conventions.md` §1.
3. **Flag every unsatisfied symbol** as a **シンボル未カバー** finding → severity **補完推奨** (a whole code element has no test — a structural coverage hole, distinct from Lens 4's *within-function* branch gaps). Cite the subject `symbol @ file:line` and propose the `TestXxx` name (and its `正常系` / `異常系` skeleton). Attach the same **criticality (1-10)** production-impact score as Lens 4 Axis A and order by it descending — a fully-untested auth/authz or persistence symbol outranks an untested trivial getter.

Hand-off to Lens 4: once Lens 5 has flagged a symbol as entirely untested, Lens 4 does NOT additionally enumerate that symbol's branches (one "no test" gap, not N branch gaps). Lens 4 picks up only symbols that Lens 5 considers satisfied but partially covered.

Output: the list of subject symbols with no test (or whose `t.Skip` reason names a covering test instead of stating unverifiability), each with `symbol @ file:line`, proposed `TestXxx`, and criticality.

## Step 3. Verify Each Finding

Each surviving finding goes to an independent `review-verifier` subagent (`subagent_type: review-verifier`) on `sonnet`. The verifier:

- Re-derives the conclusion from the code itself, **not** trusting the finder.
- Defaults to skepticism — when ambiguity remains, label PLAUSIBLE or REFUTED rather than CONFIRMED.
- Classifies the finding as **CONFIRMED** (the rule violation / gap / weak assertion is real and reproducible) / **PLAUSIBLE** (looks right but verifier could not fully reproduce the chain of reasoning) / **REFUTED** (verifier finds counter-evidence — e.g. the cited line is a comment, the heading is from a different layer, the assertion is actually sufficient given context).

Verification runs in parallel across findings (`parallel(findings.map(f => () => agent(verifyPrompt(f))))`). The orchestrator may keep the verifier on a different model than the finder where useful.

REFUTED findings are dropped from the synthesized report (but a count is mentioned in the summary so the user knows the noise floor). CONFIRMED and PLAUSIBLE findings are kept and grouped by lens for the synthesis.

## Step 4. Synthesize Report

Produce a single Japanese report. Recommended structure:

```text
# Test Review レポート

対象: <スコープ + ファイル一覧>
レンズ: 構造準拠 / 観点カバレッジ / 意味的品質 / 観点ギャップ(branch×meaning) / シンボル網羅
verifier 通過: CONFIRMED <n> 件 / PLAUSIBLE <m> 件 / REFUTED <k> 件 (フィルタ済み)

## サマリ
- 修正必須: <件数>
- 補完推奨: <件数>
- 再考: <件数>
- 追加検討: <件数>

## 構造準拠（修正必須）
- [<severity>] <file>:<line> — <violated rule>
  - 詳細: <description>
  - 出典: `<README path>` / `scaffold-test/SKILL.md` の該当節
  - verifier: CONFIRMED / PLAUSIBLE

## 観点カバレッジ（補完推奨）
- [<severity>] <test file> — <missing viewpoint>
  - 出典 README: `<README path>:<section heading>`
  - 提案: <suggested t.Run case name>
  - verifier: CONFIRMED / PLAUSIBLE

## 意味的品質（再考）
- [<severity>] <file>:<line> — <weak assertion / brittle coupling>
  - 詳細: <one-sentence why>
  - verifier: CONFIRMED / PLAUSIBLE

## シンボル網羅（補完推奨）
- <file> に対して subject <subject path> から導出（criticality 降順）:
  - シンボル未カバー: <symbol @ subject file:line>（対応 TestXxx 皆無）
  - criticality: <1-10> — 未検証で壊れた場合のリグレッション: <一文>
  - 提案: func Test<Symbol>(t *testing.T) — 正常系 / 異常系 の骨子
  - verifier: CONFIRMED / PLAUSIBLE

## 観点ギャップ: 分岐網羅（追加検討）
- <file> に対して subject <subject path> から導出（criticality 降順）:
  - 分岐未カバー: <subject file:line の分岐>
  - criticality: <1-10> — 未検証で壊れた場合のリグレッション: <一文>
  - 提案: t.Run("<case name>", ...) — カバーする分岐 / sentinel: <reason>
  - verifier: CONFIRMED / PLAUSIBLE

## 観点ギャップ: 意味網羅（再考）
- <file> に対して subject <subject path> から導出:
  - 分岐カバー済み・意味未検証: <subject file:line の分岐> を <test file:line のケース> がカバーするが固有 outcome 未 assert
  - 不足アサーション: <あるべき distinctive assertion>
  - verifier: CONFIRMED / PLAUSIBLE

## 補遺
- pkg/ 層は Test Strategy 節を意図的に持たないため、sibling tests を比較基準にしています（gap 警告なし）。
- <その他、レビュー過程で気付いた README 補完候補 / SKILL の改訂候補>
```

Severity mapping:

- **修正必須** (Structural Compliance lens): rule violations against `docs/testing-conventions.md` / `scaffold-test/SKILL.md` — these are hard rules. CONFIRMED → 修正必須; PLAUSIBLE → 確認推奨.
- **補完推奨** (Viewpoint Coverage lens + Subject Symbol Completeness lens): README declares a viewpoint that is not exercised, OR a subject symbol has no `TestXxx` at all. CONFIRMED → 補完推奨; PLAUSIBLE → 確認推奨.
- **再考** (Semantic Quality lens + Viewpoint Gap Axis B): the test compiles and passes but reveals little — a weak assertion, or a branch that is covered yet does not distinctly assert its outcome. CONFIRMED → 再考; PLAUSIBLE → 補強候補.
- **追加検討** (Viewpoint Gap Axis A): proactive suggestion for an uncovered branch found by subject inspection. CONFIRMED → 追加検討; PLAUSIBLE → 提案.

## Step 5. Next-Action Suggestion

End the report with a single concrete suggestion:

- If 「修正必須」 findings exist → suggest re-running `/scaffold-test` on the affected subjects (which will reset the structure to the canonical pattern) or hand-editing the specific lines.
- If only 「補完推奨」 / 「追加検討」 findings exist → suggest adding the proposed `t.Run` cases either manually or via a follow-up `/scaffold-test` invocation with those subjects in scope.
- If no findings survive verification → state that explicitly (`「verifier 通過後 0 件です」`).

## Chainability

`impl-review` is this skill's caller: its Step 5 delegates the test viewpoint here instead of auditing it itself, which is what that skill's `test-gap` lens means when it says it defers to `/test-review`. While the delegation runs, `impl-review` suppresses `test-gap`, so the single-owner rule this skill already applies between Lens 4 and Lens 5 extends across the skill boundary — Lens 5 owns "the symbol has no test at all", Lens 4 owns branch × meaning, and no third reporter exists for either.

This skill still does not chain *into* anything: the user reads the report and decides whether to invoke `scaffold-test` (regenerate) or hand-edit. It also does not call back into `impl-review` — the review flow's entry point is always the caller.

A caller passes a context payload with:

- `scope` — pre-resolved file list (skips First Step's `ask the user explicitly`).
- `base_ref` — when running in branch-vs-base mode.
- `reviewer_model` — the model the caller resolved to keep reviewer ≠ implementer. Apply it to both the Step 2 finders and the Step 3 verifiers, overriding the `sonnet` default.
- `skip_verifier` — boolean; allows the parent to disable the verify stage for speed (default `false`, i.e. verify by default).

Under a payload, two behaviors differ from a standalone run — everything else is identical:

- **A production file with no paired test stays in scope.** Standalone, an empty `*_test.go` list ends the run. Chained, a production source file whose `<subject>_test.go` does not exist is precisely Lens 5's subject, so keep it and let Lens 5 report the absent test. Lenses 1-3 are test-file-driven and have nothing to read for such a file — skip them for it rather than letting them return an empty result that reads as a pass.
- **The report is returned for the caller to embed, not rendered as a standalone deliverable.** Produce the same Step 4 structure and let the caller place it as a section in its own report. Keep the severities in this skill's vocabulary (修正必須 / 補完推奨 / 再考 / 追加検討, plus criticality) — never remap them onto the caller's, which would silently lose the distinction between "the rule is violated" and "this branch is unverified".

## Constraints (Summary)

- ❌ Editing any file (read-only).
- ❌ Running `make go-test` (this skill reviews tests, not runs them; coverage / pass-status is `make go-test`'s job, run separately).
- ❌ Trusting finder output without verification (the verifier stage is mandatory unless the parent passes `skip_verifier: true`).
- ❌ Hardcoding viewpoint lists (the SSOT is the layer README's Test Strategy section; `pkg/` is the documented exception).
- ❌ Hardcoding the semantic-quality anti-pattern catalogue (Lens 3) or the 意味網羅 bar (Lens 4 Axis B) — the SSOT is `docs/testing-conventions.md` section 10, read at runtime.
- ❌ Duplicating rules already in `docs/testing-conventions.md` or `scaffold-test/SKILL.md` (the skill reads them at runtime).
- ✅ Default to skepticism in the verifier (PLAUSIBLE > CONFIRMED when ambiguous).
- ✅ Default reviewer model is `sonnet` (different from Opus implementers); orchestrator may override to keep reviewer ≠ implementer.
- ✅ Default scope is changed files; alternative scopes selectable.
- ✅ Final report in Japanese, grouped by lens with severity tags.
- ✅ Surface `pkg/` as an intentional "no Test Strategy" layer, never as a documentation gap.
- ✅ criticality (1-10) は Lens 4 Axis A（追加検討）と Lens 5（補完推奨・シンボル未カバー）の finding に付す本番影響のソート鍵で、レンズ由来 severity（修正必須 / 補完推奨 / 再考 / 追加検討）を置換しない。構造準拠（修正必須）には付けない。
- ✅ コード起点は2レンズ体制（Lens 5=シンボル存在 / Lens 4=関数内 branch×meaning）。「テストが1つも無いシンボル」は Lens 5 が所管し、Lens 1 reverse / Lens 4 とは二重報告しない。`impl-review` から chain されている間は同スキルの `test-gap` lens が停止しているので、所管はスキル境界を跨いでも 1 つのまま。
- ✅ payload 受領時は First Step の質問を出さず、ペアテスト不在の production ファイルもスコープに残し（Lens 5 の対象）、レポートは呼び手が埋め込む前提で自分の severity 語彙のまま返す。

## Checklist

Before reporting completion, confirm:

- [ ] Scope was resolved (changed files / base diff / explicit paths).
- [ ] Each target `*_test.go` has its subject source file located.
- [ ] Layer README + `docs/testing-conventions.md` + `scaffold-test/SKILL.md` + sibling tests were read in Step 1.
- [ ] All five lenses ran (in parallel).
- [ ] Lens 5 built the subject symbol table and flagged every symbol with no `TestXxx` (→ 補完推奨), before Lens 4 branch analysis — the two code-origin lenses did not double-report a zero-test symbol.
- [ ] Lens 4 ran both axes per subject — Axis A 分岐網羅 (uncovered branches → 追加検討) and Axis B 意味網羅 (covered-but-vacuously-asserted branches → 再考).
- [ ] Every finding from every lens went through `review-verifier` (unless `skip_verifier: true`).
- [ ] REFUTED findings were dropped; CONFIRMED / PLAUSIBLE were kept.
- [ ] Final report is Japanese, grouped by lens, with severity tags.
- [ ] Next-action suggestion is one concrete recommendation.
- [ ] No files were edited.
