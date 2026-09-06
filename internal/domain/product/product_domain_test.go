package product

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCreatedAt は、テスト用の登録日時です。公開日時と取り違えを検出できるよう異なる値にします。
var testCreatedAt = time.Date(2025, time.December, 31, 23, 59, 58, 0, time.UTC)

// mustPrice は、テスト用に十進文字列から非負の money.Price を構築します。
func mustPrice(t *testing.T, s string) money.Price {
	t.Helper()
	p, err := money.NewPrice(decimaltestkit.MustParse(t, s))
	require.NoError(t, err)
	return p
}

// mustStatusRef は、テスト用に有効な商品ステータス参照を構築します。
func mustStatusRef(t *testing.T, salt, name string) StatusRef {
	t.Helper()
	ref, err := NewStatusRef(uuidtestkit.NewTestFromSalt(t, salt), name)
	require.NoError(t, err)
	return ref
}

// mustCategoryRef は、テスト用に有効な商品カテゴリ参照を構築します。
func mustCategoryRef(t *testing.T, salt, name string) CategoryRef {
	t.Helper()
	ref, err := NewCategoryRef(uuidtestkit.NewTestFromSalt(t, salt), name)
	require.NoError(t, err)
	return ref
}

// mustImage は、テスト用に商品画像を構築します。
func mustImage(t *testing.T, salt, path string, displaySort int) Image {
	t.Helper()
	return NewImage(uuidtestkit.NewTestFromSalt(t, salt), ImageAttributes{ImagePath: path, DisplaySort: displaySort})
}

// imagesOfCount は、表示順が 1 から連番で重複しない画像を count 枚組み立てます。
func imagesOfCount(t *testing.T, count int) []Image {
	t.Helper()
	images := make([]Image, 0, count)
	for i := range count {
		images = append(images, mustImage(
			t,
			fmt.Sprintf("product_image_id_seq_%d", i),
			fmt.Sprintf("products/seq_%d.png", i),
			minImageDisplaySort+i,
		))
	}
	return images
}

// validProductArgs は、テスト用に有効な商品 ID と属性一式を構築します。
// Description と画像パスは取り違えを検出できるよう異なる値にします。
func validProductArgs(t *testing.T) (uuid.UUID, Attributes) {
	t.Helper()
	publishedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	return uuidtestkit.NewTestFromSalt(t, "product_id"), Attributes{
		Name:                  "ワイヤレスイヤホン",
		Description:           ptr.To("ノイズキャンセリング対応"),
		Price:                 mustPrice(t, "19.99"),
		Quantity:              100,
		StockWarningThreshold: ptr.To(10),
		Status:                mustStatusRef(t, "product_status_id", "在庫あり"),
		Category:              mustCategoryRef(t, "product_category_id", "電子機器"),
		PublishedAt:           ptr.To(publishedAt),
		Images: []Image{
			mustImage(t, "product_image_id_1", "products/earphone.png", 1),
			mustImage(t, "product_image_id_2", "products/earphone-sub.png", 2),
		},
	}
}

// updatedProductAttributes は、テスト用に更新後の商品属性一式を構築します。
func updatedProductAttributes(t *testing.T) Attributes {
	t.Helper()
	publishedAt := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	return Attributes{
		Name:                  "ワイヤレスイヤホン Pro",
		Description:           ptr.To("外音取り込みに対応"),
		Price:                 mustPrice(t, "29.95"),
		Quantity:              50,
		StockWarningThreshold: ptr.To(5),
		Status:                mustStatusRef(t, "updated_product_status_id", "在庫切れ"),
		Category:              mustCategoryRef(t, "updated_product_category_id", "オーディオ"),
		PublishedAt:           ptr.To(publishedAt),
		Images: []Image{
			mustImage(t, "updated_product_image_id_1", "products/earphone-pro.png", 1),
		},
	}
}

// newTestProduct は、テスト用に有効な商品エンティティを生成します。バージョンは initialVersion です。
func newTestProduct(t *testing.T) *Product {
	t.Helper()
	id, attrs := validProductArgs(t)
	p, err := New(id, attrs, testCreatedAt)
	require.NoError(t, err)
	return p
}

func TestNew(t *testing.T) {
	t.Parallel()

	id, attrs := validProductArgs(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全フィールドが有効な場合、Productエンティティが生成される", func(t *testing.T) {
			t.Parallel()

			actual, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)
			assert.Equal(t, id, actual.ID())
			assert.Equal(t, attrs.Name, actual.Name())
			assert.Equal(t, attrs.Description, actual.Description())
			assert.Equal(t, attrs.Price, actual.Price())
			assert.Equal(t, attrs.Quantity, actual.Quantity())
			assert.Equal(t, attrs.StockWarningThreshold, actual.StockWarningThreshold())
			assert.Equal(t, attrs.Status, actual.Status())
			assert.Equal(t, attrs.Category, actual.Category())
			assert.Equal(t, attrs.PublishedAt, actual.PublishedAt())
			assert.Equal(t, attrs.Images, actual.Images())
			assert.Equal(t, initialVersion, actual.Version())

			mutatedDescription := actual.Description()
			*mutatedDescription = "mutated"
			require.NotNil(t, actual.Description())
			assert.Equal(t, *attrs.Description, *actual.Description())

			mutatedThreshold := actual.StockWarningThreshold()
			*mutatedThreshold = minThreshold
			require.NotNil(t, actual.StockWarningThreshold())
			assert.Equal(t, *attrs.StockWarningThreshold, *actual.StockWarningThreshold())

			mutatedPublishedAt := actual.PublishedAt()
			*mutatedPublishedAt = time.Time{}
			require.NotNil(t, actual.PublishedAt())
			assert.Equal(t, *attrs.PublishedAt, *actual.PublishedAt())

			mutatedImages := actual.Images()
			mutatedImages[0] = mustImage(t, "mutated_image_id", "products/mutated.png", 9)
			assert.Equal(t, attrs.Images, actual.Images())
		})

		t.Run("生成後に引数のポインタを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			// 他の並列サブテストとポインタを共有しないよう作り直す。
			localID, localAttrs := validProductArgs(t)

			actual, err := New(localID, localAttrs, testCreatedAt)
			require.NoError(t, err)

			expectedDescription, expectedThreshold := *localAttrs.Description, *localAttrs.StockWarningThreshold
			expectedPublishedAt := *localAttrs.PublishedAt
			expectedImages := slices.Clone(localAttrs.Images)

			*localAttrs.Description = "書き換え後の説明"
			*localAttrs.StockWarningThreshold = minThreshold
			*localAttrs.PublishedAt = time.Time{}
			localAttrs.Images[0] = mustImage(t, "mutated_image_id", "products/mutated.png", 9)

			require.NotNil(t, actual.Description())
			assert.Equal(t, expectedDescription, *actual.Description())
			require.NotNil(t, actual.StockWarningThreshold())
			assert.Equal(t, expectedThreshold, *actual.StockWarningThreshold())
			require.NotNil(t, actual.PublishedAt())
			assert.Equal(t, expectedPublishedAt, *actual.PublishedAt())
			assert.Equal(t, expectedImages, actual.Images())
		})

		t.Run("description・stockWarningThreshold・publishedAtがnilで画像が空でも生成される", func(t *testing.T) {
			t.Parallel()

			cleared := attrs
			cleared.Description = nil
			cleared.StockWarningThreshold = nil
			cleared.PublishedAt = nil
			cleared.Images = nil

			actual, err := New(id, cleared, testCreatedAt)
			require.NoError(t, err)
			assert.Nil(t, actual.Description())
			assert.Nil(t, actual.StockWarningThreshold())
			assert.Nil(t, actual.PublishedAt())
			assert.Empty(t, actual.Images())
		})

		t.Run("priceとquantityが0でも生成される", func(t *testing.T) {
			t.Parallel()

			minimal := attrs
			minimal.Price = mustPrice(t, "0")
			minimal.Quantity = minQuantity
			minimal.StockWarningThreshold = ptr.To(minThreshold)

			actual, err := New(id, minimal, testCreatedAt)
			require.NoError(t, err)
			assert.Equal(t, "0", actual.Price().String())
			assert.Equal(t, minQuantity, actual.Quantity())
		})

		t.Run("画像の枚数がちょうど上限の場合、Productエンティティが生成される", func(t *testing.T) {
			t.Parallel()

			atLimit := attrs
			atLimit.Images = imagesOfCount(t, maxImages)

			actual, err := New(id, atLimit, testCreatedAt)

			require.NoError(t, err)
			assert.Len(t, actual.Images(), maxImages)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			actual, err := New(uuid.UUID{}, attrs, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("廃番かつ公開の場合、ErrDiscontinuedCannotBePublishedを返しdetailsにpublishedAtを載せる", func(t *testing.T) {
			t.Parallel()

			id, invalid := validProductArgs(t)
			invalid.DiscontinuedAt = ptr.To(testCreatedAt)

			actual, err := New(id, invalid, testCreatedAt)

			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrDiscontinuedCannotBePublished)

			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldPublishedAt}, meta.Details())
		})

		t.Run("createdAtがゼロ値の場合、ErrInvalidCreatedAtを返す", func(t *testing.T) {
			t.Parallel()
			actual, err := New(id, attrs, time.Time{})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidCreatedAt)
		})

		t.Run("nameが空の場合、ErrInvalidNameを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Name = ""
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidName)
		})

		t.Run("nameが最大長を超える場合、ErrInvalidNameを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Name = strings.Repeat("あ", maxNameLength+1)
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidName)
		})

		t.Run("quantityが負数の場合、ErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Quantity = minQuantity - 1
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidQuantity)
		})

		t.Run("stockWarningThresholdが負数の場合、ErrInvalidStockWarningThresholdを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.StockWarningThreshold = ptr.To(minThreshold - 1)
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStockWarningThreshold)
		})

		t.Run("stockWarningThresholdが上限超過の場合、ErrInvalidStockWarningThresholdを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.StockWarningThreshold = ptr.To(maxThreshold)
			*invalid.StockWarningThreshold++
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStockWarningThreshold)
		})

		t.Run("statusがゼロ値の場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Status = StatusRef{}
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("categoryがゼロ値の場合、ErrInvalidCategoryIDを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Category = CategoryRef{}
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidCategoryID)
		})

		t.Run("画像の表示順が重複する場合、ErrDuplicateImageDisplaySortを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Images = []Image{
				mustImage(t, "duplicated_image_1", "products/a.png", 1),
				mustImage(t, "duplicated_image_2", "products/b.png", 1),
			}
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrDuplicateImageDisplaySort)
		})

		t.Run("画像パスが空の場合、ErrInvalidImagePathを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Images = []Image{mustImage(t, "empty_path_image", "", 1)}
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidImagePath)
		})

		t.Run("画像の枚数が上限を超える場合、ErrTooManyImagesを返す", func(t *testing.T) {
			t.Parallel()
			invalid := attrs
			invalid.Images = imagesOfCount(t, maxImages+1)
			actual, err := New(id, invalid, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrTooManyImages)
		})
	})
}

func TestReconstruct(t *testing.T) {
	t.Parallel()

	id, attrs := validProductArgs(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("永続化済みのバージョンと全フィールドを保持したProductエンティティが再構築される", func(t *testing.T) {
			t.Parallel()

			version := initialVersion + 1
			actual, err := Reconstruct(id, attrs, version, testCreatedAt)
			require.NoError(t, err)
			assert.Equal(t, id, actual.ID())
			assert.Equal(t, attrs.Name, actual.Name())
			assert.Equal(t, attrs.Description, actual.Description())
			assert.Equal(t, attrs.Price, actual.Price())
			assert.Equal(t, attrs.Quantity, actual.Quantity())
			assert.Equal(t, attrs.StockWarningThreshold, actual.StockWarningThreshold())
			assert.Equal(t, attrs.Status, actual.Status())
			assert.Equal(t, attrs.Category, actual.Category())
			assert.Equal(t, attrs.PublishedAt, actual.PublishedAt())
			assert.Equal(t, attrs.Images, actual.Images())
			assert.Equal(t, version, actual.Version())
		})

		t.Run("永続化済みの画像がちょうど上限の場合、Productエンティティが再構築される", func(t *testing.T) {
			t.Parallel()

			atLimit := attrs
			atLimit.Images = imagesOfCount(t, maxImages)

			actual, err := Reconstruct(id, atLimit, initialVersion, testCreatedAt)

			require.NoError(t, err)
			assert.Len(t, actual.Images(), maxImages)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("versionがinitialVersion未満の場合、ErrInvalidVersionを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := Reconstruct(id, attrs, initialVersion-1, testCreatedAt)
			require.ErrorIs(t, err, ErrInvalidVersion)
			assert.Nil(t, actual)
		})

		t.Run("createdAtがゼロ値の場合、ErrInvalidCreatedAtを返す", func(t *testing.T) {
			t.Parallel()
			actual, err := Reconstruct(id, attrs, initialVersion, time.Time{})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidCreatedAt)
		})

		t.Run("永続化済みの画像が枚数の上限を超える場合、ErrTooManyImagesを返す", func(t *testing.T) {
			t.Parallel()
			over := attrs
			over.Images = imagesOfCount(t, maxImages+1)
			actual, err := Reconstruct(id, over, initialVersion, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrTooManyImages)
		})
	})
}

func Test_newProduct(t *testing.T) {
	t.Parallel()

	id, attrs := validProductArgs(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("versionがinitialVersionと等しい場合、そのバージョンで生成される", func(t *testing.T) {
			t.Parallel()

			actual, err := newProduct(id, attrs, initialVersion, testCreatedAt)
			require.NoError(t, err)
			assert.Equal(t, initialVersion, actual.Version())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("versionがinitialVersion未満の場合、ErrInvalidVersionを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := newProduct(id, attrs, initialVersion-1, testCreatedAt)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidVersion)
		})
	})
}

func Test_validateQuantity(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("最小値の場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateQuantity(minQuantity))
		})

		t.Run("最大値の場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateQuantity(maxQuantity))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("最小値未満の場合、ErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateQuantity(minQuantity-1), ErrInvalidQuantity)
		})

		t.Run("最大値を超える場合、ErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateQuantity(maxQuantity+1), ErrInvalidQuantity)
		})
	})
}

func Test_validateDiscontinuedAt(t *testing.T) {
	t.Parallel()

	discontinuedAt := ptr.To(time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC))
	publishedAt := ptr.To(time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC))

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("廃番でも公開でもない場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateDiscontinuedAt(nil, nil))
		})

		t.Run("公開のみの場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateDiscontinuedAt(nil, publishedAt))
		})

		t.Run("廃番のみの場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateDiscontinuedAt(discontinuedAt, nil))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("廃番かつ公開の場合、ErrDiscontinuedCannotBePublishedを返し識別子は付けない", func(t *testing.T) {
			t.Parallel()

			err := validateDiscontinuedAt(discontinuedAt, publishedAt)

			require.ErrorIs(t, err, ErrDiscontinuedCannotBePublished)
			_, ok := apperror.MetaFrom(err)
			assert.False(t, ok)
		})
	})
}

func Test_validateStockWarningThreshold(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nilの場合、未設定として扱いエラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStockWarningThreshold(nil))
		})

		t.Run("最小値の場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStockWarningThreshold(ptr.To(minThreshold)))
		})

		t.Run("最大値の場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStockWarningThreshold(ptr.To(maxThreshold)))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("最小値未満の場合、ErrInvalidStockWarningThresholdを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStockWarningThreshold(ptr.To(minThreshold-1)), ErrInvalidStockWarningThreshold)
		})

		t.Run("最大値を超える場合、ErrInvalidStockWarningThresholdを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStockWarningThreshold(ptr.To(maxThreshold+1)), ErrInvalidStockWarningThreshold)
		})
	})
}

func Test_validateAttributes(t *testing.T) {
	t.Parallel()

	valid := Attributes{
		Name:                  "ワイヤレスイヤホン",
		Quantity:              1,
		StockWarningThreshold: ptr.To(1),
		Status:                mustStatusRef(t, "validate_attributes_status_id", "在庫あり"),
		Category:              mustCategoryRef(t, "validate_attributes_category_id", "電子機器"),
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nameが最小長の場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Name = strings.Repeat("あ", minNameLength)
			require.NoError(t, validateAttributes(attrs))
		})

		t.Run("nameが最大長の場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Name = strings.Repeat("あ", maxNameLength)
			require.NoError(t, validateAttributes(attrs))
		})

		t.Run("quantityとstockWarningThresholdが最小値の場合、エラーを返さない", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Quantity = minQuantity
			attrs.StockWarningThreshold = ptr.To(minThreshold)
			require.NoError(t, validateAttributes(attrs))
		})

		t.Run("stockWarningThresholdがnilの場合、閾値検証をスキップしてエラーを返さない", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Quantity = minQuantity
			attrs.StockWarningThreshold = nil
			require.NoError(t, validateAttributes(attrs))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nameが最小長未満の場合、ErrInvalidNameを返す", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Name = ""

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidName)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldName}, meta.Details())
		})

		t.Run("nameが最大長を超える場合、ErrInvalidNameを返す", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Name = strings.Repeat("あ", maxNameLength+1)
			require.ErrorIs(t, validateAttributes(attrs), ErrInvalidName)
		})

		t.Run("quantityが最小値未満の場合、ErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Quantity = minQuantity - 1

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidQuantity)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldQuantity}, meta.Details())
		})

		t.Run("stockWarningThresholdが最小値未満の場合、ErrInvalidStockWarningThresholdを返す", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Quantity = minQuantity
			attrs.StockWarningThreshold = ptr.To(minThreshold - 1)

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidStockWarningThreshold)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldStockWarningThreshold}, meta.Details())
		})

		t.Run("stockWarningThresholdが最大値を超える場合、ErrInvalidStockWarningThresholdを返す", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.StockWarningThreshold = ptr.To(maxThreshold + 1)
			require.ErrorIs(t, validateAttributes(attrs), ErrInvalidStockWarningThreshold)
		})

		t.Run("statusがゼロ値の場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Status = StatusRef{}

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidStatusID)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldStatusID}, meta.Details())
		})

		t.Run("categoryがゼロ値の場合、ErrInvalidCategoryIDを返す", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Category = CategoryRef{}

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidCategoryID)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldCategoryID}, meta.Details())
		})

		t.Run("画像の不変条件違反を伝播する", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Images = []Image{mustImage(t, "out_of_range_image", "products/a.png", maxImageDisplaySort+1)}

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidImageDisplaySort)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldImages}, meta.Details())
		})

		t.Run("画像IDの欠落はサーバ内部の不正なので識別子を付けない", func(t *testing.T) {
			t.Parallel()
			attrs := valid
			attrs.Images = []Image{{}}

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidID)
			_, ok := apperror.MetaFrom(err)
			assert.False(t, ok)
		})

		t.Run("廃番と公開の同時成立が他項目の違反を握りつぶさない", func(t *testing.T) {
			t.Parallel()
			at := time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC)
			attrs := valid
			attrs.Name = ""
			attrs.DiscontinuedAt = ptr.To(at)
			attrs.PublishedAt = ptr.To(at)
			attrs.Images = []Image{mustImage(t, "combined_image", "products/a.png", maxImageDisplaySort+1)}

			err := validateAttributes(attrs)
			require.ErrorIs(t, err, ErrInvalidName)
			require.ErrorIs(t, err, ErrDiscontinuedCannotBePublished)
			require.ErrorIs(t, err, ErrInvalidImageDisplaySort)

			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.ElementsMatch(t, []string{FieldName, FieldPublishedAt, FieldImages}, meta.Details())
		})
	})
}

func TestProduct_Update(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全属性が有効な場合、属性が更新されバージョンは進まない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			attrs := updatedProductAttributes(t)

			require.NoError(t, p.Update(attrs))
			assert.Equal(t, attrs.Name, p.Name())
			assert.Equal(t, attrs.Description, p.Description())
			assert.Equal(t, attrs.Price, p.Price())
			assert.Equal(t, attrs.Quantity, p.Quantity())
			assert.Equal(t, attrs.StockWarningThreshold, p.StockWarningThreshold())
			assert.Equal(t, attrs.Status, p.Status())
			assert.Equal(t, attrs.Category, p.Category())
			assert.Equal(t, attrs.PublishedAt, p.PublishedAt())
			assert.Equal(t, attrs.Images, p.Images())
			assert.Equal(t, initialVersion, p.Version())
		})

		t.Run("廃番日時を設定した場合、エンティティへ反映される", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			attrs := updatedProductAttributes(t)
			attrs.PublishedAt = nil
			attrs.DiscontinuedAt = ptr.To(testCreatedAt)

			require.NoError(t, p.Update(attrs))

			require.NotNil(t, p.DiscontinuedAt())
			assert.Equal(t, testCreatedAt, *p.DiscontinuedAt())
			assert.True(t, p.IsDiscontinued())
		})

		t.Run("画像の枚数がちょうど上限の場合、更新される", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			attrs := updatedProductAttributes(t)
			attrs.Images = imagesOfCount(t, maxImages)

			require.NoError(t, p.Update(attrs))
			assert.Len(t, p.Images(), maxImages)
		})

		t.Run("description・stockWarningThreshold・publishedAtをnil、画像を空で更新した場合、未設定になる", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			attrs := updatedProductAttributes(t)
			attrs.Description = nil
			attrs.StockWarningThreshold = nil
			attrs.PublishedAt = nil
			attrs.Images = nil

			require.NoError(t, p.Update(attrs))
			assert.Nil(t, p.Description())
			assert.Nil(t, p.StockWarningThreshold())
			assert.Nil(t, p.PublishedAt())
			assert.Empty(t, p.Images())
		})

		t.Run("更新後に引数のポインタを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			attrs := updatedProductAttributes(t)

			require.NoError(t, p.Update(attrs))

			expectedDescription, expectedThreshold := *attrs.Description, *attrs.StockWarningThreshold
			expectedPublishedAt := *attrs.PublishedAt
			expectedImages := slices.Clone(attrs.Images)

			*attrs.Description = "書き換え後の説明"
			*attrs.StockWarningThreshold = minThreshold
			*attrs.PublishedAt = time.Time{}
			attrs.Images[0] = mustImage(t, "mutated_image_id", "products/mutated.png", 9)

			require.NotNil(t, p.Description())
			assert.Equal(t, expectedDescription, *p.Description())
			require.NotNil(t, p.StockWarningThreshold())
			assert.Equal(t, expectedThreshold, *p.StockWarningThreshold())
			require.NotNil(t, p.PublishedAt())
			assert.Equal(t, expectedPublishedAt, *p.PublishedAt())
			assert.Equal(t, expectedImages, p.Images())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("廃番の商品に公開日時を設定した場合、ErrDiscontinuedCannotBePublishedを返しエンティティを一切変更しない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			discontinued := updatedProductAttributes(t)
			discontinued.PublishedAt = nil
			discontinued.DiscontinuedAt = ptr.To(testCreatedAt)
			require.NoError(t, p.Update(discontinued))

			snapshot := *p
			republish := discontinued
			republish.PublishedAt = ptr.To(testCreatedAt)

			err := p.Update(republish)

			require.ErrorIs(t, err, ErrDiscontinuedCannotBePublished)
			assert.Equal(t, snapshot, *p)

			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldPublishedAt}, meta.Details())
		})

		t.Run("quantityが最小値未満の場合、ErrInvalidQuantityを返しエンティティを一切変更しない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			snapshot := *p
			attrs := updatedProductAttributes(t)
			attrs.Quantity = minQuantity - 1

			require.ErrorIs(t, p.Update(attrs), ErrInvalidQuantity)
			assert.Equal(t, snapshot, *p)
		})

		t.Run("nameが最大長を超える場合、ErrInvalidNameを返しエンティティを一切変更しない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			snapshot := *p
			attrs := updatedProductAttributes(t)
			attrs.Name = strings.Repeat("あ", maxNameLength+1)

			require.ErrorIs(t, p.Update(attrs), ErrInvalidName)
			assert.Equal(t, snapshot, *p)
		})

		t.Run("画像の枚数が上限を超える場合、ErrTooManyImagesを返しエンティティを一切変更しない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			snapshot := *p
			attrs := updatedProductAttributes(t)
			attrs.Images = imagesOfCount(t, maxImages+1)

			require.ErrorIs(t, p.Update(attrs), ErrTooManyImages)
			assert.Equal(t, snapshot, *p)
		})
	})
}

func TestProduct_AdjustStock(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("正のdeltaを渡した場合、在庫が加算されバージョンは進まない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			before := p.Quantity()

			require.NoError(t, p.AdjustStock(50))
			assert.Equal(t, before+50, p.Quantity())
			assert.Equal(t, initialVersion, p.Version())
		})

		t.Run("負のdeltaを渡した場合、在庫が減算される", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			before := p.Quantity()

			require.NoError(t, p.AdjustStock(-30))
			assert.Equal(t, before-30, p.Quantity())
		})

		t.Run("deltaが0の場合、在庫は変わらない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			before := p.Quantity()

			require.NoError(t, p.AdjustStock(0))
			assert.Equal(t, before, p.Quantity())
		})

		t.Run("減算後がちょうど最小値になる場合、在庫が最小値になる", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)

			require.NoError(t, p.AdjustStock(minQuantity-p.Quantity()))
			assert.Equal(t, minQuantity, p.Quantity())
		})

		t.Run("加算後がちょうど最大値になる場合、在庫が最大値になる", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)

			require.NoError(t, p.AdjustStock(maxQuantity-p.Quantity()))
			assert.Equal(t, maxQuantity, p.Quantity())
		})

		t.Run("在庫以外の属性は変わらない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			snapshot := *p

			require.NoError(t, p.AdjustStock(1))

			snapshot.quantity = p.Quantity()
			assert.Equal(t, snapshot, *p)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("減算後が最小値未満になる場合、ErrInvalidQuantityを返しエンティティを一切変更しない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			snapshot := *p

			err := p.AdjustStock(minQuantity - p.Quantity() - 1)
			require.ErrorIs(t, err, ErrInvalidQuantity)
			require.ErrorIs(t, err, apperror.ErrValidation)
			assert.Equal(t, snapshot, *p)
		})

		t.Run("加算後が最大値を超える場合、ErrInvalidQuantityを返しエンティティを一切変更しない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			snapshot := *p

			err := p.AdjustStock(maxQuantity - p.Quantity() + 1)
			require.ErrorIs(t, err, ErrInvalidQuantity)
			require.ErrorIs(t, err, apperror.ErrValidation)
			assert.Equal(t, snapshot, *p)
		})

		t.Run("識別子は付けない（呼び出し元が決める）", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)

			err := p.AdjustStock(-p.Quantity() - 1)
			require.ErrorIs(t, err, ErrInvalidQuantity)
			_, ok := apperror.MetaFrom(err)
			assert.False(t, ok)
		})
	})
}

func TestProduct_EnsureVersion(t *testing.T) {
	t.Parallel()

	id, attrs := validProductArgs(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("期待バージョンが現在のバージョンと一致する場合、nilを返す", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			require.NoError(t, p.EnsureVersion(initialVersion))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("期待バージョンが現在のバージョンと一致しない場合、Conflictに分類されるErrVersionConflictを返す", func(t *testing.T) {
			t.Parallel()

			p, err := Reconstruct(id, attrs, initialVersion+1, testCreatedAt)
			require.NoError(t, err)

			err = p.EnsureVersion(initialVersion)
			require.ErrorIs(t, err, ErrVersionConflict)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})
	})
}

func TestProduct_Price(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("サブセント精度を保持した価格スナップショットを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.Price = mustPrice(t, "19.995")

			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)
			assert.Equal(t, "19.995", p.Price().String())
			assert.True(t, p.Price().Decimal().Equal(decimaltestkit.MustParse(t, "19.995")))
		})
	})
}

func TestProduct_Version(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築で渡した楽観ロックのバージョンを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := Reconstruct(id, attrs, initialVersion+2, testCreatedAt)
			require.NoError(t, err)
			assert.Equal(t, initialVersion+2, p.Version())
		})
	})
}

func TestProduct_Category(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時のカテゴリ参照をIDと名称ごと返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Equal(t, attrs.Category, p.Category())
			assert.Equal(t, "電子機器", p.Category().Name())
		})
	})
}

func TestProduct_Description(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時の商品説明を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			require.NotNil(t, p.Description())
			assert.Equal(t, "ノイズキャンセリング対応", *p.Description())
		})

		t.Run("未設定の場合、nilを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.Description = nil
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Nil(t, p.Description())
		})

		t.Run("返り値のポインタを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			got := p.Description()
			*got = "書き換え後の説明"

			require.NotNil(t, p.Description())
			assert.NotEqual(t, *got, *p.Description())
			assert.Equal(t, "ノイズキャンセリング対応", *p.Description())
		})
	})
}

func TestProduct_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時のIDを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Equal(t, id, p.ID())
		})
	})
}

func TestProduct_Images(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時の画像を表示順の昇順で返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			images := p.Images()
			require.Len(t, images, 2)
			assert.Equal(t, "products/earphone.png", images[0].ImagePath())
			assert.Equal(t, 1, images[0].DisplaySort())
			assert.Equal(t, "products/earphone-sub.png", images[1].ImagePath())
			assert.Equal(t, 2, images[1].DisplaySort())
		})

		t.Run("表示順が降順で渡された場合でも昇順に並べ替えて返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.Images = []Image{
				mustImage(t, "product_image_id_2", "products/earphone-sub.png", 2),
				mustImage(t, "product_image_id_1", "products/earphone.png", 1),
			}
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			images := p.Images()
			require.Len(t, images, 2)
			assert.Equal(t, 1, images[0].DisplaySort())
			assert.Equal(t, 2, images[1].DisplaySort())
		})

		t.Run("未設定の場合、空を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.Images = nil
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Empty(t, p.Images())
		})

		t.Run("返り値のスライスを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			got := p.Images()
			got[0] = mustImage(t, "mutated_image_id", "products/mutated.png", 9)

			require.Len(t, p.Images(), 2)
			assert.Equal(t, "products/earphone.png", p.Images()[0].ImagePath())
		})
	})
}

func TestProduct_Name(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時の商品名を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Equal(t, "ワイヤレスイヤホン", p.Name())
		})
	})
}

func TestProduct_CreatedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時の登録日時を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Equal(t, testCreatedAt, p.CreatedAt())
		})

		t.Run("公開日時とは別の値を保持する", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			require.NotNil(t, p.PublishedAt())
			assert.NotEqual(t, *p.PublishedAt(), p.CreatedAt())
		})

		t.Run("再構築時は永続化されていた登録日時を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			persisted := time.Date(2024, time.June, 1, 12, 0, 0, 0, time.UTC)
			p, err := Reconstruct(id, attrs, initialVersion, persisted)
			require.NoError(t, err)

			assert.Equal(t, persisted, p.CreatedAt())
		})
	})
}

func TestProduct_PublishedAt(t *testing.T) {
	t.Parallel()

	publishedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時の公開日時を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			require.NotNil(t, p.PublishedAt())
			assert.Equal(t, publishedAt, *p.PublishedAt())
		})

		t.Run("未公開の場合、nilを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.PublishedAt = nil
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Nil(t, p.PublishedAt())
		})

		t.Run("返り値のポインタを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			got := p.PublishedAt()
			*got = time.Date(2030, time.December, 31, 0, 0, 0, 0, time.UTC)

			require.NotNil(t, p.PublishedAt())
			assert.NotEqual(t, *got, *p.PublishedAt())
			assert.Equal(t, publishedAt, *p.PublishedAt())
		})
	})
}

func TestProduct_Quantity(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時の在庫数を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Equal(t, 100, p.Quantity())
		})

		t.Run("在庫数が下限値の場合、その値を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.Quantity = minQuantity
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Equal(t, minQuantity, p.Quantity())
		})
	})
}

func TestProduct_Status(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時のステータス参照をIDと名称ごと返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Equal(t, attrs.Status, p.Status())
			assert.Equal(t, "在庫あり", p.Status().Name())
		})
	})
}

func TestProduct_StockWarningThreshold(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時の在庫警告閾値を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			require.NotNil(t, p.StockWarningThreshold())
			assert.Equal(t, 10, *p.StockWarningThreshold())
		})

		t.Run("未設定の場合、nilを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.StockWarningThreshold = nil
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Nil(t, p.StockWarningThreshold())
		})

		t.Run("返り値のポインタを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			got := p.StockWarningThreshold()
			*got = minThreshold

			require.NotNil(t, p.StockWarningThreshold())
			assert.NotEqual(t, *got, *p.StockWarningThreshold())
			assert.Equal(t, 10, *p.StockWarningThreshold())
		})
	})
}

func TestProduct_IsPublished(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("公開日時が設定されている場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.True(t, p.IsPublished())
		})

		t.Run("公開日時が未設定の場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.PublishedAt = nil
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.False(t, p.IsPublished())
		})
	})
}

func TestProduct_Discontinue(t *testing.T) {
	t.Parallel()

	discontinuedAt := time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("廃番日時を設定し、同時に公開を取り下げる", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			assert.True(t, p.IsPublished())

			require.NoError(t, p.Discontinue(discontinuedAt))

			assert.True(t, p.IsDiscontinued())
			require.NotNil(t, p.DiscontinuedAt())
			assert.Equal(t, discontinuedAt, *p.DiscontinuedAt())
			assert.False(t, p.IsPublished())
			assert.Nil(t, p.PublishedAt())
		})

		t.Run("既に廃番の場合、状態を変えずエラーも返さない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			require.NoError(t, p.Discontinue(discontinuedAt))
			snapshot := *p

			require.NoError(t, p.Discontinue(discontinuedAt.Add(24*time.Hour)))

			assert.Equal(t, snapshot, *p)
			require.NotNil(t, p.DiscontinuedAt())
			assert.Equal(t, discontinuedAt, *p.DiscontinuedAt())
		})

		t.Run("未公開の商品も廃番にできる", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.PublishedAt = nil
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			require.NoError(t, p.Discontinue(discontinuedAt))

			assert.True(t, p.IsDiscontinued())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nowがゼロ値の場合、ErrInvalidDiscontinuedAtを返しエンティティを一切変更しない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			snapshot := *p

			require.ErrorIs(t, p.Discontinue(time.Time{}), ErrInvalidDiscontinuedAt)
			assert.Equal(t, snapshot, *p)
		})
	})
}

func TestProduct_IsDiscontinued(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("廃番日時が設定されている場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.PublishedAt = nil
			attrs.DiscontinuedAt = ptr.To(testCreatedAt)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.True(t, p.IsDiscontinued())
		})

		t.Run("廃番日時が未設定の場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.False(t, p.IsDiscontinued())
		})
	})
}

func TestIsDiscontinued(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("廃番日時が設定されている場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, IsDiscontinued(ptr.To(time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC))))
		})

		t.Run("廃番日時が未設定の場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, IsDiscontinued(nil))
		})
	})
}

func TestProduct_DiscontinuedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("保持している廃番日時を返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.PublishedAt = nil
			attrs.DiscontinuedAt = ptr.To(testCreatedAt)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			require.NotNil(t, p.DiscontinuedAt())
			assert.Equal(t, testCreatedAt, *p.DiscontinuedAt())
		})

		t.Run("廃番でない場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.Nil(t, p.DiscontinuedAt())
		})

		t.Run("返り値のポインタを書き換えてもエンティティ内部は変わらない", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.PublishedAt = nil
			attrs.DiscontinuedAt = ptr.To(testCreatedAt)
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			got := p.DiscontinuedAt()
			*got = time.Date(2030, time.December, 31, 0, 0, 0, 0, time.UTC)

			require.NotNil(t, p.DiscontinuedAt())
			assert.NotEqual(t, *got, *p.DiscontinuedAt())
			assert.Equal(t, testCreatedAt, *p.DiscontinuedAt())
		})
	})
}

func TestIsPublished(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("公開日時が設定されている場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, IsPublished(ptr.To(time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC))))
		})

		t.Run("公開日時が未設定の場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, IsPublished(nil))
		})
	})
}

func TestProduct_PrimaryImage(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("画像を持つ場合、表示順が最小の画像を代表画像として返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.Images = []Image{
				mustImage(t, "primary_image_second", "products/second.png", 2),
				mustImage(t, "primary_image_first", "products/first.png", 1),
			}
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			actual, ok := p.PrimaryImage()

			require.True(t, ok)
			assert.Equal(t, "products/first.png", actual.ImagePath())
		})

		t.Run("画像を持たない場合、代表画像を持たない", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.Images = nil
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			actual, ok := p.PrimaryImage()

			assert.False(t, ok)
			assert.Empty(t, actual.ImagePath())
		})
	})
}

func TestProduct_IsLowStock(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("在庫が閾値を下回る場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.StockWarningThreshold = ptr.To(10)
			attrs.Quantity = 9
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.True(t, p.IsLowStock())
		})

		t.Run("在庫が閾値と等しい場合、境界を含むためtrueを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.StockWarningThreshold = ptr.To(10)
			attrs.Quantity = 10
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.True(t, p.IsLowStock())
		})

		t.Run("在庫が閾値を上回る場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.StockWarningThreshold = ptr.To(10)
			attrs.Quantity = 11
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.False(t, p.IsLowStock())
		})

		t.Run("閾値が未設定の場合、在庫が0でも警告対象を持たないためfalseを返す", func(t *testing.T) {
			t.Parallel()

			id, attrs := validProductArgs(t)
			attrs.StockWarningThreshold = nil
			attrs.Quantity = 0
			p, err := New(id, attrs, testCreatedAt)
			require.NoError(t, err)

			assert.False(t, p.IsLowStock())
		})
	})
}

func TestProduct_ensureDiscontinuationKept(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未廃番の商品は廃番日時が nil のままでも通る", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)

			require.NoError(t, p.ensureDiscontinuationKept(nil))
		})

		t.Run("未廃番から廃番への向きは拒まない", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)

			require.NoError(t, p.ensureDiscontinuationKept(ptr.To(testCreatedAt)))
		})

		t.Run("廃番済みの商品が廃番日時を保ったままなら通る", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			require.NoError(t, p.Discontinue(testCreatedAt))

			require.NoError(t, p.ensureDiscontinuationKept(p.DiscontinuedAt()))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("廃番済みの商品を未廃番へ戻そうとした場合、取り消し不可のエラーを返す", func(t *testing.T) {
			t.Parallel()

			p := newTestProduct(t)
			require.NoError(t, p.Discontinue(testCreatedAt))

			err := p.ensureDiscontinuationKept(nil)

			require.ErrorIs(t, err, ErrDiscontinuationIrreversible)
			require.ErrorIs(t, err, apperror.ErrValidation)

			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldDiscontinuedAt}, meta.Details())
		})
	})
}
