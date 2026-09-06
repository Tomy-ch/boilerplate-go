package purchase

import (
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/pkg/decimal"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessorOrderedAt は、ゲッター検証用の固定注文日時です。
var accessorOrderedAt = time.Date(2026, time.July, 23, 1, 2, 3, 0, time.UTC)

// mustPrice は、テスト用に十進文字列（ドル）から非負の money.Price を構築します。
func mustPrice(t *testing.T, s string) money.Price {
	t.Helper()
	p, err := money.NewPrice(decimaltestkit.MustParse(t, s))
	require.NoError(t, err)
	return p
}

// validNewArgs は、New の有効な入力（id / code / userID / 明細入力 / ロック済み在庫）を返すテストヘルパーです。
func validNewArgs(t *testing.T) (uuid.UUID, string, uuid.UUID, []DetailInput, []LockedProduct) {
	t.Helper()
	id := uuidtestkit.NewTestFromSalt(t, "purchase_id")
	userID := uuidtestkit.NewTestFromSalt(t, "user_id")
	productA := uuidtestkit.NewTestFromSalt(t, "product_a")
	productB := uuidtestkit.NewTestFromSalt(t, "product_b")
	inputs := []DetailInput{
		{ID: uuidtestkit.NewTestFromSalt(t, "detail_a"), ProductID: productA, Quantity: 2},
		{ID: uuidtestkit.NewTestFromSalt(t, "detail_b"), ProductID: productB, Quantity: 1},
	}
	locked := []LockedProduct{
		NewLockedProduct(productA, mustPrice(t, "800"), 20),
		NewLockedProduct(productB, mustPrice(t, "15"), 10),
	}
	return id, "purchase-code-001", userID, inputs, locked
}

func Test_buildDetails(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("入力の順序を保った明細と、価格スケールのままの小計を返す", func(t *testing.T) {
			t.Parallel()

			_, _, _, inputs, locked := validNewArgs(t)
			details, subtotal, err := buildDetails(inputs, locked)
			require.NoError(t, err)

			require.Len(t, details, 2)
			assert.Equal(t, inputs[0].ProductID, details[0].ProductID())
			assert.Equal(t, inputs[1].ProductID, details[1].ProductID())
			// 単価はロック済み商品の価格スナップショット。800*2 + 15*1 = 1615 を決済スケールへ
			// 丸める前の値で返すのがこの関数の責務で、New はこの後で切り捨てて整数セントにする。
			assert.Equal(t, "800", details[0].UnitPrice().Decimal().String())
			assert.Equal(t, "1615", subtotal.String())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("同一商品IDが重複する場合、ErrDuplicateProductIDを返す", func(t *testing.T) {
			t.Parallel()

			_, _, _, inputs, locked := validNewArgs(t)
			dup := append([]DetailInput{}, inputs...)
			dup[1].ProductID = dup[0].ProductID

			_, _, err := buildDetails(dup, locked)
			require.ErrorIs(t, err, ErrDuplicateProductID)
		})
	})
}

func Test_resolveLocked(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("入力に対応するロック済み商品を返し、seenは書き換えない", func(t *testing.T) {
			t.Parallel()

			_, _, _, inputs, locked := validNewArgs(t)
			lockedByID := map[uuid.UUID]LockedProduct{locked[0].ID(): locked[0]}
			seen := map[uuid.UUID]struct{}{}

			actual, err := resolveLocked(inputs[0], lockedByID, seen)
			require.NoError(t, err)

			assert.Equal(t, locked[0].ID(), actual.ID())
			// seen へ積むのは呼び出し元の役目。ここで積むと、同じ入力を検証し直した時点で
			// 重複扱いになる。
			assert.Empty(t, seen)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対応するロック済み商品が無い場合、ErrProductNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			_, _, _, inputs, _ := validNewArgs(t)

			_, err := resolveLocked(inputs[0], map[uuid.UUID]LockedProduct{}, map[uuid.UUID]struct{}{})
			require.ErrorIs(t, err, ErrProductNotFound)
		})

		t.Run("要求数量がロック済み在庫を超える場合、ErrInsufficientStockを返す", func(t *testing.T) {
			t.Parallel()

			_, _, _, inputs, locked := validNewArgs(t)
			scarce := NewLockedProduct(locked[0].ID(), locked[0].Price(), inputs[0].Quantity-1)
			lockedByID := map[uuid.UUID]LockedProduct{scarce.ID(): scarce}

			_, err := resolveLocked(inputs[0], lockedByID, map[uuid.UUID]struct{}{})
			require.ErrorIs(t, err, ErrInsufficientStock)
		})
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("有効な入力の場合、金額を計算し未処理ステータスで購入を生成する", func(t *testing.T) {
			t.Parallel()

			id, code, userID, inputs, locked := validNewArgs(t)
			actual, err := New(id, code, userID, inputs, locked)
			require.NoError(t, err)

			assert.Equal(t, id, actual.ID())
			assert.Equal(t, code, actual.Code())
			assert.Equal(t, userID, actual.UserID())
			assert.Equal(t, StatusUnprocessed, actual.Status())
			// subtotal = 80000*2 + 1500*1 = 161500
			assert.Equal(t, 161500, actual.SubtotalAmount())
			// tax = 161500 * 10 / 100 = 16150（切り捨て）
			assert.Equal(t, 16150, actual.TaxAmount())
			assert.Equal(t, shippingFeeCents, actual.ShippingFee())
			// total = 161500 + 16150 + 500 = 178150
			assert.Equal(t, 178150, actual.TotalAmount())
			require.Len(t, actual.Details(), 2)
		})

		t.Run("明細の単価は対応するロック済み商品の価格スナップショットになる", func(t *testing.T) {
			t.Parallel()

			id, code, userID, inputs, locked := validNewArgs(t)
			actual, err := New(id, code, userID, inputs, locked)
			require.NoError(t, err)

			details := actual.Details()
			assert.Equal(t, inputs[0].ProductID, details[0].ProductID())
			assert.Equal(t, 2, details[0].Quantity())
			assert.Equal(t, "800", details[0].UnitPrice().String())
			assert.Equal(t, "15", details[1].UnitPrice().String())
		})

		t.Run("税額は切り捨てで丸められる", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "p_round")
			userID := uuidtestkit.NewTestFromSalt(t, "u_round")
			productID := uuidtestkit.NewTestFromSalt(t, "prod_round")
			// unit_price=$1.05 → subtotal=105セント → tax=105*10/100=10.5 → 切り捨て 10
			inputs := []DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "d_round"), ProductID: productID, Quantity: 1}}
			locked := []LockedProduct{NewLockedProduct(productID, mustPrice(t, "1.05"), 5)}

			actual, err := New(id, "code-round", userID, inputs, locked)
			require.NoError(t, err)
			assert.Equal(t, 10, actual.TaxAmount())
		})

		t.Run("サブセント単価は小計算出時に決済スケール(整数セント)へ切り捨てられる", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "p_subcent")
			userID := uuidtestkit.NewTestFromSalt(t, "u_subcent")
			productID := uuidtestkit.NewTestFromSalt(t, "prod_subcent")
			// unit_price=$1.005（サブセント）× 3 = $3.015 → 切り捨てで 301 セント
			inputs := []DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "d_subcent"), ProductID: productID, Quantity: 3}}
			locked := []LockedProduct{NewLockedProduct(productID, mustPrice(t, "1.005"), 5)}

			actual, err := New(id, "code-subcent", userID, inputs, locked)
			require.NoError(t, err)
			assert.Equal(t, 301, actual.SubtotalAmount())
			assert.Equal(t, "1.005", actual.Details()[0].UnitPrice().String())
		})

		t.Run("数量が最小値(1)の場合でも購入を生成する", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "p_min")
			userID := uuidtestkit.NewTestFromSalt(t, "u_min")
			productID := uuidtestkit.NewTestFromSalt(t, "prod_min")
			inputs := []DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "d_min"), ProductID: productID, Quantity: minQuantity}}
			locked := []LockedProduct{NewLockedProduct(productID, mustPrice(t, "10"), 1)}

			actual, err := New(id, "code-min", userID, inputs, locked)
			require.NoError(t, err)
			require.Len(t, actual.Details(), 1)
			assert.Equal(t, minQuantity, actual.Details()[0].Quantity())
		})

		t.Run("Detailsの返却値を変更しても内部状態は不変", func(t *testing.T) {
			t.Parallel()

			id, code, userID, inputs, locked := validNewArgs(t)
			actual, err := New(id, code, userID, inputs, locked)
			require.NoError(t, err)

			returned := actual.Details()
			returned[0] = PurchaseDetail{}
			// 返却スライスへの破壊的変更が内部へ波及しないこと。
			assert.Equal(t, inputs[0].ProductID, actual.Details()[0].ProductID())
		})

		t.Run("小計が上限ちょうどなら税と送料を載せて成立する", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "p_bound_exact")
			userID := uuidtestkit.NewTestFromSalt(t, "u_bound_exact")
			productID := uuidtestkit.NewTestFromSalt(t, "prod_bound_exact")
			inputs := []DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "d_bound_exact"), ProductID: productID, Quantity: 1}}
			locked := []LockedProduct{NewLockedProduct(productID, mustPrice(t, "9223372036854775.80"), 5)}

			actual, err := New(id, "code-bound-exact", userID, inputs, locked)
			require.NoError(t, err)

			require.NotNil(t, actual)
			assert.Equal(t, maxSubtotalCents, actual.SubtotalAmount())
			assert.Equal(t, maxSubtotalCents*taxRatePercent/100, actual.TaxAmount())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			_, code, userID, inputs, locked := validNewArgs(t)
			actual, err := New(uuid.UUID{}, code, userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("codeが空の場合、ErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()
			id, _, userID, inputs, locked := validNewArgs(t)
			actual, err := New(id, "", userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("userIDがゼロ値の場合、ErrInvalidUserIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, _, inputs, locked := validNewArgs(t)
			actual, err := New(id, code, uuid.UUID{}, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidUserID)
		})

		t.Run("明細が空の場合、ErrEmptyDetailsを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, _, locked := validNewArgs(t)
			actual, err := New(id, code, userID, nil, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrEmptyDetails)
		})

		t.Run("数量が0以下の場合、ErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, inputs, locked := validNewArgs(t)
			inputs[0].Quantity = 0
			actual, err := New(id, code, userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidQuantity)
		})

		t.Run("明細IDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, inputs, locked := validNewArgs(t)
			inputs[0].ID = uuid.UUID{}
			actual, err := New(id, code, userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("同一productIDが重複する場合、ErrDuplicateProductIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, inputs, locked := validNewArgs(t)
			inputs[1].ProductID = inputs[0].ProductID
			actual, err := New(id, code, userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrDuplicateProductID)
		})

		t.Run("明細に対応するロック済み商品が無い場合、ErrProductNotFoundを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, inputs, _ := validNewArgs(t)
			// productB のロックを欠く
			locked := []LockedProduct{NewLockedProduct(inputs[0].ProductID, mustPrice(t, "800"), 20)}
			actual, err := New(id, code, userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrProductNotFound)
		})

		t.Run("要求数量が在庫を超える場合、ErrInsufficientStockを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, inputs, _ := validNewArgs(t)
			inputs[0].Quantity = 999
			locked := []LockedProduct{
				NewLockedProduct(inputs[0].ProductID, mustPrice(t, "800"), 20),
				NewLockedProduct(inputs[1].ProductID, mustPrice(t, "15"), 10),
			}
			actual, err := New(id, code, userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInsufficientStock)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("明細が積み上がり小計が決済スケール(int64)を超える場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()
			id := uuidtestkit.NewTestFromSalt(t, "p_overflow")
			userID := uuidtestkit.NewTestFromSalt(t, "u_overflow")
			productA := uuidtestkit.NewTestFromSalt(t, "prod_overflow_a")
			productB := uuidtestkit.NewTestFromSalt(t, "prod_overflow_b")
			inputs := []DetailInput{
				{ID: uuidtestkit.NewTestFromSalt(t, "d_overflow_a"), ProductID: productA, Quantity: 1},
				{ID: uuidtestkit.NewTestFromSalt(t, "d_overflow_b"), ProductID: productB, Quantity: 1},
			}
			locked := []LockedProduct{
				NewLockedProduct(productA, mustPrice(t, "92233720368547758.07"), 5),
				NewLockedProduct(productB, mustPrice(t, "92233720368547758.07"), 5),
			}

			actual, err := New(id, "code-overflow", userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidAmount)
			require.ErrorIs(t, err, decimal.ErrOverflow)
		})

		t.Run("小計が上限をちょうど1セット超えるとErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()
			id := uuidtestkit.NewTestFromSalt(t, "p_bound_over")
			userID := uuidtestkit.NewTestFromSalt(t, "u_bound_over")
			productID := uuidtestkit.NewTestFromSalt(t, "prod_bound_over")
			inputs := []DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "d_bound_over"), ProductID: productID, Quantity: 1}}
			locked := []LockedProduct{NewLockedProduct(productID, mustPrice(t, "9223372036854775.81"), 5)}

			actual, err := New(id, "code-bound-over", userID, inputs, locked)
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidAmount)
		})

		t.Run("明細に由来する違反は details を名指しする", func(t *testing.T) {
			t.Parallel()

			t.Run("明細が空", func(t *testing.T) {
				t.Parallel()
				id, code, userID, _, locked := validNewArgs(t)

				_, err := New(id, code, userID, nil, locked)

				require.ErrorIs(t, err, ErrEmptyDetails)
				meta, ok := apperror.MetaFrom(err)
				require.True(t, ok)
				assert.Equal(t, []string{FieldDetails}, meta.Details())
			})

			t.Run("数量が0以下", func(t *testing.T) {
				t.Parallel()
				id, code, userID, inputs, locked := validNewArgs(t)
				inputs[0].Quantity = 0

				_, err := New(id, code, userID, inputs, locked)

				require.ErrorIs(t, err, ErrInvalidQuantity)
				meta, ok := apperror.MetaFrom(err)
				require.True(t, ok)
				assert.Equal(t, []string{FieldDetails}, meta.Details())
			})

			t.Run("在庫不足は状態の衝突なので識別子を付けない", func(t *testing.T) {
				t.Parallel()
				id, code, userID, inputs, _ := validNewArgs(t)
				locked := make([]LockedProduct, 0, len(inputs))
				for _, in := range inputs {
					locked = append(locked, NewLockedProduct(in.ProductID, mustPrice(t, "800"), in.Quantity-1))
				}

				_, err := New(id, code, userID, inputs, locked)

				require.ErrorIs(t, err, ErrInsufficientStock)
				_, ok := apperror.MetaFrom(err)
				assert.False(t, ok)
			})

			t.Run("対応するロック済み商品が無い", func(t *testing.T) {
				t.Parallel()
				id, code, userID, inputs, _ := validNewArgs(t)
				locked := []LockedProduct{NewLockedProduct(inputs[0].ProductID, mustPrice(t, "800"), 20)}

				_, err := New(id, code, userID, inputs, locked)

				require.ErrorIs(t, err, ErrProductNotFound)
				meta, ok := apperror.MetaFrom(err)
				require.True(t, ok)
				assert.Equal(t, []string{FieldDetails}, meta.Details())
			})
		})
	})
}

func TestReconstruct(t *testing.T) {
	t.Parallel()

	valid := func(t *testing.T) (uuid.UUID, string, uuid.UUID, uuid.UUID, []PurchaseDetail, time.Time) {
		t.Helper()
		id := uuidtestkit.NewTestFromSalt(t, "rc_id")
		userID := uuidtestkit.NewTestFromSalt(t, "rc_user")
		statusID := uuidtestkit.NewTestFromSalt(t, "rc_status")
		details := []PurchaseDetail{
			NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "rc_d1"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "rc_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		orderedAt := time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC)
		return id, "rc-code", userID, statusID, details, orderedAt
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("有効な入力の場合、永続化済みの購入を再構築する", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			require.NoError(t, err)
			assert.Equal(t, id, actual.ID())
			assert.Equal(t, code, actual.Code())
			assert.Equal(t, userID, actual.UserID())
			assert.Equal(t, statusID, actual.StatusID())
			assert.Equal(t, 160000, actual.SubtotalAmount())
			assert.Equal(t, 16000, actual.TaxAmount())
			assert.Equal(t, 500, actual.ShippingFee())
			assert.Equal(t, 176500, actual.TotalAmount())
			assert.Equal(t, StatusUnprocessed, actual.Status())
			assert.Equal(t, orderedAt, actual.OrderedAt())
			assert.Nil(t, actual.CanceledAt())
			require.Len(t, actual.Details(), 1)
			assert.Equal(t, details[0].ProductID(), actual.Details()[0].ProductID())
			assert.True(t, details[0].UnitPrice().Decimal().Equal(actual.Details()[0].UnitPrice().Decimal()))
		})

		t.Run("クーポン適用済みの購入を再構築する", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			couponID := uuidtestkit.NewTestFromSalt(t, "rc_coupon")
			// 課税の基礎は値引き後の額なので tax は (160000-16000)*10/100。
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				DiscountAmount: 16000,
				CouponID:       &couponID,
				TaxAmount:      14400,
				ShippingFee:    500,
				TotalAmount:    158900,
				Details:        details,
				OrderedAt:      orderedAt,
			})
			require.NoError(t, err)
			require.NotNil(t, actual.CouponID())
			assert.Equal(t, couponID, *actual.CouponID())
			assert.Equal(t, 16000, actual.DiscountAmount())
			assert.Equal(t, 160000, actual.SubtotalAmount())
		})

		t.Run("支払い後にキャンセルされた購入（paidAtとcanceledAtの両方セット）を再構築できる", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			paidAt := orderedAt
			canceledAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusCanceled.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         &paidAt,
				CanceledAt:     &canceledAt,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			require.NoError(t, err)
			require.NotNil(t, actual.PaidAt())
			assert.Equal(t, paidAt, *actual.PaidAt())
			require.NotNil(t, actual.CanceledAt())
		})

		t.Run("配達済みの購入（paidAtとshippedAtとdeliveredAtがセット）を再構築できる", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			paidAt := orderedAt
			shippedAt := orderedAt.Add(24 * time.Hour)
			deliveredAt := orderedAt.Add(48 * time.Hour)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusDelivered.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         &paidAt,
				CanceledAt:     nil,
				ShippedAt:      &shippedAt,
				DeliveredAt:    &deliveredAt,
			})
			require.NoError(t, err)
			require.NotNil(t, actual.PaidAt())
			require.NotNil(t, actual.ShippedAt())
			assert.Equal(t, shippedAt, *actual.ShippedAt())
			require.NotNil(t, actual.DeliveredAt())
			assert.Equal(t, deliveredAt, *actual.DeliveredAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("クーポンIDはあるが値引きが0の場合、ErrZeroDiscountを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			couponID := uuidtestkit.NewTestFromSalt(t, "rc_zero_coupon")
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				DiscountAmount: 0,
				CouponID:       &couponID,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
			})
			require.ErrorIs(t, err, ErrZeroDiscount)
			assert.Nil(t, actual)
		})

		t.Run("値引きが小計を超える場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			couponID := uuidtestkit.NewTestFromSalt(t, "rc_over_coupon")
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				DiscountAmount: 160001,
				CouponID:       &couponID,
				TaxAmount:      0,
				ShippingFee:    500,
				TotalAmount:    500,
				Details:        details,
				OrderedAt:      orderedAt,
			})
			require.ErrorIs(t, err, ErrInvalidAmount)
			assert.Nil(t, actual)
		})

		t.Run("statusIDがゼロ値の場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, _, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       uuid.UUID{},
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("金額が負の場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: -1,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidAmount)
		})

		t.Run("明細が空の場合、ErrEmptyDetailsを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, _, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        nil,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrEmptyDetails)
		})

		reconstructWithDetails := func(t *testing.T, details []PurchaseDetail) (*Purchase, error) {
			t.Helper()
			id, code, userID, statusID, _, orderedAt := valid(t)

			return Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
		}

		t.Run("明細IDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			actual, err := reconstructWithDetails(t, []PurchaseDetail{
				NewPurchaseDetail(uuid.UUID{}, PurchaseDetailAttributes{
					ProductID: uuidtestkit.NewTestFromSalt(t, "rc_bad_p"),
					Quantity:  2,
					UnitPrice: mustPrice(t, "800"),
				}),
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("明細のproductIDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			actual, err := reconstructWithDetails(t, []PurchaseDetail{
				NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "rc_nilprod_d"), PurchaseDetailAttributes{
					ProductID: uuid.UUID{},
					Quantity:  2,
					UnitPrice: mustPrice(t, "800"),
				}),
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("明細の数量が最小値未満の場合、ErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			actual, err := reconstructWithDetails(t, []PurchaseDetail{
				NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "rc_zeroqty_d"), PurchaseDetailAttributes{
					ProductID: uuidtestkit.NewTestFromSalt(t, "rc_zeroqty_p"),
					Quantity:  0,
					UnitPrice: mustPrice(t, "800"),
				}),
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidQuantity)
		})

		t.Run("明細に同一productIDが重複する場合、ErrDuplicateProductIDを返す", func(t *testing.T) {
			t.Parallel()
			productID := uuidtestkit.NewTestFromSalt(t, "rc_dup_p")
			actual, err := reconstructWithDetails(t, []PurchaseDetail{
				NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "rc_dup_d1"), PurchaseDetailAttributes{
					ProductID: productID,
					Quantity:  1,
					UnitPrice: mustPrice(t, "800"),
				}),
				NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "rc_dup_d2"), PurchaseDetailAttributes{
					ProductID: productID,
					Quantity:  2,
					UnitPrice: mustPrice(t, "800"),
				}),
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrDuplicateProductID)
		})

		t.Run("IDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			_, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(uuid.UUID{}, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("codeが空の場合、ErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()
			id, _, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           "",
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("userIDがゼロ値の場合、ErrInvalidUserIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, _, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         uuid.UUID{},
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidUserID)
		})

		t.Run("税額が負の場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      -1,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidAmount)
		})

		t.Run("送料が負の場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    -1,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidAmount)
		})

		t.Run("合計額が負の場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    -1,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidAmount)
		})

		t.Run("statusCodeが不正（未処理未満）の場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     0,
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("キャンセルstatusなのにcanceledAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusCanceled.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("支払い済みstatusなのにpaidAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusPaid.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("発送済みstatusなのにshippedAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			paidAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusShipped.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         &paidAt,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("キャンセルstatusなのにshippedAtがセット済みの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			canceledAt := orderedAt
			shippedAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusCanceled.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     &canceledAt,
				ShippedAt:      &shippedAt,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("キャンセルstatusなのにdeliveredAtがセット済みの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			canceledAt := orderedAt
			shippedAt := orderedAt
			deliveredAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusCanceled.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     &canceledAt,
				ShippedAt:      &shippedAt,
				DeliveredAt:    &deliveredAt,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("発送済みstatusなのにpaidAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			shippedAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusShipped.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     nil,
				ShippedAt:      &shippedAt,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("deliveredAtがセット済みなのにshippedAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			paidAt := orderedAt
			deliveredAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusPaid.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         &paidAt,
				CanceledAt:     nil,
				ShippedAt:      nil,
				DeliveredAt:    &deliveredAt,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("配達済みstatusなのにdeliveredAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			paidAt := orderedAt
			shippedAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusDelivered.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         &paidAt,
				CanceledAt:     nil,
				ShippedAt:      &shippedAt,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("deliveredAtがセット済みなのに配達済みstatusでない場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			paidAt := orderedAt
			shippedAt := orderedAt
			deliveredAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusShipped.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         &paidAt,
				CanceledAt:     nil,
				ShippedAt:      &shippedAt,
				DeliveredAt:    &deliveredAt,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})

		t.Run("canceledAtがセット済みなのにキャンセルstatusでない場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			id, code, userID, statusID, details, orderedAt := valid(t)
			canceledAt := orderedAt
			actual, err := Reconstruct(id, Attributes{
				Code:           code,
				UserID:         userID,
				StatusID:       statusID,
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				TaxAmount:      16000,
				ShippingFee:    500,
				TotalAmount:    176500,
				Details:        details,
				OrderedAt:      orderedAt,
				PaidAt:         nil,
				CanceledAt:     &canceledAt,
				ShippedAt:      nil,
				DeliveredAt:    nil,
			})
			assert.Nil(t, actual)
			require.ErrorIs(t, err, ErrInvalidStatusID)
		})
	})
}

func TestPurchase_Cancel(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, status Status, canceledAt, shippedAt, deliveredAt *time.Time) *Purchase {
		t.Helper()
		details := []PurchaseDetail{
			NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "cancel_d1"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "cancel_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cancel_id"), Attributes{
			Code:           "cancel-code",
			UserID:         uuidtestkit.NewTestFromSalt(t, "cancel_user"),
			StatusID:       uuidtestkit.NewTestFromSalt(t, "cancel_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         nil,
			CanceledAt:     canceledAt,
			ShippedAt:      shippedAt,
			DeliveredAt:    deliveredAt,
		})
		require.NoError(t, err)
		return p
	}
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル可能状態からキャンセルすると、statusCodeがキャンセルになりcanceledAtがセットされる", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusUnprocessed, nil, nil, nil)
			_, err := p.Cancel(now)
			require.NoError(t, err)
			assert.Equal(t, StatusCanceled, p.Status())
			require.NotNil(t, p.CanceledAt())
			assert.Equal(t, now, *p.CanceledAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既にキャンセル済み（statusCode）の場合、ErrAlreadyCanceledを返す", func(t *testing.T) {
			t.Parallel()
			canceledAt := time.Date(2026, time.July, 24, 9, 0, 0, 0, time.UTC)
			p := build(t, StatusCanceled, &canceledAt, nil, nil)
			_, err := p.Cancel(now)
			require.ErrorIs(t, err, ErrAlreadyCanceled)
			// ガードが代入より手前で効いていること（拒否後に状態が動かないこと）まで固定する。
			assert.Equal(t, StatusCanceled, p.Status())
			require.NotNil(t, p.CanceledAt())
			assert.Equal(t, canceledAt, *p.CanceledAt())
		})

		t.Run("完了の場合、ErrCancelNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusCompleted, nil, nil, nil)
			_, err := p.Cancel(now)
			require.ErrorIs(t, err, ErrCancelNotAllowed)
		})

		t.Run("発送済み（shippedAtセット済）の場合、ErrCancelNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusUnprocessed, nil, &now, nil)
			_, err := p.Cancel(now)
			require.ErrorIs(t, err, ErrCancelNotAllowed)
		})

		t.Run("配達済みの場合、ErrCancelNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusDelivered, nil, &now, &now)
			_, err := p.Cancel(now)
			require.ErrorIs(t, err, ErrCancelNotAllowed)
		})
	})
}

func TestPurchase_Pay(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, status Status, paidAt, canceledAt, shippedAt, deliveredAt *time.Time) *Purchase {
		t.Helper()
		details := []PurchaseDetail{
			NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "pay_d1"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "pay_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "pay_id"), Attributes{
			Code:           "pay-code",
			UserID:         uuidtestkit.NewTestFromSalt(t, "pay_user"),
			StatusID:       uuidtestkit.NewTestFromSalt(t, "pay_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         paidAt,
			CanceledAt:     canceledAt,
			ShippedAt:      shippedAt,
			DeliveredAt:    deliveredAt,
		})
		require.NoError(t, err)
		return p
	}
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未払い相当から支払うと、statusCodeが支払い済みになりpaidAtがセットされる", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusUnprocessed, nil, nil, nil, nil)
			_, err := p.Pay(now)
			require.NoError(t, err)
			assert.Equal(t, StatusPaid, p.Status())
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, now, *p.PaidAt())
		})

		t.Run("受付中から支払うと、statusCodeが支払い済みになりpaidAtがセットされる", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusAccepted, nil, nil, nil, nil)
			_, err := p.Pay(now)
			require.NoError(t, err)
			assert.Equal(t, StatusPaid, p.Status())
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, now, *p.PaidAt())
		})

		t.Run("確認中から支払うと、statusCodeが支払い済みになりpaidAtがセットされる", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusConfirming, nil, nil, nil, nil)
			_, err := p.Pay(now)
			require.NoError(t, err)
			assert.Equal(t, StatusPaid, p.Status())
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, now, *p.PaidAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既に支払い済み（statusCode）の場合、ErrAlreadyPaidを返す", func(t *testing.T) {
			t.Parallel()
			paidAt := time.Date(2026, time.July, 24, 9, 0, 0, 0, time.UTC)
			p := build(t, StatusPaid, &paidAt, nil, nil, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrAlreadyPaid)
			// ガードが代入より手前で効いていること（拒否後に状態が動かないこと）まで固定する。
			assert.Equal(t, StatusPaid, p.Status())
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, paidAt, *p.PaidAt())
		})

		t.Run("キャンセル済みの場合、ErrPayNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusCanceled, nil, &now, nil, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrPayNotAllowed)
		})

		t.Run("完了の場合、ErrPayNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusCompleted, nil, nil, nil, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrPayNotAllowed)
		})

		t.Run("発送済み（shippedAtセット済）の場合、ErrPayNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusUnprocessed, nil, nil, &now, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrPayNotAllowed)
		})

		t.Run("配達済みの場合、ErrPayNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusDelivered, &now, nil, &now, &now)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrPayNotAllowed)
		})

		t.Run("処理中の場合、ErrPayNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			// 処理中は支払いを終えた後の段階であり、未払い相当ではない。
			p := build(t, StatusProcessing, &now, nil, nil, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrPayNotAllowed)
		})

		t.Run("未処理でpaidAtが記録済みの場合、ErrAlreadyPaidを返しpaidAtを上書きしない", func(t *testing.T) {
			t.Parallel()
			// 受付中 / 確認中 はアプリからは遷移せず、外部運用が支払いを記録した行が入り得る。
			// 遷移させると paidAt が now で潰れるため、記録済みの日時が保たれることまで固定する。
			recorded := time.Date(2026, time.July, 24, 9, 0, 0, 0, time.UTC)
			p := build(t, StatusUnprocessed, &recorded, nil, nil, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrAlreadyPaid)
			assert.Equal(t, StatusUnprocessed, p.Status())
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, recorded, *p.PaidAt())
		})

		t.Run("受付中でpaidAtが記録済みの場合、ErrAlreadyPaidを返しpaidAtを上書きしない", func(t *testing.T) {
			t.Parallel()
			recorded := time.Date(2026, time.July, 24, 9, 0, 0, 0, time.UTC)
			p := build(t, StatusAccepted, &recorded, nil, nil, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrAlreadyPaid)
			assert.Equal(t, StatusAccepted, p.Status())
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, recorded, *p.PaidAt())
		})

		t.Run("確認中でpaidAtが記録済みの場合、ErrAlreadyPaidを返しpaidAtを上書きしない", func(t *testing.T) {
			t.Parallel()
			recorded := time.Date(2026, time.July, 24, 9, 0, 0, 0, time.UTC)
			p := build(t, StatusConfirming, &recorded, nil, nil, nil)
			_, err := p.Pay(now)
			require.ErrorIs(t, err, ErrAlreadyPaid)
			assert.Equal(t, StatusConfirming, p.Status())
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, recorded, *p.PaidAt())
		})
	})
}

func TestLockedProduct_Price(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロック時のサブセント価格スナップショットを返す", func(t *testing.T) {
			t.Parallel()

			l := NewLockedProduct(uuidtestkit.NewTestFromSalt(t, "lp_price"), mustPrice(t, "19.995"), 5)
			assert.Equal(t, "19.995", l.Price().String())
			assert.True(t, l.Price().Decimal().Equal(decimaltestkit.MustParse(t, "19.995")))
		})
	})
}

func TestPurchaseDetail_UnitPrice(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細のサブセント単価スナップショットを返す", func(t *testing.T) {
			t.Parallel()

			d := NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "pd_id"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "pd_product"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "1.005"),
			})
			assert.Equal(t, "1.005", d.UnitPrice().String())
			assert.True(t, d.UnitPrice().Decimal().Equal(decimaltestkit.MustParse(t, "1.005")))
		})
	})
}

func TestPurchase_Ship(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, status Status, paidAt, canceledAt, shippedAt, deliveredAt *time.Time) *Purchase {
		t.Helper()
		details := []PurchaseDetail{
			NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "ship_d1"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "ship_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "ship_id"), Attributes{
			Code:           "ship-code",
			UserID:         uuidtestkit.NewTestFromSalt(t, "ship_user"),
			StatusID:       uuidtestkit.NewTestFromSalt(t, "ship_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         paidAt,
			CanceledAt:     canceledAt,
			ShippedAt:      shippedAt,
			DeliveredAt:    deliveredAt,
		})
		require.NoError(t, err)
		return p
	}
	paidAt := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("支払い済みから発送すると、statusCodeが発送済みになりshippedAtがセットされる", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusPaid, &paidAt, nil, nil, nil)
			_, err := p.Ship(now)
			require.NoError(t, err)
			assert.Equal(t, StatusShipped, p.Status())
			require.NotNil(t, p.ShippedAt())
			assert.Equal(t, now, *p.ShippedAt())
		})

		t.Run("発送してもpaidAtは保持される", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusPaid, &paidAt, nil, nil, nil)
			_, err := p.Ship(now)
			require.NoError(t, err)
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, paidAt, *p.PaidAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既に発送済み（statusCode）の場合、ErrAlreadyShippedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusShipped, &paidAt, nil, &now, nil)
			_, err := p.Ship(now)
			require.ErrorIs(t, err, ErrAlreadyShipped)
		})

		t.Run("未払い（未処理）の場合、ErrShipNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusUnprocessed, nil, nil, nil, nil)
			_, err := p.Ship(now)
			require.ErrorIs(t, err, ErrShipNotAllowed)
		})

		t.Run("キャンセル済みの場合、ErrShipNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusCanceled, nil, &now, nil, nil)
			_, err := p.Ship(now)
			require.ErrorIs(t, err, ErrShipNotAllowed)
		})

		t.Run("完了の場合、ErrShipNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusCompleted, nil, nil, nil, nil)
			_, err := p.Ship(now)
			require.ErrorIs(t, err, ErrShipNotAllowed)
		})

		t.Run("配達済みの場合、二重発送ではなくErrShipNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			deliveredAt := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
			p := build(t, StatusDelivered, &paidAt, nil, &now, &deliveredAt)
			_, err := p.Ship(now)
			require.ErrorIs(t, err, ErrShipNotAllowed)
		})

		t.Run("遷移に失敗した場合、statusCodeとshippedAtを変更しない", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusUnprocessed, nil, nil, nil, nil)
			_, err := p.Ship(now)
			require.Error(t, err)
			assert.Equal(t, StatusUnprocessed, p.Status())
			assert.Nil(t, p.ShippedAt())
		})
	})
}

func TestPurchase_ShippedAt(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, shippedAt *time.Time) *Purchase {
		t.Helper()
		details := []PurchaseDetail{
			NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "sa_d1"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "sa_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		status := StatusPaid
		paidAt := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
		if shippedAt != nil {
			status = StatusShipped
		}
		p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "sa_id"), Attributes{
			Code:           "sa-code",
			UserID:         uuidtestkit.NewTestFromSalt(t, "sa_user"),
			StatusID:       uuidtestkit.NewTestFromSalt(t, "sa_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         &paidAt,
			CanceledAt:     nil,
			ShippedAt:      shippedAt,
			DeliveredAt:    nil,
		})
		require.NoError(t, err)
		return p
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未発送の場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, build(t, nil).ShippedAt())
		})

		t.Run("発送済みの場合、内部状態を保護するためコピーを返す", func(t *testing.T) {
			shippedAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
			p := build(t, &shippedAt)

			t.Run("再構築時に渡したポインタを変更しても、購入のshippedAtは変わらない", func(t *testing.T) {
				t.Parallel()

				original := shippedAt
				shippedAt = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

				require.NotNil(t, p.ShippedAt())
				assert.Equal(t, original, *p.ShippedAt())
			})

			t.Run("ShippedAtの返り値を変更しても、購入のshippedAtは変わらない", func(t *testing.T) {
				t.Parallel()

				original := *p.ShippedAt()

				got := p.ShippedAt()
				*got = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

				require.NotNil(t, p.ShippedAt())
				assert.NotEqual(t, *got, *p.ShippedAt())
				assert.Equal(t, original, *p.ShippedAt())
			})
		})
	})
}

func TestPurchase_Deliver(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, status Status, paidAt, canceledAt, shippedAt, deliveredAt *time.Time) *Purchase {
		t.Helper()
		details := []PurchaseDetail{
			NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "dlv_d1"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "dlv_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "dlv_id"), Attributes{
			Code:           "dlv-code",
			UserID:         uuidtestkit.NewTestFromSalt(t, "dlv_user"),
			StatusID:       uuidtestkit.NewTestFromSalt(t, "dlv_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         paidAt,
			CanceledAt:     canceledAt,
			ShippedAt:      shippedAt,
			DeliveredAt:    deliveredAt,
		})
		require.NoError(t, err)
		return p
	}
	paidAt := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
	shippedAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.July, 28, 9, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("発送済みから配達すると、statusCodeが配達済みになりdeliveredAtがセットされる", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusShipped, &paidAt, nil, &shippedAt, nil)
			_, err := p.Deliver(now)
			require.NoError(t, err)
			assert.Equal(t, StatusDelivered, p.Status())
			require.NotNil(t, p.DeliveredAt())
			assert.Equal(t, now, *p.DeliveredAt())
		})

		t.Run("配達してもpaidAtとshippedAtは保持される", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusShipped, &paidAt, nil, &shippedAt, nil)
			_, err := p.Deliver(now)
			require.NoError(t, err)
			require.NotNil(t, p.PaidAt())
			assert.Equal(t, paidAt, *p.PaidAt())
			require.NotNil(t, p.ShippedAt())
			assert.Equal(t, shippedAt, *p.ShippedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既に配達済み（statusCode）の場合、ErrAlreadyDeliveredを返す", func(t *testing.T) {
			t.Parallel()
			deliveredAt := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
			p := build(t, StatusDelivered, &paidAt, nil, &shippedAt, &deliveredAt)
			_, err := p.Deliver(now)
			require.ErrorIs(t, err, ErrAlreadyDelivered)
		})

		t.Run("未払い（未処理）の場合、ErrDeliverNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusUnprocessed, nil, nil, nil, nil)
			_, err := p.Deliver(now)
			require.ErrorIs(t, err, ErrDeliverNotAllowed)
		})

		t.Run("支払い済み（未発送）の場合、ErrDeliverNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusPaid, &paidAt, nil, nil, nil)
			_, err := p.Deliver(now)
			require.ErrorIs(t, err, ErrDeliverNotAllowed)
		})

		t.Run("キャンセル済みの場合、ErrDeliverNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			canceledAt := time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC)
			p := build(t, StatusCanceled, nil, &canceledAt, nil, nil)
			_, err := p.Deliver(now)
			require.ErrorIs(t, err, ErrDeliverNotAllowed)
		})

		t.Run("完了の場合、ErrDeliverNotAllowedを返す", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusCompleted, nil, nil, nil, nil)
			_, err := p.Deliver(now)
			require.ErrorIs(t, err, ErrDeliverNotAllowed)
		})

		t.Run("遷移に失敗した場合、statusCodeとdeliveredAtを変更しない", func(t *testing.T) {
			t.Parallel()
			p := build(t, StatusPaid, &paidAt, nil, nil, nil)
			_, err := p.Deliver(now)
			require.Error(t, err)
			assert.Equal(t, StatusPaid, p.Status())
			assert.Nil(t, p.DeliveredAt())
		})
	})
}

func TestPurchase_DeliveredAt(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, deliveredAt *time.Time) *Purchase {
		t.Helper()
		details := []PurchaseDetail{
			NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "da_d1"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "da_p1"),
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			}),
		}
		status := StatusShipped
		paidAt := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)
		shippedAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
		if deliveredAt != nil {
			status = StatusDelivered
		}
		p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "da_id"), Attributes{
			Code:           "da-code",
			UserID:         uuidtestkit.NewTestFromSalt(t, "da_user"),
			StatusID:       uuidtestkit.NewTestFromSalt(t, "da_status"),
			StatusCode:     status.Code(),
			SubtotalAmount: 160000,
			TaxAmount:      16000,
			ShippingFee:    500,
			TotalAmount:    176500,
			Details:        details,
			OrderedAt:      time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			PaidAt:         &paidAt,
			CanceledAt:     nil,
			ShippedAt:      &shippedAt,
			DeliveredAt:    deliveredAt,
		})
		require.NoError(t, err)
		return p
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未配達の場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, build(t, nil).DeliveredAt())
		})

		t.Run("配達済みの場合、内部状態を保護するためコピーを返す", func(t *testing.T) {
			deliveredAt := time.Date(2026, time.July, 28, 9, 0, 0, 0, time.UTC)
			p := build(t, &deliveredAt)

			t.Run("再構築時に渡したポインタを変更しても、購入のdeliveredAtは変わらない", func(t *testing.T) {
				t.Parallel()

				original := deliveredAt
				deliveredAt = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

				require.NotNil(t, p.DeliveredAt())
				assert.Equal(t, original, *p.DeliveredAt())
			})

			t.Run("DeliveredAtの返り値を変更しても、購入のdeliveredAtは変わらない", func(t *testing.T) {
				t.Parallel()

				original := *p.DeliveredAt()

				got := p.DeliveredAt()
				*got = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

				require.NotNil(t, p.DeliveredAt())
				assert.NotEqual(t, *got, *p.DeliveredAt())
				assert.Equal(t, original, *p.DeliveredAt())
			})
		})
	})
}

// accessorPurchaseWith は、ゲッター検証用の購入集約を任意の状態で再構築します。
// status と各日時は Reconstruct の不変条件を満たす組で渡します。金額・日時・ID は
// ゲッターの取り違えを検出できるよう互いに異なる値にしています。
func accessorPurchaseWith(t *testing.T, status Status, paidAt, canceledAt *time.Time) *Purchase {
	t.Helper()

	details := []PurchaseDetail{
		NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "acc_detail"), PurchaseDetailAttributes{
			ProductID: uuidtestkit.NewTestFromSalt(t, "acc_product"),
			Quantity:  2,
			UnitPrice: mustPrice(t, "800"),
		}),
	}
	p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "acc_id"), Attributes{
		Code:           "acc-code-001",
		UserID:         uuidtestkit.NewTestFromSalt(t, "acc_user"),
		StatusID:       uuidtestkit.NewTestFromSalt(t, "acc_status"),
		StatusCode:     status.Code(),
		SubtotalAmount: 160000,
		TaxAmount:      16000,
		ShippingFee:    500,
		TotalAmount:    176500,
		Details:        details,
		OrderedAt:      accessorOrderedAt,
		PaidAt:         paidAt,
		CanceledAt:     canceledAt,
		ShippedAt:      nil,
		DeliveredAt:    nil,
	})
	require.NoError(t, err)
	return p
}

// accessorPurchase は、ゲッター検証用に支払い済みの購入集約を再構築します。
// 既定値と区別できるよう、status は未処理ではなく支払い済みにしています。
func accessorPurchase(t *testing.T) *Purchase {
	t.Helper()

	paidAt := time.Date(2026, time.July, 25, 4, 5, 6, 0, time.UTC)
	return accessorPurchaseWith(t, StatusPaid, &paidAt, nil)
}

func TestLockedProduct_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロック時の商品IDを返す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "lp_id")
			l := NewLockedProduct(id, mustPrice(t, "19.99"), 5)

			assert.Equal(t, id, l.ID())
		})
	})
}

func TestLockedProduct_Quantity(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロック時点の在庫数を返す", func(t *testing.T) {
			t.Parallel()

			l := NewLockedProduct(uuidtestkit.NewTestFromSalt(t, "lp_quantity"), mustPrice(t, "19.99"), 7)

			assert.Equal(t, 7, l.Quantity())
		})

		t.Run("在庫切れの場合、0を返す", func(t *testing.T) {
			t.Parallel()

			l := NewLockedProduct(uuidtestkit.NewTestFromSalt(t, "lp_quantity_zero"), mustPrice(t, "19.99"), 0)

			assert.Equal(t, 0, l.Quantity())
		})
	})
}

func TestNewLockedProduct(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("入力した商品ID・単価・在庫数を保持したスナップショットを返す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "new_lp_id")

			actual := NewLockedProduct(id, mustPrice(t, "1.005"), 3)

			assert.Equal(t, id, actual.ID())
			assert.Equal(t, "1.005", actual.Price().String())
			assert.Equal(t, 3, actual.Quantity())
		})
	})
}

func TestNewPurchaseDetail(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("入力した明細ID・商品ID・数量・単価を保持した明細を返す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "new_pd_id")
			productID := uuidtestkit.NewTestFromSalt(t, "new_pd_product")

			actual := NewPurchaseDetail(id, PurchaseDetailAttributes{
				ProductID: productID,
				Quantity:  4,
				UnitPrice: mustPrice(t, "1.005"),
			})

			assert.Equal(t, id, actual.ID())
			assert.Equal(t, productID, actual.ProductID())
			assert.Equal(t, 4, actual.Quantity())
			assert.Equal(t, "1.005", actual.UnitPrice().String())
		})
	})
}

func TestPurchaseDetail_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("商品IDではなく明細IDを返す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "pd_detail_id")
			productID := uuidtestkit.NewTestFromSalt(t, "pd_detail_product")
			d := NewPurchaseDetail(id, PurchaseDetailAttributes{
				ProductID: productID,
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			})

			assert.Equal(t, id, d.ID())
			assert.NotEqual(t, productID, d.ID())
		})
	})
}

func TestPurchaseDetail_ProductID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細IDではなく商品IDを返す", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "pd_product_detail")
			productID := uuidtestkit.NewTestFromSalt(t, "pd_product_id")
			d := NewPurchaseDetail(id, PurchaseDetailAttributes{
				ProductID: productID,
				Quantity:  2,
				UnitPrice: mustPrice(t, "800"),
			})

			assert.Equal(t, productID, d.ProductID())
			assert.NotEqual(t, id, d.ProductID())
		})
	})
}

func TestPurchaseDetail_Quantity(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細の購入数量を返す", func(t *testing.T) {
			t.Parallel()

			d := NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "pd_quantity_id"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "pd_quantity_product"),
				Quantity:  9,
				UnitPrice: mustPrice(t, "800"),
			})

			assert.Equal(t, 9, d.Quantity())
		})
	})
}

func TestPurchase_CanceledAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未キャンセルの場合、nilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, accessorPurchase(t).CanceledAt())
		})

		t.Run("キャンセル済みの場合、キャンセル日時を返す", func(t *testing.T) {
			t.Parallel()

			canceledAt := time.Date(2026, time.July, 26, 7, 8, 9, 0, time.UTC)
			p := accessorPurchaseWith(t, StatusCanceled, nil, &canceledAt)

			require.NotNil(t, p.CanceledAt())
			assert.Equal(t, canceledAt, *p.CanceledAt())
		})

		t.Run("再構築時に渡したポインタを変更しても購入のcanceledAtは変わらない", func(t *testing.T) {
			t.Parallel()

			canceledAt := time.Date(2026, time.July, 26, 7, 8, 9, 0, time.UTC)
			p := accessorPurchaseWith(t, StatusCanceled, nil, &canceledAt)

			original := canceledAt
			canceledAt = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

			require.NotNil(t, p.CanceledAt())
			assert.Equal(t, original, *p.CanceledAt())
		})

		t.Run("返り値のポインタを書き換えても購入のcanceledAtは変わらない", func(t *testing.T) {
			t.Parallel()

			canceledAt := time.Date(2026, time.July, 26, 7, 8, 9, 0, time.UTC)
			p := accessorPurchaseWith(t, StatusCanceled, nil, &canceledAt)

			got := p.CanceledAt()
			*got = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

			require.NotNil(t, p.CanceledAt())
			assert.NotEqual(t, *got, *p.CanceledAt())
			assert.Equal(t, canceledAt, *p.CanceledAt())
		})
	})
}

func TestPurchase_Code(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の購入コードを返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "acc-code-001", accessorPurchase(t).Code())
		})
	})
}

func TestPurchase_Details(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の明細を保持したまま返す", func(t *testing.T) {
			t.Parallel()

			details := accessorPurchase(t).Details()

			require.Len(t, details, 1)
			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "acc_detail"), details[0].ID())
			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "acc_product"), details[0].ProductID())
			assert.Equal(t, 2, details[0].Quantity())
			assert.Equal(t, "800", details[0].UnitPrice().String())
		})

		t.Run("返り値のスライスを書き換えても購入の明細は変わらない", func(t *testing.T) {
			t.Parallel()

			p := accessorPurchase(t)

			got := p.Details()
			require.Len(t, got, 1)
			got[0] = NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "acc_detail_mutated"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "acc_product_mutated"),
				Quantity:  99,
				UnitPrice: mustPrice(t, "1"),
			})

			require.Len(t, p.Details(), 1)
			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "acc_detail"), p.Details()[0].ID())
			assert.Equal(t, 2, p.Details()[0].Quantity())
		})
	})
}

func TestPurchase_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の購入IDを返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "acc_id"), accessorPurchase(t).ID())
		})
	})
}

func TestPurchase_OrderedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の注文日時を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, accessorOrderedAt, accessorPurchase(t).OrderedAt())
		})

		t.Run("Newで生成した集約の場合、ゼロ値を返す", func(t *testing.T) {
			t.Parallel()

			id, code, userID, inputs, locked := validNewArgs(t)
			p, err := New(id, code, userID, inputs, locked)
			require.NoError(t, err)

			assert.True(t, p.OrderedAt().IsZero())
		})
	})
}

func TestPurchase_PaidAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未支払いの場合、nilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, accessorPurchaseWith(t, StatusUnprocessed, nil, nil).PaidAt())
		})

		t.Run("支払い済みの場合、支払い日時を返す", func(t *testing.T) {
			t.Parallel()

			paidAt := time.Date(2026, time.July, 25, 4, 5, 6, 0, time.UTC)
			p := accessorPurchaseWith(t, StatusPaid, &paidAt, nil)

			require.NotNil(t, p.PaidAt())
			assert.Equal(t, paidAt, *p.PaidAt())
		})

		t.Run("再構築時に渡したポインタを変更しても購入のpaidAtは変わらない", func(t *testing.T) {
			t.Parallel()

			paidAt := time.Date(2026, time.July, 25, 4, 5, 6, 0, time.UTC)
			p := accessorPurchaseWith(t, StatusPaid, &paidAt, nil)

			original := paidAt
			paidAt = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

			require.NotNil(t, p.PaidAt())
			assert.Equal(t, original, *p.PaidAt())
		})

		t.Run("返り値のポインタを書き換えても購入のpaidAtは変わらない", func(t *testing.T) {
			t.Parallel()

			paidAt := time.Date(2026, time.July, 25, 4, 5, 6, 0, time.UTC)
			p := accessorPurchaseWith(t, StatusPaid, &paidAt, nil)

			got := p.PaidAt()
			*got = time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

			require.NotNil(t, p.PaidAt())
			assert.NotEqual(t, *got, *p.PaidAt())
			assert.Equal(t, paidAt, *p.PaidAt())
		})
	})
}

func TestPurchase_ShippingFee(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の送料（USDセント）を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 500, accessorPurchase(t).ShippingFee())
		})
	})
}

func TestPurchase_Status(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時のステータスを値オブジェクトで返す", func(t *testing.T) {
			t.Parallel()

			actual := accessorPurchase(t).Status()
			assert.Equal(t, StatusPaid, actual)
			// code だけでなく名前と終端性も伴って返ることを固定する。
			assert.Equal(t, "paid", actual.Name())
			assert.False(t, actual.IsTerminal())
		})

		t.Run("Newで生成した集約の場合、未処理を返す", func(t *testing.T) {
			t.Parallel()

			id, code, userID, inputs, locked := validNewArgs(t)
			p, err := New(id, code, userID, inputs, locked)
			require.NoError(t, err)

			assert.Equal(t, StatusUnprocessed, p.Status())
		})
	})
}

func TestPurchase_StatusCode(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時のステータスコードを返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, StatusPaid.Code(), accessorPurchase(t).StatusCode())
		})

		t.Run("Newで生成した集約の場合、未処理のcodeを返す", func(t *testing.T) {
			t.Parallel()

			id, code, userID, inputs, locked := validNewArgs(t)
			p, err := New(id, code, userID, inputs, locked)
			require.NoError(t, err)

			assert.Equal(t, StatusUnprocessed.Code(), p.StatusCode())
		})
	})
}

func TestPurchase_StatusID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時のステータスIDを返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "acc_status"), accessorPurchase(t).StatusID())
		})

		t.Run("Newで生成した集約の場合、ステータスIDは未解決のためゼロ値を返す", func(t *testing.T) {
			t.Parallel()

			id, code, userID, inputs, locked := validNewArgs(t)
			p, err := New(id, code, userID, inputs, locked)
			require.NoError(t, err)

			assert.True(t, p.StatusID().IsNil())
		})
	})
}

func TestPurchase_SubtotalAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の小計（USDセント）を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 160000, accessorPurchase(t).SubtotalAmount())
		})
	})
}

func TestPurchase_TaxAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の税額（USDセント）を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 16000, accessorPurchase(t).TaxAmount())
		})
	})
}

func TestPurchase_TotalAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("再構築時の合計（USDセント）を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 176500, accessorPurchase(t).TotalAmount())
		})
	})
}

func TestPurchase_UserID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("購入IDではなく購入したユーザーのIDを返す", func(t *testing.T) {
			t.Parallel()

			p := accessorPurchase(t)

			assert.Equal(t, uuidtestkit.NewTestFromSalt(t, "acc_user"), p.UserID())
			assert.NotEqual(t, p.ID(), p.UserID())
		})
	})
}

func Test_validateStatusTimestamps(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未処理でイベント日時が全てnilの場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStatusTimestamps(StatusUnprocessed, nil, nil, nil, nil))
		})

		t.Run("支払い済みでpaidAtがセット済みの場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStatusTimestamps(StatusPaid, &at, nil, nil, nil))
		})

		t.Run("キャンセル済みでcanceledAtがセット済みの場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStatusTimestamps(StatusCanceled, &at, &at, nil, nil))
		})

		t.Run("発送済みでpaidAtとshippedAtがセット済みの場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStatusTimestamps(StatusShipped, &at, nil, &at, nil))
		})

		t.Run("配達済みでpaidAtとshippedAtとdeliveredAtがセット済みの場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateStatusTimestamps(StatusDelivered, &at, nil, &at, &at))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセルstatusなのにcanceledAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusCanceled, nil, nil, nil, nil), ErrInvalidStatusID)
		})

		t.Run("canceledAtがセット済みなのにキャンセルstatusでない場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusUnprocessed, nil, &at, nil, nil), ErrInvalidStatusID)
		})

		t.Run("支払い済みstatusなのにpaidAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusPaid, nil, nil, nil, nil), ErrInvalidStatusID)
		})

		t.Run("発送済みstatusなのにshippedAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusShipped, &at, nil, nil, nil), ErrInvalidStatusID)
		})

		t.Run("キャンセルstatusなのにshippedAtがセット済みの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusCanceled, nil, &at, &at, nil), ErrInvalidStatusID)
		})

		t.Run("キャンセルstatusなのにdeliveredAtがセット済みの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusCanceled, nil, &at, nil, &at), ErrInvalidStatusID)
		})

		t.Run("発送済みstatusなのにpaidAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusShipped, nil, nil, &at, nil), ErrInvalidStatusID)
		})

		t.Run("deliveredAtがセット済みなのにshippedAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusPaid, &at, nil, nil, &at), ErrInvalidStatusID)
		})

		t.Run("配達済みstatusなのにdeliveredAtがnilの場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusDelivered, &at, nil, &at, nil), ErrInvalidStatusID)
		})

		t.Run("deliveredAtがセット済みなのに配達済みstatusでない場合、ErrInvalidStatusIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateStatusTimestamps(StatusShipped, &at, nil, &at, &at), ErrInvalidStatusID)
		})
	})
}

func Test_validateDetails(t *testing.T) {
	t.Parallel()

	detail := func(t *testing.T, idSalt, productSalt string, quantity int) PurchaseDetail {
		t.Helper()
		id := uuid.UUID{}
		if idSalt != "" {
			id = uuidtestkit.NewTestFromSalt(t, idSalt)
		}
		productID := uuid.UUID{}
		if productSalt != "" {
			productID = uuidtestkit.NewTestFromSalt(t, productSalt)
		}

		return NewPurchaseDetail(id, PurchaseDetailAttributes{
			ProductID: productID,
			Quantity:  quantity,
			UnitPrice: mustPrice(t, "800"),
		})
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全ての明細が不変条件を満たす場合、nilを返す", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateDetails([]PurchaseDetail{
				detail(t, "vd_d1", "vd_p1", 1),
				detail(t, "vd_d2", "vd_p2", 3),
			}))
		})

		t.Run("空の集合は呼び出し元が別のエラーで扱うため、nilを返す", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateDetails(nil))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("明細IDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateDetails([]PurchaseDetail{detail(t, "", "vd_p1", 1)}), ErrInvalidID)
		})

		t.Run("productIDがゼロ値の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateDetails([]PurchaseDetail{detail(t, "vd_d1", "", 1)}), ErrInvalidID)
		})

		t.Run("数量が最小値未満の場合、ErrInvalidQuantityを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateDetails([]PurchaseDetail{detail(t, "vd_d1", "vd_p1", 0)}), ErrInvalidQuantity)
		})

		t.Run("同一productIDが重複する場合、ErrDuplicateProductIDを返す", func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, validateDetails([]PurchaseDetail{
				detail(t, "vd_d1", "vd_p1", 1),
				detail(t, "vd_d2", "vd_p1", 2),
			}), ErrDuplicateProductID)
		})
	})
}

func TestPurchase_IsCanceled(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キャンセル済みstatusの場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			canceledAt := time.Date(2026, time.July, 26, 7, 8, 9, 0, time.UTC)
			p := accessorPurchaseWith(t, StatusCanceled, nil, &canceledAt)

			assert.True(t, p.IsCanceled())
		})

		t.Run("キャンセル済みでないstatusの場合、falseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, accessorPurchase(t).IsCanceled())
		})
	})
}

func TestPurchase_IsShippable(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("支払い済みの場合、trueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, accessorPurchase(t).IsShippable())
		})

		t.Run("未処理の場合、支払いを終えていないためfalseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, accessorPurchaseWith(t, StatusUnprocessed, nil, nil).IsShippable())
		})

		t.Run("発送済みの場合、二重発送になるためfalseを返す", func(t *testing.T) {
			t.Parallel()

			p := accessorPurchase(t)
			_, err := p.Ship(accessorOrderedAt.Add(time.Hour))
			require.NoError(t, err)

			assert.False(t, p.IsShippable())
		})

		t.Run("配達済みの場合、終端状態のためfalseを返す", func(t *testing.T) {
			t.Parallel()

			p := accessorPurchase(t)
			_, err := p.Ship(accessorOrderedAt.Add(time.Hour))
			require.NoError(t, err)
			_, err = p.Deliver(accessorOrderedAt.Add(2 * time.Hour))
			require.NoError(t, err)

			assert.False(t, p.IsShippable())
		})

		t.Run("キャンセル済みの場合、終端状態のためfalseを返す", func(t *testing.T) {
			t.Parallel()

			canceledAt := accessorOrderedAt.Add(time.Hour)

			assert.False(t, accessorPurchaseWith(t, StatusCanceled, nil, &canceledAt).IsShippable())
		})

		t.Run("完了の場合、終端状態のためfalseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, accessorPurchaseWith(t, StatusCompleted, nil, nil).IsShippable())
		})
	})
}

func TestPurchase_ApplyCoupon(t *testing.T) {
	t.Parallel()

	// 小計 1615.00 ドル = 161500 セント（800×2 + 15×1）。
	const subtotalCents = 161500

	newTarget := func(t *testing.T) *Purchase {
		t.Helper()
		p, err := New(validNewArgs(t))
		require.NoError(t, err)

		return p
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きを適用し、値引き後の額を基礎に税と合計を計算し直す", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)
			couponID := uuidtestkit.NewTestFromSalt(t, "coupon_applied")

			require.NoError(t, p.ApplyCoupon(couponID, 500))

			require.NotNil(t, p.CouponID())
			assert.Equal(t, couponID, *p.CouponID())
			assert.Equal(t, 500, p.DiscountAmount())
			assert.Equal(t, subtotalCents, p.SubtotalAmount())
			assert.Equal(t, (subtotalCents-500)*10/100, p.TaxAmount())
			assert.Equal(t, subtotalCents-500+p.TaxAmount()+p.ShippingFee(), p.TotalAmount())
		})

		t.Run("小計は値引き前のまま据え置く", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)
			before := p.SubtotalAmount()

			require.NoError(t, p.ApplyCoupon(uuidtestkit.NewTestFromSalt(t, "coupon_subtotal"), 1000))

			assert.Equal(t, before, p.SubtotalAmount())
		})

		t.Run("値引きが小計ちょうどでも適用でき、合計は税と送料だけになる", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)

			require.NoError(t, p.ApplyCoupon(uuidtestkit.NewTestFromSalt(t, "coupon_full"), subtotalCents))

			assert.Zero(t, p.TaxAmount())
			assert.Equal(t, p.ShippingFee(), p.TotalAmount())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きが0の場合、ErrZeroDiscountを返しdetailsにcouponIdを載せる", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)

			err := p.ApplyCoupon(uuidtestkit.NewTestFromSalt(t, "coupon_zero"), 0)

			require.ErrorIs(t, err, ErrZeroDiscount)
			require.ErrorIs(t, err, apperror.ErrValidation)

			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{FieldCouponID}, meta.Details())
			assert.Nil(t, p.CouponID())
		})

		t.Run("値引きが負の場合、ErrZeroDiscountを返す", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)

			err := p.ApplyCoupon(uuidtestkit.NewTestFromSalt(t, "coupon_negative"), -1)

			require.ErrorIs(t, err, ErrZeroDiscount)
			assert.Nil(t, p.CouponID())
		})

		t.Run("値引きが小計を超える場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)

			err := p.ApplyCoupon(uuidtestkit.NewTestFromSalt(t, "coupon_over"), subtotalCents+1)

			require.ErrorIs(t, err, ErrInvalidAmount)
			assert.Nil(t, p.CouponID())
		})

		t.Run("クーポンIDが未設定の場合、ErrInvalidCouponIDを返す", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)

			err := p.ApplyCoupon(uuid.UUID{}, 500)

			require.ErrorIs(t, err, ErrInvalidCouponID)
			assert.Nil(t, p.CouponID())
		})

		t.Run("既に適用済みの場合、ErrCouponAlreadyAppliedを返し値引きを上書きしない", func(t *testing.T) {
			t.Parallel()

			p := newTarget(t)
			first := uuidtestkit.NewTestFromSalt(t, "coupon_first")
			require.NoError(t, p.ApplyCoupon(first, 500))

			err := p.ApplyCoupon(uuidtestkit.NewTestFromSalt(t, "coupon_second"), 1000)

			require.ErrorIs(t, err, ErrCouponAlreadyApplied)
			assert.Equal(t, first, *p.CouponID())
			assert.Equal(t, 500, p.DiscountAmount())
		})
	})
}

func TestPurchase_CouponID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未適用の場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			p, err := New(validNewArgs(t))
			require.NoError(t, err)

			assert.Nil(t, p.CouponID())
		})

		t.Run("返した値を書き換えてもエンティティは変わらない", func(t *testing.T) {
			t.Parallel()

			p, err := New(validNewArgs(t))
			require.NoError(t, err)
			couponID := uuidtestkit.NewTestFromSalt(t, "coupon_defensive")
			require.NoError(t, p.ApplyCoupon(couponID, 500))

			got := p.CouponID()
			require.NotNil(t, got)
			*got = uuidtestkit.NewTestFromSalt(t, "coupon_mutated")

			assert.Equal(t, couponID, *p.CouponID())
		})

		t.Run("再構築時に渡したポインタを書き換えてもエンティティは変わらない", func(t *testing.T) {
			t.Parallel()

			original := uuidtestkit.NewTestFromSalt(t, "cid_coupon")
			couponID := original
			p, err := Reconstruct(uuidtestkit.NewTestFromSalt(t, "cid_id"), Attributes{
				Code:           "cid-code",
				UserID:         uuidtestkit.NewTestFromSalt(t, "cid_user"),
				StatusID:       uuidtestkit.NewTestFromSalt(t, "cid_status"),
				StatusCode:     StatusUnprocessed.Code(),
				SubtotalAmount: 160000,
				DiscountAmount: 16000,
				CouponID:       &couponID,
				TaxAmount:      14400,
				ShippingFee:    500,
				TotalAmount:    158900,
				Details: []PurchaseDetail{
					NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "cid_d1"), PurchaseDetailAttributes{
						ProductID: uuidtestkit.NewTestFromSalt(t, "cid_p1"),
						Quantity:  2,
						UnitPrice: mustPrice(t, "800"),
					}),
				},
				OrderedAt: time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC),
			})
			require.NoError(t, err)

			couponID = uuidtestkit.NewTestFromSalt(t, "cid_mutated")

			require.NotNil(t, p.CouponID())
			assert.Equal(t, original, *p.CouponID())
		})
	})
}

func TestPurchase_DiscountAmount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未適用の場合は0を返す", func(t *testing.T) {
			t.Parallel()

			p, err := New(validNewArgs(t))
			require.NoError(t, err)

			assert.Zero(t, p.DiscountAmount())
		})

		t.Run("適用後は値引き額を返す", func(t *testing.T) {
			t.Parallel()

			p, err := New(validNewArgs(t))
			require.NoError(t, err)
			require.NoError(t, p.ApplyCoupon(uuidtestkit.NewTestFromSalt(t, "coupon_amount"), 750))

			assert.Equal(t, 750, p.DiscountAmount())
		})
	})
}

func Test_settle(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値引きが無い場合は小計をそのまま課税の基礎にする", func(t *testing.T) {
			t.Parallel()

			tax, shipping, total := settle(settlementBasis{Subtotal: 10000, Discount: 0})

			assert.Equal(t, 1000, tax)
			assert.Equal(t, shippingFeeCents, shipping)
			assert.Equal(t, 10000+1000+shippingFeeCents, total)
		})

		t.Run("値引きがある場合は値引き後の額を課税の基礎にする", func(t *testing.T) {
			t.Parallel()

			tax, _, total := settle(settlementBasis{Subtotal: 10000, Discount: 2000})

			assert.Equal(t, 800, tax)
			assert.Equal(t, 8000+800+shippingFeeCents, total)
		})

		t.Run("値引きが小計と同額の場合、税は0で合計は送料だけになる", func(t *testing.T) {
			t.Parallel()

			tax, shipping, total := settle(settlementBasis{Subtotal: 10000, Discount: 10000})

			assert.Zero(t, tax)
			assert.Equal(t, shipping, total)
		})
	})
}

func TestPurchaseDetail_LineTotal(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("単価と数量の積を丸めずに返す", func(t *testing.T) {
			t.Parallel()

			d := NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "line_total"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "line_total_product"),
				Quantity:  3,
				UnitPrice: mustPrice(t, "19.99"),
			})

			assert.Equal(t, "59.97", d.LineTotal().String())
		})

		t.Run("数量が1なら単価と同じ値になる", func(t *testing.T) {
			t.Parallel()

			d := NewPurchaseDetail(uuidtestkit.NewTestFromSalt(t, "line_total_one"), PurchaseDetailAttributes{
				ProductID: uuidtestkit.NewTestFromSalt(t, "line_total_one_product"),
				Quantity:  1,
				UnitPrice: mustPrice(t, "12.5"),
			})

			assert.Equal(t, "12.5", d.LineTotal().String())
		})
	})
}

func Test_validateDiscount(t *testing.T) {
	t.Parallel()

	couponID := func(t *testing.T) *uuid.UUID {
		t.Helper()
		id := uuidtestkit.NewTestFromSalt(t, "validate_discount_coupon")

		return &id
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("クーポンと正の値引きが揃っている場合は通る", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateDiscount(couponID(t), settlementBasis{Subtotal: 10000, Discount: 500}))
		})

		t.Run("どちらも無い場合は通る", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateDiscount(nil, settlementBasis{Subtotal: 10000, Discount: 0}))
		})

		t.Run("値引きが小計ちょうどの場合は通る", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateDiscount(couponID(t), settlementBasis{Subtotal: 10000, Discount: 10000}))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("クーポンがあるのに値引きが0の場合、ErrZeroDiscountを返す", func(t *testing.T) {
			t.Parallel()

			err := validateDiscount(couponID(t), settlementBasis{Subtotal: 10000, Discount: 0})

			require.ErrorIs(t, err, ErrZeroDiscount)
		})

		t.Run("クーポンが無いのに値引きが立っている場合、ErrZeroDiscountを返す", func(t *testing.T) {
			t.Parallel()

			err := validateDiscount(nil, settlementBasis{Subtotal: 10000, Discount: 500})

			require.ErrorIs(t, err, ErrZeroDiscount)
		})

		t.Run("値引きが負の場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()

			err := validateDiscount(couponID(t), settlementBasis{Subtotal: 10000, Discount: -1})

			require.ErrorIs(t, err, ErrInvalidAmount)
		})

		t.Run("値引きが小計を超える場合、ErrInvalidAmountを返す", func(t *testing.T) {
			t.Parallel()

			err := validateDiscount(couponID(t), settlementBasis{Subtotal: 10000, Discount: 10001})

			require.ErrorIs(t, err, ErrInvalidAmount)
		})
	})
}
