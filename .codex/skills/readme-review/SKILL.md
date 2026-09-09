---
name: readme-review
description: >-
  Review a single README (canonical English) and judge whether it has "manual-worthy" characteristics for inclusion in the portal at `docs/portal/manifest.yaml`, producing a scorecard with strengths, gaps, improvement suggestions, and a classification (manual-worthy / borderline / not-yet-manual-grade / out-of-scope-for-portal). Use it when asking whether a README belongs in the portal or how to raise it to that bar. Read-only — it never edits the README or the manifest, and points at `portal-manifest-sync` for the actual curation. Do NOT use it to audit the whole manifest (`portal-manifest-sync`) or to fix README-to-disk structural drift (`sync-readme`).
---

# Readme Review

This skill evaluates a single README against the patterns that define "manual-worthy" content in this repo's portal, then produces a structured scorecard the user can act on (improve the README, add it to the manifest, or leave it out of the portal entirely).

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

Use this skill when:

- You wrote a new README and want to know whether it's polished enough for the portal manual.
- `portal-manifest-sync` surfaced a README under `borderline` or `not-yet-manual-grade` and you want a deeper, per-criterion read before deciding to add or improve.
- You suspect a README is documenting things that should be in godoc instead of the portal.
- You want a checklist of what's missing from a thin README before extending it.

Do NOT use this skill for:

- Mass-reviewing the entire repo — use `portal-manifest-sync` for the high-level four-class classification (which applies the same criteria defined here, batched). Use this skill for individual deep dives on borderline cases.
- Editing the README content — use `sync-readme` (drift fixes) or hand-edit.
- Adding to the manifest — chain into `portal-manifest-sync` (curation flow) after this skill's verdict.

## Source-of-Truth Role for Other Skills

The criteria defined below (Step 2: P1–P7 / N1–N4 / four-class thresholds) are referenced by `portal-manifest-sync` at runtime to perform batch classification. **Do not duplicate the criteria there.** If the criteria need to evolve (e.g., a new positive observation pattern emerges from manifest entries), edit this SKILL.md and the change propagates automatically.

This skill remains the canonical place to invoke for **deep-dive single-file analysis** — full scorecard with strengths, gaps, concrete improvement suggestions, and the recommended next action. `portal-manifest-sync` produces only a per-file one-line rationale to keep the batch report readable.

## How the Criteria Were Derived

The evaluation pattern is not hardcoded from theory — it was reverse-engineered by reading every entry currently in `docs/portal/manifest.yaml` (53 unique English srcs at initial skill creation, later expanded) and identifying what they have in common. Findings:

- median prose length ≈ 225 words; max 1358
- 27/53 use Mermaid diagrams
- 48/53 use tables
- 38/53 have at least one "concept" H2 heading
- top H2 frequencies: Notes (20), Directory Structure (12), Design Policy (11), Role (7), Public API (6), Rules (5), Architecture/Architectural Position (8), Test Strategy (4), Design Principles (4)

The 6 `Public API`-bearing entries that ARE in the manifest are all hybrid — they have Public API plus substantial conceptual sections (Role, Architecture, Design Principles, How It Works). That distinguishes them from pure API references, which belong to godoc.

### Keyword evolution log

The keyword sets in Step 2 are extended in response to false-negatives observed during runs of `portal-manifest-sync`. Add to a P-criterion's keyword list when:

- A genuinely manual-quality README uses a synonym that wasn't in the original list (e.g., `Conventions` is functionally a Rules section).
- The synonym appears in at least one currently-registered manifest entry, OR in a README that the user explicitly judges manual-worthy.

Do NOT expand keywords to admit clearly low-quality READMEs; the goal is to capture the spectrum of legitimate phrasings, not to inflate scores.

## First Step: Confirm Target

This skill **MUST call `ask the user explicitly` immediately after invocation** to confirm:

1. **Target README path** — the canonical English README to review. If the user supplied a path in skill arguments or the recent message, present it as the default candidate.
2. **Output verbosity** — concise scorecard (default) or full per-pattern breakdown.

If the user provided a `*.ja.md` path, ask whether to review the Japanese file directly (rare) or switch to the canonical sibling.

Do NOT read any files for evaluation until the target is confirmed.

## Step 1. Read the Target

Read the full README. Capture:

- All H2 headings (lines starting with two hash marks and a space)
- Presence and count of ` ```mermaid ` blocks
- Presence of tables (`|...|` lines)
- Prose word count (text excluding code blocks, tables, headings)
- Cross-reference to translation (`README.ja.md`) — its existence and sync convention compliance

## Step 2. Evaluate Each Criterion

Score each of the following criteria. Match by content, not just exact heading text — the skill must read sections and judge whether they substantively address the criterion.

### Positive criteria (each +1 when present and substantive)

| # | 観点 | シグナル |
| --- | --- | --- |
| P1 | **Role / Position** | H2 in `{Role, Position, Overview, Role in Onion Architecture, Architectural Position, Role in This Project}`, with prose explaining what this package/layer is and where it sits |
| P2 | **Design Intent / Why** | H2 in `{Design Policy, Design Principles, Design Intent, Why, Why ..., Rationale, Approach, Necessity, How It Works, Strategy, Trigger Strategy, Test Strategy, Application Policy}`, explaining the reasoning, not just listing rules |
| P3 | **Rules / Boundaries** | H2 in `{Rules, Do / Don't, Don'ts, Prohibited Practices, Forbidden, Constraints, Allowed dependencies, Disallowed, Implementation Rules, Conventions, Naming Convention, Naming, Policy}` — explicit prescriptive guidance |
| P4 | **Architecture diagram** | At least one ` ```mermaid ` block accompanied by 1+ sentence of explanatory text |
| P5 | **Navigation** | H2 in `{Directory Structure, Subdirectories, Subdirectory Roles, Subpackages, Package List, Workflow List, Command List, File List, Module List}` — for layers/groups that have substructure |
| P6 | **Notes / caveats** | H2 `Notes` (or similar) with non-trivial content describing pitfalls / operational caveats |
| P7 | **Substantive prose** | Prose word count ≥ 150 (excluding code/tables/headings) |

### Negative criteria (each −2 when triggered)

| # | 兆候 | 判定 |
| --- | --- | --- |
| N1 | **Pure API reference** | `## Public API` is present AND H2 count ≤ 3 AND prose < 150 words AND no Role/Design/Architecture section. → "godoc 領域" — out-of-scope-for-portal |
| N2 | **Stub** | H2 count ≤ 1 AND prose < 50 words |
| N3 | **Index-only** | Only H2 is `Subpackages` or `Subdirectories` and it just lists items without narrative |
| N4 | **Operational reference** | H2 set looks like `{Command, Flags, Usage, Notes}` or `{Commands, Flags, Usage, Notes}` — CLI usage reference, belongs in CLI docs or godoc-cli, not portal manual |

Apply N1–N4 conservatively. If the README has *any* substantial Design / Role / Architecture content, do not trigger N1 / N3 / N4 even if API/Subpackages/Command headings are present.

### Classification thresholds

- **manual-worthy**: positive score ≥ 3 AND no negative trigger
- **borderline**: positive score 1–2 AND no negative trigger
- **not-yet-manual-grade**: positive score 0 (or any positive but with N2/N3 triggered)
- **out-of-scope-for-portal**: N1 (godoc territory) or N4 (CLI reference) triggered

## Step 3. Output Scorecard

Japanese output. Sections:

```text
README Review: <path>

[判定] manual-worthy | borderline | not-yet-manual-grade | out-of-scope-for-portal

[強み] (positive criteria met)
  ✓ P1 Role: "Role in Onion Architecture" で層境界を明示（〜「the core of the business」）
  ✓ P4 Architecture diagram: Mermaid 図 2 件 + 解説 prose
  ✓ P6 Notes: 運用上の caveats 3 項目（"transaction boundaries are managed by..."）
  ✓ P7 散文 534 語

[ギャップ] (positive criteria absent)
  ✗ P2 Design Intent / Why セクションなし
  ✗ P3 Rules / Do-Don't の明文化なし

[アンチパターン] (negative triggers)
  なし
  ※（または該当する場合）
  ⚠ N1 Pure API reference: `## Public API` が支配的（他 H2 ≤2, prose 43 語）→ godoc 領域

[補強提案]
  - "Why this layer exists" を 1〜2 段落追加すると、新規開発者・AI エージェントが層を必要性で理解できる
  - "Forbidden / Don't" を箇条書きで明示すると、規約逸脱を自動チェックしやすくなる（arch-check スキルとも連動）

[portal 適性]
  manual-worthy → portal-manifest-sync の curation flow で追加候補
  （または）borderline → README 補強後に再 review 推奨
  （または）not-yet-manual-grade → 内容拡充が先、または portal 対象外でよい
  （または）out-of-scope-for-portal → godoc 領域、portal 不要
```

For verbose mode, additionally include the raw H2 list, prose word count, mermaid/table counts, and the detected H2 → criterion mapping.

## Step 4. Suggest Next Action

Print a one-line suggestion based on the verdict:

| Verdict | Suggestion |
| --- | --- |
| manual-worthy | "/portal-manifest-sync の curation flow でこの README を追加候補として指定するか、手動で manifest に追加してください" |
| borderline | "P2/P3 等の不足 section を補ってから再 review を推奨" |
| not-yet-manual-grade | "/sync-readme で内容を拡充するか、portal 対象外として扱ってください" |
| out-of-scope-for-portal | "godoc / CLI ドキュメント側で扱うべき内容です。portal manifest への追加は不要" |

## AI Modification Scope

This skill is strictly read-only.

- Reads: the confirmed target README and (optionally) its `*.ja.md` sibling
- Writes: nothing
- Does NOT edit, stage, commit, or push

If the user requests improvements based on the review, recommend running `sync-readme` (for structural drift) or hand-editing (for substantive prose additions). Do NOT auto-rewrite within this skill.

## Constraints

- ❌ Hardcode evaluation rules in a way that drifts from `docs/portal/manifest.yaml` reality. The criteria above were derived from the current manifest snapshot; if the manifest convention evolves, re-derive
- ❌ Treat absence of all positive criteria as "the README is bad" — some READMEs are deliberately concise references and the right verdict is `out-of-scope-for-portal`, not "fix it"
- ❌ Auto-add the README to the manifest
- ❌ Edit the README itself
- ❌ Skip the target-confirmation `ask the user explicitly`
- ✅ Japanese user-facing output
- ✅ Cite specific sections / quotes from the README to justify each criterion check
- ✅ Distinguish "needs more content" (not-yet-manual-grade) from "belongs elsewhere" (out-of-scope-for-portal)
- ✅ Be honest when a README is intentionally minimal and that's fine — not every package needs to be in the manual

## Checklist

Before reporting completion, confirm:

- [ ] Target README path was confirmed via `ask the user explicitly`
- [ ] Full README content was read
- [ ] All H2 headings were enumerated and mapped to criteria
- [ ] Prose word count was computed (excluding code/tables/headings)
- [ ] Mermaid / table presence was checked
- [ ] Each positive criterion (P1–P7) was evaluated with a Yes/No + evidence
- [ ] Each negative criterion (N1–N4) was checked
- [ ] Final classification matches the thresholds
- [ ] Output is in Japanese with concrete evidence per criterion
- [ ] Next-action suggestion is included
- [ ] No file was edited / staged / committed
