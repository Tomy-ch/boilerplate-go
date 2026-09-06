package coupon

const (
	// minorUnitDigits は、決済通貨（USD）の最小単位の小数桁数です（セント = 小数 2 桁）。
	// 値引き額を価格スケールから決済スケールへ切り捨てる桁数に用います。
	minorUnitDigits = 2

	// FieldCouponID は、クーポン ID フィールドの識別子です。
	FieldCouponID = "couponId"
)
