package campaign

import (
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincampaign "go-boilerplate/internal/domain/campaign"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// expectClaimFlow は、受け取りが成立するまでの依存の呼び出しを揃えます。
// claimed は、その利用者が既に受け取っている枚数です。
func expectClaimFlow(t *testing.T, deps *testDeps, target *domaincampaign.Campaign, claimed int) {
	t.Helper()

	deps.clock.EXPECT().Now().Return(testNow)
	deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
	deps.campaignRepo.EXPECT().LockByCode(gomock.Any(), target.Code()).Return(target, nil)
	deps.campaignRepo.EXPECT().CountClaims(gomock.Any(), gomock.Any()).Return(claimed, nil)
}

func Test_usecase_ClaimCoupon(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("コードと引き換えに未使用のクーポンを1枚発行する", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, userID := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			expectClaimFlow(t, deps, target, 0)
			deps.couponRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, c *domaincoupon.Coupon) error {
					assert.Equal(t, userID, c.UserID())
					assert.False(t, c.IsUsed())
					assert.Equal(t, testCouponExpiresAt, c.ExpiresAt())

					return nil
				})
			deps.campaignRepo.EXPECT().
				RecordClaim(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, record *domaincampaign.Claim) error {
					assert.Equal(t, target.ID(), record.CampaignID())
					assert.Equal(t, userID, record.UserID())
					assert.Equal(t, testNow, record.ClaimedAt())

					return nil
				})

			got, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.NoError(t, err)
			assert.Equal(t, "rate", got.DiscountKind)
			assert.Nil(t, got.UsedAt)
			assert.Equal(t, testNow, got.IssuedAt)
		})

		t.Run("受け取り記録のクーポンIDは発行したクーポンを指す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			var issuedID uuid.UUID
			expectClaimFlow(t, deps, target, 0)
			deps.couponRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, c *domaincoupon.Coupon) error {
					issuedID = c.ID()

					return nil
				})
			deps.campaignRepo.EXPECT().
				RecordClaim(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, record *domaincampaign.Claim) error {
					assert.Equal(t, issuedID, record.CouponID())

					return nil
				})

			got, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.NoError(t, err)
			assert.Equal(t, issuedID, got.ID)
		})

		t.Run("コードの大文字小文字と前後の空白は同じコードとして扱う", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			expectClaimFlow(t, deps, target, 0)
			deps.couponRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			deps.campaignRepo.EXPECT().RecordClaim(gomock.Any(), gomock.Any()).Return(nil)

			_, err := u.ClaimCoupon(t.Context(), authn, "  welcome-2026 ")

			require.NoError(t, err)
		})

		t.Run("テンプレートの条件を配るクーポンへ焼く", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)

			minPurchase := int64(5000)
			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			code, err := domaincampaign.NewCode("WELCOME-2026")
			require.NoError(t, err)
			template, err := domaincampaign.NewTemplate(domaincampaign.TemplateAttributes{
				DiscountKindName:  "rate",
				DiscountValue:     newDecimal(t, "0.10"),
				DiscountMaxAmount: &minPurchase,
				ScopeKindName:     "all",
				MinPurchaseAmount: &minPurchase,
				UsableFrom:        &usableFrom,
				ExpiresAt:         testCouponExpiresAt,
			})
			require.NoError(t, err)
			target, err := domaincampaign.New(uuidtestkit.NewTestFromSalt(t, "conditioned_campaign"), domaincampaign.Attributes{
				Code:         code,
				Template:     template,
				StartsAt:     testStartsAt,
				EndsAt:       testEndsAt,
				TotalLimit:   10,
				PerUserLimit: 2,
			})
			require.NoError(t, err)

			expectClaimFlow(t, deps, target, 0)
			deps.couponRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ any, c *domaincoupon.Coupon) error {
					require.NotNil(t, c.MinPurchaseAmount())
					assert.Equal(t, minPurchase, *c.MinPurchaseAmount())
					require.NotNil(t, c.UsableFrom())
					assert.Equal(t, usableFrom, *c.UsableFrom())
					assert.NotNil(t, c.Discount().MaxAmount())

					return nil
				})
			deps.campaignRepo.EXPECT().RecordClaim(gomock.Any(), gomock.Any()).Return(nil)

			_, err = u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("存在しないコードの場合、NotFoundではなく検証エラーへ畳む", func(t *testing.T) {
			t.Parallel()

			// 404 を返すと、有効なコードを言い当てる手がかりになる。
			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)

			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().
				LockByCode(gomock.Any(), gomock.Any()).
				Return(nil, apperror.ErrNotFound)

			_, err := u.ClaimCoupon(t.Context(), authn, "NOSUCHCODE-0001")

			require.ErrorIs(t, err, domaincampaign.ErrCodeUnknown)
			require.ErrorIs(t, err, apperror.ErrValidation)
			require.NotErrorIs(t, err, apperror.ErrNotFound)
		})

		t.Run("形式が不正なコードの場合、存在しないコードと同じエラーを返す", func(t *testing.T) {
			t.Parallel()

			// 形式の当たりだけを先に絞り込めないようにする。永続化まで到達しない。
			u, _ := newTestUsecase(t)
			authn, _ := newTestAuthn(t)

			_, err := u.ClaimCoupon(t.Context(), authn, "SHORT")

			require.ErrorIs(t, err, domaincampaign.ErrCodeUnknown)
		})

		t.Run("配布期間の外の場合、検証エラーを返し発行しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			deps.clock.EXPECT().Now().Return(testEndsAt)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().LockByCode(gomock.Any(), target.Code()).Return(target, nil)
			deps.campaignRepo.EXPECT().CountClaims(gomock.Any(), gomock.Any()).Return(0, nil)

			_, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, domaincampaign.ErrNotDistributing)
			require.ErrorIs(t, err, apperror.ErrValidation)
		})

		t.Run("停止済みの場合、検証エラーを返し発行しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)
			require.NoError(t, target.Suspend(testStartsAt))

			expectClaimFlow(t, deps, target, 0)

			_, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, domaincampaign.ErrSuspended)
			require.ErrorIs(t, err, apperror.ErrValidation)
		})

		t.Run("総枚数上限に達している場合、検証エラーを返し発行しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			base := newTestCampaign(t, 1, 1)
			target, err := domaincampaign.Reconstruct(base.ID(), domaincampaign.Attributes{
				Code:         base.Code(),
				Template:     base.Template(),
				StartsAt:     base.StartsAt(),
				EndsAt:       base.EndsAt(),
				TotalLimit:   base.TotalLimit(),
				PerUserLimit: base.PerUserLimit(),
			}, 1, nil)
			require.NoError(t, err)

			expectClaimFlow(t, deps, target, 0)

			_, err = u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, domaincampaign.ErrTotalLimitReached)
			require.ErrorIs(t, err, apperror.ErrValidation)
		})

		t.Run("1人あたり上限に達している場合、検証エラーを返し発行しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			expectClaimFlow(t, deps, target, target.PerUserLimit())

			_, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, domaincampaign.ErrPerUserLimitReached)
			require.ErrorIs(t, err, apperror.ErrValidation)
		})

		// 応答で区別しないという決定を、エラーの族で固定する。ここが崩れると、
		// どれか 1 つだけが 404 や 409 へ漏れて有効なコードの手がかりになる。
		t.Run("存在しないコード は検証エラーの族に収まり、NotFoundにもConflictにもならない", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, domaincampaign.ErrCodeUnknown, apperror.ErrValidation)
			require.NotErrorIs(t, domaincampaign.ErrCodeUnknown, apperror.ErrNotFound)
			require.NotErrorIs(t, domaincampaign.ErrCodeUnknown, apperror.ErrConflict)
		})

		t.Run("配布期間の外 は検証エラーの族に収まり、NotFoundにもConflictにもならない", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, domaincampaign.ErrNotDistributing, apperror.ErrValidation)
			require.NotErrorIs(t, domaincampaign.ErrNotDistributing, apperror.ErrNotFound)
			require.NotErrorIs(t, domaincampaign.ErrNotDistributing, apperror.ErrConflict)
		})

		t.Run("停止済み は検証エラーの族に収まり、NotFoundにもConflictにもならない", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, domaincampaign.ErrSuspended, apperror.ErrValidation)
			require.NotErrorIs(t, domaincampaign.ErrSuspended, apperror.ErrNotFound)
			require.NotErrorIs(t, domaincampaign.ErrSuspended, apperror.ErrConflict)
		})

		t.Run("総枚数上限の到達 は検証エラーの族に収まり、NotFoundにもConflictにもならない", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, domaincampaign.ErrTotalLimitReached, apperror.ErrValidation)
			require.NotErrorIs(t, domaincampaign.ErrTotalLimitReached, apperror.ErrNotFound)
			require.NotErrorIs(t, domaincampaign.ErrTotalLimitReached, apperror.ErrConflict)
		})

		t.Run("1人あたり上限の到達 は検証エラーの族に収まり、NotFoundにもConflictにもならない", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, domaincampaign.ErrPerUserLimitReached, apperror.ErrValidation)
			require.NotErrorIs(t, domaincampaign.ErrPerUserLimitReached, apperror.ErrNotFound)
			require.NotErrorIs(t, domaincampaign.ErrPerUserLimitReached, apperror.ErrConflict)
		})

		t.Run("認証主体が解決されていない場合、永続化へ進まない", func(t *testing.T) {
			t.Parallel()

			u, _ := newTestUsecase(t)

			_, err := u.ClaimCoupon(t.Context(), &auth.Authn{}, "WELCOME-2026")

			require.Error(t, err)
		})

		t.Run("キャンペーンの取得がNotFound以外で失敗した場合、そのまま返す", func(t *testing.T) {
			t.Parallel()

			// NotFound だけを畳み替える。インフラ障害を 422 に化けさせない。
			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)

			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().
				LockByCode(gomock.Any(), gomock.Any()).
				Return(nil, apperror.ErrInternal)

			_, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, apperror.ErrInternal)
			require.NotErrorIs(t, err, domaincampaign.ErrCodeUnknown)
		})

		t.Run("受け取り枚数の集計が失敗した場合、そのまま返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			deps.clock.EXPECT().Now().Return(testNow)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.campaignRepo.EXPECT().LockByCode(gomock.Any(), target.Code()).Return(target, nil)
			deps.campaignRepo.EXPECT().
				CountClaims(gomock.Any(), gomock.Any()).
				Return(0, apperror.ErrInternal)

			_, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("クーポンの保存が失敗した場合、そのまま返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			expectClaimFlow(t, deps, target, 0)
			deps.couponRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(apperror.ErrInternal)

			_, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("受け取り記録の保存が競合した場合、Conflictを返す", func(t *testing.T) {
			t.Parallel()

			// 行ロックを取らずに呼ばれた場合に備える二重防御が、上位まで伝わることを固定する。
			u, deps := newTestUsecase(t)
			authn, _ := newTestAuthn(t)
			target := newTestCampaign(t, 10, 2)

			expectClaimFlow(t, deps, target, 0)
			deps.couponRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			deps.campaignRepo.EXPECT().
				RecordClaim(gomock.Any(), gomock.Any()).
				Return(domaincampaign.ErrIssuedConcurrently)

			_, err := u.ClaimCoupon(t.Context(), authn, "WELCOME-2026")

			require.ErrorIs(t, err, apperror.ErrConflict)
		})
	})
}

func Test_unclaimable(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取れない理由をコードのフィールドへ畳む", func(t *testing.T) {
			t.Parallel()

			got := unclaimable(domaincampaign.ErrNotDistributing)

			meta, ok := apperror.MetaFrom(got)
			require.True(t, ok)
			assert.Equal(t, []string{domaincampaign.FieldCode}, meta.Details)
		})

		t.Run("元のエラーは失われない", func(t *testing.T) {
			t.Parallel()

			// 応答では畳むが、理由はログとメトリクスから追える必要がある。
			got := unclaimable(domaincampaign.ErrTotalLimitReached)

			require.ErrorIs(t, got, domaincampaign.ErrTotalLimitReached)
			require.ErrorIs(t, got, apperror.ErrValidation)
		})
	})
}
