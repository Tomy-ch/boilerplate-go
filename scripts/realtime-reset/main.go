// Package main は、この checkout が使う Realtime Delivery の table を削除する開発用ツールです。
//
// Realtime Delivery は採番を PostgreSQL に、配送済み event を DynamoDB EventLog に置きます
// （docs/design/realtime-delivery.md）。`make slot-acquire` はスロットの PostgreSQL を作り直すため、
// 一緒に EventLog も空にしないと採番だけが 1 へ巻き戻り、既存 item の位置へ届いたところで
// 条件付き書き込みが ErrSequenceConflict を返して stream が止まります。
//
// table の作成は realtime-init（make realtime-provision）の担当なので、ここは削除だけを行います。
// 削除は emulator を相手にする前提で、endpoint を省略すると SDK 既定の解決で本番 DynamoDB を
// 指し得るため、空の endpoint は拒否します。
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"go-boilerplate/internal/cli/realtimeinit"
	"go-boilerplate/internal/config"
	"go-boilerplate/pkg/xerrors"
)

const (
	defaultEndpoint = "http://localhost:8000"
	defaultRegion   = "us-east-1"
	defaultTimeout  = 60 * time.Second

	// gonePollInterval は、waitGone が table の消失を確かめ直す間隔です。
	gonePollInterval = 200 * time.Millisecond

	// resetCredential は、emulator へ渡す静的資格情報です。emulator は認証しませんが、
	// SDK は署名のために非空の資格情報を要求します。
	resetCredential = "reset"
)

var (
	errEndpoint = xerrors.New("-endpoint は scheme 付きの URL（scheme://host:port）で指定してください")
	errGone     = xerrors.New("table の削除が制限時間内に終わりませんでした")
)

// tableAPI は、削除に必要な DynamoDB 呼び出しだけを切り出した注入点です。
type tableAPI interface {
	DeleteTable(
		ctx context.Context, in *dynamodb.DeleteTableInput, opts ...func(*dynamodb.Options),
	) (*dynamodb.DeleteTableOutput, error)
	DescribeTable(
		ctx context.Context, in *dynamodb.DescribeTableInput, opts ...func(*dynamodb.Options),
	) (*dynamodb.DescribeTableOutput, error)
}

// options は、コマンドラインで決まる実行条件です。
type options struct {
	endpoint string
	region   string
	timeout  time.Duration
}

// tableResolver は、削除対象の table 名を返す関数型です。
type tableResolver func() ([]string, error)

// apiFactory は、endpoint と region から DynamoDB クライアントを組み立てる関数型です。
type apiFactory func(ctx context.Context, opts options) (tableAPI, error)

func main() {
	log.SetFlags(0)

	err := run(context.Background(), os.Args[1:], os.Stdout, configuredTables, func(
		ctx context.Context, opts options,
	) (tableAPI, error) {
		return newClient(ctx, opts)
	})
	if err != nil {
		log.Printf("❌ %v", err)
		os.Exit(1)
	}
}

// run は、flag を解釈し、削除対象を解決して削除します。設定の読み取りとクライアントの生成は
// 引数で受けます。削除は取り消せないので、endpoint の拒否が実際に削除より先に効くことを
// テストから確かめられる形にしています（scripts/README.md の Test Strategy）。
func run(ctx context.Context, args []string, out io.Writer, tables tableResolver, newAPI apiFactory) error {
	opts, err := parseOptions(args)
	if err != nil {
		if xerrors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	names, err := tables()
	if err != nil {
		return err
	}

	api, err := newAPI(ctx, opts)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	return deleteTables(ctx, api, names, out)
}

// configuredTables は、この checkout の設定が指す 3 table の名前を返します。
func configuredTables() ([]string, error) {
	cfg, err := config.SetUpConfig()
	if err != nil {
		return nil, xerrors.Wrap(err, "load config")
	}

	return realtimeinit.TableNames(config.NewRealtimeConfig(cfg)), nil
}

func parseOptions(args []string) (options, error) {
	var opts options

	fs := flag.NewFlagSet("realtime-reset", flag.ContinueOnError)
	fs.StringVar(&opts.endpoint, "endpoint", defaultEndpoint, "DynamoDB 互換 store の endpoint")
	fs.StringVar(&opts.region, "region", defaultRegion, "署名に使う region")
	fs.DurationVar(&opts.timeout, "timeout", defaultTimeout, "実行全体の上限時間")

	if err := fs.Parse(args); err != nil {
		if xerrors.Is(err, flag.ErrHelp) {
			return opts, err
		}

		return opts, xerrors.Wrap(err, "parse flags")
	}

	if err := validateEndpoint(opts.endpoint); err != nil {
		return opts, err
	}

	return opts, nil
}

// validateEndpoint は、endpoint が emulator を指す形であることを確かめます。空の endpoint は
// SDK 既定の解決へ落ちて本番 DynamoDB を消し得るため、ここで止めます。
func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return xerrors.Wrap(errEndpoint, endpoint)
	}

	// "localhost:8000" は scheme=localhost / host="" に解釈されるので、host 空は入力ミスとして止める。
	if u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return xerrors.Wrap(errEndpoint, endpoint)
	}

	return nil
}

// newClient は、静的資格情報と endpoint 上書きでクライアントを組み立てます。
func newClient(ctx context.Context, opts options) (*dynamodb.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(opts.region),
		awsconfig.WithCredentialsProvider(awscreds.NewStaticCredentialsProvider(resetCredential, resetCredential, "")),
	)
	if err != nil {
		return nil, xerrors.Wrap(err, "load aws config")
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(opts.endpoint)
	}), nil
}
