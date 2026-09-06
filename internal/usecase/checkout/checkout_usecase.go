//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE

// Package checkout は、購入の作成と表示通貨での参考換算額を組み合わせる合成ユースケースを提供します。
//
// 購入と為替はどちらも独立した業務ユースケースであり、一方が他方を直接呼ぶと連鎖ができて
// 業務操作の境界が追跡できなくなります（internal/usecase/README.md の Forbidden dependencies 節）。
// そのため両者を繋ぐ位置に本パッケージを置き、購入側は表示通貨を知らないままに保ちます。
package checkout

import (
	"context"

	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/exchangerate"
	purchaseuc "go-boilerplate/internal/usecase/purchase"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
)

const (
	// baseCurrency は、購入金額の基軸通貨です。金額は本通貨のセント整数で保持します。
	baseCurrency = "USD"
	// centsPerBaseUnit は、基軸通貨 1 単位あたりのセント数です。参考換算の入力金額へ換算します。
	centsPerBaseUnit = 100
	// minorUnitDigits は、基軸通貨の最小単位の小数桁数です（セント = 小数 2 桁）。
	minorUnitDigits = 2
)

// ReferenceAmountView は、参考換算額の出力 DTO です（非永続）。
type ReferenceAmountView struct {
	Currency string
	Amount   int64
	Rate     decimal.Decimal
	RateDate string
}

// PurchaseView は、作成した購入と、要求された表示通貨での参考換算額を組み合わせた出力 DTO です。
// ReferenceAmount は、表示通貨が指定されなかった場合と、為替の取得に失敗した場合に nil になります。
type PurchaseView struct {
	Purchase        purchaseuc.PurchaseView
	ReferenceAmount *ReferenceAmountView
}

// CreatePurchaseParams は、購入作成の入力パラメータです。
type CreatePurchaseParams struct {
	// UserID は、購入者（認証済みの内部ユーザー ID）です。
	UserID uuid.UUID
	// Details は、購入明細の配列です。
	Details []purchaseuc.DetailParam
	// DisplayCurrency は、参考換算額の表示通貨です。nil の場合は参考換算額を返しません。
	DisplayCurrency *string
	// CouponID は、適用するクーポンの ID です。nil の場合は値引きを行いません。
	CouponID *uuid.UUID
}

// Usecase は、購入と参考換算を組み合わせるユースケースを定義します。
type Usecase interface {
	// CreatePurchase は、購入を作成し、表示通貨が指定されていれば参考換算額を添えて返します。
	CreatePurchase(ctx context.Context, params CreatePurchaseParams) (PurchaseView, error)
}

type usecase struct {
	tracer   observability.LayerTracer
	purchase purchaseuc.Usecase
	xr       exchangerate.Usecase
}

// New は、合成ユースケースを生成します。
func New(
	purchase purchaseuc.Usecase,
	xr exchangerate.Usecase,
	tf observability.TracerFactory,
) Usecase {
	return &usecase{
		tracer:   tf.Usecase(),
		purchase: purchase,
		xr:       xr,
	}
}

// CreatePurchase は、購入の作成を購入ユースケースへ委ね、その後に参考換算額を添えます。
// 参考換算の取得に失敗しても購入は成立したまま nil で degrade します。
func (u *usecase) CreatePurchase(ctx context.Context, params CreatePurchaseParams) (PurchaseView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	created, err := u.purchase.CreatePurchase(ctx, purchaseuc.CreatePurchaseParams{
		UserID:   params.UserID,
		Details:  params.Details,
		CouponID: params.CouponID,
	})
	if err != nil {
		return PurchaseView{}, err
	}

	view := PurchaseView{Purchase: created}
	if params.DisplayCurrency != nil {
		view.ReferenceAmount = u.referenceAmount(ctx, created.TotalAmount, *params.DisplayCurrency)
	}
	return view, nil
}

// referenceAmount は、合計金額（基軸通貨のセント）の表示通貨での参考換算額を算出します。
// 為替 gateway の障害時は nil を返します。
func (u *usecase) referenceAmount(ctx context.Context, totalCents int, displayCurrency string) *ReferenceAmountView {
	// 決済スケール（整数セント）の合計を価格スケール（基軸通貨の decimal）へ戻して換算入力にする。
	amount := decimal.FromInt(int64(totalCents)).DivRound(decimal.FromInt(centsPerBaseUnit), minorUnitDigits)
	res, err := u.xr.Convert(ctx, exchangerate.ConvertInput{
		Base:            baseCurrency,
		Quote:           displayCurrency,
		Amount:          amount,
		DisplayCurrency: &displayCurrency,
	})
	if err != nil || res.Reference == nil {
		return nil
	}
	return &ReferenceAmountView{
		Currency: res.Reference.Currency,
		Amount:   res.Reference.Amount,
		Rate:     res.Reference.Rate,
		RateDate: res.Reference.RateDate,
	}
}
