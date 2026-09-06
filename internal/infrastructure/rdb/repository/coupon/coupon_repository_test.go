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
	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 既存 seed のクーポンと受給者（database/seed/000041_coupons.sql）。
const (
	seedBobUserID   = "090f5b51-37ac-4413-b326-1709ae4661f4"
	seedBobFlatAll  = "0193a1c0-0001-7000-8000-000000000001"
	seedBobRateAll  = "0193a1c0-0001-7000-8000-000000000002"
	seedAliceUserID = "0b393ac1-b8a2-4f69-8972-de680aeb0a95"
	seedCategoryID  = "5dd52d84-78eb-4a52-ba0b-2e11c95c2af2"
)

func mustParse(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err)

	return id
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("依存を注入したリポジトリ実装を生成する", func(t *testing.T) {
			t.Parallel()

			testDB := testkit.NewTestDB(t)

			repo, ok := New(testDB, observability.NewNoopTracerFactory(t)).(*repository)
			require.True(t, ok)
			assert.Equal(t, testDB, repo.db)
			assert.NotNil(t, repo.tracer)
		})
	})
}

func Test_repository_FindByUserID(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保有するクーポンを発行日時の降順で返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.FindByUserID(ctx, mustParse(t, seedBobUserID))

				require.NoError(t, err)
				require.Len(t, got, 2)
				for _, c := range got {
					assert.Equal(t, mustParse(t, seedBobUserID), c.UserID())
				}
			})
		})

		t.Run("値引きと適用範囲を業務キーからドメインの値へ復元する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.FindByUserID(ctx, mustParse(t, seedBobUserID))
				require.NoError(t, err)

				kinds := make(map[int]bool, len(got))
				for _, c := range got {
					kinds[c.Discount().Kind().Code()] = true
					assert.Equal(t, domaincoupon.ScopeKindAll, c.Scope().Kind())
					assert.Nil(t, c.Scope().TargetID())
				}
				assert.True(t, kinds[domaincoupon.DiscountKindFlat.Code()])
				assert.True(t, kinds[domaincoupon.DiscountKindRate.Code()])
			})
		})

		t.Run("使用済みのクーポンも母集団に含める", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				_, err := drv.Exec(ctx, "UPDATE coupons SET used_at = NOW() WHERE id = $1", seedBobFlatAll)
				require.NoError(t, err)

				got, ferr := repo.FindByUserID(ctx, mustParse(t, seedBobUserID))

				require.NoError(t, ferr)
				assert.Len(t, got, 2)
			})
		})

		t.Run("1枚も保有しない利用者には空を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.FindByUserID(ctx, mustParse(t, "11111111-1111-4111-8111-111111111111"))

				require.NoError(t, err)
				assert.Empty(t, got)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化して返す", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			got, err := repo.FindByUserID(ctx, mustParse(t, seedBobUserID))

			assert.Nil(t, got)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})

		t.Run("ドメインが知らない値引き種別コードは再構築エラーを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				_, err := drv.Exec(ctx, "UPDATE coupons SET discount_kind = 99 WHERE id = $1", seedBobFlatAll)
				require.NoError(t, err)

				got, ferr := repo.FindByUserID(ctx, mustParse(t, seedBobUserID))

				assert.Nil(t, got)
				require.ErrorIs(t, ferr, apperror.ErrInternal)
			})
		})
	})
}

func Test_repository_LockByID(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDで1枚取得する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.LockByID(ctx, mustParse(t, seedBobRateAll))

				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, mustParse(t, seedBobRateAll), got.ID())
				assert.Equal(t, domaincoupon.DiscountKindRate, got.Discount().Kind())
			})
		})

		t.Run("使用済みのクーポンも取得できる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				_, err := drv.Exec(ctx, "UPDATE coupons SET used_at = NOW() WHERE id = $1", seedBobRateAll)
				require.NoError(t, err)

				got, lerr := repo.LockByID(ctx, mustParse(t, seedBobRateAll))

				require.NoError(t, lerr)
				require.NotNil(t, got)
				assert.True(t, got.IsUsed())
			})
		})

		t.Run("受給者で絞らないため他人のクーポンも取得できる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.LockByID(ctx, mustParse(t, seedBobRateAll))

				require.NoError(t, err)
				require.NotNil(t, got)
				assert.False(t, got.IsHeldBy(mustParse(t, seedAliceUserID)))
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("存在しないIDはNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.LockByID(ctx, mustParse(t, "22222222-2222-4222-8222-222222222222"))

				assert.Nil(t, got)
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化して返す", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			got, err := repo.LockByID(ctx, mustParse(t, seedBobRateAll))

			assert.Nil(t, got)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_repository_UpdateUsed(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	usedAt := time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未使用のクーポンを使用済みにする", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				require.NoError(t, repo.UpdateUsed(ctx, mustParse(t, seedBobFlatAll), usedAt))

				got, err := repo.LockByID(ctx, mustParse(t, seedBobFlatAll))
				require.NoError(t, err)
				require.NotNil(t, got.UsedAt())
				assert.True(t, usedAt.Equal(*got.UsedAt()))
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既に使用済みの場合、ErrUsedConcurrentlyを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				require.NoError(t, repo.UpdateUsed(ctx, mustParse(t, seedBobRateAll), usedAt))

				err := repo.UpdateUsed(ctx, mustParse(t, seedBobRateAll), usedAt.Add(time.Hour))

				require.ErrorIs(t, err, domaincoupon.ErrUsedConcurrently)
				require.ErrorIs(t, err, apperror.ErrConflict)
			})
		})

		t.Run("存在しないIDもErrUsedConcurrentlyを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				err := repo.UpdateUsed(ctx, mustParse(t, "33333333-3333-4333-8333-333333333333"), usedAt)

				require.ErrorIs(t, err, domaincoupon.ErrUsedConcurrently)
			})
		})

		t.Run("キャンセル済みコンテキストではErrCanceledへ正規化して返す", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := repo.UpdateUsed(ctx, mustParse(t, seedBobFlatAll), usedAt)

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_rowToCoupon(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ドメインが知らない適用範囲種別コードは再構築エラーを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				_, err := drv.Exec(ctx, "UPDATE coupons SET scope_kind = 99 WHERE id = $1", seedBobFlatAll)
				require.NoError(t, err)

				got, lerr := repo.LockByID(ctx, mustParse(t, seedBobFlatAll))

				assert.Nil(t, got)
				require.ErrorIs(t, lerr, apperror.ErrInternal)
			})
		})

		t.Run("全体の適用範囲に対象IDが入っている場合は再構築エラーを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				drv := driver.New(ctx, testDB)
				_, err := drv.Exec(ctx,
					"UPDATE coupons SET scope_target_id = $1 WHERE id = $2", seedCategoryID, seedBobFlatAll)
				require.NoError(t, err)

				got, lerr := repo.LockByID(ctx, mustParse(t, seedBobFlatAll))

				assert.Nil(t, got)
				require.ErrorIs(t, lerr, apperror.ErrInternal)
			})
		})
	})
}
