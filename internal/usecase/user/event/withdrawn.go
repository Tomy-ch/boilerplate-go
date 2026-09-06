// Package event は、ユーザーの outbox イベント payload を提供します。
package event

import (
	"encoding/json"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/user"
	"go-boilerplate/pkg/xerrors"
)

// TypeWithdrawn は、ユーザー退会の outbox イベント種別（version 込み）です。
const TypeWithdrawn = "user.withdrawn.v1"

// ErrInvalidWithdrawn は、user.withdrawn.v1 の payload として読めないことを示すエラーです。
var ErrInvalidWithdrawn = xerrors.Wrap(apperror.ErrInvalidArgument, "invalid user.withdrawn payload")

// Withdrawn は、user.withdrawn.v1 の通知 payload です（payload_parity.yaml の kind: notification）。
// 他の payload と違って公開しているのは、退会アーカイブのワーカーが ParseWithdrawn で読み戻すためです。
type Withdrawn struct {
	UserID    string `json:"userId"`
	DeletedAt string `json:"deletedAt"`
}

// BuildWithdrawn は、ユーザー集約から user.withdrawn.v1 の通知 payload を marshal します。
// 退会していないユーザーを渡した場合、payload の deletedAt は空文字列になります。
func BuildWithdrawn(u *user.User) ([]byte, error) {
	var deletedAt string
	if at := u.DeletedAt(); at != nil {
		deletedAt = at.Format(time.RFC3339Nano)
	}

	payload, err := json.Marshal(Withdrawn{
		UserID:    u.ID().String(),
		DeletedAt: deletedAt,
	})
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to encode user.withdrawn payload")
	}
	return payload, nil
}

// ParseWithdrawn は、user.withdrawn.v1 の payload を復元します。
// 復元できない payload は ErrInvalidWithdrawn を返します。値の妥当性（userId が空でないか等）は
// 消費側の関心なので、ここでは判定しません。
func ParseWithdrawn(payload []byte) (Withdrawn, error) {
	var w Withdrawn
	if err := json.Unmarshal(payload, &w); err != nil {
		return Withdrawn{}, xerrors.Wrap(ErrInvalidWithdrawn, err.Error())
	}
	return w, nil
}
