---
name: resolve-merge
description: >-
  Complete a requested branch merge by classifying every unmerged path, applying only the repository's existing mechanical resolution for its class, rebuilding generated and derived artifacts even after a conflict-free merge, and returning semantic conflicts to a human with their markers intact. Use whenever a merge reports conflicts, reports no conflicts but concatenated or derived outputs may be stale, or hand resolution makes untouched generated files fail gates. It takes the base from an existing PR's `baseRefName`, requires an explicit base for hotfix work, and merges rather than rebases. Do NOT use for semantic implementation-conflict decisions, rebase or squash operations, colliding-registry-key decisions, or synchronization of a branch nobody asked to sync.
argument-hint: '[--base=<ref>] [--class=<csv>] [--dry-run]'
---

# Resolve Merge

Complete a merge by routing each path to the mechanical resolver already owned by its class and
returning only semantic decisions to a human.

A Japanese reference translation is available at `SKILL.ja.md` in this directory. It is for human
reference and is not loaded as a skill.

## When to Use

- A requested merge has just reported conflicts.
- A requested merge completed without conflicts; concatenations and other derived outputs still
  require reconstruction.
- A hand-resolved merge fails a gate in an artifact nobody intentionally edited.

Do not use this skill to decide implementation semantics, rebase or squash history, choose between
duplicate registry definitions, or synchronize an unrequested branch.

## Contract

| | |
| --- | --- |
| **Owns** | Classifying conflicted paths and applying the class's existing mechanical process: regeneration, resolver rerun, set union, or derivation propagation |
| **Never** | Resolve implementation semantics; choose one side of generated output; rebase, squash, or force-push; choose a winner for a duplicate registry key |
| **Starts when** | Immediately after a requested merge, regardless of whether Git reported a conflict |
| **Stops when** | Any non-mechanical item remains: stop immediately, retain its markers, and neither commit nor offer to commit |

## Why this exists

Generated output does not have a correct merge side. Files such as `*.gen.go`, `*.sql.go`,
`*_mock.go`, `openapi.gen.yaml`, `database/gen/*.gen.sql`, generated documentation, pin lockfiles,
and version projections all have a source or resolver. Picking a side can remove conflict markers
while producing content that its source can no longer reproduce. A later `gen-*-artifacts-check` can
then fail on an unrelated change.

The skill is deliberately named for the **merge**, not the conflict. `database/gen/*.gen.sql`
concatenates DML inputs. Two branches can each add a different DML file, merge without a textual
conflict, and still leave the concatenation stale. The same risk exists for every committed
derivation, so zero conflicted paths never skips regeneration.

## Arguments

| Argument | Effect |
| --- | --- |
| `--base=<ref>` | Use exactly this base instead of resolving one. Mandatory when a hotfix line is involved |
| `--class=<csv>` | Process only the named classes; report every other class and leave it untouched |
| `--dry-run` | Inspect, classify, and report the intended operations without changing the tree |

## Step 1 — Read policy, resolve the base, and merge

Read `AGENTS.md` and the active Codex operational safeguards before acting. They govern Git behavior.
If either source conflicts with this procedure about what may be changed, stop and report both rules;
do not select the more permissive one.

Resolve the base in this order: an explicit `--base=<ref>`; the current pull request's `baseRefName`
from `gh pr view`; or, only when no pull request exists, `make -s base-branch`. Never derive it from
`refs/remotes/origin/HEAD` or `gh repo view --json defaultBranchRef`. Never replace an existing PR
base merely because a newer release line exists. If a hotfix is involved and `--base` was omitted,
stop and ask the human to rerun with it; do not infer the ref from branch names.

Unless the merge is already in progress, fetch and merge without a custom commit message:

```bash
git fetch origin "$BASE"
git merge "origin/${BASE}"
```

Use merge, never rebase. Preserve Git's generated merge subject and body.

## Step 2 — Classify every path before resolving any

Inspect unmerged paths with:

```bash
git diff --name-only --diff-filter=U
```

| Class | Paths | Mechanical resolution |
| --- | --- | --- |
| Generated Go / API | `**/*.gen.go`, `**/*.sql.go`, `**/*_mock.go`, `openapi/openapi.gen.yaml` | Remove the conflicted artifact and run the owning `make gen-api` or `make gen-query` generator |
| DML concatenation | `database/gen/*.gen.sql` | From the assigned worktree run `make merge-dml-ci work-dir=.` then `make sqlc-generate-ci`; do this even without a conflict |
| Generated docs | `docs/openapi/**`, `docs/godoc/**`, `docs/db-schema/**`, `docs/coverage/**`, `docs/portal/guides/**`, `docs/portal/docs.json` | Regenerate from source; generated docs stay out of a feature PR because the release-push workflow synchronizes them |
| Pin lockfile | `.github/actions-pin.toml`, `docker/images-pin.toml` | Rerun `make pin-actions-resolve` plus `make pin-actions-apply`, or `make pin-images-resolve` plus `make pin-images-apply`; never reconcile lockfile lines manually |
| Version projection | The `go` directive in `go.mod`, Dockerfile `FROM` values, and documented version strings | Resolve `mise.toml` first, then run `make sync-versions` |
| Vendored dependency | `vendor/**` | Reconstruct with `go mod vendor` |
| Append-only registry | Tables in `docs/adr/README.md`, `docs/spec/glossary.md`, `.agents/ddd-audit/pattern-ledger.yaml` | Union distinct entries; return any duplicate key to a human |
| Migration | `database/migrations/**` | Never edit an existing migration; for a sequence collision renumber only the newer side |
| Translation pair | `**/*.ja.md` | Resolve the English canonical file, then resynchronize through `canonicalize-doc`; never resolve the translation directly |
| Materialized environment | `env/.env` | Do not commit it; follow `repo-ops` section 7 |
| Implementation | Every unmatched path | Non-mechanical: retain its markers and return it to a human |

The fallback classification is always implementation. A false handoff costs one message; an invented
semantic resolution can silently land incorrect behavior.

References to sibling Codex skills, including `canonicalize-doc`, `repo-ops`, `commit`, and
`submit-pr`, mean their current `.codex/skills/<name>/SKILL.md`. Read the referenced file before use.
Those copies can lag behind another agent environment, so if a referenced procedure and current
repository policy disagree, report the mismatch and follow the higher-authority repository policy.

## Step 3 — Apply mechanical classes in dependency order

Resolve authored sources before their projections: `mise.toml` before `sync-versions`, migrations
before DML concatenation, DML before sqlc generation, and the OpenAPI source before API generation.

For a generated class, remove the artifact-level conflict and rebuild the entire artifact from its
resolved source. Do not merge generated lines or choose ours/theirs. For pin lockfiles, rerun the
resolver: a hand-combined tag-to-SHA cache can assert a mapping that never existed upstream.

For `--class`, leave every excluded class untouched and list it in the result. An excluded unresolved
class counts as remaining work in Step 6.

## Step 4 — Rebuild derivations even after a clean merge

Run the applicable reconstruction on every invocation, including when Step 2 returns no paths, and
state explicitly which commands ran. In a Codex-assigned worktree, use the CI-safe SQL commands:

```bash
make merge-dml-ci work-dir=.
make sqlc-generate-ci
make gen-api
make sync-versions
```

Do not run mutation commands under `--dry-run`; report that they would run. Generated documentation
is reconstructed only when its owning source changed or its class was involved, and remains excluded
from a feature PR according to repository policy.

## Step 5 — Verify proportionately

Run the deterministic, relevant checks and report the exact commands and outcomes:

```bash
make pin-actions-check
make pin-images-check
make check-migration-up-version
make check-migration-down-version
make check-migration-up-gap
make check-migration-down-gap
make md-doc-ref-lint
make md-skill-lint
```

Do not run `make go-lint`, `make go-test`, `make sql-lint`, or equivalents locally when the active Codex
CI-first safeguard applies, unless the user explicitly requested local verification. Record those
gates as deferred to CI; after an approved commit/push, use the resulting CI checks as their evidence.
Never claim an unrun gate passed.

Before declaring the tree mechanically clean, inspect again for unmerged entries and conflict
markers. Distinguish evidence (Git state and command output) from inference. Do not invent success for
a class whose resolver did not complete.

## Step 6 — End in exactly one of two states

The sole branch condition is whether any non-mechanical or excluded unresolved work remains.

### Anything remains — stop immediately

Leave the markers intact, do not commit, do not offer to commit, and do not later return to “finish”
after the human resolves it; their resolution makes the resulting commit theirs. Report in Japanese:

```text
## 機械的に統合した項目

<classes, paths, and commands>

## 人に返す項目

<path> — <実装の意味的衝突 / 重複キー / ADR 採番規則の不一致 / ベース未確定>

## 検証

<run, failed, and deferred gates>

機械的に解けるところまで統合しました。残りはここからお願いします。
```

Human-owned items include implementation semantics, the same registry key added on both sides, ADR
renumbering whose documented convention conflicts with practice, and any base that cannot be reduced
to one ref. A merge commit containing markers is a broken tree that misleadingly appears resolved.

### Nothing remains — ask before commit or push

First report the completed classes and gates under the Japanese headings `## 統合結果` and
`## 検証`. Then present numbered choices in the conversation and wait:

```text
質問: マージ元との統合が完了しました。コミットしてプッシュしますか？

1. コミットしてプッシュ
2. コミットのみ（プッシュしない）
3. 何もしない（作業ツリーのまま）
```

Never infer approval from merge success. Read `.codex/skills/commit/SKILL.md` before creating a
commit and use that workflow rather than staging or committing manually. Read
`.codex/skills/submit-pr/SKILL.md` before creating or updating a pull request. Because sibling skills
may be unsynchronized, confirm their current behavior can express the selected choice before invoking
them. If `commit` owns an automatic push for an open PR, option 2 cannot be implemented for that state:
report the policy mismatch and stop instead of bypassing the workflow.

For an existing PR, preserve the repository-required push confirmation exactly where the active
workflow permits a separate push gate:

> 変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？

Do not amend, rebase, squash, force-push, or manually work around a current skill's confirmation
contract.

## Do / Do NOT

- Do use the PR's `baseRefName`, and require `--base` for hotfix work.
- Do classify every path before changing any conflict and default unmatched paths to implementation.
- Do reconstruct generated output from source and rerun pin resolvers.
- Do rebuild concatenations and derivations when the merge had zero conflicts.
- Do identify every gate actually run and every gate deferred to CI.
- Do stop in exactly one of the two Step 6 states and report in Japanese.
- Do not hand-edit or side-pick generated output, lockfiles, or translations.
- Do not decide implementation semantics or duplicate registry definitions.
- Do not modify an existing migration or renumber the older side.
- Do not commit while markers remain or invite the user to do so.
- Do not push merely because the merge completed.
- Do not rebase, squash, force-push, or merge a line other than the requested branch's base.

## Checklist

- [ ] Policy read; base came from `--base` or the PR's `baseRefName`, with `make base-branch` used only when no PR exists.
- [ ] Merge used without a custom message; no rebase occurred.
- [ ] Every path was classified before resolution; unmatched paths became implementation.
- [ ] Generated artifacts were reconstructed from sources; pin data came from resolvers.
- [ ] Append-only registries were unioned and duplicate keys were returned undecided.
- [ ] English canonical content preceded translation synchronization.
- [ ] DML, sqlc, API, and version derivations were considered even with zero conflicts.
- [ ] Commands run, failures, and CI-deferred gates were reported accurately.
- [ ] The run ended in exactly one Step 6 state.
- [ ] Remaining work kept its markers and produced no commit or commit offer.
- [ ] A mechanically clean tree waited for numbered commit/push selection and used current Codex workflows.
