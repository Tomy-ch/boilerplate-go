package main

import (
	"bytes"
	"context"
	"flag"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解決したtableを順に削除する", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("x", "y")
			var out bytes.Buffer

			err := run(context.Background(), nil, &out, tablesOf("x", "y"), apiOf(api))
			require.NoError(t, err)
			assert.Equal(t, []string{"x", "y"}, api.deleted)
		})

		t.Run("helpは何も削除せずに正常終了する", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("x")

			require.NoError(t, run(context.Background(), []string{"-help"}, &bytes.Buffer{}, tablesOf("x"), apiOf(api)))
			assert.Empty(t, api.deleted, "使い方を表示しただけで table を消してはならない")
		})

		t.Run("timeoutの指定がdeleteTablesへ届く", func(t *testing.T) {
			t.Parallel()

			api := newFakeTableAPI("x")
			api.describeLeft = 1 << 30

			err := run(
				context.Background(), []string{"-timeout", "10ms"}, &bytes.Buffer{}, tablesOf("x"), apiOf(api),
			)
			require.ErrorIs(t, err, errGone)
			require.ErrorIs(t, err, context.DeadlineExceeded, "打ち切ったのは -timeout であって呼び出し側の cancel ではない")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("endpointの拒否はクライアント生成より先に効く", func(t *testing.T) {
			t.Parallel()

			called := false
			factory := func(context.Context, options) (tableAPI, error) {
				called = true

				return newFakeTableAPI(), nil
			}

			err := run(context.Background(), []string{"-endpoint", ""}, &bytes.Buffer{}, tablesOf("x"), factory)
			require.ErrorIs(t, err, errEndpoint)
			assert.False(t, called, "削除は取り消せないので、接続先を組み立てる前に止まらなければならない")
		})

		t.Run("未知のflagはErrHelpと区別して返す", func(t *testing.T) {
			t.Parallel()

			err := run(context.Background(), []string{"-nope"}, &bytes.Buffer{}, tablesOf("x"), apiOf(newFakeTableAPI()))
			require.Error(t, err)
			require.NotErrorIs(t, err, flag.ErrHelp)
		})

		t.Run("table名を解決できなければ削除に進まない", func(t *testing.T) {
			t.Parallel()

			called := false
			factory := func(context.Context, options) (tableAPI, error) {
				called = true

				return newFakeTableAPI(), nil
			}
			resolver := func() ([]string, error) { return nil, errAPI }

			require.ErrorIs(t, run(context.Background(), nil, &bytes.Buffer{}, resolver, factory), errAPI)
			assert.False(t, called)
		})

		t.Run("クライアントを組み立てられなければそのエラーを返す", func(t *testing.T) {
			t.Parallel()

			factory := func(context.Context, options) (tableAPI, error) { return nil, errAPI }

			require.ErrorIs(t, run(context.Background(), nil, &bytes.Buffer{}, tablesOf("x"), factory), errAPI)
		})
	})
}

func Test_configuredTables(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("設定のsuffixから3つのtable名を組み立てる", func(t *testing.T) {
			t.Parallel()

			got, err := configuredTables()
			require.NoError(t, err)
			require.Len(t, got, 3, "EventLog / StreamTicket / InstanceLease の 3 つ")
			for _, name := range got {
				assert.Contains(t, name, "realtime_", "Realtime Delivery の table だけを対象にする")
			}
		})
	})
}

func Test_newClient(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("指定したendpointとregionがクライアントへ届く", func(t *testing.T) {
			t.Parallel()

			c, err := newClient(t.Context(), options{endpoint: "http://localhost:8000", region: "ap-northeast-1"})
			require.NoError(t, err)
			assert.Equal(t, "http://localhost:8000", aws.ToString(c.Options().BaseEndpoint),
				"endpoint が届かないと SDK 既定の解決で本番 DynamoDB を指す")
			assert.Equal(t, "ap-northeast-1", c.Options().Region)
		})
	})
}

func Test_parseOptions(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既定値", func(t *testing.T) {
			t.Parallel()

			opts, err := parseOptions(nil)
			require.NoError(t, err)
			assert.Equal(t, defaultEndpoint, opts.endpoint)
			assert.Equal(t, defaultRegion, opts.region)
			assert.Equal(t, defaultTimeout, opts.timeout)
		})

		t.Run("指定した値で上書きされる", func(t *testing.T) {
			t.Parallel()

			opts, err := parseOptions([]string{
				"-endpoint", "http://127.0.0.1:9000", "-region", "ap-northeast-1", "-timeout", "5s",
			})
			require.NoError(t, err)
			assert.Equal(t, "http://127.0.0.1:9000", opts.endpoint)
			assert.Equal(t, "ap-northeast-1", opts.region)
			assert.Equal(t, 5*time.Second, opts.timeout)
		})

		t.Run("helpはErrHelpを返す", func(t *testing.T) {
			t.Parallel()

			_, err := parseOptions([]string{"-help"})
			require.ErrorIs(t, err, flag.ErrHelp)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("空のendpointは拒否される", func(t *testing.T) {
			t.Parallel()

			_, err := parseOptions([]string{"-endpoint", ""})
			require.ErrorIs(t, err, errEndpoint, "空だとSDK既定の解決で本番DynamoDBを消し得る")
		})

		t.Run("未知のflagはエラーになる", func(t *testing.T) {
			t.Parallel()

			_, err := parseOptions([]string{"-nope"})
			require.Error(t, err)
			require.NotErrorIs(t, err, flag.ErrHelp)
		})
	})
}

func Test_validateEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("httpを受け入れる", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateEndpoint("http://localhost:8000"))
		})

		t.Run("httpsを受け入れる", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateEndpoint("https://dynamodb.example.test"))
		})

		t.Run("自前ホストのemulatorを受け入れる", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateEndpoint("http://dynamo.internal.example:8000"),
				"loopback に限定すると遠隔の emulator を使う構成が壊れる")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("空文字を拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateEndpoint(""), errEndpoint, "SDK 既定の解決へ落とさない")
		})

		t.Run("schemeを省いたものを拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateEndpoint("localhost:8000"), errEndpoint)
		})

		t.Run("hostが空のものを拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateEndpoint("http://"), errEndpoint)
		})

		t.Run("パスだけのものを拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateEndpoint("/tmp/socket"), errEndpoint)
		})

		t.Run("http以外のschemeを拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateEndpoint("ftp://localhost:8000"), errEndpoint)
		})

		t.Run("URLとして解釈できないものを拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateEndpoint("http://local host:8000"), errEndpoint)
		})

		t.Run("実AWSのhostを拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t,
				validateEndpoint("https://dynamodb.ap-northeast-1.amazonaws.com"), errRealAWS,
				"静的資格情報が将来外れても二重に止まる")
		})

		t.Run("大文字で書いた実AWSのhostも拒否する", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, validateEndpoint("https://DynamoDB.AmazonAWS.CoM"), errRealAWS)
		})
	})
}

// tablesOf は、固定の table 名を返す tableResolver を作ります。
func tablesOf(names ...string) tableResolver {
	return func() ([]string, error) { return names, nil }
}

// apiOf は、渡した tableAPI を返す apiFactory を作ります。
func apiOf(api tableAPI) apiFactory {
	return func(context.Context, options) (tableAPI, error) { return api, nil }
}
