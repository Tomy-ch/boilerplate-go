package campaign

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCode(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("英数字とハイフンからなるコードを生成する", func(t *testing.T) {
			t.Parallel()

			got, err := NewCode("WELCOME-2026")

			require.NoError(t, err)
			assert.Equal(t, "WELCOME-2026", got.Value())
		})

		t.Run("小文字は大文字へ揃える", func(t *testing.T) {
			t.Parallel()

			got, err := NewCode("welcome-2026")

			require.NoError(t, err)
			assert.Equal(t, "WELCOME-2026", got.Value())
		})

		t.Run("前後の空白を落とす", func(t *testing.T) {
			t.Parallel()

			got, err := NewCode("  welcome-2026\t\n")

			require.NoError(t, err)
			assert.Equal(t, "WELCOME-2026", got.Value())
		})

		t.Run("大文字と小文字の違いは同じコードとして扱う", func(t *testing.T) {
			t.Parallel()

			upper, err := NewCode("WELCOME-2026")
			require.NoError(t, err)
			lower, err := NewCode("welcome-2026")
			require.NoError(t, err)

			assert.Equal(t, upper, lower)
		})

		t.Run("最短の長さちょうどを認める", func(t *testing.T) {
			t.Parallel()

			got, err := NewCode(strings.Repeat("A", codeMinLength))

			require.NoError(t, err)
			assert.Len(t, got.Value(), codeMinLength)
		})

		t.Run("最長の長さちょうどを認める", func(t *testing.T) {
			t.Parallel()

			got, err := NewCode(strings.Repeat("A", codeMaxLength))

			require.NoError(t, err)
			assert.Len(t, got.Value(), codeMaxLength)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("最短の長さに1文字足りない場合はErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			_, err := NewCode(strings.Repeat("A", codeMinLength-1))

			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("最長の長さを1文字超える場合はErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			_, err := NewCode(strings.Repeat("A", codeMaxLength+1))

			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("空文字の場合はErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			_, err := NewCode("")

			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("空白だけの場合は正規化後に空になりErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			_, err := NewCode("            ")

			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("記号を含む場合はErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			_, err := NewCode("WELCOME_2026")

			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("空白を内側に含む場合はErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			_, err := NewCode("WELCOME 2026")

			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("英数字以外の文字を含む場合はErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			_, err := NewCode("ウェルカム2026")

			require.ErrorIs(t, err, ErrInvalidCode)
		})
	})
}

func TestCode_Value(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("正規化済みの値を返す", func(t *testing.T) {
			t.Parallel()

			c, err := NewCode(" welcome-2026 ")
			require.NoError(t, err)

			assert.Equal(t, "WELCOME-2026", c.Value())
		})
	})
}

func TestCode_IsZero(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成を経ていないゼロ値の場合はtrueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, Code{}.IsZero())
		})

		t.Run("生成したコードの場合はfalseを返す", func(t *testing.T) {
			t.Parallel()

			c, err := NewCode("WELCOME-2026")
			require.NoError(t, err)

			assert.False(t, c.IsZero())
		})
	})
}

func Test_isCodeChar(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("大文字英字を認める", func(t *testing.T) {
			t.Parallel()

			assert.True(t, isCodeChar('A'))
			assert.True(t, isCodeChar('Z'))
		})

		t.Run("数字を認める", func(t *testing.T) {
			t.Parallel()

			assert.True(t, isCodeChar('0'))
			assert.True(t, isCodeChar('9'))
		})

		t.Run("ハイフンを認める", func(t *testing.T) {
			t.Parallel()

			assert.True(t, isCodeChar('-'))
		})

		t.Run("小文字英字は認めない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isCodeChar('a'))
		})

		t.Run("アンダースコアは認めない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isCodeChar('_'))
		})
	})
}
