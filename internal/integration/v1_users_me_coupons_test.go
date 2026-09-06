package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	usersmecoupons "go-boilerplate/internal/controller/handler/v1/users/me/coupons"
	usersmecouponsgen "go-boilerplate/internal/controller/handler/v1/users/me/coupons/gen"
	"go-boilerplate/internal/observability"
	couponuc "go-boilerplate/internal/usecase/coupon"
	mock_couponuc "go-boilerplate/internal/usecase/coupon/mock"
	"go-boilerplate/pkg/decimal"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const usersMeCouponsPath = "/v1/users/me/coupons"

// couponViewFixture は、カテゴリ限定の定率クーポンのユースケース出力を返します。
func couponViewFixture(t *testing.T) couponuc.CouponView {
	t.Helper()
	value, err := decimal.Parse("0.10")
	require.NoError(t, err)
	target := uuidtestkit.NewTestFromSalt(t, "integration_coupon_category")

	return couponuc.CouponView{
		ID:            uuidtestkit.NewTestFromSalt(t, "integration_coupon"),
		DiscountKind:  "rate",
		DiscountValue: value,
		ScopeKind:     "category",
		ScopeTargetID: &target,
		ExpiresAt:     time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		IssuedAt:      time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestV1UsersMeCoupons_Integration(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("GET /v1/users/me/coupons が保有クーポンを 2 軸つきで返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)
			uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))
			view := couponViewFixture(t)
			uc.EXPECT().ListMyCoupons(gomock.Any(), gomock.Any()).Return([]couponuc.CouponView{view}, nil)

			usersmecoupons.BindHandler(e, tf, uc)
			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_coupon_user"))

			actual := StartServer(t, e).DoJSON(http.MethodGet, usersMeCouponsPath, nil, headers)
			assert.Equal(t, http.StatusOK, actual.StatusCode)

			var body usersmecouponsgen.CouponListResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			require.Len(t, body.Coupons, 1)
			assert.Equal(t, view.ID.ToPrimitive(), body.Coupons[0].Id)
			assert.Equal(t, "rate", string(body.Coupons[0].Discount.Kind))
			assert.Equal(t, "category", string(body.Coupons[0].Scope.Kind))
			require.NotNil(t, body.Coupons[0].Scope.TargetId)
		})

		t.Run("1 枚も持たない場合は空配列を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))
			uc.EXPECT().ListMyCoupons(gomock.Any(), gomock.Any()).Return([]couponuc.CouponView{}, nil)

			usersmecoupons.BindHandler(e, observability.NewNoopTracerFactory(t), uc)
			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_coupon_empty"))

			actual := StartServer(t, e).DoJSON(http.MethodGet, usersMeCouponsPath, nil, headers)
			assert.Equal(t, http.StatusOK, actual.StatusCode)

			var body usersmecouponsgen.CouponListResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			assert.Empty(t, body.Coupons)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("トークン無しは 401 で拒まれユースケースへ届かない", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))
			uc.EXPECT().ListMyCoupons(gomock.Any(), gomock.Any()).Times(0)
			usersmecoupons.BindHandler(e, observability.NewNoopTracerFactory(t), uc)

			actual := StartServer(t, e).DoJSON(http.MethodGet, usersMeCouponsPath, nil, nil)
			AssertErrorResponse(t, actual, http.StatusUnauthorized)
		})

		t.Run("ユースケースの失敗は 500 へ写る", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))
			uc.EXPECT().ListMyCoupons(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrInternal)
			usersmecoupons.BindHandler(e, observability.NewNoopTracerFactory(t), uc)
			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_coupon_err"))

			actual := StartServer(t, e).DoJSON(http.MethodGet, usersMeCouponsPath, nil, headers)
			AssertErrorResponse(t, actual, http.StatusInternalServerError)
		})
	})
}
