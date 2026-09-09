> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Test Review

Go ユニットテストファイル (`*_test.go`) の adversarial / low-bias レビュースキル。read-only。「壊れて見える / 検証不足 / 過剰検証」を指摘し、修正は user に委ねる。

## 使うとき

- commit / PR 直前、現在の change に含まれる `*_test.go` に対して。
- `scaffold-test` 実行後、生成テストの独立 second opinion として。
- カバレッジ 100 % のまま回帰バグが出る場合（観点が構造的には準拠でも意味的に薄い signal）。
- 特定 test パッケージ / ファイルの単発監査として。

使わない場面:

- **実装コードのレビュー** → `code-review` / `impl-review` / `arch-check`。
- **HTTP 統合テスト** (`internal/integration/` 配下) → 別 convention（`internal/integration/README.md` + `scaffold-integration-test`）。本スキルは同一パッケージ unit test 専用。
- 修正の適用 → 本スキルはファイル編集しない。指摘後、user が `scaffold-test` 実行 or 手で編集する。

## 読む / 書く

**常に読む**:

- `docs/testing-conventions.md` — **section 10（意味的品質のバー / アンチパターン）を含む**。 Lens 3 と Lens 4 軸B（意味網羅）の SSOT であり、`scaffold-test` と共有する。
- `.codex/skills/scaffold-test/SKILL.md` — 生成側 canonical ルール（parallel 必須 / `t.Run` per subcase / 正常系・異常系 グルーピング / 日本語命名 / require vs assert / mock 方針 / for-loop 方針 / 1 関数 = 1 `TestXxx`）。本スキルはこれらに対してレビューする（重複定義しない）。
- 層 README。**対象テストファイルから上位へ歩き、Test Strategy 節を持つ最も近い祖先 `README.md`** を採用する（見出しの表記は README ごとに揺れる — `Test Strategy` / `Test strategy` / `Testing strategy` / `Testing Strategy` / `Testing Policy` — ので意味で判定すること。その層のテスト戦略そのものであれば名前が何であれ該当し、他のドキュメントが名前で参照している節をこの規則に合わせて改名するのは誤った直し方である。`scaffold-test/SKILL.md` と同じ規則であり、両者は歩調を合わせる）。節を持たないより近い README も、そのパッケージ固有の規約のために併読する。下記は現時点で walk が着地する先のスナップショットであり固定マップではない。一覧に無いパスは walk の対象であって、対象外ではない。
  - `internal/domain/README.md`（Testing strategy）
  - `internal/usecase/README.md`（Testing Strategy）
  - `internal/controller/handler/README.md`（Test Strategy）はハンドラ用。`internal/controller/outbox/**` / `internal/controller/worker/**` の解決先は `internal/controller/README.md`（Test Strategy、層の基準）であり、HTTP 側ではなくループ駆動のサブセクションを読むこと
  - `internal/controller/httpstack/README.md`（Test Strategy）— 各ミドルウェアのサブパッケージの解決先
  - `internal/controller/server/README.md`（Test Strategy）
  - `internal/infrastructure/README.md` + `internal/infrastructure/rdb/README.md`（Test Strategy）
  - `internal/di/README.md`（Test Strategy、層の基準）— 配下の対象では `internal/di/module/README.md` / `internal/di/server/hook/README.md` が優先される
  - `internal/apperror/` / `internal/cli/` / `internal/config/` / `internal/logging/` / `internal/observability/` / `internal/system/` — 横断的な基盤。各パッケージルートに自前の節を持つ
  - `pkg/**` については `scaffold-test/SKILL.md` 参照 — `pkg/README.md` は意図的に Test Strategy 節を持たず、sibling tests + `pkg/<name>/README.md` から観点派生。**pkg では gap 警告を出さない**。
- 対象 `*_test.go`。
- 対応する subject ソース (`<subject>.go` / `<subject>_test.go` 対）— コード起点 2 レンズ（Lens 4 分岐×意味 / Lens 5 シンボル網羅）で「何が test されていないか」を判定するために必須。
- 同一パッケージの sibling test（成立済みの conventions の参考）。

**書く**:

- 何も書かない。read-only。 最終出力は conversation 上に展開される日本語レポート。

**`make` 起動**: なし（実行はしない、レビューだけ）。

**触らない**:

- test ファイル（自動 fix なし → `scaffold-test` or 手編集の領分）。
- subject ソース。
- 生成成果物 (`*.gen.go` / `*_mock.go` / `*.sql.go`)。
- `.codex/` 配下。

## First Step: スコープ解決

**呼び手が `scope` payload を渡してきたときは、この質問を丸ごと出さない**（Chainability 参照）— ファイルリストは既に解決済みであり、再度聞くのは呼び手が下した判断についての2つ目のダイアログでしかない。

`ask the user explicitly`:

- 質問: 「test-review の対象スコープを指定してください」
- 選択肢（single-select）:
  - 「変更ファイル (HEAD-vs-working tree, 推奨)」 — `git diff --name-only` で `*_test.go` 抽出（`impl-review` / `code-review` と同じ振る舞い）。新規追加 (`--diff-filter=A`) も含める。
  - 「ブランチ base 比較 (ベースブランチ以降の変更)」 — base からの分岐点を `git merge-base` で取り、その間に touched された `*_test.go`。 PR 単位で見たいとき。base は PR があればその `baseRefName`、無ければ `make base-branch`（`origin` の実状態から最新のリリースラインを解決する）。`gh repo view --json defaultBranchRef` は使わない — GitHub のデフォルトブランチはアクティブなリリースラインより遅れており、レビュー範囲が 1 世代分黙って広がる。
  - 「特定パス / パッケージ (free-text)」 — user 指定。 ファイルでもディレクトリでも可。
  - 「キャンセル」。

解決後、対象ファイルリストを構築。スコープ内に `*_test.go` がなければ穏便に停止（レビュー対象なし）。**これは standalone のときだけ**: 呼び手の payload 下では未テストの production ファイルこそが狙いであってスコープが空なのではないため、実行を続ける（Chainability 参照）。

各 test ファイルについて **subject ソース**（同パッケージ・basename `_test` 抜き）も解決。コード起点 2 レンズ（Lens 4 / Lens 5）に必須。

## Step 1. 層コンテキスト読み込み

各対象 test ファイルにつき:

1. ファイルパスから上位へ歩いて層 README を解決する（`scaffold-test/SKILL.md` が定める walk と同じ規則。固定の band lookup ではない）。
2. 該当層 README の `Test Strategy` / `Testing strategy` 節（全文、サブセクション見出し含む）。
3. `docs/testing-conventions.md`（1 回）。
4. `.codex/skills/scaffold-test/SKILL.md`（1 回、canonical ルール）。
5. subject ソース。
6. 同パッケージの sibling test（helper シグネチャ / fixture スタイル / mock 配線 等の確立 convention）。

対象が `internal/**` 配下で、上位へ歩いてもリポジトリルートまで Test Strategy 節が見つからない場合は report に notes として記録する（レビューはブロックせず、documentation gap として surface）。**`internal/**` の全パスが節へ解決されることを期待する**。唯一の免除は `pkg/**` であり、上記スナップショット一覧に名前が無いことを理由に免除扱いしないこと。この状態では Lens 2（観点カバレッジ）に比較基準が無いため、その旨を明示する — 何も報告されずに pass と読まれてしまわないように。

## Step 2. 5 つの adversarial reviewer を fan out

`adversarial-reviewer` subagent を 5 つ、**並列**起動 (`subagent_type: adversarial-reviewer`)。 既定で `sonnet`（Opus 実装者 ≠ reviewer を保つ。orchestrator がモデルを上書きして reviewer ≠ implementer を維持してもよい）。

5 つのうち 2 つは **コード起点（subject 駆動）** — test ファイルではなく subject ソースから出発するため、「テストが 1 つも無いコード要素」も視界に入る: **Lens 5**（各 subject シンボルに規約名 `TestXxx` が存在するか）と **Lens 4**（テスト済み関数の中で各分岐が到達され固有 assert されているか）。残る 3 つ（Lens 1 / 2 / 3）は test ファイル起点 / README 起点。到達可能なのに未テストのコードは、test ファイル起点の読みでは構造的に見えず、このコード起点ペアが拾う。

各 subagent は Step 1 の context bundle（層 README / `docs/testing-conventions.md` / `scaffold-test/SKILL.md` / 対象 test / subject / sibling tests）を共通で受け取り、レンズだけ違う prompt を受ける。

### Lens 1: 構造準拠

機械的なルール準拠を監査（`scaffold-test/SKILL.md` のハードルール）:

- 全 `t.Run` の冒頭が `t.Parallel()`（または `-race` 例外コメント付きの直列化）。
- 全サブケースが `t.Run`（裸のアサーション禁止）。
- `TestXxx` 最外殻 `t.Run` group の name は literal `正常系` / `異常系` （各 `TestXxx` に最大 1 個ずつ、 さらに細分するならその **内側に** ネストする）。 最外殻に `t.Run("正常系_xxx", ...)` 形式を使っているのは違反 → finding として出す。 内側のサブケース名にも `正常系_` / `異常系_` プレフィックスは禁止（外殻 group で既に区別済みのため二重ラベル）。
- ケース名は日本語。
- エラーは `require.*`（testifylint `require-error`）、終端値は `assert.*`。`require.NotNil` / `require.True` 等は、**後続コードをガードする**場合（そのまま使うと panic / 無意味になる。例: `require.NotNil(fn); fn(...)` や `require.NotNil(rec); _ = rec.Field`）のみ正しい。値を以降で使わないなら終端なので `assert.*` にすべき — 終端の `require.NotNil` / `require.True` / `require.Equal` は違反として指摘。
- 生成 mock を駆動するサブテストは mock controller 経由で assert している扱い: `EXPECT()`（メソッドが呼ばれないことを示す意図的な *no-EXPECT* や `.Times(0)` を含む）がアサーション。`assert.*` / `require.*` 行が無いというだけで「アサーション無し」と指摘しない。
- **table-driven `for` ループは違反**（`scaffold-test/SKILL.md` Rule 5）。`(input, expected)` 構造体スライスを `for _, tc := range cases { t.Run(...) }` で回すブロックは、可読性や `dupl` 回避を理由にしても指摘する。正しい形はケース毎の逐次 `t.Run` sibling（重複は許容）。長いゲッター/境界リストでも同様。
- **subject 関数 ↔ `TestXxx` の 1:1 対応。** ここでの *subject* はペアになる本番ソース — バイナリにビルドされる非テスト・非生成の `.go` ファイル。`*.gen.go` / `*.sql.go` / `*_mock.go` とテスト専用ヘルパは対象外（手書き `TestXxx` を期待しない）。両方向を確認:
  - *順方向*: 各 `TestXxx` は 1 subject 関数 / メソッド対応。複数 subject を束ねる `TestXxx`（統合された `*_Accessors` / `*_Getters` 等）は 1:1 違反で、rationale コメントによる免除は無い。subject ごとに `TestXxx` を分解する。唯一の免除は **検証不可能であるために到達できない** subject で、その場合も規約名の `TestXxx` を宣言し `t.Skip("<なぜ検証不可能か>")` を呼ぶ — 束ねは常に指摘し、決して受け入れない（`docs/testing-conventions.md` §1、`internal/architest` が強制）。
  - *逆方向*: 各 subject 関数 / メソッドは 1 つの `TestXxx` に対応。1 つの subject が複数 `TestXxx` に分裂している（例: `TestFoo` + `TestFoo_Metrics` + `TestFoo_CloseError`、または `Test_foo` / `TestFoo_foo` の命名ゆらぎペア）のは finding → `正常系` / `異常系` group で variant を吸収する単一 `TestXxx` へ統合する (Rule 7)。`TestXxx` が 1 つも無い public subject 関数は **Lens 5（シンボル網羅）の所管** — 本レンズは既に存在する `TestXxx` の *形*（命名ゆらぎ / 束ね / 分裂）だけを指摘し、存在しないこと自体は Lens 5 に委ねて二重報告しない。
- mock は `<package>/mock/*_mock.go` から（手書き mock 禁止）。
- 層別禁則 import（`pkg/**` test から `internal/` 参照禁止、`internal/domain/` test から infrastructure 参照禁止 等、`docs/testing-conventions.md` ルール）。

Output: `file:line` 付きの違反 finding リスト + 違反したルール。

### Lens 2: 観点カバレッジ

層 README の Test Strategy サブセクションと実 test を突き合わせ:

- README の各サブセクション見出し（`### Getter contract test` / `### Immutable guarantee test` / `### Invariant preservation test` 等）に対応する `TestXxx → t.Run(case)` が 1 件以上あるか。
- 層 README の Test Strategy 節がその層の観点リストの SSOT（Step 1 で読む）。 per-layer 観点リストを本スキルにハードコードしない — README と drift する。 その層の README が宣言するサブセクションをそのまま適用し、reviewer が期待する観点が README に欠けていれば、それ自体を doc ギャップ finding（補遺で surface）として出す。ここに観点を書き戻さない。
- 上位へ歩いても Test Strategy 節が見つからない場合は sibling test パターンを比較基準にする。ただしどちらのケースかを明示する — `pkg/**` 配下ならそれが層の正常モード（gap ではない）、`internal/**` 配下なら本レンズの比較基準を欠いた documentation gap（補遺で報告）。

Output: README が宣言しているが test が exercise していない観点リスト（`file:section` で README 該当節を引用）。

### Lens 3: 意味的品質

アサーションが本当に意味を持っているかを、**`docs/testing-conventions.md` section 10（意味的品質のバー / アンチパターン）を唯一の source of truth として**監査する。 この節は `scaffold-test` と共有する SSOT（生成器はこれを満たすように生成し、本 lens はこれへの違反を検出する）— runtime で読み、その時点で列挙されている内容を適用する。 アンチパターンのカタログを本スキルにハードコードしない（doc からドリフトする）。 執筆時点の §10 の列挙: 弱いアサーション（trivial constructor の 1:1 *その場で強化*例外つき — より強い assert を推奨し、専用 `TestXxx` を削除・他 subject へ畳み込まない）、ケース名がアサートを過剰約束（分岐なし pass-through / 配線の系 — 冗長な `NotNil` 再実行は畳む）、内部結合の脆さ、過剰モック、時刻リテラル漏れ、`TestXxx` 責務肥大、ヘルパ重複、冗長コメント。 §10 がアンチパターンを追加・削除・改訂したら、この段落ではなく doc に従う。

Output: `file:line` + 違反した §10 アンチパターン + 「なぜ弱い / 脆いか」の 1 文説明 finding リスト。

### Lens 4: 観点ギャップ — 分岐 × 意味 網羅 (subject 駆動)

subject ソースを直接読み、**関数 / メソッドごと**に 2 軸の網羅マトリクスを構築する。 カバレッジ ≠ 意味のあるカバレッジ: ある分岐が「その分岐固有の何か」を一切 assert しないケースで実行されているだけのことがあり、その穴を切り分けるのが本 lens の目的。 各 subject で両軸を走らせる — 分岐は「到達済み (軸A)」かつ「固有に assert 済み (軸B)」で初めて完了。

**Lens 5 との棲み分け**: Lens 4 は既に `TestXxx` を持つシンボルの*内側*を監査する — どの分岐が到達され意味づけ assert されているか。「シンボルにテストが 1 つも無い」は **Lens 5** の finding であって Lens 4 のものではない。 Lens 5 が既にゼロテストのシンボルを挙げている場合、その全分岐をここで列挙し直さない（それは N 個ではなく 1 個のギャップ）。 Lens 4 の分岐 finding は、テストは存在するが不完全なシンボルに適用する。

**軸A — 分岐網羅**: subject の各論理分岐が最低 1 件のケースで到達されているか。

- 各条件分岐（ポジ / ネガ）に最低 1 件の `t.Run` ケースがあるか。
- 宣言 / 返却される error sentinel (`ErrInvalid*` / `apperror.*`) が最低 1 件のケースで到達されているか。
- 境界制約を持つフィールドについて min-1 / min / max / max+1 の境界値が exercise されているか。
- zero 値 / nil 入力を防御している constructor / factory に reject ケースがあるか。
- テストの harness が**実行しない** constructor / provider / factory 本体を通ってしか到達できない分岐は未カバー扱い — 依存グラフを構築するだけでコンストラクタを実行しないグラフ / 配線検証 harness はそれらの本体をカバーしないので、直接の単体テスト（関数を実際に呼ぶ）が要る。 適用される harness は層 README の Test Strategy が明示する。
- subject を他のテストがカバーしていることを理由にした `t.Skip` は、weigh するギャップではなく **修正必須** の違反: その skip はテストを別テストの実装に依存させ、カバー元が縮小しても green のまま残る。実テストを要求する（`docs/testing-conventions.md` §1、`internal/architest` の `TestSkipReasonDoesNotNameCoveringTest` が強制）。
- 「再現できない」と理由付けされた `t.Skip` は受け入れず軸A のギャップとして疑う: 層 README の Test Strategy に到達できる統合スタイルの harness が無いか確認する（例: 真の並行 / ロック競合は、直列化するテスト用 tx ヘルパでは表現できず独立コネクションが要る）。skip 分岐を具体的な再現経路つきで 追加検討 として surface する。

カバーケースが**全く無い**分岐は **分岐未カバー** finding → severity **追加検討**（proactive）。 未カバー分岐の subject `file:line` + 提案 `t.Run` ケース名を引用。加えて **criticality（1-10）** を*本番影響*で採点して付す（レンズ由来 severity とは直交する軸 —「追加検討」は*どの種類*のギャップか、criticality は*壊れたらどれだけ悪いか*）。未検証のまま壊れた場合に出荷されるリグレッションを一文添え、追加検討 finding を criticality 降順に並べて最悪から潰せるようにする: 9-10 データ破壊 / 認証・認可の穴 / 整合性違反 · 7-8 ユーザ影響のあるロジック誤り（誤った status / DTO マッピング）· 5-6 軽微な edge / boundary · 3-4 網羅性のための nice-to-have · 1-2 任意。構造準拠（修正必須）には criticality を付けない（常に即修正）。

**軸B — 意味網羅**: カバー済みの各分岐のケースが、単に実行されたことではなく*その分岐固有*の outcome を assert しているか。 本軸は **`docs/testing-conventions.md` section 10 が定める意味網羅バー**を subject の分岐集合に適用する — 「固有に assert 済み」の定義は §10 が SSOT（`scaffold-test` と共有し、生成器はこれを満たすよう生成する）。 以下の分岐別チェックはその具体適用。

- error 分岐は `require.Error` 止まりでなく `require.ErrorIs` で固有 sentinel を assert。
- 成功分岐は `require.NoError` / `assert.NotNil` 止まりでなく、他分岐と区別される結果値 / state を assert。
- state mutate メソッドのケースは「呼べた」止まりでなく mutate 後の不変条件 / 変化フィールドを assert。
- 層 README が immutable と明示するポインタ / 参照返却 getter は、値等価だけでなく不変性を assert（返り値を変更しても entity に影響しないこと）。
- 境界ケースは accept 側だけでなく境界の両側（accept vs reject）で異なる outcome を assert。

カバーは**されているが**固有 outcome を区別 assert していない分岐は **分岐カバー済み・意味未検証** finding → severity **再考**（pass して coverage は上がるが何も明らかにしない）。 finding は当該 subject 分岐 + それを名目上カバーしている test ケースに紐付ける。

Output: subject ごとに、(1) 未カバー分岐 + 提案 `t.Run` ケース名（軸A → 追加検討）、(2) カバー済みだが意味未検証の分岐 + 不足している区別アサーション（軸B → 再考）。 各々 subject 分岐の `file:line` とカバーしている test ケースを引用。

### Lens 5: シンボル網羅 (コード起点)

**test ファイルではなく subject ソース**から出発する — Lens 4 と同じ起点だが、粒度は粗い*シンボル*単位。 その唯一の仕事は、subject の完全な公開シンボル表に対して「そもそもこれにテストが存在するか」を答えること。 test ファイル起点の読み（Lens 1）は見つけた `TestXxx` しか判定できず、テストゼロのシンボルは不可視。 本レンズはまさにその盲点 — 到達可能なのに未テストのコードがすり抜ける経路 — を潰すために存在する。 先にコードを列挙し、それに対してテストを突き合わせるので、不在が「沈黙」ではなく積極的な finding になる。

手順:

1. **subject シンボル表を構築する。** ペアの `<subject>.go`（非生成・非 `*_test.go`）から、その層の規約が `TestXxx` を期待する全シンボルを列挙: 公開 func / メソッド / コンストラクタ、および層が直接テストする分岐ロジックを持つパッケージレベルの非公開 func（期待は層 README の Test Strategy + `docs/testing-conventions.md` §1 が定義。`*.gen.go` / `*.sql.go` / `*_mock.go` とテスト専用ヘルパは対象外）。 getter / accessor / provider func / env ゲートヘルパも数える — これらこそ見落とされやすい低可視性シンボル。
2. **各シンボルを `TestXxx` に対応付ける。** シンボルが*充足*なのは、規約名の `TestXxx` が実際にそれをテストしている場合のみ。 本体が `t.Skip` だけの `TestXxx` が充足なのは、理由が「なぜ subject を検証できないか」を述べている場合に限り、他テストがカバーしていることを理由にした skip は**未充足**（`docs/testing-conventions.md` §1）。
3. **充足しない全シンボルを** **シンボル未カバー** finding として挙げる → severity **補完推奨**（コード要素まるごとテストが無い — Lens 4 の*関数内*分岐ギャップとは別の構造的カバレッジ穴）。 subject `symbol @ file:line` を引用し `TestXxx` 名（と `正常系` / `異常系` 骨子）を提案。 Lens 4 軸A と同じ **criticality（1-10）** 本番影響スコアを付し降順に並べる — 完全に未テストの認証・認可 / 永続化シンボルは、未テストの些末な getter より上位。

Lens 4 への引き継ぎ: Lens 5 があるシンボルを「まるごと未テスト」と挙げたら、Lens 4 はそのシンボルの分岐を追加列挙しない（「テスト無し」ギャップは 1 個であって N 個ではない）。 Lens 4 は Lens 5 が充足と見なしたが部分的にしかカバーされていないシンボルだけを拾う。

Output: テストが無い（または `t.Skip` の理由が検証不可能性でなく被覆テストを名指している）subject シンボルのリスト。 各々 `symbol @ file:line` / 提案 `TestXxx` / criticality。

## Step 3. 各 finding を検証

生き残った各 finding を独立した `review-verifier` subagent (`subagent_type: review-verifier`、 sonnet) に渡す。 verifier は:

- コードから結論を **再導出**（finder の出力は信用しない）。
- 既定で skeptical — 曖昧なら CONFIRMED より PLAUSIBLE / REFUTED 寄り。
- **CONFIRMED**（違反 / 漏れ / 弱アサーションが実在し再現可能） / **PLAUSIBLE**（妥当そうだが verifier が完全に再導出しきれなかった） / **REFUTED**（counter-evidence — 引用行がコメント、見出しが別層、文脈考慮で十分なアサーション 等）に分類。

検証は finding 並列実行 (`parallel(findings.map(f => () => agent(verifyPrompt(f))))`)。 reviewer ≠ implementer を保つために finder と verifier で異なるモデルを使うのもよい。

REFUTED は report から落とす（合計件数だけ summary に残す → noise floor を見せる）。 CONFIRMED / PLAUSIBLE は lens ごとに残してレポート合成。

## Step 4. レポート合成

単一の日本語レポート。推奨構造:

```text
# Test Review レポート

対象: <スコープ + ファイル一覧>
レンズ: 構造準拠 / 観点カバレッジ / 意味的品質 / 観点ギャップ(branch×meaning) / シンボル網羅
verifier 通過: CONFIRMED <n> 件 / PLAUSIBLE <m> 件 / REFUTED <k> 件 (フィルタ済み)

## サマリ
- 修正必須: <件数>
- 補完推奨: <件数>
- 再考: <件数>
- 追加検討: <件数>

## 構造準拠（修正必須）
- [<severity>] <file>:<line> — <violated rule>
  - 詳細: <description>
  - 出典: `<README path>` / `scaffold-test/SKILL.md` の該当節
  - verifier: CONFIRMED / PLAUSIBLE

## 観点カバレッジ（補完推奨）
- [<severity>] <test file> — <missing viewpoint>
  - 出典 README: `<README path>:<section heading>`
  - 提案: <suggested t.Run case name>
  - verifier: CONFIRMED / PLAUSIBLE

## 意味的品質（再考）
- [<severity>] <file>:<line> — <weak assertion / brittle coupling>
  - 詳細: <one-sentence why>
  - verifier: CONFIRMED / PLAUSIBLE

## シンボル網羅（補完推奨）
- <file> に対して subject <subject path> から導出（criticality 降順）:
  - シンボル未カバー: <symbol @ subject file:line>（対応 TestXxx 皆無）
  - criticality: <1-10> — 未検証で壊れた場合のリグレッション: <一文>
  - 提案: func Test<Symbol>(t *testing.T) — 正常系 / 異常系 の骨子
  - verifier: CONFIRMED / PLAUSIBLE

## 観点ギャップ: 分岐網羅（追加検討）
- <file> に対して subject <subject path> から導出（criticality 降順）:
  - 分岐未カバー: <subject file:line の分岐>
  - criticality: <1-10> — 未検証で壊れた場合のリグレッション: <一文>
  - 提案: t.Run("<case name>", ...) — カバーする分岐 / sentinel: <reason>
  - verifier: CONFIRMED / PLAUSIBLE

## 観点ギャップ: 意味網羅（再考）
- <file> に対して subject <subject path> から導出:
  - 分岐カバー済み・意味未検証: <subject file:line の分岐> を <test file:line のケース> がカバーするが固有 outcome 未 assert
  - 不足アサーション: <あるべき distinctive assertion>
  - verifier: CONFIRMED / PLAUSIBLE

## 補遺
- pkg/ 層は Test Strategy 節を意図的に持たないため、sibling tests を比較基準にしています（gap 警告なし）。
- <その他、レビュー過程で気付いた README 補完候補 / SKILL の改訂候補>
```

severity マッピング:

- **修正必須** （構造準拠）: `docs/testing-conventions.md` / `scaffold-test/SKILL.md` のハードルール違反。 CONFIRMED → 修正必須 / PLAUSIBLE → 確認推奨。
- **補完推奨** （観点カバレッジ + シンボル網羅）: README で宣言されているのに exercise されていない観点、または subject シンボルに `TestXxx` が 1 つも無い。 CONFIRMED → 補完推奨 / PLAUSIBLE → 確認推奨。
- **再考** （意味的品質 + 観点ギャップ 軸B）: pass はするが意味が薄い — 弱アサーション、または分岐はカバーされているが固有 outcome を区別 assert していない。 CONFIRMED → 再考 / PLAUSIBLE → 補強候補。
- **追加検討** （観点ギャップ 軸A）: subject 検査から派生する未カバー分岐の proactive 提案。 CONFIRMED → 追加検討 / PLAUSIBLE → 提案。

## Step 5. 次アクション提案

レポート末尾に 1 つだけ具体提案:

- 「修正必須」がある → 該当 subject に対して `/scaffold-test` 再実行（canonical 構造にリセット）または該当行を手で修正、を提案。
- 「補完推奨」 / 「追加検討」 のみ → 提案された `t.Run` を手で追加するか、対象 subject を絞った `/scaffold-test` で補完するか、を提案。
- verifier 通過後 0 件 → その旨明示（`「verifier 通過後 0 件です」`）。

## Chainability

本スキルの呼び手は `impl-review`。 同スキルの Step 5 が、自分で監査せずテスト観点をここへ委譲する — 同スキルの `test-gap` lens が「`/test-review` に委ねる」と言っているのはこのこと。 委譲が走っている間 `impl-review` は `test-gap` を停止するので、 本スキルが既に Lens 4 と Lens 5 の間で適用している「所管は1つ」の原則がスキル境界を跨いで伸びる — Lens 5 が「テストが1つも無いシンボル」、 Lens 4 が分岐 × 意味を所管し、 どちらにも3人目の報告者はいない。

本スキルが他スキルを *chain in* する経路は依然として持たない — user がレポートを読んで `scaffold-test`（再生成）or 手編集を判断する。 `impl-review` へ呼び返すこともしない（レビューフローの入口は常に呼び手側）。

呼び手は以下を渡す:

- `scope` — 解決済みファイルリスト（First Step の `ask the user explicitly` をスキップ）。
- `base_ref` — branch-vs-base モード時の base ref。
- `reviewer_model` — 呼び手が reviewer ≠ implementer を保つために解決したモデル。 Step 2 の finder と Step 3 の verifier の両方へ適用し、 既定の `sonnet` を上書きする。
- `skip_verifier` — boolean。 親が速度優先で verify を切る場合（既定 `false` = verify する）。

payload 下で standalone と挙動が変わるのは次の2点だけ:

- **ペアのテストが無い production ファイルもスコープに残す。** standalone では `*_test.go` 0件で実行を終える。 chain 時は `<subject>_test.go` が存在しない production ソースこそ Lens 5 の subject なので、 残して Lens 5 にテスト不在を報告させる。 Lens 1-3 は test ファイル駆動でそのファイルについて読むものが無い — pass と読める空の結果を返させず、 そのファイルについては skip する。
- **レポートは呼び手が埋め込む前提で返す**（単体の成果物として描画しない）。 Step 4 と同じ構造で出し、 配置は呼び手に委ねる。 severity は本スキルの語彙（修正必須 / 補完推奨 / 再考 / 追加検討 + criticality）のまま保つ — 呼び手の体系へ写像し直すと「規約に違反している」と「この分岐が未検証」の区別が黙って失われる。

## 制約（サマリ）

- ❌ ファイル編集（read-only）。
- ❌ `make go-test` 実行（本スキルはレビュー、実行は `make go-test` 別途）。
- ❌ verifier を通さず finder 出力をそのまま信頼（`skip_verifier: true` 親指定以外、verifier は必須）。
- ❌ 観点リストのハードコード（SSOT は層 README の Test Strategy 節、`pkg/` は明文化された例外）。
- ❌ 意味的品質アンチパターンのカタログ（Lens 3）や意味網羅バー（Lens 4 軸B）のハードコード — SSOT は `docs/testing-conventions.md` section 10、runtime 読み込み。
- ❌ `docs/testing-conventions.md` / `scaffold-test/SKILL.md` のルール重複定義（runtime 読み込み）。
- ✅ verifier は skeptical 既定（曖昧時は PLAUSIBLE 寄り）。
- ✅ reviewer 既定モデル `sonnet`（Opus 実装者と異なる）。 必要なら orchestrator が上書きして reviewer ≠ implementer を維持。
- ✅ スコープ既定: 変更ファイル。 他選択肢あり。
- ✅ 最終レポートは日本語、 lens 別グルーピング、 severity tag。
- ✅ `pkg/` は意図的に「Test Strategy 節なし」層として扱い、 documentation gap として警告しない。
- ✅ criticality (1-10) は Lens 4 軸A（追加検討）と Lens 5（補完推奨・シンボル未カバー）の finding に付す本番影響のソート鍵で、レンズ由来 severity（修正必須 / 補完推奨 / 再考 / 追加検討）を置換しない。構造準拠（修正必須）には付けない。
- ✅ コード起点は 2 レンズ体制（Lens 5=シンボル存在 / Lens 4=関数内 branch×meaning）。「テストが 1 つも無いシンボル」は Lens 5 が所管し、Lens 1 逆方向 / Lens 4 とは二重報告しない。`impl-review` から chain されている間は同スキルの `test-gap` lens が停止しているため、所管はスキル境界を跨いでも 1 つのまま。
- ✅ payload 受領時は First Step の質問を出さず、ペアテスト不在の production ファイルもスコープに残し（Lens 5 の対象）、レポートは呼び手が埋め込む前提で自分の severity 語彙のまま返す。

## チェックリスト

完了報告前に確認:

- [ ] スコープ解決済み（変更ファイル / base diff / 明示パス）。
- [ ] 各対象 `*_test.go` に対し subject ソースが解決済み。
- [ ] Step 1 で 層 README + `docs/testing-conventions.md` + `scaffold-test/SKILL.md` + sibling tests を読んだ。
- [ ] 5 レンズが（並列で）走った。
- [ ] Lens 5 は subject シンボル表を構築し、`TestXxx` が 1 つも無いシンボルを（→ 補完推奨）Lens 4 の分岐分析より前に挙げた — コード起点 2 レンズがゼロテストのシンボルを二重報告していない。
- [ ] Lens 4 は subject ごとに両軸を走らせた — 軸A 分岐網羅（未カバー分岐 → 追加検討）/ 軸B 意味網羅（カバー済みだが意味未検証 → 再考）。
- [ ] 各 finding が `review-verifier` を通過した（`skip_verifier: true` 親指定以外）。
- [ ] REFUTED は落とし、 CONFIRMED / PLAUSIBLE のみ残した。
- [ ] 最終レポートが日本語、 lens 別、 severity tag 付き。
- [ ] 次アクション提案が 1 つの具体提案。
- [ ] ファイル編集していない。
