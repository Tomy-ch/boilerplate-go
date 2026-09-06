package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-boilerplate/pkg/xerrors"
)

// eventLogTable は、fixture の table 名です。他の文言に含まれない名前にしてあり、
// エラーがどの table のものかを添える wrap が消えたことを検出できます。
const eventLogTable = "realtime_event_log_local_wt3"

var errAPI = xerrors.New("api failed")

// fakeTableAPI は、削除済みの table を集合として持つ tableAPI です。
// describeLeft は「DeleteTable が返った後もしばらく引ける」非同期の削除を再現します。
type fakeTableAPI struct {
	existing     map[string]bool
	deleted      []string
	described    []string
	deleteErr    error
	describeErr  error
	describeLeft int
}

// notFound は、SDK が返すのと同じ包み方で ResourceNotFoundException を返します。
// 実際の呼び出しでは smithy が例外を OperationError の中へ入れて返すため、裸で返す fake では
// 判定を `xerrors.As` から型アサーションへ落とす退行を見逃します。
func notFound(op string) error {
	return &smithy.OperationError{
		ServiceID:     "DynamoDB",
		OperationName: op,
		Err:           &dynamodbtypes.ResourceNotFoundException{},
	}
}

func newFakeTableAPI(existing ...string) *fakeTableAPI {
	f := &fakeTableAPI{existing: make(map[string]bool, len(existing))}
	for _, name := range existing {
		f.existing[name] = true
	}

	return f
}

func (f *fakeTableAPI) DeleteTable(
	_ context.Context, in *dynamodb.DeleteTableInput, _ ...func(*dynamodb.Options),
) (*dynamodb.DeleteTableOutput, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}

	name := aws.ToString(in.TableName)
	if !f.existing[name] {
		return nil, notFound("DeleteTable")
	}

	delete(f.existing, name)
	f.deleted = append(f.deleted, name)

	return &dynamodb.DeleteTableOutput{}, nil
}

func (f *fakeTableAPI) DescribeTable(
	ctx context.Context, in *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options),
) (*dynamodb.DescribeTableOutput, error) {
	f.described = append(f.described, aws.ToString(in.TableName))

	// 実 SDK は取り消し済みの ctx で呼ばれると転送前に締切のエラーを返す。
	if err := ctx.Err(); err != nil {
		return nil, xerrors.Wrap(err, "DescribeTable")
	}

	if f.describeErr != nil {
		return nil, f.describeErr
	}

	if f.describeLeft > 0 {
		f.describeLeft--

		return &dynamodb.DescribeTableOutput{}, nil
	}

	return nil, notFound("DescribeTable")
}

func Test_deleteTables(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("渡された順に削除する", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("a", "b", "c")
			var out bytes.Buffer

			require.NoError(t, deleteTables(context.Background(), api, []string{"a", "b", "c"}, &out))
			assert.Equal(t, []string{"a", "b", "c"}, api.deleted)
			assert.Contains(t, out.String(), "a を削除しました")
		})

		t.Run("既に無いtableは待たずに読み飛ばす", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("b")
			var out bytes.Buffer

			require.NoError(t, deleteTables(context.Background(), api, []string{"a", "b"}, &out))
			assert.Equal(t, []string{"b"}, api.deleted, "存在しない a の削除は成功していない")
			assert.NotContains(t, api.described, "a", "消えるのを待つ相手がいない")
			assert.Contains(t, out.String(), "a は既にありません")
			assert.NotContains(t, out.String(), "a を削除しました")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("削除に失敗したらtable名を添えて止まる", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI(eventLogTable)
			api.deleteErr = errAPI

			err := deleteTables(context.Background(), api, []string{eventLogTable}, &bytes.Buffer{})
			require.ErrorIs(t, err, errAPI)
			require.ErrorContains(t, err, eventLogTable, "3 つのどれで失敗したかを呼び出した人が知る必要がある")
		})

		t.Run("消えたかを確かめられなければ止まる", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI(eventLogTable)
			api.describeErr = errAPI

			err := deleteTables(context.Background(), api, []string{eventLogTable}, &bytes.Buffer{})
			require.ErrorIs(t, err, errAPI, "消えたと言い切れないまま作り直しへ進ませない")
			require.ErrorContains(t, err, eventLogTable)
		})

		t.Run("途中で失敗したら後続のtableに触れない", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("a", "b")
			api.deleteErr = errAPI

			require.ErrorIs(t, deleteTables(context.Background(), api, []string{"a", "b"}, &bytes.Buffer{}), errAPI)
			assert.NotContains(t, api.described, "b", "1 つ目で止まらなければ 2 つ目にも触れてしまう")
		})
	})
}

func Test_deleteTable(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("あるtableを削除し、削除を要求したと返す", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("a")

			deleted, err := deleteTable(context.Background(), api, "a")
			require.NoError(t, err)
			assert.True(t, deleted)
			assert.Equal(t, []string{"a"}, api.deleted)
		})

		t.Run("既に無いtableは成功として読み飛ばす", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI()

			deleted, err := deleteTable(context.Background(), api, "a")
			require.NoError(t, err, "無い状態にするのが目的なので、無いのは成功である")
			assert.False(t, deleted, "削除を要求していないことまで区別する")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("NotFound以外のエラーはそのまま返す", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("a")
			api.deleteErr = errAPI

			deleted, err := deleteTable(context.Background(), api, "a")
			require.ErrorIs(t, err, errAPI)
			assert.False(t, deleted)
		})
	})
}

func Test_waitGone(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既に引けなければすぐ返る", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI()

			require.NoError(t, waitGone(context.Background(), api, "a"))
			assert.Len(t, api.described, 1)
		})

		t.Run("引ける間は間隔を置いて待ち続ける", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI()
			api.describeLeft = 2
			start := time.Now()

			require.NoError(t, waitGone(context.Background(), api, "a"))
			assert.Zero(t, api.describeLeft)
			assert.GreaterOrEqual(t, time.Since(start), 2*gonePollInterval,
				"間隔を置かずに回すと DynamoDB Local を hot loop で叩く")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("引けるかを確かめられなければそのエラーを返す", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI()
			api.describeErr = errAPI

			require.ErrorIs(t, waitGone(context.Background(), api, eventLogTable), errAPI)
		})

		t.Run("問い合わせの最中に打ち切られてもerrGoneを付ける", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI()
			api.describeLeft = 1 << 30

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := waitGone(ctx, api, eventLogTable)
			require.ErrorIs(t, err, errGone, "同じ「消え切らなかった」が経路で別の error に見えてはならない")
			require.ErrorIs(t, err, context.Canceled)
		})

		t.Run("待っている間に打ち切られたらerrGoneを返す", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI()
			api.describeLeft = 1 << 30

			ctx, cancel := context.WithTimeout(context.Background(), gonePollInterval/2)
			defer cancel()

			err := waitGone(ctx, api, eventLogTable)
			require.ErrorIs(t, err, errGone)
			require.ErrorIs(t, err, context.DeadlineExceeded)
		})
	})
}
