package purchase

import "math"

const (
	// taxRatePercent は、消費税率（パーセント）です。
	taxRatePercent = 10

	// shippingFeeCents は、固定送料（USD セント）です。
	shippingFeeCents = 500

	// percentDivisor は、パーセント計算の除数です（taxRatePercent を百分率として扱うため）。
	percentDivisor = 100

	// minorUnitDigits は、決済通貨（USD）の最小単位の小数桁数です（セント = 小数 2 桁）。
	// 価格スケール（ドル decimal）から決済スケール（整数セント）へ切り捨てる際の桁数に用います。
	minorUnitDigits = 2

	// FieldCouponID は、クーポン ID フィールドの識別子です。
	FieldCouponID = "couponId"

	// minQuantity は、明細 1 件あたりの最小購入数量です。
	minQuantity = 1

	// maxSubtotalCents は、税と送料を加えても決済スケールの整数幅に収まる小計の上限です。
	// 税額の算出が小計に taxRatePercent を掛けるため、その積が幅に収まる範囲がそのまま上限になります。
	maxSubtotalCents = math.MaxInt64 / taxRatePercent
)
