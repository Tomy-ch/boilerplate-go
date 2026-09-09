> このドキュメントは正典（英語版） [`docs/tutorial/build-user-feature.md`](build-user-feature.md) の日本語訳です。
> 内容の更新は正典側で行い、本ファイルへ反映してください。本ファイルを直接編集しないでください。

# チュートリアル: User 機能をゼロから作る

このチュートリアルは、完全なオニオンアーキテクチャの機能 —— サンプルの **User** API —— を、
まっさらな状態から構築する手順を一気通貫で辿る。これはリファレンス文書に対する *実践例
（worked example）* の対であり、[`architecture.md`](../architecture.ja.md) や
[`rules.md`](../rules.ja.md) が「ルール」を述べるのに対し、本書は 1 つの機能を依存順に全レイヤー
へ通し、**各ステップがなぜその順番で起きるのか** を説明する。

実チェックアウト上で再現できるよう意図的に設計してある。リポジトリには User 機能を丸ごと削除する
スクリプトが同梱されているため、ゼロへ戻して自分で再構築できる。

## 対象読者

- レイヤー別のリファレンス文書ではなく、コードベースを通す 1 本の連続した道筋がほしい新規参入者。
- scaffold スキルが **実際に何をしているのか** を知る必要がある人。本書はスキルが内部に符号化して
  いる正典であり、生成結果の妥当性を判断するときに見る場所である。
- `.claude/` の scaffold スキルを持たず、手作業で手順を実行する人。

> 標準経路はこれを打ち込むことではない。チュートリアル全体の自動版は `/scaffold-endpoint user` で
> ある（[次に進む先](#次に進む先) を参照）。生成されたものが正しいか判断できるよう、本書は一度
> 読んでおく。

## 何を作るか

フルスタックの User サンプル: 不変条件を持つ集約、リポジトリ + 読み取り最適化されたクエリサービス、
アプリケーションサービス、4 つの HTTP エンドポイント、CLI ジョブ、そして HTTP 境界の結合テスト ——
さらにそれらが生成元とする OpenAPI と SQL の **契約**。

## 憲法: lean A

本リポジトリは **lean A** の spec 方針に従う。spec 駆動になるのは 2 レイヤーのみ:

|レイヤー|真実の源|
|---|---|
|domain|`docs/spec/domain/<パッケージパス>.md`|
|usecase|`docs/spec/usecase/<パッケージパス>.md`|
|infrastructure|domain の Repository インターフェース + sqlc gen から **導出**（spec ファイルなし）|
|controller|OpenAPI 生成の `ServerInterface` + usecase インターフェースから **導出**（spec ファイルなし）|

つまり *内側* の 2 レイヤーには spec を書き、*外側* の 2 レイヤーは生成された契約から従わせる。これを
覚えておくこと —— Step 1 が spec ファイルを 2 つしか作らない理由がこれである。

## 依存順

依存は常に内側を向くが、ジェネレーターに消費対象を与えるため **契約を先に** 作る:

```text
spec (domain + usecase)
   └─ contracts:  OpenAPI  +  DB migration  +  DML SQL
        └─ generate:  make gen-api   make gen-query
             └─ domain  →  infrastructure  →  usecase  →  controller
                  └─ DI 配線  →  結合テスト  →  検証
```

以下の各ステップは、その **目的**、**触るファイル**、**それを正しくする規約**（「なぜ」）、そして
**それを証明するコマンド** を示す。

---

## 前提条件

- ツールチェーンが用意済み（`mise`）で Docker が使えること —— コードジェネレーターと DB はコンテナで
  動く。ルートの [`README.md`](../../README.ja.md) の Quick Start を参照。
- DB コンテナ（`database`）が起動していること。`make gen-query` は **ライブ** スキーマをダンプし、
  infra/結合テストは実 DB を必要とする。
- 参照実装を `git restore` 一発で戻せるよう、**作業用ブランチ** で行うこと:

```bash
git switch -c tutorial/build-user
```

---

## Step 0 —— ゼロへリセット

リポジトリは、サンプルを構成する全ファイルを
[`scripts/setup/remove-sample-api/sample-manifest.ts`](../../scripts/setup/remove-sample-api/sample-manifest.ts) に宣言している。
1 つのコマンドがそれらを削除し、共有 DI モジュールと `openapi.yaml` から `sample-api` マーカーブロックを
取り除いたうえで、小さくなったツリーを再生成・検証する:

```bash
# まずプレビュー —— 削除されるパスを列挙し、何も書き込まない。
DRY_RUN=1 make setup-remove-sample-api

# 実際にリセット。完了後に gen-api → gen-query → fix → lint を実行する。
make setup-remove-sample-api
```

**何を削除するか**（このマニフェストが本チュートリアル残り全体の目次になる）:
`internal/domain/user`、`internal/usecase/user`、`repository/user` 配下の infra パッケージ、`internal/controller/handler/v1/users`、`internal/controller/job/usercount`、
3 つの `internal/integration/v1_users*_test.go`、User の OpenAPI paths/components、User の DML +
migration + seed、そして `docs/spec/{domain,usecase}/user.md`。

> **注意:** このスクリプトは `product` / `order` の DB スタブも削除する（同じ「サンプル」マニフェスト
> を共有しているため）。User のみの再構築ではこれで問題ない —— それらは未使用のマイグレーションである
> —— が、リセットは「全サンプル」であって「user のみ」ではない点に注意。

`gen-query` はライブスキーマをダンプするため、削除済みの `users` テーブルが生成モデルに残らないよう、
開発用とテスト用の DB から落としてから再生成する:

```bash
make db-init-local db-init-test
make gen-query
```

この時点でツリーは User 機能 **なし** でコンパイル・lint が通る。これがゼロ状態である。以下はすべて
これを再構築する。

---

## Step 1 —— spec（内側の契約）

**目的:** lean A の 2 つの spec ファイルに、ドメイン集約とアプリケーションサービスを宣言する。これらは
内側のレイヤーが実装する、人間が書いた意図である。

**ファイル:**

- `docs/spec/domain/user.md` —— Overview / Entity / Cross-field Invariants / Behavior
  Methods / Value Objects / Repository Methods。
- `docs/spec/usecase/user.md` —— Overview / Interface / DTOs / Dependencies / Workflow。
- `docs/spec/usecase/user/search.md` —— 読み取り専用のキーワード検索サービス（クエリ側の別ユース
  ケースなので独自の spec を持つ）。

**なぜ最初か:** lean A では domain と usecase の実装はこれらの spec に対して検証され（`/verify-spec`）、
spec の Entity フィールド表は SQL マイグレーションに対して照合されるソフト契約である。これらを最初に
書くことで、以降の全ステップに目標が与えられる。例えば Entity セクションは、`prefectureID` を ID 参照
としてのみ保持することを固定する。`User` 集約は自前のクレデンシャルを一切持たない: 認証は外部の OIDC
provider に委譲され、トークンの `(issuer, subject)` から内部ユーザーへの対応は別テーブル
`user_identities` が所有する（[`docs/design/auth.md`](../design/auth.ja.md) を参照）。

**検証:** まだなし（Markdown）。スキルがあれば `/verify-spec user` が形式 + 相互参照を確認する。

---

## Step 2 —— 契約（OpenAPI + データベース）

**目的:** ジェネレーターが Go へ変換する外側の契約を定義する。これらについて `internal/` 内に手書きする
ものは何もない —— YAML と SQL を書く。

### 2a. OpenAPI

**ファイル:**

- `openapi/paths/v1/users.yaml`（一覧 + 作成）、`openapi/paths/v1/users/userId.yaml`
  （取得/更新/部分更新/削除）、`openapi/paths/v1/users/me.yaml`（認証済み自己取得）、
  `openapi/paths/v1/users/search.yaml`、`openapi/paths/v1/users/feed.yaml`。
- `openapi/components/schemas/{UserBaseInputRequest,UserResponse}.yaml`、加えて users と search 向けの
  `requests/`・`responses/`・`parameters/` 断片。
- `openapi/openapi.yaml` —— 上記を `$ref` するルート文書。ここのサンプルエントリは `paths`・
  `components.parameters`・`components.schemas` の下で `# sample-api:begin … # sample-api:end`
  マーカーブロックに収められている。

**なぜ:** **OpenAPI-first は譲れない**（`rules.md`）。ハンドラーのインターフェース、リクエスト、
レスポンス型はここから *生成* される。契約のないエンドポイントにハンドラーを手書きしてはならない。
各 `operationId`（`GetUsers`、`PostUsers`、`GetUsersSearch` …）は生成される `ServerInterface` の
1 メソッドになる。

### 2b. データベース

**ファイル:**

- `database/migrations/000004_create_users.up.sql` / `.down.sql` —— `users` テーブル
  （`id` UUID PK、`email` UNIQUE、`prefecture_id` FK、住所カラム、論理削除用の `created_at` /
  `updated_at` / `deleted_at`）。
- `database/migrations/000017_add_users_table_search_text_column.up.sql` / `.down.sql` ——
  `GENERATED ALWAYS` の `search_text` カラム + キーワード検索用の GIN トライグラム索引。
- `database/dml/repository/user/*.sql` —— 集約の CRUD クエリ
  （`insert_user`、`select_user_by_id`、`select_users`、`update_user`、`count_user`）と
  キーワード検索（`select_users_by_keyword`、`count_users_by_keyword`）。User は QueryService を
  分離せず Repository に両方を持たせており、読み取り側も射影ではなくエンティティを返す。
- `database/seed/000001_users.sql` —— サンプル行（論理削除済みの 1 件を含む）。

**なぜ:** **マイグレーションは追記専用** —— 既存のマイグレーションを編集してはならない。新しい連番の
`NNNNNN_name.up.sql` / `.down.sql` のペアを追加する。DML ファイルは sqlc の入力であり、責務で分割
される（repository = 集約の永続化、query_service = 読み取り/投影）。この分割はスタックの上まで一貫して
反映される。

**検証:** マイグレーションを開発/テスト DB へ適用する:

```bash
make db-local-migrate-up
make db-test-migrate-up
```

---

## Step 3 —— 生成

**目的:** 契約を Go へ変換する。これが「自分が書くもの」と「決して手で編集してはならないもの」の境界で
ある。

```bash
make gen-api     # OpenAPI をバンドル → openapi.gen.yaml、oapi-codegen + mockgen を実行
make gen-query   # スキーマをダンプ → DML をマージ → sqlc generate → fmt
```

**何が現れるか（生成物 —— 編集禁止）:**

- `internal/controller/handler/v1/users/gen/server.gen.go` + `type.gen.go`（および `detail/gen`・
  `search/gen` 下の同種）—— `ServerInterface` とリクエスト/レスポンス型。
- `internal/infrastructure/rdb/sqlc/gen/user_repository.gen.sql.go` —— 型安全なクエリメソッド。
- `//go:generate mockgen` ディレクティブを持つインターフェースの `*_mock.go`（次のステップで宣言する。
  宣言後に `make gen-api` を再実行する）。

**なぜ:** `**/*.gen.go`・`*.sql.go`・`*_mock.go`・`openapi.gen.yaml` は生成物であり **保護対象** である。
挙動は *契約* を変えて再生成することで変える。出力を編集することでは変えない。

---

## Step 4 —— domain レイヤー

**目的:** `docs/spec/domain/user.md` から集約を実装する: エンティティ、不変条件、振る舞いメソッド、値オブジェクト、
sentinel エラー、定数、そして Repository インターフェース。

**ファイル（`internal/domain/user/` 内）:**

- `user_domain.go` —— `User` 構造体 + `New(...)` コンストラクタ + getter + 振る舞いメソッド。
- `constant.go` —— spec のフィールド制約から導いた `min*/max*` の長さ境界。
- `error.go` —— `ErrInvalid<Field>` sentinel + `ErrAlreadyDeleted` など。
- `email.go` / `postal_code.go` —— 値オブジェクト（`Email` / `PostalCode`）。生の文字列をファクトリ
  （`NewEmail` / `NewPostalCode`）で検証し `Value()` で公開するため、不正な形式は決して構築できない。
- `user_repository.go` —— Repository インターフェース。`//go:generate mockgen` ディレクティブを持つ
  （→ `mock/`）。
- `*_test.go` —— 不変条件、振る舞いメソッド、VO 境界。

**それを正しくする規約**（`internal/domain/README.md`）:

- **フィールドは非公開、getter で公開する。** 外部コードが不変条件を迂回できない。
- **ポインタの getter/setter は `ptr.Copy` でコピーする** ので、内部状態が参照経由で漏れない。
- **検証失敗は名前付き sentinel をラップする**: `xerrors.Wrap(ErrInvalidEmail, msg)`。
- **レイヤーは純粋:** `time.Now()` なし、`uuid.New()` なし、I/O なし、domain ロジックに context なし。
  時刻と ID はコンストラクタ引数として届く。

コンストラクタは全集約が従う形である —— 検証してから構築する:

```go
// internal/domain/user/user_domain.go （抜粋 —— 本体はファイルを参照）
func New(id uuid.UUID, firstName /* … */ string, /* … */ ) (*User, error) {
 if id.IsNil() {
  return nil, xerrors.Wrap(ErrInvalidID, "id is required")
 }
 if err := validateProfileFields(firstName /* … */); err != nil {
  return nil, err
 }
 // … updatedAt/deletedAt の順序チェック …
 return &User{id: id, firstName: firstName /* … */, building: ptr.Copy(building)}, nil
}
```

変更は不変条件を再チェックするメソッドである（例: `UpdateProfile`、`MarkAsDeleted` —— いずれも
最初に `ensureNotDeleted` を呼ぶ）。実ファイルを読むこと: それが将来のあらゆる集約の正典テンプレート
である。

**検証:**

```bash
make gen-api                        # Repository モックを再生成
go test ./internal/domain/user/...
```

---

## Step 5 —— infrastructure レイヤー

**目的:** sqlc 生成関数をラップして、domain の Repository インターフェース（および usecase の
QueryService インターフェース）を実装する。**spec ファイルなし** —— このレイヤーはインターフェース +
sqlc gen から導出される。

**ファイル:**

- `internal/infrastructure/rdb/repository/user/user_repository.go` —— domain の `user.Repository`
  を実装（Create / FindByID / Update / アクティブ一覧 / 件数）。
- `*_test.go` —— トランザクションロールバックを伴う **実 DB** に対する結合テスト（rdb の `testkit`
  経由）。

**それを正しくする規約**（`internal/infrastructure/rdb/README.md`、`pgerror/README.md`）:

- **すべての sqlc の返り値は `pgerror.NormalizeError` で正規化する** ので、PostgreSQL の SQLSTATE は
  `apperror` 値になる（`pgx.ErrNoRows → ErrNotFound`、unique 違反 → `ErrConflict`）。外側のレイヤーは
  ドライバー固有のエラーを決して見ない。
- **各メソッドは tracer span を開く**（`r.tracer.Start(ctx)` / `defer endSpan()`）。
- **sqlc 型は決して漏らさない。** 返す前に行をドメインエンティティ（repository）または DTO
  （query service）へ変換する。query service は domain を飛ばして DTO へ直接投影してよい。

repository メソッドの形 —— span、sqlc 呼び出し、正規化、変換:

```go
// 形のみ —— 実本体は user_repository.go を参照
func (r *repository) Create(ctx context.Context, u *user.User) error {
 ctx, endSpan := r.tracer.Start(ctx)
 defer endSpan()
 db := gen.New(driver.New(ctx, r.db))
 if err := db.CreateUser(ctx, toCreateParams(u)); err != nil {
  return pgerror.NormalizeError(err)
 }
 return nil
}
```

実装は `internal/di/module/infrastructure.go` の `fx.Provide` で登録する（`sample-api` マーカー
ブロック —— Step 8）。

**検証:** テスト DB のマイグレーションが必要（Step 2b）:

```bash
go test ./internal/infrastructure/rdb/repository/user/...
```

---

## Step 6 —— usecase レイヤー

**目的:** `docs/spec/usecase/user.md` からアプリケーションサービスを実装する: domain + repository + boundary を
オーケストレーションし、DTO を返す。ここでは業務 *ルール* を一切発明しない —— このレイヤーは調整役で
ある。

**ファイル（`internal/usecase/user/` 内）:**

- `user_usecase.go` —— `Usecase` インターフェース + `usecase` 構造体 + `New` コンストラクタ。
- `search/user_search_usecase.go` + `search/query/…` —— キーワード検索サービスとその QueryService
  インターフェース。
- `mock/…gen.go` —— usecase インターフェースの生成モック（controller/結合テスト用）。
- `*_test.go` —— テーブル駆動、domain リポジトリはモック。

**それを正しくする規約**（`internal/usecase/README.md`、`boundary/README.md`）:

- **DTO を返す。ドメインエンティティは決して返さない。** `*user.User` → `UserView` へマップしてから
  返す。
- **時刻とトランザクションは boundary 経由で来る**。標準ライブラリではない: 現在時刻には
  `u.clock.Now()`（`time.Now()` ではない）、トランザクション境界には `u.txm.Do(ctx, fn)`。非決定的
  または外部の依存はすべて `boundary/` インターフェースとして届く。決定性とテスト容易性はこれに依存する。
- **usecase がトランザクション境界を所有する**。domain は `tx` を何も知らない。
- **オーケストレーションせよ、ルールを再実装するな。** ドメインの振る舞いメソッドを呼ぶのはよい。
  新しい不変条件をここに符号化するのはダメ —— それは domain に属する。

書き込みユースケースの形 —— clock boundary から時刻を取得し、トランザクション内で実行する:

```go
// 形のみ —— 実本体は user_usecase.go を参照
func (u *usecase) CreateUser(ctx context.Context, dto *CreateParamsDTO) (UserView, error) {
 ctx, endSpan := u.tracer.Start(ctx); defer endSpan()
 now := u.clock.Now()                                   // time.Now() ではなく boundary
 // … u.txm.Do(ctx, func(ctx) { user.New(...now...); u.repo.Create(...) }) …
 return toUserView(entity /* … */), nil                 // DTO を返す
}
```

**検証:**

```bash
make gen-api                          # usecase モックを再生成
go test ./internal/usecase/user/...
```

---

## Step 7 —— controller レイヤー

**目的:** OpenAPI 生成の `ServerInterface` を実装する —— `operationId` ごとに 1 ハンドラーメソッド ——
加えて CLI ジョブ。**spec ファイルなし** —— 生成インターフェース + usecase インターフェースから導出。

**ファイル:**

- `internal/controller/handler/v1/users/v1_users_handler.go`（+ URL 構造をミラーする `detail/`・
  `search/` サブパッケージ）—— `server` 構造体、`BindHandler` コンストラクタ、ハンドラーメソッド、
  そして presenter 関数（`toUserResponse` …）。
- `internal/controller/job/usercount/user_count_job.go` —— CLI バッチジョブ（HTTP ではない）:
  フラグを解析し、usecase を呼び、ログ出力する。`os.Exit` なし（プロセス終了は runner が所有）。
- `*_test.go` —— usecase はモック。

**それを正しくする規約**（`internal/controller/handler/README.md`）:

- **`BindHandler(echo, tracerFactory, usecase)`** が `server` を構築し、`gen.NewStrictHandler` +
  `gen.RegisterHandlers` で登録する。これはハンドラー README にある正典の参照スニペットである。
- **`operationId` ごとに 1 メソッド、名前で 1:1 一致** —— 生成された `ServerInterface` と。
- **ハンドラー本体は純粋なテンプレート:** span 開始 → リクエスト解析 → **1 つの** usecase メソッド呼び
  出し → DTO を OpenAPI レスポンス型へ変換 → 返す。業務ロジックなし、infra アクセスなし、手動ステータス
  コードなし（apperror → HTTP ステータスは自動）。
- **HTTP の語彙は controller に留める。** OpenAPI 型を `internal/controller/conv` でドメイン型へ変換する
  （例: `conv.UUID`）ので、`http.*` / `openapi_types.*` が usecase に到達しない。

```go
// 形のみ —— 実本体は v1_users_handler.go を参照
func (s *server) GetUsers(ctx context.Context, req gen.GetUsersRequestObject) (gen.GetUsersResponseObject, error) {
 ctx, endSpan := s.tracer.Start(ctx); defer endSpan()
 page, err := paging.NewPageFrom1Based(req.Params.Page, req.Params.PerPage)
 // … list, err := s.uc.<ListMethod>(ctx, …) …
 return gen.GetUsers200JSONResponse(/* マップした DTO */), nil
}
```

**検証:**

```bash
go test ./internal/controller/handler/v1/users/...
```

---

## Step 8 —— 依存注入の配線

**目的:** 新しいプロバイダー/ハンドラーを Fx モジュールに登録する。これらはリセットスクリプトが
取り除く共有の `MARKER_FILES` である —— よって `sample-api` マーカーブロック内に配線を再追加する
ことで、機能を自己完結かつ削除可能に保つ。

**ファイル（各編集は `// sample-api:begin … // sample-api:end` 内に置く）:**

- `internal/di/module/controller.go` —— `fx.Invoke(users.BindHandler, detail.BindHandler,
  search.BindHandler)`。
- `internal/di/module/usecase.go` —— usecase コンストラクタを `fx.Provide`。
- `internal/di/module/infrastructure.go` —— repository + query service を `fx.Provide`。
- `internal/di/module/job.go` —— `usercount` ジョブを登録。

```go
// internal/di/module/controller.go
fx.Invoke(
 health.BindHandler,
 // … コアのハンドラー …
 // sample-api:begin
 users.BindHandler,
 detail.BindHandler,
 search.BindHandler,
 // sample-api:end
),
```

**なぜ:** DI はこれらのレイヤーが出会う唯一の場所であり、**ここに業務ロジックはない**。マーカー
コメントこそが `make setup-remove-sample-api` にサンプルをきれいに再切除させるものである。

---

## Step 9 —— 結合テスト

**目的:** usecase をモックして HTTP 境界をエンドツーエンドで検証する —— Router → Middleware →
Handler → Presenter。

**ファイル:** `internal/integration/v1_users_test.go`、
`v1_users_detail_test.go`、`v1_users_search_test.go`。

**Step 5 のテストとどう違うか:** これらは `httptest` 経由で実際の Echo サーバーを起動するが、
**usecase はモックする**（`internal/integration/README.md`）。DB テストではない —— HTTP 配線
（ステータスコード、JSON 形状、path/param 解析、認証ヘッダー処理）を証明するものであり、ハンドラーの
モック単体テストでは完全にはカバーできない部分である。`operationId` ごとに 1 サブテスト。

**検証:**

```bash
go test ./internal/integration/...
```

---

## Step 10 —— イベント駆動側（任意）

**目的:** usecase 層へのもう 1 つの入口を見る。ここまではすべて HTTP リクエストが駆動していたが、
退会はドメインイベントも emit し、それを **worker** が消費する。

Step 1〜9 で作ったものは何も変わらない。退会の usecase は既に `user.withdrawn.v1` を削除と**同一
トランザクション**で outbox へ書いているため、イベントが失われることも、ロールバックした退会に
対して emit されることもない。そこから先は次のとおり。

|#|要素|ファイル|
|---|---|---|
|①|業務 `Handler` — 復元・分類・usecase 呼び出し|`internal/controller/worker/withdrawalarchive/withdrawal_archive_handler.go`|
|②|`Consumer` / `FailureHandler` adapter|`internal/infrastructure/queue/sqs`（生成は DI 側。ここではない）|
|③|`Worker` — 名前 / consumer / handler / failure handler を束ねる|`internal/controller/worker/withdrawalarchive/withdrawal_archive_worker.go`|
|④|登録|`internal/di/module/worker.go` の `sample-api` マーカー内|
|⑤|broker クライアント + adapter 設定|`internal/di/module/withdrawalarchive.go`|
|⑥|env|`env/.env` の `CONSUMER_QUEUE_*`|

**adapter を worker パッケージではなく DI で組み立てる理由:** controller 層は infrastructure を
import できない（depguard が強制する）。worker は組み立て済みの `Consumer` を受け取るだけで、
どの broker から読んでいるかを知らない。HTTP handler を repository から遠ざけているのと同じ規則。

**コードを開いて確かめる価値があるのは 2 点:**

- **冪等性。** 配信は at-least-once なので、この Handler は 1 回の退会に対して 2 回走ることがある。
  再実行を検出するのではなく、*操作*を冪等にしている。オブジェクトキーはユーザー ID だけから決まり、
  本文は payload を加工しないため、2 回目の実行は同一のバイト列を書く。冪等キーの伝搬より強い —
  伝搬は下流がそれを尊重して初めて効く。
- **選別。** 1 つのキューに outbox が emit する全種別が流れるため、Handler はまず `event_type` 属性を
  確認し、それ以外は処理せず ack する。他人のイベントを失敗として扱うと、購入イベントが軒並み
  DLQ へ送られる。

**検証** —— 3 つの端末（`make serve`、`make outbox-relay`、`make worker NAME=withdrawal-archive`）を
立ててからユーザーを退会させる。各段で何を見るかを含む完全な手順は
[`internal/controller/worker/withdrawalarchive/README.ja.md`](../../internal/controller/worker/withdrawalarchive/README.ja.md)
にある。

---

## Step 11 —— フル検証

**目的:** 再構築した機能が正しく、整形済みで、カバレッジゲートを満たすことを証明する。

```bash
make go-fix    # gofmt + golangci-lint --fix
make go-lint   # golangci-lint（フル設定）
make go-test   # 全テスト、キャッシュなし。テスト DB のマイグレーションが必要
```

**カバレッジ:** `make go-test` はカバレッジを下げてはならない。新規/変更したパッケージは **90%** を超える
こと（`make cover-gate` が CI でフロアを強制する）。生成パッケージ（`gen`、`cmd`、`mock`、`apperror`、
`scripts`）は計算から除外される。

パッケージが 90% に届かない場合は、先へ進む前に不足している分岐テストを追加すること —— これは Definition
of Done の一部であり、任意の仕上げではない。

---

## まとめ

|Step|レイヤー|主なファイル|一言での「なぜ」|検証|
|---|---|---|---|---|
|0|reset|`make setup-remove-sample-api`|マニフェストが機能の目次|User なしでツリーがコンパイル|
|1|spec|`docs/spec/{domain,usecase}/user.md`|lean A: 内側の 2 レイヤーのみ spec 駆動|`/verify-spec user`|
|2|contracts|`openapi/**`、`database/migrations/**`、`database/dml/**`|OpenAPI-first + 追記専用マイグレーション|`make db-*-migrate-up`|
|3|generate|`make gen-api` / `make gen-query`|契約を変え、生成物は変えない|`gen/` 下にファイルが現れる|
|4|domain|`internal/domain/user/**`|非公開フィールド + `ptr.Copy` + sentinel エラー + 純粋性|`go test ./internal/domain/user/...`|
|5|infra|`internal/infrastructure/rdb/repository/user/**`|`pgerror.NormalizeError` + tracer span + 型漏洩なし|`go test ./…/user/...`（DB）|
|6|usecase|`internal/usecase/user/**`|DTO を返す。時刻/tx は boundary 経由。調整のみ|`go test ./internal/usecase/user/...`|
|7|controller|`internal/controller/handler/v1/users/**`、`job/usercount/**`|operationId ごとに 1 メソッド。ハンドラーは純粋テンプレート|`go test ./…/users/...`|
|8|DI|`internal/di/module/*.go`|レイヤーが出会う唯一の場所。マーカーブロックで削除可能に保つ|`make go-lint`|
|9|integration|`internal/integration/v1_users*_test.go`|usecase をモックした HTTP 境界|`go test ./internal/integration/...`|
|9.5|worker|`internal/controller/worker/withdrawalarchive/**`|usecase 層へのもう 1 つの入口。操作自体が冪等|`make worker NAME=withdrawal-archive`|
|10|verify|`make go-fix` / `make go-lint` / `make go-test`|90% フロアは Done の一部|`make cover-gate`|

---

## 次に進む先

- **自動化する。** `.claude/` のスキルがあれば、このフロー全体は `/scaffold-endpoint user` である
  （`verify-spec` → `scaffold-domain` → `scaffold-infra-db` → `scaffold-usecase` →
  `scaffold-controller` → `scaffold-integration-test` を連鎖する）。本チュートリアルはそれらのスキルが
  符号化している手作業の正典である —— 一度読んだら、あとはスキルにタイピングを任せればよい。
- **ドリフトを確認する。** 複数レイヤーの変更後は、`/back-prop` と `/arch-check` が README とコードが
  まだ一致していることを確認する。
- **2 つめの機能を追加する。** `product` と `order` は DB のみのスタブとして既に同梱されている
  （`sample-manifest.ts` マニフェスト参照）。どちらかをフルスタックへ昇格させるのが自然な次の練習である。

## メンテナンスノート

本チュートリアルは User の実装を転記せず **実ファイル** を参照しているため、正典ファイルが単一の真実の
源であり続ける。レイヤーファイルをリネームしたり規約を変えたりすると、ドリフトはここにも表面化する ——
`/back-prop` を実行して捉えること。
