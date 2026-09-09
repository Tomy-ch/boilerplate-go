package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-boilerplate/pkg/xerrors"
)

// maxGoProxyProbes は go backend で公開時刻を問い合わせる版数の上限。module proxy は版の一覧に
// 日付を含めないため 1 版ごとに .info を引く必要があり、遡る範囲を切らないと 1 ツールで数百回の
// 往復になる。新しい方から数えてこの数だけ見れば、窓を満たす版はまず見つかる。
const maxGoProxyProbes = 20

// release は上流が公開している版 1 つ。
type release struct {
	version string
	at      time.Time
}

// candidate は 1 ツールの棚卸し結果。latest は上流の最新、eligible は「現在の major を保ったまま
// 窓を満たす最新」で、両者が食い違うのが常態である点がこの型の要。窓の内側にある版は採れないので、
// 人が今日動かせるのは eligible の方だけになる。
type candidate struct {
	tool        tool
	latest      string
	latestAge   int
	eligible    string
	eligibleAge int
	window      int
	err         error
}

// actionable は、今日ピンを動かせる状態かどうかを返す。
func (c candidate) actionable() bool {
	return c.err == nil && c.eligible != "" && c.eligible != c.tool.version
}

// heldByWindow は、上流に新版があるのに窓がそれを止めている状態かどうかを返す。
func (c candidate) heldByWindow() bool {
	return c.err == nil && c.latest != "" && c.latest != c.tool.version && c.latest != c.eligible
}

// surveyOutdated は各ツールの上流を調べ、窓を満たす更新先を決めます。
// client は上流への問い合わせ手段、now は齢の基準時刻で、差し替えられるよう引数で受けます。
func surveyOutdated(ctx context.Context, client *http.Client, tools []tool, now time.Time) []candidate {
	out := make([]candidate, len(tools))
	sem := make(chan struct{}, fetchWorkers)
	var wg sync.WaitGroup
	for i, t := range tools {
		wg.Add(1)
		go func(i int, t tool) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = evaluate(ctx, client, t, now)
		}(i, t)
	}
	wg.Wait()

	sort.Slice(out, func(i, j int) bool { return out[i].tool.id() < out[j].tool.id() })
	return out
}

// evaluate は 1 ツールの上流を調べて candidate に畳みます。
func evaluate(ctx context.Context, client *http.Client, t tool, now time.Time) candidate {
	c := candidate{tool: t, window: windowFor(t.backend)}

	releases, err := listReleases(ctx, client, t)
	if err != nil {
		c.err = err

		return c
	}
	// 新しい版が先に来る順へ揃える。上流ごとに一覧の順序が違うため、ここで決め直さないと
	// 「最新」も「窓を満たす最新」も上流の都合で変わる。
	sort.Slice(releases, func(i, j int) bool { return lessVersion(releases[j].version, releases[i].version) })

	if len(releases) > 0 {
		c.latest = releases[0].version
		c.latestAge = ageDays(releases[0].at, now)
	}

	major := majorOf(t.version)
	for _, r := range releases {
		if majorOf(r.version) != major {
			continue
		}
		if ageDays(r.at, now) < c.window {
			continue
		}
		c.eligible = r.version
		c.eligibleAge = ageDays(r.at, now)

		break
	}

	return c
}

// listReleases は backend ごとの上流から安定版の一覧を取ります。
func listReleases(ctx context.Context, client *http.Client, t tool) ([]release, error) {
	_, ref, _ := strings.Cut(t.backend, ":")
	switch backendKind(t.backend) {
	case "github":
		return githubReleases(ctx, client, ref)
	case "go":
		return goModuleReleases(ctx, client, ref)
	case "npm":
		return npmReleases(ctx, client, ref)
	case "pypi":
		return pypiReleases(ctx, client, extrasRe.ReplaceAllString(ref, ""))
	default:
		return nil, xerrors.Wrap(errUnsupportedBackend, t.backend)
	}
}

func githubReleases(ctx context.Context, client *http.Client, repo string) ([]release, error) {
	var body []struct {
		//nolint:tagliatelle // GitHub API の応答フィールド名
		TagName string `json:"tag_name"`
		//nolint:tagliatelle // GitHub API の応答フィールド名
		PublishedAt time.Time `json:"published_at"`
		Prerelease  bool      `json:"prerelease"`
		Draft       bool      `json:"draft"`
	}
	if err := getJSON(ctx, client, githubAPI+repo+"/releases?per_page=100", &body); err != nil {
		return nil, err
	}

	out := make([]release, 0, len(body))
	for _, r := range body {
		if r.Prerelease || r.Draft {
			continue
		}
		v := strings.TrimPrefix(r.TagName, "v")
		if !stableVersion(v) {
			continue
		}
		out = append(out, release{version: v, at: r.PublishedAt})
	}

	return out, nil
}

// goModuleReleases は module proxy の版一覧を新しい方から maxGoProxyProbes 件だけ日付付きにします。
// proxy の一覧に日付が無く 1 版ごとに .info が要るため、全件を引くと往復が跳ね上がります。
func goModuleReleases(ctx context.Context, client *http.Client, pkg string) ([]release, error) {
	versions, base, err := goModuleVersions(ctx, client, pkg)
	if err != nil {
		return nil, err
	}
	sort.Slice(versions, func(i, j int) bool { return lessVersion(versions[j], versions[i]) })
	if len(versions) > maxGoProxyProbes {
		versions = versions[:maxGoProxyProbes]
	}

	out := make([]release, 0, len(versions))
	for _, v := range versions {
		at, atErr := goModuleAt(ctx, client, base, v)
		if atErr != nil {
			continue
		}
		out = append(out, release{version: v, at: at})
	}

	return out, nil
}

// goModuleVersions は @v/list を引き、解決できたモジュールパスも返します。mise の go backend は
// パッケージパスを受けるのに proxy が知るのはモジュールパスなので、末尾要素を落としながら遡ります。
//
// 遡るのは proxy がそのパスをモジュールとして知らなかったときだけです。応答が返った時点でそこが
// モジュールで、安定版を 1 つも持たなくても答えは「無い」であって親のものではありません。
// `golang.org/x/tools/cmd/godoc` は v0.1.0-deprecated しか持たない独立モジュールで、空を理由に
// 遡ると親 `golang.org/x/tools` の版一覧を、この別物のツールの更新先として差し出すことになります。
func goModuleVersions(ctx context.Context, client *http.Client, pkg string) ([]string, string, error) {
	var lastErr error
	for path := pkg; strings.Contains(path, "/"); path = path[:strings.LastIndex(path, "/")] {
		body, err := getText(ctx, client, goProxyBase+escapeModulePath(path)+"/@v/list")
		if err != nil {
			lastErr = err

			continue
		}
		var versions []string
		for line := range strings.FieldsSeq(body) {
			v := strings.TrimPrefix(line, "v")
			if stableVersion(v) {
				versions = append(versions, v)
			}
		}

		return versions, path, nil
	}
	if lastErr == nil {
		lastErr = xerrors.Wrap(errNotFound, pkg)
	}

	return nil, "", lastErr
}

func npmReleases(ctx context.Context, client *http.Client, pkg string) ([]release, error) {
	var body struct {
		Time map[string]time.Time `json:"time"`
	}
	if err := getJSON(ctx, client, npmBase+pkg, &body); err != nil {
		return nil, err
	}

	out := make([]release, 0, len(body.Time))
	for v, at := range body.Time {
		// created / modified は版ではなくパッケージ全体の時刻なので、版として数えない。
		if !stableVersion(v) {
			continue
		}
		out = append(out, release{version: v, at: at})
	}

	return out, nil
}

func pypiReleases(ctx context.Context, client *http.Client, pkg string) ([]release, error) {
	var body struct {
		Releases map[string][]struct {
			//nolint:tagliatelle // PyPI の応答フィールド名
			UploadTime time.Time `json:"upload_time_iso_8601"`
		} `json:"releases"`
	}
	if err := getJSON(ctx, client, pypiBase+pkg+"/json", &body); err != nil {
		return nil, err
	}

	out := make([]release, 0, len(body.Releases))
	for v, files := range body.Releases {
		// 配布物を 1 つも持たない版は yank 済みで、install できないため候補にしない。
		if len(files) == 0 || !stableVersion(v) {
			continue
		}
		out = append(out, release{version: v, at: files[0].UploadTime})
	}

	return out, nil
}

// stableVersion は数値だけの区切りからなる版かどうかを返します。プレリリース識別子やビルド
// メタデータを持つ版を弾くのが目的で、ここを通さないと `1.2.0-rc1` が最新として選ばれます。
func stableVersion(v string) bool {
	if v == "" {
		return false
	}
	for part := range strings.SplitSeq(v, ".") {
		if part == "" {
			return false
		}
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}

	return true
}

// lessVersion は数値区切りを左から比べます。区切りの数が違う場合は足りない側を 0 とみなすので、
// `1.2` は `1.2.0` と同じ位置に来ます。
func lessVersion(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := 0, 0
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x < y
		}
	}

	return false
}

// majorOf は先頭の数値区切りを返します。比較ではなく分類に使うので、解釈できない版は
// 元の文字列をそのまま返し、他の major と混ざらないようにします。
func majorOf(v string) string {
	head, _, found := strings.Cut(strings.TrimPrefix(v, "v"), ".")
	if !found {
		return strings.TrimPrefix(v, "v")
	}

	return head
}

func ageDays(at, now time.Time) int {
	return int(now.Sub(at).Hours() / hoursPerDay)
}

// outdatedReport は棚卸し結果を Markdown にします。issue の本文としてそのまま使うため、
// 見出しから始めず、状態ごとの節だけを並べます。
func outdatedReport(cands []candidate, now time.Time) string {
	var actionable, held, failed []candidate
	for _, c := range cands {
		switch {
		case c.err != nil:
			failed = append(failed, c)
		case c.actionable():
			actionable = append(actionable, c)
		case c.heldByWindow():
			held = append(held, c)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "棚卸し時刻: `%s`\n", now.Format(time.RFC3339))
	fmt.Fprintf(&b, "窓: GitHub リリース %d 日 / パッケージレジストリ %d 日\n\n",
		releaseWindowDays, registryWindowDays)

	b.WriteString("## 今すぐ上げられる\n\n")
	if len(actionable) == 0 {
		b.WriteString("なし。\n\n")
	} else {
		b.WriteString("| ツール | 現在 | 窓を満たす最新 | 齢 | 宣言 |\n| --- | --- | --- | --- | --- |\n")
		for _, c := range actionable {
			fmt.Fprintf(&b, "| `%s` | %s | **%s** | %dd | `%s` |\n",
				c.tool.key, c.tool.version, c.eligible, c.eligibleAge, c.tool.file)
		}
		b.WriteString("\n`/tools-upgrade` が同じ窓でこの表を再計算し、宣言と lockfile を更新します。\n\n")
	}

	b.WriteString("## 窓が明けるのを待っている\n\n")
	if len(held) == 0 {
		b.WriteString("なし。\n\n")
	} else {
		b.WriteString("| ツール | 現在 | 上流最新 | 齢 | 窓 |\n| --- | --- | --- | --- | --- |\n")
		for _, c := range held {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %dd | %dd |\n",
				c.tool.key, c.tool.version, c.latest, c.latestAge, c.window)
		}
		b.WriteString("\n上流の最新が窓の内側にあるものです。齢が窓に届けば次回から上の表へ移ります。\n")
		b.WriteString("major 跨ぎもここに出ます（同じ major に窓を満たす新版が無いため）。\n\n")
	}

	if len(failed) > 0 {
		b.WriteString("## 上流を確認できなかった\n\n")
		for _, c := range failed {
			fmt.Fprintf(&b, "- `%s`（%s）: %v\n", c.tool.key, c.tool.backend, c.err)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// reportOutdated は棚卸しを実行し、結果を標準出力と（指定があれば）ファイルへ書きます。
// gate / audit と違い違反という概念が無いため、上流を確認できなかった場合だけ失敗させます。
func reportOutdated(client *http.Client, declared []tool, opt options, now time.Time) error {
	ctx := context.Background()
	resolved, skipped, err := resolveBackends(ctx, declared)
	if err != nil {
		return xerrors.Wrap(err, "❌ backend の解決")
	}

	cands := surveyOutdated(ctx, client, resolved, now)
	body := outdatedReport(cands, now)

	var actionable, held, failed int
	for _, c := range cands {
		switch {
		case c.err != nil:
			failed++
		case c.actionable():
			actionable++
		case c.heldByWindow():
			held++
		}
	}

	log.Printf("ℹ️ tool-cooldown outdated: 対象 %d 件 / ランタイム除外 %d 件 / 窓 GitHub %d 日・レジストリ %d 日",
		len(resolved), len(skipped), releaseWindowDays, registryWindowDays)
	log.Printf("ℹ️ 今すぐ上げられる %d 件 / 窓待ち %d 件 / 上流を確認できず %d 件", actionable, held, failed)

	if opt.summaryOut != "" {
		if writeErr := os.WriteFile(opt.summaryOut, []byte(body), summaryPerm); writeErr != nil {
			return xerrors.Wrap(writeErr, "❌ write summary")
		}
	}
	// 起票するかどうかは呼び出し側の判断なので、件数だけを渡す。
	if out := os.Getenv("GITHUB_OUTPUT"); out != "" {
		if writeErr := appendOutdatedOutput(out, actionable, held, failed); writeErr != nil {
			return xerrors.Wrap(writeErr, "❌ write GITHUB_OUTPUT")
		}
	}

	return nil
}

func appendOutdatedOutput(path string, actionable, held, failed int) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, outputPerm) //nolint:gosec // path は GITHUB_OUTPUT（ランナー提供）
	if err != nil {
		return xerrors.Wrap(err, "open "+path)
	}
	defer func() { _ = f.Close() }()

	_, err = fmt.Fprintf(f, "actionable=%d\nheld=%d\nfailed=%d\n", actionable, held, failed)

	return xerrors.Wrap(err, "write "+path)
}
