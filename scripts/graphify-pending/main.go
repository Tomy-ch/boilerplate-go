// Package main は、graphify の意味論抽出がどれだけ溜まっているかを測るツール。
//
//	-dir:       graphify の出力ディレクトリ（既定 graphify-out）
//	-ignore:    解析対象から外すパスの宣言（既定 .graphifyignore）
//	-threshold: この行数以上たまっていたら未処理ありとして報告する（既定 0 = 報告のみ）
//	-github:    GitHub Actions の job summary へ書き出す
//
// graphify のグラフは 2 つの半分でできている。コードを読む決定的な半分は tree-sitter だけで
// 完結するので無条件に回せるが、ドキュメントを読む意味論の半分はモデルを要するので、変更の
// たびに回すわけにいかない。そこで「どれだけ溜まったか」を測り、まとめて回す判断の材料にする。
//
// 溜まり具合をファイル数で測らないのは、1 行の誤字修正と ADR の全面改稿が同じ 1 件になって
// しまうためである。代わりに変更行数を累積する。基準点は
// `graphify-out/cache/semantic/` の最終変更コミット — 意味論抽出が走ったときにだけ書き込まれ、
// かつ追跡下にあるので、新しい状態を持ち込まずに「最後に回した地点」を指せる。決定的な半分が
// 何度コミットしても、この基準は動かない。
//
// 対象から外すパスは `.graphifyignore` を読む。graphify 自身がそれで解析対象を決めているので、
// ここで別の除外リストを持つと、生成物の churn を数えたりドキュメントを取りこぼしたりする形で
// 静かにずれる。
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // 変更検出のみ。graphify の manifest がこの形式で持つ // DevSkim: ignore DS126858
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go-boilerplate/pkg/xerrors"
)

const (
	// defaultDir は graphify の既定の出力ディレクトリ。
	defaultDir = "graphify-out"
	// defaultIgnore は graphify の解析対象除外の宣言。
	defaultIgnore = ".graphifyignore"
	// semanticCache は出力ディレクトリからの、セマンティックキャッシュの相対位置。
	semanticCache = "cache/semantic"
	// maxListedFiles は内訳の列挙上限。
	maxListedFiles = 15
	// numstatFields は `git diff --numstat` の 1 行の欄数（追加 / 削除 / パス）。
	numstatFields = 3
	// shortCommitLength は報告に使うコミットハッシュの桁数。
	shortCommitLength = 8
	// summaryFileMode は job summary へ追記するときのパーミッション。
	summaryFileMode = 0o600
)

// docExtensions は graphify が意味論抽出の対象とする拡張子（graphify.detect の DOC_EXTENSIONS）。
var docExtensions = []string{".md", ".mdx", ".qmd", ".skill", ".txt", ".rst", ".html", ".yaml", ".yml"}

var (
	// errManifestUnreadable は、manifest を読めないことを表す。読めないまま 0 件と報告すると、
	// 「溜まっていない」と「測れなかった」が見分けられなくなる。
	errManifestUnreadable = xerrors.New("cannot read the graphify manifest")
	// errNoBaseline は、意味論抽出を回した地点を特定できないことを表す。
	errNoBaseline = xerrors.New("cannot locate the last semantic extraction commit")
)

// pending は未処理の 1 ファイル。
type pending struct {
	path    string
	changed int
}

// manifestEntry は manifest.json の 1 行のうち、この用途で読む面だけ。
// キー名は graphify が決めるので、このリポジトリの JSON 命名規約は当てられない。
type manifestEntry struct {
	SemanticHash string `json:"semantic_hash"` //nolint:tagliatelle // graphify が書く外部スキーマ
}

// main は 1:1 テスト規約の対象外で分岐を検査できないため、判断は run に置きます。
func main() {
	log.SetFlags(0)

	if err := run(os.Args[1:], os.ReadFile, gitOutput); err != nil {
		log.Fatalf("%v", err)
	}
}

// run は、未処理のドキュメントとその変更行数を報告します。
// read はファイルの読み出し手段、git は git の実行手段で、差し替えられるよう引数で受けます。
func run(args []string, read func(name string) ([]byte, error), git func(args ...string) (string, error)) error {
	fs := flag.NewFlagSet("graphify-pending", flag.ContinueOnError)
	dir := fs.String("dir", defaultDir, "graphify の出力ディレクトリ")
	ignore := fs.String("ignore", defaultIgnore, "解析対象から外すパスの宣言")
	threshold := fs.Int("threshold", 0, "この行数以上たまっていたら未処理ありとして報告する")
	summary := fs.Bool("github", false, "GitHub Actions の job summary へ書き出す")

	if err := fs.Parse(args); err != nil {
		if xerrors.Is(err, flag.ErrHelp) {
			return nil
		}

		return xerrors.Wrap(err, "failed to parse flags")
	}

	stale, err := staleDocuments(path.Join(*dir, "manifest.json"), *ignore, read)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		log.Print("graphify: 意味論抽出の未処理はありません")

		return report(*summary, 0, 0, *threshold, nil)
	}

	baseline, err := lastSemanticCommit(path.Join(*dir, semanticCache), git)
	if err != nil {
		// 基準が無いのは意味論抽出をまだ一度もコミットしていない状態なので、落とさず件数だけ報告する。
		if xerrors.Is(err, errNoBaseline) {
			log.Printf("graphify: 意味論抽出の未処理 %d ファイル（基準コミットが無いため行数は測れません）", len(stale))
			for _, file := range stale[:min(len(stale), maxListedFiles)] {
				log.Printf("     %s", file)
			}

			return report(*summary, len(stale), 0, *threshold, nil)
		}

		return err
	}

	items, total, err := measure(baseline, stale, git)
	if err != nil {
		return err
	}

	log.Printf("graphify: 意味論抽出の未処理 %d ファイル / %d 行（基準 %s）", len(items), total, short(baseline))
	for _, item := range items[:min(len(items), maxListedFiles)] {
		log.Printf("     %6d  %s", item.changed, item.path)
	}
	if len(items) > maxListedFiles {
		log.Printf("     ... 他 %d ファイル", len(items)-maxListedFiles)
	}

	return report(*summary, len(items), total, *threshold, items)
}

// staleDocuments は、manifest 上で意味論抽出が追いついていないドキュメントを返します。
// 判定は保存された semantic_hash といまのファイル内容の MD5 の比較です。manifest 内の
// ast_hash との突き合わせでは、未処理が永久に 0 件に見えます。
func staleDocuments(manifestPath, ignorePath string, read func(name string) ([]byte, error)) ([]string, error) {
	body, err := read(manifestPath)
	if err != nil {
		return nil, xerrors.Wrap(errManifestUnreadable, manifestPath+": "+err.Error())
	}

	var entries map[string]manifestEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, xerrors.Wrap(errManifestUnreadable, manifestPath+": "+err.Error())
	}

	excluded, err := loadIgnore(ignorePath, read)
	if err != nil {
		return nil, err
	}

	var stale []string
	for file, entry := range entries {
		if !isDocument(file) || excluded(file) {
			continue
		}
		body, err := read(file)
		if err != nil {
			// manifest に載っているのに読めないのは、そのファイルが消えた場合。抽出し直す対象では
			// ないので数えません。
			continue
		}
		// nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-md5
		if entry.SemanticHash != fmt.Sprintf("%x", md5.Sum(body)) { //nolint:gosec // 変更検出のみで、graphify の manifest と同じ選択 // DevSkim: ignore DS126858
			stale = append(stale, file)
		}
	}
	sort.Strings(stale)

	return stale, nil
}

// loadIgnore は .graphifyignore を読んで、除外されるかを判定する述語を返します。
// 宣言が無いことは異常ではない（除外なしの構成）ため、その場合は何も除外しません。
func loadIgnore(ignorePath string, read func(name string) ([]byte, error)) (func(string) bool, error) {
	body, err := read(ignorePath)
	if err != nil {
		if xerrors.Is(err, os.ErrNotExist) {
			return func(string) bool { return false }, nil
		}

		return nil, xerrors.Wrap(err, "❌ "+ignorePath+" を読み込めません")
	}

	var patterns []string
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Wrap(err, "❌ "+ignorePath+" を読めません")
	}

	return func(file string) bool {
		for _, pattern := range patterns {
			if matchesIgnore(file, pattern) {
				return true
			}
		}

		return false
	}, nil
}

// matchesIgnore は、gitignore 風の 1 パターンがリポジトリ相対パスに当たるかを判定します。
// 汎用の gitignore 実装ではなく、`.graphifyignore` が実際に使う形だけを扱います。
//
//	docs/godoc/     ルートからのディレクトリ
//	**/gen/         どの階層にあってもよいディレクトリ
//	*.ja.md         ファイル名のパターン
//	**/*.gen.go     どの階層にあってもよいファイル名のパターン
//	openapi/x.yaml  素のパス
//
// 解釈が graphify 本体と一致する保証は無く、食い違いは閾値が早く鳴る向きにだけ倒れます。
func matchesIgnore(file, pattern string) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	pattern, anywhere := strings.CutPrefix(pattern, "**/")

	if directory, isDirectory := strings.CutSuffix(pattern, "/"); isDirectory {
		if anywhere {
			return slices.Contains(strings.Split(path.Dir(file), "/"), directory)
		}

		return strings.HasPrefix(file, directory+"/")
	}

	if strings.HasPrefix(pattern, "*") {
		return matchesGlob(path.Base(file), pattern)
	}
	if anywhere {
		return path.Base(file) == pattern
	}

	return file == pattern || strings.HasPrefix(file, pattern+"/")
}

// matchesGlob は `*.ja.md` のような、先頭の `*` 一つだけを持つパターンを判定します。
func matchesGlob(name, pattern string) bool {
	suffix, ok := strings.CutPrefix(pattern, "*")
	if !ok {
		return name == pattern
	}

	return strings.HasSuffix(name, suffix)
}

// isDocument は、graphify が意味論抽出の対象とする拡張子かを返します。
func isDocument(file string) bool {
	ext := strings.ToLower(path.Ext(file))

	return slices.Contains(docExtensions, ext)
}

// lastSemanticCommit は、意味論キャッシュを最後に変更したコミットを返します。
func lastSemanticCommit(cachePath string, git func(args ...string) (string, error)) (string, error) {
	out, err := git("log", "-1", "--format=%H", "--", cachePath)
	if err != nil {
		return "", xerrors.Wrap(err, "❌ "+cachePath+" の履歴を辿れません")
	}

	commit := strings.TrimSpace(out)
	if commit == "" {
		return "", xerrors.Wrap(errNoBaseline,
			cachePath+" にコミットがありません（意味論抽出をまだ一度もコミットしていない状態です）")
	}

	return commit, nil
}

// measure は、基準コミットから各ファイルが何行変わったかを数え、変更量の降順で返します。
func measure(baseline string, files []string, git func(args ...string) (string, error)) ([]pending, int, error) {
	args := append([]string{"diff", "--numstat", baseline + "..HEAD", "--"}, files...)
	out, err := git(args...)
	if err != nil {
		return nil, 0, xerrors.Wrap(err, "❌ "+short(baseline)+" からの差分を測れません")
	}

	return parseNumstat(out)
}

// parseNumstat は `git diff --numstat` の出力を積み上げます。バイナリ扱いの行は `-` になるため
// 行数としては 0 と数えますが、ファイル自体は未処理として残します。
func parseNumstat(out string) ([]pending, int, error) {
	var items []pending
	total := 0
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", numstatFields)
		if len(fields) != numstatFields {
			return nil, 0, xerrors.Wrap(errManifestUnreadable, "numstat の行を解釈できません: "+strconv.Quote(line))
		}
		changed := atoiOrZero(fields[0]) + atoiOrZero(fields[1])
		items = append(items, pending{path: fields[2], changed: changed})
		total += changed
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].changed != items[j].changed {
			return items[i].changed > items[j].changed
		}

		return items[i].path < items[j].path
	})

	return items, total, nil
}

// atoiOrZero は numstat の 1 欄を数に直します。バイナリを表す `-` は 0 として扱います。
func atoiOrZero(field string) int {
	value, err := strconv.Atoi(field)
	if err != nil {
		return 0
	}

	return value
}

// report は結果を job summary と、後続のステップが読む出力へ書き出します。閾値は判断の材料で
// あって門ではないので、超過しても終了コードは 0 のままにします。
func report(summary bool, files, total, threshold int, items []pending) error {
	if !summary {
		return nil
	}

	if err := emitCounts(files, total); err != nil {
		return err
	}

	destination := os.Getenv("GITHUB_STEP_SUMMARY")
	if destination == "" {
		return nil
	}

	var body strings.Builder
	body.WriteString("## graphify semantic extraction\n\n")
	if total >= threshold && threshold > 0 {
		fmt.Fprintf(&body,
			"⚠️ %d files / %d changed lines are waiting (threshold %d). Run `/graphify --update`.\n\n",
			files, total, threshold)
	} else {
		fmt.Fprintf(&body, "%d files / %d changed lines are waiting (threshold %d).\n\n", files, total, threshold)
	}
	for _, item := range items[:min(len(items), maxListedFiles)] {
		fmt.Fprintf(&body, "- `%s` (%d)\n", item.path, item.changed)
	}
	if len(items) > maxListedFiles {
		fmt.Fprintf(&body, "- ... %d more\n", len(items)-maxListedFiles)
	}

	file, err := os.OpenFile(destination, os.O_APPEND|os.O_WRONLY|os.O_CREATE, summaryFileMode) //nolint:gosec // 宛先は Actions が渡す固定の環境変数
	if err != nil {
		return xerrors.Wrap(err, "❌ job summary を開けません")
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(body.String()); err != nil {
		return xerrors.Wrap(err, "❌ job summary へ書けません")
	}

	return nil
}

// emitCounts は未処理量を後続のステップが読める形で書き出します。job summary は人が読むための
// もので、同じ数を機械が読む経路にはならない。抽出が何も生まなかったとき、それが「溜まって
// いなかった」のか「動かなかった」のかは、この数と差分の有無を突き合わせて初めて分かれる。
func emitCounts(files, total int) error {
	destination := os.Getenv("GITHUB_OUTPUT")
	if destination == "" {
		return nil
	}

	file, err := os.OpenFile(destination, os.O_APPEND|os.O_WRONLY|os.O_CREATE, summaryFileMode) //nolint:gosec // 宛先は Actions が渡す固定の環境変数
	if err != nil {
		return xerrors.Wrap(err, "❌ ステップ出力を開けません")
	}
	defer func() { _ = file.Close() }()

	if _, err := fmt.Fprintf(file, "files=%d\nlines=%d\n", files, total); err != nil {
		return xerrors.Wrap(err, "❌ ステップ出力へ書けません")
	}

	return nil
}

// short はコミットハッシュを短縮します。
func short(commit string) string {
	if len(commit) > shortCommitLength {
		return commit[:shortCommitLength]
	}

	return commit
}

// gitOutput は git を実行して標準出力を返します。
func gitOutput(args ...string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", args...).Output() //nolint:gosec // 引数は呼び出し側が組み立てた固定語とパス
	if err != nil {
		return "", xerrors.Wrap(err, "git "+strings.Join(args, " "))
	}

	return string(out), nil
}
