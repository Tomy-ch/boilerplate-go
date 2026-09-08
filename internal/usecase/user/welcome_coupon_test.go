package user

import (
	"testing"
	"time"

	domaincoupon "go-boilerplate/internal/domain/coupon"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newWelcomeCoupon(t *testing.T) {
	t.Parallel()

	issuedAt := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受給者と発行日時から、全体×定額の未使用クーポンが組み立てられる", func(t *testing.T) {
			t.Parallel()

			userID := uuidtestkit.NewTestFromSalt(t, "welcome_coupon_user")

			actual, err := newWelcomeCoupon(userID, issuedAt)
			require.NoError(t, err)

			expectedAmount, err := decimal.Parse(welcomeCouponAmount)
			require.NoError(t, err)

			assert.Equal(t, userID, actual.UserID())
			assert.Equal(t, domaincoupon.DiscountKindFlat, actual.Discount().Kind())
			assert.True(t, expectedAmount.Equal(actual.Discount().Value()))
			assert.Equal(t, domaincoupon.ScopeKindAll, actual.Scope().Kind())
			assert.Nil(t, actual.Scope().TargetID())
			assert.Equal(t, issuedAt, actual.IssuedAt())
			assert.Equal(t, issuedAt.Add(welcomeCouponValidity), actual.ExpiresAt())
			assert.Nil(t, actual.UsedAt())
		})

		t.Run("呼び出しごとに異なる ID が採番される", func(t *testing.T) {
			t.Parallel()

			userID := uuidtestkit.NewTestFromSalt(t, "welcome_coupon_user_ids")

			first, err := newWelcomeCoupon(userID, issuedAt)
			require.NoError(t, err)
			second, err := newWelcomeCoupon(userID, issuedAt)
			require.NoError(t, err)

			assert.NotEqual(t, first.ID(), second.ID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受給者が未設定の場合、ドメインの検証エラーが返される", func(t *testing.T) {
			t.Parallel()

			actual, err := newWelcomeCoupon(uuid.UUID{}, issuedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, domaincoupon.ErrInvalidUserID)
		})

		t.Run("発行日時がゼロ値の場合、ドメインの検証エラーが返される", func(t *testing.T) {
			t.Parallel()

			userID := uuidtestkit.NewTestFromSalt(t, "welcome_coupon_zero_time")

			actual, err := newWelcomeCoupon(userID, time.Time{})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, domaincoupon.ErrInvalidIssuedAt)
		})
	})
}
