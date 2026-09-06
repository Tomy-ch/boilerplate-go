package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// writeFile は、テスト用の一時ファイルを作成しそのパスを返します。
func writeFile(t *testing.T, name, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	return path
}

// testHealthcheck は、test に指定の値を持つ healthcheck を組み立てます。
func testHealthcheck(t *testing.T, body string) *healthcheck {
	t.Helper()

	var hc healthcheck
	require.NoError(t, yaml.Unmarshal([]byte(body), &hc))

	return &hc
}

func Test_loadServices(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("services のサービス名を読み出す", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "docker-compose.yaml", "services:\n  api_server:\n    image: x\n  db:\n    image: y\n")

			actual, err := loadServices(path)

			require.NoError(t, err)
			assert.Len(t, actual, 2)
			assert.Contains(t, actual, "api_server")
			assert.Contains(t, actual, "db")
		})

		t.Run("healthcheck の宣言の有無を読み分ける", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "docker-compose.yaml",
				"services:\n  a:\n    healthcheck:\n      test: [\"CMD\", \"true\"]\n  b:\n    image: y\n")

			actual, err := loadServices(path)

			require.NoError(t, err)
			require.NotNil(t, actual["a"].Healthcheck)
			assert.False(t, actual["a"].Healthcheck.Test.IsZero())
			assert.Nil(t, actual["b"].Healthcheck)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("読めないファイルは 0 件に寄せずエラーにする", func(t *testing.T) {
			t.Parallel()

			_, err := loadServices(filepath.Join(t.TempDir(), "absent.yaml"))

			require.Error(t, err)
		})

		t.Run("YAML として壊れたファイルは 0 件に寄せずエラーにする", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "docker-compose.yaml", "services:\n  - [\n")

			_, err := loadServices(path)

			require.Error(t, err)
		})

		t.Run("services が空なら成功と報告せずエラーにする", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "docker-compose.yaml", "volumes:\n  pg_data:\n")

			_, err := loadServices(path)

			require.ErrorIs(t, err, errNoService)
		})
	})
}

func Test_loadAppServices(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("?= の宣言からサービス名を読み出す", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "compose.mk", "INFRA_SERVICES ?= database\nAPP_SERVICES ?= api_server mock_auth_server\n")

			actual, err := loadAppServices(path)

			require.NoError(t, err)
			assert.Equal(t, []string{"api_server", "mock_auth_server"}, actual)
		})

		t.Run("= と := の宣言も読む", func(t *testing.T) {
			t.Parallel()

			for _, op := range []string{"=", ":="} {
				path := writeFile(t, "compose.mk", "APP_SERVICES "+op+" api_server\n")

				actual, err := loadAppServices(path)

				require.NoError(t, err, op)
				assert.Equal(t, []string{"api_server"}, actual, op)
			}
		})

		t.Run("前方一致する別の変数を APP_SERVICES と取り違えない", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "compose.mk", "APP_SERVICES_EXTRA ?= ignored\nAPP_SERVICES ?= api_server\n")

			actual, err := loadAppServices(path)

			require.NoError(t, err)
			assert.Equal(t, []string{"api_server"}, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("読めないファイルはエラーにする", func(t *testing.T) {
			t.Parallel()

			_, err := loadAppServices(filepath.Join(t.TempDir(), "absent.mk"))

			require.Error(t, err)
		})

		t.Run("宣言が無ければ空の一覧で走らせずエラーにする", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "compose.mk", "INFRA_SERVICES ?= database\n")

			_, err := loadAppServices(path)

			require.ErrorIs(t, err, errNoAppServices)
		})

		t.Run("宣言はあるが右辺が空ならエラーにする", func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, "compose.mk", "APP_SERVICES ?=\n")

			_, err := loadAppServices(path)

			require.ErrorIs(t, err, errNoAppServices)
		})
	})
}

func Test_appServicesValue(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("代入なら右辺を返す", func(t *testing.T) {
			t.Parallel()

			actual, ok := appServicesValue("APP_SERVICES ?= a b\n")

			assert.True(t, ok)
			assert.Equal(t, "a b", actual)
		})

		t.Run("行頭に空白があっても読む", func(t *testing.T) {
			t.Parallel()

			actual, ok := appServicesValue("  APP_SERVICES ?= a\n")

			assert.True(t, ok)
			assert.Equal(t, "a", actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("代入でない行は拾わない", func(t *testing.T) {
			t.Parallel()

			_, ok := appServicesValue("# APP_SERVICES を参照するコメント\n")

			assert.False(t, ok)
		})

		t.Run("参照は代入と区別する", func(t *testing.T) {
			t.Parallel()

			_, ok := appServicesValue("\t@$(COMPOSE_APP) up -d $(APP_SERVICES)\n")

			assert.False(t, ok)
		})
	})
}

func Test_checkHealthcheck(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全サービスが healthcheck を持てば報告しない", func(t *testing.T) {
			t.Parallel()

			services := map[string]service{
				"api_server": {Healthcheck: testHealthcheck(t, "test: [\"CMD\", \"true\"]")},
			}

			actual, err := checkHealthcheck([]string{"api_server"}, services)

			require.NoError(t, err)
			assert.Empty(t, actual)
		})

		t.Run("起動対象でないサービスの healthcheck は問わない", func(t *testing.T) {
			t.Parallel()

			services := map[string]service{
				"api_server": {Healthcheck: testHealthcheck(t, "test: [\"CMD\", \"true\"]")},
				"one_shot":   {},
			}

			actual, err := checkHealthcheck([]string{"api_server"}, services)

			require.NoError(t, err)
			assert.Empty(t, actual)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("healthcheck を持たないサービスを名指しで報告する", func(t *testing.T) {
			t.Parallel()

			services := map[string]service{"api_server": {}}

			actual, err := checkHealthcheck([]string{"api_server"}, services)

			require.NoError(t, err)
			assert.Contains(t, actual, "api_server")
		})

		t.Run("healthcheck はあるが test が空なら宣言が無いものとして報告する", func(t *testing.T) {
			t.Parallel()

			services := map[string]service{"api_server": {Healthcheck: &healthcheck{}}}

			actual, err := checkHealthcheck([]string{"api_server"}, services)

			require.NoError(t, err)
			assert.Contains(t, actual, "api_server")
		})

		t.Run("違反が複数あればすべて名前順で挙げる", func(t *testing.T) {
			t.Parallel()

			services := map[string]service{"b_server": {}, "a_server": {}}

			actual, err := checkHealthcheck([]string{"b_server", "a_server"}, services)

			require.NoError(t, err)
			assert.Contains(t, actual, "a_server b_server")
		})

		t.Run("起動対象が compose に無ければ違反ではなくエラーにする", func(t *testing.T) {
			t.Parallel()

			services := map[string]service{"api_server": {Healthcheck: testHealthcheck(t, "test: [\"CMD\", \"true\"]")}}

			_, err := checkHealthcheck([]string{"renamed_server"}, services)

			require.ErrorIs(t, err, errUnknownService)
		})
	})
}
