package campaign

import (
	"context"
	"errors"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// ClaimCoupon は、認証主体がコードを示してクーポンを 1 枚受け取ります。
//
// 認可は呼びません。自分のためにクーポンを受け取る操作であり、admin の操作ではないためです。
//
// ロックは [campaign.Campaign.Claim] の前提どおり、枚数を数えるより前に取ります。
// 押さえるのはキャンペーン行だけです（docs/spec/usecase/campaign.md の Workflow — ClaimCoupon）。
func (u *usecase) ClaimCoupon(
	ctx context.Context,
	authn *auth.Authn,
	code string,
) (ClaimedCouponView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	userID, err := requireUserID(authn)
	if err != nil {
		return ClaimedCouponView{}, err
	}

	// 形式が不正なコードも、存在しないコードと同じ扱いにします。区別できると、
	// 形式の当たりだけを先に絞り込めてしまいます。
	claimCode, err := campaign.NewCode(code)
	if err != nil {
		return ClaimedCouponView{}, unclaimable(campaign.ErrCodeUnknown)
	}

	couponID, err := uuid.New()
	if err != nil {
		return ClaimedCouponView{}, xerrors.Wrap(err, "failed to generate coupon id")
	}
	claimID, err := uuid.New()
	if err != nil {
		return ClaimedCouponView{}, xerrors.Wrap(err, "failed to generate claim id")
	}

	now := u.clock.Now()

	var issued *coupon.Coupon
	err = u.txm.Do(ctx, func(ctx context.Context) error {
		c, cerr := u.campaignRepo.LockByCode(ctx, claimCode)
		if cerr != nil {
			if errors.Is(cerr, apperror.ErrNotFound) {
				return unclaimable(campaign.ErrCodeUnknown)
			}

			return cerr
		}

		claimed, cerr := u.campaignRepo.CountClaims(ctx, campaign.ClaimCountParams{
			CampaignID: c.ID(),
			UserID:     userID,
		})
		if cerr != nil {
			return cerr
		}

		// 受け取りの事実は集約ルートだけが生む。記録を得られたことが、上限と期間の判定を
		// 通ったことと同じ意味になる。
		record, cerr := c.Claim(campaign.ClaimParams{
			ClaimedAt:     now,
			ClaimedByUser: claimed,
			ClaimID:       claimID,
			UserID:        userID,
			CouponID:      couponID,
		})
		if cerr != nil {
			return unclaimable(cerr)
		}

		attrs, cerr := couponAttributesFor(c, userID, now)
		if cerr != nil {
			return cerr
		}

		issuedCoupon, cerr := coupon.New(couponID, attrs)
		if cerr != nil {
			return cerr
		}
		if cerr = u.couponRepo.Create(ctx, issuedCoupon); cerr != nil {
			return cerr
		}
		if cerr = u.campaignRepo.RecordClaim(ctx, record); cerr != nil {
			return cerr
		}
		issued = issuedCoupon

		return nil
	})
	if err != nil {
		return ClaimedCouponView{}, err
	}

	return toClaimedCouponView(issued), nil
}

// unclaimable は、受け取れないことを表すエラーへ details を添えます。
//
// 理由そのものはエラーに残るため、ログとメトリクスからは追えます
// （畳む理由は docs/spec/usecase/campaign.md の「受け取れない理由を区別しない」）。
func unclaimable(err error) error {
	return apperror.WithDetails(err, campaign.FieldCode)
}
