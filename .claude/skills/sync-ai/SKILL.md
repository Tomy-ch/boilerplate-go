---
name: sync-ai
description: >-
  Reconcile a changed skill between this repository's Claude Code (`.claude/skills/`) and Codex (`.codex/skills/`) environments without blindly copying platform-specific instructions — one authoritative source direction is chosen, and the receiving environment's own `manage-skill` adapts the intent rather than having its skill directory written directly. Use after creating or materially updating a skill, and when asked to synchronize, migrate, port, or compare a skill across Claude and Codex. Do NOT use it for ordinary documentation translation, implementation code synchronization, or automatic two-way merges.
argument-hint: '[skill-name] [--from=claude|codex] [--to=claude|codex]'
allowed-tools: Bash(ls:*), Bash(git status:*), Bash(git diff:*), Bash(sh -n:*), Bash(sh .claude/skills/sync-ai/scripts/handoff-to-codex.sh:*), Read, Write, Edit, Glob, Grep, AskUserQuestion, Skill
---

# Sync AI Skills (Claude)

Synchronize a skill as a **one-way semantic port**, never as a raw directory copy. The source skill
is authoritative only for this run; the receiving skill remains native to its AI environment.

`manage-skill` invokes this after every new or material Claude skill change unless the skill is
explicitly Claude-only. For a Codex-originated change, use the same flow with Codex as the source.

## 1. Resolve direction and unit

Require exactly one skill name and one direction:

```text
source: .claude/skills/<name>/  → target: .codex/skills/<name>/
source: .codex/skills/<name>/   → target: .claude/skills/<name>/
```

Infer the source only when the user clearly names it. Do not infer direction from file mtimes.
If both copies changed, stop and ask which copy is authoritative; never merge two edited skills
automatically.

<!-- doc-pair:replace-begin -->
Inspect the complete source unit and the target unit if it exists. Treat `SKILL.md`,
`SKILL.ja.md`, UI metadata, scripts, references, and assets as one unit. Record only a temporary
transfer note under `tmp/skills/sync-ai/`; do not create a durable third copy or a synchronization manifest.
<!-- doc-pair:replace-with -->
<!-- = Inspect the complete source unit and the target unit if it exists. Treat `SKILL.md`, UI metadata, -->
<!-- = scripts, references, and assets as one unit. Record only a temporary transfer note under -->
<!-- = `tmp/skills/sync-ai/`; do not create a durable third copy or a synchronization manifest. -->
<!-- doc-pair:replace-end -->

## 2. Build the transfer contract

Extract purpose, trigger and non-trigger cases, inputs, approvals, side effects, validation, and
reusable resources. Classify each item as **port**, **adapt**, or **omit**. Translate platform
mechanisms instead of copying them: Codex delegation and tool instructions must not become Claude
syntax, and Claude's `AskUserQuestion` / `Agent` instructions must not become Codex syntax unchanged.

Write the contract to `tmp/skills/sync-ai/<name>-contract.md`. It is handed to a **headless** receiver that
cannot ask you anything, so completeness is not a nicety here: anything the receiver would otherwise
raise a question about must be settled in the contract, or explicitly delegated to its judgement.
Resolve ambiguity with the user on this side, before the handoff.

## 3. Receive through the native skill workflow

The receiving environment's own `manage-skill` always performs the write. Neither side writes the
other's skill directory — a port that bypasses the receiver lands a file in the right place with the
wrong idioms, which is the failure this skill exists to prevent.

For a target under `.claude/skills/`, invoke `/manage-skill` with the transfer contract. It creates
or updates the target in place, preserves Claude-native metadata, applies the skill-creator flow,
and synchronizes `SKILL.ja.md` from canonical English `SKILL.md`.

For a target under `.codex/skills/`, hand the contract to Codex's `manage-skill` by running Codex
headlessly:

```sh
sh .claude/skills/sync-ai/scripts/handoff-to-codex.sh tmp/skills/sync-ai/<name>-contract.md
```

The script exists so the invocation posture lives in one place rather than being rediscovered each
run. It pins `codex exec --sandbox workspace-write` plus one narrow config override that re-adds
`<repo>/.codex` to the writable roots: Codex excludes its own configuration directory by default, so
without it the receiver cannot write the very skill it was asked to write and fails with
`patch rejected: writing outside of the project`. The override adds that one path and does not
replace the sandbox. Approval policy is already `never` under `codex exec`, which is why widening
the roots is the fix and loosening approvals is not. Read the script's header before changing a flag.

The script also carries the child-operation preamble — no questions, no starting another agent,
writes confined to `.codex/` and `tmp/` for the single named skill. That preamble is what stops a
headless receiver from stalling on a question nobody can answer.

**The recursion bound is the lock, not the preamble.** Both handoff scripts take the same lease
(`tmp/skills/sync-ai/.handoff.lock`) and refuse to start when it is already held, so a receiver that runs
either script is turned away rather than deepening the chain. This is deliberately not left to the
preamble: an instruction can be ignored, and a chain that ignores it loops while spending real money
on both agents. A refusal (exit 3) means one of exactly two things, and the message says which — a
chain tried to recurse, which is the guard doing its job, or a previous run was killed and left the
lease behind, which you clear by removing the path it prints. It is a lock file rather than an
exported variable because Codex filters the environment it forwards to model-run commands, so a
marker may not survive the hop; the working tree is the one channel both agents certainly share.
Same shape as this repo's worktree slot leases — a lease plus a stale TTL.

Do not delete the source skill, commit, push, publish, or change unrelated skills.

## 4. Verify and report

Run the receiving environment's structural validation and inspect the target diff. Confirm that the
target preserves the source behavior while using only target-supported tools and configuration.
Report the direction, source commit/diff basis, port/adapt/omit decisions, files changed, and any
intent the target cannot express.

The receiver ran in a separate process, so its own summary is a claim rather than evidence — read
`git status` / `git diff` over the target directory and judge from that. A handoff that reports
success while writing nothing is the failure mode worth checking for first.

If the report asks for a synchronization in the other direction — the follow-up the preamble told
the receiver to report rather than run — **do not start it as part of this run.** Confirm with the
user first (`AskUserQuestion`), because a return trip that the run initiates on its own behalf is,
from the inside, indistinguishable from the first turn of a loop. Note where this gate can exist at
all: only at the top of the chain, since every process below it is headless and has no user to ask.
That asymmetry is exactly why the lock above bounds recursion and this gate does not.

Check the receiver's own checklist items against the filesystem too, not just against its prose.
The properties that go wrong quietly are the ones a diff does not surface: a newly created file
appears in `git status` without its mode, so verify the executable bit on any bundled script
<!-- doc-pair:replace-begin -->
(`ls -l`), run `sh -n` over it, and confirm the `SKILL.md` / `SKILL.ja.md` heading counts still
match. A receiver reporting these as passed is exactly the claim this paragraph exists to distrust.
<!-- doc-pair:replace-with -->
<!-- = (`ls -l`) and run `sh -n` over it. A receiver reporting these as passed is exactly the claim this -->
<!-- = paragraph exists to distrust. -->
<!-- doc-pair:replace-end -->

## Guardrails

- The source-side `manage-skill` starts synchronization; the receiver-side `manage-skill` is a
  child operation and must not start a second outbound synchronization.
- Each side hands off to the other CLI headlessly through its own bundled script
  (`scripts/handoff-to-codex.sh` here, `scripts/handoff-to-claude.sh` on the Codex side). Keeping the <!-- skill-lint-ignore -->
  posture in a script rather than in prose is what stops the two directions from drifting apart.
- A headless receiver has no user. Never hand it a contract that depends on a question being asked,
  and never give it flags that would let it wait for an approval nobody can grant.
- Keep platform-only skills platform-only and report why.
- Never use bidirectional automatic sync, timestamp conflict resolution, or wholesale overwrite.
- Keep transfer artifacts under ignored `tmp/`; maintain only the two native skill directories.
