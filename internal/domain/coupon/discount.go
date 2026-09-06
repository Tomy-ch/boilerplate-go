package coupon

import (
	"fmt"

	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/xerrors"
)

// 既知の値引き種別の業務キー。順序を持たない理由は [DiscountKind] を参照。
const (
	discountKindFlat = 1
	discountKindRate = 2
)

// 既知の値引き種別。
var (
	// DiscountKindFlat は、金額を直接差し引く「定額」です。
	DiscountKindFlat = DiscountKind{code: discountKindFlat, name: "flat"}
	// DiscountKindRate は、対象額に率を掛けて差し引く「定率」です。
	DiscountKindRate = DiscountKind{code: discountKindRate, name: "rate"}

	// maxDiscountRate は、定率の値引きが取りうる上限です。decimal は const にできないため var で置きます。
	maxDiscountRate = decimal.FromInt(1)
)

// DiscountKind は、値引きの決まり方を表す値オブジェクトです。
//
// 内側に持つ code は永続化と外部公開のための業務キーであり、種別の間に順序はありません。
// マスタ表を持たない理由は docs/spec/domain/coupon.md の Overview を参照してください。
type DiscountKind struct {
	code int
	name string
}

// Discount は、値引きを表す値オブジェクトです。決まり方（種別）と、その種別における値の組です。
//
// 定額では value が差し引く金額（USD ドルの十進量）、定率では value が対象額に掛ける率です。
// 適用範囲は関知しません。どの明細が対象かは [Scope] が答えます。
type Discount struct {
	kind  DiscountKind
	value decimal.Decimal
}

// allDiscountKinds は、既知の値引き種別一覧です。code からの解決に用います。
func allDiscountKinds() []DiscountKind {
	return []DiscountKind{DiscountKindFlat, DiscountKindRate}
}

// NewDiscountKind は、永続化されている code から値引き種別を解決します。
// 既知でない code は ErrInvalidDiscountKind を返します。
func NewDiscountKind(code int) (DiscountKind, error) {
	for _, k := range allDiscountKinds() {
		if k.code == code {
			return k, nil
		}
	}
	return DiscountKind{}, xerrors.Wrap(ErrInvalidDiscountKind, fmt.Sprintf("unknown discount kind code: %d", code))
}

// Code は、永続化と外部公開に用いる業務キーを返します。
func (k DiscountKind) Code() int { return k.code }

// Name は、値引き種別の名前を返します。外部へ種別を伝えるときは code ではなくこちらを用います。
func (k DiscountKind) Name() string { return k.name }

// IsZero は、未設定の値引き種別かどうかを返します。
func (k DiscountKind) IsZero() bool { return k.code == 0 }

// NewFlatDiscount は、定額の値引きを生成します。amount は正の十進量である必要があります。
// 0 以下は値引きにならないため ErrInvalidDiscountValue を返します。
func NewFlatDiscount(amount decimal.Decimal) (Discount, error) {
	if amount.Sign() <= 0 {
		return Discount{}, xerrors.Wrap(ErrInvalidDiscountValue, "flat discount amount must be positive")
	}
	return Discount{kind: DiscountKindFlat, value: amount}, nil
}

// NewRateDiscount は、定率の値引きを生成します。rate は 0 より大きく 1 以下である必要があります。
// 範囲外は ErrInvalidDiscountValue を返します（上限の根拠は docs/spec/domain/coupon.md の Discount）。
func NewRateDiscount(rate decimal.Decimal) (Discount, error) {
	if rate.Sign() <= 0 || rate.Cmp(maxDiscountRate) > 0 {
		return Discount{}, xerrors.Wrap(ErrInvalidDiscountValue, "rate discount must be within (0, 1]")
	}
	return Discount{kind: DiscountKindRate, value: rate}, nil
}

// ReconstructDiscount は、永続化されている種別と値から値引きを再構築します。
// 検証は生成時と同一です。
func ReconstructDiscount(kind DiscountKind, value decimal.Decimal) (Discount, error) {
	switch kind {
	case DiscountKindFlat:
		return NewFlatDiscount(value)
	case DiscountKindRate:
		return NewRateDiscount(value)
	default:
		return Discount{}, xerrors.Wrap(ErrInvalidDiscountKind, "discount kind is required")
	}
}

// Kind は、値引きの決まり方を返します。
func (d Discount) Kind() DiscountKind { return d.kind }

// Value は、種別における値を返します。定額なら差し引く金額、定率なら掛ける率です。
func (d Discount) Value() decimal.Decimal { return d.value }

// IsZero は、未設定の値引きかどうかを返します。
func (d Discount) IsZero() bool { return d.kind.IsZero() }

// Apply は、対象額に対して差し引く額を価格スケールの十進量で返します。
//
// 「いくら引くか」の定義はこのメソッドが持ちます。定額は対象額を上限に切り詰め、定率は対象額に
// 率を掛けます。どちらも対象額を超えないため、請求額が負になることはありません。
// 対象額が 0 以下なら差し引く額も 0 です。
//
// 丸めません。決済スケールへの丸めは [Coupon.DiscountFor] が 1 箇所で行います
// （ADR-0038 (two-scale-quantity-model)）。
func (d Discount) Apply(eligible decimal.Decimal) decimal.Decimal {
	if eligible.Sign() <= 0 {
		return decimal.FromInt(0)
	}

	switch d.kind {
	case DiscountKindFlat:
		if d.value.Cmp(eligible) > 0 {
			return eligible
		}
		return d.value
	case DiscountKindRate:
		return eligible.Mul(d.value)
	default:
		return decimal.FromInt(0)
	}
}
