package campaign

import (
	"testing"
	"time"

	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTemplate(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全属性が有効な場合、テンプレートを生成する", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)

			got, err := NewTemplate(attrs)

			require.NoError(t, err)
			assert.Equal(t, attrs.DiscountKindName, got.DiscountKindName())
			assert.Equal(t, attrs.ScopeKindName, got.ScopeKindName())
			assert.Equal(t, attrs.ExpiresAt, got.ExpiresAt())
			assert.Nil(t, got.DiscountMaxAmount())
			assert.Nil(t, got.MinPurchaseAmount())
			assert.Nil(t, got.UsableFrom())
		})

		t.Run("任意項目を渡した場合、そのまま保つ", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			minPurchase := int64(5000)
			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			targetID := newTestUUID(t)

			attrs := validTemplateAttrs(t)
			attrs.DiscountMaxAmount = &maxAmount
			attrs.MinPurchaseAmount = &minPurchase
			attrs.UsableFrom = &usableFrom
			attrs.ScopeTargetID = &targetID

			got, err := NewTemplate(attrs)

			require.NoError(t, err)
			require.NotNil(t, got.DiscountMaxAmount())
			assert.Equal(t, maxAmount, *got.DiscountMaxAmount())
			require.NotNil(t, got.MinPurchaseAmount())
			assert.Equal(t, minPurchase, *got.MinPurchaseAmount())
			require.NotNil(t, got.UsableFrom())
			assert.Equal(t, usableFrom, *got.UsableFrom())
			require.NotNil(t, got.ScopeTargetID())
			assert.Equal(t, targetID, *got.ScopeTargetID())
		})

		t.Run("既知でない種別の名前もここでは通す", func(t *testing.T) {
			t.Parallel()

			// 閉じた集合を所有するのはクーポン集約であり、キャンペーンは名前を保つだけである。
			// 解決は usecase 層が行う。
			attrs := validTemplateAttrs(t)
			attrs.DiscountKindName = "unknown"

			_, err := NewTemplate(attrs)

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引き種別の名前が空の場合、ErrInvalidDiscountKindを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)
			attrs.DiscountKindName = ""

			_, err := NewTemplate(attrs)

			require.ErrorIs(t, err, ErrInvalidDiscountKind)
		})

		t.Run("値引きの値が0の場合、ErrInvalidDiscountValueを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)
			attrs.DiscountValue = newTestDecimal(t, "0")

			_, err := NewTemplate(attrs)

			require.ErrorIs(t, err, ErrInvalidDiscountValue)
		})

		t.Run("値引き上限が0の場合、ErrInvalidMaxAmountを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)
			zero := int64(0)
			attrs.DiscountMaxAmount = &zero

			_, err := NewTemplate(attrs)

			require.ErrorIs(t, err, ErrInvalidMaxAmount)
		})

		t.Run("適用範囲種別の名前が空の場合、ErrInvalidScopeKindを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)
			attrs.ScopeKindName = ""

			_, err := NewTemplate(attrs)

			require.ErrorIs(t, err, ErrInvalidScopeKind)
		})

		t.Run("最低購入金額が0の場合、ErrInvalidMinPurchaseAmountを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)
			zero := int64(0)
			attrs.MinPurchaseAmount = &zero

			_, err := NewTemplate(attrs)

			require.ErrorIs(t, err, ErrInvalidMinPurchaseAmount)
		})

		t.Run("有効期限がゼロ値の場合、ErrInvalidCouponExpiresAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)
			attrs.ExpiresAt = time.Time{}

			_, err := NewTemplate(attrs)

			require.ErrorIs(t, err, ErrInvalidCouponExpiresAt)
		})

		t.Run("利用開始日時が有効期限ちょうどの場合、ErrInvalidUsableFromを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validTemplateAttrs(t)
			from := attrs.ExpiresAt
			attrs.UsableFrom = &from

			_, err := NewTemplate(attrs)

			require.ErrorIs(t, err, ErrInvalidUsableFrom)
		})
	})
}

func TestTemplate_IsZero(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成を経ていないゼロ値の場合はtrueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, Template{}.IsZero())
		})

		t.Run("生成したテンプレートの場合はfalseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, newTestTemplate(t).IsZero())
		})
	})
}

func TestTemplate_DiscountKindName(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引き種別の名前を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "rate", newTestTemplate(t).DiscountKindName())
		})
	})
}

func TestTemplate_DiscountValue(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("種別における値を返す", func(t *testing.T) {
			t.Parallel()

			assert.Zero(t, newTestTemplate(t).DiscountValue().Cmp(newTestDecimal(t, "0.10")))
		})
	})
}

func TestTemplate_ScopeKindName(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("適用範囲種別の名前を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "all", newTestTemplate(t).ScopeKindName())
		})
	})
}

func TestTemplate_ExpiresAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("配るクーポンの有効期限を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCouponExpiresAt, newTestTemplate(t).ExpiresAt())
		})
	})
}

func TestTemplate_DiscountMaxAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("返した値を書き換えてもテンプレートは変わらない", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			attrs := validTemplateAttrs(t)
			attrs.DiscountMaxAmount = &maxAmount
			tmpl, err := NewTemplate(attrs)
			require.NoError(t, err)

			got := tmpl.DiscountMaxAmount()
			require.NotNil(t, got)
			*got = 1

			require.NotNil(t, tmpl.DiscountMaxAmount())
			assert.Equal(t, int64(2000), *tmpl.DiscountMaxAmount())
		})
	})
}

func TestTemplate_MinPurchaseAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件を持たない場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, newTestTemplate(t).MinPurchaseAmount())
		})

		t.Run("返した値を書き換えてもテンプレートは変わらない", func(t *testing.T) {
			t.Parallel()

			minPurchase := int64(5000)
			attrs := validTemplateAttrs(t)
			attrs.MinPurchaseAmount = &minPurchase
			tmpl, err := NewTemplate(attrs)
			require.NoError(t, err)

			got := tmpl.MinPurchaseAmount()
			require.NotNil(t, got)
			*got = 1

			require.NotNil(t, tmpl.MinPurchaseAmount())
			assert.Equal(t, int64(5000), *tmpl.MinPurchaseAmount())
		})
	})
}

func TestTemplate_UsableFrom(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件を持たない場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, newTestTemplate(t).UsableFrom())
		})

		t.Run("返した値を書き換えてもテンプレートは変わらない", func(t *testing.T) {
			t.Parallel()

			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			attrs := validTemplateAttrs(t)
			attrs.UsableFrom = &usableFrom
			tmpl, err := NewTemplate(attrs)
			require.NoError(t, err)

			got := tmpl.UsableFrom()
			require.NotNil(t, got)
			*got = testCouponExpiresAt

			require.NotNil(t, tmpl.UsableFrom())
			assert.Equal(t, usableFrom, *tmpl.UsableFrom())
		})
	})
}

func TestTemplate_ScopeTargetID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体を対象にする場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, newTestTemplate(t).ScopeTargetID())
		})

		t.Run("返した値を書き換えてもテンプレートは変わらない", func(t *testing.T) {
			t.Parallel()

			targetID := newTestUUID(t)
			attrs := validTemplateAttrs(t)
			attrs.ScopeTargetID = &targetID
			tmpl, err := NewTemplate(attrs)
			require.NoError(t, err)

			got := tmpl.ScopeTargetID()
			require.NotNil(t, got)
			*got = uuid.UUID{}

			require.NotNil(t, tmpl.ScopeTargetID())
			assert.Equal(t, targetID, *tmpl.ScopeTargetID())
		})
	})
}
