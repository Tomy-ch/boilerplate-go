---
name: release-notes
description: >-
  Generate a Japanese release note Markdown file under `.github/release/` summarizing changes between a specified `origin` git tag and `HEAD`, in the project's canonical sectioned format. Both the FROM tag and the new release version are confirmed with the user. Triggers: 「リリースノートを作成」, "release notes", 「v1.x.y のリリースノート」.
---

# Release Notes Generation Procedure

This skill defines the work procedure for generating a Japanese release note that summarizes the changes between a specified `origin` git tag and the current `HEAD`, and writes it to `.github/release/<NEW_VERSION>.md`.

The canonical examples of the target format are:

- `.github/release/v2.1.0.md`
- `.github/release/v2.0.0.md`

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## First Step: Confirm the FROM Tag and the New Version

This skill **MUST call `AskUserQuestion` immediately after invocation** to confirm two values, in the following order. Do NOT silently adopt a value from skill arguments or recent messages — an explicit confirmation is required to prevent misconfiguration.

### 1. FROM tag (comparison base)

1. Run `git fetch --tags --prune origin` to ensure local tag refs are up to date.
2. Collect the most recent SemVer tags from `origin`:

    ```sh
    git ls-remote --tags origin | awk -F/ '{print $NF}' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -5
    ```

3. Invoke `AskUserQuestion`:
    - Question: "比較元 (FROM) の origin タグを指定してください。"
    - Provide the top 3 most recent tags as options, plus an "Other" fallback for older tags.
4. Validate that the answer matches `^v[0-9]+\.[0-9]+\.[0-9]+$` and that `git rev-parse <FROM>^{commit}` succeeds. Use it as `<FROM_TAG>`.

### 2. New version (NEW_VERSION)

1. Compute candidate bumps using the in-repo helper:

    ```sh
    tsx scripts/semver <FROM_TAG> patch
    tsx scripts/semver <FROM_TAG> minor
    tsx scripts/semver <FROM_TAG> major
    ```

2. Also detect a hint from the current branch (`git rev-parse --abbrev-ref HEAD`). If it matches `release/v[0-9]+\.[0-9]+\.[0-9]+`, surface that value as an additional candidate.
3. Invoke `AskUserQuestion`:
    - Question: "新しいリリースのバージョン (NEW_VERSION) を指定してください。"
    - Options: patch / minor / major candidates from `scripts/semver/index.ts`, plus the branch-derived candidate if present.
4. Validate the answer matches `^v[0-9]+\.[0-9]+\.[0-9]+$`. Use it as `<NEW_VERSION>`.

Do NOT read git history, run diffs, or write any file until both values are confirmed.

## Preconditions

- `<FROM_TAG>` exists on `origin` and is reachable from `HEAD`.
- The working tree may be dirty, but only changes that are committed (i.e., between `<FROM_TAG>` and `HEAD`) will appear in the release notes. If `git status --porcelain` is non-empty, mention this to the user once before writing.
- Do NOT work directly on the `production` / `develop` / `staging` / `release/*` branches when committing the generated file (see Git rules in AGENTS.md). Use the current branch as-is for *reading* git state, but instruct the user to commit on an appropriate feature branch if they are currently on a protected branch.

## AI Modification Scope

Per the "Exception: Skill Execution" clause in AGENTS.md, the normal AI Modification Scope restrictions are relaxed for the duration of this skill's execution. The following paths are permitted to be **created** while this skill is running:

- `.github/release/<NEW_VERSION>.md` (new file only)

The following remain protected even during skill execution:

- `AGENTS.md` / `CLAUDE.md`
- Existing release notes under `.github/release/` (this skill never modifies or overwrites an existing file — if `.github/release/<NEW_VERSION>.md` already exists, stop and ask the user)
- Generated files (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, generated content under `docs/`)
- Everything outside `.github/release/`

## Execution Steps

Once `<FROM_TAG>` and `<NEW_VERSION>` are confirmed, execute the following.

### 1. Guard: Output File Does Not Exist

```sh
test ! -e .github/release/<NEW_VERSION>.md
```

If the file already exists, stop and ask the user how to proceed (do not overwrite silently).

### 2. Collect Diff Metadata

Gather the following from `<FROM_TAG>..HEAD`:

```sh
# Commit count
git rev-list --count <FROM_TAG>..HEAD

# File change count and +/- lines (diffstat summary)
git diff --shortstat <FROM_TAG>..HEAD

# Commit log with subjects (non-merge first, then merges for context)
git log --no-merges --pretty=format:'%h %s' <FROM_TAG>..HEAD
git log --merges     --pretty=format:'%h %s' <FROM_TAG>..HEAD

# Changed files grouped by top-level directory (for scope inference)
git diff --name-only <FROM_TAG>..HEAD | awk -F/ '{print $1}' | sort -u
```

Optionally, for richer context on individual commits:

```sh
git log --no-merges --pretty=format:'%h%n%s%n%b%n---' <FROM_TAG>..HEAD
```

### 3. Categorize Commits

Inspect each non-merge commit subject and bucket it. Match both English and Japanese-style prefixes used in this repo (`Feat:`, `Fix:`, `Refactor:`, `Docs:`, `Chore:`, `Test:`, etc.):

| Bucket | Section in the output |
| --- | --- |
| Feat / Feature | `### 新機能・改善` |
| Refactor / Perf / Chore (impl-affecting) | `### 新機能・改善` (as sub-bullet) |
| Fix / Bugfix | `## 不具合修正` |
| Docs | `### 新機能・改善` → `#### ドキュメント整備` |
| Test / CI / Build | `### 新機能・改善` → `#### 開発ツールチェーンの同期` or `#### その他の改善` |

When the bucket is ambiguous, inspect the changed file paths from step 2 to infer scope (e.g., `database/` → DB, `openapi/` → API, `.github/workflows/` → CI).

### 4. Compose the Release Note

Write `.github/release/<NEW_VERSION>.md` in **Japanese**, following the canonical `v2.1.0` format. The required top-level structure is:

```markdown
<!-- markdownlint-disable MD041 -->
## 概要

{2–4 行で、このリリースの趣旨を要約する。FROM_TAG → NEW_VERSION の位置づけ（パッチ/マイナー/メジャー）に触れる。}

変更規模は以下です。

- `<N>` コミット
- `<M>` ファイル変更
- `+<INS> / -<DEL>`

## 変更内容

### 新機能・改善

#### {サブカテゴリ（例: ランタイム・ライブラリのアップデート / 開発ツールチェーンの同期 / ドキュメント整備 / その他の改善）}

- {変更内容を、コミットの羅列ではなく「読んで理解できる粒度」でまとめる}
  - {参照すべきコミット hash や PR があれば付記}

## 動作確認手順

1. {このリリースで変わった箇所を確認するための具体手順}
2. ...

## 影響（想定されるメリット／注意点）

- ✅ {メリット}
- ⚠️ {注意点（破壊的変更・運用変更・依存更新の影響など）}

## 追加ライブラリ

- {追加された依存。無ければ「なし（既存依存のバージョン更新のみ）」}

## 不具合修正

- {Fix 系コミットを文章化したもの}

## 補足・備考

- {OpenAPI の破壊的変更の有無、portal 反映、リリースブランチ運用などのメモ}
```

Rules for the content:

- **Do not paste raw commit subjects.** Summarize them in human-readable Japanese sentences.
- **Group by theme**, not by chronology.
- **Reference concrete file paths or component names** when they help readers locate the change (e.g., `scripts/semver/index.ts`, `internal/controller/handler/...`).
- **Be honest about scope.** If a section has no content (e.g., no bug fixes), write `- 該当なし` rather than fabricating items.
- **Match existing tone.** Compare to `.github/release/v2.1.0.md` for sentence style.

### 5. Show a Preview Before Writing

Before calling `Write`, present the proposed content to the user (either inline or via a short summary plus the file path), and confirm with one final `AskUserQuestion`:

- Question: "この内容で `.github/release/<NEW_VERSION>.md` に書き出してよいですか？"
- Options: "書き出す" / "修正したい箇所を指摘する"

Only proceed with `Write` after the user confirms.

### 6. Verify with Markdown Lint

After writing, run:

```sh
make md-fix
make md-lint
```

`make md-fix` runs `markdownlint-cli2 --fix` on the entire repository to auto-fix common issues (blank-line placement around headings / lists / code blocks, trailing whitespace, file-final newline, etc.). `make md-lint` then verifies that the result is clean against `.markdownlint-cli2.yaml`.

If `make md-lint` reports remaining errors:

1. Read the lint output.
2. Fix the violations manually (rules that auto-fix cannot resolve, e.g., heading hierarchy, duplicate headings, bare URLs).
3. Re-run `make md-fix` then `make md-lint` until clean.

Do NOT report the skill as complete until `make md-lint` exits cleanly.

`make md-fix` operates on the entire repository, so it may modify Markdown files unrelated to this release note. List any such files when reporting completion so the user can review the broader change set.

### 7. Final Verification

After writing and lint:

- Confirm the file exists at `.github/release/<NEW_VERSION>.md`.
- Do NOT stage, commit, or push the file. Inform the user that the file has been created and let them handle git operations per AGENTS.md rules.

## Checklist

Confirm the following before reporting completion:

- [ ] `<FROM_TAG>` confirmed with the user via `AskUserQuestion` and verified to exist on `origin`
- [ ] `<NEW_VERSION>` confirmed with the user via `AskUserQuestion` and validated against SemVer
- [ ] `.github/release/<NEW_VERSION>.md` does not already exist
- [ ] Diff metadata (commit count / file count / +/- lines) collected from `git`
- [ ] Commits categorized into the v2.1.0 sections
- [ ] Release note drafted in Japanese, matching the canonical format
- [ ] Preview confirmed by the user
- [ ] `.github/release/<NEW_VERSION>.md` written
- [ ] `make md-lint` exits cleanly
- [ ] User informed that no git operations were performed

## Notes

- Output language inside the file MUST be Japanese (per `CLAUDE.md` language rules), regardless of the language used during interaction with the user.
- This skill never amends, force-pushes, tags, or pushes. Git operations remain the user's responsibility.
- After updating `SKILL.md`, also update `SKILL.ja.md` to keep the Japanese translation in sync.
