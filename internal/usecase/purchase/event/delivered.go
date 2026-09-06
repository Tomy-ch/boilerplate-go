package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/pkg/xerrors"
)

// TypeDelivered は、購入配達完了の outbox イベント種別（version 込み）です。
const TypeDelivered = "purchase.delivered.v1"

// delivered は、purchase.delivered.v1 の通知 payload です（payload_parity.yaml の kind: notification）。
type delivered struct {
	PurchaseID  string `json:"purchaseId"`
	Code        string `json:"code"`
	UserID      string `json:"userId"`
	StatusCode  int    `json:"statusCode"`
	DeliveredAt string `json:"deliveredAt"`
}

// BuildDelivered は、購入集約から purchase.delivered.v1 の通知 payload を marshal します。
func BuildDelivered(p *purchase.Purchase) ([]byte, error) {
	var deliveredAt string
	if at := p.DeliveredAt(); at != nil {
		deliveredAt = at.Format(time.RFC3339Nano)
	}

	payload, err := json.Marshal(delivered{
		PurchaseID:  p.ID().String(),
		Code:        p.Code(),
		UserID:      p.UserID().String(),
		StatusCode:  p.StatusCode(),
		DeliveredAt: deliveredAt,
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode purchase.delivered payload")
	}
	return payload, nil
}
