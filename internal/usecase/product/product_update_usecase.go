package product

import (
	"context"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/internal/domain/product"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/patch"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// UpdateProductParams は、商品部分更新の入力パラメータです。
// nil のポインタは未指定（現在値を据え置く）を表し、クリアも許容するフィールドは patch.Field で 3 状態を表します。
type UpdateProductParams struct {
	// Version は、更新対象を読み込んだ時点の楽観ロックのバージョンです。
	Version int
	// Name は、商品名です。nil の場合は現在値を据え置きます。
	Name *string
	// Price は、十進文字列で表した価格です。nil の場合は現在値を据え置きます。
	Price *string
	// Quantity は、在庫数です。nil の場合は現在値を据え置きます。
	Quantity *int
	// CategoryID は、商品カテゴリ ID です。nil の場合は現在値を据え置きます。
	CategoryID *uuid.UUID
	// StatusID は、商品ステータス ID です。nil の場合は現在値を据え置きます。
	StatusID *uuid.UUID
	// Description は、商品説明です。null 指定でクリアします。
	Description patch.Field[string]
	// StockWarningThreshold は、在庫警告閾値です。null 指定でクリアします。
	StockWarningThreshold patch.Field[int]
	// PublishedAt は、公開日時です。null 指定でクリア（未公開へ戻す）します。
	PublishedAt patch.Field[time.Time]
	// Images は、商品画像の集合です。指定するとその内容へ丸ごと置き換え、null 指定で全て取り除きます。
	// 未指定の場合は現在の画像を据え置き、置き換え自体を行いません。
	Images patch.Field[[]ProductImageParams]
}

// UpdateProduct は、読み込みから更新までを 1 つのトランザクションで行い、読み込み時点のバージョンを
// 条件に更新することで並行編集による上書き（lost update）を防ぎます。
// バージョン不一致の 409 は tx.Manager の透過リトライでは解消しないため、リトライ対象と混同されては
// なりません（クライアントの再試行手順は docs/spec/usecase/product.md の UpdateProduct）。
func (u *usecase) UpdateProduct(
	ctx context.Context, authn *auth.Authn, id uuid.UUID, params UpdateProductParams,
) (ProductView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if authn == nil {
		return ProductView{}, apperror.ErrUnauthenticated
	}
	if err := u.authorizer.Authorize(ctx, authn, authz.ActionProductUpdate, authz.NewResource("product", nil)); err != nil {
		return ProductView{}, err
	}

	price, err := parseOptionalPrice(params.Price)
	if err != nil {
		return ProductView{}, err
	}

	images, err := resolveUpdatedImages(params.Images)
	if err != nil {
		return ProductView{}, err
	}

	var entity *product.Product
	var updatedVersion int
	err = u.txm.Do(ctx, func(ctx context.Context) error {
		loaded, err := u.repo.FindByID(ctx, id)
		if err != nil {
			return err
		}
		entity = loaded

		if err = entity.EnsureVersion(params.Version); err != nil {
			return err
		}

		statusRef, categoryRef, err := u.resolveUpdatedRefs(ctx, entity, params)
		if err != nil {
			return err
		}

		if err = entity.Update(product.Attributes{
			Name:                  ptr.Deref(params.Name, entity.Name()),
			Description:           params.Description.Resolve(entity.Description()),
			Price:                 ptr.Deref(price, entity.Price()),
			Quantity:              ptr.Deref(params.Quantity, entity.Quantity()),
			StockWarningThreshold: params.StockWarningThreshold.Resolve(entity.StockWarningThreshold()),
			Status:                statusRef,
			Category:              categoryRef,
			PublishedAt:           params.PublishedAt.Resolve(entity.PublishedAt()),
			// DiscontinuedAt は UpdateProductParams が持たないため、現在値を据え置きます。
			DiscontinuedAt: entity.DiscontinuedAt(),
			Images:         ptr.Deref(images, entity.Images()),
		}); err != nil {
			return err
		}

		updatedVersion, err = u.repo.Update(ctx, entity)

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

// resolveUpdatedRefs は、部分更新後の商品ステータス参照・カテゴリ参照を解決します。
// 参照はペアで扱い、両方とも未指定の場合のみ現在値を据え置いてマスタ問い合わせを行いません。
// いずれかが指定された場合は、未指定側も現在の ID でマスタと突合し、参照整合を再確認します。
func (u *usecase) resolveUpdatedRefs(
	ctx context.Context, entity *product.Product, params UpdateProductParams,
) (product.StatusRef, product.CategoryRef, error) {
	if params.StatusID == nil && params.CategoryID == nil {
		return entity.Status(), entity.Category(), nil
	}

	statusRef, categoryRef, err := u.resolveRefs(
		ctx,
		ptr.Deref(params.StatusID, entity.Status().ID()),
		ptr.Deref(params.CategoryID, entity.Category().ID()),
	)
	if err != nil {
		return product.StatusRef{}, product.CategoryRef{}, err
	}

	return statusRef, categoryRef, nil
}

// resolveUpdatedImages は、部分更新の指定から置き換え後の商品画像を解決します。
// 未指定の場合は nil を返し、呼び出し側が現在の画像を据え置きます。null 指定は空の集合として扱い、
// 画像を全て取り除きます。
func resolveUpdatedImages(field patch.Field[[]ProductImageParams]) (*[]product.Image, error) {
	if !field.IsSpecified() {
		return nil, nil //nolint:nilnil // 未指定は「画像の指定なし」を表すため nil, nil が妥当です
	}

	specified := field.Resolve(nil)
	if specified == nil {
		return &[]product.Image{}, nil
	}

	images, err := buildImages(*specified)
	if err != nil {
		return nil, err
	}

	return &images, nil
}

// parseOptionalPrice は、指定された場合のみ十進文字列を価格へ解釈します。
// 非数値は入力エラー（400）、負値は業務不変条件違反（422）として扱います。
func parseOptionalPrice(v *string) (*money.Price, error) {
	if v == nil {
		return nil, nil //nolint:nilnil // 未指定は「価格の指定なし」を表すため nil, nil が妥当です
	}

	parsed, err := decimal.Parse(*v)
	if err != nil {
		return nil, xerrors.Wrap(apperror.ErrInvalidArgument, "price is not a valid decimal: "+err.Error())
	}
	price, err := money.NewPrice(parsed)
	if err != nil {
		return nil, apperror.WithDetails(err, product.FieldPrice)
	}

	return &price, nil
}
