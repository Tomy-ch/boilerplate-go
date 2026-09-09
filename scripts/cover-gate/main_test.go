package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coverOutput は、`go tool cover -func` の出力形式（関数ごとの行＋末尾の total 行）を模したものです。
const coverOutput = `go-boilerplate/internal/domain/user/user.go:12:	New		100.0%
go-boilerplate/internal/domain/user/user.go:30:	Rename		75.0%
total:						(statements)	87.5%
`

func Test_parseTotal(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("total 行の最終フィールドからパーセント値を取り出す", func(t *testing.T) {
			t.Parallel()

			got, err := parseTotal(coverOutput)
			require.NoError(t, err)
			assert.InDelta(t, 87.5, got, 0.001)
		})

		t.Run("末尾に改行が無くても取り出せる", func(t *testing.T) {
			t.Parallel()

			got, err := parseTotal("total:\t(statements)\t90.0%")
			require.NoError(t, err)
			assert.InDelta(t, 90.0, got, 0.001)
		})

		t.Run("関数名が total で始まる行は総カバレッジ行と取り違えない", func(t *testing.T) {
			t.Parallel()

			out := "pkg/x/total.go:1:\ttotalize\t10.0%\ntotal:\t(statements)\t42.0%\n"

			got, err := parseTotal(out)
			require.NoError(t, err)
			assert.InDelta(t, 42.0, got, 0.001)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("total 行が無ければエラーにする", func(t *testing.T) {
			t.Parallel()

			_, err := parseTotal("go-boilerplate/internal/domain/user/user.go:12:\tNew\t100.0%\n")
			require.ErrorIs(t, err, errNoTotalLine)
			assert.Contains(t, err.Error(), "no total line")
		})

		t.Run("出力が空ならエラーにする", func(t *testing.T) {
			t.Parallel()

			_, err := parseTotal("")
			require.ErrorIs(t, err, errNoTotalLine)
			assert.Contains(t, err.Error(), "no total line")
		})

		t.Run("total 行にパーセントが無ければエラーにする", func(t *testing.T) {
			t.Parallel()

			_, err := parseTotal("total:\t(statements)\n")
			require.ErrorIs(t, err, errNoTotalLine)
			assert.Contains(t, err.Error(), "not a percentage")
		})

		t.Run("パーセント表記が数値でなければエラーにする", func(t *testing.T) {
			t.Parallel()

			_, err := parseTotal("total:\t(statements)\tn/a%\n")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid total coverage")
		})
	})
}

func Test_judge(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("しきい値を超えていれば合格にする", func(t *testing.T) {
			t.Parallel()

			message, ok := judge(92.34, 90)
			assert.True(t, ok)
			assert.Equal(t, "✅ 総カバレッジ 92.3% (しきい値 90%)", message)
		})

		t.Run("しきい値ちょうどは合格にする", func(t *testing.T) {
			t.Parallel()

			message, ok := judge(90.0, 90)
			assert.True(t, ok)
			assert.Contains(t, message, "✅")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("しきい値を下回れば不合格にする", func(t *testing.T) {
			t.Parallel()

			message, ok := judge(89.9, 90)
			assert.False(t, ok)
			assert.Equal(t, "❌ 総カバレッジ 89.9% がしきい値 90% を下回っています", message)
		})

		t.Run("カバレッジ 0% も不合格にする", func(t *testing.T) {
			t.Parallel()

			_, ok := judge(0, 90)
			assert.False(t, ok)
		})
	})
}

func Test_annotate(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("GitHub Actions では警告アノテーションとして出す", func(t *testing.T) {
			t.Parallel()

			got := annotate("❌ 総カバレッジ 46.9% がしきい値 70% を下回っています", true)

			assert.Equal(t, "::warning::❌ 総カバレッジ 46.9% がしきい値 70% を下回っています", got)
		})

		t.Run("GitHub Actions でなければ文言をそのまま返す", func(t *testing.T) {
			t.Parallel()

			got := annotate("❌ 総カバレッジ 46.9% がしきい値 70% を下回っています", false)

			assert.Equal(t, "❌ 総カバレッジ 46.9% がしきい値 70% を下回っています", got)
		})
	})
}

// stubTotal は、固定の総カバレッジを返す取得手段です。
func stubTotal(value float64) func(string) (float64, error) {
	return func(string) (float64, error) { return value, nil }
}

// writeProfile は、存在だけを満たす空のプロファイルを置いてそのパスを返します。
func writeProfile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "coverage.out")
	require.NoError(t, os.WriteFile(path, []byte("mode: set\n"), 0o600))

	return path
}

// captureLog は、標準ロガーの出力先をバッファへ差し替え、その内容を読む関数を返します。
// 出力先はプロセス共通のため、使うテストは並列化できません。
func captureLog(t *testing.T) func() string {
	t.Helper()

	var buf bytes.Buffer

	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	return buf.String
}

func Test_run(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("しきい値以上なら合格として終える", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"-profile", writeProfile(t), "-threshold", "90"}, stubTotal(90))

			require.NoError(t, err)
		})

		//nolint:paralleltest // 標準ロガーの出力先を差し替えるため並列化できない
		t.Run("警告モードは下限を割っても失敗させない", func(t *testing.T) {
			logged := captureLog(t)

			err := run([]string{"-profile", writeProfile(t), "-threshold", "90", "-warn"}, stubTotal(46.9))

			require.NoError(t, err)
			assert.NotContains(t, logged(), "::warning::")
		})

		//nolint:paralleltest // 標準ロガーの出力先を差し替えるため並列化できない
		t.Run("github 指定の警告は ::warning:: アノテーションで出す", func(t *testing.T) {
			logged := captureLog(t)

			err := run([]string{"-profile", writeProfile(t), "-threshold", "90", "-warn", "-github"}, stubTotal(46.9))

			require.NoError(t, err)
			assert.Contains(t, logged(), "::warning::")
		})

		//nolint:paralleltest // 標準ロガーの出力先を差し替えるため並列化できない
		t.Run("合格時は github 指定でもアノテーションを付けない", func(t *testing.T) {
			logged := captureLog(t)

			err := run([]string{"-profile", writeProfile(t), "-threshold", "90", "-warn", "-github"}, stubTotal(90))

			require.NoError(t, err)
			assert.NotContains(t, logged(), "::warning::")
		})

		t.Run("ヘルプ要求は失敗にしない", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, run([]string{"-h"}, stubTotal(0)))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("警告モードでなければ下限割れを失敗にする", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"-profile", writeProfile(t), "-threshold", "90"}, stubTotal(89.9))

			require.ErrorIs(t, err, errBelowThreshold)
			assert.ErrorContains(t, err, "下回っています")
		})

		t.Run("プロファイルが無ければ生成手順を添えて失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"-profile", filepath.Join(t.TempDir(), "absent.out")}, stubTotal(100))

			require.ErrorContains(t, err, "make go-test-cover-ci")
		})

		t.Run("総カバレッジを取得できなければ失敗する", func(t *testing.T) {
			t.Parallel()

			failing := func(string) (float64, error) { return 0, errNoTotalLine }

			err := run([]string{"-profile", writeProfile(t)}, failing)

			require.ErrorIs(t, err, errNoTotalLine)
		})

		t.Run("未知のフラグはヘルプ要求と混同せず失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"-bogus"}, stubTotal(100))

			require.ErrorContains(t, err, "failed to parse flags")
		})
	})
}

func Test_coverTotal(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("プロファイルから総カバレッジを取り出す", func(t *testing.T) {
			t.Parallel()

			// `go tool cover` はプロファイルが指すソースを実際に開くため、実在する
			// パッケージの実在する範囲を指す必要がある。テストの cwd は自パッケージ。
			path := filepath.Join(t.TempDir(), "coverage.out")
			body := "mode: set\ngo-boilerplate/scripts/cover-gate/main.go:97.44,99.2 1 1\n"
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

			got, err := coverTotal(path)

			require.NoError(t, err)
			assert.InDelta(t, 100.0, got, 0.01)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("プロファイルとして読めなければエラーを返す", func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "broken.out")
			require.NoError(t, os.WriteFile(path, []byte("not a profile\n"), 0o600))

			_, err := coverTotal(path)

			require.ErrorContains(t, err, "go tool cover")
		})
	})
}
