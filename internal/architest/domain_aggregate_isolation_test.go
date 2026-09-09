package architest

// 集約境界の隔離規約（internal/domain/README.md の Cross-aggregate reference）を機械的に検査する。
//
// domain パッケージが他の domain パッケージを import してよいのは次の 2 つだけ:
//
//  1. internal/domain/lexicon 配下（集約横断で共有する業務 VO。ADR-0039）
//  2. 自分の集約の internal/ 配下（sub-entity。他集約からは Go 自身が到達を塞ぐ）
//
// この規約が lint ではなくここに居るのは、depguard が importer を files でしか見られず
// allow / deny は imported しか見ないためである。「自分の集約の internal/ 配下だけ許す」は
// 2 端点の関係なので、depguard の 1 本のルールでは原理的に書けない。
//
// import は正規表現で拾う（realtime_isolation_test.go と同じ方式）。build tag で除外される
// ファイルの import も拾うため過大近似になるが、禁止規則としては安全側に倒れる。

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgfs "go-boilerplate/pkg/fs"
)

// domainRoot は、集約が並ぶディレクトリ（repo root 相対）。
const domainRoot = "internal/domain"

// domainImportPrefix は、domain パッケージを指す import path の接頭辞。
const domainImportPrefix = "go-boilerplate/internal/domain/"

// lexiconAggregate は、集約横断で共有する業務 VO の置き場。どの集約からも import してよい。
const lexiconAggregate = "lexicon"

// serviceAggregate は、Domain Service の置き場。集約の import を許されており本規約の対象外
// （.golangci.yaml の maintain_a_sound_domain_service が受け持つ）。
const serviceAggregate = "service"

// subEntityMarker は、sub-entity のパッケージを他集約から隔離する目印。Go 自身が
// internal/ の外からの import を拒否するため、この規約と二重に守られる。
const subEntityMarker = "internal"

func TestDomainAggregateImportIsolation(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("どの集約も他集約を直接 import していない", func(t *testing.T) {
			t.Parallel()

			violations, err := collectDomainAggregateViolations(moduleRoot(t))
			require.NoError(t, err)
			assert.Empty(t, violations,
				"domain の集約が他集約を直接 import している。集約跨ぎの参照は識別子だけにし、"+
					"共有したい業務 VO は internal/domain/lexicon へ置く"+
					"（internal/domain/README.md の Cross-aggregate reference）")
		})

		t.Run("既知の違反を含むツリーを渡すと検出する", func(t *testing.T) {
			t.Parallel()

			// 走査が黙って空になっても上の Empty は通ってしまうため、陽性対照を置く。
			// 実ツリーの中身に依存しないので、sample 撤去の前後どちらでも同じように効く。
			root := t.TempDir()
			dir := filepath.Join(root, domainRoot, "alpha")
			require.NoError(t, pkgfs.OS{}.MkdirAll(dir, 0o750))
			require.NoError(t, pkgfs.OS{}.WriteFile(
				filepath.Join(dir, "alpha.go"),
				[]byte("package alpha\n\nimport \"go-boilerplate/internal/domain/beta\"\n"),
				0o600,
			))

			violations, err := collectDomainAggregateViolations(root)
			require.NoError(t, err)
			assert.Equal(t, []string{"internal/domain/alpha/alpha.go -> go-boilerplate/internal/domain/beta"}, violations)
		})
	})
}

func Test_domainImportViolation(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("lexicon はどの集約からも許す", func(t *testing.T) {
			t.Parallel()
			assert.False(t, domainImportViolation("cart", domainImportPrefix+"lexicon/money"))
		})

		t.Run("自分の集約の internal 配下は許す", func(t *testing.T) {
			t.Parallel()
			assert.False(t, domainImportViolation("inquiry", domainImportPrefix+"inquiry/internal/message"))
		})

		t.Run("domain 以外の import は対象外", func(t *testing.T) {
			t.Parallel()
			assert.False(t, domainImportViolation("cart", "go-boilerplate/pkg/uuid"))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("他集約の直接 import は違反", func(t *testing.T) {
			t.Parallel()
			assert.True(t, domainImportViolation("cart", domainImportPrefix+"inquiry"))
		})

		t.Run("他集約の internal 配下も違反", func(t *testing.T) {
			t.Parallel()
			// Go 自身も塞ぐが、規約としてもここで落とす。
			assert.True(t, domainImportViolation("cart", domainImportPrefix+"inquiry/internal/message"))
		})

		t.Run("自分の配下でも internal を経由しないネストは違反", func(t *testing.T) {
			t.Parallel()
			// product/category は自前の Repository を持つ別集約であり sub-entity ではない。
			// パス形状だけでは sub-entity と区別できないため、internal/ を目印にして分ける。
			assert.True(t, domainImportViolation("product", domainImportPrefix+"product/category"))
		})

		t.Run("sub-entity から親の Root への import は違反", func(t *testing.T) {
			t.Parallel()
			// sub-entity は親への逆参照を持たない（internal/domain/README.md）。
			assert.True(t, domainImportViolation("inquiry", domainImportPrefix+"inquiry"))
		})
	})
}

func Test_aggregateOfDomainPath(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("集約直下のファイルから集約名を返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "cart", aggregateOfDomainPath("internal/domain/cart/cart_domain.go"))
		})

		t.Run("ネストしたファイルでも最上位の集約名を返す", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "inquiry", aggregateOfDomainPath("internal/domain/inquiry/internal/message/message.go"))
		})

		t.Run("domain 配下でなければ空を返す", func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, aggregateOfDomainPath("internal/usecase/inquiry/inquiry_usecase.go"))
		})
	})
}

// collectDomainAggregateViolations は、domain 配下の production code が持つ規約違反の import を
// `file -> import` の形で返す。
func collectDomainAggregateViolations(root string) ([]string, error) {
	var violations []string

	err := walkProductionGoFiles(filepath.Join(root, domainRoot), func(path string, content []byte) {
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		aggregate := aggregateOfDomainPath(rel)
		if aggregate == "" || aggregate == serviceAggregate {
			return
		}

		for _, m := range internalImportRe.FindAllStringSubmatch(string(content), -1) {
			if domainImportViolation(aggregate, m[1]) {
				violations = append(violations, rel+" -> "+m[1])
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return violations, nil
}

// domainImportViolation は、aggregate に属するファイルからの imp が規約違反かを返す。
func domainImportViolation(aggregate, imp string) bool {
	rest, ok := strings.CutPrefix(imp, domainImportPrefix)
	if !ok {
		return false
	}

	target, sub, _ := strings.Cut(rest, "/")
	switch {
	case target == lexiconAggregate:
		return false
	case target != aggregate:
		return true
	default:
		// 自分の集約の中でも、sub-entity の目印を経由しないネストは別集約とみなす。
		return sub != subEntityMarker && !strings.HasPrefix(sub, subEntityMarker+"/")
	}
}

// aggregateOfDomainPath は、repo root 相対のパスから集約名を返す。domain 配下でなければ空を返す。
func aggregateOfDomainPath(rel string) string {
	rest, ok := strings.CutPrefix(rel, domainRoot+"/")
	if !ok {
		return ""
	}

	aggregate, _, found := strings.Cut(rest, "/")
	if !found {
		return ""
	}

	return aggregate
}
