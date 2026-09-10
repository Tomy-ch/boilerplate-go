package campaign

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincampaign "go-boilerplate/internal/domain/campaign"
	mock_campaign "go-boilerplate/internal/domain/campaign/mock"
	"go-boilerplate/internal/domain/coupon"
	mock_coupon "go-boilerplate/internal/domain/coupon/mock"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	mock_authz "go-boilerplate/internal/usecase/boundary/authz/mock"
	mock_clock "go-boilerplate/internal/usecase/boundary/clock/mock"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	"go-boilerplate/internal/usecase/tools/paging"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	testNow             = time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)
	testStartsAt        = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	testEndsAt          = time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	testCouponExpiresAt = time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
)

type testDeps struct {
	campaignRepo *mock_campaign.MockRepository
	couponRepo   *mock_coupon.MockRepository
	clock        *mock_clock.MockClock
	txm          *mock_tx.MockManager
	authorizer   *mock_authz.MockAuthorizer
}

func newTestUsecase(t *testing.T) (*usecase, *testDeps) {
	t.Helper()

	ctrl := gomock.NewController(t)
	deps := &testDeps{
		campaignRepo: mock_campaign.NewMockRepository(ctrl),
		couponRepo:   mock_coupon.NewMockRepository(ctrl),
		clock:        mock_clock.NewMockClock(ctrl),
		txm:          mock_tx.NewMockManager(ctrl),
		authorizer:   mock_authz.NewMockAuthorizer(ctrl),
	}
	u := &usecase{
		tracer:       observability.NewMockUsecaseLayerTracer(t),
		campaignRepo: deps.campaignRepo,
		couponRepo:   deps.couponRepo,
		clock:        deps.clock,
		txm:          deps.txm,
		authorizer:   deps.authorizer,
	}

	return u, deps
}

func runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newTestAuthn(t *testing.T) (*auth.Authn, uuid.UUID) {
	t.Helper()

	userID := uuidtestkit.NewTestFromSalt(t, "campaign_claimer")
	a, err := auth.New("subject-1", auth.IssuerMock, nil, nil)
	require.NoError(t, err)
	resolved, err := a.WithUserID(userID)
	require.NoError(t, err)

	return resolved, userID
}

func newTestUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.New()
	require.NoError(t, err)

	return id
}

func newDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	require.NoError(t, err)

	return d
}

func newDefineParams(t *testing.T) DefineCampaignParams {
	t.Helper()

	return DefineCampaignParams{
		Code:            "WELCOME-2026",
		DiscountKind:    "rate",
		DiscountValue:   newDecimal(t, "0.10"),
		ScopeKind:       "all",
		CouponExpiresAt: testCouponExpiresAt,
		StartsAt:        testStartsAt,
		EndsAt:          testEndsAt,
		TotalLimit:      10,
		PerUserLimit:    2,
	}
}

// newTestCampaign は、配布中のキャンペーンを組み立てます。
func newTestCampaign(t *testing.T, totalLimit, perUserLimit int) *domaincampaign.Campaign {
	t.Helper()

	code, err := domaincampaign.NewCode("WELCOME-2026")
	require.NoError(t, err)
	template, err := domaincampaign.NewTemplate(domaincampaign.TemplateAttributes{
		DiscountKindName: "rate",
		DiscountValue:    newDecimal(t, "0.10"),
		ScopeKindName:    "all",
		ExpiresAt:        testCouponExpiresAt,
	})
	require.NoError(t, err)
	c, err := domaincampaign.New(uuidtestkit.NewTestFromSalt(t, "campaign"), domaincampaign.Attributes{
		Code:         code,
		Template:     template,
		StartsAt:     testStartsAt,
		EndsAt:       testEndsAt,
		TotalLimit:   totalLimit,
		PerUserLimit: perUserLimit,
	})
	require.NoError(t, err)

	return c
}

// newCampaignFor は、テンプレートだけを差し替えたキャンペーンを組み立てます。
func newCampaignFor(t *testing.T, tmpl domaincampaign.Template) *domaincampaign.Campaign {
	t.Helper()

	base := newTestCampaign(t, 10, 2)
	c, err := domaincampaign.New(base.ID(), domaincampaign.Attributes{
		Code:         base.Code(),
		Template:     tmpl,
		StartsAt:     base.StartsAt(),
		EndsAt:       base.EndsAt(),
		TotalLimit:   base.TotalLimit(),
		PerUserLimit: base.PerUserLimit(),
	})
	require.NoError(t, err)

	return c
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("依存を注入したユースケース実装を生成する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			got, ok := New(
				mock_campaign.NewMockRepository(ctrl),
				mock_coupon.NewMockRepository(ctrl),
				mock_clock.NewMockClock(ctrl),
				mock_tx.NewMockManager(ctrl),
				mock_authz.NewMockAuthorizer(ctrl),
				observability.NewNoopTracerFactory(t),
			).(*usecase)

			require.True(t, ok)
			assert.NotNil(t, got.campaignRepo)
			assert.NotNil(t, got.couponRepo)
		})
	})
}

func Test_usecase_DefineCampaign(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("1枚も配っていないキャンペーンを定義する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionCampaignDefine, gomock.Any()).
				Return(nil)
			deps.campaignRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, c *domaincampaign.Campaign) error {
					assert.Equal(t, "WELCOME-2026", c.Code().Value())
					assert.Zero(t, c.IssuedCount())

					return nil
				})

			got, err := u.DefineCampaign(t.Context(), &auth.Authn{}, newDefineParams(t))

			require.NoError(t, err)
			assert.Equal(t, "WELCOME-2026", got.Code)
			assert.Zero(t, got.IssuedCount)
			assert.Nil(t, got.SuspendedAt)
		})

		t.Run("コードは正規化して保存する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			params := newDefineParams(t)
			params.Code = "  welcome-2026  "

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.campaignRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

			got, err := u.DefineCampaign(t.Context(), &auth.Authn{}, params)

			require.NoError(t, err)
			assert.Equal(t, "WELCOME-2026", got.Code)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認可に失敗した場合、コードを検証せずに返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(apperror.ErrPermissionDenied)

			_, err := u.DefineCampaign(t.Context(), &auth.Authn{}, newDefineParams(t))

			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})

		t.Run("コードが規定の形式でない場合、ErrInvalidCodeを返し永続化しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			params := newDefineParams(t)
			params.Code = "SHORT"

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.DefineCampaign(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincampaign.ErrInvalidCode)
		})

		t.Run("既知でない値引き種別の場合、受け取りを待たずに定義時点で弾く", func(t *testing.T) {
			t.Parallel()

			// 誰も受け取れないキャンペーンを作れてしまわないことを固定する。
			u, deps := newTestUsecase(t)
			params := newDefineParams(t)
			params.DiscountKind = "unknown"

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.DefineCampaign(t.Context(), &auth.Authn{}, params)

			// 検証エラーの族ではなく、この分岐固有の sentinel まで見る。
			require.ErrorIs(t, err, coupon.ErrInvalidDiscountKind)
		})

		t.Run("配るクーポンの有効期限が配布期間の終了より前の場合、永続化しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			params := newDefineParams(t)
			params.CouponExpiresAt = params.EndsAt

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.DefineCampaign(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincampaign.ErrInvalidCouponExpiresAt)
		})

		t.Run("コードが既に使われている場合、Conflictを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.campaignRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(apperror.ErrConflict)

			_, err := u.DefineCampaign(t.Context(), &auth.Authn{}, newDefineParams(t))

			require.ErrorIs(t, err, apperror.ErrConflict)
		})
	})
}

func Test_usecase_ListCampaigns(t *testing.T) {
	t.Parallel()

	newPage := func(t *testing.T) *paging.Page {
		t.Helper()
		p, err := paging.NewPageFrom1Based(nil, nil)
		require.NoError(t, err)

		return p
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("一覧と総件数を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionCampaignList, gomock.Any()).
				Return(nil)
			deps.campaignRepo.EXPECT().
				FindList(gomock.Any(), gomock.Any()).
				Return(domaincampaign.Campaigns{newTestCampaign(t, 10, 2)}, nil)
			deps.campaignRepo.EXPECT().CountAll(gomock.Any()).Return(7, nil)

			got, err := u.ListCampaigns(t.Context(), &auth.Authn{}, newPage(t))

			require.NoError(t, err)
			assert.Len(t, got.Campaigns, 1)
			assert.Equal(t, 7, got.Total)
		})

		t.Run("1件も無い場合は空の一覧と0件を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.campaignRepo.EXPECT().
				FindList(gomock.Any(), gomock.Any()).
				Return(domaincampaign.Campaigns{}, nil)
			deps.campaignRepo.EXPECT().CountAll(gomock.Any()).Return(0, nil)

			got, err := u.ListCampaigns(t.Context(), &auth.Authn{}, newPage(t))

			require.NoError(t, err)
			assert.Empty(t, got.Campaigns)
			assert.Zero(t, got.Total)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認可に失敗した場合、取得しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(apperror.ErrPermissionDenied)

			_, err := u.ListCampaigns(t.Context(), &auth.Authn{}, newPage(t))

			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})

		t.Run("ページが未指定の場合、ErrInvalidArgumentを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.ListCampaigns(t.Context(), &auth.Authn{}, nil)

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})
	})
}

func Test_usecase_SuspendCampaign(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("行ロックを取ってから停止する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			target := newTestCampaign(t, 10, 2)

			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionCampaignSuspend, gomock.Any()).
				Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().LockByID(gomock.Any(), target.ID()).Return(target, nil)
			deps.campaignRepo.EXPECT().UpdateSuspended(gomock.Any(), target.ID(), testNow).Return(nil)

			got, err := u.SuspendCampaign(t.Context(), &auth.Authn{}, target.ID())

			require.NoError(t, err)
			require.NotNil(t, got.SuspendedAt)
			assert.Equal(t, testNow, *got.SuspendedAt)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("存在しない場合、NotFoundを返し更新しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			id := uuidtestkit.NewTestFromSalt(t, "missing_campaign")

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().LockByID(gomock.Any(), id).Return(nil, apperror.ErrNotFound)

			_, err := u.SuspendCampaign(t.Context(), &auth.Authn{}, id)

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("認可に失敗した場合、行ロックを取らない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(apperror.ErrPermissionDenied)

			_, err := u.SuspendCampaign(t.Context(), &auth.Authn{}, newTestUUID(t))

			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})

		t.Run("停止の更新が競合した場合、そのまま返す", func(t *testing.T) {
			t.Parallel()

			// 行ロックを取らずに呼ばれた場合に備える永続化側の二重防御が、上位まで伝わることを固定する。
			u, deps := newTestUsecase(t)
			target := newTestCampaign(t, 10, 2)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().LockByID(gomock.Any(), target.ID()).Return(target, nil)
			deps.campaignRepo.EXPECT().
				UpdateSuspended(gomock.Any(), target.ID(), testNow).
				Return(domaincampaign.ErrAlreadySuspended)

			_, err := u.SuspendCampaign(t.Context(), &auth.Authn{}, target.ID())

			require.ErrorIs(t, err, domaincampaign.ErrAlreadySuspended)
		})

		t.Run("既に停止済みの場合、ErrAlreadySuspendedを返し更新しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			target := newTestCampaign(t, 10, 2)
			require.NoError(t, target.Suspend(testNow))

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().LockByID(gomock.Any(), target.ID()).Return(target, nil)

			_, err := u.SuspendCampaign(t.Context(), &auth.Authn{}, target.ID())

			require.ErrorIs(t, err, domaincampaign.ErrAlreadySuspended)
		})
	})
}

func Test_toCampaignView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件を持たないキャンペーンを出力へ写す", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t, 10, 2)

			got := toCampaignView(c)

			assert.Equal(t, c.ID(), got.ID)
			assert.Equal(t, "WELCOME-2026", got.Code)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Zero(t, got.DiscountValue.Cmp(newDecimal(t, "0.10")))
			assert.Equal(t, "all", got.ScopeKind)
			assert.Equal(t, testCouponExpiresAt, got.CouponExpiresAt)
			assert.Equal(t, testStartsAt, got.StartsAt)
			assert.Equal(t, testEndsAt, got.EndsAt)
			assert.Equal(t, 10, got.TotalLimit)
			assert.Equal(t, 2, got.PerUserLimit)
			assert.Zero(t, got.IssuedCount)
			assert.Nil(t, got.MaxAmount)
			assert.Nil(t, got.ScopeTargetID)
			assert.Nil(t, got.MinPurchaseAmount)
			assert.Nil(t, got.UsableFrom)
			assert.Nil(t, got.SuspendedAt)
		})

		t.Run("条件と停止日時を持つキャンペーンはそれらも写す", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			minPurchase := int64(5000)
			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			targetID := newTestUUID(t)
			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.DiscountMaxAmount = &maxAmount
				a.MinPurchaseAmount = &minPurchase
				a.UsableFrom = &usableFrom
				a.ScopeKindName = "category"
				a.ScopeTargetID = &targetID
			})
			base := newTestCampaign(t, 10, 2)
			c, err := domaincampaign.New(base.ID(), domaincampaign.Attributes{
				Code:         base.Code(),
				Template:     tmpl,
				StartsAt:     base.StartsAt(),
				EndsAt:       base.EndsAt(),
				TotalLimit:   base.TotalLimit(),
				PerUserLimit: base.PerUserLimit(),
			})
			require.NoError(t, err)
			require.NoError(t, c.Suspend(testNow))

			got := toCampaignView(c)

			require.NotNil(t, got.MaxAmount)
			assert.Equal(t, maxAmount, *got.MaxAmount)
			require.NotNil(t, got.MinPurchaseAmount)
			assert.Equal(t, minPurchase, *got.MinPurchaseAmount)
			require.NotNil(t, got.UsableFrom)
			assert.Equal(t, usableFrom, *got.UsableFrom)
			assert.Equal(t, "category", got.ScopeKind)
			require.NotNil(t, got.ScopeTargetID)
			assert.Equal(t, targetID, *got.ScopeTargetID)
			require.NotNil(t, got.SuspendedAt)
			assert.Equal(t, testNow, *got.SuspendedAt)
		})
	})
}

func Test_toClaimedCouponView(t *testing.T) {
	t.Parallel()

	// newIssuedCoupon は、受け取りで生まれるクーポンを組み立てます。
	newIssuedCoupon := func(t *testing.T, attrs coupon.Attributes) *coupon.Coupon {
		t.Helper()
		c, err := coupon.New(newTestUUID(t), attrs)
		require.NoError(t, err)

		return c
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件を持たないクーポンを出力へ写す", func(t *testing.T) {
			t.Parallel()

			c := newCampaignFor(t, newTemplate(t, nil))
			attrs, err := couponAttributesFor(c, newTestUUID(t), testNow)
			require.NoError(t, err)
			issued := newIssuedCoupon(t, attrs)

			got := toClaimedCouponView(issued)

			assert.Equal(t, issued.ID(), got.ID)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Equal(t, "all", got.ScopeKind)
			assert.Equal(t, testCouponExpiresAt, got.ExpiresAt)
			assert.Equal(t, testNow, got.IssuedAt)
			assert.Nil(t, got.UsedAt)
			assert.Nil(t, got.MaxAmount)
			assert.Nil(t, got.MinPurchaseAmount)
			assert.Nil(t, got.UsableFrom)
		})

		t.Run("条件を持つクーポンはそれらも写す", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			minPurchase := int64(5000)
			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.DiscountMaxAmount = &maxAmount
				a.MinPurchaseAmount = &minPurchase
				a.UsableFrom = &usableFrom
			})
			attrs, err := couponAttributesFor(newCampaignFor(t, tmpl), newTestUUID(t), testNow)
			require.NoError(t, err)

			got := toClaimedCouponView(newIssuedCoupon(t, attrs))

			require.NotNil(t, got.MaxAmount)
			assert.Equal(t, maxAmount, *got.MaxAmount)
			require.NotNil(t, got.MinPurchaseAmount)
			assert.Equal(t, minPurchase, *got.MinPurchaseAmount)
			require.NotNil(t, got.UsableFrom)
			assert.Equal(t, usableFrom, *got.UsableFrom)
		})
	})
}

func Test_requireUserID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("内部ユーザーIDが解決済みの場合はそれを返す", func(t *testing.T) {
			t.Parallel()

			authn, userID := newTestAuthn(t)

			got, err := requireUserID(authn)

			require.NoError(t, err)
			assert.Equal(t, userID, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証主体が無い場合はErrUnauthenticatedを返す", func(t *testing.T) {
			t.Parallel()

			_, err := requireUserID(nil)

			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("内部ユーザーIDが未解決の場合はエラーを返す", func(t *testing.T) {
			t.Parallel()

			unresolved, err := auth.New("subject-unresolved", auth.IssuerMock, nil, nil)
			require.NoError(t, err)

			_, err = requireUserID(unresolved)

			require.Error(t, err)
		})
	})
}
