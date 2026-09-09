---
name: impl-review
description: >-
  Local adversarial, low-bias code review of the current change, run by subagents on a DIFFERENT model than the implementer. Mirrors `/code-review`'s finder → verify shape but keeps everything local and adds a runtime (curl + o11y) stage that mocked tests cannot cover. Confirms scope via `ask the user explicitly` (changed files vs branch-vs-base diff vs specific paths), ranks every finding by a fixed tier order (1 architecture / ddd-modeling, 2 security / correctness, 3 runtime-gap / type-design, 4 test-gap, 5 comment quality) so a higher tier's decision propagates down while a lower one never silently outranks it, fans out `adversarial-reviewer` subagents — one per lens (correctness / security / architecture / runtime-gap / test-gap, where `test-gap` is a code-origin pass that reads the changed production source and flags reachable branches / whole changed symbols left untested or vacuously asserted — a high-signal subset that defers exhaustive per-symbol enumeration to `/test-review`) — plus the dedicated `ddd-modeling-reviewer` (tier 1; asks whether the change models the domain well by this repo's own written DDD interpretation — aggregate boundary vs transaction boundary, where a rule belongs, cross-aggregate references, ubiquitous language — never against Evans directly, which is `ddd-origin-auditor`'s subject) and `comment-reviewer` subagent for comment quality, each on a user-selected model (fable / sonnet / opus / haiku; default auto = a model ≠ the implementer) so reviewer ≠ implementer — then verifies each finding with an independent `review-verifier` subagent (CONFIRMED / PLAUSIBLE / REFUTED), optionally runs the runtime curl + o11y check for touched endpoints (orchestrator-driven, per `scaffold-endpoint` Phase 7), and synthesizes a single Japanese report. The `comment-reviewer` both validates good comments (the What is correct / sufficient / substantive and a non-obvious Why is present) and flags bad ones (How narration / 経緯 / restatement / tautology). Comment quality is not just reported but PROCESSED inside the lifecycle: CONFIRMED comment findings are auto-fixed in the working tree after one confirmation (delete / rewrite / enrich, with guards — never remove functional directives like `//go:generate` / `//nolint` / build tags, rewrite-or-enrich rather than delete exported-Go doc comments so `revive exported` stays satisfied, keep good What + non-obvious Why, skip generated files / Markdown prose / the deny list), then `make go-fix` + `make go-lint` verify. The other five lenses stay read-only on source (no auto-fix) and any destructive runtime curl is confirmed with the user first. By default the surviving CONFIRMED / PLAUSIBLE findings from the read-only lenses are posted to the branch's PR as inline review comments anchored to each finding's line (opt out with `--no-comment`; comment-style findings are applied, not posted). Step 0 additionally asks — default yes — whether to delegate the test viewpoint to `/test-review`: when the diff touches non-generated production `.go` under `internal/**` / `pkg/**` or any `*_test.go`, Step 5 chains that skill inline with a `scope` / `base_ref` / `reviewer_model` / `skip_verifier` payload for the exhaustive two-axis audit (Lens 4 branch × meaning + Lens 5 symbol completeness) and SUPPRESSES the `test-gap` lens for that run so the two never double-report; the report's mandatory `テスト観点:` line always states which of the three states applied (委譲実施 / test-gap subset only / not run), so an unaudited test viewpoint can never read as covered. Use before commit / PR to get an independent second opinion that the implementer's own model would not surface. Flags: `--no-comment` (skip PR posting), `--no-apply` (report comment-style findings instead of auto-fixing).
---

# Impl Review

Independent, adversarial, **different-model** code review you can run locally — no Copilot, no cloud `/code-review`. The implementer's own model has blind spots; the whole point is to review with another model so those blind spots get caught. Built on the `/code-review` finder → verify pattern, plus a runtime curl + o11y stage that mocked unit tests structurally cannot reach.

A Japanese reference translation of this skill lives at `SKILL.ja.md` in this directory (for human reference only; not loaded as a skill).

## When to Use

- Before committing / opening a PR, to get a second opinion the implementer's model would not produce on its own.
- After a multi-layer change where mocked tests pass but DI / middleware / real-DB behavior is unverified.
- Whenever you want an adversarial pass focused on bugs, auth/IDOR, and layer violations.

Do NOT use this skill for:

- Style / formatting — `make go-fix` / `make go-lint`.
- Exhaustive layer-compliance auditing — `arch-check` (this skill's `architecture` lens flags only high-signal violations).
- Spec validation — `verify-spec`.
- Applying non-comment fixes — for the five code lenses this skill is read-only; it reports, the user fixes. (Exception: **comment-style findings are auto-applied** in Step 7 — verbose / narrating comments are actually fixed, not just reported.)

## Contract

| | |
| --- | --- |
| **Owns** | The change itself (`architecture` / `ddd-modeling` / `security` / `correctness` / `runtime-gap` / `type-design`) |
| **Does not** | Test viewpoint (`test-review`) / comment viewpoint (`comment-sweep`) / invoke other skills |
| **Starts when** | A reviewable diff and its intent exist |
| **Stops when** | A tier 1 finding requires a human design decision |

## Core Idea — reviewer ≠ implementer

Bias reduction is the design constraint, not a nicety. Reviewers therefore run as **subagents on a different model than whoever wrote the code**:

- The reviewer agents (`adversarial-reviewer`, `comment-reviewer`, `review-verifier`) default to **`sonnet`** in their frontmatter, which differs from the usual Opus implementer.
- **The reviewer model is chosen by the user in Step 0.** The options are `fable` (Fable 5) / `sonnet` / `opus` / `haiku`, plus an *auto* default that resolves to a model ≠ the session's implementer. Pass the chosen model to every reviewer subagent via the `Agent` tool's `model` parameter (it takes precedence over the agent file's `sonnet` default) — e.g. `opus` for depth, `haiku` for a cheap divergent pass, `fable` for a fresh independent perspective.
- **The orchestrator MUST guarantee reviewer ≠ implementer.** If the user selects the same model as the session's implementer, warn that it undermines the different-model bias reduction and confirm before proceeding. Never silently let reviewer and implementer be the same model.
- Reviewer subagents are **read-only** (their agent files grant no Edit/Write) — they only return findings. The single place this skill mutates source is Step 7, where the **orchestrator** (not a subagent) applies the verified comment-style fixes after user confirmation. The five code lenses are never auto-fixed.

## Precedence — findings are ranked, not just collected

Reviewers disagree, overlap, and report the same fact in two vocabularies. Without a ranking the
report is a flat list in which a comment nit outranks a wrong aggregate boundary because its finder
called it "high". The tiers in the Step 2 table are that ranking:

| Tier | Lenses | What it decides |
| --- | --- | --- |
| 1 | `architecture`, `ddd-modeling` | what the code should *be* |
| 2 | `security`, `correctness` | whether what it is, works |
| 3 | `runtime-gap`, type design | whether it holds up in the real system and in its types |
| 4 | `test-gap` | whether it is pinned down |
| 5 | comment quality | how it reads |

**A change at a higher tier propagates downward; a lower tier does not, as a rule, act on a higher
one.** Rewriting an aggregate boundary invalidates the tests written against it and the comments
describing it; a comment finding never justifies changing a boundary. Five consequences follow, and
each of them is a rule, not a suggestion:

1. **Order the report by tier, then by severity within a tier** — never by severity alone. A
   comment-quality `high` sits below an architecture `medium`, because the architecture finding may
   delete the code the comment is on.
2. **Mark a lower-tier finding 保留 while a higher-tier finding it depends on is unresolved.** Report
   it, say what it is waiting on, and do not present it as actionable. Re-check it after the
   higher-tier decision lands; it often disappears.
3. **Suppress the Step 7 comment auto-fix for any file a tier 1–2 finding is likely to rewrite.**
   Polishing prose on code that is about to change is work done twice, and it buries the real finding
   under a diff of comment edits. Say in the report which files were held back and why.
4. **When two tiers report the same fact, keep the higher tier's framing and fold the lower one in as
   corroboration** — one finding, not two. Two entries for one fact reads as two problems and
   double-counts the change's apparent risk.
5. **Agreement among lenses at the same tier raises confidence; agreement from a lower tier does
   not raise a higher finding's severity.** Two tier-2 lenses independently reaching the same defect
   is strong evidence — say so. A tier-5 lens agreeing with a tier-1 finding adds nothing to its
   severity, though it may be cited as support.

**The exception is criticality, and it is yours to notice, not to resolve.** A lower-tier finding
can be the more urgent one — an exploitable hole surfaced by the `test-gap` lens does not wait for an
architecture debate. When a lower-tier finding looks critical enough to outrank the tier above it,
**do not silently reorder: present both and ask the user.** The ranking exists so that ordinary
disagreements resolve without a human; a finding that breaks the ranking is exactly the case a human
should see.

## Step 0 — Confirm Scope

Call `ask the user explicitly` immediately. Default-detect scope by checking branch vs base; if there are unmerged commits, default to "changed files", otherwise "whole working tree / specific paths".

Resolve the base so the review covers the same diff the pull request shows: an existing PR's
`baseRefName` wins; with no PR, use `make base-branch` to resolve the latest release line (this
repository's base is always a `release/*` branch) from `origin`'s live state. Do not fall back to
`gh repo view --json defaultBranchRef`: the GitHub default branch can name an earlier release line,
silently widening the diff to a generation of changes nobody asked to review. Stop and report the
failure in Japanese if the base cannot be resolved.

```text
質問: どの範囲をレビューしますか？
選択肢:
  - 変更ファイルのみ（ベースブランチとの diff）  ← 未マージのコミットがある場合の既定
  - 作業ツリーの未コミット変更（git status の差分）
  - 特定のパス/ファイルを指定
  - キャンセル
```

### Reviewer model selection

In the same `ask the user explicitly` call (a second question alongside scope), ask which model the
reviewer subagents run on. `fable` (Fable 5) is available alongside the existing tiers:

```text
質問: レビュアーをどのモデルで実行しますか？（バイアス低減のため 実装者 ≠ レビュアー を推奨）
選択肢:
  - 自動（実装者と異なるモデルを既定選択）  ← 既定
  - fable（Fable 5）
  - sonnet
  - opus（深掘り）
  - haiku（安価・高速な発散パス）
```

*Auto* resolves to the agent-file default (`sonnet`) when the implementer is not `sonnet`,
otherwise to a different tier. If the user picks the implementer's own model, warn (per Core
Idea) that it weakens the different-model guarantee and confirm before continuing. The chosen
model is passed to every `adversarial-reviewer` / `comment-reviewer` / `review-verifier`
`Agent` call via the `model` parameter in Step 2 and Step 3, and to `/test-review` as the
`reviewer_model` payload field in Step 5.

### Test-viewpoint delegation

In the same `ask the user explicitly` call (a third question), ask whether to delegate the test
viewpoint to `/test-review`. Ask it **unconditionally** — the file-set predicate that decides
whether the delegation actually has anything to audit is resolved in Step 1, which runs after
the scope this question is asked alongside:

```text
質問: テスト観点を /test-review へ委譲しますか？（既定: 委譲する）
選択肢:
  - 委譲する（/test-review を Step 5 で実行。test-gap lens は停止）  ← 既定
  - 委譲しない（test-gap lens のみ。変更シンボルの高シグナル・サブセット）
```

The default is *delegate*: the `test-gap` lens is by its own definition a subset that defers to
`/test-review`, so leaving the delegation off by default means the deferral never happens and
the gap is only visible to whoever remembers to run the other skill. Declining is a real choice
— the full audit roughly doubles the review's cost — which is why it stays a question rather
than becoming unconditional. Whichever way it goes, Step 6 records it on the `テスト観点:` line.

### Flags

- `--no-comment` — suppress Step 8 (do not post to the PR); produce the local report only. **Default is opt-out**: when an open PR exists for the current branch, Step 8 posts the surviving findings as inline review comments unless this flag is given.
- `--no-apply` — suppress Step 7 (do not auto-fix comment findings); instead report them and let them flow into Step 8 (PR post) like the other lenses. **Default is to apply**: comment quality findings are auto-fixed in the working tree after one confirmation.

## Step 1 — Gather Context

Stamp the review boundary first — it is the one moment nothing else observes, and the loop reads
it to separate time spent reviewing from time spent implementing:

```sh
.agents/closed-loop/marks.sh reviewStartedAt 2>/dev/null || true
```

- Resolve the base ref and produce the review target: `git diff <base>...HEAD` (or `git diff` for uncommitted), plus the changed-file list (`git diff --name-only ...`).
- Detect which layers/areas are touched (`internal/controller/**`, `usecase`, `domain`, `infrastructure`, `pkg`, `openapi/**`, `database/**`).
- Note whether any **endpoint** is touched (controller handler or `openapi/**`) — this decides whether Step 4 runs.
- Note whether any **shared** OpenAPI component is edited (a `components/*` referenced by more than one operation) — this widens Step 4 to every consumer.
- Note whether any **non-generated production `.go`** under `internal/**` / `pkg/**` is touched (exclude `*_test.go`, `*.gen.go`, `*.sql.go`, `*_mock.go`) — this gives the code-origin test analysis its changed-symbol list.
- Note whether any **`*_test.go`** is touched. Together with the previous bullet this resolves the **test-viewpoint predicate**: `production .go touched OR *_test.go touched`. A test-only change satisfies it through the second disjunct alone, which is the case `test-gap` cannot see (it reads production source) and the one `/test-review` exists for.
  - Predicate true **and** the user chose to delegate in Step 0 → Step 5 runs and the `test-gap` lens does **not**.
  - Predicate true **and** the user declined **and** production `.go` was touched → `test-gap` runs as the subset, Step 5 does not.
  - Predicate true **and** the user declined **and** only `*_test.go` was touched → neither runs. `test-gap` reads changed production source, so on a test-only diff it has no symbol to enumerate; spawning it would return an empty result that reads as a clean audit. This is the one state where declining leaves the test viewpoint entirely unexamined — say so on the `テスト観点:` line rather than letting an empty lens stand in for it.
  - Predicate false → neither runs; there is no test viewpoint to audit.

  Record which of the four applies — Step 6 reports it on the `テスト観点:` line (the last two both map to `未実施`, with the reason distinguishing them).

## Step 2 — Fan-out Finders (different model, concurrent)

Spawn all finders concurrently (issue every `Agent` call in a single message). Apply the model rule from Core Idea — pass the Step 0 user-selected reviewer model to every `Agent` call via the `model` parameter (omit only when *auto* already resolves to the agent-file default). Two agent types:

- The four **code lenses** run `adversarial-reviewer` — one per lens, `agentType: "adversarial-reviewer"`, `label` like `find:security`.
- The **comment dimension** runs the dedicated `comment-reviewer` — `agentType: "comment-reviewer"`, `label: "find:comment"`. This is the stronger, comment-focused agent (a richer taxonomy than a one-paragraph lens), and its findings feed the Step 7 auto-fix.
- The **DDD modeling dimension** runs the dedicated `ddd-modeling-reviewer` — `agentType: "ddd-modeling-reviewer"`, `label: "find:ddd"` — when the diff touches `internal/domain/**` or `internal/usecase/**`. It asks whether the change models the domain well by this repository's own written interpretation (aggregate boundary vs transaction boundary, where a rule belongs, cross-aggregate reference discipline, ubiquitous language, Factory / Repository semantics). It is a **tier 1** lens: its findings decide what the code should be, so they are settled before the lower tiers act. Do NOT point it at Evans directly — that is `ddd-origin-auditor`'s subject, and its subject is the repo's documents rather than code.
- The **type-design dimension** runs the dedicated `type-design-reviewer` — `agentType: "type-design-reviewer"`, `label: "find:type-design"` — ONLY when the diff touches domain types (`internal/domain/**/*.go`). It scores each type on the four-axis rubric (Encapsulation / Invariant Expression / Invariant Usefulness / Invariant Enforcement); its findings are suggestion-level (not auto-fixed).

| Finder | Tier | Agent | Run when |
| --- | --- | --- | --- |
| `architecture` | 1 | adversarial-reviewer | always |
| `ddd-modeling` | 1 | **ddd-modeling-reviewer** | when the diff touches `internal/domain/**` or `internal/usecase/**` |
| `correctness` | 2 | adversarial-reviewer | always |
| `security` | 2 | adversarial-reviewer | always (especially when a handler / auth / DTO / `openapi/**` is touched) |
| `runtime-gap` | 3 | adversarial-reviewer | when a controller / DI / `openapi/**` / `database/**` is touched |
| `test-gap` | 4 | adversarial-reviewer | when the diff touches non-generated production `.go` under `internal/**` / `pkg/**` **and** the Step 0 test-viewpoint delegation was declined — suppressed while Step 5 runs |
| comment quality | 5 | **comment-reviewer** | when the diff adds / changes any code comment (almost always) |
| type design | 3 | **type-design-reviewer** | when the diff touches domain types (`internal/domain/**/*.go`) |

Each `adversarial-reviewer` prompt MUST include: the lens name + its definition, the base ref + changed-file list + the diff, and pointers to `AGENTS.md` / the relevant `README.md` / OpenAPI spec / migrations.

**`test-gap` lens definition** (this lens is *code-origin* — it reads the changed production source, not the test files): for each production symbol added or changed in the diff, enumerate its logical branches / error sentinels / boundary conditions / zero-value defenses, then check the paired `*_test.go` reaches each and *distinctly* asserts it (specific sentinel via `require.ErrorIs`, the distinguishing value/state — not just `require.Error` / `NoError`). Report two shapes: a production symbol changed in the diff with **no test at all**, and a reachable branch of a changed symbol left **untested or vacuously asserted**. This is a **high-signal subset** — impl-review flags the reachable gaps a test-file-first read misses on the *changed* code; it does NOT do exhaustive per-symbol enumeration across the package. The full two-axis matrix (Lens 4 branch×meaning + Lens 5 symbol completeness) over all subjects belongs to `/test-review`, and Step 5 is where this skill actually hands it over. Findings are read-only suggestions (never auto-fixed) and anchor to the subject line in the diff, so they post inline like the other code lenses.

**Single owner.** This lens and `/test-review` audit overlapping ground, so exactly one of them runs. When Step 5 delegates, do **not** spawn `test-gap`: `/test-review` Lens 5 owns "the symbol has no test at all" and Lens 4 owns branch × meaning, and adding this lens on top would report the same gap twice under two severity vocabularies. `test-gap` is what remains when the user declines the delegation — the subset that still catches the worst gaps on changed code at a fraction of the cost, not a redundant second opinion.

The `comment-reviewer` prompt MUST include: the base ref + changed-file list + the diff, the **line policy** (judge only comments on changed lines for a diff scope), and a pointer to `docs/rules.md` ("Comment Rules") as the authoritative policy it reads at runtime. The agent already encodes the all-languages-uniform standard (Go and non-Go alike — shell / `.mjs` / Dockerfile / Makefile / SQL / YAML; non-Go is higher-risk, not exempt) and the functional-directive / exported-doc-comment guards — do not re-specify or soften them here. Restrict the file list it sees to comment-bearing source files: exclude generated files (`**/*.gen.go`, `*_mock.go`, `**/openapi.gen.yaml`, `// Code generated ... DO NOT EDIT`), `vendor/**`, the deny list, and Markdown / docs prose (the Comment Rules govern source comments, not standalone documents).

## Step 3 — Adversarial Verify

Collect all findings and **dedup** by (file, line, claim) — and when two lenses report the same fact, apply Precedence rule 4: keep the higher tier's framing, fold the lower one in as corroboration, and carry ONE finding forward. For each surviving finding, spawn one `review-verifier` subagent (concurrently), handing it the single finding + the base ref. Use `agentType: "review-verifier"`, `label` like `verify:<file>`, and the Step 0 user-selected reviewer `model` (same reviewer ≠ implementer rule).

- Keep **CONFIRMED** and **PLAUSIBLE** findings. Drop **REFUTED** (but keep a count for the report).
- For a critical/high finding where a single verdict feels shaky, spawn 2–3 verifiers and go by majority — diversity beats one opinion on the findings that matter.

## Step 4 — Runtime Verification (curl + o11y) — endpoints only

Run this **only if Step 1 found a touched endpoint**, and run it from the **orchestrator (main session)**, not a subagent — it needs interactive bash, real DB/state, log reading, and possibly user confirmation. Follow `scaffold-endpoint` Phase 7:

1. `make go-test` (mocked) does NOT build the real Fx graph, run auth/OpenAPI middleware, or touch the DB — so this stage exists to catch what Step 2's `runtime-gap` lens only *predicts*.
2. Pick/seed a target row in a known state. For credential/state-sensitive checks, create a row whose plaintext/state you control.
3. `curl` the touched endpoint(s) (local auth: `Authorization: Bearer debug:<subject>`) and assert: happy path; key error paths (404 / 400 / 422); and — **if the operation declares `security:`** — no-token ⇒ 401 (prove it is actually protected). For IDOR-shaped findings, curl as a *different* subject and assert it cannot reach another subject's resource.
4. **Shared-schema impact:** if a shared `components/*` was edited (Step 1), curl **every** consumer endpoint, not just the changed one — `grep` the spec for `$ref`s and exercise each.
5. Read the o11y logs once for a single request: confirm the trace spans controller → usecase → infra and the emitted SQL is what you expect. Later re-checks can rely on o11y instead of re-curling.
6. **Destructive guard:** if a curl mutates data and the only restore path is `make db-init` (or similar), confirm with the user before running it (per `AGENTS.md`). Clean up rows you created.

Fold any runtime-confirmed defect into the report as CONFIRMED with the curl/o11y evidence.

## Step 5 — Delegate the Test Viewpoint to `/test-review`

Run this when the Step 1 test-viewpoint predicate is true **and** the user chose to delegate in
Step 0. Skip it otherwise and let the `test-gap` lens (or nothing) stand — either way Step 6 says
which happened.

Invoke the `test-review` skill via the Skill tool with:

- `scope`: the resolved file list — **both** the changed non-generated production `.go` and the
  changed `*_test.go`. Passing production files whose paired test does not exist is the point, not
  an error: that pair is Lens 5's finding.
- `base_ref`: the base resolved in Step 1, when the scope is a branch-vs-base diff.
- `reviewer_model`: the model the user selected in Step 0, so the delegated finders and verifiers
  inherit the same reviewer ≠ implementer guarantee.
- `skip_verifier`: `false`. This skill verifies every finding before reporting it; buying speed by
  dropping the verify stage would let unverified findings into a report whose other half is
  verified, which is worse than not running the audit.

The chain is **sequential and inline** — the orchestrator loads `test-review` and executes its
steps in this session, the same shape every other chain in this repo uses. `/test-review` is
read-only, so nothing here needs the working-tree confirmation that Step 7 does, and the
delegated run raises no question of its own (the `scope` payload skips its First Step). It runs
after Step 3 / Step 4 rather than alongside Step 2's fan-out: merging the two fan-outs would mean
hoisting `/test-review`'s own context-reading step into this skill's, which duplicates a procedure
that already has an owner.

Keep the returned report's structure and severities as they are (修正必須 / 補完推奨 / 再考 /
追加検討, plus criticality). Step 6 embeds it as a section — do not remap it onto CONFIRMED /
PLAUSIBLE × 重大度, which would collapse "the convention is violated" and "this branch is
unverified" into one axis.

## Step 6 — Synthesize Report (Japanese)

Produce one Japanese report:

```text
## ローカルレビュー結果（reviewer: <model> / implementer: <model>）

スコープ: <base>...HEAD（<N> files） / lens: <実際に走らせた lens のみを列挙>
ランタイム検証: 実施（curl/o11y）/ 対象外（エンドポイント変更なし）
テスト観点: <下記 3 状態のいずれか>

### CONFIRMED（要対応）
- [重大度] タイトル — path:行
  - 問題 / 根拠 / 修正案
  - 検証: verifier 判定（+ 該当すれば curl/o11y 結果）

### PLAUSIBLE（要確認・判断保留）
- ...

### テスト観点（/test-review 委譲結果）
- <委譲したときのみ。/test-review の Step 4 レポートをそのまま埋め込む>

### 補足
- REFUTED: <n> 件（finder が挙げたが verifier が否定）
- ランタイム検証でカバーした経路 / スキップした経路
```

The `lens:` line lists only the lenses that actually ran — when Step 5 delegated, `test-gap` is absent from it (it was suppressed) and `/test-review 委譲` takes its place.

The **`テスト観点:` line is mandatory** and takes exactly one of three values:

- `委譲実施（/test-review Lens 1-5 / CONFIRMED <n>・PLAUSIBLE <m>）`
- `test-gap レンズのみ（変更シンボルの高シグナル・サブセット。全シンボル網羅は未実施）`
- `未実施（テスト関連の変更なし / テストのみの変更で委譲を辞退したため test-gap にも対象が無い）`

It exists for the same reason the runtime line does: without it, a `lens:` list containing `test-gap` reads as "the tests were audited" when only a subset of the changed symbols was looked at, and a run with no test analysis at all leaves no trace. State the weaker case plainly rather than letting the omission pass for coverage.

Order by **tier first, then severity within the tier**, CONFIRMED before PLAUSIBLE (Precedence rule 1). Mark every finding that is waiting on a higher-tier decision as `保留` and name what it waits on (rule 2). Always state what runtime checks ran and what was skipped — silent omission reads as "covered everything" when it was not. In the report, keep the **comment quality** findings in their own subsection — they are *processed* in Step 7, not posted to the PR. Likewise keep the delegated test findings in their own section with their own severity vocabulary; omit the section entirely when Step 5 did not run (the `テスト観点:` line already carries that fact).

## Step 7 — Apply Comment Fixes (default; skip with `--no-apply`)

This is the one place the skill mutates source. Apply the verified **comment quality** findings (CONFIRMED, plus any PLAUSIBLE the user opts in) yourself — the `comment-reviewer` subagent never edits. The five code lenses are NOT auto-fixed here; they go to Step 8.

**First apply Precedence rule 3.** List the files a surviving tier 1–2 finding is likely to rewrite, and exclude them from this step — a comment polished onto code that is about to change is work done twice. State the held-back files and the reason in the report.

Confirm once before editing:

- `ask the user explicitly`: 「コメント指摘 <N> 件をライフサイクル内で修正適用しますか？」 — options: 「すべて適用」 / 「1件ずつ確認」 / 「適用しない（レポートのみ／PR コメント化）」.

Apply the action each finding carries — **削除 (delete)** a bad-content comment, **書換 (rewrite)** to a correct/behavioral What, or **加筆 (enrich)** a thin What / missing non-obvious contract / missing good Why. A `誤り/陳腐化` finding (the What contradicts the code) is corrected, not deleted. Obey these guards (a wrong deletion here is a real regression):

- **Never delete functional / directive comments**: `//go:generate`, `//nolint:...`, `//go:build` / `// +build`, `//go:embed`, `//export`, cgo preamble, `//revive:...`, `// Code generated ... DO NOT EDIT`, shebangs, tool directives.
- **Exported Go declarations** (uppercase `func`/`type`/`const`/`var`/method): **rewrite or enrich, never delete** the doc comment — `revive exported` requires it; keep the leading-identifier form (`// Foo は …`).
- **Keep good comments**: a correct, sufficient What and a non-obvious Why (rationale / load-bearing constraint) are not findings — do not strip them. Rewrites/enrichments describe **What + non-obvious Why**, never **How** or development 経緯. Edit only in-scope files; never touch generated files, Markdown prose, or the deny list. Use `Edit`, one finding (or one file) at a time.

After editing, verify:

1. `make go-fix` — absorb formatting / auto-fixes.
2. `make go-lint` — confirms `revive exported` still passes (catches an accidentally-removed required doc comment) and nothing else regressed.
3. `git diff` the touched files and confirm only prose comments changed (no functional directive caught). For non-Go, re-read the changed hunks.
4. On failure, surface it and stop — do not auto-revert; the user decides. Do NOT commit — leave the changes for the user (or a later `/commit`).

If `--no-apply`, skip this step and instead let the comment findings flow into Step 8 (posted to the PR like the other lenses).

## Step 8 — Post Findings as Inline PR Comments (default; opt out with `--no-comment`)

By default, after Step 7, post the surviving **CONFIRMED + PLAUSIBLE** findings **from the five code lenses** (correctness / security / architecture / runtime-gap / test-gap) to the branch's PR as **inline review comments** — one per finding, anchored to its `path:line`, instead of a single wall-of-text comment. **Never post REFUTED.** Comment quality findings are NOT posted here — they were applied in Step 7 (unless `--no-apply` was given, in which case include them in this post). The Step 6 local report is still produced regardless; this step is additive.

**Delegated test findings (Step 5)** join this post under one restriction: **only those whose anchor line falls inside a PR diff hunk**. All four severities qualify (修正必須 / 補完推奨 / 再考 / 追加検討) — narrowing to 修正必須 would drop the branch-gap findings the suppressed `test-gap` lens used to post, i.e. a regression in what the PR shows. Prefix them `🔎 [test-review · <severity>]` so they read apart from the code lenses, and keep the severity word as-is. Append `· crit <n>` only to the findings that actually carry a criticality — `/test-review` assigns it to Lens 4 Axis A and Lens 5 findings and explicitly withholds it from structural-compliance ones, so it is a per-finding attribute, not a per-severity one. Never invent a score to fill the slot.

Off-diff test findings — typically Lens 5 symbols in files this PR never touched — stay in the **local report only**. Do not fold them into the review summary `body` the way an off-diff code finding is folded: an off-diff code finding is still a defect this change causes, whereas an untested pre-existing symbol is standing coverage debt that this PR neither introduced nor is the place to argue about. Say in the local report how many were withheld and why, so the omission is visible.

Skip this step entirely when:

- invoked with `--no-comment`, OR
- no open PR exists for the current branch (`gh pr view` returns nothing) — keep the local report only and optionally offer to open a PR.

Posting to GitHub is an outward-facing action, so confirm **once** before posting — show the count and the target PR (`ask the user explicitly`: 「<N> 件の指摘を PR #<番号> にインラインコメントとして投稿しますか？」/「投稿する」「投稿しない（ローカルレポートのみ）」).

### Procedure

1. Resolve PR number, repo, and the commit the comments anchor to:

   ```sh
   gh pr view --json number,url -q '.number'        # PR number
   gh repo view --json nameWithOwner -q '.nameWithOwner'
   git rev-parse HEAD                                # anchor SHA
   git rev-parse @{u}                                # pushed head — warn if it differs from HEAD
   ```

   The anchor commit MUST be the commit pushed to the PR. If local `HEAD` ≠ `@{u}`, warn the user to push first (the API rejects comments whose `commit_id` is not on the PR).

2. Decide which findings can be inline. A GitHub inline comment must target a line present in the PR diff. Parse the diff hunks (`gh pr diff <PR> --patch` or `git diff <base>...HEAD`):
   - `(path, line)` inside an added/context hunk → inline comment, `side: "RIGHT"`.
   - `(path, line)` on a removed line → inline comment, `side: "LEFT"`.
   - Off-diff (the reviewer referenced unchanged context) → **cannot** be inline; fold it into the review summary `body`.

3. Build one review and post all comments atomically (a single review, not N standalone comments):

   ```sh
   gh api --method POST repos/<owner>/<repo>/pulls/<PR>/reviews --input payload.json
   ```

   `payload.json`:

   ```json
   {
     "commit_id": "<SHA>",
     "event": "COMMENT",
     "body": "🔎 impl-review (reviewer: <model>) — CONFIRMED <n> / PLAUSIBLE <m>\n\ndiff 外で行アンカー不可の指摘:\n- <path>: <要約>",
     "comments": [
       {
         "path": "<file>",
         "line": <n>,
         "side": "RIGHT",
         "body": "🔎 [CONFIRMED · high] <問題の要約>\n\n根拠: <...>\n修正案: <...>\n検証: <verifier 判定>"
       }
     ]
   }
   ```

   Use `event: "COMMENT"` — this is an advisory review, never `REQUEST_CHANGES` / `APPROVE`. Prefix every comment body with `🔎 impl-review` (or the `🔎 [verdict · severity]` tag) so the posts are distinguishable from human review.

4. Robustness: if the API rejects the batch (422 — a line is not in the diff), move the offending comment(s) to the summary `body` and retry. Report afterward what was posted inline vs. summarized — never silently drop a finding.

## Do / Do NOT

- ✅ Guarantee reviewer model ≠ implementer model (user selects it in Step 0; warn + confirm if they pick the implementer's model).
- ✅ Rank findings by tier (Precedence): order the report by tier, hold lower-tier findings that wait on a higher one, fold duplicate facts into the higher tier's framing, and ask the user when a lower-tier finding looks critical enough to outrank the tier above it.
- ✅ Run finders concurrently (one message, multiple `Agent` calls): the five code lenses via `adversarial-reviewer`, comment quality via `comment-reviewer`.
- ✅ Independently verify every finding before reporting; drop REFUTED.
- ✅ Run the runtime stage for touched endpoints; widen to all consumers on a shared-schema edit.
- ✅ Ask about the test-viewpoint delegation in Step 0 (default: delegate) and, when it is taken, run Step 5 with `test-gap` suppressed.
- ✅ State the test viewpoint's state on the `テスト観点:` line of every report — including the runs where nothing was audited.
- ✅ Confirm with the user before any destructive curl whose only restore path is `make db-init`.
- ✅ Apply comment quality findings in Step 7 after one confirmation (delete / rewrite-to-behavior), then `make go-fix` + `make go-lint`; skip with `--no-apply`.
- ✅ By default, post the five code lenses' CONFIRMED + PLAUSIBLE findings to the branch's PR as inline review comments (Step 8); suppress with `--no-comment` or when no open PR exists.
- ✅ Confirm once before posting to the PR (outward action); anchor each comment to its `path:line`, fold off-diff **code-lens** findings into the review summary (off-diff *test* findings are excluded — they stay local, per Step 8).
- ❌ Post REFUTED findings, or use `REQUEST_CHANGES` / `APPROVE` — the posted review is advisory `COMMENT` only.
- ❌ Auto-fix the five code lenses — those are reported, the user fixes. Only comment quality is auto-applied (Step 7).
- ❌ In Step 7, delete a functional directive (`//go:generate` etc.) or an exported-decl doc comment (rewrite it); touch generated files / Markdown / the deny list; or auto-commit.
- ❌ Run `test-gap` and `/test-review` on the same review — one owner per gap, never two reports of it.
- ❌ Remap the delegated findings' severities onto CONFIRMED / PLAUSIBLE × 重大度, or post an off-diff test finding to the PR.
- ❌ Order the report by severity alone, report one fact as two findings from two tiers, let a lower-tier lens raise a higher finding's severity, or silently reorder the tiers when a lower finding looks critical — present both and ask.
- ❌ Auto-apply comment fixes to a file a surviving tier 1–2 finding is likely to rewrite.
- ❌ Let a reviewer run on the same model as the implementer.
- ❌ Report speculative style nits as findings, or pad the list to look thorough.

## Checklist

- [ ] Scope confirmed via `ask the user explicitly`; base ref resolved.
- [ ] Reviewer model selected in Step 0 and verified ≠ implementer model (warn + confirm if same).
- [ ] Test-viewpoint delegation asked in Step 0; predicate resolved in Step 1; the resulting state recorded.
- [ ] Finders fanned out concurrently: the code lenses (`adversarial-reviewer`) + comment quality (`comment-reviewer`) — `test-gap` among them only when the delegation was declined **and** production `.go` was touched.
- [ ] Duplicate facts folded into the higher tier; report ordered by tier then severity; lower-tier findings waiting on a higher one marked `保留`.
- [ ] Comment auto-fix skipped for files a surviving tier 1–2 finding is likely to rewrite.
- [ ] Every finding independently verified; REFUTED dropped (count kept).
- [ ] Runtime curl + o11y done for touched endpoints (shared-schema → all consumers); destructive curls confirmed.
- [ ] Step 5 ran when delegated, with `scope` / `base_ref` / `reviewer_model` / `skip_verifier: false` passed and `test-gap` not spawned.
- [ ] Single Japanese report: CONFIRMED → PLAUSIBLE, comment findings in their own subsection, runtime coverage stated, `テスト観点:` line present with one of the three states.
- [ ] Unless `--no-apply`: comment findings applied in Step 7 (functional directives untouched, exported doc comments rewritten not deleted), then `make go-fix` + `make go-lint`; no auto-commit.
- [ ] Unless `--no-comment` / no PR: confirmed once, then posted the code lenses' CONFIRMED + PLAUSIBLE as inline PR comments (off-diff → summary body); REFUTED excluded; `event: COMMENT`.
- [ ] Delegated test findings posted only when their anchor is inside a diff hunk (all 4 severities); off-diff ones kept local with the withheld count stated.
