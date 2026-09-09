---
name: submit-pr
description: >-
  Push the current feature branch to `origin` and create or update its GitHub pull request, filling the body from `.github/pull_request_template.md` in Japanese and asking before any push. Its first action is to offer a pre-push `/impl-review`; choosing to review cancels submit-pr, since the fixes it produces must be committed first. Use it whenever the branch is ready to go up — 「PR 作って」「プルリクにして」. Do NOT use it to commit the working tree first (`commit`) or to merge the PR.
---

# Submit PR

This skill pushes the current branch to `origin` and ensures a GitHub pull request exists for it. It handles two cases automatically:

- **Create**: no PR exists for the current branch → push (with `-u` if no upstream) and open a new PR.
- **Update**: an open PR already exists → confirm with the user, then push (the PR's diff auto-updates).

The PR body is filled from `.github/pull_request_template.md`. The skill never auto-pushes, never overwrites an existing PR's title/body, and never force-pushes.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## Preconditions

- `gh` CLI is installed and authenticated (`gh auth status` succeeds).
- Current branch is not a protected branch (`production` / `develop` / `staging` / `release/*`).
- Working tree is clean. If there are uncommitted changes, the skill aborts and suggests running `/commit` first.

## Step 0. Pre-flight Checks

Run in parallel:

```sh
git rev-parse --abbrev-ref HEAD                          # current branch
git status --porcelain                                   # working-tree state
git rev-parse --verify '@{u}' 2>/dev/null                # upstream existence
git log '@{u}'..HEAD --oneline 2>/dev/null               # unpushed commits (if upstream)
gh auth status
```

Bail out if any of the following:

- Branch matches `^(production|develop|staging|release/.+)$` → tell the user to switch to a feature branch.
- `git status --porcelain` is non-empty → tell the user to run `/commit` (or stash) first.
- `gh auth status` fails → tell the user to run `gh auth login`.

The four valid working states going into Step 2:

| Upstream | Unpushed commits | Meaning |
| --- | --- | --- |
| none | n/a | First push case |
| set | > 0 | Subsequent push case |
| set | 0 | Nothing to push; PR may still need to be created |
| set | 0 + PR open | Nothing to do (handled in Step 2) |

### Step 1. Pre-push Local Review Gate (confirm)

Immediately after the pre-flight bail-outs pass — **before composing anything or pushing** — ask which pre-push reviews to run. Review before the change leaves the machine is the point: it inspects the local diff on a different model than the implementer and catches gaps (auth / IDOR, DI / SQL, shared-schema propagation) that mocked tests miss.

**Name all three explicitly.** The Review Phase Protocol in `AGENTS.md` gives one subject to each skill — `/impl-review` the change, `/test-review` the tests, `/comment-sweep` the comment stock — and none of them offers to run the others. So this question is the only place all three are visible at once, and listing just one would silently drop the other two from every flow that goes through here. Do NOT auto-run any of them.

Per the same protocol, **estimate each one's return before asking** — which layers the change touched, whether tests or comments moved at all, what an earlier skill in this session already covered — and say which you expect to pay off and which you expect to return nothing. Handing over three unpriced checkboxes is the failure this protocol names.

`AskUserQuestion`:

- Question: 「push 前にどのレビューを実行しますか？（それぞれの見込みを添えて提示すること）」
- Options (multi-select; each carries this run's estimate and its reason):
  - 「`/impl-review`（変更そのもの — 実装者とは別モデルの独立・敵対レビュー）」
  - 「`/test-review`（テスト — 分岐 × 意味の網羅とシンボル網羅）」
  - 「`/comment-sweep`（コメント在庫 — 触れたファイルをファイル単位で掃引）」
  - 「実行済み / 不要（このまま進める）」 — continue to Step 2.
  - 「キャンセル」 — abort.

Selecting any review cancels submit-pr and guides the user to run them, in the order listed.

**On any review choice, cancel submit-pr and guide the user to review — do NOT chain a review skill inline and do NOT try to resume this run.** Print:

> submit-pr をキャンセルします。選択したレビュー（<選ばれたスキル名を列挙>）を実行し、指摘を修正してから `/commit` で確定し、改めて `/submit-pr` を実行してください。（clean tree でないと push できないため、レビュー修正の commit を先に済ませる必要があります。次回はこの Step 1 で「実行済み」を選べばそのまま進みます。）

Why a clean cancel rather than a pause-and-resume: a local review commonly produces fixes, which must be committed *before* submit-pr can run at all (the clean-tree precondition in Step 0, and the push in Step 6). Since the working tree will change anyway, there is nothing to "resume" — the next `/submit-pr` is a fresh, cheap run that flows straight through once the fixes are committed. Guiding (not inline-chaining) keeps submit-pr free of a review + fix + commit loop it should not own.

**Depth by change type** — scale the recommendation to what the diff touches (this same scaling also drives the post-PR review at Step 9):

- **Behavior-affecting code** (`internal/**`, `pkg/**` `.go`, SQL, OpenAPI) → recommend the review by default.
- **Docs / tooling-dominant changes** (`docs/**`, `*.md`, `.claude/**`, `AGENTS.md`, CI config — no production behavior change) → note the lower ROI so the user can decline quickly; still ask.

Judge the dominant nature of the diff (changed paths / commit prefixes) for the default recommendation, but the user's choice always wins.

## Step 2. Detect Existing PR and Base Branch

```sh
gh pr view --json number,state,baseRefName,headRefName,url,title,body 2>/dev/null
make -s base-branch
```

Branch on the result:

- **PR exists and state is `OPEN`** → "update" path. Base branch is fixed (`baseRefName` from the result). The PR's own base is what the pull request is already merging into; nothing may re-resolve it.
- **PR exists but state is `MERGED` / `CLOSED`** → ask the user via `AskUserQuestion`:
  - Question: 「このブランチには `<state>` 状態の PR #N があります。新規 PR を作成しますか？」
  - Options: 「新規 PR を作成する」 / 「キャンセル」
- **No PR exists** → "create" path. The base is the branch `make base-branch` resolves: the latest release line, read from `origin`'s live state. Do not use `gh repo view --json defaultBranchRef` — the GitHub default branch lags behind the active release line, and a PR opened against it targets a generation-old base.

If `make base-branch` fails, stop and report it rather than guessing a base; opening a pull request against the wrong branch is not something the user can undo by editing the PR.

For the "create" path, confirm the resolved base via `AskUserQuestion` — a backport or a deliberate hotfix target is the case the resolver cannot know about:

- Question: 「ベースブランチをこれで作成しますか？」
- Options: 「`<resolved-base>` を使う」 / 「別のブランチを指定する」

Special early-exit cases:

- "update" path with 0 unpushed commits → tell the user there is nothing to push and stop. Print the existing PR URL.
- "create" path with 0 unpushed commits but the remote branch exists → continue to Step 3 (we will create a PR for whatever is already on the remote).

## Step 3. Gather Context and Read Template

Collect the inputs needed to compose title and body. `<base>` is the base branch decided in Step 2.

```sh
git log <base>..HEAD --pretty=format:'%h %s'                # commit titles
git log <base>..HEAD --pretty=format:'%h%n%s%n%b%n---'      # commit titles + bodies
git diff <base>...HEAD --shortstat                          # diff summary
git diff <base>...HEAD --name-only                          # changed files
```

Read `.github/pull_request_template.md` and identify sections by `#` / `##` headers. The current template defines:

- `# 概要`
- `## 変更内容`
- `## 動作確認方法`

Strip the HTML comment placeholders. If the template is absent, fall back to the same three-section structure inline.

## Step 4. Compose Title and Body

### Title

- Derive from the most significant change. Single-commit PR → use that commit's title (strip the leading `<Prefix>:` only if redundant). Multi-commit PR → summarize the overall intent in Japanese.
- ≤ 70 characters.
- If the branch name embeds an issue number (`feature/1234-...`, `bugfix/5678-...`), include `#1234` in the title naturally.
- For the "update" path: keep the existing PR title unchanged unless the user explicitly asks to change it.

### Body

Fill each template section in Japanese:

- **概要**: 1–3 sentences summarizing the PR's intent. Use commit messages as the primary source.
- **変更内容**: Bullet list grouped by area (API / DB / 内部ロジック / テスト / ドキュメント など). Reference changed files and commit titles. Group meaningfully — do not paste a raw file list.
- **動作確認方法**: Concrete verification steps. Adapt to what actually changed: `make serve` + curl for API changes, `make db-local-migrate-up` for migrations, `make go-test` for logic, etc.

If the branch name encodes an issue number, append `closes #N` at the bottom of the body (or fold it into 概要 if natural).

## Step 5. Confirm with the User

The pre-push impl-review decision was already made at **Step 1** (Phase 0) — do not re-ask it here.

Display the resolved title, base branch, push command, and full body.

### Create path

`AskUserQuestion`:

- Question: 「以下の内容で PR を作成しますか？」
- Options:
  - 「この内容で作成する」
  - 「draft で作成する」
  - 「title / body を修正したい」
  - 「キャンセル」

If the user chooses "修正したい", collect free-text feedback, regenerate the relevant section, and re-confirm.

### Update path

Display the unpushed commit list and diff summary. Then ask with the wording required by `CLAUDE.md`:

- Question: 「変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？」
- Options: 「push する」 / 「キャンセル」

## Step 6. Push

```sh
# First push (no upstream)
git push -u origin <branch>

# Subsequent push
git push
```

A branch cut from `origin/release/*` (the merged-PR recovery flow in `commit`) has its upstream pointing at that **protected** base, so a bare `git push` would target the protected branch. Always do the first push with the explicit refspec `git push -u origin <branch>` to repoint the upstream at the feature branch; only after that is a bare `git push` safe.

Never use `--force` or `--force-with-lease` unless the user has explicitly requested it.

On push failure (non-fast-forward, permission denied, network error, etc.), report the error verbatim to the user and stop. Do not attempt automatic recovery.

### The pre-push hook sizes itself — do not pre-empt it

`pre-push` runs the heavy Go gates through `make gate-go-push`, and `.makefiles/load.mk` decides from the number of open worktrees whether they run at full speed, throttled, or are deferred to CI (`repo-ops` §19; `make load-status` reports the band). Let the hook make that call.

Do **not** run `make go-lint` / `make go-test` by hand before pushing to "make sure" — with several windows open that is minutes of saturated host to rediscover what CI runs identically, and the saturation itself makes unrelated gates fail. Pushing *is* the verification step in the `ci-first` band. If the hook fails for a reason outside this change, `repo-ops` §11 covers the `--no-verify` carve-out.

When the band deferred gates to CI, say so in Step 8's report and treat the PR as unverified until its checks land — do not describe it as passing local verification it never ran.

## Step 7. Create or Update the PR

Stamp the phase as soon as the PR exists. GitHub records a creation time too, but the two answer
different questions and the join to it holds only about two thirds of the time:

```sh
.agents/closed-loop/marks.sh prOpenedAt 2>/dev/null || true
```

### Create the PR

```sh
gh pr create \
  --base "<base-branch>" \
  --title "<title>" \
  --body "$(cat <<'EOF'
<body>
EOF
)" [--draft]
```

### Update the PR

Step 6's push already updated the PR's diff. Do NOT touch the PR's title or body by default.

Only if the user explicitly asked to update them, run:

```sh
gh pr edit <number> [--title "<new-title>"] [--body "$(cat <<'EOF'
<new-body>
EOF
)"]
```

## Step 8. Report

Print the PR URL and a brief summary in Japanese. Run `make -s load-status`. When its resolved band is `ci-first`, the heavy gates were delegated by the push hook, so wait for their CI result with `gh pr checks --watch` and include the final check status in this report. If checks fail, report the failure and do not describe the PR as verified. Do not wait for CI in `full` or `low`: this step follows delegated verification, not every CI-only check.

For the create path:

```text
PR を作成しました: <url>
ベース: <base-branch>
タイトル: <title>
コミット数: N
CI 委譲: <なし / ci-first。gh pr checks の最終結果>
```

For the update path:

```text
PR を更新しました: <url>
追加コミット数: N
CI 委譲: <なし / ci-first。gh pr checks の最終結果>
```

## Step 9. Post-PR Review (confirm)

After the PR URL is reported, **always ask the user whether to run a PR-based review** — do not skip this, and do not auto-run a review. These are the reviews that need the PR to exist (pre-push `/impl-review` was already offered at Step 1). Use `AskUserQuestion`:

- Question: 「PR を作成/更新しました。コードレビューを実行しますか？」
- Options (offer the ones that apply):
  - 「`/code-review <PR#>` を実行」 — PR-based review (can post inline comments with `--comment`)
  - 「ultrareview を案内」 — cloud multi-agent review; **user-triggered and billed**, so the skill cannot launch it — only surface the command for the user to run
  - 「`/impl-review` / `/test-review` / `/comment-sweep` を実行」 — offer these only if the user skipped the pre-push gate at Step 1; list all three the way Step 1 does, each with its estimate, since they are peers and none of them will surface the others
  - 「レビューしない」

Scale the default recommendation to what changed, using the **Depth by change type** guidance in Step 1 (behavior-affecting code → recommend by default; docs / tooling-dominant → note the lower ROI). The user's choice always wins.

## Constraints

- ❌ Push to protected branches (`production` / `develop` / `staging` / `release/*`)
- ❌ `git push --force` / `--force-with-lease` (only with explicit user instruction)
- ❌ Auto-update an existing PR's title or body (only on explicit user request)
- ❌ Push while the working tree has uncommitted changes
- ❌ Create a PR without user confirmation
- ❌ Push to an existing PR branch without re-confirming with the exact wording required by `CLAUDE.md`
- ✅ Use `.github/pull_request_template.md` as the body skeleton
- ✅ Japanese title and body
- ✅ HEREDOC for the body when calling `gh pr create` / `gh pr edit`
- ✅ Detect issue number from branch name and surface it in title / body

## Checklist

Before reporting completion, confirm:

- [ ] Current branch is not a protected branch
- [ ] (必須) Phase 0 (Step 1) で push 前 `/impl-review` の実行可否を確認した（レビューを選んだら submit-pr はキャンセルし、review→fix→commit→再実行へ案内）
- [ ] Working tree was clean before the push
- [ ] `gh auth status` passed
- [ ] PR template was read and reflected in the body
- [ ] Title and body are Japanese
- [ ] Title ≤ 70 characters
- [ ] User confirmation was obtained before the push (mandatory for update path per `CLAUDE.md`)
- [ ] PR URL was reported to the user
- [ ] (必須) PR 作成/更新後に PR ベースのレビュー実行可否を確認した（深さは変更種別でスケール）
- [ ] No `--force` was used
