package dbslot

import (
	"bytes"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	root "go-boilerplate"
)

//nolint:paralleltest // t.Setenv はプロセスの環境を変更するため並列化不可
func TestLoadRealtimeBase(t *testing.T) {
	t.Run("正常系", func(t *testing.T) {
		t.Run("埋め込みenvが宣言する値をそのまま返す", func(t *testing.T) {
			b, err := root.FS.ReadFile(embeddedEnvFile)
			require.NoError(t, err)
			kv, err := godotenv.Parse(bytes.NewReader(b))
			require.NoError(t, err)

			got, err := LoadRealtimeBase()
			require.NoError(t, err)
			assert.Equal(t, kv[realtimeTableSuffixKey], got.TableSuffix, "読むファイルとキーの写像まで固定する")
			assert.Equal(t, kv[realtimeQueuePrefixKey], got.QueuePrefix)
			assert.Equal(t, kv[realtimeTopicKey], got.Topic)
		})

		t.Run("OSの環境変数を混ぜない", func(t *testing.T) {
			base, err := LoadRealtimeBase()
			require.NoError(t, err)

			t.Setenv(realtimeTableSuffixKey, base.TableSuffix+"_wt9")

			got, err := LoadRealtimeBase()
			require.NoError(t, err)
			assert.Equal(t, base.TableSuffix, got.TableSuffix,
				"db-slot は同じ名前を撒くので、混ぜると自分の出力を基底として読み直す")
		})
	})
}

func Test_realtimeBaseFrom(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("3つのキーを読む", func(t *testing.T) {
			t.Parallel()

			got, err := realtimeBaseFrom(map[string]string{
				realtimeTableSuffixKey: "local",
				realtimeQueuePrefixKey: "realtime-local",
				realtimeTopicKey:       "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local",
			})
			require.NoError(t, err)
			assert.Equal(t, "local", got.TableSuffix)
			assert.Equal(t, "realtime-local", got.QueuePrefix)
			assert.Equal(t, "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local", got.Topic)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("table suffixが無ければ止める", func(t *testing.T) {
			t.Parallel()

			_, err := realtimeBaseFrom(map[string]string{
				realtimeQueuePrefixKey: "realtime-local",
				realtimeTopicKey:       "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local",
			})
			require.ErrorIs(t, err, ErrRealtimeBaseMissing)
			assert.ErrorContains(t, err, realtimeTableSuffixKey, "どのキーが無いかを呼び出した人が知る必要がある")
		})

		t.Run("queue prefixが無ければ止める", func(t *testing.T) {
			t.Parallel()

			_, err := realtimeBaseFrom(map[string]string{
				realtimeTableSuffixKey: "local",
				realtimeTopicKey:       "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local",
			})
			require.ErrorIs(t, err, ErrRealtimeBaseMissing)
			assert.ErrorContains(t, err, realtimeQueuePrefixKey)
		})

		t.Run("topicが無ければ止める", func(t *testing.T) {
			t.Parallel()

			_, err := realtimeBaseFrom(map[string]string{
				realtimeTableSuffixKey: "local",
				realtimeQueuePrefixKey: "realtime-local",
			})
			require.ErrorIs(t, err, ErrRealtimeBaseMissing,
				"空の topic だけ既定値へ落ちると、table は分離されたまま topic だけ主 checkout と共有する")
			assert.ErrorContains(t, err, realtimeTopicKey)
		})

		t.Run("空文字は宣言が無いのと同じに扱う", func(t *testing.T) {
			t.Parallel()

			_, err := realtimeBaseFrom(map[string]string{
				realtimeTableSuffixKey: "",
				realtimeQueuePrefixKey: "realtime-local",
				realtimeTopicKey:       "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local",
			})
			require.ErrorIs(t, err, ErrRealtimeBaseMissing,
				"空を通すと compose の `${VAR:-local}` が主 checkout の名前へ置き換える")
		})
	})
}
