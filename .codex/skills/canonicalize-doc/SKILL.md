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

`AGENTS.md` tells agents never to read a `*.ja.md`, and that rule is about knowledge sourcing: a
translation lags its original, so reasoning from it produces stale answers and the English canonical
is what to read instead.

This skill's subject is the pair itself, which is the one case that reasoning does not cover. A sync
that may not read the side it is updating has to overwrite it blind — discarding the wording already
established there and the structure the two files hold in common — or else stop and hand the work
back, which is how a canonical file ends up shipped beside a translation a generation behind it.

`AGENTS.md` carries the exception for exactly this case; read it there rather than inferring it from
the paragraph above. **The permission is not this skill's to grant**, and nothing here widens it: it
covers the pair confirmed above, for the duration of this run, for the purpose of locating where a
change lands and reusing the terms already in use. English stays canonical, the translation is never
the source of truth for a fact about the system, and no `*.ja.md` outside the confirmed pair is
opened. If `AGENTS.md` does not carry that exception, stop and ask a human — a skill declaring its
own exemption from a repository rule is the loophole `AGENTS.md` forbids, not a shortcut around a
gap in it.

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
