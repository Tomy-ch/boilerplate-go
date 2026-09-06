package cartscoupons

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/carts/coupons/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	couponuc "go-boilerplate/internal/usecase/coupon"
	mock_couponuc "go-boilerplate/internal/usecase/coupon/mock"
	"go-boilerplate/pkg/decimal"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	testExpiresAt = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	testIssuedAt  = time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
)

// authnContext は、内部ユーザー ID を解決済みの認証コンテキストを返すテストヘルパーです。
func authnContext(t *testing.T) context.Context {
	t.Helper()
	ctx := ctxhelper.WithAuthn(context.Background())
	authn, err := auth.New("subject", "issuer", nil, nil)
	require.NoError(t, err)

	resolved, err := authn.WithUserID(uuidtestkit.NewTestFromSalt(t, "coupons_user"))
	require.NoError(t, err)
	require.True(t, ctxhelper.SetAuthn(ctx, *resolved))

	return ctx
}

func newServer(t *testing.T) (*server, *mock_couponuc.MockUsecase) {
	t.Helper()
	uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))

	return &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc}, uc
}

// newCouponView は、カテゴリ限定の定率クーポンのビューを組み立てます。
func newCouponView(t *testing.T, salt string) couponuc.CouponView {
	t.Helper()
	value, err := decimal.Parse("0.10")
	require.NoError(t, err)
	target := uuidtestkit.NewTestFromSalt(t, salt+"_category")

	return couponuc.CouponView{
		ID:            uuidtestkit.NewTestFromSalt(t, salt),
		DiscountKind:  "rate",
		DiscountValue: value,
		ScopeKind:     "category",
		ScopeTargetID: &target,
		ExpiresAt:     testExpiresAt,
		IssuedAt:      testIssuedAt,
	}
}

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc)

	routes := e.Router().Routes()
	require.Len(t, routes, 1)
	assert.Equal(t, http.MethodGet, routes[0].Method)
	assert.Equal(t, "/v1/carts/me/coupons", routes[0].Path)
}

func Test_server_GetCartsMeCoupons(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("使えるクーポンを値引き額つきの200で返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			view := newCouponView(t, "held")
			uc.EXPECT().
				ListApplicableToMyCart(gomock.Any(), gomock.Any()).
				Return([]couponuc.CartCouponView{{Coupon: view, DiscountAmount: 332}}, nil)

			resp, err := s.GetCartsMeCoupons(authnContext(t), gen.GetCartsMeCouponsRequestObject{})
			require.NoError(t, err)

			actual, ok := resp.(gen.GetCartsMeCoupons200JSONResponse)
			require.True(t, ok)
			require.Len(t, actual.Coupons, 1)
			assert.Equal(t, view.ID.ToPrimitive(), actual.Coupons[0].Coupon.Id)
			assert.Equal(t, int64(332), actual.Coupons[0].DiscountAmount)
		})

		t.Run("使えるものが無い場合は空配列を返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().ListApplicableToMyCart(gomock.Any(), gomock.Any()).Return([]couponuc.CartCouponView{}, nil)

			resp, err := s.GetCartsMeCoupons(authnContext(t), gen.GetCartsMeCouponsRequestObject{})
			require.NoError(t, err)

			actual, ok := resp.(gen.GetCartsMeCoupons200JSONResponse)
			require.True(t, ok)
			assert.Empty(t, actual.Coupons)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnが無い場合、認証エラーを返しユースケースを呼ばない", func(t *testing.T) {
			t.Parallel()

			s, _ := newServer(t)

			resp, err := s.GetCartsMeCoupons(context.Background(), gen.GetCartsMeCouponsRequestObject{})

			assert.Nil(t, resp)
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("ユースケースの失敗をそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().ListApplicableToMyCart(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrCanceled)

			resp, err := s.GetCartsMeCoupons(authnContext(t), gen.GetCartsMeCouponsRequestObject{})

			assert.Nil(t, resp)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_toCartCouponResponses(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("順序を保って全件を写す", func(t *testing.T) {
			t.Parallel()

			a, b := newCouponView(t, "resp_a"), newCouponView(t, "resp_b")

			got := toCartCouponResponses([]couponuc.CartCouponView{
				{Coupon: a, DiscountAmount: 100},
				{Coupon: b, DiscountAmount: 200},
			})

			require.Len(t, got, 2)
			assert.Equal(t, a.ID.ToPrimitive(), got[0].Coupon.Id)
			assert.Equal(t, int64(100), got[0].DiscountAmount)
			assert.Equal(t, b.ID.ToPrimitive(), got[1].Coupon.Id)
		})

		t.Run("空の場合は空を返す", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, toCartCouponResponses(nil))
		})
	})
}

func Test_toCouponResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きと適用範囲を2軸のまま写す", func(t *testing.T) {
			t.Parallel()

			view := newCouponView(t, "single")

			got := toCouponResponse(view)

			assert.Equal(t, view.ID.ToPrimitive(), got.Id)
			assert.Equal(t, gen.CouponDiscountKind("rate"), got.Discount.Kind)
			assert.Equal(t, "0.1", got.Discount.Value)
			assert.Equal(t, gen.CouponScopeKind("category"), got.Scope.Kind)
			require.NotNil(t, got.Scope.TargetId)
			assert.Equal(t, view.ScopeTargetID.ToPrimitive(), *got.Scope.TargetId)
			assert.Nil(t, got.UsedAt)
		})

		t.Run("使用日時があればそのまま載せる", func(t *testing.T) {
			t.Parallel()

			view := newCouponView(t, "used")
			usedAt := testIssuedAt.Add(time.Hour)
			view.UsedAt = &usedAt

			got := toCouponResponse(view)

			require.NotNil(t, got.UsedAt)
			assert.Equal(t, usedAt, *got.UsedAt)
		})
	})
}

func Test_toPrimitivePtr(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値がある場合は生成型のポインタへ写す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "primitive")

			got := toPrimitivePtr(&id)

			require.NotNil(t, got)
			assert.Equal(t, id.ToPrimitive(), *got)
		})

		t.Run("nilはnilのまま返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, toPrimitivePtr(nil))
		})
	})
}
