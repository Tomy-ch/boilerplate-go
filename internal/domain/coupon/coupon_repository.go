//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE
package coupon

import (
	"context"
	"time"

	"go-boilerplate/pkg/uuid"
)

// Repository は、クーポンの永続化操作を定義するドメインリポジトリインターフェースです。
type Repository interface {
	// FindByUserID は、指定利用者が保有するクーポンを発行日時の新しい順で返します。
	// 使用済み・失効済みも含みます。1 枚も持たない場合は空を返します。
	FindByUserID(ctx context.Context, userID uuid.UUID) (Coupons, error)
	// LockByID は、引き換えのために ID からクーポンを取得します。存在しない場合は NotFound を返します。
	// 同一クーポンへの並行更新は、先行する更新が終わるまで待機したうえで最新の状態を取得します。
	LockByID(ctx context.Context, id uuid.UUID) (*Coupon, error)
	// UpdateUsed は、クーポンを使用済みにします。対象は LockByID で取得し [Coupon.Redeem] で
	// 検証済みです。他の書き手が先に消費していた場合は ErrUsedConcurrently を返します。
	UpdateUsed(ctx context.Context, id uuid.UUID, usedAt time.Time) error
}
