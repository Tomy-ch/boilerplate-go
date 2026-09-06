package cart

import (
	"fmt"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultExpiry は、テストで用いる既定の有効期限です。
var defaultExpiry = baseTime.Add(24 * time.Hour)

func newTestGuestCart(t *testing.T) *Cart {
	t.Helper()
	c, err := NewForGuest(uuidtestkit.NewTestFromSalt(t, "cart"), newTestSessionToken(t), defaultExpiry)
	require.NoError(t, err)
	return c
}

func newTestOwnedCart(t *testing.T) *Cart {
	t.Helper()
	c, err := NewForOwner(
		uuidtestkit.NewTestFromSalt(t, "cart"),
		Attributes{OwnerID: ptr.To(uuidtestkit.NewTestFromSalt(t, "owner")), ExpiresAt: defaultExpiry},
	)
	require.NoError(t, err)
	return c
}

// productIDsOf は、明細の商品 ID を順序どおりに取り出します。
func productIDsOf(items []CartItem) []uuid.UUID {
	ids := make([]uuid.UUID, len(items))
	for i, item := range items {
		ids[i] = item.ProductID()
	}
	return ids
}

func TestNewForGuest(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("セッショントークンを持ち所有者が未確定のカートを生成する", func(t *testing.T) {
			t.Parallel()
			id := uuidtestkit.NewTestFromSalt(t, "cart")
			token := newTestSessionToken(t)

			c, err := NewForGuest(id, token, defaultExpiry)

			require.NoError(t, err)
			assert.Equal(t, id, c.ID())
			assert.Nil(t, c.OwnerID())
			require.NotNil(t, c.SessionToken())
			assert.Equal(t, token.Value(), c.SessionToken().Value())
			assert.Equal(t, defaultExpiry, c.ExpiresAt())
			assert.True(t, c.IsEmpty())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDが未設定ならErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			_, err := NewForGuest(uuid.UUID{}, newTestSessionToken(t), defaultExpiry)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("有効期限がゼロ値ならErrInvalidExpiresAtを返す", func(t *testing.T) {
			t.Parallel()
			_, err := NewForGuest(uuidtestkit.NewTestFromSalt(t, "cart"), newTestSessionToken(t), time.Time{})
			require.ErrorIs(t, err, ErrInvalidExpiresAt)
		})
	})
}

func TestNewForOwner(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("所有者を持ちセッショントークンを持たないカートを生成する", func(t *testing.T) {
			t.Parallel()
			id := uuidtestkit.NewTestFromSalt(t, "cart")
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")

			c, err := NewForOwner(id, Attributes{OwnerID: &ownerID, ExpiresAt: defaultExpiry})

			require.NoError(t, err)
			assert.Equal(t, id, c.ID())
			require.NotNil(t, c.OwnerID())
			assert.Equal(t, ownerID, *c.OwnerID())
			assert.Nil(t, c.SessionToken())
			assert.True(t, c.IsEmpty())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDが未設定ならErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			_, err := NewForOwner(uuid.UUID{}, Attributes{OwnerID: ptr.To(uuidtestkit.NewTestFromSalt(t, "owner")), ExpiresAt: defaultExpiry})
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("所有者IDが未設定ならErrInvalidUserIDを返す", func(t *testing.T) {
			t.Parallel()
			_, err := NewForOwner(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{OwnerID: &uuid.UUID{}, ExpiresAt: defaultExpiry})
			require.ErrorIs(t, err, ErrInvalidUserID)
		})

		t.Run("有効期限がゼロ値ならErrInvalidExpiresAtを返す", func(t *testing.T) {
			t.Parallel()
			_, err := NewForOwner(
				uuidtestkit.NewTestFromSalt(t, "cart"),
				Attributes{OwnerID: ptr.To(uuidtestkit.NewTestFromSalt(t, "owner")), ExpiresAt: time.Time{}},
			)
			require.ErrorIs(t, err, ErrInvalidExpiresAt)
		})
	})
}

func TestReconstruct(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細と監査時刻を含めて再構築する", func(t *testing.T) {
			t.Parallel()
			id := uuidtestkit.NewTestFromSalt(t, "cart")
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			item := newTestCartItem(t, "a", 2)

			c, err := Reconstruct(id, Attributes{
				OwnerID:   &ownerID,
				Items:     []CartItem{item},
				ExpiresAt: defaultExpiry,
				CreatedAt: baseTime,
				UpdatedAt: baseTime,
			})

			require.NoError(t, err)
			assert.Equal(t, id, c.ID())
			assert.Equal(t, baseTime, c.CreatedAt())
			assert.Equal(t, baseTime, c.UpdatedAt())
			require.Len(t, c.Items(), 1)
			assert.Equal(t, item.ProductID(), c.Items()[0].ProductID())
		})

		t.Run("監査時刻が未設定でも再構築できる", func(t *testing.T) {
			t.Parallel()
			token := newTestSessionToken(t)

			c, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				SessionToken: &token,
				ExpiresAt:    defaultExpiry,
			})

			require.NoError(t, err)
			assert.True(t, c.CreatedAt().IsZero())
		})

		t.Run("上限ちょうどの明細数を受け入れる", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			items := make([]CartItem, 0, maxItems)
			for i := range maxItems {
				items = append(items, newTestCartItem(t, fmt.Sprintf("bulk_%d", i), 1))
			}

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     items,
				ExpiresAt: defaultExpiry,
			})

			require.NoError(t, err)
		})

		t.Run("渡された明細スライスを共有しない", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			items := []CartItem{newTestCartItem(t, "a", 2)}

			c, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     items,
				ExpiresAt: defaultExpiry,
			})
			require.NoError(t, err)

			items[0] = newTestCartItem(t, "b", 9)

			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "product_a"), c.Items()[0].ProductID())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("所有者とトークンの両方が未設定ならErrInvalidOwnerを返す", func(t *testing.T) {
			t.Parallel()
			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{ExpiresAt: defaultExpiry})
			require.ErrorIs(t, err, ErrInvalidOwner)
		})

		t.Run("所有者とトークンの両方が設定済みならErrInvalidOwnerを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			token := newTestSessionToken(t)

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:      &ownerID,
				SessionToken: &token,
				ExpiresAt:    defaultExpiry,
			})
			require.ErrorIs(t, err, ErrInvalidOwner)
		})

		t.Run("所有者IDがゼロ値ならErrInvalidUserIDを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuid.UUID{}

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				ExpiresAt: defaultExpiry,
			})
			require.ErrorIs(t, err, ErrInvalidUserID)
		})

		t.Run("有効期限が作成日時と同時刻ならErrInvalidExpiresAtを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				ExpiresAt: baseTime,
				CreatedAt: baseTime,
			})
			require.ErrorIs(t, err, ErrInvalidExpiresAt)
		})

		t.Run("明細IDが未設定ならErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			item := NewCartItem(uuid.UUID{}, CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "product"),
				Quantity:  1,
			})

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     []CartItem{item},
				ExpiresAt: defaultExpiry,
			})
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("明細の商品IDが未設定ならErrInvalidProductIDを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			item := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item"), CartItemAttributes{Quantity: 1})

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     []CartItem{item},
				ExpiresAt: defaultExpiry,
			})
			require.ErrorIs(t, err, ErrInvalidProductID)
		})

		t.Run("明細の数量が下限未満ならErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     []CartItem{newTestCartItem(t, "a", minQuantity-1)},
				ExpiresAt: defaultExpiry,
			})
			require.ErrorIs(t, err, ErrInvalidQuantity)
		})

		t.Run("明細の数量が上限超過ならErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     []CartItem{newTestCartItem(t, "a", maxQuantityPerItem+1)},
				ExpiresAt: defaultExpiry,
			})
			require.ErrorIs(t, err, ErrInvalidQuantity)
		})

		t.Run("同一商品の明細が重複するならErrDuplicateProductIDを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			first := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_1"), CartItemAttributes{
				ProductID: productID, Quantity: 1,
			})
			second := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_2"), CartItemAttributes{
				ProductID: productID, Quantity: 1,
			})

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     []CartItem{first, second},
				ExpiresAt: defaultExpiry,
			})
			require.ErrorIs(t, err, ErrDuplicateProductID)
		})

		t.Run("明細数が上限を超えるならErrTooManyItemsを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			items := make([]CartItem, 0, maxItems+1)
			for i := range maxItems + 1 {
				items = append(items, newTestCartItem(t, fmt.Sprintf("bulk_%d", i), 1))
			}

			_, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				Items:     items,
				ExpiresAt: defaultExpiry,
			})
			require.ErrorIs(t, err, ErrTooManyItems)
		})
	})
}

func Test_newCart(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("3つの入口が同じ検証を通る", func(t *testing.T) {
			t.Parallel()
			_, guestErr := NewForGuest(uuid.UUID{}, newTestSessionToken(t), defaultExpiry)
			_, ownerErr := NewForOwner(uuid.UUID{}, Attributes{OwnerID: ptr.To(uuidtestkit.NewTestFromSalt(t, "owner")), ExpiresAt: defaultExpiry})
			_, reconstructErr := Reconstruct(uuid.UUID{}, Attributes{ExpiresAt: defaultExpiry})

			require.ErrorIs(t, guestErr, ErrInvalidID)
			require.ErrorIs(t, ownerErr, ErrInvalidID)
			require.ErrorIs(t, reconstructErr, ErrInvalidID)
		})

		t.Run("渡された所有者ポインタを共有しない", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")

			c, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID:   &ownerID,
				ExpiresAt: defaultExpiry,
			})
			require.NoError(t, err)

			ownerID = uuidtestkit.NewTestFromSalt(t, "attacker")

			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "owner"), *c.OwnerID())
		})
	})
}

func Test_validateOwnership(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("所有者だけが設定されていれば通る", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			require.NoError(t, validateOwnership(&ownerID, nil))
		})

		t.Run("トークンだけが設定されていれば通る", func(t *testing.T) {
			t.Parallel()
			token := newTestSessionToken(t)
			require.NoError(t, validateOwnership(nil, &token))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("両方が未設定ならErrInvalidOwnerを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateOwnership(nil, nil), ErrInvalidOwner)
		})

		t.Run("両方が設定済みならErrInvalidOwnerを返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			token := newTestSessionToken(t)
			require.ErrorIs(t, validateOwnership(&ownerID, &token), ErrInvalidOwner)
		})

		t.Run("検証を経ていないトークンはErrInvalidSessionTokenを返す", func(t *testing.T) {
			t.Parallel()
			token := SessionToken{}
			require.ErrorIs(t, validateOwnership(nil, &token), ErrInvalidSessionToken)
		})
	})
}

func Test_validateItems(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("空の集合を許容する", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateItems(nil))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("上限超過は個々の明細を見る前に検出する", func(t *testing.T) {
			t.Parallel()
			items := make([]CartItem, maxItems+1)
			require.ErrorIs(t, validateItems(items), ErrTooManyItems)
		})
	})
}

func Test_validateQuantity(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("下限と上限の境界を許容する", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateQuantity(minQuantity))
			require.NoError(t, validateQuantity(maxQuantityPerItem))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("境界の外側はErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateQuantity(minQuantity-1), ErrInvalidQuantity)
			require.ErrorIs(t, validateQuantity(maxQuantityPerItem+1), ErrInvalidQuantity)
		})
	})
}

func TestCart_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成に用いたIDを返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "cart"), newTestGuestCart(t).ID())
		})
	})
}

func TestCart_OwnerID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("所有者が未確定ならnilを返す", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, newTestGuestCart(t).OwnerID())
		})

		t.Run("戻り値を書き換えても内部状態は変わらない", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)

			got := c.OwnerID()
			require.NotNil(t, got)
			*got = uuidtestkit.NewTestFromSalt(t, "attacker")

			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "owner"), *c.OwnerID())
		})
	})
}

func TestCart_SessionToken(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("所有者が確定済みならnilを返す", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, newTestOwnedCart(t).SessionToken())
		})

		t.Run("戻り値を書き換えても内部状態は変わらない", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)

			got := c.SessionToken()
			require.NotNil(t, got)
			*got = SessionToken{value: "tampered"}

			assert.Equal(t, validTokenString, c.SessionToken().Value())
		})
	})
}

func TestCart_Items(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細が無ければ空を返す", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, newTestGuestCart(t).Items())
		})

		t.Run("戻り値を書き換えても内部状態は変わらない", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: productID, Quantity: 1}, baseTime),
			)

			got := c.Items()
			got[0] = newTestCartItem(t, "other", 9)

			assert.Equal(t, productID, c.Items()[0].ProductID())
		})
	})
}

func TestCart_ExpiresAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成に用いた有効期限を返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, defaultExpiry, newTestGuestCart(t).ExpiresAt())
		})
	})
}

func TestCart_CreatedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成直後はゼロ値を返す", func(t *testing.T) {
			t.Parallel()
			assert.True(t, newTestGuestCart(t).CreatedAt().IsZero())
		})

		t.Run("再構築時に設定した作成日時を返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			c, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID: &ownerID, ExpiresAt: defaultExpiry, CreatedAt: baseTime,
			})
			require.NoError(t, err)

			assert.Equal(t, baseTime, c.CreatedAt())
		})
	})
}

func TestCart_UpdatedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成直後はゼロ値を返す", func(t *testing.T) {
			t.Parallel()
			assert.True(t, newTestGuestCart(t).UpdatedAt().IsZero())
		})

		t.Run("再構築時に設定した更新日時を返す", func(t *testing.T) {
			t.Parallel()
			ownerID := uuidtestkit.NewTestFromSalt(t, "owner")
			c, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cart"), Attributes{
				OwnerID: &ownerID, ExpiresAt: defaultExpiry, UpdatedAt: baseTime,
			})
			require.NoError(t, err)

			assert.Equal(t, baseTime, c.UpdatedAt())
		})
	})
}

func TestCart_SetItem(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("新しい商品を明細へ追加する", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			itemID := uuidtestkit.NewTestFromSalt(t, "item")
			productID := uuidtestkit.NewTestFromSalt(t, "product")

			require.NoError(t, c.SetItem(SetItemAttributes{ItemID: itemID, ProductID: productID, Quantity: 3}, baseTime))

			require.Len(t, c.Items(), 1)
			assert.Equal(t, itemID, c.Items()[0].ID())
			assert.Equal(t, productID, c.Items()[0].ProductID())
			assert.Equal(t, 3, c.Items()[0].Quantity())
			assert.Equal(t, baseTime, c.Items()[0].AddedAt())
		})

		t.Run("既存商品は数量を置換し明細IDと追加時刻を保つ", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			itemID := uuidtestkit.NewTestFromSalt(t, "item")
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			require.NoError(t, c.SetItem(SetItemAttributes{ItemID: itemID, ProductID: productID, Quantity: 3}, baseTime))

			later := baseTime.Add(time.Hour)
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "other_item"), ProductID: productID, Quantity: 5}, later),
			)

			require.Len(t, c.Items(), 1)
			assert.Equal(t, 5, c.Items()[0].Quantity())
			assert.Equal(t, itemID, c.Items()[0].ID())
			assert.Equal(t, baseTime, c.Items()[0].AddedAt())
		})

		t.Run("数量の境界値を受け入れる", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)

			require.NoError(
				t,
				c.SetItem(
					SetItemAttributes{
						ItemID:    uuidtestkit.NewTestFromSalt(t, "item_min"),
						ProductID: uuidtestkit.NewTestFromSalt(t, "product_min"),
						Quantity:  minQuantity,
					},
					baseTime,
				),
			)
			require.NoError(
				t,
				c.SetItem(
					SetItemAttributes{
						ItemID:    uuidtestkit.NewTestFromSalt(t, "item_max"),
						ProductID: uuidtestkit.NewTestFromSalt(t, "product_max"),
						Quantity:  maxQuantityPerItem,
					},
					baseTime,
				),
			)
		})

		t.Run("明細数が上限ちょうどでも既存商品の置換はできる", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			for i := range maxItems {
				require.NoError(
					t,
					c.SetItem(
						SetItemAttributes{
							ItemID:    uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("item_%d", i)),
							ProductID: uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("product_%d", i)),
							Quantity:  1,
						},
						baseTime,
					),
				)
			}

			err := c.SetItem(
				SetItemAttributes{
					ItemID:    uuidtestkit.NewTestFromSalt(t, "item_0"),
					ProductID: uuidtestkit.NewTestFromSalt(t, "product_0"),
					Quantity:  2,
				},
				baseTime,
			)

			require.NoError(t, err)
			assert.Len(t, c.Items(), maxItems)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("商品IDが未設定ならErrInvalidProductIDを返す", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			err := c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: uuid.UUID{}, Quantity: 1}, baseTime)
			require.ErrorIs(t, err, ErrInvalidProductID)
		})

		t.Run("数量0はErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			err := c.SetItem(
				SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: uuidtestkit.NewTestFromSalt(t, "product"), Quantity: 0},
				baseTime,
			)
			require.ErrorIs(t, err, ErrInvalidQuantity)
			assert.True(t, c.IsEmpty())
		})

		t.Run("数量が上限超過ならErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			err := c.SetItem(
				SetItemAttributes{
					ItemID:    uuidtestkit.NewTestFromSalt(t, "item"),
					ProductID: uuidtestkit.NewTestFromSalt(t, "product"),
					Quantity:  maxQuantityPerItem + 1,
				},
				baseTime,
			)
			require.ErrorIs(t, err, ErrInvalidQuantity)
		})

		t.Run("新規追加で明細IDが未設定ならErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			err := c.SetItem(SetItemAttributes{ItemID: uuid.UUID{}, ProductID: uuidtestkit.NewTestFromSalt(t, "product"), Quantity: 1}, baseTime)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("明細数が上限に達していれば新規追加はErrTooManyItemsを返す", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			for i := range maxItems {
				require.NoError(
					t,
					c.SetItem(
						SetItemAttributes{
							ItemID:    uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("item_%d", i)),
							ProductID: uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("product_%d", i)),
							Quantity:  1,
						},
						baseTime,
					),
				)
			}

			err := c.SetItem(
				SetItemAttributes{
					ItemID:    uuidtestkit.NewTestFromSalt(t, "item_over"),
					ProductID: uuidtestkit.NewTestFromSalt(t, "product_over"),
					Quantity:  1,
				},
				baseTime,
			)

			require.ErrorIs(t, err, ErrTooManyItems)
			assert.Len(t, c.Items(), maxItems)
		})

		t.Run("数量が範囲外の場合、quantity を名指しする", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)

			err := c.SetItem(
				SetItemAttributes{
					ItemID:    uuidtestkit.NewTestFromSalt(t, "item_bad_quantity"),
					ProductID: uuidtestkit.NewTestFromSalt(t, "product_bad_quantity"),
					Quantity:  0,
				},
				baseTime,
			)

			require.ErrorIs(t, err, ErrInvalidQuantity)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldQuantity}, meta.Details())
		})
	})
}

func TestCart_RemoveItem(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("指定商品の明細を取り除く", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			target := uuidtestkit.NewTestFromSalt(t, "product_a")
			other := uuidtestkit.NewTestFromSalt(t, "product_b")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item_a"), ProductID: target, Quantity: 1}, baseTime),
			)
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item_b"), ProductID: other, Quantity: 1}, baseTime),
			)

			require.NoError(t, c.RemoveItem(target))

			require.Len(t, c.Items(), 1)
			assert.Equal(t, other, c.Items()[0].ProductID())
		})

		t.Run("該当明細が無くても成功する", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			require.NoError(t, c.RemoveItem(uuidtestkit.NewTestFromSalt(t, "absent")))
			assert.True(t, c.IsEmpty())
		})

		t.Run("2回呼んでも結果は同じ", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: productID, Quantity: 1}, baseTime),
			)

			require.NoError(t, c.RemoveItem(productID))
			require.NoError(t, c.RemoveItem(productID))

			assert.True(t, c.IsEmpty())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("商品IDが未設定ならErrInvalidProductIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, newTestGuestCart(t).RemoveItem(uuid.UUID{}), ErrInvalidProductID)
		})
	})
}

func TestCart_Clear(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細をすべて取り除き有効期限は維持する", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			require.NoError(
				t,
				c.SetItem(
					SetItemAttributes{
						ItemID:    uuidtestkit.NewTestFromSalt(t, "item"),
						ProductID: uuidtestkit.NewTestFromSalt(t, "product"),
						Quantity:  1,
					},
					baseTime,
				),
			)

			c.Clear()

			assert.True(t, c.IsEmpty())
			assert.Equal(t, defaultExpiry, c.ExpiresAt())
		})

		t.Run("空カートに対しても成功する", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			c.Clear()
			assert.True(t, c.IsEmpty())
		})
	})
}

// newMergeSource は、指定した明細を持つゲストカートを作ります。
func newMergeSource(t *testing.T, items []CartItem) *Cart {
	t.Helper()
	token := newTestSessionToken(t)
	c, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "source"), Attributes{
		SessionToken: &token, Items: items, ExpiresAt: defaultExpiry,
	})
	require.NoError(t, err)
	return c
}

// fillItems は、カートを指定件数の明細で満たします。
func fillItems(t *testing.T, c *Cart, count int) {
	t.Helper()
	for i := range count {
		require.NoError(t, c.SetItem(SetItemAttributes{
			ItemID:    uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("item_%d", i)),
			ProductID: uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("product_%d", i)),
			Quantity:  1,
		}, baseTime))
	}
}

// mergeIntoNearFullCart は、上限の 1 つ手前まで埋めたカートへ items を取り込みます。
func mergeIntoNearFullCart(t *testing.T, items []CartItem) MergeResult {
	t.Helper()
	c := newTestOwnedCart(t)
	fillItems(t, c, maxItems-1)
	return c.Merge(newMergeSource(t, items), baseTime)
}

// newDatedItems は、指定した追加時刻の差で並ぶ明細を count 件作ります。
func newDatedItems(t *testing.T, prefix string, count int, step time.Duration) []CartItem {
	t.Helper()
	items := make([]CartItem, 0, count)
	for i := range count {
		items = append(items, NewCartItem(
			uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("item_%s_%d", prefix, i)),
			CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("product_%s_%d", prefix, i)),
				Quantity:  1,
				AddedAt:   baseTime.Add(time.Duration(i) * step),
			},
		))
	}
	return items
}

func TestCart_Merge(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("自身に無い商品を取り込む", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			source := newMergeSource(t, []CartItem{newTestCartItem(t, "a", 2)})

			result := c.Merge(source, baseTime)

			require.Len(t, c.Items(), 1)
			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "product_a"), c.Items()[0].ProductID())
			assert.Equal(t, 2, c.Items()[0].Quantity())
			assert.Empty(t, result.Clamped())
			assert.Empty(t, result.Dropped())
		})

		t.Run("同一商品は数量を合算する", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			productID := uuidtestkit.NewTestFromSalt(t, "product_a")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: productID, Quantity: 2}, baseTime),
			)
			source := newMergeSource(t, []CartItem{newTestCartItem(t, "a", 3)})

			result := c.Merge(source, baseTime)

			require.Len(t, c.Items(), 1)
			assert.Equal(t, 5, c.Items()[0].Quantity())
			assert.Empty(t, result.Clamped())
		})

		t.Run("合算が上限を超える場合はクランプして報告する", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			productID := uuidtestkit.NewTestFromSalt(t, "product_a")
			require.NoError(
				t,
				c.SetItem(
					SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: productID, Quantity: maxQuantityPerItem},
					baseTime,
				),
			)
			source := newMergeSource(t, []CartItem{newTestCartItem(t, "a", 5)})

			result := c.Merge(source, baseTime)

			assert.Equal(t, maxQuantityPerItem, c.Items()[0].Quantity())
			assert.Equal(t, []uuid.UUID{productID}, result.Clamped())
		})

		t.Run("明細数の上限を超える分は追加が新しいものから切り捨てる", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			fillItems(t, c, maxItems-1)
			older := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_older"), CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "product_older"), Quantity: 1, AddedAt: baseTime,
			})
			newer := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_newer"), CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "product_newer"), Quantity: 1,
				AddedAt: baseTime.Add(time.Hour),
			})
			source := newMergeSource(t, []CartItem{newer, older})

			result := c.Merge(source, baseTime)

			assert.Len(t, c.Items(), maxItems)
			assert.Equal(t, []uuid.UUID{newer.ProductID()}, result.Dropped())
			assert.Contains(t, productIDsOf(c.Items()), older.ProductID())
		})

		t.Run("取り込んだ明細の追加時刻はマージ時刻になる", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			source := newMergeSource(t, []CartItem{newTestCartItem(t, "a", 1)})
			mergedAt := baseTime.Add(48 * time.Hour)

			c.Merge(source, mergedAt)

			assert.Equal(t, mergedAt, c.Items()[0].AddedAt())
		})

		t.Run("取り込み元は変更されない", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			source := newMergeSource(t, []CartItem{newTestCartItem(t, "a", 2)})

			c.Merge(source, baseTime)

			require.Len(t, source.Items(), 1)
			assert.Equal(t, 2, source.Items()[0].Quantity())
			assert.Equal(t, baseTime, source.Items()[0].AddedAt())
		})

		t.Run("入力の並び順を変えても結果が変わらない", func(t *testing.T) {
			t.Parallel()

			candidates := newDatedItems(t, "x", 5, time.Hour)
			shuffled := []CartItem{candidates[3], candidates[0], candidates[4], candidates[2], candidates[1]}

			assert.Equal(t, mergeIntoNearFullCart(t, candidates).Dropped(), mergeIntoNearFullCart(t, shuffled).Dropped())
		})

		t.Run("追加時刻が同時刻でも切り捨ての結果が一意に決まる", func(t *testing.T) {
			t.Parallel()

			same := newDatedItems(t, "same", 4, 0)
			reversed := []CartItem{same[3], same[2], same[1], same[0]}

			assert.Equal(t, mergeIntoNearFullCart(t, same).Dropped(), mergeIntoNearFullCart(t, reversed).Dropped())
		})

		t.Run("nilを渡しても状態を変えず空の結果を返す", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			require.NoError(
				t,
				c.SetItem(
					SetItemAttributes{
						ItemID:    uuidtestkit.NewTestFromSalt(t, "item"),
						ProductID: uuidtestkit.NewTestFromSalt(t, "product"),
						Quantity:  1,
					},
					baseTime,
				),
			)

			result := c.Merge(nil, baseTime)

			assert.Len(t, c.Items(), 1)
			assert.Empty(t, result.Clamped())
			assert.Empty(t, result.Dropped())
		})

		t.Run("マージ後も集約の不変条件を満たす", func(t *testing.T) {
			t.Parallel()
			c := newTestOwnedCart(t)
			for i := range maxItems - 2 {
				require.NoError(
					t,
					c.SetItem(
						SetItemAttributes{
							ItemID:    uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("item_%d", i)),
							ProductID: uuidtestkit.NewTestFromSalt(t, fmt.Sprintf("product_%d", i)),
							Quantity:  maxQuantityPerItem,
						},
						baseTime,
					),
				)
			}
			source := newMergeSource(t, []CartItem{
				newTestCartItem(t, "0", 50),
				newTestCartItem(t, "y", 1),
				newTestCartItem(t, "z", 1),
				newTestCartItem(t, "w", 1),
			})

			c.Merge(source, baseTime)

			require.NoError(t, validateItems(c.Items()))
		})
	})
}

func TestCart_Touch(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("有効期限をnowからttlだけ先へ延長する", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			now := baseTime.Add(time.Hour)

			c.Touch(now, 30*time.Minute)

			assert.Equal(t, now.Add(30*time.Minute), c.ExpiresAt())
			assert.Equal(t, now, c.UpdatedAt())
		})

		t.Run("期限切れのカートも延長できる", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			now := defaultExpiry.Add(time.Hour)
			require.True(t, c.IsExpired(now))

			c.Touch(now, time.Hour)

			assert.False(t, c.IsExpired(now))
		})
	})
}

func TestCart_MarkSeen(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("提示価格を明細へ記録する", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: productID, Quantity: 1}, baseTime),
			)
			price := newTestPrice(t, 42)

			c.MarkSeen(map[uuid.UUID]money.Price{productID: price})

			require.NotNil(t, c.Items()[0].LastSeenPrice())
			assert.Equal(t, price, *c.Items()[0].LastSeenPrice())
		})

		t.Run("価格を引けなかった明細は変更しない", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			known := uuidtestkit.NewTestFromSalt(t, "product_known")
			unknown := uuidtestkit.NewTestFromSalt(t, "product_unknown")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item_known"), ProductID: known, Quantity: 1}, baseTime),
			)
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item_unknown"), ProductID: unknown, Quantity: 1}, baseTime),
			)

			c.MarkSeen(map[uuid.UUID]money.Price{known: newTestPrice(t, 42)})

			items := c.Items()
			assert.NotNil(t, items[0].LastSeenPrice())
			assert.Nil(t, items[1].LastSeenPrice())
		})

		t.Run("記録済みの価格を上書きする", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item"), ProductID: productID, Quantity: 1}, baseTime),
			)

			c.MarkSeen(map[uuid.UUID]money.Price{productID: newTestPrice(t, 42)})
			c.MarkSeen(map[uuid.UUID]money.Price{productID: newTestPrice(t, 84)})

			assert.Equal(t, newTestPrice(t, 84), *c.Items()[0].LastSeenPrice())
		})

		t.Run("空のマップを渡しても状態は変わらない", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			require.NoError(
				t,
				c.SetItem(
					SetItemAttributes{
						ItemID:    uuidtestkit.NewTestFromSalt(t, "item"),
						ProductID: uuidtestkit.NewTestFromSalt(t, "product"),
						Quantity:  1,
					},
					baseTime,
				),
			)

			c.MarkSeen(nil)

			assert.Nil(t, c.Items()[0].LastSeenPrice())
		})
	})
}

func TestCart_IsExpired(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("有効期限より前なら期限切れではない", func(t *testing.T) {
			t.Parallel()
			assert.False(t, newTestGuestCart(t).IsExpired(defaultExpiry.Add(-time.Nanosecond)))
		})

		t.Run("有効期限ちょうどは期限切れではない", func(t *testing.T) {
			t.Parallel()
			// 境界の向きは掃除ジョブの削除条件と一致していなければならない。
			assert.False(t, newTestGuestCart(t).IsExpired(defaultExpiry))
		})

		t.Run("有効期限を1ナノ秒でも過ぎていれば期限切れ", func(t *testing.T) {
			t.Parallel()
			assert.True(t, newTestGuestCart(t).IsExpired(defaultExpiry.Add(time.Nanosecond)))
		})
	})
}

func TestCart_IsEmpty(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細が無ければtrueを返す", func(t *testing.T) {
			t.Parallel()
			assert.True(t, newTestGuestCart(t).IsEmpty())
		})

		t.Run("明細があればfalseを返す", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			require.NoError(
				t,
				c.SetItem(
					SetItemAttributes{
						ItemID:    uuidtestkit.NewTestFromSalt(t, "item"),
						ProductID: uuidtestkit.NewTestFromSalt(t, "product"),
						Quantity:  1,
					},
					baseTime,
				),
			)
			assert.False(t, c.IsEmpty())
		})
	})
}

func TestCart_IsOwnedBy(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("所有者本人ならtrueを返す", func(t *testing.T) {
			t.Parallel()
			assert.True(t, newTestOwnedCart(t).IsOwnedBy(uuidtestkit.NewTestFromSalt(t, "owner")))
		})

		t.Run("別のユーザーならfalseを返す", func(t *testing.T) {
			t.Parallel()
			assert.False(t, newTestOwnedCart(t).IsOwnedBy(uuidtestkit.NewTestFromSalt(t, "other")))
		})

		t.Run("所有者が未確定なら常にfalseを返す", func(t *testing.T) {
			t.Parallel()
			assert.False(t, newTestGuestCart(t).IsOwnedBy(uuidtestkit.NewTestFromSalt(t, "owner")))
		})

		t.Run("所有者未確定のカートはゼロ値のユーザーIDにも一致しない", func(t *testing.T) {
			t.Parallel()
			assert.False(t, newTestGuestCart(t).IsOwnedBy(uuid.UUID{}))
		})
	})
}

func Test_compareByAddedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("追加時刻が早いほうを先とする", func(t *testing.T) {
			t.Parallel()
			older := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_a"), CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "product_a"), AddedAt: baseTime,
			})
			newer := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_b"), CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "product_b"), AddedAt: baseTime.Add(time.Hour),
			})

			assert.Negative(t, compareByAddedAt(older, newer))
			assert.Positive(t, compareByAddedAt(newer, older))
		})

		t.Run("同時刻は商品IDのバイト列で決着する", func(t *testing.T) {
			t.Parallel()
			a := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_a"), CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "product_a"), AddedAt: baseTime,
			})
			b := NewCartItem(uuidtestkit.NewTestFromSalt(t, "item_b"), CartItemAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "product_b"), AddedAt: baseTime,
			})

			assert.Equal(t, -compareByAddedAt(b, a), compareByAddedAt(a, b))
			assert.NotEqual(t, 0, compareByAddedAt(a, b))
		})

		t.Run("同じ明細どうしは0を返す", func(t *testing.T) {
			t.Parallel()
			item := newTestCartItem(t, "a", 1)
			assert.Equal(t, 0, compareByAddedAt(item, item))
		})
	})
}

func TestMergeResult_Clamped(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("クランプが無ければ空を返す", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, MergeResult{}.Clamped())
		})

		t.Run("戻り値を書き換えても内部状態は変わらない", func(t *testing.T) {
			t.Parallel()
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			result := MergeResult{clamped: []uuid.UUID{productID}}

			result.Clamped()[0] = uuidtestkit.NewTestFromSalt(t, "other")

			assert.Equal(t, []uuid.UUID{productID}, result.Clamped())
		})
	})
}

func TestMergeResult_Dropped(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("切り捨てが無ければ空を返す", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, MergeResult{}.Dropped())
		})

		t.Run("戻り値を書き換えても内部状態は変わらない", func(t *testing.T) {
			t.Parallel()
			productID := uuidtestkit.NewTestFromSalt(t, "product")
			result := MergeResult{dropped: []uuid.UUID{productID}}

			result.Dropped()[0] = uuidtestkit.NewTestFromSalt(t, "other")

			assert.Equal(t, []uuid.UUID{productID}, result.Dropped())
		})
	})
}

func TestCart_indexOf(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("該当商品の位置を返す", func(t *testing.T) {
			t.Parallel()
			c := newTestGuestCart(t)
			first := uuidtestkit.NewTestFromSalt(t, "product_a")
			second := uuidtestkit.NewTestFromSalt(t, "product_b")
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item_a"), ProductID: first, Quantity: 1}, baseTime),
			)
			require.NoError(
				t,
				c.SetItem(SetItemAttributes{ItemID: uuidtestkit.NewTestFromSalt(t, "item_b"), ProductID: second, Quantity: 1}, baseTime),
			)

			assert.Equal(t, 0, c.indexOf(first))
			assert.Equal(t, 1, c.indexOf(second))
		})

		t.Run("該当が無ければ-1を返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, -1, newTestGuestCart(t).indexOf(uuidtestkit.NewTestFromSalt(t, "absent")))
		})
	})
}
