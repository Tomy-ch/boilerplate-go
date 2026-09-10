package coupon

import (
	"testing"
	"time"

	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testIssuedAt  = time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	testExpiresAt = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
)

func validCouponArgs(t *testing.T) Attributes {
	t.Helper()

	discount, err := NewRateDiscount(newTestDecimal(t, "0.10"))
	require.NoError(t, err)
	scope, err := NewCategoryScope(newTestUUID(t))
	require.NoError(t, err)

	return Attributes{
		UserID:    newTestUUID(t),
		Discount:  discount,
		Scope:     scope,
		ExpiresAt: testExpiresAt,
		IssuedAt:  testIssuedAt,
	}
}

func newTestCoupon(t *testing.T) *Coupon {
	t.Helper()
	attrs := validCouponArgs(t)
	c, err := New(newTestUUID(t), attrs)
	require.NoError(t, err)

	return c
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全属性が有効な場合、未使用のクーポンを生成する", func(t *testing.T) {
			t.Parallel()

			id := newTestUUID(t)
			attrs := validCouponArgs(t)

			actual, err := New(id, attrs)

			require.NoError(t, err)
			assert.Equal(t, id, actual.ID())
			assert.Equal(t, attrs.UserID, actual.UserID())
			assert.Equal(t, testExpiresAt, actual.ExpiresAt())
			assert.Equal(t, testIssuedAt, actual.IssuedAt())
			assert.False(t, actual.IsUsed())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)

			actual, err := New(uuid.UUID{}, attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("受給者が未設定の場合、ErrInvalidUserIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.UserID = uuid.UUID{}

			actual, err := New(newTestUUID(t), attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidUserID)
		})

		t.Run("値引きが未設定の場合、ErrInvalidDiscountを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.Discount = Discount{}

			actual, err := New(newTestUUID(t), attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidDiscount)
		})

		t.Run("適用範囲が未設定の場合、ErrInvalidScopeを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.Scope = Scope{}

			actual, err := New(newTestUUID(t), attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidScope)
		})

		t.Run("発行日時がゼロ値の場合、ErrInvalidIssuedAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.IssuedAt = time.Time{}

			actual, err := New(newTestUUID(t), attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidIssuedAt)
		})

		t.Run("有効期限がゼロ値の場合、ErrInvalidExpiresAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.ExpiresAt = time.Time{}

			actual, err := New(newTestUUID(t), attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidExpiresAt)
		})

		t.Run("有効期限が発行日時と同時刻の場合、ErrInvalidExpiresAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.ExpiresAt = attrs.IssuedAt

			actual, err := New(newTestUUID(t), attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidExpiresAt)
		})

		t.Run("有効期限が発行日時より前の場合、ErrInvalidExpiresAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.ExpiresAt = attrs.IssuedAt.Add(-time.Hour)

			actual, err := New(newTestUUID(t), attrs)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidExpiresAt)
		})
	})
}

func TestReconstruct(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未使用として再構築する", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)

			actual, err := Reconstruct(newTestUUID(t), attrs, nil)

			require.NoError(t, err)
			assert.False(t, actual.IsUsed())
		})

		t.Run("使用済みとして再構築する", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			usedAt := testIssuedAt.Add(time.Hour)

			actual, err := Reconstruct(newTestUUID(t), attrs, &usedAt)

			require.NoError(t, err)
			assert.True(t, actual.IsUsed())
			require.NotNil(t, actual.UsedAt())
			assert.Equal(t, usedAt, *actual.UsedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("永続化されている属性が不正な場合、生成時と同じ検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			attrs.UserID = uuid.UUID{}

			actual, err := Reconstruct(newTestUUID(t), attrs, nil)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidUserID)
		})
	})
}

func Test_newCoupon(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("使用日時を渡した場合、防御コピーして保持する", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			usedAt := testIssuedAt.Add(time.Hour)

			actual, err := newCoupon(newTestUUID(t), attrs, &usedAt)
			require.NoError(t, err)

			usedAt = time.Time{}

			require.NotNil(t, actual.UsedAt())
			assert.Equal(t, testIssuedAt.Add(time.Hour), *actual.UsedAt())
		})
	})
}

func TestCoupon_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保持しているIDを返す", func(t *testing.T) {
			t.Parallel()

			id := newTestUUID(t)
			attrs := validCouponArgs(t)
			c, err := New(id, attrs)
			require.NoError(t, err)

			assert.Equal(t, id, c.ID())
		})
	})
}

func TestCoupon_UserID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受給者のユーザーIDを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, attrs.UserID, c.UserID())
		})
	})
}

func TestCoupon_Discount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保持している値引きを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, DiscountKindRate, c.Discount().Kind())
		})
	})
}

func TestCoupon_Scope(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保持している適用範囲を返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, ScopeKindCategory, c.Scope().Kind())
		})
	})
}

func TestCoupon_ExpiresAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("有効期限を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testExpiresAt, newTestCoupon(t).ExpiresAt())
		})
	})
}

func TestCoupon_IssuedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("発行日時を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testIssuedAt, newTestCoupon(t).IssuedAt())
		})
	})
}

func TestCoupon_UsedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未使用の場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, newTestCoupon(t).UsedAt())
		})

		t.Run("返り値のポインタを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			usedAt := testIssuedAt.Add(time.Hour)
			c, err := Reconstruct(newTestUUID(t), attrs, &usedAt)
			require.NoError(t, err)

			got := c.UsedAt()
			*got = time.Time{}

			require.NotNil(t, c.UsedAt())
			assert.Equal(t, testIssuedAt.Add(time.Hour), *c.UsedAt())
		})
	})
}

func TestCoupon_IsUsed(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未使用の場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, newTestCoupon(t).IsUsed())
		})

		t.Run("使用済みの場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			usedAt := testIssuedAt.Add(time.Hour)
			c, err := Reconstruct(newTestUUID(t), attrs, &usedAt)
			require.NoError(t, err)

			assert.True(t, c.IsUsed())
		})
	})
}

func TestCoupon_IsExpired(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("有効期限より前の時点では、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, newTestCoupon(t).IsExpired(testExpiresAt.Add(-time.Second)))
		})

		t.Run("有効期限ちょうどの時点で、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, newTestCoupon(t).IsExpired(testExpiresAt))
		})

		t.Run("有効期限を過ぎた時点では、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, newTestCoupon(t).IsExpired(testExpiresAt.Add(time.Second)))
		})
	})
}

// newScopedCoupon は、指定した適用範囲と値引きを持つクーポンを組み立てます。
func newScopedCoupon(t *testing.T, discount Discount, scope Scope) *Coupon {
	t.Helper()

	attrs := validCouponArgs(t)
	attrs.Discount = discount
	attrs.Scope = scope
	c, err := New(newTestUUID(t), attrs)
	require.NoError(t, err)

	return c
}

func TestCoupon_IsHeldBy(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受給者本人の場合はtrueを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)

			assert.True(t, c.IsHeldBy(c.UserID()))
		})

		t.Run("別の利用者の場合はfalseを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)

			assert.False(t, c.IsHeldBy(newTestUUID(t)))
		})
	})
}

func TestCoupon_DiscountFor(t *testing.T) {
	t.Parallel()

	flat := func(t *testing.T, v string) Discount {
		t.Helper()
		d, err := NewFlatDiscount(newTestDecimal(t, v))
		require.NoError(t, err)

		return d
	}
	rate := func(t *testing.T, v string) Discount {
		t.Helper()
		d, err := NewRateDiscount(newTestDecimal(t, v))
		require.NoError(t, err)

		return d
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体の適用範囲は全明細の小計を対象にする", func(t *testing.T) {
			t.Parallel()

			c := newScopedCoupon(t, rate(t, "0.10"), NewAllScope())
			a, _ := newTestLine(t, "10.00")
			b, _ := newTestLine(t, "20.00")

			got, err := c.DiscountFor([]Line{a, b})

			require.NoError(t, err)
			assert.Equal(t, 300, got)
		})

		t.Run("カテゴリ限定は対象カテゴリの明細だけを合算する", func(t *testing.T) {
			t.Parallel()

			target, targetAttrs := newTestLine(t, "20.00")
			other, _ := newTestLine(t, "30.00")
			scope, err := NewCategoryScope(targetAttrs.CategoryID)
			require.NoError(t, err)
			c := newScopedCoupon(t, rate(t, "0.10"), scope)

			got, derr := c.DiscountFor([]Line{target, other})

			require.NoError(t, derr)
			assert.Equal(t, 200, got)
		})

		t.Run("対象の明細が1件も無い場合は0を返す", func(t *testing.T) {
			t.Parallel()

			scope, err := NewProductScope(newTestUUID(t))
			require.NoError(t, err)
			c := newScopedCoupon(t, flat(t, "5.00"), scope)
			line, _ := newTestLine(t, "20.00")

			got, derr := c.DiscountFor([]Line{line})

			require.NoError(t, derr)
			assert.Zero(t, got)
		})

		t.Run("明細が空の場合は0を返す", func(t *testing.T) {
			t.Parallel()

			c := newScopedCoupon(t, flat(t, "5.00"), NewAllScope())

			got, err := c.DiscountFor(nil)

			require.NoError(t, err)
			assert.Zero(t, got)
		})

		t.Run("最小単位に満たない値引きは切り捨てで0になる", func(t *testing.T) {
			t.Parallel()

			c := newScopedCoupon(t, rate(t, "0.10"), NewAllScope())
			line, _ := newTestLine(t, "0.05")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Zero(t, got)
		})

		t.Run("端数は切り捨てる", func(t *testing.T) {
			t.Parallel()

			c := newScopedCoupon(t, rate(t, "0.10"), NewAllScope())
			line, _ := newTestLine(t, "19.99")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Equal(t, 199, got)
		})

		t.Run("定額が対象小計を超える場合は対象小計まで切り詰める", func(t *testing.T) {
			t.Parallel()

			c := newScopedCoupon(t, flat(t, "50.00"), NewAllScope())
			line, _ := newTestLine(t, "20.00")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Equal(t, 2000, got)
		})

		t.Run("合算してから丸めるため明細ごとの丸め落ちが起きない", func(t *testing.T) {
			t.Parallel()

			c := newScopedCoupon(t, rate(t, "0.10"), NewAllScope())
			a, _ := newTestLine(t, "0.05")
			b, _ := newTestLine(t, "0.05")

			got, err := c.DiscountFor([]Line{a, b})

			require.NoError(t, err)
			assert.Equal(t, 1, got)
		})

		t.Run("値引き上限を超える場合は上限まで引く", func(t *testing.T) {
			t.Parallel()

			capped, err := rate(t, "0.10").WithMaxAmount(200)
			require.NoError(t, err)
			c := newScopedCoupon(t, capped, NewAllScope())
			line, _ := newTestLine(t, "50.00")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Equal(t, 200, got)
		})

		t.Run("値引き上限に満たない場合は上限に届かない額をそのまま引く", func(t *testing.T) {
			t.Parallel()

			capped, err := rate(t, "0.10").WithMaxAmount(2000)
			require.NoError(t, err)
			c := newScopedCoupon(t, capped, NewAllScope())
			line, _ := newTestLine(t, "50.00")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Equal(t, 500, got)
		})

		t.Run("最低購入金額ちょうどの場合は値引きする", func(t *testing.T) {
			t.Parallel()

			minAmount := int64(5000)
			attrs := validCouponArgs(t)
			attrs.Scope = NewAllScope()
			attrs.MinPurchaseAmount = &minAmount
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)
			line, _ := newTestLine(t, "50.00")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Equal(t, 500, got)
		})

		t.Run("値引き上限ちょうどの場合はその額を引く", func(t *testing.T) {
			t.Parallel()

			capped, err := rate(t, "0.10").WithMaxAmount(500)
			require.NoError(t, err)
			c := newScopedCoupon(t, capped, NewAllScope())
			line, _ := newTestLine(t, "50.00")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Equal(t, 500, got)
		})

		t.Run("最低購入金額を満たさない場合は0を返す", func(t *testing.T) {
			t.Parallel()

			minAmount := int64(5000)
			attrs := validCouponArgs(t)
			attrs.Scope = NewAllScope()
			attrs.MinPurchaseAmount = &minAmount
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)
			line, _ := newTestLine(t, "49.99")

			got, err := c.DiscountFor([]Line{line})

			require.NoError(t, err)
			assert.Zero(t, got)
		})

		t.Run("最低購入金額は適用範囲が絞る前の購入全体の小計で判定する", func(t *testing.T) {
			t.Parallel()

			// 対象は 20.00 だけだが、購入全体は 60.00 なので下限 50.00 を満たす。
			minAmount := int64(5000)
			attrs := validCouponArgs(t)
			target, targetAttrs := newTestLine(t, "20.00")
			other, _ := newTestLine(t, "40.00")
			scope, err := NewCategoryScope(targetAttrs.CategoryID)
			require.NoError(t, err)
			attrs.Scope = scope
			attrs.MinPurchaseAmount = &minAmount
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)

			got, err := c.DiscountFor([]Line{target, other})

			require.NoError(t, err)
			assert.Equal(t, 200, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引き額が決済スケールの整数に収まらない場合、ErrInvalidDiscountValueを返す", func(t *testing.T) {
			t.Parallel()

			// Line.Subtotal は観測値であって検証を持たないため、決済スケールへ落とすと
			// int64 を超える小計がドメインへ届き得る。
			c := newScopedCoupon(t, rate(t, "1"), NewAllScope())
			line, _ := newTestLine(t, "100000000000000000000")

			got, err := c.DiscountFor([]Line{line})

			require.ErrorIs(t, err, ErrInvalidDiscountValue)
			assert.Zero(t, got)
		})
	})
}

func TestCoupon_Redeem(t *testing.T) {
	t.Parallel()

	usableAt := testExpiresAt.Add(-time.Hour)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未使用かつ有効期限内なら使用日時を刻む", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)

			require.NoError(t, c.Redeem(usableAt))

			assert.True(t, c.IsUsed())
			require.NotNil(t, c.UsedAt())
			assert.Equal(t, usableAt, *c.UsedAt())
		})

		t.Run("利用開始日時ちょうどなら使用日時を刻む", func(t *testing.T) {
			t.Parallel()

			// 拒否側（1ns 前）だけでなく受理側も Redeem 経由で押さえ、比較の向きを両側から固定する。
			from := testIssuedAt.Add(24 * time.Hour)
			c := newConditionedCoupon(t, nil, &from)

			require.NoError(t, c.Redeem(from))

			assert.True(t, c.IsUsed())
			require.NotNil(t, c.UsedAt())
			assert.Equal(t, from, *c.UsedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("使用済みの場合はErrAlreadyUsedを返し状態を変えない", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)
			require.NoError(t, c.Redeem(usableAt))

			err := c.Redeem(usableAt.Add(time.Minute))

			require.ErrorIs(t, err, ErrAlreadyUsed)
			require.NotNil(t, c.UsedAt())
			assert.Equal(t, usableAt, *c.UsedAt())
		})

		t.Run("利用開始日時より前の場合はErrNotYetUsableを返し状態を変えない", func(t *testing.T) {
			t.Parallel()

			from := testIssuedAt.Add(24 * time.Hour)
			c := newConditionedCoupon(t, nil, &from)

			err := c.Redeem(from.Add(-time.Nanosecond))

			require.ErrorIs(t, err, ErrNotYetUsable)
			assert.False(t, c.IsUsed())
		})

		t.Run("有効期限ちょうどの場合はErrExpiredを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)

			err := c.Redeem(testExpiresAt)

			require.ErrorIs(t, err, ErrExpired)
			assert.False(t, c.IsUsed())
		})

		t.Run("有効期限を過ぎている場合はErrExpiredを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)

			err := c.Redeem(testExpiresAt.Add(time.Hour))

			require.ErrorIs(t, err, ErrExpired)
			assert.False(t, c.IsUsed())
		})

		t.Run("使用済みかつ失効している場合は使用済みを先に返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)
			require.NoError(t, c.Redeem(usableAt))

			err := c.Redeem(testExpiresAt.Add(time.Hour))

			require.ErrorIs(t, err, ErrAlreadyUsed)
		})
	})
}

func TestCoupon_Restore(t *testing.T) {
	t.Parallel()

	usableAt := testExpiresAt.Add(-time.Hour)

	redeemed := func(t *testing.T) *Coupon {
		t.Helper()
		c := newTestCoupon(t)
		require.NoError(t, c.Redeem(usableAt))

		return c
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("使用済みかつ有効期限内なら未使用へ戻す", func(t *testing.T) {
			t.Parallel()

			c := redeemed(t)

			restored, err := c.Restore(usableAt.Add(time.Minute))

			require.NoError(t, err)
			assert.True(t, restored)
			assert.False(t, c.IsUsed())
			assert.Nil(t, c.UsedAt())
		})

		t.Run("戻したクーポンは再び引き換えられる", func(t *testing.T) {
			t.Parallel()

			c := redeemed(t)
			_, err := c.Restore(usableAt.Add(time.Minute))
			require.NoError(t, err)

			require.NoError(t, c.Redeem(usableAt.Add(2*time.Minute)))
			assert.True(t, c.IsUsed())
		})

		t.Run("有効期限ちょうどの場合は戻さず使用日時を保つ", func(t *testing.T) {
			t.Parallel()

			c := redeemed(t)

			restored, err := c.Restore(testExpiresAt)

			require.NoError(t, err)
			assert.False(t, restored)
			require.NotNil(t, c.UsedAt())
			assert.Equal(t, usableAt, *c.UsedAt())
		})

		t.Run("有効期限を過ぎている場合は戻さず使用日時を保つ", func(t *testing.T) {
			t.Parallel()

			c := redeemed(t)

			restored, err := c.Restore(testExpiresAt.Add(time.Hour))

			require.NoError(t, err)
			assert.False(t, restored)
			require.NotNil(t, c.UsedAt())
			assert.Equal(t, usableAt, *c.UsedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未使用の場合はErrNotUsedを返し状態を変えない", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)

			restored, err := c.Restore(usableAt)

			require.ErrorIs(t, err, ErrNotUsed)
			assert.False(t, restored)
			assert.False(t, c.IsUsed())
		})

		t.Run("未使用かつ失効している場合も未使用を先に返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCoupon(t)

			_, err := c.Restore(testExpiresAt.Add(time.Hour))

			require.ErrorIs(t, err, ErrNotUsed)
		})
	})
}

func Test_validateValidity(t *testing.T) {
	t.Parallel()

	issuedAt := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("有効期限が発行日時より後なら通す", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateValidity(issuedAt, issuedAt.Add(time.Nanosecond)))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("発行日時がゼロ値なら検証エラーになる", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateValidity(time.Time{}, issuedAt.Add(time.Hour)), ErrInvalidIssuedAt)
		})

		t.Run("有効期限がゼロ値なら検証エラーになる", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateValidity(issuedAt, time.Time{}), ErrInvalidExpiresAt)
		})

		t.Run("有効期限が発行日時と同時刻なら検証エラーになる", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateValidity(issuedAt, issuedAt), ErrInvalidExpiresAt)
		})

		t.Run("有効期限が発行日時より前なら検証エラーになる", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateValidity(issuedAt, issuedAt.Add(-time.Hour)), ErrInvalidExpiresAt)
		})
	})
}

// newConditionedCoupon は、条件を持つクーポンを生成します。nil を渡した条件は設定されません。
func newConditionedCoupon(t *testing.T, minPurchaseAmount *int64, usableFrom *time.Time) *Coupon {
	t.Helper()

	attrs := validCouponArgs(t)
	attrs.MinPurchaseAmount = minPurchaseAmount
	attrs.UsableFrom = usableFrom
	c, err := New(newTestUUID(t), attrs)
	require.NoError(t, err)

	return c
}

func TestCoupon_MinPurchaseAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件を持たない場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, newTestCoupon(t).MinPurchaseAmount())
		})

		t.Run("返した値を書き換えてもクーポンの条件は変わらない", func(t *testing.T) {
			t.Parallel()

			minAmount := int64(5000)
			c := newConditionedCoupon(t, &minAmount, nil)

			got := c.MinPurchaseAmount()
			require.NotNil(t, got)
			*got = 1

			require.NotNil(t, c.MinPurchaseAmount())
			assert.Equal(t, int64(5000), *c.MinPurchaseAmount())
		})
	})
}

func TestCoupon_UsableFrom(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件を持たない場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, newTestCoupon(t).UsableFrom())
		})

		t.Run("返した値を書き換えてもクーポンの条件は変わらない", func(t *testing.T) {
			t.Parallel()

			from := testIssuedAt.Add(24 * time.Hour)
			c := newConditionedCoupon(t, nil, &from)

			got := c.UsableFrom()
			require.NotNil(t, got)
			*got = testIssuedAt

			require.NotNil(t, c.UsableFrom())
			assert.Equal(t, from, *c.UsableFrom())
		})
	})
}

func TestCoupon_IsNotYetUsable(t *testing.T) {
	t.Parallel()

	from := testIssuedAt.Add(24 * time.Hour)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("利用開始日時を持たない場合は常にfalseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, newTestCoupon(t).IsNotYetUsable(testIssuedAt))
		})

		t.Run("利用開始日時より前の場合はtrueを返す", func(t *testing.T) {
			t.Parallel()

			c := newConditionedCoupon(t, nil, &from)

			assert.True(t, c.IsNotYetUsable(from.Add(-time.Nanosecond)))
		})

		t.Run("利用開始日時ちょうどの場合はfalseを返す", func(t *testing.T) {
			t.Parallel()

			c := newConditionedCoupon(t, nil, &from)

			assert.False(t, c.IsNotYetUsable(from))
		})

		t.Run("利用開始日時より後の場合はfalseを返す", func(t *testing.T) {
			t.Parallel()

			c := newConditionedCoupon(t, nil, &from)

			assert.False(t, c.IsNotYetUsable(from.Add(time.Nanosecond)))
		})
	})
}

func TestCoupon_SatisfiesMinPurchase(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("最低購入金額を持たない場合は常に満たす", func(t *testing.T) {
			t.Parallel()

			assert.True(t, newTestCoupon(t).SatisfiesMinPurchase(0))
		})

		t.Run("下限に満たない場合は満たさない", func(t *testing.T) {
			t.Parallel()

			minAmount := int64(5000)
			c := newConditionedCoupon(t, &minAmount, nil)

			assert.False(t, c.SatisfiesMinPurchase(4999))
		})

		t.Run("下限ちょうどの場合は満たす", func(t *testing.T) {
			t.Parallel()

			minAmount := int64(5000)
			c := newConditionedCoupon(t, &minAmount, nil)

			assert.True(t, c.SatisfiesMinPurchase(5000))
		})

		t.Run("下限を超える場合は満たす", func(t *testing.T) {
			t.Parallel()

			minAmount := int64(5000)
			c := newConditionedCoupon(t, &minAmount, nil)

			assert.True(t, c.SatisfiesMinPurchase(5001))
		})
	})
}

func Test_validateConditions(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("条件をどちらも持たない場合はエラーにならない", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateConditions(validCouponArgs(t)))
		})

		t.Run("利用開始日時が有効期限より前ならエラーにならない", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			from := testExpiresAt.Add(-time.Nanosecond)
			attrs.UsableFrom = &from

			require.NoError(t, validateConditions(attrs))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("最低購入金額が0の場合はErrInvalidMinPurchaseAmountを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			minAmount := int64(0)
			attrs.MinPurchaseAmount = &minAmount

			require.ErrorIs(t, validateConditions(attrs), ErrInvalidMinPurchaseAmount)
		})

		t.Run("最低購入金額が負の場合はErrInvalidMinPurchaseAmountを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			minAmount := int64(-1)
			attrs.MinPurchaseAmount = &minAmount

			require.ErrorIs(t, validateConditions(attrs), ErrInvalidMinPurchaseAmount)
		})

		t.Run("利用開始日時が有効期限ちょうどの場合はErrInvalidUsableFromを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			from := testExpiresAt
			attrs.UsableFrom = &from

			require.ErrorIs(t, validateConditions(attrs), ErrInvalidUsableFrom)
		})

		t.Run("利用開始日時が有効期限より後の場合はErrInvalidUsableFromを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCouponArgs(t)
			from := testExpiresAt.Add(time.Nanosecond)
			attrs.UsableFrom = &from

			require.ErrorIs(t, validateConditions(attrs), ErrInvalidUsableFrom)
		})
	})
}
