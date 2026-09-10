# Coupon — Usecase Spec

> クーポンのユースケース spec。読み取り（保有一覧・いまのカートに使えるもの）と、受給者を名指しする
> 発行、不特定多数への販促一括発行を持つ。廃番に伴う発行は廃番ジャーニーの副作用として起き（[`product.md`](product.md) の廃番）、
> 引き換えは購入確定の中で行う（[`purchase.md`](purchase.md) の CreatePurchase）。

## Overview

**受給者が述語でしか決まらない発行は、いずれも CommandService を通す。** usecase が集約を 1 件ずつ
組み立てて Repository へ渡す形に分解できないため。ただし CommandService へ至る理由は 2 本あり、
この spec が持つ 2 つの一括発行はそれぞれ別の理由で到達する。

- **廃番に伴う発行**は、商品の書き込みと同一トランザクションでなければならない
  （branch 3a。[`product.md`](product.md) が持つ）
- **販促一括発行**は、束ねる相手を持たない。`coupons` だけを書き、原子性の要求も無い。それでも
  受給者を識別子で名指しできないため分解できない
  （branch 3b。[ADR-0114](../../adr/0114-predicate-defined-set-writes-on-commandservice.md)）

**受給者を名指しできる発行はこの限りではない** — 名指しできる行は分解できるので Repository に載る。
分けているのは発行枚数ではなく、行を名指しできるかどうかである。

名指しの発行と販促一括発行は authz の Action も分ける（`ActionCouponIssue` / `ActionCouponBulkIssue`）。
1 枚を受給者へ配る操作と不特定多数へ配る操作は影響範囲が桁で異なり、運用上どちらか一方だけを許可
できる必要があるため、実装経路の分岐（Repository / CommandService）とは独立に権限も分けている。

読み取りは 2 つ。保有一覧は「持っているもの」を並べ、使えるかどうかで絞らない。もう 1 つは
「いまのカートに使えるもの」で、使えるかどうかと値引き額を返す。

**後者は集約をまたぐ読みだが QueryService に置かない。** 適用範囲の判定（`Scope.Covers`）と値引き額の
計算（`Discount.Apply` / `Coupon.DiscountFor`）はドメインロジックであり、`docs/rules.md` の
Repository / QueryService Rules が QueryService へ書くことを禁じている。SQL の結合で値引き額を出すと
業務条件の著作権が infra へ移るため、Repository を束ねて usecase で結合し、判定はドメインへ渡す
（`cart.GetCart` が同じ形の先例）。

**コード配布（引き取り型）は本 spec の範囲外である。** 誰が受け取るかが事前に決まらない配布は
[`campaign.md`](campaign.md) が持ち、キャンペーンのテンプレートからクーポンが 1 枚生まれる。
両者はクーポン集約を共有するが経路は別で、押し出し型がキャンペーンを経由することはない。

## Interface

```yaml
package: internal/usecase/coupon
interface: Usecase
methods:
  - name: ListMyCoupons
    signature: ListMyCoupons(ctx, authn) ([]CouponView, error)
  - name: ListApplicableToMyCart
    signature: ListApplicableToMyCart(ctx, authn) ([]CartCouponView, error)
  - name: IssueCoupon
    signature: IssueCoupon(ctx, authn, params IssueCouponParams) (CouponView, error)
  - name: IssuePromotionalCoupons
    signature: IssuePromotionalCoupons(ctx, authn, params IssuePromotionalCouponsParams) (IssuePromotionalCouponsView, error)
```

## DTOs

```yaml
output:
  struct: CouponView
  fields:
    - name: ID
      type: uuid.UUID
    - name: DiscountKind
      type: string            # 値引きの決まり方の名前（code ではない）
    - name: DiscountValue
      type: decimal.Decimal   # 定額なら金額、定率なら率
    - name: ScopeKind
      type: string            # 適用範囲の決まり方の名前
    - name: ScopeTargetID
      type: "*uuid.UUID"      # 全体では nil
    - name: ExpiresAt
      type: time.Time
    - name: UsedAt
      type: "*time.Time"      # 未使用は nil
    - name: IssuedAt
      type: time.Time

output:
  struct: CartCouponView
  fields:
    - name: Coupon
      type: CouponView
    - name: DiscountAmount
      type: int               # 適用した場合に差し引かれる額（USD セント）

input:
  struct: IssueCouponParams
  fields:
    - name: UserID
      type: uuid.UUID         # 受給者。在籍する利用者に限る
    - name: DiscountKind
      type: string            # 値引きの決まり方の名前（code ではない）
    - name: DiscountValue
      type: decimal.Decimal   # 定額なら金額、定率なら率
    - name: ScopeKind
      type: string            # 適用範囲の決まり方の名前
    - name: ScopeTargetID
      type: "*uuid.UUID"      # 全体では nil
    - name: ExpiresAt
      type: time.Time         # 絶対時刻。締切を名指しする

input:
  struct: IssuePromotionalCouponsParams
  fields:
    - name: DiscountKind
      type: string            # 値引きの決まり方の名前（code ではない）
    - name: DiscountValue
      type: decimal.Decimal   # 定額なら金額、定率なら率
    - name: ScopeKind
      type: string            # 適用範囲の決まり方の名前
    - name: ScopeTargetID
      type: "*uuid.UUID"      # 全体では nil
    - name: ExpiresAt
      type: time.Time         # 絶対時刻。販促の終了日を名指しする

output:
  struct: IssuePromotionalCouponsView
  fields:
    - name: IssuedAt
      type: time.Time
    - name: ExpiresAt
      type: time.Time
    - name: RecipientCount
      type: int64             # 述語に当たった利用者数
    - name: IssuedCouponCount
      type: int64             # 実際に挿入された枚数。RecipientCount と一致する
```

## Dependencies

```yaml
- coupon.Repository    # FindByUserID / Create（名指しの発行）
- cart.Repository      # FindByOwnerID（対象明細の母集団）
- product.Repository   # FindByIDs（単価と商品カテゴリの解決）/ FindByID（適用範囲の対象確認）
- category.Repository  # FindByID（適用範囲の対象確認）
- user.Repository      # FindByID（受給者の在籍確認）/ CountByActive（発行枚数の上限判定）
- clock.Clock          # 失効判定の現在時刻 / 発行日時
- tx.Manager           # 発行のトランザクション境界
- authz.Authorizer     # ActionCouponIssue / ActionCouponBulkIssue
- command.CommandService  # internal/usecase/coupon/command（一括発行）
```

## Workflow

```yaml
- name: ListMyCoupons
  tx_required: false
  behavior: |
    認証主体の保有クーポンを発行日時の新しい順で返す。使用済み・失効済みも並べる。
    種別は code ではなく名前で出す。1 枚も持たない場合は空を返す。
  errors:
    - 未認証: 401

- name: ListApplicableToMyCart
  tx_required: false
  calls:
    - coupon.Repository.FindByUserID
    - cart.Repository.FindByOwnerID
    - product.Repository.FindByIDs
    - cart.CartItem.Evaluate      # 購入できる明細だけを対象にする
    - coupon.Coupon.DiscountFor   # 値引き額の算出（丸めはここ 1 箇所）
  behavior: |
    認証主体のカートに対して使えるクーポンと、それぞれの値引き額を返す。
    使用済み・失効済みと、値引きが 0 になるクーポンは並べない。

    対象にするのはいま購入できる明細だけ。カートの再評価が issue を立てた明細は購入へ進めないため、
    値引きの対象にもしない。クーポンを 1 枚も持たない場合はカートを引かずに空を返す。
    カートを持たない場合も空を返す。
  invariants:
    - 値引き額は購入確定と同じ規則（Coupon.DiscountFor）で決まる
    - ロックを取らないため、返した値は返した瞬間から古くなる
  errors:
    - 未認証: 401

- name: IssueCoupon
  tx_required: true
  calls:
    - authz.Authorizer.Authorize          # ActionCouponIssue（admin のみ）
    - coupon.NewDiscountKindByName / coupon.NewDiscount
    - coupon.NewScopeKindByName / coupon.NewScope
    - clock.Clock.Now
    - category.Repository.FindByID        # 適用範囲がカテゴリのときだけ（トランザクション内）
    - product.Repository.FindByID         # 適用範囲が商品のときだけ（トランザクション内）
    - user.Repository.FindByID            # 受給者の在籍確認（トランザクション内）
    - coupon.New
    - coupon.Repository.Create
  behavior: |
    admin が受給者を名指しして、クーポンを 1 枚発行する。値引き・適用範囲・有効期限を受け取り、
    発行したクーポンを返す。発行直後は未使用。

    最低購入金額・値引き上限・利用開始日時は任意で、省略すると条件を持たないクーポンになる。

    値引きと適用範囲は、名前の解決も値の検証もドメインへ委ねる（一括発行と同じ入口を使う）。
    適用範囲がカテゴリ・商品を指す場合はその存在を確認する。存在しない対象を範囲にしたクーポンは
    誰にも使えないため。

    書くのは coupons の 1 行だけで、受給者を識別子で名指しできる。load-mutate-save に分解できるので
    `docs/design/data-access-pattern.md` §4 Gate 1 により Repository へ載り、CommandService へ落ちる
    branch 3a / 3b のいずれにも当たらない。
  invariants:
    - 退会済みの利用者は受給者にならない。user.Repository.FindByID は在籍者だけを返すため、
      不存在と退会済みは同じ NotFound になる
    - 在籍行はロックしない（ADR-0034 の分岐 1）。在籍条件が判定後に失効しても、発行済みクーポンは
      害を生まない — 退会は保有クーポンを見ず、購入時は在籍ガードが働き、物理削除では従属行ごと消える
    - 受給者と適用範囲の対象の存在確認は書き込みと同一トランザクションで行う。Idempotency-Key の
      有無で外側のトランザクションが開くかどうかが変わるため、外に置くと境界が要求次第になる
    - 一括発行が持つ件数・定額の上限は適用しない。母集団を持たず、影響範囲が受給者 1 人に閉じるため
    - Idempotency-Key は必須。発行を取り消す経路が無く、再実行しても状態が収束せず、対象の状態が
      二度目を弾くこともないため、再送を安全にする手段がキーしかない（一括発行と同じ理由）
  errors:
    - 未認証: 401
    - admin 以外: 403
    - 受給者が存在しない、または退会済み: 404
    - 適用範囲が指すカテゴリ・商品が存在しない: 404
    - 値引き・適用範囲・有効期限がドメインの検証に落ちる: 422

- name: IssuePromotionalCoupons
  tx_required: true
  calls:
    - authz.Authorizer.Authorize          # ActionCouponBulkIssue（admin のみ）
    - coupon.NewFlatDiscount / NewRateDiscount
    - coupon.NewAllScope / NewCategoryScope / NewProductScope
    - clock.Clock.Now
    - category.Repository.FindByID        # 適用範囲がカテゴリのときだけ（トランザクション内）
    - product.Repository.FindByID         # 適用範囲が商品のときだけ（トランザクション内）
    - user.Repository.CountByActive        # 上限判定（書き込み前）
    - command.CommandService.IssuePromotionalCoupons
  behavior: |
    退会していない全ユーザーへ、同一条件のクーポンを 1 枚ずつ発行する。値引き・適用範囲・有効期限を
    受け取り、発行日時と発行枚数を返す。

    値引きと適用範囲は、名前の解決も値の検証もドメインへ委ねてから CommandService へ渡す
    （閉じた集合の権威は `allDiscountKinds` / `allScopeKinds` ただ 1 つ）。適用範囲が
    カテゴリ・商品を指す場合はその存在を確認する。存在しない対象を範囲にしたクーポンは誰にも使えず、
    発行してから気づくことになるため。

    受給者は述語（退会の除外）でしか決まらず件数に上限も無いため、usecase が集約を組み立てて
    Repository へ渡す形に分解できない。判定は [ADR-0114](../../adr/0114-predicate-defined-set-writes-on-commandservice.md)
    の branch 3b で、原子性ではなく「行を名指しできない」ことが理由。書くのは coupons だけで、
    集約境界は広がらない。
  invariants:
    - 退会済みユーザーは受給者にならない（廃番の一括発行と同一の述語）
    - 発行枚数の上限は application policy であり domain invariant ではない。CommandService の SQL では
      強制しない（domain invariant から導出されない条件を CommandService で強制しないため）
    - 上限は書き込み前の件数で判定し、トランザクション内の事後検証で確定する。users 行はロックしないため、
      判定と挿入の間に登録された利用者のぶんだけ超過しうる
    - 有効期限の検証はクーポンの構築時（coupon.New）にのみ働く。受給者が 0 人だと 1 枚も構築されない
      ため検証へ届かず、発行時点より前の有効期限でも 200 を返す。ただしその場合も 1 枚も作られない
    - 配布の記録を持たない。同じ配布を二度走らせない責務は呼び出し側の Idempotency-Key にある
    - 往復は件数取得・受給者取得・挿入の 3 回（適用範囲が対象を持つ場合はその確認で 1 回増える）で、
      発行枚数には比例しない
    - 適用範囲の対象確認は書き込みと同一トランザクションで行う。Idempotency-Key の有無で外側の
      トランザクションが開くかどうかが変わるため、外に置くと境界が要求次第になる
    - 定額の値引きには上限がある。定率はドメインが 1 を上限に持つが、定額は際限なく大きくなり得るため
  errors:
    - 未認証: 401
    - admin 以外: 403
    - 値引き・適用範囲がドメインの検証に落ちる: 422
    - 定額の値引きが上限を超える: 422
    - 適用範囲が指すカテゴリ・商品が存在しない: 404
    - 受給者数が上限を超える: 409
```

## Command Service

```yaml
- name: IssueDiscontinuationCoupons
  package: internal/usecase/product/command
  signature: IssueDiscontinuationCoupons(ctx context.Context, params IssueDiscontinuationCouponsParams) (IssueDiscontinuationCouponsResult, error)
  behavior: |
    params.ProductID の明細を持つカートの所有者のうち退会していないユーザーへ、同一条件のクーポンを
    1 枚ずつ発行する。渡された ctx のトランザクション内で実行する。

    受給者は述語（cart_items への結合と退会の除外）でしか決まらず件数に上限も無いため、呼び出し側が
    集約を組み立てて渡すことはできない。そのため引数は「決まった集約」ではなく発行条件のテンプレートで、
    個々の Coupon はこのメソッドの中で採番される。

    往復は受給者の取得と挿入の 2 回で、発行枚数に比例して増えない。2 文に分かれるのは主キーの採番を
    ドメイン層に置く ADR-0037 の要請による。集合演算が満たすべき性質は往復が母集団に比例しないことで
    あって、文がちょうど 1 つであることではない。
  invariants:
    - ゲストのカート（所有者未確定）は影響を受けるが受給者にならない
    - 退会済みユーザーは受給者にならない
    - 適用範囲は廃番商品のカテゴリで固定される（商品自身を範囲にすると買えない商品にしか使えない）

- name: IssuePromotionalCoupons
  package: internal/usecase/coupon/command
  signature: IssuePromotionalCoupons(ctx context.Context, params IssuePromotionalCouponsParams) (IssuePromotionalCouponsResult, error)
  behavior: |
    退会していないすべてのユーザーへ、同一条件のクーポンを 1 枚ずつ発行する。渡された ctx の
    トランザクション内で実行する。受給者が 0 人の場合は挿入せず、件数 0 を返す。

    引数は廃番と同じく「決まった集約」ではなく発行条件のテンプレートで、個々の Coupon はこの
    メソッドの中で採番される。受給者を読んでから Domain のコンストラクタで全行を構築するため、
    集約の不変条件を満たさない行はデータベースへ届かない。

    往復は受給者の取得と挿入の 2 回で、発行枚数に比例して増えない。廃番と同じ形。
  invariants:
    - 退会済みユーザーは受給者にならない（廃番の一括発行と同一の述語）
    - users 行はロックしない。受給者の取得と挿入の間に退会した利用者へは発行されうるが、
      退会後の物理削除でそのクーポンも消えるため許容する
    - 発行枚数の上限は強制しない。上限は application policy であり、呼び出し側の usecase が持つ
```

## Query Service

```yaml
- name: EstimateDiscontinueImpact
  package: internal/usecase/product/query
  signature: EstimateDiscontinueImpact(ctx context.Context, productID uuid.UUID) (DiscontinueImpactReadModel, error)
  behavior: |
    商品を廃番にした場合の影響を件数で返す。カート件数・受給対象の利用者数・進行中の購入件数の 3 つ。

    行をロックしない。返した値は返した瞬間から古くなり、実行時の件数と一致する保証はない。押す前に
    規模を見せるための読み取りであり、可否の判定そのものは実行時のトランザクションが持つ。

    各件数の母集団は CommandService 側の書き込みと 1 対 1 で対応する。片方だけを変えると、見積もりと
    実行が食い違って押す前に見せた数字の意味が失われる。
  invariants:
    - AffectedUserCount <= AffectedCartCount（ゲストのカートと退会済みを除くため）
```

## Notes

- 引き換え・失効判定は本 spec の射程外。値引き額の計算（`Discount.Apply`）と適用範囲の判定
  （`Scope.Covers`）も、それを呼ぶ引き換えと一緒に足す。
