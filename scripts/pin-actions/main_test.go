package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// テスト用の 40-hex commit SHA（値は任意、形式のみ意味を持つ）。
	testCommitSHALength = 40

	// absentRelease は githubTimesStub で「そのエンドポイントの資源が存在しない（404）」を表す日数。
	absentRelease = -1
)

var (
	shaCheckout = strings.Repeat("1", testCommitSHALength)
	shaSetupGo  = strings.Repeat("2", testCommitSHALength)
	shaCodeQL   = strings.Repeat("3", testCommitSHALength)
)

var (
	// errAge は、ageFn の失敗伝播を検証するためのセンチネルです。
	errAge = xerrors.New("age lookup failed")
	// errWD は、作業ディレクトリの取得失敗の伝播を検証するためのセンチネルです。
	errWD = xerrors.New("getwd failed")
)

func testLock() map[string]string {
	return map[string]string{
		"actions/checkout@v7.0.0": shaCheckout,
		"actions/setup-go@v6":     shaSetupGo,
		"github/codeql-action@v4": shaCodeQL,
	}
}

func Test_parseUses(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("owner/repo と版から key を構築する", func(t *testing.T) {
			t.Parallel()
			r, ok := parseUses("actions/checkout", "v7.0.0", "")
			require.True(t, ok)
			assert.Equal(t, "actions/checkout", r.repo)
			assert.Empty(t, r.sub)
			assert.Equal(t, "actions/checkout@v7.0.0", r.key())
		})

		t.Run("固定済み行はコメント側の版を採用する", func(t *testing.T) {
			t.Parallel()
			r, ok := parseUses("actions/checkout", shaCheckout, "v7.0.0")
			require.True(t, ok)
			assert.Equal(t, "v7.0.0", r.tag)
			assert.Equal(t, "actions/checkout@v7.0.0", r.key())
		})

		t.Run("サブパスは repo と分離して保持する", func(t *testing.T) {
			t.Parallel()
			r, ok := parseUses("github/codeql-action/init", "v4", "")
			require.True(t, ok)
			assert.Equal(t, "github/codeql-action", r.repo)
			assert.Equal(t, "init", r.sub)
			assert.Equal(t, "github/codeql-action@v4", r.key())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ローカル参照は対象外", func(t *testing.T) {
			t.Parallel()
			_, ok := parseUses("./.github/actions/setup", "", "")
			assert.False(t, ok)
		})

		t.Run("owner/repo に満たない参照は対象外", func(t *testing.T) {
			t.Parallel()
			_, ok := parseUses("single", "v1", "")
			assert.False(t, ok)
		})

		t.Run("docker:// 参照は無意味な repo を作らず対象外", func(t *testing.T) {
			t.Parallel()
			_, ok := parseUses("docker://alpine", "3.22", "")
			assert.False(t, ok)
		})

		t.Run("owner を持つ docker:// 参照も対象外", func(t *testing.T) {
			t.Parallel()
			_, ok := parseUses("docker://ghcr.io/owner/image", "1.0.0", "")
			assert.False(t, ok)
		})
	})
}

func Test_rewritePins(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("登録済みで固定済み・一致なら無変更・未登録なし", func(t *testing.T) {
			t.Parallel()
			in := "      - uses: actions/checkout@" + shaCheckout + " # v7.0.0\n"
			out, missing := rewritePins(in, testLock())
			assert.Equal(t, in, out)
			assert.Empty(t, missing)
		})

		t.Run("登録済みで tag のみなら SHA へ固定する", func(t *testing.T) {
			t.Parallel()
			in := "      - uses: actions/setup-go@v6\n"
			out, missing := rewritePins(in, testLock())
			assert.Equal(t, "      - uses: actions/setup-go@"+shaSetupGo+" # v6\n", out)
			assert.Empty(t, missing)
		})

		t.Run("サブパス付きも repo で解決し path を保持する", func(t *testing.T) {
			t.Parallel()
			in := "      - uses: github/codeql-action/init@v4\n"
			out, missing := rewritePins(in, testLock())
			assert.Equal(t, "      - uses: github/codeql-action/init@"+shaCodeQL+" # v4\n", out)
			assert.Empty(t, missing)
		})

		t.Run("固定対象外のローカル参照は書き換えず未登録にもしない", func(t *testing.T) {
			t.Parallel()
			in := "      - uses: ./.github/actions/setup-postgres@v1\n"
			out, missing := rewritePins(in, testLock())
			assert.Equal(t, in, out)
			assert.Empty(t, missing, "固定対象外を未登録として報告すると check が誤って fail-close する")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("SHA 有りでも版が未登録なら未登録として報告する", func(t *testing.T) {
			t.Parallel()
			in := "      - uses: actions/checkout@" + shaCheckout + " # v99.0.0\n"
			out, missing := rewritePins(in, testLock())
			assert.Equal(t, in, out)
			assert.Equal(t, []string{"actions/checkout@v99.0.0"}, missing)
		})

		t.Run("SHA 無しで未登録なら未登録として報告する", func(t *testing.T) {
			t.Parallel()
			in := "      - uses: some/action@v1.2.3\n"
			out, missing := rewritePins(in, testLock())
			assert.Equal(t, in, out)
			assert.Equal(t, []string{"some/action@v1.2.3"}, missing)
		})
	})
}

func Test_readLock(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("コメント行と空行は読み飛ばして repo@tag→SHA を読み込む", func(t *testing.T) {
			t.Parallel()
			body := "# comment\n" +
				"\n" +
				"\"actions/checkout@v7.0.0\" = \"" + shaCheckout + "\"\n" +
				"\"actions/setup-go@v6\" = \"" + shaSetupGo + "\"\n"

			lock, err := readLock(writeLockFile(t, body))

			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				"actions/checkout@v7.0.0": shaCheckout,
				"actions/setup-go@v6":     shaSetupGo,
			}, lock)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ファイルが存在しなければエラーを返す", func(t *testing.T) {
			t.Parallel()
			_, err := readLock(filepath.Join(t.TempDir(), "absent.toml"))
			require.Error(t, err)
		})

		t.Run("代入として解釈できない行があれば行番号付きでエラーを返す", func(t *testing.T) {
			t.Parallel()
			body := "\"actions/checkout@v7.0.0\" = \"" + shaCheckout + "\"\n" +
				"invalid line\n"

			_, err := readLock(writeLockFile(t, body))

			require.ErrorIs(t, err, errLockInvalidLine)
			require.ErrorContains(t, err, "2 行目")
		})

		t.Run("先頭行がいきなり不正でも行番号を 1 と報告する", func(t *testing.T) {
			t.Parallel()

			_, err := readLock(writeLockFile(t, "invalid line\n"))

			require.ErrorIs(t, err, errLockInvalidLine)
			require.ErrorContains(t, err, "1 行目")
		})

		t.Run("キーが重複していれば後勝ちにせずエラーを返す", func(t *testing.T) {
			t.Parallel()
			body := "\"actions/checkout@v7.0.0\" = \"" + shaCheckout + "\"\n" +
				"\"actions/checkout@v7.0.0\" = \"" + shaSetupGo + "\"\n"

			_, err := readLock(writeLockFile(t, body))

			require.ErrorIs(t, err, errLockDuplicateKey)
			require.ErrorContains(t, err, "2 行目")
			require.ErrorContains(t, err, "actions/checkout@v7.0.0")
		})
	})
}

// writeLockFile は body を lockfile として一時ディレクトリへ書き出し、そのパスを返す。
func writeLockFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actions-pin.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func Test_quarantine(t *testing.T) {
	t.Parallel()

	const key = "actions/checkout@v7.0.0"

	var (
		candidate = shaCheckout
		prev      = strings.Repeat("9", testCommitSHALength)
	)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("minAgeDays<=0 なら ageFn を呼ばず候補をそのまま採用する", func(t *testing.T) {
			t.Parallel()
			called := false
			ageFn := func() (int, error) { called = true; return 0, nil }

			use, note, err := quarantine(ageFn, key, candidate, 0, nil)

			require.NoError(t, err)
			assert.Equal(t, candidate, use)
			assert.Empty(t, note)
			assert.False(t, called, "minAgeDays<=0 では ageFn を呼ばない")
		})

		t.Run("解決先が十分に古ければ候補を採用する", func(t *testing.T) {
			t.Parallel()
			ageFn := func() (int, error) { return 30, nil }

			use, note, err := quarantine(ageFn, key, candidate, 14, nil)

			require.NoError(t, err)
			assert.Equal(t, candidate, use)
			assert.Empty(t, note)
		})

		t.Run("窓とちょうど同じ日数なら候補を採用する", func(t *testing.T) {
			t.Parallel()
			ageFn := func() (int, error) { return 14, nil }

			use, note, err := quarantine(ageFn, key, candidate, 14, nil)

			require.NoError(t, err)
			assert.Equal(t, candidate, use)
			assert.Empty(t, note)
		})

		t.Run("新しすぎる場合は既存ピンを維持しノートを返す", func(t *testing.T) {
			t.Parallel()
			ageFn := func() (int, error) { return 3, nil }
			existing := map[string]string{key: prev}

			use, note, err := quarantine(ageFn, key, candidate, 14, existing)

			require.NoError(t, err)
			assert.Equal(t, prev, use)
			assert.Contains(t, note, "既存ピンを維持")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("窓に 1 日足りなければ既存ピンを維持する", func(t *testing.T) {
			t.Parallel()
			ageFn := func() (int, error) { return 13, nil }
			existing := map[string]string{key: prev}

			use, note, err := quarantine(ageFn, key, candidate, 14, existing)

			require.NoError(t, err)
			assert.Equal(t, prev, use)
			assert.Contains(t, note, "既存ピンを維持")
		})

		t.Run("新しすぎて既存ピンも無ければ skip しノートを返す", func(t *testing.T) {
			t.Parallel()
			ageFn := func() (int, error) { return 3, nil }

			use, note, err := quarantine(ageFn, key, candidate, 14, nil)

			require.NoError(t, err)
			assert.Empty(t, use)
			assert.Contains(t, note, "skip")
		})

		t.Run("ageFn の失敗はそのまま伝播する", func(t *testing.T) {
			t.Parallel()
			ageFn := func() (int, error) { return 0, errAge }

			use, note, err := quarantine(ageFn, key, candidate, 14, nil)

			require.ErrorIs(t, err, errAge)
			assert.Empty(t, use)
			assert.Empty(t, note)
		})
	})
}

func Test_selectSHA(t *testing.T) {
	t.Parallel()

	const tag = "v7.0.0"

	var (
		derefSHA    = shaCheckout
		lightTagSHA = shaSetupGo
		headSHA     = shaCodeQL
	)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("annotated tag の deref を最優先で採用する", func(t *testing.T) {
			t.Parallel()
			out := lightTagSHA + "\trefs/tags/" + tag + "\n" +
				derefSHA + "\trefs/tags/" + tag + "^{}\n"
			sha, err := selectSHA(out, tag)
			require.NoError(t, err)
			assert.Equal(t, derefSHA, sha)
		})

		t.Run("deref が無ければ軽量 tag を採用する", func(t *testing.T) {
			t.Parallel()
			out := lightTagSHA + "\trefs/tags/" + tag + "\n"
			sha, err := selectSHA(out, tag)
			require.NoError(t, err)
			assert.Equal(t, lightTagSHA, sha)
		})

		t.Run("tag が無ければ branch head へフォールバックする", func(t *testing.T) {
			t.Parallel()
			out := headSHA + "\trefs/heads/" + tag + "\n"
			sha, err := selectSHA(out, tag)
			require.NoError(t, err)
			assert.Equal(t, headSHA, sha)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("該当 ref が無ければ未発見エラーを返す", func(t *testing.T) {
			t.Parallel()
			_, err := selectSHA("", tag)
			require.ErrorIs(t, err, errRefNotFound)
			require.ErrorContains(t, err, tag)
		})

		t.Run("無関係な ref のみなら未発見エラーを返す", func(t *testing.T) {
			t.Parallel()
			out := headSHA + "\trefs/heads/main\n"
			_, err := selectSHA(out, tag)
			require.ErrorIs(t, err, errRefNotFound)
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
			_, err := os.Open(filepath.Join(t.TempDir(), "absent.toml"))
			require.Error(t, err)
			assert.True(t, isIgnorableLockErr(err))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("不在以外のエラーは無視不可（fail-close）", func(t *testing.T) {
			t.Parallel()
			assert.False(t, isIgnorableLockErr(errAge))
		})
	})
}

func Test_pickRefTime(t *testing.T) {
	t.Parallel()

	var (
		older = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
		newer = time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)
	)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("Release の方が新しければ published_at を採用する", func(t *testing.T) {
			t.Parallel()
			got, err := pickRefTime(newer, older)
			require.NoError(t, err)
			assert.Equal(t, newer, got)
		})

		t.Run("tag 付け替えで commit の方が新しければ commit 日時を採用する", func(t *testing.T) {
			t.Parallel()
			got, err := pickRefTime(older, newer)
			require.NoError(t, err)
			assert.Equal(t, newer, got)
		})

		t.Run("両者が同一時刻ならその時刻を返す", func(t *testing.T) {
			t.Parallel()
			got, err := pickRefTime(older, older)
			require.NoError(t, err)
			assert.Equal(t, older, got)
		})

		t.Run("Release が無ければ commit 日時を採用する", func(t *testing.T) {
			t.Parallel()
			got, err := pickRefTime(time.Time{}, older)
			require.NoError(t, err)
			assert.Equal(t, older, got)
		})

		t.Run("commit 日時が無ければ published_at を採用する", func(t *testing.T) {
			t.Parallel()
			got, err := pickRefTime(older, time.Time{})
			require.NoError(t, err)
			assert.Equal(t, older, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("どちらの日時も得られなければエラーを返す", func(t *testing.T) {
			t.Parallel()
			_, err := pickRefTime(time.Time{}, time.Time{})
			require.ErrorIs(t, err, errRefDateUnavailable)
		})
	})
}

func Test_targetFiles(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("workflows の yml と yaml を対象にする", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".github/workflows/a.yml", "")
			writeFile(t, root, ".github/workflows/b.yaml", "")

			files, err := targetFiles(root)

			require.NoError(t, err)
			assert.Equal(t, []string{
				filepath.Join(root, ".github/workflows/a.yml"),
				filepath.Join(root, ".github/workflows/b.yaml"),
			}, files)
		})

		t.Run("入れ子の composite action も対象にする", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".github/actions/flat/action.yml", "")
			writeFile(t, root, ".github/actions/group/nested/action.yaml", "")

			files, err := targetFiles(root)

			require.NoError(t, err)
			assert.Equal(t, []string{
				filepath.Join(root, ".github/actions/flat/action.yml"),
				filepath.Join(root, ".github/actions/group/nested/action.yaml"),
			}, files)
		})

		t.Run("action.yml 以外のファイルは対象にしない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".github/actions/flat/action.yml", "")
			writeFile(t, root, ".github/actions/flat/README.md", "")
			writeFile(t, root, ".github/actions/flat/helper.yaml", "")

			files, err := targetFiles(root)

			require.NoError(t, err)
			assert.Equal(t, []string{filepath.Join(root, ".github/actions/flat/action.yml")}, files)
		})

		t.Run("actions ディレクトリが無ければ workflows のみを対象にする", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".github/workflows/a.yml", "")

			files, err := targetFiles(root)

			require.NoError(t, err)
			assert.Equal(t, []string{filepath.Join(root, ".github/workflows/a.yml")}, files)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("不在以外の走査エラーは握り潰さずエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".github", "")

			_, err := targetFiles(root)

			require.Error(t, err)
			assert.False(t, xerrors.Is(err, os.ErrNotExist), "不在エラーとして無視されていない")
		})
	})
}

func Test_fileRefs(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("同一ファイル内の複数の uses: を出現順に返す", func(t *testing.T) {
			t.Parallel()
			data := "      - uses: actions/checkout@v7.0.0\n" +
				"      - uses: actions/setup-go@v6\n"

			refs := fileRefs(data)

			require.Len(t, refs, 2)
			assert.Equal(t, "actions/checkout@v7.0.0", refs[0].key())
			assert.Equal(t, "actions/setup-go@v6", refs[1].key())
		})

		t.Run("ローカル参照は含めない", func(t *testing.T) {
			t.Parallel()
			refs := fileRefs("      - uses: ./.github/actions/setup-postgres\n")
			assert.Empty(t, refs)
		})

		t.Run("docker:// 参照は壊れたキーを作らず含めない", func(t *testing.T) {
			t.Parallel()
			refs := fileRefs("      - uses: docker://alpine@sha256:0000000000000000000000000000000000000000000000000000000000000000\n")
			assert.Empty(t, refs)
		})

		t.Run("クオートされたブロック記法は壊れたキーを作らず含めない", func(t *testing.T) {
			t.Parallel()
			refs := fileRefs("      - uses: 'actions/checkout@v7.0.0'\n")
			assert.Empty(t, refs)
		})
	})
}

func Test_detectLooseUses(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ブロック記法は厳密パターンで処理済みとして検出しない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, detectLooseUses("      - uses: actions/checkout@v7.0.0\n"))
		})

		t.Run("固定済みのブロック記法も検出しない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, detectLooseUses("      - uses: actions/checkout@"+shaCheckout+" # v7.0.0\n"))
		})

		t.Run("クオートされたブロック記法は解釈できない記法として検出する", func(t *testing.T) {
			t.Parallel()
			found := detectLooseUses("      - uses: 'actions/checkout@v7.0.0'\n")
			assert.Equal(t, []string{"actions/checkout@v7.0.0"}, found)
		})

		t.Run("docker:// 参照は厳密パターンで処理済みとして検出しない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(
				t,
				detectLooseUses("      - uses: docker://alpine@sha256:0000000000000000000000000000000000000000000000000000000000000000\n"),
			)
		})

		t.Run("flow mapping でもローカル参照は検出しない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, detectLooseUses("      - {name: Setup, uses: ./.github/actions/setup-postgres}\n"))
		})

		t.Run("版を持たない参照は検出しない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, detectLooseUses("      - {name: Checkout, uses: actions/checkout}\n"))
		})

		t.Run("コメント行の記述例は検出しない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, detectLooseUses("      # uses: actions/checkout@v7.0.0 のように書く\n"))
		})

		t.Run("run: スクリプト中の uses: を含む文字列は検出しない", func(t *testing.T) {
			t.Parallel()
			data := "      - run: |\n" +
				"          echo \"this workflow uses: actions/checkout@v7.0.0 internally\"\n"
			assert.Empty(t, detectLooseUses(data))
		})

		t.Run("ブロックスカラーを抜けた後の行は再び走査対象に戻る", func(t *testing.T) {
			t.Parallel()
			data := "      - run: |\n" +
				"          echo hi\n" +
				"      - {uses: actions/checkout@v7.0.0}\n"
			assert.Equal(t, []string{"actions/checkout@v7.0.0"}, detectLooseUses(data))
		})

		t.Run("uses で終わる別のキーは検出しない", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, detectLooseUses("      disuses: actions/checkout@v7.0.0\n"))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("flow mapping 記法の外部参照を検出する", func(t *testing.T) {
			t.Parallel()
			got := detectLooseUses("      - {name: Checkout, uses: actions/checkout@v7.0.0}\n")
			assert.Equal(t, []string{"actions/checkout@v7.0.0"}, got)
		})

		t.Run("クオート付きの外部参照も検出する", func(t *testing.T) {
			t.Parallel()
			got := detectLooseUses("      - {name: Checkout, uses: \"actions/checkout@v7.0.0\"}\n")
			assert.Equal(t, []string{"actions/checkout@v7.0.0"}, got)
		})

		t.Run("サブパス付きの外部参照も検出する", func(t *testing.T) {
			t.Parallel()
			got := detectLooseUses("      - {name: Init, uses: github/codeql-action/init@v4}\n")
			assert.Equal(t, []string{"github/codeql-action/init@v4"}, got)
		})

		t.Run("クオートしたキーの外部参照も検出する", func(t *testing.T) {
			t.Parallel()
			got := detectLooseUses("      - name: Checkout\n        \"uses\": actions/checkout@v7.0.0\n")
			assert.Equal(t, []string{"actions/checkout@v7.0.0"}, got)
		})

		t.Run("折り畳みブロックスカラーで値を次行へ送った uses: を検出する", func(t *testing.T) {
			t.Parallel()
			got := detectLooseUses("      - uses: >-\n          actions/checkout@v7.0.0\n")
			assert.Equal(t, []string{"uses: の値が同じ行にありません"}, got)
		})

		t.Run("リテラルブロックスカラーで値を次行へ送った uses: を検出する", func(t *testing.T) {
			t.Parallel()
			got := detectLooseUses("      - uses: |-\n          actions/checkout@v7.0.0\n")
			assert.Equal(t, []string{"uses: の値が同じ行にありません"}, got)
		})

		t.Run("YAML alias の uses: は解決できないものとして検出する", func(t *testing.T) {
			t.Parallel()
			got := detectLooseUses("      - uses: *checkout\n")
			require.Len(t, got, 1)
			assert.Contains(t, got[0], "alias")
		})

		t.Run("同一参照が複数あっても重複を除いて返す", func(t *testing.T) {
			t.Parallel()
			data := "      - {uses: actions/checkout@v7.0.0}\n" +
				"      - {uses: actions/checkout@v7.0.0}\n"
			got := detectLooseUses(data)
			assert.Equal(t, []string{"actions/checkout@v7.0.0"}, got)
		})
	})
}

func Test_orphanKeys(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全キーが参照されていれば孤児は無い", func(t *testing.T) {
			t.Parallel()
			used := map[string]bool{
				"actions/checkout@v7.0.0": true,
				"actions/setup-go@v6":     true,
				"github/codeql-action@v4": true,
			}
			assert.Empty(t, orphanKeys(testLock(), used))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("どの uses: からも参照されないキーを昇順で返す", func(t *testing.T) {
			t.Parallel()
			used := map[string]bool{"actions/setup-go@v6": true}

			got := orphanKeys(testLock(), used)

			assert.Equal(t, []string{"actions/checkout@v7.0.0", "github/codeql-action@v4"}, got)
		})
	})
}

func Test_planRewrites(t *testing.T) {
	t.Parallel()

	const (
		resolvable   = "      - uses: actions/setup-go@v6\n"
		unregistered = "      - uses: some/action@v1.2.3\n"
	)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解決可能なファイルの固定後の内容を計画に載せる", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := writeFile(t, root, ".github/workflows/a.yml", resolvable)

			plan, err := planRewrites(root, []string{path}, testLock())

			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				path: "      - uses: actions/setup-go@" + shaSetupGo + " # v6\n",
			}, plan.changes)
		})

		t.Run("固定済みで一致するファイルは変更対象に含めない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			body := "      - uses: actions/setup-go@" + shaSetupGo + " # v6\n"
			path := writeFile(t, root, ".github/workflows/a.yml", body)

			plan, err := planRewrites(root, []string{path}, testLock())

			require.NoError(t, err)
			assert.Empty(t, plan.changes)
		})

		t.Run("参照された lockfile のキーを used として記録する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := writeFile(t, root, ".github/workflows/a.yml", resolvable)

			plan, err := planRewrites(root, []string{path}, testLock())

			require.NoError(t, err)
			assert.Equal(t, map[string]bool{"actions/setup-go@v6": true}, plan.used)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未登録参照は計画に記録するだけで作業ツリーは書き換えない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			body := resolvable + unregistered
			path := writeFile(t, root, ".github/workflows/a.yml", body)

			plan, err := planRewrites(root, []string{path}, testLock())

			require.NoError(t, err)
			assert.Equal(t, []string{"some/action@v1.2.3"}, plan.missing)
			onDisk, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir() 由来
			require.NoError(t, err)
			assert.Equal(t, body, string(onDisk), "中断時に作業ツリーが書き換わっていない")
		})

		t.Run("未登録参照があっても他ファイルの解決可能な変更は計画に載せる", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			ok := writeFile(t, root, ".github/workflows/a.yml", resolvable)
			ng := writeFile(t, root, ".github/workflows/b.yml", unregistered)

			plan, err := planRewrites(root, []string{ok, ng}, testLock())

			require.NoError(t, err)
			assert.Equal(t, []string{"some/action@v1.2.3"}, plan.missing)
			assert.Contains(t, plan.changes, ok, "解決可能なファイルの計画自体は保持する")
		})

		t.Run("厳密パターンで解釈できない記法はファイル名付きで計画に載せる", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			body := unregistered + "      - {name: Checkout, uses: actions/checkout@v7.0.0}\n"
			path := writeFile(t, root, ".github/workflows/a.yml", body)

			plan, err := planRewrites(root, []string{path}, testLock())

			require.NoError(t, err)
			assert.Equal(t, []string{".github/workflows/a.yml: actions/checkout@v7.0.0"}, plan.loose)
		})

		t.Run("読み取れないファイルがあればエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()

			_, err := planRewrites(root, []string{filepath.Join(root, "absent.yml")}, testLock())

			require.ErrorContains(t, err, "absent.yml")
		})
	})
}

// writeFile は root からの相対パスへ body を書き出し、その絶対パスを返す。
func writeFile(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func Test_ref_key(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("lockfile のキーは repo@tag 形式になる", func(t *testing.T) {
			t.Parallel()
			r := ref{repo: "actions/checkout", tag: "v7.0.0"}
			assert.Equal(t, "actions/checkout@v7.0.0", r.key())
		})

		t.Run("サブパスが違っても同じ repo@tag は同一のキーになる", func(t *testing.T) {
			t.Parallel()
			init := ref{repo: "github/codeql-action", sub: "init", tag: "v4"}
			analyze := ref{repo: "github/codeql-action", sub: "analyze", tag: "v4"}
			assert.Equal(t, init.key(), analyze.key())
		})

		t.Run("版が違えば別のキーになる", func(t *testing.T) {
			t.Parallel()
			old := ref{repo: "actions/checkout", tag: "v6"}
			latest := ref{repo: "actions/checkout", tag: "v7.0.0"}
			assert.NotEqual(t, old.key(), latest.key())
		})
	})
}

func Test_collectKeys(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("複数ファイルの参照をキーで集約し解決に必要な repo と版を保持する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			a := writeFile(t, root, ".github/workflows/a.yml", "      - uses: actions/checkout@v7.0.0\n")
			b := writeFile(t, root, ".github/workflows/b.yml", "      - uses: github/codeql-action/init@v4\n")

			keys, err := collectKeys(root, []string{a, b})

			require.NoError(t, err)
			require.Len(t, keys, 2)
			assert.Equal(t, ref{repo: "actions/checkout", tag: "v7.0.0"}, keys["actions/checkout@v7.0.0"])
			assert.Equal(t, "github/codeql-action", keys["github/codeql-action@v4"].repo)
		})

		t.Run("同一参照が複数ファイルにあってもキーは 1 件に集約する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			body := "      - uses: actions/checkout@v7.0.0\n"
			a := writeFile(t, root, ".github/workflows/a.yml", body)
			b := writeFile(t, root, ".github/workflows/b.yml", body)

			keys, err := collectKeys(root, []string{a, b})

			require.NoError(t, err)
			assert.Len(t, keys, 1)
		})

		t.Run("ローカル参照だけのファイルからはキーを作らない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := writeFile(t, root, ".github/workflows/a.yml", "      - uses: ./.github/actions/setup-postgres\n")

			keys, err := collectKeys(root, []string{path})

			require.NoError(t, err)
			assert.Empty(t, keys)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できない記法があればキーを返さずファイル名付きで報告する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			body := "      - uses: actions/checkout@v7.0.0\n" +
				"      - {name: Init, uses: github/codeql-action/init@v4}\n"
			path := writeFile(t, root, ".github/workflows/a.yml", body)

			keys, err := collectKeys(root, []string{path})

			require.ErrorIs(t, err, errLooseUses)
			require.ErrorContains(t, err, ".github/workflows/a.yml: github/codeql-action/init@v4")
			assert.Nil(t, keys, "fail-close 時は解決対象を返さない")
		})

		t.Run("読み取れないファイルがあればエラーを返す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()

			_, err := collectKeys(root, []string{filepath.Join(root, "absent.yml")})

			require.ErrorContains(t, err, "absent.yml")
		})
	})
}

func Test_looseUsesValue(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ローカル参照は固定対象でないので報告しない", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(" ./.github/actions/setup-postgres}")
			assert.False(t, loose)
			assert.Empty(t, note)
		})

		t.Run("版を持たない参照は誤検知を避けて報告しない", func(t *testing.T) {
			t.Parallel()
			_, loose := looseUsesValue(" actions/checkout}")
			assert.False(t, loose)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("flow mapping の閉じ括弧より後ろを値に含めない", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(" actions/checkout@v7.0.0}")
			assert.True(t, loose)
			assert.Equal(t, "actions/checkout@v7.0.0", note)
		})

		t.Run("後続のキーがあってもカンマまでを値とする", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(" actions/checkout@v7.0.0, name: Checkout}")
			assert.True(t, loose)
			assert.Equal(t, "actions/checkout@v7.0.0", note)
		})

		t.Run("行内コメントは値に含めない", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(" actions/checkout@v7.0.0 # 固定対象")
			assert.True(t, loose)
			assert.Equal(t, "actions/checkout@v7.0.0", note)
		})

		t.Run("クオートは値に含めない", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(` "actions/checkout@v7.0.0"`)
			assert.True(t, loose)
			assert.Equal(t, "actions/checkout@v7.0.0", note)
		})

		t.Run("値が空なら同じ行に値が無いものとして報告する", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue("   ")
			assert.True(t, loose)
			assert.Equal(t, "uses: の値が同じ行にありません", note)
		})

		t.Run("ブロックスカラーは同じ行に値が無いものとして報告する", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(" >-")
			assert.True(t, loose)
			assert.Equal(t, "uses: の値が同じ行にありません", note)
		})

		t.Run("YAML alias は解決できないものとして報告する", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(" *checkout")
			assert.True(t, loose)
			assert.Contains(t, note, "alias")
		})

		t.Run("YAML anchor は解決できないものとして報告する", func(t *testing.T) {
			t.Parallel()
			note, loose := looseUsesValue(" &checkout actions/checkout@v7.0.0")
			assert.True(t, loose)
			assert.Contains(t, note, "anchor")
		})
	})
}

func Test_uniq(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("隣接する重複を 1 件に畳む", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, []string{"a", "b"}, uniq([]string{"a", "a", "b"}))
		})

		t.Run("重複が無ければ全件をそのまま返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, []string{"a", "b", "c"}, uniq([]string{"a", "b", "c"}))
		})

		t.Run("空なら空を返す", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, uniq(nil))
		})

		t.Run("隣接していない重複は畳まない（呼び出し側の整列が前提）", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, []string{"a", "b", "a"}, uniq([]string{"a", "b", "a"}))
		})
	})
}

func Test_rel(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("報告用に root からの相対パスへ直す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			assert.Equal(t,
				filepath.Join(".github", "workflows", "a.yml"),
				rel(root, filepath.Join(root, ".github", "workflows", "a.yml")))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("相対化できなければ元のパスをそのまま返す", func(t *testing.T) {
			t.Parallel()
			abs := filepath.Join(t.TempDir(), "a.yml")
			assert.Equal(t, abs, rel("relative/root", abs))
		})
	})
}

func Test_daysSince(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("経過日数を切り捨てで返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, 30, daysSince(time.Now().Add(-30*24*time.Hour)))
		})

		t.Run("24 時間に満たなければ 0 日として検疫対象にする", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, 0, daysSince(time.Now().Add(-23*time.Hour)))
		})

		t.Run("未来の日時でも古いとは見なさない", func(t *testing.T) {
			t.Parallel()
			assert.LessOrEqual(t, daysSince(time.Now().Add(24*time.Hour)), 0)
		})
	})
}

func Test_writeLock(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("書き出した lockfile をそのまま読み戻せる", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "actions-pin.toml")

			require.NoError(t, writeLock(path, testLock()))

			got, err := readLock(path)
			require.NoError(t, err)
			assert.Equal(t, testLock(), got)
		})

		t.Run("キーを昇順に並べて差分レビューが安定するようにする", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "actions-pin.toml")
			require.NoError(t, writeLock(path, testLock()))

			data, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir() 由来
			require.NoError(t, err)

			body := string(data)
			assert.Less(t,
				strings.Index(body, "actions/checkout@v7.0.0"),
				strings.Index(body, "github/codeql-action@v4"))
		})

		t.Run("参照が無ければエントリを 1 件も持たない lockfile になる", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "actions-pin.toml")

			require.NoError(t, writeLock(path, map[string]string{}))

			got, err := readLock(path)
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("書き込めないパスならエラーを返す", func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "absent-dir", "actions-pin.toml")

			require.Error(t, writeLock(path, testLock()))
		})
	})
}

//nolint:paralleltest // GITHUB_TOKEN の付与を検証するため t.Setenv を使用しており並列化できない
func Test_githubGet(t *testing.T) {
	type payload struct {
		Login string `json:"login"`
	}

	//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
	t.Run("正常系", func(t *testing.T) {
		//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
		t.Run("200 なら本文をデコードしてステータスを返す", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"login":"actions"}`))
			}))
			defer srv.Close()

			var out payload
			st, err := githubGet(t.Context(), srv.URL, &out)

			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, st)
			assert.Equal(t, "actions", out.Login)
		})

		//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
		t.Run("GitHub の JSON メディアタイプを Accept に付ける", func(t *testing.T) {
			var accept string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				accept = r.Header.Get("Accept")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			var out payload
			_, err := githubGet(t.Context(), srv.URL, &out)

			require.NoError(t, err)
			assert.Equal(t, "application/vnd.github+json", accept)
		})

		//nolint:paralleltest // t.Setenv を使用するため並列化できない
		t.Run("GITHUB_TOKEN があれば Authorization を付けてレート制限を避ける", func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", "test-token")

			var auth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth = r.Header.Get("Authorization")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			var out payload
			_, err := githubGet(t.Context(), srv.URL, &out)

			require.NoError(t, err)
			assert.Equal(t, "Bearer test-token", auth)
		})

		//nolint:paralleltest // t.Setenv を使用するため並列化できない
		t.Run("GITHUB_TOKEN が空なら Authorization を付けない", func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", "")

			var auth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth = r.Header.Get("Authorization")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			var out payload
			_, err := githubGet(t.Context(), srv.URL, &out)

			require.NoError(t, err)
			assert.Empty(t, auth)
		})
	})

	//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
	t.Run("異常系", func(t *testing.T) {
		//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
		t.Run("404 は本文を読まずステータスだけを返す", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"login":"never-decoded"}`))
			}))
			defer srv.Close()

			var out payload
			st, err := githubGet(t.Context(), srv.URL, &out)

			require.NoError(t, err)
			assert.Equal(t, http.StatusNotFound, st)
			assert.Empty(t, out.Login)
		})

		//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
		t.Run("200 でも本文が JSON でなければエラーを返す", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("not json"))
			}))
			defer srv.Close()

			var out payload
			st, err := githubGet(t.Context(), srv.URL, &out)

			require.Error(t, err)
			assert.Equal(t, http.StatusOK, st)
		})

		//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
		t.Run("接続できなければエラーを返す", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			url := srv.URL
			srv.Close()

			var out payload
			_, err := githubGet(t.Context(), url, &out)

			require.Error(t, err)
		})

		//nolint:paralleltest // 親が t.Setenv を使用するため並列化できない
		t.Run("リクエストを組み立てられない URL はステータスを返さずエラーにする", func(t *testing.T) {
			var out payload
			st, err := githubGet(t.Context(), "://no-scheme", &out)

			require.Error(t, err)
			assert.Zero(t, st, "組み立てに失敗した以上 200 と誤読されうる値を返してはならない")
		})
	})
}

func Test_refAgeDays(t *testing.T) {
	t.Parallel()

	const repo = "actions/checkout"
	const tag = "v7.0.0"

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("Release と commit のうち新しい方の日数を返す", func(t *testing.T) {
			t.Parallel()
			base := githubTimesStub(t, 100, 10)

			age, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.NoError(t, err)
			assert.Equal(t, 10, age)
		})

		t.Run("Release の方が新しければ Release の日数を返す", func(t *testing.T) {
			t.Parallel()
			base := githubTimesStub(t, 5, 100)

			age, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.NoError(t, err)
			assert.Equal(t, 5, age)
		})

		t.Run("Release が無ければ commit の日数を返す", func(t *testing.T) {
			t.Parallel()
			base := githubTimesStub(t, absentRelease, 30)

			age, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.NoError(t, err)
			assert.Equal(t, 30, age)
		})

		t.Run("スラッシュを含む tag はエスケープして問い合わせる", func(t *testing.T) {
			t.Parallel()
			var gotPath string
			base := githubAPIStub(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/commits/") {
					writeCommitDate(w, 1)
					return
				}
				gotPath = r.URL.EscapedPath()
				w.WriteHeader(http.StatusNotFound)
			})

			_, err := refAgeDays(t.Context(), base, repo, "release/v1", shaCheckout)

			require.NoError(t, err)
			assert.Equal(t, "/repos/actions/checkout/releases/tags/release%2Fv1", gotPath,
				"エスケープを欠くと別リソースを問い合わせることになる")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("Release が 200 でも 404 でもなければ状態エラーを返す", func(t *testing.T) {
			t.Parallel()
			base := githubAPIStub(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})

			_, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.ErrorIs(t, err, errGitHubAPIStatus)
			require.ErrorContains(t, err, "releases/tags/"+tag)
		})

		t.Run("commit が 200 でなければ状態エラーを返す", func(t *testing.T) {
			t.Parallel()
			base := githubAPIStub(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			})

			_, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.ErrorIs(t, err, errGitHubAPIStatus)
			require.ErrorContains(t, err, "commits/"+shaCheckout)
		})

		t.Run("Release も commit も日時を持たなければ日時不明エラーを返す", func(t *testing.T) {
			t.Parallel()
			base := githubAPIStub(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/commits/") {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})

			_, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.ErrorIs(t, err, errRefDateUnavailable)
		})

		t.Run("Release の取得で通信に失敗したらエラーを返す", func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			base := srv.URL
			srv.Close()

			_, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.ErrorContains(t, err, "/releases/tags/"+tag)
		})

		t.Run("commit の取得で通信に失敗したらエラーを返す", func(t *testing.T) {
			t.Parallel()
			base := githubAPIStub(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, "/commits/") {
					writePublishedAt(w, 100)
					return
				}
				if conn, _, err := http.NewResponseController(w).Hijack(); err == nil {
					_ = conn.Close()
				}
			})

			_, err := refAgeDays(t.Context(), base, repo, tag, shaCheckout)

			require.ErrorContains(t, err, "/commits/"+shaCheckout)
		})
	})
}

//nolint:paralleltest // useGitStub が t.Setenv を使うため並列化できない
func Test_resolveSHA(t *testing.T) {
	//nolint:paralleltest // 親が t.Setenv を使うため並列化できない
	t.Run("正常系", func(t *testing.T) {
		//nolint:paralleltest // 親が t.Setenv を使うため並列化できない
		t.Run("ls-remote の出力から tag に対応する SHA を返す", func(t *testing.T) {
			useGitStub(t)

			sha, err := resolveSHA(t.Context(), "actions/checkout", "v7.0.0")

			require.NoError(t, err)
			assert.Equal(t, shaCheckout, sha)
		})
	})

	//nolint:paralleltest // 親が t.Setenv を使うため並列化できない
	t.Run("異常系", func(t *testing.T) {
		//nolint:paralleltest // 親が t.Setenv を使うため並列化できない
		t.Run("ls-remote が失敗したら SHA を返さずエラーを伝播する", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			sha, err := resolveSHA(ctx, "actions/checkout", "v7.0.0")

			require.ErrorIs(t, err, context.Canceled)
			assert.Empty(t, sha)
		})
	})
}

func Test_rewritePlan_validate(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できない記法も未登録も孤児も無ければ書き込みを許す", func(t *testing.T) {
			t.Parallel()
			plan := &rewritePlan{used: map[string]bool{
				"actions/checkout@v7.0.0": true,
				"actions/setup-go@v6":     true,
				"github/codeql-action@v4": true,
			}}

			require.NoError(t, plan.validate(testLock()))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できない記法は未登録より先に報告する", func(t *testing.T) {
			t.Parallel()
			plan := &rewritePlan{
				loose:   []string{"a.yml: actions/checkout@v7.0.0"},
				missing: []string{"some/action@v1.2.3"},
			}

			err := plan.validate(map[string]string{})

			require.ErrorIs(t, err, errLooseUses)
			require.ErrorContains(t, err, "a.yml: actions/checkout@v7.0.0")
		})

		t.Run("未登録参照は孤児より先に報告する", func(t *testing.T) {
			t.Parallel()
			plan := &rewritePlan{missing: []string{"some/action@v1.2.3"}}

			err := plan.validate(testLock())

			require.ErrorIs(t, err, errLockMissingKey)
			require.ErrorContains(t, err, "some/action@v1.2.3")
		})

		t.Run("参照されない lockfile エントリは孤児として報告する", func(t *testing.T) {
			t.Parallel()
			plan := &rewritePlan{used: map[string]bool{"actions/setup-go@v6": true}}

			err := plan.validate(testLock())

			require.ErrorIs(t, err, errLockOrphanKey)
			require.ErrorContains(t, err, "actions/checkout@v7.0.0")
			require.ErrorContains(t, err, "github/codeql-action@v4")
		})

		t.Run("同じ未登録参照が複数あっても 1 度だけ報告する", func(t *testing.T) {
			t.Parallel()
			plan := &rewritePlan{missing: []string{"some/action@v1.2.3", "some/action@v1.2.3"}}

			err := plan.validate(map[string]string{})

			require.ErrorIs(t, err, errLockMissingKey)
			assert.Equal(t, 1, strings.Count(err.Error(), "some/action@v1.2.3"))
		})
	})
}

func Test_applyOrCheck(t *testing.T) {
	t.Parallel()

	lock := map[string]string{"actions/setup-go@v6": shaSetupGo}
	pinned := "      - uses: actions/setup-go@" + shaSetupGo + " # v6\n"

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("apply は lockfile の SHA でファイルを固定する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)
			path := writeFile(t, root, ".github/workflows/a.yml", "      - uses: actions/setup-go@v6\n")

			require.NoError(t, applyOrCheck(root, []string{path}, false))

			data, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir() 由来
			require.NoError(t, err)
			assert.Equal(t, pinned, string(data))
		})

		t.Run("check は固定済みなら作業ツリーを書き換えない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)
			path := writeFile(t, root, ".github/workflows/a.yml", pinned)

			require.NoError(t, applyOrCheck(root, []string{path}, true))

			data, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir() 由来
			require.NoError(t, err)
			assert.Equal(t, pinned, string(data))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("check は未固定の参照を検出して失敗し、作業ツリーを書き換えない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)
			unpinned := "      - uses: actions/setup-go@v6\n"
			path := writeFile(t, root, ".github/workflows/a.yml", unpinned)

			err := applyOrCheck(root, []string{path}, true)

			require.ErrorIs(t, err, errPinDrift)
			require.ErrorContains(t, err, filepath.Join(".github", "workflows", "a.yml"))

			data, readErr := os.ReadFile(path) //nolint:gosec // path は t.TempDir() 由来
			require.NoError(t, readErr)
			assert.Equal(t, unpinned, string(data))
		})

		t.Run("check は lockfile と食い違う SHA を drift として検出する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)
			stale := "      - uses: actions/setup-go@" + shaCheckout + " # v6\n"
			path := writeFile(t, root, ".github/workflows/a.yml", stale)

			require.ErrorIs(t, applyOrCheck(root, []string{path}, true), errPinDrift)
		})

		t.Run("lockfile を読めなければ固定を試みずに失敗する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := writeFile(t, root, ".github/workflows/a.yml", "      - uses: actions/setup-go@v6\n")

			require.Error(t, applyOrCheck(root, []string{path}, false))
		})

		t.Run("apply で書き込めなければ失敗する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)
			path := writeFile(t, root, ".github/workflows/a.yml", "      - uses: actions/setup-go@v6\n")
			require.NoError(t, os.Chmod(path, 0o400))
			t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

			if os.Geteuid() == 0 {
				t.Skip("特権実行では読み取り専用にしても書き込みが通ってしまい、失敗を作れない")
			}

			require.Error(t, applyOrCheck(root, []string{path}, false))
		})
	})
}

//nolint:paralleltest // useGitStub が t.Setenv を使うため並列化できない
func Test_resolve(t *testing.T) {
	const uses = "      - uses: actions/checkout@v7.0.0\n"

	//nolint:paralleltest // 親が t.Setenv を使うため並列化できない
	t.Run("正常系", func(t *testing.T) {
		//nolint:paralleltest // 親が t.Setenv を使うため並列化できない
		t.Run("参照されなくなったエントリは書き直しで lockfile から消える", func(t *testing.T) {
			root := t.TempDir()
			writeLockAt(t, root, testLock())

			require.NoError(t, resolve(root, "", nil, 0))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Empty(t, got, "走査で参照が見つからなければ lockfile は空になる")
		})

		//nolint:paralleltest // 親が t.Setenv を使うため並列化できない
		t.Run("ローカル参照しか無いファイルからはエントリを作らない", func(t *testing.T) {
			root := t.TempDir()
			writeLockAt(t, root, map[string]string{})
			path := writeFile(t, root, ".github/workflows/a.yml", "      - uses: ./.github/actions/setup-postgres\n")

			require.NoError(t, resolve(root, "", []string{path}, 0))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Empty(t, got)
		})

		//nolint:paralleltest // useGitStub が t.Setenv を使うため並列化できない
		t.Run("解決した SHA を lockfile へ書き出す", func(t *testing.T) {
			root := t.TempDir()
			writeLockAt(t, root, map[string]string{})
			path := writeFile(t, root, ".github/workflows/a.yml", uses)
			useGitStub(t)

			require.NoError(t, resolve(root, "", []string{path}, 0))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"actions/checkout@v7.0.0": shaCheckout}, got)
		})

		//nolint:paralleltest // useGitStub が t.Setenv を使うため並列化できない
		t.Run("検疫期間より新しい解決先は既存ピンを維持する", func(t *testing.T) {
			root := t.TempDir()
			writeLockAt(t, root, map[string]string{"actions/checkout@v7.0.0": shaSetupGo})
			path := writeFile(t, root, ".github/workflows/a.yml", uses)
			useGitStub(t)

			require.NoError(t, resolve(root, githubTimesStub(t, 1, 1), []string{path}, 14))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"actions/checkout@v7.0.0": shaSetupGo},
				got, "出来立ての解決先を採用すると検疫が素通りする")
		})

		//nolint:paralleltest // useGitStub が t.Setenv を使うため並列化できない
		t.Run("既存ピンが無く検疫期間より新しい解決先は lockfile へ載せない", func(t *testing.T) {
			root := t.TempDir()
			writeLockAt(t, root, map[string]string{})
			path := writeFile(t, root, ".github/workflows/a.yml", uses)
			useGitStub(t)

			require.NoError(t, resolve(root, githubTimesStub(t, 1, 1), []string{path}, 14))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Empty(t, got, "退行先の無い出来立ての解決先を載せると未検証 SHA が固定される")
		})

		//nolint:paralleltest // useGitStub が t.Setenv を使うため並列化できない
		t.Run("検疫期間より古い解決先は新しい SHA を採用する", func(t *testing.T) {
			root := t.TempDir()
			writeLockAt(t, root, map[string]string{"actions/checkout@v7.0.0": shaSetupGo})
			path := writeFile(t, root, ".github/workflows/a.yml", uses)
			useGitStub(t)

			require.NoError(t, resolve(root, githubTimesStub(t, 100, 100), []string{path}, 14))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"actions/checkout@v7.0.0": shaCheckout}, got)
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
			writeFile(t, root, ".github/workflows/a.yml", "")
			writeFile(t, root, ".github/workflows/b.txt", "")

			files, err := globFiles(root, ".github/workflows/*.yml")

			require.NoError(t, err)
			assert.Equal(t, []string{filepath.Join(root, ".github/workflows/a.yml")}, files)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解釈できないパターンは 0 件へ縮退させずエラーを返す", func(t *testing.T) {
			t.Parallel()

			files, err := globFiles(t.TempDir(), ".github/workflows/[")

			require.ErrorIs(t, err, filepath.ErrBadPattern)
			assert.Empty(t, files)
		})
	})
}

func Test_sortedChangePaths(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("map の反復順によらず昇順で返す", func(t *testing.T) {
			t.Parallel()
			changes := map[string]string{
				"/repo/e.yml": "", "/repo/c.yml": "", "/repo/a.yml": "",
				"/repo/f.yml": "", "/repo/b.yml": "", "/repo/d.yml": "",
			}

			assert.Equal(t, []string{
				"/repo/a.yml", "/repo/b.yml", "/repo/c.yml",
				"/repo/d.yml", "/repo/e.yml", "/repo/f.yml",
			}, sortedChangePaths(changes))
		})
	})
}

func Test_relAll(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("root からの相対パスへ並び順を保って写す", func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(string(filepath.Separator), "repo")

			got := relAll(root, []string{
				filepath.Join(root, ".github", "workflows", "b.yml"),
				filepath.Join(root, ".github", "workflows", "a.yml"),
			})

			assert.Equal(t, []string{
				filepath.Join(".github", "workflows", "b.yml"),
				filepath.Join(".github", "workflows", "a.yml"),
			}, got)
		})
	})
}

// writeLockAt は root 配下の既定パスへ lockfile を書き出す。
func writeLockAt(t *testing.T, root string, lock map[string]string) {
	t.Helper()
	path := filepath.Join(root, lockFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, writeLock(path, lock))
}

// useGitStub は実 git を呼ばずに ls-remote の出力を差し替えるダミーを PATH 先頭へ載せる。
// testLock の全 tag を 1 度に出力するので、呼び出し側が要求した tag だけを選べているかも合わせて検証できる。
func useGitStub(t *testing.T) {
	t.Helper()
	out := shaCheckout + "\trefs/tags/v7.0.0\n" +
		shaSetupGo + "\trefs/tags/v6\n" +
		shaCodeQL + "\trefs/tags/v4\n"
	dir := t.TempDir()
	script := "#!/bin/sh\ncat <<'GITSTUB'\n" + out + "GITSTUB\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755)) //nolint:gosec // 実行可能スタブ
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// githubAPIStub は GitHub API の代わりに応答するテストサーバを立て、その base URL を返す。
// keep-alive は無効にする。githubGet が使う http.DefaultClient の接続プールはプロセス共有で、
// httptest.Server.Close() はそのプールの idle 接続をまとめて閉じるため、接続が残っていると
// 別の並列サブテストのサーバ終了が進行中のリクエストを切る。
func githubAPIStub(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.Config.SetKeepAlivesEnabled(false)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

// githubTimesStub は releases/tags に publishedAgo 日前、commits に committedAgo 日前を返す
// スタブを立てる。日数が absentRelease なら該当エンドポイントは 404 を返す。
func githubTimesStub(t *testing.T, publishedAgo, committedAgo int) string {
	t.Helper()
	return githubAPIStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			if committedAgo == absentRelease {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			writeCommitDate(w, committedAgo)
			return
		}
		if publishedAgo == absentRelease {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writePublishedAt(w, publishedAgo)
	})
}

// writePublishedAt は releases/tags の応答として daysAgo 日前の published_at を書く。
func writePublishedAt(w http.ResponseWriter, daysAgo int) {
	_, _ = fmt.Fprintf(w, `{"published_at":%q}`, agoRFC3339(daysAgo))
}

// writeCommitDate は commits の応答として daysAgo 日前の committer date を書く。
func writeCommitDate(w http.ResponseWriter, daysAgo int) {
	_, _ = fmt.Fprintf(w, `{"commit":{"committer":{"date":%q}}}`, agoRFC3339(daysAgo))
}

// agoRFC3339 は n 日前の時刻を RFC3339 で返す。
// daysSince が切り捨てで n を返すよう、境界のちょうど上ではなく僅かに古い側へ寄せる。
func agoRFC3339(n int) string {
	return time.Now().Add(-time.Duration(n)*hoursPerDay*time.Hour - time.Minute).Format(time.RFC3339)
}

// stubWD は、固定のディレクトリを返す作業ディレクトリの取得手段です。
func stubWD(root string) func() (string, error) {
	return func() (string, error) { return root, nil }
}

func Test_run(t *testing.T) {
	t.Parallel()

	lock := map[string]string{"actions/setup-go@v6": shaSetupGo}
	unpinned := "      - uses: actions/setup-go@v6\n"
	pinned := "      - uses: actions/setup-go@" + shaSetupGo + " # v6\n"

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("resolve は走査結果で lockfile を書き直す", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, testLock())

			require.NoError(t, run([]string{"resolve"}, stubWD(root), ""))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Empty(t, got, "resolve 以外へ振り分けると lockfile が据え置かれる")
		})

		t.Run("apply は lockfile の SHA でファイルを固定する", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)
			path := writeFile(t, root, ".github/workflows/a.yml", unpinned)

			require.NoError(t, run([]string{"apply"}, stubWD(root), ""))

			data, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir() 由来
			require.NoError(t, err)
			assert.Equal(t, pinned, string(data))
		})

		t.Run("check は作業ツリーも lockfile も書き換えない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)
			path := writeFile(t, root, ".github/workflows/a.yml", pinned)

			require.NoError(t, run([]string{"check"}, stubWD(root), ""))

			data, err := os.ReadFile(path) //nolint:gosec // path は t.TempDir() 由来
			require.NoError(t, err)
			assert.Equal(t, pinned, string(data))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Equal(t, lock, got, "resolve へ振り分けると lockfile が書き直される")
		})

		t.Run("ヘルプ要求は失敗にしない", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeLockAt(t, root, lock)

			require.NoError(t, run([]string{"resolve", "-h"}, stubWD(root), ""))

			got, err := readLock(filepath.Join(root, lockFile))
			require.NoError(t, err)
			assert.Equal(t, lock, got, "ヘルプ要求で resolve まで進むと lockfile が書き直される")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("サブコマンドが無ければ使い方を返す", func(t *testing.T) {
			t.Parallel()

			err := run(nil, stubWD(t.TempDir()), "")

			require.ErrorIs(t, err, errUsage)
		})

		t.Run("未知のサブコマンドは使い方を返す", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"bogus"}, stubWD(t.TempDir()), "")

			require.ErrorIs(t, err, errUsage)
		})

		t.Run("作業ディレクトリを取得できなければ失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"check"}, func() (string, error) { return "", errWD }, "")

			require.ErrorIs(t, err, errWD)
			assert.ErrorContains(t, err, "getwd")
		})

		t.Run("走査対象を集められなければ失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"check"}, stubWD(filepath.Join(t.TempDir(), "x[")), "")

			require.ErrorIs(t, err, filepath.ErrBadPattern)
		})

		t.Run("未知のフラグはヘルプ要求と混同せず失敗する", func(t *testing.T) {
			t.Parallel()

			err := run([]string{"resolve", "-bogus"}, stubWD(t.TempDir()), "")

			require.ErrorContains(t, err, "failed to parse flags")
		})
	})
}
