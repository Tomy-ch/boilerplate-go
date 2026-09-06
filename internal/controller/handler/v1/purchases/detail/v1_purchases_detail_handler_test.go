package detail

import (
	"context"
	"math"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/purchases/detail/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	purchaseuc "go-boilerplate/internal/usecase/purchase"
	mock_purchaseuc "go-boilerplate/internal/usecase/purchase/mock"
	"go-boilerplate/pkg/decimal"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// authnContext は、内部ユーザー ID を解決済みの認証コンテキストを返すテストヘルパーです。
func authnContext(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()
	ctx := ctxhelper.WithAuthn(context.Background())
	authn, err := auth.New("subject", "issuer", nil, nil)
	require.NoError(t, err)

	resolved, err := authn.WithUserID(userID)
	require.NoError(t, err)
	require.True(t, ctxhelper.SetAuthn(ctx, *resolved))
	return ctx
}

// detailViewFixture は、購入詳細ビューを生成するテストヘルパーです。
func detailViewFixture(t *testing.T) purchaseuc.PurchaseGetDetailView {
	t.Helper()
	paidAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	return purchaseuc.PurchaseGetDetailView{
		Code:           "hd-code",
		UserID:         uuidtestkit.NewTestFromSalt(t, "hd_user"),
		StatusID:       uuidtestkit.NewTestFromSalt(t, "hd_status"),
		StatusCode:     7,
		StatusName:     "支払い済み",
		SubtotalAmount: 160000,
		TaxAmount:      16000,
		ShippingFee:    500,
		TotalAmount:    176500,
		Details: []purchaseuc.PurchaseDetailItemView{
			{ProductID: uuidtestkit.NewTestFromSalt(t, "hd_prod"), ProductName: "商品A", Quantity: 2, UnitPrice: decimaltestkit.MustParse(t, "800")},
		},
		OrderedAt:  time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
		PaidAt:     &paidAt,
		CanceledAt: nil,
	}
}

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_purchaseuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc)

	routes := e.Router().Routes()
	require.Len(t, routes, 1)
	assert.Equal(t, http.MethodGet, routes[0].Method)
	assert.Equal(t, "/v1/purchases/:purchaseCode", routes[0].Path)
}

func Test_server_GetPurchasesDetail(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証済みユーザーの購入詳細を200で返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc}

			userID := uuidtestkit.NewTestFromSalt(t, "hd_user")
			const purchaseCode = "hd-code"
			view := detailViewFixture(t)
			uc.EXPECT().GetPurchaseDetail(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, authn *auth.Authn, code string) (purchaseuc.PurchaseGetDetailView, error) {
					uid, uerr := authn.UserID()
					require.NoError(t, uerr)
					assert.Equal(t, userID, uid)
					assert.Equal(t, purchaseCode, code)
					return view, nil
				})

			resp, err := s.GetPurchasesDetail(authnContext(t, userID), gen.GetPurchasesDetailRequestObject{
				PurchaseCode: purchaseCode,
			})
			require.NoError(t, err)

			r, ok := resp.(gen.GetPurchasesDetail200JSONResponse)
			require.True(t, ok)
			assert.Equal(t, view.Code, r.Code)
			assert.Equal(t, "支払い済み", r.Status.Name)
			require.Len(t, r.Details, 1)
			assert.Equal(t, "商品A", r.Details[0].ProductName)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証情報が無い場合、ErrUnauthenticatedUserを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc}

			_, err := s.GetPurchasesDetail(context.Background(), gen.GetPurchasesDetailRequestObject{
				PurchaseCode: "hd-code-noauth",
			})
			require.ErrorIs(t, err, ctxhelper.ErrUnauthenticatedUser)
		})

		t.Run("ユースケースがエラーを返した場合はそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc}

			uc.EXPECT().GetPurchaseDetail(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(purchaseuc.PurchaseGetDetailView{}, apperror.ErrNotFound)

			userID := uuidtestkit.NewTestFromSalt(t, "hd_user_err")
			_, err := s.GetPurchasesDetail(authnContext(t, userID), gen.GetPurchasesDetailRequestObject{
				PurchaseCode: "hd-code-err",
			})
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("レスポンス変換が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc}

			view := detailViewFixture(t)
			view.Details[0].Quantity = math.MaxInt32 + 1
			uc.EXPECT().GetPurchaseDetail(gomock.Any(), gomock.Any(), gomock.Any()).Return(view, nil)

			userID := uuidtestkit.NewTestFromSalt(t, "hd_user_overflow")
			_, err := s.GetPurchasesDetail(authnContext(t, userID), gen.GetPurchasesDetailRequestObject{
				PurchaseCode: "hd-code-overflow",
			})
			require.ErrorIs(t, err, safecast.ErrOverflow)
		})
	})
}

func Test_toPurchaseGetDetailResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("詳細ビューを商品名込みのレスポンスへ写像する", func(t *testing.T) {
			t.Parallel()

			view := detailViewFixture(t)
			r, err := toPurchaseGetDetailResponse(view)
			require.NoError(t, err)
			assert.Equal(t, view.Code, r.Code)
			assert.Equal(t, view.UserID.ToPrimitive(), r.UserId)
			assert.Equal(t, view.OrderedAt, r.OrderedAt)
			assert.Equal(t, view.StatusID.ToPrimitive(), r.Status.Id)
			assert.Equal(t, "支払い済み", r.Status.Name)
			assert.EqualValues(t, view.StatusCode, r.Status.Code)
			assert.Equal(t, int64(16000), r.TaxAmount)
			assert.Equal(t, int64(500), r.ShippingFee)
			assert.Equal(t, int64(176500), r.TotalAmount)
			require.NotNil(t, r.PaidAt)
			assert.Equal(t, *view.PaidAt, *r.PaidAt)
			assert.Nil(t, r.CanceledAt)
			require.Len(t, r.Details, 1)
			assert.Equal(t, view.Details[0].ProductID.ToPrimitive(), r.Details[0].ProductId)
			assert.Equal(t, "商品A", r.Details[0].ProductName)
			assert.Equal(t, "800", r.Details[0].UnitPrice)
			assert.EqualValues(t, 2, r.Details[0].Quantity)
		})

		t.Run("クーポン適用済みビューは値引き額と適用クーポンをレスポンスへ写す", func(t *testing.T) {
			t.Parallel()

			couponID := uuidtestkit.NewTestFromSalt(t, "hd_coupon")
			view := detailViewFixture(t)
			view.DiscountAmount = 16000
			view.AppliedCoupon = &purchaseuc.AppliedCouponView{
				ID:            couponID,
				DiscountKind:  "rate",
				DiscountValue: decimaltestkit.MustParse(t, "0.10"),
				ScopeKind:     "all",
			}

			r, err := toPurchaseGetDetailResponse(view)

			require.NoError(t, err)
			assert.Equal(t, int64(16000), r.DiscountAmount)
			require.NotNil(t, r.AppliedCoupon)
			assert.Equal(t, couponID.ToPrimitive(), r.AppliedCoupon.Id)
			assert.Equal(t, gen.CouponDiscountKind("rate"), r.AppliedCoupon.Discount.Kind)
			assert.Equal(t, gen.CouponScopeKind("all"), r.AppliedCoupon.Scope.Kind)
		})

		t.Run("キャンセル済みビューはcanceledAtを写像しpaidAtはnilになる", func(t *testing.T) {
			t.Parallel()

			canceledAt := time.Date(2026, time.July, 26, 9, 0, 0, 0, time.UTC)
			view := detailViewFixture(t)
			view.StatusName = "キャンセル"
			view.PaidAt = nil
			view.CanceledAt = &canceledAt

			r, err := toPurchaseGetDetailResponse(view)
			require.NoError(t, err)
			assert.Nil(t, r.PaidAt)
			require.NotNil(t, r.CanceledAt)
			assert.Equal(t, canceledAt, *r.CanceledAt)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("数量がint32範囲を超える場合はエラーを返す", func(t *testing.T) {
			t.Parallel()

			view := detailViewFixture(t)
			view.Details[0].Quantity = math.MaxInt32 + 1
			_, err := toPurchaseGetDetailResponse(view)
			require.ErrorIs(t, err, safecast.ErrOverflow)
		})
	})
}

func Test_toAppliedCouponResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きと適用範囲を2軸のまま写す", func(t *testing.T) {
			t.Parallel()

			value, err := decimal.Parse("0.10")
			require.NoError(t, err)
			target := uuidtestkit.NewTestFromSalt(t, "applied_category")
			view := &purchaseuc.AppliedCouponView{
				ID:            uuidtestkit.NewTestFromSalt(t, "applied_coupon"),
				DiscountKind:  "rate",
				DiscountValue: value,
				ScopeKind:     "category",
				ScopeTargetID: &target,
			}

			got := toAppliedCouponResponse(view)

			require.NotNil(t, got)
			assert.Equal(t, view.ID.ToPrimitive(), got.Id)
			assert.Equal(t, gen.CouponDiscountKind("rate"), got.Discount.Kind)
			assert.Equal(t, "0.1", got.Discount.Value)
			assert.Equal(t, gen.CouponScopeKind("category"), got.Scope.Kind)
			require.NotNil(t, got.Scope.TargetId)
			assert.Equal(t, target.ToPrimitive(), *got.Scope.TargetId)
		})

		t.Run("全体の適用範囲は対象IDをnilで返す", func(t *testing.T) {
			t.Parallel()

			value, err := decimal.Parse("5")
			require.NoError(t, err)
			view := &purchaseuc.AppliedCouponView{
				ID:            uuidtestkit.NewTestFromSalt(t, "applied_all"),
				DiscountKind:  "flat",
				DiscountValue: value,
				ScopeKind:     "all",
			}

			got := toAppliedCouponResponse(view)

			require.NotNil(t, got)
			assert.Nil(t, got.Scope.TargetId)
		})

		t.Run("未適用の場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, toAppliedCouponResponse(nil))
		})
	})
}
