---
name: sync-ai
description: >-
  Reconcile a changed skill between this repository's Claude Code (`.codex/skills/`) and Codex (`.codex/skills/`) environments without blindly copying platform-specific instructions — one authoritative source direction is chosen, and the receiving environment's own `manage-skill` adapts the intent rather than having its skill directory written directly. Use after creating or materially updating a skill, and when asked to synchronize, migrate, port, or compare a skill across Claude and Codex. Do NOT use it for ordinary documentation translation, implementation code synchronization, or automatic two-way merges.
argument-hint: '[skill-name] [--from=claude|codex] [--to=claude|codex]'
---

# Sync AI Skills (Codex)

Synchronize a skill as a **one-way semantic port**, never as a raw directory copy. The source skill
is authoritative only for this run; the receiving skill remains native to its AI environment.

`manage-skill` invokes this after every new or material Codex skill change unless the skill is
explicitly Codex-only. For a Claude-originated change, use the same flow with Claude as the source.

## 1. Resolve direction and unit

Require exactly one skill name and one direction:

```text
source: .claude/skills/<name>/  → target: .codex/skills/<name>/
source: .codex/skills/<name>/   → target: .claude/skills/<name>/
```

Infer the source only when the user clearly names it. Do not infer direction from file mtimes.
If both copies changed, stop and ask which copy is authoritative; never merge two edited skills
automatically.

Inspect the complete source unit and the target unit if it exists:

```sh
git diff -- .claude/skills/<name> .codex/skills/<name>
find .claude/skills/<name> -type f | sort
find .codex/skills/<name> -type f | sort
```

<!-- doc-pair:replace-begin -->
Treat `SKILL.md`, `SKILL.ja.md`, UI metadata, scripts, references, and assets as one unit.
<!-- doc-pair:replace-with -->
<!-- = Treat `SKILL.md`, UI metadata, scripts, references, and assets as one unit. -->
<!-- doc-pair:replace-end -->
Record only a temporary transfer note under `tmp/skills/sync-ai/`; do not create a durable third copy or
a synchronization manifest.

## 2. Build the transfer contract

Extract the source skill's:

- purpose and explicit trigger/non-trigger cases;
- required inputs, approvals, side effects, and validation;
- reusable scripts, references, and assets;
- platform-specific mechanisms that must be translated rather than copied.

Classify every source item as **port**, **adapt**, or **omit**. Examples: Claude's
`AskUserQuestion` / `Agent` instructions adapt to Codex's user-input / delegation mechanisms;
Claude-only settings, hooks, and permission syntax are omitted unless Codex has an equivalent.
Never carry source-specific tool names into the target without verifying the equivalent exists.

Write the contract to `tmp/skills/sync-ai/<name>-contract.md`. It is handed to a **headless** receiver that
cannot ask you anything, so completeness is not a nicety here: anything the receiver would otherwise
raise a question about must be settled in the contract, or explicitly delegated to its judgement.
Resolve ambiguity with the user on this side, before the handoff.

## 3. Receive through the native skill workflow

The receiving environment's own `manage-skill` always performs the write. Neither side writes the
other's skill directory — a port that bypasses the receiver lands a file in the right place with the
wrong idioms, which is the failure this skill exists to prevent.

For a target under `.codex/skills/`, invoke `manage-skill` with the transfer contract. It must:

1. create or update the target skill in place;
2. preserve target-native frontmatter and `agents/openai.yaml`;
3. use the official skill-creator validation flow;
4. synchronize `SKILL.ja.md` from the English canonical `SKILL.md`.

For a target under `.claude/skills/`, hand the contract to Claude's `/manage-skill` by running
Claude headlessly:

```sh
sh .codex/skills/sync-ai/scripts/handoff-to-claude.sh tmp/skills/sync-ai/<name>-contract.md
```

The script exists so the invocation posture lives in one place rather than being rediscovered each
run. Claude treats `.claude/**` as a sensitive-file class, which `--permission-mode acceptEdits`
does not clear; `--permission-mode bypassPermissions` is therefore required. It is a deliberate,
per-invocation choice that leaves no standing permission widening, so the child-operation preamble
in the script provides the restraint. Read the script's header before changing the flag.

The script also carries the child-operation preamble — no questions, no new agent, writes confined
to `.claude/` and `tmp/` for the single named skill. That preamble is what stops a headless receiver
from stalling on a question nobody can answer.

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

Do not delete the source skill after a successful port. Do not commit, push, publish, or change
unrelated skills.

## 4. Verify and report

Run the receiving environment's structural validation, then inspect the target diff. Confirm that
the target retains the source's behavior while using only target-supported tools and configuration.
Report the direction, source commit/diff basis, port/adapt/omit decisions, files changed, and any
intent that cannot be represented on the target.

The receiver ran in a separate process, so its own summary is a claim rather than evidence — read
`git status` / `git diff` over the target directory and judge from that. A handoff that reports
success while writing nothing is the failure mode worth checking for first.

If the report asks for a synchronization in the other direction — the follow-up the preamble told
the receiver to report rather than run — **do not start it as part of this run.** Request user
confirmation first, because a return trip that the run initiates on its own behalf is, from the
inside, indistinguishable from the first turn of a loop. Note where this gate can exist at all: only
at the top of the chain, since every process below it is headless and has no user to ask. That
asymmetry is exactly why the lock above bounds recursion and this gate does not.

Check Claude's own checklist items against the filesystem too, not merely against its report. The
properties that fail quietly are the ones a diff does not show: a newly created file appears in
`git status` without its mode, so verify the executable bit on every bundled script with complete,
non-truncated output (prefer `stat -f '%Sp %N' <file>` on BSD/macOS; otherwise read `ls -l` in
<!-- doc-pair:replace-begin -->
full), run `sh -n` over it, and confirm the `SKILL.md` / `SKILL.ja.md` heading counts still match.
<!-- doc-pair:replace-with -->
<!-- = full) and run `sh -n` over it. -->
<!-- doc-pair:replace-end -->
Claude reporting these checks as passed is precisely the claim this paragraph exists to distrust.

## Guardrails

- The source-side `manage-skill` starts synchronization; the receiver-side `manage-skill` is a
  child operation and must not start a second outbound synchronization.
- Each side hands off to the other CLI headlessly through its own bundled script
  (`scripts/handoff-to-claude.sh` here, `scripts/handoff-to-codex.sh` on the Claude side). Keeping the
  posture in a script rather than in prose is what stops the two directions from drifting apart.
- A headless receiver has no user. Never hand it a contract that depends on a question being asked,
  and never give it flags that would let it wait for an approval nobody can grant.
- A platform-only skill remains platform-only; record the reason instead of forcing a weak port.
- Never use a bidirectional automatic sync, timestamp conflict resolution, or wholesale overwrite.
- Keep transfer artifacts under ignored `tmp/`; the two native skill directories are the only
  maintained copies.
