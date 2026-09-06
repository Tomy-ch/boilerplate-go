package architest

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	// attachOverlayFile は、app 層の override です。db-slot env が撒く値の 2 つの消費者のうち compose 側。
	attachOverlayFile = "docker-compose.attach.yaml"

	// resolverSourceFile は、db-slot env が撒く値を組み立てる RenderEnv の定義元です。
	resolverSourceFile = "internal/cli/dbslot/resolve.go"

	// renderEnvFuncDecl は、RenderEnv の本体を走査する範囲の開始行です。
	renderEnvFuncDecl = "func RenderEnv("

	// overlayServiceName は、override が REALTIME_* を渡すサービスです。
	overlayServiceName = "api_server"

	// realtimeEnvPrefix は、突き合わせる変数名の接頭辞です。ENDPOINT_REALTIME / ENDPOINT_REALTIME_PUBSUB は
	// 共有インフラの接続先でスロットの関数ではないため、この接頭辞から外れて対象になりません。
	realtimeEnvPrefix = "REALTIME_"
)

// requiredInterpolationRe は、値を必須とする補間の形（${KEY:?メッセージ}）にマッチし、変数名を捕捉します。
var requiredInterpolationRe = regexp.MustCompile(`^\$\{(\w+):\?([^}]+)\}$`)

// renderEnvPairRe は、RenderEnv が並べる {"KEY", value} の 1 行にマッチし、キー名を捕捉します。
var renderEnvPairRe = regexp.MustCompile(`^\t+\{"(\w+)",`)

var (
	// errOverlayService は、override が対象サービスを宣言していないことを表す。
	errOverlayService = xerrors.New("the compose overlay declares no service")
	// errOverlayEnv は、対象サービスが environment を宣言していないことを表す。
	errOverlayEnv = xerrors.New("the service declares no environment")
)

// TestAttachOverlayRequiresRealtimeEnv は、db-slot env が撒く REALTIME_* と、app 層の override が
// 要求する REALTIME_* が一致し、override 側が値を必須の形で受けていることを機械検証する。
//
// 突き合わせる集合は 2 つ。P = internal/cli/dbslot の RenderEnv が出すキー、
// C = override の api_server.environment が宣言する REALTIME_* のキー。
//
// P → C が本テストの存在理由で、override が既定値（`${VAR:-...}`）を持ち直すことを落とす。
// 既定値は env/.env（LoadRealtimeBase が読む正本）の複製になって黙ってずれ、`:-` は空値を
// 主 checkout の資源名へ置換して 2 つの checkout を同じ stream に載せる。
// C → P は逆向きで、producer 側のリネームにより誰も撒かない変数を override が要求している状態を
// 検出する。必須形にした以上、その状態は make serve を補間段で必ず落とす。
func TestAttachOverlayRequiresRealtimeEnv(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	produced := realtimeProducedKeys(readRepoFile(t, root, resolverSourceFile))
	require.NotEmpty(t, produced, "db-slot env が REALTIME_* を 1 つも出していない")

	declared, err := realtimeOverlayEnv(readRepoFile(t, root, attachOverlayFile))
	require.NoError(t, err)

	var unrequired []string
	for _, key := range produced {
		value, ok := declared[key]
		switch {
		case !ok:
			unrequired = append(unrequired, key+": override が宣言していない")
		case !isRequiredInterpolation(key, value):
			unrequired = append(unrequired, fmt.Sprintf("%s: 必須形 ${%s:?メッセージ} でない（%s）", key, key, value))
		}
	}
	sort.Strings(unrequired)
	assert.Emptyf(t, unrequired,
		"db-slot env が撒く値を %s が必須として受けていない。既定値を置くと env/.env の複製になり、"+
			"空値はこの checkout に主 checkout の資源を掴ませる", attachOverlayFile)

	var unproduced []string
	for key := range declared {
		if !slices.Contains(produced, key) {
			unproduced = append(unproduced, key)
		}
	}
	sort.Strings(unproduced)
	assert.Emptyf(t, unproduced,
		"%s が要求する値を db-slot env が撒いていない。必須形のため make serve が補間段で落ちる",
		attachOverlayFile)
}

func Test_realtimeOverlayEnv(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象サービスの REALTIME_ 接頭辞だけを返す", func(t *testing.T) {
			t.Parallel()

			declared, err := realtimeOverlayEnv("services:\n" +
				"  api_server:\n" +
				"    environment:\n" +
				"      REALTIME_TOPIC: ${REALTIME_TOPIC:?msg}\n" +
				"      REALTIME_QUEUE_PREFIX: ${REALTIME_QUEUE_PREFIX:?msg}\n" +
				"      REALTIME_TABLE_SUFFIX: ${REALTIME_TABLE_SUFFIX:?msg}\n" +
				"      ENDPOINT_REALTIME: http://host.docker.internal:8000\n" +
				"  mock_auth_server:\n" +
				"    environment:\n" +
				"      REALTIME_TOPIC: ${OTHER_TOPIC:?msg}\n")

			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				"REALTIME_TOPIC":        "${REALTIME_TOPIC:?msg}",
				"REALTIME_QUEUE_PREFIX": "${REALTIME_QUEUE_PREFIX:?msg}",
				"REALTIME_TABLE_SUFFIX": "${REALTIME_TABLE_SUFFIX:?msg}",
			}, declared)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象サービスが無ければエラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := realtimeOverlayEnv("services:\n  mock_auth_server:\n    environment:\n      A: b\n")

			require.ErrorIs(t, err, errOverlayService)
		})

		t.Run("対象サービスが environment を持たなければエラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := realtimeOverlayEnv("services:\n  api_server:\n    ports:\n      - \"8080:8080\"\n")

			require.ErrorIs(t, err, errOverlayEnv)
		})

		t.Run("YAML として読めなければエラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := realtimeOverlayEnv("services:\n  api_server:\n   - broken\n")

			require.Error(t, err)
			assert.NotErrorIs(t, err, errOverlayService)
			assert.NotErrorIs(t, err, errOverlayEnv)
		})
	})
}

func Test_isRequiredInterpolation(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("メッセージ付きの必須形を受理する", func(t *testing.T) {
			t.Parallel()

			assert.True(t, isRequiredInterpolation("REALTIME_TOPIC", "${REALTIME_TOPIC:?db-slot env が撒く値}"))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既定値付きは受理しない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isRequiredInterpolation("REALTIME_TOPIC", "${REALTIME_TOPIC:-local}"))
		})

		t.Run("コロン無しの ? は空文字を通すため受理しない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isRequiredInterpolation("REALTIME_TOPIC", "${REALTIME_TOPIC?msg}"))
		})

		t.Run("裸の参照は空文字を通すため受理しない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isRequiredInterpolation("REALTIME_TOPIC", "${REALTIME_TOPIC}"))
		})

		t.Run("メッセージが空なら受理しない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isRequiredInterpolation("REALTIME_TOPIC", "${REALTIME_TOPIC:?}"))
		})

		t.Run("別の変数を参照していれば受理しない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isRequiredInterpolation("REALTIME_TOPIC", "${REALTIME_QUEUE_PREFIX:?msg}"))
		})

		t.Run("前後に文字列が付く形は受理しない", func(t *testing.T) {
			t.Parallel()

			assert.False(t, isRequiredInterpolation("REALTIME_TOPIC", "prefix-${REALTIME_TOPIC:?msg}"))
		})
	})
}

func Test_realtimeProducedKeys(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("RenderEnv 本体の REALTIME_ 接頭辞のキーだけをソートして返す", func(t *testing.T) {
			t.Parallel()

			keys := realtimeProducedKeys("func RenderEnv(v Values) string {\n" +
				"\tpairs := [][2]string{\n" +
				"\t\t{\"DB_LOCAL\", v.DBLocal},\n" +
				"\t\t{\"REALTIME_TOPIC\", v.RealtimeTopic},\n" +
				"\t\t{\"REALTIME_TABLE_SUFFIX\", v.RealtimeTableSuffix},\n" +
				"\t\t{\"REALTIME_QUEUE_PREFIX\", v.RealtimeQueuePrefix},\n" +
				"\t}\n" +
				"}\n")

			assert.Equal(t,
				[]string{"REALTIME_QUEUE_PREFIX", "REALTIME_TABLE_SUFFIX", "REALTIME_TOPIC"}, keys)
		})

		t.Run("RenderEnv の外にある同形のキーは拾わない", func(t *testing.T) {
			t.Parallel()

			keys := realtimeProducedKeys("func Other(v Values) string {\n" +
				"\tpairs := [][2]string{\n" +
				"\t\t{\"REALTIME_STRAY\", v.Stray},\n" +
				"\t}\n" +
				"}\n" +
				"\n" +
				"func RenderEnv(v Values) string {\n" +
				"\tpairs := [][2]string{\n" +
				"\t\t{\"REALTIME_TOPIC\", v.RealtimeTopic},\n" +
				"\t}\n" +
				"}\n")

			assert.Equal(t, []string{"REALTIME_TOPIC"}, keys)
		})
	})
}

// realtimeProducedKeys は、RenderEnv の本体が並べるキーのうち REALTIME_ 接頭辞のものを返します。
// production パッケージを import せずソースを走査するのは、この package が internal/** へ依存グラフを
// 伸ばさないためです（既存の検査もすべて gofmt 済みソースのテキスト走査で完結しています）。
func realtimeProducedKeys(src string) []string {
	var (
		keys   []string
		inBody bool
	)

	for line := range strings.SplitSeq(src, "\n") {
		switch {
		case strings.HasPrefix(line, renderEnvFuncDecl):
			inBody = true
		case inBody && line == "}":
			inBody = false
		case inBody:
			m := renderEnvPairRe.FindStringSubmatch(line)
			if m != nil && strings.HasPrefix(m[1], realtimeEnvPrefix) {
				keys = append(keys, m[1])
			}
		}
	}
	sort.Strings(keys)

	return keys
}

// realtimeOverlayEnv は、compose override が overlayServiceName へ渡す REALTIME_* の宣言を返します。
// サービスと environment のどちらかへ辿り着けなければエラーにします。走査が対象を見失った状態を
// 「違反なし」と報告すると、何も見ていないまま緑であり続けるためです。
func realtimeOverlayEnv(doc string) (map[string]string, error) {
	var parsed struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
		return nil, xerrors.Wrap(err, "failed to parse "+attachOverlayFile)
	}

	service, ok := parsed.Services[overlayServiceName]
	if !ok {
		return nil, xerrors.Wrap(errOverlayService, overlayServiceName)
	}
	if len(service.Environment) == 0 {
		return nil, xerrors.Wrap(errOverlayEnv, overlayServiceName)
	}

	declared := make(map[string]string)
	for key, value := range service.Environment {
		if strings.HasPrefix(key, realtimeEnvPrefix) {
			declared[key] = value
		}
	}

	return declared, nil
}

// isRequiredInterpolation は、値が key を必須として参照する補間かを返します。
// `${KEY:-既定}` は既定値が別の正本の複製になり空値の受け皿にもなるため、`${KEY?メッセージ}` と
// 裸の `${KEY}` は空文字を通すため、いずれも満たしません。
func isRequiredInterpolation(key, value string) bool {
	m := requiredInterpolationRe.FindStringSubmatch(value)

	return m != nil && m[1] == key
}
