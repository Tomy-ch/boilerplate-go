# `.claude/` — Agent configuration for this repository

This directory holds the Claude Code configuration that ships **with the repo**: reusable skills,
subagents, the spec templates the scaffolding reads, helper scripts, and project-scoped permissions /
plugin declarations. Anyone who clones and trusts the repo inherits the same agent behavior.

## Relationship to `AGENTS.md`

`AGENTS.md` (repo root) is the human-maintained **operational contract** — what an agent may touch and
how it must behave. It is the source of truth; this README does not restate its rules. In particular,
`AGENTS.md` treats `.claude/**` as **out of scope for AI edits** unless the user explicitly requests a
change or a running skill relaxes the scope for its own duration. Read `AGENTS.md` first.

## Layout

| Path | What it is |
| --- | --- |
| `settings.json` | Project-scoped permissions (`allow` / `ask` / `deny`), enabled plugins, and known marketplaces. Generated artifacts are denied for `Edit` / `Write`; `AGENTS.md` and its `CLAUDE.md` symlink require confirmation. Shared with everyone who trusts the repo. |
| `skills/<name>/` | Reusable skills. Each has an English-canonical `SKILL.md` (+ a `SKILL.ja.md` reference translation) and optional bundled `scripts/` / `references/`. Invoked as `/<name>`. |
| `agents/` | Subagent definitions used by the skills (e.g. the read-only `arch-auditor-*` / `drift-detector-*` per-layer workers that integrator skills fan out in parallel). |
| `scaffold-spec/` | Spec-format definitions (`domain-spec.md`, `usecase-spec.md`, `verify-rules.md`, …) read **at runtime** by the `scaffold-*` / `verify-spec` / `new-spec-*` skills, so format changes propagate without editing the skills. |
| `scripts/` | Repo-level agent-tooling scripts (not project build tooling — that lives in the root `scripts/`). |

## First-time setup: official plugins

This repo depends on Anthropic's official plugins, declared at **project scope** in `settings.json`
(`enabledPlugins` + `extraKnownMarketplaces`):

- `skill-creator` — wrapped by the local `manage-skill` skill for authoring/updating skills.
- `feature-dev` — guided feature-development workflow (`/feature-dev`) with `code-explorer` /
  `code-architect` / `code-reviewer` agents.

A trusted clone already has them. If they are missing (or you added a new one to the list), run the
idempotent bootstrap:

```bash
bash .claude/scripts/bootstrap-plugins.sh
```

Newly enabled plugins load on the **next** Claude Code session.

## First-time setup: standard external skills

An **external skill** is a third-party skill that is not a marketplace plugin, so the plugin
bootstrap cannot install it. This repo ships one as **standard equipment** — not an opt-in extra:
its commands and the discipline for using them are in `AGENTS.md`, which every agent loads on every
turn.

- `graphify` (`/graphify`) — parses the repo with tree-sitter into a queryable knowledge graph
  under `graphify-out/`, then answers structural questions (`query` / `affected` / `god-nodes`)
  from the graph instead of from repeated greps.

Install it for every assistant this repo supports (Claude Code + Codex CLI) with the idempotent
bootstrap. This is part of setup, not a decision to weigh:

```bash
bash .claude/scripts/bootstrap-external-skills.sh
```

Three properties differ from the plugins above and are worth knowing before running it:

- **A dependency, not a reviewed file.** The skill lands at `.claude/skills/graphify/` and
  `.codex/skills/graphify/` inside the checkout and is gitignored, the way `vendor/` and
  `node_modules/` hold a dependency's own files: built from a pin, re-made per machine, never
  reviewed. A trusted clone does *not* carry it — every machine runs the bootstrap once. The pin is
  `python/graphify.in`, and the script reads it rather than choosing a version. Standard equipment
  and untracked are not in tension here: the same is true of `vendor/`.
- **`skill-lint` does not check it.** The repository's skill conventions — frontmatter, the
  `SKILL.ja.md` pair, references that resolve — assume a skill this repository writes. This one is
  written upstream and refers to artifacts that only exist after a run, so it is declared in
  `EXTERNAL_SKILLS` (`scripts/skill-lint/checks.ts`) and skipped. Holding it to rules nobody here
  can satisfy would only teach readers to ignore the gate.
- **`graphify uninstall` does not reach it.** The uninstaller looks in the home directory; remove
  the two directories above by hand. `--purge` additionally deletes `graphify-out/`.

**The bootstrap never lets the installer write into the checkout.** Every mode the tool offers
writes something this repository keeps hard-protected: `install --project` appends a `## graphify`
section to `CLAUDE.md` — a symlink to `AGENTS.md` here — and registers PreToolUse hooks in
`.claude/settings.json`; the default user-scope mode edits `~/.claude/CLAUDE.md`; and the
similarly named `<name> install` subcommands (`graphify claude install`, `graphify codex install`,
also `opencode` / `aider` / `kilo`) plus `graphify hook install` write `AGENTS.md`, git hooks and a
merge driver. So the bootstrap runs the install against a throwaway `HOME` and copies out only the
skill directory. Those subcommands are additionally routed to `ask` in `settings.json`, so an agent
reading `graphify --help` surfaces them for a human decision instead of running them.

The graph is a derived artifact and is gitignored, so build it locally. `update` and the query
commands are AST-only and need no API key; the docs / PDF / image extraction, `--mode deep`
inference, `--wiki`, and community *naming* call an LLM API and send content off the machine, so
keep those opt-in.

```bash
make graphify-update
```

**Run the pinned build, not `mise exec "pipx:graphifyy[sql]"`.** That form resolves whatever pipx
hands back rather than going through `python/graphify.txt`, so it drifts from the lock. The two
pinned installs are the `python_tool_runner` image (what the make target above uses) and the
bootstrap's venv at `${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify/bin/graphify`, which is
what `graph-affected.ts` invokes. `AGENTS.md` carries the commands and the rest of the discipline.

What the graph excludes (committed generated artifacts, Japanese mirrors, vendored code) is
declared in `.graphifyignore`. Changing it requires a full re-extraction — an incremental `update`
is fail-closed and keeps the now-excluded nodes.

### What the graph is good for here, and what it is not

Measured on this repository, not inherited from upstream's claims:

- **`affected` is the paying command** — reverse traversal from a symbol gives the call sites that
  break if you change it, each with a relation label and `file:line`. Reach for it when planning a
  change whose blast radius is unknown. It takes a node **id**, not a symbol name, so go through
  the wrapper, which resolves the name and lists the candidates when a name is ambiguous:

  ```bash
  node .claude/scripts/graph-affected.ts NormalizeError --depth 2
  ```

- **Raise `--budget` on `query`, or read the truncation warning.** The default (~2000 tokens) cuts
  the result on a repo this size and says so in the output; the answer may be in the cut part.
- **Ignore `god-nodes` here.** It ranks by edge count, and the 1:1 test-mapping rule this repo
  enforces means test scaffolding (`Any()`, `NewTestFromSalt()`, `NewNoopTracerFactory()`) outranks
  production code. It answers "what has the most edges", which in this repo is not "what is
  central".
- **The graph is only as fresh as the last `update`.** For a question about uncommitted work,
  rebuild first or use `grep` — for a small diff, `grep` is the cheaper of the two.

## Conventions

- **English is canonical.** Skill/README bodies are written in imperative English; the paired
  `*.ja.md` is a human reference translation kept in sync via the `canonicalize-doc` skill. Runtime
  output to the user still follows `CLAUDE.md` (Japanese).
- **Authoring skills.** Use `/manage-skill` to create or update a skill — it wraps `skill-creator` and
  applies this repo's placement, frontmatter, translation-pair, and eval-artifact conventions.
- **Discovering what exists.** Run `/tool-map` for a full inventory of skills / agents / commands and a
  dependency map, instead of maintaining a hand-written list here.
- **Definitions are linted against reality.** `make md-skill-lint` (part of `make md-lint`, so it runs
  on the `pre-commit` hook) checks frontmatter, translation-pair heading structure, and the existence
  of every `make` target and path this directory's prose references. A skill is an instruction sheet —
  a reference that has rotted makes an agent execute the wrong step. Scope and the
  `<!-- skill-lint-ignore -->` directive are documented in [`scripts/README.md`](../scripts/README.md).
