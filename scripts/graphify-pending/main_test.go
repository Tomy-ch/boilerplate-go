package main

import (
	"bytes"
	"crypto/md5" //nolint:gosec // production 側と同じ選択で、テストは実装に合わせる
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-boilerplate/pkg/xerrors"
)

// digest は manifest が持つのと同じ形式のハッシュを返します。
func digest(body string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(body))) //nolint:gosec // 変更検出のみ
}

// captureLog は標準ロガーの出力を捕まえます。未処理の内訳は戻り値ではなくログにしか出ないため、
// そこを検査するにはここを通します。
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := log.Writer()
	flags := log.Flags()
	log.SetOutput(buffer)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previous)
		log.SetFlags(flags)
	})

	return buffer
}

// staticRead は内容を固定で返す読み出し手段を作ります。未登録のパスは存在しない扱いです。
func staticRead(bodies map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		body, ok := bodies[name]
		if !ok {
			return nil, os.ErrNotExist
		}

		return []byte(body), nil
	}
}

// staticGit は git のサブコマンドごとに固定の出力を返す実行手段を作ります。
func staticGit(log, diff string) func(...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "log" {
			return log, nil
		}

		return diff, nil
	}
}

// captureLog が標準ロガーという共有状態を差し替えるため、Test_run は並列化しない。
//
//nolint:paralleltest // 標準ロガーを捕まえるため並列化できない
func Test_run(t *testing.T) {
	const (
		extracted = "# 抽出済み\n"
		edited    = "# 編集後\n"
		baseline  = "0123456789abcdef0123456789abcdef01234567"
	)

	// manifest は semantic_hash に extracted のハッシュを持ち、README.md だけが編集済みの状態。
	corpus := map[string]string{
		"graphify-out/manifest.json": fmt.Sprintf(
			`{"README.md":{"ast_hash":"%[1]s","semantic_hash":"%[1]s"},`+
				`"docs/architecture.md":{"ast_hash":"%[1]s","semantic_hash":"%[1]s"}}`,
			digest(extracted),
		),
		"README.md":            edited,
		"docs/architecture.md": extracted,
		".graphifyignore":      "",
	}

	t.Run("正常系", func(t *testing.T) {
		t.Run("内容が変わったドキュメントだけを未処理として数える", func(t *testing.T) {
			buffer := captureLog(t)

			err := run(nil, staticRead(corpus), staticGit(baseline+"\n", "12\t3\tREADME.md\n"))

			require.NoError(t, err)
			assert.Contains(t, buffer.String(), "未処理 1 ファイル / 15 行")
			assert.Contains(t, buffer.String(), "README.md")
			assert.NotContains(t, buffer.String(), "docs/architecture.md")
		})

		t.Run("全て抽出済みなら未処理なしと報告する", func(t *testing.T) {
			buffer := captureLog(t)
			clean := map[string]string{
				"graphify-out/manifest.json": fmt.Sprintf(
					`{"README.md":{"ast_hash":"%[1]s","semantic_hash":"%[1]s"}}`, digest(extracted),
				),
				"README.md":       extracted,
				".graphifyignore": "",
			}

			err := run(nil, staticRead(clean), staticGit(baseline+"\n", ""))

			require.NoError(t, err)
			assert.Contains(t, buffer.String(), "未処理はありません")
		})

		t.Run("基準コミットが無ければ落とさず件数だけ報告する", func(t *testing.T) {
			buffer := captureLog(t)

			// 意味論抽出をまだ一度もコミットしていない状態。守るべき履歴が無いだけで異常ではない。
			err := run(nil, staticRead(corpus), staticGit("", ""))

			require.NoError(t, err)
			assert.Contains(t, buffer.String(), "未処理 1 ファイル")
			assert.Contains(t, buffer.String(), "行数は測れません")
		})

		t.Run("消えたファイルは抽出し直す対象ではないので数えない", func(t *testing.T) {
			buffer := captureLog(t)
			missing := map[string]string{
				"graphify-out/manifest.json": `{"deleted.md":{"ast_hash":"x","semantic_hash":"x"}}`,
				".graphifyignore":            "",
			}

			err := run(nil, staticRead(missing), staticGit(baseline+"\n", ""))

			require.NoError(t, err)
			assert.Contains(t, buffer.String(), "未処理はありません")
		})

		t.Run("ヘルプ要求は失敗にしない", func(t *testing.T) {
			captureLog(t)

			require.NoError(t, run([]string{"-h"}, staticRead(nil), staticGit("", "")))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Run("manifestを読めなければ0件ではなくエラーにする", func(t *testing.T) {
			captureLog(t)

			err := run(nil, staticRead(nil), staticGit("", ""))

			require.ErrorIs(t, err, errManifestUnreadable)
		})

		t.Run("manifestが壊れていれば0件ではなくエラーにする", func(t *testing.T) {
			captureLog(t)

			err := run(nil, staticRead(map[string]string{"graphify-out/manifest.json": "{"}), staticGit("", ""))

			require.ErrorIs(t, err, errManifestUnreadable)
		})

		t.Run("差分を測れなければエラーにする", func(t *testing.T) {
			captureLog(t)
			sentinel := xerrors.New("bad revision")

			err := run(nil, staticRead(corpus), func(args ...string) (string, error) {
				if args[0] == "log" {
					return baseline + "\n", nil
				}

				return "", sentinel
			})

			require.ErrorIs(t, err, sentinel)
		})

		t.Run("未知のフラグは失敗にする", func(t *testing.T) {
			captureLog(t)

			require.Error(t, run([]string{"-unknown"}, staticRead(corpus), staticGit("", "")))
		})
	})
}

func Test_staleDocuments(t *testing.T) {
	t.Parallel()

	const body = "content\n"

	manifest := func(entries string) map[string]string {
		return map[string]string{"m.json": entries}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("graphifyignore のディレクトリ配下は数えない", func(t *testing.T) {
			t.Parallel()
			files := manifest(`{"docs/godoc/index.html":{"semantic_hash":"stale"},"README.md":{"semantic_hash":"stale"}}`)
			files["docs/godoc/index.html"] = body
			files["README.md"] = body
			files["ignore"] = "docs/godoc/\n"

			stale, err := staleDocuments("m.json", "ignore", staticRead(files))

			require.NoError(t, err)
			assert.Equal(t, []string{"README.md"}, stale)
		})

		t.Run("graphifyignore の拡張子パターンは数えない", func(t *testing.T) {
			t.Parallel()
			files := manifest(`{"docs/rules.ja.md":{"semantic_hash":"stale"},"docs/rules.md":{"semantic_hash":"stale"}}`)
			files["docs/rules.ja.md"] = body
			files["docs/rules.md"] = body
			files["ignore"] = "*.ja.md\n**/*.ja.md\n"

			stale, err := staleDocuments("m.json", "ignore", staticRead(files))

			require.NoError(t, err)
			assert.Equal(t, []string{"docs/rules.md"}, stale)
		})

		t.Run("意味論の対象外の拡張子は数えない", func(t *testing.T) {
			t.Parallel()
			files := manifest(`{"main.go":{"semantic_hash":"stale"},"README.md":{"semantic_hash":"stale"}}`)
			files["main.go"] = body
			files["README.md"] = body
			files["ignore"] = ""

			stale, err := staleDocuments("m.json", "ignore", staticRead(files))

			require.NoError(t, err)
			assert.Equal(t, []string{"README.md"}, stale)
		})

		t.Run("graphifyignore が無くても除外なしで動く", func(t *testing.T) {
			t.Parallel()
			files := manifest(`{"README.md":{"semantic_hash":"stale"}}`)
			files["README.md"] = body

			stale, err := staleDocuments("m.json", "absent", staticRead(files))

			require.NoError(t, err)
			assert.Equal(t, []string{"README.md"}, stale)
		})

		t.Run("semantic_hash が空なら未抽出として数える", func(t *testing.T) {
			t.Parallel()
			files := manifest(`{"README.md":{"semantic_hash":""}}`)
			files["README.md"] = body
			files["ignore"] = ""

			stale, err := staleDocuments("m.json", "ignore", staticRead(files))

			require.NoError(t, err)
			assert.Equal(t, []string{"README.md"}, stale)
		})
	})
}

func Test_matchesIgnore(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		file, pattern string
		want          bool
	}{
		"ディレクトリ接頭辞は配下に当たる":        {"docs/godoc/a/b.html", "docs/godoc/", true},
		"ディレクトリ接頭辞は別ディレクトリに当たらない": {"docs/adr/0001.md", "docs/godoc/", false},
		"拡張子パターンは深い階層にも当たる":       {"docs/adr/0001.ja.md", "*.ja.md", true},
		"拡張子パターンは正本に当たらない":        {"docs/adr/0001.md", "*.ja.md", false},
		"二重アスタリスクの拡張子パターン":        {"internal/x/y.gen.go", "**/*.gen.go", true},
		"二重アスタリスクのディレクトリ名":        {"database/gen/a.sql", "**/gen/", true},
		"素のパスは完全一致":               {"openapi/openapi.gen.yaml", "openapi/openapi.gen.yaml", true},
		"素のパスは前方一致では当たらない":        {"openapi/openapi.yaml", "openapi/openapi.gen.yaml", false},
		"素のパスはディレクトリとしても当たる":      {"vendor/x/y.md", "vendor", true},
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		for name, tt := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, tt.want, matchesIgnore(tt.file, tt.pattern))
			})
		}
	})
}

//nolint:paralleltest // t.Chdir を使うため並列化できない
func Test_gitOutput(t *testing.T) {
	t.Run("正常系", func(t *testing.T) {
		t.Run("標準出力をそのまま返す", func(t *testing.T) {
			t.Chdir(t.TempDir())

			out, err := gitOutput("--version")

			require.NoError(t, err)
			assert.Contains(t, out, "git version")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Run("gitリポジトリの外では空ではなくエラーを返す", func(t *testing.T) {
			t.Chdir(t.TempDir())

			out, err := gitOutput("log", "-1", "--format=%H")

			require.Error(t, err)
			assert.Empty(t, out)
		})
	})
}

func Test_lastSemanticCommit(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("改行を落としたコミットハッシュを返す", func(t *testing.T) {
			t.Parallel()

			got, err := lastSemanticCommit("cache", func(...string) (string, error) { return "abc123\n", nil })

			require.NoError(t, err)
			assert.Equal(t, "abc123", got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("履歴が空なら基準なしとして扱う", func(t *testing.T) {
			t.Parallel()

			_, err := lastSemanticCommit("cache", func(...string) (string, error) { return "\n", nil })

			require.ErrorIs(t, err, errNoBaseline)
		})

		t.Run("gitが失敗すれば基準なしとは区別してエラーにする", func(t *testing.T) {
			t.Parallel()
			sentinel := xerrors.New("not a repository")

			_, err := lastSemanticCommit("cache", func(...string) (string, error) { return "", sentinel })

			require.ErrorIs(t, err, sentinel)
			assert.NotErrorIs(t, err, errNoBaseline)
		})
	})
}

func Test_matchesGlob(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("先頭のアスタリスクは接尾辞一致になる", func(t *testing.T) {
			t.Parallel()

			assert.True(t, matchesGlob("rules.ja.md", "*.ja.md"))
		})

		t.Run("アスタリスクが無ければ完全一致になる", func(t *testing.T) {
			t.Parallel()

			assert.True(t, matchesGlob("LICENSE", "LICENSE"))
			assert.False(t, matchesGlob("LICENSE.md", "LICENSE"))
		})
	})
}

func Test_loadIgnore(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("コメントと空行は宣言として読まない", func(t *testing.T) {
			t.Parallel()
			body := "# 見出し\n\n  \nvendor/\n"

			excluded, err := loadIgnore("ignore", staticRead(map[string]string{"ignore": body}))

			require.NoError(t, err)
			assert.True(t, excluded("vendor/a.md"))
			assert.False(t, excluded("README.md"))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("読み込み失敗は除外なしに丸めずエラーにする", func(t *testing.T) {
			t.Parallel()
			sentinel := xerrors.New("permission denied")

			_, err := loadIgnore("ignore", func(string) ([]byte, error) { return nil, sentinel })

			require.ErrorIs(t, err, sentinel)
		})
	})
}

func Test_measure(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("基準コミットから HEAD までを対象ファイルに限って測る", func(t *testing.T) {
			t.Parallel()
			var got []string

			_, _, err := measure("abc123", []string{"docs/a.md", "docs/b.md"}, func(args ...string) (string, error) {
				got = args

				return "", nil
			})

			require.NoError(t, err)
			assert.Equal(t, []string{"diff", "--numstat", "abc123..HEAD", "--", "docs/a.md", "docs/b.md"}, got)
		})

		t.Run("numstat の結果を変更量の降順で返す", func(t *testing.T) {
			t.Parallel()

			items, total, err := measure("abc123", []string{"a.md", "b.md"},
				func(...string) (string, error) { return "1\t2\ta.md\n30\t0\tb.md\n", nil })

			require.NoError(t, err)
			assert.Equal(t, 33, total)
			require.Len(t, items, 2)
			assert.Equal(t, "b.md", items[0].path)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("git が失敗すれば基準コミットを添えてエラーにする", func(t *testing.T) {
			t.Parallel()
			sentinel := xerrors.New("bad revision")

			_, _, err := measure("abcdef1234567890", nil, func(...string) (string, error) { return "", sentinel })

			require.ErrorIs(t, err, sentinel)
			assert.Contains(t, err.Error(), short("abcdef1234567890"))
		})
	})
}

func Test_atoiOrZero(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		field string
		want  int
	}{
		"十進の数はそのまま":     {"12", 12},
		"ゼロはゼロ":         {"0", 0},
		"符号付きも解釈する":     {"-3", -3},
		"バイナリを表すハイフンは0": {"-", 0},
		"数でない欄は0":       {"x", 0},
		"空欄は0":          {"", 0},
		"前後に空白があるものは0":  {" 1 ", 0},
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		for name, tt := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, tt.want, atoiOrZero(tt.field))
			})
		}
	})
}

func Test_parseNumstat(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("追加と削除を足して変更量の降順で返す", func(t *testing.T) {
			t.Parallel()

			items, total, err := parseNumstat("1\t2\ta.md\n30\t0\tb.md\n")

			require.NoError(t, err)
			assert.Equal(t, 33, total)
			assert.Equal(t, "b.md", items[0].path)
			assert.Equal(t, 30, items[0].changed)
		})

		t.Run("バイナリ扱いの行は0行として数えるがファイルは残す", func(t *testing.T) {
			t.Parallel()

			items, total, err := parseNumstat("-\t-\tdiagram.png\n")

			require.NoError(t, err)
			assert.Equal(t, 0, total)
			require.Len(t, items, 1)
		})

		t.Run("同じ変更量ならパス順で安定する", func(t *testing.T) {
			t.Parallel()

			items, _, err := parseNumstat("1\t0\tb.md\n1\t0\ta.md\n")

			require.NoError(t, err)
			assert.Equal(t, []string{"a.md", "b.md"}, []string{items[0].path, items[1].path})
		})

		t.Run("空の出力は0件を返す", func(t *testing.T) {
			t.Parallel()

			items, total, err := parseNumstat("\n")

			require.NoError(t, err)
			assert.Empty(t, items)
			assert.Equal(t, 0, total)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("欄の足りない行は0件に丸めずエラーにする", func(t *testing.T) {
			t.Parallel()

			_, _, err := parseNumstat("1\ta.md\n")

			require.ErrorIs(t, err, errManifestUnreadable)
		})
	})
}

// t.Setenv がプロセス全体の環境を触るため、このテストは並列化しない。
func Test_emitCounts(t *testing.T) {
	t.Run("正常系", func(t *testing.T) {
		t.Run("未処理量をステップ出力へ書く", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.txt")
			t.Setenv("GITHUB_OUTPUT", path)

			require.NoError(t, emitCounts(3, 120))

			body, err := os.ReadFile(path) //nolint:gosec // テスト内で組み立てた一時パス
			require.NoError(t, err)
			assert.Equal(t, "files=3\nlines=120\n", string(body))
		})

		t.Run("溜まっていなければ 0 を書く", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.txt")
			t.Setenv("GITHUB_OUTPUT", path)

			require.NoError(t, emitCounts(0, 0))

			body, err := os.ReadFile(path) //nolint:gosec // テスト内で組み立てた一時パス
			require.NoError(t, err)
			assert.Equal(t, "files=0\nlines=0\n", string(body))
		})

		t.Run("Actions の外では何も書かない", func(t *testing.T) {
			t.Setenv("GITHUB_OUTPUT", "")

			require.NoError(t, emitCounts(3, 120))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Run("宛先を開けなければ失敗する", func(t *testing.T) {
			t.Setenv("GITHUB_OUTPUT", filepath.Join(t.TempDir(), "missing", "output.txt"))

			require.Error(t, emitCounts(3, 120))
		})
	})
}

// t.Setenv がプロセス全体の環境を触るため、このテストは並列化しない。
func Test_report(t *testing.T) {
	t.Run("正常系", func(t *testing.T) {
		t.Run("閾値を超えていれば警告付きで job summary に書く", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.md")
			t.Setenv("GITHUB_STEP_SUMMARY", path)

			require.NoError(t, report(true, 2, 40, 20, []pending{{path: "a.md", changed: 40}}))

			body, err := os.ReadFile(path) //nolint:gosec // テスト内で組み立てた一時パス
			require.NoError(t, err)
			assert.Contains(t, string(body), "⚠️")
			assert.Contains(t, string(body), "`a.md` (40)")
		})

		t.Run("閾値未満なら警告を付けない", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.md")
			t.Setenv("GITHUB_STEP_SUMMARY", path)

			require.NoError(t, report(true, 1, 3, 20, []pending{{path: "a.md", changed: 3}}))

			body, err := os.ReadFile(path) //nolint:gosec // テスト内で組み立てた一時パス
			require.NoError(t, err)
			assert.NotContains(t, string(body), "⚠️")
		})

		t.Run("上限を超えたら省略して残数を示す", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.md")
			t.Setenv("GITHUB_STEP_SUMMARY", path)
			items := make([]pending, 0, maxListedFiles+2)
			for i := range maxListedFiles + 2 {
				items = append(items, pending{path: fmt.Sprintf("%d.md", i), changed: 1})
			}

			require.NoError(t, report(true, len(items), len(items), 1, items))

			body, err := os.ReadFile(path) //nolint:gosec // テスト内で組み立てた一時パス
			require.NoError(t, err)
			assert.Contains(t, string(body), "... 2 more")
		})

		t.Run("github を指定しなければ何も書かない", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.md")
			t.Setenv("GITHUB_STEP_SUMMARY", path)

			require.NoError(t, report(false, 1, 1, 1, nil))

			_, err := os.Stat(path)
			require.ErrorIs(t, err, os.ErrNotExist)
		})

		t.Run("宛先が未設定なら何も書かない", func(t *testing.T) {
			t.Setenv("GITHUB_STEP_SUMMARY", "")

			require.NoError(t, report(true, 1, 1, 1, nil))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Run("宛先を開けなければエラーにする", func(t *testing.T) {
			t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(t.TempDir(), "absent", "summary.md"))

			require.Error(t, report(true, 1, 1, 1, nil))
		})
	})
}

func Test_short(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("長いハッシュは8桁に切る", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "0123456789abcdef"[:8], short("0123456789abcdef"))
		})

		t.Run("短い値はそのまま返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "abc", short("abc"))
		})
	})
}

func Test_isDocument(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("意味論の対象拡張子を認める", func(t *testing.T) {
			t.Parallel()

			for _, ext := range docExtensions {
				assert.True(t, isDocument("a"+ext), ext)
			}
		})

		t.Run("大文字の拡張子も認める", func(t *testing.T) {
			t.Parallel()

			assert.True(t, isDocument("README.MD"))
		})

		t.Run("コード拡張子は認めない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isDocument("main.go"))
		})

		t.Run("拡張子が無ければ認めない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isDocument(strings.TrimSuffix("LICENSE", "")))
		})
	})
}
