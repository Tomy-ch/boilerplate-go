---
name: tool-map
description: >-
  Inventory every command / skill / agent registered under this project's `.claude/` directory and produce an inventory table plus a Mermaid dependency map. Use it when asking what skills exist, which ones call which, or for a periodic sweep of the agent surface. Read-only by default; writes only when `--output=file` is chosen.
argument-hint: '[--lang=en|ja] [--output=inline|file] [--output-path=<path>] [--include=commands,skills,agents]'
allowed-tools: Bash(ls:*), Bash(find:*), Bash(test:*), Bash(make md-fix:*), Bash(make md-lint:*), Read, Write, AskUserQuestion
---

# Tool Map

You have been invoked via `/tool-map`. The user's argument string is: `$ARGUMENTS`

This command inventories every customization entry under the **project-level** `.claude/` directory (it does NOT scan `~/.claude/`). It covers three types — commands, skills, and agents — and produces a single Markdown report.

## Step 1. Resolve Inputs

Parse `$ARGUMENTS` for the following optional flags. For each flag missing or invalid, fall back to `AskUserQuestion` to confirm.

| Flag | Values | Default |
| --- | --- | --- |
| `--lang` | `en` / `ja` | `ja` |
| `--output` | `inline` / `file` | `inline` |
| `--output-path` | any relative path | `./TOOL_MAP.md` (en) or `./TOOL_MAP.ja.md` (ja) — only used if `--output=file` |
| `--include` | comma-separated subset of `commands,skills,agents` | `commands,skills,agents` (all three) |

`AskUserQuestion` fallback questions (only ask for unresolved flags):

1. Output language? — `en` / `ja`
2. Output destination? — `inline` / `file` (if `file`, also ask `--output-path`)
3. Which entry types to include? — `commands` / `skills` / `agents` / `all`

Do NOT scan or write anything until all required values are resolved.

## Step 2. Enumerate Entries

Scan only project-level paths under the current working directory:

| Type | Path glob | Entry file |
| --- | --- | --- |
| commands | `.claude/commands/` | `<name>.md` <!-- skill-lint-ignore --> |
| skills | `.claude/skills/` | `<name>/SKILL.md` (skip `SKILL.ja.md` and other `*.ja.md` translation files) |
| agents | `.claude/agents/` | `<name>.md` |

Discovery commands (use `Bash` with `find` / `ls`, then `Read` per file):

```sh
test -d .claude/commands && find .claude/commands -maxdepth 1 -type f -name '*.md'
test -d .claude/skills   && find .claude/skills   -mindepth 2 -maxdepth 2 -type f -name 'SKILL.md'
test -d .claude/agents   && find .claude/agents   -maxdepth 1 -type f -name '*.md'
```

Skip:

- `*.ja.md` translation files (they are not loaded as entries).
- Any directory under `.claude/skills/` that lacks a `SKILL.md`.
- Hidden files (`.DS_Store` etc.).

## Step 3. Extract Metadata

For each entry found, read its frontmatter and body. Extract:

| Field | commands | skills | agents |
| --- | --- | --- | --- |
| Name | filename stem | `name:` (fallback: dir name) | filename stem (also frontmatter `name:` if present) |
| Description | `description:` | `description:` | `description:` |
| Argument hint | `argument-hint:` | — | — |
| Allowed tools | `allowed-tools:` | — | `tools:` |
| Model override | `model:` | — | `model:` |
| Path | relative to working dir | relative to working dir | relative to working dir |

Description should be truncated to the first sentence (or ~120 characters) for the inventory table.

## Step 4. Detect Dependencies

A dependency is any cross-reference where one entry invokes or explicitly references another entry by name. Detect them in the body text by:

1. **Explicit invocation phrases**: `invoke the` + name + `skill`, `chains into` + name, `via the Skill tool with` + name, `Agent({ subagent_type: '` + name + `' })`, `/<name>` literal invocations of other commands.
2. **Backtick-wrapped names** that match the `name:` of another scanned entry.
3. **Section headers** like `Chain`, `Calls`, `Depends on`, `Chains into` followed by name references.

Record dependencies as directed edges `caller → callee`. Tag each edge with the caller type and callee type (e.g., `skill → skill`, `skill → command`).

Exclude:

- Self-references (an entry mentioning its own name).
- Mentions inside fenced code blocks whose language is unrelated (`bash`, `sh`, `make`, `sql`, etc.) **unless** the snippet is clearly an invocation example.
- Generic English words that coincidentally match a name. Require the match to be backtick-wrapped or follow one of the invocation phrases.

If a detected reference targets a name that does not exist in the scanned set, record it as a **broken edge** for the Notes section.

## Step 5. Render the Report

Produce the four sections below in the chosen language. Skill names and paths remain verbatim regardless of language.

### 1. Summary

A one-line count per type. Only show types that were included.

- `Commands: N`
- `Skills:   M`
- `Agents:   K`
- `Total:    N + M + K`

### 2. Inventory Tables

One table per included type, in this order: commands → skills → agents. Omit a table if its type has 0 entries (still report 0 in Summary).

- **Commands table**: Name | Description | Args | Allowed Tools | Model | Dependencies | Path
- **Skills table**: Name | Description | Dependencies | Path
- **Agents table**: Name | Description | Tools | Model | Dependencies | Path

`Dependencies` is a comma-separated list of `<callee-name> (<callee-type>)` entries; empty if none.

### 3. Dependency Graph

A Mermaid `graph LR` diagram. Apply a class to each node based on its type so the reader can distinguish:

```mermaid
graph LR
  classDef cmd fill:#cce5ff,stroke:#3b82f6
  classDef skl fill:#d4edda,stroke:#22c55e
  classDef agt fill:#fff3cd,stroke:#f59e0b
  %% nodes (include isolated entries so standalone tools appear)
  %% edges: caller --> callee
  %% class assignments
```

Use the entry name as the node id (replace any disallowed Mermaid characters with `_`).

### 4. Notes

A short prose section calling out:

- **Leaf entries** (no outgoing or incoming edges).
- **Hub entries** (depended on by 2 or more).
- **Broken edges** (references to names not present in the scan).
- **Cross-type chains** (e.g., a skill that invokes a command — possible but unusual).
- **Empty types** (e.g., "No project-level agents found").

### Language

- `en`: section headings, prose, table headers in English.
- `ja`: section headings, prose, table headers in Japanese.

## Step 6. Emit

- `inline`: include the full report in the response and stop.
- `file`: write the report to `--output-path` and respond with a short confirmation (one-line summary + the file path). Do NOT also duplicate the full report in the response.

## Step 7. Verify with Markdown Lint (only when `--output=file`)

When `--output=file`, after writing the report run:

```sh
make md-fix
make md-lint
```

`make md-fix` runs `markdownlint-cli2 --fix` on the entire repository to auto-fix common issues (blank-line placement around headings / lists / code blocks, trailing whitespace, file-final newline, etc.). `make md-lint` then verifies that the result is clean against `.markdownlint-cli2.yaml`.

If `make md-lint` reports remaining errors:

1. Read the lint output.
2. Fix the violations manually (rules that auto-fix cannot resolve, e.g., heading hierarchy, duplicate headings, bare URLs).
3. Re-run `make md-fix` then `make md-lint` until clean.

Do NOT report the command as complete until `make md-lint` exits cleanly.

`make md-fix` operates on the entire repository, so it may modify Markdown files unrelated to the report. List any such files when reporting completion so the user can review the broader change set.

When `--output=inline`, skip this step (no file was written).

## Constraints

- This command is **read-only by default**. Writes are only allowed when the user explicitly chose `--output=file`, and only to the confirmed output path.
- Scan scope is **project-level only**. Do NOT read or list anything under `~/.claude/`.
- Plugin-provided entries are out of scope.
- Do NOT modify any scanned entry. This command only inspects.
- If `.claude/commands/`, `.claude/skills/`, or `.claude/agents/` does not exist, treat its entry count as 0 and note it in the report rather than erroring. <!-- skill-lint-ignore -->

## Checklist

Before reporting completion, confirm:

- [ ] All required inputs resolved (via `$ARGUMENTS` or `AskUserQuestion`)
- [ ] Only project-level `.claude/{commands,skills,agents}/` scanned
- [ ] `*.ja.md` files excluded from the skills scan
- [ ] Frontmatter parsed for each entry (name, description, type-specific fields)
- [ ] Dependencies detected per the documented rules (self-refs excluded; broken edges recorded)
- [ ] Report contains Summary, Inventory Tables, Dependency Graph (Mermaid), Notes
- [ ] Standalone entries appear as isolated nodes in the graph
- [ ] If `--output=file`, the file was written and `make md-lint` exits cleanly
- [ ] If `--output=file`, no other path outside the confirmed output (and `make md-fix` side effects) was modified
