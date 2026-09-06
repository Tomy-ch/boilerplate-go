package event_test

import (
	"encoding/json"
	"testing"
	"time"

	"go-boilerplate/internal/domain/lexicon/money"
	domainpurchase "go-boilerplate/internal/domain/purchase"
	"go-boilerplate/internal/usecase/purchase/event"
	decimaltestkit "go-boilerplate/pkg/decimal/testkit"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testOrderedAt は、書き込み後に読み直した集約が持つ注文日時です（DB が採番する値の代役）。
var testOrderedAt = time.Date(2026, time.July, 23, 9, 30, 0, 0, time.UTC)

// asReread は、New で組み立てた集約を「書き込み後に読み直した」形へ写します。
// 注文日時は DB 採番なので New 直後の集約には載っておらず、BuildCreated は読み直した集約を受け取ります。
func asReread(t *testing.T, p *domainpurchase.Purchase) *domainpurchase.Purchase {
	t.Helper()
	r, err := domainpurchase.Reconstruct(p.ID(), domainpurchase.Attributes{
		Code:           p.Code(),
		UserID:         p.UserID(),
		StatusID:       uuidtestkit.NewTestFromSalt(t, "reread_status"),
		StatusCode:     p.StatusCode(),
		SubtotalAmount: p.SubtotalAmount(),
		DiscountAmount: p.DiscountAmount(),
		CouponID:       p.CouponID(),
		TaxAmount:      p.TaxAmount(),
		ShippingFee:    p.ShippingFee(),
		TotalAmount:    p.TotalAmount(),
		Details:        p.Details(),
		OrderedAt:      testOrderedAt,
	})
	require.NoError(t, err)

	return r
}

// mustPrice は、テスト用に十進文字列（ドル）から非負の money.Price を構築します。
//
//nolint:unparam // テスト補助ヘルパー。現行の呼び出しは同一値だが用途は可変
func mustPrice(t *testing.T, s string) money.Price {
	t.Helper()
	p, err := money.NewPrice(decimaltestkit.MustParse(t, s))
	require.NoError(t, err)
	return p
}

func TestBuildCreated(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("購入の自己完結スナップショットJSONを生成する", func(t *testing.T) {
			t.Parallel()

			productA := uuidtestkit.NewTestFromSalt(t, "bp_product")
			entity, err := domainpurchase.New(
				uuidtestkit.NewTestFromSalt(t, "bp_id"),
				"bp-code",
				uuidtestkit.NewTestFromSalt(t, "bp_user"),
				[]domainpurchase.DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "bp_d"), ProductID: productA, Quantity: 2}},
				[]domainpurchase.LockedProduct{domainpurchase.NewLockedProduct(productA, mustPrice(t, "800"), 20)},
			)
			require.NoError(t, err)

			payload, perr := event.BuildCreated(asReread(t, entity))
			require.NoError(t, perr)

			var decoded struct {
				PurchaseID     string `json:"purchaseId"`
				Code           string `json:"code"`
				UserID         string `json:"userId"`
				StatusCode     int    `json:"statusCode"`
				SubtotalAmount int    `json:"subtotalAmount"`
				TaxAmount      int    `json:"taxAmount"`
				ShippingFee    int    `json:"shippingFee"`
				TotalAmount    int    `json:"totalAmount"`
				OrderedAt      string `json:"orderedAt"`
				Details        []struct {
					ProductID string `json:"productId"`
					Quantity  int    `json:"quantity"`
					UnitPrice string `json:"unitPrice"`
				} `json:"details"`
			}
			require.NoError(t, json.Unmarshal(payload, &decoded))
			assert.Equal(t, entity.ID().String(), decoded.PurchaseID)
			assert.Equal(t, "bp-code", decoded.Code)
			assert.Equal(t, entity.UserID().String(), decoded.UserID)
			assert.Equal(t, domainpurchase.StatusUnprocessed.Code(), decoded.StatusCode)
			// subtotal=160000 / tax=16000（切り捨て10%）/ shipping=500 / total=176500
			assert.Equal(t, 160000, decoded.SubtotalAmount)
			assert.Equal(t, 16000, decoded.TaxAmount)
			assert.Equal(t, 500, decoded.ShippingFee)
			assert.Equal(t, 176500, decoded.TotalAmount)
			assert.Equal(t, testOrderedAt.Format(time.RFC3339Nano), decoded.OrderedAt)
			require.Len(t, decoded.Details, 1)
			assert.Equal(t, productA.String(), decoded.Details[0].ProductID)
			assert.Equal(t, 2, decoded.Details[0].Quantity)
			assert.Equal(t, "800", decoded.Details[0].UnitPrice)
		})

		t.Run("クーポンを適用した購入は値引き額とクーポンIDを載せ、金額が突き合う", func(t *testing.T) {
			t.Parallel()

			productA := uuidtestkit.NewTestFromSalt(t, "bpc_product")
			entity, err := domainpurchase.New(
				uuidtestkit.NewTestFromSalt(t, "bpc_id"),
				"bpc-code",
				uuidtestkit.NewTestFromSalt(t, "bpc_user"),
				[]domainpurchase.DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "bpc_d"), ProductID: productA, Quantity: 2}},
				[]domainpurchase.LockedProduct{domainpurchase.NewLockedProduct(productA, mustPrice(t, "800"), 20)},
			)
			require.NoError(t, err)
			couponID := uuidtestkit.NewTestFromSalt(t, "bpc_coupon")
			require.NoError(t, entity.ApplyCoupon(couponID, 16000))

			payload, perr := event.BuildCreated(asReread(t, entity))
			require.NoError(t, perr)

			var decoded struct {
				SubtotalAmount int     `json:"subtotalAmount"`
				DiscountAmount int     `json:"discountAmount"`
				TaxAmount      int     `json:"taxAmount"`
				ShippingFee    int     `json:"shippingFee"`
				TotalAmount    int     `json:"totalAmount"`
				CouponID       *string `json:"couponId"`
			}
			require.NoError(t, json.Unmarshal(payload, &decoded))
			require.NotNil(t, decoded.CouponID)
			assert.Equal(t, couponID.String(), *decoded.CouponID)
			// subtotal=160000 / discount=16000 / 課税基礎=144000 → tax=14400 / shipping=500 / total=158900
			assert.Equal(t, 160000, decoded.SubtotalAmount)
			assert.Equal(t, 16000, decoded.DiscountAmount)
			assert.Equal(t, 14400, decoded.TaxAmount)
			assert.Equal(t, 500, decoded.ShippingFee)
			assert.Equal(t, 158900, decoded.TotalAmount)
			// 購読側が snapshot だけで合計を組み立て直せることを固定する。
			assert.Equal(t,
				decoded.TotalAmount,
				decoded.SubtotalAmount-decoded.DiscountAmount+decoded.TaxAmount+decoded.ShippingFee,
			)
		})

		t.Run("クーポン無しの購入は値引き額 0 とクーポンID null を載せる", func(t *testing.T) {
			t.Parallel()

			productA := uuidtestkit.NewTestFromSalt(t, "bpn_product")
			entity, err := domainpurchase.New(
				uuidtestkit.NewTestFromSalt(t, "bpn_id"),
				"bpn-code",
				uuidtestkit.NewTestFromSalt(t, "bpn_user"),
				[]domainpurchase.DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "bpn_d"), ProductID: productA, Quantity: 1}},
				[]domainpurchase.LockedProduct{domainpurchase.NewLockedProduct(productA, mustPrice(t, "800"), 20)},
			)
			require.NoError(t, err)

			payload, perr := event.BuildCreated(asReread(t, entity))
			require.NoError(t, perr)

			var decoded struct {
				DiscountAmount int     `json:"discountAmount"`
				CouponID       *string `json:"couponId"`
			}
			require.NoError(t, json.Unmarshal(payload, &decoded))
			assert.Equal(t, 0, decoded.DiscountAmount)
			assert.Nil(t, decoded.CouponID)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("注文日時を持たない集約はエラーにする", func(t *testing.T) {
			t.Parallel()

			// 生成直後の集約をそのまま渡すと 0001-01-01 が snapshot に載る。
			// 型では読み直し済みかどうかを区別できないので、ここが唯一の歯止めになる。
			productA := uuidtestkit.NewTestFromSalt(t, "bpz_product")
			entity, err := domainpurchase.New(
				uuidtestkit.NewTestFromSalt(t, "bpz_id"),
				"bpz-code",
				uuidtestkit.NewTestFromSalt(t, "bpz_user"),
				[]domainpurchase.DetailInput{{ID: uuidtestkit.NewTestFromSalt(t, "bpz_d"), ProductID: productA, Quantity: 1}},
				[]domainpurchase.LockedProduct{domainpurchase.NewLockedProduct(productA, mustPrice(t, "800"), 20)},
			)
			require.NoError(t, err)
			require.True(t, entity.OrderedAt().IsZero())

			_, perr := event.BuildCreated(entity)

			require.Error(t, perr)
		})
	})
}
