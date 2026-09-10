//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE

// Package campaign は、キャンペーンの定義・列挙・停止と、コードによる受け取りのユースケースを提供します。
//
// 受け取りはキャンペーンとクーポンの 2 集約を 1 つのトランザクションで書きます。書く行が識別子で
// 名指しできるため Repository 経由で足り、CommandService は要りません
// （判定手順は docs/design/data-access-pattern.md）。テンプレートからクーポン 1 枚を導く解決は
// 本パッケージの私有関数が持ちます（campaign_template.go）。
package campaign

import (
	"context"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/internal/usecase/boundary/clock"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/internal/usecase/tools/paging"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// CampaignView は、キャンペーン 1 件の出力です。
type CampaignView struct {
	// ID は、キャンペーン ID です。
	ID uuid.UUID
	// Code は、正規化済みのキャンペーンコードです。
	Code string
	// DiscountKind は、配るクーポンの値引きの決まり方の名前です。
	DiscountKind string
	// DiscountValue は、配るクーポンの種別における値です。
	DiscountValue decimal.Decimal
	// MaxAmount は、配るクーポンの値引き上限（USD セント）です。上限が無い場合は nil です。
	MaxAmount *int64
	// ScopeKind は、配るクーポンの適用範囲の決まり方の名前です。
	ScopeKind string
	// ScopeTargetID は、配るクーポンの適用範囲が絞る対象です。全体では nil です。
	ScopeTargetID *uuid.UUID
	// MinPurchaseAmount は、配るクーポンの最低購入金額（USD セント）です。条件が無い場合は nil です。
	MinPurchaseAmount *int64
	// UsableFrom は、配るクーポンの利用開始日時です。発行時点から使える場合は nil です。
	UsableFrom *time.Time
	// CouponExpiresAt は、配るクーポンの有効期限です。
	CouponExpiresAt time.Time
	// StartsAt は、配布期間の開始日時です。
	StartsAt time.Time
	// EndsAt は、配布期間の終了日時です。
	EndsAt time.Time
	// TotalLimit は、総枚数上限です。
	TotalLimit int
	// PerUserLimit は、1 人あたり上限です。
	PerUserLimit int
	// IssuedCount は、発行済み枚数です。
	IssuedCount int
	// SuspendedAt は、停止日時です。停止していない場合は nil です。
	SuspendedAt *time.Time
}

// CampaignListView は、キャンペーン一覧と総件数の組です。
type CampaignListView struct {
	// Campaigns は、取得したキャンペーンです。
	Campaigns []CampaignView
	// Total は、絞り込みを適用した総件数です。
	Total int
}

// ClaimedCouponView は、受け取りで発行されたクーポン 1 枚の出力です。
//
// 保有クーポン一覧（internal/usecase/coupon の CouponView）と同じ内容を返します
// （写像をこちらが持つ理由は docs/spec/usecase/campaign.md の DTOs 節を参照）。
type ClaimedCouponView struct {
	// ID は、クーポン ID です。
	ID uuid.UUID
	// DiscountKind は、値引きの決まり方の名前です。
	DiscountKind string
	// DiscountValue は、種別における値です。
	DiscountValue decimal.Decimal
	// MaxAmount は、定率の値引きが 1 回に引ける額の上限（USD セント）です。上限が無い場合は nil です。
	MaxAmount *int64
	// ScopeKind は、適用範囲の決まり方の名前です。
	ScopeKind string
	// ScopeTargetID は、適用範囲が絞る対象の識別子です。全体では nil です。
	ScopeTargetID *uuid.UUID
	// MinPurchaseAmount は、使うために必要な購入額の下限（USD セント）です。条件が無い場合は nil です。
	MinPurchaseAmount *int64
	// UsableFrom は、使えるようになる日時です。発行時点から使える場合は nil です。
	UsableFrom *time.Time
	// ExpiresAt は、有効期限です。
	ExpiresAt time.Time
	// UsedAt は、使用日時です。発行直後は nil です。
	UsedAt *time.Time
	// IssuedAt は、発行日時です。
	IssuedAt time.Time
}

// Usecase は、キャンペーンの定義・列挙・停止と受け取りのユースケースを定義します。
type Usecase interface {
	// DefineCampaign は、キャンペーンを 1 件定義し、定義したキャンペーンを返します。
	// admin のみ実行できます。コードが既に使われている場合は Conflict を返します。
	DefineCampaign(ctx context.Context, authn *auth.Authn, params DefineCampaignParams) (CampaignView, error)
	// ListCampaigns は、キャンペーンを配布期間の開始が新しい順で返します。停止済みも含みます。
	// admin のみ実行できます。
	ListCampaigns(ctx context.Context, authn *auth.Authn, page *paging.Page) (CampaignListView, error)
	// SuspendCampaign は、キャンペーンの受け取りを打ち切ります。admin のみ実行できます。
	// 存在しない場合は NotFound、既に停止済みの場合は Conflict を返します。
	SuspendCampaign(ctx context.Context, authn *auth.Authn, id uuid.UUID) (CampaignView, error)
	// ClaimCoupon は、認証主体がコードを示してクーポンを 1 枚受け取ります。
	//
	// 受け取れない理由はすべて同じ検証エラーへ畳まれ、呼び出し元からは区別できません
	// （docs/spec/usecase/campaign.md の「受け取れない理由を区別しない」）。
	ClaimCoupon(ctx context.Context, authn *auth.Authn, code string) (ClaimedCouponView, error)
}

type usecase struct {
	tracer       observability.LayerTracer
	campaignRepo campaign.Repository
	couponRepo   coupon.Repository
	clock        clock.Clock
	txm          tx.Manager
	authorizer   authz.Authorizer
}

// New は、キャンペーンのユースケースを生成して返します。
func New(
	campaignRepo campaign.Repository,
	couponRepo coupon.Repository,
	clk clock.Clock,
	txm tx.Manager,
	authorizer authz.Authorizer,
	tf observability.TracerFactory,
) Usecase {
	return &usecase{
		tracer:       tf.Usecase(),
		campaignRepo: campaignRepo,
		couponRepo:   couponRepo,
		clock:        clk,
		txm:          txm,
		authorizer:   authorizer,
	}
}

// ListCampaigns は、一覧と総件数を同じ絞り込みで取得します。
func (u *usecase) ListCampaigns(
	ctx context.Context,
	authn *auth.Authn,
	page *paging.Page,
) (CampaignListView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionCampaignList, authz.NewResource("campaign", nil),
	); err != nil {
		return CampaignListView{}, err
	}
	if page == nil {
		return CampaignListView{}, xerrors.Wrap(apperror.ErrInvalidArgument, "page is required")
	}

	cs, err := u.campaignRepo.FindList(ctx, campaign.ListParams{
		Limit:  page.Limit32(),
		Offset: page.Offset32(),
	})
	if err != nil {
		return CampaignListView{}, err
	}

	total, err := u.campaignRepo.CountAll(ctx)
	if err != nil {
		return CampaignListView{}, err
	}

	views := make([]CampaignView, len(cs))
	for i, c := range cs {
		views[i] = toCampaignView(c)
	}

	return CampaignListView{Campaigns: views, Total: total}, nil
}

// SuspendCampaign は、行ロックを取ってから停止の可否をドメインへ判定させます。
func (u *usecase) SuspendCampaign(
	ctx context.Context,
	authn *auth.Authn,
	id uuid.UUID,
) (CampaignView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionCampaignSuspend, authz.NewResource("campaign", nil),
	); err != nil {
		return CampaignView{}, err
	}

	now := u.clock.Now()

	var suspended *campaign.Campaign
	err := u.txm.Do(ctx, func(ctx context.Context) error {
		c, cerr := u.campaignRepo.LockByID(ctx, id)
		if cerr != nil {
			return cerr
		}
		if cerr = c.Suspend(now); cerr != nil {
			return cerr
		}
		if cerr = u.campaignRepo.UpdateSuspended(ctx, id, now); cerr != nil {
			return cerr
		}
		suspended = c

		return nil
	})
	if err != nil {
		return CampaignView{}, err
	}

	return toCampaignView(suspended), nil
}

// toCampaignView は、集約を出力へ写します。
func toCampaignView(c *campaign.Campaign) CampaignView {
	t := c.Template()

	return CampaignView{
		ID:                c.ID(),
		Code:              c.Code().Value(),
		DiscountKind:      t.DiscountKindName(),
		DiscountValue:     t.DiscountValue(),
		MaxAmount:         t.DiscountMaxAmount(),
		ScopeKind:         t.ScopeKindName(),
		ScopeTargetID:     t.ScopeTargetID(),
		MinPurchaseAmount: t.MinPurchaseAmount(),
		UsableFrom:        t.UsableFrom(),
		CouponExpiresAt:   t.ExpiresAt(),
		StartsAt:          c.StartsAt(),
		EndsAt:            c.EndsAt(),
		TotalLimit:        c.TotalLimit(),
		PerUserLimit:      c.PerUserLimit(),
		IssuedCount:       c.IssuedCount(),
		SuspendedAt:       c.SuspendedAt(),
	}
}

// toClaimedCouponView は、受け取りで生まれたクーポンを出力へ写します。
func toClaimedCouponView(c *coupon.Coupon) ClaimedCouponView {
	return ClaimedCouponView{
		ID:                c.ID(),
		DiscountKind:      c.Discount().Kind().Name(),
		DiscountValue:     c.Discount().Value(),
		MaxAmount:         c.Discount().MaxAmount(),
		ScopeKind:         c.Scope().Kind().Name(),
		ScopeTargetID:     c.Scope().TargetID(),
		MinPurchaseAmount: c.MinPurchaseAmount(),
		UsableFrom:        c.UsableFrom(),
		ExpiresAt:         c.ExpiresAt(),
		UsedAt:            c.UsedAt(),
		IssuedAt:          c.IssuedAt(),
	}
}

// requireUserID は、認証主体から内部ユーザー ID を取り出します。
func requireUserID(authn *auth.Authn) (uuid.UUID, error) {
	if authn == nil {
		return uuid.UUID{}, apperror.ErrUnauthenticated
	}

	return authn.UserID()
}
