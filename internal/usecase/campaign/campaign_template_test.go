package campaign

import (
	"testing"
	"time"

	domaincampaign "go-boilerplate/internal/domain/campaign"
	"go-boilerplate/internal/domain/coupon"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTemplate は、任意項目だけを差し替えたテンプレートを組み立てます。
func newTemplate(t *testing.T, mutate func(*domaincampaign.TemplateAttributes)) domaincampaign.Template {
	t.Helper()

	attrs := domaincampaign.TemplateAttributes{
		DiscountKindName: "rate",
		DiscountValue:    newDecimal(t, "0.10"),
		ScopeKindName:    "all",
		ExpiresAt:        testCouponExpiresAt,
	}
	if mutate != nil {
		mutate(&attrs)
	}
	tmpl, err := domaincampaign.NewTemplate(attrs)
	require.NoError(t, err)

	return tmpl
}

func Test_resolveTemplate(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("種別の名前をクーポン集約の語彙へ解決する", func(t *testing.T) {
			t.Parallel()

			discount, scope, err := resolveTemplate(newTemplate(t, nil))

			require.NoError(t, err)
			assert.Equal(t, coupon.DiscountKindRate, discount.Kind())
			assert.Equal(t, coupon.ScopeKindAll, scope.Kind())
			assert.Nil(t, discount.MaxAmount())
		})

		t.Run("値引き上限を持つ場合は解決した値引きへ引き継ぐ", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.DiscountMaxAmount = &maxAmount
			})

			discount, _, err := resolveTemplate(tmpl)

			require.NoError(t, err)
			require.NotNil(t, discount.MaxAmount())
			assert.Equal(t, maxAmount, *discount.MaxAmount())
		})

		t.Run("適用範囲が対象を絞る場合は解決した適用範囲へ引き継ぐ", func(t *testing.T) {
			t.Parallel()

			targetID := newTestUUID(t)
			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.ScopeKindName = "category"
				a.ScopeTargetID = &targetID
			})

			_, scope, err := resolveTemplate(tmpl)

			require.NoError(t, err)
			assert.Equal(t, coupon.ScopeKindCategory, scope.Kind())
			require.NotNil(t, scope.TargetID())
			assert.Equal(t, targetID, *scope.TargetID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既知でない値引き種別の名前の場合、ErrInvalidDiscountKindを返す", func(t *testing.T) {
			t.Parallel()

			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.DiscountKindName = "unknown"
			})

			_, _, err := resolveTemplate(tmpl)

			require.ErrorIs(t, err, coupon.ErrInvalidDiscountKind)
		})

		t.Run("既知でない適用範囲種別の名前の場合、ErrInvalidScopeKindを返す", func(t *testing.T) {
			t.Parallel()

			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.ScopeKindName = "unknown"
			})

			_, _, err := resolveTemplate(tmpl)

			require.ErrorIs(t, err, coupon.ErrInvalidScopeKind)
		})

		t.Run("定率の値が1を超える場合、ErrInvalidDiscountValueを返す", func(t *testing.T) {
			t.Parallel()

			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.DiscountValue = newDecimal(t, "1.01")
			})

			_, _, err := resolveTemplate(tmpl)

			require.ErrorIs(t, err, coupon.ErrInvalidDiscountValue)
		})

		t.Run("定額に値引き上限を設けた場合、ErrInvalidMaxAmountを返す", func(t *testing.T) {
			t.Parallel()

			maxAmount := int64(2000)
			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.DiscountKindName = "flat"
				a.DiscountValue = newDecimal(t, "5.00")
				a.DiscountMaxAmount = &maxAmount
			})

			_, _, err := resolveTemplate(tmpl)

			require.ErrorIs(t, err, coupon.ErrInvalidMaxAmount)
		})

		t.Run("カテゴリ限定で対象が未設定の場合、ErrInvalidScopeTargetを返す", func(t *testing.T) {
			t.Parallel()

			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.ScopeKindName = "category"
			})

			_, _, err := resolveTemplate(tmpl)

			require.ErrorIs(t, err, coupon.ErrInvalidScopeTarget)
		})
	})
}

func Test_couponAttributesFor(t *testing.T) {
	t.Parallel()

	issuedAt := testStartsAt.Add(time.Hour)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受給者と発行日時だけが受け取りごとに変わり、残りはテンプレートから決まる", func(t *testing.T) {
			t.Parallel()

			userID := newTestUUID(t)

			got, err := couponAttributesFor(newCampaignFor(t, newTemplate(t, nil)), userID, issuedAt)

			require.NoError(t, err)
			assert.Equal(t, userID, got.UserID)
			assert.Equal(t, issuedAt, got.IssuedAt)
			assert.Equal(t, testCouponExpiresAt, got.ExpiresAt)
			assert.Equal(t, coupon.DiscountKindRate, got.Discount.Kind())
			assert.Equal(t, coupon.ScopeKindAll, got.Scope.Kind())
		})

		t.Run("テンプレートの条件を配るクーポンへ焼く", func(t *testing.T) {
			t.Parallel()

			minPurchase := int64(5000)
			usableFrom := testCouponExpiresAt.Add(-24 * time.Hour)
			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.MinPurchaseAmount = &minPurchase
				a.UsableFrom = &usableFrom
			})

			got, err := couponAttributesFor(newCampaignFor(t, tmpl), newTestUUID(t), issuedAt)

			require.NoError(t, err)
			require.NotNil(t, got.MinPurchaseAmount)
			assert.Equal(t, minPurchase, *got.MinPurchaseAmount)
			require.NotNil(t, got.UsableFrom)
			assert.Equal(t, usableFrom, *got.UsableFrom)
		})

		t.Run("導いた属性からクーポンを生成できる", func(t *testing.T) {
			t.Parallel()

			attrs, err := couponAttributesFor(newCampaignFor(t, newTemplate(t, nil)), newTestUUID(t), issuedAt)
			require.NoError(t, err)

			got, err := coupon.New(newTestUUID(t), attrs)

			require.NoError(t, err)
			assert.False(t, got.IsUsed())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("テンプレートが解決できない場合、クーポン集約の検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			tmpl := newTemplate(t, func(a *domaincampaign.TemplateAttributes) {
				a.DiscountKindName = "unknown"
			})

			_, err := couponAttributesFor(newCampaignFor(t, tmpl), newTestUUID(t), issuedAt)

			require.ErrorIs(t, err, coupon.ErrInvalidDiscountKind)
		})
	})
}
