// Package purchase は、購入リポジトリ（purchase.Repository）の RDB 実装を提供します。
package purchase

import (
	"context"

	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

type repository struct {
	db     driver.DatabaseDriver
	tracer observability.LayerTracer
}

// New は、purchase.Repository の RDB 実装を生成して返します。
func New(
	db driver.DatabaseDriver,
	tf observability.TracerFactory,
) purchase.Repository {
	return &repository{
		db:     db,
		tracer: tf.Infra(),
	}
}

// Create は、購入本体を登録してから明細を 1 件ずつ登録します。明細は購入行の外部キー先なので
// この順序でなければ 23503 で失敗します。
func (r *repository) Create(ctx context.Context, p *purchase.Purchase) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	statusCode, err := safecast.IntToInt16(p.StatusCode())
	if err != nil {
		return xerrors.Wrap(err, "invalid purchase status code")
	}

	db := gen.New(driver.New(ctx, r.db))
	if err := db.InsertPurchase(ctx, &gen.InsertPurchaseParams{
		ID:             p.ID(),
		Code:           p.Code(),
		UserID:         p.UserID(),
		StatusCode:     statusCode,
		SubtotalAmount: int64(p.SubtotalAmount()),
		TaxAmount:      int64(p.TaxAmount()),
		ShippingFee:    int64(p.ShippingFee()),
		TotalAmount:    int64(p.TotalAmount()),
		CouponID:       p.CouponID(),
		DiscountAmount: int64(p.DiscountAmount()),
	}); err != nil {
		return pgerror.NormalizeError(err)
	}

	return r.insertDetails(ctx, db, p)
}

// FindByID は、購入本体と明細の 2 クエリでロックを取らずに読み出し、集約として再構築します
// （ロックを取る LockByCode との対）。存在しない場合は NotFound を返します。
func (r *repository) FindByID(ctx context.Context, id uuid.UUID) (*purchase.Purchase, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))

	row, err := db.GetPurchaseByID(ctx, id)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	detailRows, err := db.ListPurchaseDetailsByPurchaseID(ctx, id)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	details, err := toPurchaseDetails(detailRows)
	if err != nil {
		return nil, err
	}

	p := row.Purchases
	entity, err := purchase.Reconstruct(p.ID, purchase.Attributes{
		Code:           p.Code,
		UserID:         p.UserID,
		StatusID:       p.StatusID,
		StatusCode:     int(row.StatusCode),
		SubtotalAmount: int(p.SubtotalAmount),
		CouponID:       p.CouponID,
		DiscountAmount: int(p.DiscountAmount),
		TaxAmount:      int(p.TaxAmount),
		ShippingFee:    int(p.ShippingFee),
		TotalAmount:    int(p.TotalAmount),
		Details:        details,
		OrderedAt:      p.OrderedAt,
		PaidAt:         p.PaidAt,
		CanceledAt:     p.CanceledAt,
		ShippedAt:      p.ShippedAt,
		DeliveredAt:    p.DeliveredAt,
	})
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}
	return entity, nil
}

// LockByCode は、購入コードで引いた購入行のみ悲観ロック（FOR UPDATE OF p）して明細込みで再構築し返します。
// 存在しない場合は NotFound を返します。ロックが守る状態遷移の意味は
// internal/domain/purchase.Repository の LockByCode を参照。
func (r *repository) LockByCode(ctx context.Context, code string) (*purchase.Purchase, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))

	row, err := db.LockPurchaseByCode(ctx, code)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	detailRows, err := db.ListPurchaseDetailsByPurchaseID(ctx, row.Purchases.ID)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	details, err := toPurchaseDetails(detailRows)
	if err != nil {
		return nil, err
	}

	p := row.Purchases
	entity, err := purchase.Reconstruct(p.ID, purchase.Attributes{
		Code:           p.Code,
		UserID:         p.UserID,
		StatusID:       p.StatusID,
		StatusCode:     int(row.StatusCode),
		SubtotalAmount: int(p.SubtotalAmount),
		CouponID:       p.CouponID,
		DiscountAmount: int(p.DiscountAmount),
		TaxAmount:      int(p.TaxAmount),
		ShippingFee:    int(p.ShippingFee),
		TotalAmount:    int(p.TotalAmount),
		Details:        details,
		OrderedAt:      p.OrderedAt,
		PaidAt:         p.PaidAt,
		CanceledAt:     p.CanceledAt,
		ShippedAt:      p.ShippedAt,
		DeliveredAt:    p.DeliveredAt,
	})
	if err != nil {
		return nil, pgerror.NormalizeReconstructError(err)
	}
	return entity, nil
}

// UpdatePaid は、購入の状態更新（status_id / paid_at）を渡された tx 内で実行します。
func (r *repository) UpdatePaid(ctx context.Context, p *purchase.Purchase) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	statusCode, err := safecast.IntToInt16(p.StatusCode())
	if err != nil {
		return xerrors.Wrap(err, "invalid purchase status code")
	}

	db := gen.New(driver.New(ctx, r.db))
	if err := db.UpdatePurchasePaid(ctx, &gen.UpdatePurchasePaidParams{
		StatusCode: statusCode,
		PaidAt:     p.PaidAt(),
		ID:         p.ID(),
	}); err != nil {
		return pgerror.NormalizeError(err)
	}
	return nil
}

// UpdateShipped は、購入の状態更新（status_id / shipped_at）を渡された tx 内で実行します。
// status_id は seed の UUID を焼き込まず purchase_statuses.code から解決します（FindShippable の絞り込みも同じ方針）。
// 遷移ガードを持たない理由は docs/spec/domain/purchase.md の Repository Methods > UpdateShipped を参照
// （UpdateDelivered / UpdateCancelled も同じ）。
func (r *repository) UpdateShipped(ctx context.Context, p *purchase.Purchase) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	statusCode, err := safecast.IntToInt16(p.StatusCode())
	if err != nil {
		return xerrors.Wrap(err, "invalid purchase status code")
	}

	db := gen.New(driver.New(ctx, r.db))
	if err := db.UpdatePurchaseShipped(ctx, &gen.UpdatePurchaseShippedParams{
		StatusCode: statusCode,
		ShippedAt:  p.ShippedAt(),
		ID:         p.ID(),
	}); err != nil {
		return pgerror.NormalizeError(err)
	}
	return nil
}

// UpdateDelivered は、購入の状態更新（status_id / delivered_at）を渡された tx 内で実行します。
// status_id の解決方針・遷移ガードの扱いは UpdateShipped を参照。
func (r *repository) UpdateDelivered(ctx context.Context, p *purchase.Purchase) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	statusCode, err := safecast.IntToInt16(p.StatusCode())
	if err != nil {
		return xerrors.Wrap(err, "invalid purchase status code")
	}

	db := gen.New(driver.New(ctx, r.db))
	if err := db.UpdatePurchaseDelivered(ctx, &gen.UpdatePurchaseDeliveredParams{
		StatusCode:  statusCode,
		DeliveredAt: p.DeliveredAt(),
		ID:          p.ID(),
	}); err != nil {
		return pgerror.NormalizeError(err)
	}
	return nil
}

// UpdateCancelled は、購入の状態更新（status_id / canceled_at）を渡された tx 内で実行します。
// status_id の解決方針・遷移ガードの扱いは UpdateShipped を参照。
func (r *repository) UpdateCancelled(ctx context.Context, p *purchase.Purchase) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	statusCode, err := safecast.IntToInt16(p.StatusCode())
	if err != nil {
		return xerrors.Wrap(err, "invalid purchase status code")
	}

	db := gen.New(driver.New(ctx, r.db))
	if err := db.UpdatePurchaseCanceled(ctx, &gen.UpdatePurchaseCanceledParams{
		StatusCode: statusCode,
		CanceledAt: p.CanceledAt(),
		ID:         p.ID(),
	}); err != nil {
		return pgerror.NormalizeError(err)
	}
	return nil
}

// FindShippable は、ListShippablePurchases と明細の一括取得（ListPurchaseDetailsByPurchaseIDs）の
// 2 クエリで構成し、件数分の往復を避けます。status_id の絞り込み方針は UpdateShipped を参照。
func (r *repository) FindShippable(ctx context.Context, limit int32) (purchase.Purchases, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	statusCode, err := safecast.IntToInt16(purchase.StatusPaid.Code())
	if err != nil {
		return nil, xerrors.Wrap(err, "invalid purchase status code")
	}

	db := gen.New(driver.New(ctx, r.db))

	rows, err := db.ListShippablePurchases(ctx, &gen.ListShippablePurchasesParams{
		StatusCode: statusCode,
		LimitParam: limit,
	})
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}
	if len(rows) == 0 {
		return purchase.Purchases{}, nil
	}

	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.Purchases.ID
	}

	detailRows, err := db.ListPurchaseDetailsByPurchaseIDs(ctx, ids)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	detailsByPurchaseID, err := groupPurchaseDetails(detailRows)
	if err != nil {
		return nil, err
	}

	purchases := make(purchase.Purchases, len(rows))
	for i, row := range rows {
		p := row.Purchases
		entity, rerr := purchase.Reconstruct(p.ID, purchase.Attributes{
			Code:           p.Code,
			UserID:         p.UserID,
			StatusID:       p.StatusID,
			StatusCode:     int(row.StatusCode),
			SubtotalAmount: int(p.SubtotalAmount),
			CouponID:       p.CouponID,
			DiscountAmount: int(p.DiscountAmount),
			TaxAmount:      int(p.TaxAmount),
			ShippingFee:    int(p.ShippingFee),
			TotalAmount:    int(p.TotalAmount),
			Details:        detailsByPurchaseID[p.ID],
			OrderedAt:      p.OrderedAt,
			PaidAt:         p.PaidAt,
			CanceledAt:     p.CanceledAt,
			ShippedAt:      p.ShippedAt,
			DeliveredAt:    p.DeliveredAt,
		})
		if rerr != nil {
			return nil, pgerror.NormalizeReconstructError(rerr)
		}
		purchases[i] = entity
	}
	return purchases, nil
}

// groupPurchaseDetails は、複数購入分の明細行を購入 ID ごとにまとめます。
// 各購入内の並びは取得順（明細 ID 昇順）を保ちます。
func groupPurchaseDetails(
	detailRows []*gen.ListPurchaseDetailsByPurchaseIDsRow,
) (map[uuid.UUID][]purchase.PurchaseDetail, error) {
	grouped := make(map[uuid.UUID][]purchase.PurchaseDetail)
	for _, dr := range detailRows {
		d := dr.PurchaseDetails
		detail, err := toPurchaseDetail(d)
		if err != nil {
			return nil, err
		}
		grouped[d.PurchaseID] = append(grouped[d.PurchaseID], detail)
	}
	return grouped, nil
}

// FindDetailByID は、購入ステータスマスタとの結合でステータス名を解決します。
func (r *repository) FindDetailByID(ctx context.Context, id uuid.UUID) (*purchase.Detail, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))

	row, err := db.GetPurchaseDetailByID(ctx, id)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	detailRows, err := db.ListPurchaseDetailsByPurchaseID(ctx, id)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	details, err := toPurchaseDetails(detailRows)
	if err != nil {
		return nil, err
	}

	return &purchase.Detail{
		ID:             row.ID,
		Code:           row.Code,
		UserID:         row.UserID,
		StatusID:       row.StatusID,
		StatusCode:     int(row.StatusCode),
		StatusName:     row.StatusName,
		SubtotalAmount: int(row.SubtotalAmount),
		CouponID:       row.CouponID,
		DiscountAmount: int(row.DiscountAmount),
		TaxAmount:      int(row.TaxAmount),
		ShippingFee:    int(row.ShippingFee),
		TotalAmount:    int(row.TotalAmount),
		Details:        details,
		OrderedAt:      row.OrderedAt,
		PaidAt:         row.PaidAt,
		CanceledAt:     row.CanceledAt,
		ShippedAt:      row.ShippedAt,
		DeliveredAt:    row.DeliveredAt,
	}, nil
}

// toPurchaseDetail は、明細行 1 件を購入明細の値オブジェクトへ変換します。単価は価格スケール（ドル decimal）です。
func toPurchaseDetail(d gen.PurchaseDetails) (purchase.PurchaseDetail, error) {
	unitPrice, err := money.NewPrice(d.UnitPrice)
	if err != nil {
		return purchase.PurchaseDetail{}, pgerror.NormalizeReconstructError(err)
	}
	return purchase.NewPurchaseDetail(d.ID, purchase.PurchaseDetailAttributes{
		ProductID: d.ProductID,
		Quantity:  int(d.Quantity),
		UnitPrice: unitPrice,
	}), nil
}

// toPurchaseDetails は、購入 1 件分の明細行を購入明細の値オブジェクトへ変換します。
func toPurchaseDetails(detailRows []*gen.ListPurchaseDetailsByPurchaseIDRow) ([]purchase.PurchaseDetail, error) {
	details := make([]purchase.PurchaseDetail, len(detailRows))
	for i, dr := range detailRows {
		detail, err := toPurchaseDetail(dr.PurchaseDetails)
		if err != nil {
			return nil, err
		}
		details[i] = detail
	}
	return details, nil
}

// FindStatusesByUserID は、code を purchase_statuses との結合で解決してドメインの値へ復元します。
// 絞り込み方針は internal/domain/purchase.Repository の FindStatusesByUserID を参照。
func (r *repository) FindStatusesByUserID(ctx context.Context, userID uuid.UUID) ([]purchase.Status, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	codes, err := db.SelectPurchaseStatusCodesByUserID(ctx, userID)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}

	statuses := make([]purchase.Status, len(codes))
	for i, code := range codes {
		status, serr := purchase.NewStatus(int(code))
		if serr != nil {
			return nil, serr
		}
		statuses[i] = status
	}
	return statuses, nil
}

// FindUserIDsWithPurchases は、purchases に 1 件以上の行を持つ user_id を重複排除して返します。
// users とは結合しません（理由は docs/spec/domain/purchase.md の FindUserIDsWithPurchases）。
func (r *repository) FindUserIDsWithPurchases(ctx context.Context, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	db := gen.New(driver.New(ctx, r.db))
	ids, err := db.ListUserIDsWithPurchases(ctx, userIDs)
	if err != nil {
		return nil, pgerror.NormalizeError(err)
	}
	return ids, nil
}

// insertDetails は、p が保持する明細を登録します。
func (r *repository) insertDetails(ctx context.Context, db *gen.Queries, p *purchase.Purchase) error {
	for _, d := range p.Details() {
		quantity, err := safecast.IntToInt32(d.Quantity())
		if err != nil {
			return xerrors.Wrap(err, "invalid purchase detail quantity")
		}

		if err := db.InsertPurchaseDetail(ctx, &gen.InsertPurchaseDetailParams{
			ID:         d.ID(),
			PurchaseID: p.ID(),
			ProductID:  d.ProductID(),
			Quantity:   quantity,
			UnitPrice:  d.UnitPrice().Decimal(),
		}); err != nil {
			return pgerror.NormalizeError(err)
		}
	}

	return nil
}
