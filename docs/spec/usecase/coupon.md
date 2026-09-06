# Coupon — Usecase Spec

> クーポンのユースケース spec。読み取り（保有一覧・いまのカートに使えるもの）を持つ。
> 発行は廃番ジャーニーの副作用として起き（[`product.md`](product.md) の廃番）、引き換えは購入確定の
> 中で行う（[`purchase.md`](purchase.md) の CreatePurchase）。

## Overview

クーポンの発行は、廃番のトランザクションの中で CommandService を通して行う。受給者が述語でしか
決まらないため、usecase が集約を組み立てて Repository へ渡す形に分解できない。

読み取りは 2 つ。保有一覧は「持っているもの」を並べ、使えるかどうかで絞らない。もう 1 つは
「いまのカートに使えるもの」で、使えるかどうかと値引き額を返す。

**後者は集約をまたぐ読みだが QueryService に置かない。** 適用範囲の判定（`Scope.Covers`）と値引き額の
計算（`Discount.Apply` / `Coupon.DiscountFor`）はドメインロジックであり、`docs/rules.md` の
Repository / QueryService Rules が QueryService へ書くことを禁じている。SQL の結合で値引き額を出すと
業務条件の著作権が infra へ移るため、Repository を束ねて usecase で結合し、判定はドメインへ渡す
（`cart.GetCart` が同じ形の先例）。

## Interface

```yaml
package: internal/usecase/coupon
interface: Usecase
methods:
  - name: ListMyCoupons
    signature: ListMyCoupons(ctx, authn) ([]CouponView, error)
  - name: ListApplicableToMyCart
    signature: ListApplicableToMyCart(ctx, authn) ([]CartCouponView, error)
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
```

## Dependencies

```yaml
- coupon.Repository    # FindByUserID
- cart.Repository      # FindByOwnerID（対象明細の母集団）
- product.Repository   # FindByIDs（単価と商品カテゴリの解決）
- clock.Clock          # 失効判定の現在時刻
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
