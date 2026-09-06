package purchase

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	domainpurchase "go-boilerplate/internal/domain/purchase"
	"go-boilerplate/internal/observability"
	authbd "go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/purchase/query"
	mock_query "go-boilerplate/internal/usecase/purchase/query/mock"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_usecase_GetPurchaseDetail(t *testing.T) {
	t.Parallel()

	newAuthn := func(t *testing.T, userID uuid.UUID) *authbd.Authn {
		t.Helper()
		a, err := authbd.New("sub-"+userID.String(), authbd.IssuerMock, nil, nil)
		require.NoError(t, err)

		resolved, err := a.WithUserID(userID)
		require.NoError(t, err)
		return resolved
	}

	newUsecase := func(t *testing.T, qs query.PurchaseDetailQueryService) *usecase {
		t.Helper()
		return &usecase{
			tracer:   observability.NewNoopTracerFactory(t).Usecase(),
			detailQS: qs,
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証主体のuserIDをQSへ渡し読み取りモデルをビューへ写像する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userID := uuidtestkit.NewTestFromSalt(t, "gd_user")
			purchaseID := uuidtestkit.NewTestFromSalt(t, "gd_purchase")
			paidAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)

			rm := &query.PurchaseDetailReadModel{
				ID:             purchaseID,
				Code:           "gd-code",
				UserID:         userID,
				StatusID:       uuidtestkit.NewTestFromSalt(t, "gd_status"),
				StatusCode:     domainpurchase.StatusPaid.Code(),
				StatusName:     "支払い済み",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Items: []query.PurchaseDetailItem{
					{ProductID: uuidtestkit.NewTestFromSalt(t, "gd_product"), ProductName: "商品A", Quantity: 2, UnitPrice: mustPrice(t, "800")},
				},
				OrderedAt:  time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				PaidAt:     &paidAt,
				CanceledAt: nil,
			}

			var capturedUserID uuid.UUID
			var capturedPurchaseCode string
			qs := mock_query.NewMockPurchaseDetailQueryService(ctrl)
			qs.EXPECT().FindDetailByUserAndCode(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, uid uuid.UUID, code string) (*query.PurchaseDetailReadModel, error) {
					capturedUserID = uid
					capturedPurchaseCode = code
					return rm, nil
				},
			)

			u := newUsecase(t, qs)
			view, err := u.GetPurchaseDetail(context.Background(), newAuthn(t, userID), rm.Code)
			require.NoError(t, err)

			assert.Equal(t, userID, capturedUserID)
			assert.Equal(t, rm.Code, capturedPurchaseCode)
			assert.Equal(t, rm.Code, view.Code)
			assert.Equal(t, rm.StatusID, view.StatusID)
			assert.Equal(t, rm.StatusCode, view.StatusCode)
			assert.Equal(t, rm.StatusName, view.StatusName)
			assert.Equal(t, int64(16000), view.TaxAmount)
			assert.Equal(t, int64(500), view.ShippingFee)
			assert.Equal(t, int64(176500), view.TotalAmount)
			assert.Equal(t, rm.PaidAt, view.PaidAt)
			assert.Nil(t, view.CanceledAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, "商品A", view.Details[0].ProductName)
			assert.Equal(t, "800", view.Details[0].UnitPrice.String())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合はUnauthenticatedを返しQSは呼ばれない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			qs := mock_query.NewMockPurchaseDetailQueryService(ctrl)
			qs.EXPECT().FindDetailByUserAndCode(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			u := newUsecase(t, qs)
			_, err := u.GetPurchaseDetail(context.Background(), nil, "gd-nil")
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("authnはあるがUserIDが未解決の場合はErrUserIDUnresolvedを伝播しQSは呼ばれない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			qs := mock_query.NewMockPurchaseDetailQueryService(ctrl)
			qs.EXPECT().FindDetailByUserAndCode(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			// WithUserID を呼ばず内部 UserID を未解決のままにした authn を渡す。
			unresolved, err := authbd.New("sub-unresolved", authbd.IssuerMock, nil, nil)
			require.NoError(t, err)

			u := newUsecase(t, qs)
			_, err = u.GetPurchaseDetail(context.Background(), unresolved, "gd-unresolved")
			require.ErrorIs(t, err, authbd.ErrUserIDUnresolved)
		})

		t.Run("QSのNotFoundをそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userID := uuidtestkit.NewTestFromSalt(t, "gd_nf_user")
			qs := mock_query.NewMockPurchaseDetailQueryService(ctrl)
			qs.EXPECT().FindDetailByUserAndCode(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, xerrors.Wrap(apperror.ErrNotFound, "not found"))

			u := newUsecase(t, qs)
			_, err := u.GetPurchaseDetail(context.Background(), newAuthn(t, userID), "gd-nf")
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

func Test_toPurchaseGetDetailView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("読み取りモデルを単価decimal込みのビューへ写像する", func(t *testing.T) {
			t.Parallel()

			paidAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
			rm := &query.PurchaseDetailReadModel{
				ID:             uuidtestkit.NewTestFromSalt(t, "tv_id"),
				Code:           "tv-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "tv_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "tv_status"),
				StatusCode:     domainpurchase.StatusPaid.Code(),
				StatusName:     "支払い済み",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Items: []query.PurchaseDetailItem{
					{ProductID: uuidtestkit.NewTestFromSalt(t, "tv_prod"), ProductName: "商品A", Quantity: 2, UnitPrice: mustPrice(t, "800")},
				},
				OrderedAt:  time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				PaidAt:     &paidAt,
				CanceledAt: nil,
			}

			view := toPurchaseGetDetailView(rm)
			assert.Equal(t, rm.Code, view.Code)
			assert.Equal(t, rm.StatusCode, view.StatusCode)
			assert.Equal(t, rm.StatusName, view.StatusName)
			assert.Equal(t, int64(16000), view.TaxAmount)
			assert.Equal(t, int64(500), view.ShippingFee)
			assert.Equal(t, int64(176500), view.TotalAmount)
			assert.Equal(t, rm.PaidAt, view.PaidAt)
			assert.Nil(t, view.CanceledAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, "商品A", view.Details[0].ProductName)
			assert.Equal(t, "800", view.Details[0].UnitPrice.String())
		})

		t.Run("クーポン適用済み読み取りモデルは値引き額と適用クーポンを写像する", func(t *testing.T) {
			t.Parallel()

			value, verr := decimal.Parse("0.10")
			require.NoError(t, verr)
			couponID := uuidtestkit.NewTestFromSalt(t, "tvd_coupon")
			rm := &query.PurchaseDetailReadModel{
				ID:             uuidtestkit.NewTestFromSalt(t, "tvd_id"),
				Code:           "tvd-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "tvd_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "tvd_status"),
				StatusCode:     domainpurchase.StatusUnprocessed.Code(),
				StatusName:     "未処理",
				SubtotalAmount: 160000,
				DiscountAmount: 16000,
				AppliedCoupon: &query.AppliedCouponReadModel{
					ID:            couponID,
					DiscountKind:  domaincoupon.DiscountKindRate.Code(),
					DiscountValue: value,
					ScopeKind:     domaincoupon.ScopeKindAll.Code(),
				},
				TaxAmount:   14400,
				ShippingFee: 500,
				TotalAmount: 158900,
				Items: []query.PurchaseDetailItem{
					{ProductID: uuidtestkit.NewTestFromSalt(t, "tvd_prod"), ProductName: "商品D", Quantity: 2, UnitPrice: mustPrice(t, "800")},
				},
				OrderedAt: time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			}

			view := toPurchaseGetDetailView(rm)

			assert.Equal(t, int64(16000), view.DiscountAmount)
			require.NotNil(t, view.AppliedCoupon)
			assert.Equal(t, couponID, view.AppliedCoupon.ID)
			assert.Equal(t, "rate", view.AppliedCoupon.DiscountKind)
			assert.Equal(t, "all", view.AppliedCoupon.ScopeKind)
		})

		t.Run("キャンセル済み読み取りモデルはcanceledAtを写像しpaidAtはnilになる", func(t *testing.T) {
			t.Parallel()

			canceledAt := time.Date(2026, time.July, 26, 9, 0, 0, 0, time.UTC)
			rm := &query.PurchaseDetailReadModel{
				ID:             uuidtestkit.NewTestFromSalt(t, "tvc_id"),
				Code:           "tvc-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "tvc_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "tvc_status"),
				StatusCode:     domainpurchase.StatusCanceled.Code(),
				StatusName:     "キャンセル",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Items: []query.PurchaseDetailItem{
					{ProductID: uuidtestkit.NewTestFromSalt(t, "tvc_prod"), ProductName: "商品C", Quantity: 1, UnitPrice: mustPrice(t, "800")},
				},
				OrderedAt:  time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				PaidAt:     nil,
				CanceledAt: &canceledAt,
			}

			view := toPurchaseGetDetailView(rm)
			assert.Nil(t, view.PaidAt)
			require.NotNil(t, view.CanceledAt)
			assert.Equal(t, rm.CanceledAt, view.CanceledAt)
		})
	})
}

func Test_toAppliedCouponViewFromReadModel(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("業務キーを種別の名前へ解決する", func(t *testing.T) {
			t.Parallel()

			value, err := decimal.Parse("0.10")
			require.NoError(t, err)
			target := uuidtestkit.NewTestFromSalt(t, "rm_category")

			got := toAppliedCouponViewFromReadModel(&query.AppliedCouponReadModel{
				ID:            uuidtestkit.NewTestFromSalt(t, "rm_coupon"),
				DiscountKind:  domaincoupon.DiscountKindRate.Code(),
				DiscountValue: value,
				ScopeKind:     domaincoupon.ScopeKindCategory.Code(),
				ScopeTargetID: &target,
			})

			require.NotNil(t, got)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Equal(t, "category", got.ScopeKind)
			require.NotNil(t, got.ScopeTargetID)
			assert.Equal(t, target, *got.ScopeTargetID)
		})

		t.Run("未適用の場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, toAppliedCouponViewFromReadModel(nil))
		})

		t.Run("ドメインが知らないコードは名前を空のまま返す", func(t *testing.T) {
			t.Parallel()

			value, err := decimal.Parse("1")
			require.NoError(t, err)

			got := toAppliedCouponViewFromReadModel(&query.AppliedCouponReadModel{
				ID:            uuidtestkit.NewTestFromSalt(t, "rm_unknown"),
				DiscountKind:  99,
				DiscountValue: value,
				ScopeKind:     99,
			})

			require.NotNil(t, got)
			assert.Empty(t, got.DiscountKind)
			assert.Empty(t, got.ScopeKind)
		})
	})
}
