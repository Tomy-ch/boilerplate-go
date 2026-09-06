package product

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domainproduct "go-boilerplate/internal/domain/product"
	"go-boilerplate/internal/domain/product/category"
	mock_category "go-boilerplate/internal/domain/product/category/mock"
	mock_product "go-boilerplate/internal/domain/product/mock"
	"go-boilerplate/internal/domain/product/status"
	mock_status "go-boilerplate/internal/domain/product/status/mock"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	mock_authz "go-boilerplate/internal/usecase/boundary/authz/mock"
	clocktestkit "go-boilerplate/internal/usecase/boundary/clock/testkit"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// mustCategory は、テスト用に有効な商品カテゴリエンティティ（電子機器）を構築します。
func mustCategory(t *testing.T, id uuid.UUID) *category.Category {
	t.Helper()
	c, err := category.New(id, category.Attributes{Name: "電子機器", Code: 1, SortKey: 1})
	require.NoError(t, err)
	return c
}

// mustStatus は、テスト用に有効な商品ステータスエンティティ（在庫あり）を構築します。
func mustStatus(t *testing.T, id uuid.UUID) *status.Status {
	t.Helper()
	s, err := status.New(id, status.Attributes{Name: "在庫あり", Code: 1, SortKey: 1})
	require.NoError(t, err)
	return s
}

// runInTx は、tx.Manager.Do がコールバックを同一 context で実行するモック挙動です。
func runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func Test_usecase_CreateProduct(t *testing.T) {
	t.Parallel()

	validParams := func(t *testing.T) (CreateProductParams, uuid.UUID, uuid.UUID) {
		t.Helper()
		statusID := uuidtestkit.NewTestFromSalt(t, "create_status")
		categoryID := uuidtestkit.NewTestFromSalt(t, "create_category")
		publishedAt := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
		return CreateProductParams{
			Name:                  "ワイヤレスイヤホン",
			Description:           ptr.To("<p>ノイズキャンセリング対応</p>"),
			Price:                 "19.99",
			Quantity:              100,
			StockWarningThreshold: ptr.To(10),
			CategoryID:            categoryID,
			StatusID:              statusID,
			PublishedAt:           ptr.To(publishedAt),
			Images: []ProductImageParams{
				{ImagePath: "products/earphone.png", DisplaySort: 1},
				{ImagePath: "products/earphone-sub.png", DisplaySort: 2},
			},
		}, statusID, categoryID
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("adminが商品を作成し、images・publishedAt・リッチテキスト説明を保持したViewを返す", func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)

			params, statusID, categoryID := validParams(t)

			txm := mock_tx.NewMockManager(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			statusRepo := mock_status.NewMockRepository(ctrl)
			statusRepo.EXPECT().FindByID(gomock.Any(), statusID).Return(mustStatus(t, statusID), nil)
			categoryRepo := mock_category.NewMockRepository(ctrl)
			categoryRepo.EXPECT().FindByID(gomock.Any(), categoryID).Return(mustCategory(t, categoryID), nil)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).Return(nil)
			repo := mock_product.NewMockRepository(ctrl)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

			u := &usecase{
				tracer: lt, txm: txm, repo: repo, categoryRepo: categoryRepo, statusRepo: statusRepo,
				authorizer: authorizer, clock: clocktestkit.NewMockClock(t, testCreatedAt),
			}
			actual, err := u.CreateProduct(ctx, &auth.Authn{}, params)
			require.NoError(t, err)

			assert.Equal(t, params.Name, actual.Name)
			assert.Equal(t, params.Description, actual.Description)
			assert.Equal(t, "19.99", actual.Price.String())
			assert.Equal(t, params.Quantity, actual.Quantity)
			assert.Equal(t, statusID, actual.StatusID)
			assert.Equal(t, "在庫あり", actual.StatusName)
			assert.Equal(t, categoryID, actual.CategoryID)
			assert.Equal(t, "電子機器", actual.CategoryName)
			require.NotNil(t, actual.PublishedAt)
			assert.Equal(t, *params.PublishedAt, *actual.PublishedAt)
			require.Len(t, actual.Images, 2)
			assert.Equal(t, "products/earphone.png", actual.Images[0].Path)
			assert.Equal(t, 1, actual.Images[0].DisplaySort)
			assert.Equal(t, "products/earphone-sub.png", actual.Images[1].Path)
			assert.Equal(t, 2, actual.Images[1].DisplaySort)
		})

		t.Run("publishedAtがnil・画像が空（未公開・画像未設定）でも作成できる", func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)

			params, statusID, categoryID := validParams(t)
			params.PublishedAt = nil
			params.Images = nil

			txm := mock_tx.NewMockManager(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			statusRepo := mock_status.NewMockRepository(ctrl)
			statusRepo.EXPECT().FindByID(gomock.Any(), statusID).Return(mustStatus(t, statusID), nil)
			categoryRepo := mock_category.NewMockRepository(ctrl)
			categoryRepo.EXPECT().FindByID(gomock.Any(), categoryID).Return(mustCategory(t, categoryID), nil)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).Return(nil)
			repo := mock_product.NewMockRepository(ctrl)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

			u := &usecase{
				tracer: lt, txm: txm, repo: repo, categoryRepo: categoryRepo, statusRepo: statusRepo,
				authorizer: authorizer, clock: clocktestkit.NewMockClock(t, testCreatedAt),
			}
			actual, err := u.CreateProduct(ctx, &auth.Authn{}, params)
			require.NoError(t, err)
			assert.Nil(t, actual.PublishedAt)
			assert.Empty(t, actual.Images)
		})

		t.Run("商品作成を所有者なしリソースとして認可する", func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)

			params, statusID, categoryID := validParams(t)

			txm := mock_tx.NewMockManager(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			statusRepo := mock_status.NewMockRepository(ctrl)
			statusRepo.EXPECT().FindByID(gomock.Any(), statusID).Return(mustStatus(t, statusID), nil)
			categoryRepo := mock_category.NewMockRepository(ctrl)
			categoryRepo.EXPECT().FindByID(gomock.Any(), categoryID).Return(mustCategory(t, categoryID), nil)
			repo := mock_product.NewMockRepository(ctrl)
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, authz.NewResource("product", nil)).
				Return(nil)

			u := &usecase{
				tracer: lt, txm: txm, repo: repo, categoryRepo: categoryRepo, statusRepo: statusRepo,
				authorizer: authorizer, clock: clocktestkit.NewMockClock(t, testCreatedAt),
			}
			_, err := u.CreateProduct(ctx, &auth.Authn{}, params)
			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合、ErrUnauthenticatedを返す", func(t *testing.T) {
			t.Parallel()

			lt := observability.NewMockUsecaseLayerTracer(t)
			params, _, _ := validParams(t)

			u := &usecase{tracer: lt}
			_, err := u.CreateProduct(context.Background(), nil, params)
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("認可が拒否された場合、権限エラーを返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			params, _, _ := validParams(t)

			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().
				Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).
				Return(apperror.ErrPermissionDenied)

			u := &usecase{tracer: lt, authorizer: authorizer}
			_, err := u.CreateProduct(context.Background(), &auth.Authn{}, params)
			require.ErrorIs(t, err, apperror.ErrPermissionDenied)
		})

		t.Run("価格が数値として解釈できない場合、ErrInvalidArgument(400)を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			params, _, _ := validParams(t)
			params.Price = "not-a-number"

			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).Return(nil)

			u := &usecase{tracer: lt, authorizer: authorizer}
			_, err := u.CreateProduct(context.Background(), &auth.Authn{}, params)
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})

		t.Run("価格が負の場合、検証エラー(422)を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			params, _, _ := validParams(t)
			params.Price = "-1"

			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).Return(nil)

			u := &usecase{tracer: lt, authorizer: authorizer}
			_, err := u.CreateProduct(context.Background(), &auth.Authn{}, params)
			require.ErrorIs(t, err, apperror.ErrValidation)
			meta, ok := apperror.MetaFrom(err)
			require.True(t, ok)
			assert.Equal(t, []string{domainproduct.FieldPrice}, meta.Details())
		})

		t.Run("在庫数が負の場合、検証エラー(422)を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			params, statusID, categoryID := validParams(t)
			params.Quantity = -1

			txm := mock_tx.NewMockManager(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			statusRepo := mock_status.NewMockRepository(ctrl)
			statusRepo.EXPECT().FindByID(gomock.Any(), statusID).Return(mustStatus(t, statusID), nil)
			categoryRepo := mock_category.NewMockRepository(ctrl)
			categoryRepo.EXPECT().FindByID(gomock.Any(), categoryID).Return(mustCategory(t, categoryID), nil)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).Return(nil)

			u := &usecase{
				tracer: lt, txm: txm, categoryRepo: categoryRepo, statusRepo: statusRepo,
				authorizer: authorizer, clock: clocktestkit.NewMockClock(t, testCreatedAt),
			}
			_, err := u.CreateProduct(context.Background(), &auth.Authn{}, params)
			require.ErrorIs(t, err, apperror.ErrValidation)
		})

		t.Run("statusが存在しない場合、整合性異常(500)を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			params, statusID, _ := validParams(t)

			txm := mock_tx.NewMockManager(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			statusRepo := mock_status.NewMockRepository(ctrl)
			statusRepo.EXPECT().FindByID(gomock.Any(), statusID).Return(nil, apperror.ErrNotFound)
			categoryRepo := mock_category.NewMockRepository(ctrl)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).Return(nil)

			u := &usecase{
				tracer: lt, txm: txm, categoryRepo: categoryRepo, statusRepo: statusRepo,
				authorizer: authorizer, clock: clocktestkit.NewMockClock(t, testCreatedAt),
			}
			_, err := u.CreateProduct(context.Background(), &auth.Authn{}, params)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("categoryが存在しない場合、整合性異常(500)を返す", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			params, statusID, categoryID := validParams(t)

			txm := mock_tx.NewMockManager(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runInTx)
			statusRepo := mock_status.NewMockRepository(ctrl)
			statusRepo.EXPECT().FindByID(gomock.Any(), statusID).Return(mustStatus(t, statusID), nil)
			categoryRepo := mock_category.NewMockRepository(ctrl)
			categoryRepo.EXPECT().FindByID(gomock.Any(), categoryID).Return(nil, apperror.ErrNotFound)
			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any(), authz.ActionProductCreate, gomock.Any()).Return(nil)

			u := &usecase{
				tracer: lt, txm: txm, categoryRepo: categoryRepo, statusRepo: statusRepo,
				authorizer: authorizer, clock: clocktestkit.NewMockClock(t, testCreatedAt),
			}
			_, err := u.CreateProduct(context.Background(), &auth.Authn{}, params)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})
	})
}

func Test_usecase_resolveRefs(t *testing.T) {
	t.Parallel()

	statusID := func(t *testing.T) uuid.UUID { t.Helper(); return uuidtestkit.NewTestFromSalt(t, "resolve_refs_status") }
	categoryID := func(t *testing.T) uuid.UUID {
		t.Helper()
		return uuidtestkit.NewTestFromSalt(t, "resolve_refs_category")
	}

	newRepos := func(t *testing.T) (*mock_status.MockRepository, *mock_category.MockRepository) {
		t.Helper()
		ctrl := gomock.NewController(t)
		return mock_status.NewMockRepository(ctrl), mock_category.NewMockRepository(ctrl)
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("status/categoryのIDから名称を解決した参照を返す", func(t *testing.T) {
			t.Parallel()

			sid, cid := statusID(t), categoryID(t)
			statusRepo, categoryRepo := newRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), sid).Return(mustStatus(t, sid), nil)
			categoryRepo.EXPECT().FindByID(gomock.Any(), cid).Return(mustCategory(t, cid), nil)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			statusRef, categoryRef, err := u.resolveRefs(context.Background(), sid, cid)
			require.NoError(t, err)
			assert.Equal(t, sid, statusRef.ID())
			assert.Equal(t, "在庫あり", statusRef.Name())
			assert.Equal(t, cid, categoryRef.ID())
			assert.Equal(t, "電子機器", categoryRef.Name())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("status不在はErrInternalへ正規化される", func(t *testing.T) {
			t.Parallel()

			sid, cid := statusID(t), categoryID(t)
			statusRepo, categoryRepo := newRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), sid).Return(nil, apperror.ErrNotFound)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			_, _, err := u.resolveRefs(context.Background(), sid, cid)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("category不在はErrInternalへ正規化される", func(t *testing.T) {
			t.Parallel()

			sid, cid := statusID(t), categoryID(t)
			statusRepo, categoryRepo := newRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), sid).Return(mustStatus(t, sid), nil)
			categoryRepo.EXPECT().FindByID(gomock.Any(), cid).Return(nil, apperror.ErrNotFound)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			_, _, err := u.resolveRefs(context.Background(), sid, cid)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("status名称が参照制約違反(NewStatusRef失敗)の場合、errMissingStatus(ErrInternal)へ正規化しErrValidationを露出しない", func(t *testing.T) {
			t.Parallel()

			sid, cid := statusID(t), categoryID(t)
			statusRepo, categoryRepo := newRepos(t)
			// ゼロ値 status（ID が nil）を返すと NewStatusRef が ErrInvalidStatusID で失敗する。
			statusRepo.EXPECT().FindByID(gomock.Any(), sid).Return(&status.Status{}, nil)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			_, _, err := u.resolveRefs(context.Background(), sid, cid)

			require.ErrorIs(t, err, errMissingStatus)
			require.ErrorIs(t, err, apperror.ErrInternal)
			require.NotErrorIs(t, err, apperror.ErrValidation) // サーバ側データ不整合を 4xx として露出しない
		})

		t.Run("category名称が参照制約違反(NewCategoryRef失敗)の場合、errMissingCategory(ErrInternal)へ正規化する", func(t *testing.T) {
			t.Parallel()

			sid, cid := statusID(t), categoryID(t)
			statusRepo, categoryRepo := newRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), sid).Return(mustStatus(t, sid), nil)
			categoryRepo.EXPECT().FindByID(gomock.Any(), cid).Return(&category.Category{}, nil)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			_, _, err := u.resolveRefs(context.Background(), sid, cid)

			require.ErrorIs(t, err, errMissingCategory)
			require.NotErrorIs(t, err, apperror.ErrValidation)
		})

		t.Run("NotFound以外のエラーはそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			sid, cid := statusID(t), categoryID(t)
			statusRepo, categoryRepo := newRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), sid).Return(nil, apperror.ErrCanceled)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			_, _, err := u.resolveRefs(context.Background(), sid, cid)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})

		t.Run("category取得のNotFound以外のエラーはそのまま伝播する", func(t *testing.T) {
			t.Parallel()

			sid, cid := statusID(t), categoryID(t)
			statusRepo, categoryRepo := newRepos(t)
			statusRepo.EXPECT().FindByID(gomock.Any(), sid).Return(mustStatus(t, sid), nil)
			categoryRepo.EXPECT().FindByID(gomock.Any(), cid).Return(nil, apperror.ErrCanceled)

			u := &usecase{statusRepo: statusRepo, categoryRepo: categoryRepo}
			_, _, err := u.resolveRefs(context.Background(), sid, cid)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}

func Test_buildImages(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("入力の順序と値を保ったままIDを採番する", func(t *testing.T) {
			t.Parallel()

			actual, err := buildImages([]ProductImageParams{
				{ImagePath: "products/a.png", DisplaySort: 1},
				{ImagePath: "products/b.png", DisplaySort: 2},
			})

			require.NoError(t, err)
			require.Len(t, actual, 2)
			assert.Equal(t, "products/a.png", actual[0].ImagePath())
			assert.Equal(t, 1, actual[0].DisplaySort())
			assert.Equal(t, "products/b.png", actual[1].ImagePath())
			assert.Equal(t, 2, actual[1].DisplaySort())
		})

		t.Run("採番したIDは画像ごとに異なる", func(t *testing.T) {
			t.Parallel()

			actual, err := buildImages([]ProductImageParams{
				{ImagePath: "products/a.png", DisplaySort: 1},
				{ImagePath: "products/b.png", DisplaySort: 2},
			})

			require.NoError(t, err)
			require.Len(t, actual, 2)
			assert.NotEqual(t, actual[0].ID(), actual[1].ID())
		})

		t.Run("入力が空の場合、空を返す", func(t *testing.T) {
			t.Parallel()

			actual, err := buildImages(nil)

			require.NoError(t, err)
			assert.Empty(t, actual)
		})
	})
}
