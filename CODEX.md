# Codex CLI Operational Safeguards

This file applies only to Codex CLI. It supplements `AGENTS.md`; shared repository rules remain
authoritative unless this file states a Codex-specific execution detail.

## Git operations

- When synchronizing a feature branch with its PR base, run `git merge origin/<pr-base>` without
  supplying a commit message. Preserve Git's generated merge subject and body; do not rename it
  to a conventional-commit-style subject such as `Merge: ...`.
- Do not amend, rebase, squash, or force-push unless the user explicitly requests that operation.
- Use this repository's `/commit` workflow for every commit. Do not stage, commit, or push
  manually; `/commit` owns change grouping, verification, and an ordinary push for an open PR.
- Use this repository's `/submit-pr` workflow to create or update a pull request. It owns the
  PR template, review decision, and required push confirmation.

## Scope and confirmation

- A user request to implement, change, fix, or configure this repository authorizes every
  non-destructive edit required anywhere inside the repository. Do not ask whether an individual
  file may be changed.
- Treat an implementation request as active until its stated acceptance criteria are met. Do not
  present partial work as complete, ask whether to continue after an ordinary implementation step,
  or end a turn merely because one check is pending. Continue through implementation, verification,
  and remediation; report a genuine blocker only when a human decision, new authority, or an
  external-state change is required.
- Do not ask for confirmation of normal implementation commands, including `git add`, `git
  commit`, and an ordinary `git push` to the current feature branch. Check the worktree to avoid
  including unrelated changes, but perform that check without pausing for confirmation.
- Do not broaden the task: configuration changes for Codex belong under `.codex/` or this file.
  Do not modify `AGENTS.md` for Codex-only behavior unless the user explicitly asks.
- Before changing behavior outside the user's stated scope, report the consequence and ask for
  direction.

## Execution and approval policy

- Act autonomously inside this repository and an assigned Git worktree. A prior `go`, `proceed`,
  `fix`, or implementation request authorizes ordinary reversible work required to complete it.
- Do not ask for confirmation for repository-local inspection, edits, file moves/removals owned by
  the task, generation, formatting, compilation, tests, lint, validation, Git inspection,
  synchronization, or GitHub metadata reads related to a user-referenced repository, issue, PR,
  or branch.
- Treat an explicitly assigned worktree as trusted task workspace even when its path differs from
  the primary checkout. Do not stop merely because of that path difference.
- Continue until acceptance criteria and validation succeed. Routine implementation steps are not
  synchronization points. Ask only for a material, non-derivable design/security decision, a
  repository-boundary crossing, sensitive-data access, a new dependency, unusually costly work,
  external persistent state, or a destructive/shared-state operation.
- Evaluate risk by effect: repository/worktree boundary, reversibility, shared mutation, external
  side effect, sensitive-data exposure, resource or deletion blast radius, and architectural or
  user-visible scope. Repository-local, reversible, task-scoped work proceeds without approval.
- For a real escalation, ask once at the highest meaningful level, explain the risk or unresolved
  decision, and group directly related operations. Never ask solely because a routine command is
  needed for an already-approved task.
- Never force-push, delete remote refs, use `git branch -D`, `git reset --hard`, `git clean -fd*`,
  destroy shared stashes, or perform destructive DB/filesystem operations without explicit
  authorization or a governing repository policy. Do not modify another agent's worktree or branch
  unless the task explicitly requires it.

<!-- boilerplate-only:begin -->
## CI-first validation

- Unless the user explicitly asks for local verification, run test and lint workloads in CI. Do
  not consume the host machine's resources by running `make go-test`, `make go-lint`, `make sql-lint`,
  or their equivalent local commands; push the scoped change and use the resulting CI checks as
  the validation record.

## Generated SQL artifacts

- Generate DML and sqlc artifacts from the assigned Git worktree, never from the primary checkout.
  From the worktree root, run `make merge-dml-ci work-dir=.` followed by `make sqlc-generate-ci`.
  Passing `work-dir=.` is required: the CI target otherwise defaults to `/app`, which exists in
  the tool-runner container but not on the host and makes the merge command fail before it can
  read `database/dml`.

<!-- boilerplate-only:end -->

## Worktree and release synchronization

- Before investigating or changing an implementation request, fetch `origin`, resolve the latest
  numeric `release/*` branch, and update the local release branch to that remote state. Treat this
  as the work's required starting point, not as a later synchronization step.
- Never implement directly on a release branch. Create the task feature branch from the updated
  release branch.
- Perform every implementation task in a Git worktree beneath `.codex/worktrees/`. Do not use the
  primary checkout as the implementation location. When a task already has an assigned worktree,
  continue there rather than creating or modifying another checkout.
