// Package campaign は、キャンペーンリポジトリ（campaign.Repository）の RDB 実装を提供します。
package campaign

import (
	"context"
	"time"

	"go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
)

type repository struct {
	db     driver.DatabaseDriver
	tracer observability.LayerTracer
}

// New は、campaign.Repository の RDB 実装を生成して返します。
func New(
	db driver.DatabaseDriver,
	tf observability.TracerFactory,
) campaign.Repository {
	return &repository{
		db:     db,
		tracer: tf.Infra(),
	}
}

// Create は、定義済みの集約をそのまま 1 行へ写します。コードが既に使われている場合は
// 一意制約違反が正規化された結果を返します。
func (r *repository) Create(ctx context.Context, c *campaign.Campaign) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	params, err := campaignToCreateParams(c)
	if err != nil {
		return err
	}

	db := gen.New(driver.New(ctx, r.db))
	if err = db.CreateCampaign(ctx, params); err != nil {
		return pgerror.NormalizeError(err)
	}

	return nil
}

// FindList は、配布期間の開始が新しい順で取得します。
func (r *repository) FindList(ctx context.Context, params campaign.ListParams) (campaign.Campaigns, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	rows, err := db.SelectCampaigns(ctx, &gen.SelectCampaignsParams{
		Limit:  params.Limit,
		Offset: params.Offset,
	})
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	cs := make(campaign.Campaigns, 0, len(rows))
	for _, row := range rows {
		c, cerr := rowToCampaign(row.Campaigns)
		if cerr != nil {
			return nil, cerr
		}
		cs = append(cs, c)
	}

	return cs, nil
}

// CountAll は、キャンペーンの総件数を返します。
func (r *repository) CountAll(ctx context.Context) (int, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	total, err := db.CountCampaigns(ctx)
	if err != nil {
		return 0, pgerror.NormalizeError(err)
	}

	return int(total), nil
}

// LockByCode は、正規化済みコードで行ロックを取り、集約を再構築します。
func (r *repository) LockByCode(ctx context.Context, code campaign.Code) (*campaign.Campaign, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	row, err := db.LockCampaignByCode(ctx, code.Value())
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	return rowToCampaign(row.Campaigns)
}

// LockByID は、ID で行ロックを取り、集約を再構築します。
func (r *repository) LockByID(ctx context.Context, id uuid.UUID) (*campaign.Campaign, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	row, err := db.LockCampaignByID(ctx, id)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	return rowToCampaign(row.Campaigns)
}

// CountClaims は、指定利用者がそのキャンペーンから既に受け取った枚数を返します。
func (r *repository) CountClaims(ctx context.Context, params campaign.ClaimCountParams) (int, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	count, err := db.CountCampaignClaimsByCampaignIDAndUserID(
		ctx,
		&gen.CountCampaignClaimsByCampaignIDAndUserIDParams{
			CampaignID: params.CampaignID,
			UserID:     params.UserID,
		},
	)
	if err != nil {
		return 0, pgerror.NormalizeError(err)
	}

	return int(count), nil
}

// RecordClaim は、発行済み枚数の加算と受け取り記録の挿入を 1 つのトランザクションの中で行います。
// 加算は issued_count < total_limit を条件とし、0 行を ErrIssuedConcurrently へ写します。
// 0 行を NotFound へ正規化しないのは、行が無いのではなく上限が埋まったことを表すためです。
func (r *repository) RecordClaim(ctx context.Context, claim *campaign.Claim) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	affected, err := db.UpdateCampaignIssuedCount(ctx, claim.CampaignID())
	if err != nil {
		return pgerror.NormalizeError(err)
	}
	if affected == 0 {
		return campaign.ErrIssuedConcurrently
	}

	if err = db.CreateCampaignClaim(ctx, &gen.CreateCampaignClaimParams{
		ID:         claim.ID(),
		CampaignID: claim.CampaignID(),
		UserID:     claim.UserID(),
		CouponID:   claim.CouponID(),
		ClaimedAt:  claim.ClaimedAt(),
	}); err != nil {
		return pgerror.NormalizeError(err)
	}

	return nil
}

// UpdateSuspended は、suspended_at IS NULL を条件に更新し、0 行を ErrAlreadySuspended へ写します。
func (r *repository) UpdateSuspended(ctx context.Context, id uuid.UUID, suspendedAt time.Time) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	affected, err := db.UpdateCampaignSuspended(ctx, &gen.UpdateCampaignSuspendedParams{
		ID:          id,
		SuspendedAt: &suspendedAt,
	})
	if err != nil {
		return pgerror.NormalizeError(err)
	}
	if affected == 0 {
		return campaign.ErrAlreadySuspended
	}

	return nil
}

// campaignToCreateParams は、集約を挿入パラメータへ写します。
func campaignToCreateParams(c *campaign.Campaign) (*gen.CreateCampaignParams, error) {
	t := c.Template()

	totalLimit, err := safecast.IntToInt32(c.TotalLimit())
	if err != nil {
		return nil, err
	}
	perUserLimit, err := safecast.IntToInt32(c.PerUserLimit())
	if err != nil {
		return nil, err
	}
	issuedCount, err := safecast.IntToInt32(c.IssuedCount())
	if err != nil {
		return nil, err
	}

	return &gen.CreateCampaignParams{
		ID:                      c.ID(),
		Code:                    c.Code().Value(),
		DiscountKind:            t.DiscountKindName(),
		DiscountValue:           t.DiscountValue(),
		DiscountMaxAmount:       t.DiscountMaxAmount(),
		ScopeKind:               t.ScopeKindName(),
		ScopeTargetID:           t.ScopeTargetID(),
		CouponMinPurchaseAmount: t.MinPurchaseAmount(),
		CouponUsableFrom:        t.UsableFrom(),
		CouponExpiresAt:         t.ExpiresAt(),
		StartsAt:                c.StartsAt(),
		EndsAt:                  c.EndsAt(),
		TotalLimit:              totalLimit,
		PerUserLimit:            perUserLimit,
		IssuedCount:             issuedCount,
	}, nil
}

// rowToCampaign は、永続化された行からキャンペーンを再構築します。
func rowToCampaign(row gen.Campaigns) (*campaign.Campaign, error) {
	code, err := campaign.NewCode(row.Code)
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}

	template, err := campaign.NewTemplate(campaign.TemplateAttributes{
		DiscountKindName:  row.DiscountKind,
		DiscountValue:     row.DiscountValue,
		DiscountMaxAmount: row.DiscountMaxAmount,
		ScopeKindName:     row.ScopeKind,
		ScopeTargetID:     row.ScopeTargetID,
		MinPurchaseAmount: row.CouponMinPurchaseAmount,
		UsableFrom:        row.CouponUsableFrom,
		ExpiresAt:         row.CouponExpiresAt,
	})
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}

	c, err := campaign.Reconstruct(row.ID, campaign.Attributes{
		Code:         code,
		Template:     template,
		StartsAt:     row.StartsAt,
		EndsAt:       row.EndsAt,
		TotalLimit:   int(row.TotalLimit),
		PerUserLimit: int(row.PerUserLimit),
	}, int(row.IssuedCount), row.SuspendedAt)
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}

	return c, nil
}
