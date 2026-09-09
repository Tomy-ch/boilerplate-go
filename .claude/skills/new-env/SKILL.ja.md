> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# New Env

新規環境変数をプロジェクトに end-to-end で追加するスキル。`Loader` 構造体での読み込み・型付き `Config` へのマッピング・getter 公開・各環境サンプル `env` ファイルへの設定・`env/README.md` へのドキュメント追記を一貫して行います。

## 使うとき

- 新規 env var が必要なとき（feature flag、調整可能な timeout、外部サービス URL 等）
- 複数ファイルを同期更新する必要があり、一貫性を保ちたいとき

以下の用途には使いません:

- 既存 env var のリネーム（別ワークフロー、全箇所を一斉リネーム）
- 既存 env var の削除（逆方向の操作で手動推奨）
- 新規サブシステムの追加全体（例: `RedisConfig` 群の新設）。本スキルは既存サブシステム前提。新規サブシステムは最初の変数を手動追加してから本スキルを使用

## 読み書き範囲

**読み込み（常時）**:

- `internal/config/envspec.go` — Loader サブシステム一覧（envPrefix → struct 対応）
- `internal/config/model.go` — Config サブシステム一覧（private フィールド命名）
- `internal/config/config.go` — `New()` 本体（マッピングパターン）、既存 getter（命名規約）
- `internal/config/config_testing_mock.go` — `expected*` 変数ブロックと mock のフィールド初期化
- `env/.env`（local 既定）, `.env.ci`, `.env.dev`, `.env.stg`, `.env.prd`, `.env.dast` — 環境別値の配置
- `env/README.md`, `env/README.ja.md` — テーブル形式とサブシステム節名

**書き込み（承認後のみ）**:

- 上記のファイル。各ファイル最小編集（1 箇所追加）

**触らない**:

- `config_testing_setter.go`（ユーザーが明示要求しない限り）
- `internal/config/` 以外の `internal/` コード
- 生成物

## 最初のステップ: 仕様を収集

**起動直後に必ず `AskUserQuestion` を呼ぶ**。複数質問を 1 回でまとめて収集（バッチ）。

### Question 1: 変数名とサブシステム

- 質問: 「環境変数名（`{PREFIX}_{NAME}` 形式）を入力してください。例: `APP_FEATURE_X` / `DB_READ_REPLICA_HOST`」
- フリーテキスト入力。スキルは:
  1. 最初の `_` で prefix を分離
  2. live `envspec.go` の `envPrefix:"XXX_"` で照合
  3. ヒット時: 「推定サブシステム: `Application` (envPrefix `APP_`)」を表示して確認
  4. ヒットなし: 候補を提示して prefix 変更を促す、または新規サブシステムは手動追加後に再実行を案内

### Question 2: Go 型と required / default

`AskUserQuestion`（4 択）:

- 質問: 「型と必須・デフォルトを選んでください」
- 選択肢:
  - 「string, required」
  - 「string, default あり（値を後で指定）」
  - 「int / bool / duration, required（型は後で指定）」
  - 「int / bool / duration, default あり（型と値を後で指定）」

後続のフリーテキストで具体型（`int` / `bool` / `time.Duration` / `[]string` 等）と default 値を収集。

### Question 3: 説明

フリーテキスト。ユーザーは **英語または日本語のどちらか**（または両方）を入力。スキルは欠けている側を inline 翻訳で補完し、両 README を同期させる（ユーザーに 2 度書きを強いない）。

- 「説明（日本語または英語のどちらか）」 — 該当する言語の README 行にそのまま記入
- Notes 列（任意） — Secret management / 環境依存等の注記。説明と同じ言語で受け、反対側は翻訳

解決ルール:

- 日本語のみ供与 → `env/README.ja.md` に日本語そのまま、`env/README.md` に英訳を記入
- 英語のみ供与 → 逆方向
- 両方供与 → そのまま使用、翻訳しない
- 翻訳は短く直接的（単行、周辺行と同じテクニカルレジスター）。説明が非自明 / ドメイン特化の場合、Step 2 のプランサマリで提案翻訳を surface してユーザーレビューを取る

### Question 4: 環境別値

`AskUserQuestion`:

- 質問: 「環境別の値を指定しますか？」
- 選択肢:
  - 「全環境同じ値（または default で OK）」
  - 「local だけ別の値を入れる」
  - 「prd だけ別の値を入れる」
  - 「環境ごとに個別指定する（追加質問）」

選択に応じて環境別値を収集。prd 特化の場合は placeholder commented out（`# DB_HOST` スタイル）の慣例を提示して、それに従うか確認。

### Question 5: モック / setter

- 質問: 「テストヘルパーをどこまで追加しますか？」
- 選択肢:
  - 「mock のみ（config_testing_mock.go の expected 値）」（推奨）
  - 「mock + setter（config_testing_setter.go にも追加）」 — 明確なテスト需要があるときのみ
  - 「テストヘルパーは追加しない」

## Step 1. 挿入箇所のプラン

各 touch point について、既存パターンを読み取って正確な挿入位置を決める:

### `envspec.go`

該当サブシステム構造体を見つけ、末尾に新規フィールドを追加（フィールド順序を維持）。タグ形式:

- required: `env:"NAME,required"`
- default あり: `env:"NAME" default:"value"`

フィールド型は Go 型と対応（`string` / `int` / `bool` / `time.Duration` / `[]string`）。

### `model.go`

該当 Config 子構造体（例: `ApplicationConfig`）の末尾に env 名対応の camelCase 名で private フィールド追加（例: `APP_FEATURE_X` → `featureX`）。

### `config.go`

2 箇所変更:

1. `New()` 内で該当サブシステムブロックを見つけ、フィールドマッピング追加（`fieldName: cfg.Sub.FieldName,`）
2. 子構造体に getter メソッド追加: `func (a *ApplicationConfig) FeatureX() T { return a.featureX }`。同じ子構造体の既存 getter をフォーマット template にする（単行 / 複数行、doc コメントスタイル）

### `config_testing_mock.go`

1. `expected*` ブロックに expected 値変数を追加（`expectedApplicationFeatureX = ...`）
2. `MockConfigForTest` が組み立てる子構造体リテラルに、その変数を使ってフィールドを追加

setter は `config_testing_setter.go` の側にあり、ここには置かない。追加するのは Question 5 で
opt-in されたときだけ。

### テスト更新（必須 — カバレッジを下げない）

`internal/config` パッケージは現在 100% カバレッジ。新規フィールドを追加すると、対応テスト更新を忘れた瞬間にカバレッジが下がる。production 変更と lockstep で 3 つのテストファイルを更新する:

| テストファイル | 更新内容 |
| --- | --- |
| `internal/config/config_test.go` | `TestNew` の期待値 `&Config{...}` リテラルに該当 sub-struct で 1 行追加。値は `config_testing_mock.go` で定義した `expected*` 変数を使用 |
| `internal/config/model_test.go` | getter 1 つにつき `Test<SubStruct>_<Field>` 関数を 1 つ追加（厳格に 1:1）。sibling 関数に倣う（各段で `t.Parallel()`、`正常系` グループ、`MockConfigForTest(t).<sub>`、`assert.Equal(t, expected<...>, sub.<Field>())`）。既存テストに束ねない |
| `internal/config/config_testing_mock_test.go` | `TestMockConfigForTest` の期待値 `&Config{...}` リテラルに該当 sub-struct で 1 行追加 |

`expected*` 変数は Question 5 の mock スコープで追加された `config_testing_mock.go` 由来。mock を opt-out した場合はテスト更新も skip となるため、Step 2 のプランで明示する: "テストヘルパー追加なし → カバレッジ維持テストもスキップ。100% から下がる可能性あり"。

### env ファイル

`env/.env`（local 既定）, `.env.ci`, `.env.dev`, `.env.stg`, `.env.prd`, `.env.dast` それぞれで:

- セクションコメント（例: `# Application`）を見つける
- 該当セクション下に新変数行を追記（整列を維持）
- prd はユーザーが prd 値を指定しない限り commented placeholder 慣例（`# APP_FEATURE_X`）をデフォルト

### `env/README.md` と `README.ja.md`

該当サブシステム表（例: `### Application`）の末尾に行追加:

```text
|APP_FEATURE_X|<description>|<type>|<example>|<notes>|
```

日本語 README は日本語の説明 / notes。

## Step 2. プラン表示と承認

提案変更内容を日本語サマリで表示:

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

`AskUserQuestion`:

- 質問: 「以上の内容で適用しますか？」
- 選択肢: 「適用する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 3. 変更適用

各ファイルに対して `Edit` ツールを使用し、読み取った context から正確な anchor 文字列（該当サブシステム / 節の最終既存フィールド）を導出。順序:

1. `envspec.go`
2. `model.go`
3. `config.go`（マッピング + getter）
4. `config_testing_mock.go`（expected 値変数 + mock リテラル）
5. `config_test.go`（TestNew リテラル — カバレッジ維持）
6. `model_test.go`（getter ごとの `Test<SubStruct>_<Field>` — カバレッジ維持）
7. `config_testing_mock_test.go`（TestMockConfigForTest リテラル — カバレッジ維持）
8. env ファイル（各 1 Edit: `env/.env`（local 既定）, `.env.ci`, `.env.dev`, `.env.stg`, `.env.prd`, `.env.dast`）
9. `env/README.md` → `env/README.ja.md`

各 edit 後に成功確認（`Edit` ツールが成否を返す。失敗時は停止して報告）。

## Step 4. 検証

```sh
make go-fix    # フォーマットを吸収
make go-test   # config ロード + getter のコンパイル、テスト通過、カバレッジ維持を確認
```

`make go-test` 失敗時は surface して停止。自動 rollback はしない（ユーザーが fix-forward を判断）。

`make go-fix` が編集対象外を変更した場合も diff を surface。

### カバレッジ確認

`make go-test` 成功後、テスト出力内の `internal/config` パッケージのカバレッジ行を確認。期待値: `go-boilerplate/internal/config <時間>  coverage: 100.0% of statements`。100% を下回った場合:

1. テスト未到達の新コードパスを特定（典型的には新 getter または `New()` 内の新マッピング行）
2. Step 3 のテスト更新（`config_test.go`, `model_test.go`, `config_testing_mock_test.go`）が全て新フィールドを含むか確認
3. 漏れがあれば追加して `make go-test` 再実行

mock + テスト更新を opt-out した場合を除き、カバレッジ低下のままスキル完了報告をしない。

## Step 5. クロージング

- 1 行サマリ: 「<VAR_NAME> を追加。<N> ファイル更新。make go-test OK。」
- スキルは commit しない。レビュー後に `/commit` 推奨
- `internal/config/README.md` も更新したい場合（個別変数の説明ではなく package 全体）は `/sync-readme` を推奨

## AI 修正スコープ

CLAUDE.md / AGENTS.md の "Exception: Skill Execution" により、本スキル実行中は AI Modification Scope の制約が緩和される。スコープ:

- 上記のファイル（またはユーザーが確認したサブセット）

保護対象:

- `internal/config/config_testing_setter.go`（ユーザー明示 opt-in 時のみ）
- `internal/config/` 以外のすべてのコード
- 生成物

## 制約事項

- ❌ サブシステムリストをハードコードする — 必ず live `envspec.go` から導出
- ❌ `config_testing_setter.go` ヘルパーをユーザー明示 opt-in なしで追加
- ❌ 列挙した 6 ファイル以外の env ファイルを変更
- ❌ 仕様確認 `AskUserQuestion` をスキップ
- ❌ プラン表示なしで変更を適用
- ❌ 新変数と無関係なコードに触る
- ✅ ユーザー向け出力は日本語
- ✅ env ファイルのフォーマット維持（列整列、コメントスタイル）
- ✅ README テーブルのフォーマット維持（列数と順序）
- ✅ 書き込み後に `make go-fix` + `make go-test` を実行
- ✅ 検証失敗を surface、自動 rollback しない

## チェックリスト

- [ ] `AskUserQuestion` で変数名 + サブシステムを確認
- [ ] Go 型と required/default を確認
- [ ] 説明を英語または日本語のどちらかで受け、欠けている側を自動翻訳して Step 2 プランでレビュー surface
- [ ] 環境別値を解決（単一 default または環境別 override）
- [ ] mock ヘルパースコープを確認
- [ ] プラン全体を表示してユーザーが承認
- [ ] `envspec.go`, `model.go`, `config.go` を該当サブシステムで 1 箇所挿入で更新
- [ ] `config_testing_mock.go` を更新（スキップ選択時を除く）
- [ ] `config_test.go`, `model_test.go`, `config_testing_mock_test.go` を新フィールドカバー用に更新（mock スコープ skip 時は明示記録）
- [ ] 6 env ファイル全てを該当サブシステムコメント配下に追加
- [ ] `env/README.md`, `env/README.ja.md` にテーブル行追加
- [ ] `config_testing_setter.go` は明示要求時のみ更新
- [ ] `make go-fix`, `make go-test` を実行し結果を報告
- [ ] テスト出力で `internal/config` のカバレッジ 100.0% を確認（mock スコープ skip 時は明示低下記録）
- [ ] commit / push を行っていない
- [ ] 最終サマリは日本語
