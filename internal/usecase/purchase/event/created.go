// Package event は、購入ユースケースが発行する outbox イベントの本文と、その marshal を提供します。
// 版付きのイベント種別と JSON のワイヤ表現を本パッケージへ隔離し、usecase 本体を薄く保ちます
// （ADR-0054 (transactional-outbox)）。payload の種別と集約との対応は payload_parity.yaml が宣言します
// （ADR-0113 (outbox-payload-kinds-and-parity-declaration)）。
package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/xerrors"
)

// TypeCreated は、購入作成の outbox イベント種別（version 込み）です。
const TypeCreated = "purchase.created.v1"

// errOrderedAtUnset は、注文日時を持たない集約から snapshot を組もうとした場合のエラーです。
var errOrderedAtUnset = xerrors.Wrap(apperror.ErrInternal, "purchase.created payload requires a persisted orderedAt")

// created は、purchase.created.v1 の snapshot payload です。金額は決済スケール（整数セント）です。
// 運ぶ項目と落とす項目の対応は payload_parity.yaml が宣言します。
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
	OrderedAt      string          `json:"orderedAt"`
	Details        []createdDetail `json:"details"`
}

// createdDetail は、購入明細の snapshot です。UnitPrice は価格スケール（ドル decimal）を可逆に保つため文字列で保持します。
type createdDetail struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
	UnitPrice string `json:"unitPrice"`
}

// BuildCreated は、購入集約から purchase.created.v1 の snapshot payload を marshal します。
//
// 注文日時は DB 採番なので、書き込み後に読み直した集約を渡してください。生成直後の集約はまだ
// 持っておらず、そのまま組むとゼロ値が snapshot に載ります。型では区別できないため、ここで弾きます。
func BuildCreated(p *purchase.Purchase) ([]byte, error) {
	if p.OrderedAt().IsZero() {
		return nil, errOrderedAtUnset
	}

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
		OrderedAt:      p.OrderedAt().Format(time.RFC3339Nano),
		Details:        details,
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode purchase.created payload")
	}
	return payload, nil
}
