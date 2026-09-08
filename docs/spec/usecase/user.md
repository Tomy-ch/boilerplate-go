# User — Usecase Spec

> 既存実装（`internal/usecase/user`）を spec 化したベースに、詳細系エンドポイント（GetUsersDetail / Put / Patch / Delete）向けの GetUser / UpdateUser / UpdateUserPartially / DeleteUser を追記したもの。
> 更新・論理削除は「load → ドメインメソッドで変更 → Update で永続化」に統一。認証は外部の OIDC/JWT に委譲し、ユーザーの認証情報は本ユースケースでは扱わない。
> 論理削除済みユーザーは detail 系（GET / PUT / PATCH / DELETE）の対象外とし、`FindByID` / `Update` の SQL で `deleted_at IS NULL` をフィルタする。これにより削除済みへの取得・更新・再削除はすべて `NotFound`（404）に統一される（GET で削除済みを返したり、更新でエラー種別がぶれることを防ぐ）。

## Overview

ユーザーユースケースは、ユーザー一覧取得・作成・件数取得を提供するアプリケーションサービス。ドメインの `user.Repository` と `prefecture.Repository` をオーケストレーションし、ドメインエンティティを外側に晒さず DTO（出力 `UserView` / 更新入力 `UpdateProfileParams`）へ変換して返す。

都道府県は ID 参照のみを保持する設計のため、一覧・作成ともに `prefecture.Repository` から都道府県名を解決して DTO に詰める。作成時はトランザクション境界内で都道府県解決・エンティティ生成・永続化を行い、同じ境界でウェルカムクーポンを 1 枚発行する。

認可は 2 通りに分かれる。詳細系（GetUser / UpdateUser / UpdateUserPartially / DeleteUser）は対象ユーザーを所有者とするリソースとして問い合わせるため admin または本人が通る。列挙系（ListUsers / ListUsersWithTotal / ListUsersFeed）は他ユーザーを開示する操作であり、所有者を持たないリソース（`ownerID = nil`）として問い合わせて所有者フォールバックを成立させないことで admin 限定にする。いずれも認可はリポジトリ呼び出しより前に置き、拒否された呼出元がデータへ到達しないようにする。

## Interface

```yaml
package: internal/usecase/user
name: Usecase
methods:
  # 列挙系（他ユーザーを開示するため admin 限定。認可判定のため認証主体 authn を受け取る）
  - name: ListUsers
    signature: ListUsers(ctx context.Context, authn *auth.Authn, active *bool, page *paging.Page) ([]UserView, error)
  - name: ListUsersWithTotal
    signature: ListUsersWithTotal(ctx context.Context, authn *auth.Authn, active *bool, page *paging.Page) (*UserListView, error)
  - name: ListUsersFeed
    signature: ListUsersFeed(ctx context.Context, authn *auth.Authn, cursor *paging.Cursor) (*UserFeedView, error)
  - name: CreateUser
    signature: CreateUser(ctx context.Context, dto *CreateParamsDTO) (UserView, error)
  # 件数のみを返し個々のユーザーを開示しないため、認証主体を持たないジョブ（usercount）からの利用を許して認可を要求しない
  - name: CountUsers
    signature: CountUsers(ctx context.Context, active *bool) (int64, error)
  # 詳細系エンドポイント向け（いずれも認可判定のため認証主体 authn を受け取る。admin または対象ユーザー本人）
  - name: GetUser
    signature: GetUser(ctx context.Context, authn *auth.Authn, id uuid.UUID) (UserView, error)
  - name: UpdateUser
    signature: UpdateUser(ctx context.Context, authn *auth.Authn, id uuid.UUID, dto *UpdateProfileParams) (UserView, error)
  - name: UpdateUserPartially
    signature: UpdateUserPartially(ctx context.Context, authn *auth.Authn, id uuid.UUID, dto *PatchParamsDTO) (UserView, error)
  - name: DeleteUser
    signature: DeleteUser(ctx context.Context, authn *auth.Authn, id uuid.UUID) error
```

退会後の物理削除はジョブ専用の入口で、HTTP ハンドラが使う `Usecase` とは利用者も依存も異なるため、独立したインターフェースに分ける（`outbox.GCUsecase` と同じ形）。

```yaml
package: internal/usecase/user
name: PurgeUsecase
methods:
  - name: PurgeDeleted
    signature: PurgeDeleted(ctx context.Context, retention time.Duration, batchSize int32, dryRun bool) (PurgeResult, error)
```

## DTOs

```yaml
- name: UserView
  description: ユーザー取得結果の出力 DTO。
  fields:
    - name: FirstName
      type: string
    - name: LastName
      type: string
    - name: Email
      type: string
    - name: Phone
      type: string
    - name: PostalCode
      type: string
    - name: PrefectureName
      type: string          # prefecture.Repository から解決した都道府県名
    - name: City
      type: string
    - name: Street
      type: string
    - name: Building
      type: "*string"
    - name: DeletedAt
      type: "*time.Time"
- name: UpdateProfileParams
  description: ユーザープロフィール更新の入力（可変フィールド）。出力専用の DeletedAt は含まない。
  fields:
    - name: FirstName
      type: string
    - name: LastName
      type: string
    - name: Email
      type: string
    - name: Phone
      type: string
    - name: PostalCode
      type: string
    - name: PrefectureName
      type: string
    - name: City
      type: string
    - name: Street
      type: string
    - name: Building
      type: "*string"
- name: CreateParamsDTO
  description: ユーザー作成に必要なパラメータ。UpdateProfileParams を埋め込む。
  fields:
    - name: UserID
      type: uuid.UUID
    - name: UpdateProfileParams
      type: UpdateProfileParams   # embedded
# 詳細系エンドポイント向け（追記分）
# UpdateUser（PUT・プロフィール全更新）は入力に UpdateProfileParams、出力に UserView を使う
# （DeletedAt は出力専用で更新入力には持たせない）。
- name: PatchParamsDTO
  description: |
    PATCH（部分更新）の入力。指定フィールドのみ更新し、nil（未指定）は据え置き。
    PATCH は「部分マージ」セマンティクスを採用し、**フィールドのクリア（null 設定）は提供しない**。
    送信が省略でも null でも同じ nil として扱い「据え置き」となる（生成リクエスト型が `*string` で
    未指定と null を区別できないため、JSON Merge Patch 的な区別は持たない）。
    フィールドを空/クリアしたい場合は **PUT（全更新、building は null 指定可）** を使う。
  fields:
    - name: FirstName
      type: "*string"
    - name: LastName
      type: "*string"
    - name: Email
      type: "*string"
    - name: Phone
      type: "*string"
    - name: PostalCode
      type: "*string"
    - name: PrefectureName
      type: "*string"
    - name: City
      type: "*string"
    - name: Street
      type: "*string"
    - name: Building
      type: "*string"   # nullable だが、未指定/null とも nil=据え置き扱い。クリアは PUT を使う
- name: PurgeResult
  description: 退会後の物理削除の実行結果。
  fields:
    - name: Purged
      type: int64   # 物理削除したユーザー件数。dryRun では削除対象となった件数
    - name: SkippedWithPurchases
      type: int64   # 購入を保持しているため削除しなかったユーザー件数
```

## Dependencies

```yaml
- tracer            # observability.TracerFactory -> LayerTracer（メソッドごとに span）
- tx_manager        # boundary/tx.Manager
- clock             # boundary/clock.Clock
- authorizer        # boundary/authz.Authorizer（詳細系は admin または対象ユーザー本人、列挙系は admin 限定）
- user_repository   # domain/user.Repository
- user_lock_repository   # domain/user.LockRepository（退会時の対象行の排他ロック。[ADR-0036 (ordered-pessimistic-row-locks)]）
- prefecture_repository  # domain/prefecture.Repository
- purchase_repository    # domain/purchase.Repository（退会時の進行中購入の確認）
- coupon_repository      # domain/coupon.Repository（登録時のウェルカムクーポンの発行）
- domain/service/membership        # EnsureWithdrawable（退会可否の判定）
- outbox_emit       # usecase/outbox.EmitUsecase（退会イベントの発行）
```

## Workflow

### ListUsers

```yaml
tx_required: false
steps:
  - authorizer.Authorize で列挙の認可を確認（所有者を持たないリソースとして問い合わせるため所有者フォールバックが成立せず admin のみ許可。authn が nil なら ErrUnauthenticated）
  - userRepo.FindByActive で active / ページング条件に基づきユーザー一覧を取得
  - 取得した各ユーザーの prefectureID を集約し、pftRepo.FindByIDs で都道府県をまとめて解決（N+1 回避、子 span で計測）
  - prefectureID -> Prefecture のマップを構築
  - 各ユーザーを UserView へ変換し、都道府県名を埋める
calls:
  - authorizer.Authorize
  - user_repository.FindByActive
  - prefecture_repository.FindByIDs
errors:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）。認可判定そのものを行わない
  - 認可が拒否された場合は authz.ErrForbidden（403）。リポジトリには到達しない
  - userRepo / pftRepo のエラーを伝播。ユーザーが参照する都道府県を FindByIDs で解決できない場合は参照整合性破れとして ErrInternal(500)
```

### ListUsersWithTotal

```yaml
tx_required: false
steps:
  - authorizer.Authorize で列挙の認可を確認（ListUsers と同じ admin 限定の判定。authn が nil なら ErrUnauthenticated）
  - 認可済みの内部処理として一覧と総件数を取得し、UserListView へまとめる（認可はこの入口で 1 度だけ行い、内部処理では繰り返さない）
calls:
  - authorizer.Authorize
  - user_repository.FindByActive
  - prefecture_repository.FindByIDs
  - user_repository.CountByActive
errors:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）。認可判定そのものを行わない
  - 認可が拒否された場合は authz.ErrForbidden（403）。リポジトリには到達しない
  - 一覧取得・件数取得のエラーをそのまま伝播
```

### ListUsersFeed

```yaml
tx_required: false
steps:
  - authorizer.Authorize で列挙の認可を確認（ListUsers と同じ admin 限定の判定。authn が nil なら ErrUnauthenticated）
  - cursor を復号し、userRepo.FindFeed で未削除ユーザーを作成日時の降順に limit+1 件取得
  - limit を超えた分を切り落として次ページの有無を判定し、最終要素から次カーソルを構築
  - 各ユーザーを UserView へ変換し、UserFeedView へまとめる
calls:
  - authorizer.Authorize
  - user_repository.FindFeed
  - prefecture_repository.FindByIDs
errors:
  - authn が nil の場合は apperror.ErrUnauthenticated（401）。認可判定そのものを行わない
  - 認可が拒否された場合は authz.ErrForbidden（403）。cursor の復号にもリポジトリにも到達しない
  - cursor が nil または復号できない場合は ErrInvalidArgument（400）
  - userRepo / pftRepo のエラーを伝播。都道府県を解決できない場合は参照整合性破れとして ErrInternal(500)
```

### CreateUser

```yaml
tx_required: true
steps:
  - clock.Now で現在時刻を取得
  - トランザクション内で
      - pftRepo.FindByName で都道府県を名前解決
      - user.New でエンティティ生成（不変条件検証）
      - userRepo.Create で永続化
      - coupon.New でウェルカムクーポンを生成し、couponRepo.Create で永続化
  - 生成したエンティティと都道府県名から UserView を構築して返す
calls:
  - clock.Now
  - tx_manager.Do            # トランザクション境界。内部で以下を実行
  - prefecture_repository.FindByName
  - user.New
  - user_repository.Create
  - coupon.New
  - coupon_repository.Create
errors:
  - FindByName / user.New / Create のエラーを伝播
  - ウェルカムクーポンの生成・永続化のエラーも伝播し、登録ごと巻き戻す
notes:
  - 登録という業務イベントの副作用としてクーポンを 1 枚発行する。受給者は登録者本人で、
    値引きは定額・適用範囲は全体に固定。金額と有効期間は要求では受けず、ユースケースが定数で持つ
    （クーポン集約は発行事由を保持しないため、「ウェルカム」であることを表すのはこの配線だけ）。
  - 発行を登録と同じトランザクションに置くことで、登録者は必ず 1 枚持つか 1 枚も持たないかのどちらかになる。
  - 二重発行を防ぐ専用の判定は置かない。入口の PostUsers が認証主体の ID をそのまま PK に使うため、
    同一主体の 2 回目は user_repository.Create が一意制約で落ち、同じトランザクションのクーポンも巻き戻る。
invariants:
  - 受給者は登録されたユーザー本人である
  - 発行されたクーポンは未使用で、有効期限は発行日時より後である
```

### CountUsers

列挙系で唯一 `authn` を取らない。件数のみを返し個々のユーザーを開示しないため、認証主体を持たない
`usercount` ジョブ（`internal/controller/job/usercount`）からの利用を許している。HTTP 経路から件数を返す
`ListUsersWithTotal` は authn を取り、この入口で admin 判定を通してから内部処理として件数を数える。

```yaml
tx_required: false
steps:
  - userRepo.CountByActive で active 条件に基づく総件数を取得して返す
calls:
  - user_repository.CountByActive
errors:
  - userRepo のエラーをそのまま伝播
```

### GetUser

```yaml
tx_required: false
steps:
  - user_repository.FindByID で単一ユーザーを取得（存在しない / 論理削除済みなら NotFound 伝播）
  - prefecture_repository.FindByID で都道府県名を解決
  - UserView へ変換して返す（PrefectureName を埋める）
calls:
  - user_repository.FindByID
  - prefecture_repository.FindByID
errors:
  - user FindByID の NotFound は伝播（404）。prefecture FindByID の NotFound は参照整合性破れとして ErrInternal(500)、その他は伝播
```

### UpdateUser

```yaml
tx_required: true
steps:
  - clock.Now で現在時刻を取得
  - トランザクション内で
      - user_repository.FindByID で対象を取得（存在しない / 論理削除済みなら NotFound 伝播）
      - prefecture_repository.FindByName で都道府県を名前解決
      - user.UpdateProfile で全プロフィールフィールド + updatedAt を置換
      - user_repository.Update で永続化
  - UserView へ変換して返す
calls:
  - clock.Now
  - tx_manager.Do
  - user_repository.FindByID
  - prefecture_repository.FindByName
  - user.UpdateProfile
  - user_repository.Update
errors:
  - FindByID(NotFound) / FindByName / UpdateProfile / Update を伝播
```

### UpdateUserPartially

```yaml
tx_required: true
steps:
  - clock.Now で現在時刻を取得
  - トランザクション内で
      - user_repository.FindByID で対象を取得（存在しない / 論理削除済みなら NotFound 伝播）
      - PrefectureName が指定されていれば prefecture_repository.FindByName で都道府県解決（入力エラーは伝播）、未指定なら prefecture_repository.FindByID で現在の都道府県を解決（レスポンス名用。未解決は参照整合性破れ）
      - 各フィールドは provided なら新値、nil なら現在値（getter）をマージしてフルセットを構築（未指定/null とも nil=据え置き。フィールドのクリアは PATCH では提供せず PUT を使う）
      - user.UpdateProfile でマージ後のフルセット + updatedAt を置換
      - user_repository.Update で永続化
  - UserView へ変換して返す
calls:
  - clock.Now
  - tx_manager.Do
  - user_repository.FindByID
  - prefecture_repository.FindByName
  - user.UpdateProfile
  - user_repository.Update
errors:
  - user FindByID(NotFound) / FindByName / UpdateProfile / Update を伝播。指定なし時に現在の都道府県(FindByID)が NotFound なら ErrInternal(500)
```

### DeleteUser

```yaml
tx_required: true
steps:
  - authorizer.Authorize で退会の認可を確認（対象ユーザーを所有者とするリソース。admin または本人のみ許可。authn が nil なら ErrUnauthenticated）
  - clock.Now で現在時刻を取得
  - トランザクション内で（論理削除・イベント発行・拒否判定を単一 tx にまとめ、退会だけが成立してイベントが失われることを防ぐ）
      - user_lock_repository.LockByID で対象を排他ロックして取得（存在しない / 論理削除済みなら NotFound 伝播。SQL が deleted_at IS NULL でフィルタするため、削除済みへの再 DELETE は NotFound になる）
      - purchase_repository.FindStatusesByUserID で購入のステータスを取得し、membership.EnsureWithdrawable で退会可否を判定する。進行中の購入が残っていれば Conflict で拒否（論理削除もイベントも残さない）
      - user.MarkAsDeleted で deletedAt を設定（ドメイン不変条件として既削除なら ErrAlreadyDeleted を返すが、LockByID フィルタにより通常経路では到達しない防御的チェック）
      - user_repository.Update で永続化（論理削除）
      - event.BuildWithdrawn で自己完結スナップショットを構築し、outbox_emit.Emit で user.withdrawn.v1 を発行（退会に伴う関連データの後始末は受信側の結果整合に委ねる）
calls:
  - authorizer.Authorize
  - clock.Now
  - tx_manager.Do
  - user_lock_repository.LockByID
  - purchase_repository.FindStatusesByUserID
  - membership.EnsureWithdrawable
  - user.MarkAsDeleted
  - user_repository.Update
  - outbox_emit.Emit
errors:
  - authn が nil なら ErrUnauthenticated / Authorize 拒否は ErrForbidden(PermissionDenied) を伝播
  - 進行中の購入が残っている場合は ErrConflict
  - LockByID(NotFound) / FindStatusesByUserID / MarkAsDeleted(ErrAlreadyDeleted) / Update / Emit を伝播
```

ロックの取得順が不変条件である（[ADR-0036 (ordered-pessimistic-row-locks)]）。`LockByID` は進行中購入の判定より**前**に置く。判定より後だと
「退会が判定を通過 → 購入作成が成立 → 退会が確定」の順序を止められず、退会済みユーザーに進行中の購入が
ぶら下がる。購入作成側は同じ行を共有ロックで押さえるため、この排他ロックとだけ衝突して直列化される。

この拒否は、購入作成側の「退会済みユーザーは購入できない」と**対になる 1 つの業務ルール**である。片方だけを
読んでも全体は分からないため、もう一方は [`docs/spec/usecase/purchase.md`](./purchase.md) の
`CreatePurchase` に記述してある。両方向とも 409（`ErrConflict`）で答える（主体自身のライフサイクル状態と要求
操作の衝突であり、他者のリソースの存在を秘匿する 404 とは性質が違う）。

不変条件は入口で閉じているため、**退会側に補償処理は置かない**。`user.withdrawn.v1` の consumer が残った
進行中購入をキャンセルする設計も考えられるが、閉じた不変条件のもとでは対象が存在しない。既存 consumer
（`internal/controller/worker/withdrawalarchive`）はアーカイブの責務のみを持ち、キャンセルの責務は負わない。

### PurgeDeleted（PurgeUsecase）

```yaml
tx_required: true   # 1 バッチ = 1 トランザクション
steps:
  - retention / batchSize が 0 以下なら既定値（30 日 / 1000 件）に置き換える
  - clock.Now から retention を引いて打ち切り時刻 cutoff を決める
  - 候補が尽きるまでバッチを反復（各バッチをトランザクション内で実行）
      - user_repository.FindDeletedBefore で cutoff より古い候補を ID 昇順の keyset で最大 batchSize 件取得
      - 候補が 0 件ならそのバッチは何もしない
      - purchase_repository.FindUserIDsWithPurchases で購入を持つ候補を特定し、スキップ件数に計上
      - dryRun でなければ user_repository.PurgeByIDs で残りを物理削除し、削除件数を加算（dryRun では対象件数のみ加算）
      - 取得件数が batchSize に満たなければ終了。満ちていれば境界を「取得した候補の末尾 ID」へ進めて次バッチへ
  - 累計の PurgeResult を返す
calls:
  - clock.Now
  - tx_manager.Do
  - user_repository.FindDeletedBefore
  - purchase_repository.FindUserIDsWithPurchases
  - user_repository.PurgeByIDs
errors:
  - 各 Repository のエラーを伝播。失敗したバッチはロールバックされるが、それ以前にコミットされた
    バッチの物理削除は取り消せないため、エラー時もそこまでの累計を PurgeResult に含めて返す
notes:
  - 境界は削除可否によらず必ず候補の末尾まで進める。購入保持でスキップされた候補は削除されず残るため、
    境界を進めないと同じ候補を取り直し続け、先頭バッチが全件スキップ対象のときに無限ループする。
```

## GET ロール（me）

`GET /v1/users/me/roles`。認証主体自身に割り当てられたロールを返す読み取り経路。用途はクライアント側の
**楽観的な権限分岐**（管理者向け導線を出すかどうか）であり、確定的な認可判断は各操作で `authz.Authorizer` が行う。
所有権は「パスに他者識別子を持たず `authn.UserID()` のみを引数にする」ことで担保され、認可判定に分岐の余地が
ないため `authorizer` は経由しない。書き込みを伴わないため tx は不要。

ユースケースは `internal/usecase/user/user_usecase.go`（tx / 認可 / 都道府県解決を持つプロフィール中心の集約）ではなく、
読み取り専用の別パッケージ `internal/usecase/user/role` に置く（`purchase/summary` と同じ `/me` 配下サブリソースの形）。
ロールの供給源はトークンの claim ではなく `user_roles` を引く本経路とする。claim は発行時点のスナップショットのため
`user_roles` の更新と乖離するうえ、`docs/design/auth.md` § 1 が invariant として置く「Standard core only」
「Byte-equal contract」を崩すため採らない。

```yaml
input:
  - authn: "*auth.Authn"       # 認証主体。nil は Unauthenticated（401）。UserID() を Repository へ渡す

output:
  struct: RolesView            # package role
  fields:
    - name: Roles
      type: "[]RoleView"       # { Code string; Name string }。割り当てが 0 件でも空スライスを返しエラーにしない

dependencies:
  - user.RoleRepository        # FindRolesByUserID（認可判定と同じ権威ある user_roles を引く）
  - observability.TracerFactory

workflow:
  tx_required: false               # read-only
  steps:
    - authn == nil なら ErrUnauthenticated（401）
    - authn.UserID() を取得（未解決はエラー伝播）
    - roleRepo.FindRolesByUserID(userID) で割り当て済みの全ロールを取得
    - RoleCode を外部向けの安定コード（admin / general）へ写像し RolesView を構築
  errors:
    - ErrUnauthenticated → 401（Authn 不在）
```

ワイヤには UUID や表示名ではなく安定コードを出す。表示名は変更され得るため、クライアントの分岐を名称一致に
依存させると表示の都合が権限判定を壊す。ロールの追加は `code` の enum 拡張として現れる。

[ADR-0036 (ordered-pessimistic-row-locks)]: ../../adr/0036-ordered-pessimistic-row-locks.md
