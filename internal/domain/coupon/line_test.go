package coupon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestLine(t *testing.T, subtotal string) (Line, LineAttributes) {
	t.Helper()
	attrs := LineAttributes{
		ProductID:  newTestUUID(t),
		CategoryID: newTestUUID(t),
		Subtotal:   newTestDecimal(t, subtotal),
	}

	return NewLine(attrs), attrs
}

func TestNewLine(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("観測値をそのまま保持する", func(t *testing.T) {
			t.Parallel()

			line, attrs := newTestLine(t, "19.99")

			assert.Equal(t, attrs.ProductID, line.ProductID())
			assert.Equal(t, attrs.CategoryID, line.CategoryID())
			assert.True(t, attrs.Subtotal.Equal(line.Subtotal()))
		})

		t.Run("小計が0でも検証で弾かない", func(t *testing.T) {
			t.Parallel()

			line, _ := newTestLine(t, "0")

			assert.True(t, line.Subtotal().IsZero())
		})
	})
}

func TestLine_ProductID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("組み立てた商品IDを返す", func(t *testing.T) {
			t.Parallel()

			line, attrs := newTestLine(t, "10.00")

			assert.Equal(t, attrs.ProductID, line.ProductID())
		})
	})
}

func TestLine_CategoryID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("組み立てた商品カテゴリIDを返す", func(t *testing.T) {
			t.Parallel()

			line, attrs := newTestLine(t, "10.00")

			assert.Equal(t, attrs.CategoryID, line.CategoryID())
		})
	})
}

func TestLine_Subtotal(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("丸めずに小計をそのまま返す", func(t *testing.T) {
			t.Parallel()

			line, _ := newTestLine(t, "19.999")

			assert.Equal(t, "19.999", line.Subtotal().String())
		})
	})
}
