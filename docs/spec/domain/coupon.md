# Coupon — Domain Spec

> クーポン（廃番の代替として一括発行される）のドメイン spec。値引きと適用範囲という**直交する 2 つの
> 値オブジェクト**を持つのが最大の特徴で、1 枚のクーポンは 1 つの値引きと 1 つの適用範囲を持つ。
> 複数枚の併用も、複数の適用範囲の合成も表さない。

## Overview

クーポン集約（Coupon）は、受給者・値引き・適用範囲・有効期限・使用状態を保持するドメインエンティティ。
**受給者は発行時に確定し、以後移らない。** 譲渡を表さないのは、クーポンが特定の利用者に生じた事情への
補償として発行されるためである。

**種別はマスタ表ではなくドメインが閉じた集合として持ち切る。** 「定額か定率か」「全体かカテゴリか商品か」は
業務の語彙であって、行として編集できることに意味がない。`purchase.Status` と同じ形で、業務キーは `code`、
意味はメソッドが持ち、UUID はドメインに焼き込まない。

**値引きと適用範囲を 1 軸の列挙に畳まない。** 畳むと `定率 × カテゴリ限定` のような組み合わせごとに
メンバーが要り、軸を足すたび積で増える。2 つに割れば、それぞれが 1 つの問い（いくら引くか / どの明細が
対象か）に答えるだけで済む。

**値引き額の計算と適用範囲の判定はこの集約が持つ。** どちらも「このクーポンは何をするか」の問いで、
1 枚について閉じている。丸めは `DiscountFor` の 1 箇所だけで起きる（[ADR-0038](../../adr/0038-two-scale-quantity-model.md)）。

`id` は UUIDv7（[ADR-0037](../../adr/0037-uuidv7-identifiers.md)）で、生成は usecase 層（廃番では
CommandService）が行いドメインへ渡す。有効期限も時刻境界から供給された値を受け取る。

## Entity

```yaml
package: internal/domain/coupon
struct: Coupon
constructors:
  - name: New          # 発行時。生成直後は未使用
  - name: Reconstruct  # 永続化済みの再構築（usedAt を受け取る）
fields:
  - name: id
    type: uuid.UUID
    required: true          # IsNil の場合は ErrInvalidID
  - name: userID
    type: uuid.UUID
    required: true          # 受給者。IsNil の場合は ErrInvalidUserID
  - name: discount
    type: Discount
    required: true          # ゼロ値の場合は ErrInvalidDiscount
  - name: scope
    type: Scope
    required: true          # ゼロ値の場合は ErrInvalidScope
  - name: expiresAt
    type: time.Time
    required: true          # ゼロ値は ErrInvalidExpiresAt
  - name: usedAt
    type: "*time.Time"      # nil 許容（未使用）。使用済みへの遷移は引き換え（#1473）が持つ
  - name: issuedAt
    type: time.Time
    required: true          # ゼロ値は ErrInvalidIssuedAt
```

## Cross-field Invariants

- `expiresAt > issuedAt`（違反は `ErrInvalidExpiresAt`）。発行した時点で既に使えないクーポンは
  発行の意味を持たないため、同時刻も許さない。`New` / `Reconstruct` が共有する検証ゲートで課す。

## Behavior Methods

```yaml
- name: IsUsed
  signature: IsUsed() bool
  behavior: |
    使用済みかどうかを返す。使用日時が設定されていることを指す。
- name: IsExpired
  signature: IsExpired(now time.Time) bool
  behavior: |
    渡された時点で失効しているかを返す。有効期限ちょうどは失効として扱う。
    失効を一括更新する機構は持たず、判定のたびに現在時刻と突き合わせる（カートの期限切れと同じ形）。
    時刻はドメインの外から渡す。
- name: IsHeldBy
  signature: IsHeldBy(userID uuid.UUID) bool
  behavior: |
    そのユーザーが受給者かどうかを返す。受給者は発行時に確定し以後移らないため、等値比較で足りる。
- name: DiscountFor
  signature: DiscountFor(lines []Line) (int, error)
  behavior: |
    渡された明細のうち適用範囲に入るものを対象として、差し引く額を決済スケールの整数（USD セント）で返す。
    対象が 1 件も無い場合と、差し引く額が最小単位に満たない場合は 0。

    **値引き額の丸めはここが唯一の点。** 対象小計は価格スケールのまま合算し、差し引く額を求めてから
    一度だけ切り捨てる（ADR-0038）。事前確認と購入確定の双方がこのメソッドを通るため、見せた額と
    引かれる額が同じ規則で決まる。明細ごとに丸めないので端数の落ちも起きない。
- name: Redeem
  signature: Redeem(now time.Time) error
  behavior: |
    使用済みにする。一度きりの遷移で、取り消せない。
    既に使用済みなら ErrAlreadyUsed、渡された時点で失効しているなら ErrExpired を返し、状態を変えない。
    使用済みと失効が同時に成り立つ場合は使用済みを先に返す。
    日時は引数で受け取る（ドメインは時刻へ直接依存しない）。
```

## Value Objects

```yaml
- name: DiscountKind
  underlying_type: struct    # code int / name string
  validation: |
    既知の集合（定額 / 定率）だけを許す。永続化されている code からの解決は NewDiscountKind が行い、
    既知でない code は ErrInvalidDiscountKind（永続化状態の破損を再構築時に弾く）。
  factory: NewDiscountKind
  methods:
    - name: Code
      returns: int
    - name: Name
      returns: string
    - name: IsZero
      returns: bool

- name: Discount
  underlying_type: struct    # kind DiscountKind / value decimal.Decimal
  validation: |
    定額は正の金額、定率は 0 より大きく 1 以下。範囲外は ErrInvalidDiscountValue。
    1 を超える率は対象額より多く差し引くことになり、値引きの意味を失うため許さない。
    適用範囲は関知しない。どの明細が対象かは Scope が答える。
  factory: NewFlatDiscount / NewRateDiscount / ReconstructDiscount
  methods:
    - name: Kind
      returns: DiscountKind
    - name: Value
      returns: decimal.Decimal
    - name: IsZero
      returns: bool

- name: ScopeKind
  underlying_type: struct    # code int / name string
  validation: |
    既知の集合（全体 / カテゴリ限定 / 商品限定）だけを許す。扱いは DiscountKind と同じ。
  factory: NewScopeKind
  methods:
    - name: Code
      returns: int
    - name: Name
      returns: string
    - name: IsZero
      returns: bool

- name: Line
  underlying_type: struct    # productID / categoryID uuid.UUID / subtotal decimal.Decimal
  validation: |
    検証を持たない。観測した事実をそのまま運ぶ値であり、正しさの責務は観測元にある
    （カートの ProductSnapshot と同じ形）。
  factory: NewLine
  methods:
    - name: ProductID
      returns: uuid.UUID
    - name: CategoryID
      returns: uuid.UUID
    - name: Subtotal
      returns: decimal.Decimal

- name: Scope
  underlying_type: struct    # kind ScopeKind / targetID *uuid.UUID
  validation: |
    カテゴリ限定・商品限定は対象 ID を必須とし、全体は対象を持ってはならない（ErrInvalidScopeTarget）。
    対象は識別子だけを持ち、商品集約もカテゴリ集約も参照しない
    （集約をまたぐ参照は識別子に限る。internal/domain/README.md の Aggregate Design）。
  factory: NewAllScope / NewCategoryScope / NewProductScope / ReconstructScope
  methods:
    - name: Kind
      returns: ScopeKind
    - name: TargetID
      returns: "*uuid.UUID"   # 防御コピーを返す
    - name: IsZero
      returns: bool
    - name: Covers
      returns: bool           # 明細がこの適用範囲に入るか。全体は常に true、限定は対象の一致で決まる
```

**`Discount.Apply(eligible decimal.Decimal) decimal.Decimal`** は、対象額に対して差し引く額を
価格スケールのまま返す。定額は対象額を上限に切り詰め、定率は率を掛ける。どちらも対象額を超えないため
請求額が負にならない。丸めないのは、丸めを `Coupon.DiscountFor` の 1 箇所に集めるため。

## Repository Methods

```yaml
- name: FindByUserID
  signature: FindByUserID(ctx context.Context, userID uuid.UUID) (Coupons, error)
  behavior: |
    指定利用者が保有するクーポンを発行日時の新しい順で返す。使用済み・失効済みも含む。
    保有一覧が「使えるもの」ではなく「持っているもの」を並べるためで、使えるかどうかの判定はドメインが持つ。
- name: LockByID
  signature: LockByID(ctx context.Context, id uuid.UUID) (*Coupon, error)
  behavior: |
    引き換えのために ID からクーポンを悲観ロックして取得する。存在しない場合は NotFound。
    使用済み・失効・受給者では絞らない。いずれもドメインが述語を持つ条件であり、SQL 側へ書き写すと
    業務条件の著作権が infra へ移る。
- name: UpdateUsed
  signature: UpdateUsed(ctx context.Context, id uuid.UUID, usedAt time.Time) error
  behavior: |
    使用済みにする。対象は LockByID で取得し Redeem で検証済み。
    条件付き更新（used_at IS NULL）の 0 行は ErrUsedConcurrently（409）へ写す。
    行ロックの下では通常到達せず、ロックを取らずに呼ばれた場合の二重防御として立つ。
```

**廃番に伴う一括発行は Repository に持たない。** 発行対象が述語でしか決まらず件数に上限も無いため、
集約を 1 件ずつ構築して書く形に分解できない。その書き込みは CommandService が担う
（判定基準は [ADR-0034](../../adr/0034-commandservice-atomicity-criterion.md)、
実例は [`docs/spec/usecase/product.md`](../usecase/product.md) の廃番）。

**発行事由は保持しない。** 「どの商品の廃番が配ったクーポンか」は適用範囲からは辿れない
（範囲は廃番商品ではなくそのカテゴリで固定されるため）。廃番の再実行は過去の発行実績を返さず、
その実行が起こしたことだけを返す設計なので、事由を持つ動機が無い。

## Notes

- **クーポンの行を消す経路は持たない。** 使用済み・失効済みのいずれも行として残す。控えが値引きの
  理由を結合で解決するため、行が消えると金額の説明が付かなくなる。
  例外は利用者の物理削除で、そのとき発行済みのクーポンも一緒に消える（[`docs/spec/domain/user.md`](user.md)
  の `PurgeByIDs`）。物理削除の対象は購入を 1 件も持たない利用者に限られ、説明すべき控えがそもそも
  存在しないため、行を残す根拠がこの対象には届かない。
- 引き換え・失効判定と、それが読む振る舞い（値引き額の計算・適用範囲の判定）は本 spec の射程外。
