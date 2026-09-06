package product

import (
	"context"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/product"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/pkg/uuid"
)

// UpdateProductStockParams は、商品在庫の増減の入力パラメータです。
type UpdateProductStockParams struct {
	// Delta は、在庫の増減量です。正で補充、負で差し引きを表します。
	Delta int
}

// UpdateProductStock は、admin 認可のうえ、商品の在庫を増減して更新後の商品を返します。
// 取得から更新までを 1 つのトランザクションで行い、購入による在庫減算と同じ商品を対象とする更新を直列化します。
// 更新はバージョンを進めるため、更新前のバージョンを条件とする部分更新（UpdateProduct）は 409 で拒否されます。
// 取得後に他者が更新していたことを検出した場合も 409 を返し、同じ内容の再送では解消しないことを示します。
// 直列化の待機がタイムアウトした場合など、待てば解消しうる失敗は一時的な失敗（503）として返ります。
func (u *usecase) UpdateProductStock(
	ctx context.Context, authn *auth.Authn, id uuid.UUID, params UpdateProductStockParams,
) (ProductView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if authn == nil {
		return ProductView{}, apperror.ErrUnauthenticated
	}
	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionProductStockUpdate, authz.NewResource("product", nil),
	); err != nil {
		return ProductView{}, err
	}

	var entity *product.Product
	var updatedVersion int
	err := u.txm.Do(ctx, func(ctx context.Context) error {
		locked, err := u.repo.LockByID(ctx, id)
		if err != nil {
			return err
		}
		entity = locked

		if err = entity.AdjustStock(params.Delta); err != nil {
			return apperror.WithDetails(err, product.FieldDelta)
		}

		updatedVersion, err = u.repo.UpdateStock(ctx, entity)
		return err
	})
	if err != nil {
		return ProductView{}, err
	}

	view := toProductView(entity)
	// バージョンの採番は永続化時に DB が行うため、エンティティが保持する読み込み時点の値を採番後の値で置き換えます。
	view.Version = updatedVersion

	return view, nil
}
