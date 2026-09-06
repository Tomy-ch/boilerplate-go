// Package main は、この checkout が使う Realtime Delivery の table を削除する開発用ツールです。
//
// Realtime Delivery は採番を PostgreSQL に、配送済み event を DynamoDB EventLog に置きます
// （docs/design/realtime-delivery.md）。`make slot-acquire` はスロットの PostgreSQL を作り直すため、
// 一緒に EventLog も空にしないと採番だけが 1 へ巻き戻り、既存 item の位置へ届いたところで
// 条件付き書き込みが ErrSequenceConflict を返して stream が止まります。
//
// table の作成は realtime-init（make realtime-provision）の担当なので、ここは削除だけを行います。
// 相手は emulator に限る前提で、実 AWS の host と、host を持たない endpoint は拒否します。
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
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
	// SDK は署名のために非空の資格情報を要求します。実 AWS が拒む鍵であることが本番到達を止める
	// 防御の一部なので、app の資格情報（config の REALTIME_*）へ寄せないこと（scripts/README.md の
	// realtime-reset）。別の鍵でも app の table が見えるのは dynamodb_local が -sharedDb で動くためで
	// （docker-compose.yaml）、外すと削除が「既にありません」で空振りします。
	resetCredential = "reset"

	// awsHostSuffixes は、実 AWS の endpoint を見分ける host の末尾です。
	// 中国パーティション（.amazonaws.com.cn）と dual-stack（.api.aws）は別の末尾を持つので個別に挙げます。
	awsHostSuffixes = ".amazonaws.com,.amazonaws.com.cn,.api.aws"
)

var (
	errFlags    = xerrors.New("コマンドラインの解釈に失敗しました")
	errEndpoint = xerrors.New("-endpoint は scheme 付きの URL（scheme://host:port）で指定してください")
	errRealAWS  = xerrors.New("-endpoint に実 AWS を指定できません（このツールは emulator 専用です）")
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

// run は、flag を解釈し、削除対象を解決して削除します。削除は取り消せないので、endpoint の拒否は
// 接続先の生成より先に効かせます。
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

		return opts, xerrors.Join(errFlags, err)
	}

	if err := validateEndpoint(opts.endpoint); err != nil {
		return opts, err
	}

	return opts, nil
}

// validateEndpoint は、endpoint が emulator を指す形であることを確かめます。空や host の無い
// endpoint は入力ミスとして止め、実 AWS の host は拒みます。自前ホストの emulator を使う構成が
// あるので loopback には限定しません。
func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return xerrors.Wrap(errEndpoint, endpoint)
	}

	// "localhost:8000" は scheme=localhost / host="" に解釈されるので、host 空は入力ミスとして止める。
	if u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return xerrors.Wrap(errEndpoint, endpoint)
	}

	// 末尾のドットは FQDN の書き方の違いでしかなく、TLS 検証も剥がして通す。落としてから比べる。
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	for suffix := range strings.SplitSeq(awsHostSuffixes, ",") {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return xerrors.Wrap(errRealAWS, endpoint)
		}
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
