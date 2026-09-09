---
name: resolve-merge
description: >-
  Land a merge correctly by classifying every conflicted path into a resolution class and applying the mechanical resolution each class already has — regenerate generated artifacts from their source of truth rather than picking a side, re-resolve pin lockfiles through their resolver, union append-only registries, propagate version derivations from `mise.toml` — then hand back only what genuinely needs a human. Use it whenever a merge reports conflicts, and equally whenever one reports none: several committed artifacts here are concatenations or derivations that drift on a clean merge, so "no conflict" is not the same as "nothing to do", which is why this skill is named for the merge and not for the conflicts. Most conflict markers in this repository sit in files nobody should be hand-editing at all — `*.gen.go`, `*.sql.go`, `*_mock.go`, `openapi.gen.yaml`, `database/gen/*.gen.sql`, the docs trees, the pin lockfiles, `go.mod`'s version lines — and choosing a side in one of those produces a file that looks resolved, passes review, and no longer reproduces from its generator. Resolves the base from the pull request's own `baseRefName` (never a stale local symref or the GitHub default) and merges rather than rebases, per `CLAUDE.md`. Verifies with the pin checks, migration numbering checks, doc-ref and skill lint, `make go-lint` and `make go-test`. Read `CLAUDE.md`'s Git Rules at runtime; hardcodes no policy of its own. It ends in exactly one of two states: anything non-mechanical left means it stops there with the markers intact, commits nothing and offers nothing, because a merge commit carrying conflict markers is a broken tree that reads as resolved; a fully clean integration means it asks whether to commit and push rather than assuming, since a branch can take a release line in without that merge being ready to leave the machine. Do NOT use it to resolve a semantic conflict in implementation code — that is a human's judgment and it hands those back untouched — to rebase or squash anything, to decide which of two colliding registry keys wins, or to sync a branch nobody asked to sync.
argument-hint: '[--base=<ref>] [--class=<csv>] [--dry-run]'
---

# Resolve Merge

Land a merge by routing each conflicted path to the resolution its class already has, and handing
back the rest.

A Japanese reference translation lives at `SKILL.ja.md` in this directory (for human reference only;
not loaded as a skill).

## When to Use

- A merge reported conflicts.
- A merge reported **no** conflicts — the derived and concatenated artifacts still need regenerating.
- A merge was resolved by hand and the gates are now failing in files nobody edited.

Do NOT use it to resolve a semantic conflict in implementation code, to rebase or squash anything, or
to sync a branch nobody asked to sync.

## Contract

| | |
| --- | --- |
| **Owns** | 衝突パスのクラス分けと、クラスごとの機械的解決（再生成 / resolver 再実行 / 和集合 / 派生の再伝播） |
| **Never** | 実装の意味的衝突を解く / 生成物の片側を選ぶ / rebase・squash・force-push / レジストリの鍵衝突をどちらか採用する |
| **Starts when** | merge を実行した直後（衝突の有無を問わず） |
| **Stops when** | 機械的に解けないものが 1 つでも残ったとき — その場でマーカーを残して打ち切り、コミットしない |

## Why this exists

**Most conflict markers in this repository land in files nobody should be editing.** `*.gen.go`,
`*.sql.go`, `*_mock.go`, `openapi.gen.yaml`, `database/gen/*.gen.sql`, the `docs/` generated trees,
the pin lockfiles, the version lines in `go.mod` and the Dockerfiles — every one of them has a
generator or a resolver, and none of them has a correct "side".

Picking a side there produces the worst available outcome: a file with no markers, that reviews
clean, and that no longer reproduces from its source. The `gen-*-artifacts-check` workflows catch it
later, on someone else's pull request, as a failure in code they never touched.

The second reason is quieter. **A clean merge is not a finished merge.** `database/gen/*.gen.sql` is a
concatenation of the DML files; two branches can each add a file, conflict on nothing, and leave the
concatenation stale. The same holds for anything derived rather than authored. A skill named for
conflicts would be skipped in exactly that case, which is why this one is named for the merge.

## Arguments

| Argument | Effect |
| --- | --- |
| `--base=<ref>` | Use this base instead of resolving one. Required during a hotfix (Step 1) |
| `--class=<csv>` | Restrict to the named classes; the rest are reported and left untouched |
| `--dry-run` | Classify and report the plan; change nothing |

## Step 1 — Resolve the base, and merge

`CLAUDE.md`'s Git Rules govern this and are read at runtime. Two of them decide correctness here:

- **A pull request's `baseRefName` wins.** It is what the branch is already merging into. Only with
  no PR does `make base-branch` resolve the active release line from `origin`'s live state. Never
  take the base from `refs/remotes/origin/HEAD` or `gh repo view --json defaultBranchRef` — both
  answer with an earlier line without warning.
- **Merge, never rebase.** Beyond `CLAUDE.md`'s rule, a rebase here actively breaks append-only
  files: the same content re-lands under a different hash on both sides and reads as two independent
  additions.

```bash
BASE=$(gh pr view --json baseRefName -q '.baseRefName' 2>/dev/null || make -s base-branch)
test -n "$BASE" || { echo "ベースを解決できませんでした"; exit 1; }
git fetch origin "$BASE"
git merge "origin/${BASE}"
```

Merge the branch this one was cut from — the PR's base — not whatever resolves as newest today. If a
newer release line has opened since, merging it retargets the branch instead of catching it up.

**When a hotfix line is in play, resolve nothing — ask.** `make base-branch` considers `release/*`
only and will not name it, and inferring a base from branch names is guessing at the moment guessing
is most expensive. Require `--base=<ref>` from the human.

## Step 2 — Classify every conflicted path

```bash
git diff --name-only --diff-filter=U
```

| Class | Paths | Resolution |
| --- | --- | --- |
| Generated — Go / API | `**/*.gen.go`, `**/*.sql.go`, `**/*_mock.go`, `openapi/openapi.gen.yaml` | Discard both sides; `make gen-api` / `make gen-query` |
| Generated — DML concatenation | `database/gen/*.gen.sql` | `make merge-dml` then `make gen-sqlc`. **Also runs when it did not conflict** |
| Generated — docs | `docs/openapi/**`, `docs/godoc/**`, `docs/db-schema/**`, `docs/coverage/**`, `docs/portal/guides/**`, `docs/portal/docs.json` | Regenerate. Keep them out of a feature PR — the release-push workflow syncs them |
| Pin lockfile | `.github/actions-pin.toml`, `docker/images-pin.toml` | Never pick lines. `make pin-actions-resolve` + `make pin-actions-apply`, `make pin-images-resolve` + `make pin-images-apply` |
| Version derivation | the `go` line in `go.mod`, Dockerfile `FROM`, versions quoted in READMEs | The source is `mise.toml`. Settle that file first, then `make sync-versions` |
| Vendored deps | `vendor/**` | Untracked; `go mod vendor` |
| Append-only registry | the tables in `docs/adr/README.md`, `docs/spec/glossary.md`, `.agents/ddd-audit/pattern-ledger.yaml` | Union both sides' entries — unless a key appears on both, which Step 6 hands back |
| Migration | `database/migrations/**` | Never edit an existing migration (`CLAUDE.md`). A numbering collision is renumbered on the newer side only |
| Translation pair | `**/*.ja.md` | Resolve the English canonical first, then re-sync via `canonicalize-doc`. Never resolve the `.ja.md` directly |
| Materialized env | `env/.env` | Not committed at all (`repo-ops` section 7) |
| Implementation | everything else | **Not mechanical.** Leave the markers in place and hand it back |

Classify before resolving anything. A path that matches no row is implementation by default — the
safe direction, since the cost of handing back a mechanical case is one message and the cost of
mechanically "resolving" a semantic one is a silent wrong merge.

## Step 3 — Apply, class by class

Resolve in dependency order, because several classes feed each other: `mise.toml` before
`sync-versions`, migrations before `merge-dml`, DML before `gen-sqlc`, the OpenAPI source before
`gen-api`.

For every generated class the move is the same and it is worth stating plainly: **do not merge the
file, delete the conflict and rebuild it.** Take the source of truth from both sides — the migration
set, the DML files, `openapi/openapi.yaml`, `mise.toml` — resolve *that* if it conflicted, then run
the generator and let it produce the artifact.

For a pin lockfile, run the resolver rather than reconciling entries. The lockfile is a cache of
tag → SHA; hand-merging it can leave an entry whose SHA never corresponded to its tag, which every
check downstream then treats as authoritative.

## Step 4 — Regenerate what did not conflict

Run this whether or not Step 2 found anything, and say that you did:

```bash
make merge-dml && make gen-sqlc     # database/gen/*.gen.sql is a concatenation
make gen-api                        # OpenAPI-derived code and mocks
make sync-versions                  # mise.toml → go.mod / Dockerfile / README
```

A derived artifact goes stale from the *other* side's changes, not from a textual conflict with them.
This is the step whose absence produces a green local merge and a red CI on the next unrelated PR.

## Step 5 — Verify

```bash
make pin-actions-check && make pin-images-check
make check-migration-up-version && make check-migration-down-version
make check-migration-up-gap && make check-migration-down-gap
make md-doc-ref-lint && make md-skill-lint
make go-lint && make go-test
```

`make go-lint` / `make go-test` may be deferred to CI per this repository's local-gate load bands; say which
gates you actually ran rather than implying all of them.

## Step 6 — Two ways this ends

The run ends in one of exactly two states. Which one is decided by whether anything non-mechanical is
left, never by how much was resolved.

### Anything left → stop there

Report what remains, with the conflict markers **still in place**, and end the run:

| Left to a human | Why |
| --- | --- |
| Implementation conflicts | Which behavior is correct is a reading, not a merge |
| The same key added on both sides — an ADR number, a glossary term, a ledger pattern | Union is mechanical; deciding which definition survives is not |
| ADR renumbering | The numbering convention is `docs/adr/README.md`'s. Read it at runtime, and if it and current practice disagree, surface the conflict per `CLAUDE.md`'s *Conflicting Authority* instead of choosing |
| A base that could not be resolved to one ref | Guessing a base is how a branch silently merges the wrong line |

> 機械的に解けるところまで統合しました。残りはここからお願いします: `<path>`（実装の意味的衝突）…

**Do not commit in this state, and do not offer to.** A merge commit carrying conflict markers is a
broken tree in the history, and it is broken in a way that reads as resolved — the same failure this
skill exists to prevent, one level up. Leave the working tree as it stands so the human continues from
where you stopped, and do not re-run to "finish" it after they resolve the rest; that is their commit.

### Nothing left → ask, then commit and push

Only when every conflict is resolved and Step 5's gates are green:

```text
質問: マージ元との統合が完了しました。コミットしてプッシュしますか？
選択肢:
  - コミットしてプッシュ
  - コミットのみ（プッシュしない）
  - 何もしない（作業ツリーのまま）
```

**Ask every time; never push on the strength of the merge succeeding.** A branch can take a release
line in without that merge being ready to leave the machine, and this skill cannot tell those apart —
whether the result should reach the remote is about the branch's state, which is the author's
knowledge, not the merge's.

For the push half specifically, `CLAUDE.md` requires the confirmation and its wording when the branch
already has a pull request:

> 変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？

Delegate the commit itself to `commit`, which splits and words it by the repository's convention.

## Do / Do NOT

- ✅ Take the base from the PR's `baseRefName`; require `--base` when a hotfix line is in play.
- ✅ Classify every conflicted path before resolving any of it.
- ✅ Rebuild generated artifacts from their source instead of choosing a side.
- ✅ Run resolvers for pin lockfiles; run `sync-versions` for version derivations.
- ✅ Regenerate the derived artifacts even when nothing conflicted, and say you did.
- ✅ Treat an unmatched path as implementation.
- ✅ Say which gates were actually run.
- ✅ End in exactly one of the two states, and ask before committing when it is the clean one.
- ✅ Report in Japanese.
- ❌ Hand-edit or side-pick any generated artifact, lockfile, or `.ja.md`.
- ❌ Rebase, squash, force-push, or merge a line other than the branch's own base.
- ❌ Edit an existing migration, or renumber the older side of a collision.
- ❌ Decide which of two colliding registry keys wins.
- ❌ Resolve a semantic conflict in implementation code.
- ❌ Commit while any conflict marker remains, or offer to.
- ❌ Push because the merge succeeded — the branch's readiness is the author's knowledge, not the merge's.

## Checklist

- [ ] Base resolved from the PR's `baseRefName`, or supplied via `--base`; merge used, never rebase.
- [ ] Every conflicted path classified; unmatched paths treated as implementation.
- [ ] Generated artifacts rebuilt from source, not side-picked.
- [ ] Pin lockfiles re-resolved through their resolvers; versions propagated from `mise.toml`.
- [ ] Registries unioned; colliding keys handed back rather than decided.
- [ ] English canonical resolved before any `.ja.md`, then re-synced.
- [ ] Step 4 regeneration run regardless of conflict count, and stated.
- [ ] Gates run and named; deferred ones said to be deferred.
- [ ] Ended in exactly one of the two states.
- [ ] Anything left → markers intact, nothing committed, nothing offered, run stopped.
- [ ] Nothing left → commit / push asked, never assumed; `commit` used for the commit itself.
