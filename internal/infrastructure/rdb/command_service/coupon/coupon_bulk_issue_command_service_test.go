package coupon

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/coupon/command"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 既存 seed の FK 対象。
const seedPrefecture = "a03aaec4-3bd6-4bfb-8e47-2fbfa026d344"

// insertUser は、受給者となるユーザーを挿入します。
// deleted が true のときは退会済みとして挿入し、受給者から外れることを確かめられるようにします。
func insertUser(ctx context.Context, t *testing.T, db driver.DBTX, id uuid.UUID, deleted bool) {
	t.Helper()
	deletedAt := "NULL"
	if deleted {
		deletedAt = "NOW()"
	}
	_, err := db.Exec(ctx,
		"INSERT INTO users "+
			"(id, first_name, last_name, email, phone, prefecture_id, city, street, postal_code, deleted_at) "+
			"VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,"+deletedAt+")",
		id, "Bulk", "Recipient", "bulk-"+id.String()+"@example.com", "0000000000",
		seedPrefecture, "City", "Street", "000-0000",
	)
	require.NoError(t, err)
}

// countActiveUsers は、退会していない利用者の数を返します。seed を含む母集団なので、
// 発行結果は必ずこの値との相対で確かめます。
func countActiveUsers(ctx context.Context, t *testing.T, db driver.DBTX) int64 {
	t.Helper()
	var count int64
	row := db.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE deleted_at IS NULL")
	require.NoError(t, row.Scan(&count))

	return count
}

// newBulkIssueParams は、既定の発行条件（定率 15% / 全体 / 30 日有効）を組み立てます。
func newBulkIssueParams(t *testing.T) command.IssuePromotionalCouponsParams {
	t.Helper()
	discount, err := domaincoupon.NewRateDiscount(decimaltestkit.MustParse(t, "0.15"))
	require.NoError(t, err)

	// timestamptz はマイクロ秒までしか保たないため、往復で一致させるにはそこへ丸めて渡す。
	issuedAt := time.Now().Truncate(time.Microsecond)

	return command.IssuePromotionalCouponsParams{
		Scope:     domaincoupon.NewAllScope(),
		Discount:  discount,
		ExpiresAt: issuedAt.Add(30 * 24 * time.Hour),
		IssuedAt:  issuedAt,
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("依存を注入したコマンドサービス実装を生成する", func(t *testing.T) {
			t.Parallel()

			testDB := testkit.NewTestDB(t)

			svc, ok := New(testDB, observability.NewNoopTracerFactory(t)).(*commandService)
			require.True(t, ok)
			assert.Equal(t, testDB, svc.db)
			assert.NotNil(t, svc.tracer)
		})
	})
}

func Test_commandService_IssuePromotionalCoupons(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	svc := &commandService{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	countCoupons := func(ctx context.Context, t *testing.T, db driver.DBTX) int {
		t.Helper()
		var count int
		row := db.QueryRow(ctx, "SELECT COUNT(*) FROM coupons")
		require.NoError(t, row.Scan(&count))

		return count
	}

	countCouponsOf := func(ctx context.Context, t *testing.T, db driver.DBTX, userID uuid.UUID) int {
		t.Helper()
		var count int
		row := db.QueryRow(ctx, "SELECT COUNT(*) FROM coupons WHERE user_id = $1", userID)
		require.NoError(t, row.Scan(&count))

		return count
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("退会していない利用者へ1人1枚を発行し件数を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				userA := uuidtestkit.NewTestFromSalt(t, "bulk_cs_user_a")
				userB := uuidtestkit.NewTestFromSalt(t, "bulk_cs_user_b")
				insertUser(ctx, t, drv, userA, false)
				insertUser(ctx, t, drv, userB, false)

				want := countActiveUsers(ctx, t, drv)

				got, err := svc.IssuePromotionalCoupons(ctx, newBulkIssueParams(t))

				require.NoError(t, err)
				assert.Equal(t, want, got.RecipientCount)
				assert.Equal(t, want, got.IssuedCouponCount)
				assert.Equal(t, 1, countCouponsOf(ctx, t, drv, userA))
				assert.Equal(t, 1, countCouponsOf(ctx, t, drv, userB))
			})
		})

		t.Run("退会済みの利用者は受給者にならない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				active := uuidtestkit.NewTestFromSalt(t, "bulk_cs_active")
				withdrawn := uuidtestkit.NewTestFromSalt(t, "bulk_cs_withdrawn")
				insertUser(ctx, t, drv, active, false)
				insertUser(ctx, t, drv, withdrawn, true)

				want := countActiveUsers(ctx, t, drv)

				got, err := svc.IssuePromotionalCoupons(ctx, newBulkIssueParams(t))

				require.NoError(t, err)
				assert.Equal(t, want, got.RecipientCount)
				assert.Equal(t, 1, countCouponsOf(ctx, t, drv, active))
				assert.Equal(t, 0, countCouponsOf(ctx, t, drv, withdrawn))
			})
		})

		t.Run("発行したクーポンは条件を共有しつつ受給者ごとに別のidを持つ", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				userA := uuidtestkit.NewTestFromSalt(t, "bulk_cs_shape_a")
				userB := uuidtestkit.NewTestFromSalt(t, "bulk_cs_shape_b")
				insertUser(ctx, t, drv, userA, false)
				insertUser(ctx, t, drv, userB, false)

				params := newBulkIssueParams(t)
				_, err := svc.IssuePromotionalCoupons(ctx, params)
				require.NoError(t, err)

				var (
					idA, idB                  uuid.UUID
					discountKindA, scopeKindA int16
					expiresAtA                time.Time
				)
				row := drv.QueryRow(ctx,
					"SELECT id, discount_kind, scope_kind, expires_at FROM coupons WHERE user_id = $1", userA)
				require.NoError(t, row.Scan(&idA, &discountKindA, &scopeKindA, &expiresAtA))
				row = drv.QueryRow(ctx, "SELECT id FROM coupons WHERE user_id = $1", userB)
				require.NoError(t, row.Scan(&idB))

				assert.NotEqual(t, idA, idB)
				wantDiscountKind, err := safecast.IntToInt16(params.Discount.Kind().Code())
				require.NoError(t, err)
				wantScopeKind, err := safecast.IntToInt16(params.Scope.Kind().Code())
				require.NoError(t, err)
				assert.Equal(t, wantDiscountKind, discountKindA)
				assert.Equal(t, wantScopeKind, scopeKindA)
				assert.Equal(t, params.ExpiresAt.UTC(), expiresAtA.UTC())
			})
		})

		t.Run("適用範囲がカテゴリの場合、対象IDを保持して発行する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				user := uuidtestkit.NewTestFromSalt(t, "bulk_cs_category_user")
				insertUser(ctx, t, drv, user, false)

				categoryID, err := uuid.Parse("5dd52d84-78eb-4a52-ba0b-2e11c95c2af2")
				require.NoError(t, err)
				scope, err := domaincoupon.NewCategoryScope(categoryID)
				require.NoError(t, err)

				params := newBulkIssueParams(t)
				params.Scope = scope

				_, err = svc.IssuePromotionalCoupons(ctx, params)
				require.NoError(t, err)

				var got uuid.UUID
				row := drv.QueryRow(ctx, "SELECT scope_target_id FROM coupons WHERE user_id = $1", user)
				require.NoError(t, row.Scan(&got))
				assert.Equal(t, categoryID, got)
			})
		})

		t.Run("受給者が 0 人の場合は挿入せず件数 0 を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				// seed が確定済みユーザーを持つため、母集団を 0 にするには全員を退会させる必要がある。
				_, err := drv.Exec(ctx, "UPDATE users SET deleted_at = NOW() WHERE deleted_at IS NULL")
				require.NoError(t, err)

				before := countCoupons(ctx, t, drv)

				got, err := svc.IssuePromotionalCoupons(ctx, newBulkIssueParams(t))

				require.NoError(t, err)
				assert.Zero(t, got.RecipientCount)
				assert.Zero(t, got.IssuedCouponCount)
				assert.Equal(t, before, countCoupons(ctx, t, drv))
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化して返す", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			_, err := svc.IssuePromotionalCoupons(ctx, newBulkIssueParams(t))

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})

		t.Run("値引きが未設定の場合、ドメインの検証に落ちて発行しない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				user := uuidtestkit.NewTestFromSalt(t, "bulk_cs_invalid_discount")
				insertUser(ctx, t, drv, user, false)

				params := newBulkIssueParams(t)
				params.Discount = domaincoupon.Discount{}

				_, err := svc.IssuePromotionalCoupons(ctx, params)

				require.ErrorIs(t, err, domaincoupon.ErrInvalidDiscount)
				assert.Equal(t, 0, countCouponsOf(ctx, t, drv, user))
			})
		})

		t.Run("有効期限が発行日時より後でない場合、ドメインの検証に落ちて発行しない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				user := uuidtestkit.NewTestFromSalt(t, "bulk_cs_invalid_expires")
				insertUser(ctx, t, drv, user, false)

				params := newBulkIssueParams(t)
				params.ExpiresAt = params.IssuedAt

				_, err := svc.IssuePromotionalCoupons(ctx, params)

				require.ErrorIs(t, err, domaincoupon.ErrInvalidExpiresAt)
				assert.Equal(t, 0, countCouponsOf(ctx, t, drv, user))
			})
		})
	})
}
