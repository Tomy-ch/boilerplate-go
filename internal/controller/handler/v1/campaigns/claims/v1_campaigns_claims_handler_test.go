package claims

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/testkit/testassert"
	"go-boilerplate/internal/controller/handler/v1/campaigns/claims/gen"
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
	testIssuedAt  = time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)
	testExpiresAt = time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
)

// authnContext は、内部ユーザー ID を解決済みの認証コンテキストを返すテストヘルパーです。
func authnContext(t *testing.T) context.Context {
	t.Helper()
	ctx := ctxhelper.WithAuthn(context.Background())
	authn, err := auth.New("subject", "issuer", nil, nil)
	require.NoError(t, err)

	resolved, err := authn.WithUserID(uuidtestkit.NewTestFromSalt(t, "claimer"))
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

// newClaimedView は、ユースケースが返す受け取り済みクーポンを組み立てます。
func newClaimedView(t *testing.T) campaignuc.ClaimedCouponView {
	t.Helper()

	value, err := decimal.Parse("0.10")
	require.NoError(t, err)

	return campaignuc.ClaimedCouponView{
		ID:            uuidtestkit.NewTestFromSalt(t, "claimed_coupon"),
		DiscountKind:  "rate",
		DiscountValue: value,
		ScopeKind:     "all",
		ExpiresAt:     testExpiresAt,
		IssuedAt:      testIssuedAt,
	}
}

func Test_server_PostCampaignsClaims(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取りに成功した場合、201で未使用のクーポンを返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			want := newClaimedView(t)
			uc.EXPECT().
				ClaimCoupon(gomock.Any(), gomock.Any(), "welcome-2026").
				Return(want, nil)

			got, err := s.PostCampaignsClaims(authnContext(t), gen.PostCampaignsClaimsRequestObject{
				Body: &gen.CampaignClaimsPostRequest{Code: "welcome-2026"},
			})

			require.NoError(t, err)
			res, ok := got.(gen.PostCampaignsClaims201JSONResponse)
			require.True(t, ok)
			assert.Equal(t, want.ID.ToPrimitive(), res.Id)
			assert.Nil(t, res.UsedAt)
		})

		t.Run("コードは正規化せずそのままユースケースへ渡す", func(t *testing.T) {
			t.Parallel()

			// 正規化はドメインの責務であり、ハンドラは要求と応答だけを扱う。
			s, uc := newServer(t)
			uc.EXPECT().
				ClaimCoupon(gomock.Any(), gomock.Any(), "  WELCOME-2026  ").
				Return(newClaimedView(t), nil)

			_, err := s.PostCampaignsClaims(authnContext(t), gen.PostCampaignsClaimsRequestObject{
				Body: &gen.CampaignClaimsPostRequest{Code: "  WELCOME-2026  "},
			})

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取れない場合のエラーはそのまま返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().
				ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.ClaimedCouponView{}, apperror.ErrValidation)

			_, err := s.PostCampaignsClaims(authnContext(t), gen.PostCampaignsClaimsRequestObject{
				Body: &gen.CampaignClaimsPostRequest{Code: "NOSUCHCODE-0001"},
			})

			require.ErrorIs(t, err, apperror.ErrValidation)
		})
	})
}

func Test_toCouponResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取ったクーポンを応答の語彙へ写す", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			minPurchase := int64(5000)
			usableFrom := testExpiresAt.Add(-24 * time.Hour)
			targetID := uuidtestkit.NewTestFromSalt(t, "scope_target")
			view := newClaimedView(t)
			view.MaxAmount = &maxAmount
			view.MinPurchaseAmount = &minPurchase
			view.UsableFrom = &usableFrom
			view.ScopeKind = "product"
			view.ScopeTargetID = &targetID

			got := toCouponResponse(view)

			assert.Equal(t, view.ID.ToPrimitive(), got.Id)
			assert.Equal(t, gen.Rate, got.Discount.Kind)
			assert.Equal(t, "0.1", got.Discount.Value)
			require.NotNil(t, got.Discount.MaxAmount)
			assert.Equal(t, maxAmount, *got.Discount.MaxAmount)
			assert.Equal(t, gen.Product, got.Scope.Kind)
			require.NotNil(t, got.Scope.TargetId)
			assert.Equal(t, targetID.ToPrimitive(), *got.Scope.TargetId)
			require.NotNil(t, got.MinPurchaseAmount)
			assert.Equal(t, minPurchase, *got.MinPurchaseAmount)
			require.NotNil(t, got.UsableFrom)
			assert.Equal(t, usableFrom, *got.UsableFrom)
			assert.Equal(t, testExpiresAt, got.ExpiresAt)
			assert.Equal(t, testIssuedAt, got.IssuedAt)
			assert.Nil(t, got.UsedAt)
		})

		t.Run("条件を持たないクーポンは任意項目がnilになる", func(t *testing.T) {
			t.Parallel()

			got := toCouponResponse(newClaimedView(t))

			assert.Nil(t, got.Discount.MaxAmount)
			assert.Nil(t, got.Scope.TargetId)
			assert.Nil(t, got.MinPurchaseAmount)
			assert.Nil(t, got.UsableFrom)
		})
	})
}

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_campaignuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc, idempotency.Deps{})

	testassert.AssertEchoRouterPath(t, "/v1/campaigns/claims", e.Router().Routes())
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

			id := uuidtestkit.NewTestFromSalt(t, "claims_to_primitive_ptr")

			got := toPrimitivePtr(&id)

			require.NotNil(t, got)
			assert.Equal(t, id.ToPrimitive(), *got)
		})
	})
}
