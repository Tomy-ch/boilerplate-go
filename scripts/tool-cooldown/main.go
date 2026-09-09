// Package main はこのリポジトリが宣言するツールのバージョンが供給網 cooldown を満たしているかを
// 検査するツール。
//
//	gate:  base からの差分で追加 / 更新されたツールのうち、公開から窓日数未満のものを違反として
//	       報告し、非ゼロで終了する。
//	audit: 宣言の全件を棚卸しし、窓内のものを報告する。窓を理由には落ちないが、バイパス自身の
//	       規約違反（期限切れ・期限過長・対象不在）では失敗する。
//
// バイパスは .github/tool-cooldown-bypass.toml が受ける。期限は宣言が変わらなくても訪れるので、
// このツールはスケジュール実行にも載せる必要がある。窓の値と規約は scripts/README.md の
// tool-cooldown の行が持つ。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-boilerplate/pkg/xerrors"
)

const (
	miseFile   = "mise.toml"
	bypassFile = ".github/tool-cooldown-bypass.toml" //nolint:gosec // 資格情報ではなくバイパス lockfile のパス
	// pyRequirementsGlob は PyPI ツールの直接宣言。解決結果は同名の .txt が持つ。
	pyRequirementsGlob  = "python/*.in"
	pyRequirementSuffix = ".in"
	pyLockSuffix        = ".txt"

	// releaseWindowDays は GitHub リリースに紐づく backend の窓。
	releaseWindowDays = 14
	// registryWindowDays はパッケージレジストリに紐づく backend の窓。
	registryWindowDays = 7

	maxBypassMonths = 3 // バイパスの期限に許す最大の先送り幅

	fetchTimeout = 30 * time.Second
	miseTimeout  = 30 * time.Second
	gitTimeout   = 30 * time.Second
	fetchWorkers = 4 // GitHub API のレート制限に配慮して go-cooldown より絞る
	minFenceLen  = 3 // CommonMark のフェンス下限
	hoursPerDay  = 24
	summaryPerm  = 0o644
	outputPerm   = 0o600

	githubAPI   = "https://api.github.com/repos/"
	goProxyBase = "https://proxy.golang.org/"
	npmBase     = "https://registry.npmjs.org/"
	pypiBase    = "https://pypi.org/pypi/"
)

var (
	// errUsage は、サブコマンドやフラグの与え方が誤っていることを表すエラー。
	errUsage = xerrors.New("invalid usage")
	// errBlocking は、gate を通せない違反が残っていることを表すエラー。
	errBlocking = xerrors.New("blocking violations")
	// errUnsupportedBackend は、公開時刻の取得経路を持たない backend を指すエラー。
	errUnsupportedBackend = xerrors.New("no publish-time source for this backend")
	// errNotFound は、上流がそのバージョンを知らない場合のエラー。
	errNotFound = xerrors.New("version not found upstream")
	// errUpstreamStatus は、上流が想定外のステータスを返した場合のエラー。
	errUpstreamStatus = xerrors.New("unexpected upstream status")
	// errBypassInvalidLine は、バイパス lockfile に解釈できない行があった場合のエラー。
	errBypassInvalidLine = xerrors.New("invalid bypass line")
	// errBypassDuplicateKey は、バイパス lockfile にキーの重複があった場合のエラー。
	errBypassDuplicateKey = xerrors.New("duplicate bypass key")

	// toolLineRe は `[tools]` の 1 行を key と version へ割る。key は裸でも引用符付きでもよい。
	toolLineRe = regexp.MustCompile(`^\s*(?:"([^"]+)"|([A-Za-z0-9_.\-]+))\s*=\s*"([^"]+)"\s*$`)
	// toolTableLineRe は tool option 付きの 1 行宣言（`key = { version = "...", ... }`）から key と version を読む。
	// 宣言の形が変わっただけでゲートから外れると、窓の検査が黙って抜ける。
	toolTableLineRe = regexp.MustCompile(
		`^\s*(?:"([^"]+)"|([A-Za-z0-9_.\-]+))\s*=\s*\{.*\bversion\s*=\s*"([^"]+)".*\}\s*$`,
	)
	// sectionRe は TOML のセクション見出し。
	sectionRe = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*$`)
	// extrasRe は extras 表記（`graphifyy[sql]` の `[sql]`）。
	extrasRe = regexp.MustCompile(`\[[^\]]*\]$`)
	// pyRequirementRe は requirements の 1 行を名前（extras 込み）と版へ割る。`==` で固定された
	// 行だけを読む。範囲指定は宣言としては版を決めておらず、cooldown を測る対象にならない。
	pyRequirementRe = regexp.MustCompile(
		`^([A-Za-z0-9][A-Za-z0-9._\-]*(?:\[[^\]]*\])?)\s*==\s*([A-Za-z0-9][A-Za-z0-9.\-+!]*)`,
	)
	// pyNameSepRe は PyPI の名前正規化（`-` `_` `.` の連続を `-` 1 つへ寄せる / PEP 503）。
	pyNameSepRe = regexp.MustCompile(`[-_.]+`)
	// bypassLineRe は `"key@version" = { expires = ..., issue = ..., reason = "..." }` を読む。
	bypassLineRe = regexp.MustCompile(
		`^"([^"]+)"\s*=\s*\{\s*expires\s*=\s*(\d{4}-\d{2}-\d{2})\s*,\s*issue\s*=\s*(\d+)\s*,\s*reason\s*=\s*"([^"]*)"\s*\}$`,
	)
)

// tool は宣言の 1 行。backend は解決後の値で、短縮名なら mise registry が決める。
type tool struct {
	key     string // 宣言に書かれたキーそのもの
	version string
	backend string // 例: aqua:owner/repo, go:module, npm:pkg, pypi:pkg, core:go
	file    string // 宣言元のパス。注釈を宣言した場所へ付けるために持つ
}

// declaration は宣言ファイル 1 つ分の中身。読み取りと解析を分けて、解析だけを純粋に保つ。
type declaration struct {
	path    string
	content []byte
}

// violation は宣言側の規約違反 1 件。file は注釈を付ける先で、違反の種類ごとに違う。
type violation struct {
	file string
	msg  string
}

// bypass は cooldown を明示的に外す 1 エントリ。
type bypass struct {
	expires time.Time
	issue   int
	reason  string
	line    int
}

// finding は窓を満たさないツール 1 件。
type finding struct {
	tool      tool
	published time.Time
	ageDays   int
	window    int
}

// options はサブコマンドに続けて与えられたフラグ。
type options struct {
	base       string
	summaryOut string
	github     bool
}

func (t tool) id() string { return t.key + "@" + t.version }

// main は 1:1 テスト規約の対象外で分岐を検査できないため、判断は run に置きます。
func main() {
	log.SetFlags(0)

	if err := run(os.Args[1:], &http.Client{Timeout: fetchTimeout}, time.Now().UTC()); err != nil {
		log.Fatalf("%v", err)
	}
}

// run は mise.toml の宣言を cooldown に照らして報告します。gate で違反が残った場合はエラーを
// 返し、終了コードへの変換は呼び出し側に委ねます。
// client は上流への問い合わせ手段、now は窓と期限の基準時刻で、差し替えられるよう引数で受けます。
func run(args []string, client *http.Client, now time.Time) error {
	sub, opt, err := parseArgs(args)
	if err != nil {
		// ヘルプ要求は失敗ではないので 0 で終える。usage は flag が既に出力している。
		if xerrors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	// 期限の比較は暦日で行う。時刻を残すと 3 ヶ月の境界が実行時刻とタイムゾーンで動く。
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	paths, err := declarationPaths()
	if err != nil {
		return xerrors.Wrap(err, "❌ 宣言ファイルの列挙")
	}
	decls, err := readDeclarations(paths)
	if err != nil {
		return xerrors.Wrap(err, "❌ 宣言ファイルの読み取り")
	}
	declared, err := parseDeclarations(decls)
	if err != nil {
		return xerrors.Wrap(err, "❌ 宣言ファイルの解析")
	}

	if sub == "outdated" {
		return reportOutdated(client, declared, opt, now)
	}

	bypasses, err := readBypasses(bypassFile)
	if err != nil {
		return xerrors.Wrap(err, "❌ "+bypassFile)
	}
	policyViolations, invalidBypasses := validateBypasses(bypasses, declared, today)
	policyViolations = append(policyViolations, verifyLocks(declared)...)

	targets := declared
	if sub == "gate" {
		targets, err = added(opt.base, paths, declared)
		if err != nil {
			return xerrors.Wrap(err, "❌ base との差分")
		}
	}

	ctx := context.Background()
	resolved, skipped, err := resolveBackends(ctx, targets)
	if err != nil {
		return xerrors.Wrap(err, "❌ backend の解決")
	}

	findings, unresolved := inspect(ctx, client, resolved, now)

	blocking := report(
		sub,
		findings,
		unresolved,
		skipped,
		policyViolations,
		bypasses,
		invalidBypasses,
		resolved,
		opt.github,
	)

	if opt.summaryOut != "" {
		body := summary(sub, findings, unresolved, skipped, policyViolations, bypasses, invalidBypasses, resolved)
		if writeErr := os.WriteFile(opt.summaryOut, []byte(body), summaryPerm); writeErr != nil {
			return xerrors.Wrap(writeErr, "❌ write summary")
		}
	}
	if out := os.Getenv("GITHUB_OUTPUT"); out != "" {
		if writeErr := appendOutput(out, len(findings), len(resolved), blocking); writeErr != nil {
			return xerrors.Wrap(writeErr, "❌ write GITHUB_OUTPUT")
		}
	}

	if blocking > 0 {
		return xerrors.Wrap(errBlocking, fmt.Sprintf("❌ tool-cooldown %s: %d 件の違反が残っています", sub, blocking))
	}

	return nil
}

// parseArgs はサブコマンドとフラグを解釈します。ヘルプ要求は flag.ErrHelp を含むエラーとして
// 返し、失敗として扱うかどうかの判断は呼び出し側に委ねます。
func parseArgs(args []string) (string, options, error) {
	if len(args) == 0 {
		return "", options{}, xerrors.Wrap(errUsage,
			"❌ usage: tool-cooldown <gate|audit|outdated> [--base=<git-ref>] [--summary-out=<path>] [--github]")
	}

	sub := args[0]
	if sub != "gate" && sub != "audit" && sub != "outdated" {
		return "", options{}, xerrors.Wrap(errUsage,
			fmt.Sprintf("❌ 未知のサブコマンド %q（gate | audit | outdated）", sub))
	}

	fs := flag.NewFlagSet(sub, flag.ContinueOnError)
	base := fs.String("base", "", "gate: この git ref からの差分で追加 / 更新されたツールだけを対象にする")
	summaryOut := fs.String("summary-out", "", "Markdown サマリの出力先")
	github := fs.Bool("github", false, "GitHub Actions のアノテーションを出力する")
	if err := fs.Parse(args[1:]); err != nil {
		return "", options{}, xerrors.Wrap(err, "❌ フラグを解釈できません")
	}

	// gate は差分を取れないと何も検査しないまま通るため、比較対象の不在を先に落とす。
	if sub == "gate" && *base == "" {
		return "", options{}, xerrors.Wrap(errUsage, "❌ gate には --base が要ります（比較対象が無いと差分を取れません）")
	}

	return sub, options{base: *base, summaryOut: *summaryOut, github: *github}, nil
}

// parseTools は `[tools]` セクションの宣言だけを読む。`[settings]` の `pipx.uvx` や `[env]` の
// バージョン値をツールとして拾わないよう、セクションを見て範囲を限る。
func parseTools(content []byte) ([]tool, error) {
	var tools []tool
	section := ""
	sc := bufio.NewScanner(strings.NewReader(string(content)))
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := sectionRe.FindStringSubmatch(trimmed); m != nil {
			section = m[1]
			continue
		}
		if section != "tools" {
			continue
		}
		m := toolLineRe.FindStringSubmatch(trimmed)
		if m == nil {
			m = toolTableLineRe.FindStringSubmatch(trimmed)
		}
		if m == nil {
			continue
		}
		key := m[1]
		if key == "" {
			key = m[2]
		}
		tools = append(tools, tool{key: key, version: m[3]})
	}
	return tools, sc.Err()
}

// parsePyRequirements は requirements の直接宣言を読む。キーは backend を明示した `pypi:` 付きに
// 揃える。mise の宣言と同じ形にしておくと、窓の判定もバイパスのキーも経路で分かれずに済む。
func parsePyRequirements(content []byte) ([]tool, error) {
	var tools []tool
	sc := bufio.NewScanner(strings.NewReader(string(content)))
	for sc.Scan() {
		trimmed := strings.TrimSpace(sc.Text())
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		m := pyRequirementRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		tools = append(tools, tool{key: "pypi:" + m[1], version: m[2]})
	}
	return tools, sc.Err()
}

// declarationPaths は宣言ファイルを列挙する。
func declarationPaths() ([]string, error) {
	reqs, err := filepath.Glob(pyRequirementsGlob)
	if err != nil {
		return nil, xerrors.Wrap(err, pyRequirementsGlob)
	}
	sort.Strings(reqs)
	return append([]string{miseFile}, reqs...), nil
}

// readDeclarations は作業ツリーの宣言ファイルを読む。
func readDeclarations(paths []string) ([]declaration, error) {
	decls := make([]declaration, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path) //nolint:gosec // path は declarationPaths が決めたもの
		if err != nil {
			return nil, xerrors.Wrap(err, path)
		}
		decls = append(decls, declaration{path: path, content: content})
	}
	return decls, nil
}

// parseDeclarations は宣言ファイル群をツールの一覧へ畳む。形式はパスで決まる。
func parseDeclarations(decls []declaration) ([]tool, error) {
	var tools []tool
	for _, d := range decls {
		var (
			parsed []tool
			err    error
		)
		if d.path == miseFile {
			parsed, err = parseTools(d.content)
		} else {
			parsed, err = parsePyRequirements(d.content)
		}
		if err != nil {
			return nil, xerrors.Wrap(err, d.path)
		}
		for _, t := range parsed {
			t.file = d.path
			tools = append(tools, t)
		}
	}
	return tools, nil
}

// added は base 時点の宣言に無かった (key, version) の組を返す。
func added(base string, paths []string, current []tool) ([]tool, error) {
	decls, err := baseDeclarations(base, paths)
	if err != nil {
		return nil, err
	}
	return addedFrom(decls, current)
}

// baseDeclarations は base 時点の宣言ファイルを読む。宣言を増やした pull request では新しい
// ファイルが base に無いのが正常なので、それは空として扱う。ただし mise.toml はどの base にも
// あるはずのファイルで、無いのは base の取り違えを意味する。ref そのものを解決できない場合と
// 併せてエラーにする。差分を取れないまま進むと gate は何も検査せずに通る。
func baseDeclarations(base string, paths []string) ([]declaration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	decls := make([]declaration, 0, len(paths))
	for _, path := range paths {
		//nolint:gosec // base は呼び出し側が与える git ref、path は declarationPaths が決めたもの
		out, err := exec.CommandContext(ctx, "git", "show", base+":"+path).Output()
		if err != nil {
			if path == miseFile {
				return nil, xerrors.Wrap(err, fmt.Sprintf("git show %s:%s（base を取得できていない可能性があります）", base, path))
			}
			if refErr := verifyRef(ctx, base); refErr != nil {
				return nil, xerrors.Wrap(refErr, fmt.Sprintf("git show %s:%s（base を取得できていない可能性があります）", base, path))
			}
			continue
		}
		decls = append(decls, declaration{path: path, content: out})
	}
	return decls, nil
}

// verifyRef は base が commit として解決できるかを見る。
func verifyRef(ctx context.Context, base string) error {
	//nolint:gosec // base は呼び出し側が与える git ref
	if err := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", base+"^{commit}").Run(); err != nil {
		return xerrors.Wrap(err, "git rev-parse "+base)
	}
	return nil
}

// addedFrom は base 時点の宣言と現在の宣言を突き合わせる。base を読めなかった場合に空の差分を
// 返すと、gate は何も検査しないまま通る。解析の失敗はエラーとして表に出す。
func addedFrom(baseDecls []declaration, current []tool) ([]tool, error) {
	before, err := parseDeclarations(baseDecls)
	if err != nil {
		return nil, xerrors.Wrap(err, "base の宣言")
	}
	return diffAdded(before, current), nil
}

// diffAdded は before に無い (key, version) の組を返す。
func diffAdded(before, current []tool) []tool {
	known := make(map[string]struct{}, len(before))
	for _, t := range before {
		known[t.id()] = struct{}{}
	}
	var fresh []tool
	for _, t := range current {
		if _, ok := known[t.id()]; !ok {
			fresh = append(fresh, t)
		}
	}
	return fresh
}

// resolveBackends は各ツールの backend を決め、対象と除外（core backend）へ振り分ける。
// 短縮名は mise registry の先頭候補を採る。mise 自身が選ぶのと同じ順序である。
func resolveBackends(ctx context.Context, tools []tool) ([]tool, []tool, error) {
	var targets, skipped []tool
	for _, t := range tools {
		if strings.Contains(t.key, ":") {
			t.backend = t.key
		} else {
			backend, resolveErr := miseRegistry(ctx, t.key)
			if resolveErr != nil {
				return nil, nil, resolveErr
			}
			t.backend = backend
		}
		if strings.HasPrefix(t.backend, "core:") {
			skipped = append(skipped, t)
			continue
		}
		targets = append(targets, t)
	}
	return targets, skipped, nil
}

func miseRegistry(ctx context.Context, name string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, miseTimeout)
	defer cancel()

	out, err := exec.CommandContext(cctx, "mise", "registry", name).Output() //nolint:gosec // name は mise.toml のキー
	if err != nil {
		return "", xerrors.Wrap(err, "mise registry "+name)
	}
	return firstBackend(out, name)
}

// firstBackend は `mise registry` の出力から先頭の候補だけを採る。mise 自身が選ぶのと同じ順序で、
// 出力全体を採ると backend が空白を含み、以降の種別判定がすべて空振りする。
func firstBackend(registryOutput []byte, name string) (string, error) {
	fields := strings.Fields(string(registryOutput))
	if len(fields) == 0 {
		return "", xerrors.Wrap(errUnsupportedBackend, name)
	}
	return fields[0], nil
}

// windowFor は backend の性質から窓を決める。
func windowFor(backend string) int {
	switch backendKind(backend) {
	case "github":
		return releaseWindowDays
	default:
		return registryWindowDays
	}
}

func backendKind(backend string) string {
	name, _, _ := strings.Cut(backend, ":")
	switch name {
	case "aqua", "ubi", "github":
		return "github"
	case "go":
		return "go"
	case "npm":
		return "npm"
	case "pypi", "pipx", "uvx":
		return "pypi"
	default:
		return ""
	}
}

// verifyLocks は *.in が宣言する版を、隣の *.txt が固定しているかどうかで確かめる。ハッシュの
// 照合そのものは install 側の --require-hashes が担うので、ここで見るのは宣言と lockfile が
// 同じ版を指しているかだけ。食い違ったままだと、cooldown を通した版と実際に入る版が別になる。
func verifyLocks(declared []tool) []violation {
	var violations []violation
	for _, t := range declared {
		if !strings.HasSuffix(t.file, pyRequirementSuffix) {
			continue
		}
		lockPath := strings.TrimSuffix(t.file, pyRequirementSuffix) + pyLockSuffix
		content, err := os.ReadFile(lockPath) //nolint:gosec // 宣言ファイルのパスから決まる
		if err != nil {
			violations = append(violations, violation{file: t.file, msg: fmt.Sprintf(
				"%s を読めません（%v）。`make py-lock` で生成してください", lockPath, err,
			)})
			continue
		}
		name := pyPackageName(t.key)
		locked, ok := lockedVersion(content, name)
		if !ok {
			violations = append(violations, violation{file: t.file, msg: fmt.Sprintf(
				"%s が宣言する %s が %s にありません。`make py-lock` で再生成してください", t.file, t.id(), lockPath,
			)})
			continue
		}
		if locked != t.version {
			violations = append(violations, violation{file: t.file, msg: fmt.Sprintf(
				"%s の宣言は %s ですが %s が固定しているのは %s です。`make py-lock` で再生成してください",
				t.file, t.id(), lockPath, name+"=="+locked,
			)})
		}
	}
	return violations
}

// lockedVersion は lockfile が name に対して固定している版を返す。
func lockedVersion(lockContent []byte, name string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(string(lockContent)))
	for sc.Scan() {
		m := pyRequirementRe.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		if pyPackageName(m[1]) == name {
			return m[2], true
		}
	}
	return "", false
}

// pyPackageName は宣言のキーから PyPI の正規化名を取り出す。extras は同じ配布物の付随依存を
// 選ぶ指定でしかなく、パッケージの同一性には関わらない。
func pyPackageName(key string) string {
	_, name, found := strings.Cut(key, ":")
	if !found {
		name = key
	}
	return pyNameSepRe.ReplaceAllString(strings.ToLower(extrasRe.ReplaceAllString(name, "")), "-")
}

// inspect は対象ツールの公開時刻を引き、窓内のものと取得できなかったものを返す。
func inspect(ctx context.Context, client *http.Client, targets []tool, now time.Time) ([]finding, []tool) {
	var (
		mu         sync.Mutex
		findings   []finding
		unresolved []tool
		wg         sync.WaitGroup
	)
	sem := make(chan struct{}, fetchWorkers)

	for _, t := range targets {
		wg.Add(1)
		go func(t tool) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			at, err := publishedAt(ctx, client, t)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				unresolved = append(unresolved, t)
				return
			}
			window := windowFor(t.backend)
			if age := int(now.Sub(at).Hours() / hoursPerDay); age < window {
				findings = append(findings, finding{tool: t, published: at, ageDays: age, window: window})
			}
		}(t)
	}
	wg.Wait()

	sort.Slice(findings, func(i, j int) bool { return findings[i].ageDays < findings[j].ageDays })
	sort.Slice(unresolved, func(i, j int) bool { return unresolved[i].id() < unresolved[j].id() })
	return findings, unresolved
}

// publishedAt は backend ごとの上流からそのバージョンの公開時刻を取る。
func publishedAt(ctx context.Context, client *http.Client, t tool) (time.Time, error) {
	_, ref, _ := strings.Cut(t.backend, ":")
	switch backendKind(t.backend) {
	case "github":
		return githubReleaseAt(ctx, client, ref, t.version)
	case "go":
		return goModuleAt(ctx, client, ref, t.version)
	case "npm":
		return npmPackageAt(ctx, client, ref, t.version)
	case "pypi":
		return pypiPackageAt(ctx, client, extrasRe.ReplaceAllString(ref, ""), t.version)
	default:
		return time.Time{}, xerrors.Wrap(errUnsupportedBackend, t.backend)
	}
}

func getJSON(ctx context.Context, client *http.Client, url string, into any) error {
	body, err := fetchBody(ctx, client, url)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	return json.NewDecoder(body).Decode(into)
}

// getText は本文をそのまま読む。module proxy の @v/list だけが JSON ではない。
func getText(ctx context.Context, client *http.Client, url string) (string, error) {
	body, err := fetchBody(ctx, client, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()

	out, err := io.ReadAll(body)
	if err != nil {
		return "", xerrors.Wrap(err, "read "+url)
	}

	return string(out), nil
}

// fetchBody は GET して本文を返す。呼び出し側が Close する。
func fetchBody(ctx context.Context, client *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, xerrors.Wrap(err, "new request")
	}
	// 未認証の GitHub API は 60 req/hour（IP 単位）で、このツールの 1 回の実行を賄えない。
	if strings.HasPrefix(url, githubAPI) {
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, xerrors.Wrap(err, "fetch "+url)
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		_ = resp.Body.Close()

		return nil, xerrors.Wrap(errNotFound, url)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()

		return nil, xerrors.Wrap(errUpstreamStatus, fmt.Sprintf("%s: %d", url, resp.StatusCode))
	}

	return resp.Body, nil
}

// githubReleaseAt は Release の published_at を返す。タグに `v` を付ける流儀と付けない流儀が
// あるため、宣言された版そのものと `v` 付きの両方を試す。
func githubReleaseAt(ctx context.Context, client *http.Client, repo, version string) (time.Time, error) {
	var body struct {
		//nolint:tagliatelle // GitHub API の応答フィールド名
		PublishedAt time.Time `json:"published_at"`
	}
	var lastErr error
	for _, tag := range []string{"v" + version, version} {
		err := getJSON(ctx, client, githubAPI+repo+"/releases/tags/"+tag, &body)
		if err == nil {
			return body.PublishedAt, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

// goModuleAt は module proxy の .info を返す。mise の go backend はパッケージパスを受けるが
// proxy が知るのはモジュールパスなので、解決するまで末尾要素を落としながら遡る。
func goModuleAt(ctx context.Context, client *http.Client, pkg, version string) (time.Time, error) {
	var body struct {
		//nolint:tagliatelle // module proxy の応答フィールド名
		Time time.Time `json:"Time"`
	}
	var lastErr error
	for path := pkg; strings.Contains(path, "/"); path = path[:strings.LastIndex(path, "/")] {
		err := getJSON(ctx, client, goProxyBase+escapeModulePath(path)+"/@v/v"+version+".info", &body)
		if err == nil {
			return body.Time, nil
		}
		lastErr = err
	}
	// pkg に `/` が無いとループが 1 度も回らない。ここで nil を返すと呼び出し側はゼロ値時刻を
	// 「公開から数十年経過」と読み、そのツールが cooldown を無条件で通過する。「問い合わせた結果
	// 見つからない」と「一度も問い合わせていない」は、公開時刻を得られていない点で同じ扱いにする。
	if lastErr == nil {
		return time.Time{}, xerrors.Wrap(errNotFound, pkg+"@"+version)
	}
	return time.Time{}, lastErr
}

// escapeModulePath は module path を proxy のパス表記へ変換する。proxy は大文字を `!` + 小文字で
// 受けるため、未エスケープのまま問い合わせると 404 になり「存在しないバージョン」に化ける。
func escapeModulePath(path string) string {
	var b strings.Builder
	for _, r := range path {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func npmPackageAt(ctx context.Context, client *http.Client, pkg, version string) (time.Time, error) {
	var body struct {
		Time map[string]time.Time `json:"time"`
	}
	if err := getJSON(ctx, client, npmBase+pkg, &body); err != nil {
		return time.Time{}, err
	}
	at, ok := body.Time[version]
	if !ok {
		return time.Time{}, xerrors.Wrap(errNotFound, pkg+"@"+version)
	}
	return at, nil
}

func pypiPackageAt(ctx context.Context, client *http.Client, pkg, version string) (time.Time, error) {
	var body struct {
		Releases map[string][]struct {
			//nolint:tagliatelle // PyPI の応答フィールド名
			UploadTime time.Time `json:"upload_time_iso_8601"`
		} `json:"releases"`
	}
	if err := getJSON(ctx, client, pypiBase+pkg+"/json", &body); err != nil {
		return time.Time{}, err
	}
	files, ok := body.Releases[version]
	if !ok || len(files) == 0 {
		return time.Time{}, xerrors.Wrap(errNotFound, pkg+"@"+version)
	}
	return files[0].UploadTime, nil
}

// readBypasses はバイパス lockfile を key@version→bypass として読む。空行とコメント行以外で
// 解釈できない行、および既出キーの再定義はエラーにする。読み飛ばしや後勝ちの上書きは、その
// エントリが「存在しない」あるいは「行順で決まる」状態を警告なく作る。
func readBypasses(path string) (map[string]bypass, error) {
	f, err := os.Open(path) //nolint:gosec // path はリテラル
	if os.IsNotExist(err) {
		return map[string]bypass{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	out := map[string]bypass{}
	sc := bufio.NewScanner(f)
	for lineNo := 1; sc.Scan(); lineNo++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := bypassLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, xerrors.Wrap(errBypassInvalidLine, fmt.Sprintf(
				`%d 行目: %q（形式は "<key>@<version>" = { expires = YYYY-MM-DD, issue = <N>, reason = "..." }）`, lineNo, line,
			))
		}
		if _, dup := out[m[1]]; dup {
			return nil, xerrors.Wrap(errBypassDuplicateKey, fmt.Sprintf("%d 行目: %q", lineNo, m[1]))
		}
		expires, err := time.Parse(time.DateOnly, m[2])
		if err != nil {
			return nil, xerrors.Wrap(err, fmt.Sprintf("%d 行目の expires", lineNo))
		}
		issue, err := strconv.Atoi(m[3])
		if err != nil {
			return nil, xerrors.Wrap(err, fmt.Sprintf("%d 行目の issue", lineNo))
		}
		out[m[1]] = bypass{expires: expires, issue: issue, reason: m[4], line: lineNo}
	}
	return out, sc.Err()
}

// validateBypasses はバイパス自身の規約違反と、そのせいで無効になったキーを返す。期限切れを
// 失敗にするのは、外したまま放置されたバイパスが恒久 allowlist と区別できなくなるため。上限を
// 置くのは、期限を遠い未来へ置くだけで同じ状態を作れてしまうため。無効なバイパスは効力も失う。
func validateBypasses(bypasses map[string]bypass, declared []tool, today time.Time) ([]violation, map[string]struct{}) {
	inDeclarations := make(map[string]struct{}, len(declared))
	for _, t := range declared {
		inDeclarations[t.id()] = struct{}{}
	}
	limit := today.AddDate(0, maxBypassMonths, 0)
	invalid := map[string]struct{}{}
	var violations []violation

	for _, key := range sortedKeys(bypasses) {
		b := bypasses[key]
		switch {
		case b.expires.Before(today):
			invalid[key] = struct{}{}
			violations = append(violations, violation{file: bypassFile, msg: fmt.Sprintf(
				"%s:%d %s の期限 %s が切れています。窓明けの版へ更新してエントリを消すか、期限を延ばす判断を #%d で記録してください",
				bypassFile, b.line, key, b.expires.Format(time.DateOnly), b.issue,
			)})
		case b.expires.After(limit):
			invalid[key] = struct{}{}
			violations = append(violations, violation{file: bypassFile, msg: fmt.Sprintf(
				"%s:%d %s の期限 %s が上限（%s から %d ヶ月 = %s）を越えています。恒久 allowlist にしないための上限です",
				bypassFile, b.line, key, b.expires.Format(time.DateOnly),
				today.Format(time.DateOnly), maxBypassMonths, limit.Format(time.DateOnly),
			)})
		default:
			if _, ok := inDeclarations[key]; !ok {
				violations = append(violations, violation{file: bypassFile, msg: fmt.Sprintf(
					"%s:%d %s は %s / %s のどちらにも存在しません。不要になったエントリは消してください",
					bypassFile, b.line, key, miseFile, pyRequirementsGlob,
				)})
			}
		}
	}
	return violations, invalid
}

func sortedKeys(m map[string]bypass) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// classify は finding を「バイパス済み」「gate を落とすもの」「報告に留めるもの」へ振り分ける。
func classify(sub string, findings []finding, bypasses map[string]bypass, invalid map[string]struct{}) ([]finding, []finding, []finding) {
	var bypassed, blocking, reported []finding
	for _, f := range findings {
		switch {
		case hasBypass(bypasses, invalid, f.tool):
			bypassed = append(bypassed, f)
		case sub == "gate":
			blocking = append(blocking, f)
		default:
			reported = append(reported, f)
		}
	}
	return bypassed, blocking, reported
}

func hasBypass(bypasses map[string]bypass, invalid map[string]struct{}, t tool) bool {
	if _, bad := invalid[t.id()]; bad {
		return false
	}
	_, ok := bypasses[t.id()]
	return ok
}

// report は結果を標準出力へ書き、終了コードを非ゼロにすべき件数を返す。audit は窓内の finding では
// 落ちない。ただしバイパス自身の規約違反だけは audit でも失敗させる。期限切れの回収がスケジュール
// 実行に懸かっているため。
func report(
	sub string, findings []finding, unresolved, skipped []tool,
	policyViolations []violation, bypasses map[string]bypass, invalid map[string]struct{},
	targets []tool, github bool,
) int {
	//nolint:gosec // sub は gate / audit のいずれかに限定済み
	log.Printf("ℹ️ tool-cooldown %s: 対象 %d 件 / ランタイム除外 %d 件 / 窓 GitHub %d 日・レジストリ %d 日",
		sub, len(targets), len(skipped), releaseWindowDays, registryWindowDays)

	blockingCount := len(policyViolations)
	for _, v := range policyViolations {
		log.Printf("❌ %s", v.msg)
		if github {
			log.Printf("::error file=%s::%s", v.file, v.msg)
		}
	}

	bypassed, blocked, reported := classify(sub, findings, bypasses, invalid)

	for _, f := range bypassed {
		b := bypasses[f.tool.id()]
		log.Printf("🔓 %s は公開 %d 日ですが、#%d のバイパスで通します（期限 %s / %s）",
			f.tool.id(), f.ageDays, b.issue, b.expires.Format(time.DateOnly), b.reason)
	}

	blockingCount += len(blocked)
	for _, f := range blocked {
		msg := fmt.Sprintf("%s（%s）は公開 %d 日で cooldown %d 日を満たしていません。窓明けを待つか、"+
			"待てない根拠を %s へ期限付きで記録してください", f.tool.id(), f.tool.backend, f.ageDays, f.window, bypassFile)
		log.Printf("❌ %s", msg)
		if github {
			log.Printf("::error file=%s::%s", f.tool.file, msg)
		}
	}

	for _, f := range reported {
		log.Printf("⚠️ %s（%s）は公開 %d 日で cooldown %d 日を満たしていません", f.tool.id(), f.tool.backend, f.ageDays, f.window)
	}

	for _, t := range unresolved {
		// 取得できないものを黙って通すと、検査が「見た結果 OK」と「見ていない」を区別できなくなる。
		msg := fmt.Sprintf("%s（%s）の公開時刻を取得できませんでした", t.id(), t.backend)
		if sub == "gate" {
			blockingCount++
			log.Printf("❌ %s", msg)
			if github {
				log.Printf("::error file=%s::%s", t.file, msg)
			}
			continue
		}
		log.Printf("⚠️ %s", msg)
	}

	if blockingCount == 0 {
		//nolint:gosec // sub は gate / audit のいずれかに限定済み
		log.Printf("✅ tool-cooldown %s: 違反なし（バイパス %d 件）", sub, len(bypasses))
	}
	return blockingCount
}

// fenceFor は text を包むのに足りるフェンスを返す。長さは text 中の最長バッククォート連 + 1。
func fenceFor(text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
			continue
		}
		run = 0
	}
	return strings.Repeat("`", max(minFenceLen, longest+1))
}

// fenced は見出しと、フェンスで包んだ本体を書く。値は mise.toml 由来で pull request が中身を
// 決めるため、見出しだけをテンプレート側に残して値はフェンスへ入れる。
func fenced(b *strings.Builder, heading string, lines []string) {
	body := strings.Join(lines, "\n")
	fence := fenceFor(body)
	fmt.Fprintf(b, "## %s (%d)\n\n%stext\n%s\n%s\n\n", heading, len(lines), fence, body, fence)
}

// summary は GITHUB_STEP_SUMMARY 用の Markdown を組む。
func summary(
	sub string, findings []finding, unresolved, skipped []tool,
	policyViolations []violation, bypasses map[string]bypass, invalid map[string]struct{},
	targets []tool,
) string {
	var b strings.Builder
	_, blocked, reported := classify(sub, findings, bypasses, invalid)

	fmt.Fprintf(&b, "対象 %d 件 / ランタイム除外 %d 件 / バイパス %d 件 / 窓 GitHub %d 日・レジストリ %d 日\n\n",
		len(targets), len(skipped), len(bypasses), releaseWindowDays, registryWindowDays)

	if len(policyViolations) > 0 {
		lines := make([]string, 0, len(policyViolations))
		for _, v := range policyViolations {
			lines = append(lines, v.msg)
		}
		fenced(&b, "宣言側の違反", lines)
	}
	if len(blocked) > 0 {
		lines := make([]string, 0, len(blocked))
		for _, f := range blocked {
			lines = append(lines, fmt.Sprintf("- %s（%s）— 公開 %d 日 / 窓 %d 日（%s）",
				f.tool.id(), f.tool.backend, f.ageDays, f.window, f.published.Format(time.DateOnly)))
		}
		fenced(&b, "cooldown 未達", lines)
	}
	if len(reported) > 0 {
		lines := make([]string, 0, len(reported))
		for _, f := range reported {
			lines = append(lines, fmt.Sprintf("- %s（%s）— 公開 %d 日 / 窓 %d 日", f.tool.id(), f.tool.backend, f.ageDays, f.window))
		}
		fenced(&b, "参考: 窓内だがブロックしないもの", lines)
	}
	if len(unresolved) > 0 {
		lines := make([]string, 0, len(unresolved))
		for _, t := range unresolved {
			lines = append(lines, fmt.Sprintf("- %s（%s）", t.id(), t.backend))
		}
		fenced(&b, "公開時刻を取得できなかったもの", lines)
	}
	return b.String()
}

// appendOutput は後続ステップ用に件数を GITHUB_OUTPUT へ書く。
func appendOutput(path string, findings, audited, blocking int) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, outputPerm) //nolint:gosec // path は GITHUB_OUTPUT（ランナー提供）
	if err != nil {
		return xerrors.Wrap(err, "open GITHUB_OUTPUT")
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "findings=%d\naudited=%d\nblocking=%d\n", findings, audited, blocking)
	return err
}
