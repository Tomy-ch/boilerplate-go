// Package coupon は、クーポンリポジトリ（coupon.Repository）の RDB 実装を提供します。
package coupon

import (
	"context"
	"time"

	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/uuid"
)

type repository struct {
	db     driver.DatabaseDriver
	tracer observability.LayerTracer
}

// New は、coupon.Repository の RDB 実装を生成して返します。
func New(
	db driver.DatabaseDriver,
	tf observability.TracerFactory,
) coupon.Repository {
	return &repository{
		db:     db,
		tracer: tf.Infra(),
	}
}

// FindByUserID は、発行日時の降順で取得します。
func (r *repository) FindByUserID(ctx context.Context, userID uuid.UUID) (coupon.Coupons, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	rows, err := db.ListCouponsByUserID(ctx, userID)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	coupons := make(coupon.Coupons, len(rows))
	for i, row := range rows {
		c, cerr := rowToCoupon(row.Coupons)
		if cerr != nil {
			return nil, cerr
		}
		coupons[i] = c
	}

	return coupons, nil
}

// LockByID は、悲観ロック（FOR UPDATE）で取得し、0 行は NotFound へ正規化します。
func (r *repository) LockByID(ctx context.Context, id uuid.UUID) (*coupon.Coupon, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	row, err := db.LockCouponByID(ctx, id)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	return rowToCoupon(row.Coupons)
}

// UpdateUsed は、used_at IS NULL を条件に更新し、0 行を ErrUsedConcurrently へ写します。
// 0 行を NotFound へ正規化しない理由は docs/spec/domain/coupon.md の Repository Methods > UpdateUsed を参照。
func (r *repository) UpdateUsed(ctx context.Context, id uuid.UUID, usedAt time.Time) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	affected, err := db.UpdateCouponUsed(ctx, &gen.UpdateCouponUsedParams{
		ID:     id,
		UsedAt: &usedAt,
	})
	if err != nil {
		return pgerror.NormalizeError(err)
	}
	if affected == 0 {
		return coupon.ErrUsedConcurrently
	}

	return nil
}

// rowToCoupon は、永続化された行からクーポンを再構築します。
func rowToCoupon(row gen.Coupons) (*coupon.Coupon, error) {
	discountKind, err := coupon.NewDiscountKind(int(row.DiscountKind))
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}
	discount, err := coupon.ReconstructDiscount(discountKind, row.DiscountValue)
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}

	scopeKind, err := coupon.NewScopeKind(int(row.ScopeKind))
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}
	scope, err := coupon.ReconstructScope(scopeKind, row.ScopeTargetID)
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}

	c, err := coupon.Reconstruct(row.ID, coupon.Attributes{
		UserID:    row.UserID,
		Discount:  discount,
		Scope:     scope,
		ExpiresAt: row.ExpiresAt,
		IssuedAt:  row.IssuedAt,
	}, row.UsedAt)
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}

	return c, nil
}
