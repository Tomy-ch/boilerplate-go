//go:generate oapi-codegen --include-tags=v1/coupons --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/coupons --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package coupons は、/v1/coupons エンドポイントに関連するハンドラを提供します。
package coupons

import (
	"context"
	"net/http"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/coupons/gen"
	idempotencymw "go-boilerplate/internal/controller/httpstack/idempotency"
	"go-boilerplate/internal/observability"
	couponuc "go-boilerplate/internal/usecase/coupon"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/labstack/echo/v5"
)

type server struct {
	tracer observability.LayerTracer
	uc     couponuc.Usecase
	idem   idempotency.Deps
}

// BindHandler は、クーポン発行のハンドラを Echo に登録します。
func BindHandler(
	e *echo.Echo,
	tf observability.TracerFactory,
	uc couponuc.Usecase,
	idem idempotency.Deps,
) {
	gen.RegisterHandlers(e, gen.NewStrictHandler(&server{
		tracer: tf.Controller(),
		uc:     uc,
		idem:   idem,
	}, []gen.StrictMiddlewareFunc{idempotencymw.StrictMiddleware[gen.StrictHandlerFunc]()}))
}

// PostCoupons は、受給者を名指ししてクーポンを 1 枚発行します。
func (s *server) PostCoupons(
	ctx context.Context, request gen.PostCouponsRequestObject,
) (gen.PostCouponsResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	params, err := toIssueCouponParams(request.Body)
	if err != nil {
		return nil, err
	}

	view, _, err := idempotency.Run(ctx, s.idem, http.StatusCreated, func(ctx context.Context) (couponuc.CouponView, error) {
		return s.uc.IssueCoupon(ctx, &authn, params)
	})
	if err != nil {
		return nil, err
	}

	return gen.PostCoupons201JSONResponse(toCouponResponse(view)), nil
}

// toIssueCouponParams は、要求本文をユースケース入力の語彙へ写します。
func toIssueCouponParams(body *gen.CouponsPostRequest) (couponuc.IssueCouponParams, error) {
	value, err := decimal.Parse(body.Discount.Value)
	if err != nil {
		return couponuc.IssueCouponParams{}, xerrors.Join(apperror.ErrInvalidArgument, err)
	}

	return couponuc.IssueCouponParams{
		UserID:            uuid.FromPrimitive(body.UserId),
		DiscountKind:      string(body.Discount.Kind),
		DiscountValue:     value,
		MaxAmount:         body.Discount.MaxAmount,
		ScopeKind:         string(body.Scope.Kind),
		ScopeTargetID:     fromPrimitivePtr(body.Scope.TargetId),
		MinPurchaseAmount: body.MinPurchaseAmount,
		UsableFrom:        body.UsableFrom,
		ExpiresAt:         body.ExpiresAt,
	}, nil
}

// fromPrimitivePtr は、OpenAPI 生成型の任意 UUID をドメイン語彙のポインタへ写します。nil はそのまま nil です。
func fromPrimitivePtr(id *openapi_types.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	converted := uuid.FromPrimitive(*id)

	return &converted
}

// toPrimitivePtr は、任意の UUID を OpenAPI 生成型のポインタへ写します。nil はそのまま nil です。
func toPrimitivePtr(id *uuid.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	p := id.ToPrimitive()

	return &p
}

// toCouponResponse は、発行したクーポンを応答の語彙へ写します。
func toCouponResponse(v couponuc.CouponView) gen.CouponResponse {
	return gen.CouponResponse{
		Id: v.ID.ToPrimitive(),
		Discount: gen.CouponDiscount{
			Kind:      gen.CouponDiscountKind(v.DiscountKind),
			Value:     v.DiscountValue.String(),
			MaxAmount: v.MaxAmount,
		},
		Scope: gen.CouponScope{
			Kind:     gen.CouponScopeKind(v.ScopeKind),
			TargetId: toPrimitivePtr(v.ScopeTargetID),
		},
		MinPurchaseAmount: v.MinPurchaseAmount,
		UsableFrom:        v.UsableFrom,
		ExpiresAt:         v.ExpiresAt,
		UsedAt:            v.UsedAt,
		IssuedAt:          v.IssuedAt,
	}
}
