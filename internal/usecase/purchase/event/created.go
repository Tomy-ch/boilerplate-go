// Package event は、購入ユースケースが発行する outbox イベントの本文（自己完結 snapshot）と
// その marshal を提供します。版付きのイベント種別と JSON のワイヤ表現を本パッケージへ隔離し、
// usecase 本体を薄く保ちます（ADR-0054 (transactional-outbox)）。
package event

import (
	"encoding/json"

	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/xerrors"
)

// TypeCreated は、購入作成の outbox イベント種別（version 込み）です。
const TypeCreated = "purchase.created.v1"

// created は、purchase.created.v1 の自己完結 snapshot payload です。金額は決済スケール（整数セント）です。
//
// 値引き額を載せるのは、載せないと snapshot が自己完結しなくなるためです。課税の基礎は値引き後の額
// なので（docs/spec/domain/purchase.md の Cross-field Invariants）、値引き額が無いと購読側で
// subtotal + tax + shipping が total に一致しません。
type created struct {
	PurchaseID     string          `json:"purchaseId"`
	Code           string          `json:"code"`
	UserID         string          `json:"userId"`
	StatusCode     int             `json:"statusCode"`
	SubtotalAmount int             `json:"subtotalAmount"`
	DiscountAmount int             `json:"discountAmount"`
	TaxAmount      int             `json:"taxAmount"`
	ShippingFee    int             `json:"shippingFee"`
	TotalAmount    int             `json:"totalAmount"`
	CouponID       *string         `json:"couponId"`
	Details        []createdDetail `json:"details"`
}

// createdDetail は、購入明細の snapshot です。UnitPrice は価格スケール（ドル decimal）を可逆に保つため文字列で保持します。
type createdDetail struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
	UnitPrice string `json:"unitPrice"`
}

// BuildCreated は、購入集約から purchase.created.v1 の自己完結 snapshot payload を marshal します。
func BuildCreated(p *purchase.Purchase) ([]byte, error) {
	src := p.Details()
	details := make([]createdDetail, len(src))
	for i, d := range src {
		details[i] = createdDetail{
			ProductID: d.ProductID().String(),
			Quantity:  d.Quantity(),
			UnitPrice: d.UnitPrice().String(),
		}
	}

	var couponID *string
	if id := p.CouponID(); id != nil {
		couponID = ptr.To(id.String())
	}

	payload, err := json.Marshal(created{
		PurchaseID:     p.ID().String(),
		Code:           p.Code(),
		UserID:         p.UserID().String(),
		StatusCode:     p.StatusCode(),
		SubtotalAmount: p.SubtotalAmount(),
		DiscountAmount: p.DiscountAmount(),
		TaxAmount:      p.TaxAmount(),
		ShippingFee:    p.ShippingFee(),
		TotalAmount:    p.TotalAmount(),
		CouponID:       couponID,
		Details:        details,
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode purchase.created payload")
	}
	return payload, nil
}
