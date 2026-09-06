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
