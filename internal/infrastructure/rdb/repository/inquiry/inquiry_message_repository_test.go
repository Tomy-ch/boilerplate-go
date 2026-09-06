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
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/observability"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
)

// newMessage は、指定した位置のメッセージを組み立てます。
// 更新日時を動かさずに値だけが要るため、追加ではなく再構築の入口を使います。
func newMessage(
	t *testing.T,
	kind domaininquiry.AuthorKind,
	subject uuid.UUID,
	seq int64,
) *domaininquiry.Message {
	t.Helper()
	author, err := domaininquiry.NewAuthor(kind, subject)
	require.NoError(t, err)
	m, err := domaininquiry.ReconstructMessage(mustNewUUID(t), domaininquiry.MessageAttributes{
		Author:   author,
		Body:     "本文",
		Sequence: seq,
	})
	require.NoError(t, err)
	return m
}

func Test_repository_CreateMessage(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("メッセージを登録し送り手ごと再構築できる", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 0)
				m := newMessage(t, domaininquiry.AuthorKindUser, i.UserID(), 1)

				require.NoError(t, repo.CreateMessage(ctx, i.ID(), m))

				got, err := repo.ListMessages(ctx, i.ID(), domaininquiry.HistoryParams{UpToSequence: 1, Limit: 10})
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, m.ID(), got[0].ID())
				assert.Equal(t, domaininquiry.AuthorKindUser, got[0].Author().Kind())
				assert.Equal(t, i.UserID(), got[0].Author().SubjectID())
				assert.Equal(t, int64(1), got[0].Sequence())
			})
		})

		t.Run("回答者のメッセージも種別を保って往復する", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 1)
				m := newMessage(t, domaininquiry.AuthorKindOperator, i.UserID(), 1)

				require.NoError(t, repo.CreateMessage(ctx, i.ID(), m))

				got, err := repo.ListMessages(ctx, i.ID(), domaininquiry.HistoryParams{UpToSequence: 1, Limit: 10})
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, domaininquiry.AuthorKindOperator, got[0].Author().Kind())
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("同じ問い合わせに同じ位置を二重登録するとConflictを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 2)
				require.NoError(t, repo.CreateMessage(ctx, i.ID(), newMessage(t, domaininquiry.AuthorKindUser, i.UserID(), 1)))

				err := repo.CreateMessage(ctx, i.ID(), newMessage(t, domaininquiry.AuthorKindUser, i.UserID(), 1))
				require.ErrorIs(t, err, apperror.ErrConflict)
			})
		})
	})
}

func Test_repository_ListMessages(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}
	txm := testkit.NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("位置の昇順で返し上限より後ろは含めない", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 3)
				for seq := int64(1); seq <= 3; seq++ {
					require.NoError(t, repo.CreateMessage(ctx, i.ID(), newMessage(t, domaininquiry.AuthorKindUser, i.UserID(), seq)))
				}

				got, err := repo.ListMessages(ctx, i.ID(), domaininquiry.HistoryParams{UpToSequence: 2, Limit: 10})
				require.NoError(t, err)
				require.Len(t, got, 2)
				assert.Equal(t, int64(1), got[0].Sequence())
				assert.Equal(t, int64(2), got[1].Sequence())
			})
		})

		t.Run("開始位置より後ろだけを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				i := createInquiry(ctx, t, repo, testDB, 4)
				for seq := int64(1); seq <= 3; seq++ {
					require.NoError(t, repo.CreateMessage(ctx, i.ID(), newMessage(t, domaininquiry.AuthorKindUser, i.UserID(), seq)))
				}

				got, err := repo.ListMessages(ctx, i.ID(), domaininquiry.HistoryParams{
					AfterSequence: ptr.To(int64(1)), UpToSequence: 3, Limit: 10,
				})
				require.NoError(t, err)
				require.Len(t, got, 2)
				assert.Equal(t, int64(2), got[0].Sequence())
			})
		})

		t.Run("メッセージが無ければ空を返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				got, err := repo.ListMessages(ctx, mustNewUUID(t), domaininquiry.HistoryParams{UpToSequence: 10, Limit: 10})
				require.NoError(t, err)
				assert.Empty(t, got)
			})
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("int32へ収まらない上限件数はエラーを返す", func(t *testing.T) {
			t.Parallel()

			txm.WithinTx(func(ctx context.Context) {
				_, err := repo.ListMessages(ctx, mustNewUUID(t), domaininquiry.HistoryParams{
					UpToSequence: 1, Limit: math.MaxInt32 + 1,
				})
				require.Error(t, err)
			})
		})
	})
}

func Test_reconstructMessage(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("送り手を列の2つ組から値オブジェクトへ組み直す", func(t *testing.T) {
			t.Parallel()
			id, subjectID := mustNewUUID(t), mustNewUUID(t)
			createdAt := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)

			got, err := reconstructMessage(gen.InquiryMessages{
				ID:              id,
				InquiryID:       mustNewUUID(t),
				AuthorKind:      "operator",
				AuthorSubjectID: subjectID,
				Body:            "本文",
				StreamSequence:  3,
				CreatedAt:       createdAt,
			})

			require.NoError(t, err)
			assert.Equal(t, id, got.ID())
			assert.Equal(t, domaininquiry.AuthorKindOperator, got.Author().Kind())
			assert.Equal(t, subjectID, got.Author().SubjectID())
			assert.Equal(t, int64(3), got.Sequence())
			assert.Equal(t, createdAt, got.CreatedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		// 表の CHECK 制約と NOT NULL がある限り到達しないが、種別の検証はこの関数が
		// 担う境界なので、壊れた行を渡したときにメッセージを組み立てないことを固定する。
		t.Run("既知でない種別の行はErrInternalへ正規化し元の分類は露出しない", func(t *testing.T) {
			t.Parallel()

			_, err := reconstructMessage(gen.InquiryMessages{
				ID: mustNewUUID(t), InquiryID: mustNewUUID(t),
				AuthorKind: "admin", AuthorSubjectID: mustNewUUID(t),
				Body: "本文", StreamSequence: 1,
			})
			require.ErrorIs(t, err, apperror.ErrInternal)
			require.NotErrorIs(t, err, domaininquiry.ErrInvalidAuthorKind)
		})

		t.Run("送り手の主体IDが空の行はErrInternalへ正規化する", func(t *testing.T) {
			t.Parallel()

			_, err := reconstructMessage(gen.InquiryMessages{
				ID: mustNewUUID(t), InquiryID: mustNewUUID(t),
				AuthorKind: "user", AuthorSubjectID: uuid.UUID{},
				Body: "本文", StreamSequence: 1,
			})
			require.ErrorIs(t, err, apperror.ErrInternal)
		})

		t.Run("本文が空の行はErrInternalへ正規化し識別子も落ちる", func(t *testing.T) {
			t.Parallel()

			_, err := reconstructMessage(gen.InquiryMessages{
				ID: mustNewUUID(t), InquiryID: mustNewUUID(t),
				AuthorKind: "user", AuthorSubjectID: mustNewUUID(t),
				Body: "", StreamSequence: 1,
			})
			require.ErrorIs(t, err, apperror.ErrInternal)
			_, ok := apperror.MetaFrom(err)
			assert.False(t, ok)
		})
	})
}
