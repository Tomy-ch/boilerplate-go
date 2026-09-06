package purchase

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	mock_coupon "go-boilerplate/internal/domain/coupon/mock"
	"go-boilerplate/internal/domain/lexicon/money"
	domainproduct "go-boilerplate/internal/domain/product"
	mock_product "go-boilerplate/internal/domain/product/mock"
	domainpurchase "go-boilerplate/internal/domain/purchase"
	mock_purchase "go-boilerplate/internal/domain/purchase/mock"
	"go-boilerplate/internal/domain/service/membership"
	domainuser "go-boilerplate/internal/domain/user"
	mock_user "go-boilerplate/internal/domain/user/mock"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	mock_authz "go-boilerplate/internal/usecase/boundary/authz/mock"
	clocktestkit "go-boilerplate/internal/usecase/boundary/clock/testkit"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	"go-boilerplate/internal/usecase/outbox"
	mock_outbox "go-boilerplate/internal/usecase/outbox/mock"
	"go-boilerplate/internal/usecase/purchase/event"
	mock_query "go-boilerplate/internal/usecase/purchase/query/mock"
	"go-boilerplate/internal/usecase/testkit"
	"go-boilerplate/pkg/decimal"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// testCreatedAt は、テスト用の商品の登録日時です。公開日時と取り違えを検出できるよう異なる値にします。
var testCreatedAt = time.Date(2025, time.December, 31, 23, 59, 58, 0, time.UTC)

// purchaserBaseTime は、購入者ユーザーの作成・更新日時の基準です。
var purchaserBaseTime = time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)

// mustPrice は、テスト用に十進文字列（ドル）から非負の money.Price を構築します。
//

func mustPrice(t *testing.T, s string) money.Price {
	t.Helper()
	p, err := money.NewPrice(decimaltestkit.MustParse(t, s))
	require.NoError(t, err)
	return p
}

// activePurchaser は、在籍している購入者のユーザーエンティティを生成するテストヘルパーです。
func activePurchaser(t *testing.T, id uuid.UUID) *domainuser.User {
	t.Helper()
	u, err := domainuser.New(id, domainuser.Attributes{
		FirstName:    "John",
		LastName:     "Doe",
		Email:        "john.doe@example.com",
		Phone:        "1234567890",
		PrefectureID: uuidtestkit.NewTestFromSalt(t, "purchaser_prefecture"),
		City:         "Shibuya",
		Street:       "1-2-3",
		PostalCode:   "150-0001",
		CreatedAt:    purchaserBaseTime,
		UpdatedAt:    purchaserBaseTime,
	})
	require.NoError(t, err)
	return u
}

// withdrawnPurchaser は、退会済みの購入者のユーザーエンティティを生成するテストヘルパーです。
func withdrawnPurchaser(t *testing.T, id uuid.UUID) *domainuser.User {
	t.Helper()
	u := activePurchaser(t, id)
	require.NoError(t, u.MarkAsDeleted(purchaserBaseTime.Add(time.Hour)))
	return u
}

// lockedProducts は、productRepo.LockByIDs が返す悲観ロック済みの商品群を生成するテストヘルパーです。
func lockedProducts(t *testing.T, id uuid.UUID, quantity int) domainproduct.Products {
	t.Helper()
	status, err := domainproduct.NewStatusRef(uuidtestkit.NewTestFromSalt(t, "locked_status"), "在庫あり")
	require.NoError(t, err)
	category, err := domainproduct.NewCategoryRef(uuidtestkit.NewTestFromSalt(t, "locked_category"), "電子機器")
	require.NoError(t, err)
	p, err := domainproduct.New(id, domainproduct.Attributes{
		Name:     "ロック対象商品",
		Price:    mustPrice(t, "800"),
		Quantity: quantity,
		Status:   status,
		Category: category,
	}, testCreatedAt)
	require.NoError(t, err)
	return domainproduct.Products{p}
}

// rereadPurchase は、repo.FindByID が返す再構築済みの購入を生成するテストヘルパーです。
func rereadPurchase(t *testing.T) *domainpurchase.Purchase {
	t.Helper()
	details := []domainpurchase.PurchaseDetail{
		domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "reread_detail"), domainpurchase.PurchaseDetailAttributes{
			ProductID: uuidtestkit.NewTestFromSalt(t, "reread_product"),
			Quantity:  2,
			UnitPrice: mustPrice(t, "800"),
		}),
	}
	p, err := domainpurchase.Reconstruct(uuidtestkit.NewTestFromSalt(t, "reread_id"), domainpurchase.Attributes{
		Code:           "reread-code",
		UserID:         uuidtestkit.NewTestFromSalt(t, "reread_user"),
		StatusID:       uuidtestkit.NewTestFromSalt(t, "reread_status"),
		StatusCode:     domainpurchase.StatusUnprocessed.Code(),
		SubtotalAmount: 160000,
		TaxAmount:      16000,
		ShippingFee:    500,
		TotalAmount:    176500,
		Details:        details,
		OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
		PaidAt:         nil,
		CanceledAt:     nil,
		ShippedAt:      nil,
		DeliveredAt:    nil,
	})
	require.NoError(t, err)
	return p
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("依存を注入したUsecaseを生成する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			tf := observability.NewNoopTracerFactory(t)
			txm := testkit.NewMockTransactionManager(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			couponRepo := mock_coupon.NewMockRepository(ctrl)
			userLock := mock_user.NewMockLockRepository(ctrl)
			detailQS := mock_query.NewMockPurchaseDetailQueryService(ctrl)
			feedQS := mock_query.NewMockPurchaseFeedQueryService(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			clk := clocktestkit.NewMockClock(t, time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC))
			expected := &usecase{
				tracer:      tf.Usecase(),
				txm:         txm,
				repo:        repo,
				productRepo: productRepo,
				couponRepo:  couponRepo,
				userLock:    userLock,
				detailQS:    detailQS,
				feedQS:      feedQS,
				emit:        emit,
				clock:       clk,
				authorizer:  authorizer,
			}
			actual := New(txm, repo, productRepo, couponRepo, userLock, detailQS, feedQS, emit, clk, authorizer, tf)
			assert.Equal(t, expected, actual)
		})
	})
}

func Test_usecase_CreatePurchase(t *testing.T) {
	t.Parallel()

	productA := uuidtestkit.NewTestFromSalt(t, "cp_product")
	// activeUserLock は、購入者が在籍している（在籍ガードを通過する）ユーザー LockRepository モックを返します。
	activeUserLock := func(ctrl *gomock.Controller) *mock_user.MockLockRepository {
		r := mock_user.NewMockLockRepository(ctrl)
		r.EXPECT().LockShareByID(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, id uuid.UUID) (*domainuser.User, error) {
				return activePurchaser(t, id), nil
			},
		)
		return r
	}
	// newUsecase は、指定 mock を注入した usecase を生成するローカルヘルパーです。
	newUsecase := func(
		t *testing.T,
		userLock *mock_user.MockLockRepository,
		repo *mock_purchase.MockRepository,
		productRepo *mock_product.MockRepository,
		emit *mock_outbox.MockEmitUsecase,
	) *usecase {
		t.Helper()
		return &usecase{
			tracer: observability.NewNoopTracerFactory(t).Usecase(),
			txm:    testkit.NewMockTransactionManager(t),
			repo:   repo, productRepo: productRepo, userLock: userLock, emit: emit,
			clock: clocktestkit.NewMockClock(t, time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)),
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロック→生成→書き込み→emit→再検証を経て購入ビューを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(lockedProducts(t, productA, 20), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			reread := rereadPurchase(t)
			repo.EXPECT().FindByID(gomock.Any(), gomock.Any()).Return(reread, nil)

			u := newUsecase(t, activeUserLock(ctrl), repo, productRepo, emit)

			view, err := u.CreatePurchase(context.Background(), CreatePurchaseParams{
				UserID:  uuidtestkit.NewTestFromSalt(t, "cp_user"),
				Details: []DetailParam{{ProductID: productA, Quantity: 2}},
			})
			require.NoError(t, err)
			// 再検証で読み直したエンティティが DTO へ写像される。
			assert.Equal(t, reread.Code(), view.Code)
			assert.Equal(t, reread.UserID(), view.UserID)
			assert.Equal(t, reread.StatusID(), view.StatusID)
			assert.Equal(t, reread.SubtotalAmount(), view.SubtotalAmount)
			assert.Equal(t, reread.TaxAmount(), view.TaxAmount)
			assert.Equal(t, reread.ShippingFee(), view.ShippingFee)
			assert.Equal(t, reread.TotalAmount(), view.TotalAmount)
			assert.Equal(t, reread.OrderedAt(), view.OrderedAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, reread.Details()[0].ProductID(), view.Details[0].ProductID)
			assert.True(t, reread.Details()[0].UnitPrice().Decimal().Equal(view.Details[0].UnitPrice))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		validParams := CreatePurchaseParams{
			UserID:  uuidtestkit.NewTestFromSalt(t, "cp_user_err"),
			Details: []DetailParam{{ProductID: productA, Quantity: 2}},
		}

		t.Run("在籍ガードに弾かれた場合は在庫ロック以降を行わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userLock := mock_user.NewMockLockRepository(ctrl)
			userLock.EXPECT().LockShareByID(gomock.Any(), validParams.UserID).
				Return(withdrawnPurchaser(t, validParams.UserID), nil)

			// 在庫ロック・書き込み・emit・再検証に EXPECT を張らないことで未呼び出しを担保する。
			u := newUsecase(
				t,
				userLock,
				mock_purchase.NewMockRepository(ctrl),
				mock_product.NewMockRepository(ctrl),
				mock_outbox.NewMockEmitUsecase(ctrl),
			)

			_, err := u.CreatePurchase(context.Background(), validParams)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("在庫不足の場合、ErrInsufficientStockを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			// 在庫 1 に対し 2 を要求 → domain New が ErrInsufficientStock
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(lockedProducts(t, productA, 1), nil)

			u := newUsecase(
				t,
				activeUserLock(ctrl),
				mock_purchase.NewMockRepository(ctrl),
				productRepo,
				mock_outbox.NewMockEmitUsecase(ctrl),
			)

			_, err := u.CreatePurchase(context.Background(), validParams)
			require.ErrorIs(t, err, domainpurchase.ErrInsufficientStock)
		})

		t.Run("商品の悲観ロックが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrUnavailable)

			u := newUsecase(
				t,
				activeUserLock(ctrl),
				mock_purchase.NewMockRepository(ctrl),
				productRepo,
				mock_outbox.NewMockEmitUsecase(ctrl),
			)

			_, err := u.CreatePurchase(context.Background(), validParams)
			require.ErrorIs(t, err, apperror.ErrUnavailable)
		})

		t.Run("CreatePurchase書き込みが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(lockedProducts(t, productA, 20), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(apperror.ErrConflict)

			u := newUsecase(
				t,
				activeUserLock(ctrl),
				repo,
				productRepo,
				mock_outbox.NewMockEmitUsecase(ctrl),
			)

			_, err := u.CreatePurchase(context.Background(), validParams)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("outbox発行が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(lockedProducts(t, productA, 20), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			// 注文日時は DB 採番なので、emit は再検証の読み直しのあとに走る。
			repo.EXPECT().FindByID(gomock.Any(), gomock.Any()).Return(rereadPurchase(t), nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, apperror.ErrInternal)

			u := newUsecase(t, activeUserLock(ctrl), repo, productRepo, emit)

			_, err := u.CreatePurchase(context.Background(), validParams)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("再検証の読み直しが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(lockedProducts(t, productA, 20), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().FindByID(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrNotFound)
			// emit に EXPECT を張らないことで、読み直しに失敗したら発行へ進まないことを担保する。

			u := newUsecase(t, activeUserLock(ctrl), repo, productRepo, emit)

			_, err := u.CreatePurchase(context.Background(), validParams)
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

func Test_newPurchaseDraft(t *testing.T) {
	t.Parallel()

	productA := uuidtestkit.NewTestFromSalt(t, "draft_product_a")
	productB := uuidtestkit.NewTestFromSalt(t, "draft_product_b")

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細と同じ並びでドメイン入力と商品ID列を組み立てる", func(t *testing.T) {
			t.Parallel()

			draft, err := newPurchaseDraft([]DetailParam{
				{ProductID: productA, Quantity: 2},
				{ProductID: productB, Quantity: 3},
			})
			require.NoError(t, err)

			require.Len(t, draft.inputs, 2)
			require.Len(t, draft.productIDs, 2)
			assert.Equal(t, []uuid.UUID{productA, productB}, draft.productIDs)
			assert.Equal(t, productA, draft.inputs[0].ProductID)
			assert.Equal(t, 2, draft.inputs[0].Quantity)
			assert.Equal(t, productB, draft.inputs[1].ProductID)
			assert.Equal(t, 3, draft.inputs[1].Quantity)
		})

		t.Run("購入ID_購入コード_各明細IDはいずれも異なる値が採番される", func(t *testing.T) {
			t.Parallel()

			draft, err := newPurchaseDraft([]DetailParam{
				{ProductID: productA, Quantity: 1},
				{ProductID: productB, Quantity: 1},
			})
			require.NoError(t, err)

			assert.False(t, draft.purchaseID.IsNil())
			assert.NotEqual(t, draft.purchaseID.String(), draft.code)
			assert.NotEqual(t, draft.inputs[0].ID, draft.inputs[1].ID)
			assert.NotEqual(t, draft.purchaseID, draft.inputs[0].ID)
		})

		t.Run("明細が空でも空のスライスを返す", func(t *testing.T) {
			t.Parallel()

			draft, err := newPurchaseDraft(nil)
			require.NoError(t, err)
			assert.Empty(t, draft.inputs)
			assert.Empty(t, draft.productIDs)
		})
	})
}

func Test_usecase_ensurePurchaserActive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuidtestkit.NewTestFromSalt(t, "ensure_active_user")

	newGuard := func(t *testing.T, userLock *mock_user.MockLockRepository) *usecase {
		t.Helper()
		return &usecase{tracer: observability.NewNoopTracerFactory(t).Usecase(), userLock: userLock}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("在籍しているユーザーは通過する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userLock := mock_user.NewMockLockRepository(ctrl)
			userLock.EXPECT().LockShareByID(gomock.Any(), userID).Return(activePurchaser(t, userID), nil)

			require.NoError(t, newGuard(t, userLock).ensurePurchaserActive(ctx, userID))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("退会済みの購入者はErrConflictで拒否される", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userLock := mock_user.NewMockLockRepository(ctrl)
			userLock.EXPECT().LockShareByID(gomock.Any(), userID).
				Return(withdrawnPurchaser(t, userID), nil)

			err := newGuard(t, userLock).ensurePurchaserActive(ctx, userID)
			require.ErrorIs(t, err, membership.ErrPurchaserWithdrawn)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("購入者が存在しない場合のNotFoundはErrConflictへ写像されNotFoundへは畳まれない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userLock := mock_user.NewMockLockRepository(ctrl)
			userLock.EXPECT().LockShareByID(gomock.Any(), userID).Return(nil, apperror.ErrNotFound)

			err := newGuard(t, userLock).ensurePurchaserActive(ctx, userID)
			require.ErrorIs(t, err, apperror.ErrConflict)
			require.NotErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("NotFound以外はErrConflictへ化けずそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userLock := mock_user.NewMockLockRepository(ctrl)
			userLock.EXPECT().LockShareByID(gomock.Any(), userID).Return(nil, apperror.ErrUnavailable)

			err := newGuard(t, userLock).ensurePurchaserActive(ctx, userID)
			require.ErrorIs(t, err, apperror.ErrUnavailable)
			require.NotErrorIs(t, err, apperror.ErrConflict)
		})
	})
}

func Test_toPurchaseView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("購入集約を出力DTOへ変換する", func(t *testing.T) {
			t.Parallel()

			entity := rereadPurchase(t)
			view := toPurchaseView(entity, nil)
			assert.Equal(t, entity.Code(), view.Code)
			assert.Equal(t, entity.UserID(), view.UserID)
			assert.Equal(t, entity.StatusID(), view.StatusID)
			assert.Equal(t, entity.SubtotalAmount(), view.SubtotalAmount)
			assert.Equal(t, entity.TaxAmount(), view.TaxAmount)
			assert.Equal(t, entity.ShippingFee(), view.ShippingFee)
			assert.Equal(t, entity.TotalAmount(), view.TotalAmount)
			assert.Equal(t, entity.OrderedAt(), view.OrderedAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, entity.Details()[0].ProductID(), view.Details[0].ProductID)
			assert.Equal(t, entity.Details()[0].Quantity(), view.Details[0].Quantity)
			assert.True(t, entity.Details()[0].UnitPrice().Decimal().Equal(view.Details[0].UnitPrice))
		})

		t.Run("クーポン適用済みの購入は値引き額と適用クーポンを写す", func(t *testing.T) {
			t.Parallel()

			entity, err := newPurchaseForCoupon(t)
			require.NoError(t, err)
			userID := uuidtestkit.NewTestFromSalt(t, "view_coupon_user")
			c := newRedemptionCoupon(t, "view_coupon", userID, domaincoupon.NewAllScope())
			require.NoError(t, entity.ApplyCoupon(c.ID(), 1000))

			view := toPurchaseView(entity, c)

			assert.Equal(t, 1000, view.DiscountAmount)
			require.NotNil(t, view.AppliedCoupon)
			assert.Equal(t, c.ID(), view.AppliedCoupon.ID)
			assert.Equal(t, c.Discount().Kind().Name(), view.AppliedCoupon.DiscountKind)
			assert.Equal(t, c.Scope().Kind().Name(), view.AppliedCoupon.ScopeKind)
		})
	})
}

func Test_usecase_CancelPurchase(t *testing.T) {
	t.Parallel()

	userID := uuidtestkit.NewTestFromSalt(t, "cancel_uc_user")
	purchaseID := uuidtestkit.NewTestFromSalt(t, "cancel_uc_id")
	purchaseCode := "cancel-uc-code"

	// lockable は、repo.LockByCode が返す再構築済み購入を生成するローカルヘルパーです。
	lockable := func(t *testing.T, owner uuid.UUID, status domainpurchase.Status) *domainpurchase.Purchase {
		t.Helper()
		details := []domainpurchase.PurchaseDetail{
			domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "cancel_uc_d1"), domainpurchase.PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "cancel_uc_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := domainpurchase.Reconstruct(purchaseID, domainpurchase.Attributes{
			Code:           "cancel-uc-code",
			UserID:         owner,
			StatusID:       uuidtestkit.NewTestFromSalt(t, "cancel_uc_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         nil,
			CanceledAt:     nil,
			ShippedAt:      nil,
			DeliveredAt:    nil,
		})
		require.NoError(t, err)
		return p
	}

	// newUC は、指定 mock を注入した usecase を生成するローカルヘルパーです。
	newUC := func(
		t *testing.T,
		repo *mock_purchase.MockRepository,
		productRepo *mock_product.MockRepository,
		emit *mock_outbox.MockEmitUsecase,
	) *usecase {
		t.Helper()
		return &usecase{
			tracer: observability.NewNoopTracerFactory(t).Usecase(),
			txm:    testkit.NewMockTransactionManager(t),
			repo:   repo, productRepo: productRepo, emit: emit,
			clock: clocktestkit.NewMockClock(t, time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)),
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロック→所有権検証→キャンセル→在庫復元→emit→再読込を経てキャンセルビューを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed), nil)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).
				Return(lockedProducts(t, uuidtestkit.NewTestFromSalt(t, "cancel_uc_p1"), 10), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().UpdateCancelled(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)

			canceledAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
			detail := &domainpurchase.Detail{
				ID:             purchaseID,
				Code:           "cancel-uc-code",
				UserID:         userID,
				StatusID:       uuidtestkit.NewTestFromSalt(t, "cancel_uc_canceled_status"),
				StatusCode:     domainpurchase.StatusCanceled.Code(),
				StatusName:     "キャンセル",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details: []domainpurchase.PurchaseDetail{
					domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "cancel_uc_d1"), domainpurchase.PurchaseDetailAttributes{
						ProductID: uuidtestkit.NewTestFromSalt(t, "cancel_uc_p1"),
						Quantity:  2,
						UnitPrice: mustPrice(t, "800"),
					}),
				},
				OrderedAt:  time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				CanceledAt: &canceledAt,
			}
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(detail, nil)

			u := newUC(t, repo, productRepo, emit)
			view, err := u.CancelPurchase(context.Background(), CancelPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.NoError(t, err)
			assert.Equal(t, detail.StatusID, view.StatusID)
			assert.Equal(t, "キャンセル", view.StatusName)
			require.NotNil(t, view.CanceledAt)
			assert.Equal(t, canceledAt, *view.CanceledAt)
			require.Len(t, view.Details, 1)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("他人の購入の場合、ErrNotFoundを返し在庫復元を行わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			otherOwner := uuidtestkit.NewTestFromSalt(t, "cancel_uc_other")
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, otherOwner, domainpurchase.StatusUnprocessed), nil)

			u := newUC(t, repo, productRepo, emit)
			_, err := u.CancelPurchase(context.Background(), CancelPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("不正遷移（完了）の場合、ErrCancelNotAllowedを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusCompleted), nil)

			u := newUC(t, repo, productRepo, emit)
			_, err := u.CancelPurchase(context.Background(), CancelPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, domainpurchase.ErrCancelNotAllowed)
		})

		t.Run("LockPurchaseが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(nil, apperror.ErrUnavailable)

			u := newUC(t, repo, productRepo, emit)
			_, err := u.CancelPurchase(context.Background(), CancelPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrUnavailable)
		})

		t.Run("CancelPurchase書き込みが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed), nil)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).
				Return(lockedProducts(t, uuidtestkit.NewTestFromSalt(t, "cancel_uc_p1"), 10), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().UpdateCancelled(gomock.Any(), gomock.Any()).Return(apperror.ErrConflict)

			u := newUC(t, repo, productRepo, emit)
			_, err := u.CancelPurchase(context.Background(), CancelPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("outbox発行が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed), nil)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).
				Return(lockedProducts(t, uuidtestkit.NewTestFromSalt(t, "cancel_uc_p1"), 10), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().UpdateCancelled(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, apperror.ErrInternal)

			u := newUC(t, repo, productRepo, emit)
			_, err := u.CancelPurchase(context.Background(), CancelPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("再読込が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			productRepo := mock_product.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed), nil)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).
				Return(lockedProducts(t, uuidtestkit.NewTestFromSalt(t, "cancel_uc_p1"), 10), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo.EXPECT().UpdateCancelled(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(nil, apperror.ErrNotFound)

			u := newUC(t, repo, productRepo, emit)
			_, err := u.CancelPurchase(context.Background(), CancelPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

func Test_usecase_PayPurchase(t *testing.T) {
	t.Parallel()

	userID := uuidtestkit.NewTestFromSalt(t, "pay_uc_user")
	purchaseID := uuidtestkit.NewTestFromSalt(t, "pay_uc_id")
	purchaseCode := "pay-uc-code"

	// lockable は、repo.LockByCode が返す再構築済み購入を生成するローカルヘルパーです。
	lockable := func(t *testing.T, owner uuid.UUID, status domainpurchase.Status, paidAt *time.Time) *domainpurchase.Purchase {
		t.Helper()
		details := []domainpurchase.PurchaseDetail{
			domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "pay_uc_d1"), domainpurchase.PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "pay_uc_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := domainpurchase.Reconstruct(purchaseID, domainpurchase.Attributes{
			Code:           "pay-uc-code",
			UserID:         owner,
			StatusID:       uuidtestkit.NewTestFromSalt(t, "pay_uc_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         paidAt,
			CanceledAt:     nil,
			ShippedAt:      nil,
			DeliveredAt:    nil,
		})
		require.NoError(t, err)
		return p
	}

	// newUC は、指定 mock を注入した usecase を生成するローカルヘルパーです。
	newUC := func(t *testing.T, repo *mock_purchase.MockRepository, emit *mock_outbox.MockEmitUsecase) *usecase {
		t.Helper()
		return &usecase{
			tracer: observability.NewNoopTracerFactory(t).Usecase(),
			txm:    testkit.NewMockTransactionManager(t),
			repo:   repo, emit: emit,
			clock: clocktestkit.NewMockClock(t, time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)),
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロック→所有権検証→支払い→emit→再読込を経て支払いビューを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed, nil), nil)
			repo.EXPECT().UpdatePaid(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)

			paidAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
			detail := &domainpurchase.Detail{
				ID:             purchaseID,
				Code:           "pay-uc-code",
				UserID:         userID,
				StatusID:       uuidtestkit.NewTestFromSalt(t, "pay_uc_paid_status"),
				StatusCode:     domainpurchase.StatusPaid.Code(),
				StatusName:     "支払い済み",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details: []domainpurchase.PurchaseDetail{
					domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "pay_uc_d1"), domainpurchase.PurchaseDetailAttributes{
						ProductID: uuidtestkit.NewTestFromSalt(t, "pay_uc_p1"),
						Quantity:  2,
						UnitPrice: mustPrice(t, "800"),
					}),
				},
				OrderedAt: time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				PaidAt:    &paidAt,
			}
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(detail, nil)

			u := newUC(t, repo, emit)
			view, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.NoError(t, err)
			assert.Equal(t, detail.StatusID, view.StatusID)
			assert.Equal(t, "支払い済み", view.StatusName)
			require.NotNil(t, view.PaidAt)
			assert.Equal(t, paidAt, *view.PaidAt)
			require.Len(t, view.Details, 1)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("他人の購入の場合、ErrNotFoundを返し支払い書き込みを行わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			otherOwner := uuidtestkit.NewTestFromSalt(t, "pay_uc_other")
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, otherOwner, domainpurchase.StatusUnprocessed, nil), nil)

			u := newUC(t, repo, emit)
			_, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("二重支払い（既に支払い済み）の場合、ErrAlreadyPaidを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			paidAt := time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusPaid, &paidAt), nil)

			u := newUC(t, repo, emit)
			_, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, domainpurchase.ErrAlreadyPaid)
		})

		t.Run("不正遷移（キャンセル済み）の場合、ErrPayNotAllowedを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusCompleted, nil), nil)

			u := newUC(t, repo, emit)
			_, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, domainpurchase.ErrPayNotAllowed)
		})

		t.Run("LockPurchaseが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(nil, apperror.ErrUnavailable)

			u := newUC(t, repo, emit)
			_, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrUnavailable)
		})

		t.Run("PayPurchase書き込みが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed, nil), nil)
			repo.EXPECT().UpdatePaid(gomock.Any(), gomock.Any()).Return(apperror.ErrConflict)

			u := newUC(t, repo, emit)
			_, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("outbox発行が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed, nil), nil)
			repo.EXPECT().UpdatePaid(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, apperror.ErrInternal)

			u := newUC(t, repo, emit)
			_, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("再読込が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)

			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(lockable(t, userID, domainpurchase.StatusUnprocessed, nil), nil)
			repo.EXPECT().UpdatePaid(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(nil, apperror.ErrNotFound)

			u := newUC(t, repo, emit)
			_, err := u.PayPurchase(context.Background(), PayPurchaseParams{PurchaseCode: purchaseCode, UserID: userID})
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

//nolint:dupl // 支払い/キャンセルの DTO 写像テストは対称で構造の重複は不可避
func Test_toCancelPurchaseView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("詳細読み取りモデルをキャンセルビューへ写像する", func(t *testing.T) {
			t.Parallel()

			canceledAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
			d := &domainpurchase.Detail{
				ID:             uuidtestkit.NewTestFromSalt(t, "tcv_id"),
				Code:           "tcv-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "tcv_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "tcv_status"),
				StatusCode:     domainpurchase.StatusCanceled.Code(),
				StatusName:     "キャンセル",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details: []domainpurchase.PurchaseDetail{
					domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "tcv_d"), domainpurchase.PurchaseDetailAttributes{
						ProductID: uuidtestkit.NewTestFromSalt(t, "tcv_p"),
						Quantity:  2,
						UnitPrice: mustPrice(t, "800"),
					}),
				},
				OrderedAt:  time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				CanceledAt: &canceledAt,
			}

			view := toCancelPurchaseView(d)
			assert.Equal(t, d.Code, view.Code)
			assert.Equal(t, d.UserID, view.UserID)
			assert.Equal(t, d.StatusID, view.StatusID)
			assert.Equal(t, d.StatusCode, view.StatusCode)
			assert.Equal(t, "キャンセル", view.StatusName)
			assert.Equal(t, d.TotalAmount, view.TotalAmount)
			require.NotNil(t, view.CanceledAt)
			assert.Equal(t, canceledAt, *view.CanceledAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, d.Details[0].ProductID(), view.Details[0].ProductID)
			assert.True(t, d.Details[0].UnitPrice().Decimal().Equal(view.Details[0].UnitPrice))
		})
	})
}

//nolint:dupl // 支払い/キャンセルの DTO 写像テストは対称で構造の重複は不可避
func Test_toPayPurchaseView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("詳細読み取りモデルを支払いビューへ写像する", func(t *testing.T) {
			t.Parallel()

			paidAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
			d := &domainpurchase.Detail{
				ID:             uuidtestkit.NewTestFromSalt(t, "tpv_id"),
				Code:           "tpv-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "tpv_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "tpv_status"),
				StatusCode:     domainpurchase.StatusPaid.Code(),
				StatusName:     "支払い済み",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details: []domainpurchase.PurchaseDetail{
					domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "tpv_d"), domainpurchase.PurchaseDetailAttributes{
						ProductID: uuidtestkit.NewTestFromSalt(t, "tpv_p"),
						Quantity:  2,
						UnitPrice: mustPrice(t, "800"),
					}),
				},
				OrderedAt: time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				PaidAt:    &paidAt,
			}

			view := toPayPurchaseView(d)
			assert.Equal(t, d.Code, view.Code)
			assert.Equal(t, d.UserID, view.UserID)
			assert.Equal(t, d.StatusID, view.StatusID)
			assert.Equal(t, d.StatusCode, view.StatusCode)
			assert.Equal(t, "支払い済み", view.StatusName)
			assert.Equal(t, d.TotalAmount, view.TotalAmount)
			require.NotNil(t, view.PaidAt)
			assert.Equal(t, paidAt, *view.PaidAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, d.Details[0].ProductID(), view.Details[0].ProductID)
			assert.True(t, d.Details[0].UnitPrice().Decimal().Equal(view.Details[0].UnitPrice))
		})
	})
}

func Test_usecase_ShipPurchase(t *testing.T) {
	t.Parallel()

	purchaseID := uuidtestkit.NewTestFromSalt(t, "ship_uc_id")
	purchaseCode := "ship-uc-code"
	ownerID := uuidtestkit.NewTestFromSalt(t, "ship_uc_owner")
	shippedAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)

	// lockable は、repo.LockByCode が返す再構築済み購入を生成するローカルヘルパーです。
	lockable := func(t *testing.T, status domainpurchase.Status, paidAt, shipped *time.Time) *domainpurchase.Purchase {
		t.Helper()
		details := []domainpurchase.PurchaseDetail{
			domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "ship_uc_d1"), domainpurchase.PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "ship_uc_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := domainpurchase.Reconstruct(purchaseID, domainpurchase.Attributes{
			Code:           "ship-uc-code",
			UserID:         ownerID,
			StatusID:       uuidtestkit.NewTestFromSalt(t, "ship_uc_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         paidAt,
			CanceledAt:     nil,
			ShippedAt:      shipped,
			DeliveredAt:    nil,
		})
		require.NoError(t, err)
		return p
	}

	// paidLockable は、発送可能な支払い済み購入を返すローカルヘルパーです。
	paidLockable := func(t *testing.T) *domainpurchase.Purchase {
		t.Helper()
		paidAt := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
		return lockable(t, domainpurchase.StatusPaid, &paidAt, nil)
	}

	// shippedDetail は、発送後の再読込で返す詳細読み取りモデルを生成するローカルヘルパーです。
	shippedDetail := func(t *testing.T) *domainpurchase.Detail {
		t.Helper()
		return &domainpurchase.Detail{
			ID:             purchaseID,
			Code:           "ship-uc-code",
			UserID:         ownerID,
			StatusID:       uuidtestkit.NewTestFromSalt(t, "ship_uc_shipped_status"),
			StatusCode:     domainpurchase.StatusShipped.Code(),
			StatusName:     "発送済み",
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details: []domainpurchase.PurchaseDetail{
				domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "ship_uc_d1"), domainpurchase.PurchaseDetailAttributes{
					ProductID: uuidtestkit.NewTestFromSalt(t, "ship_uc_p1"),
					Quantity:  2,
					UnitPrice: mustPrice(t, "800"),
				}),
			},
			OrderedAt: time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			ShippedAt: &shippedAt,
		}
	}

	// newUC は、指定 mock を注入した usecase を生成するローカルヘルパーです。
	newUC := func(t *testing.T, repo *mock_purchase.MockRepository, emit *mock_outbox.MockEmitUsecase, authorizer *mock_authz.MockAuthorizer) *usecase {
		t.Helper()
		return &usecase{
			tracer: observability.NewNoopTracerFactory(t).Usecase(),
			txm:    testkit.NewMockTransactionManager(t),
			repo:   repo, emit: emit, authorizer: authorizer,
			clock: clocktestkit.NewMockClock(t, shippedAt),
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認可→ロック→発送→emit→再読込を経て発送ビューを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(paidLockable(t), nil)
			repo.EXPECT().UpdateShipped(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			detail := shippedDetail(t)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(detail, nil)

			u := newUC(t, repo, emit, authorizer)
			view, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.NoError(t, err)
			assert.Equal(t, detail.StatusID, view.StatusID)
			assert.Equal(t, "発送済み", view.StatusName)
			require.NotNil(t, view.ShippedAt)
			assert.Equal(t, shippedAt, *view.ShippedAt)
			require.Len(t, view.Details, 1)
		})

		t.Run("購入の所有者と異なる認証主体でも発送でき所有権を問わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authn, err := auth.New("ship-uc-subject", "ship-uc-issuer", nil, nil)
			require.NoError(t, err)
			other, err := authn.WithUserID(uuidtestkit.NewTestFromSalt(t, "ship_uc_other"))
			require.NoError(t, err)

			authorizer.EXPECT().Authorize(gomock.Any(), other, gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(paidLockable(t), nil)
			repo.EXPECT().UpdateShipped(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(shippedDetail(t), nil)

			u := newUC(t, repo, emit, authorizer)
			view, err := u.ShipPurchase(context.Background(), other, purchaseCode)
			require.NoError(t, err)
			// 所有者は購入側の userID のまま（認証主体で上書きされない）。
			assert.Equal(t, ownerID, view.UserID)
		})

		t.Run("発送操作を所有者なしリソースとして認可する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			var capturedAction authz.Action
			var capturedResource *authz.Resource
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ *auth.Authn, action authz.Action, resource *authz.Resource) error {
					capturedAction = action
					capturedResource = resource
					return nil
				})
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(paidLockable(t), nil)
			repo.EXPECT().UpdateShipped(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(shippedDetail(t), nil)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.NoError(t, err)
			assert.Equal(t, authz.ActionPurchaseShip, capturedAction)
			require.NotNil(t, capturedResource)
			assert.Equal(t, "purchase", capturedResource.Kind())
			assert.Nil(t, capturedResource.OwnerID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合、ErrUnauthenticatedを返し認可を行わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), nil, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("非adminの場合、認可エラーを伝播しロックを行わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(authz.ErrForbidden)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})

		t.Run("二重発送（既に発送済み）の場合、ErrAlreadyShippedを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			paidAt := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).
				Return(lockable(t, domainpurchase.StatusShipped, &paidAt, &shippedAt), nil)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, domainpurchase.ErrAlreadyShipped)
		})

		t.Run("不正遷移（未払い）の場合、ErrShipNotAllowedを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).
				Return(lockable(t, domainpurchase.StatusUnprocessed, nil, nil), nil)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, domainpurchase.ErrShipNotAllowed)
		})

		t.Run("存在しない購入の場合、ErrNotFoundを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(nil, apperror.ErrNotFound)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("発送書き込みが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(paidLockable(t), nil)
			repo.EXPECT().UpdateShipped(gomock.Any(), gomock.Any()).Return(apperror.ErrConflict)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("outbox発行が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(paidLockable(t), nil)
			repo.EXPECT().UpdateShipped(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, apperror.ErrInternal)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("再読込が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(paidLockable(t), nil)
			repo.EXPECT().UpdateShipped(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(nil, apperror.ErrNotFound)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.ShipPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

//nolint:dupl // 発送/支払いの DTO 写像テストは対称で構造の重複は不可避
func Test_toShipPurchaseView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("詳細読み取りモデルを発送ビューへ写像する", func(t *testing.T) {
			t.Parallel()

			shippedAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
			d := &domainpurchase.Detail{
				ID:             uuidtestkit.NewTestFromSalt(t, "tsv_id"),
				Code:           "tsv-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "tsv_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "tsv_status"),
				StatusCode:     domainpurchase.StatusShipped.Code(),
				StatusName:     "発送済み",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details: []domainpurchase.PurchaseDetail{
					domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "tsv_d"), domainpurchase.PurchaseDetailAttributes{
						ProductID: uuidtestkit.NewTestFromSalt(t, "tsv_p"),
						Quantity:  2,
						UnitPrice: mustPrice(t, "800"),
					}),
				},
				OrderedAt: time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				ShippedAt: &shippedAt,
			}

			view := toShipPurchaseView(d)
			assert.Equal(t, d.Code, view.Code)
			assert.Equal(t, d.UserID, view.UserID)
			assert.Equal(t, d.StatusID, view.StatusID)
			assert.Equal(t, d.StatusCode, view.StatusCode)
			assert.Equal(t, "発送済み", view.StatusName)
			assert.Equal(t, d.TotalAmount, view.TotalAmount)
			require.NotNil(t, view.ShippedAt)
			assert.Equal(t, shippedAt, *view.ShippedAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, d.Details[0].ProductID(), view.Details[0].ProductID)
			assert.True(t, d.Details[0].UnitPrice().Decimal().Equal(view.Details[0].UnitPrice))
		})
	})
}

func Test_usecase_DeliverPurchase(t *testing.T) {
	t.Parallel()

	purchaseID := uuidtestkit.NewTestFromSalt(t, "dlv_uc_id")
	purchaseCode := "dlv-uc-code"
	ownerID := uuidtestkit.NewTestFromSalt(t, "dlv_uc_owner")
	paidAt := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
	shippedAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	deliveredAt := time.Date(2026, time.July, 28, 9, 0, 0, 0, time.UTC)

	// lockable は、repo.LockByCode が返す再構築済み購入を生成するローカルヘルパーです。
	lockable := func(t *testing.T, status domainpurchase.Status, paid, shipped, delivered *time.Time) *domainpurchase.Purchase {
		t.Helper()
		details := []domainpurchase.PurchaseDetail{
			domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "dlv_uc_d1"), domainpurchase.PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "dlv_uc_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := domainpurchase.Reconstruct(purchaseID, domainpurchase.Attributes{
			Code:           "dlv-uc-code",
			UserID:         ownerID,
			StatusID:       uuidtestkit.NewTestFromSalt(t, "dlv_uc_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         paid,
			CanceledAt:     nil,
			ShippedAt:      shipped,
			DeliveredAt:    delivered,
		})
		require.NoError(t, err)
		return p
	}

	// shippedLockable は、配達可能な発送済み購入を返すローカルヘルパーです。
	shippedLockable := func(t *testing.T) *domainpurchase.Purchase {
		t.Helper()
		return lockable(t, domainpurchase.StatusShipped, &paidAt, &shippedAt, nil)
	}

	// deliveredDetail は、配達完了後の再読込で返す詳細読み取りモデルを生成するローカルヘルパーです。
	deliveredDetail := func(t *testing.T) *domainpurchase.Detail {
		t.Helper()
		return &domainpurchase.Detail{
			ID:             purchaseID,
			Code:           "dlv-uc-code",
			UserID:         ownerID,
			StatusID:       uuidtestkit.NewTestFromSalt(t, "dlv_uc_delivered_status"),
			StatusCode:     domainpurchase.StatusDelivered.Code(),
			StatusName:     "配達済み",
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details: []domainpurchase.PurchaseDetail{
				domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "dlv_uc_d1"), domainpurchase.PurchaseDetailAttributes{
					ProductID: uuidtestkit.NewTestFromSalt(t, "dlv_uc_p1"),
					Quantity:  2,
					UnitPrice: mustPrice(t, "800"),
				}),
			},
			OrderedAt:   time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			DeliveredAt: &deliveredAt,
		}
	}

	// newUC は、指定 mock を注入した usecase を生成するローカルヘルパーです。
	newUC := func(t *testing.T, repo *mock_purchase.MockRepository, emit *mock_outbox.MockEmitUsecase, authorizer *mock_authz.MockAuthorizer) *usecase {
		t.Helper()
		return &usecase{
			tracer: observability.NewNoopTracerFactory(t).Usecase(),
			txm:    testkit.NewMockTransactionManager(t),
			repo:   repo, emit: emit, authorizer: authorizer,
			clock: clocktestkit.NewMockClock(t, deliveredAt),
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認可→ロック→配達→emit→再読込を経て配達ビューを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(shippedLockable(t), nil)
			repo.EXPECT().UpdateDelivered(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			detail := deliveredDetail(t)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(detail, nil)

			u := newUC(t, repo, emit, authorizer)
			view, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.NoError(t, err)
			assert.Equal(t, detail.StatusID, view.StatusID)
			assert.Equal(t, "配達済み", view.StatusName)
			require.NotNil(t, view.DeliveredAt)
			assert.Equal(t, deliveredAt, *view.DeliveredAt)
			require.Len(t, view.Details, 1)
		})

		t.Run("配達完了イベントをpurchase.delivered.v1として同一tx内で発行する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			var captured outbox.EmitInput
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(shippedLockable(t), nil)
			repo.EXPECT().UpdateDelivered(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, in outbox.EmitInput) (uuid.UUID, error) {
					captured = in
					return uuid.UUID{}, nil
				})
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(deliveredDetail(t), nil)

			// 更新と emit が 1 つのトランザクションに収まることを、Do の呼び出し回数で固定する。
			singleTx := mock_tx.NewMockManager(ctrl)
			singleTx.EXPECT().Do(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
				func(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) },
			)

			u := newUC(t, repo, emit, authorizer)
			u.txm = singleTx
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.NoError(t, err)
			assert.Equal(t, event.TypeDelivered, captured.EventType)
			assert.Equal(t, aggregateType, captured.AggregateType)
			assert.Equal(t, purchaseID.String(), captured.AggregateID)

			var payload struct {
				PurchaseID  string `json:"purchaseId"`
				UserID      string `json:"userId"`
				StatusCode  int    `json:"statusCode"`
				DeliveredAt string `json:"deliveredAt"`
			}
			require.NoError(t, json.Unmarshal(captured.Payload, &payload))
			assert.Equal(t, purchaseID.String(), payload.PurchaseID)
			assert.Equal(t, domainpurchase.StatusDelivered.Code(), payload.StatusCode)
			assert.Equal(t, deliveredAt.Format(time.RFC3339Nano), payload.DeliveredAt)
		})

		t.Run("購入の所有者と異なる認証主体でも配達でき所有権を問わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authn, err := auth.New("dlv-uc-subject", "dlv-uc-issuer", nil, nil)
			require.NoError(t, err)
			other, err := authn.WithUserID(uuidtestkit.NewTestFromSalt(t, "dlv_uc_other"))
			require.NoError(t, err)

			authorizer.EXPECT().Authorize(gomock.Any(), other, gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(shippedLockable(t), nil)
			repo.EXPECT().UpdateDelivered(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(deliveredDetail(t), nil)

			u := newUC(t, repo, emit, authorizer)
			view, err := u.DeliverPurchase(context.Background(), other, purchaseCode)
			require.NoError(t, err)
			// 所有者は購入側の userID のまま（認証主体で上書きされない）。
			assert.Equal(t, ownerID, view.UserID)
		})

		t.Run("配達完了操作を所有者なしリソースとして認可する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			var capturedAction authz.Action
			var capturedResource *authz.Resource
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ *auth.Authn, action authz.Action, resource *authz.Resource) error {
					capturedAction = action
					capturedResource = resource
					return nil
				})
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(shippedLockable(t), nil)
			repo.EXPECT().UpdateDelivered(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(deliveredDetail(t), nil)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.NoError(t, err)
			assert.Equal(t, authz.ActionPurchaseDeliver, capturedAction)
			require.NotNil(t, capturedResource)
			assert.Equal(t, "purchase", capturedResource.Kind())
			assert.Nil(t, capturedResource.OwnerID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合、ErrUnauthenticatedを返し認可を行わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), nil, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("非adminの場合、認可エラーを伝播しロックを行わない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(authz.ErrForbidden)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})

		t.Run("二重配達（既に配達済み）の場合、ErrAlreadyDeliveredを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).
				Return(lockable(t, domainpurchase.StatusDelivered, &paidAt, &shippedAt, &deliveredAt), nil)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, domainpurchase.ErrAlreadyDelivered)
		})

		t.Run("不正遷移（未発送の支払い済み）の場合、ErrDeliverNotAllowedを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).
				Return(lockable(t, domainpurchase.StatusPaid, &paidAt, nil, nil), nil)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, domainpurchase.ErrDeliverNotAllowed)
		})

		t.Run("存在しない購入の場合、ErrNotFoundを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(nil, apperror.ErrNotFound)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("配達書き込みが失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(shippedLockable(t), nil)
			repo.EXPECT().UpdateDelivered(gomock.Any(), gomock.Any()).Return(apperror.ErrConflict)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("outbox発行が失敗した場合はtxを中断しエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(shippedLockable(t), nil)
			repo.EXPECT().UpdateDelivered(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, apperror.ErrInternal)
			// FindDetailByID を EXPECT しないことで、emit 失敗時に tx 関数が即座に中断する
			// （＝ランナーがロールバックする）ことを担保する。

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("再読込が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_purchase.NewMockRepository(ctrl)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().LockByCode(gomock.Any(), purchaseCode).Return(shippedLockable(t), nil)
			repo.EXPECT().UpdateDelivered(gomock.Any(), gomock.Any()).Return(nil)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)
			repo.EXPECT().FindDetailByID(gomock.Any(), purchaseID).Return(nil, apperror.ErrNotFound)

			u := newUC(t, repo, emit, authorizer)
			_, err := u.DeliverPurchase(context.Background(), &auth.Authn{}, purchaseCode)
			require.ErrorIs(t, err, apperror.ErrNotFound)
		})
	})
}

//nolint:dupl // 配達/発送の DTO 写像テストは対称で構造の重複は不可避
func Test_toDeliverPurchaseView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("詳細読み取りモデルを配達ビューへ写像する", func(t *testing.T) {
			t.Parallel()

			deliveredAt := time.Date(2026, time.July, 28, 9, 0, 0, 0, time.UTC)
			d := &domainpurchase.Detail{
				ID:             uuidtestkit.NewTestFromSalt(t, "tdv_id"),
				Code:           "tdv-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "tdv_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "tdv_status"),
				StatusCode:     domainpurchase.StatusDelivered.Code(),
				StatusName:     "配達済み",
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details: []domainpurchase.PurchaseDetail{
					domainpurchase.NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "tdv_d"), domainpurchase.PurchaseDetailAttributes{
						ProductID: uuidtestkit.NewTestFromSalt(t, "tdv_p"),
						Quantity:  2,
						UnitPrice: mustPrice(t, "800"),
					}),
				},
				OrderedAt:   time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
				DeliveredAt: &deliveredAt,
			}

			view := toDeliverPurchaseView(d)
			assert.Equal(t, d.Code, view.Code)
			assert.Equal(t, d.UserID, view.UserID)
			assert.Equal(t, d.StatusID, view.StatusID)
			assert.Equal(t, d.StatusCode, view.StatusCode)
			assert.Equal(t, "配達済み", view.StatusName)
			assert.Equal(t, d.TotalAmount, view.TotalAmount)
			require.NotNil(t, view.DeliveredAt)
			assert.Equal(t, deliveredAt, *view.DeliveredAt)
			require.Len(t, view.Details, 1)
			assert.Equal(t, d.Details[0].ProductID(), view.Details[0].ProductID)
			assert.True(t, d.Details[0].UnitPrice().Decimal().Equal(view.Details[0].UnitPrice))
		})
	})
}

func Test_detailProductIDs(t *testing.T) {
	t.Parallel()

	productX := uuidtestkit.NewTestFromSalt(t, "detail_ids_x")
	productY := uuidtestkit.NewTestFromSalt(t, "detail_ids_y")
	detail := func(salt string, productID uuid.UUID) domainpurchase.PurchaseDetail {
		return domainpurchase.NewPurchaseDetail(
			uuidtestkit.NewTestFromSalt(t, salt),
			domainpurchase.PurchaseDetailAttributes{ProductID: productID, Quantity: 1, UnitPrice: mustPrice(t, "800")},
		)
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細の順序を保って商品IDを返す", func(t *testing.T) {
			t.Parallel()

			got := detailProductIDs([]domainpurchase.PurchaseDetail{detail("d1", productX), detail("d2", productY)})

			assert.Equal(t, []uuid.UUID{productX, productY}, got)
		})

		t.Run("同一商品を参照する明細が複数あっても商品IDは1つに畳む", func(t *testing.T) {
			t.Parallel()

			got := detailProductIDs([]domainpurchase.PurchaseDetail{detail("d1", productX), detail("d2", productX)})

			assert.Equal(t, []uuid.UUID{productX}, got)
		})

		t.Run("明細が無い場合は空を返す", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, detailProductIDs(nil))
		})
	})
}

func Test_usecase_applyStockDelta(t *testing.T) {
	t.Parallel()

	productID := uuidtestkit.NewTestFromSalt(t, "apply_delta_product")
	details := []domainpurchase.PurchaseDetail{
		domainpurchase.NewPurchaseDetail(
			uuidtestkit.NewTestFromSalt(t, "apply_delta_detail"),
			domainpurchase.PurchaseDetailAttributes{ProductID: productID, Quantity: 3, UnitPrice: mustPrice(t, "800")},
		),
	}

	newUC := func(t *testing.T, productRepo *mock_product.MockRepository) *usecase {
		t.Helper()
		return &usecase{tracer: observability.NewNoopTracerFactory(t).Usecase(), productRepo: productRepo}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("符号が負の場合、明細の数量だけ在庫を減らして永続化する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			products := lockedProducts(t, productID, 10)
			productRepo.EXPECT().UpdateStock(gomock.Any(), products[0]).Return(2, nil)

			require.NoError(t, newUC(t, productRepo).applyStockDelta(context.Background(), products, details, -1))
			assert.Equal(t, 7, products[0].Quantity())
		})

		t.Run("符号が正の場合、明細の数量だけ在庫を戻して永続化する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			products := lockedProducts(t, productID, 10)
			productRepo.EXPECT().UpdateStock(gomock.Any(), products[0]).Return(2, nil)

			require.NoError(t, newUC(t, productRepo).applyStockDelta(context.Background(), products, details, 1))
			assert.Equal(t, 13, products[0].Quantity())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細が参照する商品がロック結果に無い場合、ErrNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			other := lockedProducts(t, uuidtestkit.NewTestFromSalt(t, "apply_delta_other"), 10)

			err := newUC(t, productRepo).applyStockDelta(context.Background(), other, details, -1)

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("在庫が不足して範囲外になる場合、ドメインのエラーを伝播し永続化しない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			products := lockedProducts(t, productID, 1)

			err := newUC(t, productRepo).applyStockDelta(context.Background(), products, details, -1)

			require.ErrorIs(t, err, domainproduct.ErrInvalidQuantity)
		})

		t.Run("在庫の永続化が失敗した場合はエラーを伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			products := lockedProducts(t, productID, 10)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(0, apperror.ErrConflict)

			err := newUC(t, productRepo).applyStockDelta(context.Background(), products, details, -1)

			require.ErrorIs(t, err, apperror.ErrConflict)
		})
	})
}

func Test_usecase_restoreStock(t *testing.T) {
	t.Parallel()

	productID := uuidtestkit.NewTestFromSalt(t, "restore_stock_product")
	details := []domainpurchase.PurchaseDetail{
		domainpurchase.NewPurchaseDetail(
			uuidtestkit.NewTestFromSalt(t, "restore_stock_detail"),
			domainpurchase.PurchaseDetailAttributes{ProductID: productID, Quantity: 3, UnitPrice: mustPrice(t, "800")},
		),
	}

	newUC := func(t *testing.T, productRepo *mock_product.MockRepository) *usecase {
		t.Helper()
		return &usecase{tracer: observability.NewNoopTracerFactory(t).Usecase(), productRepo: productRepo}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象商品をロックしてから明細の数量だけ在庫を戻す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			products := lockedProducts(t, productID, 10)
			productRepo.EXPECT().LockByIDs(gomock.Any(), []uuid.UUID{productID}).Return(products, nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), products[0]).Return(2, nil)

			require.NoError(t, newUC(t, productRepo).restoreStock(context.Background(), details))
			assert.Equal(t, 13, products[0].Quantity())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロックが失敗した場合はエラーを伝播し在庫を書き換えない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			productRepo := mock_product.NewMockRepository(ctrl)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrUnavailable)

			err := newUC(t, productRepo).restoreStock(context.Background(), details)

			require.ErrorIs(t, err, apperror.ErrUnavailable)
		})
	})
}

// newPurchaseForCoupon は、クーポン適用のテストで使う購入を組み立てます。
func newPurchaseForCoupon(t *testing.T) (*domainpurchase.Purchase, error) {
	t.Helper()

	productID := uuidtestkit.NewTestFromSalt(t, "apply_product")

	return domainpurchase.New(
		uuidtestkit.NewTestFromSalt(t, "apply_purchase"),
		"apply-code-001",
		uuidtestkit.NewTestFromSalt(t, "apply_user"),
		[]domainpurchase.DetailInput{{
			ID:        uuidtestkit.NewTestFromSalt(t, "apply_detail"),
			ProductID: productID,
			Quantity:  1,
		}},
		[]domainpurchase.LockedProduct{
			domainpurchase.NewLockedProduct(productID, mustPrice(t, "100"), 10),
		},
	)
}

// newRedemptionCoupon は、引き換えのテストで使う未使用のクーポンを組み立てます。
func newRedemptionCoupon(t *testing.T, salt string, userID uuid.UUID, scope domaincoupon.Scope) *domaincoupon.Coupon {
	t.Helper()

	value, err := decimal.Parse("0.10")
	require.NoError(t, err)
	discount, err := domaincoupon.NewRateDiscount(value)
	require.NoError(t, err)

	c, err := domaincoupon.New(uuidtestkit.NewTestFromSalt(t, salt), domaincoupon.Attributes{
		UserID:    userID,
		Discount:  discount,
		Scope:     scope,
		ExpiresAt: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		IssuedAt:  time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	return c
}

func Test_notHeldError(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("detailsにcouponIdを載せた422の族を返す", func(t *testing.T) {
			t.Parallel()

			err := notHeldError()

			require.ErrorIs(t, err, domaincoupon.ErrNotHeld)
			require.ErrorIs(t, err, apperror.ErrValidation)

			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{domaincoupon.FieldCouponID}, meta.Details())
		})
	})
}

func Test_toAppliedCouponView(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("種別を名前で写す", func(t *testing.T) {
			t.Parallel()

			userID := uuidtestkit.NewTestFromSalt(t, "view_user")
			categoryID := uuidtestkit.NewTestFromSalt(t, "view_category")
			scope, err := domaincoupon.NewCategoryScope(categoryID)
			require.NoError(t, err)
			c := newRedemptionCoupon(t, "view_coupon", userID, scope)

			got := toAppliedCouponView(c)

			require.NotNil(t, got)
			assert.Equal(t, c.ID(), got.ID)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Equal(t, "category", got.ScopeKind)
			require.NotNil(t, got.ScopeTargetID)
			assert.Equal(t, categoryID, *got.ScopeTargetID)
		})

		t.Run("未適用の場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, toAppliedCouponView(nil))
		})
	})
}

func Test_applyRedeemedCoupon(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("クーポンが無い場合は何もしない", func(t *testing.T) {
			t.Parallel()

			entity, err := newPurchaseForCoupon(t)
			require.NoError(t, err)

			require.NoError(t, applyRedeemedCoupon(entity, nil, nil))
			assert.Nil(t, entity.CouponID())
		})

		t.Run("対象の明細がある場合、値引きを購入へ反映する", func(t *testing.T) {
			t.Parallel()

			entity, err := newPurchaseForCoupon(t)
			require.NoError(t, err)
			userID := uuidtestkit.NewTestFromSalt(t, "apply_ok_user")
			// カテゴリ限定にすることで、商品 ID からカテゴリ ID を引く写像が正しいことまで確かめる。
			products := lockedProducts(t, uuidtestkit.NewTestFromSalt(t, "apply_product"), 10)
			scope, serr := domaincoupon.NewCategoryScope(products[0].Category().ID())
			require.NoError(t, serr)
			c := newRedemptionCoupon(t, "apply_ok_coupon", userID, scope)

			require.NoError(t, applyRedeemedCoupon(entity, c, products))

			require.NotNil(t, entity.CouponID())
			assert.Equal(t, c.ID(), *entity.CouponID())
			// 単価 100.00 × 数量 1 の 10% で 1000 セント。
			assert.Equal(t, 1000, entity.DiscountAmount())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象の明細が無い場合、値引き0として422を返す", func(t *testing.T) {
			t.Parallel()

			entity, err := newPurchaseForCoupon(t)
			require.NoError(t, err)
			userID := uuidtestkit.NewTestFromSalt(t, "apply_user")
			scope, serr := domaincoupon.NewProductScope(uuidtestkit.NewTestFromSalt(t, "apply_other"))
			require.NoError(t, serr)
			c := newRedemptionCoupon(t, "apply_coupon", userID, scope)

			aerr := applyRedeemedCoupon(entity, c, nil)

			require.ErrorIs(t, aerr, domainpurchase.ErrZeroDiscount)
			assert.Nil(t, entity.CouponID())
		})
	})
}

// newCouponUsecase は、クーポン Repository だけを差し替えた usecase を返します。
func newCouponUsecase(t *testing.T, couponRepo *mock_coupon.MockRepository) *usecase {
	t.Helper()

	return &usecase{
		tracer:     observability.NewNoopTracerFactory(t).Usecase(),
		couponRepo: couponRepo,
	}
}

func Test_usecase_redeemRequestedCoupon(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("指定が無い場合はリポジトリを引かずnilを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			u := newCouponUsecase(t, mock_coupon.NewMockRepository(ctrl))

			got, err := u.redeemRequestedCoupon(context.Background(), CreatePurchaseParams{}, now)

			require.NoError(t, err)
			assert.Nil(t, got)
		})

		t.Run("指定がある場合は引き換えて返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			userID := uuidtestkit.NewTestFromSalt(t, "req_user")
			c := newRedemptionCoupon(t, "req_coupon", userID, domaincoupon.NewAllScope())
			repo.EXPECT().LockByID(gomock.Any(), c.ID()).Return(c, nil)

			u := newCouponUsecase(t, repo)
			got, err := u.redeemRequestedCoupon(
				context.Background(), CreatePurchaseParams{UserID: userID, CouponID: ptr.To(c.ID())}, now,
			)

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.True(t, got.IsUsed())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("指定があるが引き換えに失敗した場合、エラーをそのまま返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			couponID := uuidtestkit.NewTestFromSalt(t, "req_missing_coupon")
			repo.EXPECT().LockByID(gomock.Any(), couponID).Return(nil, apperror.ErrNotFound)

			u := newCouponUsecase(t, repo)
			got, err := u.redeemRequestedCoupon(
				context.Background(),
				CreatePurchaseParams{
					UserID:   uuidtestkit.NewTestFromSalt(t, "req_missing_user"),
					CouponID: ptr.To(couponID),
				},
				now,
			)

			require.ErrorIs(t, err, domaincoupon.ErrNotHeld)
			assert.Nil(t, got)
		})
	})
}

func Test_usecase_redeemCoupon(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保有する未使用のクーポンを使用済みにして返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			userID := uuidtestkit.NewTestFromSalt(t, "redeem_user")
			c := newRedemptionCoupon(t, "redeem_coupon", userID, domaincoupon.NewAllScope())
			repo.EXPECT().LockByID(gomock.Any(), c.ID()).Return(c, nil)

			got, err := newCouponUsecase(t, repo).redeemCoupon(context.Background(), c.ID(), userID, now)

			require.NoError(t, err)
			require.NotNil(t, got.UsedAt())
			assert.Equal(t, now, *got.UsedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("存在しない場合、保有していない扱いの422を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			repo.EXPECT().LockByID(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrNotFound)

			_, err := newCouponUsecase(t, repo).redeemCoupon(
				context.Background(), uuidtestkit.NewTestFromSalt(t, "absent"), uuid.UUID{}, now,
			)

			require.ErrorIs(t, err, domaincoupon.ErrNotHeld)
		})

		t.Run("他人のクーポンの場合、存在しない場合と同じ422を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			owner := uuidtestkit.NewTestFromSalt(t, "other_owner")
			c := newRedemptionCoupon(t, "other_coupon", owner, domaincoupon.NewAllScope())
			repo.EXPECT().LockByID(gomock.Any(), c.ID()).Return(c, nil)

			_, err := newCouponUsecase(t, repo).redeemCoupon(
				context.Background(), c.ID(), uuidtestkit.NewTestFromSalt(t, "stranger"), now,
			)

			require.ErrorIs(t, err, domaincoupon.ErrNotHeld)
			assert.False(t, c.IsUsed())
		})

		t.Run("使用済みの場合、detailsにcouponIdを載せて返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			userID := uuidtestkit.NewTestFromSalt(t, "used_user")
			c := newRedemptionCoupon(t, "used_coupon", userID, domaincoupon.NewAllScope())
			require.NoError(t, c.Redeem(now))
			repo.EXPECT().LockByID(gomock.Any(), c.ID()).Return(c, nil)

			_, err := newCouponUsecase(t, repo).redeemCoupon(context.Background(), c.ID(), userID, now)

			require.ErrorIs(t, err, domaincoupon.ErrAlreadyUsed)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{domaincoupon.FieldCouponID}, meta.Details())
		})

		t.Run("リポジトリの失敗はそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			repo.EXPECT().LockByID(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrCanceled)

			_, err := newCouponUsecase(t, repo).redeemCoupon(
				context.Background(), uuidtestkit.NewTestFromSalt(t, "err"), uuid.UUID{}, now,
			)

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_usecase_markCouponUsed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("クーポンが無い場合はリポジトリを呼ばない", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			u := newCouponUsecase(t, mock_coupon.NewMockRepository(ctrl))

			require.NoError(t, u.markCouponUsed(context.Background(), nil, now))
		})

		t.Run("クーポンがある場合は使用済みへ更新する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			c := newRedemptionCoupon(t, "mark_coupon", uuidtestkit.NewTestFromSalt(t, "mark_user"), domaincoupon.NewAllScope())
			repo.EXPECT().UpdateUsed(gomock.Any(), c.ID(), now).Return(nil)

			require.NoError(t, newCouponUsecase(t, repo).markCouponUsed(context.Background(), c, now))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("競合をそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mock_coupon.NewMockRepository(ctrl)
			c := newRedemptionCoupon(t, "conflict_coupon", uuidtestkit.NewTestFromSalt(t, "conflict_user"), domaincoupon.NewAllScope())
			repo.EXPECT().UpdateUsed(gomock.Any(), c.ID(), now).Return(domaincoupon.ErrUsedConcurrently)

			err := newCouponUsecase(t, repo).markCouponUsed(context.Background(), c, now)

			require.ErrorIs(t, err, apperror.ErrConflict)
		})
	})
}

func Test_usecase_createPurchaseInTx(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("クーポン指定ありのとき引き換えから使用済み確定まで通して購入を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userID := uuidtestkit.NewTestFromSalt(t, "txok_user")
			productID := uuidtestkit.NewTestFromSalt(t, "txok_product")
			c := newRedemptionCoupon(t, "txok_coupon", userID, domaincoupon.NewAllScope())
			now := time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)

			userLock := mock_user.NewMockLockRepository(ctrl)
			userLock.EXPECT().LockShareByID(gomock.Any(), userID).Return(activePurchaser(t, userID), nil)
			couponRepo := mock_coupon.NewMockRepository(ctrl)
			couponRepo.EXPECT().LockByID(gomock.Any(), c.ID()).Return(c, nil)
			couponRepo.EXPECT().UpdateUsed(gomock.Any(), c.ID(), now).Return(nil)
			productRepo := mock_product.NewMockRepository(ctrl)
			productRepo.EXPECT().LockByIDs(gomock.Any(), gomock.Any()).Return(lockedProducts(t, productID, 20), nil)
			productRepo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(2, nil)
			repo := mock_purchase.NewMockRepository(ctrl)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			reread := rereadPurchase(t)
			repo.EXPECT().FindByID(gomock.Any(), gomock.Any()).Return(reread, nil)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			// payload の注文日時が読み直した集約のものであることを見ることで、
			// emit が読み直しのあとに走っていることを固定する（順序を戻す退行を赤にする）。
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, in outbox.EmitInput) (uuid.UUID, error) {
					assertPayloadOrderedAt(t, in.Payload, reread.OrderedAt())

					return uuid.UUID{}, nil
				})

			u := &usecase{
				tracer:      observability.NewNoopTracerFactory(t).Usecase(),
				repo:        repo,
				productRepo: productRepo,
				userLock:    userLock,
				couponRepo:  couponRepo,
				emit:        emit,
			}
			draft, err := newPurchaseDraft([]DetailParam{{ProductID: productID, Quantity: 2}})
			require.NoError(t, err)

			entity, redeemed, cerr := u.createPurchaseInTx(
				context.Background(),
				CreatePurchaseParams{UserID: userID, CouponID: ptr.To(c.ID())},
				draft,
				now,
			)

			require.NoError(t, cerr)
			assert.Equal(t, reread, entity)
			require.NotNil(t, redeemed)
			assert.Equal(t, c.ID(), redeemed.ID())
			assert.True(t, redeemed.IsUsed())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("在籍ガードに落ちた場合、クーポンも商品も引かずエラーを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			userLock := mock_user.NewMockLockRepository(ctrl)
			userLock.EXPECT().LockShareByID(gomock.Any(), gomock.Any()).Return(nil, apperror.ErrNotFound)

			u := &usecase{
				tracer:      observability.NewNoopTracerFactory(t).Usecase(),
				userLock:    userLock,
				couponRepo:  mock_coupon.NewMockRepository(ctrl),
				productRepo: mock_product.NewMockRepository(ctrl),
			}
			draft, err := newPurchaseDraft([]DetailParam{
				{ProductID: uuidtestkit.NewTestFromSalt(t, "tx_product"), Quantity: 1},
			})
			require.NoError(t, err)

			_, _, cerr := u.createPurchaseInTx(
				context.Background(),
				CreatePurchaseParams{UserID: uuidtestkit.NewTestFromSalt(t, "tx_user")},
				draft,
				time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC),
			)

			require.Error(t, cerr)
		})
	})
}

// assertPayloadOrderedAt は、outbox payload の注文日時が期待どおりであることを確かめます。
// 生成直後の集約から組まれていればゼロ値になるため、これが読み直し後であることの証拠になります。
func assertPayloadOrderedAt(t *testing.T, payload []byte, want time.Time) {
	t.Helper()

	var decoded struct {
		OrderedAt string `json:"orderedAt"`
	}
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Equal(t, want.Format(time.RFC3339Nano), decoded.OrderedAt)
}

func Test_usecase_emitCreated(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("購入作成のイベントをoutboxへ積む", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, nil)

			u := &usecase{tracer: observability.NewNoopTracerFactory(t).Usecase(), emit: emit}
			entity := rereadPurchase(t)

			require.NoError(t, u.emitCreated(context.Background(), entity))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("outboxの失敗をそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			emit := mock_outbox.NewMockEmitUsecase(ctrl)
			emit.EXPECT().Emit(gomock.Any(), gomock.Any()).Return(uuid.UUID{}, apperror.ErrCanceled)

			u := &usecase{tracer: observability.NewNoopTracerFactory(t).Usecase(), emit: emit}
			entity := rereadPurchase(t)

			err := u.emitCreated(context.Background(), entity)

			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}
