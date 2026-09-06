package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"go-boilerplate/internal/apperror"
	cartscoupons "go-boilerplate/internal/controller/handler/v1/carts/coupons"
	cartscouponsgen "go-boilerplate/internal/controller/handler/v1/carts/coupons/gen"
	"go-boilerplate/internal/observability"
	couponuc "go-boilerplate/internal/usecase/coupon"
	mock_couponuc "go-boilerplate/internal/usecase/coupon/mock"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const cartsMeCouponsPath = "/v1/carts/me/coupons"

func TestV1CartsMeCoupons_Integration(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("GET /v1/carts/me/coupons が使えるクーポンを値引き額つきで返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			tf := observability.NewNoopTracerFactory(t)
			uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))
			view := couponViewFixture(t)
			uc.EXPECT().ListApplicableToMyCart(gomock.Any(), gomock.Any()).
				Return([]couponuc.CartCouponView{{Coupon: view, DiscountAmount: 332}}, nil)

			cartscoupons.BindHandler(e, tf, uc)
			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_cart_coupon_user"))

			actual := StartServer(t, e).DoJSON(http.MethodGet, cartsMeCouponsPath, nil, headers)
			assert.Equal(t, http.StatusOK, actual.StatusCode)

			var body cartscouponsgen.CartCouponListResponse
			require.NoError(t, json.NewDecoder(actual.Body).Decode(&body))
			require.Len(t, body.Coupons, 1)
			assert.Equal(t, view.ID.ToPrimitive(), body.Coupons[0].Coupon.Id)
			assert.Equal(t, int64(332), body.Coupons[0].DiscountAmount)
			assert.Equal(t, "rate", string(body.Coupons[0].Coupon.Discount.Kind))
			assert.Equal(t, "category", string(body.Coupons[0].Coupon.Scope.Kind))
		})

		t.Run("使えるものが無い場合は空配列を返す", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))
			uc.EXPECT().ListApplicableToMyCart(gomock.Any(), gomock.Any()).Return([]couponuc.CartCouponView{}, nil)

			cartscoupons.BindHandler(e, observability.NewNoopTracerFactory(t), uc)
			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_cart_coupon_empty"))

			actual := StartServer(t, e).DoJSON(http.MethodGet, cartsMeCouponsPath, nil, headers)
			assert.Equal(t, http.StatusOK, actual.StatusCode)

			var body cartscouponsgen.CartCouponListResponse
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
			uc.EXPECT().ListApplicableToMyCart(gomock.Any(), gomock.Any()).Times(0)
			cartscoupons.BindHandler(e, observability.NewNoopTracerFactory(t), uc)

			actual := StartServer(t, e).DoJSON(http.MethodGet, cartsMeCouponsPath, nil, nil)
			AssertErrorResponse(t, actual, http.StatusUnauthorized)
		})

		t.Run("ユースケースの失敗は 500 へ写る", func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			UseAppErrorHandler(t, e)
			uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))
			uc.EXPECT().ListApplicableToMyCart(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrInternal)
			cartscoupons.BindHandler(e, observability.NewNoopTracerFactory(t), uc)
			headers := MakeAvailableUserID(t, e, uuidtestkit.NewTestFromSalt(t, "integration_cart_coupon_err"))

			actual := StartServer(t, e).DoJSON(http.MethodGet, cartsMeCouponsPath, nil, headers)
			AssertErrorResponse(t, actual, http.StatusInternalServerError)
		})
	})
}
