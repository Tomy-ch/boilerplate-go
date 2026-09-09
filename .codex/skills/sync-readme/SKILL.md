---
name: sync-readme
description: >-
  Update a specified canonical README so it matches the actual files and directories beneath its location — missing files, renamed files, removed entries, outdated descriptions — and re-sync its sibling translation afterwards via `canonicalize-doc`. Use it when files were added, renamed or removed under a documented directory and its README no longer describes what is there. Do NOT use it to judge the README's prose quality (`readme-review` / `doc-reviewer`) or to fix README-to-code semantic drift (`back-prop`).
---

# Sync README

This skill reconciles a README document with the actual file/directory layout under its directory. It rewrites the README to fix drift while respecting nested README boundaries: when a subdirectory has its own README, that subdirectory is summarized via a one-line digest and a reference link, not recursively expanded.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

Use this skill when:

- A README has fallen out of sync with its directory (files added/removed/renamed, descriptions outdated).
- You want a single directory's README refreshed without touching nested READMEs.
- You need to surface "documented but missing" or "exists but undocumented" entries before deciding what to fix.

Do NOT use this skill to recursively rewrite every README in a tree. It operates on exactly one README per invocation. To refresh a whole tree, invoke it once per directory.

## First Step: Confirm Input

This skill **MUST call `ask the user explicitly` immediately after invocation** to confirm:

1. **Target README path** — the canonical README to update. If the user supplied a path in skill arguments or the recent message, present it as a candidate.
2. **Scope confirmation** — confirm the directory boundary (the directory containing the target README) and the depth limit (default: only direct children; nested directories are summarized via their own README if present).

Do NOT read the file tree or write any file until these are confirmed.

This skill always operates on the **canonical** (English) README. Translation files (`README.ja.md` etc.) are re-synced automatically by chaining into the `canonicalize-doc` skill after the canonical update completes — do NOT modify translation files inline within this skill.

If the target the user supplied is itself a translation file (e.g., `README.ja.md` without a sibling `README.md`), ask whether to:

- Treat it as the canonical (rare; only when no English version exists).
- Generate the canonical first via the `canonicalize-doc` skill, then re-run this skill against the canonical.

## How the Sync Works

### Scope

- The README's directory is the **scope root**.
- The skill enumerates entries directly inside the scope root.
- For each entry:
  - **File**: include it in the README listing with a one-line description.
  - **Directory without its own README**: list it and (optionally, if shallow) summarize its immediate notable children.
  - **Directory with its own README**: include only a one-line digest + a reference link to that nested README. Do NOT recurse into its contents.

### Detecting drift

Compare the README's documented entries against the actual entries:

- **Documented but missing on disk** → remove the entry (or flag for user confirmation if the entry is load-bearing, e.g., links from elsewhere).
- **Exists on disk but undocumented** → add it.
- **Renamed** (likely: same description, similar path) → update the path.
- **Outdated description** (e.g., the file's role has changed) → infer the new role from the file's contents and update.

### Things NOT to touch

- Hidden files/dirs (`.git`, `.DS_Store`, `.gitkeep`, etc.) unless the README clearly documents them.
- Build artifacts and ignored files (anything matched by `.gitignore` at or above the scope root).
- Generated files (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, `vendor/`).
- Nested directory internals when that directory has its own README.

## Repo Conventions

- The canonical README is `README.md` (English). The Japanese translation, if present, is `README.ja.md` co-located in the same directory.
- When updating both, keep heading structure, list order, and table columns 1:1 between the two files.
- Preserve existing section ordering and styling (tables vs lists vs prose) unless the user explicitly asks to restructure.
- Preserve existing prose that is still accurate. Do not rewrite for stylistic reasons.

## AI Modification Scope

Per the "Exception: Skill Execution" clause in AGENTS.md, the normal AI Modification Scope restrictions are relaxed during this skill's execution, scoped to:

- The confirmed target canonical README file.
- Sibling translation files are NOT modified by this skill directly; they are updated in the subsequent `canonicalize-doc` invocation (which runs under its own scope exception).

The following remain protected even during skill execution:

- `AGENTS.md`
- Generated files (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, generated content under `docs/`)
- All other files and directories under the scope root (the skill reads them but never modifies them).

## Execution Steps

### 1. Read the target README

Read the target README in full to understand:

- Its existing section structure.
- The entries it currently documents (files, directories, links).
- Any custom conventions (e.g., a "Directory Layout" table, a "Subprojects" list).

### 2. Enumerate the actual file tree

List the entries directly inside the scope root. For each:

- Record its name, type (file / directory), and (for directories) whether it contains its own README.
- For files, read enough to determine the role/purpose (the file's package comment, top-level docstring, or first few lines).
- For directories with their own README, read only the first paragraph of that nested README to extract a digest.
- For directories without their own README, optionally list immediately-notable children (do not deep-recurse).

### 3. Compute the diff

Build three lists:

- **To add**: exists on disk, absent or outdated in the README.
- **To remove**: documented in the README, no longer exists on disk.
- **To update**: present in both but description/path is wrong.

If any item in **To remove** is referenced from elsewhere (links from other docs), surface that to the user before deletion.

### 4. Apply the update

Rewrite the README so it reflects reality:

- Insert new entries in the appropriate section, matching the existing format.
- Remove stale entries.
- Update outdated descriptions and paths.
- For child directories with their own README, ensure the entry is a one-line digest with a relative link to the nested README (e.g., `- [sub/](sub/README.md) — short digest`).
- Preserve unrelated sections (introduction, badges, license, etc.) verbatim.

### 5. Verify the canonical update

- Confirm every entry in the updated README maps to a real file or directory.
- Confirm no real entry (other than ignored ones) is missing.
- Confirm no nested README was inadvertently expanded.

### 6. Chain into `canonicalize-doc` to sync the translation

After the canonical README is written:

1. Check whether a sibling translation file exists (e.g., `README.ja.md` next to the updated `README.md`).
2. If it does, invoke the `canonicalize-doc` skill via the Skill tool with:
    - source path: the canonical README that was just updated
    - direction: `translation-from-canonical` (or `sync-both` with the canonical as source of truth, if the translation already exists)
3. If no translation file exists, skip this step and report that the canonical was updated standalone.

The chained `canonicalize-doc` call will perform its own `ask the user explicitly` confirmation; that is expected and not redundant — it lets the user veto the translation sync if needed.

### 7. Verify with Markdown Lint

After writing the canonical README (and after `canonicalize-doc` has produced any translation), run:

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

`make md-fix` operates on the entire repository, so it may modify Markdown files unrelated to the README pair. List any such files when reporting completion so the user can review the broader change set.

### 8. Final verification

- Confirm both files (canonical and translation) have parity if the translation was synced.
- Report which entries were added / removed / updated and which files were written.

## Checklist

Confirm the following before reporting completion:

- [ ] Target canonical README path confirmed with the user via `ask the user explicitly`
- [ ] Scope root and depth limit confirmed
- [ ] Drift computed (add / remove / update lists produced)
- [ ] Canonical README rewritten with correct entries and preserved structure
- [ ] Child directories with their own README represented as one-line digests + reference links (not expanded)
- [ ] If a sibling translation file exists, `canonicalize-doc` was invoked to re-sync it
- [ ] `make md-lint` exits cleanly
- [ ] No file outside the canonical README (and the chained `canonicalize-doc` scope) modified

## Notes

- Do NOT recursively rewrite nested READMEs. Each invocation handles exactly one README's scope.
- Do NOT delete documented entries blindly. When an entry is removed from disk, confirm it isn't referenced from elsewhere before pruning the line.
- Do NOT restructure or restyle sections that are still accurate. Minimize churn.
- If the directory has no obvious convention to follow (e.g., a fresh README with no structure), ask the user whether to use a table, a bulleted list, or prose.
- If the README intentionally documents items outside its directory (e.g., a top-level README listing project-wide entries), confirm the scope with the user before treating those external references as drift.
