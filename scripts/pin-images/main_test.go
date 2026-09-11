package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// テスト用の 64-hex digest（値は任意、形式のみ意味を持つ）。
const (
	digestAlpine = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestGolang = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	digestStale  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	digestUnreg  = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	digestFresh  = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

// createdLayout は docker buildx imagetools inspect --format が出力する created の形式。
const createdLayout = "2006-01-02 15:04:05 -0700 MST"

// errWD は、作業ディレクトリの取得失敗の伝播を検証するためのセンチネルです。
var errWD = xerrors.New("getwd failed")

// usesTarget / composeTarget / dockerfileTarget は、ファイルを介さず走査対象 1 件を組み立てる。
func usesTarget() target {
	return target{re: usesDockerRe, loose: looseDockerUsesRe}
}

func composeTarget() target {
	return target{re: composeImageRe, loose: composeImageLoose}
}

func dockerfileTarget() target {
	return target{re: fromRe, loose: fromLooseRe, exemptTagless: dockerfileExemptTagless}
}

// testTargets は root 配下の走査対象を集める。
func testTargets(t *testing.T, root string) []target {
	t.Helper()
	targets, err := targetFiles(root)
	require.NoError(t, err)
	return targets
}

func testLock() map[string]string {
	return map[string]string{
		"alpine:3.24":        digestAlpine,
		"golang:1.26-alpine": digestGolang,
	}
}

// writeFile は親ディレクトリごとファイルを作る。
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

// dockerStubBody は、digest 問い合わせには Digest 行を、--format 指定（created 問い合わせ）には
// created を返すダミー docker のシェル本体を組み立てる。
func dockerStubBody(digest string, created time.Time) string {
	return "case \"$*\" in\n" +
		"  *--format*) printf '%s\\n' '" + created.UTC().Format(createdLayout) + "' ;;\n" +
		"  *) printf 'Digest: %s\\n' '" + digest + "' ;;\n" +
		"esac\n"
}

// writeDockerStub は実 docker を呼ばずに inspect の入出力を差し替えるダミーを書き出し、その
// ディレクトリを返す（PATH へは載せない）。
func writeDockerStub(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" + body
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755)) //nolint:gosec // 実行可能スタブ
	return dir
}

// useDockerStub はダミー docker を PATH 先頭へ載せる。
func useDockerStub(t *testing.T, body string) {
	t.Helper()
	t.Setenv("PATH", writeDockerStub(t, body)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// captureLog は log の出力先を差し替え、書き込まれた内容を読めるようにする。
func captureLog(t *testing.T) *strings.Builder {
	t.Helper()
	var b strings.Builder
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &b
}

func Test_imageRef_key(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("image と tag をコロンで連結する", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "golang:1.26-alpine", imageRef{image: "golang", tag: "1.26-alpine"}.key())
		})

		t.Run("registry ポートを含む image でも先頭から連結する", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "localhost:5000/app:v2", imageRef{image: "localhost:5000/app", tag: "v2"}.key())
		})
	})
}

func Test_lastColonSplit(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("最後のコロンで分割する", func(t *testing.T) {
			t.Parallel()
			head, tail, ok := lastColonSplit("localhost:5000/app:v2")
			require.True(t, ok)
			assert.Equal(t, "localhost:5000/app", head)
			assert.Equal(t, "v2", tail)
		})

		t.Run("末尾がコロンなら後半は空になる", func(t *testing.T) {
			t.Parallel()
			head, tail, ok := lastColonSplit("alpine:")
			require.True(t, ok)
			assert.Equal(t, "alpine", head)
			assert.Empty(t, tail)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("コロンが無ければ分割しない", func(t *testing.T) {
			t.Parallel()
			head, tail, ok := lastColonSplit("scratch")
			assert.False(t, ok)
			assert.Empty(t, head)
			assert.Empty(t, tail)
		})
	})
}

func Test_parseRef(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("image:tag@digest は digest を捨て image:tag を返す", func(t *testing.T) {
			t.Parallel()
			r, ok := parseRef("golang:1.26-alpine@" + digestGolang)
			require.True(t, ok)
			assert.Equal(t, "golang", r.image)
			assert.Equal(t, "1.26-alpine", r.tag)
			assert.Equal(t, "golang:1.26-alpine", r.key())
		})

		t.Run("registry ホスト付き image も最後のコロンで tag を分離する", func(t *testing.T) {
			t.Parallel()
			r, ok := parseRef("ghcr.io/foo/bar:v1.0")
			require.True(t, ok)
			assert.Equal(t, "ghcr.io/foo/bar", r.image)
			assert.Equal(t, "v1.0", r.tag)
		})

		t.Run("registry ポート付きでも tag を正しく分離する", func(t *testing.T) {
			t.Parallel()
			r, ok := parseRef("localhost:5000/app:v2")
			require.True(t, ok)
			assert.Equal(t, "localhost:5000/app", r.image)
			assert.Equal(t, "v2", r.tag)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("tag 無し（scratch）は対象外", func(t *testing.T) {
			t.Parallel()
			_, ok := parseRef("scratch")
			assert.False(t, ok)
		})

		t.Run("ビルドステージ参照（コロン無し）は対象外", func(t *testing.T) {
			t.Parallel()
			_, ok := parseRef("builder")
			assert.False(t, ok)
		})

		t.Run("最後のコロンが registry:port で tag が無い形は対象外", func(t *testing.T) {
			t.Parallel()
			_, ok := parseRef("localhost:5000/app")
			assert.False(t, ok)
		})
	})
}

func Test_targetFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("Dockerfile には FROM、compose には image 行の正規表現を割り当てる", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			writeFile(t, filepath.Join(root, "docker-compose.yaml"), "services:\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)
			require.Len(t, targets, 2)

			byPath := map[string]*regexp.Regexp{}
			for _, tg := range targets {
				byPath[tg.path] = tg.re
			}
			assert.Same(t, fromRe, byPath[filepath.Join(root, "docker", "app", "Dockerfile")])
			assert.Same(t, composeImageRe, byPath[filepath.Join(root, "docker-compose.yaml")])
		})

		t.Run("compose は接尾辞違いも収集しパス順に整列する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker-compose.yaml"), "services:\n")
			writeFile(t, filepath.Join(root, "docker-compose.attach.yaml"), "services:\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)
			require.Len(t, targets, 2)
			assert.Equal(t, filepath.Join(root, "docker-compose.attach.yaml"), targets[0].path)
			assert.Equal(t, filepath.Join(root, "docker-compose.yaml"), targets[1].path)
		})

		t.Run("走査対象の階層から外れたファイルは含めない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "nested", "Dockerfile"), "FROM alpine:3.24\n")
			writeFile(t, filepath.Join(root, "docker", "Dockerfile"), "FROM alpine:3.24\n")
			writeFile(t, filepath.Join(root, "sub", "docker-compose.yaml"), "services:\n")
			writeFile(t, filepath.Join(root, "docker", "app", "README.md"), "\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)
			assert.Empty(t, targets)
		})

		t.Run("workflow には uses: docker:// と service の image の 2 つを割り当てる", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"), "jobs:\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)
			require.Len(t, targets, 2)

			// target は 1 つにつき 1 つの正規表現しか持てないため、同じファイルを 2 度登録する。
			res := []*regexp.Regexp{targets[0].re, targets[1].re}
			assert.Contains(t, res, usesDockerRe)
			assert.Contains(t, res, serviceImageRe)

			looses := []*regexp.Regexp{targets[0].loose, targets[1].loose}
			assert.Contains(t, looses, looseDockerUsesRe)
			assert.Contains(t, looses, serviceImageLoose)
		})

		t.Run("workflow 定義と入れ子の composite action 定義を集める", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"), "jobs:\n")
			writeFile(t, filepath.Join(root, ".github", "workflows", "lint.yml"), "jobs:\n")
			writeFile(t, filepath.Join(root, ".github", "actions", "g", "setup", "action.yml"), "runs:\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)

			paths := make([]string, 0, len(targets))
			for _, tg := range targets {
				paths = append(paths, tg.path)
			}
			// workflow / composite action は 2 つの正規表現で 2 度ずつ登録される。
			assert.Equal(t, []string{
				filepath.Join(root, ".github", "actions", "g", "setup", "action.yml"),
				filepath.Join(root, ".github", "actions", "g", "setup", "action.yml"),
				filepath.Join(root, ".github", "workflows", "ci.yaml"),
				filepath.Join(root, ".github", "workflows", "ci.yaml"),
				filepath.Join(root, ".github", "workflows", "lint.yml"),
				filepath.Join(root, ".github", "workflows", "lint.yml"),
			}, paths)
		})

		t.Run("すべての対象が取りこぼし検出のパターンを持つ", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			writeFile(t, filepath.Join(root, "docker-compose.yaml"), "services:\n")
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"), "jobs:\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)
			// Dockerfile / compose が 1 つずつ、workflow が 2 つ。
			require.Len(t, targets, 4)
			for _, tg := range targets {
				assert.NotNil(t, tg.loose, tg.path)
			}
		})

		t.Run("tag 無しの除外は Dockerfile の対象だけが持つ", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			writeFile(t, filepath.Join(root, "docker-compose.yaml"), "services:\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)

			exempt := map[string]bool{}
			for _, tg := range targets {
				exempt[tg.path] = tg.exemptTagless != nil
			}
			assert.Equal(t, map[string]bool{
				filepath.Join(root, "docker", "app", "Dockerfile"): true,
				filepath.Join(root, "docker-compose.yaml"):         false,
			}, exempt)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("workflow ディレクトリの YAML 以外は含めない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "README.md"), "\n")
			writeFile(t, filepath.Join(root, ".github", "actions", "setup", "README.md"), "\n")

			targets, err := targetFiles(root)
			require.NoError(t, err)
			assert.Empty(t, targets)
		})
	})
}

func Test_usesDockerRe(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("uses 行を接頭辞・参照・接尾辞へ割る", func(t *testing.T) {
			t.Parallel()
			m := usesDockerRe.FindStringSubmatch("      - uses: docker://alpine:3.24 # 補助\n")
			require.NotNil(t, m)
			assert.Equal(t, "      - uses: docker://", m[1])
			assert.Equal(t, "alpine:3.24", m[2])
			assert.Equal(t, " # 補助", m[3])
		})

		t.Run("digest を参照側へ取り込み接尾辞へ残さない", func(t *testing.T) {
			t.Parallel()
			m := usesDockerRe.FindStringSubmatch("      - uses: docker://alpine:3.24@" + digestUnreg + "\n")
			require.NotNil(t, m)
			assert.Equal(t, "alpine:3.24@"+digestUnreg, m[2])
		})

		t.Run("registry を含む参照をそのまま取り出す", func(t *testing.T) {
			t.Parallel()
			m := usesDockerRe.FindStringSubmatch("      - uses: docker://ghcr.io/owner/app:1.0.0\n")
			require.NotNil(t, m)
			assert.Equal(t, "ghcr.io/owner/app:1.0.0", m[2])
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("tag を持たない参照に一致しない", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, usesDockerRe.FindStringSubmatch("      - uses: docker://alpine\n"))
		})

		t.Run("registry ポートだけで tag を持たない参照に一致しない", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, usesDockerRe.FindStringSubmatch("      - uses: docker://localhost:5000/app\n"))
		})

		t.Run("owner/repo 形式の uses に一致しない", func(t *testing.T) {
			t.Parallel()
			assert.Nil(t, usesDockerRe.FindStringSubmatch("      - uses: actions/checkout@v7\n"))
		})
	})
}

func Test_serviceImageRe(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		cases := map[string]struct {
			line   string
			prefix string
			ref    string
			suffix string
		}{
			"service の image 行を接頭辞・参照・接尾辞へ割る": {
				line:   "        image: postgres:18.4-trixie # 補助\n",
				prefix: "        image: ",
				ref:    "postgres:18.4-trixie",
				suffix: " # 補助",
			},
			"digest を参照側へ取り込み接尾辞へ残さない": {
				line:   "        image: postgres:18.4-trixie@" + digestUnreg + "\n",
				prefix: "        image: ",
				ref:    "postgres:18.4-trixie@" + digestUnreg,
				suffix: "",
			},
			"registry を含む参照をそのまま取り出す": {
				line:   "        image: amazon/dynamodb-local:3.3.1\n",
				prefix: "        image: ",
				ref:    "amazon/dynamodb-local:3.3.1",
				suffix: "",
			},
		}

		for name, c := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				m := serviceImageRe.FindStringSubmatch(c.line)
				require.NotNil(t, m)
				assert.Equal(t, c.prefix, m[1])
				assert.Equal(t, c.ref, m[2])
				assert.Equal(t, c.suffix, m[3])
			})
		}
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		// 式で組み立てた image は step の with: 配下に現れる。固定のしようがないので走査から外す。
		cases := map[string]string{
			"式で組み立てた image に一致しない": "          image: ${{ steps.meta.outputs.image }}\n",
			"式を後ろに含む image に一致しない": "          image: reg/app@${{ steps.build.outputs.digest }}\n",
			"行頭の image に一致しない":     "image: postgres:18.4-trixie\n",
		}

		for name, line := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				assert.Nil(t, serviceImageRe.FindStringSubmatch(line))
			})
		}
	})
}

func Test_detectLooseRefs(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できたブロック記法を取りこぼしとして数えない", func(t *testing.T) {
			t.Parallel()
			got := detectLooseRefs("      - uses: docker://alpine:3.24\n", usesTarget())
			assert.Empty(t, got)
		})

		t.Run("owner/repo 形式の uses を取りこぼしとして数えない", func(t *testing.T) {
			t.Parallel()
			got := detectLooseRefs("      - uses: actions/checkout@v7\n", usesTarget())
			assert.Empty(t, got)
		})

		t.Run("行全体がコメントなら反応しない", func(t *testing.T) {
			t.Parallel()
			got := detectLooseRefs("  # uses: docker://alpine\n", usesTarget())
			assert.Empty(t, got)
		})

		t.Run("run スクリプトが出力する uses: docker:// を取りこぼしとして数えない", func(t *testing.T) {
			t.Parallel()
			data := "steps:\n  - run: |\n      echo \"- uses: docker://alpine:3.24\"\n"
			assert.Empty(t, detectLooseRefs(data, usesTarget()))
		})

		t.Run("ブロックスカラーを抜けた後の行は再び走査対象に戻る", func(t *testing.T) {
			t.Parallel()
			data := "steps:\n  - run: |\n      echo \"- uses: docker://alpine:3.24\"\n  - uses: docker://busybox\n"
			assert.Equal(t, []int{4}, detectLooseRefs(data, usesTarget()))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("tag を持たない参照を行番号で返す", func(t *testing.T) {
			t.Parallel()
			data := "jobs:\n      - uses: docker://alpine\n"
			assert.Equal(t, []int{2}, detectLooseRefs(data, usesTarget()))
		})

		t.Run("引用符付きの参照を行番号で返す", func(t *testing.T) {
			t.Parallel()
			data := "      - uses: \"docker://alpine:3.24\"\n"
			assert.Equal(t, []int{1}, detectLooseRefs(data, usesTarget()))
		})

		t.Run("flow mapping で書かれた参照を行番号で返す", func(t *testing.T) {
			t.Parallel()
			data := "      - {name: X, uses: docker://alpine:3.24}\n"
			assert.Equal(t, []int{1}, detectLooseRefs(data, usesTarget()))
		})
	})
}

func Test_dockerfileExemptTagless(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("宣言済みのビルドステージと scratch を返す", func(t *testing.T) {
			t.Parallel()
			got := dockerfileExemptTagless("FROM alpine:3.24 AS base\nFROM base AS final\n")
			assert.Equal(t, map[string]bool{"scratch": true, "base": true, "final": true}, got)
		})

		t.Run("ステージ名を小文字へ揃える", func(t *testing.T) {
			t.Parallel()
			got := dockerfileExemptTagless("FROM alpine:3.24 AS Base\n")
			assert.True(t, got["base"])
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("AS を持たない FROM だけなら scratch のみを返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, map[string]bool{"scratch": true}, dockerfileExemptTagless("FROM alpine:3.24\n"))
		})
	})
}

func Test_taglessLines(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("tag を持つ参照は返さない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, taglessLines("    image: alpine:3.24\n", composeTarget()))
		})

		t.Run("宣言済みのビルドステージを参照する FROM は返さない", func(t *testing.T) {
			t.Parallel()
			data := "FROM alpine:3.24 AS base\nFROM base AS final\n"
			assert.Empty(t, taglessLines(data, dockerfileTarget()))
		})

		t.Run("scratch を参照する FROM は返さない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, taglessLines("FROM scratch\n", dockerfileTarget()))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("tag を持たない compose の image を行番号で返す", func(t *testing.T) {
			t.Parallel()
			data := "services:\n  a:\n    image: alpine\n"
			assert.Equal(t, []int{3}, taglessLines(data, composeTarget()))
		})

		t.Run("registry ポートを tag と誤認する形も行番号で返す", func(t *testing.T) {
			t.Parallel()
			data := "services:\n  a:\n    image: localhost:5000/app\n"
			assert.Equal(t, []int{3}, taglessLines(data, composeTarget()))
		})

		t.Run("宣言されていない tag 無しの FROM を行番号で返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, []int{1}, taglessLines("FROM alpine\n", dockerfileTarget()))
		})
	})
}

func Test_validateLoose(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できる参照だけなら通す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"),
				"      - uses: docker://alpine:3.24\n")

			assert.NoError(t, validateLoose(root, testTargets(t, root)))
		})

		t.Run("ビルドステージを参照する tag 無しの FROM は通す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"),
				"FROM alpine:3.24 AS base\nFROM base AS final\n")

			assert.NoError(t, validateLoose(root, testTargets(t, root)))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("tag を持たない参照を位置付きで報告する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"),
				"jobs:\n      - uses: docker://alpine\n")

			err := validateLoose(root, testTargets(t, root))
			require.ErrorIs(t, err, errLooseRef)
			assert.Contains(t, err.Error(), filepath.Join(".github", "workflows", "ci.yaml")+":2")
		})

		t.Run("読めないファイルはエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, ".github", "workflows", "ci.yaml")

			assert.Error(t, validateLoose(root, []target{
				{path: path, re: usesDockerRe, loose: looseDockerUsesRe},
			}))
		})
	})
}

func Test_globFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("root 配下でパターンに一致したパスだけを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "")
			writeFile(t, filepath.Join(root, "docker", "app", "README.md"), "")

			files, err := globFiles(root, "docker/*/Dockerfile")

			require.NoError(t, err)
			assert.Equal(t, []string{filepath.Join(root, "docker", "app", "Dockerfile")}, files)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できないパターンは 0 件へ縮退させずエラーを返す", func(t *testing.T) {
			t.Parallel()

			files, err := globFiles(t.TempDir(), "docker/[")

			require.ErrorIs(t, err, filepath.ErrBadPattern)
			assert.Empty(t, files)
		})
	})
}

func Test_collectKeys(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("Dockerfile と compose の参照を重複なく集約する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			cf := filepath.Join(root, "docker-compose.yaml")
			writeFile(t, df, "FROM golang:1.26-alpine AS builder\nFROM alpine:3.24@"+digestAlpine+"\n")
			writeFile(t, cf, "  db:\n    image: alpine:3.24\n")

			keys, err := collectKeys([]target{{path: df, re: fromRe}, {path: cf, re: composeImageRe}})
			require.NoError(t, err)
			require.Len(t, keys, 2)
			assert.Equal(t, imageRef{image: "alpine", tag: "3.24"}, keys["alpine:3.24"])
			assert.Equal(t, imageRef{image: "golang", tag: "1.26-alpine"}, keys["golang:1.26-alpine"])
		})

		t.Run("scratch とビルドステージ参照は集約しない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			writeFile(t, df, "FROM scratch\nFROM builder AS final\n")

			keys, err := collectKeys([]target{{path: df, re: fromRe}})
			require.NoError(t, err)
			assert.Empty(t, keys)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("走査対象を読めなければ空マップへ縮退させずエラーを返す", func(t *testing.T) {
			t.Parallel()
			absent := filepath.Join(t.TempDir(), "absent", "Dockerfile")

			keys, err := collectKeys([]target{{path: absent, re: fromRe}})

			require.ErrorIs(t, err, os.ErrNotExist)
			assert.Nil(t, keys)
		})
	})
}

func Test_uniq(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("隣接する重複を畳む", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, []string{"a", "b", "c"}, uniq([]string{"a", "a", "b", "c", "c", "c"}))
		})

		t.Run("重複が無ければ全要素を残す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, []string{"a", "b"}, uniq([]string{"a", "b"}))
		})

		t.Run("空スライスは空のまま返す", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, uniq(nil))
		})
	})
}

func Test_rel(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("root 配下は root からの相対パスにする", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, filepath.Join("docker", "app", "Dockerfile"),
				rel(filepath.FromSlash("/repo"), filepath.FromSlash("/repo/docker/app/Dockerfile")))
		})

		t.Run("相対化できない組み合わせは元のパスを返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "docker/app/Dockerfile", rel(filepath.FromSlash("/repo"), "docker/app/Dockerfile"))
		})
	})
}

func Test_rewritePins(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("登録済みで digest 固定済み・一致なら無変更・未登録なし", func(t *testing.T) {
			t.Parallel()
			in := "FROM alpine:3.24@" + digestAlpine + " AS base\n"
			out, missing := rewritePins(in, fromRe, testLock())
			assert.Equal(t, in, out)
			assert.Empty(t, missing)
		})

		t.Run("登録済みで tag のみなら digest を付与する（drift）", func(t *testing.T) {
			t.Parallel()
			in := "FROM alpine:3.24\n"
			out, missing := rewritePins(in, fromRe, testLock())
			assert.Equal(t, "FROM alpine:3.24@"+digestAlpine+"\n", out)
			assert.Empty(t, missing)
		})

		t.Run("登録済みで古い digest なら lock の digest へ置換する", func(t *testing.T) {
			t.Parallel()
			in := "FROM alpine:3.24@" + digestStale + "\n"
			out, missing := rewritePins(in, fromRe, testLock())
			assert.Equal(t, "FROM alpine:3.24@"+digestAlpine+"\n", out)
			assert.Empty(t, missing)
		})

		t.Run("AS ステージ名を保持したまま digest を付与する", func(t *testing.T) {
			t.Parallel()
			in := "FROM golang:1.26-alpine AS builder\n"
			out, missing := rewritePins(in, fromRe, testLock())
			assert.Equal(t, "FROM golang:1.26-alpine@"+digestGolang+" AS builder\n", out)
			assert.Empty(t, missing)
		})

		t.Run("scratch とビルドステージ参照は対象外で無変更", func(t *testing.T) {
			t.Parallel()
			in := "FROM scratch\nFROM builder AS final\n"
			out, missing := rewritePins(in, fromRe, testLock())
			assert.Equal(t, in, out)
			assert.Empty(t, missing)
		})

		t.Run("compose の image 行に digest を付与しインデントを保持する", func(t *testing.T) {
			t.Parallel()
			in := "  database:\n    image: alpine:3.24\n"
			out, missing := rewritePins(in, composeImageRe, testLock())
			assert.Equal(t, "  database:\n    image: alpine:3.24@"+digestAlpine+"\n", out)
			assert.Empty(t, missing)
		})

		t.Run("compose の image 行の末尾コメントを保持したまま digest を置換する", func(t *testing.T) {
			t.Parallel()
			in := "    image: alpine:3.24@" + digestStale + " # base\n"
			out, missing := rewritePins(in, composeImageRe, testLock())
			assert.Equal(t, "    image: alpine:3.24@"+digestAlpine+" # base\n", out)
			assert.Empty(t, missing)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("tag のみで未登録なら無変更のまま未登録として報告する", func(t *testing.T) {
			t.Parallel()
			in := "FROM busybox:1.36\n"
			out, missing := rewritePins(in, fromRe, testLock())
			assert.Equal(t, in, out)
			assert.Equal(t, []string{"busybox:1.36"}, missing)
		})

		t.Run("digest 有りでも未登録なら digest を剥がさず未登録として報告する", func(t *testing.T) {
			t.Parallel()
			in := "FROM busybox:1.36@" + digestUnreg + "\n"
			out, missing := rewritePins(in, fromRe, testLock())
			assert.Equal(t, in, out)
			assert.Equal(t, []string{"busybox:1.36"}, missing)
		})

		t.Run("compose の未登録 image は digest を剥がさず未登録として報告する", func(t *testing.T) {
			t.Parallel()
			in := "    image: busybox:1.36@" + digestUnreg + "\n"
			out, missing := rewritePins(in, composeImageRe, testLock())
			assert.Equal(t, in, out)
			assert.Equal(t, []string{"busybox:1.36"}, missing)
		})
	})
}

func Test_isIgnorableLockErr(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("nil は無視可能", func(t *testing.T) {
			t.Parallel()
			assert.True(t, isIgnorableLockErr(nil))
		})

		t.Run("ファイル不在は無視可能", func(t *testing.T) {
			t.Parallel()
			_, err := readLock(filepath.Join(t.TempDir(), "absent.toml"))
			require.ErrorIs(t, err, os.ErrNotExist)
			assert.True(t, isIgnorableLockErr(err))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("不在以外のエラーは無視不可", func(t *testing.T) {
			t.Parallel()
			assert.False(t, isIgnorableLockErr(errLockInvalidLine))
		})
	})
}

func Test_readLock(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("コメントと空行を読み飛ばして image:tag→digest として読み込む", func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "images-pin.toml")
			body := "# comment\n" +
				"\n" +
				"\"alpine:3.24\" = \"" + digestAlpine + "\"\n" +
				"\"golang:1.26-alpine\" = \"" + digestGolang + "\"\n"
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

			lock, err := readLock(path)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				"alpine:3.24":        digestAlpine,
				"golang:1.26-alpine": digestGolang,
			}, lock)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できない行は読み飛ばさずエラーにする", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "images-pin.toml")
			body := "\"alpine:3.24\" = \"" + digestAlpine + "\"\n" + "invalid line\n"
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

			_, err := readLock(path)

			require.ErrorIs(t, err, errLockInvalidLine)
			assert.ErrorContains(t, err, "2 行目")
		})

		t.Run("キーの重複は後勝ちにせずエラーにする", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "images-pin.toml")
			body := "\"alpine:3.24\" = \"" + digestAlpine + "\"\n" +
				"\"alpine:3.24\" = \"" + digestStale + "\"\n"
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

			_, err := readLock(path)

			require.ErrorIs(t, err, errLockDuplicateKey)
			assert.ErrorContains(t, err, "alpine:3.24")
		})

		t.Run("ファイルが存在しなければエラーを返す", func(t *testing.T) {
			t.Parallel()
			_, err := readLock(filepath.Join(t.TempDir(), "absent.toml"))
			require.Error(t, err)
		})
	})
}

func Test_writeLock(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("キー昇順で書き出し readLock で読み戻せる", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "images-pin.toml")
			require.NoError(t, writeLock(path, testLock()))

			body, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir 配下
			require.NoError(t, err)
			assert.Less(t,
				strings.Index(string(body), `"alpine:3.24"`),
				strings.Index(string(body), `"golang:1.26-alpine"`))

			lock, err := readLock(path)
			require.NoError(t, err)
			assert.Equal(t, testLock(), lock)
		})

		t.Run("空の lock でも SSOT の説明ヘッダを持つファイルを作る", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "images-pin.toml")
			require.NoError(t, writeLock(path, map[string]string{}))

			body, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir 配下
			require.NoError(t, err)
			assert.Contains(t, string(body), "pin-images-resolve")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("存在しないディレクトリ配下へは書けずエラーを返す", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "absent", "images-pin.toml")
			require.Error(t, writeLock(path, testLock()))
		})
	})
}

func Test_maxCreated(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("マルチアーキの複数行から最新を返す", func(t *testing.T) {
			t.Parallel()
			got, ok := maxCreated("2026-07-08 01:02:03.4 +0000 UTC\n2026-07-01 00:00:00 +0000 UTC\n")
			require.True(t, ok)
			assert.Equal(t, time.Date(2026, time.July, 8, 1, 2, 3, 400000000, time.UTC), got.UTC())
		})

		t.Run("行の並び順によらず最新を返す", func(t *testing.T) {
			t.Parallel()
			got, ok := maxCreated("2026-07-01 00:00:00 +0000 UTC\n2026-07-08 01:02:03 +0000 UTC\n")
			require.True(t, ok)
			assert.Equal(t, time.Date(2026, time.July, 8, 1, 2, 3, 0, time.UTC), got.UTC())
		})

		t.Run("フィールド数が足りない行と解析できない行は無視する", func(t *testing.T) {
			t.Parallel()
			got, ok := maxCreated("\nnot a time\n2026-07-08\n2026-07-08 01:02:03 +0000 UTC\n")
			require.True(t, ok)
			assert.Equal(t, time.Date(2026, time.July, 8, 1, 2, 3, 0, time.UTC), got.UTC())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解析できる行が一つも無ければ見つからないと報告する", func(t *testing.T) {
			t.Parallel()
			_, ok := maxCreated("no timestamps here\n")
			assert.False(t, ok)
		})
	})
}

func Test_inspect(t *testing.T) { //nolint:paralleltest // useDockerStub が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("buildx imagetools inspect へ ref と追加引数をこの順で渡し標準出力を返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "printf '%s' \"$*\"\n")

			out, err := inspect(context.Background(), "alpine:3.24", "--format", "{{ .Name }}")
			require.NoError(t, err)
			assert.Equal(t, "buildx imagetools inspect alpine:3.24 --format {{ .Name }}", out)
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("docker が非 0 終了なら ref と標準エラー出力を含むエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "echo 'toomanyrequests' >&2\nexit 1\n")

			_, err := inspect(context.Background(), "alpine:3.24")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "alpine:3.24")
			assert.Contains(t, err.Error(), "toomanyrequests")
		})
	})
}

func Test_resolveDigest(t *testing.T) { //nolint:paralleltest // useDockerStub が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("inspect 出力の Digest 行から digest を取り出す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "printf 'Name: alpine:3.24\\nDigest: %s\\n' '"+digestAlpine+"'\n")

			got, err := resolveDigest(context.Background(), "alpine:3.24")
			require.NoError(t, err)
			assert.Equal(t, digestAlpine, got)
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("Digest 行が無ければ解析不能として扱う", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "printf 'Name: alpine:3.24\\n'\n")

			_, err := resolveDigest(context.Background(), "alpine:3.24")
			require.ErrorIs(t, err, errDigestUnparsable)
		})

		t.Run("inspect が失敗すればそのエラーを伝播する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "exit 1\n")

			_, err := resolveDigest(context.Background(), "alpine:3.24")
			require.Error(t, err)
			require.NotErrorIs(t, err, errDigestUnparsable)
		})
	})
}

func Test_latestCreated(t *testing.T) { //nolint:paralleltest // useDockerStub が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("マルチアーキは全アーキの created のうち最新を返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "printf '2026-07-08 01:02:03 +0000 UTC\\n2026-07-01 00:00:00 +0000 UTC\\n'\n")

			got, err := latestCreated(context.Background(), "alpine:3.24")
			require.NoError(t, err)
			assert.Equal(t, time.Date(2026, time.July, 8, 1, 2, 3, 0, time.UTC), got.UTC())
		})

		t.Run("index 用テンプレートで解析できなければ単一アーキ用へフォールバックする", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "case \"$*\" in\n"+
				"  *range*) exit 0 ;;\n"+
				"esac\n"+
				"printf '2026-07-08 01:02:03 +0000 UTC\\n'\n")

			got, err := latestCreated(context.Background(), "alpine:3.24")
			require.NoError(t, err)
			assert.Equal(t, time.Date(2026, time.July, 8, 1, 2, 3, 0, time.UTC), got.UTC())
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("inspect 自体が失敗し続ければ解析不能ではなくその失敗を返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "echo 'toomanyrequests' >&2\nexit 1\n")

			_, err := latestCreated(context.Background(), "alpine:3.24")
			require.Error(t, err)
			require.NotErrorIs(t, err, errCreatedUnparsable)
			assert.Contains(t, err.Error(), "toomanyrequests")
		})

		t.Run("どちらのテンプレートでも created を読めなければ解析不能として扱う", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "printf 'no timestamps here\\n'\n")

			_, err := latestCreated(context.Background(), "alpine:3.24")
			require.ErrorIs(t, err, errCreatedUnparsable)
		})
	})
}

func Test_digestAgeDays(t *testing.T) { //nolint:paralleltest // useDockerStub が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("created からの経過を日数へ丸めて返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			created := time.Now().UTC().Add(-30*hoursPerDay*time.Hour - time.Hour)
			useDockerStub(t, dockerStubBody(digestAlpine, created))

			got, err := digestAgeDays(context.Background(), "alpine:3.24")
			require.NoError(t, err)
			assert.Equal(t, 30, got)
		})

		t.Run("公開直後は 0 日として返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestAlpine, time.Now().UTC().Add(-time.Hour)))

			got, err := digestAgeDays(context.Background(), "alpine:3.24")
			require.NoError(t, err)
			assert.Zero(t, got)
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("created が未来時刻なら負の日数を返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestAlpine, time.Now().UTC().Add(48*time.Hour)))

			got, err := digestAgeDays(context.Background(), "alpine:3.24")
			require.NoError(t, err)
			assert.Negative(t, got)
		})

		t.Run("created を取得できなければエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "exit 1\n")

			_, err := digestAgeDays(context.Background(), "alpine:3.24")
			require.Error(t, err)
		})
	})
}

func Test_quarantine(t *testing.T) { //nolint:paralleltest // useDockerStub が t.Setenv を使うため並列化不可
	ref := imageRef{image: "alpine", tag: "3.24"}
	key := ref.key()
	existing := map[string]string{key: digestStale}

	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("cooldown 無効なら age を問わず現 digest を採用する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 0, existing)
			require.NoError(t, err)
			assert.Equal(t, digestFresh, use)
			assert.Empty(t, note)
		})

		t.Run("窓を越えた digest はそのまま採用する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC().Add(-30*hoursPerDay*time.Hour)))

			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 14, existing)
			require.NoError(t, err)
			assert.Equal(t, digestFresh, use)
			assert.Empty(t, note)
		})

		t.Run("窓とちょうど同じ日数の digest も採用する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC().Add(-14*hoursPerDay*time.Hour-time.Hour)))

			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 14, existing)
			require.NoError(t, err)
			assert.Equal(t, digestFresh, use)
			assert.Empty(t, note)
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("窓の内側で既存ピンがあれば出来立てを採らず前回 lock へ退く", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC()))

			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 14, existing)
			require.NoError(t, err)
			assert.Equal(t, digestStale, use)
			assert.Contains(t, note, key)
			assert.Contains(t, note, "既存ピンを維持")
		})

		t.Run("窓に 1 日足りなければ既存ピンへ退く", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC().Add(-13*hoursPerDay*time.Hour-time.Hour)))

			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 14, existing)
			require.NoError(t, err)
			assert.Equal(t, digestStale, use)
			assert.Contains(t, note, "既存ピンを維持")
		})

		t.Run("窓の内側で既存ピンが無ければ採用せず skip する", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC()))

			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 14, map[string]string{})
			require.NoError(t, err)
			assert.Empty(t, use)
			assert.Contains(t, note, "skip")
		})

		t.Run("created が未来時刻でも窓の内側として既存ピンへ退く", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC().Add(48*time.Hour)))

			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 14, existing)
			require.NoError(t, err)
			assert.Equal(t, digestStale, use)
			assert.Contains(t, note, "既存ピンを維持")
		})

		t.Run("経過日数を取得できなければ採用も skip もせずエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			useDockerStub(t, "exit 1\n")

			use, note, err := quarantine(context.Background(), ref, key, digestFresh, 14, existing)
			require.Error(t, err)
			assert.Empty(t, use)
			assert.Empty(t, note)
		})
	})
}

func Test_resolve(t *testing.T) { //nolint:paralleltest // useDockerStub が t.Setenv を使うため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("収集した image の digest を解決し lockfile へ書き出す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			useDockerStub(t, dockerStubBody(digestAlpine, time.Now().UTC()))

			require.NoError(t, resolve(root, testTargets(t, root), 0))

			lock, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"alpine:3.24": digestAlpine}, lock)
		})

		t.Run("窓の内側なら既存 lockfile の digest を維持して書き出す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			writeFile(t, filepath.Join(root, lockFile), "\"alpine:3.24\" = \""+digestStale+"\"\n")
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC()))

			require.NoError(t, resolve(root, testTargets(t, root), 14))

			lock, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"alpine:3.24": digestStale}, lock)
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
		t.Run("退行先の無い出来立て image は lockfile へ載せずエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC()))

			err := resolve(root, testTargets(t, root), 14)

			require.ErrorIs(t, err, errNoStepBack)
			require.ErrorContains(t, err, "alpine:3.24")
			assert.NotContains(t, readAll(t, filepath.Join(root, lockFile)), digestFresh)
		})

		t.Run("解釈できない lockfile は既存ピン無しと見なさずエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			body := "\"alpine:3.24\" = \"" + digestStale + "\"\n" + "invalid line\n"
			writeFile(t, filepath.Join(root, lockFile), body)
			useDockerStub(t, dockerStubBody(digestFresh, time.Now().UTC()))

			err := resolve(root, testTargets(t, root), 14)

			require.ErrorIs(t, err, errLockInvalidLine)
			assert.Equal(t, body, readAll(t, filepath.Join(root, lockFile)),
				"読めない lockfile を空と見なすと退行先ごと書き潰される")
		})

		t.Run("走査対象を読めなければエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()

			err := resolve(root, []target{{path: filepath.Join(root, "absent"), re: fromRe}}, 0)

			require.ErrorIs(t, err, os.ErrNotExist)
		})

		t.Run("解釈できない uses: docker:// があれば registry へ問い合わせる前に落ちる", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"), "      - uses: docker://alpine\n")

			err := resolve(root, testTargets(t, root), 0)

			require.ErrorIs(t, err, errLooseRef)
		})

		t.Run("digest を解決できなければどのキーで失敗したか示すエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			useDockerStub(t, "exit 1\n")

			err := resolve(root, testTargets(t, root), 0)

			require.Error(t, err)
			assert.ErrorContains(t, err, "resolve alpine:3.24")
		})

		t.Run("経過日数を取得できなければどのキーで失敗したか示すエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")
			useDockerStub(t, "case \"$*\" in\n"+
				"  *--format*) exit 1 ;;\n"+
				"  *) printf 'Digest: %s\\n' '"+digestAlpine+"' ;;\n"+
				"esac\n")

			err := resolve(root, testTargets(t, root), 14)

			require.Error(t, err)
			assert.ErrorContains(t, err, "age alpine:3.24")
		})

		t.Run("lockfile を書き出せなければエラーを返す", func(t *testing.T) { //nolint:paralleltest // t.Setenv 使用
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker-compose.yaml"), "  db:\n    image: alpine:3.24\n")
			useDockerStub(t, dockerStubBody(digestAlpine, time.Now().UTC()))

			err := resolve(root, testTargets(t, root), 0)

			require.ErrorIs(t, err, os.ErrNotExist)
			assert.ErrorContains(t, err, "write lockfile")
		})
	})
}

func Test_applyOrCheck(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("lockfile の digest で Dockerfile と compose を固定する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			cf := filepath.Join(root, "docker-compose.yaml")
			writeFile(t, df, "FROM golang:1.26-alpine AS builder\n")
			writeFile(t, cf, "  db:\n    image: alpine:3.24\n")
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))

			require.NoError(t, applyOrCheck(root, testTargets(t, root), false))

			assert.Equal(t, "FROM golang:1.26-alpine@"+digestGolang+" AS builder\n", readAll(t, df))
			assert.Equal(t, "  db:\n    image: alpine:3.24@"+digestAlpine+"\n", readAll(t, cf))
		})

		t.Run("lockfile の digest で uses: docker:// を固定する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
			writeFile(t, wf, "      - uses: actions/checkout@v7\n      - uses: docker://alpine:3.24\n")
			require.NoError(t, os.MkdirAll(filepath.Join(root, "docker"), 0o750))
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))

			require.NoError(t, applyOrCheck(root, testTargets(t, root), false))

			assert.Equal(t,
				"      - uses: actions/checkout@v7\n      - uses: docker://alpine:3.24@"+digestAlpine+"\n",
				readAll(t, wf))
		})

		t.Run("固定済みなら check は失敗せずファイルも書き換えない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			body := "FROM alpine:3.24@" + digestAlpine + "\n"
			writeFile(t, df, body)
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))

			require.NoError(t, applyOrCheck(root, testTargets(t, root), true))

			assert.Equal(t, body, readAll(t, df))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できない uses: docker:// があると apply しても書き換えない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
			body := "      - uses: docker://alpine\n"
			writeFile(t, wf, body)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "docker"), 0o750))
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))

			err := applyOrCheck(root, testTargets(t, root), false)

			require.ErrorIs(t, err, errLooseRef)
			assert.Equal(t, body, readAll(t, wf))
		})

		t.Run("未固定のまま check すると書き換えずにエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			writeFile(t, df, "FROM alpine:3.24\n")
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))

			err := applyOrCheck(root, testTargets(t, root), true)

			require.ErrorIs(t, err, errPinDrift)
			require.ErrorContains(t, err, filepath.Join("docker", "app", "Dockerfile"))
			assert.Equal(t, "FROM alpine:3.24\n", readAll(t, df))
		})

		t.Run("後続ファイルに未登録があれば先行ファイルも書き換えずにエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			cf := filepath.Join(root, "docker-compose.yaml")
			writeFile(t, df, "FROM alpine:3.24\n")
			writeFile(t, cf, "  db:\n    image: busybox:1.36\n")
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))

			err := applyOrCheck(root, testTargets(t, root), false)

			require.ErrorIs(t, err, errLockMissingImage)
			require.ErrorContains(t, err, "busybox:1.36")
			assert.Equal(t, "FROM alpine:3.24\n", readAll(t, df))
			assert.Equal(t, "  db:\n    image: busybox:1.36\n", readAll(t, cf))
		})

		t.Run("lockfile を読めなければ resolve を促すエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "docker", "app", "Dockerfile"), "FROM alpine:3.24\n")

			err := applyOrCheck(root, testTargets(t, root), true)

			require.ErrorIs(t, err, os.ErrNotExist)
			assert.ErrorContains(t, err, "pin-images-resolve")
		})

		t.Run("走査対象を読めなければエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "docker"), 0o750))
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))

			err := applyOrCheck(root, []target{{path: filepath.Join(root, "absent"), re: fromRe}}, false)

			require.ErrorIs(t, err, os.ErrNotExist)
		})

		t.Run("固定後の書き込みに失敗すればエラーを返す", func(t *testing.T) {
			t.Parallel()
			if os.Geteuid() == 0 {
				t.Skip("特権実行では読み取り専用ファイルへも書けるため検証できない")
			}
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			writeFile(t, df, "FROM alpine:3.24\n")
			require.NoError(t, writeLock(filepath.Join(root, lockFile), testLock()))
			require.NoError(t, os.Chmod(df, 0o400))

			err := applyOrCheck(root, testTargets(t, root), false)

			require.ErrorIs(t, err, os.ErrPermission)
			assert.ErrorContains(t, err, filepath.Join("docker", "app", "Dockerfile"))
		})
	})
}

func Test_report(t *testing.T) { //nolint:paralleltest // captureLog が log の出力先を差し替えるため並列化不可
	t.Run("正常系", func(t *testing.T) { //nolint:paralleltest // log の出力先を共有するため
		t.Run("apply では固定したファイル数を報告する", func(t *testing.T) { //nolint:paralleltest // log の出力先を共有するため
			out := captureLog(t)

			require.NoError(t, report(nil, false, 3))

			assert.Contains(t, out.String(), "3 ファイルを固定しました")
		})

		t.Run("check では未固定も未登録も無いことを報告する", func(t *testing.T) { //nolint:paralleltest // log の出力先を共有するため
			out := captureLog(t)

			require.NoError(t, report(nil, true, 0))

			assert.Contains(t, out.String(), "全 base image が lockfile 通りに固定されています")
		})
	})

	t.Run("異常系", func(t *testing.T) { //nolint:paralleltest // log の出力先を共有するため
		t.Run("check で drift を見つけたら該当ファイルを挙げてエラーを返す", func(t *testing.T) { //nolint:paralleltest // log の出力先を共有するため
			err := report([]string{"docker/app/Dockerfile"}, true, 0)

			require.ErrorIs(t, err, errPinDrift)
			assert.ErrorContains(t, err, "docker/app/Dockerfile")
		})
	})
}

func Test_validateMissing(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未登録が無ければエラーを返さず処理を続けさせる", func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateMissing(nil))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("lockfile 未登録があれば重複を畳んでエラーを返す", func(t *testing.T) {
			t.Parallel()

			err := validateMissing([]string{"busybox:1.36", "busybox:1.36"})

			require.ErrorIs(t, err, errLockMissingImage)
			assert.Equal(t, 1, strings.Count(err.Error(), "busybox:1.36"))
		})
	})
}

// readAll はテスト対象が書き換えたファイルの内容を返す。
func readAll(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir 配下
	require.NoError(t, err)
	return string(b)
}

// stubWD は、固定のディレクトリを返す作業ディレクトリの取得手段です。
func stubWD(root string) func() (string, error) {
	return func() (string, error) { return root, nil }
}

func Test_run(t *testing.T) {
	t.Parallel()

	lockBody := "\"alpine:3.24\" = \"" + digestAlpine + "\"\n"

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("resolve は走査結果で lockfile を書き直す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, lockFile), lockBody)

			require.NoError(t, run([]string{"resolve"}, stubWD(root)))

			lock, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Empty(t, lock, "resolve 以外へ振り分けると lockfile が据え置かれる")
		})

		t.Run("apply は lockfile の digest でファイルを固定する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			writeFile(t, df, "FROM alpine:3.24\n")
			writeFile(t, filepath.Join(root, lockFile), lockBody)

			require.NoError(t, run([]string{"apply"}, stubWD(root)))

			assert.Equal(t, "FROM alpine:3.24@"+digestAlpine+"\n", readAll(t, df))
		})

		t.Run("check は作業ツリーも lockfile も書き換えない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			df := filepath.Join(root, "docker", "app", "Dockerfile")
			body := "FROM alpine:3.24@" + digestAlpine + "\n"
			writeFile(t, df, body)
			writeFile(t, filepath.Join(root, lockFile), lockBody)

			require.NoError(t, run([]string{"check"}, stubWD(root)))

			assert.Equal(t, body, readAll(t, df))
			assert.Equal(t, lockBody, readAll(t, filepath.Join(root, lockFile)),
				"resolve へ振り分けると lockfile が書き直される")
		})

		t.Run("ヘルプ要求は失敗にしない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, lockFile), lockBody)

			require.NoError(t, run([]string{"resolve", "-h"}, stubWD(root)))

			assert.Equal(t, lockBody, readAll(t, filepath.Join(root, lockFile)),
				"ヘルプ要求で resolve まで進むと lockfile が書き直される")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("サブコマンドが無ければ使い方を返す", func(t *testing.T) {
			t.Parallel()

			err := run(nil, stubWD(t.TempDir()))

			require.ErrorIs(t, err, errUsage)
		})

		t.Run("未知のサブコマンドは使い方を返す", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"bogus"}, stubWD(t.TempDir()))

			require.ErrorIs(t, err, errUsage)
		})

		t.Run("作業ディレクトリを取得できなければ失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"check"}, func() (string, error) { return "", errWD })

			require.ErrorIs(t, err, errWD)
			assert.ErrorContains(t, err, "getwd")
		})

		t.Run("走査対象を集められなければ失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"check"}, stubWD(filepath.Join(t.TempDir(), "x[")))

			require.ErrorIs(t, err, filepath.ErrBadPattern)
		})

		t.Run("未知のフラグはヘルプ要求と混同せず失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"resolve", "-bogus"}, stubWD(t.TempDir()))

			require.ErrorContains(t, err, "failed to parse flags")
		})
	})
}
