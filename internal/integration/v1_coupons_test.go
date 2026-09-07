package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	coupons "go-boilerplate/internal/controller/handler/v1/coupons"
	couponsgen "go-boilerplate/internal/controller/handler/v1/coupons/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	clocktest "go-boilerplate/internal/usecase/boundary/clock/testkit"
	mock_idempotency "go-boilerplate/internal/usecase/boundary/idempotency/mock"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	couponuc "go-boilerplate/internal/usecase/coupon"
	mock_coupon "go-boilerplate/internal/usecase/coupon/mock"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const couponsPath = "/v1/coupons"

func TestV1Coupons_Integration(t *testing.T) {
	t.Parallel()

	issuedAt := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	recipientID := uuidtestkit.NewTestFromSalt(t, "integration_issue_recipient")

	// Idempotency-Key はこの操作では任意なので、付ける場合だけ key を渡す。
	// key はケースごとに変えて、リプレイが混ざらないようにする。
	availableAdmin := func(t *testing.T, e *echo.Echo, key string) http.Header {
		t.Helper()

		headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_issue_admin"))
		if key != "" {
			headers.Set("Idempotency-Key", key)
		}

		return headers
	}

	// claims / completes に期待回数を渡し、キーを伴う要求で実際に claim → complete まで
	// 走ったかを検証する。AnyTimes にすると middleware の配線が壊れて idempotency.Run が
	// 素通りしても 201 が返るため、退行を検出できない。
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
			Clock: clocktest.NewMockClock(t, issuedAt),
		}
	}

	newBody := func() *couponsgen.PostCouponsJSONRequestBody {
		return &couponsgen.PostCouponsJSONRequestBody{
			UserId:    recipientID.ToPrimitive(),
			Discount:  couponsgen.CouponDiscountInput{Kind: couponsgen.CouponDiscountInputKindFlat, Value: "500"},
			Scope:     couponsgen.CouponScopeInput{Kind: couponsgen.CouponScopeInputKindAll},
			ExpiresAt: expiresAt,
		}
	}

	newView := func(t *testing.T) couponuc.CouponView {
		t.Helper()

		value, err := decimal.Parse("500")
		require.NoError(t, err)

		return couponuc.CouponView{
			ID:            uuidtestkit.NewTestFromSalt(t, "integration_issued_coupon"),
			DiscountKind:  "flat",
			DiscountValue: value,
			ScopeKind:     "all",
			ExpiresAt:     expiresAt,
			IssuedAt:      issuedAt,
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("POST /v1/coupons が admin で発行したクーポンの 201 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)

			var captured couponuc.IssueCouponParams
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(
					_ context.Context, authn *auth.Authn, params couponuc.IssueCouponParams,
				) (couponuc.CouponView, error) {
					require.NotNil(t, authn)
					captured = params

					return newView(t), nil
				},
			)

			coupons.BindHandler(e, tf, mockUC, newIdempotencyDeps(t, 1, 1))

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, newBody(), availableAdmin(t, e, "ok-basic"))
			assert.Equal(t, http.StatusCreated, actual.StatusCode)
			var body couponsgen.CouponResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))

			assert.Equal(t, couponsgen.CouponDiscountKind("flat"), body.Discount.Kind)
			assert.Equal(t, "500", body.Discount.Value)
			assert.Equal(t, couponsgen.CouponScopeKind("all"), body.Scope.Kind)
			assert.Nil(t, body.Scope.TargetId)
			assert.Nil(t, body.UsedAt)
			assert.Equal(t, expiresAt, body.ExpiresAt)
			assert.Equal(t, issuedAt, body.IssuedAt)
			assert.Equal(t, recipientID, captured.UserID)
			assert.Equal(t, "flat", captured.DiscountKind)
			assert.Equal(t, "500", captured.DiscountValue.String())
			assert.Equal(t, "all", captured.ScopeKind)
			assert.Nil(t, captured.ScopeTargetID)
			assert.Equal(t, expiresAt, captured.ExpiresAt)
		})

		t.Run("Idempotency-Key を欠く要求も 201 を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)

			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).Return(newView(t), nil)

			coupons.BindHandler(e, tf, mockUC, newIdempotencyDeps(t, 0, 0))
			useOpenAPIValidation(t, e)

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, newBody(), availableAdmin(t, e, ""))
			assert.Equal(t, http.StatusCreated, actual.StatusCode)
		})

		t.Run("適用範囲の対象IDをユースケースへ渡す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)
			targetID := uuidtestkit.NewTestFromSalt(t, "integration_issue_target")
			primitive := targetID.ToPrimitive()

			var captured *uuid.UUID
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(
					_ context.Context, _ *auth.Authn, params couponuc.IssueCouponParams,
				) (couponuc.CouponView, error) {
					captured = params.ScopeTargetID

					return newView(t), nil
				},
			)

			coupons.BindHandler(e, tf, mockUC, newIdempotencyDeps(t, 1, 1))

			body := newBody()
			body.Scope = couponsgen.CouponScopeInput{Kind: couponsgen.CouponScopeInputKindCategory, TargetId: &primitive}

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, body, availableAdmin(t, e, "ok-target"))
			assert.Equal(t, http.StatusCreated, actual.StatusCode)
			require.NotNil(t, captured)
			assert.Equal(t, targetID, *captured)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("トークン無しの発行要求は401で拒まれユースケースへ届かない", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			coupons.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, newBody(), http.Header{})
			AssertErrorResponse(t, actual, http.StatusUnauthorized)
		})

		t.Run("非 admin の権限エラーは403を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			tf := observability.NewNoopTracerFactory(t)

			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.CouponView{}, apperror.ErrPermissionDenied)

			coupons.BindHandler(e, tf, mockUC, newIdempotencyDeps(t, 1, 0))

			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_issue_member"))
			headers.Set("Idempotency-Key", "forbidden-member")
			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, newBody(), headers)
			AssertErrorResponse(t, actual, http.StatusForbidden)
		})

		t.Run("受給者が在籍しない場合は404を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			tf := observability.NewNoopTracerFactory(t)

			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.CouponView{}, apperror.ErrNotFound)

			coupons.BindHandler(e, tf, mockUC, newIdempotencyDeps(t, 1, 0))

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, newBody(), availableAdmin(t, e, "not-found"))
			AssertErrorResponse(t, actual, http.StatusNotFound)
		})

		t.Run("宣言に無いフィールドを含む要求は400で拒まれユースケースへ届かない", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			coupons.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))
			headers := availableAdmin(t, e, "unknown-field")
			useOpenAPIValidation(t, e)

			body := map[string]any{
				"userId":    recipientID.String(),
				"discount":  map[string]any{"kind": "flat", "value": "500"},
				"scope":     map[string]any{"kind": "all", "targetId": nil},
				"expiresAt": expiresAt.Format(time.RFC3339),
				"reason":    "compensation",
			}

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, body, headers)
			AssertErrorResponse(t, actual, http.StatusBadRequest)
		})

		t.Run("値引きの値が形式に合わない要求は400で拒まれユースケースへ届かない", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			mockUC := mock_coupon.NewMockUsecase(gomock.NewController(t))
			mockUC.EXPECT().IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			coupons.BindHandler(e, observability.NewNoopTracerFactory(t), mockUC, newIdempotencyDeps(t, 0, 0))

			body := newBody()
			body.Discount.Value = "five-hundred"

			actual := StartServer(t, e).DoJSON(http.MethodPost, couponsPath, body, availableAdmin(t, e, ""))
			AssertErrorResponse(t, actual, http.StatusBadRequest)
		})
	})
}
