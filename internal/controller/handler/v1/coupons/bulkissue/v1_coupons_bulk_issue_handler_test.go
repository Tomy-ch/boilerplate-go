package couponsbulkissue

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/coupons/bulkissue/gen"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	couponuc "go-boilerplate/internal/usecase/coupon"
	mock_couponuc "go-boilerplate/internal/usecase/coupon/mock"
	"go-boilerplate/internal/usecase/idempotency"
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

	resolved, err := authn.WithUserID(uuidtestkit.NewTestFromSalt(t, "bulk_issue_admin"))
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

// newRequestBody は、定率 15% × 全体 の要求本文を組み立てます。
func newRequestBody() *gen.CouponBulkIssuePostRequest {
	return &gen.CouponBulkIssuePostRequest{
		Discount:  gen.CouponDiscountInput{Kind: gen.Rate, Value: "0.15"},
		Scope:     gen.CouponScopeInput{Kind: gen.All},
		ExpiresAt: testExpiresAt,
	}
}

func Test_server_PostCouponsBulkIssue(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ユースケースの結果を件数つきで返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ *auth.Authn, params couponuc.IssuePromotionalCouponsParams,
				) (couponuc.IssuePromotionalCouponsView, error) {
					assert.Equal(t, "rate", params.DiscountKind)
					assert.Equal(t, "0.15", params.DiscountValue.String())
					assert.Equal(t, "all", params.ScopeKind)
					assert.Nil(t, params.ScopeTargetID)
					assert.Equal(t, testExpiresAt, params.ExpiresAt)

					return couponuc.IssuePromotionalCouponsView{
						IssuedAt:          testIssuedAt,
						ExpiresAt:         testExpiresAt,
						RecipientCount:    1204,
						IssuedCouponCount: 1204,
					}, nil
				})

			got, err := s.PostCouponsBulkIssue(authnContext(t), gen.PostCouponsBulkIssueRequestObject{
				Body: newRequestBody(),
			})

			require.NoError(t, err)
			res, ok := got.(gen.PostCouponsBulkIssue200JSONResponse)
			require.True(t, ok)
			assert.Equal(t, testIssuedAt, res.IssuedAt)
			assert.Equal(t, testExpiresAt, res.ExpiresAt)
			assert.Equal(t, int64(1204), res.RecipientCount)
			assert.Equal(t, int64(1204), res.IssuedCouponCount)
		})

		t.Run("適用範囲の対象IDをユースケースへ渡す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			categoryID := uuidtestkit.NewTestFromSalt(t, "bulk_issue_handler_category")
			primitive := categoryID.ToPrimitive()

			body := newRequestBody()
			body.Scope = gen.CouponScopeInput{Kind: gen.Category, TargetId: &primitive}

			uc.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ *auth.Authn, params couponuc.IssuePromotionalCouponsParams,
				) (couponuc.IssuePromotionalCouponsView, error) {
					assert.Equal(t, "category", params.ScopeKind)
					require.NotNil(t, params.ScopeTargetID)
					assert.Equal(t, categoryID, *params.ScopeTargetID)

					return couponuc.IssuePromotionalCouponsView{}, nil
				})

			_, err := s.PostCouponsBulkIssue(authnContext(t), gen.PostCouponsBulkIssueRequestObject{Body: body})

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証コンテキストが無い場合、ユースケースを呼ばずエラーを返す", func(t *testing.T) {
			t.Parallel()

			s, _ := newServer(t)
			// ユースケースを呼ばないことを、EXPECT を置かないことで表す。

			_, err := s.PostCouponsBulkIssue(context.Background(), gen.PostCouponsBulkIssueRequestObject{
				Body: newRequestBody(),
			})

			require.Error(t, err)
		})

		t.Run("値引きの値が十進数として解釈できない場合、ユースケースを呼ばずエラーを返す", func(t *testing.T) {
			t.Parallel()

			s, _ := newServer(t)

			body := newRequestBody()
			body.Discount.Value = "not-a-number"
			// ユースケースを呼ばないことを、EXPECT を置かないことで表す。

			_, err := s.PostCouponsBulkIssue(authnContext(t), gen.PostCouponsBulkIssueRequestObject{Body: body})

			require.Error(t, err)
		})

		t.Run("ユースケースが失敗した場合、そのままエラーを返す", func(t *testing.T) {
			t.Parallel()

			s, uc := newServer(t)
			uc.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(couponuc.IssuePromotionalCouponsView{}, apperror.ErrConflict)

			_, err := s.PostCouponsBulkIssue(authnContext(t), gen.PostCouponsBulkIssueRequestObject{
				Body: newRequestBody(),
			})

			require.ErrorIs(t, err, apperror.ErrConflict)
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

func TestBindHandler(t *testing.T) {
	t.Parallel()

	e := echo.New()
	tf := observability.NewNoopTracerFactory(t)
	uc := mock_couponuc.NewMockUsecase(gomock.NewController(t))

	BindHandler(e, tf, uc, idempotency.Deps{})

	routes := e.Router().Routes()
	require.Len(t, routes, 1)
	assert.Equal(t, http.MethodPost, routes[0].Method)
	assert.Equal(t, "/v1/coupons/bulk-issue", routes[0].Path)
}

func Test_toIssuePromotionalCouponsParams(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("要求本文をユースケース入力へ写す", func(t *testing.T) {
			t.Parallel()

			got, err := toIssuePromotionalCouponsParams(newRequestBody())

			require.NoError(t, err)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Equal(t, "0.15", got.DiscountValue.String())
			assert.Equal(t, "all", got.ScopeKind)
			assert.Nil(t, got.ScopeTargetID)
			assert.Equal(t, testExpiresAt, got.ExpiresAt)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きの値が十進数として解釈できない場合、要求の不正として返す", func(t *testing.T) {
			t.Parallel()

			body := newRequestBody()
			body.Discount.Value = "not-a-number"

			_, err := toIssuePromotionalCouponsParams(body)

			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})
	})
}

func Test_toBulkIssueResponse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ユースケース出力を応答へ写す", func(t *testing.T) {
			t.Parallel()

			got := toBulkIssueResponse(couponuc.IssuePromotionalCouponsView{
				IssuedAt:          testIssuedAt,
				ExpiresAt:         testExpiresAt,
				RecipientCount:    7,
				IssuedCouponCount: 7,
			})

			assert.Equal(t, testIssuedAt, got.IssuedAt)
			assert.Equal(t, testExpiresAt, got.ExpiresAt)
			assert.Equal(t, int64(7), got.RecipientCount)
			assert.Equal(t, int64(7), got.IssuedCouponCount)
		})
	})
}
