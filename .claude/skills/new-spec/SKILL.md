---
name: new-spec
description: >-
  Integrator skill that creates the 2-layer spec set for one feature — `docs/spec/domain/<pkgpath>.md` + `docs/spec/usecase/<pkgpath>.md` — by chaining `new-spec-domain` / `new-spec-usecase`, asking which layers to scaffold (default both) and the package path each spec is filed under. Layers whose target file already exists are skipped rather than aborting the chain, and it invents no business content. Use it when starting a new feature that needs both specs; use a per-layer skill directly for a single layer. Do NOT use it to implement the layers (`scaffold-endpoint` and the `scaffold-*` family) or to validate an existing spec (`verify-spec`).
---

# New Spec

Integrator for creating the 2-layer spec template set for one feature (lean A: one domain spec + one usecase spec).

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Starting a brand-new feature and want both spec templates created in one chained flow.
- A feature already has one of its two specs and you want to add the missing one.

Do NOT use this skill for:

- Creating a single layer template — invoke the matching per-layer skill directly:
  - `new-spec-domain` / `new-spec-usecase`
- Editing existing spec files — open in the editor.
- Generating Go code from spec — that's `scaffold-endpoint` (or per-layer `scaffold-<layer>`).
- Validating spec consistency — that's `verify-spec`.

## Why only 2 specs (lean A)

controller / infra layers are not spec-driven — they are derived at scaffold time from OpenAPI gen and sqlc gen, and the convention is enforced by `arch-check`. Full rationale: `.claude/scaffold-spec/lifecycle.md`.

## What This Skill Reads / Writes

This skill itself **does not read or write spec files directly**. All structural work is delegated to per-layer skills, each of which:

- Reads its matching `.claude/scaffold-spec/<layer>-spec.md` at runtime for the section list.
- Asks for layer-specific identity via its own `AskUserQuestion`.
- Writes one Markdown file under `docs/spec/domain/` or `docs/spec/usecase/`.

This skill is purely orchestration: it confirms the feature name, the layer selection and the per-layer package paths, then invokes per-layer skills in dependency order.

**A feature is a bundle, not a directory.** The spec tree is keyed by package path (`docs/spec/<layer>/<rest>.md` ⇔ `internal/<layer>/<rest>`), so the two specs of one feature sit in two different trees and need not share a path — `internal/usecase/user/search` is one feature with `internal/domain/user`. The feature name this skill collects labels the bundle and fills the glossary's Owner column; it never decides a file location.

## First Step: Confirm Feature and Layer Selection

This skill **MUST call `AskUserQuestion` immediately after invocation**. Start with one batched call carrying these two questions:

### Question 1: Feature name

- Free-text: 「feature 名（kebab-case）。例: `user-management`, `order-fulfillment`」
- Validate `^[a-z][a-z0-9-]*$`. Re-ask on invalid input.
- This is the bundle label — it names the feature in the report and in the glossary's Owner column. It is **not** a path.

### Question 2: Layers to scaffold

- Multi-select, 2 options:
  - 「domain」
  - 「usecase」
- Default: both. The user may deselect either (e.g., the domain spec already exists and only the usecase one is needed).

### Then: the package path per selected layer

A second batched `AskUserQuestion`, one free-text question per selected layer, because the paths are what the skip check needs and they are not derivable from the feature name:

- domain: 「domain パッケージパス（`internal/domain/` 以下）。例: `cart`, `product/category`」
- usecase: 「usecase パッケージパス（`internal/usecase/` 以下）。例: `address`, `product/ranking`, `user/search`」

Validate each with `^[a-z][a-z0-9]*(/[a-z][a-z0-9]*)*$` — Go package names carry no hyphens, so the kebab-case feature name is usually **not** a valid package path. Default each to the feature name with `-` replaced by `/` and let the user correct it.

After the answers come back:

1. Build the work plan: feature name + ordered list of (layer, package path) pairs to scaffold (dependency order: `domain` → `usecase`).
2. For each selected layer, check whether `docs/spec/<layer>/<pkgpath>.md` already exists. If it does, mark it as **skip** rather than failing the entire chain.
3. Show the plan in Japanese with skip markers, then confirm via `AskUserQuestion`:
   - Question: 「以下の順番で per-layer skill を chain します。進めますか？」
   - Options: 「進める」 / 「キャンセル」

If after applying skips the executable list is empty, report it and stop:

```text
全 layer の spec ファイルが既に存在します。実行対象がありません。
```

## Step 1. Chain Per-Layer Skills in Dependency Order

For each layer remaining in the plan (in order `domain` → `usecase`):

1. Invoke the matching skill: `new-spec-<layer>` via the `Skill` tool.
2. Pass the feature name **and that layer's package path** in the chain context so the child skill can skip its own path `AskUserQuestion` and only ask for layer-specific identity (aggregate name / interface name).
3. The child still asks the user to confirm and writes its own file.
4. If the child reports failure (e.g., user cancels the per-layer confirmation), stop the chain and surface the status; do NOT auto-rollback already-written layers.

Each per-layer skill is independent — they only share the feature name and the layer-execution order. Their package paths are separate answers, and they do not pass aggregate or interface names between each other.

## Step 2. Closing Report

After all selected layers have been processed (chain completed or stopped early), print a Japanese summary:

```text
new-spec 完了（feature: <feature>）。
  ✓ domain   : 作成済み (docs/spec/domain/<domainPkgPath>.md)
  ✓ usecase  : 作成済み (docs/spec/usecase/<usecasePkgPath>.md)

次のアクション:
  - editor で TODO を埋める
  - 2 spec 揃ったら verify-spec で format / 派生元 / cross-layer 参照を検証
  - 検証通過後に scaffold-endpoint で実装一括生成
    (controller / infra は OpenAPI / sqlc gen から自動導出される)
```

Adjust marks per actual result:

- ✓ = newly created
- `-` = skipped (file already existed)
- ✗ = failed / cancelled

Do NOT commit. Do NOT trigger any scaffold skill automatically.

## AI Modification Scope

This skill itself writes no files. All write scope is delegated to per-layer skills, each scoped to `docs/spec/<layer>/<pkgpath>.md`.

## Constraints

- ❌ Write spec files directly (always delegate to per-layer skills)
- ❌ Hardcode section lists (per-layer skills read `.claude/scaffold-spec/<layer>-spec.md` at runtime)
- ❌ Abort the entire chain because one layer's file already exists — mark as skip and continue
- ❌ Auto-rollback earlier created layers if a later layer fails
- ❌ Skip the feature-confirmation `AskUserQuestion`
- ❌ Derive a layer's package path from the feature name without asking (kebab-case is not a package path, and the two layers' paths differ)
- ❌ Run per-layer skills out of dependency order
- ❌ Offer controller / infra as spec options (lean A: those layers are derived, not spec-driven)
- ✅ Japanese user-facing output
- ✅ Multi-select layer choice (domain / usecase) with both default
- ✅ Chain in dependency order: `domain` → `usecase`
- ✅ Surface skip / fail status per layer in the final report

## Checklist

- [ ] Feature name + layer selection + per-layer package path confirmed via `AskUserQuestion`
- [ ] Existing layer files marked as skip (not failure)
- [ ] Per-layer skills invoked in dependency order
- [ ] Each per-layer skill ran its own identity `AskUserQuestion`
- [ ] Per-layer failure stopped the chain without auto-rollback
- [ ] Final Japanese summary uses ✓ / - / ✗ marks
- [ ] No direct file writes by this skill
- [ ] No commit / scaffold auto-trigger
