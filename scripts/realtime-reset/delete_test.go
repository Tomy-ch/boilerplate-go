package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-boilerplate/pkg/xerrors"
)

var errAPI = xerrors.New("api failed")

// fakeTableAPI は、削除済みの table を集合として持つ tableAPI です。
// describeLeft は「DeleteTable が返った後もしばらく引ける」非同期の削除を再現します。
type fakeTableAPI struct {
	existing     map[string]bool
	deleted      []string
	deleteErr    error
	describeErr  error
	describeLeft int
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
		return nil, &dynamodbtypes.ResourceNotFoundException{}
	}

	delete(f.existing, name)
	f.deleted = append(f.deleted, name)

	return &dynamodb.DeleteTableOutput{}, nil
}

func (f *fakeTableAPI) DescribeTable(
	_ context.Context, _ *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options),
) (*dynamodb.DescribeTableOutput, error) {
	if f.describeErr != nil {
		return nil, f.describeErr
	}

	if f.describeLeft > 0 {
		f.describeLeft--

		return &dynamodb.DescribeTableOutput{}, nil
	}

	return nil, &dynamodbtypes.ResourceNotFoundException{}
}

func Test_deleteTables(t *testing.T) {
	t.Parallel()

	t.Run("渡された順に削除する", func(t *testing.T) {
		t.Parallel()

		api := newFakeTableAPI("a", "b", "c")
		var out bytes.Buffer

		require.NoError(t, deleteTables(context.Background(), api, []string{"a", "b", "c"}, &out))
		assert.Equal(t, []string{"a", "b", "c"}, api.deleted)
		assert.Contains(t, out.String(), "a を削除しました")
	})

	t.Run("既に無いtableは成功として読み飛ばす", func(t *testing.T) {
		t.Parallel()

		api := newFakeTableAPI("b")
		var out bytes.Buffer

		require.NoError(t, deleteTables(context.Background(), api, []string{"a", "b"}, &out))
		assert.Equal(t, []string{"b"}, api.deleted, "存在しない a は削除要求に数えない")
		assert.Contains(t, out.String(), "a は既にありません")
	})

	t.Run("消え切るまで待つ", func(t *testing.T) {
		t.Parallel()

		api := newFakeTableAPI("a")
		api.describeLeft = 2
		var out bytes.Buffer

		require.NoError(t, deleteTables(context.Background(), api, []string{"a"}, &out))
		assert.Zero(t, api.describeLeft, "引ける間は待ち続ける")
	})

	t.Run("削除に失敗したらtable名を添えて止まる", func(t *testing.T) {
		t.Parallel()

		api := newFakeTableAPI("a")
		api.deleteErr = errAPI

		err := deleteTables(context.Background(), api, []string{"a"}, &bytes.Buffer{})
		require.ErrorIs(t, err, errAPI)
		assert.Contains(t, err.Error(), "a")
	})

	t.Run("消えたかを確かめられなければ止まる", func(t *testing.T) {
		t.Parallel()

		api := newFakeTableAPI("a")
		api.describeErr = errAPI

		err := deleteTables(context.Background(), api, []string{"a"}, &bytes.Buffer{})
		require.ErrorIs(t, err, errAPI, "消えたと言い切れないまま作り直しへ進ませない")
	})

	t.Run("制限時間内に消え切らなければerrGoneで止まる", func(t *testing.T) {
		t.Parallel()

		api := newFakeTableAPI("a")
		api.describeLeft = 1 << 30

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := deleteTables(ctx, api, []string{"a"}, &bytes.Buffer{})
		require.ErrorIs(t, err, errGone)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
