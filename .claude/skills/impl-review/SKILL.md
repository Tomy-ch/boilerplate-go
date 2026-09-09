---
name: impl-review
description: >-
  Local adversarial, low-bias review of THE CHANGE ITSELF, run on a model different from the implementer's, with lenses for correctness, security, architecture, runtime gap, DDD modeling and domain type design — plus a runtime (curl + o11y) stage that mocked tests cannot cover. Scope (changed files / branch-vs-base diff / specific paths) and reviewer model are confirmed in one question at the start; findings are reported in Japanese and posted to the branch's PR as inline comments by default (`--no-comment` to skip). Read-only on source — every lens reports and the user fixes. Use before commit / PR to get an independent second opinion the implementer's own model would not surface. Its subject is the implementation and nothing else: it carries no test lens and no comment lens and chains no other skill — `/test-review` and `/comment-sweep` are peers asked for separately under the Review Phase Protocol in `AGENTS.md`. Do NOT use it to review tests (`test-review`), comments (`comment-sweep`), or to run the mechanical layer-rule audit (`arch-check`).
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
- Applying fixes — this skill is read-only on source; it reports, the user fixes.
- Auditing the tests (`/test-review`) or the comments (`/comment-sweep`) — peers, not sub-steps.

## Contract

| | |
| --- | --- |
| **Owns** | 変更そのもの（architecture / ddd-modeling / security / correctness / runtime-gap / type-design） |
| **Never** | テスト観点（`test-review`）/ コメント観点（`comment-sweep`）/ 他スキルの呼び出し |
| **Starts when** | レビュー可能な差分とその意図が存在するとき |
| **Stops when** | tier 1 の finding が人間の設計判断を要するとき |

## Core Idea — reviewer ≠ implementer

Bias reduction is the design constraint, not a nicety. Reviewers therefore run as **subagents on a different model than whoever wrote the code**:

- The reviewer agents (`adversarial-reviewer`, `review-verifier`) default to **`sonnet`** in their frontmatter, which differs from the usual Opus implementer.
- **The reviewer model is chosen by the user in Step 0.** The options are `fable` (Fable 5) / `sonnet` / `opus` / `haiku`, plus an *auto* default that resolves to a model ≠ the session's implementer. Pass the chosen model to every reviewer subagent via the `Agent` tool's `model` parameter (it takes precedence over the agent file's `sonnet` default) — e.g. `opus` for depth, `haiku` for a cheap divergent pass, `fable` for a fresh independent perspective.
- **The orchestrator MUST guarantee reviewer ≠ implementer.** If the user selects the same model as the session's implementer, warn that it undermines the different-model bias reduction and confirm before proceeding. Never silently let reviewer and implementer be the same model.
- Reviewer subagents are **read-only** (their agent files grant no Edit/Write) — they only return findings, and this skill never mutates source at all. What to change is the user's call, made from the report.

**This skill audits the change and nothing else.** It has no test lens and no comment lens, and it
invokes no other skill. Those are `/test-review`'s and `/comment-sweep`'s subjects, each asked for and
run in its own right beside this one, per the Review Phase Protocol in `AGENTS.md`. A review skill
that offers to run the next one makes the subjects stop being independently answerable and lets a
drift in one skill's question silently drop the other two from every flow that went through it.

## Precedence — findings are ranked, not just collected

Reviewers disagree, overlap, and report the same fact in two vocabularies. Without a ranking the
report is a flat list in which a comment nit outranks a wrong aggregate boundary because its finder
called it "high". The tiers in the Step 2 table are that ranking:

| Tier | Lenses | What it decides |
| --- | --- | --- |
| 1 | `architecture`, `ddd-modeling` | what the code should *be* |
| 2 | `security`, `correctness` | whether what it is, works |
| 3 | `runtime-gap`, type design | whether it holds up in the real system and in its types |

**A change at a higher tier propagates downward; a lower tier does not, as a rule, act on a higher
one.** Rewriting an aggregate boundary invalidates the behavior verified against it; a naming nit
never justifies changing a boundary. Four consequences follow, and each of them is a rule, not a
suggestion:

1. **Order the report by tier, then by severity within a tier** — never by severity alone. A tier-3
   `high` sits below an architecture `medium`, because the architecture finding may delete the code
   the lower one is about.
2. **Mark a lower-tier finding 保留 while a higher-tier finding it depends on is unresolved.** Report
   it, say what it is waiting on, and do not present it as actionable. Re-check it after the
   higher-tier decision lands; it often disappears.
3. **When two tiers report the same fact, keep the higher tier's framing and fold the lower one in as
   corroboration** — one finding, not two. Two entries for one fact reads as two problems and
   double-counts the change's apparent risk.
4. **Agreement among lenses at the same tier raises confidence; agreement from a lower tier does
   not raise a higher finding's severity.** Two tier-2 lenses independently reaching the same defect
   is strong evidence — say so. A tier-3 lens agreeing with a tier-1 finding adds nothing to its
   severity, though it may be cited as support.

**The exception is criticality, and it is yours to notice, not to resolve.** A lower-tier finding
can be the more urgent one — an exploitable hole surfaced by the `runtime-gap` lens does not wait for
an architecture debate. When a lower-tier finding looks critical enough to outrank the tier above it,
**do not silently reorder: present both and ask the user.** The ranking exists so that ordinary
disagreements resolve without a human; a finding that breaks the ranking is exactly the case a human
should see.

## Step 0 — Confirm Scope

Call `AskUserQuestion` immediately. Default-detect scope by checking branch vs base; if there are unmerged commits, default to "changed files", otherwise "whole working tree / specific paths".

Resolve the base like this — the review has to cover the same diff the pull request shows, so an
existing PR's `baseRefName` wins; with no PR, `make base-branch` resolves the latest release line
(this repo's base is always a `release/*` branch) from `origin`'s live state. Do not fall back to
`gh repo view --json defaultBranchRef`: the GitHub default branch keeps answering with an earlier
release line, which silently widens the diff to a generation of changes nobody asked to review.

```sh
BASE=$(gh pr view --json baseRefName -q '.baseRefName' 2>/dev/null || make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }
git diff --name-only "origin/${BASE}...HEAD"
```

```text
質問: どの範囲をレビューしますか？
選択肢:
  - 変更ファイルのみ（ベースブランチとの diff）  ← 未マージのコミットがある場合の既定
  - 作業ツリーの未コミット変更（git status の差分）
  - 特定のパス/ファイルを指定
  - キャンセル
```

### Reviewer model selection

In the same `AskUserQuestion` call (a second question alongside scope), ask which model the
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
model is passed to every `adversarial-reviewer` / `review-verifier` `Agent` call via the
`model` parameter in Step 2 and Step 3.

**Two questions, and no more.** There is no test question and no comment question here. Those
subjects belong to `/test-review` and `/comment-sweep`, which the user asks for separately; folding
them in would put a decision about one subject inside a run started for another, and would make this
skill the single point through which the other two are remembered.

### Flags

- `--no-comment` — suppress Step 6 (do not post to the PR); produce the local report only. **Default is opt-out**: when an open PR exists for the current branch, Step 6 posts the surviving findings as inline review comments unless this flag is given.

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
- Note whether the diff touches **domain types** (`internal/domain/**/*.go`) — this decides whether the type-design lens runs.

## Step 2 — Fan-out Finders (different model, concurrent)

Spawn all finders concurrently (issue every `Agent` call in a single message). Apply the model rule from Core Idea — pass the Step 0 user-selected reviewer model to every `Agent` call via the `model` parameter (omit only when *auto* already resolves to the agent-file default). Two agent types:

- The four **code lenses** run `adversarial-reviewer` — one per lens, `agentType: "adversarial-reviewer"`, `label` like `find:security`.
- The **DDD modeling dimension** runs the dedicated `ddd-modeling-reviewer` — `agentType: "ddd-modeling-reviewer"`, `label: "find:ddd"` — when the diff touches `internal/domain/**` or `internal/usecase/**`. It asks whether the change models the domain well by this repository's own written interpretation (aggregate boundary vs transaction boundary, where a rule belongs, cross-aggregate reference discipline, ubiquitous language, Factory / Repository semantics). It is a **tier 1** lens: its findings decide what the code should be, so they are settled before the lower tiers act. Do NOT point it at Evans directly — that is `ddd-origin-auditor`'s subject, and its subject is the repo's documents rather than code.
- The **type-design dimension** runs the dedicated `type-design-reviewer` — `agentType: "type-design-reviewer"`, `label: "find:type-design"` — ONLY when the diff touches domain types (`internal/domain/**/*.go`). It scores each type on the four-axis rubric (Encapsulation / Invariant Expression / Invariant Usefulness / Invariant Enforcement); its findings are suggestion-level (not auto-fixed).

| Finder | Tier | Agent | Run when |
| --- | --- | --- | --- |
| `architecture` | 1 | adversarial-reviewer | always |
| `ddd-modeling` | 1 | **ddd-modeling-reviewer** | when the diff touches `internal/domain/**` or `internal/usecase/**` |
| `security` | 2 | adversarial-reviewer | always (especially when a handler / auth / DTO / `openapi/**` is touched) |
| `correctness` | 2 | adversarial-reviewer | always |
| `runtime-gap` | 3 | adversarial-reviewer | when a controller / DI / `openapi/**` / `database/**` is touched |
| type design | 3 | **type-design-reviewer** | when the diff touches domain types (`internal/domain/**/*.go`) |

Each `adversarial-reviewer` prompt MUST include: the lens name + its definition, the base ref + changed-file list + the diff, and pointers to `CLAUDE.md` / the relevant `README.md` / OpenAPI spec / migrations.

**No lens here audits the tests or the comments.** A finding that the change is untested belongs to
`/test-review`, and one about a comment's content belongs to `/comment-sweep`. If a lens surfaces
either in passing, say so in the 補足 section as an observation and name the skill that owns it —
do not grow a lens to cover it, which is how this skill acquired the two it just shed.

## Step 3 — Adversarial Verify

Collect all findings and **dedup** by (file, line, claim) — and when two lenses report the same fact,
apply Precedence rule 4: keep the higher tier's framing, fold the lower one in as corroboration, and
carry ONE finding forward. Textual dedup alone does not catch this: the same defect arrives worded as
an architecture violation and as a type-design suggestion, and shipping both double-counts it. For each surviving finding, spawn one `review-verifier` subagent (concurrently), handing it the single finding + the base ref. Use `agentType: "review-verifier"`, `label` like `verify:<file>`, and the Step 0 user-selected reviewer `model` (same reviewer ≠ implementer rule).

- Keep **CONFIRMED** and **PLAUSIBLE** findings. Drop **REFUTED** (but keep a count for the report).
- For a critical/high finding where a single verdict feels shaky, spawn 2–3 verifiers and go by majority — diversity beats one opinion on the findings that matter.

## Step 4 — Runtime Verification (curl + o11y) — endpoints only

Run this **only if Step 1 found a touched endpoint**, and run it from the **orchestrator (main session)**, not a subagent — it needs interactive bash, real DB/state, log reading, and possibly user confirmation. Follow `scaffold-endpoint` Phase 7:

1. `make go-test` (mocked) does NOT build the real Fx graph, run auth/OpenAPI middleware, or touch the DB — so this stage exists to catch what Step 2's `runtime-gap` lens only *predicts*.
2. Pick/seed a target row in a known state. For credential/state-sensitive checks, create a row whose plaintext/state you control.
3. `curl` the touched endpoint(s) (local auth: `Authorization: Bearer debug:<subject>`) and assert: happy path; key error paths (404 / 400 / 422); and — **if the operation declares `security:`** — no-token ⇒ 401 (prove it is actually protected). For IDOR-shaped findings, curl as a *different* subject and assert it cannot reach another subject's resource.
4. **Shared-schema impact:** if a shared `components/*` was edited (Step 1), curl **every** consumer endpoint, not just the changed one — `grep` the spec for `$ref`s and exercise each.
5. Read the o11y logs once for a single request: confirm the trace spans controller → usecase → infra and the emitted SQL is what you expect. Later re-checks can rely on o11y instead of re-curling.
6. **Destructive guard:** if a curl mutates data and the only restore path is `make db-init` (or similar), confirm with the user before running it (per `CLAUDE.md`). Clean up rows you created.

Fold any runtime-confirmed defect into the report as CONFIRMED with the curl/o11y evidence.

## Step 5 — Synthesize Report (Japanese)

Produce one Japanese report:

```text
## ローカルレビュー結果（reviewer: <model> / implementer: <model>）

スコープ: <base>...HEAD（<N> files） / lens: <実際に走らせた lens のみを列挙>
ランタイム検証: 実施（curl/o11y）/ 対象外（エンドポイント変更なし）
未監査の観点: テスト（/test-review）・コメント（/comment-sweep）は本スキルの対象外

### CONFIRMED（要対応）
- [重大度] タイトル — path:行
  - 問題 / 根拠 / 修正案
  - 検証: verifier 判定（+ 該当すれば curl/o11y 結果）

### PLAUSIBLE（要確認・判断保留）
- ...

### 補足
- REFUTED: <n> 件（finder が挙げたが verifier が否定）
- ランタイム検証でカバーした経路 / スキップした経路
- 他スキルが所管する観点として気づいた点（あれば。所管スキル名を添える）
```

The `lens:` line lists only the lenses that actually ran.

The **`未監査の観点:` line is mandatory**, and it is not boilerplate: this skill audits one of the
three review subjects, and a report that says nothing about the other two reads as a full review to
anyone who did not run them. State plainly that the tests and the comments were not looked at here,
so the omission is visible rather than inferred from a `lens:` list that never mentioned them. Do not
soften it into a recommendation — whether to run the other two is the user's call under the Review
Phase Protocol, and this line only records what this run did not cover.

Order by **tier first, then severity within the tier**, CONFIRMED before PLAUSIBLE (Precedence rule 1).
Mark every finding that is waiting on a higher-tier decision as `保留` and name what it waits on
(rule 2). Always state what runtime checks ran and what was skipped — silent omission reads as "covered everything" when it was not.

## Step 6 — Post Findings as Inline PR Comments (default; opt out with `--no-comment`)

By default, post the surviving **CONFIRMED + PLAUSIBLE** findings to the branch's PR as **inline review comments** — one per finding, anchored to its `path:line`, instead of a single wall-of-text comment. **Never post REFUTED.** The Step 5 local report is still produced regardless; this step is additive.

Only this skill's own findings are posted. `/test-review` and `/comment-sweep` produce their own output for the user to act on, and nothing here reaches into them — posting another skill's findings under this skill's review would make one subject's audit look like it happened inside another's.

Skip this step entirely when:

- invoked with `--no-comment`, OR
- no open PR exists for the current branch (`gh pr view` returns nothing) — keep the local report only and optionally offer to open a PR.

Posting to GitHub is an outward-facing action, so confirm **once** before posting — show the count and the target PR (`AskUserQuestion`: 「<N> 件の指摘を PR #<番号> にインラインコメントとして投稿しますか？」/「投稿する」「投稿しない（ローカルレポートのみ）」).

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

   `payload.json`: <!-- skill-lint-ignore -->

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
- ✅ Run finders concurrently (one message, multiple `Agent` calls): the code lenses via `adversarial-reviewer`, plus `ddd-modeling-reviewer` / `type-design-reviewer` when their trigger applies.
- ✅ Independently verify every finding before reporting; drop REFUTED.
- ✅ Run the runtime stage for touched endpoints; widen to all consumers on a shared-schema edit.
- ✅ State on the `未監査の観点:` line of every report that the tests and the comment stock were not audited here.
- ✅ Confirm with the user before any destructive curl whose only restore path is `make db-init`.
- ✅ By default, post the CONFIRMED + PLAUSIBLE findings to the branch's PR as inline review comments (Step 6); suppress with `--no-comment` or when no open PR exists.
- ✅ Confirm once before posting to the PR (outward action); anchor each comment to its `path:line`, fold off-diff findings into the review summary.
- ❌ Post REFUTED findings, or use `REQUEST_CHANGES` / `APPROVE` — the posted review is advisory `COMMENT` only.
- ❌ Mutate source at all — every lens reports, the user fixes.
- ❌ Grow a lens that audits the tests or the comments, or invoke `/test-review` or `/comment-sweep` from here. They are peers under the Review Phase Protocol; surface such an observation in 補足 and name the skill that owns it.
- ❌ Order the report by severity alone, report one fact as two findings from two tiers, let a lower-tier lens raise a higher finding's severity, or silently reorder the tiers when a lower finding looks critical — present both and ask.
- ❌ Let a reviewer run on the same model as the implementer.
- ❌ Report speculative style nits as findings, or pad the list to look thorough.

## Checklist

- [ ] Scope confirmed via `AskUserQuestion`; base ref resolved.
- [ ] Reviewer model selected in Step 0 and verified ≠ implementer model (warn + confirm if same).
- [ ] Finders fanned out concurrently: the code lenses (`adversarial-reviewer`) plus `ddd-modeling-reviewer` / `type-design-reviewer` where triggered — no test lens, no comment lens.
- [ ] Duplicate facts folded into the higher tier; report ordered by tier then severity; lower-tier findings waiting on a higher one marked `保留`.
- [ ] Every finding independently verified; REFUTED dropped (count kept).
- [ ] Runtime curl + o11y done for touched endpoints (shared-schema → all consumers); destructive curls confirmed.
- [ ] No other skill invoked from this run.
- [ ] Single Japanese report: CONFIRMED → PLAUSIBLE, runtime coverage stated, `未監査の観点:` line present.
- [ ] Unless `--no-comment` / no PR: confirmed once, then posted CONFIRMED + PLAUSIBLE as inline PR comments (off-diff → summary body); REFUTED excluded; `event: COMMENT`.
