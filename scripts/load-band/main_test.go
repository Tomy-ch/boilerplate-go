package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// worktreeListOutput は、`git worktree list` の出力形式を模したものです。
const worktreeListOutput = `/Users/x/dev/src/go-boilerplate  86dcb1385 [release/v2.2.0]
/Users/x/dev/src/gobp-wt-1       40c1f5b77 [feature/a]
/Users/x/dev/src/gobp-wt-2       ce8c6d411 [feature/b]
`

// testCPUs は、run の検証で使う CPU 数。帯の判定には効かないため固定します。
const testCPUs = 8

// mustResolve は、エラーにならない前提の解決結果を返します。
func mustResolve(t *testing.T, requested string, windows, cpus int) band {
	t.Helper()

	b, err := resolve(requested, windows, cpus, defaultLowThreshold, defaultCIFirstThreshold)
	require.NoError(t, err)

	return b
}

func Test_countWindows(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("worktree の行数を数える", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 3, countWindows(worktreeListOutput))
		})

		t.Run("末尾に改行が無くても数える", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 1, countWindows("/Users/x/dev/src/go-boilerplate  86dcb1385 [release/v2.2.0]"))
		})

		t.Run("空行は数えない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 2, countWindows("/a  1 [x]\n\n/b  2 [y]\n\n"))
		})

		t.Run("git が使えず出力が空でも 1 窓として扱う", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 1, countWindows(""))
		})
	})
}

func Test_resolve(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("auto は閾値未満の窓数で full を選ぶ", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandFull, mustResolve(t, bandAuto, 2, 8).resolved)
		})

		t.Run("auto は low の閾値ちょうどで low を選ぶ", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandLow, mustResolve(t, bandAuto, 3, 8).resolved)
		})

		t.Run("auto は ci-first の閾値ちょうどで ci-first を選ぶ", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandCIFirst, mustResolve(t, bandAuto, 5, 8).resolved)
		})

		t.Run("空文字は auto として扱う", func(t *testing.T) {
			t.Parallel()

			b := mustResolve(t, "", 5, 8)
			assert.Equal(t, bandCIFirst, b.resolved)
			assert.Equal(t, bandAuto, b.requested)
		})

		t.Run("明示指定は窓数より優先する", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandFull, mustResolve(t, bandFull, 9, 8).resolved)
			assert.Equal(t, bandLow, mustResolve(t, bandLow, 1, 8).resolved)
			assert.Equal(t, bandCIFirst, mustResolve(t, bandCIFirst, 1, 8).resolved)
		})

		t.Run("full では share を CPU 数のまま渡す", func(t *testing.T) {
			t.Parallel()

			b := mustResolve(t, bandFull, 4, 8)
			assert.Equal(t, 8, b.share)
			assert.False(t, b.throttled())
		})

		t.Run("絞る帯では share を窓数で割る", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 2, mustResolve(t, bandAuto, 4, 8).share)
		})

		t.Run("窓数が CPU 数を超えても share は 1 を下回らない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 1, mustResolve(t, bandAuto, 20, 8).share)
		})

		t.Run("CPU 数を取得できず 0 でも 1 として扱う", func(t *testing.T) {
			t.Parallel()

			b := mustResolve(t, bandFull, 1, 0)
			assert.Equal(t, 1, b.cpus)
			assert.Equal(t, 1, b.share)
		})

		t.Run("窓数が 0 でも 1 として扱い share を CPU 数のまま渡す", func(t *testing.T) {
			t.Parallel()

			b := mustResolve(t, bandLow, 0, 8)
			assert.Equal(t, 1, b.windows)
			assert.Equal(t, 8, b.share)
		})

		t.Run("窓数が 0 のとき auto は下限の 1 窓として帯を決める", func(t *testing.T) {
			t.Parallel()

			b, err := resolve(bandAuto, 0, 8, 1, 5)
			require.NoError(t, err)
			assert.Equal(t, bandLow, b.resolved)
			assert.Equal(t, 1, b.windows)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未知の帯名はエラーにする", func(t *testing.T) {
			t.Parallel()

			_, err := resolve("ci_first", 1, 8, defaultLowThreshold, defaultCIFirstThreshold)
			require.ErrorIs(t, err, errUnknownBand)
			assert.Contains(t, err.Error(), "GOBP_LOAD=ci_first")
		})
	})
}

func Test_renderEnv(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("絞る帯では各ツールへ渡す並列度を埋める", func(t *testing.T) {
			t.Parallel()

			got := renderEnv(mustResolve(t, bandAuto, 4, 8))

			assert.Contains(t, got, "GOBP_LOAD_RESOLVED='low'\n")
			assert.Contains(t, got, "GOBP_WINDOWS='4'\n")
			assert.Contains(t, got, "GOBP_CPUS='8'\n")
			assert.Contains(t, got, "GOBP_SHARE='2'\n")
			assert.Contains(t, got, "GOBP_THROTTLED='1'\n")
			assert.Contains(t, got, "GOBP_NICE='nice -n 10'\n")
			assert.Contains(t, got, "GOLANGCI_CONCURRENCY_FLAG='--concurrency 2'\n")
			assert.Contains(t, got, "GO_TEST_P_FLAG='-p 2'\n")
			assert.Contains(t, got, "GO_TEST_LOAD_ENV='GOMAXPROCS=2'\n")
		})

		t.Run("full では並列度のフラグを空にしてツールの既定へ委ねる", func(t *testing.T) {
			t.Parallel()

			got := renderEnv(mustResolve(t, bandFull, 1, 8))

			assert.Contains(t, got, "GOBP_THROTTLED=''\n")
			assert.Contains(t, got, "GOBP_NICE=''\n")
			assert.Contains(t, got, "GOLANGCI_CONCURRENCY_FLAG=''\n")
			assert.Contains(t, got, "GO_TEST_P_FLAG=''\n")
			assert.Contains(t, got, "GO_TEST_LOAD_ENV=''\n")
		})

		t.Run("値は eval できるようクォートし 1 行 1 変数で出す", func(t *testing.T) {
			t.Parallel()

			got := renderEnv(mustResolve(t, bandLow, 4, 8))

			for line := range strings.Lines(got) {
				assert.Regexp(t, `^[A-Z_]+='[^']*'\n$`, line)
			}
		})

		t.Run("git が使えなくても窓数は整数 1 として出力する", func(t *testing.T) {
			t.Parallel()

			b := mustResolve(t, bandAuto, countWindows(""), 8)

			assert.Contains(t, renderEnv(b), "GOBP_WINDOWS='1'\n")
			assert.Equal(t, bandFull, b.resolved)
		})
	})
}

func Test_renderStatus(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("絞る帯では解決結果と ci-first への案内を出す", func(t *testing.T) {
			t.Parallel()

			got := renderStatus(mustResolve(t, bandAuto, 4, 8))

			assert.Contains(t, got, "load      : low  (GOBP_LOAD=auto)\n")
			assert.Contains(t, got, "windows   : 4 worktree  (low >= 3, ci-first >= 5)\n")
			assert.Contains(t, got, "cpus      : 8  ->  share 2 / 窓\n")
			assert.Contains(t, got, "golangci  : --concurrency 2\n")
			assert.Contains(t, got, "go test   : -p 2 GOMAXPROCS=2\n")
			assert.Contains(t, got, "nice      : nice -n 10\n")
			assert.Contains(t, got, "GOBP_LOAD=ci-first")
		})

		t.Run("full では委譲先を明示する", func(t *testing.T) {
			t.Parallel()

			got := renderStatus(mustResolve(t, bandFull, 1, 8))

			assert.Contains(t, got, "golangci  : 設定ファイルの concurrency に委譲\n")
			assert.Contains(t, got, "go test   : 既定（ホスト全体）\n")
			assert.Contains(t, got, "nice      : なし\n")
			assert.Contains(t, got, "💡 窓が少ないためホスト全体を使います")
		})

		t.Run("ci-first では手元で回す方法を案内する", func(t *testing.T) {
			t.Parallel()

			got := renderStatus(mustResolve(t, bandAuto, 5, 8))

			assert.Contains(t, got, "💡 窓が多いため CI-first です。")
			assert.Contains(t, got, "make go-lint GOBP_LOAD=low")
		})
	})
}

//nolint:paralleltest // t.Chdir はプロセス全体の作業ディレクトリを変えるため並列化できない
func Test_worktreeList(t *testing.T) {
	t.Run("正常系", func(t *testing.T) {
		t.Run("リポジトリの中では窓を数えられる出力を返す", func(t *testing.T) {
			dir := t.TempDir()
			//nolint:gosec // 引数はテスト内で組み立てた一時ディレクトリのパスと固定値
			out, err := exec.CommandContext(t.Context(), "git", "-C", dir, "init", "--quiet").CombinedOutput()
			require.NoError(t, err, string(out))
			t.Chdir(dir)

			assert.Equal(t, 1, countWindows(worktreeList()))
		})

		t.Run("git が使えない場所では部分的な出力を返さず空にする", func(t *testing.T) {
			t.Chdir(t.TempDir())

			assert.Empty(t, worktreeList())
		})
	})
}

func Test_bandFor(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("窓が low の閾値未満なら full", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandFull, bandFor(2, 3, 5))
		})

		t.Run("窓が low の閾値ちょうどなら low", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandLow, bandFor(3, 3, 5))
		})

		t.Run("窓が ci-first の閾値ちょうどなら ci-first", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandCIFirst, bandFor(5, 3, 5))
		})

		t.Run("閾値は既定値ではなく引数の値で判定する", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandFull, bandFor(9, 10, 20))
			assert.Equal(t, bandLow, bandFor(10, 10, 20))
		})

		t.Run("閾値が同じなら重い側の ci-first を選ぶ", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, bandCIFirst, bandFor(5, 5, 5))
		})
	})
}

func Test_band_throttled(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("low は絞る帯", func(t *testing.T) {
			t.Parallel()

			assert.True(t, band{resolved: bandLow}.throttled())
		})

		t.Run("ci-first も絞る帯（軽いゲートは手元に残るため）", func(t *testing.T) {
			t.Parallel()

			assert.True(t, band{resolved: bandCIFirst}.throttled())
		})

		t.Run("full は絞らない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, band{resolved: bandFull}.throttled())
		})
	})
}

func Test_throttledFlag(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("絞る帯では 1 を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "1", throttledFlag(band{resolved: bandLow}))
		})

		t.Run("絞らない帯ではシェルの -n 判定が偽になるよう空にする", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, throttledFlag(band{resolved: bandFull}))
		})
	})
}

func Test_nice(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("絞る帯では優先度を下げる指定を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "nice -n 10", nice(band{resolved: bandCIFirst}))
		})

		t.Run("絞らない帯では優先度を変えない", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, nice(band{resolved: bandFull}))
		})
	})
}

func Test_golangciConcurrencyFlag(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("絞る帯では share を並列度として渡す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "--concurrency 3", golangciConcurrencyFlag(band{resolved: bandLow, share: 3}))
		})

		t.Run("絞らない帯では設定ファイルの concurrency へ委ねるため空にする", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, golangciConcurrencyFlag(band{resolved: bandFull, share: 8}))
		})
	})
}

func Test_goTestPFlag(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("絞る帯では share をパッケージ同時実行数として渡す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "-p 3", goTestPFlag(band{resolved: bandLow, share: 3}))
		})

		t.Run("絞らない帯では go test の既定へ委ねるため空にする", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, goTestPFlag(band{resolved: bandFull, share: 8}))
		})
	})
}

func Test_goTestLoadEnv(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("絞る帯ではテストバイナリ内部の並列度も share に揃える", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "GOMAXPROCS=3", goTestLoadEnv(band{resolved: bandLow, share: 3}))
		})

		t.Run("絞らない帯では環境変数を足さない", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, goTestLoadEnv(band{resolved: bandFull, share: 8}))
		})
	})
}

func Test_shellQuote(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値をシングルクォートで囲む", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "'nice -n 10'", shellQuote("nice -n 10"))
		})

		t.Run("値にシングルクォートが含まれてもクォートを閉じ直して渡す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, `'it'\''s'`, shellQuote("it's"))
		})

		t.Run("空の値でも代入形が壊れないよう空のクォートを返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "''", shellQuote(""))
		})
	})
}

func Test_advice(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ci-first では重いゲートを CI へ委ねる案内を出す", func(t *testing.T) {
			t.Parallel()

			got := advice(band{resolved: bandCIFirst})

			assert.Contains(t, got, "CI-first")
			assert.Contains(t, got, "make go-lint GOBP_LOAD=low")
		})

		t.Run("low では CPU 数ではなく share を案内に埋め込む", func(t *testing.T) {
			t.Parallel()

			got := advice(band{resolved: bandLow, cpus: 8, share: 3})

			assert.Contains(t, got, "CPU share 3")
			assert.Contains(t, got, "GOBP_LOAD=ci-first")
		})

		t.Run("full ではホスト全体を使うと伝える", func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, advice(band{resolved: bandFull}), "ホスト全体")
		})
	})
}

func Test_orElse(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値があればそのまま返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "-p 2", orElse("-p 2", "既定（ホスト全体）"))
		})

		t.Run("値が空なら代替を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "既定（ホスト全体）", orElse("", "既定（ホスト全体）"))
		})

		t.Run("空白だけの値は空とみなさずそのまま返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, " ", orElse(" ", "既定（ホスト全体）"))
		})
	})
}

// runOutput は、run を成功させたうえで書き出された内容を返します。
// CPU 数は解決の入力でしかないため、帯の判定に関わる窓の数だけを受けます。
func runOutput(t *testing.T, args []string, windows int) string {
	t.Helper()

	var buf bytes.Buffer

	require.NoError(t, run(args, &buf, windows, testCPUs))

	return buf.String()
}

func Test_run(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("env は解決結果を KEY=VALUE で書き出す", func(t *testing.T) {
			t.Parallel()

			got := runOutput(t, []string{"env"}, 1)

			assert.Contains(t, got, "GOBP_LOAD_RESOLVED='full'\n")
			assert.Contains(t, got, "GOBP_WINDOWS='1'\n")
			assert.Contains(t, got, "GOBP_CPUS='8'\n")
		})

		t.Run("status は導出に使った値を人間向けに書き出す", func(t *testing.T) {
			t.Parallel()

			got := runOutput(t, []string{"status"}, 1)

			assert.Contains(t, got, "load      : full")
			assert.Contains(t, got, "windows   : 1 worktree")
		})

		t.Run("帯の指定は窓の数より優先される", func(t *testing.T) {
			t.Parallel()

			got := runOutput(t, []string{"env", "--load=ci-first"}, 1)

			assert.Contains(t, got, "GOBP_LOAD_RESOLVED='ci-first'\n")
		})

		t.Run("low へ落とす窓の数はフラグで差し替えられる", func(t *testing.T) {
			t.Parallel()

			got := runOutput(t, []string{"env", "--low=2", "--ci-first=99"}, 2)

			assert.Contains(t, got, "GOBP_LOAD_RESOLVED='low'\n")
		})

		t.Run("ci-first へ落とす窓の数はフラグで差し替えられる", func(t *testing.T) {
			t.Parallel()

			got := runOutput(t, []string{"env", "--low=1", "--ci-first=2"}, 2)

			assert.Contains(t, got, "GOBP_LOAD_RESOLVED='ci-first'\n")
		})

		t.Run("ヘルプ要求は失敗にせず何も書き出さない", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, runOutput(t, []string{"env", "-h"}, 1))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("サブコマンドが無ければ使い方を示して失敗する", func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			require.ErrorIs(t, run(nil, &buf, 1, 8), errUsage)
			assert.Empty(t, buf.String())
		})

		t.Run("未知のサブコマンドでは何も書き出さずに失敗する", func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			require.ErrorIs(t, run([]string{"bogus"}, &buf, 1, 8), errUnknownSubcommand)
			assert.Empty(t, buf.String())
		})

		t.Run("未知の帯名は既定へ縮退させず失敗する", func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			require.ErrorIs(t, run([]string{"env", "--load=bogus"}, &buf, 1, 8), errUnknownBand)
			assert.Empty(t, buf.String())
		})

		t.Run("未知のフラグはヘルプ要求と混同せず失敗する", func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			err := run([]string{"env", "-bogus"}, &buf, 1, 8)

			require.ErrorContains(t, err, "failed to parse flags")
			assert.Empty(t, buf.String())
		})
	})
}
