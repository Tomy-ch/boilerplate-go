package coupon

import (
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
)

// Line は、値引きの対象になりうる明細 1 件の観測値です。
//
// クーポンはカート集約も購入集約も参照しないため、どちらの明細もこの形に写してから渡します
// （集約をまたぐ参照は識別子に限る。internal/domain/README.md の Aggregate Design）。
// 商品カテゴリを持つのは、適用範囲がカテゴリで絞れるためです。
type Line struct {
	productID  uuid.UUID
	categoryID uuid.UUID
	subtotal   decimal.Decimal
}

// LineAttributes は、明細の観測値一式です。productID と categoryID が同型のため構造体で受けます
// （基準は docs/rules.md の Function Signature Rules）。
type LineAttributes struct {
	// ProductID は、明細が指す商品です。
	ProductID uuid.UUID
	// CategoryID は、その商品が属する商品カテゴリです。
	CategoryID uuid.UUID
	// Subtotal は、明細の小計（単価 × 数量）です。価格スケールの十進量で、丸めません。
	Subtotal decimal.Decimal
}

// NewLine は、明細の観測値を組み立てます。検証は持ちません
// （理由は docs/spec/domain/coupon.md の Value Objects > Line を参照）。
func NewLine(attrs LineAttributes) Line {
	return Line{
		productID:  attrs.ProductID,
		categoryID: attrs.CategoryID,
		subtotal:   attrs.Subtotal,
	}
}

// ProductID は、明細が指す商品の ID を返します。
func (l Line) ProductID() uuid.UUID { return l.productID }

// CategoryID は、明細が指す商品の属する商品カテゴリの ID を返します。
func (l Line) CategoryID() uuid.UUID { return l.categoryID }

// Subtotal は、明細の小計を返します。
func (l Line) Subtotal() decimal.Decimal { return l.subtotal }
