---
name: manage-skill
description: >-
  Create, update, evaluate, and optimize skills under this repository's `.codex/skills/`, carrying this repo's conventions for them (English-canonical SKILL.md + mandatory `SKILL.ja.md` translation pair, dense "pushy" description frontmatter, AGENTS.md scope + hard-protected paths, eval artifacts kept out of version control). This is the single entry point for ANY change to an existing skill under `.codex/skills/`; ALWAYS use it before hand-editing a `SKILL.md` or `SKILL.ja.md`. Use this WHENEVER the user wants to update / modify / change / edit / fix / improve / refactor / rename / extend / adjust / tune an existing skill — its steps, `description`, frontmatter, or behavior — or to build a new skill, author/scaffold a `/<name>` command, turn a repeated workflow into a skill, tune a skill's triggering description, or run evals/benchmarks on a skill — even if they don't say the word "skill-creator". Japanese triggers also apply, e.g. 「スキルを更新したい」「スキルを修正して」「このスキルの手順 / description / 挙動を変えて」. Do NOT use it for editing canonical docs (`docs/**`, per-package `README.md` — those have `sync-readme` / `canonicalize-doc` / `back-prop`), other AI-tool configs (`.cursor/`, `.gemini/`, Copilot), or generated files.
argument-hint: '[skill-name] [--update|--new|--optimize]'
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, ask the user explicitly, Skill, Agent
---

# Manage Skill

You have been invoked via `/manage-skill`. Argument string: `$ARGUMENTS`.

This skill authors and maintains skills under `.codex/skills/` for **this** repository. It carries
both halves of the job: the methodology (draft → test → review → improve → optionally optimize the
description) and the repository's own conventions, so the produced skill fits in alongside `commit`,
`new-env`, `canonicalize-doc`, and the rest. An external methodology plugin, when one is installed,
supplements it — see Step 0; nothing here depends on that.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

## When to Use

Use this skill when the user wants to:

- Create a brand-new skill / `/<name>` command, or turn a repeated workflow from the current
  conversation into one.
- Update, modify, improve, refactor, rename, extend, or fix an existing skill under
  `.codex/skills/` — this is the entry point for any such change, used before hand-editing a
  <!-- doc-pair:replace-begin -->
  `SKILL.md` / `SKILL.ja.md`.
  <!-- doc-pair:replace-with -->
  <!-- = `SKILL.md`. -->
  <!-- doc-pair:replace-end -->
- Optimize a skill's `description` for better triggering, or run evals/benchmarks on a skill.

Do NOT use it for:

- Editing canonical docs (`docs/**`, per-package `README.md`) — those have their own flows
  (`sync-readme`, `canonicalize-doc`, `back-prop`).
- Other AI-tool configs (`.cursor/`, `.gemini/`, `.github/copilot-instructions.md`) — out of scope
  per `AGENTS.md`.

## Scope note (AGENTS.md)

`AGENTS.md` normally treats `.codex/**` as out of scope. **Invoking this skill is the explicit user
instruction that relaxes that** — but only for `.codex/skills/**`, and only for the duration of this
run. The hard-protected paths from `AGENTS.md` stay protected even here: never touch `AGENTS.md`,
generated files (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`), or generated
`docs/` content.

## Step 0 — Resolve optional external methodology (do this first)

Codex CLI can manage plugins through `codex plugin marketplace` and `codex plugin add`, but this
repository does not declare a marketplace or install `skill-creator` at project scope. Do not assume
that an external methodology is available, and do not run a bootstrap script.

Inspect the installed and available plugin entries first:

```bash
codex plugin list
```

If the output identifies an installed `skill-creator` plugin, read the `SKILL.md` from the exact path
reported in its `PATH` column and use its methodology as an optional supplement. Repository rules in
this skill remain authoritative on conflict.

If `skill-creator` is listed but not installed, ask the user before installing anything. After explicit
approval, install the exact plugin selector that `codex plugin list` reported:

```bash
codex plugin add <plugin@marketplace>
```

If no installed `skill-creator` is available, continue with this skill's self-contained workflow:
capture the intent, inspect comparable repository skills, write or revise the canonical skill, review
it against concrete invocation cases, and improve it before final validation. Do not add a marketplace
or install a plugin as an implicit prerequisite.

## The repository workflow

Apply the following repository rules whether or not an optional external methodology is available.

### 1. Placement and structure

- New skills live at `.codex/skills/<name>/SKILL.md`, `<name>` in kebab-case matching the `name:`
  frontmatter. Look at neighbors first (`commit`, `new-env`, `canonicalize-doc`, the `scaffold-*`
  family, the integrator `arch-check` / `back-prop`) and prefer editing/extending an existing skill
  over adding a near-duplicate.
- Bundled resources (`scripts/`, `references/`, `assets/`) follow the same layout the existing skills use.
  Keep `SKILL.md` under ~500 lines and push detail into `references/` with clear pointers.

### 2. Frontmatter conventions (match the existing skills)

- `name`: kebab-case, equal to the directory name.
- `description`: **one dense paragraph**, English, written to trigger reliably
  (state what it does AND concrete when-to-use contexts, plus explicit *when NOT* to trigger). Study
  the descriptions of `commit` / `new-env` for the density and tone this repo uses — match it.
- Optional: `argument-hint` and `allowed-tools` when the skill is a `/command` that takes args or runs
  a fixed tool set (see `tool-map` for the narrow per-command form). Omit them when not needed (most
  skills do). **Codex reads `allowed-tools` as prompt guidance, not as an enforced allowlist** — it
  documents which tools a skill expects to reach for, and restricts nothing. Never present it as a
  permission boundary, and never narrow it here on the theory that doing so contains the skill. The
  Claude side of this repository is the opposite: there the field is an auto-approval list, so a bare
  `Bash` grant really does widen what runs without a prompt.

### 3. Language rules (`AGENTS.md`)

- `SKILL.md` is **English canonical** — its body is written in imperative English, like every existing
  skill here. Do not write the canonical `SKILL.md` in Japanese.
- But the skill's *runtime behavior* must obey `AGENTS.md`: all user-visible output the skill produces
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

Evaluation writes a `<skill-name>-workspace/` with iteration/eval dirs, benchmarks, and viewer
output. A sibling of the skill directory would land inside the tracked `.codex/skills/**`.
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
- If the skill has a deterministic multi-step flow gated on user choices, use `ask the user explicitly` for the
  gates like `new-env` / `commit` do, rather than free-text prompts.
- Read the target layer's `README.md` / `docs/` **at runtime as the source of truth** (the `arch-*`,
  `test-review`, `scaffold-*` skills all do this) instead of hardcoding rules that will drift.

## Creating a new skill

Capture the intent, identify concrete invocation cases, write `SKILL.md`, and review the draft against
those cases. Then apply the repository workflow: correct placement, matching frontmatter/description
<!-- doc-pair:replace-begin -->
density, English-canonical body, and the eval workspace under `tmp/`. When the draft stabilizes,
generate `SKILL.ja.md` via `canonicalize-doc`.
<!-- doc-pair:replace-with -->
<!-- = density, English-canonical body, and the eval workspace under `tmp/`. -->
<!-- doc-pair:replace-end -->

## Updating an existing skill

- Confirm which skill (`.codex/skills/<name>/`). Preserve its `name` and directory unchanged.
- Unlike an *installed* plugin skill, repo skills are writeable in place — **edit them directly** under
  `.codex/skills/<name>/`; there is no read-only copy-to-`/tmp` dance.
- For a material behavior change, snapshot the pre-edit skill into `tmp/` and compare representative
  invocation cases before and after the update.
- After editing, re-sync `SKILL.ja.md` via `canonicalize-doc` — an updated `SKILL.md` with a stale
  Japanese pair is drift.

## Exercise it on an unknown real problem before completion

The author of an instruction sheet is uniquely unable to audit it as a reader. Because the author
already holds the context omitted from the page, they silently supply it while re-reading and cannot
see the gap itself. Re-reading the draft, even as part of a review, therefore does not expose the
class of failure this exercise is meant to find.

Choose **one real problem whose answer you do not know**, and follow the skill from beginning to end
without skipping any instruction. The unknown answer is essential: with a familiar problem, memory
can carry the run past a missing field without revealing that the contract is incomplete.

Record every point where the run either **needed to write information for which the contract provides
no field, or could not supply information that the contract requires**. Treat each such point as a
defect. Across five skills newly introduced in this repository, every defect revealed by this method
was an omitted part of the output contract, and none was found by reading alone. Examples included:

- a mandatory verdict with no field for its supporting evidence, forcing the run to invent one
- an `UNDEFINED` state with no rule for what an adjacent field should contain
- an execution mode that removes one analysis axis but provides nowhere to disclose the omission

If the same defect shape appears in several related skills, **standardize the field across the family
instead of applying separate patches**. A recurring gap indicates a missing convention; three
different remedies leave three contracts that no longer read as members of one family.

This exercise validates the skill contract itself, not the quality of the produced answer. The eval
workflow below measures output quality, so keep the two checks separate.

## Evaluating / optimizing

Evaluate the revised skill against representative invocation cases and review whether its description
triggers for intended requests without capturing excluded requests. Keep any comparison artifacts under
`tmp/` (§5). If an installed `skill-creator` provides a compatible evaluation or description-optimization
workflow, it may supplement this review.

## Definition of Done

- The self-contained workflow was followed: intent captured, representative invocation cases reviewed,
  and the draft improved before validation.
- `.codex/skills/<name>/SKILL.md` present, kebab `name` = dir, dense English "pushy" `description`.
- `SKILL.ja.md` generated/synced from the canonical `SKILL.md` and in sync.
- No eval artifacts committed (workspace under gitignored `tmp/`).
- No hard-protected path touched; only `.codex/skills/**` modified.
- The skill was exercised once on a real problem whose answer was unknown, and every place where the
  contract lacked a needed field or required an unfillable one was corrected.
- The user has reviewed outputs (viewer or inline) and is satisfied.
- For every new or materially changed skill that is not platform-only, invoke `sync-ai` after local
  validation with this environment as the source. Pass the transfer contract to the receiving
  environment's `manage-skill`. When this invocation is itself the receiving child operation, do
  not invoke `sync-ai` again.
- `make md-skill-lint` passes. It mechanically enforces the existence aspect of the preceding
  `sync-ai` requirement: a skill present in only one environment fails. To declare one deliberately
  platform-only, follow the [Skill Lint section](../../../scripts/README.md#skill-lint) of
  `scripts/README.md`.
