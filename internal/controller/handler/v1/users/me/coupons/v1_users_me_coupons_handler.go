//go:generate oapi-codegen --include-tags=v1/users/me/coupons --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/users/me/coupons --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package usersmecoupons は、/v1/users/me/coupons エンドポイントに関連するハンドラを提供します。
package usersmecoupons

import (
	"context"

	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/users/me/coupons/gen"
	"go-boilerplate/internal/observability"
	couponuc "go-boilerplate/internal/usecase/coupon"
	"go-boilerplate/pkg/uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/labstack/echo/v5"
)

type server struct {
	tracer observability.LayerTracer
	uc     couponuc.Usecase
}

// BindHandler は、認証ユーザー自身の保有クーポンのハンドラを Echo に登録します。
func BindHandler(e *echo.Echo, tf observability.TracerFactory, uc couponuc.Usecase) {
	gen.RegisterHandlers(e, gen.NewStrictHandler(&server{
		tracer: tf.Controller(),
		uc:     uc,
	}, nil))
}

// GetUsersMeCoupons は、認証コンテキストの内部 UserID に該当するユーザーの保有クーポンを取得します。
func (s *server) GetUsersMeCoupons(
	ctx context.Context, _ gen.GetUsersMeCouponsRequestObject,
) (gen.GetUsersMeCouponsResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	views, err := s.uc.ListMyCoupons(ctx, &authn)
	if err != nil {
		return nil, err
	}

	return gen.GetUsersMeCoupons200JSONResponse(gen.CouponListResponse{
		Coupons: toCouponResponses(views),
	}), nil
}

// toCouponResponses は、ユースケース出力を応答の語彙へ写します。
func toCouponResponses(views []couponuc.CouponView) []gen.CouponResponse {
	responses := make([]gen.CouponResponse, len(views))
	for i, v := range views {
		responses[i] = toCouponResponse(v)
	}

	return responses
}

// toPrimitivePtr は、任意の UUID を OpenAPI 生成型のポインタへ写します。nil はそのまま nil です。
func toPrimitivePtr(id *uuid.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	p := id.ToPrimitive()

	return &p
}

// toCouponResponse は、クーポン 1 枚を応答の語彙へ写します。
func toCouponResponse(v couponuc.CouponView) gen.CouponResponse {
	return gen.CouponResponse{
		Id: v.ID.ToPrimitive(),
		Discount: gen.CouponDiscount{
			Kind:  gen.CouponDiscountKind(v.DiscountKind),
			Value: v.DiscountValue.String(),
		},
		Scope: gen.CouponScope{
			Kind:     gen.CouponScopeKind(v.ScopeKind),
			TargetId: toPrimitivePtr(v.ScopeTargetID),
		},
		ExpiresAt: v.ExpiresAt,
		UsedAt:    v.UsedAt,
		IssuedAt:  v.IssuedAt,
	}
}
