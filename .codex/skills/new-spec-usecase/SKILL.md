---
name: new-spec-usecase
description: >-
  Create the usecase layer spec template at `docs/spec/usecase/<pkgpath>.md`, where `<pkgpath>` is the Go package path under `internal/usecase/` (`address`, `product/ranking`, `user/search`) — the spec tree mirrors the package tree. Asks for the package path + Usecase interface name (default `Usecase`), then writes a Markdown file with YAML placeholders and TODO markers. NEVER overwrites an existing file, and invents no business content. Use it when starting a new usecase package. Do NOT use it to scaffold both spec layers at once (`new-spec`) or to implement the layer (`scaffold-usecase`).
---

# New Spec — Usecase

Create the usecase layer spec template for one feature.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Starting a new feature and need the usecase layer spec template.
- Adding the usecase spec for a feature whose domain spec is already in place.

Do NOT use this skill for:

- Editing an existing usecase spec — open in the editor directly.
- Generating Go code from spec — that's `scaffold-usecase`.
- Validating spec consistency — that's `verify-spec`.
- Creating both layer specs (domain + usecase) in one go — use the integrator `new-spec`. lean A only requires these two specs; controller / infra are derived from OpenAPI gen + sqlc gen, no spec file.

## What This Skill Reads / Writes

**Reads (always)**:

- `.codex/scaffold-spec/usecase-spec.md` — canonical section list for the usecase layer.
- `docs/spec/usecase/<pkgpath>.md` — checks whether the spec already exists.

**Writes (with confirmation)**:

- `docs/spec/usecase/<pkgpath>.md` — template file.

**Never touches**:

- An existing usecase spec at that path (aborts if found).
- Any other layer's spec file.

## First Step: Confirm Identity

Spec identity is the **package path**: `docs/spec/usecase/<rest>.md` corresponds to `internal/usecase/<rest>`, and the Interface YAML's `package:` declares the same path. The package path replaces the old feature-name + package-name pair — it is one identifier, and it is the one that decides where the file goes.

This skill **MUST call `ask the user explicitly` immediately after invocation** (unless invoked from the `new-spec` integrator with the package path in context):

1. **Usecase package path** — free-text, the path under `internal/usecase/` (e.g., `address`, `product/ranking`, `user/search`). Validate `^[a-z][a-z0-9]*(/[a-z][a-z0-9]*)*$` — Go package names carry no hyphens, so `user-search` is not a package path; the nested form `user/search` is.
2. **Usecase interface name** — free-text, default `Usecase` (PascalCase).

If `docs/spec/usecase/<pkgpath>.md` exists → abort. Otherwise create its parent directory if missing.

## Step 1. Read Section Definitions

Read `.codex/scaffold-spec/usecase-spec.md` for the canonical section list:

1. Overview
2. Interface
3. DTOs
4. Dependencies
5. Workflow

NEVER hardcode — re-read the spec format file each run.

## Step 2. Generate the Template

Assemble Markdown with H1 title `<Package Display> — Usecase Spec` (the package path title-cased — `user/search` → `User Search`), then per section:

- Overview: single `TODO:` line
- Interface: YAML code block with `package` / `name` / `methods` (1 placeholder method). `package:` MUST be `internal/usecase/<pkgpath>` — the same path the file sits at, which `verify-spec` asserts
- DTOs: YAML code block with placeholder DTO struct
- Dependencies: YAML code block with placeholder boundary list. This section is also what resolves the domain specs this spec is checked against — a Repository entry names `internal/domain/<X>`, and `verify-spec` reads `docs/spec/domain/<X>.md` from it, never from this file's own path
- Workflow: H3 per method with YAML block (`tx_required` / `steps` / `calls` / `errors`)

Use the example output shown in `.codex/scaffold-spec/usecase-spec.md` as the literal template format.

## Step 3. Confirm and Write

Display proposed path + first ~20 lines, then ask:

- 「以下の内容で `docs/spec/usecase/<pkgpath>.md` を作成しますか？」
- Options: 「作成する」 / 「キャンセル」

If approved, `mkdir -p` the file's parent directory (`docs/spec/usecase/<pkgpath>` minus the last segment) and `Write` the file.

## Step 4. Closing

```text
docs/spec/usecase/<pkgpath>.md を作成しました。次は editor で TODO を埋めてください。
domain spec も必要なら new-spec-domain または統合 new-spec を使ってください
（lean A 構成: controller / infra spec は不要、OpenAPI gen + sqlc gen から導出されます）。
```

## AI Modification Scope

- Write scope: new files under `docs/spec/usecase/` only.
- Aborts if the usecase spec at that path exists.

## Constraints

- ❌ Overwrite an existing usecase spec
- ❌ Invent business content (methods / DTOs / workflow)
- ❌ Hardcode section list (always read `.codex/scaffold-spec/usecase-spec.md`)
- ❌ Skip the identity `ask the user explicitly`
- ❌ Touch any tree other than `docs/spec/usecase/`
- ✅ Japanese user-facing output
- ✅ Validate the package path (lowercase, `/`-separated, no hyphens)

## Checklist

- [ ] Usecase package path + interface name confirmed via `ask the user explicitly`
- [ ] `.codex/scaffold-spec/usecase-spec.md` read for current section list
- [ ] The usecase spec at that path did NOT already exist (skill aborts otherwise)
- [ ] Interface YAML `package:` matches the file's path
- [ ] Template written with H2 sections + YAML codeblocks + TODO markers
- [ ] Final summary in Japanese
- [ ] Only `docs/spec/usecase/<pkgpath>.md` was written
