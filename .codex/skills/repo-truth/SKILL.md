---
name: repo-truth
description: >-
  Establish what is true in this repository right now from primary code and governing documents, keeping evidence separate from inference and publishing the exact search frontier behind every absence claim, so 未定義 stays distinct from 確認できず. Use when someone asks where behavior is implemented, which rule or canonical document governs it, why a design exists, what a repository term means, or whether a procedure or convention exists at all. Read-only: it reports conflicts and drift without resolving or editing them. Do NOT use for general programming questions with no repository-specific answer, known operational symptoms covered by `repo-ops`, undecided design choices needing `research`, change review, or issue filing.
argument-hint: '[question] [--depth=quick|full] [--kind=fact|rule|rationale|procedure|vocabulary|history]'
---

# Repo Truth

Answer what is true of this repository now from its own primary sources, and make the reasoning
auditable.

A Japanese reference translation is available at `SKILL.ja.md` in this directory. It is for human
reference and is not loaded as the skill.

## When to Use

Use this skill when someone asks:

- where a behavior is implemented or how a repository path behaves;
- which rule, convention, or canonical document governs a concern;
- why a design is the way it is;
- what a repository-specific term means; or
- whether a rule, procedure, term, or implementation exists at all.

Do NOT use it for a general programming or library question with no repository-specific answer, a
known operational failure documented by `repo-ops`, an undecided choice that requires `research`, a
diff review (`impl-review`, `test-review`, or `comment-sweep`), or filing a finding (`new-issue`).

## Contract

| | |
| --- | --- |
| **Owns** | リポジトリの現状に関する事実回答、事実と推論の分離、未定義 / 確認できず の判定 |
| **Never** | リポジトリ外の一般論を主根拠にした将来設計、未決事項の採択、変更、検出した drift の修復 |
| **Starts when** | 現状、規約、根拠、語義、存在有無を問われたとき |
| **Stops when** | 権威を持つ二つの出典が矛盾したときは矛盾を提示して停止する。所有索引を通読せず断定を求められたときは断定せず確認できずとする |

## Why This Exists

Do not answer repository questions from memory. A layer rule, target behavior, response status, or
package responsibility may have changed since it was last seen, and a confident approximation will
be acted on as fact.

Locating the governing source is a separate operation from reading search hits. This repository has
518 canonical Markdown files after exclusions, including 110 ADRs and 150 package READMEs, plus 52
`.mk` files. It also carries 495 `*.ja.md` mirrors and 144 generated copies that must not be treated
as authority. The canonical corpus is too large for a partial keyword sweep to establish absence.

More importantly, `AGENTS.md` says documents are named for the concern they own. The file governing
a question commonly contains none of the question's vocabulary in its name. Therefore grep cannot
establish that an answer does not exist: a missing hit is not a missing rule. Publish the explored
frontier so every absence claim is falsifiable.

Keep Evidence and Interpretation separate. This is not ceremonial uncertainty; it lets the reader
replace an inference without mistaking it for something the repository explicitly states.

## Arguments and Entry Check

Do not ask for confirmation before starting. Resolve behavior from these arguments:

| Argument | Effect |
| --- | --- |
| `--depth=quick` *(default)* | Skim the indexes that own the concern. `未定義` is unavailable; report an unlocated answer only as `確認できず` |
| `--depth=full` | Read the owning indexes in full and walk the applicable README chain; this earns the right to report `未定義` |
| `--kind=<kind>` | Accept the caller's classification and skip classification in Step 1 |

Escalate from `quick` to `full` when the answer depends on non-existence, and disclose that escalation.
The cost is deliberate: an authoritative absence requires reading the complete owning indexes.

Before searching, verify that this is the correct route:

| Intent | Route |
| --- | --- |
| A known operation failed or a gate broke | `repo-ops` |
| The user wants to perform an operation | `how-to` |
| Nobody has decided among design options | `research` |
| The user asks how the repository works, what governs it, or whether something exists | this skill |
| The wording supports more than one of these meanings | `question` |

If another route owns the request, say so in one sentence and name it. If ambiguity must be resolved
in an interactive session, present concise numbered choices in the normal conversation and wait for
the user's reply; do not refer to a Claude-specific modal or tool API.

The route names `how-to` and `research` come from the authoritative workflow contract but may not yet
exist in `.codex/skills/`. If a named route is unavailable, report that missing Codex handoff instead
of pretending to invoke it or absorbing its responsibility into this skill.

Graphify's broad trigger does not replace this routing. It is one discovery instrument used in Step
2. A graph result alone does not separate evidence from inference, publish a frontier, or distinguish
`未定義` from `確認できず`.

## Step 1 — Classify the Question

Classify before searching because the kind determines which corpus governs and what absence means.

| Kind | Primary answer source | Meaning of absence |
| --- | --- | --- |
| Fact | implementation along the request path | the path does not exist; do not substitute a plausible path |
| Rule | governing document | the rule may be undefined, but only after exhausting the owning index |
| Rationale | `docs/adr/`, `docs/design/` | the decision may never have been recorded |
| Procedure | `.makefiles/**`, `.lefthook.yaml`, `.github/workflows/`, `docs/maintenance/` | there may be no canonical procedure; never invent one |
| Vocabulary | `docs/spec/glossary.md`, `docs/spec/{domain,usecase}/` | the term may be unregistered or not business vocabulary |
| History | `git log`, merged pull requests | report the reachable history |

Do not let surface vocabulary pick the kind. For example, running integration tests is a Procedure
question, while why those tests require a DB slot is a Rationale question.

## Step 2 — Establish the Search Frontier and Locate the Source

Write down the intended frontier before drawing conclusions. Track indexes read in full, README
chains walked, globs searched, structural traversal performed, and every relevant sweep deliberately
left undone with its reason. Listing only covered ground makes an intentional exclusion
indistinguishable from an overlooked sweep.

### Search Index-First by Owned Concern

Select documents from indexes by the concern each entry owns, not by whether its title repeats a
word in the question:

| Index or chain | Coverage |
| --- | --- |
| `docs/adr/README.md` | recorded reasons and rejected alternatives across 110 ADRs |
| `docs/design/README.md` | subsystem and cross-cutting design |
| `.makefiles/README.md` | all make targets grouped by concern |
| README chain from the path to its layer root | package responsibilities, prohibitions, and design intent |
| `docs/spec/glossary.md`, `docs/spec/{domain,usecase}/` | business vocabulary and feature behavior |

At runtime, read §0, **Finding the authoritative source**, in
`.codex/skills/repo-ops/SKILL.md`. That section is the canonical answer-target-to-source lookup and
the canonical noise-excluding search recipe; do not duplicate it here. The Codex `repo-ops` copy may
lag the current Claude-authoritative version, including its `Contract`, the owning-skill column in
the index, the newest §21 material, and §99 numbering. State that possible staleness when relying on
it, and do not infer missing newer content from the Codex copy.

Use keyword search only after the concern-owning indexes, as a final net. Never read a `*.ja.md`.
A translation hit may locate the English sibling, but only the English canonical file may be read or
cited. Exclude generated trees such as `docs/portal/**`, `docs/godoc/**`, `docs/db-schema/**`,
`docs/openapi/**`, and `docs/coverage/**` from authority and from broad searches.

### Reserve the Two Absence Verdicts

| Verdict | Required frontier |
| --- | --- |
| **未定義** — no governing rule or procedure exists | every index owning the concern read in full and the applicable README chain walked |
| **確認できず** — this run could not establish it | any smaller frontier |

`確認できず` is always safe. Never promote a partial search, a keyword miss, or a quick run to
`未定義`. A false authoritative absence is worse than no answer because later work will build on it.

Apply source precedence from `AGENTS.md` when sources disagree:
`AGENTS.md` → `docs/rules.md` → `docs/architecture.md` → user instructions. For design intent and
implementation policy, use **README > Code > SKILL**. Report the disagreement rather than silently
using precedence to erase it.

### Use Graphify for Structural Reachability

Use the repository graph where structure answers better than text, especially for callers, impact
radius, and any scope claim such as "only X does this." The measured repository-specific guidance is
in the Claude-environment `.claude/README.md`; its measurements describe the repository and are valid
across agent environments.

```bash
# Resolve a symbol and reverse-traverse callers / affected nodes.
node .claude/scripts/graph-affected.ts <symbol> --depth 2

# Structural natural-language lookup; raise the budget to avoid default truncation.
GRAPHIFY="${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify/bin/graphify"
"$GRAPHIFY" query "<question>" --budget 8000
```

Prefer `graph-affected.ts` for exhaustive scope evidence. It resolves node candidates and reports
relationships with source locations. Do not use `god-nodes` as a proxy for importance; this
repository's one-to-one test mapping makes high-edge test scaffolding dominate that ranking.

Apply all of these guardrails:

- A graph node, edge, or generated summary is a route to evidence, never evidence itself. Open and
  cite the source file it identifies.
- Whenever Graphify is used, compare `Built from commit:` in
  `graphify-out/GRAPH_REPORT.md` with `git rev-parse HEAD` and report both freshness points.
- The graph cannot see uncommitted work. Because this skill is read-only, use an exhaustive direct
  search and report the graph as stale for that portion. Mention rebuilding with
  `make graphify-update` only as a possible next action; do not run it here.
- Graphify output may be absent because it is gitignored and machine-local. Fall back to indexes,
  direct source reading, search, and history; record the unavailable structural sweep in the
  frontier.
- A default query budget can truncate the relevant result. Raise it or explicitly account for the
  truncation.

## Step 3 — Read Primary Sources and Mark the Seam

Open every source you cite. A discovered path that was not opened is not evidence.

Maintain two explicit sets:

- **Evidence:** behavior expressed by code or a statement actually present in a governing source.
- **Interpretation:** a conclusion combining sources, reasoning from absence, or analogy with a
  sibling implementation.

Inference is useful, but label it as inference. Scope statements are absence claims: back them with
Graphify reverse traversal or another exhaustive search. Otherwise narrow the statement to the
frontier actually covered.

## Step 4 — Check Currency and Conflict

- Check recent history for deciding paths with `git log --oneline -10 -- <paths>` when currency
  affects the answer.
- When two authoritative sources conflict, report both sources and their freshness, then stop. Do
  not choose a policy on the user's behalf.
- Treat README-versus-code divergence as drift: the README governs and the code is the finding.
  Do not repair either under this skill. `back-prop` owns reconciliation.
- Treat a skill body as navigation, not authority over a README or governing document.

## Step 5 — Emit the Japanese Answer Contract

Always answer in Japanese with every section below. Keep `回答` to one to three sentences.

```markdown
## 回答
<結論を 1〜3 文で>

## 根拠
- <主張> — `<path>` の `<symbol / target / 節>`

## 推論
- <根拠から導いた内容。事実と異なる表現で>

## 矛盾 / 欠落
- <矛盾する出典と鮮度> / <未定義または確認できず> / <古い可能性>
- 探索範囲: <通読した索引 / README 連鎖 / 検索 glob / 構造探索 / 回さなかった掃引とその理由>

## 確度
High | Medium | Low — <根拠>

## 次にできること
<追加確認 / research / new-issue / ADR 化 / back-prop など。ここでは実行しない>
```

Cite paths plus stable symbols, targets, or section names. Do not cite line numbers, which become
stale after refactoring.

Judge confidence from sources:

| Level | Use when |
| --- | --- |
| High | multiple current primary sources agree and govern the question |
| Medium | one primary source, an implicit source, or unverified currency |
| Low | mostly inference, conflicting sources, or an unreachable deciding source |

When only part of the question is verified, answer that part and identify the rest as a gap.

## Gaps Are an Answer

An absent canonical procedure is a result, not an invitation to invent a plausible command. Report
it with the frontier that makes the absence falsifiable. For example:

> 正規手順は**未定義**。`.makefiles/README.md` と `docs/maintenance/` の所有索引を通読し、
> `.lefthook.yaml` と `.github/workflows/` を確認したが該当なし。近いのは `<target>` だが、
> `<difference>` のため目的が異なる。

If any owning index remains unread, use `確認できず` and name what was not covered. `repo-ops` is a
lookup of known symptoms and does not own either absence verdict; a symptom missing from its index
returns here.

## Standalone by Design

Do not invoke a follow-on skill. Report a possible next action—`research` for an undecided design,
`new-issue` to file a finding, `back-prop` for drift, or `repo-ops` for a known symptom—and let the
user decide whether to run it.

## Do / Do NOT

- Do route the request elsewhere when this is the wrong entry point.
- Do search concern-owning indexes before keyword search.
- Do publish the frontier, including deliberately omitted sweeps and their reasons, and distinguish
  `未定義` from `確認できず`.
- Do open every cited source and cite stable symbols, targets, or sections.
- Do separate Evidence from Interpretation.
- Do report conflicts with both sources and their freshness.
- Do account for Graphify in either direction: use it where structure dominates text, verify its
  discoveries in source, and state freshness; or deliberately skip it and record why in the
  frontier.
- Do answer in Japanese.
- Do NOT answer from memory about repository-decided behavior.
- Do NOT read or cite `*.ja.md` or generated documentation as authority.
- Do NOT treat code as authority over governing documents or skills as authority over READMEs.
- Do NOT resolve conflicting authority, repair drift, or edit anything.
- Do NOT report `未定義` from a partial or keyword-only search.
- Do NOT invent a rule, command, rationale, or path to fill a gap.
- Do NOT claim exhaustive scope from a partial search.
- Do NOT use line numbers in the answer.

## Checklist

- [ ] Entry check complete; symptoms route to `repo-ops`, operations to `how-to`, ambiguity to
      `question`.
- [ ] `--depth` resolved; any escalation to `full` disclosed.
- [ ] Question classified before searching.
- [ ] Concern-owning indexes inspected before keyword search; `repo-ops` §0 read at runtime with
      its possible Codex-side staleness noted.
- [ ] Frontier recorded: full indexes, README chain, globs, structural traversal, and every
      deliberately omitted sweep with its reason.
- [ ] No `*.ja.md` read and generated trees excluded.
- [ ] Every cited source opened; stable symbols and paths used instead of line numbers.
- [ ] Evidence and Interpretation separated; scope claims supported exhaustively.
- [ ] Currency checked when material; conflicts reported without resolution.
- [ ] Graphify accounted for in either direction: used where structure dominates text with results
      verified in source and freshness stated, or deliberately skipped with the fact and reason
      recorded in the frontier.
- [ ] Full Japanese output contract emitted with a source-based confidence reason.
- [ ] `未定義` reserved for exhausted owning indexes; otherwise `確認できず` names what is unread.
- [ ] Nothing edited or mutated, and no follow-on skill invoked.
