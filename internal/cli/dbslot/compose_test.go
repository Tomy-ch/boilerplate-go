package dbslot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDocker は、PATH 先頭に引数と COMPOSE_PROJECT_NAME を記録するダミー docker を仕込み、記録ファイルの
// パスを返す。実 docker を呼ばずに ExecCompose の実装（コマンド構築・環境変数注入）を検証する。
func stubDocker(t *testing.T, exit int) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	script := "#!/bin/sh\necho \"proj=$COMPOSE_PROJECT_NAME args=$*\" >> " + out + "\nexit " + strconv.Itoa(exit) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755)) //nolint:gosec // 実行可能スタブ
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return out
}

// stubDockerStdout は、標準出力に stdout を返すダミー docker を PATH 先頭に仕込む。
func stubDockerStdout(t *testing.T, stdout string, exit int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s' '" + stdout + "'\nexit " + strconv.Itoa(exit) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755)) //nolint:gosec // 実行可能スタブ
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExecCompose_UpSharedDB(t *testing.T) { //nolint:paralleltest // stubDocker が t.Setenv を使うため並列化不可
	out := stubDocker(t, 0)

	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("固定プロジェクトで database profile を up --wait --no-recreate する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			require.NoError(t, ExecCompose{}.UpSharedDB(context.Background(), "myproj"))

			b, err := os.ReadFile(out) //nolint:gosec // テスト内の固定パス
			require.NoError(t, err)
			assert.Contains(t, string(b), "proj=myproj")
			assert.Contains(t, string(b), "compose --profile database up -d --wait --no-recreate database")
		})
	})
}

func TestExecCompose_DownServe(t *testing.T) { //nolint:paralleltest // stubDocker が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("app 層の profile を付け attach override は重ねずに down する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			out := stubDocker(t, 0)

			require.NoError(t, ExecCompose{}.DownServe(context.Background(), "gobp-wt-1"))

			b, err := os.ReadFile(out) //nolint:gosec // テスト内の固定パス
			require.NoError(t, err)
			assert.Contains(t, string(b), "proj=gobp-wt-1")
			// profile を落とすと down は対象ゼロのまま exit 0 で返り、コンテナが残る。
			assert.Contains(t, string(b), "--profile development -f docker-compose.yaml down")
			// override は down が参照しない。重ねると片付け経路が compose の補間の癖に依存する。
			assert.NotContains(t, string(b), "docker-compose.attach.yaml")
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("down が実失敗（docker 非 0 終了）ならエラーを伝播する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			stubDocker(t, 1)

			require.Error(t, ExecCompose{}.DownServe(context.Background(), "gobp-wt-1"))
		})
	})
}

func TestExecCompose_RunningContainers(t *testing.T) { //nolint:paralleltest // stubDocker が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("稼働中コンテナの ID 行数を返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			stubDockerStdout(t, "abc123\ndef456\n", 0)

			n, err := ExecCompose{}.RunningContainers(context.Background(), "gobp-wt-1")

			require.NoError(t, err)
			assert.Equal(t, 2, n)
		})

		t.Run("出力が空なら 0 を返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			stubDockerStdout(t, "\n", 0)

			n, err := ExecCompose{}.RunningContainers(context.Background(), "gobp-wt-1")

			require.NoError(t, err)
			assert.Zero(t, n)
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("docker が失敗すればエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			stubDockerStdout(t, "", 1)

			_, err := ExecCompose{}.RunningContainers(context.Background(), "gobp-wt-1")

			require.Error(t, err)
		})
	})
}

func TestExecCompose_run(t *testing.T) { //nolint:paralleltest // stubDocker が t.Setenv を使うため並列化不可
	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("docker が非 0 終了ならエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			stubDocker(t, 1)

			err := ExecCompose{}.UpSharedDB(context.Background(), "p")
			require.Error(t, err)
		})
	})
}

func Test_newComposeCmd(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("docker compose に引数を連結しプロジェクトを環境変数で与える", func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer

			cmd := newComposeCmd(context.Background(), "gobp-wt-1", &stdout, "ps", "-q", "--status", "running")

			assert.Equal(t, []string{"docker", "compose", "ps", "-q", "--status", "running"}, cmd.Args)
			assert.Contains(t, cmd.Env, "COMPOSE_PROJECT_NAME=gobp-wt-1")
		})

		t.Run("標準出力は引数の writer へ、進捗ログは stderr へ振り分ける", func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer

			cmd := newComposeCmd(context.Background(), "gobp-wt-1", &stdout, "ps")

			assert.Same(t, &stdout, cmd.Stdout)
			assert.Same(t, os.Stderr, cmd.Stderr)
		})
	})
}

func TestExecCompose_output(t *testing.T) { //nolint:paralleltest // stubDockerStdout が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("docker compose の標準出力をそのまま返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			stubDockerStdout(t, "abc123\ndef456\n", 0)

			got, err := ExecCompose{}.output(context.Background(), "gobp-wt-1", "ps", "-q")

			require.NoError(t, err)
			assert.Equal(t, "abc123\ndef456\n", got)
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("docker が非 0 終了なら出力を捨ててエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			stubDockerStdout(t, "partial output", 1)

			got, err := ExecCompose{}.output(context.Background(), "gobp-wt-1", "ps")

			require.Error(t, err)
			assert.Empty(t, got) // 失敗時は途中出力を呼び出し元へ渡さない
		})
	})
}
