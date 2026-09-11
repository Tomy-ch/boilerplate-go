# Product — Usecase Spec

> 商品一覧（`GET /v1/products`）と一致件数（`GET /v1/products/count`）は、単一集約・products 自身の列への検索であり、
> QueryService ではなく domain `product.Repository` の `FindPublishedList` / `CountPublished`（未公開込みは `FindAllList` / `CountAll`）に委譲する（ADR-0032 (lightweight-cqrs) / `docs/rules.md` の Repository 境界に準拠）。

## Overview

商品一覧ユースケースは、商品を cursor ページネーションで取得する read-only な thin orchestrator。商品一致件数ユースケースは同じ検索入力の検証・正規化と可視範囲の判定を共有し、ページングや並び順を持たず一致件数のみを返す。

**可視範囲は既定で公開済みのみ**で、`IncludeUnpublished` を指定したときだけ未公開を含む。指定は admin だけが通り、未認証は 401、admin でなければ 403 を返す。判定は `authorizeUnpublishedRead` の 1 箇所に集約し、一覧・一致件数・単体取得が同じ能力（`authz.ActionProductReadUnpublished`）を共有する。可視範囲によって委譲先が変わり、公開済みのみは `FindPublishedList` / `CountPublished` / `FindPublishedByID`、未公開込みは `FindAllList` / `CountAll` / `FindByID` を呼ぶ。

cursor ページングは「直前ページ末尾行のソートキーを不透明トークン化して次ページを取得する」keyset 方式。**ソートキーの軸は可視範囲によって変わる。** 公開済みのみは `(publishedAt, id)`、未公開込みは `(createdAt, id)` で、未公開の商品が公開日時を持たない以上、公開日時を並び順の第 1 キーにできないためである。usecase は符号化・復号（`encodeProductCursor` / `decodeProductCursor`、`encodeAllProductCursor` / `decodeAllProductCursor`）を担い、domain へは境界を primitive（`AfterPublishedAt` / `AfterCreatedAt` と `AfterID`）で渡す。次ページ有無は `limit + 1` 件取得して超過分で判定し（`hasNext`）、超過時のみ末尾行から `NextCursor` を生成する。

**カーソルは発行時の可視範囲に紐づく。** 未公開込みモードのカーソルは先頭に識別子を持つ 3 キーで、公開済みのみの 2 キーとはキー数が異なる。これにより、一方で得たカーソルを他方へ持ち越すと必ず `apperror.ErrInvalidArgument` になり、公開日時と登録日時を取り違えたまま解釈されることがない。

sort は `-publishedAt`（既定=降順）/ `publishedAt`（昇順）の 2 値のみ。controller が enum を `Ascending` bool に写像して渡す。未公開込みモードでは軸が登録日時になるため、sort は向きだけを適用する。ドメイン集約を outer 層へ露出させないため、`Product` は usecase 内で `ProductView` へ写像してから返す（DTO Boundary）。

商品作成（`POST /v1/products`）は admin 認可のうえ、`tx.Manager` の境界内で商品ステータス / カテゴリの名称を ID から解決し、`Product` を構築して登録する write ユースケース。マスタ不在はサーバ側整合性異常（500）、価格・在庫などの業務不変条件違反は 422 に落とす。

在庫の増減（`PATCH /v1/products/{productId}/stock`）は admin 認可のうえ、`tx.Manager` の境界内で対象商品を悲観ロックしてから在庫を増減する write ユースケース。購入による在庫減算と同じ行を対象とするため、取得〜更新が直列化され、並行する増減は失われず合成される（ADR-0036 (ordered-pessimistic-row-locks)）。待てば解消しうる競合は一時障害（503）、取得後の変化を検出した恒久的な衝突は 409 として露出する。

在庫僅少一覧（`GET /v1/products/low-stock`）は admin 認可のうえ、在庫が在庫警告閾値以下まで減った商品を在庫の少ない順に上位 `limit` 件返す read-only な thin orchestrator。判定条件と並び順は `product.Repository` の `FindAllLowStock` が持ち、usecase は取得件数の正規化（既定値 20 / 範囲 1〜100 へのクランプ）と DTO への写像のみを担う。cursor ページングは持たない。

商品部分更新（`PATCH /v1/products/{productId}`）は admin 認可のうえ、`tx.Manager` の境界内で read-modify-write を行う write ユースケース。読み込み時点のバージョンを条件に更新することで、並行編集による上書き（lost update）を防ぐ。送られたフィールドのみを反映し、未送信は現在値を据え置き、null 明示はクリアする 3 状態の解決は usecase が担う（domain へは解決後の確定値のみを渡す）。参照の再解決は `statusId` / `categoryId` が指定された場合に限る。

## Interface

```yaml
package: internal/usecase/product
name: Usecase
methods:
  - name: ListProducts
    signature: ListProducts(ctx context.Context, authn *auth.Authn, params ListProductsParams) (ProductListView, error)
  - name: CountProducts
    signature: CountProducts(ctx context.Context, authn *auth.Authn, params CountProductsParams) (ProductCountView, error)
  - name: GetProduct
    signature: GetProduct(ctx context.Context, authn *auth.Authn, params GetProductParams) (ProductView, error)
  - name: CreateProduct
    signature: CreateProduct(ctx context.Context, authn *auth.Authn, params CreateProductParams) (ProductView, error)
  - name: UpdateProduct
    signature: UpdateProduct(ctx context.Context, authn *auth.Authn, id uuid.UUID, params UpdateProductParams) (ProductView, error)
  - name: UpdateProductStock
    signature: UpdateProductStock(ctx context.Context, authn *auth.Authn, id uuid.UUID, params UpdateProductStockParams) (ProductView, error)
  - name: ListLowStockProducts
    signature: ListLowStockProducts(ctx context.Context, authn *auth.Authn, params ListLowStockProductsParams) (ProductLowStockListView, error)
```

未参照画像の回収はジョブ専用の入口で、HTTP ハンドラが使う `Usecase` とは利用者も依存も異なるため、
独立したインターフェースに分ける（`outbox.GCUsecase` / `user.PurgeUsecase` と同じ形）。

```yaml
package: internal/usecase/product
name: ImageGCUsecase
methods:
  - name: SweepOrphans
    signature: SweepOrphans(ctx context.Context, grace time.Duration, batchSize int32, dryRun bool) (ImageGCResult, error)
```

## DTOs

```yaml
- name: ListProductsParams
  description: 商品一覧取得の入力。cursor（取得件数 + 境界）・並び順・可視範囲を持ち、検索条件は SearchFilter を埋め込む。
  fields:
    - name: SearchFilter
      type: SearchFilter    # 埋め込み。一致件数と共有する検索条件
    - name: Cursor
      type: "*paging.Cursor"
    - name: Ascending
      type: bool
    - name: IncludeUnpublished
      type: bool            # true は admin のみ。並び順の軸も (createdAt, id) へ変わる
- name: CountProductsParams
  description: 商品一致件数取得の入力。一覧と母集団を揃えるため、検索条件と可視範囲を一覧と同じ形で受ける。
  fields:
    - name: SearchFilter
      type: SearchFilter    # 埋め込み
    - name: IncludeUnpublished
      type: bool
- name: GetProductParams
  description: 単一商品取得の入力。
  fields:
    - name: ID
      type: uuid.UUID
    - name: IncludeUnpublished
      type: bool
- name: SearchFilter
  description: 一覧と一致件数が共有する検索条件。cursor・並び順・可視範囲は持たない（可視範囲は取得メソッドの選択で表す）。
  fields:
    - name: CategoryID
      type: "*uuid.UUID"
    - name: StatusID
      type: "*uuid.UUID"
    - name: Keyword
      type: "*string"
    - name: MinPrice
      type: "*string"
    - name: MaxPrice
      type: "*string"
    - name: MinQuantity
      type: "*int32"
    - name: MaxQuantity
      type: "*int32"
- name: ProductCountView
  description: 公開商品検索の一致件数。
  fields:
    - name: Count
      type: int64
- name: ProductView
  description: 商品 1 件分の usecase 出力 DTO。domain エンティティ Product から写像する。Price は価格スケールの十進量（pkg/decimal.Decimal）で、controller が decimal 文字列へ整形する。
  fields:
    - name: ID
      type: uuid.UUID
    - name: Name
      type: string
    - name: Description
      type: "*string"
    - name: Price
      type: decimal.Decimal
    - name: Quantity
      type: int
    - name: StockWarningThreshold
      type: "*int"
    - name: StatusID
      type: uuid.UUID
    - name: CategoryID
      type: uuid.UUID
    - name: PublishedAt
      type: "*time.Time"     # 未公開は nil
    - name: Images
      type: "[]ProductImageItemView"  # 表示順の昇順。画像未設定は空
    - name: Version
      type: int              # 楽観ロックのバージョン。部分更新の要求へそのまま渡す
- name: CreateProductParams
  description: 商品作成の入力。price は十進文字列で受け取り usecase で decimal へ解釈する（負値は 422）。publishedAt は nil 許容、images は空許容。
  fields:
    - name: Name
      type: string
    - name: Description
      type: "*string"
    - name: Price
      type: string
    - name: Quantity
      type: int
    - name: StockWarningThreshold
      type: "*int"
    - name: CategoryID
      type: uuid.UUID
    - name: StatusID
      type: uuid.UUID
    - name: PublishedAt
      type: "*time.Time"
    - name: Images
      type: "[]ProductImageParams"    # 画像の ID は usecase が採番する
- name: ProductImageParams
  description: 商品画像 1 件分の入力。
  fields:
    - name: ImagePath
      type: string                    # 画像アップロードで得たオブジェクトのパス
    - name: DisplaySort
      type: int                       # 同一商品内での表示順。重複は集約が 422 で拒否する
- name: ProductImageItemView
  description: 商品画像 1 件分の出力 DTO。
  fields:
    - name: Path
      type: string
    - name: DisplaySort
      type: int
- name: UpdateProductParams
  description: |
    商品部分更新の入力。nil のポインタは未指定（現在値を据え置く）を表す。
    クリアも許容するフィールドは pkg/patch.Field で 3 状態（未指定 / null 明示 / 値指定）を表す。
  fields:
    - name: Version
      type: int                       # 更新対象を読み込んだ時点のバージョン。不一致は 409
    - name: Name
      type: "*string"
    - name: Price
      type: "*string"
    - name: Quantity
      type: "*int"
    - name: CategoryID
      type: "*uuid.UUID"
    - name: StatusID
      type: "*uuid.UUID"
    - name: Description
      type: "patch.Field[string]"     # null 明示でクリア
    - name: StockWarningThreshold
      type: "patch.Field[int]"        # null 明示でクリア
    - name: PublishedAt
      type: "patch.Field[time.Time]"  # null 明示でクリア（未公開へ戻す）
    - name: Images
      type: "patch.Field[[]ProductImageParams]"  # 指定で集合ごと置換、null 明示で全て取り除く
- name: UpdateProductStockParams
  description: 在庫の増減の入力。
  fields:
    - name: Delta
      type: int                       # 在庫の増減量。正で補充、負で差し引き。増減後が負になる要求は 422
- name: ProductListView
  description: 公開商品一覧（cursor ページネーション）の取得結果。
  fields:
    - name: Items
      type: "[]ProductView"
    - name: NextCursor
      type: "*string"        # 最終ページの場合は nil
- name: ListLowStockProductsParams
  description: 在庫僅少商品一覧取得の入力。cursor を持たない top-N のため取得件数のみを受け取る。
  fields:
    - name: Limit
      type: int              # 下限（1）未満は既定値 20、上限（100）超過は 100 へクランプ
- name: ProductLowStockListView
  description: 在庫僅少商品一覧の取得結果。cursor を持たないため NextCursor はない。
  fields:
    - name: Items
      type: "[]ProductView"  # 在庫の少ない順（同数は商品 ID 昇順）
```

## Dependencies

```yaml
- tracer              # observability.TracerFactory -> LayerTracer
- txm                 # boundary/tx.Manager（CreateProduct / UpdateProduct / UpdateProductStock のトランザクション境界）
- product_repository  # domain/product.Repository（FindPublishedList / FindAllLowStock / FindPublishedByID / FindByID / LockByID / Create / Update / UpdateStock）
- category_repository # domain/product/category.Repository（FindByID でカテゴリ名称を解決）
- status_repository   # domain/product/status.Repository（FindByID でステータス名称を解決）
- authorizer          # boundary/authz.Authorizer（CreateProduct / UpdateProduct / UpdateProductStock / ListLowStockProducts の admin 認可）
- product_image_query_service # usecase/product/query.ProductImageQueryService（SweepOrphans の参照照合。商品を経由しない横断読みのため QueryService。docs/design/data-access-pattern.md §3.3）
```

## Workflow

### ListProducts

```yaml
tx_required: false
steps:
  - Cursor が nil の場合は apperror.ErrInvalidArgument を返す
  - MinPrice / MaxPrice を money.Price へ変換する。非数値・負値・40 文字超過は apperror.ErrInvalidArgument を返す
  - MinQuantity / MaxQuantity が負値の場合は apperror.ErrInvalidArgument を返す
  - 価格または在庫数の両境界が指定され、下限が上限を超える場合は apperror.ErrInvalidArgument を返す
  - authorizeUnpublishedRead で可視範囲の指定を検査する（IncludeUnpublished が false なら何も課さない）
  - IncludeUnpublished が false の場合は findPublishedPage、true の場合は findAllPage でページを取得する
  - findPublishedPage は decodeProductCursor で境界（publishedAt, id）へ復号し、ListParams を組み立てて product_repository.FindPublishedList を呼び、取得した各 Product が Product.IsPublished を満たすことを確かめる（SQL の絞り込みとドメインの定義の乖離検出）
  - findAllPage は decodeAllProductCursor で境界（createdAt, id）へ復号し、AllListParams を組み立てて product_repository.FindAllList を呼ぶ（未公開を含むため IsPublished の検算は行わない）
  - 取得件数が Cursor.Limit() を超える場合は次ページありと判定し、末尾を切り詰める
  - 各 Product を ProductView（ID / Name / Description / Price / Quantity / StockWarningThreshold / StatusID / CategoryID / PublishedAt）へ写像する
  - 次ページありの場合、切り詰め後の末尾行から取得した枝に対応するカーソル（encodeProductCursor / encodeAllProductCursor）で NextCursor を生成する
calls:
  - authz.Authorize
  - product_repository.FindPublishedList
  - product_repository.FindAllList
errors:
  - Cursor が nil の場合は apperror.ErrInvalidArgument
  - 価格の非数値・負値・40 文字超過、在庫数の負値、または上下限の逆転は apperror.ErrInvalidArgument
  - IncludeUnpublished が true で authn が nil の場合は apperror.ErrUnauthenticated（401）
  - IncludeUnpublished が true で Authorizer が拒否した場合はその理由（admin でなければ 403）
  - カーソル復号失敗時は apperror.ErrInvalidArgument（発行時と異なる可視範囲へ持ち越したカーソルを含む）
  - product_repository.FindPublishedList / FindAllList のエラーをそのまま伝播する
  - 公開済みのみの取得行に Product.IsPublished を満たさないものが混じっていた場合は apperror.ErrInternal（500。SQL とドメインの乖離）
```

### CountProducts

```yaml
tx_required: false
steps:
  - 一覧と共通の検索入力検証を行い、価格を money.Price へ変換する
  - authorizeUnpublishedRead で可視範囲の指定を検査する（一覧と同一の判定）
  - domain の SearchFilter を組み立てる
  - IncludeUnpublished が false なら product_repository.CountPublished、true なら product_repository.CountAll で一致件数を取得する
  - ProductCountView へ写像して返す
calls:
  - authz.Authorize
  - product_repository.CountPublished
  - product_repository.CountAll
errors:
  - 価格の非数値・負値・40 文字超過、在庫数の負値、または上下限の逆転は apperror.ErrInvalidArgument
  - IncludeUnpublished が true で authn が nil の場合は apperror.ErrUnauthenticated（401）
  - IncludeUnpublished が true で Authorizer が拒否した場合はその理由（admin でなければ 403）
  - product_repository.CountPublished / CountAll のエラーをそのまま伝播する
```

### GetProduct

```yaml
tx_required: false
steps:
  - authorizeUnpublishedRead で可視範囲の指定を検査する（商品を引く前に判定するため、拒否から存在有無は分からない）
  - IncludeUnpublished が false の場合は product_repository.FindPublishedByID で公開中の単一商品を取得し（未存在・非公開はいずれも NotFound）、取得した Product が Product.IsPublished を満たすことを確かめる（SQL の絞り込みとドメインの定義の乖離検出）
  - IncludeUnpublished が true の場合は product_repository.FindByID で公開状態を問わず取得する（未存在は NotFound）
  - Product を ProductView（ID / Name / Description / Price / Quantity / StockWarningThreshold / StatusID / CategoryID / PublishedAt）へ写像する
calls:
  - authz.Authorize
  - product_repository.FindPublishedByID
  - product_repository.FindByID
errors:
  - IncludeUnpublished が true で authn が nil の場合は apperror.ErrUnauthenticated（401）
  - IncludeUnpublished が true で Authorizer が拒否した場合はその理由（admin でなければ 403）
  - product_repository.FindPublishedByID / FindByID のエラーをそのまま伝播する（未存在・非公開は apperror.ErrNotFound → 404）
  - 公開済みのみの取得行が Product.IsPublished を満たさない場合は apperror.ErrInternal（500。SQL とドメインの乖離）
```

### CreateProduct

```yaml
tx_required: true
steps:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）を返す
  - authorizer.Authorize（ActionProductCreate / resource=product）で admin 認可を確認する（拒否は 403）
  - price（文字列）を decimal へ解釈し money.NewPrice で非負を検証する（負値は 422 / 非数値は 400。非数値は OpenAPI でも弾く）
  - uuid.New で商品 ID を採番する
  - txm.Do 内で以下を実行する:
      - status_repository.FindByID / category_repository.FindByID で名称を解決する（未存在は整合性異常として ErrInternal=500）
      - product.New で商品エンティティを構築する（負在庫・名称長超過は 422）
      - product_repository.Create で登録する（DB の FK 制約は多層防御の保険。正典の 500 は上のマスタ確認）
  - 生成した Product を ProductView（Images / PublishedAt を含む）へ写像して返す
calls:
  - status_repository.FindByID
  - category_repository.FindByID
  - product_repository.Create
errors:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）
  - 認可拒否は authz 由来の apperror.ErrPermissionDenied（403）
  - 負価格・負在庫・名称長超過は apperror.ErrValidation（422）
  - status_id / category_id 不在は apperror.ErrInternal（500・整合性異常）
```

### UpdateProduct

```yaml
tx_required: true
steps:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）を返す
  - authorizer.Authorize（ActionProductUpdate / resource=product）で admin 認可を確認する（拒否は 403）
  - price が指定された場合のみ decimal へ解釈し money.NewPrice で非負を検証する（負値は 422 / 非数値は 400）
  - txm.Do 内で以下を実行する:
      - product_repository.FindByID で現在の商品を読み込む（未存在は 404。未公開商品も対象）
      - EnsureVersion で要求バージョンと現在バージョンの一致を確認する（不一致は 409。この時点で書き込みへ進まない）
      - statusId / categoryId のいずれかが指定された場合、status / category の参照をペアで再解決する（未指定側も現在の ID でマスタと突合し参照整合を再確認する）。両方とも未指定の場合のみ現在値を据え置き、マスタ問い合わせを行わない
      - 未指定は現在値、null 明示は nil へ解決した確定値で product.Update を呼ぶ（不変条件違反は 422）
      - product_repository.Update で読み込み時点のバージョンを条件に更新し、採番後のバージョンを受け取る（0 行は 409）。画像も確定値の集合へ併せて一致する
  - Product を ProductView へ写像し、Version を採番後の値で上書きして返す
calls:
  - product_repository.FindByID
  - status_repository.FindByID
  - category_repository.FindByID
  - product_repository.Update
errors:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）
  - 認可拒否は authz 由来の apperror.ErrPermissionDenied（403）
  - 未存在は apperror.ErrNotFound（404）
  - バージョン不一致は product.ErrVersionConflict（apperror.ErrConflict → 409）
  - 負価格・負在庫・名称長超過は apperror.ErrValidation（422）
  - status_id / category_id 不在は apperror.ErrInternal（500・整合性異常）
```

> 部分更新の 3 状態（未送信 / null 明示 / 値指定）は `pkg/patch.Field` で表し、usecase が現在値に対して解決する。
> domain は解決後の確定値のみを受け取り、3 状態を知らない。
>
> 409（バージョン不一致）は、`tx.Manager` が透過的にリトライする serialization_failure（ADR-0034 (commandservice-atomicity-criterion)）とは別物で、
> 同じ内容の再送では解消しない。クライアントは最新を取得し直してからやり直す必要がある。
>
> 画像は `Update` が同じ呼び出しの中で同期するため、usecase は画像の書き込みを別途指示しない。同期は ID を
> 鍵とする差分であり、`images` が未指定の更新ではエンティティが読み込み時の画像を ID ごと保持しているため
> 差分が空になる（価格だけの部分更新で画像の履歴が埋まることはない）。画像だけを差し替えた場合も
> `lock_version` は進む（画像は集約の一部であり、他の編集者が 409 で検出できる必要がある）。

### UpdateProductStock

```yaml
tx_required: true
steps:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）を返す
  - authorizer.Authorize（ActionProductStockUpdate / resource=product）で admin 認可を確認する（拒否は 403）
  - txm.Do 内で以下を実行する:
      - product_repository.LockByID で対象商品を悲観ロックして取得する（未存在は 404。未公開商品も対象）
      - product.AdjustStock(delta) で在庫を増減する（増減後が負になる場合は 422。この時点で書き込みへ進まない）
      - product_repository.UpdateStock で取得時点のバージョンを条件に在庫を更新し、採番後のバージョンを受け取る（0 行は 409）
  - Product を ProductView へ写像し、Version を採番後の値で上書きして返す
calls:
  - product_repository.LockByID
  - product_repository.UpdateStock
errors:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）
  - 認可拒否は authz 由来の apperror.ErrPermissionDenied（403）
  - 未存在は apperror.ErrNotFound（404）
  - 取得後に他者が更新していた場合は product.ErrVersionConflict（apperror.ErrConflict → 409）
  - 増減後の在庫が保持できる範囲を外れる場合は product.ErrInvalidQuantity（apperror.ErrValidation → 422）
  - 直列化の待機が解消できない場合は apperror.ErrUnavailable（503）
```

> 行ロックで取得〜更新を直列化するため、並行する在庫の増減は失われず合成される。行ロック競合
> （serialization_failure / deadlock）は `tx.Manager` が透過的にリトライし、枯渇した場合とロック待ちが
> タイムアウトした場合のみ一時障害（503）として露出する。
>
> 409（バージョン不一致）は、直列化を経てもなお取得後の変化を検出した場合の fail-closed な応答であり、
> 一時障害と異なり同じ内容の再送では解消しない。クライアントは最新を取得し直してからやり直す。
>
> 在庫の更新もバージョンを進めるため、更新前のバージョンを持つ部分更新（`UpdateProduct`）は 409 で弾かれる。
> これにより、悲観ロック（同時更新の直列化）と楽観ロック（読み込み後の変化の検出）が同じ行で噛み合う。

### ListLowStockProducts

```yaml
tx_required: false
steps:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）を返す
  - authorizer.Authorize（ActionProductListLowStock / resource=product・所有者なし）で admin 認可を確認する（拒否は 403）
  - paging.NewLimit（lowStockLimitPolicy）で取得件数を正規化する（0 以下は既定値 20、上限超過は 100 へクランプ）
  - product_repository.FindAllLowStock で在庫僅少商品を在庫の少ない順に最大 limit 件取得する
  - 取得した各 Product が Product.IsLowStock を満たすことを確かめる（絞り込みを実行する SQL と、在庫僅少を定義するドメイン述語の乖離検出）
  - 各 Product を ProductView へ写像する
calls:
  - product_repository.FindAllLowStock
errors:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）
  - 認可拒否は authz 由来の apperror.ErrPermissionDenied（403）
  - product_repository.FindAllLowStock のエラーをそのまま伝播する
  - 取得行が Product.IsLowStock を満たさない場合は apperror.ErrInternal（500。SQL とドメインの乖離）
```

> `limit` の既定値は OpenAPI の `default: 20` ではなく usecase の `paging.NewLimit` が与える。
> oapi-codegen は任意クエリパラメータへ既定値を適用せず、未指定は nil のままハンドラへ届くため。
> 範囲外（1 未満 / 100 超）の要求は OpenAPI リクエストバリデータが 400 で弾くため、クランプは
> バリデータを通らない経路に対する二重防御として働く。

### SweepOrphans（ImageGCUsecase）

```yaml
tx_required: false   # DB は読み取りのみ。削除先はオブジェクトストレージで、2 相コミットできない
steps:
  - grace / batchSize が 0 以下なら既定値（24 時間 / 1000 件）に置き換える
  - clock.Now から grace を引いて打ち切り時刻 cutoff を決める
  - 列挙が尽きるまでページを反復
      - object_storage.List で prefix="products/" のオブジェクトを最大 batchSize 件取得する
      - 接頭辞が products/ で、かつ ModifiedAt が cutoff より前のキーだけを候補に絞る（検査件数へ計上）
      - 候補が 0 件ならそのページは照合も削除も行わない
      - product_image_query_service.FilterExistingImagePaths で参照済みのパスを特定し、候補から除外する
        （論理削除された画像は現在の参照ではないため、差し替えで外れた画像はここで孤児になる）
      - dryRun でなければ object_storage.Delete で残りを削除し、削除件数を加算（dryRun では対象件数のみ加算）
      - NextCursor が空なら終了。非空なら次ページへ
  - 累計の ImageGCResult を返す
calls:
  - clock.Now
  - object_storage.List
  - product_image_query_service.FilterExistingImagePaths
  - object_storage.Delete
errors:
  - 各依存のエラーを伝播。削除済みのオブジェクトは復元できないため、エラー時もそこまでの累計を
    ImageGCResult に含めて返す
notes:
  - 参照照合が失敗したページでは、オブジェクトを 1 件も削除せずに中断する。
    「照合エラー = 未参照」と倒れるとバケット内の全画像を消すため、唯一の致命的な失敗モードにあたる。
  - 猶予期間が方式の核心。アップロード直後のオブジェクトは「商品作成フォーム記入中でまだ参照されていない」
    正常な状態と区別がつかないため、年齢述語なしでは正常なアップロードを削除してしまう。
  - 列挙が prefix を無視する実装に当たっても商品画像以外を消さないよう、候補を絞る際に接頭辞を再検査する。
```

## Workflow — DiscontinueProduct

```yaml
- name: DiscontinueProduct
  signature: DiscontinueProduct(ctx, authn, id, params) (DiscontinueProductView, error)
  tx_required: true
  calls:
    - authz.Authorize             # ActionProductDiscontinue（admin のみ）
    - coupon.NewRateDiscount      # 要求の率を検証。範囲外はここで 422
    - coupon.NewCategoryScope     # 適用範囲を検証。廃番商品のカテゴリで固定
    - clock.Now
    - product.Repository.LockByID
    - purchase.Repository.FindStatusesByProductID
    - discontinuation.EnsureDiscontinuable
    - product.Product.Discontinue
    - product.Repository.Update
    - command.CommandService.IssueDiscontinuationCoupons
  behavior: |
    商品を廃番にし、その商品の明細を持っていた確定済みユーザーへ代替クーポンを一括発行する。
    非公開化と発行は同一トランザクションで、「クーポンだけ配られて商品がまだ買える」状態は
    observable にならない（ADR-0034 の分岐 3。判別式への当てはめは同 ADR の worked instance）。

    商品行を先にロックしてから進行中の購入を判定する。購入作成が同じ商品行を id 順にロックする経路と
    直列化されるため、判定から commit まで条件が覆らない（分岐 2 / ADR-0036）。

    既に廃番の商品への再実行は、新たな発行を伴わず現在の状態だけを返す。明細を取り除かない設計のため、
    毎回発行すると同じ母集団へ重複して配ることになる。3 つの件数はいずれも「その実行が起こしたこと」を
    表すため、何も起こさない再実行では 0 になる。過去の発行実績は返さない。適用範囲は廃番商品の
    カテゴリで固定されるため、発行事由（どの商品の廃番が配ったか）は適用範囲からは辿れない。
  invariants:
    - カートの明細は取り除かない。以降の取得で discontinued の issue が立ち、理由が利用者に残る
    - クーポンの受給対象は実行時点で明細を保持していた確定済みユーザーのみ。ゲストと退会済み、および
      実行後に投入した利用者は対象外
    - 適用範囲は廃番商品のカテゴリで固定される
  errors:
    - 非 admin: 403
    - 未存在: 404
    - 進行中の購入が残っている: 409（discontinuation.ErrInProgressPurchaseExists）
    - 値引き率が範囲外: 422（coupon.ErrInvalidDiscountValue）
  notes:
    - 楽観ロックのバージョンを受け取らない。廃番は属性の書き換えではなく命令であり、実行の可否は
      商品の現在値ではなく進行中の購入の有無で決まる。そのため 409 は「進行中の購入がある」の一義。
    - 代替クーポンの条件（率・有効期間）を要求で受けるのは、何をどれだけ補償するかが業務の判断であり、
      この操作はその判断を実行するだけだからである。

- name: GetDiscontinueImpact
  signature: GetDiscontinueImpact(ctx, authn, id) (DiscontinueImpactView, error)
  tx_required: false
  calls:
    - authz.Authorize             # ActionProductDiscontinue（実行と同じ権限で守る）
    - product.Repository.FindByID
    - query.DiscontinueImpactQueryService.EstimateDiscontinueImpact
  behavior: |
    廃番にした場合の影響を実行前に件数で返す。行をロックしないため、返した値は実行時の件数と一致する
    保証を持たない。商品の存在は見積もりの前に確かめる（存在しない商品の影響は 0 件ではなく引けない）。
  invariants:
    - AffectedUserCount <= AffectedCartCount（ゲストのカートと退会済みを除くため）
  errors:
    - 非 admin: 403
    - 未存在: 404
  notes:
    - 参照でありながら実行と同じ ActionProductDiscontinue で守る。見積もりが返すのは廃番によって
      失われる顧客影響の規模であり、それを知ってよいのは廃番を実行できる主体だけだからである。
      参照だからという理由で別の弱い Action を与えると、実行できない主体が実行の判断材料だけを
      得られることになる。
```
