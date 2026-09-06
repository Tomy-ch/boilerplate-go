package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	couponsbulkissue "go-boilerplate/internal/controller/handler/v1/coupons/bulkissue"
	couponsbulkissuegen "go-boilerplate/internal/controller/handler/v1/coupons/bulkissue/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	couponuc "go-boilerplate/internal/usecase/coupon"
	mock_coupon "go-boilerplate/internal/usecase/coupon/mock"
	"go-boilerplate/internal/usecase/idempotency"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const couponsBulkIssuePath = "/v1/coupons/bulk-issue"

func TestV1CouponsBulkIssue_Integration(t *testing.T) {
	t.Parallel()

	issuedAt := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

	availableAdmin := func(t *testing.T, e *echo.Echo) http.Header {
		t.Helper()

		return MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_bulk_issue_admin"))
	}

	newBody := func() *couponsbulkissuegen.PostCouponsBulkIssueJSONRequestBody {
		return &couponsbulkissuegen.PostCouponsBulkIssueJSONRequestBody{
			Discount:  couponsbulkissuegen.CouponDiscountInput{Kind: couponsbulkissuegen.Rate, Value: "0.15"},
			Scope:     couponsbulkissuegen.CouponScopeInput{Kind: couponsbulkissuegen.All},
			ExpiresAt: expiresAt,
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("POST /v1/coupons/bulk-issue が admin で件数つきの 200 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)

			var captured couponuc.IssuePromotionalCouponsParams
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(
					_ context.Context, authn *auth.Authn, params couponuc.IssuePromotionalCouponsParams,
				) (couponuc.IssuePromotionalCouponsView, error) {
					require.NotNil(t, authn)
					captured = params

					return couponuc.IssuePromotionalCouponsView{
						IssuedAt:          issuedAt,
						ExpiresAt:         expiresAt,
						RecipientCount:    1204,
						IssuedCouponCount: 1204,
					}, nil
				},
			)

			couponsbulkissue.BindHandler(e, tf, mockUC, idempotency.Deps{})

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, newBody(), availableAdmin(t, e))
			assert.Equal(t, http.StatusOK, actual.StatusCode)
			var body couponsbulkissuegen.CouponBulkIssueResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))

			assert.Equal(t, issuedAt, body.IssuedAt)
			assert.Equal(t, expiresAt, body.ExpiresAt)
			assert.Equal(t, int64(1204), body.RecipientCount)
			assert.Equal(t, int64(1204), body.IssuedCouponCount)
			assert.Equal(t, "rate", captured.DiscountKind)
			assert.Equal(t, "0.15", captured.DiscountValue.String())
			assert.Equal(t, "all", captured.ScopeKind)
			assert.Nil(t, captured.ScopeTargetID)
			assert.Equal(t, expiresAt, captured.ExpiresAt)
		})

		t.Run("受給者が 0 人でも件数 0 の 200 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)

			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.IssuePromotionalCouponsView{IssuedAt: issuedAt, ExpiresAt: expiresAt}, nil)

			couponsbulkissue.BindHandler(e, tf, mockUC, idempotency.Deps{})

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, newBody(), availableAdmin(t, e))
			assert.Equal(t, http.StatusOK, actual.StatusCode)
			var body couponsbulkissuegen.CouponBulkIssueResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))

			assert.Zero(t, body.RecipientCount)
			assert.Zero(t, body.IssuedCouponCount)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("トークン無しの発行要求は401で拒まれユースケースへ届かない", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			couponsbulkissue.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, idempotency.Deps{})

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, newBody(), nil)
			AssertErrorResponse(t, actual, http.StatusUnauthorized)
		})

		t.Run("非 admin の権限エラーは403を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			tf := observability.NewNoopTracerFactory(t)

			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.IssuePromotionalCouponsView{}, apperror.ErrPermissionDenied)

			couponsbulkissue.BindHandler(e, tf, mockUC, idempotency.Deps{})

			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_bulk_issue_member"))
			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, newBody(), headers)
			AssertErrorResponse(t, actual, http.StatusForbidden)
		})

		t.Run("受給者が上限を超える場合は409を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			tf := observability.NewNoopTracerFactory(t)

			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.IssuePromotionalCouponsView{}, couponuc.ErrTooManyRecipients)

			couponsbulkissue.BindHandler(e, tf, mockUC, idempotency.Deps{})

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, newBody(), availableAdmin(t, e))
			AssertErrorResponse(t, actual, http.StatusConflict)
		})

		t.Run("適用範囲が指す対象が存在しない場合は404を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			tf := observability.NewNoopTracerFactory(t)

			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.IssuePromotionalCouponsView{}, apperror.ErrNotFound)

			couponsbulkissue.BindHandler(e, tf, mockUC, idempotency.Deps{})

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, newBody(), availableAdmin(t, e))
			AssertErrorResponse(t, actual, http.StatusNotFound)
		})

		t.Run("宣言に無いフィールドを含む要求は400で拒まれユースケースへ届かない", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			couponsbulkissue.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, idempotency.Deps{})
			headers := availableAdmin(t, e)
			useOpenAPIValidation(t, e)

			body := map[string]any{
				"discount":  map[string]any{"kind": "rate", "value": "0.15"},
				"scope":     map[string]any{"kind": "all", "targetId": nil},
				"expiresAt": expiresAt.Format(time.RFC3339),
				"recipient": "everyone",
			}

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, body, headers)
			AssertErrorResponse(t, actual, http.StatusBadRequest)
		})

		t.Run("値引きの値が形式に合わない要求は400で拒まれユースケースへ届かない", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			couponsbulkissue.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, idempotency.Deps{})

			body := newBody()
			body.Discount.Value = "fifteen-percent"

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsBulkIssuePath, body, availableAdmin(t, e))
			AssertErrorResponse(t, actual, http.StatusBadRequest)
		})
	})
}
