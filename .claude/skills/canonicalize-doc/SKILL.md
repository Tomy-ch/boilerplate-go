---
name: canonicalize-doc
description: >-
  Create or sync a canonical English / Japanese translation pair for a specified README, SKILL, or similar Markdown document, producing the missing side or syncing both while preserving repo conventions (SKILL frontmatter rules, sync-note headers, cross-reference links). The source file and direction are confirmed with the user. Use it when a document exists in only one language, or when the pair has fallen out of sync.
---

# Canonicalize Doc

This skill produces or synchronizes a paired English (canonical) / Japanese (translation) Markdown document set, following the conventions used in this repository.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

Use this skill when:

- A Japanese-only Markdown doc exists and you need to produce the English canonical version (the typical case in this repo, where English is canonical).
- An English-only Markdown doc exists and you need to produce its Japanese translation.
- Both versions exist but have drifted apart and need to be synchronized.

Supported document types:

- Claude Code skill files: `SKILL.md` / `SKILL.ja.md` (under `.claude/skills/<name>/`)
- READMEs: `README.md` / `README.ja.md` (co-located in the same directory, e.g. `docker/server/`)
- Generic Markdown docs with the `*.ja.md` suffix convention (co-located).
- `docs/**` documents, which follow the same co-located convention (`docs/**/<name>.md` and `docs/**/<name>.ja.md`).

## First Step: Confirm Input

This skill **MUST call `AskUserQuestion` immediately after invocation** to confirm:

1. **Source file path** — the file the user is pointing at (canonical or translation).
2. **Direction** — what to produce:
    - `canonical-from-translation`: translate the Japanese into English and create the canonical file.
    - `translation-from-canonical`: translate the English into Japanese and create the translation file.
    - `sync-both`: both files exist; reconcile differences and rewrite the side that should be updated.

Procedure:

1. If the user supplied a file path in skill arguments or the most recent message, include it as a candidate.
2. Inspect the surrounding directory to detect the counterpart file (if any) and the convention being used (co-located `*.ja.md`).
3. Call `AskUserQuestion` with:
    - Question 1: "Confirm the source file path." (include the detected candidate)
    - Question 2: "Which direction? (canonical-from-translation / translation-from-canonical / sync-both)" — include the recommended option based on what files already exist.
    - If `sync-both`, also ask: "Which side is the source of truth for this sync?"

Do NOT read or write any files for translation until the source path and direction are confirmed.

## Repo Conventions

Apply these rules when producing each side of the pair.

### Language

- **English is canonical.** The English file is the source of truth.
- **Japanese is a translation.** It must be kept in sync with the canonical English version.

### SKILL files (`.claude/skills/<name>/`)

- `SKILL.md` (canonical, English):
  - MUST include YAML frontmatter with `name` and `description` fields.
  - `description` should be in English and written to maximize skill-selection accuracy.
  - Include a one-line pointer near the top: `A Japanese reference translation of this skill is available at SKILL.ja.md in the same directory (not loaded as a skill; for human reference only).`
- `SKILL.ja.md` (translation, Japanese):
  - MUST NOT include YAML frontmatter (frontmatter would risk being misinterpreted by tooling; only `SKILL.md` is loaded as a skill, but keeping it absent removes any ambiguity).
  - MUST begin with a blockquote header in Japanese stating that it is a translation, that direct edits are forbidden, and that updates flow from `SKILL.md`.

### README and generic docs

- `README.md` / `README.ja.md` (co-located in the same directory):
  - No frontmatter requirements.
  - The translation file should begin with a short note that it is a translation of the canonical file.
  - The canonical file may optionally link to the translation (`See [README.ja.md](README.ja.md) for the Japanese version.`).

### `docs/**` parallel-tree docs

- Canonical: `docs/<path>/<name>.md`
- Translation: `docs/<path>/<name>.ja.md`
- Keep section structure, headings, and link targets 1:1 between the two files.

## AI Modification Scope

Per the "Exception: Skill Execution" clause in AGENTS.md, the normal AI Modification Scope restrictions are relaxed during this skill's execution, scoped to the specific document pair the user confirmed in the first step.

Paths that may be modified:

- The confirmed source file.
- Its counterpart file (created or rewritten).
- Nothing else.

The following remain protected even during skill execution:

- `AGENTS.md` / `CLAUDE.md`
- Generated files (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, generated content under `docs/`)
- Any path listed under `permissions.deny` in `.claude/settings.json`

### Reading the translation side is permitted here

`AGENTS.md` tells agents never to read a `*.ja.md`. That rule is about using a translation as a
source of knowledge: it lags its original, so reasoning from it produces stale answers, and the
English canonical is what to read instead.

This skill's subject is the pair itself, which is the one case the rule does not fit. A sync that
may not read the side it is updating has to overwrite it blind — discarding the wording already
established there and the structure the two files hold in common — or else stop and hand the work
back, which is how a `SKILL.md` ends up shipped beside a translation a generation behind it.

So for the duration of this run, and only for the pair confirmed in the first step, read the
`*.ja.md` freely: to locate where a change lands, and to reuse the terms it already uses. Nothing
else moves. English stays canonical, the translation is never the source of truth for a fact about
the system, and no `*.ja.md` outside the confirmed pair is opened.

## Execution Steps

### 1. Read the source

Read the confirmed source file in full. If the direction is `sync-both`, read both files.

### 2. Determine the output path

- `canonical-from-translation`:
  - `foo.ja.md` → `foo.md` in the same directory.
  - `docs/<path>/<name>.ja.md` → `docs/<path>/<name>.md`.
- `translation-from-canonical`:
  - `foo.md` → `foo.ja.md` in the same directory.
  - `docs/<path>/<name>.md` → `docs/<path>/<name>.ja.md`.
- `sync-both`:
  - Rewrite the non-source-of-truth side.

### 3. Translate (or sync)

- Preserve heading structure, list nesting, code blocks, and link targets exactly.
- Translate prose, comments within code blocks (only if originally translated), and inline text.
- Do NOT translate: identifiers, file paths, command invocations, code samples (unless they contain natural-language strings that were originally translated).
- For SKILL files, also apply the frontmatter and sync-note rules described in "Repo Conventions" above.

### 4. Write the output

- Write the produced file to its target path.
- If both files exist and the user chose `sync-both`, write only to the non-source-of-truth side.

### 5. Add cross-references

- If creating a `SKILL.md`, ensure it contains the one-line pointer to `SKILL.ja.md`.
- If creating a `SKILL.ja.md`, ensure the leading blockquote sync note is present.
- For README pairs, add the link from canonical to translation only if requested.

### 6. Verify

- Diff section counts and heading text between the two files; they should match 1:1.
- Confirm code blocks are byte-identical (except where prose was translated inside them).
- Report any sections that could not be cleanly mapped and ask the user how to resolve them.

### 7. Verify with Markdown Lint

After writing the produced file (and the synced side in `sync-both` mode), run:

```sh
make md-fix
make md-lint
```

`make md-fix` runs `markdownlint-cli2 --fix` on the entire repository to auto-fix common issues (blank-line placement around headings / lists / code blocks, trailing whitespace, file-final newline, etc.). `make md-lint` then verifies that the result is clean against `.markdownlint-cli2.yaml`.

If `make md-lint` reports remaining errors:

1. Read the lint output.
2. Fix the violations manually (rules that auto-fix cannot resolve, e.g., heading hierarchy, duplicate headings, bare URLs).
3. Re-run `make md-fix` then `make md-lint` until clean.

Do NOT report the skill as complete until `make md-lint` exits cleanly.

`make md-fix` operates on the entire repository, so it may modify Markdown files unrelated to the confirmed pair. List any such files when reporting completion so the user can review the broader change set.

## Checklist

Confirm the following before reporting completion:

- [ ] Source file path confirmed with the user via `AskUserQuestion`
- [ ] Direction (`canonical-from-translation` / `translation-from-canonical` / `sync-both`) confirmed
- [ ] If `sync-both`, source-of-truth side confirmed
- [ ] Output file path follows the repo convention for that doc type
- [ ] Frontmatter rules applied correctly (canonical SKILL has it; translation SKILL does not)
- [ ] Translation sync-note header present in `*.ja.md` SKILL files
- [ ] Section structure and code blocks match 1:1
- [ ] `make md-lint` exits cleanly
- [ ] No unintended files modified outside the confirmed pair

## Notes

- Do NOT modify files outside the confirmed document pair.
- Do NOT translate identifiers, file paths, commands, or other technical tokens.
- When translating to Japanese, follow AGENTS.md's "Output Language" rule (Japanese for human-visible prose); when translating to English, prefer concise, declarative wording suitable for AI skill descriptions.
- If the user asks for a doc type not listed in "Supported document types", confirm the intended naming convention before proceeding.
