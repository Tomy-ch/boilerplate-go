// Package coupon は、クーポンドメインを定義します。値引き（Discount）と適用範囲（Scope）という
// 直交する 2 つの値オブジェクトを持つ Coupon エンティティを提供します。
//
// 種別はマスタ表ではなくドメインが閉じた集合として持ち切ります。「定額か定率か」「全体かカテゴリか商品か」は
// 業務の語彙であってデータではなく、行として編集できることに意味がないためです（purchase.Status と同じ形）。
//
// 1 枚のクーポンは 1 つの値引きと 1 つの適用範囲を持ちます。複数枚の併用も、複数の適用範囲の合成も
// 表しません。
package coupon

import (
	"time"

	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// Coupons は、Coupon エンティティのスライス型です。
type Coupons []*Coupon

// Coupon は、クーポンを表すドメインエンティティです。受給者は発行時に確定し、以後移りません。
// 譲渡を表さない理由は docs/spec/domain/coupon.md の Overview を参照してください。
type Coupon struct {
	id                uuid.UUID
	userID            uuid.UUID
	discount          Discount
	scope             Scope
	minPurchaseAmount *int64
	usableFrom        *time.Time
	expiresAt         time.Time
	usedAt            *time.Time
	issuedAt          time.Time
}

// Attributes は、クーポンの属性一式です。ExpiresAt と IssuedAt が同型のため構造体で受けます
// （基準は docs/rules.md の Function Signature Rules）。
type Attributes struct {
	// UserID は、受給者です。
	UserID uuid.UUID
	// Discount は、いくら引くかです。
	Discount Discount
	// Scope は、どの明細が対象かです。
	Scope Scope
	// MinPurchaseAmount は、使うために必要な購入額の下限です。条件が無ければ nil です。
	MinPurchaseAmount *int64
	// UsableFrom は、使えるようになる日時です。発行時点から使えるなら nil です。
	UsableFrom *time.Time
	// ExpiresAt は、有効期限です。
	ExpiresAt time.Time
	// IssuedAt は、発行日時です。
	IssuedAt time.Time
}

// New は、クーポンエンティティの検証と生成を行います。生成直後は未使用です。
// id・UserID が未設定、Discount・Scope が未設定、ExpiresAt・IssuedAt がゼロ値の場合は検証エラーを返します。
// 有効期限は発行日時より後である必要があります（理由は docs/spec/domain/coupon.md の
// Cross-field Invariants を参照）。
func New(id uuid.UUID, attrs Attributes) (*Coupon, error) {
	return newCoupon(id, attrs, nil)
}

// Reconstruct は、永続化済みのクーポンを再構築します。usedAt は使用済みなら使用日時、
// 未使用なら nil です。その他の検証は New と同一です。
func Reconstruct(id uuid.UUID, attrs Attributes, usedAt *time.Time) (*Coupon, error) {
	return newCoupon(id, attrs, usedAt)
}

// validateValidity は、発行日時と有効期限の組を検証します。有効期限は発行日時より後である必要があります
// （理由は docs/spec/domain/coupon.md の Cross-field Invariants を参照）。
func validateValidity(issuedAt, expiresAt time.Time) error {
	if issuedAt.IsZero() {
		return xerrors.Wrap(ErrInvalidIssuedAt, "issuedAt is required")
	}
	if expiresAt.IsZero() {
		return xerrors.Wrap(ErrInvalidExpiresAt, "expiresAt is required")
	}
	if !expiresAt.After(issuedAt) {
		return xerrors.Wrap(ErrInvalidExpiresAt, "expiresAt must be after issuedAt")
	}

	return nil
}

// newCoupon は、生成・再構築に共通の検証を行いクーポンエンティティを構築します。
func newCoupon(id uuid.UUID, attrs Attributes, usedAt *time.Time) (*Coupon, error) {
	if id.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidID, "id is required")
	}
	if attrs.UserID.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidUserID, "userID is required")
	}
	if attrs.Discount.IsZero() {
		return nil, ErrInvalidDiscount
	}
	if attrs.Scope.IsZero() {
		return nil, ErrInvalidScope
	}
	if err := validateValidity(attrs.IssuedAt, attrs.ExpiresAt); err != nil {
		return nil, err
	}
	if err := validateConditions(attrs); err != nil {
		return nil, err
	}

	used := ptr.Copy(usedAt)

	return &Coupon{
		id:                id,
		userID:            attrs.UserID,
		discount:          attrs.Discount,
		scope:             attrs.Scope,
		minPurchaseAmount: ptr.Copy(attrs.MinPurchaseAmount),
		usableFrom:        ptr.Copy(attrs.UsableFrom),
		expiresAt:         attrs.ExpiresAt,
		usedAt:            used,
		issuedAt:          attrs.IssuedAt,
	}, nil
}

// validateConditions は、使えるかどうかを決める条件の組を検証します。どちらも任意で、
// 未設定（nil）は「条件なし」を表します
// （利用開始日時と有効期限の関係は docs/spec/domain/coupon.md の Cross-field Invariants）。
func validateConditions(attrs Attributes) error {
	if attrs.MinPurchaseAmount != nil && *attrs.MinPurchaseAmount <= 0 {
		return xerrors.Wrap(ErrInvalidMinPurchaseAmount, "minPurchaseAmount must be positive")
	}
	if attrs.UsableFrom != nil && !attrs.ExpiresAt.After(*attrs.UsableFrom) {
		return xerrors.Wrap(ErrInvalidUsableFrom, "usableFrom must be before expiresAt")
	}

	return nil
}

// ID は、クーポン ID を返します。
func (c *Coupon) ID() uuid.UUID { return c.id }

// UserID は、受給者のユーザー ID を返します。
func (c *Coupon) UserID() uuid.UUID { return c.userID }

// Discount は、値引きを返します。
func (c *Coupon) Discount() Discount { return c.discount }

// Scope は、適用範囲を返します。
func (c *Coupon) Scope() Scope { return c.scope }

// MinPurchaseAmount は、使うために必要な購入額の下限を決済スケール（USD セント）で返します。
// 条件が無い場合は nil です。
func (c *Coupon) MinPurchaseAmount() *int64 { return ptr.Copy(c.minPurchaseAmount) }

// UsableFrom は、使えるようになる日時を返します。発行時点から使える場合は nil です。
func (c *Coupon) UsableFrom() *time.Time { return ptr.Copy(c.usableFrom) }

// ExpiresAt は、有効期限を返します。
func (c *Coupon) ExpiresAt() time.Time { return c.expiresAt }

// IssuedAt は、発行日時を返します。
func (c *Coupon) IssuedAt() time.Time { return c.issuedAt }

// UsedAt は、使用日時を返します。未使用の場合は nil です。
func (c *Coupon) UsedAt() *time.Time { return ptr.Copy(c.usedAt) }

// IsUsed は、クーポンが使用済みかどうかを返します。
func (c *Coupon) IsUsed() bool { return c.usedAt != nil }

// IsHeldBy は、そのユーザーがこのクーポンの受給者かどうかを返します。
// 受給者は変わらないため（[Coupon] を参照）、判定は等値比較だけで足ります。
func (c *Coupon) IsHeldBy(userID uuid.UUID) bool { return c.userID == userID }

// DiscountFor は、渡された明細のうち適用範囲に入るものを対象として、差し引く額を決済スケールの
// 整数（USD セント）で返します。対象が 1 件も無い場合、購入額が最低購入金額に満たない場合、
// 差し引く額が最小単位に満たない場合はいずれも 0 です。
//
// 最低購入金額が見るのは購入の小計で、値引き上限は丸めたあとの額に効きます
// （根拠は docs/spec/domain/coupon.md の Behavior Methods > DiscountFor）。
//
// 丸めはこのメソッドでのみ行います（[Discount.Apply] は丸めません。ADR-0038 (two-scale-quantity-model)）。
func (c *Coupon) DiscountFor(lines []Line) (int, error) {
	subtotal := decimal.FromInt(0)
	eligible := decimal.FromInt(0)
	for _, line := range lines {
		subtotal = subtotal.Add(line.Subtotal())
		if c.scope.Covers(line) {
			eligible = eligible.Add(line.Subtotal())
		}
	}

	subtotalCents, err := subtotal.Truncate(minorUnitDigits).ToScaledInt64(minorUnitDigits)
	if err != nil {
		return 0, xerrors.Wrap(ErrInvalidMinPurchaseAmount, "purchase subtotal exceeds the settlement range")
	}
	if !c.SatisfiesMinPurchase(subtotalCents) {
		return 0, nil
	}

	cents, err := c.discount.Apply(eligible).Truncate(minorUnitDigits).ToScaledInt64(minorUnitDigits)
	if err != nil {
		return 0, xerrors.Wrap(ErrInvalidDiscountValue, "discount exceeds the settlement range")
	}

	return int(c.discount.LimitToMaxAmount(cents)), nil
}

// SatisfiesMinPurchase は、購入の小計が最低購入金額を満たすかどうかを返します。
// 条件が無いクーポンは常に満たします。額は決済スケール（USD セント）の整数です。
func (c *Coupon) SatisfiesMinPurchase(subtotalCents int64) bool {
	return c.minPurchaseAmount == nil || subtotalCents >= *c.minPurchaseAmount
}

// Redeem は、クーポンを使用済みにします。使用済みから未使用へ戻すのは [Coupon.Restore] だけです。
//
// 既に使用済みなら ErrAlreadyUsed、渡された時点で失効しているなら ErrExpired、
// 利用開始日時より前なら ErrNotYetUsable を返し、状態を変えません。
func (c *Coupon) Redeem(now time.Time) error {
	if c.IsUsed() {
		return ErrAlreadyUsed
	}
	if c.IsExpired(now) {
		return ErrExpired
	}
	if c.IsNotYetUsable(now) {
		return ErrNotYetUsable
	}

	c.usedAt = &now

	return nil
}

// Restore は、使用済みのクーポンを未使用へ戻し、戻したかどうかを返します。
//
// 渡された時点で失効している場合は状態を変えず false を返します（理由は
// docs/spec/domain/coupon.md の Behavior Methods > Restore を参照）。
// 未使用のクーポンには ErrNotUsed を返します。
func (c *Coupon) Restore(now time.Time) (bool, error) {
	if !c.IsUsed() {
		return false, ErrNotUsed
	}
	if c.IsExpired(now) {
		return false, nil
	}

	c.usedAt = nil

	return true, nil
}

// IsExpired は、渡された時点でクーポンが失効しているかどうかを返します。有効期限ちょうども
// 失効として扱います。判定の設計は docs/spec/domain/coupon.md の Behavior Methods を参照してください。
func (c *Coupon) IsExpired(now time.Time) bool { return !now.Before(c.expiresAt) }

// IsNotYetUsable は、渡された時点でクーポンがまだ使えないかどうかを返します。
// 利用開始日時ちょうどは使える側に含めます。利用開始日時を持たないクーポンは常に false です
// （区間の閉じ方は docs/spec/domain/coupon.md の Behavior Methods）。
func (c *Coupon) IsNotYetUsable(now time.Time) bool {
	return c.usableFrom != nil && now.Before(*c.usableFrom)
}
