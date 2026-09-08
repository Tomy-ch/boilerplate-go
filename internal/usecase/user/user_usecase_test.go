package user

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/apperror"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	mock_coupon "go-boilerplate/internal/domain/coupon/mock"
	"go-boilerplate/internal/domain/prefecture"
	mock_prefecture "go-boilerplate/internal/domain/prefecture/mock"
	mock_purchase "go-boilerplate/internal/domain/purchase/mock"
	"go-boilerplate/internal/domain/user"
	mock_user "go-boilerplate/internal/domain/user/mock"
	"go-boilerplate/internal/observability"
	authbd "go-boilerplate/internal/usecase/boundary/auth"
	mock_auth "go-boilerplate/internal/usecase/boundary/auth/mock"
	"go-boilerplate/internal/usecase/boundary/authz"
	mock_authz "go-boilerplate/internal/usecase/boundary/authz/mock"
	clocktest "go-boilerplate/internal/usecase/boundary/clock/testkit"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	mock_outbox "go-boilerplate/internal/usecase/outbox/mock"
	"go-boilerplate/internal/usecase/testkit"
	"go-boilerplate/internal/usecase/tools/paging"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		tf := observability.NewNoopTracerFactory(t)
		mockTxManager := mock_tx.NewMockManager(ctrl)
		clock := clocktest.NewMockClock(t, time.Time{})
		authorizer := mock_authz.NewMockAuthorizer(ctrl)
		userRepo := mock_user.NewMockRepository(ctrl)
		userLock := mock_user.NewMockLockRepository(ctrl)
		pftRepo := mock_prefecture.NewMockRepository(ctrl)
		purchaseRepo := mock_purchase.NewMockRepository(ctrl)
		couponRepo := mock_coupon.NewMockRepository(ctrl)
		identities := mock_auth.NewMockIdentityRegistrar(ctrl)
		emit := mock_outbox.NewMockEmitUsecase(ctrl)

		expected := &usecase{
			tracer:       tf.Usecase(),
			txm:          mockTxManager,
			clock:        clock,
			authorizer:   authorizer,
			userRepo:     userRepo,
			userLock:     userLock,
			pftRepo:      pftRepo,
			purchaseRepo: purchaseRepo,
			couponRepo:   couponRepo,
			identities:   identities,
			emit:         emit,
		}
		actual := New(tf, mockTxManager, clock, authorizer, userRepo, userLock, pftRepo, purchaseRepo, couponRepo, identities, emit)

		assert.Equal(t, expected, actual)
	})
}

func Test_usecase_ListUsers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lt := observability.NewMockUsecaseLayerTracer(t)
	now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	prefectureID := uuidtestkit.NewTestFromSalt(t, "prefecture_domain")

	userDomain, err := user.New(uuidtestkit.NewTestFromSalt(t, "user_domain"), user.Attributes{
		FirstName:    "first_name",
		LastName:     "last_name",
		Email:        "user@example.com",
		Phone:        "phone_number",
		PrefectureID: prefectureID,
		City:         "city_name",
		Street:       "town_address",
		PostalCode:   "150-0001",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		page := 1
		perPage := 100
		p, err := paging.NewPageFrom1Based(&page, &perPage)
		require.NoError(t, err)

		prefectureDomain, err := prefecture.New(
			prefectureID,
			"prefecture_name",
			1,
		)
		require.NoError(t, err)

		expected := []UserView{
			{
				ID:             userDomain.ID(),
				FirstName:      userDomain.FirstName(),
				LastName:       userDomain.LastName(),
				PostalCode:     userDomain.PostalCode(),
				PrefectureName: prefectureDomain.Name(),
				City:           userDomain.City(),
				Street:         userDomain.Street(),
				Building:       userDomain.Building(),
				Email:          userDomain.Email(),
				Phone:          userDomain.Phone(),
			},
		}

		t.Run("activeがnilの場合、全件取得が実行されユーザー情報がリストで取得できる", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{userDomain}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)
			uc := &usecase{
				authorizer: newAllowAuthorizer(ctrl),
				tracer:     lt,
				userRepo:   userRepo,
				pftRepo:    pftRepo,
			}

			actual, err := uc.ListUsers(ctx, newTestAuthn(t), nil, p)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})

		t.Run("該当ユーザーが0件の場合、エラーではなく空リストが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{}).Return(prefecture.Prefectures{}, nil)
			uc := &usecase{
				authorizer: newAllowAuthorizer(ctrl),
				tracer:     lt,
				userRepo:   userRepo,
				pftRepo:    pftRepo,
			}

			actual, err := uc.ListUsers(ctx, newTestAuthn(t), nil, p)
			require.NoError(t, err)
			assert.Empty(t, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ユーザー取得時にエラーが発生した場合、エラーが返される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()

			page := 1
			perPage := 100
			p, actualErr := paging.NewPageFrom1Based(&page, &perPage)
			require.NoError(t, actualErr)

			repo := mock_user.NewMockRepository(ctrl)
			repo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(nil, expectedErr)
			uc := &usecase{
				authorizer: newAllowAuthorizer(ctrl),
				tracer:     lt,
				userRepo:   repo,
			}

			actual, actualErr := uc.ListUsers(ctx, newTestAuthn(t), nil, p)
			require.Nil(t, actual)
			require.ErrorIs(t, actualErr, expectedErr)
		})

		t.Run("都道府県取得時にエラーが発生した場合、エラーが返される", func(t *testing.T) {
			t.Parallel()

			expectedErr := testkit.ExpectedDBError()

			page := 1
			perPage := 100
			p, actualErr := paging.NewPageFrom1Based(&page, &perPage)
			require.NoError(t, actualErr)

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{userDomain}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(nil, expectedErr)
			uc := &usecase{
				authorizer: newAllowAuthorizer(ctrl),
				tracer:     lt,
				userRepo:   userRepo,
				pftRepo:    pftRepo,
			}

			actual, err := uc.ListUsers(ctx, newTestAuthn(t), nil, p)
			require.Nil(t, actual)
			require.ErrorIs(t, err, expectedErr)
		})

		t.Run("ユーザーの都道府県が解決できない場合、ErrInternal が返される", func(t *testing.T) {
			t.Parallel()

			page := 1
			perPage := 100
			p, err := paging.NewPageFrom1Based(&page, &perPage)
			require.NoError(t, err)

			ctrl := gomock.NewController(t)
			lt := observability.NewMockUsecaseLayerTracer(t)
			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{userDomain}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{}, nil)
			uc := &usecase{
				authorizer: newAllowAuthorizer(ctrl),
				tracer:     lt,
				userRepo:   userRepo,
				pftRepo:    pftRepo,
			}

			actual, err := uc.ListUsers(ctx, newTestAuthn(t), nil, p)
			require.Nil(t, actual)
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("page が nil の場合、ErrInvalidArgument が返される", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := &usecase{tracer: lt, authorizer: newAllowAuthorizer(ctrl)}
			actual, err := uc.ListUsers(ctx, newTestAuthn(t), nil, nil)
			require.Nil(t, actual)
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})

		t.Run("認可が拒否される場合、ErrForbidden が返りリポジトリへ到達しない", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			page := 1
			perPage := 100
			p, err := paging.NewPageFrom1Based(&page, &perPage)
			require.NoError(t, err)

			// userRepo は EXPECT を持たないため、認可より先にリポジトリを呼ぶ実装に戻ると失敗する。
			uc := &usecase{
				tracer:     lt,
				authorizer: newDenyAuthorizer(ctrl),
				userRepo:   mock_user.NewMockRepository(ctrl),
				pftRepo:    mock_prefecture.NewMockRepository(ctrl),
			}

			actual, err := uc.ListUsers(ctx, newTestAuthn(t), nil, p)
			require.Nil(t, actual)
			require.ErrorIs(t, err, authz.ErrForbidden)
		})
	})
}

func Test_usecase_CreateUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lt := observability.NewMockUsecaseLayerTracer(t)
	mockTxManager := testkit.NewMockTransactionManager(t)
	now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	prefectureID := uuidtestkit.NewTestFromSalt(t, "prefecture_domain")

	userDomain, err := user.New(uuidtestkit.NewTestFromSalt(t, "user_domain"), user.Attributes{
		FirstName:    "first_name",
		LastName:     "last_name",
		Email:        "user@example.com",
		Phone:        "phone_number",
		PrefectureID: prefectureID,
		City:         "city_name",
		Street:       "town_address",
		PostalCode:   "150-0001",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)

	prefectureName := "prefecture_name"

	pftDomain, err := prefecture.New(
		prefectureID,
		prefectureName,
		1,
	)
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ユーザー作成が成功した場合、作成したユーザー情報が返される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			createDTO := newCreateDTO(userDomain, prefectureName)
			// ID はユースケースが採番するため、期待値は Create が受け取った実物から取る。
			var issuedUserID uuid.UUID
			expected := UserView{
				FirstName:      createDTO.FirstName,
				LastName:       createDTO.LastName,
				Email:          createDTO.Email,
				Phone:          createDTO.Phone,
				PostalCode:     createDTO.PostalCode,
				PrefectureName: createDTO.PrefectureName,
				City:           createDTO.City,
				Street:         createDTO.Street,
				Building:       createDTO.Building,
			}

			clock := clocktest.NewMockClockOnce(t, now)
			userRepo := mock_user.NewMockRepository(ctrl)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			couponRepo := mock_coupon.NewMockRepository(ctrl)
			identities := mock_auth.NewMockIdentityRegistrar(ctrl)

			gomock.InOrder(
				pftRepo.EXPECT().FindByName(
					gomock.Any(),
					prefectureName,
				).Return(pftDomain, nil),
				userRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, u *user.User) error {
						assert.False(t, u.ID().IsNil())
						issuedUserID = u.ID()
						assert.Equal(t, createDTO.FirstName, u.FirstName())
						assert.Equal(t, createDTO.LastName, u.LastName())
						assert.Equal(t, createDTO.Email, u.Email())
						assert.Equal(t, createDTO.Phone, u.Phone())
						assert.Equal(t, createDTO.PostalCode, u.PostalCode())
						assert.Equal(t, createDTO.City, u.City())
						assert.Equal(t, createDTO.Street, u.Street())
						assert.Equal(t, createDTO.Building, u.Building())
						assert.Equal(t, pftDomain.ID(), u.PrefectureID())
						assert.Equal(t, now, u.CreatedAt())
						assert.Equal(t, now, u.UpdatedAt())
						assert.Nil(t, u.DeletedAt())
						return nil
					},
				),
				identities.EXPECT().Register(gomock.Any(), gomock.Any(), createDTO.Issuer, createDTO.Subject).DoAndReturn(
					func(_ context.Context, id uuid.UUID, _, _ string) error {
						// 結び付ける先は、いま作ったユーザーそのものでなければならない。
						assert.Equal(t, issuedUserID, id)
						return nil
					},
				),
				couponRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, c *domaincoupon.Coupon) error {
						// このユースケースが担うのは受給者と発行日時を渡すところまで。
						// 値引き・適用範囲・有効期限の中身は newWelcomeCoupon の契約で、
						// welcome_coupon_test.go が別に固定している。
						assert.Equal(t, issuedUserID, c.UserID())
						assert.Equal(t, now, c.IssuedAt())
						return nil
					},
				),
			)

			uc := &usecase{
				tracer:     lt,
				txm:        mockTxManager,
				clock:      clock,
				userRepo:   userRepo,
				pftRepo:    pftRepo,
				couponRepo: couponRepo,
				identities: identities,
			}

			actual, err := uc.CreateUser(ctx, createDTO)
			require.NoError(t, err)

			expected.ID = issuedUserID
			assert.Equal(t, expected, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("都道府県の取得に失敗した場合、エラーが返される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()

			createDTO := newCreateDTO(userDomain, prefectureName)

			clock := clocktest.NewMockClockOnce(t, now)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByName(
				gomock.Any(),
				prefectureName,
			).Return(nil, expectedErr)

			uc := &usecase{
				tracer:  lt,
				txm:     mockTxManager,
				clock:   clock,
				pftRepo: pftRepo,
			}

			actual, err := uc.CreateUser(ctx, createDTO)
			assert.Equal(t, UserView{}, actual)
			require.ErrorIs(t, err, expectedErr)
		})

		t.Run("FirstNameが空でドメイン生成に失敗した場合、エラーが返される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			createDTO := newCreateDTO(userDomain, prefectureName)
			createDTO.FirstName = "" // FirstNameを空にしてエラーを発生させる

			clock := clocktest.NewMockClockOnce(t, now)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByName(
				gomock.Any(),
				prefectureName,
			).Return(pftDomain, nil)

			uc := &usecase{
				tracer:  lt,
				txm:     mockTxManager,
				clock:   clock,
				pftRepo: pftRepo,
			}

			actual, err := uc.CreateUser(ctx, createDTO)
			assert.Equal(t, UserView{}, actual)
			require.ErrorIs(t, err, user.ErrInvalidFirstName)
		})

		t.Run("既に登録済みの主体からの2回目は、Conflictが返りクーポンも発行されない", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			createDTO := newCreateDTO(userDomain, prefectureName)

			clock := clocktest.NewMockClockOnce(t, now)
			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().Create(
				gomock.Any(),
				gomock.AssignableToTypeOf(userDomain),
			).Return(nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByName(
				gomock.Any(),
				prefectureName,
			).Return(pftDomain, nil)
			// クーポンの発行までは到達しない（couponRepo に EXPECT を置かないことで固定する）。
			identities := mock_auth.NewMockIdentityRegistrar(ctrl)
			identities.EXPECT().Register(
				gomock.Any(), gomock.Any(), createDTO.Issuer, createDTO.Subject,
			).Return(apperror.ErrConflict)
			couponRepo := mock_coupon.NewMockRepository(ctrl)

			uc := &usecase{
				tracer:     lt,
				txm:        mockTxManager,
				clock:      clock,
				userRepo:   userRepo,
				pftRepo:    pftRepo,
				couponRepo: couponRepo,
				identities: identities,
			}

			actual, err := uc.CreateUser(ctx, createDTO)
			assert.Equal(t, UserView{}, actual)
			require.ErrorIs(t, err, apperror.ErrConflict)
		})

		t.Run("ユーザー作成に失敗した場合、エラーが返される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()

			createDTO := newCreateDTO(userDomain, prefectureName)

			clock := clocktest.NewMockClockOnce(t, now)
			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().Create(
				gomock.Any(),
				gomock.AssignableToTypeOf(userDomain),
			).Return(expectedErr)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByName(
				gomock.Any(),
				prefectureName,
			).Return(pftDomain, nil)

			uc := &usecase{
				tracer:   lt,
				txm:      mockTxManager,
				clock:    clock,
				userRepo: userRepo,
				pftRepo:  pftRepo,
			}

			actual, err := uc.CreateUser(ctx, createDTO)
			assert.Equal(t, UserView{}, actual)
			require.ErrorIs(t, err, expectedErr)
		})

		t.Run("ウェルカムクーポンの発行に失敗した場合、エラーが返される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()

			createDTO := newCreateDTO(userDomain, prefectureName)

			clock := clocktest.NewMockClockOnce(t, now)
			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().Create(
				gomock.Any(),
				gomock.AssignableToTypeOf(userDomain),
			).Return(nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByName(
				gomock.Any(),
				prefectureName,
			).Return(pftDomain, nil)
			identities := mock_auth.NewMockIdentityRegistrar(ctrl)
			identities.EXPECT().Register(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			couponRepo := mock_coupon.NewMockRepository(ctrl)
			couponRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(expectedErr)

			uc := &usecase{
				tracer:     lt,
				txm:        mockTxManager,
				clock:      clock,
				userRepo:   userRepo,
				pftRepo:    pftRepo,
				couponRepo: couponRepo,
				identities: identities,
			}

			actual, err := uc.CreateUser(ctx, createDTO)
			assert.Equal(t, UserView{}, actual)
			require.ErrorIs(t, err, expectedErr)
		})

		t.Run("トランザクションの実行に失敗した場合、エラーが伝播される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()

			createDTO := newCreateDTO(userDomain, prefectureName)

			clock := clocktest.NewMockClockOnce(t, now)
			// tx.Manager 自体が失敗する経路（接続断等）。fn は実行されないため repo 呼び出しはない。
			txm := mock_tx.NewMockManager(ctrl)
			txm.EXPECT().Do(gomock.Any(), gomock.Any()).Return(expectedErr)

			uc := &usecase{
				tracer: lt,
				txm:    txm,
				clock:  clock,
			}

			actual, err := uc.CreateUser(ctx, createDTO)
			assert.Equal(t, UserView{}, actual)
			require.ErrorIs(t, err, expectedErr)
		})
	})
}

func Test_usecase_ListUsersWithTotal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lt := observability.NewMockUsecaseLayerTracer(t)
	now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	prefectureID := uuidtestkit.NewTestFromSalt(t, "prefecture_domain")
	userDomain, err := user.New(uuidtestkit.NewTestFromSalt(t, "user_domain"), user.Attributes{
		FirstName:    "first_name",
		LastName:     "last_name",
		Email:        "user@example.com",
		Phone:        "phone_number",
		PrefectureID: prefectureID,
		City:         "city_name",
		Street:       "town_address",
		PostalCode:   "150-0001",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	prefectureDomain, err := prefecture.New(prefectureID, "prefecture_name", 1)
	require.NoError(t, err)

	page := 1
	perPage := 100
	p, err := paging.NewPageFrom1Based(&page, &perPage)
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()
		t.Run("一覧と総件数をまとめて取得できる", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{userDomain}, nil)
			userRepo.EXPECT().CountByActive(gomock.Any(), nil).Return(int64(1), nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)

			uc := &usecase{tracer: lt, userRepo: userRepo, pftRepo: pftRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, err := uc.ListUsersWithTotal(ctx, newTestAuthn(t), nil, p)
			require.NoError(t, err)
			assert.Len(t, actual.Items, 1)
			assert.Equal(t, int64(1), actual.Total)
		})

		t.Run("該当ユーザーが0件の場合、空リストと総件数0が返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{}, nil)
			userRepo.EXPECT().CountByActive(gomock.Any(), nil).Return(int64(0), nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{}).Return(prefecture.Prefectures{}, nil)

			uc := &usecase{tracer: lt, userRepo: userRepo, pftRepo: pftRepo, authorizer: newAllowAuthorizer(ctrl)}

			// 該当なしはエラーではなく空リスト。NotFound にすると呼出元の分岐が変わる。
			actual, err := uc.ListUsersWithTotal(ctx, newTestAuthn(t), nil, p)
			require.NoError(t, err)
			assert.Empty(t, actual.Items)
			assert.Equal(t, int64(0), actual.Total)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("一覧取得でエラーが発生した場合、エラーが返る", func(t *testing.T) {
			t.Parallel()
			expectedErr := testkit.ExpectedDBError()
			ctrl := gomock.NewController(t)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(nil, expectedErr)
			uc := &usecase{tracer: lt, userRepo: userRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, err := uc.ListUsersWithTotal(ctx, newTestAuthn(t), nil, p)
			require.ErrorIs(t, err, expectedErr)
			require.Nil(t, actual)
		})

		t.Run("総件数取得でエラーが発生した場合、エラーが返る", func(t *testing.T) {
			t.Parallel()
			expectedErr := testkit.ExpectedDBError()
			ctrl := gomock.NewController(t)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{userDomain}, nil)
			userRepo.EXPECT().CountByActive(gomock.Any(), nil).Return(int64(0), expectedErr)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)
			uc := &usecase{tracer: lt, userRepo: userRepo, pftRepo: pftRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, err := uc.ListUsersWithTotal(ctx, newTestAuthn(t), nil, p)
			require.ErrorIs(t, err, expectedErr)
			require.Nil(t, actual)
		})

		t.Run("page が nil の場合、ErrInvalidArgument が返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			uc := &usecase{tracer: observability.NewNoopTracerFactory(t).Usecase(), authorizer: newAllowAuthorizer(ctrl)}
			actual, err := uc.ListUsersWithTotal(ctx, newTestAuthn(t), nil, nil)
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
			require.Nil(t, actual)
		})

		t.Run("認可が拒否される場合、ErrForbidden が返りリポジトリへ到達しない", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			page := 1
			perPage := 100
			p, err := paging.NewPageFrom1Based(&page, &perPage)
			require.NoError(t, err)

			// userRepo は EXPECT を持たないため、認可より先にリポジトリを呼ぶ実装に戻ると失敗する。
			uc := &usecase{
				tracer:     lt,
				authorizer: newDenyAuthorizer(ctrl),
				userRepo:   mock_user.NewMockRepository(ctrl),
				pftRepo:    mock_prefecture.NewMockRepository(ctrl),
			}

			actual, err := uc.ListUsersWithTotal(ctx, newTestAuthn(t), nil, p)
			require.Nil(t, actual)
			require.ErrorIs(t, err, authz.ErrForbidden)
		})
	})
}

func Test_usecase_CountUsers(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	tf := observability.NewNoopTracerFactory(t)
	userRepo := mock_user.NewMockRepository(ctrl)

	u := &usecase{
		tracer:   tf.Usecase(),
		userRepo: userRepo,
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ユーザーの総件数が正常に取得できること", func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			active := new(true)
			expectedCount := int64(42)

			userRepo.
				EXPECT().
				CountByActive(gomock.Any(), active).
				Return(expectedCount, nil)

			actualCount, err := u.CountUsers(ctx, active)
			require.NoError(t, err)
			assert.Equal(t, expectedCount, actualCount)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("総件数取得でエラーが発生した場合、エラーが伝播される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			ctx := t.Context()
			active := new(true)
			expectedErr := testkit.ExpectedDBError()

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.
				EXPECT().
				CountByActive(gomock.Any(), active).
				Return(int64(0), expectedErr)

			uc := &usecase{
				tracer:   observability.NewNoopTracerFactory(t).Usecase(),
				userRepo: userRepo,
			}

			actualCount, err := uc.CountUsers(ctx, active)
			require.ErrorIs(t, err, expectedErr)
			assert.Zero(t, actualCount)
		})
	})
}

func Test_usecase_ListUsersFeed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lt := observability.NewMockUsecaseLayerTracer(t)
	now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	prefectureID := uuidtestkit.NewTestFromSalt(t, "prefecture_domain")
	prefectureDomain, err := prefecture.New(prefectureID, "prefecture_name", 1)
	require.NoError(t, err)

	// newFeedUser は、作成日時違いのフィードユーザーを生成するヘルパーです。
	newFeedUser := func(salt string, createdAt time.Time) *user.User {
		u, uErr := user.New(uuidtestkit.NewTestFromSalt(t, salt), user.Attributes{
			FirstName:    "first_name",
			LastName:     "last_name",
			Email:        "user@example.com",
			Phone:        "phone_number",
			PrefectureID: prefectureID,
			City:         "city_name",
			Street:       "town_address",
			PostalCode:   "150-0001",
			CreatedAt:    createdAt,
			UpdatedAt:    createdAt,
		})
		require.NoError(t, uErr)
		return u
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("先頭ページの場合、afterがnilでリポジトリが呼ばれ次ページが無ければNextCursorはnilになる", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			first := 20
			cursor, cErr := paging.NewCursor(nil, &first)
			require.NoError(t, cErr)

			u1 := newFeedUser("feed_user_1", now)

			userRepo := mock_user.NewMockRepository(ctrl)
			// limit+1（=21）件を要求し、先頭ページは after=nil で呼ばれる。
			userRepo.EXPECT().FindFeed(gomock.Any(), (*user.FeedCursor)(nil), int32(21)).Return(user.Users{u1}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)

			uc := &usecase{tracer: lt, userRepo: userRepo, pftRepo: pftRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.NoError(t, actualErr)
			assert.Len(t, actual.Items, 1)
			assert.Nil(t, actual.NextCursor)
		})

		t.Run("カーソル指定の場合、afterが解釈されてリポジトリが呼ばれる", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			afterUser := newFeedUser("feed_user_after", now)
			encoded := paging.EncodeCursor(afterUser.CreatedAt().Format(time.RFC3339Nano), afterUser.ID().String())
			first := 20
			cursor, cErr := paging.NewCursor(&encoded, &first)
			require.NoError(t, cErr)

			u1 := newFeedUser("feed_user_1", now)

			// カーソルの復号正当性は feed_cursor_test.go が担保するため、ここでは
			// 「after 指定時に limit+1 件でリポジトリが呼ばれる」オーケストレーションのみ検証する。
			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindFeed(
				gomock.Any(),
				gomock.Any(),
				int32(21),
			).Return(user.Users{u1}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)

			uc := &usecase{tracer: lt, userRepo: userRepo, pftRepo: pftRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.NoError(t, actualErr)
			assert.Len(t, actual.Items, 1)
			assert.Nil(t, actual.NextCursor)
		})

		t.Run("limit+1件取得できた場合、limit件に切り詰められ末尾行からNextCursorが生成される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			first := 1
			cursor, cErr := paging.NewCursor(nil, &first)
			require.NoError(t, cErr)

			// limit=1 に対し 2 件返す。先頭が表示分、2件目は次ページ存在判定用。
			head := newFeedUser("feed_user_head", now)
			tail := newFeedUser("feed_user_tail", now.Add(-time.Hour))

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindFeed(gomock.Any(), (*user.FeedCursor)(nil), int32(2)).Return(user.Users{head, tail}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			// 切り詰め後の 1 件分のみ都道府県解決される。
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)

			uc := &usecase{tracer: lt, userRepo: userRepo, pftRepo: pftRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.NoError(t, actualErr)
			assert.Len(t, actual.Items, 1)
			require.NotNil(t, actual.NextCursor)
			// NextCursor は切り詰め後の末尾（head）のソートキーから生成される。
			expectedCursor := paging.EncodeCursor(head.CreatedAt().Format(time.RFC3339Nano), head.ID().String())
			assert.Equal(t, expectedCursor, *actual.NextCursor)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("cursorがnilの場合、ErrInvalidArgumentが返る", func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			uc := &usecase{tracer: lt, authorizer: newAllowAuthorizer(ctrl)}
			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), nil)
			require.ErrorIs(t, actualErr, apperror.ErrInvalidArgument)
			require.Nil(t, actual)
		})

		t.Run("カーソルキーが2個でない場合、ErrInvalidArgumentが返る", func(t *testing.T) {
			t.Parallel()

			// キー1個の不正カーソル。
			encoded := paging.EncodeCursor("only_one_key")
			first := 20
			cursor, cErr := paging.NewCursor(&encoded, &first)
			require.NoError(t, cErr)

			ctrl := gomock.NewController(t)
			uc := &usecase{tracer: lt, authorizer: newAllowAuthorizer(ctrl)}
			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.ErrorIs(t, actualErr, apperror.ErrInvalidArgument)
			require.Nil(t, actual)
		})

		t.Run("カーソルのcreated_atがRFC3339Nanoでない場合、ErrInvalidArgumentが返る", func(t *testing.T) {
			t.Parallel()

			encoded := paging.EncodeCursor("not-a-time", uuidtestkit.NewTestFromSalt(t, "any").String())
			first := 20
			cursor, cErr := paging.NewCursor(&encoded, &first)
			require.NoError(t, cErr)

			ctrl := gomock.NewController(t)
			uc := &usecase{tracer: lt, authorizer: newAllowAuthorizer(ctrl)}
			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.ErrorIs(t, actualErr, apperror.ErrInvalidArgument)
			require.Nil(t, actual)
		})

		t.Run("カーソルのidがUUIDでない場合、ErrInvalidArgumentが返る", func(t *testing.T) {
			t.Parallel()

			encoded := paging.EncodeCursor(now.Format(time.RFC3339Nano), "not-a-uuid")
			first := 20
			cursor, cErr := paging.NewCursor(&encoded, &first)
			require.NoError(t, cErr)

			ctrl := gomock.NewController(t)
			uc := &usecase{tracer: lt, authorizer: newAllowAuthorizer(ctrl)}
			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.ErrorIs(t, actualErr, apperror.ErrInvalidArgument)
			require.Nil(t, actual)
		})

		t.Run("リポジトリ取得でエラーが発生した場合、エラーが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()
			first := 20
			cursor, cErr := paging.NewCursor(nil, &first)
			require.NoError(t, cErr)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindFeed(gomock.Any(), (*user.FeedCursor)(nil), int32(21)).Return(nil, expectedErr)
			uc := &usecase{tracer: lt, userRepo: userRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.ErrorIs(t, actualErr, expectedErr)
			require.Nil(t, actual)
		})

		t.Run("ユーザーの都道府県が解決できない場合、ErrInternalが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			first := 20
			cursor, cErr := paging.NewCursor(nil, &first)
			require.NoError(t, cErr)

			u1 := newFeedUser("feed_user_1", now)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindFeed(gomock.Any(), (*user.FeedCursor)(nil), int32(21)).Return(user.Users{u1}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{}, nil)
			uc := &usecase{tracer: lt, userRepo: userRepo, pftRepo: pftRepo, authorizer: newAllowAuthorizer(ctrl)}

			actual, actualErr := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.ErrorIs(t, actualErr, apperror.ErrInternal)
			require.Nil(t, actual)
		})

		t.Run("認可が拒否される場合、ErrForbidden が返りリポジトリへ到達しない", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			cursor, err := paging.NewCursor(nil, nil)
			require.NoError(t, err)

			// userRepo は EXPECT を持たないため、認可より先にリポジトリを呼ぶ実装に戻ると失敗する。
			uc := &usecase{
				tracer:     lt,
				authorizer: newDenyAuthorizer(ctrl),
				userRepo:   mock_user.NewMockRepository(ctrl),
				pftRepo:    mock_prefecture.NewMockRepository(ctrl),
			}

			actual, err := uc.ListUsersFeed(ctx, newTestAuthn(t), cursor)
			require.Nil(t, actual)
			require.ErrorIs(t, err, authz.ErrForbidden)
		})
	})
}

func Test_usecase_authorizeUserCollection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	authn, err := authbd.New("subject", "mock", nil, nil)
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("所有者を持たないリソースとして問い合わせ、許可された場合はnilが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().
				Authorize(gomock.Any(), authn, authz.ActionUserList, authz.NewResource("user", nil)).
				Return(nil)

			uc := &usecase{authorizer: authorizer}

			require.NoError(t, uc.authorizeUserCollection(ctx, authn))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合、ErrUnauthenticatedが返り認可判定は行われない", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			uc := &usecase{authorizer: authorizer}

			require.ErrorIs(t, uc.authorizeUserCollection(ctx, nil), apperror.ErrUnauthenticated)
		})

		t.Run("認可が拒否された場合、認可エラーが伝播される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().
				Authorize(gomock.Any(), authn, authz.ActionUserList, authz.NewResource("user", nil)).
				Return(authz.ErrForbidden)

			uc := &usecase{authorizer: authorizer}

			require.ErrorIs(t, uc.authorizeUserCollection(ctx, authn), authz.ErrForbidden)
		})
	})
}

func Test_usecase_authorizeUserAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := uuidtestkit.NewTestFromSalt(t, "authorize_user_access")

	authn, err := authbd.New("subject", "mock", nil, nil)
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnが非nilで認可が許可された場合、nilが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), authn, authz.ActionUserGet, authz.NewResource("user", &id)).Return(nil)

			uc := &usecase{authorizer: authorizer}

			err := uc.authorizeUserAccess(ctx, authn, authz.ActionUserGet, id)
			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("authnがnilの場合、ErrUnauthenticatedが返り認可判定は行われない", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			authorizer := mock_authz.NewMockAuthorizer(ctrl)

			uc := &usecase{authorizer: authorizer}

			err := uc.authorizeUserAccess(ctx, nil, authz.ActionUserGet, id)
			require.ErrorIs(t, err, apperror.ErrUnauthenticated)
		})

		t.Run("認可が拒否された場合、認可エラーが伝播される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			authorizer := mock_authz.NewMockAuthorizer(ctrl)
			authorizer.EXPECT().Authorize(gomock.Any(), authn, authz.ActionUserGet, authz.NewResource("user", &id)).Return(authz.ErrForbidden)

			uc := &usecase{authorizer: authorizer}

			err := uc.authorizeUserAccess(ctx, authn, authz.ActionUserGet, id)
			require.ErrorIs(t, err, authz.ErrForbidden)
		})
	})
}

func Test_usecase_resolvePatchPrefecture(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	prefectureID := uuidtestkit.NewTestFromSalt(t, "resolve_patch_prefecture")

	pftDomain, err := prefecture.New(prefectureID, "prefecture_name", 1)
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("名前指定がある場合、名前で解決した都道府県が返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			name := "prefecture_name"
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByName(gomock.Any(), name).Return(pftDomain, nil)

			uc := &usecase{pftRepo: pftRepo}

			actual, err := uc.resolvePatchPrefecture(ctx, &name, prefectureID)
			require.NoError(t, err)
			assert.Equal(t, pftDomain, actual)
		})

		t.Run("名前指定がない場合、既存IDで解決した都道府県が返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByID(gomock.Any(), prefectureID).Return(pftDomain, nil)

			uc := &usecase{pftRepo: pftRepo}

			actual, err := uc.resolvePatchPrefecture(ctx, nil, prefectureID)
			require.NoError(t, err)
			assert.Equal(t, pftDomain, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("名前指定がなく既存IDが解決できない場合、参照整合性破れが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByID(gomock.Any(), prefectureID).Return(nil, apperror.ErrNotFound)

			uc := &usecase{pftRepo: pftRepo}

			actual, err := uc.resolvePatchPrefecture(ctx, nil, prefectureID)
			require.Nil(t, actual)
			require.ErrorIs(t, err, errOrphanPrefecture)
		})

		t.Run("名前指定がなく既存ID解決で他のエラーが発生した場合、エラーが伝播される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByID(gomock.Any(), prefectureID).Return(nil, expectedErr)

			uc := &usecase{pftRepo: pftRepo}

			actual, err := uc.resolvePatchPrefecture(ctx, nil, prefectureID)
			require.Nil(t, actual)
			require.ErrorIs(t, err, expectedErr)
		})
	})
}

func Test_usecase_toUserViews(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	prefectureID := uuidtestkit.NewTestFromSalt(t, "to_user_views_prefecture")

	userDomain, err := user.New(uuidtestkit.NewTestFromSalt(t, "to_user_views_user"), user.Attributes{
		FirstName:    "first_name",
		LastName:     "last_name",
		Email:        "user@example.com",
		Phone:        "phone_number",
		PrefectureID: prefectureID,
		City:         "city_name",
		Street:       "town_address",
		PostalCode:   "150-0001",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)

	prefectureDomain, err := prefecture.New(prefectureID, "prefecture_name", 1)
	require.NoError(t, err)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全ユーザーの都道府県が解決できる場合、UserViewのリストが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)

			uc := &usecase{
				tracer:  observability.NewNoopTracerFactory(t).Usecase(),
				pftRepo: pftRepo,
			}

			actual, err := uc.toUserViews(ctx, user.Users{userDomain})
			require.NoError(t, err)
			require.Len(t, actual, 1)
			assert.Equal(t, prefectureDomain.Name(), actual[0].PrefectureName)
			assert.Equal(t, userDomain.Email(), actual[0].Email)
		})

		t.Run("ユーザーが0件の場合、空リストが返り都道府県の解決も空で行われる", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			// 0 件でも解決自体は空スライスで呼ばれる。呼ばない最適化に変えると
			// 呼び出し回数が変わるため、ここで現在の契約を固定する。
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{}).Return(prefecture.Prefectures{}, nil)

			uc := &usecase{
				tracer:  observability.NewNoopTracerFactory(t).Usecase(),
				pftRepo: pftRepo,
			}

			actual, err := uc.toUserViews(ctx, user.Users{})
			require.NoError(t, err)
			assert.Empty(t, actual)
			// 一覧は「該当なし」を空リストで表す。nil にすると JSON が null になり契約が変わる。
			assert.NotNil(t, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("都道府県取得でエラーが発生した場合、エラーが伝播される", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			expectedErr := testkit.ExpectedDBError()
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(nil, expectedErr)

			uc := &usecase{
				tracer:  observability.NewNoopTracerFactory(t).Usecase(),
				pftRepo: pftRepo,
			}

			actual, err := uc.toUserViews(ctx, user.Users{userDomain})
			require.Nil(t, actual)
			require.ErrorIs(t, err, expectedErr)
		})

		t.Run("ユーザーが参照する都道府県が解決できない場合、参照整合性破れが返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{}, nil)

			uc := &usecase{
				tracer:  observability.NewNoopTracerFactory(t).Usecase(),
				pftRepo: pftRepo,
			}

			actual, err := uc.toUserViews(ctx, user.Users{userDomain})
			require.Nil(t, actual)
			require.ErrorIs(t, err, errOrphanPrefecture)
		})
	})
}

// newCreateDTO は、テスト用のCreateParamsDTOを生成するヘルパー関数です。
func newCreateDTO(u *user.User, pName string) *CreateParamsDTO {
	return &CreateParamsDTO{
		Issuer:         "https://issuer.example.com",
		Subject:        "subject-" + u.ID().String(),
		FirstName:      u.FirstName(),
		LastName:       u.LastName(),
		Email:          u.Email(),
		Phone:          u.Phone(),
		PostalCode:     u.PostalCode(),
		PrefectureName: pName,
		City:           u.City(),
		Street:         u.Street(),
		Building:       u.Building(),
	}
}

func Test_toUserView(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	building := "building_name"

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("エンティティの属性と引数の都道府県名からDTOを構築する", func(t *testing.T) {
			t.Parallel()

			u, err := user.New(uuidtestkit.NewTestFromSalt(t, "to_user_view_user"), user.Attributes{
				FirstName:    "first_name",
				LastName:     "last_name",
				Email:        "user@example.com",
				Phone:        "phone_number",
				PrefectureID: uuidtestkit.NewTestFromSalt(t, "to_user_view_prefecture"),
				City:         "city_name",
				Street:       "town_address",
				Building:     &building,
				PostalCode:   "150-0001",
				CreatedAt:    now,
				UpdatedAt:    now,
			})
			require.NoError(t, err)

			actual := toUserView(u, "prefecture_name")

			assert.Equal(t, u.ID(), actual.ID)
			assert.Equal(t, "first_name", actual.FirstName)
			assert.Equal(t, "last_name", actual.LastName)
			assert.Equal(t, "user@example.com", actual.Email)
			assert.Equal(t, "phone_number", actual.Phone)
			assert.Equal(t, "150-0001", actual.PostalCode)
			assert.Equal(t, "prefecture_name", actual.PrefectureName)
			assert.Equal(t, "city_name", actual.City)
			assert.Equal(t, "town_address", actual.Street)
			require.NotNil(t, actual.Building)
			assert.Equal(t, building, *actual.Building)
			assert.Nil(t, actual.DeletedAt)
		})

		t.Run("削除済みエンティティの場合、DeletedAtを引き継ぐ", func(t *testing.T) {
			t.Parallel()

			deletedAt := now.Add(24 * time.Hour)
			u, err := user.New(uuidtestkit.NewTestFromSalt(t, "to_user_view_deleted"), user.Attributes{
				FirstName:    "first_name",
				LastName:     "last_name",
				Email:        "deleted@example.com",
				Phone:        "phone_number",
				PrefectureID: uuidtestkit.NewTestFromSalt(t, "to_user_view_prefecture"),
				City:         "city_name",
				Street:       "town_address",
				PostalCode:   "150-0001",
				CreatedAt:    now,
				UpdatedAt:    now,
				DeletedAt:    &deletedAt,
			})
			require.NoError(t, err)

			actual := toUserView(u, "prefecture_name")

			assert.Nil(t, actual.Building)
			require.NotNil(t, actual.DeletedAt)
			assert.Equal(t, deletedAt, *actual.DeletedAt)
		})
	})
}

func Test_usecase_listUsers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lt := observability.NewMockUsecaseLayerTracer(t)
	now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	prefectureID := uuidtestkit.NewTestFromSalt(t, "prefecture_domain")

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("認可済みの呼出元向けの内部処理として、認可を経ずに一覧を返す", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			page := 1
			perPage := 100
			p, err := paging.NewPageFrom1Based(&page, &perPage)
			require.NoError(t, err)

			userDomain := newActiveUser(t, uuidtestkit.NewTestFromSalt(t, "list_users_helper"), prefectureID, now)
			prefectureDomain, err := prefecture.New(prefectureID, "prefecture_name", 1)
			require.NoError(t, err)

			userRepo := mock_user.NewMockRepository(ctrl)
			userRepo.EXPECT().FindByActive(gomock.Any(), nil, p.Limit32(), p.Offset32()).Return(user.Users{userDomain}, nil)
			pftRepo := mock_prefecture.NewMockRepository(ctrl)
			pftRepo.EXPECT().FindByIDs(gomock.Any(), []uuid.UUID{prefectureID}).Return(prefecture.Prefectures{prefectureDomain}, nil)

			// authorizer は EXPECT を持たない。ヘルパ自身が認可を行う実装に変わると、
			// 合成メソッドが二重に認可することになるため、ここで落とす。
			uc := &usecase{
				tracer:     lt,
				authorizer: mock_authz.NewMockAuthorizer(ctrl),
				userRepo:   userRepo,
				pftRepo:    pftRepo,
			}

			expected := []UserView{{
				ID:             userDomain.ID(),
				FirstName:      userDomain.FirstName(),
				LastName:       userDomain.LastName(),
				Email:          userDomain.Email(),
				Phone:          userDomain.Phone(),
				PostalCode:     userDomain.PostalCode(),
				PrefectureName: prefectureDomain.Name(),
				City:           userDomain.City(),
				Street:         userDomain.Street(),
				Building:       userDomain.Building(),
				DeletedAt:      userDomain.DeletedAt(),
			}}

			actual, err := uc.listUsers(ctx, nil, p)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("page が nil の場合、ErrInvalidArgument が返る", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)

			uc := &usecase{
				tracer:     lt,
				authorizer: mock_authz.NewMockAuthorizer(ctrl),
				userRepo:   mock_user.NewMockRepository(ctrl),
			}

			actual, err := uc.listUsers(ctx, nil, nil)
			require.Nil(t, actual)
			require.ErrorIs(t, err, apperror.ErrInvalidArgument)
		})
	})
}
