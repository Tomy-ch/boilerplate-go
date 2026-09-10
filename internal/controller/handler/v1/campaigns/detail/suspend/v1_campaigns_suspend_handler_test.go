package suspend

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/testkit/testassert"
	"go-boilerplate/internal/controller/handler/v1/campaigns/detail/suspend/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	campaignuc "go-boilerplate/internal/usecase/campaign"
	mock_campaignuc "go-boilerplate/internal/usecase/campaign/mock"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
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
	testSuspendedAt     = time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)
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

// newSuspendedView は、ユースケースが返す停止済みキャンペーンを組み立てます。
func newSuspendedView(t *testing.T, id uuid.UUID) campaignuc.CampaignView {
	t.Helper()

	value, err := decimal.Parse("0.10")
	require.NoError(t, err)
	suspendedAt := testSuspendedAt

	return campaignuc.CampaignView{
		ID:              id,
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
		SuspendedAt:     &suspendedAt,
	}
}

func Test_server_PostCampaignsSuspend(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止に成功した場合、200で停止日時を持つキャンペーンを返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			id := uuidtestkit.NewTestFromSalt(t, "campaign")
			uc.EXPECT().
				SuspendCampaign(gomock.Any(), gomock.Any(), id).
				Return(newSuspendedView(t, id), nil)

			got, err := s.PostCampaignsSuspend(authnContext(t), gen.PostCampaignsSuspendRequestObject{
				CampaignId: id.ToPrimitive(),
			})

			require.NoError(t, err)
			res, ok := got.(gen.PostCampaignsSuspend200JSONResponse)
			require.True(t, ok)
			assert.Equal(t, id.ToPrimitive(), res.Id)
			require.NotNil(t, res.SuspendedAt)
			assert.Equal(t, testSuspendedAt, *res.SuspendedAt)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既に停止済みの場合のConflictはそのまま返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			id := uuidtestkit.NewTestFromSalt(t, "campaign")
			uc.EXPECT().
				SuspendCampaign(gomock.Any(), gomock.Any(), id).
				Return(campaignuc.CampaignView{}, apperror.ErrConflict)

			_, err := s.PostCampaignsSuspend(authnContext(t), gen.PostCampaignsSuspendRequestObject{
				CampaignId: id.ToPrimitive(),
			})

			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("存在しない場合のNotFoundはそのまま返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			id := uuidtestkit.NewTestFromSalt(t, "missing_campaign")
			uc.EXPECT().
				SuspendCampaign(gomock.Any(), gomock.Any(), id).
				Return(campaignuc.CampaignView{}, apperror.ErrNotFound)

			_, err := s.PostCampaignsSuspend(authnContext(t), gen.PostCampaignsSuspendRequestObject{
				CampaignId: id.ToPrimitive(),
			})

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

func Test_toCampaignResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止したキャンペーンを応答の語彙へ写す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "campaign")

			got, err := toCampaignResponse(newSuspendedView(t, id))

			require.NoError(t, err)
			assert.Equal(t, id.ToPrimitive(), got.Id)
			assert.Equal(t, "WELCOME-2026", got.Code)
			assert.Equal(t, gen.Rate, got.Discount.Kind)
			assert.Equal(t, gen.All, got.Scope.Kind)
			assert.Equal(t, int32(42), got.IssuedCount)
			require.NotNil(t, got.SuspendedAt)
			assert.Equal(t, testSuspendedAt, *got.SuspendedAt)
		})
	})
}

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_campaignuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc, idempotency.Deps{})

	testassert.AssertEchoRouterPath(t, "/v1/campaigns/:campaignId/suspend", e.Router().Routes())
	testassert.AssertEchoRouterMethods(t, []string{http.MethodPost}, e.Router().Routes())
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

			id := uuidtestkit.NewTestFromSalt(t, "suspend_to_primitive_ptr")

			got := toPrimitivePtr(&id)

			require.NotNil(t, got)
			assert.Equal(t, id.ToPrimitive(), *got)
		})
	})
}
