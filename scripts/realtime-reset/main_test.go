package main

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-boilerplate/pkg/xerrors"
)

func Test_parseOptions(t *testing.T) {
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

		opts, err := parseOptions([]string{"-endpoint", "http://127.0.0.1:9000", "-region", "ap-northeast-1"})
		require.NoError(t, err)
		assert.Equal(t, "http://127.0.0.1:9000", opts.endpoint)
		assert.Equal(t, "ap-northeast-1", opts.region)
	})

	t.Run("空のendpointは拒否される", func(t *testing.T) {
		t.Parallel()

		_, err := parseOptions([]string{"-endpoint", ""})
		require.ErrorIs(t, err, errEndpoint, "空だとSDK既定の解決で本番DynamoDBを消し得る")
	})

	t.Run("helpはErrHelpを返す", func(t *testing.T) {
		t.Parallel()

		_, err := parseOptions([]string{"-help"})
		require.ErrorIs(t, err, flag.ErrHelp)
	})

	t.Run("未知のflagはエラーになる", func(t *testing.T) {
		t.Parallel()

		_, err := parseOptions([]string{"-nope"})
		require.Error(t, err)
		assert.NotErrorIs(t, err, flag.ErrHelp)
	})
}

func Test_validateEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("httpとhttpsを受け入れる", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, validateEndpoint("http://localhost:8000"))
		require.NoError(t, validateEndpoint("https://dynamodb.example.test"))
	})

	t.Run("hostが無いものを拒否する", func(t *testing.T) {
		t.Parallel()

		for _, endpoint := range []string{"", "localhost:8000", "http://", "/tmp/socket"} {
			require.ErrorIs(t, validateEndpoint(endpoint), errEndpoint, endpoint)
		}
	})

	t.Run("http以外のschemeを拒否する", func(t *testing.T) {
		t.Parallel()

		require.ErrorIs(t, validateEndpoint("ftp://localhost:8000"), errEndpoint)
	})

	t.Run("URLとして解釈できないものを拒否する", func(t *testing.T) {
		t.Parallel()

		err := validateEndpoint("http://local host:8000")
		require.ErrorIs(t, err, errEndpoint)
		assert.False(t, xerrors.Is(err, flag.ErrHelp))
	})
}
