// Package main は、ホスト負荷の配分（負荷帯・CPU シェア・各ツールへ渡す並列度）を解決するツール。
//
//	env:    解決結果を KEY=VALUE で出力する。make のレシピが eval して読む。
//	status: 解決結果と帯ごとの助言を人間向けに表示する（make load-status）。
//
// 帯の意味と閾値の意図は .makefiles/README.md の `.makefiles/load` group が持つ。ここが持つのは
// 「窓の数と CPU 数から帯・シェア・フラグを導く」計算だけ。
//
// このツールはコミット毎・push 毎に走るゲートの並列度を決める。壊れ方が「黙って
// 既定の帯へ縮退し、ホストを飽和させる／検査を飛ばす」方向に出るため、判定ロジックは
// シェルの中ではなくテストの当たる Go 側に置く。窓の数え方も同じ理由でここが持つ。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"go-boilerplate/pkg/xerrors"
)

// 負荷帯。auto は窓の数から決めるため、解決後の値には現れない。
const (
	bandAuto    = "auto"
	bandFull    = "full"
	bandLow     = "low"
	bandCIFirst = "ci-first"
)

const (
	// defaultLowThreshold / defaultCIFirstThreshold は帯域の境目の既定値。
	// make 側は上書き値だけを渡すため、既定値の在処はここ 1 箇所。
	defaultLowThreshold     = 3
	defaultCIFirstThreshold = 5

	// niceValue は絞る帯で重いゲートに付ける優先度。
	niceValue = "nice -n 10"

	// minShare は 1 窓あたりの CPU share の下限。0 を渡すとツールによっては「無制限」と
	// 解釈され、絞る意図が反転する。
	minShare = 1

	// minWindows は窓の数の下限。share は CPU 数を窓の数で割って求めるため、0 が入ると
	// 0 除算になる。countWindows も同じ下限を保証するが、resolve はそれを経ずに呼べる。
	minWindows = 1
)

var (
	// errUnknownBand は、GOBP_LOAD に未知の値が指定されたことを表す。
	errUnknownBand = xerrors.New("unknown load band (auto / full / low / ci-first)")
	// errUsage は、サブコマンドが指定されていないことを表す。
	errUsage = xerrors.New("usage: load-band <env|status> [--load=...] [--low=N] [--ci-first=N]")
	// errUnknownSubcommand は、未知のサブコマンドが指定されたことを表す。
	errUnknownSubcommand = xerrors.New("unknown subcommand (env / status)")
)

// band は、解決済みの負荷帯とその導出に使った値。
type band struct {
	requested        string
	resolved         string
	windows          int
	cpus             int
	share            int
	lowThreshold     int
	ciFirstThreshold int
}

// throttled は、絞る側の帯（low / ci-first）かを返します。ci-first でもローカルに残す軽い
// ゲートは走るため、絞り自体は両方に効かせます。
func (b band) throttled() bool {
	return b.resolved == bandLow || b.resolved == bandCIFirst
}

// main は 1:1 テスト規約の対象外で分岐を検査できないため、判断は run に置きます。
func main() {
	log.SetFlags(0)

	if err := run(os.Args[1:], os.Stdout, countWindows(worktreeList()), runtime.NumCPU()); err != nil {
		log.Fatalf("%v", err)
	}
}

// run は、サブコマンドとフラグから負荷帯を解決し、解決結果を out へ書き出します。
// 窓の数と CPU 数はホストの実測値で、差し替えられるよう引数で受けます。
func run(args []string, out io.Writer, windows, cpus int) error {
	if len(args) == 0 {
		return errUsage
	}

	subcommand := args[0]

	fs := flag.NewFlagSet(subcommand, flag.ContinueOnError)
	load := fs.String("load", "", "負荷帯 (auto / full / low / ci-first。空なら auto)")
	low := fs.Int("low", defaultLowThreshold, "low へ落とす窓の数")
	ciFirst := fs.Int("ci-first", defaultCIFirstThreshold, "ci-first へ落とす窓の数")

	if err := fs.Parse(args[1:]); err != nil {
		// ヘルプ要求は失敗ではないので 0 で終える。usage は flag が既に出力している。
		if xerrors.Is(err, flag.ErrHelp) {
			return nil
		}

		return xerrors.Wrap(err, "failed to parse flags")
	}

	resolved, err := resolve(*load, windows, cpus, *low, *ciFirst)
	if err != nil {
		return xerrors.Wrap(err, "❌ 負荷帯を解決できません")
	}

	switch subcommand {
	case "env":
		_, _ = fmt.Fprint(out, renderEnv(resolved))
	case "status":
		_, _ = fmt.Fprint(out, renderStatus(resolved))
	default:
		return errUnknownSubcommand
	}

	return nil
}

// worktreeList は `git worktree list` の出力を返します。git が使えない・リポジトリでない等で
// 数えられない場合は空文字を返します（呼び出し側で 1 窓として扱われます）。
func worktreeList() string {
	out, err := exec.CommandContext(context.Background(), "git", "worktree", "list").Output()
	if err != nil {
		return ""
	}

	return string(out)
}

// countWindows は、`git worktree list` の出力から窓（worktree）の数を数えます。
// 数えられない場合は 1 を返します。0 を返すと「窓が無い」という有り得ない状態が
// 帯の判定へ流れ込むため、下限は常に 1 です。
//
// スロットのリースではなく worktree を数えるのは、スロット取得が opt-in で「窓はあるが
// スロットは取っていない」状態が普通にあるため。CPU を食うのは窓のほうです。
func countWindows(gitOutput string) int {
	count := 0

	for line := range strings.Lines(gitOutput) {
		if strings.TrimSuffix(line, "\n") != "" {
			count++
		}
	}

	if count < 1 {
		return 1
	}

	return count
}

// resolve は、指定された帯（空 / auto なら窓の数）から負荷帯と CPU シェアを決めます。
// 未知の帯名はエラーにします（黙って full へ落とすと、絞る指定の書き損じに気づけません）。
func resolve(requested string, windows, cpus, lowThreshold, ciFirstThreshold int) (band, error) {
	if requested == "" {
		requested = bandAuto
	}

	// 下限は帯の判定より前に効かせる。判定に渡す数と報告する数が食い違うと、
	// 表示された窓の数から帯を再現できなくなる。
	windows = max(windows, minWindows)
	cpus = max(cpus, minShare)

	b := band{
		requested:        requested,
		windows:          windows,
		cpus:             cpus,
		lowThreshold:     lowThreshold,
		ciFirstThreshold: ciFirstThreshold,
	}

	switch requested {
	case bandAuto:
		b.resolved = bandFor(windows, lowThreshold, ciFirstThreshold)
	case bandFull, bandLow, bandCIFirst:
		b.resolved = requested
	default:
		return band{}, xerrors.Wrap(errUnknownBand, "GOBP_LOAD="+requested)
	}

	b.share = b.cpus
	if b.throttled() {
		b.share = max(b.cpus/b.windows, minShare)
	}

	return b, nil
}

// bandFor は、窓の数から負荷帯を決めます。
func bandFor(windows, lowThreshold, ciFirstThreshold int) string {
	switch {
	case windows >= ciFirstThreshold:
		return bandCIFirst
	case windows >= lowThreshold:
		return bandLow
	default:
		return bandFull
	}
}

// renderEnv は、レシピが eval して読む KEY=VALUE を組み立てます。
// 絞らない帯では各フラグを空にし、ツールの既定（golangci-lint は設定ファイルの
// concurrency、go test は GOMAXPROCS）へ委ねます。
func renderEnv(b band) string {
	values := [][2]string{
		{"GOBP_LOAD_RESOLVED", b.resolved},
		{"GOBP_WINDOWS", strconv.Itoa(b.windows)},
		{"GOBP_CPUS", strconv.Itoa(b.cpus)},
		{"GOBP_SHARE", strconv.Itoa(b.share)},
		{"GOBP_THROTTLED", throttledFlag(b)},
		{"GOBP_NICE", nice(b)},
		{"GOLANGCI_CONCURRENCY_FLAG", golangciConcurrencyFlag(b)},
		{"GO_TEST_P_FLAG", goTestPFlag(b)},
		{"GO_TEST_LOAD_ENV", goTestLoadEnv(b)},
	}

	var sb strings.Builder
	for _, kv := range values {
		fmt.Fprintf(&sb, "%s=%s\n", kv[0], shellQuote(kv[1]))
	}

	return sb.String()
}

// throttledFlag は、絞る帯かどうかを空文字 / "1" で返します（シェルの -n 判定に合わせる）。
func throttledFlag(b band) string {
	if b.throttled() {
		return "1"
	}

	return ""
}

// nice は、重いゲートに付ける優先度指定を返します。絞る帯では他の窓の対話操作
// （git / docker / エディタ）を待たせないよう優先度を下げます。
func nice(b band) string {
	if b.throttled() {
		return niceValue
	}

	return ""
}

// golangciConcurrencyFlag は、golangci-lint へ渡す並列度フラグを返します。
func golangciConcurrencyFlag(b band) string {
	if b.throttled() {
		return "--concurrency " + strconv.Itoa(b.share)
	}

	return ""
}

// goTestPFlag は、go test へ渡すパッケージ同時実行数フラグを返します。
func goTestPFlag(b band) string {
	if b.throttled() {
		return "-p " + strconv.Itoa(b.share)
	}

	return ""
}

// goTestLoadEnv は、go test に渡す環境変数の代入形を返します。-p はパッケージの同時実行数しか
// 絞れず、各テストバイナリ内部の並列度（t.Parallel() の上限＝GOMAXPROCS）は絞れません。
// -race はここが特に重いため両方を絞ります。
func goTestLoadEnv(b band) string {
	if b.throttled() {
		return "GOMAXPROCS=" + strconv.Itoa(b.share)
	}

	return ""
}

// shellQuote は、値をシングルクォートで囲んで eval 可能にします。
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// renderStatus は、`make load-status` が表示する人間向けの解決結果を組み立てます。
// 負荷帯まわりで人間がデバッグに使う唯一の窓口なので、導出に使った値を残らず印字します。
func renderStatus(b band) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "load      : %s  (GOBP_LOAD=%s)\n", b.resolved, b.requested)
	fmt.Fprintf(&sb, "windows   : %d worktree  (low >= %d, ci-first >= %d)\n", b.windows, b.lowThreshold, b.ciFirstThreshold)
	fmt.Fprintf(&sb, "cpus      : %d  ->  share %d / 窓\n", b.cpus, b.share)
	fmt.Fprintf(&sb, "golangci  : %s\n", orElse(golangciConcurrencyFlag(b), "設定ファイルの concurrency に委譲"))
	fmt.Fprintf(&sb, "go test   : %s\n", orElse(strings.TrimSpace(goTestPFlag(b)+" "+goTestLoadEnv(b)), "既定（ホスト全体）"))
	fmt.Fprintf(&sb, "nice      : %s\n", orElse(nice(b), "なし"))
	sb.WriteString("\n")
	sb.WriteString(advice(b))

	return sb.String()
}

// advice は、帯ごとの助言（💡）を返します。
func advice(b band) string {
	switch b.resolved {
	case bandCIFirst:
		return "💡 窓が多いため CI-first です。重いゲート（lint / test）はローカルで走らせず、\n" +
			"   push して CI で検証します。手元に残るのは commitlint / secret-scan / pin 検査など、\n" +
			"   push 後では手遅れになる軽いゲートだけです。\n" +
			"   一時的に手元で回すなら: make go-lint GOBP_LOAD=low\n"
	case bandLow:
		return fmt.Sprintf(
			"💡 窓が多いため低負荷モードです。重いゲートは CPU share %d に絞り、逐次で走ります。\n"+
				"   さらに窓を増やすなら CI へ委ねる方が速く確実です: GOBP_LOAD=ci-first\n", b.share,
		)
	default:
		return "💡 窓が少ないためホスト全体を使います（従来どおり）。\n"
	}
}

// orElse は、value が空なら fallback を返します。
func orElse(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
