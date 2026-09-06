package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/pkg/xerrors"
)

// TypeCanceled は、購入キャンセルの outbox イベント種別（version 込み）です。
const TypeCanceled = "purchase.canceled.v1"

// errNotCanceled は、キャンセルされていない購入からキャンセルのイベントを組み立てようとした場合の
// エラーです。イベントは起きた事実であり、事実でないものを発行すれば購読側はそれを取り消せません。
var errNotCanceled = xerrors.Wrap(apperror.ErrInternal, "purchase is not canceled")

// canceled は、purchase.canceled.v1 の通知 payload です（payload_parity.yaml の kind: notification）。
type canceled struct {
	PurchaseID string `json:"purchaseId"`
	Code       string `json:"code"`
	UserID     string `json:"userId"`
	StatusCode int    `json:"statusCode"`
	CanceledAt string `json:"canceledAt"`
}

// BuildCanceled は、購入集約から purchase.canceled.v1 の通知 payload を marshal します。
// キャンセルされていない購入を渡された場合は payload を生成せず errNotCanceled を返します。
func BuildCanceled(p *purchase.Purchase) ([]byte, error) {
	if !p.IsCanceled() {
		return nil, errNotCanceled
	}

	var canceledAt string
	if at := p.CanceledAt(); at != nil {
		canceledAt = at.Format(time.RFC3339Nano)
	}

	payload, err := json.Marshal(canceled{
		PurchaseID: p.ID().String(),
		Code:       p.Code(),
		UserID:     p.UserID().String(),
		StatusCode: p.StatusCode(),
		CanceledAt: canceledAt,
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode purchase.canceled payload")
	}
	return payload, nil
}
