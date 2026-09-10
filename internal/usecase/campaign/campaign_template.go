package campaign

import (
	"time"

	"go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/pkg/uuid"
)

// resolveTemplate は、キャンペーンのテンプレートが保つ種別の名前を、クーポン集約の語彙へ解決します。
// 名前の解決と値の検証はドメインが行い、ここは橋渡しだけです
// （設計は docs/spec/usecase/campaign.md の Overview を参照）。
func resolveTemplate(t campaign.Template) (coupon.Discount, coupon.Scope, error) {
	discountKind, err := coupon.NewDiscountKindByName(t.DiscountKindName())
	if err != nil {
		return coupon.Discount{}, coupon.Scope{}, err
	}
	discount, err := coupon.NewDiscount(discountKind, t.DiscountValue())
	if err != nil {
		return coupon.Discount{}, coupon.Scope{}, err
	}
	if maxAmount := t.DiscountMaxAmount(); maxAmount != nil {
		discount, err = discount.WithMaxAmount(*maxAmount)
		if err != nil {
			return coupon.Discount{}, coupon.Scope{}, err
		}
	}

	scopeKind, err := coupon.NewScopeKindByName(t.ScopeKindName())
	if err != nil {
		return coupon.Discount{}, coupon.Scope{}, err
	}
	scope, err := coupon.NewScope(scopeKind, t.ScopeTargetID())
	if err != nil {
		return coupon.Discount{}, coupon.Scope{}, err
	}

	return discount, scope, nil
}

// couponAttributesFor は、キャンペーンが受給者へ配るクーポン 1 枚の属性を組み立てます。
// 受給者と発行日時だけが受け取りごとに変わり、残りはテンプレートから決まります。
//
// テンプレートの内容をクーポンへ固定する箇所はここです
// （焼き付けの根拠は docs/spec/domain/campaign.md の Overview を参照）。
func couponAttributesFor(
	c *campaign.Campaign, userID uuid.UUID, issuedAt time.Time,
) (coupon.Attributes, error) {
	t := c.Template()

	discount, scope, err := resolveTemplate(t)
	if err != nil {
		return coupon.Attributes{}, err
	}

	return coupon.Attributes{
		UserID:            userID,
		Discount:          discount,
		Scope:             scope,
		MinPurchaseAmount: t.MinPurchaseAmount(),
		UsableFrom:        t.UsableFrom(),
		ExpiresAt:         t.ExpiresAt(),
		IssuedAt:          issuedAt,
	}, nil
}
