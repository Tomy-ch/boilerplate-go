package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	campaigns "go-boilerplate/internal/controller/handler/v1/campaigns"
	campaignsgen "go-boilerplate/internal/controller/handler/v1/campaigns/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	clocktest "go-boilerplate/internal/usecase/boundary/clock/testkit"
	mock_idempotency "go-boilerplate/internal/usecase/boundary/idempotency/mock"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	campaignuc "go-boilerplate/internal/usecase/campaign"
	mock_campaign "go-boilerplate/internal/usecase/campaign/mock"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/decimal"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const campaignsPath = "/v1/campaigns"

func TestV1Campaigns_Integration(t *testing.T) {
	t.Parallel()

	definedAt := time.Date(2026, time.September, 20, 0, 0, 0, 0, time.UTC)
	startsAt := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	endsAt := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	couponExpiresAt := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)

	availableAdmin := func(t *testing.T, e *echo.Echo, key string) http.Header {
		t.Helper()

		headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_campaign_admin"))
		if key != "" {
			headers.Set("Idempotency-Key", key)
		}

		return headers
	}

	newIdempotencyDeps := func(t *testing.T, claims, completes int) idempotency.Deps {
		t.Helper()

		ctrl := gomock.NewController(t)
		store := mock_idempotency.NewMockStore(ctrl)
		store.EXPECT().Claim(gomock.Any(), gomock.Any()).Return(true, nil).Times(claims)
		store.EXPECT().Complete(gomock.Any(), gomock.Any()).Return(nil).Times(completes)
		txm := mock_tx.NewMockManager(ctrl)
		txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
		).Times(claims)

		return idempotency.Deps{
			Txm:   txm,
			Store: store,
			Clock: clocktest.NewMockClock(t, definedAt),
		}
	}

	newBody := func() *campaignsgen.PostCampaignsJSONRequestBody {
		return &campaignsgen.PostCampaignsJSONRequestBody{
			Code:            "welcome-2026",
			Discount:        campaignsgen.CouponDiscountInput{Kind: campaignsgen.CouponDiscountInputKindRate, Value: "0.10"},
			Scope:           campaignsgen.CouponScopeInput{Kind: campaignsgen.CouponScopeInputKindAll},
			CouponExpiresAt: couponExpiresAt,
			StartsAt:        startsAt,
			EndsAt:          endsAt,
			TotalLimit:      1000,
			PerUserLimit:    1,
		}
	}

	newView := func(t *testing.T) campaignuc.CampaignView {
		t.Helper()

		value, err := decimal.Parse("0.10")
		require.NoError(t, err)

		return campaignuc.CampaignView{
			ID:              uuidtestkit.NewTestFromSalt(t, "integration_campaign"),
			Code:            "WELCOME-2026",
			DiscountKind:    "rate",
			DiscountValue:   value,
			ScopeKind:       "all",
			CouponExpiresAt: couponExpiresAt,
			StartsAt:        startsAt,
			EndsAt:          endsAt,
			TotalLimit:      1000,
			PerUserLimit:    1,
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("POST /v1/campaigns が定義したキャンペーンの 201 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			var captured campaignuc.DefineCampaignParams
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().DefineCampaign(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(
					_ context.Context, authn *auth.Authn, params campaignuc.DefineCampaignParams,
				) (campaignuc.CampaignView, error) {
					require.NotNil(t, authn)
					captured = params

					return newView(t), nil
				},
			)

			campaigns.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 1))

			actual := StartServer(t, e).
				DoJSON(http.MethodPost, campaignsPath, newBody(), availableAdmin(t, e, "define-ok"))

			assert.Equal(t, http.StatusCreated, actual.StatusCode)
			var body campaignsgen.CampaignResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Equal(t, "WELCOME-2026", body.Code)
			assert.Equal(t, campaignsgen.CouponDiscountKind("rate"), body.Discount.Kind)
			assert.Equal(t, int32(1000), body.TotalLimit)
			assert.Equal(t, int32(1), body.PerUserLimit)
			assert.Zero(t, body.IssuedCount)
			assert.Nil(t, body.SuspendedAt)
			assert.Equal(t, "welcome-2026", captured.Code)
			assert.Equal(t, 1000, captured.TotalLimit)
		})

		t.Run("GET /v1/campaigns が一覧と総件数の 200 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ListCampaigns(gomock.Any(), gomock.Any(), gomock.Any()).Return(
				campaignuc.CampaignListView{Campaigns: []campaignuc.CampaignView{newView(t)}, Total: 7}, nil,
			)

			campaigns.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			actual := StartServer(t, e).Do(http.MethodGet, campaignsPath, nil, "", availableAdmin(t, e, ""))

			assert.Equal(t, http.StatusOK, actual.StatusCode)
			var body campaignsgen.CampaignListResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			require.Len(t, body.Campaigns, 1)
			assert.Equal(t, int32(7), body.Total)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証していない場合は 401 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			campaigns.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			actual := StartServer(t, e).DoJSON(http.MethodPost, campaignsPath, newBody(), nil)

			assert.Equal(t, http.StatusUnauthorized, actual.StatusCode)
		})

		t.Run("非 admin は 403 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().DefineCampaign(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.CampaignView{}, apperror.ErrPermissionDenied)

			campaigns.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).
				DoJSON(http.MethodPost, campaignsPath, newBody(), availableAdmin(t, e, "define-forbidden"))

			assert.Equal(t, http.StatusForbidden, actual.StatusCode)
		})

		t.Run("Idempotency-Key を欠く場合は 400 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			campaigns.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			actual := StartServer(t, e).DoJSON(http.MethodPost, campaignsPath, newBody(), availableAdmin(t, e, ""))

			assert.Equal(t, http.StatusBadRequest, actual.StatusCode)
		})

		t.Run("コードが既に使われている場合は 409 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().DefineCampaign(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.CampaignView{}, apperror.ErrConflict)

			campaigns.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).
				DoJSON(http.MethodPost, campaignsPath, newBody(), availableAdmin(t, e, "define-conflict"))

			assert.Equal(t, http.StatusConflict, actual.StatusCode)
		})

		t.Run("宣言に無いフィールドを含む要求は 400 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			campaigns.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			body := map[string]any{
				"code":            "welcome-2026",
				"discount":        map[string]any{"kind": "rate", "value": "0.10"},
				"scope":           map[string]any{"kind": "all"},
				"couponExpiresAt": couponExpiresAt,
				"startsAt":        startsAt,
				"endsAt":          endsAt,
				"totalLimit":      1000,
				"perUserLimit":    1,
				"unknownField":    "x",
			}

			actual := StartServer(t, e).
				DoJSON(http.MethodPost, campaignsPath, body, availableAdmin(t, e, "define-unknown"))

			assert.Equal(t, http.StatusBadRequest, actual.StatusCode)
		})
	})
}
