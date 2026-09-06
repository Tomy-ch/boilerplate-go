package coupon

import (
	"testing"

	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.New()
	require.NoError(t, err)

	return id
}

func Test_allScopeKinds(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既知の適用範囲種別をすべて返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []ScopeKind{ScopeKindAll, ScopeKindCategory, ScopeKindProduct}, allScopeKinds())
		})
	})
}

func TestNewScopeKind(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体のコードから全体を解決する", func(t *testing.T) {
			t.Parallel()

			actual, err := NewScopeKind(scopeKindAll)

			require.NoError(t, err)
			assert.Equal(t, ScopeKindAll, actual)
		})

		t.Run("カテゴリ限定のコードからカテゴリ限定を解決する", func(t *testing.T) {
			t.Parallel()

			actual, err := NewScopeKind(scopeKindCategory)

			require.NoError(t, err)
			assert.Equal(t, ScopeKindCategory, actual)
		})

		t.Run("商品限定のコードから商品限定を解決する", func(t *testing.T) {
			t.Parallel()

			actual, err := NewScopeKind(scopeKindProduct)

			require.NoError(t, err)
			assert.Equal(t, ScopeKindProduct, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既知でないコードの場合、ErrInvalidScopeKindを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewScopeKind(0)

			require.ErrorIs(t, err, ErrInvalidScopeKind)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestScopeKind_Code(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("業務キーを返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, scopeKindAll, ScopeKindAll.Code())
			assert.Equal(t, scopeKindCategory, ScopeKindCategory.Code())
			assert.Equal(t, scopeKindProduct, ScopeKindProduct.Code())
		})
	})
}

func TestScopeKind_Name(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("種別の名前を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "all", ScopeKindAll.Name())
			assert.Equal(t, "category", ScopeKindCategory.Name())
			assert.Equal(t, "product", ScopeKindProduct.Name())
		})
	})
}

func TestScopeKind_IsZero(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ゼロ値の場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, ScopeKind{}.IsZero())
		})

		t.Run("既知の種別の場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, ScopeKindAll.IsZero())
		})
	})
}

func TestNewAllScope(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象を持たない適用範囲を生成する", func(t *testing.T) {
			t.Parallel()

			actual := NewAllScope()

			assert.Equal(t, ScopeKindAll, actual.Kind())
			assert.Nil(t, actual.TargetID())
		})
	})
}

func TestNewCategoryScope(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("カテゴリを対象とする適用範囲を生成する", func(t *testing.T) {
			t.Parallel()

			categoryID := newTestUUID(t)

			actual, err := NewCategoryScope(categoryID)

			require.NoError(t, err)
			assert.Equal(t, ScopeKindCategory, actual.Kind())
			require.NotNil(t, actual.TargetID())
			assert.Equal(t, categoryID, *actual.TargetID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("カテゴリIDが未設定の場合、ErrInvalidScopeTargetを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewCategoryScope(uuid.UUID{})

			require.ErrorIs(t, err, ErrInvalidScopeTarget)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestNewProductScope(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("商品を対象とする適用範囲を生成する", func(t *testing.T) {
			t.Parallel()

			productID := newTestUUID(t)

			actual, err := NewProductScope(productID)

			require.NoError(t, err)
			assert.Equal(t, ScopeKindProduct, actual.Kind())
			require.NotNil(t, actual.TargetID())
			assert.Equal(t, productID, *actual.TargetID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("商品IDが未設定の場合、ErrInvalidScopeTargetを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := NewProductScope(uuid.UUID{})

			require.ErrorIs(t, err, ErrInvalidScopeTarget)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestReconstructScope(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体を再構築する", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructScope(ScopeKindAll, nil)

			require.NoError(t, err)
			assert.Equal(t, ScopeKindAll, actual.Kind())
		})

		t.Run("カテゴリ限定を再構築する", func(t *testing.T) {
			t.Parallel()

			categoryID := newTestUUID(t)

			actual, err := ReconstructScope(ScopeKindCategory, &categoryID)

			require.NoError(t, err)
			require.NotNil(t, actual.TargetID())
			assert.Equal(t, categoryID, *actual.TargetID())
		})

		t.Run("商品限定を再構築する", func(t *testing.T) {
			t.Parallel()

			productID := newTestUUID(t)

			actual, err := ReconstructScope(ScopeKindProduct, &productID)

			require.NoError(t, err)
			require.NotNil(t, actual.TargetID())
			assert.Equal(t, productID, *actual.TargetID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体が対象を持つ場合、ErrInvalidScopeTargetを返す", func(t *testing.T) {
			t.Parallel()

			targetID := newTestUUID(t)

			actual, err := ReconstructScope(ScopeKindAll, &targetID)

			require.ErrorIs(t, err, ErrInvalidScopeTarget)
			assert.True(t, actual.IsZero())
		})

		t.Run("カテゴリ限定が対象を持たない場合、ErrInvalidScopeTargetを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructScope(ScopeKindCategory, nil)

			require.ErrorIs(t, err, ErrInvalidScopeTarget)
			assert.True(t, actual.IsZero())
		})

		t.Run("商品限定が対象を持たない場合、ErrInvalidScopeTargetを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructScope(ScopeKindProduct, nil)

			require.ErrorIs(t, err, ErrInvalidScopeTarget)
			assert.True(t, actual.IsZero())
		})

		t.Run("種別が未設定の場合、ErrInvalidScopeKindを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := ReconstructScope(ScopeKind{}, nil)

			require.ErrorIs(t, err, ErrInvalidScopeKind)
			assert.True(t, actual.IsZero())
		})
	})
}

func TestScope_Kind(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保持している適用範囲種別を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, ScopeKindAll, NewAllScope().Kind())
		})
	})
}

func TestScope_TargetID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体の場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, NewAllScope().TargetID())
		})

		t.Run("返り値のポインタを書き換えても適用範囲の内部は変わらない", func(t *testing.T) {
			t.Parallel()

			categoryID := newTestUUID(t)
			s, err := NewCategoryScope(categoryID)
			require.NoError(t, err)

			got := s.TargetID()
			*got = uuid.UUID{}

			require.NotNil(t, s.TargetID())
			assert.Equal(t, categoryID, *s.TargetID())
		})
	})
}

func TestScope_IsZero(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ゼロ値の場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, Scope{}.IsZero())
		})

		t.Run("生成済みの場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, NewAllScope().IsZero())
		})
	})
}

func TestScope_Covers(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全体はどの明細も対象にする", func(t *testing.T) {
			t.Parallel()

			line, _ := newTestLine(t, "10.00")

			assert.True(t, NewAllScope().Covers(line))
		})

		t.Run("カテゴリ限定は明細のカテゴリが一致するときだけ対象にする", func(t *testing.T) {
			t.Parallel()

			line, attrs := newTestLine(t, "10.00")
			scope, err := NewCategoryScope(attrs.CategoryID)
			require.NoError(t, err)

			assert.True(t, scope.Covers(line))
		})

		t.Run("カテゴリ限定は別のカテゴリの明細を対象にしない", func(t *testing.T) {
			t.Parallel()

			line, _ := newTestLine(t, "10.00")
			scope, err := NewCategoryScope(newTestUUID(t))
			require.NoError(t, err)

			assert.False(t, scope.Covers(line))
		})

		t.Run("商品限定は明細の商品が一致するときだけ対象にする", func(t *testing.T) {
			t.Parallel()

			line, attrs := newTestLine(t, "10.00")
			scope, err := NewProductScope(attrs.ProductID)
			require.NoError(t, err)

			assert.True(t, scope.Covers(line))
		})

		t.Run("商品限定は別の商品の明細を対象にしない", func(t *testing.T) {
			t.Parallel()

			line, _ := newTestLine(t, "10.00")
			scope, err := NewProductScope(newTestUUID(t))
			require.NoError(t, err)

			assert.False(t, scope.Covers(line))
		})

		t.Run("カテゴリ限定は商品IDが一致していてもカテゴリが違えば対象にしない", func(t *testing.T) {
			t.Parallel()

			line, attrs := newTestLine(t, "10.00")
			scope, err := NewCategoryScope(attrs.ProductID)
			require.NoError(t, err)

			assert.False(t, scope.Covers(line))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未設定の適用範囲はどの明細も対象にしない", func(t *testing.T) {
			t.Parallel()

			line, _ := newTestLine(t, "10.00")

			assert.False(t, Scope{}.Covers(line))
		})
	})
}
