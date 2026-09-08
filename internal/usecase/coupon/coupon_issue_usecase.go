package coupon

import (
	"context"
	"time"

	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// IssueCouponParams は、受給者を名指ししてクーポンを 1 枚発行する要求の入力です。
type IssueCouponParams struct {
	// UserID は、受給者の利用者 ID です。
	UserID uuid.UUID
	// DiscountKind は、値引きの決まり方の名前です（"flat" / "rate"）。
	DiscountKind string
	// DiscountValue は、種別における値です。定額なら金額、定率なら率です。
	DiscountValue decimal.Decimal
	// ScopeKind は、適用範囲の決まり方の名前です（"all" / "category" / "product"）。
	ScopeKind string
	// ScopeTargetID は、適用範囲が絞る対象の識別子です。全体では nil です。
	ScopeTargetID *uuid.UUID
	// ExpiresAt は、発行するクーポンの有効期限です。締切を名指しするため絶対時刻で受けます。
	ExpiresAt time.Time
}

// IssueCoupon は、admin が受給者を名指ししてクーポンを 1 枚発行します。
//
// Repository への書き込みで足りる理由は docs/spec/usecase/coupon.md の Workflow — IssueCoupon を参照。
func (u *usecase) IssueCoupon(
	ctx context.Context,
	authn *auth.Authn,
	params IssueCouponParams,
) (CouponView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionCouponIssue, authz.NewResource("coupon", nil),
	); err != nil {
		return CouponView{}, err
	}

	discount, err := newDiscount(params.DiscountKind, params.DiscountValue)
	if err != nil {
		return CouponView{}, err
	}

	scope, err := newScope(params.ScopeKind, params.ScopeTargetID)
	if err != nil {
		return CouponView{}, err
	}

	id, err := uuid.New()
	if err != nil {
		return CouponView{}, xerrors.Wrap(err, "failed to generate coupon id")
	}

	now := u.clock.Now()

	var issued *coupon.Coupon
	err = u.txm.Do(ctx, func(ctx context.Context) error {
		// 存在確認をトランザクションの外へ出さないこと。理由は docs/spec/usecase/coupon.md の
		// Workflow — IssueCoupon の invariants を参照。
		if serr := u.ensureScopeTargetExists(ctx, scope); serr != nil {
			return serr
		}

		// 退会済みの利用者へは発行しません。FindByID は在籍者だけを返すため、
		// 不存在と退会済みは同じ NotFound になります。
		if _, uerr := u.userRepo.FindByID(ctx, params.UserID); uerr != nil {
			return uerr
		}

		c, cerr := coupon.New(id, coupon.Attributes{
			UserID:    params.UserID,
			Discount:  discount,
			Scope:     scope,
			ExpiresAt: params.ExpiresAt,
			IssuedAt:  now,
		})
		if cerr != nil {
			return cerr
		}

		if cerr = u.couponRepo.Create(ctx, c); cerr != nil {
			return cerr
		}
		issued = c

		return nil
	})
	if err != nil {
		return CouponView{}, err
	}

	return toCouponView(issued), nil
}
