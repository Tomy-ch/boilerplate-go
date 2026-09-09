---
name: new-env
description: >-
  Add a new environment variable to the project end-to-end, keeping the typed config struct, env file samples, and documentation in sync — the config Loader field, the `Config` struct and its getter, the testing mock, `env/.env` plus the per-environment `env/.env.{ci,dev,stg,prd,dast}`, and the `env/README` table row. Confirms variable name, type, description (en + ja), required vs default, and per-environment values before writing. Use it whenever a new setting must be readable from the environment — 「環境変数を追加して」. It adds a testing setter helper only on explicit request. Do NOT use it to change an existing variable's value only (edit the env file directly).
---

# New Env

Adds a new environment variable to the project end-to-end. The variable is read by the `Loader` struct, mapped into the typed `Config`, exposed via a getter, set in each per-environment sample `env` file, and documented in `env/README.md`.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

Use this skill when:

- A new env var is needed (new feature flag, tunable timeout, external service URL, etc.).
- Multiple files need to stay in sync and you want them updated consistently.

Do NOT use this skill for:

- Renaming an existing env var (different workflow — rename across all places).
- Removing an existing env var (deletion is reverse-direction and safer to do by hand).
- Adding a whole new subsystem (e.g., a new `RedisConfig` group). This skill assumes the subsystem already exists. For a new subsystem, do the first variable manually then use this skill for subsequent additions.

## What This Skill Reads / Writes

**Reads (always)**:

- `internal/config/envspec.go` — Loader subsystem inventory (envPrefix → struct mapping).
- `internal/config/model.go` — Config subsystem inventory (private field naming).
- `internal/config/config.go` — `New()` body for mapping pattern; existing getters for naming convention.
- `internal/config/config_testing_mock.go` — the `expected*` variable block and the mock's field initialisation.
- `env/.env` (the local default), `.env.ci`, `.env.dev`, `.env.stg`, `.env.prd`, `.env.dast` — per-environment value placement.
- `env/README.md`, `env/README.ja.md` — table format and subsystem section names.

**Writes (only with confirmation)**:

- The files listed above. Per-file edits are minimal (one insertion in the correct location each).

**Never touches**:

- `config_testing_setter.go` unless the user explicitly asks.
- Any code under `internal/` outside `config/`.
- Generated files.

## First Step: Gather the Spec

This skill **MUST call `AskUserQuestion` immediately after invocation**. Ask in batches (multiple questions per call) to collect the spec:

### Question 1: Variable name and subsystem

- Question: 「環境変数名（`{PREFIX}_{NAME}` 形式）を入力してください。例: `APP_FEATURE_X` / `DB_READ_REPLICA_HOST`」
- Free-text input. The skill then:
  1. Splits at the first `_` to get the prefix.
  2. Looks up the prefix in the live `envspec.go` (each subsystem struct has `envPrefix:"XXX_"`).
  3. If matched: shows "推定サブシステム: `Application` (envPrefix `APP_`)" and asks for confirmation.
  4. If not matched: surfaces the candidates and asks the user to either pick a different prefix, or stop and add a new subsystem manually first.

### Question 2: Go type and required/default

Ask the user (single AskUserQuestion with 4 options):

- Question: 「型と必須・デフォルトを選んでください」
- Options:
  - 「string, required」
  - 「string, default あり（値を後で指定）」
  - 「int / bool / duration, required（型は後で指定）」
  - 「int / bool / duration, default あり（型と値を後で指定）」

Follow up with free-text for the concrete type (`int` / `bool` / `time.Duration` / `[]string` / etc.) and default value when applicable.

### Question 3: Description

Free-text. The user provides EITHER English OR Japanese (or both). The skill auto-fills the missing side via inline translation so both READMEs stay in sync without forcing the user to write twice.

- 「説明（日本語または英語のどちらか）」 — written into the matching language's README directly
- Notes column（任意） — Secret management / 環境依存等の注記。Provided in the same language as the description; the skill translates for the other.

Resolution rules:

- If only Japanese provided → skill writes Japanese to `env/README.ja.md` row, then translates to English for `env/README.md` row.
- If only English provided → reverse direction.
- If both provided → use as-is, no translation.
- Translations are kept short and direct (single-line, technical register matching surrounding rows). If the description is non-trivial or domain-specific, surface the proposed translation in the Step 2 plan summary for user review before writing.

### Question 4: Per-environment values

Single AskUserQuestion:

- Question: 「環境別の値を指定しますか？」
- Options:
  - 「全環境同じ値（または default で OK）」
  - 「local だけ別の値を入れる」
  - 「prd だけ別の値を入れる」
  - 「環境ごとに個別指定する（追加質問）」

Based on choice, collect per-env values. For prd-specific, surface that the value is typically a placeholder commented out (`# DB_HOST` style) and ask whether to follow that convention.

### Question 5: Mock / setter

- Question: 「テストヘルパーをどこまで追加しますか？」
- Options:
  - 「mock のみ（config_testing_mock.go の expected 値）」（推奨）
  - 「mock + setter（config_testing_setter.go にも追加）」 — only when the user has a clear testing need
  - 「テストヘルパーは追加しない」

## Step 1. Plan the Insertion Points

For each touch point, compute the exact insertion location by reading existing patterns:

### `envspec.go`

Find the matched subsystem struct. Append the new field at the end (preserve struct field order). Field tag format:

- Required: `env:"NAME,required"`
- With default: `env:"NAME" default:"value"`

Field type matches the Go type (`string` / `int` / `bool` / `time.Duration` / `[]string`).

### `model.go`

In the matching Config sub-struct (e.g., `ApplicationConfig`), append the private field with the camelCase name corresponding to the env name (e.g., `APP_FEATURE_X` → `featureX`).

### `config.go`

Two changes:

1. In `New()`, find the matching subsystem block and append the field mapping (`fieldName: cfg.Sub.FieldName,`).
2. Append a getter method on the sub-struct: `func (a *ApplicationConfig) FeatureX() T { return a.featureX }`. Use the existing getters in the same sub-struct as the formatting template (single-line or multi-line, doc comment style).

### `config_testing_mock.go`

1. Append an expected value variable to the `expected*` block (`expectedApplicationFeatureX = ...`).
2. Add the field to the sub-struct literal that `MockConfigForTest` builds, using that variable.

Setters live in `config_testing_setter.go`, not here — add one only under the Question-5 opt-in.

### Test updates (mandatory — coverage must not regress)

The `internal/config` package is currently 100% covered. Adding a field without updating the matching tests breaks coverage. Update three test files in lockstep with the production changes:

| Test file | What to update |
| --- | --- |
| `internal/config/config_test.go` | In `TestNew`, add the new field to the expected `&Config{...}` literal under the matching sub-struct, using the `expected*` variable defined in `config_testing_mock.go`. |
| `internal/config/model_test.go` | Add a new `Test<SubStruct>_<Field>` function — one per getter, strictly 1:1, following the sibling functions (`t.Parallel()` at every level, `正常系` group, `MockConfigForTest(t).<sub>`, `assert.Equal(t, expected<...>, sub.<Field>())`). Do NOT bundle it into an existing test. |
| `internal/config/config_testing_mock_test.go` | In `TestMockConfigForTest`, add the new field to the expected `&Config{...}` literal under the matching sub-struct. |

The `expected*` variable comes from the `config_testing_mock.go` addition (Question-5 mock scope). If the user opted out of mock additions, the test updates also must be skipped — surface this trade-off explicitly in Step 2 plan: "テストヘルパー追加なし → カバレッジ維持テストもスキップ。100% から下がる可能性あり".

### env files

For each of `env/.env` (the local default), `.env.ci`, `.env.dev`, `.env.stg`, `.env.prd`, `.env.dast`:

- Locate the section comment (e.g., `# Application`).
- Append the new var line under the matching section, preserving alignment.
- For prd, default to the commented placeholder convention (`# APP_FEATURE_X`) unless the user supplied an explicit prd value.

### `env/README.md` and `README.ja.md`

In the matching subsystem table (e.g., `### Application`), append a row:

```text
|APP_FEATURE_X|<description>|<type>|<example>|<notes>|
```

The Japanese README gets the Japanese description and notes.

## Step 2. Show the Plan and Confirm

Display the full set of proposed changes as a Japanese summary:

```text
追加する環境変数: APP_FEATURE_X
  サブシステム: Application (envPrefix "APP_")
  型: bool
  required / default: required
  英語説明: Enable experimental X feature
  日本語説明: 試験的な機能 X を有効化
  notes (en): 開発環境のみ true 推奨
  per-env 値: local=true / ci=true / dev=false / stg=false / prd=false
  テストヘルパー: mock のみ

修正対象:
  - internal/config/envspec.go (Application 構造体に 1 行追加)
  - internal/config/model.go (ApplicationConfig 構造体に 1 行追加)
  - internal/config/config.go (New() マッピング + Getter 追加)
  - internal/config/config_testing_mock.go (expected 値 + setter 追加)
  - internal/config/config_test.go (TestNew 期待値構造体に 1 行追加)
  - internal/config/model_test.go (Test<SubStruct>_<Field> を 1 つ追加)
  - internal/config/config_testing_mock_test.go (TestMockConfigForTest 期待値構造体に 1 行追加)
  - env/.env, .env.ci, .env.dev, .env.stg, .env.prd, .env.dast (各 1 行追加)
  - env/README.md, env/README.ja.md (Application 表に行追加)

説明の補完:
  - 入力: 日本語のみ供与
  - 英語訳（自動生成、レビュー対象）: "Enable the experimental X feature"
```

Confirm with `AskUserQuestion`:

- Question: 「以上の内容で適用しますか？」
- Options: 「適用する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 3. Apply Changes

For each file, use the `Edit` tool with exact anchor strings derived from the read context (the last existing field in the target struct / section). Stage changes in this order:

1. `envspec.go`
2. `model.go`
3. `config.go` (mapping + getter)
4. `config_testing_mock.go` (expected value variable + mock literal)
5. `config_test.go` (TestNew literal — coverage maintenance)
6. `model_test.go` (getter ごとの `Test<SubStruct>_<Field>` — coverage maintenance)
7. `config_testing_mock_test.go` (TestMockConfigForTest literal — coverage maintenance)
8. env files (one Edit per file: `env/.env` (the local default), `.env.ci`, `.env.dev`, `.env.stg`, `.env.prd`, `.env.dast`)
9. `env/README.md` then `env/README.ja.md`

After each file edit, verify the edit landed (the `Edit` tool reports success or failure — if any fails, stop and report).

## Step 4. Verify

Run:

```sh
make go-fix    # absorb formatting
make go-test   # confirm config loading + getters compile, tests pass, coverage maintained
```

If `make go-test` fails, surface the failure and stop. Do NOT roll back the edits; the user decides whether to fix forward.

If `make go-fix` modifies anything beyond the just-edited files, surface the diff.

### Coverage check

After `make go-test` succeeds, confirm the `internal/config` package's coverage line in the test output. Expected: `go-boilerplate/internal/config <time>  coverage: 100.0% of statements`. If coverage dropped below 100%:

1. Identify which new code path lacks a test (typically the new getter or the new `New()` mapping line).
2. Verify the Step-3 test updates (`config_test.go`, `model_test.go`, `config_testing_mock_test.go`) all included the new field.
3. If any was missed, add it and re-run `make go-test`.

Do NOT mark the skill complete with reduced coverage unless the user explicitly opted out of mock + test updates (Question 5).

## Step 5. Closing

- Print a 1-line summary: `<VAR_NAME> を追加。<N> ファイル更新。make go-test OK。`
- The skill does NOT commit. Use `/commit` after reviewing.
- If the user also wants the corresponding documentation in `internal/config/README.md` to be refreshed (rare — `internal/config/README.md` describes the package, not individual vars), recommend `/sync-readme`.

## AI Modification Scope

Per the "Exception: Skill Execution" clause in `CLAUDE.md` / `AGENTS.md`, the AI modification scope is relaxed during this skill's run, scoped to:

- The files listed above (or the subset the user confirmed).

Remains protected:

- `internal/config/config_testing_setter.go` unless the user explicitly opts in.
- All other code outside `internal/config/`.
- Generated files.

## Constraints

- ❌ Hardcode the subsystem list — always derive from live `envspec.go`
- ❌ Add a `config_testing_setter.go` helper without explicit user opt-in
- ❌ Modify env files outside the listed 6 set
- ❌ Skip the spec-confirmation `AskUserQuestion`
- ❌ Apply changes without showing the full plan first
- ❌ Touch code unrelated to the new var
- ✅ Japanese user-facing output
- ✅ Preserve formatting in env files (column alignment, comment style)
- ✅ Preserve formatting in README tables (column count and order)
- ✅ Run `make go-fix` + `make go-test` after the writes
- ✅ Surface any verification failure; do not auto-rollback

## Checklist

Before reporting completion, confirm:

- [ ] Variable name + subsystem confirmed via `AskUserQuestion`
- [ ] Go type and required/default confirmed
- [ ] Description provided in either English or Japanese; the missing side was auto-translated and surfaced in the Step-2 plan for review
- [ ] Per-environment values resolved (single default or per-env override)
- [ ] Mock-helper scope confirmed
- [ ] Full plan was displayed and the user approved it
- [ ] `envspec.go`, `model.go`, `config.go` each updated with one insertion in the matching subsystem
- [ ] `config_testing_mock.go` updated (unless skipped)
- [ ] `config_test.go`, `model_test.go`, `config_testing_mock_test.go` each updated to cover the new field (unless mock scope was skipped — in which case this is noted explicitly)
- [ ] All 6 env files updated with the new var under the matching subsystem comment
- [ ] `env/README.md` and `env/README.ja.md` updated with a new table row
- [ ] `config_testing_setter.go` was NOT touched unless explicitly requested
- [ ] `make go-fix` and `make go-test` were run and the results reported
- [ ] `internal/config` coverage was checked against the test output; expected 100.0% (or explicitly noted lower if mock scope skipped)
- [ ] No commits / pushes were performed
- [ ] Final summary is in Japanese
