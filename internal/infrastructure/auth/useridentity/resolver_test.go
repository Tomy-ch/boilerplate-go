package useridentity

import (
	"context"
	"testing"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/config"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/observability"
	authbd "go-boilerplate/internal/usecase/boundary/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIssuerMock = "mock"
	// johnUserID は JWT/mock 両 issuer に登録された未削除ユーザー（John Doe）の内部 ID。
	johnUserID = "550e8400-e29b-41d4-a716-446655440000"
	// zeroUserSubject は、ゼロ値 UUID のユーザーへ紐づく identity の subject。
	zeroUserSubject = "user-zero-uuid"
)

// seed が投入する JWT identity の issuer は環境の AUTH_ISSUER で、worktree の DB スロットでポートがずれる。
// テスト用 DB を seed するのも値を渡すのも make のため、DB を使う本テストは make go-test / make go-test-cached
// 経由で実行する（素の go test は make の渡す値を受け取らない）。

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("resolver のインスタンスが生成される", func(t *testing.T) {
			t.Parallel()

			testDB := testkit.NewTestDB(t)
			tf := observability.NewNoopTracerFactory(t)
			expected := &resolver{
				db:     testDB,
				tracer: tf.Infra(),
			}
			actual := New(testDB, tf)
			assert.Equal(t, expected, actual)
		})
	})
}

func Test_resolver_Resolve(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	lt := observability.NewMockInfraLayerTracer(t)
	txm := testkit.NewTestTransactionRunner(t)

	repo := &resolver{
		tracer: lt,
		db:     testDB,
	}

	newAuthn := func(t *testing.T, issuer, subject string) *authbd.Authn {
		t.Helper()
		authn, err := authbd.New(subject, issuer, nil, nil)
		require.NoError(t, err)
		return authn
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("JWT issuer と subject に対応する未削除ユーザーの UserID を解決する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				resolved, err := repo.Resolve(ctx, newAuthn(t, config.ResolvedAuthIssuer(t), "user-john-doe"))
				require.NoError(t, err)
				require.True(t, resolved.HasUserID())
				userID, err := resolved.UserID()
				require.NoError(t, err)
				assert.Equal(t, johnUserID, userID.String())
			})
		})

		t.Run("mock issuer と subject でも同じユーザーの UserID を解決する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				resolved, err := repo.Resolve(ctx, newAuthn(t, testIssuerMock, johnUserID))
				require.NoError(t, err)
				require.True(t, resolved.HasUserID())
				userID, err := resolved.UserID()
				require.NoError(t, err)
				assert.Equal(t, johnUserID, userID.String())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("削除済みユーザーの identity は ErrUserUnavailable を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				resolved, err := repo.Resolve(ctx, newAuthn(t, config.ResolvedAuthIssuer(t), "user-charlie-davis"))
				assert.Nil(t, resolved)
				require.ErrorIs(t, err, authbd.ErrUserUnavailable)
			})
		})

		t.Run("未登録の subject は ErrIdentityNotFound を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				resolved, err := repo.Resolve(ctx, newAuthn(t, config.ResolvedAuthIssuer(t), "user-nonexistent"))
				assert.Nil(t, resolved)
				require.ErrorIs(t, err, authbd.ErrIdentityNotFound)
			})
		})

		t.Run("issuer が一致しない場合は ErrIdentityNotFound を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				resolved, err := repo.Resolve(ctx, newAuthn(t, "https://evil.example.com", "user-john-doe"))
				assert.Nil(t, resolved)
				require.ErrorIs(t, err, authbd.ErrIdentityNotFound)
			})
		})

		t.Run("解決したユーザーIDがゼロ値の場合は ErrUserIDZero を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				// users.id はゼロ値 UUID を禁じていないため、seed には置けない不正データを
				// ロールバックされるトランザクション内で直接投入して経路を作る。
				db := driver.New(ctx, testDB)
				_, err := db.Exec(ctx, `
					INSERT INTO users (id, first_name, last_name, email, phone, prefecture_id, city, street, postal_code)
					VALUES ('00000000-0000-0000-0000-000000000000', 'Zero', 'User', 'zero.user@example.com', '000-0000',
					        (SELECT id FROM prefectures LIMIT 1), '市区町村', '1-1', '000-0000')`)
				require.NoError(t, err)

				_, err = db.Exec(ctx, `
					INSERT INTO user_identities (id, user_id, issuer, subject)
					VALUES ('3f2a1c4e-0000-4000-8000-0000000000ff', '00000000-0000-0000-0000-000000000000', $1, $2)`,
					testIssuerMock, zeroUserSubject)
				require.NoError(t, err)

				resolved, err := repo.Resolve(ctx, newAuthn(t, testIssuerMock, zeroUserSubject))
				assert.Nil(t, resolved)
				require.ErrorIs(t, err, authbd.ErrUserIDZero)
			})
		})

		t.Run("キャンセル済みコンテキストでは ErrCanceled へ正規化される", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			resolved, err := repo.Resolve(ctx, newAuthn(t, config.ResolvedAuthIssuer(t), "user-john-doe"))
			assert.Nil(t, resolved)
			require.ErrorIs(t, err, apperror.ErrCanceled)
		})
	})
}
