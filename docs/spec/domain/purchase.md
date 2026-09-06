# Purchase — Domain Spec

> `POST /v1/purchases`（購入作成・原子性/売り越し禁止）のドメイン spec。
> 決済通貨は USD のみで、決済スケールの金額はすべて USD セント単位の整数で保持・計算する（float 不使用）。
> 単価は価格スケール（サブセント可の decimal）で保持し、決済スケールへの変換は最小単位 2 桁（セント）で
> **切り捨て**、ドメイン内 1 箇所に集約する。2 スケールの型分けそのものは [ADR-0038 (two-scale-quantity-model)]。
> `referenceAmount`（JPY 参考換算）はドメインの関心ではなく usecase 層の関心であり、切り捨てではなく
> half-up で丸める（非永続の参考表示のため。[`docs/spec/usecase/exchangerate.md`](../usecase/exchangerate.md)）。

## Overview

購入集約（Purchase）は、購入コード・購入者・初期ステータス・金額（小計 / 税 / 送料 / 合計）と明細（PurchaseDetail）を保持するドメイン集約。生成時に「明細が 1 件以上」「同一 productID の重複なし」「数量が 1 以上」「要求数量がロック済み在庫以下（売り越し禁止）」の不変条件を検証し、違反する `Purchase` は構築できない。

明細の `unitPrice` は在庫ロック取得直後の `products.price` スナップショットであり、購入成立後の価格改定に不変（価格の一貫性）。金額計算（`subtotal = Σ unitPrice × quantity` / `tax = subtotal × taxRate` / `shippingFee = 定数` / `total = subtotal + tax + shippingFee`）はドメイン内で完結し、税・送料の丸めは切り捨てで 1 箇所に集約する。

初期ステータスは「未処理」（`purchase_statuses.code = 1`）。ドメインは code（安定した業務キー）を定数として持ち、`purchase_statuses` の UUID は焼き込まない（seed との二重管理を避けるため、UUID 解決は永続化時の infra 責務）。`id` / `code` は UUIDv7（[ADR-0037 (uuidv7-identifiers)]）で、生成は usecase 層が行いドメインへ渡す（ドメインは乱数・時刻に直接依存しない）。

書き込み後のドメイン整合の再検証とレスポンス組み立ての取得元として、`Repository.FindByID` で永続化済みの購入を明細込みで読み出す（[ADR-0032 (lightweight-cqrs)] / [ADR-0034 (commandservice-atomicity-criterion)]）。

## Entity

```yaml
package: internal/domain/purchase
struct: Purchase
constructors:
  - name: New            # 新規作成（金額計算・売り越し検証・未処理ステータス）
  - name: Reconstruct    # 永続化済みの再構築（Repository の読み出し / 再検証）
fields:
  - name: id
    type: uuid.UUID
    required: true          # IsNil の場合は ErrInvalidID
  - name: code
    type: string            # UUIDv7 文字列。空の場合は ErrInvalidCode
    required: true
  - name: userID
    type: uuid.UUID
    required: true          # IsNil の場合は ErrInvalidUserID
  - name: statusID
    type: uuid.UUID         # New ではゼロ値（DB が code から解決）。Reconstruct で設定
  - name: subtotalAmount
    type: int               # USD セント。負値は ErrInvalidAmount（Reconstruct）
  - name: taxAmount
    type: int               # USD セント（切り捨て）
  - name: shippingFee
    type: int               # USD セント（固定送料定数）
  - name: totalAmount
    type: int               # USD セント
  - name: details
    type: "[]PurchaseDetail" # 空は ErrEmptyDetails
  - name: orderedAt
    type: time.Time         # New ではゼロ値（DB 既定 NOW()）。Reconstruct で設定
  - name: status
    type: Status            # 現在状態（status_id を JOIN で解決した業務キーから NewStatus で構築）。状態機械の source of truth
                            # StatusCode() は status.Code() の導出
  - name: paidAt
    type: "*time.Time"      # 支払い日時（未支払いは nil）。支払い後も残る sticky な監査記録
  - name: canceledAt
    type: "*time.Time"      # キャンセル日時（未キャンセルは nil）。監査記録
  - name: shippedAt
    type: "*time.Time"      # 発送日時（未発送は nil）。キャンセル可否の timestamps ガード対象
  - name: deliveredAt
    type: "*time.Time"      # 配達日時（未配達は nil）。キャンセル可否の timestamps ガード対象
```

```yaml
struct: PurchaseDetail   # 値オブジェクト
fields:
  - name: id
    type: uuid.UUID
  - name: productID
    type: uuid.UUID
  - name: quantity
    type: int
  - name: unitPrice
    type: int             # 購入時点の単価スナップショット（USD セント）
```

```yaml
struct: LockedProduct    # 在庫ロック取得直後の商品スナップショット（New の入力）
fields:
  - name: id
    type: uuid.UUID
  - name: price
    type: int
  - name: quantity
    type: int             # ロック時点の在庫数
```

## Cross-field Invariants

- **クーポンの適用と値引きは常に一致する。** `couponID != nil` と `discountAmount > 0` は双条件で、
  違反は `ErrZeroDiscount` → 422（`details` に `couponId`）。片方だけの状態を作ると「クーポンは使ったが
  説明が付かない」控えが生まれ、表現層が値引き行を出すかどうかを金額だけで決められなくなる。
  `New` / `Reconstruct` / `ApplyCoupon` が共有する検証ゲートで課す。

- **値引きは小計を超えない。** 超えると請求額が負になる。違反は `ErrInvalidAmount`。
  定額クーポンが対象小計を超える場合はクーポン側が対象小計まで切り詰めるため、通常経路では起きない。

- **課税の基礎は値引き後の額。** `tax = (subtotal − discount) × 10%`、
  `total = subtotal − discount + tax + shipping`。値引いた分にまで課税しないためで、規則の所在は
  `settle` 1 箇所。`subtotalAmount` は値引き前のまま据え置く。

- 明細は 1 件以上（空は `ErrEmptyDetails` → 422）。
- 明細内の `productID` は一意（重複は `ErrDuplicateProductID` → 422。在庫行のロック順序固定の前提を守る）。
- 各明細の `quantity >= 1`（違反は `ErrInvalidQuantity` → 422）。
- 各明細の `productID` に対応するロック済み商品が存在すること（欠落は `ErrProductNotFound` → 422）。
- 各明細の `quantity <= ロック済み在庫`（超過は `ErrInsufficientStock` → 409）。
- `unitPrice` は対応するロック済み商品の `price` スナップショット。
- `subtotal = Σ unitPrice × quantity` / `tax = subtotal × taxRatePercent / 100`（切り捨て）/ `total = subtotal + tax + shippingFee`。
- `subtotal` は決済スケールの整数幅に収まり、かつ **税と送料を加えても収まる**こと（超過は `ErrInvalidAmount`）。
  単価 1 件が幅に収まることは `money.Price` の構築時に保証されるため、超えるのは明細が積み上がった結果に限られる。
  整数演算は溢れてもエラーを返さないため、算術に入る前に拒まなければ壊れた合計がそのまま購入として確定する。

## Behavior Methods

```yaml
- name: Cancel
  signature: Cancel(now time.Time) (Event, error)
  behavior: |
    購入をキャンセル状態へ遷移させる（PATCH /v1/purchases/{purchaseId}/cancel の状態遷移）。
    遷移可否は statusCode の許可遷移表を一次判定とし、status enum に現れないイベント既発生は timestamps ガードで補完する:
      - 既にキャンセル（statusCode == 6 または canceledAt != nil）→ ErrAlreadyCanceled（409）
      - 完了（statusCode == 5）・配達済み（statusCode == 9）・発送済み（shippedAt != nil）→ ErrCancelNotAllowed（409）
      - それ以外（未処理 / 受付中 / 確認中 / 処理中 / 支払い済み）→ 許可
    許可時は statusCode をキャンセル（6）へ、canceledAt を now へ同時にセットする（status と timestamp の同時セット・
    既存 timestamps は不変という不変条件）。now は時刻境界（clock）から供給し、ドメインは時刻へ直接依存しない。
    遷移に成功したときだけキャンセルの事実（Event）を返す。
  invariants:
    - status（statusCode）と対応 timestamp（canceledAt）は必ず同時セット
    - 既にセット済みの timestamps は不変（NULL → 値 の単調セットのみ）

- name: Pay
  signature: Pay(now time.Time) (Event, error)
  behavior: |
    購入を支払い済み状態へ遷移させる（PATCH /v1/purchases/{purchaseId}/pay の状態遷移・擬似決済）。
    遷移可否は statusCode を一次判定とし、status enum に現れないイベント既発生は timestamps ガードで補完する:
      - 既に支払い済み（statusCode == 7）→ ErrAlreadyPaid（409。二重支払い）
      - 処理中（statusCode == 4）・キャンセル済み（statusCode == 6）・完了（statusCode == 5）・配達済み（statusCode == 9）・発送済み（shippedAt != nil）→ ErrPayNotAllowed（409）
      - 未払い相当でも paidAt != nil → ErrAlreadyPaid（409。受付中 / 確認中 へはアプリから遷移せず、
        これらは外部運用が書く行であるため、支払い済みの事実が status ではなく paidAt にだけ現れることがある）
      - それ以外（未処理 / 受付中 / 確認中）→ 許可
    許可時は statusCode を支払い済み（7）へ、paidAt を now へ同時にセットする。now は時刻境界（clock）から供給する。
    決済 SDK / PSP 連携・金額検証は行わず、paidAt とステータスの記録のみを担う。
    遷移に成功したときだけ支払いの事実（Event）を返す。
  invariants:
    - 支払い済み status（statusCode == 7）は paidAt を必須とする（一方向。paidAt は支払い後も残り、以降の遷移で status が変わっても保持されるためキャンセルのような双条件にはしない）
    - 既にセット済みの paidAt は不変（NULL → 値 の単調セットのみ）。上書きは記録済みの支払い日時を失わせるため、遷移そのものを拒否して守る

- name: Ship
  signature: Ship(now time.Time) (Event, error)
  behavior: |
    購入を発送済み状態へ遷移させる（PATCH /v1/purchases/{purchaseId}/ship の状態遷移）。遷移可否は statusCode の等値比較のみで判定する:
      - 既に発送済み（statusCode == 8）→ ErrAlreadyShipped（409。二重発送）
      - 支払い済み（statusCode == 7）→ 許可
      - それ以外（未払い相当 / 処理中 / 完了 / キャンセル済み / 配達済み）→ ErrShipNotAllowed（409）
    許可時は statusCode を発送済み（8）へ、shippedAt を now へ同時にセットする。now は時刻境界（clock）から供給する。
    配送追跡（追跡番号 / 配送業者 / 追跡 URL）は扱わず、shippedAt とステータスの記録のみを担う。
    遷移に成功したときだけ発送の事実（Event）を返す。
    Cancel / Pay と違い timestamps ガードを併用しないのは、遷移元が支払い済みの 1 状態に限られ statusCode の等値比較だけで
    判別できるため（配達済みは statusCode != 7 として不正遷移側に落ちる）。
  invariants:
    - 発送済み status（statusCode == 8）は shippedAt を必須とする（一方向。shippedAt は発送後も残り、以降の配達済み等の遷移で status が変わっても保持されるためキャンセルのような双条件にはしない）

- name: Deliver
  signature: Deliver(now time.Time) (Event, error)
  behavior: |
    購入を配達済み状態へ遷移させる（PATCH /v1/purchases/{purchaseId}/deliver の状態遷移）。遷移可否は statusCode の等値比較のみで判定する:
      - 既に配達済み（statusCode == 9）→ ErrAlreadyDelivered（409。二重配達）
      - 発送済み（statusCode == 8）→ 許可
      - それ以外（未払い相当 / 処理中 / 支払い済み / 完了 / キャンセル済み）→ ErrDeliverNotAllowed（409）
    許可時は statusCode を配達済み（9）へ、deliveredAt を now へ同時にセットする。now は時刻境界（clock）から供給する。
    配達確認の証跡（署名 / 受領写真 / GPS 位置）は扱わず、deliveredAt とステータスの記録のみを担う。
    遷移に成功したときだけ配達の事実（Event）を返す。
    Ship と同じく timestamps ガードを併用しないのは、遷移元が発送済みの 1 状態に限られ statusCode の等値比較だけで判別できるため。
  invariants:
    - 配達済み status（statusCode == 9）と deliveredAt は必ず同時セット（双条件。配達済みは終端状態であり配達後に別 status へ遷移して deliveredAt だけが残ることがないため、paidAt / shippedAt のような一方向にはしない）

- name: IsShippable
  signature: IsShippable() bool
  behavior: |
    購入が発送可能な状態にあるかを返す（「発送可能」の定義そのもの）。発送可能＝支払い済み（statusCode == 7）だが、
    その条件は Status.CanTransitionTo(StatusShipped) が既に持っているため、条件を書き下さずそこから導出する。
    これにより発送可能なステータスが増減しても定義は 1 箇所のままになる。
    発送待ちを絞り込む読み取り経路（FindShippable の SQL）はこの述語の実行形であって定義ではない。片方だけを
    変更してはならず、Usecase は取得した行をこの述語で検証して乖離をエラーにする。
  invariants:
    - 業務語彙で名前を持つ条件はドメインに述語として存在する（docs/rules.md § Domain Layer Constraints）
```

## Domain Service

集合についての問いはどの Purchase 1 件のメソッドにもなり得ないため、購入集約の外の
[`internal/domain/service/dispatch`](../../../internal/domain/service/dispatch) が持つ。
語るのは購入集約だけで、単一集約に閉じたドメインサービスである（[`internal/domain/README.md`](../../../internal/domain/README.md)
§ Where a Domain Service lives）。

```yaml
- name: GroupForDispatch
  signature: GroupForDispatch(purchases Purchases) []Purchases
  behavior: |
    発送待ちの購入を、まとめて発送してよい組に分ける（まとめ発送）。まとめる軸は **同一の購入者** のみで、
    同じ購入者宛ての購入が 1 つの組になる。購入が 1 件だけの購入者もその 1 件からなる組になる。
    結果は入力の並び順に依存せず一意に定まる。組の中は注文日時の古い順（同時刻は購入 ID の昇順）、
    組同士はその組の最も古い購入の同じ順序で並ぶ。purchases が空なら空を返す。
    受け取るのは発送可能な購入の集合であることを前提とし、発送可能性はここでは検証しない
    （定義は Purchase.IsShippable が持ち、取得した行との突き合わせは Usecase が行う。定義を二重に書かない）。
    純粋な導出であり拒否の判定ではないため error を返さない。状態を持たず I/O も context.Context も持たない。
  invariants:
    - まとめの軸は購入者のみ。同梱可能期間などの閾値条件は現時点では持たない（placeholder 定数を増やさないため。
      実要件が立った時点でこの spec に足す）
    - 組は算出結果であり永続化しない。したがって組そのものの識別子を持たない
```

## Value Objects

- `PurchaseDetail`（明細）/ `LockedProduct`（ロック済み在庫スナップショット・New の入力）。上記 Entity 参照。
- `Detail`（購入 1 件の詳細読み取りモデル・read 側）。書き込み集約 Purchase とは別型で、ステータスの業務キーと名称
  （`StatusCode` / `StatusName`）を購入ステータスマスタとの JOIN で解決済み、`PaidAt` は支払い日時（未支払いは nil）、`CanceledAt` はキャンセル日時
  （未キャンセルは nil）、`ShippedAt` は発送日時（未発送は nil）、`DeliveredAt` は配達日時（未配達は nil）。`FindDetailByID` の返り値。

## Repository Methods

```yaml
- name: Create
  signature: Create(ctx context.Context, p *Purchase) error
  behavior: |
    購入を新規登録する。集約が保持する明細（PurchaseDetail）も同じ tx 内で併せて登録する。
    在庫の減算は商品集約の書き込みであり、呼び出し側が同一 tx 内で product.Repository を通して行う
    （対象行を identity で名指しできるため分解でき、CommandService は要さない
    ／[ADR-0034 (commandservice-atomicity-criterion)] の判定軸）。購入コードの重複は Conflict。
- name: FindByID
  signature: FindByID(ctx context.Context, id uuid.UUID) (*Purchase, error)
  behavior: |
    ID から購入を明細込みで取得し Reconstruct で再構築する。存在しない場合は NotFound。
    書き込み後のドメイン整合の再検証とレスポンスの取得元に用いる。
- name: LockByCode
  signature: LockByCode(ctx context.Context, code string) (*Purchase, error)
  behavior: |
    購入コードから購入を購入行のみ悲観ロック（SELECT FOR UPDATE OF p）して明細込みで再構築する。存在しない場合は NotFound。
    公開識別子が code であるため、外部から指定された購入を引く入口はここに揃える（内部再読込は FindByID / FindDetailByID）。
    支払いの状態遷移の競合（同一購入への並行支払い）を購入行ロックで直列化する。擬似決済は単一集約書き込みのため
    Repository が担う（[ADR-0034 (commandservice-atomicity-criterion)] の判定軸）。
- name: UpdatePaid
  signature: UpdatePaid(ctx context.Context, p *Purchase) error
  behavior: |
    購入の状態更新（status_id は code から解決 / paid_at）を渡された ctx の tx 内で実行する。擬似決済のため
    単一集約（purchases）のみを更新し在庫操作は伴わない。対象行は LockByCode で取得・検証済み（遷移可否ガードは付けない）。
- name: UpdateShipped
  signature: UpdateShipped(ctx context.Context, p *Purchase) error
  behavior: |
    購入の状態更新（status_id は code から解決 / shipped_at）を渡された ctx の tx 内で実行する。配送追跡を扱わないため
    単一集約（purchases）のみを更新し在庫操作は伴わない。対象行は LockByCode で取得・検証済み（遷移可否ガードは付けない）。
- name: UpdateDelivered
  signature: UpdateDelivered(ctx context.Context, p *Purchase) error
  behavior: |
    購入の状態更新（status_id は code から解決 / delivered_at）を渡された ctx の tx 内で実行する。配達確認の証跡を扱わないため
    単一集約（purchases）のみを更新し在庫操作は伴わない。対象行は LockByCode で取得・検証済み（遷移可否ガードは付けない）。
- name: UpdateCancelled
  signature: UpdateCancelled(ctx context.Context, p *Purchase) error
  behavior: |
    購入の状態更新（status_id は code から解決）を渡された ctx の tx 内で実行する。対象行は LockByCode で
    取得・検証済み（遷移可否ガードは付けない）。在庫の復元は商品集約の書き込みであり、Create と同じく
    呼び出し側が同一 tx 内で product.Repository を通して行う。
- name: FindDetailByID
  signature: FindDetailByID(ctx context.Context, id uuid.UUID) (*Detail, error)
  behavior: |
    ID から購入詳細（読み取りモデル Detail）を明細込みで取得する。ステータス名は購入ステータスマスタとの
    JOIN で解決する（購入ステータスは購入集約に属する固定参照マスタで、[ADR-0032 (lightweight-cqrs)] の
    子参照マスタ例外により単一集約の Repository read）。存在しない場合は NotFound。キャンセル後の
    状態名解決・レスポンスの取得元に用いる（GET 詳細 #569 でも再利用可能）。
- name: FindStatusesByUserID
  signature: FindStatusesByUserID(ctx context.Context, userID uuid.UUID) ([]Status, error)
  behavior: |
    指定ユーザーの購入が取っているステータスを重複なく返す（退会の可否判定の取得元）。進行中かどうかでは
    絞り込まず、その判定（Status.IsTerminal の否定）は呼び出し側が行う。重複を除くため行数はステータスの
    種類数で頭打ちになる。購入を 1 件も持たない場合は空を返し、順序は保証しない。ステータスコードは
    購入ステータスマスタとの JOIN で解決する（FindDetailByID と同じ子参照マスタ例外により単一集約の
    Repository read）。
- name: FindStatusesByProductID
  signature: FindStatusesByProductID(ctx context.Context, productID uuid.UUID) ([]Status, error)
  behavior: |
    指定商品を明細に持つ購入が取っているステータスを重複なく返す（廃番の可否判定の取得元）。
    FindStatusesByUserID と対称で、絞り込む軸が購入者から商品へ変わるだけである。進行中かどうかでは
    絞り込まず、その判定（Status.IsTerminal の否定）は呼び出し側が行う。該当する購入が 1 件も無い場合は
    空を返し、順序は保証しない。
- name: FindShippable
  signature: FindShippable(ctx context.Context, limit int32) (Purchases, error)
  behavior: |
    発送可能な購入を注文日時の古い順（同時刻は ID 昇順）で明細込みに最大 limit 件返す（まとめ発送一覧の取得元）。
    該当が無ければ空を返す。cursor ページングは持たない top-N で、limit の既定値適用とクランプは Usecase が担う。
    「発送可能」を定義するのは Purchase.IsShippable であり、絞り込みの WHERE はその実行形である。片方だけを
    変更してはならず、Usecase は返却行を同じ述語で検証する。ステータスは購入ステータスマスタとの JOIN で
    code を解決し（seed UUID を焼き込まない）、支払い済みを表す code は infra がドメイン定数から渡す。
    明細は購入 1 件ずつではなく取得した購入 ID をまとめて 1 クエリで引く（件数分の往復を避けるため）。
- name: FindUserIDsWithPurchases
  signature: FindUserIDsWithPurchases(ctx context.Context, userIDs []uuid.UUID) ([]uuid.UUID, error)
  behavior: |
    与えたユーザー ID のうち、購入を 1 件以上持つものを返す（退会後の物理削除でユーザーを
    残すかの判定の取得元）。ステータスは問わず、順序は保証しない。userIDs が空なら空を返す。
    ユーザーは独立集約のため users とは結合せず、ID 群の照会として切り出す。
```

なお、状態遷移に伴う購入行の悲観ロック（`FOR UPDATE`）は、書き込みが集約を跨ぐかで担い手が分かれる（[ADR-0034 (commandservice-atomicity-criterion)] の判定軸）:

- **キャンセル**は在庫復元（`products`）+ 購入更新（`purchases`）の複数集約への書き込みだが、対象行を identity で名指しできるため分解できる。商品を `product.Repository.LockByIDs` でロックしてから `product.AdjustStock` で戻し、購入は `purchase.Repository`（`LockByCode` / `UpdateCancelled`）が担う（判定軸は [ADR-0034 (commandservice-atomicity-criterion)]）。
- **支払い**は `purchases` の status/paid_at のみの**単一集約書き込み**（在庫操作なし）で、**Repository（`LockByCode` / `UpdatePaid`）**が担う。行ロックは並行制御（二重支払い防止）であって集約横断の原子性ではない。
- **発送**も `purchases` の status/shipped_at のみの単一集約書き込みのため、支払いと同じく **Repository（`LockByCode` / `UpdateShipped`）**が担う。
- **配達完了**も `purchases` の status/delivered_at のみの単一集約書き込みのため、同じく **Repository（`LockByCode` / `UpdateDelivered`）**が担う。

## Notes

- 定数（`taxRatePercent` / `shippingFeeCents` は sample の placeholder。実要件が立った時点で config / マスタへ移す。
  送料を 0 ではなく固定の非 0 値にしているのは、`shipping_fee` 列・`total` の計算経路・レスポンス項目を
  実際に通すため）: `statusCodeUnprocessed = 1` / `statusCodeAccepted = 2` / `statusCodeConfirming = 3` / `statusCodeProcessing = 4` / `statusCodeCompleted = 5` / `statusCodeCanceled = 6` / `statusCodePaid = 7` / `statusCodeShipped = 8` / `statusCodeDelivered = 9` / `taxRatePercent = 10` / `shippingFeeCents = 500`。ステータス UUID は焼き込まず、code から infra で解決する（支払い済み=7 / 発送済み=8 / 配達済み=9 を含め、購入ステータスマスタで定義）。code 値は状態の到達順序を意味しないため、遷移判定は等値比較のみで行う。
- エラー写像: `ErrInsufficientStock` / `ErrAlreadyCanceled` / `ErrCancelNotAllowed` / `ErrAlreadyPaid` / `ErrPayNotAllowed` / `ErrAlreadyShipped` / `ErrShipNotAllowed` / `ErrAlreadyDelivered` / `ErrDeliverNotAllowed` → `apperror.ErrConflict`（409）、その他検証系 → `apperror.ErrValidation`（422）。

[ADR-0032 (lightweight-cqrs)]: ../../adr/0032-lightweight-cqrs.md
[ADR-0034 (commandservice-atomicity-criterion)]: ../../adr/0034-commandservice-atomicity-criterion.md
[ADR-0037 (uuidv7-identifiers)]: ../../adr/0037-uuidv7-identifiers.md
[ADR-0038 (two-scale-quantity-model)]: ../../adr/0038-two-scale-quantity-model.md
