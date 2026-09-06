package purchase

import (
	"time"

	"go-boilerplate/pkg/uuid"
)

// Detail は、購入 1 件の詳細読み取りモデルです。ステータス名は購入ステータスマスタで
// 解決済みで、書き込み集約 Purchase とは別型です（read 側・CQRS）。明細は購入明細（PurchaseDetail）の
// スライスです。
type Detail struct {
	// ID は、購入 ID です。
	ID uuid.UUID
	// Code は、購入コード（UUIDv7 文字列・一意）です。
	Code string
	// UserID は、購入したユーザーの ID です。
	UserID uuid.UUID
	// StatusID は、購入ステータス ID（購入ステータスマスタで解決済み）です。
	StatusID uuid.UUID
	// StatusCode は、購入ステータスの業務キー（Status.Code）です。
	StatusCode int
	// StatusName は、購入ステータスの名称（購入ステータスマスタで解決済み）です。
	StatusName string
	// SubtotalAmount は、小計（USD セント）です。値引き前の額です。
	SubtotalAmount int
	// CouponID は、適用したクーポンの ID です。未適用の場合は nil です。
	CouponID *uuid.UUID
	// DiscountAmount は、値引き額（USD セント）です。未適用の場合は 0 です。
	DiscountAmount int
	// TaxAmount は、税額（USD セント）です。
	TaxAmount int
	// ShippingFee は、送料（USD セント）です。
	ShippingFee int
	// TotalAmount は、合計（USD セント）です。
	TotalAmount int
	// Details は、購入明細のスライスです。
	Details []PurchaseDetail
	// OrderedAt は、注文日時です。
	OrderedAt time.Time
	// PaidAt は、支払い日時です。未支払いの場合は nil です。
	PaidAt *time.Time
	// CanceledAt は、キャンセル日時です。未キャンセルの場合は nil です。
	CanceledAt *time.Time
	// ShippedAt は、発送日時です。未発送の場合は nil です。
	ShippedAt *time.Time
	// DeliveredAt は、配達日時です。未配達の場合は nil です。
	DeliveredAt *time.Time
}
