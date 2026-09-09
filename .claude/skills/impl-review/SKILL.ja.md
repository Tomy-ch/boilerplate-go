> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Impl Review

実装者とは**別モデル**で回す、ローカルの敵対的・低バイアスなコードレビュー。Copilot もクラウド `/code-review` も使わない。実装者自身のモデルには盲点があり、その盲点を別モデルで拾うのが本質。`/code-review` の finder → verify パターンを下敷きにしつつ、すべてローカルで完結させ、さらにモック単体テストでは構造的に届かない **ランタイム（curl + o11y）検証** を足す。既定では、verifier を通過した CONFIRMED / PLAUSIBLE の指摘を、ブランチの PR に各指摘の行へアンカーした**インラインレビューコメント**として投稿する（`--no-comment` でオプトアウト。PR が無ければローカルレポートのみにフォールバック）。

## 使うとき

- commit / PR 前に、実装者のモデル単独では出ないセカンドオピニオンが欲しいとき。
- 複数 layer に跨る変更で、モックテストは通るが DI / middleware / 実 DB 挙動が未検証のとき。
- バグ・認証/IDOR・レイヤ違反に絞った敵対的パスをかけたいとき。

以下には使わない:

- formatting / style — `make go-fix` / `make go-lint`
- 網羅的なレイヤ適合監査 — `arch-check`（本スキルの `architecture` lens は高シグナルな違反のみ）
- spec 検証 — `verify-spec`
- 修正の適用 — 本スキルはソースに対し read-only。指摘するだけで直すのはユーザー。
- テストの監査（`/test-review`）・コメントの監査（`/comment-sweep`）— 下位ステップではなく対等な別スキル

## 契約

| | |
| --- | --- |
| **所管** | 変更そのもの（architecture / ddd-modeling / security / correctness / runtime-gap / type-design） |
| **しないこと** | テスト観点（`test-review`）/ コメント観点（`comment-sweep`）/ 他スキルの呼び出し |
| **開始条件** | レビュー可能な差分とその意図が存在するとき |
| **停止条件** | tier 1 の finding が人間の設計判断を要するとき |

## 中核アイデア — reviewer ≠ implementer

バイアス低減が設計上の制約であって、おまけではない。よって reviewer は **コードを書いた者とは別モデルの subagent** として動く:

- reviewer エージェント（`adversarial-reviewer` / `review-verifier`）は frontmatter で既定 **`sonnet`**。通常の Opus 実装者と異なる。
- **reviewer のモデルは Step 0 でユーザーが選ぶ。** 選択肢は `fable`（Fable 5）/ `sonnet` / `opus` / `haiku`、加えて実装者と異なるモデルへ解決される *自動* 既定。選んだモデルを `Agent` ツールの `model` 引数で各 reviewer subagent に渡す（この引数はエージェント定義の `sonnet` 既定より優先）— 深さなら `opus`、安価な発散なら `haiku`、独立した新しい視点なら `fable`。
- **オーケストレーターは reviewer ≠ implementer を必ず保証する。** ユーザーが本セッションの実装者と同一モデルを選んだ場合は、別モデルによるバイアス低減が損なわれる旨を警告し、確認してから進める。reviewer と implementer を無言で同一モデルにしない。
- reviewer は **read-only**（エージェント定義に Edit/Write 権限なし）— finding を返すだけであり、本スキルはソースを一切書き換えない。何を直すかはレポートを読んだユーザーの判断である。

**本スキルが監査するのは変更そのものだけである。** テスト lens もコメント lens も持たず、他のスキルを起動もしない。それらは `/test-review` と `/comment-sweep` の主題であり、`AGENTS.md` の Review Phase Protocol のとおり、本スキルの横で個別に依頼され個別に走る。次のスキルの実行を提案するレビュースキルは、3 つの主題を独立に答えられないものにし、1 つのスキルの質問のずれが、そこを通った全ての流れから残り 2 つを黙って落とす。

## 評価順 — 指摘は集めるだけでなく順位を付ける

レビュアーは食い違い、重なり、同じ事実を別の語彙で報告する。順位が無ければレポートは平坦な一覧に
なり、finder が「high」と呼んだだけのコメント指摘が、誤った集約境界より上に出る。Step 2 の表の
階層がその順位である。

| 階層 | lens | 何を決めるか |
| --- | --- | --- |
| 1 | `architecture` / `ddd-modeling` | コードが何で**あるべきか** |
| 2 | `security` / `correctness` | それが**動くか** |
| 3 | `runtime-gap` / 型設計 | 実システムと型の上で**保つか** |

**上位の変更は下位へ伝播するが、下位が上位に働きかけることは原則としてない。** 集約境界を書き直せば、
それに対して検証していた振る舞いも前提ごと消える。逆に命名の些事が境界の変更を正当化することはない。
帰結は 4 つあり、いずれも提案ではなく規則である。

1. **レポートは階層順に並べ、階層内で重大度順にする** — 重大度だけで並べない。階層 3 の `high` は
   architecture の `medium` より下に置く。後者が、下位が語っているコードごと消すかもしれないため。
2. **上位の未決な指摘に依存する下位の指摘は `保留` として出す。** 報告はするが、何を待っているかを書き、
   着手可能なものとして提示しない。上位の決着後に見直すと、多くは消えている。
3. **同じ事実を 2 階層が報告したら、上位の枠組みを残し、下位はその裏付けとして畳む** — 指摘は 1 件。
   1 つの事実に 2 エントリは 2 つの問題に見え、変更のリスクを二重に数える。
4. **同一階層どうしの合致は確度を上げてよいが、下位からの合致は上位の重大度を引き上げない。** 階層 2 の
   2 つの lens が独立に同じ欠陥へ到達したなら、それは強い証拠なので明記する。階層 3 が階層 1 に同意しても
   重大度は動かない（裏付けとして引用するのは構わない）。

**例外はクリティカルさであり、気づくのは担当だが裁定は担当ではない。** 下位の指摘のほうが緊急なことは
ある — `runtime-gap` が露呈させた悪用可能な穴は、アーキテクチャの議論を待たない。下位の指摘が上の階層を
outrank しそうなときは、**黙って並べ替えず、両方を提示してユーザーに問う。** 順位は普通の食い違いを
人間なしで解くために在るのであって、順位を破る指摘こそ人間が見るべき場面である。

## Step 0 — スコープ確認

即座に `AskUserQuestion`。未マージのコミットがあれば「変更ファイルのみ」を既定、なければ作業ツリー / 指定パスを既定。

ベースは次の手順で解決する。レビュー対象は PR が見せている diff と一致していなければならないので、PR が
既にあるならその `baseRefName` が正。PR が無い場合は `make base-branch` が `origin` の実状態から最新の
リリースライン（本リポジトリのベースは常に `release/*`）を解決する。fallback に
`gh repo view --json defaultBranchRef` を使ってはならない。GitHub のデフォルトブランチは前のリリース
ラインを指したまま答え続け、diff が誰も依頼していない 1 世代分の変更まで黙って広がる。

```sh
BASE=$(gh pr view --json baseRefName -q '.baseRefName' 2>/dev/null || make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }
git diff --name-only "origin/${BASE}...HEAD"
```

```text
質問: どの範囲をレビューしますか？
選択肢:
  - 変更ファイルのみ（ベースブランチとの diff）  ← 未マージのコミットがある場合の既定
  - 作業ツリーの未コミット変更（git status の差分）
  - 特定のパス/ファイルを指定
  - キャンセル
```

### reviewer モデルの選択

同じ `AskUserQuestion` 呼び出しの中で（スコープと並べて2つ目の質問として）、reviewer subagent を
どのモデルで動かすかを聞く。既存のティアに加え `fable`（Fable 5）が利用可能:

```text
質問: レビュアーをどのモデルで実行しますか？（バイアス低減のため 実装者 ≠ レビュアー を推奨）
選択肢:
  - 自動（実装者と異なるモデルを既定選択）  ← 既定
  - fable（Fable 5）
  - sonnet
  - opus（深掘り）
  - haiku（安価・高速な発散パス）
```

*自動* は、実装者が `sonnet` でなければエージェント定義の既定（`sonnet`）に、そうでなければ別ティアに
解決する。ユーザーが実装者と同一モデルを選んだ場合は（中核アイデアのとおり）別モデル保証が弱まる旨を
警告し、確認してから進める。選んだモデルは Step 2・Step 3 の各 `Agent` 呼び出しへ `model` 引数で渡す。

**質問は 2 つ、それ以上は無い。** ここにテストの質問もコメントの質問も置かない。それらは `/test-review` と
`/comment-sweep` の主題で、ユーザーが個別に依頼する。畳み込めば、ある主題についての判断が別の主題のために
始めた実行の中に埋まり、しかも残り 2 つを思い出す唯一の入口が本スキルになってしまう。

### フラグ

- `--no-comment` — Step 6 を抑止（PR に投稿せず）ローカルレポートのみ。**既定はオプトアウト**: ブランチに open な PR があれば、このフラグが無い限り Step 6 が残った指摘をインラインコメントとして投稿する。

## Step 1 — コンテキスト収集

最初にレビュー境界を打刻する。ここは他の何もが観測していない瞬間であり、ループはこれを読んで
レビューに費やした時間と実装に費やした時間を分ける。

```sh
.agents/closed-loop/marks.sh reviewStartedAt 2>/dev/null || true
```

- ベース ref を解決しレビュー対象を作る: `git diff <base>...HEAD`（未コミットなら `git diff`）+ 変更ファイル一覧（`git diff --name-only ...`）。
- どの layer/領域が触られたか検出（`internal/controller/**`, `usecase`, `domain`, `infrastructure`, `pkg`, `openapi/**`, `database/**`）。
- **エンドポイント** が触られたか（controller handler か `openapi/**`）— Step 4 を回すかの判定。
- **共有** OpenAPI コンポーネント（複数 operation から参照される `components/*`）が編集されたか — Step 4 を全 consumer に広げる判定。
- diff が **domain 型**（`internal/domain/**/*.go`）に触れたか — 型設計 lens を回すかの判定。

## Step 2 — Finder の fan-out（別モデル、並列）

全 finder を並列起動する（`Agent` 呼び出しを1メッセージにまとめる）。中核アイデアのモデル規則を適用し、Step 0 で選んだ reviewer モデルを各 `Agent` 呼び出しの `model` 引数で渡す（*自動* がエージェント定義の既定に解決するときのみ省略可）。エージェント種別は 2 つ。

- 4 つの **コード lens** は `adversarial-reviewer` を使う。lens ごとに1体、`agentType: "adversarial-reviewer"`、`label` は `find:security` のように。
- **DDD モデリング次元**は専用の `ddd-modeling-reviewer` を使う（`agentType: "ddd-modeling-reviewer"`, `label: "find:ddd"`）。diff が `internal/domain/**` または `internal/usecase/**` に触れた時。この変更がドメインをうまくモデル化できているかを、このリポジトリ自身が書き残した解釈で問う（集約境界とトランザクション境界の一致、規則の配置先、集約間参照の規律、ユビキタス言語、Factory / Repository の意味論）。**階層 1** の lens であり、その指摘は「コードが何であるべきか」を決めるため、下位の階層が動く前に決着させる。Evans の原典を直接の基準にしないこと — それは `ddd-origin-auditor` の担当で、対象も文書でありコードではない。
- **型設計次元**は専用の `type-design-reviewer` を使う（`agentType: "type-design-reviewer"`, `label: "find:type-design"`）。diff が domain 型（`internal/domain/**/*.go`）に触れた時のみ。4 軸ルーブリック（Encapsulation / Invariant Expression / Invariant Usefulness / Invariant Enforcement）で各型を採点し、指摘は suggestion 級（自動修正しない）。

| Finder | 階層 | エージェント | 起動条件 |
| --- | --- | --- | --- |
| `architecture` | 1 | adversarial-reviewer | 常時 |
| `ddd-modeling` | 1 | **ddd-modeling-reviewer** | diff が `internal/domain/**` または `internal/usecase/**` に触れた時 |
| `security` | 2 | adversarial-reviewer | 常時（handler / auth / DTO / `openapi/**` が触られた時は特に） |
| `correctness` | 2 | adversarial-reviewer | 常時 |
| `runtime-gap` | 3 | adversarial-reviewer | controller / DI / `openapi/**` / `database/**` が触られた時 |
| 型設計 | 3 | **type-design-reviewer** | diff が domain 型（`internal/domain/**/*.go`）に触れた時 |

各 `adversarial-reviewer` プロンプトに必ず含める: lens 名 + その定義、ベース ref + 変更ファイル一覧 + diff、`CLAUDE.md` / 該当 `README.md` / OpenAPI spec / migrations へのポインタ。

**ここのどの lens もテストやコメントを監査しない。** 「変更が未テストである」は `/test-review` の、
コメントの内容についての指摘は `/comment-sweep` の主題である。lens がついでに気づいた場合は、補足節に
観察として書き、所管するスキル名を添える — lens を広げて覆おうとするのが、本スキルが今そぎ落とした
2 つを抱え込んだ経緯そのものである。

## Step 3 — 敵対的 verify

全 finding を集め、(file, line, claim) で **dedup**。加えて 2 つの lens が同じ事実を報告した場合は評価順の規則 4 を適用する — 上位の枠組みを残し、下位はその裏付けとして畳み、指摘は 1 件だけ先へ送る。文言一致の dedup ではこれを捕まえられない（同じ欠陥が architecture の違反と型設計の suggestion という別の言葉で届き、両方を通すと二重に数える）。残った finding ごとに `review-verifier` subagent を1体（並列）起動し、単一 finding + ベース ref を渡す。`agentType: "review-verifier"`、`label` は `verify:<file>`、Step 0 で選んだ reviewer `model`（reviewer ≠ implementer は同様に維持）。

- **CONFIRMED** と **PLAUSIBLE** を残す。**REFUTED** は落とす（件数はレポート用に保持）。
- critical/high で単一判定が頼りないときは verifier を 2〜3 体立て多数決。重要な finding ほど単一意見より多様性。

## Step 4 — ランタイム検証（curl + o11y）— エンドポイント時のみ

**Step 1 でエンドポイントが触られた場合のみ** 実行し、subagent ではなく **オーケストレーター（メインセッション）** が行う（対話的 bash・実 DB/状態・ログ読み・ユーザー確認が要るため）。`scaffold-endpoint` Phase 7 に倣う:

1. `make go-test`（モック）は実 Fx グラフを組まず、auth/OpenAPI middleware も DB も通らない。だから本ステージは Step 2 の `runtime-gap` lens が *予測* したものを実地で拾う場。
2. 既知状態の対象行を用意/seed。認証/状態依存の検査は平文/状態を自分で握る行を作る。
3. 対象エンドポイントを `curl`（ローカル認証: `Authorization: Bearer debug:<subject>`）し検証: 正常系 / 主要異常系（404 / 400 / 422）/ — **operation が `security:` 宣言を持つなら** トークン無し ⇒ 401（実際に保護されているか証明）。IDOR 形の finding は *別の* subject で curl し他 subject のリソースに到達できないことを検証。
4. **共有スキーマ波及:** 共有 `components/*` を編集した場合（Step 1）、変更分だけでなく **全 consumer** を curl。spec を `$ref` で grep し各々を叩く。
5. o11y ログを1リクエスト分だけ読む: trace が controller → usecase → infra を貫き、発行 SQL が期待どおりか確認。以降の再確認は再 curl せず o11y で足りる。
6. **破壊ガード:** データを変える curl で復旧手段が `make db-init`（等）しかない場合、実行前にユーザー確認（`CLAUDE.md` 準拠）。検証で作った行は片付ける。

ランタイムで確証した不具合は CONFIRMED として curl/o11y 証拠付きでレポートに統合。

## Step 5 — レポート合成（日本語）

1つの日本語レポートを出す:

```text
## ローカルレビュー結果（reviewer: <model> / implementer: <model>）

スコープ: <base>...HEAD（<N> files） / lens: <実際に走らせた lens のみを列挙>
ランタイム検証: 実施（curl/o11y）/ 対象外（エンドポイント変更なし）
未監査の観点: テスト（/test-review）・コメント（/comment-sweep）は本スキルの対象外

### CONFIRMED（要対応）
- [重大度] タイトル — path:行
  - 問題 / 根拠 / 修正案
  - 検証: verifier 判定（+ 該当すれば curl/o11y 結果）

### PLAUSIBLE（要確認・判断保留）
- ...

### 補足
- REFUTED: <n> 件（finder が挙げたが verifier が否定）
- ランタイム検証でカバーした経路 / スキップした経路
- 他スキルが所管する観点として気づいた点（あれば。所管スキル名を添える）
```

`lens:` 行には実際に走った lens だけを並べる。

**`未監査の観点:` 行は必須**であり、定型句ではない。本スキルが監査するのはレビューの 3 主題のうち 1 つで
あって、残り 2 つに何も触れないレポートは、それらを回していない読み手には完全なレビューとして読める。
テストとコメントはここでは見ていないと明記し、省略が推測ではなく可視になるようにする。推奨に和らげない
こと — 残り 2 つを回すかは Review Phase Protocol の下でユーザーが決めることであり、この行が記録するのは
この実行が覆わなかった範囲だけである。

**階層順に並べ、階層内で重大度順**、CONFIRMED を PLAUSIBLE より先に（評価順の規則 1）。上位の決着を
待っている指摘は `保留` と明記し、何を待っているかを書く（規則 2）。ランタイムで何を検査し何をスキップ
したかは必ず明記（黙って省くと「全部見た」と誤読される）。

## Step 6 — 指摘を PR にインラインコメント投稿（既定。`--no-comment` でオプトアウト）

既定で、残った **CONFIRMED + PLAUSIBLE** の指摘を、ブランチの PR に **インラインレビューコメント**として投稿する — 1 指摘につき 1 コメント、その `path:行` にアンカーし、1 つの長文コメントにまとめない。**REFUTED は投稿しない。** Step 5 のローカルレポートは常に出す（本ステップは追加動作）。

投稿するのは本スキル自身の指摘だけである。`/test-review` と `/comment-sweep` はそれぞれ自分の出力を出し、ここからそれらへ手を伸ばすことはない — 他スキルの指摘を本スキルのレビューとして投稿すれば、ある主題の監査が別の主題の中で行われたように見えてしまう。

### 手順

1. PR 番号・リポジトリ・コメントをアンカーする commit を解決:

   ```sh
   gh pr view --json number,url -q '.number'
   gh repo view --json nameWithOwner -q '.nameWithOwner'
   git rev-parse HEAD                                # アンカー SHA
   git rev-parse @{u}                                # push 済み head — HEAD と異なれば警告
   ```

   アンカー commit は PR に push 済みの commit でなければならない。ローカル `HEAD` ≠ `@{u}` なら先に push するよう警告（`commit_id` が PR に無いコメントは API が拒否する）。

2. どの指摘をインラインにできるか判定。GitHub のインラインコメントは PR diff に含まれる行にしか付けられない。diff の hunk を解析（`gh pr diff <PR> --patch` か `git diff <base>...HEAD`）:
   - `(path, line)` が追加/文脈の hunk 内 → インライン、`side: "RIGHT"`。
   - `(path, line)` が削除行 → インライン、`side: "LEFT"`。
   - diff 外（reviewer が未変更の文脈を参照）→ インライン不可。レビュー要約 `body` にまとめる。

3. 1つのレビューに全コメントをまとめて atomic に投稿（N 個の単発コメントにしない）:

   ```sh
   gh api --method POST repos/<owner>/<repo>/pulls/<PR>/reviews --input payload.json
   ```

   `payload.json`: <!-- skill-lint-ignore -->

   ```json
   {
     "commit_id": "<SHA>",
     "event": "COMMENT",
     "body": "🔎 impl-review (reviewer: <model>) — CONFIRMED <n> / PLAUSIBLE <m>\n\ndiff 外で行アンカー不可の指摘:\n- <path>: <要約>",
     "comments": [
       {
         "path": "<file>",
         "line": <n>,
         "side": "RIGHT",
         "body": "🔎 [CONFIRMED · high] <問題の要約>\n\n根拠: <...>\n修正案: <...>\n検証: <verifier 判定>"
       }
     ]
   }
   ```

   `event: "COMMENT"` を使う — これは助言的レビューであり `REQUEST_CHANGES` / `APPROVE` にしない。各コメント本文の先頭に `🔎 impl-review`（または `🔎 [判定 · 重大度]` タグ）を付け、人間のレビューと区別できるようにする。

4. 堅牢性: API がバッチを拒否（422 — 行が diff に無い）したら、該当コメントを要約 `body` へ移して再投稿。最後にインライン投稿分と要約分を報告 — 指摘を黙って落とさない。

## やる / やらない

- ✅ reviewer モデル ≠ implementer モデルを保証（Step 0 でユーザーが選択。実装者と同一を選んだ場合は警告して確認）。
- ✅ 指摘を階層で順位づける（評価順）: レポートを階層順に並べ、上位待ちの下位指摘は保留にし、同じ事実は上位の枠組みへ畳み、下位がクリティカルで上位を outrank しそうなときはユーザーに問う。
- ✅ finder は並列（1メッセージ・複数 `Agent` 呼び出し）、lens ごとに1体。
- ✅ レポート前に全 finding を独立 verify、REFUTED は落とす。
- ✅ 触られたエンドポイントはランタイム検証、共有スキーマ編集なら全 consumer に拡大。
- ✅ どのレポートでも、テストとコメント在庫をここでは監査していないことを `未監査の観点:` 行に明記。
- ✅ 復旧手段が `make db-init` しかない破壊系 curl は事前にユーザー確認。
- ✅ 既定で CONFIRMED + PLAUSIBLE をブランチの PR にインラインコメント投稿（Step 6）。`--no-comment` か PR 無しのとき抑止。
- ✅ PR 投稿前に一度だけ確認（外向きアクション）。各コメントは `path:行` にアンカーし、diff 外の指摘はレビュー要約にまとめる。
- ❌ REFUTED を投稿する / `REQUEST_CHANGES`・`APPROVE` を使う — 投稿レビューは助言的 `COMMENT` のみ。
- ❌ ソースを書き換える — どの lens も指摘までで、直すのはユーザー。
- ❌ テストやコメントを監査する lens を生やす / ここから `/test-review` や `/comment-sweep` を起動する。Review Phase Protocol の下では対等な別スキルであり、気づいた点は補足節に所管スキル名を添えて書く。
- ❌ reviewer を implementer と同一モデルで回す。
- ❌ 思いつきの style nit を finding として出す / 網羅に見せるための水増し。

## チェックリスト

- [ ] `AskUserQuestion` でスコープ確認、ベース ref 解決。
- [ ] reviewer モデル ≠ implementer モデルを確認。
- [ ] finder を並列 fan-out（コード lens は `adversarial-reviewer`、条件を満たせば `ddd-modeling-reviewer` / `type-design-reviewer`）。テスト lens もコメント lens も無い。
- [ ] 同じ事実を上位の階層へ畳んだ。レポートは階層順→重大度順。上位待ちの下位指摘は `保留` と明記。
- [ ] 全 finding を独立 verify、REFUTED は除外（件数は保持）。
- [ ] 触られたエンドポイントの curl + o11y 実施（共有スキーマ → 全 consumer）、破壊系は確認済み。
- [ ] この実行から他スキルを起動していない。
- [ ] 1 つの日本語レポート: CONFIRMED → PLAUSIBLE、ランタイムのカバー範囲を明記、`未監査の観点:` 行が存在。
