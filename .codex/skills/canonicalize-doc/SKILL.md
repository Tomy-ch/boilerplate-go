---
name: canonicalize-doc
description: >-
  Create or synchronize a canonical English Markdown document and its Japanese translation. Use for `README.md`/`README.ja.md`, `SKILL.md`/`SKILL.ja.md`, generic co-located `*.ja.md` pairs, or the same pairs under `docs/`. Modifies only the selected pair, confirming the source and direction when they are not explicit.
---

# Canonical Document Sync

English is canonical; Japanese is a translation. Work on exactly one confirmed document pair.

## Resolve the pair

Accept a supplied file path and direction. If either is absent or ambiguous, inspect only the surrounding paths and ask the user to specify:

- source file;
- direction: `canonical-from-translation`, `translation-from-canonical`, or `sync-both`;
- for `sync-both`, which file is authoritative.

Supported mappings:

| Document type | Canonical | Translation |
| --- | --- | --- |
| co-located README/generic Markdown | `foo.md` | `foo.ja.md` |
| skill | `SKILL.md` | `SKILL.ja.md` |
| documentation tree | `docs/<path>/foo.md` | `docs/<path>/foo.ja.md` |

Do not proceed if the pair cannot be determined safely.

### Reading the translation side

`AGENTS.md`'s instruction never to read a `*.ja.md` governs knowledge sourcing: a translation can
lag its original, so reason from the English canonical instead. This skill operates on the pair
itself, where that rule does not fit. Refusing to read the confirmed translation would force a blind
overwrite that loses established terms and shared structure, or stop the sync and leave a canonical
file beside a translation one generation behind.

For the duration of this run, and only for the pair confirmed above, reading the `*.ja.md` is
permitted to locate where a change lands and reuse the terms already in use there. English remains
canonical; the translation is never a source of truth for facts about the system; do not open any
other `*.ja.md`.

## Translate or synchronize

1. Read the source file completely; for `sync-both`, read both files.
2. Preserve heading hierarchy, list nesting, link destinations, identifiers, paths, commands, and code blocks. Translate prose only.
3. For a canonical `SKILL.md`, keep valid English `name` and `description` frontmatter. Add a brief pointer to `SKILL.ja.md` when the translation exists.
4. For `SKILL.ja.md`, omit YAML frontmatter and begin with a Japanese blockquote stating it is a translation maintained from `SKILL.md` and should not be edited independently.
5. For README and generic translations, begin with a concise note that the English counterpart is canonical. Add a link from the English side only when requested.

## Write and verify

- Modify only the confirmed source/counterpart pair. Never modify `AGENTS.md`, generated content, or unrelated Markdown.
- Compare the final heading structure and section count. Code blocks must remain byte-identical except for intentional natural-language prose inside them.
- Run `make md-lint` after writing. Do not run repository-wide `make md-fix` without explicit approval because it may change unrelated files.
- Report unmapped sections or translation uncertainty instead of guessing.

State the source, direction, files changed, and Markdown lint result. Do not stage, commit, or push.
