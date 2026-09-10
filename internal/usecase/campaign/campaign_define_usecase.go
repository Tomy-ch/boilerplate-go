package campaign

import (
	"context"
	"time"

	"go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// DefineCampaignParams は、キャンペーンを 1 件定義する要求の入力です。
type DefineCampaignParams struct {
	// Code は、利用者が示す符号です。正規化はドメインが行うため、受け取った値をそのまま渡します。
	Code string
	// DiscountKind は、配るクーポンの値引きの決まり方の名前です（"flat" / "rate"）。
	DiscountKind string
	// DiscountValue は、配るクーポンの種別における値です。
	DiscountValue decimal.Decimal
	// MaxAmount は、配るクーポンの値引き上限（USD セント）です。上限を設けない場合は nil です。
	MaxAmount *int64
	// ScopeKind は、配るクーポンの適用範囲の決まり方の名前です（"all" / "category" / "product"）。
	ScopeKind string
	// ScopeTargetID は、配るクーポンの適用範囲が絞る対象です。全体では nil です。
	ScopeTargetID *uuid.UUID
	// MinPurchaseAmount は、配るクーポンの最低購入金額（USD セント）です。条件を課さない場合は nil です。
	MinPurchaseAmount *int64
	// UsableFrom は、配るクーポンの利用開始日時です。発行時点から使えるようにする場合は nil です。
	UsableFrom *time.Time
	// CouponExpiresAt は、配るクーポンの有効期限です。配布期間の終了より後である必要があります。
	CouponExpiresAt time.Time
	// StartsAt は、配布期間の開始日時です。
	StartsAt time.Time
	// EndsAt は、配布期間の終了日時です。
	EndsAt time.Time
	// TotalLimit は、総枚数上限です。
	TotalLimit int
	// PerUserLimit は、1 人あたり上限です。
	PerUserLimit int
}

// DefineCampaign は、admin がキャンペーンを 1 件定義します。
//
// テンプレートが実際にクーポンへ解決できるかを定義時に確かめます
// （理由は docs/spec/usecase/campaign.md の Workflow — DefineCampaign を参照）。
func (u *usecase) DefineCampaign(
	ctx context.Context,
	authn *auth.Authn,
	params DefineCampaignParams,
) (CampaignView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionCampaignDefine, authz.NewResource("campaign", nil),
	); err != nil {
		return CampaignView{}, err
	}

	code, err := campaign.NewCode(params.Code)
	if err != nil {
		return CampaignView{}, err
	}

	template, err := campaign.NewTemplate(campaign.TemplateAttributes{
		DiscountKindName:  params.DiscountKind,
		DiscountValue:     params.DiscountValue,
		DiscountMaxAmount: params.MaxAmount,
		ScopeKindName:     params.ScopeKind,
		ScopeTargetID:     params.ScopeTargetID,
		MinPurchaseAmount: params.MinPurchaseAmount,
		UsableFrom:        params.UsableFrom,
		ExpiresAt:         params.CouponExpiresAt,
	})
	if err != nil {
		return CampaignView{}, err
	}
	if _, _, err = resolveTemplate(template); err != nil {
		return CampaignView{}, err
	}

	id, err := uuid.New()
	if err != nil {
		return CampaignView{}, xerrors.Wrap(err, "failed to generate campaign id")
	}

	c, err := campaign.New(id, campaign.Attributes{
		Code:         code,
		Template:     template,
		StartsAt:     params.StartsAt,
		EndsAt:       params.EndsAt,
		TotalLimit:   params.TotalLimit,
		PerUserLimit: params.PerUserLimit,
	})
	if err != nil {
		return CampaignView{}, err
	}

	if err = u.campaignRepo.Create(ctx, c); err != nil {
		return CampaignView{}, err
	}

	return toCampaignView(c), nil
}
