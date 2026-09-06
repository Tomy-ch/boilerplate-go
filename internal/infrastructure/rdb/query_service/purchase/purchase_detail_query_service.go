// Package purchase は、購入詳細クエリサービス（query.PurchaseDetailQueryService）の RDB 実装を提供します。
package purchase

import (
	"context"

	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/purchase/query"
	"go-boilerplate/pkg/uuid"
)

type service struct {
	db     driver.DatabaseDriver
	tracer observability.LayerTracer
}

// New は、購入詳細クエリサービスの RDB 実装を生成して返します。
func New(
	db driver.DatabaseDriver,
	tf observability.TracerFactory,
) query.PurchaseDetailQueryService {
	return &service{
		db:     db,
		tracer: tf.Infra(),
	}
}

// FindDetailByUserAndCode は、GetPurchaseDetailForUser と ListPurchaseDetailItemsForUser の
// 固定 2 クエリで構成します。所有権と 0 行の扱いは docs/spec/usecase/purchase.md の GET 詳細を参照。
func (s *service) FindDetailByUserAndCode(ctx context.Context, userID uuid.UUID, code string) (*query.PurchaseDetailReadModel, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, s.db))

	row, err := db.GetPurchaseDetailForUser(ctx, &gen.GetPurchaseDetailForUserParams{
		Code:   code,
		UserID: userID,
	})
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	itemRows, err := db.ListPurchaseDetailItemsForUser(ctx, row.ID)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	items, err := toPurchaseDetailItems(itemRows)
	if err != nil {
		return nil, err
	}

	return &query.PurchaseDetailReadModel{
		ID:             row.ID,
		Code:           row.Code,
		UserID:         row.UserID,
		StatusID:       row.StatusID,
		StatusCode:     int(row.StatusCode),
		StatusName:     row.StatusName,
		SubtotalAmount: row.SubtotalAmount,
		DiscountAmount: row.DiscountAmount,
		AppliedCoupon:  toAppliedCoupon(row),
		TaxAmount:      row.TaxAmount,
		ShippingFee:    row.ShippingFee,
		TotalAmount:    row.TotalAmount,
		Items:          items,
		OrderedAt:      row.OrderedAt,
		PaidAt:         row.PaidAt,
		CanceledAt:     row.CanceledAt,
	}, nil
}

// toAppliedCoupon は、結合で解決したクーポンの 2 軸を読み取りモデルへ写します。
// クーポンを適用していない購入は結合先が無いため nil を返します。
func toAppliedCoupon(row *gen.GetPurchaseDetailForUserRow) *query.AppliedCouponReadModel {
	if row.CouponID == nil {
		return nil
	}

	return &query.AppliedCouponReadModel{
		ID:            *row.CouponID,
		DiscountKind:  int(*row.CouponDiscountKind),
		DiscountValue: *row.CouponDiscountValue,
		ScopeKind:     int(*row.CouponScopeKind),
		ScopeTargetID: row.CouponScopeTargetID,
	}
}

// toPurchaseDetailItems は、明細行を読み取りモデルへ変換します。単価は価格スケール（ドル decimal）で、
// 値オブジェクト再構築に失敗した行は内部エラーへ正規化します。
func toPurchaseDetailItems(rows []*gen.ListPurchaseDetailItemsForUserRow) ([]query.PurchaseDetailItem, error) {
	items := make([]query.PurchaseDetailItem, len(rows))
	for i, r := range rows {
		unitPrice, perr := money.NewPrice(r.UnitPrice)
		if perr != nil {
			return nil, pgerror.NormalizeReconstructError(perr)
		}
		items[i] = query.PurchaseDetailItem{
			ProductID:   r.ProductID,
			ProductName: r.ProductName,
			Quantity:    int(r.Quantity),
			UnitPrice:   unitPrice,
		}
	}
	return items, nil
}
