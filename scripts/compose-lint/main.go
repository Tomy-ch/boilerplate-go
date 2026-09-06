// Package main は docker-compose のサービス宣言を、このリポジトリが守りたい規則に照らして検査するツール。
//
// 現在の規則は 1 つ。
//
//	app 層として起動されるサービス（.makefiles/docker/compose.mk の APP_SERVICES）は
//	healthcheck を宣言していなければならない。
//
// 対象サービスの一覧を持たず compose.mk から読むのは、両方に置くと片方だけを直したときに黙って
// ずれるため（同じ理由が compose.mk 自身のコメントにも書かれている）。
//
// compose ファイルの構文と補間は docker compose 自身が見るのでここでは扱わない。ここが埋めるのは
// 「構文は通るが、サービス宣言がリポジトリの約束を満たしていない」という、どの検査器も見ていない層。
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"go-boilerplate/pkg/xerrors"
)

// main は 1:1 テスト規約の対象外で分岐を検査できないため、判断は run に置きます。
func main() {
	log.SetFlags(0)

	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatalf("%v", err)
	}
}

// run は、compose ファイルと compose.mk を読み、規則違反があれば報告して errViolation を返します。
func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("compose-lint", flag.ContinueOnError)
	composePath := fs.String("compose", "docker-compose.yaml", "検査する compose ファイル")
	makefilePath := fs.String("makefile", ".makefiles/docker/compose.mk", "APP_SERVICES を宣言する mk ファイル")

	if err := fs.Parse(args); err != nil {
		// ヘルプ要求は失敗ではないので 0 で終える。usage は flag が既に出力している。
		if xerrors.Is(err, flag.ErrHelp) {
			return nil
		}

		return xerrors.Wrap(err, "failed to parse flags")
	}

	services, err := loadServices(*composePath)
	if err != nil {
		return err
	}

	appServices, err := loadAppServices(*makefilePath)
	if err != nil {
		return err
	}

	report, err := checkHealthcheck(appServices, services)
	if err != nil {
		return err
	}

	if report == "" {
		fmt.Fprintf(out, "✅ app 層の %d サービスすべてが healthcheck を宣言しています。\n", len(appServices))

		return nil
	}

	// 報告文は違反サービスごとの複数行なので、エラーのメッセージへ畳み込まずそのまま出す。
	log.Print(report)

	return errViolation
}
