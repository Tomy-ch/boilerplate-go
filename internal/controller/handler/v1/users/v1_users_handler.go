//go:generate oapi-codegen --include-tags=v1/users --package=gen --generate=types -o ./gen/type.gen.go /app/openapi/openapi.gen.yaml
//go:generate oapi-codegen --include-tags=v1/users --package=gen --generate=echo5-server,strict-server -o ./gen/server.gen.go /app/openapi/openapi.gen.yaml

// Package users は、/v1/users エンドポイントに関連するハンドラを提供します。
package users

import (
	"context"
	"net/http"

	"go-boilerplate/internal/controller/conv"
	"go-boilerplate/internal/controller/ctxhelper"
	"go-boilerplate/internal/controller/handler/v1/users/gen"
	idempotencymw "go-boilerplate/internal/controller/httpstack/idempotency"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/idempotency"
	"go-boilerplate/internal/usecase/tools/paging"
	"go-boilerplate/internal/usecase/user"

	"github.com/labstack/echo/v5"
	"github.com/oapi-codegen/runtime/types"
)

type server struct {
	tracer observability.LayerTracer
	uc     user.Usecase
	idem   idempotency.Deps
}

// BindHandler は、ユーザー一覧のハンドラを Echo に登録します。
func BindHandler(e *echo.Echo, tf observability.TracerFactory, uc user.Usecase, idem idempotency.Deps) {
	gen.RegisterHandlers(e, gen.NewStrictHandler(&server{
		tracer: tf.Controller(),
		uc:     uc,
		idem:   idem,
	}, []gen.StrictMiddlewareFunc{idempotencymw.StrictMiddleware[gen.StrictHandlerFunc]()}))
}

// GetUsers は、ユーザー一覧を取得します。
func (s *server) GetUsers(ctx context.Context, request gen.GetUsersRequestObject) (gen.GetUsersResponseObject, error) {
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

	list, err := s.uc.ListUsersWithTotal(ctx, &authn, request.Params.Active, page)
	if err != nil {
		return nil, err
	}

	users := make([]gen.UserResponse, len(list.Items))
	for i, dto := range list.Items {
		users[i] = toUserResponse(dto)
	}

	res := gen.UsersResponse{
		Users:  users,
		Total:  list.Total,
		Limit:  page.Limit(),
		Offset: page.Offset(),
	}

	return gen.GetUsers200JSONResponse(res), nil
}

// PostUsers は、ユーザーを作成します。
func (s *server) PostUsers(ctx context.Context, request gen.PostUsersRequestObject) (gen.PostUsersResponseObject, error) {
	ctx, endSpan := s.tracer.Start(ctx)
	defer endSpan()

	// 登録の入口には内部ユーザーが未解決のまま到達する（docs/design/auth.md）。
	// 内部ユーザー ID はユースケースが採番するため、渡すのは主体を指す issuer + subject だけ。
	authn, err := ctxhelper.RequireAuthn(ctx)
	if err != nil {
		return nil, err
	}

	createParams := &user.CreateParamsDTO{
		Issuer:         authn.Issuer(),
		Subject:        authn.Subject(),
		FirstName:      request.Body.FirstName,
		LastName:       request.Body.LastName,
		Email:          conv.Email(request.Body.Email),
		Phone:          request.Body.Phone,
		PostalCode:     request.Body.PostalCode,
		PrefectureName: request.Body.Prefecture,
		City:           request.Body.City,
		Street:         request.Body.Street,
		Building:       request.Body.Building,
	}

	dto, _, err := idempotency.Run(ctx, s.idem, http.StatusCreated, func(ctx context.Context) (user.UserView, error) {
		return s.uc.CreateUser(ctx, createParams)
	})
	if err != nil {
		return nil, err
	}

	return gen.PostUsers201JSONResponse(toUserResponse(dto)), nil
}

// toUserResponse は、ユースケースのDTOをHTTPレスポンスへ変換します。
func toUserResponse(dto user.UserView) gen.UserResponse {
	return gen.UserResponse{
		Id:         dto.ID.ToPrimitive(),
		FirstName:  dto.FirstName,
		LastName:   dto.LastName,
		Email:      types.Email(dto.Email),
		Phone:      dto.Phone,
		PostalCode: dto.PostalCode,
		Prefecture: dto.PrefectureName,
		City:       dto.City,
		Street:     dto.Street,
		Building:   dto.Building,
		DeletedAt:  dto.DeletedAt,
	}
}
