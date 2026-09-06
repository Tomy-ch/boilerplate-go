//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE

// Package query は、購入の読み取り専用クエリ（QueryService）のインターフェースと読み取りモデルを提供します。
// 購入と商品は独立集約であり、明細に商品名を含む集約跨ぎの read 投影のため QueryService として定義します（ADR-0032 (lightweight-cqrs)）。
package query

import (
	"context"
	"time"

	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
)

// PurchaseDetailQueryService は、購入詳細の集約跨ぎ read 投影を提供する QueryService です。
type PurchaseDetailQueryService interface {
	// FindDetailByUserAndCode は、認証主体（userID）が所有する購入 1 件を購入コードで引き、
	// 明細（商品名込み）とともに取得します。
	// 所有権は本サービス側の絞り込みで担保し、他人の購入・不存在はいずれも apperror.ErrNotFound を返して秘匿します。
	FindDetailByUserAndCode(ctx context.Context, userID uuid.UUID, code string) (*PurchaseDetailReadModel, error)
}

// PurchaseDetailReadModel は、購入詳細の読み取りモデルです。金額はすべて USD セント単位の整数、
// ステータスは購入ステータスマスタで解決済みの ID と名称です。
type PurchaseDetailReadModel struct {
	ID             uuid.UUID
	Code           string
	UserID         uuid.UUID
	StatusID       uuid.UUID
	StatusCode     int
	StatusName     string
	SubtotalAmount int64
	DiscountAmount int64
	AppliedCoupon  *AppliedCouponReadModel
	TaxAmount      int64
	ShippingFee    int64
	TotalAmount    int64
	Items          []PurchaseDetailItem
	OrderedAt      time.Time
	PaidAt         *time.Time
	CanceledAt     *time.Time
}

// AppliedCouponReadModel は、購入に適用したクーポンの読み取りモデルです。
// 商品名と同じく結合で解決した現在値です（docs/spec/usecase/purchase.md の GET 詳細）。
type AppliedCouponReadModel struct {
	ID            uuid.UUID
	DiscountKind  int
	DiscountValue decimal.Decimal
	ScopeKind     int
	ScopeTargetID *uuid.UUID
}

// PurchaseDetailItem は、購入明細 1 件の読み取りモデルです。ProductName は購入時点ではなく現在の商品名、
// UnitPrice は購入時点の単価スナップショット（価格スケール）です。
type PurchaseDetailItem struct {
	ProductID   uuid.UUID
	ProductName string
	Quantity    int
	UnitPrice   money.Price
}
