//go:generate oapi-codegen --include-tags=v1/campaigns --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/campaigns --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package campaigns は、/v1/campaigns エンドポイントに関連するハンドラを提供します。
package campaigns

import (
	"context"
	"net/http"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/campaigns/gen"
	idempotencymw "go-boilerplate/internal/controller/httpstack/idempotency"
	"go-boilerplate/internal/observability"
	campaignuc "go-boilerplate/internal/usecase/campaign"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/internal/usecase/tools/paging"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/safecast"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/labstack/echo/v5"
)

type server struct {
	tracer observability.LayerTracer
	uc     campaignuc.Usecase
	idem   idempotency.Deps
}

// BindHandler は、キャンペーンの定義と一覧のハンドラを Echo に登録します。
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

// PostCampaigns は、キャンペーンを 1 件定義します。
func (s *server) PostCampaigns(
	ctx context.Context, request gen.PostCampaignsRequestObject,
) (gen.PostCampaignsResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	params, err := toDefineCampaignParams(request.Body)
	if err != nil {
		return nil, err
	}

	view, _, err := idempotency.Run(
		ctx, s.idem, http.StatusCreated, func(ctx context.Context) (campaignuc.CampaignView, error) {
			return s.uc.DefineCampaign(ctx, &authn, params)
		},
	)
	if err != nil {
		return nil, err
	}

	res, err := toCampaignResponse(view)
	if err != nil {
		return nil, err
	}

	return gen.PostCampaigns201JSONResponse(res), nil
}

// GetCampaigns は、キャンペーンを配布期間の開始が新しい順で返します。
func (s *server) GetCampaigns(
	ctx context.Context, request gen.GetCampaignsRequestObject,
) (gen.GetCampaignsResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	page, err := paging.NewPageFrom1Based(request.Params.Page, request.Params.PerPage)
	if err != nil {
		return nil, err
	}

	view, err := s.uc.ListCampaigns(ctx, &authn, page)
	if err != nil {
		return nil, err
	}

	total, err := safecast.IntToInt32(view.Total)
	if err != nil {
		return nil, err
	}

	campaigns := make([]gen.CampaignResponse, len(view.Campaigns))
	for i, c := range view.Campaigns {
		res, cerr := toCampaignResponse(c)
		if cerr != nil {
			return nil, cerr
		}
		campaigns[i] = res
	}

	return gen.GetCampaigns200JSONResponse(gen.CampaignListResponse{
		Campaigns: campaigns,
		Total:     total,
	}), nil
}

// toDefineCampaignParams は、要求本文をユースケース入力の語彙へ写します。
func toDefineCampaignParams(body *gen.CampaignsPostRequest) (campaignuc.DefineCampaignParams, error) {
	value, err := decimal.Parse(body.Discount.Value)
	if err != nil {
		return campaignuc.DefineCampaignParams{}, xerrors.Join(apperror.ErrInvalidArgument, err)
	}

	return campaignuc.DefineCampaignParams{
		Code:              body.Code,
		DiscountKind:      string(body.Discount.Kind),
		DiscountValue:     value,
		MaxAmount:         body.Discount.MaxAmount,
		ScopeKind:         string(body.Scope.Kind),
		ScopeTargetID:     fromPrimitivePtr(body.Scope.TargetId),
		MinPurchaseAmount: body.MinPurchaseAmount,
		UsableFrom:        body.UsableFrom,
		CouponExpiresAt:   body.CouponExpiresAt,
		StartsAt:          body.StartsAt,
		EndsAt:            body.EndsAt,
		TotalLimit:        int(body.TotalLimit),
		PerUserLimit:      int(body.PerUserLimit),
	}, nil
}

// toCampaignResponse は、キャンペーンを応答の語彙へ写します。
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
