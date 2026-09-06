package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/domain/inquiry"
	"go-boilerplate/pkg/xerrors"
)

// TypeThreadUpdated は、問い合わせ更新の outbox イベント種別（version 込み）です。
const TypeThreadUpdated = "inquiry.thread.updated.v1"

// SchemaVersionThreadUpdated は、threadUpdated payload の schema 版です。
const SchemaVersionThreadUpdated = 1

// threadUpdated は、inquiry.thread.updated.v1 の通知 payload です（payload_parity.yaml の kind: notification）。
// 本文を持たない理由は docs/spec/usecase/inquiry.md の Notes を参照してください。
type threadUpdated struct {
	InquiryID string `json:"inquiryId"`
	UserID    string `json:"userId"`
	Sequence  int64  `json:"sequence"`
	UpdatedAt string `json:"updatedAt"`
}

// BuildThreadUpdated は、問い合わせ集約から inquiry.thread.updated.v1 の payload を marshal します。
// sequence は会話 stream 側で採番された位置で、一覧が「どこまで進んだか」を判断するために載せます。
func BuildThreadUpdated(i *inquiry.Inquiry, sequence int64) ([]byte, error) {
	payload, err := json.Marshal(threadUpdated{
		InquiryID: i.ID().String(),
		UserID:    i.UserID().String(),
		Sequence:  sequence,
		UpdatedAt: i.UpdatedAt().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode inquiry.thread.updated payload")
	}
	return payload, nil
}
