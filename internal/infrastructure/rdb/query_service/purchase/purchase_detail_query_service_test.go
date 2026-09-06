package purchase

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	domainpurchase "go-boilerplate/internal/domain/purchase"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/observability"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 既存 seed 由来の FK 対象。
const (
	seedStatusInStock  = "093170fb-83a2-4864-a2b3-53236eaf3597" // 商品ステータス（在庫あり）
	seedCategory       = "5dd52d84-78eb-4a52-ba0b-2e11c95c2af2" // 商品カテゴリ
	seedUserA          = "550e8400-e29b-41d4-a716-446655440000" // 購入所有者
	seedUserB          = "a95a2dd3-2b37-4def-8041-23d2138faccc" // 別ユーザー（他人）
	seedUnprocessedSID = "a66c996c-86b2-41d8-9bdd-9b685fb7c47d" // 購入ステータス（未処理）
	seedPaidSID        = "4b8f0e2a-1c3d-4a5e-8b7f-2d9c0e1a3b4c" // 購入ステータス（支払い済み）
	seedCanceledSID    = "e9d72547-adfe-48d9-9037-bd1f55d4158b" // 購入ステータス（キャンセル）
)

func mustParse(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err)
	return id
}

// insertProduct は、FK を満たす商品を挿入し商品 ID を返します。
func insertProduct(ctx context.Context, t *testing.T, db driver.DBTX, seed, name string, price int) uuid.UUID {
	t.Helper()
	productID := mustParse(t, seed)
	_, err := db.Exec(ctx,
		"INSERT INTO products (id, name, description, price, quantity, stock_warning_threshold, status_id, category_id, published_at) "+
			"VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())",
		productID, name, nil, price, 20, nil, mustParse(t, seedStatusInStock), mustParse(t, seedCategory),
	)
	require.NoError(t, err)
	return productID
}

// insertPurchase は、指定ユーザー・ステータスの購入を挿入し購入 ID を返します。
// paidAt / canceledAt が非 nil の場合はそれぞれ paid_at / canceled_at をセットします。
func insertPurchase(
	ctx context.Context,
	t *testing.T,
	db driver.DBTX,
	userID, statusID uuid.UUID,
	code string,
	paidAt, canceledAt *time.Time,
) uuid.UUID {
	t.Helper()
	purchaseID, err := uuid.New()
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		"INSERT INTO purchases (id, code, user_id, status_id, subtotal_amount, tax_amount, shipping_fee, total_amount, paid_at, canceled_at) "+
			"VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
		purchaseID, code, userID, statusID, 160000, 16000, 500, 176500, paidAt, canceledAt,
	)
	require.NoError(t, err)
	return purchaseID
}

// insertCoupon は、全体スコープ・定率のクーポンを 1 件挿入し ID を返します。
func insertCoupon(ctx context.Context, t *testing.T, db driver.DBTX, seed string, userID uuid.UUID) uuid.UUID {
	t.Helper()
	couponID := mustParse(t, seed)
	_, err := db.Exec(ctx,
		"INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, scope_target_id, expires_at, issued_at) "+
			"VALUES ($1,$2,$3,$4,$5,NULL,NOW() + INTERVAL '30 days',NOW() - INTERVAL '1 days')",
		couponID, userID,
		domaincoupon.DiscountKindRate.Code(), "0.10", domaincoupon.ScopeKindAll.Code(),
	)
	require.NoError(t, err)
	return couponID
}

// applyCouponToPurchase は、挿入済みの購入へクーポンの適用結果を書き込みます。
func applyCouponToPurchase(
	ctx context.Context, t *testing.T, db driver.DBTX, purchaseID, couponID uuid.UUID, discount int,
) {
	t.Helper()
	_, err := db.Exec(ctx,
		"UPDATE purchases SET coupon_id = $2, discount_amount = $3, tax_amount = $4, total_amount = $5 WHERE id = $1",
		purchaseID, couponID, discount, 14400, 158900,
	)
	require.NoError(t, err)
}

// insertDetail は、購入明細を 1 件挿入します。
func insertDetail(ctx context.Context, t *testing.T, db driver.DBTX, purchaseID, productID uuid.UUID, quantity, unitPrice int) {
	t.Helper()
	detailID, err := uuid.New()
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		"INSERT INTO purchase_details (id, purchase_id, product_id, quantity, unit_price) VALUES ($1,$2,$3,$4,$5)",
		detailID, purchaseID, productID, quantity, unitPrice,
	)
	require.NoError(t, err)
}

func Test_service_FindDetailByUserAndCode(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)
	svc := &service{tracer: lt, db: testDB}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("本人の購入を明細と商品名の結合込みで取得する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				userA := mustParse(t, seedUserA)
				productA := insertProduct(ctx, t, drv, "e1000000-0000-4000-8000-000000000001", "商品A", 80000)
				productB := insertProduct(ctx, t, drv, "e1000000-0000-4000-8000-000000000002", "商品B", 120000)
				const purchaseCode = "qs-code-ok"
				purchaseID := insertPurchase(ctx, t, drv, userA, mustParse(t, seedUnprocessedSID), purchaseCode, nil, nil)
				// 明細 2 件を 1 回のリスト取得で束ねられること（固定 2 クエリ・N+1 でない）を確認する。
				insertDetail(ctx, t, drv, purchaseID, productA, 2, 800)
				insertDetail(ctx, t, drv, purchaseID, productB, 1, 1500)

				got, err := svc.FindDetailByUserAndCode(ctx, userA, purchaseCode)
				require.NoError(t, err)
				assert.Equal(t, purchaseID, got.ID)
				assert.Equal(t, userA, got.UserID)
				assert.Equal(t, mustParse(t, seedUnprocessedSID), got.StatusID)
				assert.Equal(t, "未処理", got.StatusName)
				assert.Equal(t, domainpurchase.StatusUnprocessed.Code(), got.StatusCode)
				assert.Equal(t, int64(160000), got.SubtotalAmount)
				assert.Equal(t, int64(16000), got.TaxAmount)
				assert.Equal(t, int64(500), got.ShippingFee)
				assert.Equal(t, int64(176500), got.TotalAmount)
				assert.Nil(t, got.PaidAt)
				assert.Nil(t, got.CanceledAt)
				// クーポン未適用の行では LEFT JOIN が全列 NULL を返す。
				assert.Zero(t, got.DiscountAmount)
				assert.Nil(t, got.AppliedCoupon)

				require.Len(t, got.Items, 2)
				assert.Equal(t, productA, got.Items[0].ProductID)
				assert.Equal(t, "商品A", got.Items[0].ProductName)
				assert.Equal(t, 2, got.Items[0].Quantity)
				assert.Equal(t, "800", got.Items[0].UnitPrice.String())
				assert.Equal(t, productB, got.Items[1].ProductID)
				assert.Equal(t, "商品B", got.Items[1].ProductName)
				assert.Equal(t, "1500", got.Items[1].UnitPrice.String())
			})
		})

		t.Run("クーポン適用済み購入は値引き額と適用クーポンを結合込みで返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				userA := mustParse(t, seedUserA)
				productA := insertProduct(ctx, t, drv, "e5000000-0000-4000-8000-000000000001", "商品K", 80000)
				const purchaseCode = "qs-code-coupon"
				purchaseID := insertPurchase(ctx, t, drv, userA, mustParse(t, seedUnprocessedSID), purchaseCode, nil, nil)
				insertDetail(ctx, t, drv, purchaseID, productA, 2, 800)
				couponID := insertCoupon(ctx, t, drv, "e5000000-0000-4000-8000-0000000000c1", userA)
				applyCouponToPurchase(ctx, t, drv, purchaseID, couponID, 16000)

				got, err := svc.FindDetailByUserAndCode(ctx, userA, purchaseCode)

				require.NoError(t, err)
				assert.Equal(t, int64(16000), got.DiscountAmount)
				require.NotNil(t, got.AppliedCoupon)
				assert.Equal(t, couponID, got.AppliedCoupon.ID)
				assert.Equal(t, domaincoupon.DiscountKindRate.Code(), got.AppliedCoupon.DiscountKind)
				assert.Equal(t, "0.1", got.AppliedCoupon.DiscountValue.String())
				assert.Equal(t, domaincoupon.ScopeKindAll.Code(), got.AppliedCoupon.ScopeKind)
				assert.Nil(t, got.AppliedCoupon.ScopeTargetID)
			})
		})

		t.Run("支払い済み購入はpaidAtを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				userA := mustParse(t, seedUserA)
				productA := insertProduct(ctx, t, drv, "e2000000-0000-4000-8000-000000000001", "商品P", 80000)
				paidAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
				const purchaseCode = "qs-code-paid"
				purchaseID := insertPurchase(ctx, t, drv, userA, mustParse(t, seedPaidSID), purchaseCode, &paidAt, nil)
				insertDetail(ctx, t, drv, purchaseID, productA, 1, 800)

				got, err := svc.FindDetailByUserAndCode(ctx, userA, purchaseCode)
				require.NoError(t, err)
				assert.Equal(t, "支払い済み", got.StatusName)
				assert.Equal(t, domainpurchase.StatusPaid.Code(), got.StatusCode)
				require.NotNil(t, got.PaidAt)
				assert.True(t, paidAt.Equal(*got.PaidAt))
				assert.Nil(t, got.CanceledAt)
			})
		})

		t.Run("キャンセル済み購入はcanceledAtを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				userA := mustParse(t, seedUserA)
				productA := insertProduct(ctx, t, drv, "e4000000-0000-4000-8000-000000000001", "商品C", 80000)
				canceledAt := time.Date(2026, time.July, 26, 9, 0, 0, 0, time.UTC)
				const purchaseCode = "qs-code-canceled"
				purchaseID := insertPurchase(ctx, t, drv, userA, mustParse(t, seedCanceledSID), purchaseCode, nil, &canceledAt)
				insertDetail(ctx, t, drv, purchaseID, productA, 1, 800)

				got, err := svc.FindDetailByUserAndCode(ctx, userA, purchaseCode)
				require.NoError(t, err)
				assert.Equal(t, "キャンセル", got.StatusName)
				assert.Equal(t, domainpurchase.StatusCanceled.Code(), got.StatusCode)
				assert.Nil(t, got.PaidAt)
				require.NotNil(t, got.CanceledAt)
				assert.True(t, canceledAt.Equal(*got.CanceledAt))
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("他人の購入IDはNotFoundを返し存在を秘匿する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				userB := mustParse(t, seedUserB)
				productA := insertProduct(ctx, t, drv, "e3000000-0000-4000-8000-000000000001", "商品O", 80000)
				// 購入は userB が所有する。userA で問い合わせると所有権述語で 0 行になる。
				const purchaseCode = "qs-code-other"
				purchaseID := insertPurchase(ctx, t, drv, userB, mustParse(t, seedUnprocessedSID), purchaseCode, nil, nil)
				insertDetail(ctx, t, drv, purchaseID, productA, 1, 800)

				_, err := svc.FindDetailByUserAndCode(ctx, mustParse(t, seedUserA), purchaseCode)
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})

		t.Run("存在しない購入IDはNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				_, ferr := svc.FindDetailByUserAndCode(ctx, mustParse(t, seedUserA), "qs-code-missing")
				require.ErrorIs(t, ferr, apperror.ErrNotFound)
			})
		})
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("依存を注入したクエリサービス実装を生成する", func(t *testing.T) {
			t.Parallel()

			testDB := testkit.NewTestDB(t)
			tf := observability.NewNoopTracerFactory(t)

			svc, ok := New(testDB, tf).(*service)
			require.True(t, ok)
			assert.Equal(t, testDB, svc.db)
			assert.NotNil(t, svc.tracer)
		})
	})
}

func Test_toPurchaseDetailItems(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細行を商品名込みの読み取りモデルへ写像する", func(t *testing.T) {
			t.Parallel()

			rows := []*gen.ListPurchaseDetailItemsForUserRow{
				{ProductID: uuidtestkit.NewTestFromSalt(t, "ti_p1"), ProductName: "商品A", Quantity: 2, UnitPrice: decimaltestkit.MustParse(t, "800")},
				{ProductID: uuidtestkit.NewTestFromSalt(t, "ti_p2"), ProductName: "商品B", Quantity: 1, UnitPrice: decimaltestkit.MustParse(t, "1500")},
			}

			items, err := toPurchaseDetailItems(rows)
			require.NoError(t, err)
			require.Len(t, items, 2)
			assert.Equal(t, "商品A", items[0].ProductName)
			assert.Equal(t, 2, items[0].Quantity)
			assert.Equal(t, "800", items[0].UnitPrice.String())
			assert.Equal(t, "商品B", items[1].ProductName)
			assert.Equal(t, "1500", items[1].UnitPrice.String())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("単価が負の行はErrInternalへ正規化する", func(t *testing.T) {
			t.Parallel()

			rows := []*gen.ListPurchaseDetailItemsForUserRow{
				{ProductID: uuidtestkit.NewTestFromSalt(t, "ti_neg"), ProductName: "商品N", Quantity: 1, UnitPrice: decimaltestkit.MustParse(t, "-1")},
			}

			_, err := toPurchaseDetailItems(rows)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})
	})
}

func Test_toAppliedCoupon(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("結合で解決したクーポンを読み取りモデルへ写す", func(t *testing.T) {
			t.Parallel()

			couponID := uuidtestkit.NewTestFromSalt(t, "qs_coupon")
			target := uuidtestkit.NewTestFromSalt(t, "qs_category")
			value := decimaltestkit.MustParse(t, "0.10")
			kind, scopeKind := int16(2), int16(2)

			got := toAppliedCoupon(&gen.GetPurchaseDetailForUserRow{
				CouponID:            &couponID,
				CouponDiscountKind:  &kind,
				CouponDiscountValue: &value,
				CouponScopeKind:     &scopeKind,
				CouponScopeTargetID: &target,
			})

			require.NotNil(t, got)
			assert.Equal(t, couponID, got.ID)
			assert.Equal(t, 2, got.DiscountKind)
			assert.True(t, value.Equal(got.DiscountValue))
			assert.Equal(t, 2, got.ScopeKind)
			require.NotNil(t, got.ScopeTargetID)
			assert.Equal(t, target, *got.ScopeTargetID)
		})

		t.Run("クーポンを適用していない購入はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, toAppliedCoupon(&gen.GetPurchaseDetailForUserRow{}))
		})
	})
}
