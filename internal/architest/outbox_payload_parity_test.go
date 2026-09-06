package architest

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	pkgfs "go-boilerplate/pkg/fs"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// usecaseRoot は、event パッケージを探す起点です。
const usecaseRoot = "internal/usecase"

// parityFileName は、各 event パッケージが置く対応宣言のファイル名です。
const parityFileName = "payload_parity.yaml"

// buildFuncRe は、payload を組み立てる `func Build<Name>(` にマッチし Name を捕捉します。
var buildFuncRe = regexp.MustCompile(`^func Build(\w+)\(`)

// structOpenRe は、`type <Name> struct {` にマッチし Name を捕捉します。
var structOpenRe = regexp.MustCompile(`^type (\w+) struct \{$`)

// structFieldRe は、gofmt 済み struct の 1 フィールド行にマッチし名前を捕捉します。
var structFieldRe = regexp.MustCompile(`^\t(\w+) +\S`)

// jsonTagRe は、struct タグから JSON 名（オプションを除いた部分）を捕捉します。
var jsonTagRe = regexp.MustCompile(`json:"([^",]+)`)

// payloadParityDoc は、payload_parity.yaml の全体です。
type payloadParityDoc struct {
	Payloads map[string]payloadParityEntry `yaml:"payloads"`
}

// payloadParityEntry は、payload 1 つ分の宣言です。Fields は snapshot のときだけ書きます。
type payloadParityEntry struct {
	Kind   string                      `yaml:"kind"`
	Of     string                      `yaml:"of"`
	Fields map[string]payloadFieldRule `yaml:"fields"`
}

// payloadFieldRule は、集約のフィールド 1 つの扱いです。
// スカラは運ぶ（値は payload の JSON 名）、マッピングは運ばない（omit に理由）を表します。
type payloadFieldRule struct {
	Carried string
	Omit    string
}

// UnmarshalYAML は、スカラ形と `{omit: ...}` 形の両方を受け取ります。
func (r *payloadFieldRule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		r.Carried = value.Value

		return nil
	}

	var m struct {
		Omit string `yaml:"omit"`
	}
	if err := value.Decode(&m); err != nil {
		return err
	}
	r.Omit = m.Omit

	return nil
}

// TestOutboxPayloadParity は、outbox の payload と、その写し元となる集約との対応が
// 宣言されていることを機械検証します。
//
// 集約にフィールドが増えたとき、payload 側は黙って古いままになれます。#1473 では
// 課税の基礎を値引き後へ変えたのに purchase.created.v1 が値引き額を運ばず、購読側で
// subtotal + tax + shipping が total と一致しない payload が出荷されました。これを
// 落とすのがこの検査です。
//
// 「自己完結」を名乗るだけでは判別式になりません。8 payload はすべてそう名乗りますが、
// 状態を運ぶもの（snapshot）と、起きた事実と識別子だけを運ぶもの（notification）が
// 混ざっており、後者が集約の全フィールドを持たないのは正しい姿です。そこで各 payload に
// 種別を宣言させ、snapshot に限って集約の全フィールドが「運ぶ / 運ばない（理由つき）」に
// 分類されていることを課します（ADR-0113 (outbox-payload-kinds-and-parity-declaration)）。
//
// 対象 0 件は許容します（sample API 撤去後は event パッケージごと消えます）。走査が黙って
// 空になる縮退は、下の陽性対照と Test_collect* が受け持ちます。
func TestOutboxPayloadParity(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("すべての event パッケージが対応を宣言し、宣言が実物と一致する", func(t *testing.T) {
			t.Parallel()

			violations, err := collectOutboxPayloadParityViolations(moduleRoot(t))
			require.NoError(t, err)
			assert.Empty(t, violations,
				"outbox payload と集約の対応宣言が実物とずれている。"+
					"payload_parity.yaml を実物へ合わせるか、集約に増えたフィールドの扱いを宣言すること"+
					"（ADR-0113 (outbox-payload-kinds-and-parity-declaration)）")
		})

		t.Run("event パッケージが 1 つも無いツリーは違反なしで通す", func(t *testing.T) {
			t.Parallel()

			// sample API 撤去後は internal/usecase ごと消えるため、対象 0 件は正常。
			// 走査が黙って空になる縮退のほうは Test_collect* が受け持つ。
			violations, err := collectOutboxPayloadParityViolations(t.TempDir())

			require.NoError(t, err)
			assert.Empty(t, violations)
		})

		t.Run("宣言の無い event パッケージを検出する", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeEventPackage(t, root, "alpha", "package event\n\nfunc BuildOpened(x int) ([]byte, error) { return nil, nil }\n")

			violations, err := collectOutboxPayloadParityViolations(root)
			require.NoError(t, err)
			assert.Equal(t, []string{
				"internal/usecase/alpha/event: payload_parity.yaml が無い（payload: Opened）",
			}, violations)
		})

		t.Run("宣言から漏れた集約フィールドを検出する", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeEventPackage(t, root, "beta",
				"package event\n\ntype opened struct {\n\tID string `json:\"id\"`\n}\n\nfunc BuildOpened(x int) ([]byte, error) { return nil, nil }\n")
			writeParityYAML(t, root, "beta", "payloads:\n  opened:\n    kind: snapshot\n    of: beta.Thing\n    fields:\n      id: id\n")
			writeDomainStruct(t, root, "beta", "Thing", "\tid   string\n\tname string\n")

			violations, err := collectOutboxPayloadParityViolations(root)
			require.NoError(t, err)
			assert.Equal(t, []string{
				"internal/usecase/beta/event/payload_parity.yaml: opened: 集約 beta.Thing のフィールド name が未分類",
			}, violations)
		})

		t.Run("notification は集約の全フィールドを持たなくても違反にしない", func(t *testing.T) {
			t.Parallel()

			// 起きた事実と識別子だけを運ぶのが notification の正しい姿なので、
			// フィールド並列の対象から外れることを carve-out として固定する。
			root := t.TempDir()
			writeEventPackage(t, root, "delta",
				"package event\n\ntype closed struct {\n\tID string `json:\"id\"`\n}\n\nfunc BuildClosed(x int) ([]byte, error) { return nil, nil }\n")
			writeParityYAML(t, root, "delta", "payloads:\n  closed:\n    kind: notification\n    of: delta.Thing\n")
			writeDomainStruct(t, root, "delta", "Thing", "\tid   string\n\tname string\n")

			violations, err := collectOutboxPayloadParityViolations(root)
			require.NoError(t, err)
			assert.Empty(t, violations)
		})

		t.Run("宣言が運ぶと言う JSON 名が payload に無いことを検出する", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeEventPackage(t, root, "gamma",
				"package event\n\ntype opened struct {\n\tID string `json:\"id\"`\n}\n\nfunc BuildOpened(x int) ([]byte, error) { return nil, nil }\n")
			writeParityYAML(t, root, "gamma",
				"payloads:\n  opened:\n    kind: snapshot\n    of: gamma.Thing\n"+
					"    fields:\n      id: id\n      name: fullName\n")
			writeDomainStruct(t, root, "gamma", "Thing", "\tid   string\n\tname string\n")

			violations, err := collectOutboxPayloadParityViolations(root)
			require.NoError(t, err)
			assert.Equal(t, []string{
				"internal/usecase/gamma/event/payload_parity.yaml: opened: 運ぶと宣言した fullName が payload の JSON タグに無い",
			}, violations)
		})
	})
}

// writeEventPackage は、陽性対照用に internal/usecase/<agg>/event/<agg>_event.go を書きます。
func writeEventPackage(t *testing.T, root, agg, src string) {
	t.Helper()

	dir := filepath.Join(root, usecaseRoot, agg, "event")
	require.NoError(t, pkgfs.OS{}.MkdirAll(dir, 0o750))
	require.NoError(t, pkgfs.OS{}.WriteFile(filepath.Join(dir, agg+"_event.go"), []byte(src), 0o600))
}

// writeParityYAML は、陽性対照用に対応宣言を書きます。
func writeParityYAML(t *testing.T, root, agg, src string) {
	t.Helper()

	path := filepath.Join(root, usecaseRoot, agg, "event", parityFileName)
	require.NoError(t, pkgfs.OS{}.WriteFile(path, []byte(src), 0o600))
}

// writeDomainStruct は、陽性対照用に internal/domain/<pkg>/<pkg>.go へ struct を書きます。
func writeDomainStruct(t *testing.T, root, pkg, name, fields string) {
	t.Helper()

	dir := filepath.Join(root, "internal", "domain", pkg)
	require.NoError(t, pkgfs.OS{}.MkdirAll(dir, 0o750))
	src := "package " + pkg + "\n\ntype " + name + " struct {\n" + fields + "}\n"
	require.NoError(t, pkgfs.OS{}.WriteFile(filepath.Join(dir, pkg+".go"), []byte(src), 0o600))
}

// collectOutboxPayloadParityViolations は、root 配下の event パッケージを走査して違反を集めます。
// 戻り値はソート済みで、同じツリーからは常に同じ並びになります。
func collectOutboxPayloadParityViolations(root string) ([]string, error) {
	dirs, err := collectEventPackages(root)
	if err != nil {
		return nil, err
	}

	violations := []string{}
	for _, dir := range dirs {
		found, verr := verifyEventPackage(root, dir)
		if verr != nil {
			return nil, verr
		}
		violations = append(violations, found...)
	}
	sort.Strings(violations)

	return violations, nil
}

// collectEventPackages は、`func Build*` を持つ event パッケージのディレクトリを返します。
func collectEventPackages(root string) ([]string, error) {
	base := filepath.Join(root, usecaseRoot)
	dirs := map[string]struct{}{}

	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if isNotExist(walkErr) {
				return filepath.SkipAll
			}

			return walkErr
		}
		if d.IsDir() || !isPlainGoFile(path) || filepath.Base(filepath.Dir(path)) != "event" {
			return nil
		}

		names, rerr := collectBuildPayloadNames(path)
		if rerr != nil {
			return rerr
		}
		if len(names) > 0 {
			dirs[filepath.Dir(path)] = struct{}{}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(dirs))
	for d := range dirs {
		out = append(out, d)
	}
	sort.Strings(out)

	return out, nil
}

// isPlainGoFile は、生成物でもテストでもない Go ファイルかどうかを返します。
func isPlainGoFile(path string) bool {
	return strings.HasSuffix(path, ".go") &&
		!strings.HasSuffix(path, "_test.go") &&
		!strings.HasSuffix(path, ".gen.go")
}

// isNotExist は、対象が存在しないだけのエラーかどうかを返します。
func isNotExist(err error) bool {
	return xerrors.Is(err, fs.ErrNotExist)
}

// collectBuildPayloadNames は、ファイルを読んで payloadNamesFromLines へ渡します。
func collectBuildPayloadNames(path string) ([]string, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}

	return payloadNamesFromLines(lines), nil
}

// payloadNamesFromLines は、`func Build<Name>(` から payload 型名を導きます。
// 同じファイルに <Name> の非公開形が在ればそちらを、無ければ <Name> をそのまま採ります。
func payloadNamesFromLines(lines []string) []string {
	declared := map[string]struct{}{}
	for _, line := range lines {
		if m := structOpenRe.FindStringSubmatch(line); m != nil {
			declared[m[1]] = struct{}{}
		}
	}

	names := []string{}
	for _, line := range lines {
		m := buildFuncRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lower := lowerFirst(m[1])
		if _, ok := declared[lower]; ok {
			names = append(names, lower)

			continue
		}
		names = append(names, m[1])
	}

	return names
}

// lowerFirst は、先頭 1 文字を小文字にします（ASCII の識別子のみを想定します）。
func lowerFirst(s string) string {
	if s == "" {
		return s
	}

	return strings.ToLower(s[:1]) + s[1:]
}

// readLines は、ファイルを行へ分割して返します。
func readLines(path string) ([]string, error) {
	src, err := pkgfs.OS{}.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return strings.Split(string(src), "\n"), nil
}

// verifyEventPackage は、1 つの event パッケージについて宣言と実物を突き合わせます。
func verifyEventPackage(root, dir string) ([]string, error) {
	rel := relPath(root, dir)

	payloads, err := collectDirPayloadNames(dir)
	if err != nil {
		return nil, err
	}

	doc, ok, err := readParityDoc(dir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []string{fmt.Sprintf("%s: %s が無い（payload: %s）", rel, parityFileName, strings.Join(payloads, ", "))}, nil
	}

	violations := diffPayloadNames(rel, payloads, doc)
	for _, name := range payloads {
		entry, declared := doc.Payloads[name]
		if !declared {
			continue
		}
		found, eerr := verifyPayloadEntry(root, dir, name, entry)
		if eerr != nil {
			return nil, eerr
		}
		violations = append(violations, found...)
	}

	return violations, nil
}

// collectDirPayloadNames は、ディレクトリ内の全 Go ファイルから payload 型名を集めます。
func collectDirPayloadNames(dir string) ([]string, error) {
	paths, err := plainGoFiles(dir)
	if err != nil {
		return nil, err
	}

	names := []string{}
	for _, path := range paths {
		found, rerr := collectBuildPayloadNames(path)
		if rerr != nil {
			return nil, rerr
		}
		names = append(names, found...)
	}
	sort.Strings(names)

	return names, nil
}

// plainGoFiles は、ディレクトリ直下の生成物でもテストでもない Go ファイルをソートして返します。
func plainGoFiles(dir string) ([]string, error) {
	matches, err := pkgfs.OS{}.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}

	paths := []string{}
	for _, m := range matches {
		if isPlainGoFile(m) {
			paths = append(paths, m)
		}
	}
	sort.Strings(paths)

	return paths, nil
}

// readParityDoc は、ディレクトリの対応宣言を読みます。宣言が無い場合は ok=false を返します。
func readParityDoc(dir string) (payloadParityDoc, bool, error) {
	src, err := pkgfs.OS{}.ReadFile(filepath.Join(dir, parityFileName))
	if err != nil {
		if isNotExist(err) {
			return payloadParityDoc{}, false, nil
		}

		return payloadParityDoc{}, false, err
	}

	var doc payloadParityDoc
	if uerr := yaml.Unmarshal(src, &doc); uerr != nil {
		return payloadParityDoc{}, false, uerr
	}

	return doc, true, nil
}

// diffPayloadNames は、実在する payload と宣言されている payload の双方向の差を違反にします。
func diffPayloadNames(rel string, payloads []string, doc payloadParityDoc) []string {
	declared := map[string]struct{}{}
	for name := range doc.Payloads {
		declared[name] = struct{}{}
	}

	violations := []string{}
	for _, name := range payloads {
		if _, ok := declared[name]; !ok {
			violations = append(violations, fmt.Sprintf("%s/%s: payload %s が宣言されていない", rel, parityFileName, name))
		}
		delete(declared, name)
	}
	for name := range declared {
		violations = append(violations, fmt.Sprintf("%s/%s: 宣言 %s に対応する Build 関数が無い", rel, parityFileName, name))
	}

	return violations
}

// verifyPayloadEntry は、宣言 1 件を実物へ突き合わせます。
func verifyPayloadEntry(root, dir, name string, entry payloadParityEntry) ([]string, error) {
	rel := relPath(root, dir) + "/" + parityFileName

	fields, ok, err := lookupAggregateFields(root, entry.Of)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []string{fmt.Sprintf("%s: %s: 写し元 %s が見つからない", rel, name, entry.Of)}, nil
	}

	switch entry.Kind {
	case "notification":
		return nil, nil
	case "snapshot":
		tags, terr := collectPayloadJSONTags(dir, name)
		if terr != nil {
			return nil, terr
		}

		return verifySnapshotFields(rel, name, entry, fields, tags), nil
	default:
		msg := fmt.Sprintf("%s: %s: kind は snapshot か notification のどちらかにすること（実際: %q）", rel, name, entry.Kind)

		return []string{msg}, nil
	}
}

// verifySnapshotFields は、snapshot の分類が集約の全フィールドを覆い、
// 運ぶと宣言した JSON 名が payload に実在することを確かめます。
func verifySnapshotFields(rel, name string, entry payloadParityEntry, fields, tags []string) []string {
	rules := entry.Fields
	violations := []string{}

	tagSet := map[string]struct{}{}
	for _, tag := range tags {
		tagSet[tag] = struct{}{}
	}

	remaining := map[string]struct{}{}
	for field := range rules {
		remaining[field] = struct{}{}
	}

	for _, field := range fields {
		rule, declared := rules[field]
		if !declared {
			violations = append(violations, fmt.Sprintf("%s: %s: 集約 %s のフィールド %s が未分類", rel, name, entry.Of, field))

			continue
		}
		delete(remaining, field)
		violations = append(violations, verifyFieldRule(rel, name, field, rule, tagSet)...)
	}

	for field := range remaining {
		violations = append(violations, fmt.Sprintf("%s: %s: 宣言 %s は集約 %s に無い", rel, name, field, entry.Of))
	}

	return violations
}

// verifyFieldRule は、1 フィールドの宣言が実物と噛み合っているかを確かめます。
func verifyFieldRule(rel, name, field string, rule payloadFieldRule, tagSet map[string]struct{}) []string {
	if rule.Carried == "" && strings.TrimSpace(rule.Omit) == "" {
		return []string{fmt.Sprintf("%s: %s: %s は運ぶ JSON 名か omit の理由のどちらかを書くこと", rel, name, field)}
	}
	if rule.Carried == "" {
		return nil
	}
	if _, ok := tagSet[rule.Carried]; !ok {
		return []string{fmt.Sprintf("%s: %s: 運ぶと宣言した %s が payload の JSON タグに無い", rel, name, rule.Carried)}
	}

	return nil
}

// lookupAggregateFields は、`<pkg>.<Struct>` 形の写し元から internal/domain 配下の struct を探し、
// gofmt 済み前提でトップレベルのフィールド名を宣言順に返します。
func lookupAggregateFields(root, of string) ([]string, bool, error) {
	pkg, name, ok := strings.Cut(of, ".")
	if !ok {
		return nil, false, nil
	}

	paths, err := plainGoFiles(filepath.Join(root, "internal", "domain", pkg))
	if err != nil {
		return nil, false, err
	}

	for _, path := range paths {
		lines, rerr := readLines(path)
		if rerr != nil {
			return nil, false, rerr
		}
		if fields, found := collectStructFields(lines, name); found {
			return fields, true, nil
		}
	}

	return nil, false, nil
}

// collectStructFields は、`type <name> struct {` から `}` までのフィールド名を返します。
// 埋め込みや複数名宣言のように解析できない行は、黙って読み飛ばさず擬似フィールド名として返し、
// 呼び出し側で未分類として loud に落とします。
func collectStructFields(lines []string, name string) ([]string, bool) {
	open := "type " + name + " struct {"
	fields := []string{}
	inside := false

	for _, line := range lines {
		if !inside {
			inside = line == open

			continue
		}
		if line == "}" {
			return fields, true
		}
		if trimmed := strings.TrimSpace(line); trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if m := structFieldRe.FindStringSubmatch(line); m != nil {
			fields = append(fields, m[1])

			continue
		}
		fields = append(fields, "<解析できない行: "+strings.TrimSpace(line)+">")
	}

	return nil, false
}

// collectPayloadJSONTags は、payload struct が持つ JSON 名を返します。
func collectPayloadJSONTags(dir, name string) ([]string, error) {
	paths, err := plainGoFiles(dir)
	if err != nil {
		return nil, err
	}

	for _, path := range paths {
		lines, rerr := readLines(path)
		if rerr != nil {
			return nil, rerr
		}
		if tags, found := extractJSONTags(lines, name); found {
			return tags, nil
		}
	}

	return nil, nil
}

// extractJSONTags は、指定 struct のブロック内に現れる JSON 名を集めます。
func extractJSONTags(lines []string, name string) ([]string, bool) {
	open := "type " + name + " struct {"
	tags := []string{}
	inside := false

	for _, line := range lines {
		if !inside {
			inside = line == open

			continue
		}
		if line == "}" {
			return tags, true
		}
		if m := jsonTagRe.FindStringSubmatch(line); m != nil {
			tags = append(tags, m[1])
		}
	}

	return nil, false
}

// relPath は、root からの相対パスをスラッシュ区切りで返します。
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}

	return filepath.ToSlash(rel)
}

func Test_payloadNamesFromLines(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("同じファイルに非公開の型が在ればそちらを payload 名にする", func(t *testing.T) {
			t.Parallel()

			lines := []string{
				"type created struct {",
				"}",
				"func BuildCreated(p *purchase.Purchase) ([]byte, error) {",
			}

			assert.Equal(t, []string{"created"}, payloadNamesFromLines(lines))
		})

		t.Run("非公開の型が無ければ公開名をそのまま採る", func(t *testing.T) {
			t.Parallel()

			lines := []string{
				"type Withdrawn struct {",
				"}",
				"func BuildWithdrawn(u *user.User) ([]byte, error) {",
			}

			assert.Equal(t, []string{"Withdrawn"}, payloadNamesFromLines(lines))
		})

		t.Run("Build で始まらない関数は拾わない", func(t *testing.T) {
			t.Parallel()

			lines := []string{"func WireType(t purchase.EventType) (string, error) {"}

			assert.Empty(t, payloadNamesFromLines(lines))
		})
	})
}

func Test_collectStructFields(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("宣言順にフィールド名を返す", func(t *testing.T) {
			t.Parallel()

			lines := []string{
				"type Thing struct {",
				"\tid   uuid.UUID",
				"\tname string",
				"}",
			}

			fields, found := collectStructFields(lines, "Thing")

			assert.True(t, found)
			assert.Equal(t, []string{"id", "name"}, fields)
		})

		t.Run("空行とコメント行は読み飛ばす", func(t *testing.T) {
			t.Parallel()

			lines := []string{
				"type Thing struct {",
				"\t// id は識別子です。",
				"",
				"\tid uuid.UUID",
				"}",
			}

			fields, found := collectStructFields(lines, "Thing")

			assert.True(t, found)
			assert.Equal(t, []string{"id"}, fields)
		})

		t.Run("解析できない行は擬似フィールドとして返し、未分類として落とさせる", func(t *testing.T) {
			t.Parallel()

			// 埋め込みや複数名宣言を黙って読み飛ばすと、走査が縮んだことに気づけなくなる。
			lines := []string{
				"type Thing struct {",
				"\tembedded.Base",
				"\ta, b int",
				"}",
			}

			fields, found := collectStructFields(lines, "Thing")

			assert.True(t, found)
			assert.Equal(t, []string{
				"<解析できない行: embedded.Base>",
				"<解析できない行: a, b int>",
			}, fields)
		})

		t.Run("目当ての struct が無ければ見つからないと返す", func(t *testing.T) {
			t.Parallel()

			_, found := collectStructFields([]string{"type Other struct {", "}"}, "Thing")

			assert.False(t, found)
		})
	})
}

func Test_extractJSONTags(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("オプション付きのタグは名前だけを採る", func(t *testing.T) {
			t.Parallel()

			lines := []string{
				"type created struct {",
				"\tPurchaseID string `json:\"purchaseId\"`",
				"\tCouponID   *string `json:\"couponId,omitempty\"`",
				"}",
			}

			tags, found := extractJSONTags(lines, "created")

			assert.True(t, found)
			assert.Equal(t, []string{"purchaseId", "couponId"}, tags)
		})

		t.Run("目当ての struct が無ければ見つからないと返す", func(t *testing.T) {
			t.Parallel()

			_, found := extractJSONTags([]string{"type other struct {", "}"}, "created")

			assert.False(t, found)
		})
	})
}

func Test_verifyFieldRule(t *testing.T) {
	t.Parallel()

	tags := map[string]struct{}{"orderedAt": {}}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("運ぶ宣言の JSON 名が payload に在れば違反にしない", func(t *testing.T) {
			t.Parallel()

			rule := payloadFieldRule{Carried: "orderedAt"}

			assert.Empty(t, verifyFieldRule("f", "created", "orderedAt", rule, tags))
		})

		t.Run("理由のある omit は違反にしない", func(t *testing.T) {
			t.Parallel()

			rule := payloadFieldRule{Omit: "作成時点では常に未設定"}

			assert.Empty(t, verifyFieldRule("f", "created", "paidAt", rule, tags))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("運ぶと言いながら payload に JSON 名が無ければ違反にする", func(t *testing.T) {
			t.Parallel()

			rule := payloadFieldRule{Carried: "missing"}

			assert.Equal(t, []string{
				"f: created: 運ぶと宣言した missing が payload の JSON タグに無い",
			}, verifyFieldRule("f", "created", "code", rule, tags))
		})

		t.Run("理由の無い omit は違反にする", func(t *testing.T) {
			t.Parallel()

			rule := payloadFieldRule{Omit: "   "}

			assert.Equal(t, []string{
				"f: created: paidAt は運ぶ JSON 名か omit の理由のどちらかを書くこと",
			}, verifyFieldRule("f", "created", "paidAt", rule, tags))
		})
	})
}
