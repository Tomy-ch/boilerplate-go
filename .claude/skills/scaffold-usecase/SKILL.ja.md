> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Scaffold Usecase

`docs/spec/usecase/<pkgpath>.md`（`<pkgpath>` は `internal/usecase/` 以下のパッケージパス）を入力に、1 feature の usecase 層を 1 パスで生成するスキル。interface / struct / constructor / methods / DTOs / tests / DI 登録を生成する。

## 使うとき

- `scaffold-domain`（entity + Repository IF）と `scaffold-infra-db`（Repository impl）完了後
- `scaffold-endpoint` の 3 番目のステップ（自動 chain）
- usecase 層だけ scaffold したい単独利用（domain / infra 既存ケース）

以下の用途には使いません:

- 既存 usecase パッケージの変更（skill は新規パッケージ前提）
- 既存 usecase に 1 メソッド追加 — 手編集
- 新規 boundary interface の作成（`internal/usecase/boundary/` 配下は手動 bootstrap）

## 読み書き範囲

**読み込み（常時）**:

- `docs/spec/usecase/<pkgpath>.md` — single source of truth
- `internal/usecase/README.md` — layer 規約
- `internal/usecase/boundary/README.md` — boundary 規約
- `internal/usecase/<sibling>/<sibling>_usecase.go` — **二次** 参照（README の Implementation Example が canonical、既存コードは観察のみ）
- `internal/domain/<aggregate>/<aggregate>_repository.go` + entity — `calls:` 参照検証
- `internal/usecase/boundary/<name>/` — spec の `Dependencies` boundary 存在検証
- `internal/di/module/usecase.go` — DI 登録対象

**書き込み（承認後）**:

- `internal/usecase/<package>/<package>_usecase.go`（interface + `//go:generate mockgen` + DTOs + struct + constructor + methods）
- `internal/usecase/<package>/<package>_usecase_test.go`（gomock テスト）
- `internal/di/module/usecase.go`（`UsecaseModule` の `fx.Provide(...)` に `<package>.New` 追加）

**`make` トリガ**:

- `make gen-api` — `internal/usecase/<package>/mock/` 配下に Usecase mock 再生成
- `make go-fix` + `make go-test` — 最終検証

**触らない**:

- domain / infra 層
- boundary interface 定義
- `docs/spec/` ファイル
- 生成 mock ファイル（手編集禁止）

## 前提条件

書き込み前に確認:

1. `docs/spec/usecase/<pkgpath>.md` 存在 + パース可
2. spec の各 `calls:` 参照:
   - `<aggregate>.Repository.<Method>` → domain Repository IF に存在
   - `<aggregate>.<BehaviorMethod>` or `<aggregate>.New` → domain に存在
   - `<boundary>.<Method>` → boundary 型が `internal/usecase/boundary/` に存在
3. `internal/usecase/<package>/` 未存在（あれば中断）

未充足時は中断、対応ステップ（`/scaffold-domain`、手動 boundary 作成、手動 cleanup）を案内。

## 最初のステップ: spec パス解決

`AskUserQuestion` 起動直後（`scaffold-endpoint` から context 提供時は除く）:

- 質問: 「対象 usecase パッケージパス（`internal/usecase/` 以下。例: `address`, `product/ranking`, `user/search`）」
- フリーテキスト。`docs/spec/usecase/<pkgpath>.md` として解決 — spec のツリーはパッケージのツリーを写すので、対象パッケージと spec パスは同じ 1 つの回答である

スタンドアロン代替: `--spec=<path>`

## Step 1. spec + 参考 context 読み込み

1. usecase spec の YAML を inventory にパース:
   - `interface`: package, name, methods (name + signature)
   - `dtos`: (name, fields) リスト
   - `dependencies`: (name, type) リスト — boundary + Repository IF
   - `workflow`: メソッドごと `(tx_required, steps, calls, errors)`
2. `internal/usecase/README.md` を読み layer 規約取得（特に "Application Service Design Policy"、"Time Handling Policy"、"Boundary Concept"、"Allowed dependencies"、"Forbidden dependencies"）
3. `internal/usecase/boundary/README.md` を読み boundary 規約取得
4. 既存 usecase パッケージ（例: `internal/usecase/<sibling>/<sibling>_usecase.go`）は **二次参照のみ** — observability tracer 配線、Tx wrap パターン、DTO 変換、error wrap などは README の Implementation Example が canonical。既存コードと README が衝突した場合 README が勝つ（README から drift したコードに skill が黙って従わない方針）
5. 各 `calls:` 参照を実コード（domain Repository IF、domain entity factory/methods、boundary 型）に対して検証

## Step 2. test 観点 subagent

実装前に Agent tool で subagent を起動し usecase 層 test 観点を列挙。

- `subagent_type: general-purpose`
- prompt（日本語）: spec inventory + `internal/usecase/README.md` "Testing Strategy" 節 + usecase 期待観点:
  - workflow 呼び出し順序（domain → repo → boundary が正しい順か）
  - mock 戦略: Repository / Boundary を mock、Domain entity は実物
  - transaction 境界の正当性（`tx.Manager.Do` wrap）
  - error 伝播（boundary error → return、Repository error → apperror へ map）
  - DTO 構成（最終返却値の構造）
  - 空 / zero-result 扱い
- 出力: メソッドごとのテストケースリスト

subagent が観点を返さない場合は最小デフォルトで継続、警告。

## Step 3. プランと承認

日本語サマリ表示:

- 生成予定ファイル + DI 更新
- 各メソッド: signature + workflow 概要（calls、tx_required）
- subagent のテストメソッドリスト

質問:

- 「以下の構成で usecase 層を生成しますか？」
- 選択肢: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 4. ファイル書き込み

順序:

1. `<package>_usecase.go`（interface + `//go:generate mockgen` + DTOs + struct + constructor + methods）
2. `<package>_usecase_test.go`（gomock テスト、subagent 観点使用）
3. `internal/di/module/usecase.go` 更新 — `usecase` の `fx.Provide(...)` に `<package>.New` 追加

実装ファイル convention:

- `package <package>`（lowercase aggregate name）
- ファイル先頭に `//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE`（リポジトリ共通の標準ディレクティブ・全 interface ファイルで同一）
- `type Usecase interface { ... }` を spec Interface から
- DTO は `type XxxDTO struct { ... }` を spec DTOs から
- `type usecase struct { tracer observability.LayerTracer; <deps...> }` を spec Dependencies から
- `func New(tf observability.TracerFactory, <deps...>) Usecase { return &usecase{tracer: tf.Usecase(), <init...>} }`
- 各メソッド本体:
  - `ctx, endSpan := u.tracer.Start(ctx); defer endSpan()`
  - `tx_required: true` なら body を `u.txm.Do(ctx, func(ctx context.Context) error { ... })` で wrap
  - Workflow steps を順序通りに実装し、宣言された `calls:` を呼ぶ
  - spec 通り error mapping 適用
  - DTO を return

テストファイル convention:

- subtest ごと gomock コントローラ
- Repository + Boundary を mock、Domain entity は実物（プロジェクト規約）
- 日本語 subtest 名
- subagent 観点リストをテストケースマップに使用

## Step 5. mock 再生成

```sh
make gen-api
```

新 usecase ファイルの `//go:generate mockgen` を処理し `internal/usecase/<package>/mock/mock_<package>_usecase.go.gen.go` を生成。ファイル存在確認。

## Step 6. 検証

```sh
make go-fix
make go-test
```

`internal/usecase/<package>` のカバレッジ行確認。プロジェクト規約で 100% 目標。低下時は未到達 error / branch を特定して追加推奨。

失敗時: TODO + FB summary、自動 rollback なし。

> **DI 検証（runtime）:** `go build` / `make go-test` は Fx グラフを構築しない — provider 欠落・`New` の未登録・コンストラクタのシグネチャ不整合は、コンパイル/テストではなく**アプリ起動時**に初めて失敗する。DI 登録（`fx.Provide(<package>.New)`）後はアプリが実際に起動するか確認する: `make serve` 稼働中なら保存で `air` が再ビルドするので、`api_server` のログが `[Fx] RUNNING`（"http server started"）に到達し、Fx の `provide` / `invoke` エラーが無いことを確認する。新規環境の注意: コンテナは **vendor モード**でビルドするため先に `make tidy-lib`（`vendor/` 生成）を実行する — 未生成だと Fx 実行前に `inconsistent vendoring` で失敗する。

## Step 7. クロージング

```text
<Package> usecase 層を生成しました。<N> ファイル作成 + DI 1 行追加、make go-test OK、coverage <X>%。
次は scaffold-controller で handler、または scaffold-endpoint で残層を続行できます。
```

commit しない。次の scaffold skill を起動しない。

## AI 修正スコープ

"Exception: Skill Execution" 緩和:

- 書き込みスコープ: `internal/usecase/<package>/`（新規ディレクトリ）+ `internal/di/module/usecase.go`（1 行追加）
- 既存パッケージディレクトリあれば中断

保護対象:

- domain / infra 層
- boundary interface ファイル（read-only）
- spec ファイル
- mock ファイル（`make gen-api` 経由のみ）

## 制約事項

- ❌ コードを言い換える／*なぜ*その設計にしたかを説明するコメントを足す — コードコメントは最小（振る舞い・契約のみ）。理由は commit message / README に置きコードに書かない。宣言の godoc（unexported 含む）は1行で残す。**分量も対象**: このスキルが生成する面は構造上すべて慣用的であり、コンストラクタ / Params 構造体 / 行→エンティティ変換 / handler テンプレートに複数行の説明を付けるのはノイズ。契約を1行で述べて止める。`docs/rules.md` にある repo 全体のルールを書き写さない。抑制であって根絶ではなく、真に非自明な Why は残す。
- ❌ spec に無いメソッド / DTO / dependency / workflow を発明
- ❌ business rule の実装（domain entity の責務）
- ❌ infrastructure への直接アクセス（Repository / Boundary interface 経由のみ）
- ❌ `time.Now()` 直接利用 — 時刻が必要なら `clock.Clock` boundary 経由
- ❌ test 観点 subagent (Step 2) をスキップ
- ❌ spec 確認 `AskUserQuestion` をスキップ
- ❌ 既存 usecase パッケージの上書き
- ❌ mock ファイルの手編集
- ❌ 失敗時の自動 rollback（TODO + FB）
- ✅ ユーザー向け出力 / テストケース名は日本語
- ✅ `internal/usecase/README.md` の Implementation Example を canonical 構造 template として利用、既存パッケージは二次参照のみ
- ✅ 書き込み前に全 `calls:` 参照の存在を検証
- ✅ DI 登録を同じ skill 実行内で更新

## チェックリスト

- [ ] spec パスを解決しファイル存在確認
- [ ] 前提条件確認（domain 参照、boundary 参照、対象ディレクトリ未存在）
- [ ] spec YAML をパース成功
- [ ] `internal/usecase/README.md`（Implementation Example 含む）+ `boundary/README.md` を canonical template として読み込み、sibling は二次参照のみ
- [ ] test 観点 subagent を起動、書き込み前に観点取得
- [ ] プラン表示 + `AskUserQuestion` 確認
- [ ] 実装ファイル書き込み（interface / DTOs / struct / constructor / methods）
- [ ] テストファイル書き込み（gomock セット + subagent 観点）
- [ ] `internal/di/module/usecase.go` に新 `fx.Provide` 追加
- [ ] `make gen-api` 実行、mock ファイル存在確認
- [ ] `make go-fix` + `make go-test` 実行、カバレッジ報告（または失敗時 TODO + FB）
- [ ] commit / push なし
- [ ] 最終サマリは日本語
