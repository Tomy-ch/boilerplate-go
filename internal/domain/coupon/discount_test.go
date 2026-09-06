package coupon

import (
	"testing"

	"go-boilerplate/pkg/decimal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	require.NoError(t, err)

	return d
}

func Test_allDiscountKinds(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既知の値引き種別をすべて返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []DiscountKind{DiscountKindFlat, DiscountKindRate}, allDiscountKinds())
		})
	})
}

func TestNewDiscountKind(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("定額のコードから定額を解決する", func(t *testing.T) {
			t.Parallel()

			actual, err := NewDiscountKind(discountKindFlat)

			require.NoError(t, err)
			assert.Equal(t, DiscountKindFlat, actual)
		})

		t.Run("定率のコードから定率を解決する", func(t *testing.T) {
			t.Parallel()

			actual, err := NewDiscountKind(discountKindRate)

			require.NoError(t, err)
			assert.Equal(t, DiscountKindRate, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既知でないコードの場合、ErrInvalidDiscountKindを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewDiscountKind(0)

			require.ErrorIs(t, err, ErrInvalidDiscountKind)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestDiscountKind_Code(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("業務キーを返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, discountKindFlat, DiscountKindFlat.Code())
			assert.Equal(t, discountKindRate, DiscountKindRate.Code())
		})
	})
}

func TestDiscountKind_Name(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("種別の名前を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "flat", DiscountKindFlat.Name())
			assert.Equal(t, "rate", DiscountKindRate.Name())
		})
	})
}

func TestDiscountKind_IsZero(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ゼロ値の場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, DiscountKind{}.IsZero())
		})

		t.Run("既知の種別の場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, DiscountKindFlat.IsZero())
		})
	})
}

func TestNewFlatDiscount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("正の金額の場合、定額の値引きを生成する", func(t *testing.T) {
			t.Parallel()

			amount := newTestDecimal(t, "5.00")

			actual, err := NewFlatDiscount(amount)

			require.NoError(t, err)
			assert.Equal(t, DiscountKindFlat, actual.Kind())
			assert.True(t, amount.Equal(actual.Value()))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("0の場合、ErrInvalidDiscountValueを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewFlatDiscount(decimal.FromInt(0))

			require.ErrorIs(t, err, ErrInvalidDiscountValue)
			assert.True(t, actual.IsZero())
		})

		t.Run("負の金額の場合、ErrInvalidDiscountValueを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewFlatDiscount(newTestDecimal(t, "5.00").Neg())

			require.ErrorIs(t, err, ErrInvalidDiscountValue)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestNewRateDiscount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("0より大きく1未満の率の場合、定率の値引きを生成する", func(t *testing.T) {
			t.Parallel()

			rate := newTestDecimal(t, "0.10")

			actual, err := NewRateDiscount(rate)

			require.NoError(t, err)
			assert.Equal(t, DiscountKindRate, actual.Kind())
			assert.True(t, rate.Equal(actual.Value()))
		})

		t.Run("上限のちょうど1の場合、生成する", func(t *testing.T) {
			t.Parallel()

			actual, err := NewRateDiscount(decimal.FromInt(1))

			require.NoError(t, err)
			assert.Equal(t, DiscountKindRate, actual.Kind())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("0の場合、ErrInvalidDiscountValueを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewRateDiscount(decimal.FromInt(0))

			require.ErrorIs(t, err, ErrInvalidDiscountValue)
			assert.True(t, actual.IsZero())
		})

		t.Run("1を超える場合、ErrInvalidDiscountValueを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewRateDiscount(newTestDecimal(t, "1.01"))

			require.ErrorIs(t, err, ErrInvalidDiscountValue)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestReconstructDiscount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("定額を再構築する", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructDiscount(DiscountKindFlat, newTestDecimal(t, "5.00"))

			require.NoError(t, err)
			assert.Equal(t, DiscountKindFlat, actual.Kind())
		})

		t.Run("定率を再構築する", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructDiscount(DiscountKindRate, newTestDecimal(t, "0.10"))

			require.NoError(t, err)
			assert.Equal(t, DiscountKindRate, actual.Kind())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("種別が未設定の場合、ErrInvalidDiscountKindを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructDiscount(DiscountKind{}, newTestDecimal(t, "5.00"))

			require.ErrorIs(t, err, ErrInvalidDiscountKind)
			assert.True(t, actual.IsZero())
		})

		t.Run("永続化されている値が範囲外の場合、生成時と同じ検証エラーを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructDiscount(DiscountKindRate, newTestDecimal(t, "1.01"))

			require.ErrorIs(t, err, ErrInvalidDiscountValue)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestDiscount_Kind(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保持している値引き種別を返す", func(t *testing.T) {
			t.Parallel()

			d, err := NewFlatDiscount(newTestDecimal(t, "5.00"))
			require.NoError(t, err)

			assert.Equal(t, DiscountKindFlat, d.Kind())
		})
	})
}

func TestDiscount_Value(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保持している値を返す", func(t *testing.T) {
			t.Parallel()

			d, err := NewRateDiscount(newTestDecimal(t, "0.10"))
			require.NoError(t, err)

			assert.Equal(t, "0.1", d.Value().String())
		})
	})
}

func TestDiscount_IsZero(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ゼロ値の場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, Discount{}.IsZero())
		})

		t.Run("生成済みの場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			d, err := NewFlatDiscount(newTestDecimal(t, "5.00"))
			require.NoError(t, err)

			assert.False(t, d.IsZero())
		})
	})
}

func TestDiscount_Apply(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("定額は対象額から額面をそのまま引く", func(t *testing.T) {
			t.Parallel()

			d, err := NewFlatDiscount(newTestDecimal(t, "5.00"))
			require.NoError(t, err)

			assert.Equal(t, "5", d.Apply(newTestDecimal(t, "20.00")).String())
		})

		t.Run("定額が対象額を超える場合は対象額まで切り詰める", func(t *testing.T) {
			t.Parallel()

			d, err := NewFlatDiscount(newTestDecimal(t, "50.00"))
			require.NoError(t, err)

			assert.Equal(t, "20", d.Apply(newTestDecimal(t, "20.00")).String())
		})

		t.Run("定額が対象額ちょうどの場合は対象額を返す", func(t *testing.T) {
			t.Parallel()

			d, err := NewFlatDiscount(newTestDecimal(t, "20.00"))
			require.NoError(t, err)

			assert.Equal(t, "20", d.Apply(newTestDecimal(t, "20.00")).String())
		})

		t.Run("定率は対象額に率を掛ける", func(t *testing.T) {
			t.Parallel()

			d, err := NewRateDiscount(newTestDecimal(t, "0.10"))
			require.NoError(t, err)

			assert.Equal(t, "2", d.Apply(newTestDecimal(t, "20.00")).String())
		})

		t.Run("定率は丸めずに十進量のまま返す", func(t *testing.T) {
			t.Parallel()

			d, err := NewRateDiscount(newTestDecimal(t, "0.10"))
			require.NoError(t, err)

			assert.Equal(t, "0.001", d.Apply(newTestDecimal(t, "0.01")).String())
		})

		t.Run("率が1の場合は対象額の全額を引く", func(t *testing.T) {
			t.Parallel()

			d, err := NewRateDiscount(newTestDecimal(t, "1"))
			require.NoError(t, err)

			assert.Equal(t, "20", d.Apply(newTestDecimal(t, "20.00")).String())
		})

		t.Run("対象額が0の場合は0を返す", func(t *testing.T) {
			t.Parallel()

			d, err := NewFlatDiscount(newTestDecimal(t, "5.00"))
			require.NoError(t, err)

			assert.True(t, d.Apply(newTestDecimal(t, "0")).IsZero())
		})

		t.Run("対象額が負の場合は0を返す", func(t *testing.T) {
			t.Parallel()

			d, err := NewFlatDiscount(newTestDecimal(t, "5.00"))
			require.NoError(t, err)

			assert.True(t, d.Apply(newTestDecimal(t, "-1.00")).IsZero())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未設定の値引きは0を返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, Discount{}.Apply(newTestDecimal(t, "20.00")).IsZero())
		})
	})
}
