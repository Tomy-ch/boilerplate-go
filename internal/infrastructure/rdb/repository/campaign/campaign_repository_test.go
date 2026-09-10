package campaign

import (
	"context"
	"math"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincampaign "go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 既存 seed の利用者とカテゴリ（database/seed）。
const (
	seedAliceUserID = "0b393ac1-b8a2-4f69-8972-de680aeb0a95"
	seedCategoryID  = "5dd52d84-78eb-4a52-ba0b-2e11c95c2af2"
)

func mustParse(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err)

	return id
}

func newID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.New()
	require.NoError(t, err)

	return id
}

// newTestCampaign は、一意なコードを持つキャンペーンを組み立てます。
func newTestCampaign(t *testing.T, mutate func(*domaincampaign.TemplateAttributes)) *domaincampaign.Campaign {
	t.Helper()

	// 一意制約に当たらないよう、コードは UUID の先頭 16 桁から作る。
	code, err := domaincampaign.NewCode("C" + newID(t).String()[:15])
	require.NoError(t, err)

	value, err := decimal.Parse("0.10")
	require.NoError(t, err)
	attrs := domaincampaign.TemplateAttributes{
		DiscountKindName: "rate",
		DiscountValue:    value,
		ScopeKindName:    "all",
		ExpiresAt:        time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Microsecond),
	}
	if mutate != nil {
		mutate(&attrs)
	}
	template, err := domaincampaign.NewTemplate(attrs)
	require.NoError(t, err)

	startsAt := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	c, err := domaincampaign.New(newID(t), domaincampaign.Attributes{
		Code:         code,
		Template:     template,
		StartsAt:     startsAt,
		EndsAt:       startsAt.Add(30 * 24 * time.Hour),
		TotalLimit:   10,
		PerUserLimit: 2,
	})
	require.NoError(t, err)

	return c
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

func Test_repository_Create(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件を持たないキャンペーンを往復させる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				want := newTestCampaign(t, nil)

				require.NoError(t, repo.Create(ctx, want))

				got, err := repo.LockByCode(ctx, want.Code())
				require.NoError(t, err)
				assert.Equal(t, want.ID(), got.ID())
				assert.Equal(t, want.Code(), got.Code())
				assert.Equal(t, "rate", got.Template().DiscountKindName())
				assert.Equal(t, "all", got.Template().ScopeKindName())
				assert.Nil(t, got.Template().DiscountMaxAmount())
				assert.Nil(t, got.Template().MinPurchaseAmount())
				assert.Nil(t, got.Template().UsableFrom())
				assert.Equal(t, want.TotalLimit(), got.TotalLimit())
				assert.Equal(t, want.PerUserLimit(), got.PerUserLimit())
				assert.Zero(t, got.IssuedCount())
				assert.False(t, got.IsSuspended())
			})
		})

		t.Run("条件を持つキャンペーンは条件も往復させる", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			minPurchase := int64(5000)
			targetID := mustParse(t, seedCategoryID)

			txm.WithinTx(func(ctx context.Context) {
				usableFrom := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
				want := newTestCampaign(t, func(a *domaincampaign.TemplateAttributes) {
					a.DiscountMaxAmount = &maxAmount
					a.MinPurchaseAmount = &minPurchase
					a.UsableFrom = &usableFrom
					a.ScopeKindName = "category"
					a.ScopeTargetID = &targetID
				})

				require.NoError(t, repo.Create(ctx, want))

				got, err := repo.LockByCode(ctx, want.Code())
				require.NoError(t, err)
				require.NotNil(t, got.Template().DiscountMaxAmount())
				assert.Equal(t, maxAmount, *got.Template().DiscountMaxAmount())
				require.NotNil(t, got.Template().MinPurchaseAmount())
				assert.Equal(t, minPurchase, *got.Template().MinPurchaseAmount())
				require.NotNil(t, got.Template().UsableFrom())
				assert.WithinDuration(t, usableFrom, *got.Template().UsableFrom(), time.Millisecond)
				assert.Equal(t, "category", got.Template().ScopeKindName())
				require.NotNil(t, got.Template().ScopeTargetID())
				assert.Equal(t, targetID, *got.Template().ScopeTargetID())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("同じコードのキャンペーンが既にある場合、Conflictを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				first := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, first))

				second := newTestCampaign(t, nil)
				dup, err := domaincampaign.New(newID(t), domaincampaign.Attributes{
					Code:         first.Code(),
					Template:     second.Template(),
					StartsAt:     second.StartsAt(),
					EndsAt:       second.EndsAt(),
					TotalLimit:   second.TotalLimit(),
					PerUserLimit: second.PerUserLimit(),
				})
				require.NoError(t, err)

				require.ErrorIs(t, repo.Create(ctx, dup), apperror.ErrConflict)
			})
		})
	})
}

func Test_repository_LockByCode(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止済みでも配布期間の外でも取得できる", func(t *testing.T) {
			t.Parallel()

			// 絞り込みを SQL に置かないため、「不在」と「対象外」が同じ 0 行に潰れない。
			txm.WithinTx(func(ctx context.Context) {
				want := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, want))
				require.NoError(t, repo.UpdateSuspended(ctx, want.ID(), time.Now().UTC()))

				got, err := repo.LockByCode(ctx, want.Code())

				require.NoError(t, err)
				assert.True(t, got.IsSuspended())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("該当するコードが無い場合、NotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				code, err := domaincampaign.NewCode("NOSUCHCODE-0001")
				require.NoError(t, err)

				_, err = repo.LockByCode(ctx, code)

				require.ErrorIs(t, err, apperror.ErrNotFound)
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

		t.Run("IDからキャンペーンを取得する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				want := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, want))

				got, err := repo.LockByID(ctx, want.ID())

				require.NoError(t, err)
				assert.Equal(t, want.Code(), got.Code())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("該当するIDが無い場合、NotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				_, err := repo.LockByID(ctx, newID(t))

				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})
	})
}

func Test_repository_RecordClaim(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("発行済み枚数の加算と受け取り記録の挿入をまとめて行う", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				c := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, c))
				couponID := insertTestCoupon(ctx, t, testDB)

				record, err := c.Claim(newClaimParams(t, couponID, 0))
				require.NoError(t, err)

				require.NoError(t, repo.RecordClaim(ctx, record))

				got, err := repo.LockByID(ctx, c.ID())
				require.NoError(t, err)
				assert.Equal(t, 1, got.IssuedCount())

				count, err := repo.CountClaims(ctx, domaincampaign.ClaimCountParams{
					CampaignID: c.ID(),
					UserID:     mustParse(t, seedAliceUserID),
				})
				require.NoError(t, err)
				assert.Equal(t, 1, count)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("在籍しない受給者の外部キー違反はErrInvalidArgumentへ正規化して返す", func(t *testing.T) {
			t.Parallel()

			// RecordClaim は 2 回書き込む。2 本目（受け取り記録の挿入）の制約違反が
			// 正規化されることを、1 本目の業務エラーとは別に押さえる。
			txm.WithinTx(func(ctx context.Context) {
				c := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, c))
				couponID := insertTestCoupon(ctx, t, testDB)

				params := newClaimParams(t, couponID, 0)
				params.UserID = newID(t) // 存在しない利用者
				record, err := c.Claim(params)
				require.NoError(t, err)

				require.ErrorIs(t, repo.RecordClaim(ctx, record), apperror.ErrInvalidArgument)
			})
		})

		t.Run("総枚数上限が既に埋まっている場合、ErrIssuedConcurrentlyを返す", func(t *testing.T) {
			t.Parallel()

			// 行ロックを取らずに呼ばれた場合に備える二重防御が働くことを固定する。
			txm.WithinTx(func(ctx context.Context) {
				c := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, c))
				fillToTotalLimit(ctx, t, testDB, c.ID(), c.TotalLimit())
				couponID := insertTestCoupon(ctx, t, testDB)

				record, err := c.Claim(newClaimParams(t, couponID, 0))
				require.NoError(t, err)

				require.ErrorIs(t, repo.RecordClaim(ctx, record), domaincampaign.ErrIssuedConcurrently)
			})
		})
	})
}

func Test_repository_CountClaims(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取ったことが無い場合は0を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				c := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, c))

				got, err := repo.CountClaims(ctx, domaincampaign.ClaimCountParams{
					CampaignID: c.ID(),
					UserID:     mustParse(t, seedAliceUserID),
				})

				require.NoError(t, err)
				assert.Zero(t, got)
			})
		})
	})
}

func Test_repository_UpdateSuspended(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止日時を刻む", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				c := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, c))
				suspendedAt := time.Now().UTC().Truncate(time.Microsecond)

				require.NoError(t, repo.UpdateSuspended(ctx, c.ID(), suspendedAt))

				got, err := repo.LockByID(ctx, c.ID())
				require.NoError(t, err)
				require.NotNil(t, got.SuspendedAt())
				assert.WithinDuration(t, suspendedAt, *got.SuspendedAt(), time.Millisecond)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既に停止済みの場合、ErrAlreadySuspendedを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				c := newTestCampaign(t, nil)
				require.NoError(t, repo.Create(ctx, c))
				require.NoError(t, repo.UpdateSuspended(ctx, c.ID(), time.Now().UTC()))

				err := repo.UpdateSuspended(ctx, c.ID(), time.Now().UTC())

				require.ErrorIs(t, err, domaincampaign.ErrAlreadySuspended)
			})
		})
	})
}

func Test_repository_FindList(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("配布期間の開始が新しい順で返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				// 隣接ペアの非逆転だけを見ると、期待する並びを名指ししていないことになる。
				// 2 件の相対位置そのものを固定する。
				older := newTestCampaignStartingAt(t, time.Now().UTC().Add(-48*time.Hour))
				require.NoError(t, repo.Create(ctx, older))
				newer := newTestCampaignStartingAt(t, time.Now().UTC().Add(-24*time.Hour))
				require.NoError(t, repo.Create(ctx, newer))

				got, err := repo.FindList(ctx, domaincampaign.ListParams{Limit: 100, Offset: 0})

				require.NoError(t, err)
				assert.Less(t, indexOfCampaign(t, got, newer.ID()), indexOfCampaign(t, got, older.ID()))
			})
		})

		t.Run("取得件数の上限を超えて返さない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				require.NoError(t, repo.Create(ctx, newTestCampaign(t, nil)))
				require.NoError(t, repo.Create(ctx, newTestCampaign(t, nil)))

				got, err := repo.FindList(ctx, domaincampaign.ListParams{Limit: 1, Offset: 0})

				require.NoError(t, err)
				assert.Len(t, got, 1)
			})
		})
	})
}

func Test_repository_CountAll(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	txm := testkit.NewTestTransactionRunner(t)
	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("追加した分だけ件数が増える", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				before, err := repo.CountAll(ctx)
				require.NoError(t, err)

				require.NoError(t, repo.Create(ctx, newTestCampaign(t, nil)))

				after, err := repo.CountAll(ctx)
				require.NoError(t, err)
				assert.Equal(t, before+1, after)
			})
		})
	})
}

// newTestCampaignStartingAt は、配布期間の開始日時だけを指定したキャンペーンを組み立てます。
func newTestCampaignStartingAt(t *testing.T, startsAt time.Time) *domaincampaign.Campaign {
	t.Helper()

	base := newTestCampaign(t, nil)
	c, err := domaincampaign.New(base.ID(), domaincampaign.Attributes{
		Code:         base.Code(),
		Template:     base.Template(),
		StartsAt:     startsAt.Truncate(time.Microsecond),
		EndsAt:       startsAt.Add(30 * 24 * time.Hour).Truncate(time.Microsecond),
		TotalLimit:   base.TotalLimit(),
		PerUserLimit: base.PerUserLimit(),
	})
	require.NoError(t, err)

	return c
}

// indexOfCampaign は、一覧の中で指定 ID が現れる位置を返します。見つからなければテストを失敗させます。
func indexOfCampaign(t *testing.T, cs domaincampaign.Campaigns, id uuid.UUID) int {
	t.Helper()

	for i, c := range cs {
		if c.ID() == id {
			return i
		}
	}
	t.Fatalf("キャンペーン %s が一覧に現れませんでした", id)

	return -1
}

// newClaimParams は、受け取り 1 件の入力を組み立てます。受給者は seed の利用者で固定します。
func newClaimParams(t *testing.T, couponID uuid.UUID, claimedByUser int) domaincampaign.ClaimParams {
	t.Helper()

	return domaincampaign.ClaimParams{
		ClaimedAt:     time.Now().UTC(),
		ClaimedByUser: claimedByUser,
		ClaimID:       newID(t),
		UserID:        mustParse(t, seedAliceUserID),
		CouponID:      couponID,
	}
}

// insertTestCoupon は、受け取り記録の外部キーを満たすためのクーポン行を 1 件立てて ID を返します。
// 受け取り記録はクーポンを参照するため、記録だけを単独で作ることはできません。
func insertTestCoupon(ctx context.Context, t *testing.T, testDB driver.DatabaseDriver) uuid.UUID {
	t.Helper()

	id := newID(t)
	now := time.Now().UTC()
	_, err := driver.New(ctx, testDB).Exec(ctx,
		`INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, expires_at, issued_at)
		 VALUES ($1, $2, 2, '0.10', 1, $3, $4)`,
		id, mustParse(t, seedAliceUserID), now.Add(30*24*time.Hour), now)
	require.NoError(t, err)

	return id
}

// fillToTotalLimit は、発行済み枚数を総枚数上限まで埋めます。
// ドメインを通さずに埋めるのは、永続化側の二重防御だけを単独で確かめるためです。
func fillToTotalLimit(
	ctx context.Context, t *testing.T, testDB driver.DatabaseDriver, campaignID uuid.UUID, totalLimit int,
) {
	t.Helper()

	_, err := driver.New(ctx, testDB).Exec(ctx,
		"UPDATE campaigns SET issued_count = $2 WHERE id = $1", campaignID, totalLimit)
	require.NoError(t, err)
}

func Test_rowToCampaign(t *testing.T) {
	t.Parallel()

	// validRow は、再構築に成功する行を組み立てます。
	validRow := func(t *testing.T) gen.Campaigns {
		t.Helper()

		startsAt := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)

		return gen.Campaigns{
			ID:              newID(t),
			Code:            "WELCOME-2026",
			DiscountKind:    "rate",
			DiscountValue:   mustDecimal(t, "0.10"),
			ScopeKind:       "all",
			CouponExpiresAt: startsAt.Add(90 * 24 * time.Hour),
			StartsAt:        startsAt,
			EndsAt:          startsAt.Add(30 * 24 * time.Hour),
			TotalLimit:      10,
			PerUserLimit:    2,
			IssuedCount:     3,
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("永続化された行からキャンペーンを再構築する", func(t *testing.T) {
			t.Parallel()

			row := validRow(t)

			got, err := rowToCampaign(row)

			require.NoError(t, err)
			assert.Equal(t, row.ID, got.ID())
			assert.Equal(t, "WELCOME-2026", got.Code().Value())
			assert.Equal(t, "rate", got.Template().DiscountKindName())
			assert.Equal(t, 3, got.IssuedCount())
			assert.False(t, got.IsSuspended())
		})

		t.Run("既知でない種別の名前でも再構築は成功する", func(t *testing.T) {
			t.Parallel()

			// 閉じた集合の権威はクーポン側にあり、キャンペーンは名前を保つだけである。
			// 名前の解決に失敗するのは受け取り時であって、読み込み時ではない。
			row := validRow(t)
			row.DiscountKind = "unknown"

			got, err := rowToCampaign(row)

			require.NoError(t, err)
			assert.Equal(t, "unknown", got.Template().DiscountKindName())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("コードが形式を外れている場合、再構築エラーへ正規化する", func(t *testing.T) {
			t.Parallel()

			row := validRow(t)
			row.Code = "SHORT"

			_, err := rowToCampaign(row)

			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("テンプレートが検証を通らない場合、再構築エラーへ正規化する", func(t *testing.T) {
			t.Parallel()

			row := validRow(t)
			row.DiscountValue = mustDecimal(t, "0")

			_, err := rowToCampaign(row)

			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("発行済み枚数が総枚数上限を超えている場合、再構築エラーへ正規化する", func(t *testing.T) {
			t.Parallel()

			row := validRow(t)
			row.IssuedCount = row.TotalLimit + 1

			_, err := rowToCampaign(row)

			require.ErrorIs(t, err, apperror.ErrInternal)
		})
	})
}

func Test_campaignToCreateParams(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("集約を挿入パラメータへ写す", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t, nil)

			got, err := campaignToCreateParams(c)

			require.NoError(t, err)
			assert.Equal(t, c.ID(), got.ID)
			assert.Equal(t, c.Code().Value(), got.Code)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Equal(t, "all", got.ScopeKind)
			assert.Equal(t, int32(10), got.TotalLimit)
			assert.Equal(t, int32(2), got.PerUserLimit)
			assert.Zero(t, got.IssuedCount)
			assert.Nil(t, got.DiscountMaxAmount)
			assert.Nil(t, got.CouponMinPurchaseAmount)
			assert.Nil(t, got.CouponUsableFrom)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("総枚数上限がint32に収まらない場合、エラーを返す", func(t *testing.T) {
			t.Parallel()

			// ドメインは上限が正であることしか検証しないため、ここが最後の防御になる。
			c := newOverflowingCampaign(t, math.MaxInt32+1, 2)

			_, err := campaignToCreateParams(c)

			require.Error(t, err)
		})

		t.Run("1人あたり上限がint32に収まらない場合、エラーを返す", func(t *testing.T) {
			t.Parallel()

			c := newOverflowingCampaign(t, math.MaxInt32+2, math.MaxInt32+1)

			_, err := campaignToCreateParams(c)

			require.Error(t, err)
		})
	})
}

// newOverflowingCampaign は、int32 に収まらない上限を持つキャンペーンを組み立てます。
// ドメインは上限の大きさを制限しないため、この状態は正当に構築できます。
func newOverflowingCampaign(t *testing.T, totalLimit, perUserLimit int) *domaincampaign.Campaign {
	t.Helper()

	base := newTestCampaign(t, nil)
	c, err := domaincampaign.New(base.ID(), domaincampaign.Attributes{
		Code:         base.Code(),
		Template:     base.Template(),
		StartsAt:     base.StartsAt(),
		EndsAt:       base.EndsAt(),
		TotalLimit:   totalLimit,
		PerUserLimit: perUserLimit,
	})
	require.NoError(t, err)

	return c
}

// mustDecimal は、テスト用に十進量を組み立てます。
func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	require.NoError(t, err)

	return d
}
