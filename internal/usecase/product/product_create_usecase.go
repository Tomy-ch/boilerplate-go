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
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// 商品が参照する status / category を解決できない参照整合性破れ（サーバ側データ不整合）を表します。
var (
	errMissingStatus   = xerrors.Wrap(apperror.ErrInternal, "status not found for product")
	errMissingCategory = xerrors.Wrap(apperror.ErrInternal, "category not found for product")
)

// CreateProductParams は、商品作成の入力パラメータです。price は十進文字列で受け取り usecase で解釈します。
type CreateProductParams struct {
	Name                  string
	Description           *string
	Price                 string
	Quantity              int
	StockWarningThreshold *int
	CategoryID            uuid.UUID
	StatusID              uuid.UUID
	PublishedAt           *time.Time
	Images                []ProductImageParams
}

// ProductImageParams は、商品画像 1 件分の入力です。
type ProductImageParams struct {
	// ImagePath は、画像アップロード（POST /v1/products/images）で得たオブジェクトのパス（キー）です。
	ImagePath string
	// DisplaySort は、同一商品内での表示順です。
	DisplaySort int
}

func (u *usecase) CreateProduct(ctx context.Context, authn *auth.Authn, params CreateProductParams) (ProductView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if authn == nil {
		return ProductView{}, apperror.ErrUnauthenticated
	}
	if err := u.authorizer.Authorize(ctx, authn, authz.ActionProductCreate, authz.NewResource("product", nil)); err != nil {
		return ProductView{}, err
	}

	price, err := decimal.Parse(params.Price)
	if err != nil {
		return ProductView{}, xerrors.Wrap(apperror.ErrInvalidArgument, "price is not a valid decimal: "+err.Error())
	}
	productPrice, err := money.NewPrice(price)
	if err != nil {
		return ProductView{}, apperror.WithDetails(err, product.FieldPrice)
	}

	id, err := uuid.New()
	if err != nil {
		return ProductView{}, xerrors.Wrap(err, "failed to generate product id")
	}
	images, err := buildImages(params.Images)
	if err != nil {
		return ProductView{}, err
	}

	var entity *product.Product
	err = u.txm.Do(ctx, func(ctx context.Context) error {
		statusRef, categoryRef, err := u.resolveRefs(ctx, params.StatusID, params.CategoryID)
		if err != nil {
			return err
		}

		entity, err = product.New(id, product.Attributes{
			Name:                  params.Name,
			Description:           params.Description,
			Price:                 productPrice,
			Quantity:              params.Quantity,
			StockWarningThreshold: params.StockWarningThreshold,
			Status:                statusRef,
			Category:              categoryRef,
			PublishedAt:           params.PublishedAt,
			Images:                images,
		}, u.clock.Now())
		if err != nil {
			return err
		}

		return u.repo.Create(ctx, entity)
	})
	if err != nil {
		return ProductView{}, err
	}

	return toProductView(entity), nil
}

// buildImages は、入力の商品画像へ ID を採番してドメインの値へ変換します。
// 表示順の重複や範囲外は集約の構築時に検証されるため、ここでは検証しません。
func buildImages(params []ProductImageParams) ([]product.Image, error) {
	images := make([]product.Image, len(params))
	for i, p := range params {
		id, err := uuid.New()
		if err != nil {
			return nil, xerrors.Wrap(err, "failed to generate product image id")
		}
		images[i] = product.NewImage(id, product.ImageAttributes{ImagePath: p.ImagePath, DisplaySort: p.DisplaySort})
	}

	return images, nil
}

// resolveRefs は、status / category の ID から名称を解決し、商品ステータス参照・カテゴリ参照を構築します。
// マスタ不在に加え、取得できたマスタの名称が参照不変条件（NewStatusRef / NewCategoryRef の検証）に
// 反する場合も、いずれもサーバ側データ不整合による参照整合性破れ（errMissingStatus / errMissingCategory）
// としてサーバ内部エラーへ正規化します。クライアント起因の入力エラーではないため 4xx へは露出させません。
func (u *usecase) resolveRefs(
	ctx context.Context, statusID, categoryID uuid.UUID,
) (product.StatusRef, product.CategoryRef, error) {
	statusEntity, err := u.statusRepo.FindByID(ctx, statusID)
	if err != nil {
		if xerrors.Is(err, apperror.ErrNotFound) {
			return product.StatusRef{}, product.CategoryRef{}, errMissingStatus
		}
		return product.StatusRef{}, product.CategoryRef{}, err
	}
	statusRef, err := product.NewStatusRef(statusEntity.ID(), statusEntity.Name())
	if err != nil {
		return product.StatusRef{}, product.CategoryRef{}, errMissingStatus
	}

	categoryEntity, err := u.categoryRepo.FindByID(ctx, categoryID)
	if err != nil {
		if xerrors.Is(err, apperror.ErrNotFound) {
			return product.StatusRef{}, product.CategoryRef{}, errMissingCategory
		}
		return product.StatusRef{}, product.CategoryRef{}, err
	}
	categoryRef, err := product.NewCategoryRef(categoryEntity.ID(), categoryEntity.Name())
	if err != nil {
		return product.StatusRef{}, product.CategoryRef{}, errMissingCategory
	}

	return statusRef, categoryRef, nil
}
