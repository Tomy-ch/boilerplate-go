//go:generate oapi-codegen --include-tags=v1/purchases/detail --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/purchases/detail --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package detail は、GET /v1/purchases/{purchaseCode} エンドポイントに関連するハンドラを提供します。
package detail

import (
	"context"

	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/purchases/detail/gen"
	"go-boilerplate/internal/observability"
	purchaseuc "go-boilerplate/internal/usecase/purchase"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/xerrors"

	"github.com/labstack/echo/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type server struct {
	tracer observability.LayerTracer
	uc     purchaseuc.Usecase
}

// BindHandler は、購入詳細のハンドラを Echo に登録します。
func BindHandler(e *echo.Echo, tf observability.TracerFactory, uc purchaseuc.Usecase) {
	gen.RegisterHandlers(e, gen.NewStrictHandler(&server{
		tracer: tf.Controller(),
		uc:     uc,
	}, nil))
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

// GetPurchasesDetail は、本人の購入 1 件を明細込みで取得します。認証必須。404: 不存在 / 他人の購入
// （理由は docs/spec/usecase/purchase.md § GET 詳細（購入詳細・集約跨ぎ QS）を参照）。
func (s *server) GetPurchasesDetail(
	ctx context.Context,
	request gen.GetPurchasesDetailRequestObject,
) (gen.GetPurchasesDetailResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	view, err := s.uc.GetPurchaseDetail(ctx, &authn, request.PurchaseCode)
	if err != nil {
		return nil, err
	}

	res, err := toPurchaseGetDetailResponse(view)
	if err != nil {
		return nil, err
	}
	return gen.GetPurchasesDetail200JSONResponse(res), nil
}

// toPurchaseGetDetailResponse は、購入詳細の取得 DTO を HTTP レスポンスへ変換します。
// 数量が int32 に収まらない場合はエラーを返します。
func toPurchaseGetDetailResponse(v purchaseuc.PurchaseGetDetailView) (gen.PurchaseGetDetailResponse, error) {
	details := make([]gen.PurchaseDetailItemResponse, len(v.Details))
	for i, d := range v.Details {
		quantity, err := safecast.IntToInt32(d.Quantity)
		if err != nil {
			return gen.PurchaseGetDetailResponse{}, xerrors.Wrap(err, "invalid purchase detail quantity")
		}
		details[i] = gen.PurchaseDetailItemResponse{
			ProductId:   d.ProductID.ToPrimitive(),
			ProductName: d.ProductName,
			Quantity:    quantity,
			UnitPrice:   d.UnitPrice.String(),
		}
	}

	return gen.PurchaseGetDetailResponse{
		Code:   v.Code,
		UserId: v.UserID.ToPrimitive(),
		Status: gen.PurchaseStatusRef{
			Id:   v.StatusID.ToPrimitive(),
			Code: int64(v.StatusCode),
			Name: v.StatusName,
		},
		SubtotalAmount: v.SubtotalAmount,
		DiscountAmount: v.DiscountAmount,
		AppliedCoupon:  toAppliedCouponResponse(v.AppliedCoupon),
		TaxAmount:      v.TaxAmount,
		ShippingFee:    v.ShippingFee,
		TotalAmount:    v.TotalAmount,
		Details:        details,
		OrderedAt:      v.OrderedAt,
		PaidAt:         v.PaidAt,
		CanceledAt:     v.CanceledAt,
	}, nil
}
