package purchases

import (
	"context"
	"math"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/purchases/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	checkoutuc "go-boilerplate/internal/usecase/checkout"
	mock_checkoutuc "go-boilerplate/internal/usecase/checkout/mock"
	"go-boilerplate/internal/usecase/idempotency"
	purchaseuc "go-boilerplate/internal/usecase/purchase"
	mock_purchaseuc "go-boilerplate/internal/usecase/purchase/mock"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"
	"go-boilerplate/pkg/xerrors"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestPurchaseView(t *testing.T) purchaseuc.PurchaseView {
	t.Helper()
	return purchaseuc.PurchaseView{
		Code:           "h-code",
		UserID:         uuidtestkit.NewTestFromSalt(t, "h_user"),
		StatusID:       uuidtestkit.NewTestFromSalt(t, "h_status"),
		SubtotalAmount: 160000,
		TaxAmount:      16000,
		ShippingFee:    500,
		TotalAmount:    176500,
		Details: []purchaseuc.PurchaseDetailView{
			{ProductID: uuidtestkit.NewTestFromSalt(t, "h_prod"), Quantity: 2, UnitPrice: decimaltestkit.MustParse(t, "800")},
		},
		OrderedAt: time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
	}
}

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

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_purchaseuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc, mock_checkoutuc.NewMockUsecase(gomock.NewController(t)), idempotency.Deps{})

	routes := e.Router().Routes()
	require.Len(t, routes, 2)
	// GET（一覧取得）と POST（作成）が同一パスに登録される。登録順は保証されないためメソッドで引く。
	paths := map[string]string{}
	for _, r := range routes {
		paths[r.Method] = r.Path
	}
	assert.Equal(t, "/v1/purchases", paths[http.MethodGet])
	assert.Equal(t, "/v1/purchases", paths[http.MethodPost])
}

func Test_server_PostPurchases(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証済みユーザーの購入を作成し201で返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			checkout := mock_checkoutuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), checkout: checkout, idem: idempotency.Deps{}}

			userID := uuidtestkit.NewTestFromSalt(t, "h_user")
			productID := uuidtestkit.NewTestFromSalt(t, "h_prod")
			view := checkoutuc.PurchaseView{Purchase: newTestPurchaseView(t)}
			checkout.EXPECT().CreatePurchase(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, params checkoutuc.CreatePurchaseParams) (checkoutuc.PurchaseView, error) {
					assert.Equal(t, userID, params.UserID)
					require.Len(t, params.Details, 1)
					assert.Equal(t, productID, params.Details[0].ProductID)
					assert.Equal(t, 2, params.Details[0].Quantity)
					assert.Nil(t, params.DisplayCurrency)
					return view, nil
				})

			resp, err := s.PostPurchases(authnContext(t, userID), gen.PostPurchasesRequestObject{
				Params: gen.PostPurchasesParams{},
				Body: &gen.PurchasesPostRequest{
					Details: []gen.PurchaseDetailInput{{ProductId: productID.ToPrimitive(), Quantity: 2}},
				},
			})
			require.NoError(t, err)

			r, ok := resp.(gen.PostPurchases201JSONResponse)
			require.True(t, ok)
			assert.Equal(t, view.Purchase.Code, r.Code)
			assert.Equal(t, view.Purchase.UserID.ToPrimitive(), r.UserId)
			assert.Equal(t, view.Purchase.StatusID.ToPrimitive(), r.StatusId)
			assert.Equal(t, view.Purchase.TotalAmount, int(r.TotalAmount))
			require.Len(t, r.Details, 1)
			assert.Equal(t, productID.ToPrimitive(), r.Details[0].ProductId)
		})

		t.Run("displayCurrency指定時はUsecaseへ伝達し参考換算額を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			checkout := mock_checkoutuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), checkout: checkout, idem: idempotency.Deps{}}

			userID := uuidtestkit.NewTestFromSalt(t, "h_user_dc")
			productID := uuidtestkit.NewTestFromSalt(t, "h_prod_dc")
			view := checkoutuc.PurchaseView{
				Purchase: newTestPurchaseView(t),
				ReferenceAmount: &checkoutuc.ReferenceAmountView{
					Currency: "JPY",
					Amount:   26475,
					Rate:     decimaltestkit.MustParse(t, "150.5"),
					RateDate: "2026-07-21",
				},
			}
			checkout.EXPECT().CreatePurchase(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, params checkoutuc.CreatePurchaseParams) (checkoutuc.PurchaseView, error) {
					require.NotNil(t, params.DisplayCurrency)
					assert.Equal(t, "JPY", *params.DisplayCurrency)
					return view, nil
				})

			jpy := gen.PostPurchasesParamsDisplayCurrency("JPY")
			resp, err := s.PostPurchases(authnContext(t, userID), gen.PostPurchasesRequestObject{
				Params: gen.PostPurchasesParams{DisplayCurrency: &jpy},
				Body: &gen.PurchasesPostRequest{
					Details: []gen.PurchaseDetailInput{{ProductId: productID.ToPrimitive(), Quantity: 2}},
				},
			})
			require.NoError(t, err)

			r, ok := resp.(gen.PostPurchases201JSONResponse)
			require.True(t, ok)
			require.NotNil(t, r.ReferenceAmount)
			assert.Equal(t, int64(26475), r.ReferenceAmount.Amount)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証情報が無い場合、ErrUnauthenticatedUserを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			checkout := mock_checkoutuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), checkout: checkout, idem: idempotency.Deps{}}

			_, err := s.PostPurchases(context.Background(), gen.PostPurchasesRequestObject{
				Body: &gen.PurchasesPostRequest{Details: []gen.PurchaseDetailInput{}},
			})
			require.ErrorIs(t, err, ctxhelper.ErrUnauthenticatedUser)
		})

		t.Run("内部UserIDが未解決の場合、エラーを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			// WithUserID を呼ばず内部 UserID を未解決のまま載せる。
			ctx := ctxhelper.WithAuthn(context.Background())
			authn, err := auth.New("subject", "issuer", nil, nil)
			require.NoError(t, err)
			require.True(t, ctxhelper.SetAuthn(ctx, *authn))

			_, err = s.PostPurchases(ctx, gen.PostPurchasesRequestObject{
				Body: &gen.PurchasesPostRequest{Details: []gen.PurchaseDetailInput{}},
			})
			require.ErrorIs(t, err, auth.ErrUserIDUnresolved)
		})

		t.Run("ユースケースがエラーを返した場合はそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			checkout := mock_checkoutuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), checkout: checkout, idem: idempotency.Deps{}}

			wantErr := xerrors.Wrap(apperror.ErrConflict, "insufficient stock")
			checkout.EXPECT().CreatePurchase(gomock.Any(), gomock.Any()).
				Return(checkoutuc.PurchaseView{}, wantErr)

			userID := uuidtestkit.NewTestFromSalt(t, "h_user_err")
			productID := uuidtestkit.NewTestFromSalt(t, "h_prod_err")
			_, err := s.PostPurchases(authnContext(t, userID), gen.PostPurchasesRequestObject{
				Body: &gen.PurchasesPostRequest{
					Details: []gen.PurchaseDetailInput{{ProductId: productID.ToPrimitive(), Quantity: 2}},
				},
			})
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("レスポンス変換が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			checkout := mock_checkoutuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), checkout: checkout, idem: idempotency.Deps{}}

			purchaseView := newTestPurchaseView(t)
			purchaseView.Details[0].Quantity = math.MaxInt32 + 1
			checkout.EXPECT().CreatePurchase(gomock.Any(), gomock.Any()).
				Return(checkoutuc.PurchaseView{Purchase: purchaseView}, nil)

			userID := uuidtestkit.NewTestFromSalt(t, "h_user_overflow")
			productID := uuidtestkit.NewTestFromSalt(t, "h_prod_overflow")
			_, err := s.PostPurchases(authnContext(t, userID), gen.PostPurchasesRequestObject{
				Body: &gen.PurchasesPostRequest{
					Details: []gen.PurchaseDetailInput{{ProductId: productID.ToPrimitive(), Quantity: 2}},
				},
			})
			require.ErrorIs(t, err, safecast.ErrOverflow)
		})
	})
}

func Test_toPurchaseResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("DTOをレスポンスへ全フィールド変換する", func(t *testing.T) {
			t.Parallel()

			view := newTestPurchaseView(t)
			actual, err := toPurchaseResponse(view, nil)
			require.NoError(t, err)
			assert.Equal(t, view.Code, actual.Code)
			assert.Equal(t, view.UserID.ToPrimitive(), actual.UserId)
			assert.Equal(t, view.StatusID.ToPrimitive(), actual.StatusId)
			assert.Equal(t, int64(160000), actual.SubtotalAmount)
			assert.Equal(t, int64(16000), actual.TaxAmount)
			assert.Equal(t, int64(500), actual.ShippingFee)
			assert.Equal(t, int64(176500), actual.TotalAmount)
			assert.Equal(t, view.OrderedAt, actual.OrderedAt)
			require.Len(t, actual.Details, 1)
			assert.Equal(t, view.Details[0].ProductID.ToPrimitive(), actual.Details[0].ProductId)
			assert.Equal(t, int32(2), actual.Details[0].Quantity)
			assert.Equal(t, "800", actual.Details[0].UnitPrice)
			assert.Nil(t, actual.ReferenceAmount)
		})

		t.Run("参考換算額がある場合はレスポンスに含める", func(t *testing.T) {
			t.Parallel()

			view := newTestPurchaseView(t)
			ref := &checkoutuc.ReferenceAmountView{
				Currency: "JPY",
				Amount:   26475,
				Rate:     decimaltestkit.MustParse(t, "150.5"),
				RateDate: "2026-07-21",
			}
			actual, err := toPurchaseResponse(view, ref)
			require.NoError(t, err)
			require.NotNil(t, actual.ReferenceAmount)
			assert.Equal(t, "JPY", actual.ReferenceAmount.Currency)
			assert.Equal(t, int64(26475), actual.ReferenceAmount.Amount)
			assert.Equal(t, "150.5", actual.ReferenceAmount.Rate)
			assert.Equal(t, "2026-07-21", actual.ReferenceAmount.RateDate)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("数量がint32範囲を超える場合はエラーを返す", func(t *testing.T) {
			t.Parallel()

			view := newTestPurchaseView(t)
			view.Details[0].Quantity = math.MaxInt32 + 1
			_, err := toPurchaseResponse(view, nil)
			require.ErrorIs(t, err, safecast.ErrOverflow)
		})
	})
}

func Test_toReferenceAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nilの場合はnilを返す", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, toReferenceAmount(nil))
		})

		t.Run("値がある場合はレスポンス型へ変換する", func(t *testing.T) {
			t.Parallel()
			actual := toReferenceAmount(
				&checkoutuc.ReferenceAmountView{Currency: "JPY", Amount: 15050, Rate: decimaltestkit.MustParse(t, "150.5"), RateDate: "2026-07-21"},
			)
			require.NotNil(t, actual)
			assert.Equal(t, int64(15050), actual.Amount)
			assert.Equal(t, "2026-07-21", actual.RateDate)
		})
	})
}

func Test_toPurchaseSummaryResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("概要DTOをステータス名込みのレスポンスへ写像する", func(t *testing.T) {
			t.Parallel()

			statusID := uuidtestkit.NewTestFromSalt(t, "summary_status")
			orderedAt := time.Date(2026, time.July, 24, 10, 30, 0, 0, time.UTC)
			actual := toPurchaseSummaryResponse(purchaseuc.PurchaseSummaryView{
				Code:          "summary-code",
				TotalAmount:   176500,
				StatusID:      statusID,
				StatusCode:    7, // 支払い済みの業務キー（値そのものは infra の統合テストがドメイン定数と突き合わせる）
				StatusName:    "支払い済み",
				FirstItemName: "ワイヤレスイヤホン",
				ItemCount:     3,
				OrderedAt:     orderedAt,
			})

			assert.Equal(t, "summary-code", actual.Code)
			assert.Equal(t, int64(176500), actual.TotalAmount)
			assert.Equal(t, statusID.ToPrimitive(), actual.Status.Id)
			assert.Equal(t, "支払い済み", actual.Status.Name)
			assert.EqualValues(t, 7, actual.Status.Code) // 支払い済みの業務キー
			assert.Equal(t, "ワイヤレスイヤホン", actual.FirstItemName)
			assert.EqualValues(t, 3, actual.ItemCount)
			assert.Equal(t, orderedAt, actual.OrderedAt)
		})
	})
}

func Test_toAppliedCouponResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きと適用範囲を2軸のまま写す", func(t *testing.T) {
			t.Parallel()

			target := uuidtestkit.NewTestFromSalt(t, "applied_category")
			view := &purchaseuc.AppliedCouponView{
				ID:            uuidtestkit.NewTestFromSalt(t, "applied_coupon"),
				DiscountKind:  "rate",
				DiscountValue: decimaltestkit.MustParse(t, "0.10"),
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

			view := &purchaseuc.AppliedCouponView{
				ID:            uuidtestkit.NewTestFromSalt(t, "applied_all"),
				DiscountKind:  "flat",
				DiscountValue: decimaltestkit.MustParse(t, "5"),
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
