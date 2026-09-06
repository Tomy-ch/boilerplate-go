package inquiry

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-boilerplate/internal/apperror"
	domaininquiry "go-boilerplate/internal/domain/inquiry"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/uuid"
)

// takeSeedUser は、seed 済みの利用者を ID 順で index 番目に選びます。
//
// seed の SQL から ID を書き写すと、その利用者が実際に投入されているかは環境によって変わり、
// 外部キー違反で落ちます。並び順で取ることで、どの環境でも実在する利用者だけを使います。
// 問い合わせは利用者ごとに 1 件しか持てないため、並行するケースには別の index を渡します。
func takeSeedUser(ctx context.Context, t *testing.T, testDB driver.DatabaseDriver, index int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := driver.New(ctx, testDB).
		QueryRow(ctx, "SELECT id FROM users ORDER BY id LIMIT 1 OFFSET $1", index).
		Scan(&id)
	require.NoError(t, err, "seed 済みの利用者が %d 人未満です", index+1)
	return id
}

func mustNewUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.New()
	require.NoError(t, err)
	return id
}

// createInquiry は、指定した利用者の問い合わせを 1 件登録し、その集約を返します。
func createInquiry(ctx context.Context, t *testing.T, repo *repository, testDB driver.DatabaseDriver, userIndex int) *domaininquiry.Inquiry {
	t.Helper()
	i, err := domaininquiry.New(mustNewUUID(t), domaininquiry.Attributes{UserID: takeSeedUser(ctx, t, testDB, userIndex)})
	require.NoError(t, err)
	created, cerr := repo.CreateIfAbsent(ctx, i)
	require.NoError(t, cerr)
	return created
}

// updateUpdatedAt は、更新日時だけを差し替えて永続化します。
// 更新日時を進める入口はドメインでは AppendMessage だけなので、時刻を直接指定したい検証では
// 再構築した集約を Update へ渡します。
func updateUpdatedAt(
	ctx context.Context,
	t *testing.T,
	repo *repository,
	i *domaininquiry.Inquiry,
	now time.Time,
) {
	t.Helper()
	updated, err := domaininquiry.Reconstruct(i.ID(), domaininquiry.Attributes{UserID: i.UserID(), UpdatedAt: now})
	require.NoError(t, err)
	require.NoError(t, repo.Update(ctx, updated))
}

func TestNew(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	tf := observability.NewNoopTracerFactory(t)
	expected := &repository{tracer: tf.Infra(), db: testDB}

	assert.Equal(t, expected, New(testDB, tf))
}

func Test_repository_CreateIfAbsent(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("問い合わせを登録し利用者から引ける", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 0)

				got, err := repo.FindActiveByUserID(ctx, i.UserID())
				require.NoError(t, err)
				assert.Equal(t, i.ID(), got.ID())
				assert.Equal(t, i.UserID(), got.UserID())
			})
		})

		t.Run("作成日時と更新日時はDBの既定値が入る", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 1)

				got, err := repo.FindByID(ctx, i.ID())
				require.NoError(t, err)
				assert.False(t, got.CreatedAt().IsZero())
				assert.False(t, got.UpdatedAt().IsZero())
			})
		})

		t.Run("競合しても同じトランザクションを続けられる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				first := createInquiry(ctx, t, repo, testDB, 3)
				candidate, err := domaininquiry.New(
					mustNewUUID(t), domaininquiry.Attributes{UserID: first.UserID()},
				)
				require.NoError(t, err)

				_, cerr := repo.CreateIfAbsent(ctx, candidate)
				require.NoError(t, cerr)

				got, ferr := repo.FindActiveByUserID(ctx, first.UserID())
				require.NoError(t, ferr)
				assert.Equal(t, first.ID(), got.ID())
			})
		})

		t.Run("同じ利用者の問い合わせが既にあれば既存のものを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				first := createInquiry(ctx, t, repo, testDB, 2)
				candidate, err := domaininquiry.New(
					mustNewUUID(t), domaininquiry.Attributes{UserID: first.UserID()},
				)
				require.NoError(t, err)

				got, cerr := repo.CreateIfAbsent(ctx, candidate)

				require.NoError(t, cerr)
				assert.Equal(t, first.ID(), got.ID())
			})
		})
	})
}

func Test_repository_FindByID(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDから問い合わせを再構築して返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 3)

				got, err := repo.FindByID(ctx, i.ID())
				require.NoError(t, err)
				assert.Equal(t, i.ID(), got.ID())
				assert.Equal(t, i.UserID(), got.UserID())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("存在しないIDはNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				_, err := repo.FindByID(ctx, mustNewUUID(t))
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})
	})
}

func Test_repository_FindActiveByUserID(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("利用者から問い合わせを再構築して返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 4)

				got, err := repo.FindActiveByUserID(ctx, i.UserID())
				require.NoError(t, err)
				assert.Equal(t, i.ID(), got.ID())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("問い合わせを持たない利用者はNotFoundを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				_, err := repo.FindActiveByUserID(ctx, mustNewUUID(t))
				require.ErrorIs(t, err, apperror.ErrNotFound)
			})
		})
	})
}

func Test_repository_Update(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("更新日時を指定した時刻へ進める", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 5)
				stored, err := repo.FindByID(ctx, i.ID())
				require.NoError(t, err)
				now := stored.UpdatedAt().Add(time.Hour).Truncate(time.Microsecond)

				updateUpdatedAt(ctx, t, repo, i, now)

				got, err := repo.FindByID(ctx, i.ID())
				require.NoError(t, err)
				assert.True(t, now.Equal(got.UpdatedAt()), "期待 %s / 実際 %s", now, got.UpdatedAt())
			})
		})

		t.Run("存在しないIDでもエラーにしない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				absent, rerr := domaininquiry.Reconstruct(mustNewUUID(t), domaininquiry.Attributes{
					UserID: takeSeedUser(ctx, t, testDB, 0), UpdatedAt: time.Now().UTC(),
				})
				require.NoError(t, rerr)
				require.NoError(t, repo.Update(ctx, absent))
			})
		})
	})
}

func Test_repository_ListForOperator(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("更新日時の新しい順に先頭ページを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				older := createInquiry(ctx, t, repo, testDB, 6)
				base := time.Now().UTC().Truncate(time.Microsecond)
				// 作成日時より前へ動かすと読み直しが不変条件で落ちる（表に CHECK 制約が無く書けてしまう)。
				updateUpdatedAt(ctx, t, repo, older, base.Add(2*time.Hour))

				got, err := repo.ListForOperator(ctx, domaininquiry.ListParams{Limit: 100})
				require.NoError(t, err)

				require.NotEmpty(t, got)
				for idx := 1; idx < len(got); idx++ {
					assert.False(t, got[idx-1].UpdatedAt().Before(got[idx].UpdatedAt()))
				}
			})
		})

		t.Run("cursorを渡すとその位置より後ろだけを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				first, err := repo.ListForOperator(ctx, domaininquiry.ListParams{Limit: 1})
				require.NoError(t, err)
				if len(first) == 0 {
					t.Skip("問い合わせが 1 件も無いため cursor を組めない")
				}
				cursor := &domaininquiry.Cursor{UpdatedAt: first[0].UpdatedAt(), ID: first[0].ID()}

				got, err := repo.ListForOperator(ctx, domaininquiry.ListParams{Cursor: cursor, Limit: 100})
				require.NoError(t, err)

				for _, i := range got {
					assert.False(t, i.UpdatedAt().After(cursor.UpdatedAt))
					assert.NotEqual(t, cursor.ID, i.ID())
				}
			})
		})

		t.Run("上限件数を超えて返さない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.ListForOperator(ctx, domaininquiry.ListParams{Limit: 1})
				require.NoError(t, err)
				assert.LessOrEqual(t, len(got), 1)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("int32へ収まらない上限件数はエラーを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				_, err := repo.ListForOperator(ctx, domaininquiry.ListParams{Limit: math.MaxInt32 + 1})
				require.Error(t, err)
			})
		})
	})
}

func Test_reconstruct(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("行の各列を集約へ写す", func(t *testing.T) {
			t.Parallel()
			id, userID := mustNewUUID(t), mustNewUUID(t)
			createdAt := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)
			updatedAt := createdAt.Add(time.Hour)

			got, err := reconstruct(gen.Inquiries{
				ID: id, UserID: userID, CreatedAt: createdAt, UpdatedAt: updatedAt,
			})

			require.NoError(t, err)
			assert.Equal(t, id, got.ID())
			assert.Equal(t, userID, got.UserID())
			assert.Equal(t, createdAt, got.CreatedAt())
			assert.Equal(t, updatedAt, got.UpdatedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		// 表には更新日時と作成日時の前後関係を強制する制約が無いため、この分岐は
		// 破損した行から現実に到達する（構造的に到達不能な防御ではない）。
		t.Run("更新日時が作成日時より前の行はErrInternalへ正規化し元の分類は露出しない", func(t *testing.T) {
			t.Parallel()
			createdAt := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)

			_, err := reconstruct(gen.Inquiries{
				ID:        mustNewUUID(t),
				UserID:    mustNewUUID(t),
				CreatedAt: createdAt,
				UpdatedAt: createdAt.Add(-time.Nanosecond),
			})

			require.ErrorIs(t, err, apperror.ErrInternal)
			require.NotErrorIs(t, err, domaininquiry.ErrInvalidTime)
		})

		t.Run("利用者が未設定の行はErrInternalへ正規化し元の分類は露出しない", func(t *testing.T) {
			t.Parallel()

			_, err := reconstruct(gen.Inquiries{ID: mustNewUUID(t)})

			require.ErrorIs(t, err, apperror.ErrInternal)
			require.NotErrorIs(t, err, domaininquiry.ErrInvalidUserID)
		})
	})
}

func Test_flatten(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("埋め込み行から表の行だけを取り出す", func(t *testing.T) {
			t.Parallel()
			first, second := mustNewUUID(t), mustNewUUID(t)
			rows := []*gen.ListInquiriesForOperatorFirstRow{
				{Inquiries: gen.Inquiries{ID: first}},
				{Inquiries: gen.Inquiries{ID: second}},
			}

			got := flatten(rows, func(row *gen.ListInquiriesForOperatorFirstRow) gen.Inquiries {
				return row.Inquiries
			})

			require.Len(t, got, 2)
			assert.Equal(t, first, got[0].ID)
			assert.Equal(t, second, got[1].ID)
		})

		t.Run("空の入力では空のスライスを返す", func(t *testing.T) {
			t.Parallel()

			got := flatten(nil, func(row *gen.ListInquiriesForOperatorFirstRow) gen.Inquiries {
				return row.Inquiries
			})

			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	})
}

func Test_repository_listRows(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("cursorが無ければ先頭ページのクエリを選ぶ", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				db := gen.New(driver.New(ctx, testDB))

				got, err := repo.listRows(ctx, db, nil, 1)

				require.NoError(t, err)
				assert.LessOrEqual(t, len(got), 1)
			})
		})

		t.Run("cursorがあれば続きのページのクエリを選ぶ", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				db := gen.New(driver.New(ctx, testDB))
				cursor := &domaininquiry.Cursor{UpdatedAt: time.Now().UTC(), ID: mustNewUUID(t)}

				got, err := repo.listRows(ctx, db, cursor, 1)

				require.NoError(t, err)
				assert.LessOrEqual(t, len(got), 1)
			})
		})
	})
}
