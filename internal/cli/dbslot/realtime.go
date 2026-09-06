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

// LoadRealtimeBase は、埋め込み env が宣言する基底を返します。OS の環境変数はマージしません —
// db-slot env が撒く同名の変数を読み戻さないためです（README「Resolved values」）。
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

// realtimeBaseFrom は、env の key-value から基底を取り出します。3 つのいずれかが空なら
// ErrRealtimeBaseMissing です。空を compose へ渡すと既定値（スロットを持たないときの名前）へ
// 置き換わり、その資源だけ主 checkout と共有してしまいます
// （docs/maintenance/db-worktree-pool.md「The Realtime Delivery emulators are shared instances」）。
func realtimeBaseFrom(kv map[string]string) (RealtimeBase, error) {
	base := RealtimeBase{
		TableSuffix: kv[realtimeTableSuffixKey],
		QueuePrefix: kv[realtimeQueuePrefixKey],
		Topic:       kv[realtimeTopicKey],
	}

	// 複数欠けたときに報告するキーが揺れないよう、順序を固定して見る。
	for _, declared := range [][2]string{
		{realtimeTableSuffixKey, base.TableSuffix},
		{realtimeQueuePrefixKey, base.QueuePrefix},
		{realtimeTopicKey, base.Topic},
	} {
		if declared[1] == "" {
			return RealtimeBase{}, xerrors.Wrap(ErrRealtimeBaseMissing, declared[0])
		}
	}

	return base, nil
}
