---
name: research
description: >-
  Reduce an unresolved design, technology, or operational choice to an evidence-backed comparison a human can decide: options compared on axes fixed before the options are named, one recommendation with explicit reversal conditions, and a stop at the human decision gate. Use when nobody has selected an approach yet, someone asks for options or normal practice, or a prior choice needs its trade-offs reconsidered. It first looks for a structurally isomorphic repository precedent rather than manufacturing a design branch, and cites every external claim. Do NOT use to establish current repository truth (`repo-truth` first), execute a decided issue (`impl-issue`), file an outcome (`new-issue`), review a diff, adopt a choice, write an ADR, or implement anything.
argument-hint: '[question] [--stage=dissolve|full] [--sources=repo|external] [--axes=<csv>]'
---

# Research

Turn an open question into a comparison a human can decide from, or establish that the question has
already been dissolved by a standing decision or an isomorphic precedent.

A Japanese reference translation is available at `SKILL.ja.md` in this directory. It is maintained
from this canonical file and is not loaded as a skill.

## When to Use

Use this skill when:

- a design, technology, or operational choice remains open;
- someone asks for alternatives or for the usual approach; or
- a previous decision is explicitly being reconsidered and its trade-offs need to be compared again.

Do not use it to determine the repository's current state, execute an already-decided issue, file the
result, or review a change. Route those subjects to the Codex-side `repo-truth`, `impl-issue`,
`new-issue`, or the applicable `impl-review` / `test-review` / `comment-sweep` skill. Those
cross-skill references point to `.codex/skills/<name>/SKILL.md`; because the Codex copies may lag
another environment after an incomplete synchronization, inspect their current contracts before
relying on them.

## Contract

| | |
| --- | --- |
| **Owns** | 未決の比較、評価軸の確定、推奨とその反転条件、決めるべきことの提示 |
| **Never** | 採択 / ADR 承認 / 実装 / 案数合わせの選択肢捏造 / 出典・数値の捏造 |
| **Starts when** | 選択が開いていて、まだ誰も決めていないとき |
| **Stops when** | 現状が不明（`repo-truth` へ）、既に ADR / spec が決めている、外部情報を確認できない |

Stopping means report the verified result and the unresolved boundary. Do not silently bridge a gap
with an assumption and do not continue into adoption or implementation.

## Why This Skill Exists

Prevent four comparison failures that can look like careful work:

1. **Answer-shaped criteria.** A preferred answer exists first, so only criteria that favor it are
   recorded. Fix the axes before naming any option.
2. **Cost-shaped conclusions.** Cost is important scope information, but this repository's
   `AGENTS.md` says template quality and consistency outrank the work required to reach them. State
   the cost so a human can decline scope without changing direction; never let cost select the
   recommendation.
3. **Invented evidence.** Plausible benchmark values, versions, dates, and citations are not
   evidence. Mark anything unverified and lower the confidence of conclusions that depend on it.
4. **A fabricated branch.** If a standing decision or a structurally identical mechanism already
   settles placement, construction, and invocation, there is nothing to score.

## Arguments

| Argument | Behavior |
| --- | --- |
| `--stage=full` *(default)* | Continue through comparison and recommendation |
| `--stage=dissolve` | Perform only the standing-decision and isomorphic-precedent checks, then stop |
| `--sources=repo` | Use repository evidence only; omit external claims that cannot be verified |
| `--sources=external` *(default)* | Permit external research under the source rules below |
| `--axes=<csv>` | Use the caller's axes rather than selecting them in Step 2 |

Treat `--stage=dissolve` as a complete, low-cost result when the question is whether a choice is
actually open. Treat `--sources=repo` as an explicit evidence boundary, not permission to fill the
external gap from memory. Name the claims that boundary prevented you from making.

## Step 0 — Establish That the Choice Is Open

Confirm what currently exists, which rules constrain it, and whether `docs/adr/` or `docs/design/`
already contains a governing decision. If the repository state is unclear or disputed, stop and
direct the user to `repo-truth`; do not invoke that skill automatically.

If an ADR or spec already decides the matter, report the decision, assess whether its recorded
premises still hold, and stop. Only a human may reopen a settled architecture, domain, or policy
decision.

Search the decision indexes by the concern each record governs, not merely by words in the user's
question. Read `docs/adr/README.md` and `docs/design/README.md`, then select the relevant entries by
ownership. Missing a differently named governing record creates a false comparison rather than an
obvious error.

## Step 1 — Try to Dissolve the Question by Structure

Before calling the matter a design branch, look for a mechanism with the same structure elsewhere in
the repository. Enumerate the owning layer, its sibling packages, and then `pkg/`; inspect shapes,
not vocabulary. Read `docs/architecture.md` and the nearest owning `README.md` at runtime.

Keyword search is insufficient because an isomorphic precedent may use unrelated domain language.
For example, a page cursor can establish the shape needed for a time window without sharing the word
"window." Use the repository graph for structural discovery when it is available, then confirm each
candidate in source:

```bash
# Reverse traversal: callers and load-bearing relationships.
node .claude/scripts/graph-affected.ts <symbol> --depth 2

# Structural query. The default budget is too small for this repository.
GRAPHIFY="${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify/bin/graphify"
"$GRAPHIFY" query "<structural question>" --budget 8000
```

The recorded graph behavior is repository knowledge documented in the Claude-side
`.claude/README.md`; read that file as an environment-independent repository reference. A graph edge
is a lead, not proof of isomorphism. Check `Built from commit:` in
`graphify-out/GRAPH_REPORT.md` against `git rev-parse HEAD`; disclose a missing or stale graph because
it weakens an absence claim.

A found precedent is independently inspectable. A claim that no precedent exists needs its search
frontier: identify every layer and package family enumerated. When the structural sweep is partial,
label the comparison provisional instead of asserting the branch is real.

When the precedent settles the question, report this in Japanese and stop:

> 設計分岐なし。`<X>` は既存の `<Y>` と同型で、配置・構築・呼び出し位置はそちらの前例で確定する。

Under `--stage=dissolve`, stop after this step whether the question dissolved or remains genuinely
open.

## Step 2 — Freeze the Evaluation Axes Before Options

Choose and publish the criteria for this question before naming even one option. Once published, do
not alter them to favor an emerging answer. When `--axes=<csv>` is supplied, adopt those axes and
explain how each bears on the question.

The repository's useful starting set is:

- 業界的正しさ
- DDD 整合性
- アーキテクチャ純粋性
- template 先の純粋性

This is not a fixed scorecard. A build-gate question may have no DDD dimension; a vocabulary question
may turn almost entirely on it. Retain only the axes that decide this question and justify every
one.

Also record axes that were not raised and explain why. An axis may be absent because the question
does not turn on it, or because this run could not obtain the evidence needed to assess it.
`--sources=repo`, for example, excludes an industry-practice axis when that axis depends on external
evidence. Without this distinction, the reader cannot tell irrelevance from an evidence gap. The
shape of the comparison is itself a claim about what mattered.

Always consider the repository's actual product: the coherent snapshot received at `useTemplate`
time by a reader who has never seen the repository history and will never consult its Git log.

## Step 3 — Enumerate Only Genuine Options

Do not target a fixed number. Two alternatives are correct for a binary introduce/do-not-introduce
question. Five materially distinct approaches require five. Never add a straw option to reach three,
and never hide a real distinction merely to shorten the table; merge only near-duplicates.

Compare every option against the frozen axes and include:

| Field | Required content |
| --- | --- |
| 案 | A descriptive name, not a letter |
| 利点 / 欠点 | Observable consequences rather than adjectives |
| リスク | Failure modes, migration, operations, security, and lock-in |
| 既存構造との整合 | Owning layer, applicable `docs/rules.md` rule, and downstream effects |
| コスト | Files affected, who or what breaks, and required regeneration; never part of the verdict weight |

Measure checkable claims. Count hot-path queries instead of merely asserting that an option adds a
read. Use reverse graph traversal to estimate blast radius rather than inventing an approximate file
count. Put a cost that could not be measured under `未確認`, together with why it was not measured and
what the result would change. Do not leave a qualified estimate in the cost field: a hedged number is
still read and carried into the decision as a number.

## Step 4 — Recommend and State Reversal Conditions

Give one recommendation and tie it to the frozen axes and verified evidence. A neutral list returns
the analysis burden to the human.

Then state the facts that would reverse the recommendation. Express uncertainty as falsifiable
conditions, for example:

> この推奨は、`<前提>` が成り立つ限り。`<条件>` なら B が優位に転じる。

The recommendation is advisory. Do not adopt it.

## Step 5 — Return the Decision Packet

Write user-visible output in Japanese and lead with the recommendation. Use these headings and
meanings:

```markdown
## 問題
<何を決めようとしているか、1〜2 文>

## 前提
- <置いた仮定と、それが崩れたときの影響>

## 評価軸
- <軸> — <なぜこの問いでこの軸なのか>
- 立てなかった軸: <軸> — <なぜこの問いでは立てられない / 関係しないのか>

## 選択肢
### <案の名前>
- 利点 / 欠点 / リスク / 既存構造との整合 / コスト

## 推奨
<どれを、なぜ>

## 推奨が変わる条件
- <この事実が違えば結論が変わる>

## 未確認
- <測れなかったコスト / 確認できなかった主張> — <なぜ測れなかったか、測れば何が変わるか>

## 決めるべきこと
- <人間が決める事項> — 記録先: ADR / issue / spec のどれか

## 出典
- <発行主体> 「<資料名>」 <URL> （参照 <日付>）
```

When the human must choose among alternatives, present the genuine alternatives as a numbered list
in the ordinary conversation and wait for the reply. Do not assume a modal or a Claude-specific
question API. Do not interpret silence as approval.

For a dissolved question or a stopped partial run, return only the sections supported by the work,
state what remains unresearched, and name the required next human action. A precise partial answer is
better than a complete-looking one.

## Sources

Attach publisher, document title, URL, and access date to every external claim. Prefer primary and
official sources such as specifications, project documentation, and release notes. Follow the
runtime's browsing and citation requirements when external lookup is permitted.

If lookup fails or a claim cannot be confirmed, label it unverified and reduce confidence in any
conclusion that depends on it. Never fabricate a benchmark result, version, deprecation date,
quotation, or citation.

For repository evidence, cite the path and symbol, not a line number. Keep evidence distinct from
inference.

## Standalone by Design

End at `決めるべきこと`. Do not adopt an option, create or approve an ADR, file an issue, modify a
spec, or begin implementation. Do not automatically chain `repo-truth`, `new-issue`, or
`impl-issue`.

The human route after this skill is: approve an option, record it in the identified authority, and
only then use the appropriate issue or implementation workflow. When this skill asks for that
approval, use numbered alternatives in the response and wait.

## Checklist

- [ ] Resolve `--stage`, `--sources`, and any caller-supplied axes.
- [ ] Under `--sources=repo`, name the external claims intentionally omitted.
- [ ] Establish current state, or stop and identify `repo-truth` as the next step.
- [ ] Read `docs/adr/README.md` and `docs/design/README.md` by governed concern.
- [ ] Report and stop for a standing ADR or spec decision.
- [ ] Search enumerated layers for isomorphic mechanisms by shape, using the graph when current.
- [ ] State the enumeration frontier before claiming there is no precedent.
- [ ] Freeze and justify axes before naming any option; list deliberately omitted axes and why.
- [ ] Put costs and claims that could not be established under `未確認`, not as qualified inline
      estimates.
- [ ] Enumerate the question's genuine alternatives without targeting a count.
- [ ] Give every option consequences, risks, structural fit, and cost.
- [ ] Keep cost out of the recommendation weight.
- [ ] Provide a recommendation, its evidence, and conditions that reverse it.
- [ ] Cite external claims with publisher, title, URL, and access date; discount unverified claims.
- [ ] Name what the human must decide and where that decision belongs.
- [ ] Keep all user-visible output in Japanese.
- [ ] Stop without adoption, filing, documentation changes, or implementation.
