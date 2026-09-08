package coupon

import (
	"context"
	"testing"

	"go-boilerplate/internal/apperror"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/domain/user"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func runIssueInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// newTestRecipient は、受給者の在籍確認が返す利用者を組み立てます。
func newTestRecipient(t *testing.T, id uuid.UUID) *user.User {
	t.Helper()

	u, err := user.New(id, user.Attributes{
		FirstName:    "John",
		LastName:     "Doe",
		Email:        "john@example.com",
		Phone:        "1234567890",
		PrefectureID: uuidtestkit.NewTestFromSalt(t, "issue_prefecture"),
		City:         "Shibuya",
		Street:       "1-2-3",
		PostalCode:   "150-0001",
		CreatedAt:    testIssuedAt,
		UpdatedAt:    testIssuedAt,
	})
	require.NoError(t, err)

	return u
}

func newIssueParams(t *testing.T, recipientID uuid.UUID) IssueCouponParams {
	t.Helper()

	return IssueCouponParams{
		UserID:        recipientID,
		DiscountKind:  domaincoupon.DiscountKindFlat.Name(),
		DiscountValue: newDecimal(t, "500"),
		ScopeKind:     domaincoupon.ScopeKindAll.Name(),
		ExpiresAt:     testExpiresAt,
	}
}

func Test_usecase_IssueCoupon(t *testing.T) {
	t.Parallel()

	recipientID := uuidtestkit.NewTestFromSalt(t, "issue_recipient")

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("名指しした受給者へ未使用のクーポンを 1 枚発行する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionCouponIssue, gomock.Any()).
				Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runIssueInTx)
			deps.userRepo.EXPECT().FindByID(gomock.Any(), recipientID).Return(newTestRecipient(t, recipientID), nil)
			deps.couponRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, c *domaincoupon.Coupon) error {
					assert.Equal(t, recipientID, c.UserID())
					assert.Equal(t, domaincoupon.DiscountKindFlat, c.Discount().Kind())
					assert.Equal(t, domaincoupon.ScopeKindAll, c.Scope().Kind())
					assert.Nil(t, c.Scope().TargetID())
					assert.Equal(t, testExpiresAt, c.ExpiresAt())
					assert.Equal(t, testNow, c.IssuedAt())
					assert.Nil(t, c.UsedAt())

					return nil
				})

			view, err := u.IssueCoupon(t.Context(), &auth.Authn{}, newIssueParams(t, recipientID))

			require.NoError(t, err)
			assert.Equal(t, domaincoupon.DiscountKindFlat.Name(), view.DiscountKind)
			assert.Equal(t, domaincoupon.ScopeKindAll.Name(), view.ScopeKind)
			assert.Equal(t, testExpiresAt, view.ExpiresAt)
			assert.Equal(t, testNow, view.IssuedAt)
			assert.Nil(t, view.UsedAt)
			assert.False(t, view.ID.IsNil())
		})

		t.Run("適用範囲が商品限定のときは対象の存在を確かめてから発行する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			targetID := uuidtestkit.NewTestFromSalt(t, "issue_target_product")

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runIssueInTx)
			deps.productRepo.EXPECT().FindByID(gomock.Any(), targetID).Return(newTestProductEntity(t, targetID), nil)
			deps.userRepo.EXPECT().FindByID(gomock.Any(), recipientID).Return(newTestRecipient(t, recipientID), nil)
			deps.couponRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, c *domaincoupon.Coupon) error {
					assert.Equal(t, domaincoupon.ScopeKindProduct, c.Scope().Kind())
					require.NotNil(t, c.Scope().TargetID())
					assert.Equal(t, targetID, *c.Scope().TargetID())

					return nil
				})

			params := newIssueParams(t, recipientID)
			params.DiscountKind = domaincoupon.DiscountKindRate.Name()
			params.DiscountValue = newDecimal(t, "0.15")
			params.ScopeKind = domaincoupon.ScopeKindProduct.Name()
			params.ScopeTargetID = &targetID

			view, err := u.IssueCoupon(t.Context(), &auth.Authn{}, params)

			require.NoError(t, err)
			assert.Equal(t, domaincoupon.ScopeKindProduct.Name(), view.ScopeKind)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認可されない場合は発行せずエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(apperror.ErrPermissionDenied)

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, newIssueParams(t, recipientID))

			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})

		t.Run("値引きの種別が未知の場合は検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			params := newIssueParams(t, recipientID)
			params.DiscountKind = "unknown"

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidDiscountKind)
		})

		t.Run("適用範囲が全体なのに対象を指定した場合は検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			targetID := uuidtestkit.NewTestFromSalt(t, "issue_unexpected_target")
			params := newIssueParams(t, recipientID)
			params.ScopeTargetID = &targetID

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidScopeTarget)
		})

		t.Run("適用範囲の対象が存在しない場合は発行せずエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			targetID := uuidtestkit.NewTestFromSalt(t, "issue_missing_category")

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runIssueInTx)
			deps.categoryRepo.EXPECT().FindByID(gomock.Any(), targetID).Return(nil, apperror.ErrNotFound)

			params := newIssueParams(t, recipientID)
			params.ScopeKind = domaincoupon.ScopeKindCategory.Name()
			params.ScopeTargetID = &targetID

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("受給者が在籍しない場合は発行せずエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runIssueInTx)
			deps.userRepo.EXPECT().FindByID(gomock.Any(), recipientID).Return(nil, apperror.ErrNotFound)

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, newIssueParams(t, recipientID))

			require.ErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("有効期限が発行日時より後でない場合は検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runIssueInTx)
			deps.userRepo.EXPECT().FindByID(gomock.Any(), recipientID).Return(newTestRecipient(t, recipientID), nil)

			params := newIssueParams(t, recipientID)
			params.ExpiresAt = testIssuedAt

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, params)

			require.ErrorIs(t, err, domaincoupon.ErrInvalidExpiresAt)
		})

		t.Run("永続化に失敗した場合はそのエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			wantErr := xerrors.New("create failed")

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runIssueInTx)
			deps.userRepo.EXPECT().FindByID(gomock.Any(), recipientID).Return(newTestRecipient(t, recipientID), nil)
			deps.couponRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(wantErr)

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, newIssueParams(t, recipientID))

			require.ErrorIs(t, err, wantErr)
		})

		t.Run("トランザクションが失敗した場合はそのエラーを返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			wantErr := xerrors.New("tx failed")

			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).Return(wantErr)

			_, err := u.IssueCoupon(t.Context(), &auth.Authn{}, newIssueParams(t, recipientID))

			require.ErrorIs(t, err, wantErr)
		})
	})
}
