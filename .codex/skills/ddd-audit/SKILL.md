---
name: ddd-audit
description: >-
  Audit this repository's documented DDD interpretation against Eric Evans's original pattern language, and reconcile the layer-1 ledger at `.agents/ddd-audit/pattern-ledger.yaml` with its ADR and README corpus. Emits `差異なし`, `差異あり`, or `逸脱宣言あり`, and never decides whether a divergence from Evans is intentional. Use when checking whether a DDD pattern such as Aggregate, Value Object, Repository, Factory, Specification, Bounded Context, Anticorruption Layer, Ubiquitous Language, or Domain Event has been interpreted, after changing an ADR or domain README, when the DDD ledger may be stale, or when onboarding a reviewer without Evans context — 「DDD 原義と照らして」「Evans 的に正しいか」「この概念は解釈済みか」「台帳を更新して」. Do NOT use for Go architecture checks (`arch-check`), domain-type quality, README-to-code drift (`back-prop`), or feature-spec validation.
---

# DDD Audit

## Contract

| | |
| --- | --- |
| **Owns** | 層2（ADR / README）を層1（Evans 原義）に照らす判定と、パターン台帳の同期 |
| **Never** | 裁定（差異が意図か見落としかの決定）/ コードの監査 / 層3 の代替 |
| **Starts when** | 原義照合、パターン解釈の有無、台帳の陳腐化が問われたとき |
| **Stops when** | 差異が意図的な設計選択か否かの判断に達したとき（人間へ） |

Audit layer 2, the repository's documented interpretation of DDD, against layer 1, Evans's
pattern language recorded in the ledger. Do not audit layer 3 enforcement in code; CI and
`arch-check` own that deterministic work.

| Layer | Content | Source |
| --- | --- | --- |
| 1 | Evans's project-independent pattern language | `.agents/ddd-audit/pattern-ledger.yaml` |
| 2 | This repository's interpretation | ledger-defined ADR / README corpus |
| 3 | Code enforcement | linters and analyzers |

The result is an observation, not an order. This repository claims DDD alignment, not strict
Evans conformance, so whether a divergence is deliberate remains a human decision.

## Scope and sources

1. Confirm both scope and whether ledger updates are wanted before inspecting findings. Offer:
   all patterns, only `unexamined` / `examining` / `uninterpreted` patterns, core patterns
   (`scope: core`), or `quick` patterns related to changed corpus documents; and `update` or
   read-only reporting. When invoked by `arch-check` with preset `quick` scope, do not ask again.
2. Read `.agents/ddd-audit/pattern-ledger.yaml`. The ledger is the source of truth for both the
   pattern list and its `corpus` globs; never hardcode either in this skill.
3. For `quick`, resolve changed files and select patterns whose `interpreted_by` points to a changed
   file, plus every `unexamined` or `uninterpreted` pattern. An existing pull request's
   `baseRefName` is authoritative; without a pull request, use `make base-branch` to resolve the
   latest release line from origin's live state. Never use `gh repo view --json defaultBranchRef`
   as the fallback. If the
   base cannot be resolved, stop and report the failure in Japanese, distinguishing it from the
   normal Japanese clean exit when no patterns are selected.
4. Before delegation, deterministically compare the changed files with the ledger's corpus globs.
   If a corpus file changed but the ledger did not, report a ledger-staleness banner. This is useful
   information, not a gate, and must be a set comparison rather than an LLM judgement.

## Pattern-level audit

The fan-out unit is one Evans pattern, never one document. Answering whether Aggregate is
interpreted requires a sweep of the entire corpus: the interpretation may be in a README under a
different name from an ADR. Re-reading documents is the necessary cost of a distributed question.

For every selected pattern, use `.codex/agents/ddd-origin-auditor.toml` as the read-only role
contract. Delegate independent patterns when delegation is available; otherwise execute that role's
instructions inline, one pattern at a time, and disclose the fallback in the report. Give each
instance the pattern id, `full` or `quick` mode, and the resolved quick file list.

The auditor returns exactly one of:

- `差異なし`: the corpus interprets the pattern consistently with Evans.
- `差異あり`: the corpus diverges or does not interpret the pattern, without an explicit declaration.
- `逸脱宣言あり`: the corpus diverges and explicitly explains the alternative and its reason.

Require an Evans premise, `file:line` evidence, and proposed ledger data for every result. The
auditor is strictly read-only; it must not arbitrate, ask the user, or write the ledger, prose, or
source.

## Verify potential differences

Independently verify each `差異あり` result before showing it. Use `review-verifier` when available;
otherwise perform an explicit skeptical pass. Test the stated Evans premise, whether evidence was
missed because the corpus uses another vocabulary, and whether an explicit divergence declaration
exists elsewhere. Drop refuted findings; retain confirmed or plausible findings with that label.
`差異なし` and `逸脱宣言あり` need no second pass because their citations are directly checkable.

## Report and approved ledger writes

First return a Japanese, read-only report grouped by verdict, but split `差異あり` by the ledger's
`status`: `[差異あり・解釈あり]` for `interpreted`, and `[差異あり・未解釈]` for `uninterpreted`.
The former is a position that diverges from Evans while the latter is an undecided blank, so readers
need different next actions; the ledger header's `status` × `verdict` table is authoritative and
the report must preserve its verdict values. Use the total form `差異あり <n>（解釈あり <k> / 未解釈 <m>、うち
CONFIRMED <c>）`. For `未解釈`, evidence must state the searched range and what was absent rather than
inventing a `file:line` citation. Include the staleness banner, each Evans premise, evidence,
verification label where applicable, and any dropped finding. Use neutral wording: never say
`修正してください`, `対応必須`, or `違反`.

If updates were not selected, finish after that report. Otherwise, for each changed proposal:

1. Obtain explicit approval for that individual ledger entry and its supported options.
2. Show the YAML diff before writing.
3. On approval, update only `.agents/ddd-audit/pattern-ledger.yaml`, set `last_audited` to today,
   then verify it parses and its entry count is unchanged.

Never write ADRs, READMEs, implementation code, generated files, or `AGENTS.md`. A desired prose
change is a maintainer decision to surface as follow-up, not wording for this audit to author.

## Completion

Return the Japanese aggregate report, state whether delegation or inline fallback was used, and list
approved / declined ledger updates. Do not stage, commit, push, or write outside the approved ledger.
