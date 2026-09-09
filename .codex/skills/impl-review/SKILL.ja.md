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

## 責務契約

| | |
| --- | --- |
| **所管** | 変更そのもの（`architecture` / `ddd-modeling` / `security` / `correctness` / `runtime-gap` / `type-design`） |
| **しないこと** | テスト観点（`test-review`）/ コメント観点（`comment-sweep`）/ 他スキルの呼び出し |
| **開始条件** | レビュー可能な差分とその意図が存在するとき |
| **停止条件** | tier 1 の finding が人間の設計判断を要するとき |

## 中核アイデア — reviewer ≠ implementer

バイアス低減が設計上の制約であって、おまけではない。よって reviewer は **コードを書いた者とは別モデルの subagent** として動く:

- reviewer エージェント（`adversarial-reviewer` / `comment-reviewer` / `review-verifier`）は frontmatter で既定 **`sonnet`**。通常の Opus 実装者と異なる。
- **reviewer のモデルは Step 0 でユーザーが選ぶ。** 選択肢は `fable`（Fable 5）/ `sonnet` / `opus` / `haiku`、加えて実装者と異なるモデルへ解決される *自動* 既定。選んだモデルを `Agent` ツールの `model` 引数で各 reviewer subagent に渡す（この引数はエージェント定義の `sonnet` 既定より優先）— 深さなら `opus`、安価な発散なら `haiku`、独立した新しい視点なら `fable`。
- **オーケストレーターは reviewer ≠ implementer を必ず保証する。** ユーザーが本セッションの実装者と同一モデルを選んだ場合は、別モデルによるバイアス低減が損なわれる旨を警告し、確認してから進める。reviewer と implementer を無言で同一モデルにしない。
- reviewer は **read-only**（エージェント定義に Edit/Write 権限なし）。本スキルが修正を当てることはない。

## 評価順 — 指摘は集めるだけでなく順位を付ける

レビュアーは食い違い、重なり、同じ事実を別の語彙で報告する。順位が無ければレポートは平坦な一覧に
なり、finder が「high」と呼んだだけのコメント指摘が、誤った集約境界より上に出る。Step 2 の表の
階層がその順位である。

| 階層 | lens | 何を決めるか |
| --- | --- | --- |
| 1 | `architecture` / `ddd-modeling` | コードが何で**あるべきか** |
| 2 | `security` / `correctness` | それが**動くか** |
| 3 | `runtime-gap` / 型設計 | 実システムと型の上で**保つか** |
| 4 | `test-gap` | **固定されているか** |
| 5 | コメント品質 | **どう読めるか** |

**上位の変更は下位へ伝播するが、下位が上位に働きかけることは原則としてない。** 集約境界を書き直せば、
それに対して書いたテストも、それを説明するコメントも前提ごと消える。逆にコメントの指摘が境界の変更を
正当化することはない。帰結は 5 つあり、いずれも提案ではなく規則である。

1. **レポートは階層順に並べ、階層内で重大度順にする** — 重大度だけで並べない。コメント品質の `high` は
   architecture の `medium` より下に置く。後者がそのコメントの乗るコードごと消すかもしれないため。
2. **上位の未決な指摘に依存する下位の指摘は `保留` として出す。** 報告はするが、何を待っているかを書き、
   着手可能なものとして提示しない。上位の決着後に見直すと、多くは消えている。
3. **階層 1〜2 の指摘が書き換えそうなファイルについては、Step 7 のコメント自動適用を抑止する。**
   これから変わるコードの文章を磨くのは二度手間であり、しかもコメント修正の diff が本題の指摘を埋める。
   抑止したファイルとその理由をレポートに書く。
4. **同じ事実を 2 階層が報告したら、上位の枠組みを残し、下位はその裏付けとして畳む** — 指摘は 1 件。
   1 つの事実に 2 エントリは 2 つの問題に見え、変更のリスクを二重に数える。
5. **同一階層どうしの合致は確度を上げてよいが、下位からの合致は上位の重大度を引き上げない。** 階層 2 の
   2 つの lens が独立に同じ欠陥へ到達したなら、それは強い証拠なので明記する。階層 5 が階層 1 に同意しても
   重大度は動かない（裏付けとして引用するのは構わない）。

**例外はクリティカルさであり、気づくのは担当だが裁定は担当ではない。** 下位の指摘のほうが緊急なことは
ある — `test-gap` が露呈させた悪用可能な穴は、アーキテクチャの議論を待たない。下位の指摘が上の階層を
outrank しそうなときは、**黙って並べ替えず、両方を提示してユーザーに問う。** 順位は普通の食い違いを
人間なしで解くために在るのであって、順位を破る指摘こそ人間が見るべき場面である。

## Step 0 — スコープ確認

即座に `ask the user explicitly`。未マージのコミットがあれば「変更ファイルのみ」を既定、なければ作業ツリー / 指定パスを既定。

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

同じ `ask the user explicitly` 呼び出しの中で（スコープと並べて2つ目の質問として）、reviewer subagent を
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
警告し、確認してから進める。選んだモデルは Step 2・Step 3 の各 `Agent` 呼び出しへ `model` 引数で渡し、
Step 5 では `/test-review` へ `reviewer_model` payload として渡す。

### テスト観点の委譲

同じ `ask the user explicitly` 呼び出しの中で（3つ目の質問として）、テスト観点を `/test-review` へ委譲するかを聞く。
委譲に監査対象があるかを決めるファイル集合の判定は Step 1（この質問と同時に確定するスコープの後）で行うため、
質問自体は **無条件** に出す:

```text
質問: テスト観点を /test-review へ委譲しますか？（既定: 委譲する）
選択肢:
  - 委譲する（/test-review を Step 5 で実行。test-gap lens は停止）  ← 既定
  - 委譲しない（test-gap lens のみ。変更シンボルの高シグナル・サブセット）
```

既定は *委譲する*。`test-gap` lens は自身の定義からして `/test-review` へ委ねるサブセットであり、
委譲を既定オフにすると「委ねる」が永久に起きず、ギャップは別スキルを打つのを覚えていた人にしか見えない。
一方で完全監査はレビューのコストをおよそ倍にするため、辞退は実際の選択肢であり、無条件実行ではなく質問として残す。
どちらに転んでも Step 6 の `テスト観点:` 行に記録する。

### フラグ

- `--no-comment` — Step 8 を抑止（PR に投稿せず）ローカルレポートのみ。**既定はオプトアウト**: ブランチに open な PR があれば、このフラグが無い限り Step 8 が残った指摘をインラインコメントとして投稿する。
- `--no-apply` — Step 7 を抑止（コメント指摘を自動修正しない）。代わりに報告し、他のレンズと同様に Step 8（PR 投稿）へ流す。**既定は適用する**: コメント品質の指摘は 1 回の確認のうえで作業ツリーへ自動修正される。

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
- **非生成の本番 `.go`**（`internal/**` / `pkg/**` 配下。`*_test.go` / `*.gen.go` / `*.sql.go` / `*_mock.go` を除く）が触られたか — コード起点のテスト解析に変更シンボル一覧を与える。
- **`*_test.go`** が触られたか。前項と合わせて **テスト観点判定式** を確定する: `本番 .go が触られた OR *_test.go が触られた`。テストのみの変更は後者だけで真になり、これは本番ソースを読む `test-gap` には見えないケースであり `/test-review` が存在する理由そのもの。
  - 判定式が真 **かつ** Step 0 で委譲を選んだ → Step 5 を実行し、`test-gap` lens は **起動しない**。
  - 判定式が真 **かつ** 辞退した **かつ** 本番 `.go` が触られている → `test-gap` をサブセットとして起動し、Step 5 は実行しない。
  - 判定式が真 **かつ** 辞退した **かつ** `*_test.go` しか触られていない → どちらも実行しない。`test-gap` は変更された本番ソースを読むレンズなので、テストのみの diff では列挙する対象が無く、起動しても空の結果が返って監査済みと読めてしまう。辞退がテスト観点を完全に未検査のまま残す唯一の状態なので、空の lens で代用せず `テスト観点:` 行にそう書く。
  - 判定式が偽 → どちらも実行しない。監査すべきテスト観点が無い。

  この4状態のどれかを記録する — Step 6 の `テスト観点:` 行に出す（後ろ2つはどちらも `未実施` に落ち、理由で区別する）。

## Step 2 — Finder の fan-out（別モデル、並列）

全 finder を並列起動する（`Agent` 呼び出しを1メッセージにまとめる）。中核アイデアのモデル規則を適用し、Step 0 で選んだ reviewer モデルを各 `Agent` 呼び出しの `model` 引数で渡す（*自動* がエージェント定義の既定に解決するときのみ省略可）。エージェント種別は 2 つ。

- 4 つの **コード lens** は `adversarial-reviewer` を使う。lens ごとに1体、`agentType: "adversarial-reviewer"`、`label` は `find:security` のように。
- **コメント次元**は専用の `comment-reviewer` を使う（`agentType: "comment-reviewer"`, `label: "find:comment"`）。1 段落の lens より豊かな分類を持つコメント特化の強いエージェントであり、その指摘が Step 7 の自動修正へ流れる。
- **DDD モデリング次元**は専用の `ddd-modeling-reviewer` を使う（`agentType: "ddd-modeling-reviewer"`, `label: "find:ddd"`）。diff が `internal/domain/**` または `internal/usecase/**` に触れた時。この変更がドメインをうまくモデル化できているかを、このリポジトリ自身が書き残した解釈で問う。**階層 1** の lens であり、その指摘は「コードが何であるべきか」を決めるため、下位の階層が動く前に決着させる。Evans の原典を直接の基準にしないこと — それは `ddd-origin-auditor` の担当で、対象も文書でありコードではない。
- **型設計次元**は専用の `type-design-reviewer` を使う（`agentType: "type-design-reviewer"`, `label: "find:type-design"`）。diff が domain 型（`internal/domain/**/*.go`）に触れた時のみ。4 軸ルーブリック（Encapsulation / Invariant Expression / Invariant Usefulness / Invariant Enforcement）で各型を採点し、指摘は suggestion 級（自動修正しない）。

| Finder | 階層 | エージェント | 起動条件 |
| --- | --- | --- | --- |
| `architecture` | 1 | adversarial-reviewer | 常時 |
| `ddd-modeling` | 1 | **ddd-modeling-reviewer** | diff が `internal/domain/**` または `internal/usecase/**` に触れた時 |
| `correctness` | 2 | adversarial-reviewer | 常時 |
| `security` | 2 | adversarial-reviewer | 常時（handler / auth / DTO / `openapi/**` が触られた時は特に） |
| `runtime-gap` | 3 | adversarial-reviewer | controller / DI / `openapi/**` / `database/**` が触られた時 |
| `test-gap` | 4 | adversarial-reviewer | `internal/**` / `pkg/**` 配下の非生成本番 `.go` が触られ、**かつ** Step 0 のテスト観点委譲を辞退した時 — Step 5 実行中は停止 |
| コメント品質 | 5 | **comment-reviewer** | diff がコメントを追加 / 変更した時（ほぼ常時） |
| 型設計 | 3 | **type-design-reviewer** | diff が domain 型（`internal/domain/**/*.go`）に触れた時 |

各 `adversarial-reviewer` プロンプトに必ず含める: lens 名 + その定義、ベース ref + 変更ファイル一覧 + diff、`AGENTS.md` / 該当 `README.md` / OpenAPI spec / migrations へのポインタ。

**`test-gap` lens 定義**（本 lens は *コード起点* — test ファイルではなく変更された本番ソースを読む）: diff で追加/変更された各本番シンボルについて、その論理分岐 / error sentinel / 境界条件 / zero 値防御を列挙し、ペアの `*_test.go` が各々を到達し*固有に* assert しているか（`require.ErrorIs` で固有 sentinel、区別される値/state — `require.Error` / `NoError` 止まりでない）を確認する。2 形を報告: diff で変更された本番シンボルに **テストが全く無い**、および変更シンボルの到達可能分岐が **未テスト or 空虚 assert**。 これは **high-signal サブセット** — impl-review は *変更された* コードで test ファイル起点の読みが見落とす到達ギャップを挙げるだけで、パッケージ全体の網羅的なシンボル列挙はしない。 全 subject に対する完全な 2 軸マトリクス（Lens 4 分岐×意味 + Lens 5 シンボル網羅）は `/test-review` の担当であり、実際に引き渡すのが Step 5。 指摘は read-only の提案（自動修正しない）で、diff 内の subject 行にアンカーするため他のコード lens 同様インライン投稿される。

**所管は1つ。** 本 lens と `/test-review` は重なる領域を監査するため、走るのは常にどちらか一方。Step 5 で委譲したときは `test-gap` を **起動しない**: `/test-review` の Lens 5 が「テストが1つも無いシンボル」を、Lens 4 が分岐×意味を所管しており、その上に本 lens を重ねると同じギャップを2つの severity 語彙で二重報告することになる。`test-gap` は委譲を辞退したときに残るもの — 変更コード上の最悪のギャップを低コストで拾うサブセットであって、冗長なセカンドオピニオンではない。

`comment-reviewer` プロンプトに必ず含める: ベース ref + 変更ファイル一覧 + diff、**行ポリシー**（diff スコープでは変更行上のコメントだけを判定する）、および実行時に読む権威として `docs/rules.md`（"Comment Rules"）へのポインタ。全言語一律の基準（Go も非 Go も同じ — shell / `.mjs` / Dockerfile / Makefile / SQL / YAML。非 Go は免除ではなくむしろ高リスク）と、機能的ディレクティブ / export doc コメントのガードはエージェント側が既に内蔵しているため、ここで再指定も緩和もしない。エージェントに見せるファイル一覧はコメントを持つソースに絞る: 生成物（`**/*.gen.go`、`*_mock.go`、`**/openapi.gen.yaml`、`// Code generated ... DO NOT EDIT`）、`vendor/**`、deny リスト、Markdown / docs の散文（Comment Rules が統べるのはソースコメントであって独立した文書ではない）を除外する。

## Step 3 — 敵対的 verify

全 finding を集め、(file, line, claim) で **dedup**。加えて 2 つの lens が同じ事実を報告した場合は評価順の規則 4 を適用し、上位の枠組みを残して下位はその裏付けとして畳み、指摘は 1 件だけ先へ送る。残った finding ごとに `review-verifier` subagent を1体（並列）起動し、単一 finding + ベース ref を渡す。`agentType: "review-verifier"`、`label` は `verify:<file>`、Step 0 で選んだ reviewer `model`（reviewer ≠ implementer は同様に維持）。

- **CONFIRMED** と **PLAUSIBLE** を残す。**REFUTED** は落とす（件数はレポート用に保持）。
- critical/high で単一判定が頼りないときは verifier を 2〜3 体立て多数決。重要な finding ほど単一意見より多様性。

## Step 4 — ランタイム検証（curl + o11y）— エンドポイント時のみ

**Step 1 でエンドポイントが触られた場合のみ** 実行し、subagent ではなく **オーケストレーター（メインセッション）** が行う（対話的 bash・実 DB/状態・ログ読み・ユーザー確認が要るため）。`scaffold-endpoint` Phase 7 に倣う:

1. `make go-test`（モック）は実 Fx グラフを組まず、auth/OpenAPI middleware も DB も通らない。だから本ステージは Step 2 の `runtime-gap` lens が *予測* したものを実地で拾う場。
2. 既知状態の対象行を用意/seed。認証/状態依存の検査は平文/状態を自分で握る行を作る。
3. 対象エンドポイントを `curl`（ローカル認証: `Authorization: Bearer debug:<subject>`）し検証: 正常系 / 主要異常系（404 / 400 / 422）/ — **operation が `security:` 宣言を持つなら** トークン無し ⇒ 401（実際に保護されているか証明）。IDOR 形の finding は *別の* subject で curl し他 subject のリソースに到達できないことを検証。
4. **共有スキーマ波及:** 共有 `components/*` を編集した場合（Step 1）、変更分だけでなく **全 consumer** を curl。spec を `$ref` で grep し各々を叩く。
5. o11y ログを1リクエスト分だけ読む: trace が controller → usecase → infra を貫き、発行 SQL が期待どおりか確認。以降の再確認は再 curl せず o11y で足りる。
6. **破壊ガード:** データを変える curl で復旧手段が `make db-init`（等）しかない場合、実行前にユーザー確認（`AGENTS.md` 準拠）。検証で作った行は片付ける。

ランタイムで確証した不具合は CONFIRMED として curl/o11y 証拠付きでレポートに統合。

## Step 5 — テスト観点を `/test-review` へ委譲

Step 1 のテスト観点判定式が真 **かつ** Step 0 でユーザーが委譲を選んだときに実行する。それ以外はスキップし、`test-gap` lens（または何も）に委ねる — どちらにせよ Step 6 がどうなったかを述べる。

`test-review` スキルを Skill ツールで起動し、以下を渡す:

- `scope`: 解決済みファイル一覧 — 変更された非生成本番 `.go` と変更された `*_test.go` の **両方**。ペアのテストが存在しない本番ファイルを渡すのは誤りではなく狙いそのもの。その組が Lens 5 の finding になる。
- `base_ref`: スコープが branch-vs-base の diff のとき、Step 1 で解決したベース。
- `reviewer_model`: Step 0 でユーザーが選んだモデル。委譲先の finder / verifier も同じ reviewer ≠ implementer 保証を継ぐ。
- `skip_verifier`: `false`。本スキルは全 finding を verify してから報告する。verify 段を落として速度を買うと、他半分が verify 済みのレポートに未検証の finding が混じることになり、監査しないより悪い。

チェインは **逐次・インライン** — オーケストレーターが `test-review` を読み込み、その手順をこのセッションで実行する。本リポジトリの他のチェインと同じ形。`/test-review` は read-only なので Step 7 のような作業ツリー確認は不要で、委譲先が独自の `ask the user explicitly` を出すこともない（`scope` payload が First Step の質問を飛ばす）。Step 2 の fan-out と並走させず Step 3 / Step 4 の後に置くのは、2つの fan-out を融合すると `/test-review` 側のコンテキスト読解ステップを本スキルへ引き上げることになり、既に所管のある手順を二重に持つため。

返ってきたレポートの構造と severity（修正必須 / 補完推奨 / 再考 / 追加検討 + criticality）はそのまま保つ。Step 6 が1節として埋め込む — CONFIRMED / PLAUSIBLE × 重大度 に写像し直さない。「規約に違反している」と「この分岐が未検証」を1軸に潰してしまう。

## Step 6 — レポート合成（日本語）

1つの日本語レポートを出す:

```text
## ローカルレビュー結果（reviewer: <model> / implementer: <model>）

スコープ: <base>...HEAD（<N> files） / lens: <実際に走らせた lens のみを列挙>
ランタイム検証: 実施（curl/o11y）/ 対象外（エンドポイント変更なし）
テスト観点: <下記 3 状態のいずれか>

### CONFIRMED（要対応）
- [重大度] タイトル — path:行
  - 問題 / 根拠 / 修正案
  - 検証: verifier 判定（+ 該当すれば curl/o11y 結果）

### PLAUSIBLE（要確認・判断保留）
- ...

### テスト観点（/test-review 委譲結果）
- <委譲したときのみ。/test-review の Step 4 レポートをそのまま埋め込む>

### 補足
- REFUTED: <n> 件（finder が挙げたが verifier が否定）
- ランタイム検証でカバーした経路 / スキップした経路
```

`lens:` 行には実際に走った lens だけを並べる — Step 5 で委譲したときは（停止させたので）`test-gap` は載らず、代わりに `/test-review 委譲` が入る。

**`テスト観点:` 行は必須**で、次の3値のいずれか1つを取る:

- `委譲実施（/test-review Lens 1-5 / CONFIRMED <n>・PLAUSIBLE <m>）`
- `test-gap レンズのみ（変更シンボルの高シグナル・サブセット。全シンボル網羅は未実施）`
- `未実施（テスト関連の変更なし / テストのみの変更で委譲を辞退したため test-gap にも対象が無い）`

存在理由はランタイム行と同じ。これが無いと、`test-gap` を含む `lens:` の羅列が「テストを監査した」と読めてしまうのに実際は変更シンボルのサブセットしか見ていないし、テスト解析を一切していない実行は痕跡すら残らない。弱いほうのケースこそ明記し、省略をカバレッジと取り違えさせない。

**階層順に並べ、階層内で重大度順**、CONFIRMED を PLAUSIBLE より先に（評価順の規則 1）。上位の決着を待っている指摘は `保留` と明記する（規則 2）。ランタイムで何を検査し何をスキップしたかは必ず明記（黙って省くと「全部見た」と誤読される）。委譲したテスト指摘も同様に、独自の severity 語彙のまま専用節に置く。Step 5 を実行しなかったときはその節ごと省く（`テスト観点:` 行が既にその事実を伝えている）。

## Step 7 — コメント指摘の適用（既定。`--no-apply` でスキップ）

本スキルがソースを書き換えるのはここだけ。verify を通った**コメント品質**の指摘（CONFIRMED、およびユーザーがオプトインした PLAUSIBLE）は自分で適用する — `comment-reviewer` サブエージェントは決して編集しない。コード 5 lens はここでは自動修正せず、Step 8 へ回す。

編集前に 1 度だけ確認する:

**先に評価順の規則 3 を適用する。** 生き残っている階層 1〜2 の指摘が書き換えそうなファイルを列挙し、この
ステップの対象から外す。外したファイルと理由をレポートに書く。

- `ask the user explicitly`: 「コメント指摘 <N> 件をライフサイクル内で修正適用しますか？」 — 選択肢: 「すべて適用」 / 「1件ずつ確認」 / 「適用しない（レポートのみ／PR コメント化）」。

各指摘が持つアクションを適用する — 内容が悪いコメントは **削除**、正しく振る舞いを述べる What へ **書換**、薄い What / 非自明な契約の欠落 / 良い Why の欠落は **加筆**。`誤り/陳腐化` の指摘（What がコードと矛盾）は削除ではなく訂正する。次のガードを守る（ここでの誤削除は実害のあるリグレッションになる）:

- **機能的・ディレクティブなコメントは絶対に削除しない**: `//go:generate`、`//nolint:...`、`//go:build` / `// +build`、`//go:embed`、`//export`、cgo preamble、`//revive:...`、`// Code generated ... DO NOT EDIT`、shebang、ツールディレクティブ。
- **エクスポートされた Go 宣言**（大文字始まりの `func`/`type`/`const`/`var`/メソッド）: doc コメントは **書換または加筆のみ、削除しない** — `revive exported` が要求するため。先頭識別子形式（`// Foo は …`）を保つ。
- **良いコメントは残す**: 正しく十分な What と非自明な Why（根拠 / 効いている制約）は指摘対象ではない — 剥がさない。書換・加筆は **What + 非自明な Why** を述べ、**How** や開発の経緯は書かない。編集はスコープ内ファイルに限り、生成ファイル・Markdown 散文・deny リストには触れない。`Edit` を使い、1 指摘（または 1 ファイル）ずつ進める。

編集後に検証する:

1. `make go-fix` — フォーマット / 自動修正を吸収する。
2. `make go-lint` — `revive exported` が通ることを確認し（必須 doc コメントの誤削除を検出）、他に劣化が無いことを見る。
3. 触れたファイルを `git diff` し、散文コメントだけが変わったことを確認する（機能ディレクティブを巻き込んでいないか）。非 Go は変更ハンクを読み直す。
4. 失敗したら提示して停止する — 自動 revert はせず、ユーザーが判断する。コミットはしない — 変更はユーザー（または後の `/commit`）に委ねる。

`--no-apply` の場合は本ステップを飛ばし、コメント指摘は他 lens と同様に Step 8 で PR へ投稿する。

## Step 8 — 指摘を PR にインラインコメント投稿（既定。`--no-comment` でオプトアウト）

既定では Step 6 の後、残った **CONFIRMED + PLAUSIBLE** の指摘を、ブランチの PR に **インラインレビューコメント**として投稿する — 1指摘につき1コメント、その `path:行` にアンカーし、1つの長文コメントにまとめない。**REFUTED は投稿しない。** Step 6 のローカルレポートは常に出す（本ステップは追加動作）。

**委譲したテスト指摘（Step 5）** も1つの制約付きで投稿対象に加わる: **アンカー行が PR の diff ハンク内にあるものだけ**。severity は4種すべて（修正必須 / 補完推奨 / 再考 / 追加検討）が対象 — 修正必須に絞ると、停止させた `test-gap` lens が従来投稿していた分岐ギャップが落ち、PR に見える情報が後退する。コード lens と区別がつくよう `🔎 [test-review · <severity>]` を接頭し、severity の語はそのまま残す。`· crit <n>` は criticality を実際に持つ finding にだけ付ける — `/test-review` は Lens 4 Axis A と Lens 5 の finding にのみ criticality を付し、構造準拠には明示的に付けないため、これは severity 単位ではなく finding 単位の属性である。枠を埋めるためにスコアを捏造しない。

diff 外のテスト指摘（多くは本 PR が触っていないファイルの Lens 5 シンボル）は **ローカルレポートのみ** に留める。diff 外のコード指摘のようにレビュー要約 `body` へ畳み込まない: diff 外のコード指摘は本変更が引き起こす欠陥だが、未テストの既存シンボルは本 PR が持ち込んだものでも、ここで議論する場でもない既存のカバレッジ負債である。何件を伏せたかとその理由をローカルレポートに書き、省略を見えるようにする。

以下のときは本ステップを丸ごとスキップ:

- `--no-comment` 指定時、または
- ブランチに open な PR が無い（`gh pr view` が空）— ローカルレポートのみとし、必要なら PR 作成を提案。

GitHub への投稿は外向きアクションなので、投稿前に **一度だけ** 確認する — 件数と対象 PR を提示（`ask the user explicitly`: 「<N> 件の指摘を PR #<番号> にインラインコメントとして投稿しますか？」/「投稿する」「投稿しない（ローカルレポートのみ）」）。

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
- ✅ Step 0 でテスト観点の委譲を聞き（既定: 委譲する）、委譲したら `test-gap` を停止して Step 5 を実行。
- ✅ どのレポートでもテスト観点の状態を `テスト観点:` 行に明記 — 何も監査しなかった実行を含めて。
- ✅ 復旧手段が `make db-init` しかない破壊系 curl は事前にユーザー確認。
- ✅ コメント品質の指摘は Step 7 で 1 回の確認のうえ適用（削除 / 振る舞いへの書き直し）し、その後 `make go-fix` + `make go-lint`。`--no-apply` で省略。
- ✅ 既定で CONFIRMED + PLAUSIBLE をブランチの PR にインラインコメント投稿（Step 8）。`--no-comment` か PR 無しのとき抑止。
- ✅ PR 投稿前に一度だけ確認（外向きアクション）。各コメントは `path:行` にアンカーし、diff 外の **コード lens** 指摘はレビュー要約にまとめる（diff 外の *テスト* 指摘は対象外 — Step 8 のとおりローカルに留める）。
- ❌ REFUTED を投稿する / `REQUEST_CHANGES`・`APPROVE` を使う — 投稿レビューは助言的 `COMMENT` のみ。
- ❌ 5 つのコード lens を自動修正する — これらは指摘までで、直すのはユーザー。自動適用されるのはコメント品質だけ（Step 7）。
- ❌ Step 7 で機能的ディレクティブ（`//go:generate` など）や export 宣言の doc コメントを削除する（doc コメントは書き直す）、生成ファイル / Markdown / deny リスト対象に触れる、自動コミットする。
- ❌ 同じレビューで `test-gap` と `/test-review` の両方を回す — ギャップの所管は1つ、報告も1つ。
- ❌ 委譲した指摘の severity を CONFIRMED / PLAUSIBLE × 重大度 に写像し直す / diff 外のテスト指摘を PR に投稿する。
- ❌ 重大度だけでレポートを並べる / 1 つの事実を 2 階層から 2 件として出す / 下位の合致で上位の重大度を上げる / 下位がクリティカルに見えるときに黙って階層を並べ替える。
- ❌ 階層 1〜2 の指摘が書き換えそうなファイルへコメント修正を自動適用する。
- ❌ reviewer を implementer と同一モデルで回す。
- ❌ 思いつきの style nit を finding として出す / 網羅に見せるための水増し。

## チェックリスト

- [ ] `ask the user explicitly` でスコープ確認、ベース ref 解決。
- [ ] reviewer モデル ≠ implementer モデルを確認。
- [ ] Step 0 でテスト観点の委譲を確認、Step 1 で判定式を解決、結果の状態を記録。
- [ ] lens ごとに finder を fan-out（並列）。`test-gap` は委譲を辞退したときのみ含める。
- [ ] 同じ事実を上位の階層へ畳んだ。レポートは階層順→重大度順。上位待ちの下位指摘は `保留` と明記。
- [ ] 階層 1〜2 の指摘が書き換えそうなファイルはコメント自動適用から外した。
- [ ] 全 finding を独立 verify、REFUTED は除外（件数は保持）。
- [ ] 触られたエンドポイントの curl + o11y 実施（共有スキーマ → 全 consumer）、破壊系は確認済み。
- [ ] 委譲したときは Step 5 を実行（`scope` / `base_ref` / `reviewer_model` / `skip_verifier: false` を渡し、`test-gap` は起動しない）。
- [ ] 1つの日本語レポート: CONFIRMED → PLAUSIBLE、ランタイムのカバー範囲を明記、`テスト観点:` 行が3状態のいずれかで存在。
- [ ] `--no-apply` 以外: コメント指摘を Step 7 で適用（機能的ディレクティブは不可侵、export doc コメントは削除でなく書き直し）、その後 `make go-fix` + `make go-lint`。自動コミットはしない。
- [ ] `--no-comment` / PR 無し以外: 一度確認のうえ CONFIRMED + PLAUSIBLE をインライン PR コメント投稿（diff 外 → 要約 body）、REFUTED は除外、`event: COMMENT`。
- [ ] 委譲したテスト指摘は diff ハンク内にアンカーできるものだけ投稿（severity 4種すべて）、diff 外はローカルに留め伏せた件数を明記。
