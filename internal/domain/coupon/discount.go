package coupon

import (
	"fmt"

	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/ptr"
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
// 定率は 1 回に引ける額の上限を持てます（[Discount.WithMaxAmount]）。定額は持ちません。
// 適用範囲は関知しません。どの明細が対象かは [Scope] が答えます。
type Discount struct {
	kind      DiscountKind
	value     decimal.Decimal
	maxAmount *int64
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

// NewDiscountKindByName は、外部から渡された名前から値引き種別を解決します。
// 既知でない名前は ErrInvalidDiscountKind を返します。閉じた集合の権威は allDiscountKinds ただ 1 つで、
// code からの解決（[NewDiscountKind]）と同じ一覧を走査します。
func NewDiscountKindByName(name string) (DiscountKind, error) {
	for _, k := range allDiscountKinds() {
		if k.name == name {
			return k, nil
		}
	}
	return DiscountKind{}, xerrors.Wrap(ErrInvalidDiscountKind, fmt.Sprintf("unknown discount kind name: %q", name))
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

// NewDiscount は、解決済みの種別と値から値引きを生成します。種別ごとの生成関数へ振り分けるだけで、
// 検証はその先が行います。永続化された行からの復元も、要求が渡した名前からの構築も、
// 種別を解決したあとはこの入口を通ります。
func NewDiscount(kind DiscountKind, value decimal.Decimal) (Discount, error) {
	switch kind {
	case DiscountKindFlat:
		return NewFlatDiscount(value)
	case DiscountKindRate:
		return NewRateDiscount(value)
	default:
		return Discount{}, xerrors.Wrap(ErrInvalidDiscountKind, "discount kind is required")
	}
}

// WithMaxAmount は、1 回に引ける額の上限を設けた値引きを返します。上限は決済スケール
// （USD セント）の整数です。元の値引きは変わりません。
//
// 上限は定率にのみ意味を持ちます。定額に設定しようとした場合は ErrInvalidMaxAmount を返します
// （定額は引く額そのものが決まっているため、上限は同じことを二重に述べるだけになります）。
// 0 以下の上限は値引きを成立させないため、同じく ErrInvalidMaxAmount を返します。
func (d Discount) WithMaxAmount(maxAmount int64) (Discount, error) {
	if d.kind != DiscountKindRate {
		return Discount{}, xerrors.Wrap(ErrInvalidMaxAmount, "max amount is only meaningful for a rate discount")
	}
	if maxAmount <= 0 {
		return Discount{}, xerrors.Wrap(ErrInvalidMaxAmount, "max amount must be positive")
	}

	d.maxAmount = &maxAmount

	return d, nil
}

// Kind は、値引きの決まり方を返します。
func (d Discount) Kind() DiscountKind { return d.kind }

// MaxAmount は、1 回に引ける額の上限を決済スケール（USD セント）で返します。上限が無い場合は nil です。
func (d Discount) MaxAmount() *int64 { return ptr.Copy(d.maxAmount) }

// LimitToMaxAmount は、決済スケール（USD セント）の値引き額へ上限を適用した額を返します。
// 上限が無い場合と、上限に満たない場合はそのまま返します。
//
// 上限は決済スケール（USD セント）の額に対する条件です
// （適用の位置は docs/spec/domain/coupon.md の Discount を参照）。
func (d Discount) LimitToMaxAmount(cents int64) int64 {
	if d.maxAmount == nil || cents <= *d.maxAmount {
		return cents
	}

	return *d.maxAmount
}

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
