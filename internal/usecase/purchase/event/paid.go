package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/pkg/xerrors"
)

// TypePaid は、購入支払いの outbox イベント種別（version 込み）です。
const TypePaid = "purchase.paid.v1"

// paid は、purchase.paid.v1 の通知 payload です。識別子と、起きた事実の時刻だけを運びます
// （payload_parity.yaml の kind: notification）。
type paid struct {
	PurchaseID string `json:"purchaseId"`
	Code       string `json:"code"`
	UserID     string `json:"userId"`
	StatusCode int    `json:"statusCode"`
	PaidAt     string `json:"paidAt"`
}

// BuildPaid は、購入集約から purchase.paid.v1 の通知 payload を marshal します。
func BuildPaid(p *purchase.Purchase) ([]byte, error) {
	var paidAt string
	if at := p.PaidAt(); at != nil {
		paidAt = at.Format(time.RFC3339Nano)
	}

	payload, err := json.Marshal(paid{
		PurchaseID: p.ID().String(),
		Code:       p.Code(),
		UserID:     p.UserID().String(),
		StatusCode: p.StatusCode(),
		PaidAt:     paidAt,
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode purchase.paid payload")
	}
	return payload, nil
}
