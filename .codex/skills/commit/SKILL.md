---
name: commit
description: Analyze working-tree changes, propose coherent Japanese Git commits, and create them after explicit approval. Use when the user asks to commit current changes or split a mixed change set. Enforce this repository's protected-branch, explicit-staging, commit-prefix, verification, and confirmation-gated push rules. Support `--dry-run` and `--scope=staged|all`.
---

# Scoped Git Commit

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

Parse `--dry-run` and `--scope=staged|all` (`all` is default). Never commit without first presenting the proposed grouping and receiving explicit approval.

## Preflight

1. Run `make gate-fix` once. It runs on every commit, while `fix` uses the same full-config lint as `lint`, so the load band defers it in `ci-first` and CI's lint reports formatting drift instead; a bare `make go-fix` still runs unconditionally. When the band defers it, producing no changes is by design and does not mean the tree was already formatted. Stop if it fails; include its resulting changes in the candidate set.
2. Inspect branch, `HEAD`, staged and unstaged status, diff summaries, and merge/cherry-pick/rebase state.
3. Stop without writing if the branch is `production`, `develop`, `staging`, or `release/*`; if there is no in-scope change; or if a Git operation is in progress.
4. When available, inspect the current branch's PR. If it is merged, recommend a fresh feature branch before committing from the current active release line resolved by `make base-branch` from `origin`'s live state—not the merged PR's `baseRefName`, which records the line where the old work merged and may now be one release behind, nor `gh repo view --json defaultBranchRef`, whose default branch can lag behind the active line. Stop and report rather than guessing if that base cannot be resolved. If `git switch -c` from `origin/<base>` establishes the protected base as the new branch's upstream, the eventual push must use the explicit refspec `git push -u origin <new-branch>`, never a bare `git push`. Do not switch branches without approval. If the PR is open, retain the branch; after verification, report that the commits are local and wait for explicit push confirmation.
5. Read `.lefthook.yaml` when present so the proposal can name the checks that will be run once after all split commits.

## Inspect and propose

Read both staged and unstaged diffs, unless `--scope=staged` was supplied. Treat generated output and `vendor/` as riders: they belong with the source change that caused them.

Use exactly one prefix per commit:

`Feat:`, `Fix:`, `Refactor:`, `Perf:`, `Docs:`, `Test:`, `Build:`, `CI:`, `Chore:`, `Style:`, or `Revert:`.

Create one commit per semantic change. Tests can accompany the implementation they validate; documentation is otherwise independent; unrelated formatting is a separate `Style:` commit. Propose the result in Japanese, with every file and a short rationale. State that each commit uses `--no-verify`, then that the complete pre-commit gate runs once after the final commit.

For `--dry-run`, provide the proposal only.

## Create approved commits

For each approved group:

1. Stage only the listed paths. Never use `git add .`, `git add -A`, or `git commit -a`.
2. Commit with `git commit --no-verify` and a Japanese message headed `<Prefix>: <title>`. Include the required `Co-Authored-By` footer for Codex. If the change applies a documented review finding, add its `Refs:` footer.
3. Do not amend, force-push, disable signing, or use destructive reset commands.

If one group fails, stop immediately. Report completed commits and the error. Offer the user a recoverable `git reset --mixed <original-head>`; never run it without approval and never use `--hard`.

## Verify and hand off

After every commit succeeds, run `lefthook run pre-commit --force` when available, then `make gate-fix`.

The hook sizes itself: `.makefiles/load.mk` decides from the number of open worktrees whether heavy Go gates run at full speed, throttled, or are deferred to CI. `make load-status` reports the current band; `repo-ops` section 19 explains it. Do not work around that decision by invoking `make go-lint` or `make go-test` directly to "really" verify: with several worktrees open, a full local lint costs minutes of saturated host to rediscover what CI re-runs identically. Report what the band did and let the push carry the rest.

1. Run `make -s load-status` and note the resolved band, so the later summary can state which verification actually happened: locally under `full` / `low`, or deferred to CI under `ci-first`.
2. Run `lefthook run pre-commit --force` when available, then `make gate-fix`.
3. If verification fails, report the failed command and stop; do not roll back commits.
4. If the final formatter changes files, show the diff and ask whether to create a follow-up commit.
5. Do not push automatically. For an existing PR branch, report that the commits are local, ask exactly 「変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？」, and stop for the user's answer. Never force-push, rebase, or push directly to a protected branch.

Report created commits, verification outcome, remaining changes, and that no push was performed. When the band was `ci-first`, state plainly which gates were deferred and that CI is what verifies them; do not say 「検証が通りました」 without that qualification.
