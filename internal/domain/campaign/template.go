package campaign

import (
	"time"

	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// Template は、キャンペーンが配るクーポン 1 枚の内容を表す値オブジェクトです。
//
// 値引きと適用範囲の種別は、クーポン集約が持つ語彙の名前を保つだけで、既知かどうかはここでは
// 判定しません（理由は docs/spec/domain/campaign.md の Template を参照）。
type Template struct {
	discountKindName  string
	discountValue     decimal.Decimal
	discountMaxAmount *int64
	scopeKindName     string
	scopeTargetID     *uuid.UUID
	minPurchaseAmount *int64
	usableFrom        *time.Time
	expiresAt         time.Time
}

// TemplateAttributes は、テンプレートの属性一式です。同型の任意項目が並ぶため構造体で受けます
// （基準は docs/rules.md の Function Signature Rules）。
type TemplateAttributes struct {
	// DiscountKindName は、値引きの決まり方の名前です。
	DiscountKindName string
	// DiscountValue は、種別における値です。定額なら差し引く金額、定率なら掛ける率です。
	DiscountValue decimal.Decimal
	// DiscountMaxAmount は、定率の値引きが 1 回に引ける額の上限です。上限が無ければ nil です。
	DiscountMaxAmount *int64
	// ScopeKindName は、適用範囲の絞り方の名前です。
	ScopeKindName string
	// ScopeTargetID は、適用範囲が絞る対象です。全体を対象にするなら nil です。
	ScopeTargetID *uuid.UUID
	// MinPurchaseAmount は、配るクーポンの最低購入金額です。条件が無ければ nil です。
	MinPurchaseAmount *int64
	// UsableFrom は、配るクーポンの利用開始日時です。発行時点から使えるなら nil です。
	UsableFrom *time.Time
	// ExpiresAt は、配るクーポンの有効期限です。
	ExpiresAt time.Time
}

// NewTemplate は、テンプレートの検証と生成を行います。
//
// 種別の名前が空、値引きの値が 0 以下、任意の金額が 0 以下、有効期限がゼロ値の場合は検証エラーを返します。
// 利用開始日時は有効期限より前である必要があります。
func NewTemplate(attrs TemplateAttributes) (Template, error) {
	if attrs.DiscountKindName == "" {
		return Template{}, xerrors.Wrap(ErrInvalidDiscountKind, "discountKindName is required")
	}
	if attrs.DiscountValue.Sign() <= 0 {
		return Template{}, xerrors.Wrap(ErrInvalidDiscountValue, "discountValue must be positive")
	}
	if attrs.DiscountMaxAmount != nil && *attrs.DiscountMaxAmount <= 0 {
		return Template{}, xerrors.Wrap(ErrInvalidMaxAmount, "discountMaxAmount must be positive")
	}
	if attrs.ScopeKindName == "" {
		return Template{}, xerrors.Wrap(ErrInvalidScopeKind, "scopeKindName is required")
	}
	if attrs.MinPurchaseAmount != nil && *attrs.MinPurchaseAmount <= 0 {
		return Template{}, xerrors.Wrap(ErrInvalidMinPurchaseAmount, "minPurchaseAmount must be positive")
	}
	if attrs.ExpiresAt.IsZero() {
		return Template{}, xerrors.Wrap(ErrInvalidCouponExpiresAt, "expiresAt is required")
	}
	if attrs.UsableFrom != nil && !attrs.ExpiresAt.After(*attrs.UsableFrom) {
		return Template{}, xerrors.Wrap(ErrInvalidUsableFrom, "usableFrom must be before expiresAt")
	}

	return Template{
		discountKindName:  attrs.DiscountKindName,
		discountValue:     attrs.DiscountValue,
		discountMaxAmount: ptr.Copy(attrs.DiscountMaxAmount),
		scopeKindName:     attrs.ScopeKindName,
		scopeTargetID:     ptr.Copy(attrs.ScopeTargetID),
		minPurchaseAmount: ptr.Copy(attrs.MinPurchaseAmount),
		usableFrom:        ptr.Copy(attrs.UsableFrom),
		expiresAt:         attrs.ExpiresAt,
	}, nil
}

// DiscountKindName は、値引きの決まり方の名前を返します。
func (t Template) DiscountKindName() string { return t.discountKindName }

// DiscountValue は、種別における値を返します。
func (t Template) DiscountValue() decimal.Decimal { return t.discountValue }

// DiscountMaxAmount は、値引き上限を返します。上限が無い場合は nil です。
func (t Template) DiscountMaxAmount() *int64 { return ptr.Copy(t.discountMaxAmount) }

// ScopeKindName は、適用範囲の絞り方の名前を返します。
func (t Template) ScopeKindName() string { return t.scopeKindName }

// ScopeTargetID は、適用範囲が絞る対象を返します。全体を対象にする場合は nil です。
func (t Template) ScopeTargetID() *uuid.UUID { return ptr.Copy(t.scopeTargetID) }

// MinPurchaseAmount は、配るクーポンの最低購入金額を返します。条件が無い場合は nil です。
func (t Template) MinPurchaseAmount() *int64 { return ptr.Copy(t.minPurchaseAmount) }

// UsableFrom は、配るクーポンの利用開始日時を返します。発行時点から使える場合は nil です。
func (t Template) UsableFrom() *time.Time { return ptr.Copy(t.usableFrom) }

// ExpiresAt は、配るクーポンの有効期限を返します。
func (t Template) ExpiresAt() time.Time { return t.expiresAt }

// IsZero は、未設定のテンプレートかどうかを返します。
func (t Template) IsZero() bool { return t.discountKindName == "" }
