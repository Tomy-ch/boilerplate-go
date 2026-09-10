package campaigns

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/testkit/testassert"
	"go-boilerplate/internal/controller/handler/v1/campaigns/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	campaignuc "go-boilerplate/internal/usecase/campaign"
	mock_campaignuc "go-boilerplate/internal/usecase/campaign/mock"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/decimal"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	testStartsAt        = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	testEndsAt          = time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	testCouponExpiresAt = time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
)

// authnContext は、内部ユーザー ID を解決済みの認証コンテキストを返すテストヘルパーです。
func authnContext(t *testing.T) context.Context {
	t.Helper()
	ctx := ctxhelper.WithAuthn(context.Background())
	authn, err := auth.New("subject", "issuer", nil, nil)
	require.NoError(t, err)

	resolved, err := authn.WithUserID(uuidtestkit.NewTestFromSalt(t, "campaign_admin"))
	require.NoError(t, err)
	require.True(t, ctxhelper.SetAuthn(ctx, *resolved))

	return ctx
}

func newServer(t *testing.T) (*server, *mock_campaignuc.MockUsecase) {
	t.Helper()
	uc := mock_campaignuc.NewMockUsecase(gomock.NewController(t))

	return &server{
		tracer: observability.NewMockControllerLayerTracer(t),
		uc:     uc,
		idem:   idempotency.Deps{},
	}, uc
}

// newRequestBody は、定率 10% × 全体 を配るキャンペーンの要求本文を組み立てます。
func newRequestBody() *gen.CampaignsPostRequest {
	return &gen.CampaignsPostRequest{
		Code:            "welcome-2026",
		Discount:        gen.CouponDiscountInput{Kind: gen.CouponDiscountInputKindRate, Value: "0.10"},
		Scope:           gen.CouponScopeInput{Kind: gen.CouponScopeInputKindAll},
		CouponExpiresAt: testCouponExpiresAt,
		StartsAt:        testStartsAt,
		EndsAt:          testEndsAt,
		TotalLimit:      1000,
		PerUserLimit:    1,
	}
}

// newCampaignView は、ユースケースが返すキャンペーンを組み立てます。
func newCampaignView(t *testing.T) campaignuc.CampaignView {
	t.Helper()

	value, err := decimal.Parse("0.10")
	require.NoError(t, err)

	return campaignuc.CampaignView{
		ID:              uuidtestkit.NewTestFromSalt(t, "campaign"),
		Code:            "WELCOME-2026",
		DiscountKind:    "rate",
		DiscountValue:   value,
		ScopeKind:       "all",
		CouponExpiresAt: testCouponExpiresAt,
		StartsAt:        testStartsAt,
		EndsAt:          testEndsAt,
		TotalLimit:      1000,
		PerUserLimit:    1,
		IssuedCount:     42,
	}
}

func Test_server_PostCampaigns(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("定義に成功した場合、201でキャンペーンを返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			want := newCampaignView(t)
			uc.EXPECT().
				DefineCampaign(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, _ any, params campaignuc.DefineCampaignParams) (campaignuc.CampaignView, error) {
					// 正規化はドメインの責務なので、受け取った値をそのまま渡す。
					assert.Equal(t, "welcome-2026", params.Code)
					assert.Equal(t, "rate", params.DiscountKind)
					assert.Equal(t, 1000, params.TotalLimit)

					return want, nil
				})

			got, err := s.PostCampaigns(authnContext(t), gen.PostCampaignsRequestObject{Body: newRequestBody()})

			require.NoError(t, err)
			res, ok := got.(gen.PostCampaigns201JSONResponse)
			require.True(t, ok)
			assert.Equal(t, want.Code, res.Code)
			assert.Equal(t, int32(42), res.IssuedCount)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きの値が十進量として読めない場合、ユースケースを呼ばずに弾く", func(t *testing.T) {
			t.Parallel()

			s, _ := newServer(t)
			body := newRequestBody()
			body.Discount.Value = "not-a-number"

			_, err := s.PostCampaigns(authnContext(t), gen.PostCampaignsRequestObject{Body: body})

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})

		t.Run("ユースケースのエラーはそのまま返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().
				DefineCampaign(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.CampaignView{}, apperror.ErrConflict)

			_, err := s.PostCampaigns(authnContext(t), gen.PostCampaignsRequestObject{Body: newRequestBody()})

			require.ErrorIs(t, err, apperror.ErrConflict)
		})
	})
}

func Test_server_GetCampaigns(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("一覧と総件数を200で返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().
				ListCampaigns(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.CampaignListView{
					Campaigns: []campaignuc.CampaignView{newCampaignView(t)},
					Total:     7,
				}, nil)

			got, err := s.GetCampaigns(authnContext(t), gen.GetCampaignsRequestObject{})

			require.NoError(t, err)
			res, ok := got.(gen.GetCampaigns200JSONResponse)
			require.True(t, ok)
			assert.Len(t, res.Campaigns, 1)
			assert.Equal(t, int32(7), res.Total)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ユースケースのエラーはそのまま返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().
				ListCampaigns(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.CampaignListView{}, apperror.ErrPermissionDenied)

			_, err := s.GetCampaigns(authnContext(t), gen.GetCampaignsRequestObject{})

			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})
	})
}

func Test_toDefineCampaignParams(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("要求本文をユースケース入力へ写す", func(t *testing.T) {
			t.Parallel()

			targetID := uuidtestkit.NewTestFromSalt(t, "scope_target")
			maxAmount := int64(2000)
			minPurchase := int64(5000)
			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			body := newRequestBody()
			body.Discount.MaxAmount = &maxAmount
			body.Scope = gen.CouponScopeInput{
				Kind:     gen.CouponScopeInputKindCategory,
				TargetId: ptrOf(targetID.ToPrimitive()),
			}
			body.MinPurchaseAmount = &minPurchase
			body.UsableFrom = &usableFrom

			got, err := toDefineCampaignParams(body)

			require.NoError(t, err)
			assert.Equal(t, "welcome-2026", got.Code)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Zero(t, got.DiscountValue.Cmp(mustDecimal(t, "0.10")))
			require.NotNil(t, got.MaxAmount)
			assert.Equal(t, maxAmount, *got.MaxAmount)
			assert.Equal(t, "category", got.ScopeKind)
			require.NotNil(t, got.ScopeTargetID)
			assert.Equal(t, targetID, *got.ScopeTargetID)
			require.NotNil(t, got.MinPurchaseAmount)
			assert.Equal(t, minPurchase, *got.MinPurchaseAmount)
			require.NotNil(t, got.UsableFrom)
			assert.Equal(t, usableFrom, *got.UsableFrom)
			assert.Equal(t, testCouponExpiresAt, got.CouponExpiresAt)
			assert.Equal(t, 1000, got.TotalLimit)
			assert.Equal(t, 1, got.PerUserLimit)
		})

		t.Run("任意項目を持たない場合はnilのまま写す", func(t *testing.T) {
			t.Parallel()

			got, err := toDefineCampaignParams(newRequestBody())

			require.NoError(t, err)
			assert.Nil(t, got.MaxAmount)
			assert.Nil(t, got.ScopeTargetID)
			assert.Nil(t, got.MinPurchaseAmount)
			assert.Nil(t, got.UsableFrom)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きの値が十進量として読めない場合、ErrInvalidArgumentを返す", func(t *testing.T) {
			t.Parallel()

			body := newRequestBody()
			body.Discount.Value = ""

			_, err := toDefineCampaignParams(body)

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})
	})
}

func Test_toCampaignResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンペーンを応答の語彙へ写す", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			minPurchase := int64(5000)
			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			suspendedAt := testStartsAt.Add(time.Hour)
			targetID := uuidtestkit.NewTestFromSalt(t, "scope_target")
			view := newCampaignView(t)
			view.MaxAmount = &maxAmount
			view.MinPurchaseAmount = &minPurchase
			view.UsableFrom = &usableFrom
			view.SuspendedAt = &suspendedAt
			view.ScopeKind = "category"
			view.ScopeTargetID = &targetID

			got, err := toCampaignResponse(view)

			require.NoError(t, err)
			assert.Equal(t, view.ID.ToPrimitive(), got.Id)
			assert.Equal(t, "WELCOME-2026", got.Code)
			assert.Equal(t, gen.CouponDiscountKindRate, got.Discount.Kind)
			assert.Equal(t, "0.1", got.Discount.Value)
			require.NotNil(t, got.Discount.MaxAmount)
			assert.Equal(t, maxAmount, *got.Discount.MaxAmount)
			assert.Equal(t, gen.CouponScopeKindCategory, got.Scope.Kind)
			require.NotNil(t, got.Scope.TargetId)
			assert.Equal(t, targetID.ToPrimitive(), *got.Scope.TargetId)
			require.NotNil(t, got.MinPurchaseAmount)
			assert.Equal(t, minPurchase, *got.MinPurchaseAmount)
			require.NotNil(t, got.UsableFrom)
			assert.Equal(t, usableFrom, *got.UsableFrom)
			assert.Equal(t, int32(1000), got.TotalLimit)
			assert.Equal(t, int32(1), got.PerUserLimit)
			assert.Equal(t, int32(42), got.IssuedCount)
			require.NotNil(t, got.SuspendedAt)
			assert.Equal(t, suspendedAt, *got.SuspendedAt)
		})

		t.Run("停止していない場合はSuspendedAtがnilになる", func(t *testing.T) {
			t.Parallel()

			got, err := toCampaignResponse(newCampaignView(t))

			require.NoError(t, err)
			assert.Nil(t, got.SuspendedAt)
			assert.Nil(t, got.Scope.TargetId)
		})
	})
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	require.NoError(t, err)

	return d
}

func ptrOf[T any](v T) *T { return &v }

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_campaignuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc, idempotency.Deps{})

	testassert.AssertEchoRouterPath(t, "/v1/campaigns", e.Router().Routes())
	testassert.AssertEchoRouterMethods(t, []string{http.MethodPost, http.MethodGet}, e.Router().Routes())
}

func Test_fromPrimitivePtr(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nil はそのまま nil を返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, fromPrimitivePtr(nil))
		})

		t.Run("値を持つ場合はドメイン語彙の UUID へ写す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "from_primitive_ptr")
			primitive := id.ToPrimitive()

			got := fromPrimitivePtr(&primitive)

			require.NotNil(t, got)
			assert.Equal(t, id, *got)
		})
	})
}

func Test_toPrimitivePtr(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nil はそのまま nil を返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, toPrimitivePtr(nil))
		})

		t.Run("値を持つ場合は OpenAPI 生成型の UUID へ写す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "to_primitive_ptr")

			got := toPrimitivePtr(&id)

			require.NotNil(t, got)
			assert.Equal(t, id.ToPrimitive(), *got)
		})
	})
}
