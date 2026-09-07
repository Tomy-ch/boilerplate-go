// Package coupon は、販促クーポン一括発行のコマンドサービス（command.CommandService）の RDB 実装を
// 提供します。
package coupon

import (
	"context"

	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/coupon/command"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
)

type commandService struct {
	db     driver.DatabaseDriver
	tracer observability.LayerTracer
}

// New は、販促クーポン一括発行のコマンドサービスの RDB 実装を生成して返します。
func New(
	db driver.DatabaseDriver,
	tf observability.TracerFactory,
) command.CommandService {
	return &commandService{
		db:     db,
		tracer: tf.Infra(),
	}
}

// IssuePromotionalCoupons は、退会していないすべての利用者へクーポンを一括発行します。
//
// 分割の正当性・往復回数・行構築の契約は docs/spec/usecase/coupon.md の Command Service と
// ADR-0114 (predicate-defined-set-writes-on-commandservice) を参照。
//
// 空配列を unnest しても 0 行ですが、無駄な往復を避けるため受給者が 0 人なら早期に返します。
func (s *commandService) IssuePromotionalCoupons(
	ctx context.Context,
	params command.IssuePromotionalCouponsParams,
) (command.IssuePromotionalCouponsResult, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, s.db))

	recipients, err := db.SelectBulkIssueRecipients(ctx)
	if err != nil {
		return command.IssuePromotionalCouponsResult{}, pgerror.NormalizeError(err)
	}
	if len(recipients) == 0 {
		return command.IssuePromotionalCouponsResult{}, nil
	}

	coupons := make(coupon.Coupons, len(recipients))
	ids := make([]uuid.UUID, len(recipients))
	for i, recipient := range recipients {
		id, ierr := uuid.New()
		if ierr != nil {
			return command.IssuePromotionalCouponsResult{}, ierr
		}

		issuedCoupon, cerr := coupon.New(id, coupon.Attributes{
			UserID:    recipient,
			Discount:  params.Discount,
			Scope:     params.Scope,
			ExpiresAt: params.ExpiresAt,
			IssuedAt:  params.IssuedAt,
		})
		if cerr != nil {
			return command.IssuePromotionalCouponsResult{}, cerr
		}

		coupons[i] = issuedCoupon
		ids[i] = issuedCoupon.ID()
	}

	// 共有列は検証済みの集約から取ります。素の params は使いません
	// （理由は internal/infrastructure/rdb/README.md の command_service を参照）。
	template := coupons[0]

	discountKind, err := safecast.IntToInt16(template.Discount().Kind().Code())
	if err != nil {
		return command.IssuePromotionalCouponsResult{}, err
	}
	scopeKind, err := safecast.IntToInt16(template.Scope().Kind().Code())
	if err != nil {
		return command.IssuePromotionalCouponsResult{}, err
	}

	issued, err := db.InsertCoupons(ctx, &gen.InsertCouponsParams{
		Ids:           ids,
		UserIds:       recipients,
		DiscountKind:  discountKind,
		DiscountValue: template.Discount().Value(),
		ScopeKind:     scopeKind,
		ScopeTargetID: template.Scope().TargetID(),
		ExpiresAt:     template.ExpiresAt(),
		IssuedAt:      template.IssuedAt(),
	})
	if err != nil {
		return command.IssuePromotionalCouponsResult{}, pgerror.NormalizeError(err)
	}

	return command.IssuePromotionalCouponsResult{
		RecipientCount:    int64(len(recipients)),
		IssuedCouponCount: issued,
	}, nil
}
