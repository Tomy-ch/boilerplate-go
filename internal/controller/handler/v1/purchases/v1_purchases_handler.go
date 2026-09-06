//go:generate oapi-codegen --include-tags=v1/purchases --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/purchases --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package purchases は、/v1/purchases エンドポイントに関連するハンドラを提供します。
package purchases

import (
	"context"
	"net/http"

	"go-boilerplate/internal/controller/conv"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/purchases/gen"
	idempotencymw "go-boilerplate/internal/controller/httpstack/idempotency"
	"go-boilerplate/internal/observability"
	checkoutuc "go-boilerplate/internal/usecase/checkout"
	"go-boilerplate/internal/usecase/idempotency"
	purchaseuc "go-boilerplate/internal/usecase/purchase"
	"go-boilerplate/internal/usecase/tools/paging"
	"go-boilerplate/internal/usecase/tools/timewindow"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/xerrors"

	"github.com/labstack/echo/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type server struct {
	tracer   observability.LayerTracer
	uc       purchaseuc.Usecase
	checkout checkoutuc.Usecase
	idem     idempotency.Deps
}

// BindHandler は、購入の一覧取得・作成のハンドラを Echo に登録します。作成には冪等ミドルウェアを併用します。
func BindHandler(
	e *echo.Echo,
	tf observability.TracerFactory,
	uc purchaseuc.Usecase,
	checkout checkoutuc.Usecase,
	idem idempotency.Deps,
) {
	gen.RegisterHandlers(e, gen.NewStrictHandler(&server{
		tracer:   tf.Controller(),
		uc:       uc,
		checkout: checkout,
		idem:     idem,
	}, []gen.StrictMiddlewareFunc{idempotencymw.StrictMiddleware[gen.StrictHandlerFunc]()}))
}

// GetPurchases は、購入履歴を注文日時降順（cursor ページネーション）で取得します。認証必須です。
// includeOtherUsers の指定時のみ他ユーザーの購入を含みます。
func (s *server) GetPurchases(ctx context.Context, request gen.GetPurchasesRequestObject) (gen.GetPurchasesResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := paging.NewCursor(request.Params.After, request.Params.First)
	if err != nil {
		return nil, err
	}

	window, err := timewindow.New(timewindow.Bounds{
		After:  request.Params.OrderedAfter,
		Before: request.Params.OrderedBefore,
	})
	if err != nil {
		return nil, err
	}

	statusCodes, err := conv.Int16sPtr(request.Params.StatusCodes)
	if err != nil {
		return nil, err
	}

	list, err := s.uc.GetPurchases(ctx, &authn, purchaseuc.ListPurchasesParams{
		Cursor:            cursor,
		Window:            window,
		StatusCodes:       statusCodes,
		ProductID:         conv.UUIDPtr(request.Params.ProductId),
		IncludeOtherUsers: ptr.Deref(request.Params.IncludeOtherUsers, false),
	})
	if err != nil {
		return nil, err
	}

	items := make([]gen.PurchaseSummaryResponse, len(list.Items))
	for i, v := range list.Items {
		items[i] = toPurchaseSummaryResponse(v)
	}

	return gen.GetPurchases200JSONResponse(gen.PurchaseListResponse{
		Items:      items,
		NextCursor: list.NextCursor,
		HasNext:    list.NextCursor != nil,
	}), nil
}

// toPurchaseSummaryResponse は、購入履歴一覧のユースケース DTO を HTTP レスポンスへ変換します。
func toPurchaseSummaryResponse(v purchaseuc.PurchaseSummaryView) gen.PurchaseSummaryResponse {
	return gen.PurchaseSummaryResponse{
		Code:        v.Code,
		TotalAmount: int64(v.TotalAmount),
		Status: gen.PurchaseStatusRef{
			Id:   v.StatusID.ToPrimitive(),
			Code: int64(v.StatusCode),
			Name: v.StatusName,
		},
		FirstItemName: v.FirstItemName,
		ItemCount:     int64(v.ItemCount),
		OrderedAt:     v.OrderedAt,
	}
}

// PostPurchases は、明細から購入を作成します。認証必須で、同一 Idempotency-Key の再送には
// 最初の結果を再生して返します。
func (s *server) PostPurchases(ctx context.Context, request gen.PostPurchasesRequestObject) (gen.PostPurchasesResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	userID, err := ctxhelper.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	details := make([]purchaseuc.DetailParam, len(request.Body.Details))
	for i, d := range request.Body.Details {
		details[i] = purchaseuc.DetailParam{
			ProductID: conv.UUID(d.ProductId),
			Quantity:  int(d.Quantity),
		}
	}

	var displayCurrency *string
	if request.Params.DisplayCurrency != nil {
		s := string(*request.Params.DisplayCurrency)
		displayCurrency = &s
	}

	view, _, err := idempotency.Run(ctx, s.idem, http.StatusCreated, func(ctx context.Context) (checkoutuc.PurchaseView, error) {
		return s.checkout.CreatePurchase(ctx, checkoutuc.CreatePurchaseParams{
			UserID:          userID,
			Details:         details,
			DisplayCurrency: displayCurrency,
			CouponID:        conv.UUIDPtr(request.Body.CouponId),
		})
	})
	if err != nil {
		return nil, err
	}

	res, err := toPurchaseResponse(view.Purchase, view.ReferenceAmount)
	if err != nil {
		return nil, err
	}
	return gen.PostPurchases201JSONResponse(res), nil
}

// toAppliedCouponResponse は、適用したクーポンを応答の語彙へ写します。未適用の場合は nil です。
func toAppliedCouponResponse(v *purchaseuc.AppliedCouponView) *gen.AppliedCouponResponse {
	if v == nil {
		return nil
	}

	var targetID *openapi_types.UUID
	if v.ScopeTargetID != nil {
		p := v.ScopeTargetID.ToPrimitive()
		targetID = &p
	}

	return &gen.AppliedCouponResponse{
		Id: v.ID.ToPrimitive(),
		Discount: gen.CouponDiscount{
			Kind:  gen.CouponDiscountKind(v.DiscountKind),
			Value: v.DiscountValue.String(),
		},
		Scope: gen.CouponScope{
			Kind:     gen.CouponScopeKind(v.ScopeKind),
			TargetId: targetID,
		},
	}
}

// toPurchaseResponse は、ユースケースの DTO を HTTP レスポンスへ変換します。
// 数量が int32 に収まらない場合はエラーを返します。
func toPurchaseResponse(v purchaseuc.PurchaseView, ref *checkoutuc.ReferenceAmountView) (gen.PurchaseResponse, error) {
	details := make([]gen.PurchaseDetailResponse, len(v.Details))
	for i, d := range v.Details {
		quantity, err := safecast.IntToInt32(d.Quantity)
		if err != nil {
			return gen.PurchaseResponse{}, xerrors.Wrap(err, "invalid purchase detail quantity")
		}
		details[i] = gen.PurchaseDetailResponse{
			ProductId: d.ProductID.ToPrimitive(),
			Quantity:  quantity,
			UnitPrice: d.UnitPrice.String(),
		}
	}

	return gen.PurchaseResponse{
		Code:            v.Code,
		UserId:          v.UserID.ToPrimitive(),
		StatusId:        v.StatusID.ToPrimitive(),
		SubtotalAmount:  int64(v.SubtotalAmount),
		DiscountAmount:  int64(v.DiscountAmount),
		AppliedCoupon:   toAppliedCouponResponse(v.AppliedCoupon),
		TaxAmount:       int64(v.TaxAmount),
		ShippingFee:     int64(v.ShippingFee),
		TotalAmount:     int64(v.TotalAmount),
		Details:         details,
		OrderedAt:       v.OrderedAt,
		ReferenceAmount: toReferenceAmount(ref),
	}, nil
}

// toReferenceAmount は、参考換算額の DTO を HTTP レスポンスへ変換します（nil はそのまま nil）。
func toReferenceAmount(r *checkoutuc.ReferenceAmountView) *gen.ReferenceAmount {
	if r == nil {
		return nil
	}
	return &gen.ReferenceAmount{
		Currency: r.Currency,
		Amount:   r.Amount,
		Rate:     r.Rate.String(),
		RateDate: r.RateDate,
	}
}
