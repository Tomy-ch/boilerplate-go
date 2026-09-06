package purchases

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/purchases/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/idempotency"
	purchaseuc "go-boilerplate/internal/usecase/purchase"
	mock_purchaseuc "go-boilerplate/internal/usecase/purchase/mock"
	"go-boilerplate/internal/usecase/tools/timewindow"
	"go-boilerplate/pkg/safecast"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestSummaryView(t *testing.T) purchaseuc.PurchaseSummaryView {
	t.Helper()
	return purchaseuc.PurchaseSummaryView{
		Code:          "h-code",
		TotalAmount:   176500,
		StatusID:      uuidtestkit.NewTestFromSalt(t, "h_status"),
		StatusCode:    5,
		StatusName:    "完了",
		FirstItemName: "ワイヤレスイヤホン",
		ItemCount:     3,
		OrderedAt:     time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
	}
}

func Test_server_GetPurchases(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証済みユーザーの一覧をNextCursor付きHasNext=trueで返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			userID := uuidtestkit.NewTestFromSalt(t, "get_user")
			view := newTestSummaryView(t)
			nextCursor := "next-opaque-cursor"
			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, authn *auth.Authn, _ purchaseuc.ListPurchasesParams) (*purchaseuc.PurchaseListView, error) {
					require.NotNil(t, authn)
					uid, err := authn.UserID()
					require.NoError(t, err)
					assert.Equal(t, userID, uid)
					return &purchaseuc.PurchaseListView{Items: []purchaseuc.PurchaseSummaryView{view}, NextCursor: &nextCursor}, nil
				})

			resp, err := s.GetPurchases(authnContext(t, userID), gen.GetPurchasesRequestObject{Params: gen.GetPurchasesParams{}})
			require.NoError(t, err)

			actual, ok := resp.(gen.GetPurchases200JSONResponse)
			require.True(t, ok)
			assert.Equal(t, gen.PurchaseListResponse{
				Items: []gen.PurchaseSummaryResponse{{
					Code:        "h-code",
					TotalAmount: 176500,
					Status: gen.PurchaseStatusRef{
						Id:   view.StatusID.ToPrimitive(),
						Code: 5,
						Name: "完了",
					},
					FirstItemName: view.FirstItemName,
					ItemCount:     int64(view.ItemCount),
					OrderedAt:     view.OrderedAt,
				}},
				NextCursor: &nextCursor,
				HasNext:    true,
			}, gen.PurchaseListResponse(actual))
		})

		t.Run("末尾ページはNextCursorがnilでHasNextがfalseになる", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			userID := uuidtestkit.NewTestFromSalt(t, "get_user")
			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&purchaseuc.PurchaseListView{Items: []purchaseuc.PurchaseSummaryView{}, NextCursor: nil}, nil)

			resp, err := s.GetPurchases(authnContext(t, userID), gen.GetPurchasesRequestObject{Params: gen.GetPurchasesParams{}})
			require.NoError(t, err)

			actual, ok := resp.(gen.GetPurchases200JSONResponse)
			require.True(t, ok)
			assert.Empty(t, actual.Items)
			assert.Nil(t, actual.NextCursor)
			assert.False(t, actual.HasNext)
		})
		t.Run("orderedAfter・orderedBeforeを対象期間へ変換してユースケースへ渡す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			after := time.Date(2026, time.January, 21, 0, 0, 0, 0, time.UTC)
			before := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)

			var captured timewindow.Window
			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ *auth.Authn, params purchaseuc.ListPurchasesParams,
				) (*purchaseuc.PurchaseListView, error) {
					captured = params.Window
					return &purchaseuc.PurchaseListView{Items: []purchaseuc.PurchaseSummaryView{}}, nil
				})

			_, err := s.GetPurchases(
				authnContext(t, uuidtestkit.NewTestFromSalt(t, "get_window")),
				gen.GetPurchasesRequestObject{
					Params: gen.GetPurchasesParams{OrderedAfter: &after, OrderedBefore: &before},
				},
			)
			require.NoError(t, err)

			assert.Equal(t, after, *captured.After())
			assert.Equal(t, before, *captured.Before())
		})

		t.Run("statusCodesとincludeOtherUsersをユースケースへ渡す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			var captured purchaseuc.ListPurchasesParams
			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ *auth.Authn, params purchaseuc.ListPurchasesParams,
				) (*purchaseuc.PurchaseListView, error) {
					captured = params
					return &purchaseuc.PurchaseListView{Items: []purchaseuc.PurchaseSummaryView{}}, nil
				})

			statusCodes := []int32{7, 8}
			includeOtherUsers := true
			_, err := s.GetPurchases(
				authnContext(t, uuidtestkit.NewTestFromSalt(t, "get_admin")),
				gen.GetPurchasesRequestObject{
					Params: gen.GetPurchasesParams{StatusCodes: &statusCodes, IncludeOtherUsers: &includeOtherUsers},
				},
			)
			require.NoError(t, err)

			assert.Equal(t, []int16{7, 8}, captured.StatusCodes)
			assert.True(t, captured.IncludeOtherUsers)
		})

		t.Run("statusCodesとincludeOtherUsersの未指定は絞り込みなし・自分の購入のみになる", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			var captured purchaseuc.ListPurchasesParams
			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ *auth.Authn, params purchaseuc.ListPurchasesParams,
				) (*purchaseuc.PurchaseListView, error) {
					captured = params
					return &purchaseuc.PurchaseListView{Items: []purchaseuc.PurchaseSummaryView{}}, nil
				})

			_, err := s.GetPurchases(
				authnContext(t, uuidtestkit.NewTestFromSalt(t, "get_default")),
				gen.GetPurchasesRequestObject{Params: gen.GetPurchasesParams{}},
			)
			require.NoError(t, err)

			assert.Nil(t, captured.StatusCodes)
			assert.False(t, captured.IncludeOtherUsers)
		})

		t.Run("内部UserIDが未解決でもハンドラは認証主体をそのままユースケースへ渡す", func(t *testing.T) {
			t.Parallel()

			// 母集団の決定はユースケースが行い、他ユーザーを含める指定では認証主体の内部 UserID を
			// 必要としない。ハンドラが手前で弾かないことを固定する。
			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, authn *auth.Authn, _ purchaseuc.ListPurchasesParams,
				) (*purchaseuc.PurchaseListView, error) {
					require.NotNil(t, authn)
					assert.False(t, authn.HasUserID())
					return &purchaseuc.PurchaseListView{Items: []purchaseuc.PurchaseSummaryView{}}, nil
				})

			// WithUserID を呼ばず内部 UserID を未解決のまま Authn を載せる（JWT 検証済みだが DB ユーザー未解決の状態）。
			ctx := ctxhelper.WithAuthn(context.Background())
			authn, err := auth.New("subject", "issuer", nil, nil)
			require.NoError(t, err)
			require.True(t, ctxhelper.SetAuthn(ctx, *authn))

			_, err = s.GetPurchases(ctx, gen.GetPurchasesRequestObject{Params: gen.GetPurchasesParams{}})
			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("orderedBeforeがorderedAfter以前の場合、ユースケースを呼ばずErrInvalidArgumentを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}
			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			after := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
			before := time.Date(2026, time.January, 21, 0, 0, 0, 0, time.UTC)

			_, err := s.GetPurchases(
				authnContext(t, uuidtestkit.NewTestFromSalt(t, "get_badwindow")),
				gen.GetPurchasesRequestObject{
					Params: gen.GetPurchasesParams{OrderedAfter: &after, OrderedBefore: &before},
				},
			)
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})

		t.Run("未認証のときErrUnauthenticatedを返しUsecaseを呼ばない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			// 認証情報を仕込まない context のため GetAuthn が false を返す。
			resp, err := s.GetPurchases(context.Background(), gen.GetPurchasesRequestObject{Params: gen.GetPurchasesParams{}})
			assert.Nil(t, resp)
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("statusCodesがint16の範囲を超えるときErrOverflowを返しUsecaseを呼ばない", func(t *testing.T) {
			t.Parallel()

			// OpenAPI の maximum が int16 の上限と一致するため通常のリクエストでは到達しないが、
			// 契約側の上限が動いたときに黙って切り捨てへ倒れないことを固定する防御的な分岐である。
			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			statusCodes := []int32{40000}
			resp, err := s.GetPurchases(
				authnContext(t, uuidtestkit.NewTestFromSalt(t, "get_overflow")),
				gen.GetPurchasesRequestObject{Params: gen.GetPurchasesParams{StatusCodes: &statusCodes}},
			)
			assert.Nil(t, resp)
			require.ErrorIs(t, err, safecast.ErrOverflow)
		})

		t.Run("不正なcursorのときErrInvalidArgumentを返しUsecaseを呼ばない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			bad := "!!!"
			resp, err := s.GetPurchases(authnContext(t, uuidtestkit.NewTestFromSalt(t, "get_user")), gen.GetPurchasesRequestObject{
				Params: gen.GetPurchasesParams{After: &bad},
			})
			assert.Nil(t, resp)
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})

		t.Run("Usecaseのエラーが伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := mock_purchaseuc.NewMockUsecase(ctrl)
			s := &server{tracer: observability.NewMockControllerLayerTracer(t), uc: uc, idem: idempotency.Deps{}}

			userID := uuidtestkit.NewTestFromSalt(t, "get_user")
			uc.EXPECT().GetPurchases(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, apperror.ErrInternal)

			resp, err := s.GetPurchases(authnContext(t, userID), gen.GetPurchasesRequestObject{Params: gen.GetPurchasesParams{}})
			assert.Nil(t, resp)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})
	})
}
