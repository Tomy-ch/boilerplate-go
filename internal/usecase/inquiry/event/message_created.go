// Package event は、問い合わせの outbox イベントの転送契約（Published Language）を持ちます。
// version 込みの種別名と JSON のフィールド名はここが所有し、ドメインは知りません。
package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/domain/inquiry"
	"go-boilerplate/pkg/xerrors"
)

// TypeMessageCreated は、問い合わせメッセージ追加の outbox イベント種別（version 込み）です。
const TypeMessageCreated = "inquiry.message.created.v1"

// SchemaVersionMessageCreated は、messageCreated payload の schema 版です。
// 種別名の末尾 v1 と同じものを、封筒が読める形で持ちます。
const SchemaVersionMessageCreated = 1

// messageAuthor は、送り手のうち購読側へ出す部分です。主体 ID は出しません
// （docs/spec/usecase/inquiry.md Notes「author に主体 ID を載せない」）。
type messageAuthor struct {
	Kind string `json:"kind"`
}

// messageCreated は、inquiry.message.created.v1 の snapshot payload です。
// 運ぶ項目と落とす項目の対応は payload_parity.yaml が宣言します。
type messageCreated struct {
	MessageID string        `json:"messageId"`
	InquiryID string        `json:"inquiryId"`
	Author    messageAuthor `json:"author"`
	Body      string        `json:"body"`
	Sequence  int64         `json:"sequence"`
	CreatedAt string        `json:"createdAt"`
}

// BuildMessageCreated は、問い合わせとメッセージから inquiry.message.created.v1 の payload を
// marshal します。メッセージは親への逆参照を持たないため、問い合わせを併せて受け取ります。
func BuildMessageCreated(i *inquiry.Inquiry, m *inquiry.Message) ([]byte, error) {
	payload, err := json.Marshal(messageCreated{
		MessageID: m.ID().String(),
		InquiryID: i.ID().String(),
		Author:    messageAuthor{Kind: m.Author().Kind().String()},
		Body:      m.Body(),
		Sequence:  m.Sequence(),
		CreatedAt: m.CreatedAt().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode inquiry.message.created payload")
	}
	return payload, nil
}
