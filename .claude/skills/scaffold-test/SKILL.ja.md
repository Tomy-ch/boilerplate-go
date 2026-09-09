> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Scaffold Test

既存の関数 / メソッドに対する Go ユニットテストファイルを生成するスキル。書き方は `internal/domain/<aggregate>/<aggregate>_domain_test.go` から抽象化したパターン（全階層 `t.Parallel()` ＋ ネスト `t.Run` ＋ 日本語ケース名 ＋ 最外殻 `正常系` / `異常系` グループ、table-driven の `for` ループは原則禁止）に従う。

## 使うとき

- 既に実装が存在しコンパイルが通っている関数 / メソッドに対し、ユニットテストを追加するとき。
- `make go-test` のカバレッジが 90 % を下回ったパッケージを埋めるとき。
- 手動編集で挙動が変わった関数について、既存テストが意図を反映しなくなったとき。
- `scaffold-domain` / `scaffold-usecase` / `scaffold-controller` / `scaffold-infra-db` から chain される一段として（親が target + 観点を渡すので、First Step と Step 2 はスキップ）。

使わない場面:

- **HTTP 境界の統合テスト**（`internal/integration/` 配下）→ `scaffold-integration-test` を使う。本スキルは同一パッケージ内ユニットテスト専用。
- 既存テストファイル全体の書き直し → 手で編集する。
- 実装コードの生成 → 本スキルは test ファイルしか書かない。
- 生成ファイル (`*.gen.go` / `*_mock.go` / `*.sql.go`) に対するテスト。

## 読む / 書く

**常に読む**:

- `docs/testing-conventions.md`（parallel 必須・命名・require vs assert・mock 方針・層構造ルール）**および section 10 の意味的品質バー / アンチパターン** — 生成テストが満たすべき SSOT（各ケースはその分岐固有の outcome を assert し、列挙されたアンチパターンを出力しない）。 `test-review` も同じ節を読んでレビューするため、生成器とレビュアは対称に保たれる — 観点・アンチパターンのリストを本スキルへ複製しない。
- 対象ソースファイル（シグネチャ・引数・戻り値・エラーセンチネル・package 内ヘルパを抽出）。
- 層別 README。**対象ファイルから上位ディレクトリへ歩き、Test Strategy 節を実際に持つ最も近い祖先 `README.md`** を採用する（見出しの表記は README ごとに揺れる — `Test Strategy` / `Test strategy` / `Testing strategy` / `Testing Strategy` / `Testing Policy` — ので意味で判定すること。その層のテスト戦略そのものであれば名前が何であれ該当し、他のドキュメントが名前で参照している節をこの規則に合わせて改名するのは誤った直し方である）。節を持たないより近い README も併読する（観点は祖先から来ても、そのパッケージの命名・ヘルパ・不変条件の規約はそこが持つ）。解決は lookup ではなく walk で行う: 下記は現時点で walk が着地する先のスナップショットであり、一覧に無い層は「歩いて辿る対象」であって「対象外」ではない。
  - `internal/domain/README.md` （`internal/domain/**`）
  - `internal/usecase/README.md`（＋ `internal/usecase/boundary/README.md`）（`internal/usecase/**`）
  - `internal/controller/README.md` — controller 層の基準。駆動方式ごとにスコープされる: HTTP ハンドラ（＋ `internal/controller/handler/README.md`）は `internal/controller/handler/**`、ループ駆動の controller は `internal/controller/outbox/**` / `internal/controller/worker/**`
  - `internal/controller/httpstack/README.md`（`internal/controller/httpstack/**`）— 各ミドルウェアのサブパッケージはこの親へ解決される
  - `internal/controller/server/README.md`（`internal/controller/server/**`）
  - `internal/infrastructure/README.md`（＋ `internal/infrastructure/rdb/README.md`）（`internal/infrastructure/**`）
  - `internal/di/README.md`（`internal/di/**`）。対象が配下にある場合はより近い `internal/di/module/README.md` / `internal/di/server/hook/README.md` が優先される
  - `internal/apperror/` / `internal/cli/` / `internal/config/` / `internal/logging/` / `internal/observability/` / `internal/system/` — 横断的な基盤パッケージ群。各パッケージルートに自前の節を持つ
  - `pkg/README.md`（＋ 最寄りの `pkg/<name>/README.md`）（`pkg/**`）
- 同一パッケージ内の他 test ファイル（import 構成、ヘルパスタイル `newValidUser(t)` 等、assertion 文体、fixture 慣例）。README と矛盾するときは **README 優先**。
- `<package>/mock/*_mock.go` （対象が DI 注入インタフェースを使う場合のみ）。

**書く（承認後のみ）**:

- 対象と同じ階層に 1 つの test ファイル `<subject>_test.go`（`<subject>` は元ファイルから拡張子を除いた basename）。既存ファイルがある場合は **末尾追記** モードに切り替え、事前に承認を取る。

**`make` 経由で発火**:

- `make go-fix`（生成 test ファイルの自動整形）
- `make go-test`（生成テストの実行 + パッケージ全体カバレッジが落ちないことの確認）

**触らない**:

- 対象ソースファイル本体（`<subject>.go`）。
- 生成成果物（`**/*.gen.go` / `*_mock.go` / `*.sql.go`）。
- `*/mock/` 配下（読むだけ）。
- 他パッケージの test ファイル。

## First Step: 対象解決

`scaffold-*` から chain されている場合を除いて、最初に `AskUserQuestion`:

- 質問: 「テストを書きたい対象を指定してください」
- 選択肢（single-select）:
  - 「対象ファイル全体」 — free-text path。ファイル内の export 済み top-level 関数 / メソッドを全て列挙し、それぞれに 1 つの `TestXxx` を生成。
  - 「ファイル内の特定関数 / メソッドのみ」 — free-text `<file>:<symbol>`。指定の 1 件に対してのみ `TestXxx` 生成。
  - 「キャンセル」。

解決後、上述の walk 規則をファイルパスに適用して層を判定し（固定の層プレフィックス集合とのパターンマッチはしない）、解決された README を以降のステップで使う。

対象ファイルが存在しない場合は中断してパス確認。

## Step 1. 層コンテキストを読む

1. 上述の層別 README を読む。命名・ヘルパスタイル・層固有のテスト規約（domain の `pkg/ptr.Copy` 不変性チェック、controller の `testecho` 利用、infra の `pgerror.NormalizeError` アサーション 等）はここに canonical。
2. 対象パッケージ内の全 `*_test.go` を読む。次を抽出:
   - top-level test ヘルパ（例: `newValidUser(t *testing.T) (*User, time.Time)`）。
   - `TestXxx` 関数冒頭で慣例的に宣言されている fixture 変数。
   - 実際に使われている assertion スタイルと import セット。
3. `docs/testing-conventions.md` を 1 回読み、parallel 必須・命名・require vs assert・mock 方針・層構造制約として扱う。

sibling と README が矛盾する場合、**README 優先**。

## Step 2. テスト観点 subagent

コード生成前に必ず subagent (`subagent_type: general-purpose`) を起動して **対象 subject に対する** テスト観点を列挙させる。このステップは必須でメインループにインライン化しない（観点は層 README の Test Strategy 節 + 対象シグネチャから派生させるもので、スキル自身の記憶からパターンマッチさせる対象ではない）。

このスキルは層別の観点シードリストをハードコードしない。観点の SSOT は層 README （canonical）で、本スキルは README が現時点で書いている内容にそのまま従う。README が進化すれば観点も自動的に追従する — スキル側の編集は不要。

プロンプト内容（日本語）:

- 対象 subject のシグネチャ + Doc コメント。
- 該当層 README の `Test Strategy` / `Testing strategy` 節（存在する見出しの方）の **全文**。Step 1 で読み取った内容をサブセクション見出しごとそのまま渡す。subagent の仕事はそれらの見出しを対象 subject に対する具体観点へ落とすこと。本スキル作成時点での READMEs の状況（記述的、固定マップではない）:
  - `internal/domain/README.md` → `## Testing strategy`（Getter contract / Immutable guarantee / Domain behavior / Error classification / Test design policy / Test Fixture / Invariant preservation）
  - `internal/usecase/README.md` → `## Testing Strategy`（Test dependencies / Testing goals / Test targets / Test structure / What not to test）
  - `internal/controller/handler/README.md` → `## Test Strategy`（Test Dependencies / Test Targets / Test Structure / Router Test / Handler Test / Error Test / Thin Controller Test Scope / Observability Test / Test Policy / Not Covered in Controller Tests / Test Kit testkit / testassert / testauth / testecho / testspan）
  - `internal/controller/httpstack/README.md` → `## Test Strategy`（実体を使う対象とモックにする対象 / 全ミドルウェア共通で押さえる観点 / `Before` `After` フックの観点）— ミドルウェアは単体として独立にテストし、運用系パス除外・`server.ResponseOf` の nil 縮退・フックの発火/複数回発火/非発火が反復して現れる観点
  - `internal/controller/server/README.md` → `## Test Strategy`（Echo コンテキストのユーティリティ / サーバーの構築）
  - `internal/infrastructure/README.md` + `internal/infrastructure/rdb/README.md` → `## Test Strategy` / `### 7. Test Strategy (Integration-based)`
  - `internal/di/README.md` → `## Test Strategy`（DI レイヤの基準: グラフ妥当性 / provider 本体 / ライフサイクルフック / 環境ゲート付き配線）。サブツリーの詳細は `internal/di/module/README.md` と `internal/di/server/hook/README.md` が持ち、後者は HTTP フックの 3 経路（bind 失敗 / graceful shutdown / ログのみの `Serve` 終了）を明示する
  - `pkg/README.md` → **意図的に Test Strategy 節を持たない**。`pkg/` は framework-agnostic な pure utility (`docs/testing-conventions.md`) で、観点は標準 Go テストパターン（input-output 検証 / edge / boundary 値 / nil / zero ハンドリング）に帰着し、sibling tests（`pkg/datetime` / `pkg/envutil` / `pkg/ptr` / `pkg/uuid` / `pkg/xerrors` 等の既存テスト）でパターンが明示されている。subagent は sibling tests + `docs/testing-conventions.md` から観点を派生する。ドキュメントの穴ではなく **層として正常**なので、gap 警告は出さない。 package 個別の不変条件は `pkg/<name>/README.md` を併読する。
  この一覧は本スキル作成時点の READMEs を記述したもので、固定マッピングではない — README が更新されれば subagent はその時点の見出しを読んで適応する。見出しが renamed / removed / added されても、subagent は実行時点の README をそのまま使う。
- Step 1 で確認した sibling のテストパターン（補助参照）。
- `docs/testing-conventions.md`（プロジェクト横断ベースライン）。

期待される戻り値: 生成すべき `TestXxx → t.Run(正常系) → t.Run(case)` / `t.Run(異常系) → t.Run(case)` のパスを構造化したリスト（各 case に対応する README サブセクション or sibling test パターンを添付）。

フォールバック:

- **層が `pkg/**`** — README が意図的に Test Strategy 節を持たない。観点は標準 Go パターン（input-output / edge / nil / zero）に帰着するので、 sibling tests + 該当する `pkg/<name>/README.md`（あれば）+ `docs/testing-conventions.md` から派生し、**警告は出さない**。これが pkg 層の正常モード。
- **対象が `internal/**` 配下で、上位へ歩いてもリポジトリルートまで Test Strategy 節が見つからない場合** — gap として user に surface（`「<歩いたパス> のいずれにも Test Strategy 節がないため、sibling テストパターン + docs/testing-conventions.md からフォールバックで観点を導出しています。README を補完する余地があります」`）。**`internal/**` の全ての層が節を持つことを期待する**。唯一の免除は `pkg/**`。今たまたま節を持っている層のリストへ狭めないこと — その狭め方こそが、ミドルウェア / サーバライフサイクル / DI 配線といった層まるごとを比較基準の無いまま放置させ、しかもチェックは何も報告しない状態を作った原因である（未列挙の層が「未文書」ではなく「免除」に見えてしまう）。節を置くべき場所がユーザに分かるよう、歩いて通過した README を明示する。
- subagent が観点を返さない場合（層を問わず）、最小デフォルト（正常系成功 1 件 + 異常系全捕捉 1 件）にフォールバックしてユーザに警告。

## Step 3. テスト構造の設計

ハードルール:

1. **1 関数 / メソッド = 1 `TestXxx`**。 `Foo` → `func TestFoo(t *testing.T)`、`(*User).UpdateProfile` → `func TestUser_UpdateProfile(t *testing.T)`。 同一 subject に対する複数 `TestXxx` は絶対に作らない。
   - **逆方向 — 公開関数 / メソッドは各々が自分の `TestXxx` を持ち、1:1 の対応は弱いテストの回避より優先する。** 現時点で誠実に書ける assert が薄い（例: 些末なコンストラクタに対する `assert.NotNil(NewClock())`）というだけで、その subject 専用の `TestXxx` を削ったり作らなかったりしてはならない。将来の意味あるテストの置き場所として 1:1 の枠を残す。他の subject のテストから推移的に実行されていても、自分の `TestXxx` を持たない公開 subject は 1:1 違反である。
   - **ある subject の検証を別の subject の `TestXxx` に畳み込まない** — 畳み込まれた側のテストの責務が濁る。`NewClock` の契約を `TestClockNow` の中で assert するのが畳み込みであり、コンストラクタ自身の assert は `TestNewClock` に属する。メソッドのテストが SUT を得るための fixture としてコンストラクタを*呼ぶ*のは畳み込みではない（畳み込みとは、コンストラクタ自体についての別の assert をメソッドのテストへ加えることを指す）。
2. **複数 subject を 1 `TestXxx` に束ねない — 厳密 1:1、例外なし**。 全 getter を `TestEntity_Accessors` / `*_Getters` で一括検証するような統合テストは作らず、getter / accessor ごとに 1 つの `TestXxx` を用意する。 `AskUserQuestion` による束ねの分岐も rationale コメントによる免除も無い。 唯一の免除は、**検証不可能であるために到達できない** subject（例: 失敗経路が `tb.Fatalf` を呼ぶヘルパーは呼び出し側テストの終了を伴う）で、その場合も規約どおりの名前の `TestXxx` を宣言し `t.Skip("<なぜ検証不可能か>")` を呼ぶ — allowlist は持たず、理由は `t.Skip` の文字列に残す。 **「他のテストでカバー済み」は免除にならない**: その skip は subject を別テストの実装に依存させ、カバー元が縮小しても green のまま残るため、呼び出し元 / 統合 / DI グラフテストがたまたま通っていてもテスト可能な subject には実テストを書く。 `docs/testing-conventions.md` §1 に準拠し、`internal/architest`（1:1 枠は `TestUnitTestMappingCompleteness`、skip 理由は `TestSkipReasonDoesNotNameCoveringTest`）が機械的に強制する。
3. **最外殻 2 つの `t.Run` の name は 必ず literal の `正常系` / `異常系` の 2 文字のみ**。 prefix 形式 (`正常系_xxx` / `異常系_xxx`) は NG。
   - 使うのは `t.Run("正常系", ...)` と `t.Run("異常系", ...)` のみ。group name はリテラルの 2 文字であって、 case 名のプレフィックスではない。
   - **禁止パターン**: 最外殻に `t.Run("正常系_ユーザーが存在する場合", ...)` を書くこと。 `正常系_` / `異常系_` プレフィックスをサブケース名に付けるのも、 「グループ軸 (正常系/異常系)」と「ケース説明軸 (具体的に何を試すか)」を混同させる。
   - **正しい形**:

     ```go
     t.Run("正常系", func(t *testing.T) {
         t.Parallel()
         t.Run("ユーザーが存在する場合エンティティを返す", func(t *testing.T) { ... })
         t.Run("ユーザーが論理削除済みの場合は除外する", func(t *testing.T) { ... })
     })
     t.Run("異常系", func(t *testing.T) {
         t.Parallel()
         t.Run("IDがゼロ値の場合エラーを返す", func(t *testing.T) { ... })
     })
     ```

   - 両 group の直後に `t.Parallel()` を呼ぶ。 さらに細分化のためのネストグループ（例: `t.Run("firstNameが範囲外の場合、エラーを返す", ...)`) は可読性が上がるなら推奨で、 正常系 / 異常系 group の **内側に** 置く。
   - 1 つの `TestXxx` には `正常系` group が最大 1 個、 `異常系` group が最大 1 個。 正常系のみで構成されるなら `異常系` group は作らない（逆も同様）。 空のグループは作らない。
4. **全ての `t.Run` の冒頭で `t.Parallel()` を呼ぶ**。例外: sibling ブロックと共有しているポインタを mutate する場合は外側の `t.Run` を逐次にする。**ブロック直上にコメント必須**（`-race` で検出される競合を意図的に避けている旨を書く）。内部 case は引き続き `t.Parallel()`。
5. **table-driven `for` ループは禁止 — 常に逐次 `t.Run` sibling で書く**。 各ケースをそれぞれ独立した `t.Run` にする（`user_domain_test.go` のパターン）。`(input, expected)` の構造体スライスを `for _, tc := range cases` で回さない。個別に書き出すことで、失敗時に該当ケース名が出て、各ケースが `t.Parallel()` を呼べ、共有ループ本体でケース同士が結合しない。ゲッター/境界の似たアサーションが長く並ぶ場合でも同様（重複は許容し、table へ畳み込まない）。ケース単位の例外は無い — 尋ねず、逐次 `t.Run` で書く。
6. **ケース名は日本語。 サブケース名に `正常系_` / `異常系_` プレフィックスは付けない**。 最外殻 group の name はリテラルの `正常系` / `異常系`。 サブケースは入力クラスと期待結果を 1 文で表す自由記述（`「<入力クラス>の場合、<結果>」`）。 そのまま読める文章になるように。サブケースは既に 正常系 / 異常系 group の下にいるため、 名前に `正常系_` / `異常系_` を付けると `正常系 > 正常系_xxx` のような二重ラベルになり冗長 → 禁止。 prefix は剥がす。
7. **`require` vs `assert`**（`docs/testing-conventions.md` 準拠）:
   - `require.NoError` / `require.Error` / `require.ErrorIs` / `require.ErrorContains` — エラー系アサーション全般（testifylint `require-error` ルールが `assert.ErrorIs` を拒絶）。
   - `require.Not<Nil>` は以降の dereference をガードする場合のみ。
   - `assert.Equal` / `assert.Len` / `assert.Contains` / `assert.True` / `assert.False` / `assert.Empty` は終端の値検証（失敗時に以降を止めない）。
8. **mock は常に `<package>/mock/` から**（`go.uber.org/mock` + `make gen-api` の生成物）。手書き mock は `docs/testing-conventions.md` で禁止。
9. **DB / HTTP / 外部 IO はユニットテストでは行わない**（domain / usecase / controller 層）。 infra の実 DB 結合は、同パッケージにすでに sibling test がいて慣行が確立されている場合のみ本スキルの守備範囲。それ以外は `scaffold-integration-test` への切替を案内する。

## Step 4. プランの確認

日本語で:

- 対象ファイルパス
- 検出した層
- 生成予定の各 `TestXxx` と、その配下の 正常系 / 異常系 サブケースのリスト
- 各 case を生んだ観点との対応理由
- 提案テストファイル冒頭 ~20 行のプレビュー
- 検証不可能なため skip-test（`TestXxx` + `t.Skip("<なぜ検証不可能か>")`）として出力した subject（なぜ検証できないかを明記）

を提示してから `AskUserQuestion`:

- 質問: 「以下の構成でテストを生成しますか？」
- 選択肢: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」。

## Step 5. test ファイル書き出し

`internal/domain/<aggregate>/<aggregate>_domain_test.go` から抽象化した骨格に従う:

```go
func Test<Subject>(t *testing.T) {
    t.Parallel()

    // サブテスト間で共有する fixture をここで宣言。time は固定 baseTime に pin して
    // determinism を確保する。
    <fixture variables>

    t.Run("正常系", func(t *testing.T) {
        t.Parallel()

        t.Run("<日本語の入力クラス>の場合、<期待される結果>", func(t *testing.T) {
            t.Parallel()
            actual, err := Subject(<args>)
            require.NoError(t, err)
            assert.Equal(t, <expected>, actual.<field>)
        })
        // ... 追加の正常系
    })

    t.Run("異常系", func(t *testing.T) {
        t.Parallel()

        t.Run("<日本語の入力クラス>の場合、エラーを返す", func(t *testing.T) {
            t.Parallel()
            actual, err := Subject(<invalid args>)
            assert.Nil(t, actual)
            require.ErrorIs(t, err, <ErrSentinel>)
        })
        // ... 必要なら細分化のためにネスト
    })
}
```

追加の生成ルール:

- **意味的品質バー（`docs/testing-conventions.md` section 10）を満たす。** 生成する各ケースはその分岐*固有*の outcome を assert すること — エラー分岐は固有 sentinel を `require.ErrorIs`、成功 / 状態変更分岐は結果の値 / フィールド、境界は両側 — 分岐が実行されたことだけを示す空虚な `require.NoError` / `assert.NotNil` のみの本体にしない。 §10 のアンチパターン（弱いアサーション、ケース名の過剰約束、内部結合の脆さ、過剰モック、時刻リテラル漏れ、冗長コメント）を一切出力しない。 これは `test-review` がレビューする節と同一 — リストを本スキルに再掲せず runtime で読む。
- **テストヘルパ**は package 内既存のもの（例: `newValidUser(t)`）を再利用。 未定義かつ同じ fixture を 3 回以上繰り返すならば、test ファイル末尾に `t.Helper()` 付きの unexported ヘルパを生成。
- **`uuid.NewTestFromSalt(t, "<salt>")`** がテスト内で deterministic な UUID を取得する canonical 手段（`pkg/uuid`）。
- **`ptr.To(...)` / `ptr.Copy(...)`** は nullable ポインタフィールドで利用（domain README の "Handling time and ID" / "Invariants" 節 参照）。
- **mock セットアップ**: 対象が DI 注入インタフェースを使う場合、対応する `mock.NewMock<Interface>(t)` を生成し、 `.EXPECT().<Method>(...).Return(...)` を対象の呼び出し順序通りに並べる。
- **import**: 同一 package の sibling test と同じ構成に揃える。未使用 import は入れない。

test ファイル既存の場合は全書き換えしない — 末尾追記モード（既存ヘルパ宣言の後ろ）に切り替え、`AskUserQuestion` で事前承認を取る。

## Step 6. 検証

順次:

1. `make go-fix` — 生成 test ファイルを整形。対象外ファイルが整形されたら diff を提示。
2. `make go-test` — 生成テスト通過確認 + パッケージのカバレッジが下がっていないこと（`docs/testing-conventions.md` の新規 / 変更パッケージ 90 % 閾値）。
3. **回帰の要となるケースはミューテーション検証する。** 特定の振る舞いを固定する／既知・直前修正のバグを守るために書いたケース（汎用カバレッジ目的でないもの）は、**subject** に退行を一時注入（条件反転・ガード削除・フィールド/引数すり替え）して当該テストだけ再実行し、**FAIL すること**を確認してから注入を戻す。注入下でも通るテストは何も守っていない＝アサーションを FAIL するまで強化する。これが本物の回帰テストとトートロジーの差。全ケースでなく回帰の要だけで行う。

`make go-test` 失敗時:

- 生成 test ファイルはそのまま残す（user 検査用）。
- 失敗テスト名 + assertion メッセージを提示。
- 自動 rollback はしない。テスト修正か subject 修正かは user 判断。

カバレッジが 90 % 未満に落ちた場合（テスト自体は通る場合も）は、不足観点を提示して再呼び出しを促す。

## Chainability

`scaffold-domain` / `scaffold-usecase` / `scaffold-controller` / `scaffold-infra-db` から chain された場合、親は最低限以下を渡す:

- `target_file` — テスト対象ソースの絶対パス。
- `target_subjects` — 親がテストを欲しい `<func or method name>` のリスト。
- `layer` — 解決済み層キー（`domain` / `usecase` / `controller` / `infra` / `pkg`）。
- `viewpoints` — 親が既に subagent から得たテスト観点（あれば）。

chain モードでは以下をスキップ:

- First Step（target 解決）。
- Step 2（test-perspective subagent）— `viewpoints` が非空のとき。
- Step 4 の `AskUserQuestion`（親が feature 単位の承認を既に取得済み）— 監査用に 1 行サマリは表示する。

厳密 1:1 ルール（束ね禁止、免除は検証不可能な subject に限る）は chain モードでも同様 — 束ねは常に不許可なので確認を取る対象自体が無い。（table-driven にも例外は無い — 禁止であり常に逐次 `t.Run`。）

## 制約（サマリ）

- ❌ 生成テストにコード言い換え／*なぜ*の説明コメントを足す — テストコメントは最小（振る舞いのみ）。ケースの意図は日本語 `t.Run` 名で表し、インラインコメントに書かない（godoc 以外で必須なのは `-race` 直列ブロック例外の理由コメントのみ）。書く価値のあるコメントでも1行に収める — フィクスチャや慣用的なアサーションへの複数行の説明は、読むコストが返る情報を上回る。
- ❌ 同一関数 / メソッドに対する複数 `TestXxx`。
- ❌ 複数 subject を 1 `TestXxx` に束ねる（厳密 1:1、例外なし。getter / accessor 含む）。 subject ごとに named `TestXxx` を用意する — 束ねない。
- ❌ 「他のテストがカバーしている」を理由にした `t.Skip`（テストが別テストの実装に依存する）。 skip は検証不可能なものに限り、*なぜ検証できないか* を書く。
- ❌ table-driven `for` ループ（禁止 — 常にケース毎の逐次 `t.Run` sibling で書く）。
- ❌ 手書き mock（`<package>/mock/*_mock.go` を使う）。
- ❌ subject ソースファイルの編集。
- ❌ 生成物の編集（`*.gen.go` / `*_mock.go` / `*.sql.go`）。
- ❌ `assert.ErrorIs` / `assert.NoError`（testifylint `require-error`）。
- ❌ コメント付き `-race` 理由なしでの `t.Parallel()` 省略。
- ❌ サブケースでの `t.Run` 省略。
- ❌ 英語ケース名（`docs/testing-conventions.md` の通り日本語）。
- ❌ 最外殻に `t.Run("正常系_xxx", ...)` / `t.Run("異常系_xxx", ...)` を書く（外殻 group の name は literal `正常系` / `異常系` のみ）。
- ❌ サブケース名に `正常系_` / `異常系_` プレフィックスを付ける（外殻 group で既に区別済み）。
- ✅ 全階層 `t.Parallel()`（明文化された例外のみ可）。
- ✅ サブケース毎の `t.Run`。
- ✅ 日本語ケース名。 最外殻 group は literal `正常系` / `異常系`、 サブケース名は prefix なしの日本語自由記述。
- ✅ エラーは `require` / 終端値は `assert`。
- ✅ mock は `*/mock/` 由来のみ。
- ✅ deterministic な fixture（固定 `baseTime`、`uuid.NewTestFromSalt(t, ...)`）。
- ✅ 生成 test ファイル内ヘルパに `t.Helper()`。
- ✅ section 10 の意味的品質バーを満たす（各ケースは分岐固有の outcome を assert・アンチパターンを出力しない）— `docs/testing-conventions.md` から読み、本スキルに再掲しない。
- ✅ 回帰の要となるケースはミューテーション検証（subject に退行を注入→テストが FAIL することを確認→戻す）。注入下で緑のままのケースはガードでなくトートロジー。

## チェックリスト

完了報告前に確認:

- [ ] 対象ファイル + 層が確定（standalone）または親 scaffold-* から受領（chain）した。
- [ ] Step 1 で層 README + `docs/testing-conventions.md` + sibling test を読んだ。
- [ ] Step 2 の test-perspective subagent を実行した（または親から `viewpoints` を受領）。
- [ ] 各 `TestXxx` が単一 subject に対応している（厳密 1:1、束ねなし）。 検証不可能な subject のみ named `TestXxx` + `t.Skip("<なぜ検証不可能か>")` で出力し、他テストを名指しした skip 理由が無い。
- [ ] 最外殻 `t.Run` group の name が literal `正常系` / `異常系` （`正常系_xxx` 形式ではない）。 サブケース名にも `正常系_` / `異常系_` プレフィックスが含まれない。
- [ ] 全 `t.Run` の冒頭で `t.Parallel()` を呼んでいる、または `-race` 例外の説明コメントが付いている。
- [ ] `for` ループ table は生成していない（禁止 — 常に逐次 `t.Run` sibling、例外なし）。
- [ ] エラー系は `require.*`、終端値は `assert.*`。
- [ ] mock は `<package>/mock/` 由来のみ。手書き mock を追加していない。
- [ ] subject ソースは編集していない。
- [ ] `make go-fix` + `make go-test` が pass し、カバレッジが落ちていない。
