---
name: portal-manifest-sync
description: >-
  Audit `docs/portal/manifest.yaml` against the real `README.md` / `README.ja.md` files on disk, treating the manifest as a curated reading list rather than a complete mirror of disk. It judges each uncurated README for manual-worthiness against `readme-review`'s criteria — which stay there as the single source of truth — and surfaces the stale, manual-worthy, borderline and not-yet-manual-grade sets. Edits only `docs/portal/manifest.yaml`; never touches `docs/portal/guides/**`. Use it when a README is added, removed or renamed, or when the portal's reading list looks out of date. Do NOT use it to judge one README's quality (`readme-review`) or to regenerate the portal output.
---

# Portal Manifest Sync

This skill audits `docs/portal/manifest.yaml` against the real `README.md` / `README.ja.md` files on disk. The manifest drives `scripts/portal/gen-portal-docs.ts`, which copies each `src` to its `dst` under `docs/portal/guides/`.

## Key Assumptions

### 1. manifest is a manual, not a dictionary

The portal is a **curated, narrative manual for humans to read**. It is intentionally NOT a complete dictionary of every README in the repo:

- An exhaustive enumeration of every sub-package's README would bury the conceptual flow under noise (a `di/server/extension/decoration/README.md`-level entry adds little to a reader trying to understand the architecture).
- The `dst` filenames inside `manifest.yaml` are curated for human navigability (e.g., `database/dml/README.md` → `sqlc-query-guide.md`, not a mechanical `database-dml.md`). Bulk additions break this curation discipline.

Therefore an on-disk README that is not registered is **not drift** — it is a candidate awaiting human curation judgment.

### 2. API surface is godoc's responsibility, not the portal's

Public Go API (functions, types, methods) is documented at the source level and surfaced via godoc — handled by a separate generator pipeline (out of scope for this skill). READMEs that primarily list API surface (heuristic: have a `## Public API` heading) are therefore **filtered out of candidates** by this skill. They are reported as a count only ("godoc 領域として除外"), never proposed as portal additions.

### 3. Candidate quality is judged by the criteria in `readme-review`

The evaluation criteria for "manual-worthy" are NOT duplicated in this skill. They live in `.claude/skills/readme-review/SKILL.md` as the single source of truth (the P1–P7 positive criteria, the N1–N4 negative criteria, and the four-class thresholds). This skill **applies** those criteria during Step 3; if the criteria evolve, only `readme-review` needs editing and this skill's behavior follows.

Result classes (per `readme-review` thresholds):

- **`manual-worthy`** — positive score ≥ 3 and no negative trigger → suitable for curation
- **`borderline`** — positive 1–2 and no negative trigger → README needs minor補強 before being portal-grade
- **`not-yet-manual-grade`** — stub / index-only (N2/N3 triggered) → README needs substantial work or stays out
- **`out-of-scope-for-portal`** — Pure API ref (N1) or Operational ref (N4) → belongs in godoc / CLI docs, never in portal

All four classes are reported but **none are auto-added**. Adding from any class remains the user's deliberate curation decision.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

Use this skill when:

- Files have been moved or removed and you want to clean up stale `src` entries that produce `gen-portal-docs.ts` warnings.
- You want to detect translation pair gaps (`README.md` without a `README.ja.md` sibling, or vice versa).
- Before merging a feature branch, to verify no portal regression was introduced.
- You want a snapshot of which on-disk READMEs are *not* currently exposed in the portal — to inform a manual curation decision, NOT to mass-add them.
- Periodically as a hygiene pass.

Do NOT use this skill for:

- Generating the portal output itself — use `make gen-portal-docs`.
- Mass-adding undocumented READMEs to the manifest — that defeats the manual's curation intent. Treat additions as a deliberate per-file editorial decision.
- Creating the missing translation file — chain into `canonicalize-doc` (this skill only flags the drift).
- Rewriting individual README contents — use `sync-readme`.
- A **deep-dive review** of a single README (strengths / gaps / improvement suggestions) — use `/readme-review <path>`.

## What This Skill Reads / Writes

**Reads (always)**:

- `docs/portal/manifest.yaml` — source of truth for which READMEs are exposed.
- `.claude/skills/readme-review/SKILL.md` — source of truth for the evaluation criteria; re-read on every run.
- All `README.md` / `README.ja.md` files in the repo, with these always excluded:
  - `docs/portal/guides/**` (generated copies of the originals)
  - `vendor/**`, `node_modules/**`
  - `.git/**`, `.claude/**` (skill / config files; not portal content)
  - Any path matching `.gitignore`
- `scripts/portal/gen-portal-docs.ts` — to confirm the dst convention is still `docs/portal/guides/<flat-name>.md`.

**Writes (only with confirmation)**:

- `docs/portal/manifest.yaml` — adds new entries, removes stale entries.

**Never touches**:

- `docs/portal/guides/**` (regenerated by `make gen-portal-docs` on every run)
- The source READMEs themselves
- Generated artifacts under `docs/`

## First Step: Confirm Scope

This skill **MUST call `AskUserQuestion` immediately after invocation** to confirm scope.

Question text (Japanese):

- 「manifest 同期のモードを選んでください」
- Options:
  - 「検出 + 適用（差分を提示して、承認後に manifest.yaml を更新）」
  - 「検出のみ（dry-run、書き込みなし）」
  - 「キャンセル」

Do not read any files (beyond what's needed for the question) before scope is confirmed.

## Step 1. Enumerate Sources

Build two sets:

```sh
# manifest_srcs: every `src:` value in docs/portal/manifest.yaml
# (parse the YAML and collect across all groups)

# disk_srcs: every README{,.ja}.md present on disk, excluding the exclusion list above
find . -name 'README*.md' -type f \
  -not -path './docs/portal/guides/*' \
  -not -path './vendor/*' \
  -not -path './node_modules/*' \
  -not -path './.git/*' \
  -not -path './.claude/*' \
  | sed 's|^\./||' | sort
```

Then compute:

- **`stale`** (actionable) = `manifest_srcs - disk_srcs` — manifest entries whose source files no longer exist
- **`pair_drift`** (preflight target) — for each disk README, check the sibling translation exists in both directions
- **`uncurated_raw`** = `disk_srcs - manifest_srcs` — on-disk READMEs not in the manifest. Will be passed through filtering (Step 3) before being shown.

## Step 2. Pair_drift Preflight

Pair_drift fragments the portal: an entry that gets registered while its sibling translation is still missing leads to broken navigation and creates immediate drift in subsequent runs of this skill. Resolve it before continuing.

If `pair_drift` is empty: skip to Step 3.

If `pair_drift` is non-empty: show the list and ask the user via `AskUserQuestion`:

- Question: 「pair_drift が N 件あります。本流に進む前に解消しますか？」
- Options:
  - 「canonicalize-doc を chain して順次解消」 — for each pair_drift item, invoke the `canonicalize-doc` skill. After all are processed, re-enumerate (Step 1) and continue.
  - 「未解消のまま続行（レポートに残す）」 — leave pair_drift as a reported item in Step 6; the rest of the workflow proceeds.
  - 「中断」 — stop the skill entirely; user will handle pair_drift externally.

If chained into `canonicalize-doc`, do NOT silently auto-translate: the chained skill itself confirms each file with the user.

## Step 3. Classify Uncurated via `readme-review` Criteria

This skill does NOT define its own evaluation criteria. It applies the criteria specified in `.claude/skills/readme-review/SKILL.md` (Step 2 "Evaluate Each Criterion" plus the classification thresholds). Before running this step, **re-read `readme-review`'s SKILL.md** to pick up any updates to the criteria.

For each entry in `uncurated_raw`:

1. Read the English README content (and the `*.ja.md` sibling for completeness check; sibling existence already established via Step 2 preflight).
2. Apply the P1–P7 positive criteria and N1–N4 negative criteria from `readme-review`.
3. Compute the verdict using `readme-review`'s thresholds, into one of the four result classes listed under *Key Assumptions* above.

4. For each file, record:
   - The verdict
   - A one-line rationale citing the dominant positive criteria met (for `manual-worthy` / `borderline`) or the dominant negative trigger (for the other two)
   - For `manual-worthy` and `borderline` only: the inferred group (Step 4) and hypothetical dst (Step 5), so the user can curate quickly

Do NOT skip this step or treat everything as `manual-worthy`. The four-class breakdown is the main user-facing value of this skill — getting it right matters more than getting through it fast.

Note: the per-file evaluation can be performed inline; you do not need to invoke the `readme-review` skill as a sub-skill 32 times (that would just generate 32 separate scorecards). Apply the criteria mentally and produce one aggregate report. The `readme-review` skill itself remains useful for **deep-dive single-file review** (longer explanation, improvement suggestions); for batch classification, this skill applies the criteria directly.

## Step 4. Infer Path → Category Mapping (no hardcoding) — for manual-worthy and borderline only

Derive the path → manifest-category mapping **from the existing manifest at runtime** so that, if the user *does* decide to add a `manual-worthy` or `borderline` entry manually later, the proposed category and dst can be shown alongside.

This information is informational only. Do NOT use it to drive bulk additions.

Algorithm:

1. For each manifest group, collect all `src` paths.
2. For each group, compute the longest common path prefix of its srcs (treat root README as prefix `""`).
3. Build a lookup table: `prefix → group`.
4. For each `manual-worthy` or `borderline` file, find the longest matching prefix and tag it with the inferred group (just for the report).
5. If no prefix matches, tag it `unmatched` (also just for the report).

Example (from the current manifest):

| Prefix | Group |
| --- | --- |
| `` (root) | `overview` |
| `.makefiles/` | `make_commands` |
| `env/` | `environment_variables` |
| `internal/controller/` | `controller` |
| `internal/domain/` | `domain` |
| `internal/usecase/` | `usecase` |
| `internal/infrastructure/` | `infrastructure` |
| `database/` | `database` |
| `internal/di/` | `di` |
| `internal/cli/` | `cli` |
| `internal/integration/` | `integration` |
| `pkg/` | `pkg` |
| `docker/` | `docker` |
| `scripts/` | `scripts` |
| `.github/workflows/` | `ci` |

(The actual table is built from `manifest.yaml` at run time, not from this list.)

## Step 5. Derive dst Path (no hardcoding) — for manual-worthy and borderline only

For each `manual-worthy` or `borderline` file, derive a hypothetical `dst` by inspecting how the assigned group already names its dsts. This is shown in the report so that a human curator can quickly see what the dst *would* be if they chose to add it — it is NOT an addition proposal.

Observed convention (verify per group at runtime):

- English: `docs/portal/guides/<flat-hyphenated-name>.md`
- Japanese: `docs/portal/guides/ja/<flat-hyphenated-name>.ja.md`

Where `<flat-hyphenated-name>` is the src path with the layer prefix stripped and `/` replaced by `-`. Examples from the existing manifest:

- `internal/controller/handler/README.md` → `docs/portal/guides/controller-handler.md`
- `internal/controller/handler/debug/README.md` → `docs/portal/guides/controller-handler-debug.md` (if it were added) <!-- skill-lint-ignore -->
- `internal/controller/README.md` → `docs/portal/guides/controller.md`

If a group's dsts deviate from this pattern, follow that group's actual convention. Some groups use bespoke names (e.g., `database/dml/README.md` → `sqlc-query-guide.md`); do not try to mechanically rename — show the convention literally.

## Step 6. Report

Format the report in three sections: actionable, candidates (for human curation), informational counts. Use the four-class taxonomy from `readme-review`. Japanese output.

```text
Portal Manifest Sync 結果

== 修正対象 ==

[削除候補 = stale] N 件
  - src: foo/bar/README.md  (ファイルが存在しない)
  - src: foo/bar/README.ja.md

[翻訳ペアの欠落 = pair_drift] N 件
  ※ Step 2 で「未解消のまま続行」を選んだ場合のみここに残ります
  internal/controller/handler/testkit/testauth/README.md
    → README.ja.md が存在しない（canonicalize-doc で生成推奨）

== キュレーション候補 ==

[manual-worthy] N 件（※自動追加はしません。curation 判断のうえ追加してください。
判定基準: readme-review の P1〜P7 で positive ≥ 3、negative トリガなし）

  infrastructure (M 件) — group=infrastructure, dst=docs/portal/guides/infrastructure-rdb-…
    internal/infrastructure/rdb/pgerror/README.md
      → P1 Role + P2 Why (Necessity) + P4 Mermaid + P7 prose 385 語

[borderline] N 件（※P1〜P7 のいずれか 1〜2 のみ満たす。少し補強すれば
manual-worthy 化できる候補。詳細は /readme-review で個別レビュー推奨）

  internal/di/server/extension/decoration/README.md
    → P5 Navigation のみ。Role/Design の追加で manual-worthy 化可能

[not-yet-manual-grade] N 件（※N2 Stub または N3 Index-only。
README 側の充実が先。必要なら sync-readme で内容拡充推奨）

  internal/controller/handler/testkit/README.md
    → N3 Index-only: H2 が Subpackages のみで narrative なし

== 情報のみ ==

[out-of-scope-for-portal] N 件（※N1 Pure API ref または N4 Operational ref。
godoc / CLI ドキュメント領域で portal 不要）

  pkg/datetime/README.md
    → N1: `## Public API` 支配的、他 H2 ≤2、prose <150 語
  internal/cli/dumpschema/README.md
    → N4: Command/Flags/Usage/Notes パターンの CLI ref

[未マッチ = unmatched group] N 件
  path/that/doesnt/match/any/existing/group/README.md
    既存グループのどれにも prefix が一致しない（新規グループ要否は人間判断）
```

If all classes are empty, report and stop:

```text
Portal Manifest Sync 結果
ドリフトなし、追加候補なし、除外なし。
```

## Step 7. Confirm Each Class of Change

Confirm only the **actionable** class (`stale`) via `AskUserQuestion`. For the four candidate classes (`manual-worthy` / `borderline` / `not-yet-manual-grade` / `out-of-scope-for-portal`), NO confirmation is asked — the skill never proposes additions on its own initiative.

### Removals (stale)

- Question: 「manifest に残っているが実体のない N 件を削除しますか？」
- Options: 「すべて削除」/「一部のみ削除」/「スキップ」

Stale removal is generally safe, but ask anyway — the file may be moved temporarily during a refactor and the user wants to keep the entry.

### Pair drift (only if Step 2 left it unresolved)

If pair_drift was deferred at Step 2, just keep the report entry. Do not re-prompt here.

### Optional curation flow (only if the user requests after seeing the report)

If — after seeing the report — the user explicitly asks to add specific manual-worthy entries, switch into a curation sub-flow:

1. Ask the user to name the specific files they want added (paths or copy-paste from the report).
2. For each, present the inferred group and proposed dst, and ask for confirmation per file (or batched in small chunks of ≤4 with `AskUserQuestion`).
3. Then apply the additions via Step 8.

Default behavior is NOT to enter this flow. The skill's responsibility ends at surfacing the candidates with quality classification.

## Step 8. Update manifest.yaml

Apply the approved changes by **editing the existing file in place** (not by re-serializing the whole YAML, which would lose comments and formatting).

For additions (only via the optional curation flow above):

1. Locate the target group's last entry in the file.
2. Insert the new entry pair (English entry + Japanese entry) preserving the file's existing `# English` / `# Japanese` comment style if present in that group.

For removals (from approved `stale` deletions):

1. Locate the specific `src:` / `dst:` lines.
2. Remove the two-line entry block (and any leading category-marker comment if the entry was the only one under that comment).

Always:

- Preserve indentation (2 spaces).
- Preserve trailing blank lines and section separators.
- Keep en/ja entries together within a group.

## Step 9. Verification

After writing, run:

```sh
make gen-portal-docs
```

If the command fails or warns about missing `src`s, surface the output and stop. The user must reconcile before this skill's contract is fulfilled.

If it succeeds, also run:

```sh
git diff docs/portal/manifest.yaml
```

Show the diff so the user can verify the manifest edits.

`docs/portal/guides/**` changes produced by `make gen-portal-docs` are expected; do NOT stage them in this skill (they are regenerated by CI and protected per `CLAUDE.md`).

## Step 10. Closing

- The skill does NOT commit. Commits are the user's call (chain into `/commit` if desired).
- The skill does NOT bulk-add candidates of any class. Adding is the user's editorial decision after seeing the report.
- If the user wants a deeper per-file analysis (full scorecard with improvement suggestions) for a specific README, recommend invoking `/readme-review <path>`.
- Print a short summary using the four-class taxonomy:
  - stale M 件削除（または スキップ）
  - pair_drift K 件: P 件 canonicalize-doc で解消 / Q 件は未解消（情報のみ）
  - manual-worthy A 件（追加要否は user 判断）
  - borderline B 件（補強候補、/readme-review で深堀推奨）
  - not-yet-manual-grade C 件（情報のみ、README 充実が先）
  - out-of-scope-for-portal D 件（godoc / CLI 領域）

## AI Modification Scope

Per the "Exception: Skill Execution" clause in `CLAUDE.md` / `AGENTS.md`, the AI modification scope is relaxed during this skill's run, scoped to:

- `docs/portal/manifest.yaml` — the only file this skill writes.

Remains protected (do not touch):

- `docs/portal/guides/**` (generated)
- `docs/portal/docs.json` (generated by `make gen-docs-json`)
- The source README files themselves
- Anything else outside the manifest

## Constraints

- ❌ Hardcode the category list — always derive from the existing manifest
- ❌ Skip Step 2 (pair_drift preflight); always offer the resolution choice when pair_drift is non-empty
- ❌ Skip Step 3 by classifying everything as `manual-worthy` without reading content
- ❌ Duplicate `readme-review`'s criteria here — always re-read its SKILL.md at runtime so updates flow automatically
- ❌ Use the deprecated "any `## Public API` → exclude" heuristic. Always use N1's full condition (Public API + H2 ≤3 + prose <150 + no Role/Design/Architecture). Pure-heading detection over-rejects hybrid READMEs (already-registered manifest entries proved this)
- ❌ Auto-create missing translation files directly (chain `canonicalize-doc` instead, which itself confirms per file)
- ❌ Touch `docs/portal/guides/**` directly
- ❌ Re-serialize the whole YAML (loses comments / order)
- ❌ Skip the scope-confirmation `AskUserQuestion`
- ❌ Apply changes without per-class confirmation
- ❌ **Bulk-add candidates from any class.** The manifest is a curated manual, not a dictionary
- ❌ Frame uncurated entries as "drift to fix" — they are candidates for human editorial judgment
- ✅ Japanese user-facing output
- ✅ Edit `manifest.yaml` in place, preserving formatting (only for stale removals or explicit curation requests)
- ✅ Run `make gen-portal-docs` after writing to verify
- ✅ Exclude generated / vendor / `.claude` / `.git` from disk enumeration
- ✅ Surface dst collisions explicitly

## Checklist

Before reporting completion, confirm:

- [ ] Scope was confirmed via `AskUserQuestion`
- [ ] manifest.yaml was parsed and `src` set was extracted
- [ ] Disk README enumeration excluded `docs/portal/guides/`, vendor, .git, .claude
- [ ] Step 2 pair_drift preflight was offered (or skipped because pair_drift was empty)
- [ ] `readme-review` SKILL.md was re-read for the current criteria
- [ ] Step 3 applied P1–P7 / N1–N4 + thresholds from `readme-review`, with per-file one-line rationale
- [ ] Path→category mapping was derived from the manifest (not hardcoded)
- [ ] dst convention was derived from existing entries (not hardcoded)
- [ ] Stale class was confirmed before deletion
- [ ] No candidate class was auto-added — all surfaced with explicit "curation is user's call" framing
- [ ] `manifest.yaml` was edited in place (no full re-serialization)
- [ ] `make gen-portal-docs` was run as verification (only when manifest was actually edited)
- [ ] `docs/portal/guides/**` was not modified by this skill
- [ ] No commits / pushes were performed
- [ ] Final report is in Japanese, with the four-class breakdown (manual-worthy / borderline / not-yet-manual-grade / out-of-scope-for-portal) plus stale and pair_drift
