package product

import (
	"context"
	"testing"

	"go-boilerplate/internal/apperror"
	domainproduct "go-boilerplate/internal/domain/product"
	mock_product "go-boilerplate/internal/domain/product/mock"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	mock_authz "go-boilerplate/internal/usecase/boundary/authz/mock"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// stockTestDeps は、UpdateProductStock のテストで注入する依存モック一式です。
type stockTestDeps struct {
	txm        *mock_tx.MockManager
	repo       *mock_product.MockRepository
	authorizer *mock_authz.MockAuthorizer
}

// newStockTestUsecase は、モック依存のみで構成した usecase とそのモック一式を返します。
func newStockTestUsecase(t *testing.T) (*usecase, *stockTestDeps) {
	t.Helper()

	ctrl := gomock.NewController(t)
	deps := &stockTestDeps{
		txm:        mock_tx.NewMockManager(ctrl),
		repo:       mock_product.NewMockRepository(ctrl),
		authorizer: mock_authz.NewMockAuthorizer(ctrl),
	}
	u := &usecase{
		tracer:     observability.NewMockUsecaseLayerTracer(t),
		txm:        deps.txm,
		repo:       deps.repo,
		authorizer: deps.authorizer,
	}

	return u, deps
}

func Test_usecase_UpdateProductStock(t *testing.T) {
	t.Parallel()

	const (
		currentVersion = 3
		nextVersion    = 4
	)

	// expectAuthorizedLock は、認可通過・トランザクション実行・対象商品の取得までの期待を設定します。
	expectAuthorizedLock := func(deps *stockTestDeps, entity *domainproduct.Product) {
		deps.authorizer.EXPECT().
			Authorize(gomock.Any(), gomock.Any(), authz.ActionProductStockUpdate, gomock.Any()).
			Return(nil)
		deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
		deps.repo.EXPECT().LockByID(gomock.Any(), entity.ID()).Return(entity, nil)
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("正のdeltaを指定した場合、在庫が加算され採番後のバージョンが返る", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, currentVersion)
			before := entity.Quantity()
			u, deps := newStockTestUsecase(t)
			expectAuthorizedLock(deps, entity)

			var captured *domainproduct.Product
			deps.repo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, p *domainproduct.Product) (int, error) {
					captured = p
					return nextVersion, nil
				},
			)

			actual, err := u.UpdateProductStock(context.Background(), &auth.Authn{}, entity.ID(),
				UpdateProductStockParams{Delta: 10})
			require.NoError(t, err)
			require.NotNil(t, captured)
			assert.Equal(t, before+10, captured.Quantity())
			assert.Equal(t, before+10, actual.Quantity)
			assert.Equal(t, nextVersion, actual.Version)
		})

		t.Run("負のdeltaを指定した場合、在庫が減算される", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, currentVersion)
			before := entity.Quantity()
			u, deps := newStockTestUsecase(t)
			expectAuthorizedLock(deps, entity)
			deps.repo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(nextVersion, nil)

			actual, err := u.UpdateProductStock(context.Background(), &auth.Authn{}, entity.ID(),
				UpdateProductStockParams{Delta: -2})
			require.NoError(t, err)
			assert.Equal(t, before-2, actual.Quantity)
		})

		t.Run("名称・ステータス・カテゴリ・公開日時は取得時の値がそのまま返る", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, currentVersion)
			u, deps := newStockTestUsecase(t)
			expectAuthorizedLock(deps, entity)
			deps.repo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(nextVersion, nil)

			actual, err := u.UpdateProductStock(context.Background(), &auth.Authn{}, entity.ID(),
				UpdateProductStockParams{Delta: 1})
			require.NoError(t, err)
			assert.Equal(t, entity.Name(), actual.Name)
			assert.Equal(t, entity.Status().ID(), actual.StatusID)
			assert.Equal(t, entity.Status().Name(), actual.StatusName)
			assert.Equal(t, entity.Category().ID(), actual.CategoryID)
			assert.Equal(t, entity.PublishedAt(), actual.PublishedAt)
		})

		t.Run("在庫更新を所有者なしリソースとして認可する", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, currentVersion)
			u, deps := newStockTestUsecase(t)

			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductStockUpdate, authz.NewResource("product", nil)).
				Return(nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.repo.EXPECT().LockByID(gomock.Any(), entity.ID()).Return(entity, nil)
			deps.repo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(nextVersion, nil)

			_, err := u.UpdateProductStock(context.Background(), &auth.Authn{}, entity.ID(),
				UpdateProductStockParams{Delta: 1})
			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合、ErrUnauthenticated(401)を返し認可へ進まない", func(t *testing.T) {
			t.Parallel()

			u, deps := newStockTestUsecase(t)
			deps.authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			actual, err := u.UpdateProductStock(context.Background(), nil,
				uuidtestkit.NewTestFromSalt(t, "stock_unauthenticated"), UpdateProductStockParams{Delta: 1})
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
			assert.Equal(t, ProductView{}, actual)
		})

		t.Run("認可が拒否された場合、権限エラー(403)を返しトランザクションを開始しない", func(t *testing.T) {
			t.Parallel()

			u, deps := newStockTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductStockUpdate, gomock.Any()).
				Return(apperror.ErrPermissionDenied)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).Times(0)

			actual, err := u.UpdateProductStock(context.Background(), &auth.Authn{},
				uuidtestkit.NewTestFromSalt(t, "stock_forbidden"), UpdateProductStockParams{Delta: 1})
			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
			assert.Equal(t, ProductView{}, actual)
		})

		t.Run("対象商品が存在しない場合、ErrNotFound(404)を返し更新を実行しない", func(t *testing.T) {
			t.Parallel()

			id := uuidtestkit.NewTestFromSalt(t, "stock_not_found")
			u, deps := newStockTestUsecase(t)
			deps.authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductStockUpdate, gomock.Any()).
				Return(nil)
			deps.txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			deps.repo.EXPECT().LockByID(gomock.Any(), id).Return(nil, apperror.ErrNotFound)
			deps.repo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Times(0)

			actual, err := u.UpdateProductStock(context.Background(), &auth.Authn{}, id,
				UpdateProductStockParams{Delta: 1})
			require.ErrorIs(t, err, apperror.ErrNotFound)
			assert.Equal(t, ProductView{}, actual)
		})

		t.Run("減算後の在庫が負になる場合、ErrValidation(422)を返し更新を実行しない", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, currentVersion)
			u, deps := newStockTestUsecase(t)
			expectAuthorizedLock(deps, entity)
			deps.repo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Times(0)

			actual, err := u.UpdateProductStock(context.Background(), &auth.Authn{}, entity.ID(),
				UpdateProductStockParams{Delta: -(entity.Quantity() + 1)})
			require.ErrorIs(t, err, apperror.ErrValidation)
			assert.Equal(t, ProductView{}, actual)

			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{domainproduct.FieldDelta}, meta.Details())
		})

		t.Run("在庫の更新に失敗した場合、エラーを返す", func(t *testing.T) {
			t.Parallel()

			entity := newUpdateTarget(t, currentVersion)
			u, deps := newStockTestUsecase(t)
			expectAuthorizedLock(deps, entity)
			deps.repo.EXPECT().UpdateStock(gomock.Any(), gomock.Any()).Return(0, apperror.ErrUnavailable)

			actual, err := u.UpdateProductStock(context.Background(), &auth.Authn{}, entity.ID(),
				UpdateProductStockParams{Delta: 1})
			require.ErrorIs(t, err, apperror.ErrUnavailable)
			assert.Equal(t, ProductView{}, actual)
		})
	})
}
