---
name: back-prop
description: >-
  Integrator skill for documentation drift detection across all layers. Confirms scope (changed files vs full repo) and detection categories once, then surfaces findings in four categories — README-to-code drift, an undocumented pattern the code now repeats, skill/README duplication, and the DDD pattern ledger against the ADR / README corpus — and performs the approved README / SKILL / ledger writes after per-item confirmation. Does NOT modify implementation code — code fixes are surfaced and left to the user. Use it after a multi-layer refactor or before a major PR review, to confirm the docs and skills still match code reality (priority README > Code > SKILL). To check a single layer, run this integrator and pick that layer in the scope question. Do NOT use it to review a diff (`impl-review`), to judge doc prose quality (`readme-review`), or to sync a README's file/dir structure (`sync-readme`).
---

# Back-Prop

Integrator for drift detection across layers. Fans out per-layer **read-only drift-detector subagents** in parallel based on scope, aggregates, then drives the per-item approval + write loop itself.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- After multi-layer refactor, before PR review
- Periodic hygiene sweep (catch undocumented conventions / skill bloat / README drift across all layers)
- When introducing a new layer-wide convention (to see where it's already followed / not yet)

To check a single layer, run this integrator and pick that layer in the scope question (「特定 layer のみ」).

Do NOT use for:

- Implementation code fixes (surface only; nothing here writes code)
- Architecture compliance per file — `arch-check` (with TODO hand-off)
- Spec validation — `verify-spec`

## Architecture: parallel detector subagents + integrator-side approval

Detection is delegated to five **read-only worker subagents** under `.claude/agents/`, one per layer. The integrator runs them concurrently via the Agent tool (`subagent_type`):

| Detector subagent | Layer | Canonical doc(s) |
| --- | --- | --- |
| `drift-detector-domain` | `internal/domain/**` | `internal/domain/README.md` |
| `drift-detector-usecase` | `internal/usecase/**` | `internal/usecase/README.md` + `boundary/README.md` |
| `drift-detector-controller` | `internal/controller/**` | `internal/controller/README.md` + `handler/README.md` (reference snippet) |
| `drift-detector-infra` | `internal/infrastructure/**` | infra / rdb / pgerror README（principles-focused、sibling code が de facto reference） |
| `drift-detector-pkg` | `pkg/**` | `pkg/README.md` + 各 `pkg/<name>/README.md` |

Two further detectors are keyed to a corpus rather than to a layer, and each runs only when its category is selected:

| Detector subagent | 対象 | Canonical doc(s) |
| --- | --- | --- |
| `drift-detector-ddd` | `.agents/ddd-audit/pattern-ledger.yaml` ↔ ADR / README コーパス | 台帳自身の `corpus` グロブ |
| `drift-detector-glossary` | `docs/spec/glossary.md` の用語 ↔ README / ADR / `docs/rules.md` | 用語表と Mechanism vocabulary 節 |

これは README↔コードでなく**台帳↔正本**のドリフトを見る。台帳は「どの Evans パターンをどこで解釈したか」
の帳簿なので、正本が動いた瞬間に静かに嘘になる — しかも誰も読まないファイルなので、放置しても誰も気づかない。
Evans 原義に忠実かどうかは扱わない（`ddd-audit` / `ddd-origin-auditor` の担当）。

(E) は**業務語彙が家を出ていないか**を見る。業務語は `docs/spec/` に住み、README / ADR は実装の構造と
その意思決定を述べる側なので、層 README へ育った業務語は家を出た語である。同じ語がどこか別の場所で
定義し直され、誰も気づかない。用語表そのものの生成・保守は `glossary` の担当で、**back-prop は正本を
作らず参照するだけ**。(D) と同型である。

(E) には他と違う制約が 1 つある。**findings のうち直せるのは層 README だけ**で、ADR と `docs/rules.md`
への漏れは報告に留まる。前者は back-prop の write scope そのものだが、後者は決定記録と統べる文書で、
検出器を満足させるために書き換えれば誰が決めるのかが反転する。detector 側が E1 / E2 として分けて返す
ので、integrator は E2 を承認対象に載せないこと。

These detectors are the per-layer drift-detection workers. They are **strictly read-only**: they surface (A)(B)(C) findings with reasoning + candidate options, but they **never call `AskUserQuestion` and never write**. The approval + write loop runs in **this integrator**, **single-threaded after aggregation**, so the five read-only detectors can fan out in parallel with zero write contention. Priority remains **README > Code > SKILL**.

## First Step: Confirm Scope + Detection Categories

`AskUserQuestion` with two batched questions (defaults auto-detected by git diff):

1. 質問: 「back-prop のスコープを選んでください」
   - 選択肢: 「変更ファイルのみ（ベースブランチとの diff、touched layer のみ fan-out）」 / 「リポジトリ全体（5 layer 全部 fan-out）」 / 「特定 layer のみ（layer を続けて指定）」 / 「キャンセル」

2. 質問: 「検出する drift 種別を選んでください（multi-select、既定 4 種類すべて）」
   - 選択肢: 「(A) README → Code drift」 / 「(B) Code → README undocumented pattern」 / 「(C) Skill ↔ README duplication」 / 「(D) DDD 台帳 ↔ ADR/README コーパス」 / 「(E) 業務語彙 ↔ README/ADR」

Detection categories are propagated to every detector. (D) and (E) are the ones that do not depend on
Go files, so they are worth selecting even when the diff touches no code at all — a prose-only change
is exactly the case that rots the ledger, and the one that lets a business term settle into a README.

## Step 1. Resolve Layers + File Lists in Scope

For "changed files" mode:

```sh
BASE=$(gh pr view --json baseRefName -q '.baseRefName' 2>/dev/null || make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }
git diff --name-only "origin/${BASE}...HEAD" -- '*.go' | grep -vE '\.gen\.go$|\.sql\.go$|_mock\.go$|_test\.go$'
```

An existing pull request's `baseRefName` stays the authority — the drift you report has to sit in the
diff the PR shows. With no PR, `make base-branch` resolves the latest release line from `origin`'s live
state; `gh repo view --json defaultBranchRef` is not the fallback, because the GitHub default branch
keeps answering with an earlier release line. Stop on an unresolved base rather than continuing: an
empty file list fans out zero detectors and reports no drift, which reads exactly like a clean run.

Map to layers by path prefix, keeping the per-layer file list (you pass it to each detector so it does not re-resolve git):

| Path prefix | Detector subagent |
| --- | --- |
| `internal/domain/` | `drift-detector-domain` |
| `internal/usecase/` | `drift-detector-usecase` |
| `internal/controller/` | `drift-detector-controller` |
| `internal/infrastructure/` | `drift-detector-infra` |
| `pkg/` | `drift-detector-pkg` |

For "full repo": fan out all 5. For "specific layer": ask user which, fan out only those.

When (D) is selected, additionally resolve the **DDD corpus** — not `*.go`-filtered, since the corpus is prose. Read the `corpus` globs from `.agents/ddd-audit/pattern-ledger.yaml` (never hardcode them here) and intersect with the diff for `changed` scope. Add `drift-detector-ddd` to the fan-out whenever that intersection is non-empty, or always in `full` scope.

When (E) is selected, resolve the **prose corpus** the same way: `internal/**/README.md`,
`pkg/**/README.md`, `docs/adr/*.md`, `docs/rules.md`, `docs/architecture.md`, minus `*.ja.md`.
Add `drift-detector-glossary` whenever that intersection is non-empty, or always in `full` scope.
**Skip it, and say why, when `docs/spec/glossary.md` does not exist** — the detector's probe list is
that page's term table, and without it there is nothing to look for.

No Go changes **and** no corpus changes in changed-files mode → exit cleanly. Go-only changes with (D) or (E) selected still skip those detectors; a diff that touches no prose cannot rot the ledger or move a term.

## Step 2. Fan Out Detector Subagents IN PARALLEL

For each layer in scope, spawn its detector with the **Agent tool**, all in **a single message with multiple tool calls** so they run concurrently. Pass each detector:

- `scope` — `changed` or `full`
- `files` — the pre-resolved newline list of in-scope `.go` files for that layer (from Step 1)
- `baseRef` — the base branch (fallback)
- `categories` — the selected subset of `A` / `B` / `C`

`drift-detector-ddd` and `drift-detector-glossary` take a prose file list instead of a Go file list,
and no `categories` (each only ever detects its own). Spawn them in the **same message** as the layer
detectors so everything runs concurrently.

Each detector's final message **is** its findings (Japanese, each with reasoning + candidate options). Collect them with their layer label.

> If the `drift-detector-*` subagents cannot be spawned in the current environment, follow each `drift-detector-<layer>.md` procedure inline instead — the integrator still runs the per-item approval + write loop (Step 4) single-threaded afterward.

## Step 3. Aggregate Findings (read-only checkpoint)

Combine all detector findings into a single Japanese summary grouped by layer + category, so the user sees the full surface before any decision:

```text
back-prop drift 検出結果（scope: <X>, 種別: A/B/C）

[domain]     A <n> / B <m> / C <k>
  ...（各 finding: rule・reasoning・options）
[usecase]    ...
[controller] ...
[infra]      ...
[pkg]        ...
[ddd]        D1 <n> / D2 <m> / D3 <k>

総 finding: <sum>。これから 1 件ずつ承認 / 棄却を確認します。
```

If all clean:

```text
back-prop drift 検出結果（scope: <X>, 種別: A/B/C）
全 layer で drift は検出されませんでした（チェック済み: <layer list>）。
```

## Step 4. Per-Item Approval + Write (integrator-side, single-threaded)

Detector subagents are read-only. For each finding, **the integrator** now drives the decision — there is no write contention because this loop is single-threaded:

1. `AskUserQuestion` with the candidate options the detector surfaced for that finding (e.g. コード修正 / README 更新 / ルール緩和 / skill 簡略化 / 無視).
2. If the user approves a **doc / skill** change:
   - State the reasoning explicitly, then show the draft as a diff (変更前 / 変更後).
   - After final confirmation, write via `Edit` / `Write` — only to the **canonical README** of that layer or the relevant **skill `SKILL.md`** (and never code).
3. If the user chooses a **code fix**: surface it as the user's task (this skill never writes implementation code).
4. Loop over all findings; the user may abort partway.

Write scope is restricted to: layer READMEs (`internal/<layer>/README.md` and sub-READMEs), skill `SKILL.md` files, and — for (D) findings only — `.agents/ddd-audit/pattern-ledger.yaml`. Never implementation code, never generated files, never `AGENTS.md`.

**(E) narrows further, and the split is the detector's, not yours to re-open.** E1 findings sit in
layer READMEs and are inside the scope above. E2 findings sit in an ADR or `docs/rules.md` and are
**never put to approval at all** — surface them in the report and leave them there. Nor may you
"resolve" an E2 by editing `docs/spec/glossary.md`: the vocabulary is `glossary`'s to maintain, and
deleting a term to silence a finding would destroy the definition rather than move it.

A (D) finding may tempt you to fix the corpus instead of the ledger — rewriting the ADR section the ledger points at, so the pointer becomes true again. Do not. The ledger is bookkeeping and yours to correct; an ADR is a decision record in the maintainer's voice, and a detector editing one would convert its own finding into the record of a decision nobody made. Surface that as the user's task.

After writes, run `make md-lint` (or `make md-fix` then `make md-lint`) to verify the edited Markdown.

## Step 5. Closing Report (Japanese)

```text
back-prop 完了（scope: <X>, 種別: A/B/C）

[domain]   findings <N>, README 更新 <X>, Skill 簡略化 <Y>, コード修正委任 <Z>, 無視 <W>
[usecase]  ...
[controller] ...
[infra]    ...
[pkg]      ...

総 finding: <sum>, README/Skill 書き込み: <sum>, コード修正委任: <sum>
最終 make md-lint OK
```

- 検出は read-only detector subagent に委譲。書き込みは integrator が per-item 承認後に単一スレッドで実施
- 実装コードへの書き込みは一切なし（surface のみ、コード修正は user 作業）
- commit / push なし

## AI Modification Scope

- 読み込み: 各 layer の README + 実装 + 関連 skill 本体（detector subagent が実施）。(E) 時は
  `docs/spec/glossary.md` と `.agents/glossary-drift/exclusions.yaml` も読む（参照のみ）
- 書き込み: **integrator のみ**、user の per-item 承認 + 理由明示 + draft 提示の後に、layer README / 関連 skill `SKILL.md` / (D) 時のみ `.agents/ddd-audit/pattern-ledger.yaml` へ。detector subagent は一切書き込まない。
- 触らない: 実装コード、生成物、`AGENTS.md`、ADR 本文、`docs/spec/**`、`.agents/glossary-drift/exclusions.yaml`

## Constraints

- ❌ detector を逐次起動（必ず1メッセージ内で複数 Agent 呼び出し＝並列）
- ❌ scope + 種別 `AskUserQuestion` をスキップ
- ❌ detector に書き込み / `AskUserQuestion` をさせる（read-only surface 専用）
- ❌ user 承認なしの README / skill 自動更新
- ❌ 理由を述べずに draft を実行
- ❌ 実装コードへの書き込み（surface のみ、修正は user）
- ❌ recurring threshold 3 未満の (B) pattern を surface（detector 側で抑止、integrator も respect）
- ❌ (D) の解消として ADR / README 本文を書き換える（台帳側を直す。正本の変更は user 作業）
- ❌ (D) で Evans 原義への忠実性を判定する（`ddd-audit` の担当）
- ❌ (E) の E2（ADR / `docs/rules.md` への漏れ）を承認対象に載せる / 書き換える
- ❌ (E) の解消として `docs/spec/glossary.md` を編集する（用語表は `glossary` の担当）
- ❌ (E) を `.agents/glossary-drift/exclusions.yaml` へ追記して黙らせる（除外の宣言は user の判断）
- ✅ Japanese aggregated report
- ✅ Fan out only touched layers (changed-files mode)
- ✅ per-layer detector / skill が独立 standalone 動作可能であることを維持
- ✅ Categories propagation to all detectors
- ✅ 書き込みは integrator 単一スレッドのみ（並列 detector は read-only）
- ✅ README が canonical の前提（README > Code > SKILL）

## Checklist

- [ ] Scope + 種別を `AskUserQuestion` で確認
- [ ] Layer + per-layer ファイルリスト解決（changed files / full repo / specific layer）
- [ ] touched layer の `drift-detector-*` を **1メッセージ内で並列起動**（scope / files / baseRef / categories を渡す）
- [ ] (D) 選択かつコーパス変更ありなら `drift-detector-ddd` を同じメッセージで並列起動
- [ ] (E) 選択かつ散文変更ありなら `drift-detector-glossary` を同じメッセージで並列起動（用語表が無ければスキップして理由を述べる）
- [ ] (E) の E2 は承認対象に載せず報告のみ
- [ ] 各 detector が README + 実装 + skill を読み (A)(B)(C) を read-only 検出
- [ ] 集約サマリ出力（決定前のチェックポイント）
- [ ] integrator が per-item で reasoning + user 承認 + draft + 最終確認 + 書き込み（README / skill のみ）
- [ ] 実装コードへの書き込みなし
- [ ] 最終 make md-lint OK
- [ ] commit / push なし
