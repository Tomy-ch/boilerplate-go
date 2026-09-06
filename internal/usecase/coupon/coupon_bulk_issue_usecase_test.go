package coupon

import (
	"context"
	"testing"

	"go-boilerplate/internal/apperror"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/internal/usecase/coupon/command"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func runBulkIssueInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newBulkIssueParams(t *testing.T) IssuePromotionalCouponsParams {
	t.Helper()

	return IssuePromotionalCouponsParams{
		DiscountKind:  domaincoupon.DiscountKindRate.Name(),
		DiscountValue: newDecimal(t, "0.15"),
		ScopeKind:     domaincoupon.ScopeKindAll.Name(),
		ExpiresAt:     testExpiresAt,
	}
}

func Test_usecase_IssuePromotionalCoupons(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("退会していない全員へ発行し、件数と日時を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionCouponBulkIssue, gomock.Any()).
				Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().CountByActive(gomock.Any(), ptr.To(true)).Return(int64(3), nil)
			deps.bulkIssueCmd.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, p command.IssuePromotionalCouponsParams) (command.IssuePromotionalCouponsResult, error) {
					assert.Equal(t, domaincoupon.DiscountKindRate, p.Discount.Kind())
					assert.Equal(t, domaincoupon.ScopeKindAll, p.Scope.Kind())
					assert.Nil(t, p.Scope.TargetID())
					assert.Equal(t, testExpiresAt, p.ExpiresAt)
					assert.Equal(t, testNow, p.IssuedAt)

					return command.IssuePromotionalCouponsResult{RecipientCount: 3, IssuedCouponCount: 3}, nil
				})

			view, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, newBulkIssueParams(t))

			require.NoError(t, err)
			assert.Equal(t, testNow, view.IssuedAt)
			assert.Equal(t, testExpiresAt, view.ExpiresAt)
			assert.Equal(t, int64(3), view.RecipientCount)
			assert.Equal(t, int64(3), view.IssuedCouponCount)
		})

		t.Run("受給者が 0 人でも成功し、件数 0 を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().CountByActive(gomock.Any(), ptr.To(true)).Return(int64(0), nil)
			deps.bulkIssueCmd.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any()).
				Return(command.IssuePromotionalCouponsResult{}, nil)

			view, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, newBulkIssueParams(t))

			require.NoError(t, err)
			assert.Equal(t, int64(0), view.RecipientCount)
			assert.Equal(t, int64(0), view.IssuedCouponCount)
		})

		t.Run("適用範囲がカテゴリの場合、対象の存在を確かめてから発行する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			categoryID := uuidtestkit.NewTestFromSalt(t, "bulk_issue_category")

			params := newBulkIssueParams(t)
			params.ScopeKind = domaincoupon.ScopeKindCategory.Name()
			params.ScopeTargetID = &categoryID

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.categoryRepo.EXPECT().FindByID(gomock.Any(), categoryID).Return(nil, nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().CountByActive(gomock.Any(), ptr.To(true)).Return(int64(1), nil)
			deps.bulkIssueCmd.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, p command.IssuePromotionalCouponsParams) (command.IssuePromotionalCouponsResult, error) {
					assert.Equal(t, domaincoupon.ScopeKindCategory, p.Scope.Kind())
					assert.Equal(t, categoryID, *p.Scope.TargetID())

					return command.IssuePromotionalCouponsResult{RecipientCount: 1, IssuedCouponCount: 1}, nil
				})

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.NoError(t, err)
		})

		t.Run("適用範囲が商品の場合、商品の存在を確かめてから発行する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			productID := uuidtestkit.NewTestFromSalt(t, "bulk_issue_product")

			params := newBulkIssueParams(t)
			params.ScopeKind = domaincoupon.ScopeKindProduct.Name()
			params.ScopeTargetID = &productID

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.productRepo.EXPECT().FindByID(gomock.Any(), productID).Return(nil, nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().CountByActive(gomock.Any(), ptr.To(true)).Return(int64(1), nil)
			deps.bulkIssueCmd.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any()).
				Return(command.IssuePromotionalCouponsResult{RecipientCount: 1, IssuedCouponCount: 1}, nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.NoError(t, err)
		})

		t.Run("定額の値引きも発行できる", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			params := newBulkIssueParams(t)
			params.DiscountKind = domaincoupon.DiscountKindFlat.Name()
			params.DiscountValue = newDecimal(t, "500")

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().CountByActive(gomock.Any(), ptr.To(true)).Return(int64(1), nil)
			deps.bulkIssueCmd.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, p command.IssuePromotionalCouponsParams) (command.IssuePromotionalCouponsResult, error) {
					assert.Equal(t, domaincoupon.DiscountKindFlat, p.Discount.Kind())

					return command.IssuePromotionalCouponsResult{RecipientCount: 1, IssuedCouponCount: 1}, nil
				})

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認可に失敗した場合、書き込みを行わずエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionCouponBulkIssue, gomock.Any()).
				Return(authz.ErrForbidden)
			// 値引きの組み立て・トランザクション・発行のいずれも行わないことを、EXPECT を置かないことで表す。

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, newBulkIssueParams(t))

			require.ErrorIs(t, err, authz.ErrForbidden)
		})

		t.Run("未知の値引き種別を渡した場合、検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			params := newBulkIssueParams(t)
			params.DiscountKind = "unknown"

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidDiscountKind)
		})

		t.Run("値引きの値がドメインの範囲外の場合、検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			params := newBulkIssueParams(t)
			params.DiscountValue = newDecimal(t, "2")

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidDiscountValue)
		})

		t.Run("未知の適用範囲種別を渡した場合、検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			params := newBulkIssueParams(t)
			params.ScopeKind = "unknown"

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidScopeKind)
		})

		t.Run("全体の適用範囲に対象を渡した場合、検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			targetID := uuidtestkit.NewTestFromSalt(t, "bulk_issue_unexpected_target")

			params := newBulkIssueParams(t)
			params.ScopeTargetID = &targetID

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidScopeTarget)
		})

		t.Run("カテゴリの適用範囲に対象を渡さない場合、検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			params := newBulkIssueParams(t)
			params.ScopeKind = domaincoupon.ScopeKindCategory.Name()

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidScopeTarget)
		})

		t.Run("商品の適用範囲に対象を渡さない場合、検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			params := newBulkIssueParams(t)
			params.ScopeKind = domaincoupon.ScopeKindProduct.Name()

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidScopeTarget)
		})

		t.Run("有効期限が発行時点より後でない場合、書き込みを行わず検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			params := newBulkIssueParams(t)
			params.ExpiresAt = testNow

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			// 受給者が 0 人なら coupon.New に届かないため、母集団の大きさによらずここで弾く。

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidExpiresAt)
		})

		t.Run("適用範囲が指すカテゴリが存在しない場合、書き込みを行わずエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			categoryID := uuidtestkit.NewTestFromSalt(t, "bulk_issue_missing_category")

			params := newBulkIssueParams(t)
			params.ScopeKind = domaincoupon.ScopeKindCategory.Name()
			params.ScopeTargetID = &categoryID

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.categoryRepo.EXPECT().FindByID(gomock.Any(), categoryID).Return(nil, apperror.ErrNotFound)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("適用範囲が指す商品が存在しない場合、書き込みを行わずエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			productID := uuidtestkit.NewTestFromSalt(t, "bulk_issue_missing_product")

			params := newBulkIssueParams(t)
			params.ScopeKind = domaincoupon.ScopeKindProduct.Name()
			params.ScopeTargetID = &productID

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.productRepo.EXPECT().FindByID(gomock.Any(), productID).Return(nil, apperror.ErrNotFound)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("受給対象が上限を超える場合、発行せず衝突を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().
				CountByActive(gomock.Any(), ptr.To(true)).
				Return(maxPromotionRecipients+1, nil)
			// 発行を行わないことを、EXPECT を置かないことで表す。

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, newBulkIssueParams(t))

			require.ErrorIs(t, err, ErrTooManyRecipients)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("事前判定を通っても実際に書いた枚数が上限を超えた場合、衝突を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().
				CountByActive(gomock.Any(), ptr.To(true)).
				Return(maxPromotionRecipients, nil)
			deps.bulkIssueCmd.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any()).
				Return(command.IssuePromotionalCouponsResult{
					RecipientCount:    maxPromotionRecipients + 1,
					IssuedCouponCount: maxPromotionRecipients + 1,
				}, nil)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, newBulkIssueParams(t))

			require.ErrorIs(t, err, ErrTooManyRecipients)
		})

		t.Run("受給者数の取得に失敗した場合、発行せずエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().
				CountByActive(gomock.Any(), ptr.To(true)).
				Return(int64(0), apperror.ErrInternal)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, newBulkIssueParams(t))

			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("一括発行に失敗した場合、エラーをそのまま返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runBulkIssueInTx)
			deps.userRepo.EXPECT().CountByActive(gomock.Any(), ptr.To(true)).Return(int64(1), nil)
			deps.bulkIssueCmd.EXPECT().
				IssuePromotionalCoupons(gomock.Any(), gomock.Any()).
				Return(command.IssuePromotionalCouponsResult{}, apperror.ErrInternal)

			_, err := u.IssuePromotionalCoupons(t.Context(), &auth.Authn{}, newBulkIssueParams(t))

			require.ErrorIs(t, err, apperror.ErrInternal)
		})
	})
}

func Test_newScope(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体は対象を持たない適用範囲になる", func(t *testing.T) {
			t.Parallel()

			scope, err := newScope(domaincoupon.ScopeKindAll.Name(), nil)

			require.NoError(t, err)
			assert.Equal(t, domaincoupon.ScopeKindAll, scope.Kind())
			assert.Nil(t, scope.TargetID())
		})

		t.Run("カテゴリは対象 ID を保持する", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "new_scope_category")

			scope, err := newScope(domaincoupon.ScopeKindCategory.Name(), &id)

			require.NoError(t, err)
			assert.Equal(t, domaincoupon.ScopeKindCategory, scope.Kind())
			assert.Equal(t, id, *scope.TargetID())
		})

		t.Run("商品は対象 ID を保持する", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "new_scope_product")

			scope, err := newScope(domaincoupon.ScopeKindProduct.Name(), &id)

			require.NoError(t, err)
			assert.Equal(t, domaincoupon.ScopeKindProduct, scope.Kind())
			assert.Equal(t, id, *scope.TargetID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未設定の対象 ID を渡したカテゴリは検証エラーになる", func(t *testing.T) {
			t.Parallel()

			var nilID uuid.UUID

			_, err := newScope(domaincoupon.ScopeKindCategory.Name(), &nilID)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidScopeTarget)
		})

		t.Run("未知の名前は検証エラーになる", func(t *testing.T) {
			t.Parallel()

			_, err := newScope("unknown", nil)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidScopeKind)
		})
	})
}

func Test_newDiscount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("定額は金額をそのまま保持する", func(t *testing.T) {
			t.Parallel()

			discount, err := newDiscount(domaincoupon.DiscountKindFlat.Name(), newDecimal(t, "500"))

			require.NoError(t, err)
			assert.Equal(t, domaincoupon.DiscountKindFlat, discount.Kind())
			assert.Equal(t, "500", discount.Value().String())
		})

		t.Run("定率は率をそのまま保持する", func(t *testing.T) {
			t.Parallel()

			discount, err := newDiscount(domaincoupon.DiscountKindRate.Name(), newDecimal(t, "0.15"))

			require.NoError(t, err)
			assert.Equal(t, domaincoupon.DiscountKindRate, discount.Kind())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未知の名前は検証エラーになる", func(t *testing.T) {
			t.Parallel()

			_, err := newDiscount("unknown", newDecimal(t, "1"))

			require.ErrorIs(t, err, domaincoupon.ErrInvalidDiscountKind)
		})

		t.Run("定額に 0 以下を渡すと検証エラーになる", func(t *testing.T) {
			t.Parallel()

			_, err := newDiscount(domaincoupon.DiscountKindFlat.Name(), newDecimal(t, "0"))

			require.ErrorIs(t, err, domaincoupon.ErrInvalidDiscountValue)
		})
	})
}

func Test_usecase_ensureScopeTargetExists(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体の適用範囲は対象を持たないため何も確認しない", func(t *testing.T) {
			t.Parallel()

			u, _ := newTestUsecase(t)
			// カテゴリ・商品のいずれも引かないことを、EXPECT を置かないことで表す。

			require.NoError(t, u.ensureScopeTargetExists(t.Context(), domaincoupon.NewAllScope()))
		})

		t.Run("カテゴリの適用範囲はカテゴリを引く", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			id := uuidtestkit.NewTestFromSalt(t, "ensure_scope_category")
			scope, err := domaincoupon.NewCategoryScope(id)
			require.NoError(t, err)

			deps.categoryRepo.EXPECT().FindByID(gomock.Any(), id).Return(nil, nil)

			require.NoError(t, u.ensureScopeTargetExists(t.Context(), scope))
		})

		t.Run("商品の適用範囲は商品を引く", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			id := uuidtestkit.NewTestFromSalt(t, "ensure_scope_product")
			scope, err := domaincoupon.NewProductScope(id)
			require.NoError(t, err)

			deps.productRepo.EXPECT().FindByID(gomock.Any(), id).Return(nil, nil)

			require.NoError(t, u.ensureScopeTargetExists(t.Context(), scope))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象が存在しない場合、取得のエラーをそのまま返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			id := uuidtestkit.NewTestFromSalt(t, "ensure_scope_missing")
			scope, err := domaincoupon.NewCategoryScope(id)
			require.NoError(t, err)

			deps.categoryRepo.EXPECT().FindByID(gomock.Any(), id).Return(nil, apperror.ErrNotFound)

			require.ErrorIs(t, u.ensureScopeTargetExists(t.Context(), scope), apperror.ErrNotFound)
		})
	})
}
