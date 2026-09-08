package useridentity

import (
	"context"
	"testing"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/config"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/observability"
	authbd "go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistrar(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("registrar のインスタンスが生成される", func(t *testing.T) {
			t.Parallel()

			testDB := testkit.NewTestDB(t)
			tf := observability.NewNoopTracerFactory(t)
			expected := &registrar{
				db:     testDB,
				tracer: tf.Infra(),
			}
			actual := NewRegistrar(testDB, tf)
			assert.Equal(t, expected, actual)
		})
	})
}

func Test_registrar_Register(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	reg := &registrar{
		tracer: lt,
		db:     testDB,
	}
	res := &resolver{
		tracer: lt,
		db:     testDB,
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("結び付けた issuer と subject が resolver から解決できるようになる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				issuer := config.ResolvedAuthIssuer(t)
				subject := "user-registrar-roundtrip"

				userID, err := uuid.Parse(johnUserID)
				require.NoError(t, err)

				require.NoError(t, reg.Register(ctx, userID, issuer, subject))

				authn, err := authbd.New(subject, issuer, nil, nil)
				require.NoError(t, err)

				resolved, err := res.Resolve(ctx, authn)
				require.NoError(t, err)
				require.True(t, resolved.HasUserID())
				got, err := resolved.UserID()
				require.NoError(t, err)
				assert.Equal(t, johnUserID, got.String())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("同じissuerとsubjectの組を二度結び付けるとErrConflictを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				issuer := config.ResolvedAuthIssuer(t)
				subject := "user-registrar-duplicate"

				userID, err := uuid.Parse(johnUserID)
				require.NoError(t, err)

				require.NoError(t, reg.Register(ctx, userID, issuer, subject))
				// 二重登録を止めるのはこの一意制約。登録ユースケースはこの Conflict に乗っている。
				err = reg.Register(ctx, userID, issuer, subject)
				require.ErrorIs(t, err, apperror.ErrConflict)
			})
		})

		t.Run("在籍しない利用者への結び付けはErrInvalidArgumentを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				unknown, err := uuid.New()
				require.NoError(t, err)

				err = reg.Register(ctx, unknown, config.ResolvedAuthIssuer(t), "user-registrar-orphan")
				require.ErrorIs(t, err, apperror.ErrInvalidArgument)
			})
		})
	})
}
