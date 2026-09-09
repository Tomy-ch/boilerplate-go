// Package main は、カバレッジプロファイルの総カバレッジがしきい値以上かを検査するツール。
//
//	-profile:   `go test -coverprofile` が出力したプロファイル（既定 coverage.out）
//	-threshold: 下回ってはならない総カバレッジのパーセント（既定 90）
//	-warn:      下限割れを失敗させず警告に留める（scripts/ 配下の計測で使う）
//	-github:    警告を GitHub Actions の ::warning:: アノテーションで出す
//
// 守る対象の違う 2 つの計測がこのツールを共有する。boilerplate 本体（internal / pkg）は
// 下限割れを失敗にするが、scripts/ 配下の開発ツールは警告に留める。後者の劣化が前者の
// ゲートを止めてしまうと、出荷物と無関係な理由でマージがブロックされるためである。
//
// 総カバレッジは `go tool cover -func` の `total:` 行から取る。プロファイルの
// 集計規則（-covermode ごとの重み付けなど）を写し取らずに済ませるためで、
// ここが持つのは「取り出す」ことと「比較する」ことだけ。
//
// このツールは CI のカバレッジゲートから呼ばれる。壊れ方が「何も検査しなくなる」
// 方向に出るため、判定ロジックはシェルの中ではなくテストの当たる Go 側に置く。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"go-boilerplate/pkg/xerrors"
)

// defaultThreshold は、総カバレッジの下限の既定値（docs/rules.md の 90% フロア）。
// 実際の値は make 側（COVERAGE_THRESHOLD）が -threshold で渡す。
const defaultThreshold = 90

var (
	// errNoTotalLine は、`go tool cover -func` の出力に総カバレッジ行が無いことを表す。
	errNoTotalLine = xerrors.New("no total line in cover output")
	// errBelowThreshold は、総カバレッジが下限を割ったことを表す。
	errBelowThreshold = xerrors.New("coverage below threshold")
)

// main は 1:1 テスト規約の対象外で分岐を検査できないため、判断は run に置きます。
func main() {
	log.SetFlags(0)

	if err := run(os.Args[1:], coverTotal); err != nil {
		log.Fatalf("%v", err)
	}
}

// run は、プロファイルの総カバレッジをしきい値に照らして報告します。
// total は総カバレッジの取得手段で、差し替えられるよう引数で受けます。
func run(args []string, total func(profile string) (float64, error)) error {
	fs := flag.NewFlagSet("cover-gate", flag.ContinueOnError)
	profile := fs.String("profile", "coverage.out", "検査するカバレッジプロファイル")
	threshold := fs.Int("threshold", defaultThreshold, "総カバレッジの下限（パーセント）")
	warn := fs.Bool("warn", false, "下限を割っても失敗させず警告に留める")
	github := fs.Bool("github", false, "GitHub Actions の ::warning:: アノテーションを出力する")

	if err := fs.Parse(args); err != nil {
		// ヘルプ要求は失敗ではないので 0 で終える。usage は flag が既に出力している。
		if xerrors.Is(err, flag.ErrHelp) {
			return nil
		}

		return xerrors.Wrap(err, "failed to parse flags")
	}

	if _, err := os.Stat(*profile); err != nil {
		return xerrors.Wrap(err, "❌ "+*profile+" がありません（先に make go-test-cover-ci を実行）")
	}

	value, err := total(*profile)
	if err != nil {
		return xerrors.Wrap(err, "❌ 総カバレッジを取得できません")
	}

	message, ok := judge(value, *threshold)
	if ok {
		log.Print(message)

		return nil
	}

	// 下限割れを失敗にするか警告に留めるかの理由は package doc を参照。ここはフラグの指示に従う。
	if *warn {
		log.Print(annotate(message, *github))

		return nil
	}

	return xerrors.Wrap(errBelowThreshold, message)
}

// coverTotal は、`go tool cover -func` を起動して総カバレッジを取り出します。
func coverTotal(profile string) (float64, error) {
	cover := exec.CommandContext(context.Background(), "go", "tool", "cover", "-func="+profile) //nolint:gosec // 引数は profile のみで、コマンド自体は固定

	out, err := cover.Output()
	if err != nil {
		return 0, xerrors.Wrap(err, "go tool cover")
	}

	return parseTotal(string(out))
}

// annotate は、GitHub Actions で読ませる場合に警告アノテーションの接頭辞を付けます。
// 付けないときは元の文言をそのまま返します。
func annotate(message string, github bool) string {
	if !github {
		return message
	}

	return "::warning::" + message
}

// parseTotal は、`go tool cover -func` の出力から総カバレッジのパーセント値を取り出します。
// `total:` で始まる行の最終フィールド（`87.5%` 形式）を読み、総カバレッジ行が無い場合や
// パーセント表記が数値でない場合はエラーを返します。
func parseTotal(coverOutput string) (float64, error) {
	for line := range strings.Lines(coverOutput) {
		if !strings.HasPrefix(line, "total:") {
			continue
		}

		fields := strings.Fields(line)

		last := fields[len(fields)-1]
		if !strings.HasSuffix(last, "%") {
			return 0, xerrors.Wrap(errNoTotalLine, "total is not a percentage: "+last)
		}

		total, err := strconv.ParseFloat(strings.TrimSuffix(last, "%"), 64)
		if err != nil {
			return 0, xerrors.Wrap(err, "invalid total coverage: "+last)
		}

		return total, nil
	}

	return 0, errNoTotalLine
}

// judge は、総カバレッジがしきい値以上かを判定し、表示する報告文と合否を返します。
// しきい値ちょうどは合格です。
func judge(total float64, threshold int) (string, bool) {
	if total < float64(threshold) {
		return fmt.Sprintf("❌ 総カバレッジ %.1f%% がしきい値 %d%% を下回っています", total, threshold), false
	}

	return fmt.Sprintf("✅ 総カバレッジ %.1f%% (しきい値 %d%%)", total, threshold), true
}
