package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/pkg/xerrors"
)

// TypeShipped は、購入発送の outbox イベント種別（version 込み）です。
const TypeShipped = "purchase.shipped.v1"

// shipped は、purchase.shipped.v1 の通知 payload です。識別子と、起きた事実の時刻だけを運びます
// （payload_parity.yaml の kind: notification）。
type shipped struct {
	PurchaseID string `json:"purchaseId"`
	Code       string `json:"code"`
	UserID     string `json:"userId"`
	StatusCode int    `json:"statusCode"`
	ShippedAt  string `json:"shippedAt"`
}

// BuildShipped は、購入集約から purchase.shipped.v1 の通知 payload を marshal します。
func BuildShipped(p *purchase.Purchase) ([]byte, error) {
	var shippedAt string
	if at := p.ShippedAt(); at != nil {
		shippedAt = at.Format(time.RFC3339Nano)
	}

	payload, err := json.Marshal(shipped{
		PurchaseID: p.ID().String(),
		Code:       p.Code(),
		UserID:     p.UserID().String(),
		StatusCode: p.StatusCode(),
		ShippedAt:  shippedAt,
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode purchase.shipped payload")
	}
	return payload, nil
}
