---
name: new-spec-domain
description: >-
  Create the domain layer spec template at `docs/spec/domain/<pkgpath>.md`, where `<pkgpath>` is the Go package path under `internal/domain/` (`cart`, `product/category`) — the spec tree mirrors the package tree. Asks for the package path + aggregate name (PascalCase), writes a Markdown file with YAML placeholders and TODO markers for the user to fill in, and registers the aggregate in `docs/spec/glossary.md`, reporting a homonym rather than resolving it. NEVER overwrites an existing file, and does not invent business content. Use it when starting a new aggregate. Do NOT use it to scaffold both spec layers at once (`new-spec`) or to implement the layer (`scaffold-domain`).
---

# New Spec — Domain

Create the domain layer spec template for one feature.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Starting a new feature and need the domain layer spec template.
- Adding the domain spec for a feature whose usecase spec is already in place.

Do NOT use this skill for:

- Editing an existing domain spec — open in the editor directly.
- Generating Go code from spec — that's `scaffold-domain`.
- Validating spec consistency — that's `verify-spec`.
- Creating both layer specs (domain + usecase) in one go — use the integrator `new-spec`. lean A only requires these two specs; controller / infra are derived from OpenAPI gen + sqlc gen, no spec file.

## What This Skill Reads / Writes

**Reads (always)**:

- `.codex/scaffold-spec/domain-spec.md` — canonical section list for the domain layer.
- `docs/spec/domain/<pkgpath>.md` — checks whether the spec already exists.
- `docs/spec/glossary.md` — the business-vocabulary spec, to check the aggregate name against.

**Writes (with confirmation)**:

- `docs/spec/domain/<pkgpath>.md` — template file.
- `docs/spec/glossary.md` — one row for the aggregate, only when the user approves it.

**Never touches**:

- An existing domain spec at that path (aborts if found).
- Any other layer's spec file.
- Any glossary row other than the one being added.

## First Step: Confirm Identity

Spec identity is the **package path**: `docs/spec/domain/<rest>.md` corresponds to `internal/domain/<rest>`, and the Entity YAML's `package:` declares the same path. The package path is therefore what this skill must collect — it decides where the file goes.

This skill **MUST call `ask the user explicitly` immediately after invocation** (unless invoked from the `new-spec` integrator with the package path in context):

1. **Domain package path** — free-text, the path under `internal/domain/` (e.g., `cart`, `product/category`). Validate `^[a-z][a-z0-9]*(/[a-z][a-z0-9]*)*$` — Go package names carry no hyphens, so `product-category` is not a package path; the nested form `product/category` is.
2. **Aggregate name** — free-text, PascalCase (e.g., `User`, `Order`). Becomes the Go struct name.

If `docs/spec/domain/<pkgpath>.md` exists → abort. Otherwise create its parent directory if missing.

## Step 1. Read Section Definitions

Read `.codex/scaffold-spec/domain-spec.md` and extract the canonical section list. NEVER hardcode — this file is the source of truth.

Current section list (mirror dynamically):

1. Overview
2. Entity
3. Cross-field Invariants
4. Behavior Methods
5. Value Objects
6. Repository Methods

## Step 2. Generate the Template

Assemble Markdown with H1 title `<Aggregate Display> — Domain Spec`, then per section:

- Overview: single `TODO:` line
- Entity: YAML code block with `package` / `struct` / `fields` (1 placeholder field). `package:` MUST be `internal/domain/<pkgpath>` — the same path the file sits at, which `verify-spec` asserts
- Cross-field Invariants: bullet list with `TODO:` line
- Behavior Methods: YAML code block with placeholder method
- Value Objects: YAML code block with note "利用しない場合は節ごと削除"
- Repository Methods: YAML code block with placeholder method

Use the example output shown in `.codex/scaffold-spec/domain-spec.md` as the literal template format.

## Step 3. Confirm and Write

Display proposed path + first ~20 lines of template, then ask:

- 「以下の内容で `docs/spec/domain/<pkgpath>.md` を作成しますか？」
- Options: 「作成する」 / 「キャンセル」

If approved, `mkdir -p` the file's parent directory (`docs/spec/domain/<pkgpath>` minus the last segment) and `Write` the file.

## Step 4. Register the aggregate in the glossary

**Only the aggregate name is a term at this point.** This skill writes a template of TODOs; the
fields, value objects and behaviours are not decided yet, so there is nothing else to register. One
row, or none.

Read `docs/spec/glossary.md` and compare the aggregate name against the existing rows.

- **Already present with a different owner** — a homonym. Report both rows side by side and stop:
  the same word owned by two features is the finding this page exists to surface, and which name
  wins is a decision about how the business will talk. **Never resolve it here.**
- **Already present with the same owner** — nothing to do; say so.
- **Not present** — propose one row, announce plainly that its definition is a draft, then ask the
  user explicitly whether to add it or skip it for now.

The Owner column names the **feature** in kebab-case, not the package path. Default it to the package
path with `/` replaced by `-` (`product/category` → `product-category`) and present it as editable:
the two do not always agree (`exchangerate` is owned by `exchange-rate`), and how the business names
the feature is not a generator's call.

Draft the definition from the feature and aggregate names. A definition nobody edited is a
definition nobody agreed to; the row is worth more empty than plausibly wrong.

<!-- sample-api:begin -->
Place the row inside the `sample-api:begin` / `sample-api:end` markers when the feature is
sample-derived, outside them otherwise. A row on the wrong side of a marker either vanishes with the
sample or outlives it.
<!-- sample-api:end -->

This responsibility exists only when a domain layer introduces the terms. A projection-only feature
with no aggregate and no domain spec — only a QueryService — would otherwise have nobody to introduce
its read-side words, so `/glossary` owns that case. This skill runs precisely when an aggregate is
being created; extending it to projections would duplicate `/glossary`'s responsibility.

## Step 5. Closing

```text
docs/spec/domain/<pkgpath>.md を作成しました。次は editor で TODO を埋めてください。
用語表には集約名だけを登録しました。TODO を埋めると値オブジェクトや状態の語が現れるので、
そのときは /glossary で用語表へ反映してください（新出用語・orphan・同音異義を出します）。
usecase spec も必要なら new-spec-usecase または統合 new-spec を使ってください
（lean A 構成: controller / infra spec は不要、OpenAPI gen + sqlc gen から導出されます）。
```

**`new-spec-usecase` has no equivalent step, deliberately.** A usecase spec declares an Interface,
DTOs and a Workflow — application-layer names like `Create<Aggregate>` or `<Aggregate>View`, which do not
pass the glossary's bar (would someone who knows the business recognise it as a word of the
business?). The domain layer introduces terms; the usecase layer uses them.

## AI Modification Scope

- Write scope: new files under `docs/spec/domain/`, plus one appended row in
  `docs/spec/glossary.md`.
- Aborts if the domain spec at that path exists.

## Constraints

- ❌ Overwrite an existing domain spec
- ❌ Invent business content (fields / methods)
- ❌ Hardcode section list (always read `.codex/scaffold-spec/domain-spec.md`)
- ❌ Skip the identity `ask the user explicitly`
- ❌ Touch any tree other than `docs/spec/domain/`
- ❌ Resolve a glossary homonym, or decide which name wins
- ❌ Register anything beyond the aggregate name (the rest are still TODOs)
- ❌ Write a definition without saying it is a draft to be rewritten
- ✅ Japanese user-facing output
- ✅ Validate the package path (lowercase, `/`-separated, no hyphens) + aggregate name PascalCase

## Checklist

- [ ] Domain package path + aggregate name confirmed via `ask the user explicitly`
- [ ] `.codex/scaffold-spec/domain-spec.md` read for current section list
- [ ] The domain spec at that path did NOT already exist (skill aborts otherwise)
- [ ] Entity YAML `package:` matches the file's path
- [ ] Template written with H2 sections + YAML codeblocks + TODO markers
- [ ] Aggregate name checked against `docs/spec/glossary.md` (homonym → report and stop)
- [ ] Glossary row added only after asking the user explicitly, on the correct side of the sample markers
- [ ] Final summary in Japanese, handing the remaining terms to `/glossary`
- [ ] Only `docs/spec/domain/<pkgpath>.md` and one glossary row were written
