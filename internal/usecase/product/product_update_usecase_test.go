package product

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	domainproduct "go-boilerplate/internal/domain/product"
	mock_category "go-boilerplate/internal/domain/product/category/mock"
	mock_product "go-boilerplate/internal/domain/product/mock"
	mock_status "go-boilerplate/internal/domain/product/status/mock"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	mock_authz "go-boilerplate/internal/usecase/boundary/authz/mock"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	"go-boilerplate/pkg/patch"
	"go-boilerplate/pkg/ptr"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// updateBasePublishedAt は、更新対象商品が現在保持している公開日時です。
var updateBasePublishedAt = time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)

// updateTestDeps は、UpdateProduct のテストで注入する依存モック一式です。
type updateTestDeps struct {
	txm          *mock_tx.MockManager
	repo         *mock_product.MockRepository
	statusRepo   *mock_status.MockRepository
	categoryRepo *mock_category.MockRepository
	authorizer   *mock_authz.MockAuthorizer
}

// newUpdateTarget は、部分更新の対象として読み込み済みの商品エンティティを構築します。
// クリア可能な 4 フィールド（説明・在庫警告閾値・公開日時・画像）はいずれも値を持ち、
// status / category の名称はマスタ再解決との差分が判別できるよう現行値であることが分かる名称にしています。
func newUpdateTarget(t *testing.T, version int) *domainproduct.Product {
	t.Helper()

	statusRef, err := domainproduct.NewStatusRef(uuidtestkit.NewTestFromSalt(t, "update_current_status"), "現行ステータス")
	require.NoError(t, err)
	categoryRef, err := domainproduct.NewCategoryRef(uuidtestkit.NewTestFromSalt(t, "update_current_category"), "現行カテゴリ")
	require.NoError(t, err)

	p, err := domainproduct.Reconstruct(uuidtestkit.NewTestFromSalt(t, "update_product"), domainproduct.Attributes{
		Name:                  "現行商品名",
		Description:           ptr.To("現行説明"),
		Price:                 mustPrice(t, "10.5"),
		Quantity:              5,
		StockWarningThreshold: ptr.To(2),
		Status:                statusRef,
		Category:              categoryRef,
		PublishedAt:           ptr.To(updateBasePublishedAt),
		Images: []domainproduct.Image{
			domainproduct.NewImage(
				uuidtestkit.NewTestFromSalt(t, "update_current_image"),
				domainproduct.ImageAttributes{ImagePath: "products/current.png", DisplaySort: 1},
			),
		},
	}, version, testCreatedAt)
	require.NoError(t, err)

	return p
}

// newUpdateTestUsecase は、モック依存のみで構成した usecase とそのモック一式を返します。
func newUpdateTestUsecase(t *testing.T) (*usecase, *updateTestDeps) {
	t.Helper()

	ctrl := gomock.NewController(t)
	deps := &updateTestDeps{
		txm:          mock_tx.NewMockManager(ctrl),
		repo:         mock_product.NewMockRepository(ctrl),
		statusRepo:   mock_status.NewMockRepository(ctrl),
		categoryRepo: mock_category.NewMockRepository(ctrl),
		authorizer:   mock_authz.NewMockAuthorizer(ctrl),
	}
	u := &usecase{
		tracer:       observability.NewMockUsecaseLayerTracer(t),
		txm:          deps.txm,
		repo:         deps.repo,
		statusRepo:   deps.statusRepo,
		categoryRepo: deps.categoryRepo,
		authorizer:   deps.authorizer,
	}

	return u, deps
}

// expectAuthorizedLoad は、認可通過・トランザクション実行・更新対象の読み込みまでの期待を設定します。
func expectAuthorizedLoad(deps *updateTestDeps, entity *domainproduct.Product) {
	deps.authorizer.EXPECT().
		Authorize(gomock.Any(), gomock.Any(), authz.ActionProductUpdate, gomock.Any()).
		Return(nil)
	deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
	deps.repo.EXPECT().FindByID(gomock.Any(), entity.ID()).Return(entity, nil)
}

func Test_usecase_UpdateProduct(t *testing.T) {
	t.Parallel()

	const (
		currentVersion = 3
		nextVersion    = 4
	)

	// captureUpdate は、repo.Update へ渡されたエンティティを捕捉し、採番後のバージョンを返させます。
	captureUpdate := func(deps *updateTestDeps, captured **domainproduct.Product) {
		deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, p *domainproduct.Product) (int, error) {
				*captured = p
				return nextVersion, nil
			},
		)
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("指定したフィールドのみ更新され、未指定フィールドは現在値が据え置かれる", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)

			var captured *domainproduct.Product
			captureUpdate(deps, &captured)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version:  currentVersion,
				Name:     ptr.To("更新後商品名"),
				Quantity: ptr.To(9),
			})
			require.NoError(t, err)
			require.NotNil(t, captured)

			assert.Equal(t, "更新後商品名", captured.Name())
			assert.Equal(t, 9, captured.Quantity())
			assert.Equal(t, "10.5", captured.Price().String())
			require.NotNil(t, captured.Description())
			assert.Equal(t, "現行説明", *captured.Description())
			require.NotNil(t, captured.StockWarningThreshold())
			assert.Equal(t, 2, *captured.StockWarningThreshold())
			require.NotNil(t, captured.PublishedAt())
			assert.Equal(t, updateBasePublishedAt, *captured.PublishedAt())
			require.Len(t, captured.Images(), 1)
			assert.Equal(t, "products/current.png", captured.Images()[0].ImagePath())
			assert.Equal(t, "現行ステータス", captured.Status().Name())
			assert.Equal(t, "現行カテゴリ", captured.Category().Name())

			assert.Equal(t, "更新後商品名", actual.Name)
			assert.Equal(t, 9, actual.Quantity)
		})

		t.Run("null指定されたフィールドがクリアされる", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)

			var captured *domainproduct.Product
			captureUpdate(deps, &captured)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version:               currentVersion,
				Description:           patch.Null[string](),
				StockWarningThreshold: patch.Null[int](),
				PublishedAt:           patch.Null[time.Time](),
				Images:                patch.Null[[]ProductImageParams](),
			})
			require.NoError(t, err)
			require.NotNil(t, captured)

			assert.Nil(t, captured.Description())
			assert.Nil(t, captured.StockWarningThreshold())
			assert.Nil(t, captured.PublishedAt())
			assert.Empty(t, captured.Images())

			assert.Nil(t, actual.Description)
			assert.Nil(t, actual.PublishedAt)
			assert.Empty(t, actual.Images)
		})

		t.Run("値指定されたフィールドがその値へ更新される", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)

			var captured *domainproduct.Product
			captureUpdate(deps, &captured)

			publishedAt := updateBasePublishedAt.Add(24 * time.Hour)
			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version:               currentVersion,
				Price:                 ptr.To("29.95"),
				Description:           patch.Value("更新後説明"),
				StockWarningThreshold: patch.Value(7),
				PublishedAt:           patch.Value(publishedAt),
				Images: patch.Value([]ProductImageParams{
					{ImagePath: "products/updated.png", DisplaySort: 1},
				}),
			})
			require.NoError(t, err)
			require.NotNil(t, captured)

			assert.Equal(t, "29.95", captured.Price().String())
			require.NotNil(t, captured.Description())
			assert.Equal(t, "更新後説明", *captured.Description())
			require.NotNil(t, captured.StockWarningThreshold())
			assert.Equal(t, 7, *captured.StockWarningThreshold())
			require.NotNil(t, captured.PublishedAt())
			assert.Equal(t, publishedAt, *captured.PublishedAt())
			require.Len(t, captured.Images(), 1)
			assert.Equal(t, "products/updated.png", captured.Images()[0].ImagePath())

			assert.Equal(t, "29.95", actual.Price.String())
		})

		t.Run("imagesが未指定の場合、現在の画像を保持したエンティティで更新する", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)

			var captured *domainproduct.Product
			captureUpdate(deps, &captured)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version: currentVersion,
				Name:    ptr.To("更新後商品名"),
			})
			require.NoError(t, err)
			require.NotNil(t, captured)

			require.Len(t, captured.Images(), 1)
			assert.Equal(t, "products/current.png", captured.Images()[0].ImagePath())
			require.Len(t, actual.Images, 1)
			assert.Equal(t, "products/current.png", actual.Images[0].Path)
		})

		t.Run("statusIdが指定された場合、再解決した名称付き参照へ更新される", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)

			newStatusID := uuidtestkit.NewTestFromSalt(t, "update_new_status")
			deps.statusRepo.EXPECT().FindByID(gomock.Any(), newStatusID).Return(mustStatus(t, newStatusID), nil)
			deps.categoryRepo.EXPECT().
				FindByID(gomock.Any(), entity.Category().ID()).
				Return(mustCategory(t, entity.Category().ID()), nil)

			var captured *domainproduct.Product
			captureUpdate(deps, &captured)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version:  currentVersion,
				StatusID: &newStatusID,
			})
			require.NoError(t, err)
			require.NotNil(t, captured)

			assert.Equal(t, newStatusID, captured.Status().ID())
			assert.Equal(t, "在庫あり", captured.Status().Name())
			assert.Equal(t, newStatusID, actual.StatusID)
			assert.Equal(t, "在庫あり", actual.StatusName)
		})

		t.Run("返却Viewのバージョンは、エンティティ保持値ではなくrepo.Updateが採番した値になる", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)

			var captured *domainproduct.Product
			captureUpdate(deps, &captured)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version: currentVersion,
				Name:    ptr.To("採番確認商品"),
			})
			require.NoError(t, err)
			require.NotNil(t, captured)

			assert.Equal(t, currentVersion, captured.Version())
			assert.Equal(t, nextVersion, actual.Version)
		})

		t.Run("商品更新を所有者なしリソースとして認可する", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)

			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductUpdate, authz.NewResource("product", nil)).
				Return(nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.repo.EXPECT().FindByID(gomock.Any(), entity.ID()).Return(entity, nil)

			var captured *domainproduct.Product
			captureUpdate(deps, &captured)

			_, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version: currentVersion,
				Name:    ptr.To("認可確認商品"),
			})
			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合、ErrUnauthenticated(401)を返す", func(t *testing.T) {
			t.Parallel()

			u, _ := newUpdateTestUsecase(t)

			actual, err := u.UpdateProduct(context.Background(), nil,
				uuidtestkit.NewTestFromSalt(t, "update_unauthenticated"), UpdateProductParams{Version: currentVersion})
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
			assert.Equal(t, ProductView{}, actual)
		})

		t.Run("認可が拒否された場合、権限エラー(403)を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductUpdate, gomock.Any()).
				Return(apperror.ErrPermissionDenied)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).Times(0)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{},
				uuidtestkit.NewTestFromSalt(t, "update_forbidden"), UpdateProductParams{Version: currentVersion})
			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
			assert.Equal(t, ProductView{}, actual)
		})

		t.Run("statusIdに対応するマスタが存在しない場合、整合性異常(500)を返し更新を実行しない", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, currentVersion)
			u, deps := newUpdateTestUsecase(t)
			expectAuthorizedLoad(deps, entity)
			newStatusID := uuidtestkit.NewTestFromSalt(t, "update_missing_status")
			deps.statusRepo.EXPECT().FindByID(gomock.Any(), newStatusID).Return(nil, apperror.ErrNotFound)
			deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(),
				UpdateProductParams{Version: currentVersion, StatusID: &newStatusID})
			require.ErrorIs(t, err, errMissingStatus)
			assert.Equal(t, ProductView{}, actual)
		})

		t.Run("価格が数値として解釈できない場合、トランザクションに入る前にErrInvalidArgument(400)を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductUpdate, gomock.Any()).
				Return(nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).Times(0)

			_, err := u.UpdateProduct(context.Background(), &auth.Authn{},
				uuidtestkit.NewTestFromSalt(t, "update_invalid_price"), UpdateProductParams{
					Version: currentVersion,
					Price:   ptr.To("not-a-number"),
				})
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})

		t.Run("価格が負の場合、トランザクションに入る前に検証エラー(422)を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductUpdate, gomock.Any()).
				Return(nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).Times(0)

			_, err := u.UpdateProduct(context.Background(), &auth.Authn{},
				uuidtestkit.NewTestFromSalt(t, "update_negative_price"), UpdateProductParams{
					Version: currentVersion,
					Price:   ptr.To("-1"),
				})
			require.ErrorIs(t, err, money.ErrNegativePrice)
		})

		t.Run("対象が存在しない場合、NotFound(404)をそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			id := uuidtestkit.NewTestFromSalt(t, "update_missing")
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductUpdate, gomock.Any()).
				Return(nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.repo.EXPECT().FindByID(gomock.Any(), id).Return(nil, apperror.ErrNotFound)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, id,
				UpdateProductParams{Version: currentVersion})
			require.ErrorIs(t, err, apperror.ErrNotFound)
			assert.Equal(t, ProductView{}, actual)
		})

		t.Run("要求バージョンが現在値と一致しない場合、更新を実行せずErrVersionConflict(409)を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)
			deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version: currentVersion - 1,
				Name:    ptr.To("競合商品名"),
			})
			require.ErrorIs(t, err, domainproduct.ErrVersionConflict)
			assert.Equal(t, ProductView{}, actual)
			assert.Equal(t, "現行商品名", entity.Name())
		})

		t.Run("更新後の値が不変条件に違反する場合、更新を実行せず検証エラー(422)を返す", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)
			deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)

			_, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version:  currentVersion,
				Quantity: ptr.To(-1),
			})
			require.ErrorIs(t, err, domainproduct.ErrInvalidQuantity)
		})

		t.Run("永続化時に並行更新でバージョンが一致しない場合、ErrVersionConflict(409)をそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			u, deps := newUpdateTestUsecase(t)
			entity := newUpdateTarget(t, currentVersion)
			expectAuthorizedLoad(deps, entity)
			deps.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(0, domainproduct.ErrVersionConflict)

			actual, err := u.UpdateProduct(context.Background(), &auth.Authn{}, entity.ID(), UpdateProductParams{
				Version: currentVersion,
				Name:    ptr.To("並行更新商品名"),
			})
			require.ErrorIs(t, err, domainproduct.ErrVersionConflict)
			assert.Equal(t, ProductView{}, actual)
		})
	})
}

func Test_usecase_resolveUpdatedRefs(t *testing.T) {
	t.Parallel()

	newRefRepos := func(t *testing.T) (*mock_status.MockRepository, *mock_category.MockRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		return mock_status.NewMockRepository(ctrl), mock_category.NewMockRepository(ctrl)
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("statusId/categoryIdがいずれも未指定の場合、マスタを再解決せず現在の参照を返す", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, 1)
			statusRepo, categoryRepo := newRefRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).Times(0)
			categoryRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).Times(0)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			statusRef, categoryRef, err := u.resolveUpdatedRefs(context.Background(), entity, UpdateProductParams{})
			require.NoError(t, err)
			assert.Equal(t, entity.Status(), statusRef)
			assert.Equal(t, entity.Category(), categoryRef)
		})

		t.Run("statusIdのみ指定の場合、categoryは現在のIDで再解決される", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, 1)
			newStatusID := uuidtestkit.NewTestFromSalt(t, "resolve_updated_status")
			statusRepo, categoryRepo := newRefRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), newStatusID).Return(mustStatus(t, newStatusID), nil)
			categoryRepo.EXPECT().
				FindByID(gomock.Any(), entity.Category().ID()).
				Return(mustCategory(t, entity.Category().ID()), nil)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			statusRef, categoryRef, err := u.resolveUpdatedRefs(context.Background(), entity,
				UpdateProductParams{StatusID: &newStatusID})
			require.NoError(t, err)
			assert.Equal(t, newStatusID, statusRef.ID())
			assert.Equal(t, "在庫あり", statusRef.Name())
			assert.Equal(t, entity.Category().ID(), categoryRef.ID())
			assert.Equal(t, "電子機器", categoryRef.Name())
		})

		t.Run("categoryIdのみ指定の場合、statusは現在のIDで再解決される", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, 1)
			newCategoryID := uuidtestkit.NewTestFromSalt(t, "resolve_updated_category")
			statusRepo, categoryRepo := newRefRepos(t)
			statusRepo.EXPECT().
				FindByID(gomock.Any(), entity.Status().ID()).
				Return(mustStatus(t, entity.Status().ID()), nil)
			categoryRepo.EXPECT().FindByID(gomock.Any(), newCategoryID).Return(mustCategory(t, newCategoryID), nil)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			statusRef, categoryRef, err := u.resolveUpdatedRefs(context.Background(), entity,
				UpdateProductParams{CategoryID: &newCategoryID})
			require.NoError(t, err)
			assert.Equal(t, entity.Status().ID(), statusRef.ID())
			assert.Equal(t, "在庫あり", statusRef.Name())
			assert.Equal(t, newCategoryID, categoryRef.ID())
			assert.Equal(t, "電子機器", categoryRef.Name())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("マスタの再解決に失敗した場合、ゼロ値の参照とエラーを返す", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, 1)
			newStatusID := uuidtestkit.NewTestFromSalt(t, "resolve_updated_missing_status")
			statusRepo, categoryRepo := newRefRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), newStatusID).Return(nil, apperror.ErrNotFound)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			statusRef, categoryRef, err := u.resolveUpdatedRefs(context.Background(), entity,
				UpdateProductParams{StatusID: &newStatusID})
			require.ErrorIs(t, err, errMissingStatus)
			assert.Equal(t, domainproduct.StatusRef{}, statusRef)
			assert.Equal(t, domainproduct.CategoryRef{}, categoryRef)
		})
	})
}

func Test_parseOptionalPrice(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未指定(nil)の場合、価格なしとしてnilを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := parseOptionalPrice(nil)
			require.NoError(t, err)
			assert.Nil(t, actual)
		})

		t.Run("十進文字列をサブセント精度の価格へ解釈する", func(t *testing.T) {
			t.Parallel()

			actual, err := parseOptionalPrice(ptr.To("19.995"))
			require.NoError(t, err)
			require.NotNil(t, actual)
			assert.Equal(t, "19.995", actual.String())
		})

		t.Run("ゼロを受理する", func(t *testing.T) {
			t.Parallel()

			actual, err := parseOptionalPrice(ptr.To("0"))
			require.NoError(t, err)
			require.NotNil(t, actual)
			assert.Equal(t, "0", actual.String())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("数値として解釈できない場合、ErrInvalidArgument(400)を返す", func(t *testing.T) {
			t.Parallel()

			actual, err := parseOptionalPrice(ptr.To("not-a-number"))
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
			assert.Nil(t, actual)
		})

		t.Run("負値の場合、ErrNegativePrice(422)を返す", func(t *testing.T) {
			t.Parallel()

			actual, err := parseOptionalPrice(ptr.To("-0.01"))
			require.ErrorIs(t, err, money.ErrNegativePrice)
			assert.Nil(t, actual)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{domainproduct.FieldPrice}, meta.Details())
		})
	})
}

func Test_resolveUpdatedImages(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未指定の場合、置き換えを行わないことを表すnilを返す", func(t *testing.T) {
			t.Parallel()

			actual, err := resolveUpdatedImages(patch.Unspecified[[]ProductImageParams]())

			require.NoError(t, err)
			assert.Nil(t, actual)
		})

		t.Run("null指定の場合、空の集合を返す", func(t *testing.T) {
			t.Parallel()

			actual, err := resolveUpdatedImages(patch.Null[[]ProductImageParams]())

			require.NoError(t, err)
			require.NotNil(t, actual)
			assert.Empty(t, *actual)
		})

		t.Run("値指定の場合、その内容へ変換した集合を返す", func(t *testing.T) {
			t.Parallel()

			actual, err := resolveUpdatedImages(patch.Value([]ProductImageParams{
				{ImagePath: "products/a.png", DisplaySort: 3},
			}))

			require.NoError(t, err)
			require.NotNil(t, actual)
			require.Len(t, *actual, 1)
			assert.Equal(t, "products/a.png", (*actual)[0].ImagePath())
			assert.Equal(t, 3, (*actual)[0].DisplaySort())
		})
	})
}
