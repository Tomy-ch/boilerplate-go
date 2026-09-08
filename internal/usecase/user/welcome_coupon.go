package user

import (
	"time"

	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// ウェルカムクーポンの条件です。登録という業務イベントが配るものなので、要求では受けずにここで決めます。
// クーポン集約は発行事由を保持しないため（docs/spec/domain/coupon.md）、
// 「ウェルカム」であることを表すのはこの配線だけです。
//
// template 先が金額や期間を自分の要件へ差し替える場所は、この 2 つに閉じています。
const (
	// welcomeCouponValidity は、発行日時からの有効期間です。
	welcomeCouponValidity = 30 * 24 * time.Hour
	// welcomeCouponAmount は、定額で差し引く金額（決済通貨 USD）です。
	welcomeCouponAmount = "5.00"
)

// newWelcomeCoupon は、受給者と発行日時からウェルカムクーポンを組み立てます。
// 適用範囲は全体、値引きは定額で固定です。
func newWelcomeCoupon(userID uuid.UUID, issuedAt time.Time) (*coupon.Coupon, error) {
	amount, err := decimal.Parse(welcomeCouponAmount)
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to parse welcome coupon amount")
	}

	discount, err := coupon.NewFlatDiscount(amount)
	if err != nil {
		return nil, err
	}

	id, err := uuid.New()
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to generate welcome coupon id")
	}

	return coupon.New(id, coupon.Attributes{
		UserID:    userID,
		Discount:  discount,
		Scope:     coupon.NewAllScope(),
		ExpiresAt: issuedAt.Add(welcomeCouponValidity),
		IssuedAt:  issuedAt,
	})
}
