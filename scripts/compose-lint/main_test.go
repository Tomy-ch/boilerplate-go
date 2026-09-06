package main

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// composeWithHealthcheck は、api_server が healthcheck を持つ compose ファイルの本文です。
const composeWithHealthcheck = "services:\n  api_server:\n    healthcheck:\n      test: [\"CMD\", \"true\"]\n"

// composeWithoutHealthcheck は、api_server が healthcheck を持たない compose ファイルの本文です。
const composeWithoutHealthcheck = "services:\n  api_server:\n    image: x\n"

// appServicesMk は、api_server だけを起動対象に挙げる compose.mk の本文です。
const appServicesMk = "APP_SERVICES ?= api_server\n"

func Test_run(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("起動対象がすべて healthcheck を持てば成功する", func(t *testing.T) {
			t.Parallel()

			var out strings.Builder
			err := run([]string{
				"-compose", writeFile(t, "docker-compose.yaml", composeWithHealthcheck),
				"-makefile", writeFile(t, "compose.mk", appServicesMk),
			}, &out)

			require.NoError(t, err)
			assert.Contains(t, out.String(), "1")
		})

		t.Run("ヘルプ要求は失敗にしない", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"-h"}, io.Discard)

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("healthcheck を持たない起動対象があれば違反として返す", func(t *testing.T) {
			t.Parallel()

			err := run([]string{
				"-compose", writeFile(t, "docker-compose.yaml", composeWithoutHealthcheck),
				"-makefile", writeFile(t, "compose.mk", appServicesMk),
			}, io.Discard)

			require.ErrorIs(t, err, errViolation)
		})

		t.Run("compose ファイルが読めなければ違反ゼロと報告せず失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{
				"-compose", filepath.Join(t.TempDir(), "absent.yaml"),
				"-makefile", writeFile(t, "compose.mk", appServicesMk),
			}, io.Discard)

			require.Error(t, err)
			assert.NotErrorIs(t, err, errViolation)
		})

		t.Run("起動対象の宣言が読めなければ違反ゼロと報告せず失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{
				"-compose", writeFile(t, "docker-compose.yaml", composeWithHealthcheck),
				"-makefile", writeFile(t, "compose.mk", "INFRA_SERVICES ?= database\n"),
			}, io.Discard)

			require.ErrorIs(t, err, errNoAppServices)
		})

		t.Run("未知のフラグはエラーにする", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"-nope"}, io.Discard)

			require.Error(t, err)
		})
	})
}
