package dbslot

import (
	"bytes"

	root "go-boilerplate"
	"go-boilerplate/pkg/xerrors"

	"github.com/joho/godotenv"
)

// embeddedEnvFile は、バイナリへ埋め込む env ファイルのパスです。
const embeddedEnvFile = "env/.env"

const (
	realtimeTableSuffixKey = "REALTIME_TABLE_SUFFIX"
	realtimeQueuePrefixKey = "REALTIME_QUEUE_PREFIX"
	realtimeTopicKey       = "REALTIME_TOPIC"
)

// ErrRealtimeBaseMissing は、埋め込み env が Realtime の資源名の基底を宣言していないことを表します。
var ErrRealtimeBaseMissing = xerrors.New("the embedded env declares no realtime resource name")

// RealtimeBase は、Realtime Delivery の資源名の基底です。スロットを保持しない checkout では
// これがそのまま使われ、保持していればスロット番号を継いだ名前になります（realtimeName）。
type RealtimeBase struct {
	TableSuffix string // REALTIME_TABLE_SUFFIX
	QueuePrefix string // REALTIME_QUEUE_PREFIX
	Topic       string // REALTIME_TOPIC（fan-out topic の ARN）
}

// LoadRealtimeBase は、埋め込み env が宣言する基底を返します。
//
// OS の環境変数をマージしません。db-slot 自身が同じ名前を撒くため、マージすると自分の出力を
// 入力として読み直し、スロット番号を二重に継ぎます（local_wt2 を基底として local_wt2_wt2）。
// application の設定読み込み（config.Load）が実行時 env を優先するのは正しく、ここだけが違います。
func LoadRealtimeBase() (RealtimeBase, error) {
	b, err := root.FS.ReadFile(embeddedEnvFile)
	if err != nil {
		return RealtimeBase{}, xerrors.Wrap(err, "read "+embeddedEnvFile)
	}

	kv, err := godotenv.Parse(bytes.NewReader(b))
	if err != nil {
		return RealtimeBase{}, xerrors.Wrap(err, "parse "+embeddedEnvFile)
	}

	return realtimeBaseFrom(kv)
}

// realtimeBaseFrom は、env の key-value から基底を取り出します。
//
// topic だけは空を許します。空の topic で fan-out を配線すると application が起動時に落ちる、
// というのが env の宣言する契約で（env/README.md）、ここで先に止めるとその契約を狭めます。
func realtimeBaseFrom(kv map[string]string) (RealtimeBase, error) {
	base := RealtimeBase{
		TableSuffix: kv[realtimeTableSuffixKey],
		QueuePrefix: kv[realtimeQueuePrefixKey],
		Topic:       kv[realtimeTopicKey],
	}

	if base.TableSuffix == "" {
		return RealtimeBase{}, xerrors.Wrap(ErrRealtimeBaseMissing, realtimeTableSuffixKey)
	}

	if base.QueuePrefix == "" {
		return RealtimeBase{}, xerrors.Wrap(ErrRealtimeBaseMissing, realtimeQueuePrefixKey)
	}

	return base, nil
}
