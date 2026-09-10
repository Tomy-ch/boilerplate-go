//go:generate oapi-codegen --include-tags=v1/campaigns/detail/suspend --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/campaigns/detail/suspend --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package suspend は、POST /v1/campaigns/{campaignId}/suspend エンドポイントに関連するハンドラを提供します。
package suspend

import (
	"context"
	"net/http"

	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/campaigns/detail/suspend/gen"
	idempotencymw "go-boilerplate/internal/controller/httpstack/idempotency"
	"go-boilerplate/internal/observability"
	campaignuc "go-boilerplate/internal/usecase/campaign"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/labstack/echo/v5"
)

type server struct {
	tracer observability.LayerTracer
	uc     campaignuc.Usecase
	idem   idempotency.Deps
}

// BindHandler は、キャンペーン停止のハンドラを Echo に登録します。
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

// PostCampaignsSuspend は、キャンペーンの受け取りを打ち切ります。
func (s *server) PostCampaignsSuspend(
	ctx context.Context, request gen.PostCampaignsSuspendRequestObject,
) (gen.PostCampaignsSuspendResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	id := uuid.FromPrimitive(request.CampaignId)

	view, _, err := idempotency.Run(
		ctx, s.idem, http.StatusOK, func(ctx context.Context) (campaignuc.CampaignView, error) {
			return s.uc.SuspendCampaign(ctx, &authn, id)
		},
	)
	if err != nil {
		return nil, err
	}

	res, err := toCampaignResponse(view)
	if err != nil {
		return nil, err
	}

	return gen.PostCampaignsSuspend200JSONResponse(res), nil
}

// toCampaignResponse は、停止したキャンペーンを応答の語彙へ写します。
func toCampaignResponse(v campaignuc.CampaignView) (gen.CampaignResponse, error) {
	totalLimit, err := safecast.IntToInt32(v.TotalLimit)
	if err != nil {
		return gen.CampaignResponse{}, err
	}
	perUserLimit, err := safecast.IntToInt32(v.PerUserLimit)
	if err != nil {
		return gen.CampaignResponse{}, err
	}
	issuedCount, err := safecast.IntToInt32(v.IssuedCount)
	if err != nil {
		return gen.CampaignResponse{}, err
	}

	return gen.CampaignResponse{
		Id:   v.ID.ToPrimitive(),
		Code: v.Code,
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
		CouponExpiresAt:   v.CouponExpiresAt,
		StartsAt:          v.StartsAt,
		EndsAt:            v.EndsAt,
		TotalLimit:        totalLimit,
		PerUserLimit:      perUserLimit,
		IssuedCount:       issuedCount,
		SuspendedAt:       v.SuspendedAt,
	}, nil
}

// toPrimitivePtr は、任意の UUID を OpenAPI 生成型のポインタへ写します。nil はそのまま nil です。
func toPrimitivePtr(id *uuid.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	p := id.ToPrimitive()

	return &p
}
