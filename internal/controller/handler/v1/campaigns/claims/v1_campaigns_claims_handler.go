//go:generate oapi-codegen --include-tags=v1/campaigns/claims --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/campaigns/claims --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package claims は、POST /v1/campaigns/claims エンドポイントに関連するハンドラを提供します。
package claims

import (
	"context"
	"net/http"

	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/campaigns/claims/gen"
	idempotencymw "go-boilerplate/internal/controller/httpstack/idempotency"
	"go-boilerplate/internal/observability"
	campaignuc "go-boilerplate/internal/usecase/campaign"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/labstack/echo/v5"
)

type server struct {
	tracer observability.LayerTracer
	uc     campaignuc.Usecase
	idem   idempotency.Deps
}

// BindHandler は、コードによる受け取りのハンドラを Echo に登録します。
func BindHandler(
	e *echo.Echo,
	tf observability.TracerFactory,
	uc campaignuc.Usecase,
	idem idempotency.Deps,
) {
	gen.RegisterHandlers(e, gen.NewStrictHandler(&server{
		tracer: tf.Controller(),
		uc:     uc,
		idem:   idem,
	}, []gen.StrictMiddlewareFunc{idempotencymw.StrictMiddleware[gen.StrictHandlerFunc]()}))
}

// PostCampaignsClaims は、コードを示してクーポンを 1 枚受け取ります。
func (s *server) PostCampaignsClaims(
	ctx context.Context, request gen.PostCampaignsClaimsRequestObject,
) (gen.PostCampaignsClaimsResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	view, _, err := idempotency.Run(
		ctx, s.idem, http.StatusCreated, func(ctx context.Context) (campaignuc.ClaimedCouponView, error) {
			return s.uc.ClaimCoupon(ctx, &authn, request.Body.Code)
		},
	)
	if err != nil {
		return nil, err
	}

	return gen.PostCampaignsClaims201JSONResponse(toCouponResponse(view)), nil
}

// toCouponResponse は、受け取ったクーポンを応答の語彙へ写します。
func toCouponResponse(v campaignuc.ClaimedCouponView) gen.CouponResponse {
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

// toPrimitivePtr は、任意の UUID を OpenAPI 生成型のポインタへ写します。nil はそのまま nil です。
func toPrimitivePtr(id *uuid.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	p := id.ToPrimitive()

	return &p
}
