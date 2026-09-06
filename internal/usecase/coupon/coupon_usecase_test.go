package coupon

import (
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/cart"
	mock_cart "go-boilerplate/internal/domain/cart/mock"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	mock_coupon "go-boilerplate/internal/domain/coupon/mock"
	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/internal/domain/product"
	mock_product "go-boilerplate/internal/domain/product/mock"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	mock_clock "go-boilerplate/internal/usecase/boundary/clock/mock"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	testNow       = time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)
	testExpiresAt = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	testIssuedAt  = time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
)

type testDeps struct {
	couponRepo  *mock_coupon.MockRepository
	cartRepo    *mock_cart.MockRepository
	productRepo *mock_product.MockRepository
	clock       *mock_clock.MockClock
}

func newTestUsecase(t *testing.T) (*usecase, *testDeps) {
	t.Helper()

	ctrl := gomock.NewController(t)
	deps := &testDeps{
		couponRepo:  mock_coupon.NewMockRepository(ctrl),
		cartRepo:    mock_cart.NewMockRepository(ctrl),
		productRepo: mock_product.NewMockRepository(ctrl),
		clock:       mock_clock.NewMockClock(ctrl),
	}
	u := &usecase{
		tracer:      observability.NewMockUsecaseLayerTracer(t),
		couponRepo:  deps.couponRepo,
		cartRepo:    deps.cartRepo,
		productRepo: deps.productRepo,
		clock:       deps.clock,
	}

	return u, deps
}

func newTestAuthn(t *testing.T) (*auth.Authn, uuid.UUID) {
	t.Helper()

	userID := uuidtestkit.NewTestFromSalt(t, "coupon_owner")
	a, err := auth.New("subject-1", auth.IssuerMock, nil, nil)
	require.NoError(t, err)
	resolved, err := a.WithUserID(userID)
	require.NoError(t, err)

	return resolved, userID
}

func newDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	require.NoError(t, err)

	return d
}

// newCoupon は、指定した値引きと適用範囲を持つ未使用のクーポンを組み立てます。
func newCoupon(t *testing.T, salt string, userID uuid.UUID, discount domaincoupon.Discount, scope domaincoupon.Scope) *domaincoupon.Coupon {
	t.Helper()

	c, err := domaincoupon.New(uuidtestkit.NewTestFromSalt(t, salt), domaincoupon.Attributes{
		UserID:    userID,
		Discount:  discount,
		Scope:     scope,
		ExpiresAt: testExpiresAt,
		IssuedAt:  testIssuedAt,
	})
	require.NoError(t, err)

	return c
}

func rateDiscount(t *testing.T, v string) domaincoupon.Discount {
	t.Helper()
	d, err := domaincoupon.NewRateDiscount(newDecimal(t, v))
	require.NoError(t, err)

	return d
}

func flatDiscount(t *testing.T, v string) domaincoupon.Discount {
	t.Helper()
	d, err := domaincoupon.NewFlatDiscount(newDecimal(t, v))
	require.NoError(t, err)

	return d
}

// newTestProduct は、公開済みで在庫のある商品を組み立てます。
func newTestProduct(t *testing.T, salt, price string, quantity int) *product.Product {
	t.Helper()

	status, err := product.NewStatusRef(uuidtestkit.NewTestFromSalt(t, "coupon_status"), "販売中")
	require.NoError(t, err)
	category, err := product.NewCategoryRef(uuidtestkit.NewTestFromSalt(t, "coupon_category"), "家具")
	require.NoError(t, err)
	amount, err := money.NewPrice(newDecimal(t, price))
	require.NoError(t, err)

	p, err := product.New(uuidtestkit.NewTestFromSalt(t, salt), product.Attributes{
		Name:        "テスト商品",
		Price:       amount,
		Quantity:    quantity,
		Status:      status,
		Category:    category,
		PublishedAt: ptr.To(testIssuedAt),
	}, testIssuedAt)
	require.NoError(t, err)

	return p
}

// newTestCart は、指定商品を数量 1 で 1 件持つカートを組み立てます。
func newTestCart(t *testing.T, ownerID uuid.UUID, productIDs ...uuid.UUID) *cart.Cart {
	t.Helper()

	return newTestCartExpiringAt(t, ownerID, testExpiresAt, productIDs...)
}

// newTestCartExpiringAt は、有効期限を指定してカートを組み立てます。
func newTestCartExpiringAt(
	t *testing.T, ownerID uuid.UUID, expiresAt time.Time, productIDs ...uuid.UUID,
) *cart.Cart {
	t.Helper()

	items := make([]cart.CartItem, len(productIDs))
	for i, id := range productIDs {
		items[i] = cart.NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_"+id.String()), cart.CartItemAttributes{
			ProductID: id,
			Quantity:  1,
			AddedAt:   testIssuedAt,
		})
	}

	c, err := cart.Reconstruct(uuidtestkit.NewTestFromSalt(t, "coupon_cart"), cart.Attributes{
		OwnerID:   &ownerID,
		Items:     items,
		ExpiresAt: expiresAt,
		CreatedAt: testIssuedAt,
		UpdatedAt: testIssuedAt,
	})
	require.NoError(t, err)

	return c
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("依存を注入したユースケースを生成する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			couponRepo := mock_coupon.NewMockRepository(ctrl)
			cartRepo := mock_cart.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			clk := mock_clock.NewMockClock(ctrl)

			u, ok := New(couponRepo, cartRepo, productRepo, clk, observability.NewNoopTracerFactory(t)).(*usecase)

			require.True(t, ok)
			assert.Equal(t, couponRepo, u.couponRepo)
			assert.Equal(t, cartRepo, u.cartRepo)
			assert.Equal(t, productRepo, u.productRepo)
			assert.Equal(t, clk, u.clock)
		})
	})
}

func Test_usecase_ListMyCoupons(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保有するクーポンを種別の名前つきで返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			c := newCoupon(t, "list_rate", userID, rateDiscount(t, "0.25"), domaincoupon.NewAllScope())
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)

			got, err := u.ListMyCoupons(t.Context(), authn)

			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, c.ID(), got[0].ID)
			assert.Equal(t, "rate", got[0].DiscountKind)
			assert.Equal(t, "all", got[0].ScopeKind)
			assert.Nil(t, got[0].ScopeTargetID)
			assert.Nil(t, got[0].UsedAt)
		})

		t.Run("使用済みのクーポンも並べる", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			c := newCoupon(t, "list_used", userID, flatDiscount(t, "5.00"), domaincoupon.NewAllScope())
			require.NoError(t, c.Redeem(testNow))
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)

			got, err := u.ListMyCoupons(t.Context(), authn)

			require.NoError(t, err)
			require.Len(t, got, 1)
			require.NotNil(t, got[0].UsedAt)
		})

		t.Run("1枚も持たない場合は空を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{}, nil)

			got, err := u.ListMyCoupons(t.Context(), authn)

			require.NoError(t, err)
			assert.Empty(t, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnが無い場合、認証エラーを返しリポジトリを呼ばない", func(t *testing.T) {
			t.Parallel()

			u, _ := newTestUsecase(t)

			_, err := u.ListMyCoupons(t.Context(), nil)

			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("リポジトリの失敗をそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(nil, apperror.ErrCanceled)

			_, err := u.ListMyCoupons(t.Context(), authn)

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_usecase_ListApplicableToMyCart(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("使えるクーポンを値引き額つきで返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			p := newTestProduct(t, "applicable_product", "100.00", 10)
			c := newCoupon(t, "applicable_rate", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())

			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID, p.ID()), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(product.Products{p}, nil)
			deps.clock.EXPECT().Now().Return(testNow)

			got, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, c.ID(), got[0].Coupon.ID)
			assert.Equal(t, 1000, got[0].DiscountAmount)
		})

		t.Run("使用済みのクーポンは並べない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			p := newTestProduct(t, "used_product", "100.00", 10)
			c := newCoupon(t, "used_coupon", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())
			require.NoError(t, c.Redeem(testNow))

			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID, p.ID()), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(product.Products{p}, nil)
			deps.clock.EXPECT().Now().Return(testNow)

			got, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("失効したクーポンは並べない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			p := newTestProduct(t, "expired_product", "100.00", 10)
			c := newCoupon(t, "expired_coupon", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())

			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID, p.ID()), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(product.Products{p}, nil)
			deps.clock.EXPECT().Now().Return(testExpiresAt)

			got, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("値引きが0になるクーポンは並べない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			p := newTestProduct(t, "zero_product", "100.00", 10)
			scope, err := domaincoupon.NewProductScope(uuidtestkit.NewTestFromSalt(t, "other_product"))
			require.NoError(t, err)
			c := newCoupon(t, "zero_coupon", userID, rateDiscount(t, "0.10"), scope)

			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID, p.ID()), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(product.Products{p}, nil)
			deps.clock.EXPECT().Now().Return(testNow)

			got, gerr := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, gerr)
			assert.Empty(t, got)
		})

		t.Run("在庫切れの明細は対象小計に入れない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			p := newTestProduct(t, "oos_product", "100.00", 0)
			c := newCoupon(t, "oos_coupon", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())

			deps.clock.EXPECT().Now().Return(testNow)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID, p.ID()), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(product.Products{p}, nil)

			got, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("1枚も保有しない場合はカートを引かずに空を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{}, nil)
			// カートも商品も引かないことを、EXPECT を置かないことで表す。

			got, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("カートを持たない場合は空を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			c := newCoupon(t, "nocart_coupon", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())
			deps.clock.EXPECT().Now().Return(testNow)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(nil, apperror.ErrNotFound)

			got, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("商品を引けない明細は対象小計に入れない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			c := newCoupon(t, "missing_coupon", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())
			missing := uuidtestkit.NewTestFromSalt(t, "missing_product")

			deps.clock.EXPECT().Now().Return(testNow)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID, missing), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(product.Products{}, nil)

			got, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.NoError(t, err)
			assert.Empty(t, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnが無い場合、認証エラーを返しリポジトリを呼ばない", func(t *testing.T) {
			t.Parallel()

			u, _ := newTestUsecase(t)

			_, err := u.ListApplicableToMyCart(t.Context(), nil)

			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("カートの取得に失敗した場合、商品を引かずエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			c := newCoupon(t, "carterr_coupon", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())
			deps.clock.EXPECT().Now().Return(testNow)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(nil, apperror.ErrCanceled)

			_, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})

		t.Run("値引き額の計算が失敗した場合、エラーを伝播する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			// Line.Subtotal はドメインでは検証しない観測値なので、決済スケールへ落とすと
			// int64 を超える対象小計が DiscountFor へ届き得る。
			a := newTestProduct(t, "overflow_a", "90000000000000000", 10)
			b := newTestProduct(t, "overflow_b", "90000000000000000", 10)
			c := newCoupon(t, "overflow_coupon", userID, rateDiscount(t, "1"), domaincoupon.NewAllScope())

			deps.clock.EXPECT().Now().Return(testNow)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().
				FindByOwnerID(gomock.Any(), userID).
				Return(newTestCart(t, userID, a.ID(), b.ID()), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(product.Products{a, b}, nil)

			_, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidDiscountValue)
		})

		t.Run("商品の取得に失敗した場合、エラーを伝播する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			c := newCoupon(t, "producterr_coupon", userID, rateDiscount(t, "0.10"), domaincoupon.NewAllScope())
			p := newTestProduct(t, "producterr_product", "100.00", 10)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.couponRepo.EXPECT().FindByUserID(gomock.Any(), userID).Return(domaincoupon.Coupons{c}, nil)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID, p.ID()), nil)
			deps.productRepo.EXPECT().FindByIDs(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrCanceled)

			_, err := u.ListApplicableToMyCart(t.Context(), authn)

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_usecase_buildLines(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("購入できる明細だけを商品カテゴリつきで写す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			_, userID := newTestAuthn(t)
			ok := newTestProduct(t, "lines_ok", "100.00", 10)
			oos := newTestProduct(t, "lines_oos", "50.00", 0)

			deps.cartRepo.EXPECT().
				FindByOwnerID(gomock.Any(), userID).
				Return(newTestCart(t, userID, ok.ID(), oos.ID()), nil)
			deps.productRepo.EXPECT().
				FindByIDs(gomock.Any(), gomock.Any()).
				Return(product.Products{ok, oos}, nil)

			lines, err := u.buildLines(t.Context(), userID, testNow)

			require.NoError(t, err)
			require.Len(t, lines, 1)
			assert.Equal(t, ok.ID(), lines[0].ProductID())
			assert.Equal(t, ok.Category().ID(), lines[0].CategoryID())
			assert.Equal(t, "100", lines[0].Subtotal().String())
		})

		t.Run("明細が空のカートは空を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			_, userID := newTestAuthn(t)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(newTestCart(t, userID), nil)

			lines, err := u.buildLines(t.Context(), userID, testNow)

			require.NoError(t, err)
			assert.Empty(t, lines)
		})

		t.Run("カートを持たない場合はエラーにせず空を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			_, userID := newTestAuthn(t)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(nil, apperror.ErrNotFound)

			lines, err := u.buildLines(t.Context(), userID, testNow)

			require.NoError(t, err)
			assert.Empty(t, lines)
		})

		t.Run("期限切れのカートは明細を持っていても空を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			_, userID := newTestAuthn(t)
			p := newTestProduct(t, "lines_expired", "100.00", 10)
			expired := newTestCartExpiringAt(t, userID, testNow.Add(-time.Second), p.ID())
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(expired, nil)

			lines, err := u.buildLines(t.Context(), userID, testNow)

			require.NoError(t, err)
			assert.Empty(t, lines)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("NotFound 以外のカート取得エラーはそのまま返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			_, userID := newTestAuthn(t)
			deps.cartRepo.EXPECT().FindByOwnerID(gomock.Any(), userID).Return(nil, apperror.ErrCanceled)

			_, err := u.buildLines(t.Context(), userID, testNow)

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_toCouponView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("種別を名前で、適用範囲の対象を識別子で写す", func(t *testing.T) {
			t.Parallel()

			_, userID := newTestAuthn(t)
			categoryID := uuidtestkit.NewTestFromSalt(t, "view_category")
			scope, err := domaincoupon.NewCategoryScope(categoryID)
			require.NoError(t, err)
			c := newCoupon(t, "view_coupon", userID, flatDiscount(t, "5.00"), scope)

			got := toCouponView(c)

			assert.Equal(t, c.ID(), got.ID)
			assert.Equal(t, "flat", got.DiscountKind)
			assert.Equal(t, "5", got.DiscountValue.String())
			assert.Equal(t, "category", got.ScopeKind)
			require.NotNil(t, got.ScopeTargetID)
			assert.Equal(t, categoryID, *got.ScopeTargetID)
			assert.Equal(t, testExpiresAt, got.ExpiresAt)
			assert.Equal(t, testIssuedAt, got.IssuedAt)
		})
	})
}

func Test_requireUserID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認証主体から内部ユーザーIDを取り出す", func(t *testing.T) {
			t.Parallel()

			authn, userID := newTestAuthn(t)

			got, err := requireUserID(authn)

			require.NoError(t, err)
			assert.Equal(t, userID, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nilの場合は認証エラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := requireUserID(nil)

			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("内部ユーザーIDが未解決の場合はErrUserIDUnresolvedを返す", func(t *testing.T) {
			t.Parallel()

			a, err := auth.New("subject-unresolved", auth.IssuerMock, nil, nil)
			require.NoError(t, err)

			_, uerr := requireUserID(a)

			require.ErrorIs(t, uerr, auth.ErrUserIDUnresolved)
		})
	})
}
