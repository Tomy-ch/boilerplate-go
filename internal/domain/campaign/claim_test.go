package campaign

import (
	"testing"
	"time"

	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validClaimAttrs(t *testing.T) ClaimAttributes {
	t.Helper()

	return ClaimAttributes{
		CampaignID: newTestUUID(t),
		UserID:     newTestUUID(t),
		CouponID:   newTestUUID(t),
		ClaimedAt:  testStartsAt.Add(time.Hour),
	}
}

func Test_newClaim(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全属性が有効な場合、受け取り記録を生成する", func(t *testing.T) {
			t.Parallel()

			id := newTestUUID(t)
			attrs := validClaimAttrs(t)

			got, err := newClaim(id, attrs)

			require.NoError(t, err)
			assert.Equal(t, id, got.ID())
			assert.Equal(t, attrs.CampaignID, got.CampaignID())
			assert.Equal(t, attrs.UserID, got.UserID())
			assert.Equal(t, attrs.CouponID, got.CouponID())
			assert.Equal(t, attrs.ClaimedAt, got.ClaimedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDが未設定の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()

			_, err := newClaim(uuid.UUID{}, validClaimAttrs(t))

			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("キャンペーンIDが未設定の場合、ErrInvalidCampaignIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			attrs.CampaignID = uuid.UUID{}

			_, err := newClaim(newTestUUID(t), attrs)

			require.ErrorIs(t, err, ErrInvalidCampaignID)
		})

		t.Run("ユーザーIDが未設定の場合、ErrInvalidUserIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			attrs.UserID = uuid.UUID{}

			_, err := newClaim(newTestUUID(t), attrs)

			require.ErrorIs(t, err, ErrInvalidUserID)
		})

		t.Run("クーポンIDが未設定の場合、ErrInvalidCouponIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			attrs.CouponID = uuid.UUID{}

			_, err := newClaim(newTestUUID(t), attrs)

			require.ErrorIs(t, err, ErrInvalidCouponID)
		})

		t.Run("受け取り日時がゼロ値の場合、ErrInvalidClaimedAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			attrs.ClaimedAt = time.Time{}

			_, err := newClaim(newTestUUID(t), attrs)

			require.ErrorIs(t, err, ErrInvalidClaimedAt)
		})
	})
}

func TestClaim_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時に渡したIDを返す", func(t *testing.T) {
			t.Parallel()

			id := newTestUUID(t)
			c, err := newClaim(id, validClaimAttrs(t))
			require.NoError(t, err)

			assert.Equal(t, id, c.ID())
		})
	})
}

func TestClaim_CampaignID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取り元のキャンペーンIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			c, err := newClaim(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, attrs.CampaignID, c.CampaignID())
		})
	})
}

func TestClaim_UserID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取った利用者のユーザーIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			c, err := newClaim(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, attrs.UserID, c.UserID())
		})
	})
}

func TestClaim_CouponID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取りで発行されたクーポンのIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			c, err := newClaim(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, attrs.CouponID, c.CouponID())
		})
	})
}

func TestClaim_ClaimedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取り日時を返す", func(t *testing.T) {
			t.Parallel()

			attrs := validClaimAttrs(t)
			c, err := newClaim(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, attrs.ClaimedAt, c.ClaimedAt())
		})
	})
}
