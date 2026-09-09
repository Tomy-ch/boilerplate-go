package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outdatedNow は齢の基準時刻。相対日数で書けるよう固定する。
var outdatedNow = time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)

// daysAgo は基準時刻から n 日前の RFC3339 文字列を返す。
func daysAgo(n int) string {
	return outdatedNow.AddDate(0, 0, -n).Format(time.RFC3339)
}

// upstreamStub は RequestURI ごとに応答を引く上流を立て、そこへ向いたクライアントを返す。
// 上流の URL は定数なので、fakeUpstream の transport 差し替えでホストごと寄せる。
func upstreamStub(t *testing.T, routes map[string]string) *http.Client {
	t.Helper()

	return fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.RequestURI()]
		if !ok {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		_, _ = w.Write([]byte(body))
	})
}

func Test_stableVersion(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("数値だけの区切りは安定版とみなす", func(t *testing.T) {
			t.Parallel()

			assert.True(t, stableVersion("1.2.3"))
			assert.True(t, stableVersion("0.117.0"))
			assert.True(t, stableVersion("4"))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("プレリリース識別子を持つ版は安定版とみなさない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, stableVersion("1.2.0-rc1"))
			assert.False(t, stableVersion("0.1.0-deprecated"))
		})

		t.Run("空文字と空の区切りは安定版とみなさない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, stableVersion(""))
			assert.False(t, stableVersion("1..3"))
		})

		t.Run("npm の created や modified は版ではないので弾く", func(t *testing.T) {
			t.Parallel()

			assert.False(t, stableVersion("created"))
			assert.False(t, stableVersion("modified"))
		})
	})
}

func Test_lessVersion(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("数値として比べるので桁数では並ばない", func(t *testing.T) {
			t.Parallel()

			assert.True(t, lessVersion("1.9.0", "1.10.0"), "文字列比較なら 1.9.0 が後になる")
			assert.False(t, lessVersion("1.10.0", "1.9.0"))
		})

		t.Run("区切りの数が違えば足りない側を 0 とみなす", func(t *testing.T) {
			t.Parallel()

			assert.False(t, lessVersion("1.2", "1.2.0"))
			assert.True(t, lessVersion("1.2", "1.2.1"))
		})

		t.Run("同じ版は小さくない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, lessVersion("2.1.1", "2.1.1"))
		})
	})
}

func Test_majorOf(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("先頭の数値区切りを返し v 接頭辞は落とす", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "2", majorOf("2.13.1"))
			assert.Equal(t, "2", majorOf("v2.13.1"))
			assert.Equal(t, "0", majorOf("0.117.0"))
		})

		t.Run("区切りが無ければ全体を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "4", majorOf("4"))
		})
	})
}

func Test_evaluate(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("窓を満たす最新を選び上流最新は別に持つ", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{
				"/repos/owner/repo/releases?per_page=100": `[
					{"tag_name":"v1.3.0","published_at":"` + daysAgo(2) + `","prerelease":false,"draft":false},
					{"tag_name":"v1.2.0","published_at":"` + daysAgo(20) + `","prerelease":false,"draft":false},
					{"tag_name":"v1.1.0","published_at":"` + daysAgo(40) + `","prerelease":false,"draft":false}
				]`,
			})

			c := evaluate(t.Context(), client, tool{
				key: "aqua:owner/repo", version: "1.1.0", backend: "aqua:owner/repo",
			}, outdatedNow)

			require.NoError(t, c.err)
			assert.Equal(t, "1.3.0", c.latest, "上流最新は窓に関係なく最新")
			assert.Equal(t, "1.2.0", c.eligible, "窓 14 日を満たす最新は 1 つ前")
			assert.True(t, c.actionable())
			assert.True(t, c.heldByWindow(), "上流最新が窓の内側なので待ちも同時に立つ")
		})

		t.Run("上流最新が窓を満たしていれば待ちは立たない", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{
				"/repos/owner/repo/releases?per_page=100": `[
					{"tag_name":"v1.2.0","published_at":"` + daysAgo(20) + `","prerelease":false,"draft":false},
					{"tag_name":"v1.1.0","published_at":"` + daysAgo(40) + `","prerelease":false,"draft":false}
				]`,
			})

			c := evaluate(t.Context(), client, tool{
				key: "aqua:owner/repo", version: "1.1.0", backend: "aqua:owner/repo",
			}, outdatedNow)

			require.NoError(t, c.err)
			assert.Equal(t, "1.2.0", c.eligible)
			assert.False(t, c.heldByWindow())
		})

		t.Run("major を跨ぐ版は更新先に選ばない", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{
				"/repos/owner/repo/releases?per_page=100": `[
					{"tag_name":"v2.0.0","published_at":"` + daysAgo(30) + `","prerelease":false,"draft":false},
					{"tag_name":"v1.1.0","published_at":"` + daysAgo(40) + `","prerelease":false,"draft":false}
				]`,
			})

			c := evaluate(t.Context(), client, tool{
				key: "aqua:owner/repo", version: "1.1.0", backend: "aqua:owner/repo",
			}, outdatedNow)

			require.NoError(t, c.err)
			assert.Equal(t, "2.0.0", c.latest)
			assert.Equal(t, "1.1.0", c.eligible, "同 major の最新は現在の版そのもの")
			assert.False(t, c.actionable(), "major 跨ぎは今すぐ上げられるとは言わない")
			assert.True(t, c.heldByWindow(), "上流に新版があることは待ちとして見える")
		})

		t.Run("プレリリースと draft は候補にしない", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{
				"/repos/owner/repo/releases?per_page=100": `[
					{"tag_name":"v1.3.0-rc1","published_at":"` + daysAgo(30) + `","prerelease":true,"draft":false},
					{"tag_name":"v1.2.9","published_at":"` + daysAgo(30) + `","prerelease":false,"draft":true},
					{"tag_name":"v1.2.0","published_at":"` + daysAgo(20) + `","prerelease":false,"draft":false}
				]`,
			})

			c := evaluate(t.Context(), client, tool{
				key: "aqua:owner/repo", version: "1.1.0", backend: "aqua:owner/repo",
			}, outdatedNow)

			require.NoError(t, c.err)
			assert.Equal(t, "1.2.0", c.latest)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("上流を確認できなければ err を持たせ候補にしない", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{})

			c := evaluate(t.Context(), client, tool{
				key: "aqua:owner/repo", version: "1.1.0", backend: "aqua:owner/repo",
			}, outdatedNow)

			require.Error(t, c.err)
			assert.False(t, c.actionable())
			assert.False(t, c.heldByWindow())
		})
	})
}

func Test_goModuleVersions(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("パスがモジュールでなければ末尾を落として遡る", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{
				"/go.uber.org/mock/@v/list": "v0.6.0\nv0.5.0\n",
			})

			versions, base, err := goModuleVersions(t.Context(), client, "go.uber.org/mock/mockgen")

			require.NoError(t, err)
			assert.Equal(t, "go.uber.org/mock", base)
			assert.ElementsMatch(t, []string{"0.6.0", "0.5.0"}, versions)
		})

		t.Run("安定版を持たないモジュールでは遡らず空を返す", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{
				"/golang.org/x/tools/cmd/godoc/@v/list": "v0.1.0-deprecated\n",
				"/golang.org/x/tools/@v/list":           "v0.49.0\n",
			})

			versions, base, err := goModuleVersions(t.Context(), client, "golang.org/x/tools/cmd/godoc")

			require.NoError(t, err)
			assert.Equal(t, "golang.org/x/tools/cmd/godoc", base)
			assert.Empty(t, versions, "親モジュールの版一覧を別ツールの更新先として返してはいけない")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("どこまで遡ってもモジュールが無ければエラーを返す", func(t *testing.T) {
			t.Parallel()
			client := upstreamStub(t, map[string]string{})

			_, _, err := goModuleVersions(t.Context(), client, "example.com/nothing/here")

			require.Error(t, err)
		})
	})
}

func Test_outdatedReport(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("状態ごとに節を分けて出す", func(t *testing.T) {
			t.Parallel()
			body := outdatedReport([]candidate{
				{
					tool:   tool{key: "lefthook", version: "2.1.10", backend: "aqua:evilmartians/lefthook", file: miseFile},
					latest: "2.1.12", latestAge: 12,
					eligible: "2.1.11", eligibleAge: 19, window: releaseWindowDays,
				},
				{
					tool:   tool{key: "gofumpt", version: "0.11.0", backend: "aqua:mvdan/gofumpt", file: miseFile},
					latest: "0.12.0", latestAge: 1,
					eligible: "0.11.0", eligibleAge: 60, window: releaseWindowDays,
				},
			}, outdatedNow)

			assert.Contains(t, body, "## 今すぐ上げられる")
			assert.Contains(t, body, "`lefthook` | 2.1.10 | **2.1.11**")
			assert.Contains(t, body, "## 窓が明けるのを待っている")
			assert.Contains(t, body, "`gofumpt` | 0.11.0 | 0.12.0")
			assert.NotContains(t, body, "## 上流を確認できなかった")
		})

		t.Run("どちらも無ければ両方の節がなしになる", func(t *testing.T) {
			t.Parallel()
			body := outdatedReport([]candidate{
				{
					tool:   tool{key: "sqlfluff", version: "4.3.0", backend: "pypi:sqlfluff", file: "python/sqlfluff.in"},
					latest: "4.3.0", latestAge: 33,
					eligible: "4.3.0", eligibleAge: 33, window: registryWindowDays,
				},
			}, outdatedNow)

			assert.Equal(t, 2, strings.Count(body, "なし。"))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("上流を確認できなかったツールは専用の節に出す", func(t *testing.T) {
			t.Parallel()
			body := outdatedReport([]candidate{
				{
					tool: tool{key: "broken", version: "1.0.0", backend: "aqua:owner/repo", file: miseFile},
					err:  errNotFound,
				},
			}, outdatedNow)

			assert.Contains(t, body, "## 上流を確認できなかった")
			assert.Contains(t, body, "`broken`")
		})
	})
}
