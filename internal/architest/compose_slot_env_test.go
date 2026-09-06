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

	// overlayServiceName は、override が解決値を渡すサービスです。
	overlayServiceName = "api_server"
)

// containerBound は、override が app コンテナへ必ず渡す解決値と、それを載せる環境変数名の対応です。
// 渡し損ねると app には変数が届かず、config.Load() が埋め込み env/.env の値（スロットを持たない
// checkout の資源名）で埋めるため、スロットを保持している worktree でも主 checkout と同じ
// データベース・stream を掴みます。make の require-db-owner はスロット未取得のリンク worktree しか
// 止めないので、この経路は塞ぎません。
//
// 一覧であって除外リストではありません。両方向で検査します — ここに挙げた名前は RenderEnv が
// 出していなければならず（陳腐化した項目が落ちる）、override はそのすべてを必須形で宣言して
// いなければなりません。
var containerBound = map[string]string{
	"DB_LOCAL":              "DB_NAME",
	"REALTIME_TOPIC":        "REALTIME_TOPIC",
	"REALTIME_QUEUE_PREFIX": "REALTIME_QUEUE_PREFIX",
	"REALTIME_TABLE_SUFFIX": "REALTIME_TABLE_SUFFIX",
}

var (
	// interpolationRe は、値に含まれる compose の変数参照を捕捉します。先頭の選択肢が $$（リテラルの $）を
	// 先に食うので、$${NAME} は参照として拾いません。波括弧付き ${NAME<残り>} と裸の $NAME の両方を見るのは、
	// 裸の形も compose が展開する一方で値の不在を止める術が無く、書き換えられると検査が素通りするためです。
	interpolationRe = regexp.MustCompile(`\$\$|\$\{(\w+)([^}]*)\}|\$(\w+)`)

	// renderEnvPairRe は、RenderEnv が並べる {"KEY", value} の 1 行にマッチし、キー名を捕捉します。
	renderEnvPairRe = regexp.MustCompile(`^\t+\{"(\w+)",`)

	// requiredOperator は、値が無ければ補間段で止める compose の演算子です。
	// `:-` は既定値へ、`?`（コロン無し）と裸の参照は空文字へ落ちるため、これ以外は必須になりません。
	requiredOperator = ":?"
)

var (
	// errOverlayService は、override が対象サービスを宣言していないことを表す。
	errOverlayService = xerrors.New("the compose overlay declares no service")
	// errOverlayEnv は、対象サービスが environment を宣言していないことを表す。
	errOverlayEnv = xerrors.New("the service declares no environment")
)

// interpolation は、override の値が参照する 1 つの変数です。
type interpolation struct {
	name     string // 参照している変数名
	required bool   // 値が無ければ補間段で止まる形か
}

// TestAttachOverlayRequiresResolvedEnv は、db-slot env が撒く解決値を app 層の override が
// 必須の形で受けていることを機械検証する。
//
// 突き合わせる集合は 2 つ。P = internal/cli/dbslot の RenderEnv が並べるキー、
// C = override の api_server.environment が持つ変数参照（env のキー名ではなく参照先の変数名）。
// この 2 つに containerBound（コンテナへ必ず渡す値の宣言）を重ねて、3 方向で見る。
//
// ① C のうち P を参照するものは必須形（${VAR:?メッセージ}）でなければならない — 本テストの存在理由。
// override が既定値を置き直すと、その既定値は解決側の正本の複製になって黙ってずれ、`:-` は空値を
// その複製へ置換して、この checkout に主 checkout の資源を掴ませる。P を参照しない
// ${MOCK_AUTH_HOST_PORT:-2010} のような値は、解決側に正本が無いので対象外。
//
// ② containerBound の各項目を override が宣言している — 宣言ごと消えると app には変数が届かず、
// config.Load() が埋め込み env/.env の値で埋めるため、スロットを保持していても主 checkout の資源を掴む。
//
// ③ 必須形で参照している変数は P に含まれている — producer 側のリネームで誰も撒かない変数を
// override が要求する状態は、必須形にした以上 make serve を補間段で必ず落とす。
func TestAttachOverlayRequiresResolvedEnv(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	produced := renderEnvKeys(readRepoFile(t, root, resolverSourceFile))
	require.NotEmpty(t, produced, "db-slot env が値を 1 つも出していない")

	declared, err := overlayEnv(readRepoFile(t, root, attachOverlayFile))
	require.NoError(t, err)

	optional, missing, unbound := classifyOverlayEnv(produced, containerBound, declared)

	assert.Emptyf(t, optional,
		"db-slot env が撒く値を %s が必須として受けていない。既定値を置くと解決側の複製になり、"+
			"空値はこの checkout に主 checkout の資源を掴ませる", attachOverlayFile)
	assert.Emptyf(t, missing,
		"%s がコンテナへ渡すべき解決値を宣言していない。app には変数が届かず、"+
			"config.Load() が埋め込み env/.env の値で埋める", attachOverlayFile)
	assert.Emptyf(t, unbound,
		"%s が要求する値を db-slot env が撒いていない。必須形のため make serve が補間段で落ちる",
		attachOverlayFile)
}

func Test_classifyOverlayEnv(t *testing.T) {
	t.Parallel()

	produced := []string{"APP_PROJECT", "DB_LOCAL", "REALTIME_TOPIC"}
	bound := map[string]string{"DB_LOCAL": "DB_NAME", "REALTIME_TOPIC": "REALTIME_TOPIC"}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("すべて必須形で宣言されていれば違反なし", func(t *testing.T) {
			t.Parallel()

			optional, missing, unbound := classifyOverlayEnv(produced, bound, map[string]string{
				"DB_NAME":        "${DB_LOCAL:?msg}",
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
			})

			assert.Empty(t, optional)
			assert.Empty(t, missing)
			assert.Empty(t, unbound)
		})

		t.Run("解決値を参照しない補間は対象外", func(t *testing.T) {
			t.Parallel()

			optional, _, unbound := classifyOverlayEnv(produced, bound, map[string]string{
				"DB_NAME":        "${DB_LOCAL:?msg}",
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
				"AUTH_ISSUER":    "http://localhost:${MOCK_AUTH_HOST_PORT:-2010}/default",
			})

			assert.Empty(t, optional)
			assert.Empty(t, unbound)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解決値を既定値付きで参照していれば ① だけが発火する", func(t *testing.T) {
			t.Parallel()

			optional, missing, unbound := classifyOverlayEnv(produced, bound, map[string]string{
				"DB_NAME":        "${DB_LOCAL:-local}",
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
			})

			assert.Equal(t, []string{"DB_NAME: DB_LOCAL を :? なしで参照している（${DB_LOCAL:-local}）"}, optional)
			assert.Empty(t, missing)
			assert.Empty(t, unbound)
		})

		t.Run("解決値を裸の $VAR で参照していても ① が発火する", func(t *testing.T) {
			t.Parallel()

			optional, _, _ := classifyOverlayEnv(produced, bound, map[string]string{
				"DB_NAME":        "$DB_LOCAL",
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
			})

			assert.Equal(t, []string{"DB_NAME: DB_LOCAL を :? なしで参照している（$DB_LOCAL）"}, optional)
		})

		t.Run("宣言ごと消えていれば ② だけが発火する", func(t *testing.T) {
			t.Parallel()

			optional, missing, unbound := classifyOverlayEnv(produced, bound, map[string]string{
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
			})

			assert.Empty(t, optional)
			assert.Equal(t, []string{"DB_NAME: DB_LOCAL を渡していない"}, missing)
			assert.Empty(t, unbound)
		})

		t.Run("キー名だけ変えて参照を残しても ② が発火する", func(t *testing.T) {
			t.Parallel()

			_, missing, _ := classifyOverlayEnv(produced, bound, map[string]string{
				"DB_NAME_ALT":    "${DB_LOCAL:?msg}",
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
			})

			assert.Equal(t, []string{"DB_NAME: DB_LOCAL を渡していない"}, missing)
		})

		t.Run("誰も撒かない変数を必須形で要求していれば ③ だけが発火する", func(t *testing.T) {
			t.Parallel()

			optional, missing, unbound := classifyOverlayEnv(produced, bound, map[string]string{
				"DB_NAME":        "${DB_LOCAL:?msg}",
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
				"EXTRA":          "${NOBODY:?msg}",
			})

			assert.Empty(t, optional)
			assert.Empty(t, missing)
			assert.Equal(t, []string{"EXTRA: NOBODY を誰も撒いていない"}, unbound)
		})

		t.Run("宣言が producer から消えていれば ③ が発火する", func(t *testing.T) {
			t.Parallel()

			_, _, unbound := classifyOverlayEnv([]string{"REALTIME_TOPIC"}, bound, map[string]string{
				"DB_NAME":        "${DB_LOCAL:?msg}",
				"REALTIME_TOPIC": "${REALTIME_TOPIC:?msg}",
			})

			assert.Equal(t, []string{
				"DB_NAME: DB_LOCAL を誰も撒いていない",
				"containerBound: DB_LOCAL を RenderEnv が出していない",
			}, unbound)
		})
	})
}

func Test_overlayEnv(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象サービスの environment をそのまま返す", func(t *testing.T) {
			t.Parallel()

			declared, err := overlayEnv("services:\n" +
				"  api_server:\n" +
				"    ports: !override\n" +
				"      - \"8080:8080\"\n" +
				"    environment:\n" +
				"      DB_NAME: ${DB_LOCAL:?msg}\n" +
				"      ENDPOINT_REALTIME: http://host.docker.internal:8000\n" +
				"  mock_auth_server:\n" +
				"    environment:\n" +
				"      DB_NAME: ${OTHER:?msg}\n")

			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				"DB_NAME":           "${DB_LOCAL:?msg}",
				"ENDPOINT_REALTIME": "http://host.docker.internal:8000",
			}, declared)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象サービスが無ければエラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := overlayEnv("services:\n  mock_auth_server:\n    environment:\n      A: b\n")

			require.ErrorIs(t, err, errOverlayService)
		})

		t.Run("対象サービスが environment を持たなければエラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := overlayEnv("services:\n  api_server:\n    ports:\n      - \"8080:8080\"\n")

			require.ErrorIs(t, err, errOverlayEnv)
		})

		t.Run("YAML として読めなければエラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := overlayEnv("services:\n  api_server:\n   - broken\n")

			require.Error(t, err)
			require.NotErrorIs(t, err, errOverlayService)
			require.NotErrorIs(t, err, errOverlayEnv)
		})
	})
}

func Test_interpolations(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("メッセージ付きの :? だけを必須と見なす", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "DB_LOCAL", required: true}},
				interpolations("${DB_LOCAL:?msg}"))
		})

		t.Run("値の途中に埋め込まれた参照も拾う", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "MOCK_AUTH_HOST_PORT", required: false}},
				interpolations("http://localhost:${MOCK_AUTH_HOST_PORT:-2010}/default"))
		})

		t.Run("1 つの値が複数の参照を持てる", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{
				{name: "A", required: true},
				{name: "B", required: false},
			}, interpolations("${A:?msg}-${B:-x}"))
		})

		t.Run("参照を含まない値は空を返す", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, interpolations("http://host.docker.internal:8000"))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("既定値付きは必須と見なさない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "DB_LOCAL", required: false}},
				interpolations("${DB_LOCAL:-local}"))
		})

		t.Run("コロン無しの ? は空文字を通すため必須と見なさない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "DB_LOCAL", required: false}},
				interpolations("${DB_LOCAL?msg}"))
		})

		t.Run("裸の参照は必須と見なさない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "DB_LOCAL", required: false}},
				interpolations("${DB_LOCAL}"))
		})

		t.Run("メッセージが空なら必須と見なさない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "DB_LOCAL", required: false}},
				interpolations("${DB_LOCAL:?}"))
		})

		t.Run(":+ は値の不在を止めないため必須と見なさない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "DB_LOCAL", required: false}},
				interpolations("${DB_LOCAL:+alt}"))
		})

		t.Run("波括弧無しの $VAR も参照として拾う", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []interpolation{{name: "DB_LOCAL", required: false}},
				interpolations("$DB_LOCAL"))
		})

		t.Run("$$ はエスケープなので参照として数えない", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, interpolations("$${DB_LOCAL}"))
		})
	})
}

func Test_renderEnvKeys(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("RenderEnv 本体のキーをソートして返す", func(t *testing.T) {
			t.Parallel()

			keys := renderEnvKeys("func RenderEnv(v Values) string {\n" +
				"\tpairs := [][2]string{\n" +
				"\t\t{\"DB_LOCAL\", v.DBLocal},\n" +
				"\t\t{\"REALTIME_TOPIC\", v.RealtimeTopic},\n" +
				"\t\t{\"APP_PROJECT\", v.AppProject},\n" +
				"\t}\n" +
				"}\n")

			assert.Equal(t, []string{"APP_PROJECT", "DB_LOCAL", "REALTIME_TOPIC"}, keys)
		})

		t.Run("RenderEnv の外にある同形のキーは拾わない", func(t *testing.T) {
			t.Parallel()

			keys := renderEnvKeys("func Other(v Values) string {\n" +
				"\tpairs := [][2]string{\n" +
				"\t\t{\"STRAY\", v.Stray},\n" +
				"\t}\n" +
				"}\n" +
				"\n" +
				"func RenderEnv(v Values) string {\n" +
				"\tpairs := [][2]string{\n" +
				"\t\t{\"DB_LOCAL\", v.DBLocal},\n" +
				"\t}\n" +
				"}\n")

			assert.Equal(t, []string{"DB_LOCAL"}, keys)
		})
	})
}

// renderEnvKeys は、RenderEnv の本体が並べるキーを返します。
// production パッケージを import せずソースを走査するのは、この package が internal/** へ依存グラフを
// 伸ばさないためです（既存の検査もすべて gofmt 済みソースのテキスト走査で完結しています）。
func renderEnvKeys(src string) []string {
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
			if m != nil {
				keys = append(keys, m[1])
			}
		}
	}
	sort.Strings(keys)

	return keys
}

// overlayEnv は、compose override が overlayServiceName へ渡す environment を返します。
// サービスと environment のどちらかへ辿り着けなければエラーにします。走査が対象を見失った状態を
// 「違反なし」と報告すると、何も見ていないまま緑であり続けるためです。
func overlayEnv(doc string) (map[string]string, error) {
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

	return service.Environment, nil
}

// interpolations は、値が参照している変数を出現順に返します。
// 必須と見なすのは `${NAME:?メッセージ}` だけです。`${NAME:-既定}` は既定値へ、`${NAME?メッセージ}`・
// 裸の `${NAME}`・波括弧無しの `$NAME` は空文字へ落ちるため、いずれも値の不在を止めません。
// `$$` は compose のエスケープ（リテラルの `$`）なので参照として数えません。
func interpolations(value string) []interpolation {
	var refs []interpolation

	for _, m := range interpolationRe.FindAllStringSubmatch(value, -1) {
		switch {
		case m[1] != "":
			refs = append(refs, interpolation{
				name:     m[1],
				required: strings.HasPrefix(m[2], requiredOperator) && len(m[2]) > len(requiredOperator),
			})
		case m[3] != "":
			refs = append(refs, interpolation{name: m[3]})
		}
	}

	return refs
}

// classifyOverlayEnv は、解決値の集合 produced・コンテナへ渡す宣言 bound・override の宣言 declared を
// 突き合わせ、3 方向の違反を宣言箇所つき・安定順で返します。
func classifyOverlayEnv(produced []string, bound, declared map[string]string) (optional, missing, unbound []string) {
	for key, value := range declared {
		for _, ref := range interpolations(value) {
			switch {
			case slices.Contains(produced, ref.name) && !ref.required:
				optional = append(optional,
					fmt.Sprintf("%s: %s を %s なしで参照している（%s）", key, ref.name, requiredOperator, value))
			case ref.required && !slices.Contains(produced, ref.name):
				unbound = append(unbound, fmt.Sprintf("%s: %s を誰も撒いていない", key, ref.name))
			}
		}
	}

	for name, key := range bound {
		if !slices.Contains(produced, name) {
			unbound = append(unbound, fmt.Sprintf("containerBound: %s を RenderEnv が出していない", name))
		}
		if !slices.ContainsFunc(interpolations(declared[key]), func(r interpolation) bool { return r.name == name }) {
			missing = append(missing, fmt.Sprintf("%s: %s を渡していない", key, name))
		}
	}

	sort.Strings(optional)
	sort.Strings(missing)
	sort.Strings(unbound)

	return optional, missing, unbound
}
