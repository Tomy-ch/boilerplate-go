//go:generate oapi-codegen --include-tags=v1/coupons/bulk-issue --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/coupons/bulk-issue --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package couponsbulkissue は、/v1/coupons/bulk-issue エンドポイントに関連するハンドラを提供します。
package couponsbulkissue

import (
	"context"
	"net/http"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/coupons/bulkissue/gen"
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

// BindHandler は、販促クーポン一括発行のハンドラを Echo に登録します。
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

// PostCouponsBulkIssue は、退会していないすべての利用者へ販促クーポンを一括発行します。
func (s *server) PostCouponsBulkIssue(
	ctx context.Context, request gen.PostCouponsBulkIssueRequestObject,
) (gen.PostCouponsBulkIssueResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	params, err := toIssuePromotionalCouponsParams(request.Body)
	if err != nil {
		return nil, err
	}

	view, _, err := idempotency.Run(ctx, s.idem, http.StatusOK, func(ctx context.Context) (couponuc.IssuePromotionalCouponsView, error) {
		return s.uc.IssuePromotionalCoupons(ctx, &authn, params)
	})
	if err != nil {
		return nil, err
	}

	return gen.PostCouponsBulkIssue200JSONResponse(toBulkIssueResponse(view)), nil
}

// toIssuePromotionalCouponsParams は、要求本文をユースケース入力の語彙へ写します。
func toIssuePromotionalCouponsParams(
	body *gen.CouponBulkIssuePostRequest,
) (couponuc.IssuePromotionalCouponsParams, error) {
	value, err := decimal.Parse(body.Discount.Value)
	if err != nil {
		return couponuc.IssuePromotionalCouponsParams{}, xerrors.Join(apperror.ErrInvalidArgument, err)
	}

	return couponuc.IssuePromotionalCouponsParams{
		DiscountKind:  string(body.Discount.Kind),
		DiscountValue: value,
		ScopeKind:     string(body.Scope.Kind),
		ScopeTargetID: fromPrimitivePtr(body.Scope.TargetId),
		ExpiresAt:     body.ExpiresAt,
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

// toBulkIssueResponse は、ユースケース出力を応答の語彙へ写します。
func toBulkIssueResponse(view couponuc.IssuePromotionalCouponsView) gen.CouponBulkIssueResponse {
	return gen.CouponBulkIssueResponse{
		IssuedAt:          view.IssuedAt,
		ExpiresAt:         view.ExpiresAt,
		RecipientCount:    view.RecipientCount,
		IssuedCouponCount: view.IssuedCouponCount,
	}
}
