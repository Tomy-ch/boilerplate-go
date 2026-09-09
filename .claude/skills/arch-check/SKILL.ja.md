> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Arch Check

layer 別アーキ適合性チェックの統合スキル。scope に応じて 1〜5 の per-layer **read-only auditor サブエージェント**を並列 fan-out し、集約する。

## 使うとき

- commit / push 直前に touched layer 全体の適合性チェック
- 複数 layer に跨る feature ブランチのレビュー
- マージ前の CI gate

単一 layer だけ検査したい時はこの統合スキルを実行し、scope 質問で「特定 layer のみ」を選ぶ。1+ layer に跨り並列 fan-out + 集約レポートが欲しい時もこの統合スキルを使う。

以下の用途には使いません:

- formatting / style — `make go-fix` / `make go-lint`
- general code review — `/review` / `/ultrareview` / `impl-review`
- spec validation — `verify-spec`

## 契約

| | |
| --- | --- |
| **所管** | コード vs リポジトリ自身の規約（層別 5 auditor）、および差分を持たないドメインモデリング監査 |
| **しないこと** | 一般的な好みを違反として扱う / 実装を修正する（`// TODO:` 注記のみ） |
| **開始条件** | 監査対象の差分または構造が存在するとき |
| **停止条件** | 規約の出所を特定できないとき（README 不在は `back-prop` の領分） |

## アーキテクチャ: 並列 auditor サブエージェント

検出は `.claude/agents/` 配下の **read-only ワーカーサブエージェント**（layer ごとに1つ）へ委譲。統合スキルは Agent tool（`subagent_type`）でこれらを並列起動するため、per-layer 監査はもう逐次実行されない:

| auditor サブエージェント | layer | lean A 強制 |
| --- | --- | --- |
| `arch-auditor-domain` | `internal/domain/**` | entity ↔ SQL カラム soft 対応（method 形式 / VO ラップは逸脱許容、suggestion のみ） |
| `arch-auditor-usecase` | `internal/usecase/**` | thin orchestrator + boundary 利用 + tx 境界 |
| `arch-auditor-controller` | `internal/controller/**` | handler pure template + operationId ↔ method 一致 |
| `arch-auditor-infra` | `internal/infrastructure/**` | Repository pure template + sqlc gen soft 対応（multi-query / switch dispatch / JOIN 許容）+ pgerror 利用 |
| `arch-auditor-pkg` | `pkg/**` | `internal/` 依存禁止 + framework 非依存 |

もう 1 つ、層ではなく変更内容に紐づく auditor が相乗りする:

| Auditor subagent | 起動条件 | 検査内容 |
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

これらの auditor は層別の監査ワーカー。**厳密に read-only**（TODO 書き込みなし）なので、5並列実行してもソースへの同時書き込みが発生しない。ソース書き込み（TODO hand-off）は集約後に**この統合スキルが単一スレッドで**実施する。

## 最初のステップ: scope + TODO opt 確認

`AskUserQuestion` を起動直後に 2 質問 batched で呼ぶ。

既定検出 scope: ブランチ vs base（base の解決は Step 1 と同じ手順で行う）:

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

## Step 1. scope を layer + ファイルリストに解決

「変更ファイルのみ」モード:

```sh
BASE=$(gh pr view --json baseRefName -q '.baseRefName' 2>/dev/null || make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }
git diff --name-only "origin/${BASE}...HEAD" -- '*.go' | grep -vE '\.gen\.go$|\.sql\.go$|_mock\.go$|_test\.go$' || true
```

PR が既にあるならその `baseRefName` が正である。検査する diff は PR が見せている diff と一致していな
ければならない。PR が無い場合は `make base-branch` が `origin` の実状態から最新のリリースラインを解決
する。fallback に `gh repo view --json defaultBranchRef` を使ってはならない。GitHub のデフォルトブランチ
は前のリリースラインを指したまま答え続けるためである。空チェックは形式ではない。解決に失敗すると次行の
`|| true` がそれを空のファイルリストへ変えてしまい、空のファイルリストは「検査して何も無かった」と
区別が付かない。

path prefix で layer マッピングし、**per-layer のファイルリストを保持**（各 auditor へ渡し、auditor が git を再解決しないようにする）:

| path prefix | auditor サブエージェント |
| --- | --- |
| `internal/domain/` | `arch-auditor-domain` |
| `internal/usecase/` | `arch-auditor-usecase` |
| `internal/controller/` | `arch-auditor-controller` |
| `internal/infrastructure/` | `arch-auditor-infra` |
| `pkg/` | `arch-auditor-pkg` |

その他 `internal/` パス（cli / system / di / config 等） → 報告のみ、専用 auditor 無し（CLAUDE.md guidance を直接適用）。

これとは別に、同じ diff で変更された **DDD コーパス**を解決する（こちらは `*.go` で絞らない。コーパスは散文である）:

```sh
git diff --name-only "origin/${BASE}...HEAD" -- 'docs/adr/*.md' 'docs/rules.md' 'docs/architecture.md' '*/README.md' || true
```

この一覧が空でない、**または** touched layer に `internal/domain/` が含まれるなら、fan-out に `ddd-origin-auditor` を加える。パターン一覧は `.agents/ddd-audit/pattern-ledger.yaml` から読み、`interpreted_by` が変更ファイルを指すパターンに加えて `unexamined` / `uninterpreted` のパターンをすべて選ぶ — 誰も解釈していないパターンは一致するポインタを持たず、しかもそれこそが表に出す価値のある欠落だからである。

「リポジトリ全体」モード: 5 auditor 全部 fan-out（各自が full-layer ファイルリストを解決、または `scope=full` を渡す）。ただし全量の DDD 掃引は**ここからは回さない** — 全パターン × 全コーパスは `ddd-audit` の担当であり、黙って部分監査で済ませずレポートにその旨を書く。

「特定 layer のみ」モード: layer を user に追加質問、該当のみ fan-out。

layer 検出なし（changed-files で Go 変更無し） → 明示メッセージで exit。

## Step 2. `make go-lint` を1回だけ実行（共有ベースライン）

`make go-lint` はリポジトリ全体を対象とするため1回だけ実行し、出力を全 auditor で共有する。各 auditor に再実行させない（N 並列で full-repo lint が重複するため）:

```sh
make go-lint 2>&1 | tee /tmp/arch-check-lint.out
```

監査対象 layer と無関係な理由で lint が失敗したら、verbatim 出力を提示して停止（壊れたベースラインに対して auditor を fan-out しない）。

## Step 3. auditor サブエージェントを並列 fan-out

scope 内の各 layer について、その auditor を **Agent tool** で起動。**1メッセージ内に複数 tool 呼び出し**を並べて並列実行する。各 auditor に渡す:

- `scope` — `changed` or `full`
- `files` — その layer の in-scope `.go` ファイル改行リスト（Step 1 で解決済み）
- `baseRef` — base ブランチ（auditor が自前解決する場合の fallback）
- `lintOutput` — `/tmp/arch-check-lint.out`（Step 2 の共有実行 — auditor はこれを filter、lint 再実行しない）

例（概念 — 解決された layer 集合に合わせて調整）:

```text
Agent(subagent_type="arch-auditor-domain",     prompt=<domain の scope/files/baseRef/lintOutput>)
Agent(subagent_type="arch-auditor-usecase",    prompt=<...usecase>)
Agent(subagent_type="arch-auditor-controller", prompt=<...controller>)
Agent(subagent_type="arch-auditor-infra",      prompt=<...infra>)
Agent(subagent_type="arch-auditor-pkg",        prompt=<...pkg>)
Agent(subagent_type="ddd-origin-auditor",      prompt=<pattern=<id>, mode=quick, files=<変更コーパス>>)   # 選択パターンごとに 1 つ
Agent(subagent_type="ddd-modeling-reviewer",   prompt=<scope/files for domain>)   # scope が full / layer のときだけ
```

`ddd-origin-auditor` は 1 起動＝1 パターンなので、選択パターンが複数あればその数だけ同じメッセージ内に並べる。

`ddd-modeling-reviewer` は差分ではなく**現状のコード**を読む。渡すのは domain の解決済みファイル一覧で、
`baseRef` は渡さない — 差分を渡すとこのレンズは `impl-review` の縮小版に退化する。scope が
「変更ファイルのみ」のときは起動せず、レポートにその旨を 1 行書く（差分レビューは `impl-review` の担当、
と読者に分かる形で）。

各 auditor の最終メッセージ**が** findings（日本語・構造化）。layer ラベル付きで収集。「違反なし」を返した auditor は空セクション扱い。

> `arch-auditor-*` サブエージェントを起動できない環境では、各 `arch-auditor-<layer>.md` の手順を本文がインラインで実行する — TODO 書き込みは集約後に integrator が単一スレッドで行う。

## Step 4. 集約レポート

全 auditor findings を 1 つの日本語レポートに集約:

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

全 clean:

```text
arch-check 統合結果（スコープ: <scope>）
全 layer で違反は検出されませんでした（チェック済み: <layer list>）。
```

## Step 5. TODO hand-off 挿入（integrator 側、opt-in）

auditor サブエージェントは read-only で一切書き込まない。Step 0 で「TODO 追加」を選んだ場合、**integrator** が今ここで `// TODO:` hand-off コメントを挿入する — 単一スレッドなので書き込み競合は発生しない。対象は **domain / controller / infra** の `suggestion` レベル findings のみ（usecase / pkg は対象外）:

各 suggestion（`file:line`）について:

1. ソースの逸脱位置を特定（struct field 行 / handler method 行 / Repository method 行）。
2. 直上 3 行に既存コメントブロックがあれば **skip**（de-dup）。
3. なければ逸脱直前に `// TODO:` コメントを挿入。検出内容と人間向けの解決オプションを記述。標準 `// TODO:` prefix のみ — AI 識別 prefix（`// TODO(arch-check):` 等）は禁止。

コメントは AI の判断ではなく **hand-off baton**: 逸脱が意図的かを AI は判定しない。`violation` レベルには TODO を付けない（fix 必須、defer 不可）。追加 / skip 件数を報告:

```text
TODO hand-off: 追加 <sum> 件, スキップ <sum> 件（既存コメント）
```

「TODO 追加なし」を選んだ場合はこの Step を完全にスキップ（厳密 read-only 実行）。

## Step 6. クロージング

- 検出は read-only auditor サブエージェントに委譲。integrator の唯一の書き込みは opt-in 時の TODO hand-off コメントのみ
- `/commit` から chain 時は violations > 0 で non-zero status
- 単独実行時は情報的、exit 0
- 自動修正なし（violation の自動 fix はしない）

## AI 修正スコープ

- 読み込み: 各 layer の README + 関連ファイル（auditor サブエージェントが実施）、`make go-lint`（integrator が1回実行、`/tmp/arch-check-lint.out`）
- 書き込み: user opt 時のみ、`internal/{domain,controller,infrastructure}/**/*.go` の suggestion 位置への `// TODO:` hand-off コメント追加（**integrator が単一スレッドで実施**）。auditor サブエージェントは一切書き込まない。

## 制約事項

- ❌ auditor を逐次起動（必ず1メッセージ内で複数 Agent 呼び出し＝並列）
- ❌ 各 auditor に `make go-lint` を再実行させる（共有 `lintOutput` を渡す）
- ❌ scope + TODO opt `AskUserQuestion` をスキップ
- ❌ heuristic findings (handler bloat 等) を hard violation 扱い（auditor が `suggestion` ラベル付け、integrator は respect）
- ❌ `[ddd-origin]` の差異を violation に合算する / TODO hand-off の対象にする（裁定しない検出なので defer 先も無い）
- ❌ arch-check から全パターン × 全コーパスの DDD 監査を回す（`quick` のみ。全量は `ddd-audit`）
- ❌ violation 位置への TODO 書き込み（fix 必須、defer 不可）
- ❌ TODO に AI 識別 prefix を使う（`// TODO:` のみ）
- ❌ 既存コメントの上書き（3 行以内に既存あれば skip）
- ✅ 日本語集約レポート
- ✅ touched layer のみ fan-out（changed-files モードで効率化）
- ✅ per-layer auditor / skill が独立 standalone 動作可能であることを維持
- ✅ TODO 書き込みは integrator 単一スレッドのみ（並列 auditor は read-only）

## チェックリスト

- [ ] scope + TODO opt を `AskUserQuestion` で確認
- [ ] 変更ファイル or full repo で layer + per-layer ファイルリスト解決
- [ ] `make go-lint` を1回だけ実行し `/tmp/arch-check-lint.out` に保存
- [ ] touched layer の `arch-auditor-*` を **1メッセージ内で並列起動**（scope / files / baseRef / lintOutput を渡す）
- [ ] domain / ADR / README が touched なら `ddd-origin-auditor` を同じメッセージで並列起動（`quick`、選択パターンごとに1つ）
- [ ] 各 auditor が自身の README + lean A 規則を適用（read-only）
- [ ] 集約日本語レポート出力
- [ ] TODO hand-off は opt-in 時のみ integrator が単一スレッドで実施（domain / controller / infra の suggestion、既存コメントは skip）
- [ ] commit / push なし
