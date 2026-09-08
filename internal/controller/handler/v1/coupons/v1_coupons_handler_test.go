package coupons

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/coupons/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	couponuc "go-boilerplate/internal/usecase/coupon"
	mock_couponuc "go-boilerplate/internal/usecase/coupon/mock"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/decimal"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	testIssuedAt  = time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	testExpiresAt = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
)

// authnContext は、内部ユーザー ID を解決済みの認証コンテキストを返すテストヘルパーです。
func authnContext(t *testing.T) context.Context {
	t.Helper()
	ctx := ctxhelper.WithAuthn(context.Background())
	authn, err := auth.New("subject", "issuer", nil, nil)
	require.NoError(t, err)

	resolved, err := authn.WithUserID(uuidtestkit.NewTestFromSalt(t, "issue_admin"))
	require.NoError(t, err)
	require.True(t, ctxhelper.SetAuthn(ctx, *resolved))

	return ctx
}

func newServer(t *testing.T) (*server, *mock_couponuc.MockUsecase) {
	t.Helper()
	uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))

	return &server{
		tracer: observability.NewMockControllerLayerTracer(t),
		uc:     uc,
		idem:   idempotency.Deps{},
	}, uc
}

// newRequestBody は、定額 500 × 全体 の要求本文を組み立てます。
func newRequestBody(t *testing.T) *gen.CouponsPostRequest {
	t.Helper()

	return &gen.CouponsPostRequest{
		UserId:    uuidtestkit.NewTestFromSalt(t, "issue_recipient").ToPrimitive(),
		Discount:  gen.CouponDiscountInput{Kind: gen.CouponDiscountInputKindFlat, Value: "500"},
		Scope:     gen.CouponScopeInput{Kind: gen.CouponScopeInputKindAll},
		ExpiresAt: testExpiresAt,
	}
}

// newCouponView は、ユースケースが返す発行済みクーポンを組み立てます。
func newCouponView(t *testing.T) couponuc.CouponView {
	t.Helper()

	value, err := decimal.Parse("500")
	require.NoError(t, err)

	return couponuc.CouponView{
		ID:            uuidtestkit.NewTestFromSalt(t, "issued_coupon"),
		DiscountKind:  "flat",
		DiscountValue: value,
		ScopeKind:     "all",
		ExpiresAt:     testExpiresAt,
		IssuedAt:      testIssuedAt,
	}
}

func Test_server_PostCoupons(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("発行したクーポンを 201 で返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			recipientID := uuidtestkit.NewTestFromSalt(t, "issue_recipient")
			uc.EXPECT().
				IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ *auth.Authn, params couponuc.IssueCouponParams,
				) (couponuc.CouponView, error) {
					assert.Equal(t, recipientID, params.UserID)
					assert.Equal(t, "flat", params.DiscountKind)
					assert.Equal(t, "500", params.DiscountValue.String())
					assert.Equal(t, "all", params.ScopeKind)
					assert.Nil(t, params.ScopeTargetID)
					assert.Equal(t, testExpiresAt, params.ExpiresAt)

					return newCouponView(t), nil
				})

			got, err := s.PostCoupons(authnContext(t), gen.PostCouponsRequestObject{Body: newRequestBody(t)})

			require.NoError(t, err)
			res, ok := got.(gen.PostCoupons201JSONResponse)
			require.True(t, ok)
			assert.Equal(t, gen.CouponDiscountKind("flat"), res.Discount.Kind)
			assert.Equal(t, "500", res.Discount.Value)
			assert.Equal(t, gen.CouponScopeKind("all"), res.Scope.Kind)
			assert.Nil(t, res.Scope.TargetId)
			assert.Nil(t, res.UsedAt)
			assert.Equal(t, testIssuedAt, res.IssuedAt)
		})

		t.Run("適用範囲の対象IDをユースケースへ渡す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			productID := uuidtestkit.NewTestFromSalt(t, "issue_handler_product")
			primitive := productID.ToPrimitive()

			body := newRequestBody(t)
			body.Scope = gen.CouponScopeInput{Kind: gen.CouponScopeInputKindProduct, TargetId: &primitive}

			uc.EXPECT().
				IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ *auth.Authn, params couponuc.IssueCouponParams,
				) (couponuc.CouponView, error) {
					assert.Equal(t, "product", params.ScopeKind)
					require.NotNil(t, params.ScopeTargetID)
					assert.Equal(t, productID, *params.ScopeTargetID)

					return newCouponView(t), nil
				})

			_, err := s.PostCoupons(authnContext(t), gen.PostCouponsRequestObject{Body: body})

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証コンテキストが無い場合、ユースケースを呼ばずエラーを返す", func(t *testing.T) {
			t.Parallel()

			s, _ := newServer(t)
			// ユースケースを呼ばないことを、EXPECT を置かないことで表す。

			_, err := s.PostCoupons(context.Background(), gen.PostCouponsRequestObject{Body: newRequestBody(t)})

			require.ErrorIs(t, err, ctxhelper.ErrUnauthenticatedUser)
		})

		t.Run("値引きの値が十進数として解釈できない場合、ユースケースを呼ばずエラーを返す", func(t *testing.T) {
			t.Parallel()

			s, _ := newServer(t)

			body := newRequestBody(t)
			body.Discount.Value = "not-a-number"
			// ユースケースを呼ばないことを、EXPECT を置かないことで表す。

			_, err := s.PostCoupons(authnContext(t), gen.PostCouponsRequestObject{Body: body})

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})

		t.Run("ユースケースが失敗した場合、そのままエラーを返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().
				IssueCoupon(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.CouponView{}, apperror.ErrNotFound)

			_, err := s.PostCoupons(authnContext(t), gen.PostCouponsRequestObject{Body: newRequestBody(t)})

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

func Test_toIssueCouponParams(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("要求本文をユースケース入力へ写す", func(t *testing.T) {
			t.Parallel()

			got, err := toIssueCouponParams(newRequestBody(t))

			require.NoError(t, err)
			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "issue_recipient"), got.UserID)
			assert.Equal(t, "flat", got.DiscountKind)
			assert.Equal(t, "500", got.DiscountValue.String())
			assert.Equal(t, "all", got.ScopeKind)
			assert.Nil(t, got.ScopeTargetID)
			assert.Equal(t, testExpiresAt, got.ExpiresAt)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きの値が十進数として解釈できない場合、要求の不正として返す", func(t *testing.T) {
			t.Parallel()

			body := newRequestBody(t)
			body.Discount.Value = "not-a-number"

			_, err := toIssueCouponParams(body)

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})
	})
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

func Test_toCouponResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("適用範囲の対象を持たないクーポンを応答へ写す", func(t *testing.T) {
			t.Parallel()

			view := newCouponView(t)

			got := toCouponResponse(view)

			assert.Equal(t, view.ID.ToPrimitive(), got.Id)
			assert.Equal(t, gen.CouponDiscountKind("flat"), got.Discount.Kind)
			assert.Equal(t, "500", got.Discount.Value)
			assert.Equal(t, gen.CouponScopeKind("all"), got.Scope.Kind)
			assert.Nil(t, got.Scope.TargetId)
			assert.Nil(t, got.UsedAt)
			assert.Equal(t, testExpiresAt, got.ExpiresAt)
			assert.Equal(t, testIssuedAt, got.IssuedAt)
		})

		t.Run("適用範囲の対象を持つクーポンは対象IDも写す", func(t *testing.T) {
			t.Parallel()

			targetID := uuidtestkit.NewTestFromSalt(t, "response_target")
			view := newCouponView(t)
			view.ScopeKind = "product"
			view.ScopeTargetID = &targetID

			got := toCouponResponse(view)

			assert.Equal(t, gen.CouponScopeKind("product"), got.Scope.Kind)
			require.NotNil(t, got.Scope.TargetId)
			assert.Equal(t, targetID.ToPrimitive(), *got.Scope.TargetId)
		})
	})
}

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc, idempotency.Deps{})

	routes := e.Router().Routes()
	require.Len(t, routes, 1)
	assert.Equal(t, http.MethodPost, routes[0].Method)
	assert.Equal(t, "/v1/coupons", routes[0].Path)
}
