package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	claims "go-boilerplate/internal/controller/handler/v1/campaigns/claims"
	claimsgen "go-boilerplate/internal/controller/handler/v1/campaigns/claims/gen"
	domaincampaign "go-boilerplate/internal/domain/campaign"
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

const campaignClaimsPath = "/v1/campaigns/claims"

func TestV1CampaignsClaims_Integration(t *testing.T) {
	t.Parallel()

	issuedAt := time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)

	// Idempotency-Key はこの操作では必須なので、ヘッダには必ず載せる。
	// key はケースごとに変えて、リプレイが混ざらないようにする。
	availableClaimer := func(t *testing.T, e *echo.Echo, key string) http.Header {
		t.Helper()

		headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_claimer"))
		headers.Set("Idempotency-Key", key)

		return headers
	}

	newIdempotencyDeps := func(t *testing.T, claimCount, completes int) idempotency.Deps {
		t.Helper()

		ctrl := gomock.NewController(t)
		store := mock_idempotency.NewMockStore(ctrl)
		store.EXPECT().Claim(gomock.Any(), gomock.Any()).Return(true, nil).Times(claimCount)
		store.EXPECT().Complete(gomock.Any(), gomock.Any()).Return(nil).Times(completes)
		txm := mock_tx.NewMockManager(ctrl)
		txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
		).Times(claimCount)

		return idempotency.Deps{
			Txm:   txm,
			Store: store,
			Clock: clocktest.NewMockClock(t, issuedAt),
		}
	}

	newBody := func() *claimsgen.PostCampaignsClaimsJSONRequestBody {
		return &claimsgen.PostCampaignsClaimsJSONRequestBody{Code: "welcome-2026"}
	}

	newView := func(t *testing.T) campaignuc.ClaimedCouponView {
		t.Helper()

		value, err := decimal.Parse("0.10")
		require.NoError(t, err)

		return campaignuc.ClaimedCouponView{
			ID:            uuidtestkit.NewTestFromSalt(t, "integration_claimed_coupon"),
			DiscountKind:  "rate",
			DiscountValue: value,
			ScopeKind:     "all",
			ExpiresAt:     expiresAt,
			IssuedAt:      issuedAt,
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("POST /v1/campaigns/claims が受け取ったクーポンの 201 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)

			var capturedCode string
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, authn *auth.Authn, code string) (campaignuc.ClaimedCouponView, error) {
					require.NotNil(t, authn)
					capturedCode = code

					return newView(t), nil
				},
			)

			claims.BindHandler(e, tf, mockUC, newIdempotencyDeps(t, 1, 1))

			actual := StartServer(t, e).
				DoJSON(http.MethodPost, campaignClaimsPath, newBody(), availableClaimer(t, e, "claim-ok"))

			assert.Equal(t, http.StatusCreated, actual.StatusCode)
			var body claimsgen.CouponResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Equal(t, claimsgen.CouponDiscountKind("rate"), body.Discount.Kind)
			assert.Equal(t, "0.1", body.Discount.Value)
			assert.Nil(t, body.UsedAt)
			assert.Equal(t, expiresAt, body.ExpiresAt)
			assert.Equal(t, issuedAt, body.IssuedAt)
			// 正規化はドメインの責務なので、要求の値がそのまま届く。
			assert.Equal(t, "welcome-2026", capturedCode)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証していない場合は 401 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			// 冪等キーは載せる。欠くと認証より前に 400 で落ち、401 を検証できない。
			headers := http.Header{}
			headers.Set("Idempotency-Key", "claim-unauthenticated")

			actual := StartServer(t, e).DoJSON(http.MethodPost, campaignClaimsPath, newBody(), headers)

			assert.Equal(t, http.StatusUnauthorized, actual.StatusCode)
		})

		// 受け取れない理由は外からは区別できない。5 つとも同じ 422 + details:["code"] になることを、
		// 理由ごとに独立したケースで固定する（どれが壊れたかがテスト名に出るように）。
		t.Run("存在しないコード は 422 と details:[\"code\"] を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.ClaimedCouponView{}, apperror.WithDetails(domaincampaign.ErrCodeUnknown, domaincampaign.FieldCode))

			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).DoJSON(
				http.MethodPost, campaignClaimsPath, newBody(), availableClaimer(t, e, "claim-unknown"),
			)

			assert.Equal(t, http.StatusUnprocessableEntity, actual.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Equal(t, []any{"code"}, body["details"])
		})

		t.Run("配布期間の外 は 422 と details:[\"code\"] を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.ClaimedCouponView{}, apperror.WithDetails(domaincampaign.ErrNotDistributing, domaincampaign.FieldCode))

			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).DoJSON(
				http.MethodPost, campaignClaimsPath, newBody(), availableClaimer(t, e, "claim-not-distributing"),
			)

			assert.Equal(t, http.StatusUnprocessableEntity, actual.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Equal(t, []any{"code"}, body["details"])
		})

		t.Run("停止済み は 422 と details:[\"code\"] を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.ClaimedCouponView{}, apperror.WithDetails(domaincampaign.ErrSuspended, domaincampaign.FieldCode))

			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).DoJSON(
				http.MethodPost, campaignClaimsPath, newBody(), availableClaimer(t, e, "claim-suspended"),
			)

			assert.Equal(t, http.StatusUnprocessableEntity, actual.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Equal(t, []any{"code"}, body["details"])
		})

		t.Run("総枚数上限に到達 は 422 と details:[\"code\"] を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.ClaimedCouponView{}, apperror.WithDetails(domaincampaign.ErrTotalLimitReached, domaincampaign.FieldCode))

			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).DoJSON(
				http.MethodPost, campaignClaimsPath, newBody(), availableClaimer(t, e, "claim-total-limit"),
			)

			assert.Equal(t, http.StatusUnprocessableEntity, actual.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Equal(t, []any{"code"}, body["details"])
		})

		t.Run("1人あたり上限に到達 は 422 と details:[\"code\"] を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.ClaimedCouponView{}, apperror.WithDetails(domaincampaign.ErrPerUserLimitReached, domaincampaign.FieldCode))

			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).DoJSON(
				http.MethodPost, campaignClaimsPath, newBody(), availableClaimer(t, e, "claim-per-user-limit"),
			)

			assert.Equal(t, http.StatusUnprocessableEntity, actual.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Equal(t, []any{"code"}, body["details"])
		})

		t.Run("Idempotency-Key を欠く場合は 400 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_claimer"))

			actual := StartServer(t, e).DoJSON(http.MethodPost, campaignClaimsPath, newBody(), headers)

			assert.Equal(t, http.StatusBadRequest, actual.StatusCode)
		})

		t.Run("ユースケースが Conflict を返した場合は 409 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_campaign.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().ClaimCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(campaignuc.ClaimedCouponView{}, apperror.ErrConflict)

			claims.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).DoJSON(
				http.MethodPost, campaignClaimsPath, newBody(), availableClaimer(t, e, "claim-conflict"),
			)

			assert.Equal(t, http.StatusConflict, actual.StatusCode)
		})
	})
}
