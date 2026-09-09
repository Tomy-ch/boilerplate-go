---
name: manage-skill
description: >-
  Create, update, evaluate, and optimize skills under this repository's `.claude/skills/`, wrapping Anthropic's official `skill-creator` methodology and layering this repo's conventions on top. This is the single entry point for ANY change to an existing skill under `.claude/skills/`; ALWAYS use it before hand-editing a `SKILL.md` or `SKILL.ja.md`. Use this WHENEVER the user wants to update / modify / change / edit / fix / improve / refactor / rename / extend / adjust / tune an existing skill — its steps, `description`, frontmatter, or behavior — or to build a new skill, author/scaffold a `/<name>` command, turn a repeated workflow into a skill, tune a skill's triggering description, compress descriptions that have grown into restatements of the body, normalize ones that have gone stale against it, or run evals/benchmarks on a skill — even if they don't say the word "skill-creator". Japanese triggers also apply, e.g. 「スキルを更新したい」「スキルを修正して」「このスキルの手順 / description / 挙動を変えて」「description が長い / 古い」. Do NOT use it for editing canonical docs (`docs/**`, per-package `README.md` — those have `sync-readme` / `canonicalize-doc` / `back-prop`), other AI-tool configs (`.cursor/`, `.gemini/`, Copilot), or generated files.
argument-hint: '[skill-name] [--update|--new|--optimize]'
allowed-tools: Bash(ls:*), Bash(bash .claude/scripts/bootstrap-plugins.sh:*), Bash(make md-skill-lint:*), Read, Write, Edit, Glob, Grep, AskUserQuestion, Skill, Agent
---

# Manage Skill

You have been invoked via `/manage-skill`. Argument string: `$ARGUMENTS`.

This skill authors and maintains skills under `.claude/skills/` for **this** repository. It is a
thin wrapper: the *methodology* (draft → test → review → improve → optionally optimize the
description) comes from Anthropic's official `skill-creator` skill, and this file layers the
repository's own conventions on top so the produced skill fits in alongside `commit`, `new-env`,
`canonicalize-doc`, and the rest.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

## When to Use

Use this skill when the user wants to:

- Create a brand-new skill / `/<name>` command, or turn a repeated workflow from the current
  conversation into one.
- Update, modify, improve, refactor, rename, extend, or fix an existing skill under
  `.claude/skills/` — this is the entry point for any such change, used before hand-editing a
  <!-- doc-pair:replace-begin -->
  `SKILL.md` / `SKILL.ja.md`.
  <!-- doc-pair:replace-with -->
  <!-- = `SKILL.md`. -->
  <!-- doc-pair:replace-end -->
- Optimize a skill's `description` for better triggering, compress one that has grown into a
  restatement of the body, or normalize one that has gone stale against it.
- Run evals/benchmarks on a skill.

Do NOT use it for:

- Editing canonical docs (`docs/**`, per-package `README.md`) — those have their own flows
  (`sync-readme`, `canonicalize-doc`, `back-prop`).
- Other AI-tool configs (`.cursor/`, `.gemini/`, `.github/copilot-instructions.md`) — out of scope
  per `AGENTS.md`.

## Scope note (AGENTS.md)

`AGENTS.md` normally treats `.claude/**` as out of scope. **Invoking this skill is the explicit user
instruction that relaxes that** — but only for `.claude/skills/**`, and only for the duration of this
run. The hard-protected paths from `AGENTS.md` stay protected even here: never touch `AGENTS.md`,
generated files (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`), generated `docs/`
content, or anything under `permissions.deny`.

## Step 0 — Ensure and load the official methodology (do this first)

The official `skill-creator` is the source of truth for the *how*. Operate on the assumption that it
is present: it is declared at **project scope** in this repo's `.claude/settings.json`, so a trusted
clone already has it. If it is missing, ensure it via the repo-level plugin bootstrap:

```bash
bash .claude/scripts/bootstrap-plugins.sh
```

The bootstrap declares the `claude-plugins-official` marketplace and enables the official plugins this
repo depends on (`skill-creator`, `feature-dev`) at project scope. It is idempotent; re-running is a
no-op. Newly enabled plugins load on the next session, when `skill-creator` also becomes invocable as
`/skill-creator` — but this wrapper does not depend on that; it reads the file by path so it works in
the same session.

Read that `SKILL.md` in full (the path the script prints; discover it with the glob below if needed):

```bash
ls ~/.claude/plugins/marketplaces/*/plugins/skill-creator/skills/skill-creator/SKILL.md
```

Everything it says about **Creating a skill**, **Running and evaluating test cases**, **Improving the
skill**, **Description Optimization**, blind comparison, and packaging applies verbatim. Its bundled
resources live next to it and are used as-is:

- `scripts/` — `aggregate_benchmark`, `run_loop` (description optimizer), `package_skill`, etc. Run
  them from the official skill's directory; do not reimplement them.
- `eval-viewer/generate_review.py` — the review viewer. Always use it; never hand-write review HTML.
- `agents/` (`grader.md`, `comparator.md`, `analyzer.md`) and `references/schemas.md`.

If the bootstrap fails (e.g. no network, `claude` CLI missing), report the failure to the user and ask
whether to proceed with the methodology summarized inline (draft → test → review → improve) instead of
aborting.

## The repository overlay (this is what the wrapper adds)

Follow the official process, but apply these repo-specific rules wherever they differ. On conflict,
these win, because a skill that ignores them won't fit this repo.

### 1. Placement and structure

- New skills live at `.claude/skills/<name>/SKILL.md`, `<name>` in kebab-case matching the `name:`
  frontmatter. Look at neighbors first (`commit`, `new-env`, `canonicalize-doc`, the `scaffold-*`
  family, the integrator `arch-check` / `back-prop`) and prefer editing/extending an existing skill
  over adding a near-duplicate.
- Bundled resources (`scripts/`, `references/`, `assets/`) follow the official anatomy when needed.
  Keep `SKILL.md` under ~500 lines and push detail into `references/` with clear pointers.

### 2. Frontmatter conventions (match the existing skills)

- `name`: kebab-case, equal to the directory name.
- `description`: **one dense paragraph**, English, following the official "pushy" triggering guidance
  (state what it does AND concrete when-to-use contexts, plus explicit *when NOT* to trigger). Study
  the descriptions of `commit` / `new-env` for the density and tone this repo uses — match it.
- **The admission test for a `description` is invocation, not completeness.** Every skill's
  `description` is loaded at the start of every session whether or not that skill is ever called, so
  a line earns its place only by helping decide *whether to call this skill*. Admit five things:
  what it does, when to use it (Japanese trigger phrases included), when NOT to and which sibling
  owns that subject instead, the prerequisites a caller must already have met, and the flags a user
  would name when asking for it. Everything else belongs in the body, which costs nothing until the
  skill actually runs — internal mechanism (subagent names, model defaults, fan-out shape, pipeline
  stages), the rules the skill enforces, the commands it verifies with, its output format, and the
  reasoning behind any of those. "Dense" is about the trigger surface, not about restating the body.
- **Before deleting a line from a `description`, confirm the body already carries it.** The two
  drift apart precisely because a fact gets written to only one of them; a delete that is really a
  move is a silent loss of instruction.
- **Normalize a rotted `description` whenever you touch a skill** — it is the one part of a skill
  that is never read while the skill runs, so nothing fails when it lies and no gate catches it.
  Check every claim in it against the body and against the siblings it names: a chaining
  relationship that no longer exists, a lens / mode / flag that was renamed or removed, a subject
  another skill has since taken over, a path that moved. Correct those in the same edit rather than
  filing them.
- Both of the above apply as a **sweep** too: run them across every `.claude/skills/*/SKILL.md` when
  descriptions have accumulated, not only on the skill in front of you. The saving is per session
  and repo-wide, and the criterion is the same one, applied file by file.
- Optional: `argument-hint` and `allowed-tools` when the skill is a `/command` that takes args or runs
  a fixed tool set (see `commit` for the pattern). Omit them when not needed (most skills do).

### 3. Language rules (`CLAUDE.md`)

- `SKILL.md` is **English canonical** — its body is written in imperative English, like every existing
  skill here. Do not write the canonical `SKILL.md` in Japanese.
- But the skill's *runtime behavior* must obey `CLAUDE.md`: all user-visible output the skill produces
  (responses, generated code comments, test case names, PR/commit text) must be **Japanese**. When
  authoring a skill, bake that requirement into its instructions.

<!-- doc-pair:begin -->
### 4. Mandatory Japanese translation pair

Every skill in this repo ships a `SKILL.ja.md` reference translation next to `SKILL.md`. This is not
optional here. After the canonical `SKILL.md` is finalized (create) or changed (update):

- Chain the `canonicalize-doc` skill to produce/sync `SKILL.ja.md` from the canonical `SKILL.md`.
- The translation carries the standard sync-note header and is not itself loaded as a skill. Confirm
  the pair is in sync before considering the task done.

<!-- doc-pair:end -->
<!-- doc-pair:replace-begin -->
### 5. Eval artifacts stay out of version control
<!-- doc-pair:replace-with -->
<!-- = ### 4. Eval artifacts stay out of version control -->
<!-- doc-pair:replace-end -->

The official process writes a `<skill-name>-workspace/` with iteration/eval dirs, benchmarks, and
viewer output. A sibling of the skill directory would land inside the tracked `.claude/skills/**`.
**Override the location**: put the workspace under the repo's gitignored `tmp/` (e.g.
`tmp/skills/manage-skill/<skill-name>-workspace/`), consistent with this repo's work-artifact convention
(plans/artifacts live outside git; `tmp/` is ignored). Never commit eval runs, benchmarks, feedback
JSON, or viewer HTML.

<!-- doc-pair:replace-begin -->
### 6. Reuse repo patterns when they fit the skill's shape
<!-- doc-pair:replace-with -->
<!-- = ### 5. Reuse repo patterns when they fit the skill's shape -->
<!-- doc-pair:replace-end -->

- If the new skill fans out read-only analysis across layers, mirror the **integrator + per-layer
  subagent** pattern (`arch-check` / `back-prop` spawn `*-auditor-*` / `*-detector-*` agents in
  parallel; the integrator does the single-threaded writes). Reuse existing subagent types rather than
  inventing new ones where possible.
- If the skill has a deterministic multi-step flow gated on user choices, use `AskUserQuestion` for the
  gates like `new-env` / `commit` do, rather than free-text prompts.
- Read the target layer's `README.md` / `docs/` **at runtime as the source of truth** (the `arch-*`,
  `test-review`, `scaffold-*` skills all do this) instead of hardcoding rules that will drift.

## Creating a new skill

Run the official **Creating a skill** flow (Capture Intent → Interview → Write SKILL.md → Test Cases),
<!-- doc-pair:replace-begin -->
then apply the overlay: correct placement, matching frontmatter/description density, English-canonical
body, and the eval workspace under `tmp/`. When the draft stabilizes, generate `SKILL.ja.md` via
`canonicalize-doc`.
<!-- doc-pair:replace-with -->
<!-- = then apply the overlay: correct placement, matching frontmatter/description density, English-canonical -->
<!-- = body, and the eval workspace under `tmp/`. -->
<!-- doc-pair:replace-end -->

## Updating an existing skill

- Confirm which skill (`.claude/skills/<name>/`). Preserve its `name` and directory unchanged.
- Unlike an *installed* plugin skill, repo skills are writeable in place — **edit them directly** under
  `.claude/skills/<name>/`; there is no read-only copy-to-`/tmp` dance.
- For the eval baseline, snapshot the pre-edit skill per the official guidance (into `tmp/`), so
  before/after can be compared.
- After editing, re-sync `SKILL.ja.md` via `canonicalize-doc` — an updated `SKILL.md` with a stale
  Japanese pair is drift.

## Run it once on a real problem before calling it done

A skill is an instruction sheet, and its author is the one reader who cannot audit it: you hold the
context the sheet leaves out, so you fill the gaps in your head as you read and never see them. That
is why re-reading a draft — or having it reviewed — does not surface the failure this step catches.

Take **one real problem whose answer you do not already know**, and run the skill through without
skipping steps. The unknown answer is what makes it work: on a problem you know, you complete the run
from memory and the missing field never announces itself.

Then record every place where you **wrote something the contract has no field for, or could not write
something it demands**. Those are the defects. Measured on the five skills added in this repository,
every gap found this way was a hole in the output contract, and none had been visible while reading:

- a required verdict whose supporting evidence had nowhere to go, so the run invented a field for it
- a state (`UNDEFINED`) with no rule for what the neighbouring field should then say
- a run mode that removes an axis from the analysis, with no place to say the axis was removed

When the same shape of hole appears in more than one skill, **align the field across them rather than
patching each one**. A gap that recurs is a missing convention, and fixing it three different ways
produces three contracts that no longer read as one family.

This is verification of the skill, not of its output quality — that is what the official eval loop
below measures, and the two answer different questions.

## Evaluating / optimizing

Use the official test-run, benchmark, viewer, and **Description Optimization** flows unchanged, with
two adaptations: workspaces go under `tmp/` (§5), and for the description optimizer pass the model ID
powering this session (see the environment/system prompt) so triggering matches production.

## Definition of Done

- The official `skill-creator` resolved (project-scope plugin; ensured via the plugin bootstrap if
  missing) and its methodology is loaded.
- `.claude/skills/<name>/SKILL.md` present, kebab `name` = dir, dense English "pushy" `description`.
- The `description` passes the admission test in §2: nothing in it that fails to help decide whether
  to invoke, no claim the body no longer supports, and nothing deleted from it that the body does
  not already carry.
- `SKILL.ja.md` generated/synced from the canonical `SKILL.md` and in sync.
- No eval artifacts committed (workspace under gitignored `tmp/`).
- No hard-protected path touched; only `.claude/skills/**` modified.
- The skill was run once on a real problem whose answer was not already known, and every place
  where the contract had no field — or demanded one that could not be filled — was fixed.
- The user has reviewed outputs (viewer or inline) and is satisfied, per the official loop.
- For every new or materially changed skill that is not platform-only, invoke `sync-ai` after local
  validation with this environment as the source. Pass the transfer contract to the receiving
  environment's `manage-skill`. When this invocation is itself the receiving child operation, do
  not invoke `sync-ai` again.
- `make md-skill-lint` passes. It enforces the previous item's existence half — a skill present in
  only one environment fails. To declare one deliberately platform-only, follow the Skill Lint
  section of `scripts/README.md`.
