package campaign

import (
	"context"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// claimInput は、受け取り 1 件をトランザクション内で成立させるための入力です。
// 同型の識別子が並ぶため構造体で受けます（基準は docs/rules.md の Function Signature Rules）。
type claimInput struct {
	// Code は、正規化済みのキャンペーンコードです。
	Code campaign.Code
	// UserID は、受け取る利用者です。
	UserID uuid.UUID
	// CouponID は、発行するクーポンの ID です。
	CouponID uuid.UUID
	// ClaimID は、受け取り記録の ID です。
	ClaimID uuid.UUID
	// ClaimedAt は、受け取り日時です。
	ClaimedAt time.Time
}

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
		c, cerr := u.claimWithin(ctx, claimInput{
			Code:      claimCode,
			UserID:    userID,
			CouponID:  couponID,
			ClaimID:   claimID,
			ClaimedAt: now,
		})
		if cerr != nil {
			return cerr
		}
		issued = c

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

// claimWithin は、開かれたトランザクションの中で受け取りを成立させ、発行したクーポンを返します。
//
// 呼び出し順がそのまま直列化の前提になります。ロック（LockByCode）を最初に取り、
// 枚数を数えるのはその後です（ADR-0036 (ordered-pessimistic-row-locks) の決定 2）。
func (u *usecase) claimWithin(ctx context.Context, in claimInput) (*coupon.Coupon, error) {
	c, err := u.campaignRepo.LockByCode(ctx, in.Code)
	if err != nil {
		if xerrors.Is(err, apperror.ErrNotFound) {
			return nil, unclaimable(campaign.ErrCodeUnknown)
		}

		return nil, err
	}

	claimed, err := u.campaignRepo.CountClaims(ctx, campaign.ClaimCountParams{
		CampaignID: c.ID(),
		UserID:     in.UserID,
	})
	if err != nil {
		return nil, err
	}

	// 受け取りの事実は集約ルートだけが生む。記録を得られたことが、上限と期間の判定を
	// 通ったことと同じ意味になる。
	record, err := c.Claim(campaign.ClaimParams{
		ClaimedAt:     in.ClaimedAt,
		ClaimedByUser: claimed,
		ClaimID:       in.ClaimID,
		UserID:        in.UserID,
		CouponID:      in.CouponID,
	})
	if err != nil {
		return nil, unclaimable(err)
	}

	attrs, err := couponAttributesFor(c, in.UserID, in.ClaimedAt)
	if err != nil {
		return nil, err
	}

	issued, err := coupon.New(in.CouponID, attrs)
	if err != nil {
		return nil, err
	}
	if err = u.couponRepo.Create(ctx, issued); err != nil {
		return nil, err
	}
	if err = u.campaignRepo.RecordClaim(ctx, record); err != nil {
		return nil, err
	}

	return issued, nil
}
