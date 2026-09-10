---
name: arch-check
description: >-
  Integrator skill for architectural compliance checks across the onion layers. Confirms scope (changed files vs full repo vs one layer) plus an optional `// TODO:` hand-off opt-in, then audits each touched layer against its README's rules and reports findings grouped by layer in Japanese. Under a diff-independent scope (whole repo or a chosen layer) it also runs the domain-modeling lens, which makes it the entry point for a modeling question with no diff behind it — 「この集約境界は今のままで妥当か」「この規則はこのエンティティが持つべきか」「Order と Shipment の境界は意味的に整合するか」 — since `impl-review` owns that lens only for a change. To audit a single layer, run this integrator and pick that layer in the scope question. Do NOT use it to review a diff's correctness or security (`impl-review`), or to check documentation drift (`back-prop`).
---

# Arch Check

Integrator for layer-scoped architectural compliance checks. Fans out 1〜5 per-layer **read-only auditor subagents** in parallel based on scope, then aggregates.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- About to commit / push and want comprehensive layer-compliance check across all touched layers.
- Reviewing a feature branch that touches multiple layers.
- CI gate before merge.

To audit a single layer, run this integrator and pick that layer in the scope question (「特定 layer のみ」). Use it when 1+ layers are touched and you want parallel fan-out + a single aggregated report.

Do NOT use this skill for:

- Style / formatting — `make go-fix` / `make go-lint`.
- General code review — `/review` / `/ultrareview` / `impl-review`.
- Spec validation — `verify-spec`.

## Contract

| | |
| --- | --- |
| **Owns** | コード vs リポジトリ自身の規約（層別 5 auditor）、および差分を持たないドメインモデリング監査 |
| **Never** | 一般的な好みを違反として扱う / 実装を修正する（`// TODO:` 注記のみ） |
| **Starts when** | 監査対象の差分または構造が存在するとき |
| **Stops when** | 規約の出所を特定できないとき（README 不在は `back-prop` の領分） |

## Architecture: parallel auditor subagents

Detection is delegated to five **read-only worker subagents** under `.claude/agents/`, one per layer. The integrator runs them concurrently via the Agent tool (`subagent_type`), so per-layer audits no longer execute sequentially:

| Auditor subagent | Layer | Lean A enforcement |
| --- | --- | --- |
| `arch-auditor-domain` | `internal/domain/**` | entity ↔ SQL カラム soft 対応（method 形式 / VO ラップは逸脱許容、suggestion のみ） |
| `arch-auditor-usecase` | `internal/usecase/**` | thin orchestrator + boundary 利用 + tx 境界 |
| `arch-auditor-controller` | `internal/controller/**` | handler pure template + operationId ↔ method 一致 |
| `arch-auditor-infra` | `internal/infrastructure/**` | Repository pure template + sqlc gen soft 対応（multi-query / switch dispatch / JOIN 許容）+ pgerror 利用 |
| `arch-auditor-pkg` | `pkg/**` | `internal/` 依存禁止 + framework 非依存 |

One further auditor rides along, keyed to the change rather than to a layer:

| Auditor subagent | Trigger | 検査内容 |
| --- | --- | --- |
| `ddd-origin-auditor` | `internal/domain/**` または `docs/adr/**` / `internal/**/README.md` が touched | 層2（ADR / README）の DDD 解釈と Evans 原義との差異、および逸脱宣言の有無 |
| `ddd-modeling-reviewer` | `internal/domain/**` が in scope **かつ** スコープが「リポジトリ全体」または「特定 layer」 | 集約境界とトランザクション境界、規則の所属（entity / VO / Domain Service / Usecase）、集約間参照の規律、`docs/spec/glossary.md` に対するユビキタス言語 |

その判定は他の 5 つと性質が違う。5 つはリポジトリ自身の規則にコードを照らすが、これはリポジトリの外
（Evans）を物差しにして**文書のほう**を見る。したがって出力は `violation` ではなく 3 値のフラグであり、
裁定は含まない。深掘りは専用スキル `ddd-audit` の担当で、ここでは変更に関係するパターンだけを見る
`quick` モードで回す — 毎回 全パターン × 全コーパスを走らせると arch-check が重くなり、結局
誰も回さなくなるからである。

`ddd-modeling-reviewer` の trigger 条件にある**スコープ制限は意図的**である。このレンズは
`impl-review` が tier 1 として既に所有しており、差分レビューではそちらが回る。両方が同じ差分に対して
回れば同一 finding が二重に出て、しかもどちらの指摘なのか読者には区別できない。一方で `impl-review`
の 3 つのスコープはすべて差分前提なので、**差分が無い問い**——「この集約境界は今のままで妥当か」——には
入口が存在しない。ここがその入口であり、差分があるときは黙って `impl-review` に譲る。

同じ理由で、このレンズは 5 つの層 auditor とも重ならない。`arch-auditor-domain` が見るのは機械的規則
（禁止 import、`time.Now()`、entity ↔ SQL 対応）であって、境界がそもそも正しい場所に引かれているか
という問いは誰も受けていない。

These auditors are the per-layer audit workers. They are **strictly read-only** (no TODO writes) so that running five in parallel never produces concurrent writes to source. Any source write (TODO hand-off) is performed by **this integrator**, single-threaded, after aggregation.

## First Step: Confirm Scope + TODO opt

This skill **MUST call `AskUserQuestion` immediately after invocation** with 2 questions (batched).

Default-detect scope by checking branch vs base (resolve the base exactly as Step 1 does):

- 未マージのコミットあり → 「変更ファイルのみ」を既定
- main / release/* / no diff → 「リポジトリ全体」を既定

```text
質問 1: どのスコープでアーキ検査を実行しますか？
選択肢:
  - 変更ファイルのみ（ベースブランチとの diff、touched layer のみ fan-out）
  - リポジトリ全体（5 auditor 全部 fan-out）
  - 特定 layer のみ（layer を続けて指定）
  - キャンセル

質問 2: suggestion 検出箇所に TODO hand-off コメントを追加しますか？
選択肢:
  - 追加する（既定） — 集約後に integrator が逸脱位置へ `// TODO:` を書き込む（人間に解決を委ねる）
  - 追加しない（read-only） — レポートのみ、コード一切触らない
```

TODO opt は domain / controller / infra の suggestion 検出にのみ適用。usecase / pkg は violation 中心なので TODO 書き込み対象外（opt 関係なく read-only）。

## Step 1. Resolve Scope to Layers + File Lists

For "changed files" mode:

```sh
BASE=$(gh pr view --json baseRefName -q '.baseRefName' 2>/dev/null || make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }
git diff --name-only "origin/${BASE}...HEAD" -- '*.go' | grep -vE '\.gen\.go$|\.sql\.go$|_mock\.go$|_test\.go$' || true
```

An existing pull request's `baseRefName` stays the authority — the audited diff has to be the one the
PR shows. With no PR, `make base-branch` resolves the latest release line from `origin`'s live state;
`gh repo view --json defaultBranchRef` is not the fallback, because the GitHub default branch keeps
answering with an earlier release line. The emptiness check is not ceremony: the `|| true` on the next
line would turn a failed resolution into an empty file list, and an empty file list is
indistinguishable from a clean audit.

Map to layers by path prefix, and **keep the per-layer file list** (you pass it to each auditor so the auditor does not re-resolve git):

| Path prefix | Auditor subagent |
| --- | --- |
| `internal/domain/` | `arch-auditor-domain` |
| `internal/usecase/` | `arch-auditor-usecase` |
| `internal/controller/` | `arch-auditor-controller` |
| `internal/infrastructure/` | `arch-auditor-infra` |
| `pkg/` | `arch-auditor-pkg` |

Other `internal/` paths (cli / system / di / config 等) → 報告のみ、専用 auditor 無し（CLAUDE.md guidance を直接適用）。

Separately, resolve the **DDD corpus** changed in the same diff (this one is not `*.go`-filtered — the corpus is prose):

```sh
git diff --name-only "origin/${BASE}...HEAD" -- 'docs/adr/*.md' 'docs/rules.md' 'docs/architecture.md' '*/README.md' || true
```

If that list is non-empty, **or** `internal/domain/` is among the touched layers, add `ddd-origin-auditor` to the fan-out. Read the pattern list from `.agents/ddd-audit/pattern-ledger.yaml` and select the patterns whose `interpreted_by` points at a changed file, plus every pattern still `unexamined` or `uninterpreted` — a pattern nobody has interpreted has no pointer to match, and those are exactly the gaps worth surfacing.

For "full repo" mode: fan out all 5 auditors (each resolves its own full-layer file list, or pass `scope=full`). Do **not** run the full DDD sweep from here — a whole-corpus pass over every pattern belongs to `ddd-audit`; say so in the report instead of silently doing a partial one.

For "specific layer" mode: ask user which layer(s), then fan out only those.

If no layers detected (changed-files mode with no Go changes) → exit cleanly with message.

## Step 2. Run `make go-lint` ONCE (shared baseline)

`make go-lint` covers the whole repo, so run it a single time and share the output with every auditor — never let each auditor re-run it (that would be N concurrent full-repo lints):

```sh
make go-lint 2>&1 | tee /tmp/arch-check-lint.out
```

If lint fails for reasons unrelated to the audited layers, surface the verbatim failure and stop (do not fan out auditors against a broken baseline).

## Step 3. Fan Out Auditor Subagents IN PARALLEL

For each layer in scope, spawn its auditor with the **Agent tool**, all in **a single message with multiple tool calls** so they run concurrently. Pass each auditor:

- `scope` — `changed` or `full`
- `files` — the pre-resolved newline list of in-scope `.go` files for that layer (from Step 1)
- `baseRef` — the base branch (fallback if the auditor must resolve files itself)
- `lintOutput` — `/tmp/arch-check-lint.out` (the shared run from Step 2 — auditors filter this instead of re-running lint)

Example (conceptual — adjust the touched set to the resolved layers):

```text
Agent(subagent_type="arch-auditor-domain",     prompt=<scope/files/baseRef/lintOutput for domain>)
Agent(subagent_type="arch-auditor-usecase",    prompt=<...usecase>)
Agent(subagent_type="arch-auditor-controller", prompt=<...controller>)
Agent(subagent_type="arch-auditor-infra",      prompt=<...infra>)
Agent(subagent_type="arch-auditor-pkg",        prompt=<...pkg>)
Agent(subagent_type="ddd-origin-auditor",      prompt=<pattern=<id>, mode=quick, files=<changed corpus>>)   # 選択パターンごとに 1 つ
Agent(subagent_type="ddd-modeling-reviewer",   prompt=<scope/files for domain>)   # scope が full / layer のときだけ
```

`ddd-origin-auditor` は 1 起動＝1 パターンなので、選択パターンが複数あればその数だけ同じメッセージ内に並べる。

`ddd-modeling-reviewer` は差分ではなく**現状のコード**を読む。渡すのは domain の解決済みファイル一覧で、
`baseRef` は渡さない — 差分を渡すとこのレンズは `impl-review` の縮小版に退化する。scope が
「変更ファイルのみ」のときは起動せず、レポートにその旨を 1 行書く（差分レビューは `impl-review` の担当、
と読者に分かる形で）。

Each auditor's final message **is** its findings (Japanese, structured). Collect them with their layer label. An auditor that returns "違反なし" contributes an empty section.

> If the `arch-auditor-*` subagents cannot be spawned in the current environment, follow each `arch-auditor-<layer>.md` procedure inline instead — the integrator still performs the TODO write single-threaded afterward.

## Step 4. Aggregate Report

Combine all auditor findings into a single Japanese report:

```text
arch-check 統合結果（スコープ: <scope>）

[lint baseline]
  make go-lint: OK / FAIL (<n>件)

[domain] violations: N, suggestions: K
  internal/domain/foo/bar.go:12 ...

[usecase] violations: N, suggestions: K
  ...

[controller] violations: N, suggestions: K (lean A)
  ...

[infra] violations: N, suggestions: K (lean A)
  ...

[pkg] violations: N, suggestions: K
  ...

[ddd-origin] 差異あり: N, 逸脱宣言あり: K, 差異なし: M（quick / 対象パターン: <ids>）
  <pattern> — Evans 原義: <前提> / 根拠: <file:line>
  ※ フラグのみ。裁定はしない。全パターンの棚卸しは `ddd-audit` で実行

総計: violations <sum>, suggestions <sum>, DDD 差異 <sum>
```

`[ddd-origin]` の件数は violations の合計に足さない。層2 の差異は「直すべき違反」ではなく
「意図的な逸脱か見落としかを人間が判定する材料」であり、両者を足すとその区別が消える。

If all clean:

```text
arch-check 統合結果（スコープ: <scope>）
全 layer で違反は検出されませんでした（チェック済み: <layer list>）。
```

## Step 5. TODO Hand-off Insertion (integrator-side, opt-in)

Auditor subagents are read-only and never write. If the user opted into "TODO 追加" at Step 0, **the integrator** now inserts the `// TODO:` hand-off comments — single-threaded, so there is no write contention. Apply only to **domain / controller / infra** `suggestion`-level findings (usecase / pkg are out of scope):

For each such suggestion (`file:line`):

1. Locate the deviation point in source (struct field line / handler method line / Repository method line).
2. Check the 3 lines immediately above for an existing comment block → if found, **skip** (de-dup).
3. Otherwise insert a `// TODO:` comment immediately before the deviation describing what was detected and listing resolution options for the human. Standard `// TODO:` prefix only — no AI-identifying prefix (`// TODO(arch-check):` 等は禁止).

The comment is a **hand-off baton**, not AI's judgment: the AI does not decide whether the deviation is intentional. `violation`-level findings do NOT get TODO comments (those must be fixed, not deferred). Report the add / skip counts:

```text
TODO hand-off: 追加 <sum> 件, スキップ <sum> 件（既存コメント）
```

If the user opted "TODO 追加なし", skip this step entirely (strictly read-only run).

## Step 6. Closing

- 検出は read-only auditor subagent に委譲。integrator の唯一の書き込みは opt-in 時の TODO hand-off コメントのみ
- `/commit` から chain 時は violations > 0 で non-zero status
- 単独実行時は情報的、exit 0
- 自動修正なし（violation の自動 fix はしない）

## AI Modification Scope

- 読み込み: 各 layer の README + 関連ファイル（auditor subagent が実施）、`make go-lint`（integrator が1回実行、`/tmp/arch-check-lint.out`）
- 書き込み: user opt 時のみ、`internal/{domain,controller,infrastructure}/**/*.go` の suggestion 位置への `// TODO:` hand-off コメント追加（**integrator が単一スレッドで実施**）。auditor subagent は一切書き込まない。

## Constraints

- ❌ auditor を逐次起動する（必ず1メッセージ内で複数 Agent 呼び出し＝並列）
- ❌ 各 auditor に `make go-lint` を再実行させる（共有 `lintOutput` を渡す）
- ❌ Skip scope + TODO opt `AskUserQuestion`
- ❌ Heuristic findings (handler bloat 等) を hard violation 扱い（auditor が `suggestion` ラベル付け、integrator は respect）
- ❌ `[ddd-origin]` の差異を violation に合算する / TODO hand-off の対象にする（裁定しない検出なので defer 先も無い）
- ❌ arch-check から全パターン × 全コーパスの DDD 監査を回す（`quick` のみ。全量は `ddd-audit`）
- ❌ violation 位置への TODO 書き込み（fix 必須、defer 不可）
- ❌ TODO に AI 識別 prefix を使う（`// TODO:` のみ）
- ❌ 既存コメントの上書き（3 行以内に既存あれば skip）
- ✅ Japanese aggregated report
- ✅ Fan out only touched layers (changed-files mode で効率化)
- ✅ Per-layer auditor / skill が独立 standalone 動作可能であることを維持
- ✅ TODO 書き込みは integrator 単一スレッドのみ（並列 auditor は read-only）

## Checklist

- [ ] Scope + TODO opt confirmed via `AskUserQuestion`
- [ ] Layer detection + per-layer file list resolved from changed files / full repo
- [ ] `make go-lint` を1回だけ実行し `/tmp/arch-check-lint.out` に保存
- [ ] Touched layers の `arch-auditor-*` を **1メッセージ内で並列起動**（scope / files / baseRef / lintOutput を渡す）
- [ ] domain / ADR / README が touched なら `ddd-origin-auditor` を同じメッセージで並列起動（`quick`、選択パターンごとに1つ）
- [ ] 各 auditor が自身の README + lean A 規則を適用（read-only）
- [ ] Aggregated Japanese report 出力
- [ ] TODO hand-off は opt-in 時のみ integrator が単一スレッドで実施（domain / controller / infra の suggestion、既存コメントは skip）
- [ ] No commit / push
