// Package main は Dockerfile の `FROM` base image、docker compose の `image:`、そして
// workflow / composite action の `uses: docker://` を不変の digest へ固定するツール。
//
// registry の image を指す参照は、書かれた場所によらずこの機構が持つ。`uses:` の行であっても
// 参照先は GitHub のリポジトリではないため、tag を git ls-remote で commit へ解決する
// pin-actions では扱えない（docs/design/security.md）。
//
//	resolve: docker/*/Dockerfile の FROM、docker-compose*.yaml の image:、.github の
//	         uses: docker:// を走査し、
//	         image:tag を registry の現在 digest へ解決して lockfile (docker/images-pin.toml) へ書き出す。
//	apply:   lockfile を SSOT に各参照を `image:tag@sha256:...` へ固定する（FROM は AS stage を保持）。
//	check:   apply と同じ判定を書き換えなしで行い、未固定/未登録/drift があれば非ゼロ終了する（CI / hook 用）。
//
// tag は版の SSOT として FROM 行に残し、digest を lock 側で管理する（tag 再割り当て攻撃の遮断）。
//
// supply-chain cooldown: --min-age-days 未満（＝公開直後）の digest は採用しない。
// mutable tag は「N 日前に何を指していたか」を registry へ問えないため、step-back 先は
// git tag のような版履歴ではなく自分の前回 lock。前回 lock が無い初回は退行先が無く、
// 出来立ての未検証 digest を掴む危険があるため、resolve は tag のまま残さず非ゼロ終了する
// （緊急ブートストラップのみ --min-age-days=0 で明示採用）。
//
// apply/check は fail-closed：管理対象の FROM は image:tag@sha256:... へ固定され、かつ lock(SSOT)
// に登録済みでなければならない。lock 未登録の image は「未登録」、digest 無し（tag のみ）は
// 「未固定」として非ゼロ終了する（tag のみ運用は一切許容しない）。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go-boilerplate/pkg/xerrors"
	"go-boilerplate/scripts/lib/ghfiles"
	"go-boilerplate/scripts/lib/yamlblock"
)

const (
	lockFile        = "docker/images-pin.toml"
	filePerm        = 0o644
	inspectTimeout  = 60 * time.Second
	hoursPerDay     = 24
	createdTimeCols = 3 // "2006-01-02 15:04:05.000 +0000 UTC" を最初の 3 フィールドで再構成
)

var (
	// FROM [--platform=...] <ref> [AS <stage>]
	// クオートを ref から締め出すのは、含めると `FROM "alpine:3.24"` がクオートごと一致し、
	// `"alpine` を image 名として固定対象に載せてしまうため。締め出せば一致しなくなり、
	// detectLooseRefs が対応記法の外として拾う。
	fromRe      = regexp.MustCompile(`(?m)^(FROM[ \t]+)(?:--platform=\S+[ \t]+)?([^\s'"]+)([ \t]+(?i:AS)[ \t]+\S+)?[ \t]*$`)
	fromLooseRe = regexp.MustCompile(`(?i)^[ \t]*FROM[ \t]+\S`)
	// docker compose service の `image: <ref>`。FROM と同じ prefix/ref/suffix の 3 グループ構成に
	// して rewritePins を共用する。suffix は末尾空白と行末コメントを取り込み保持する。
	// クオートを締め出す理由は fromRe と同じ。
	composeImageRe    = regexp.MustCompile(`(?m)^([ \t]+image:[ \t]+)([^\s'"]+)([ \t]*(?:#.*)?)$`)
	composeImageLoose = regexp.MustCompile(`^[ \t]+image[ \t]*:[ \t]*\S`)
	// `FROM <ref> AS <stage>` の接尾辞から宣言されたステージ名を取り出す。
	fromStageNameRe = regexp.MustCompile(`[ \t]+(?i:AS)[ \t]+(\S+)`)
	// workflow / composite action の `uses: [-] docker://<ref>`。GitHub Actions が registry の
	// image を直接実行するステップの記法で、参照先は GitHub のリポジトリではない。pin-actions は
	// tag を git ls-remote で commit へ解決する機構なので registry には効かず、digest を扱う
	// こちらが持つ（docs/design/security.md）。
	//
	// ref に tag を必須とするのは、省略が :latest を意味するため。tag 無しを通すと parseRef が
	// false を返して固定対象から静かに外れ、可動タグのまま CI で走る。一致しない形は
	// detectLooseRefs が拾って fail-close する。
	usesDockerRe = regexp.MustCompile(
		`(?m)^([ \t]*(?:-[ \t]*)?uses:[ \t]*docker://)((?:[^\s@]+/)?[^\s@/:]+:[^\s@/]+(?:@\S+)?)([ \t]*(?:#.*)?)$`,
	)
	// usesDockerRe の取りこぼしを拾う緩いパターン。引用符付き・flow mapping・tag 無しのいずれも
	// ここへ落ちる。owner/repo 形式の uses: には反応しない（pin-actions の担当）。
	looseDockerUsesRe = regexp.MustCompile(`\buses[ \t]*:[ \t]*["']?docker://`)
	// workflow / composite action の service container の `image: <ref>`。ジョブと同じランナー上で
	// 走り CI の判定に直接効くので、FROM や compose と同じく固定する（docs/design/security.md）。
	//
	// compose の image: と同形だが composeImageRe をそのまま当てられない。workflow の image: は
	// step の with: 配下にも現れ、そちらは ${{ }} で組み立てられて固定のしようがないためである。
	// ref に ${{ }} を含む行は、この正規表現にも下の loose にも一致させない。
	serviceImageRe = regexp.MustCompile(
		`(?m)^([ 	]+image:[ 	]+)((?:[^\s'"$]|\$[^{])+)([ 	]*(?:#.*)?)$`,
	)
	// serviceImageRe の取りこぼしを拾う緩いパターン。${{ }} を含む行だけは固定対象でないので外す。
	serviceImageLoose = regexp.MustCompile(`^[ 	]+image[ 	]*:[ 	]*(?:[^\s$]|\$[^{])`)
	// lockfile 行: "image:tag" = "sha256:..."
	lockRe   = regexp.MustCompile(`^"([^"]+)"\s*=\s*"(sha256:[0-9a-f]+)"`)
	digestRe = regexp.MustCompile(`(?m)^Digest:[ \t]+(sha256:[0-9a-f]+)`)
)

var (
	// errUsage は、サブコマンドが無いか未知の場合のエラー。
	errUsage = xerrors.New("usage: pin-images <resolve|apply|check>")
	// errDigestUnparsable は、inspect 出力から Digest 行を解析できなかった場合のエラー。
	errDigestUnparsable = xerrors.New("Digest 行を解析できません")
	// errCreatedUnparsable は、inspect 出力から image config の created を解析できなかった場合のエラー。
	errCreatedUnparsable = xerrors.New("created を解析できません")
	// errLockInvalidLine は、lockfile に代入として解釈できない行があった場合のエラー。
	errLockInvalidLine = xerrors.New("lockfile に解釈できない行があります")
	// errLockDuplicateKey は、lockfile に同一キーが複数回現れた場合のエラー。
	errLockDuplicateKey = xerrors.New("lockfile にキーの重複があります")
	// errLockMissingImage は、参照されている image が lockfile に登録されていない場合のエラー。
	errLockMissingImage = xerrors.New("lockfile に未登録の base image があります")
	// errPinDrift は、check で未固定・lockfile 不一致の image 参照を検出した場合のエラー。
	errPinDrift = xerrors.New("image 参照が未固定か lockfile と不一致です")
	// errNoStepBack は、退行先の無い出来立て digest しか無く採用を見送った場合のエラー。
	errNoStepBack = xerrors.New("退行先の無い出来立て image は採用できません")
	// errLooseRef は、固定対象として解釈できない image 参照を検出した場合のエラー。
	errLooseRef = xerrors.New("固定対象として解釈できない image 参照があります")
)

// target は走査対象のファイルと、その参照行を捕捉する正規表現（prefix/ref/suffix の 3 グループ）。
// loose は厳格な re が取りこぼした参照行を検出する正規表現。
//
// exemptTagless は tag を持たなくても固定対象外として正当な参照値を返す。tag の省略は :latest を
// 意味するため原則は取りこぼしとして落とすが、Dockerfile だけは registry の image を指さない FROM が
// 正当に存在する——先行する AS で宣言したビルドステージと scratch で、どちらも固定のしようがない。
// 持たない対象では nil。
type target struct {
	path          string
	re            *regexp.Regexp
	loose         *regexp.Regexp
	exemptTagless func(data string) map[string]bool
}

// imageRef は走査対象（Dockerfile FROM / compose image: / workflow uses: docker:// と service image:）が
// 参照する registry image 1 件。key は image:tag。
type imageRef struct {
	image string // 例: golang, nginx, ghcr.io/foo/bar
	tag   string // 例: 1.26.5-alpine
}

func (r imageRef) key() string { return r.image + ":" + r.tag }

// main は 1:1 テスト規約の対象外で分岐を検査できないため、判断は run に置きます。
func main() {
	log.SetFlags(0)

	if err := run(os.Args[1:], os.Getwd); err != nil {
		log.Fatalf("❌ %v", err)
	}
}

// run は、サブコマンドを解釈して固定処理へ振り分けます。
// wd は走査の基点となるディレクトリの取得手段で、差し替えられるよう引数で受けます。
func run(args []string, wd func() (string, error)) error {
	if len(args) == 0 {
		return errUsage
	}

	root, err := wd()
	if err != nil {
		return xerrors.Wrap(err, "getwd")
	}

	targets, err := targetFiles(root)
	if err != nil {
		return err
	}

	switch args[0] {
	case "resolve":
		fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
		minAge := fs.Int("min-age-days", 0, "N 日未満の新しすぎる digest は quarantine（0 で無効）")
		if err := fs.Parse(args[1:]); err != nil {
			// ヘルプ要求は失敗ではないので 0 で終える。usage は flag が既に出力している。
			if xerrors.Is(err, flag.ErrHelp) {
				return nil
			}

			return xerrors.Wrap(err, "failed to parse flags")
		}

		return resolve(root, targets, *minAge)
	case "apply":
		return applyOrCheck(root, targets, false)
	case "check":
		return applyOrCheck(root, targets, true)
	default:
		return errUsage
	}
}

// targetFiles は走査対象を返す。Dockerfile の FROM、compose の image:、workflow / composite action の
// uses: docker:// と services の image: の 4 種で、registry の image を指す参照は書かれた場所に
// よらずこの機構が固定する（docs/design/security.md）。
//
// workflow は 2 つの target として登録する。target が 1 ファイルにつき 1 つの正規表現しか持たない
// ためで、uses: docker:// と service の image: は書式が違う。同じファイルを 2 度走査するが掴む行は
// 重ならない。uses: owner/repo@<sha> は pin-actions の担当で、ファイル集合そのものは ghfiles が
// 両者へ与える。
func targetFiles(root string) ([]target, error) {
	dockerfiles, err := globFiles(root, "docker/*/Dockerfile")
	if err != nil {
		return nil, err
	}
	compose, err := globFiles(root, "docker-compose*.yaml")
	if err != nil {
		return nil, err
	}
	workflows, err := ghfiles.Collect(root)
	if err != nil {
		return nil, err
	}
	var targets []target
	for _, f := range dockerfiles {
		targets = append(targets, target{
			path: f, re: fromRe, loose: fromLooseRe, exemptTagless: dockerfileExemptTagless,
		})
	}
	for _, f := range compose {
		targets = append(targets, target{path: f, re: composeImageRe, loose: composeImageLoose})
	}
	for _, f := range workflows {
		targets = append(targets, target{path: f, re: usesDockerRe, loose: looseDockerUsesRe})
		targets = append(targets, target{path: f, re: serviceImageRe, loose: serviceImageLoose})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].path < targets[j].path })
	return targets, nil
}

// detectLooseRefs は固定対象として解釈できない参照の行番号を返す。
//
// usesDockerRe は行頭にアンカーした 1 行 1 ステップのブロック記法かつ tag 付きにしか一致しない
// が、YAML は同じ内容を flow mapping や引用符でも書けるし、tag を省いた形も書ける。いずれも
// 一致ゼロになり、その状態は「固定漏れ無し」と区別が付かない。緩いパターンで補い、残った行は
// 呼び出し元が fail-close する。
//
// ブロックスカラーの中身は YAML の構造ではなく単なるテキストなので走査から外す。外さないと
// `run:` スクリプトが uses: を含む文字列を出力するだけで検出が誤爆する。
func detectLooseRefs(data string, t target) []int {
	blanked := t.re.ReplaceAllStringFunc(data, func(line string) string {
		return strings.Repeat(" ", len(line))
	})
	// 範囲の判定は潰す前の内容で行う。潰した行は字下げごと空白になり、ブロックの終わりに見える。
	inBlockScalar := yamlblock.ContentLines(data)
	var lines []int
	for i, line := range strings.Split(blanked, "\n") {
		if inBlockScalar[i+1] {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if t.loose.MatchString(line) {
			lines = append(lines, i+1)
		}
	}
	lines = append(lines, taglessLines(data, t)...)
	sort.Ints(lines)

	return lines
}

// validateLoose は全対象を走査し、解釈できない参照行があれば fail-close する。resolve と
// applyOrCheck の両方から呼ぶ——lockfile へ載らない参照を作らないため、走査の前に検査する。
func validateLoose(root string, targets []target) error {
	var found []string
	for _, t := range targets {
		data, err := os.ReadFile(t.path)
		if err != nil {
			return xerrors.Wrap(err, "read "+rel(root, t.path))
		}
		for _, line := range detectLooseRefs(string(data), t) {
			found = append(found, fmt.Sprintf("%s:%d", rel(root, t.path), line))
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)

	return xerrors.Wrap(errLooseRef,
		strings.Join(found, ", ")+"（引用符を外し、1 行 1 参照・tag 明示の形へ直してください）")
}

// dockerfileExemptTagless は registry の image を指さない FROM の値を返す。ビルドステージ参照は
// Docker が大文字小文字を区別しないため、比較は小文字へ揃える。
func dockerfileExemptTagless(data string) map[string]bool {
	exempt := map[string]bool{"scratch": true}
	for _, m := range fromRe.FindAllStringSubmatch(data, -1) {
		if stage := fromStageNameRe.FindStringSubmatch(m[3]); stage != nil {
			exempt[strings.ToLower(stage[1])] = true
		}
	}

	return exempt
}

// taglessLines は厳格なパターンには一致したが tag を持たない参照の行番号を返す。
//
// この形は detectLooseRefs の走査では拾えない——厳格なパターンに一致した時点で空白へ潰され、
// 残らないためである。一方 parseRef は tag が無ければ false を返すので固定対象にも載らず、
// どの報告にも現れないまま :latest が実行される。固定の網から参照が黙って消える向きの穴。
func taglessLines(data string, t target) []int {
	var exempt map[string]bool
	if t.exemptTagless != nil {
		exempt = t.exemptTagless(data)
	}
	var lines []int
	for _, loc := range t.re.FindAllStringSubmatchIndex(data, -1) {
		ref := data[loc[4]:loc[5]]
		if _, ok := parseRef(ref); ok || exempt[strings.ToLower(ref)] {
			continue
		}
		lines = append(lines, strings.Count(data[:loc[0]], "\n")+1)
	}

	return lines
}

// globFiles は root からの相対パターン pat に一致するパスを返す。
// パターンが不正なら 0 件へ縮退させずエラーを返す。走査対象が静かに空になると、そこに書かれた
// image 参照が検疫・固定・drift 検査のいずれからも外れたまま「全て固定済み」と report される。
func globFiles(root, pat string) ([]string, error) {
	m, err := filepath.Glob(filepath.Join(root, pat))
	if err != nil {
		return nil, xerrors.Wrap(err, "glob "+pat)
	}
	return m, nil
}

// parseRef は参照 ref を image:tag へ分解する。第2戻り値が false なら対象外
// （tag 無し＝ビルドステージ参照 / scratch、あるいは registry port を tag と誤認する形）。
func parseRef(ref string) (imageRef, bool) {
	name, _, _ := strings.Cut(ref, "@") // 既存の @digest を捨てる
	image, tag, ok := lastColonSplit(name)
	if !ok || tag == "" || strings.Contains(tag, "/") {
		return imageRef{}, false // tag が無い / 最後の ':' が registry:port だった
	}
	return imageRef{image: image, tag: tag}, true
}

// lastColonSplit は最後の ':' で分割する（tag に ':' は含まれない前提）。
func lastColonSplit(s string) (string, string, bool) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

func resolve(root string, targets []target, minAgeDays int) error {
	if err := validateLoose(root, targets); err != nil {
		return err
	}
	keys, err := collectKeys(targets)
	if err != nil {
		return err
	}
	// lockfile 不在（初回）は空マップで続行するが、それ以外の読み込み失敗は握り潰さず fail-close する
	// （既存ピンが lock から脱落して quarantine の退行先が消え、出来立ての未検証 digest を掴むか
	// errNoStepBack で落ちるかの二択になるのを防ぐ。applyOrCheck と対称）。
	existing, err := readLock(filepath.Join(root, lockFile))
	if !isIgnorableLockErr(err) {
		return xerrors.Wrap(err, "read lockfile")
	}

	ctx := context.Background()
	lock := map[string]string{}
	var notes, skipped []string
	for k, r := range keys {
		digest, err := resolveDigest(ctx, r.key())
		if err != nil {
			return xerrors.Wrap(err, "resolve "+k)
		}
		use, note, err := quarantine(ctx, r, k, digest, minAgeDays, existing)
		if err != nil {
			return xerrors.Wrap(err, "age "+k)
		}
		if note != "" {
			notes = append(notes, note)
		}
		if use == "" {
			skipped = append(skipped, k)
			continue
		}
		lock[k] = use
		log.Printf("  %s -> %s", k, use)
	}
	sort.Strings(notes)
	for _, n := range notes {
		log.Printf("  ⚠️ %s", n)
	}

	if err := writeLock(filepath.Join(root, lockFile), lock); err != nil {
		return xerrors.Wrap(err, "write lockfile")
	}
	log.Printf("✅ %s に %d 件を書き出しました", lockFile, len(lock))

	// 退行先の無い出来立て image（fresh + 既存ピン無し）は tag のまま残さず失敗させる。
	// tag のみ運用を許すと未検証 digest を掴むため、check も同状態を弾く（fail-closed）。
	if len(skipped) > 0 {
		sort.Strings(skipped)

		return xerrors.Wrap(errNoStepBack, fmt.Sprintf(
			"%d 日未満・既存ピン無し。aged 後に再実行するか、緊急時のみ --min-age-days=0 で明示採用してください: %s",
			minAgeDays, strings.Join(skipped, ", "),
		))
	}

	return nil
}

func collectKeys(targets []target) (map[string]imageRef, error) {
	keys := map[string]imageRef{}
	for _, t := range targets {
		data, err := os.ReadFile(t.path)
		if err != nil {
			return nil, xerrors.Wrap(err, "read "+t.path)
		}
		for _, m := range t.re.FindAllStringSubmatch(string(data), -1) {
			if r, ok := parseRef(m[2]); ok {
				keys[r.key()] = r
			}
		}
	}
	return keys, nil
}

// quarantine は minAgeDays 未満の新しすぎる digest を採用しない。第1戻り値 "" は skip。
// 経過日数の取得に失敗した場合は err で呼び出し元へ伝播する。
func quarantine(ctx context.Context, r imageRef, key, candidate string, minAgeDays int, existing map[string]string) (string, string, error) {
	if minAgeDays <= 0 {
		return candidate, "", nil
	}
	age, err := digestAgeDays(ctx, r.key())
	if err != nil {
		return "", "", err
	}
	if age >= minAgeDays {
		return candidate, "", nil
	}
	if prev, ok := existing[key]; ok {
		return prev, fmt.Sprintf("%s: 現 digest が %d 日 (<%d) のため既存ピンを維持", key, age, minAgeDays), nil
	}
	return "", fmt.Sprintf("%s: 現 digest が %d 日 (<%d)・既存ピン無しのため tag のまま skip", key, age, minAgeDays), nil
}

// resolveDigest は image:tag の現在の index digest を registry から取得する。
func resolveDigest(ctx context.Context, ref string) (string, error) {
	out, err := inspect(ctx, ref)
	if err != nil {
		return "", err
	}
	m := digestRe.FindStringSubmatch(out)
	if m == nil {
		return "", xerrors.Wrap(errDigestUnparsable, ref)
	}
	return m[1], nil
}

// digestAgeDays は image:tag が指す digest の image config created から経過日数を返す。
// 公式イメージのビルド日は publish 日にほぼ一致する。マルチアーキの場合は最も新しい created を採る
// （向きの理由は docs/design/security.md の Build inputs 節を参照）。
func digestAgeDays(ctx context.Context, ref string) (int, error) {
	created, err := latestCreated(ctx, ref)
	if err != nil {
		return 0, err
	}
	return int(time.Since(created).Hours() / hoursPerDay), nil
}

func latestCreated(ctx context.Context, ref string) (time.Time, error) {
	// マルチアーキ (index) は .Image が map、単一アーキは struct。前者→後者でフォールバックする。
	// inspect 自体のエラー（429 等）は握り潰さず伝播し、テンプレート不一致のときだけ次を試す。
	var lastErr error
	for _, tmpl := range []string{
		`{{ range .Image }}{{ println .Created }}{{ end }}`,
		`{{ println .Image.Created }}`,
	} {
		out, err := inspect(ctx, ref, "--format", tmpl)
		if err != nil {
			lastErr = err
			continue
		}
		if t, ok := maxCreated(out); ok {
			return t, nil
		}
	}
	if lastErr != nil {
		return time.Time{}, lastErr
	}
	return time.Time{}, xerrors.Wrap(errCreatedUnparsable, ref)
}

// maxCreated は inspect 出力の各行（"2006-01-02 15:04:05.9 +0000 UTC"）から最新の時刻を返す。
func maxCreated(out string) (time.Time, bool) {
	var latest time.Time
	found := false
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < createdTimeCols {
			continue
		}
		// "date time zone" の 3 フィールドだけを RFC 化して parse（末尾 "UTC" 等は捨てる）。
		t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700",
			strings.Join(fields[:createdTimeCols], " "))
		if err != nil {
			continue
		}
		if !found || t.After(latest) {
			latest, found = t, true
		}
	}
	return latest, found
}

func inspect(ctx context.Context, ref string, extra ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	args := append([]string{"buildx", "imagetools", "inspect", ref}, extra...)
	cmd := exec.CommandContext(cctx, "docker", args...) //nolint:gosec // ref は Dockerfile 由来
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", xerrors.Wrap(err, "docker buildx imagetools inspect "+ref+": "+strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// rewritePins は lock を元に image 参照を digest 固定した内容と、lock 未登録の image キー一覧を返す。
// lock に無い image は「未登録」として報告し、行は書き換えない（digest は剥がさない）。
func rewritePins(data string, re *regexp.Regexp, lock map[string]string) (string, []string) {
	var missing []string
	out := re.ReplaceAllStringFunc(data, func(line string) string {
		m := re.FindStringSubmatch(line)
		r, ok := parseRef(m[2])
		if !ok {
			return line
		}
		digest, found := lock[r.key()]
		if !found {
			missing = append(missing, r.key())
			return line
		}
		return m[1] + r.key() + "@" + digest + m[3]
	})
	return out, missing
}

// applyOrCheck は lockfile を SSOT に image 参照を digest 固定する。dryRun=true は書き換えず
// 未固定/未登録/drift を非ゼロ終了で報告する。tag のみへ戻す正規化はしない（fail-closed）。
//
// 全ファイルを読み切って未登録の有無を確定させてから書き込む。1 ファイルずつ書きながら進むと、
// 後続ファイルの未登録参照で中断したときに「exit 1 なのに作業ツリーは書き換え済み」という
// 中途半端な状態が残る。
func applyOrCheck(root string, targets []target, dryRun bool) error {
	if err := validateLoose(root, targets); err != nil {
		return err
	}
	lock, err := readLock(filepath.Join(root, lockFile))
	if err != nil {
		return xerrors.Wrap(err, "read lockfile（先に make pin-images-resolve を実行してください）")
	}
	var missing, drifted, pending []string
	rewritten := map[string]string{}
	for _, t := range targets {
		data, err := os.ReadFile(t.path)
		if err != nil {
			return xerrors.Wrap(err, "read "+rel(root, t.path))
		}
		out, miss := rewritePins(string(data), t.re, lock)
		missing = append(missing, miss...)
		if out == string(data) {
			continue
		}
		if dryRun {
			drifted = append(drifted, rel(root, t.path))
			continue
		}
		pending = append(pending, t.path)
		rewritten[t.path] = out
	}

	if err := validateMissing(missing); err != nil {
		return err
	}

	for _, path := range pending {
		if err := os.WriteFile(path, []byte(rewritten[path]), filePerm); err != nil {
			return xerrors.Wrap(err, "write "+rel(root, path))
		}
		log.Printf("  updated %s", rel(root, path))
	}

	return report(drifted, dryRun, len(pending))
}

// validateMissing は lockfile 未登録の image があればエラーを返す。書き込みより前に呼ぶことで、
// 未登録を検出した実行が作業ツリーを一切変更しないことを保証する。
func validateMissing(missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)

	return xerrors.Wrap(errLockMissingImage,
		strings.Join(uniq(missing), ", ")+"（make pin-images-resolve を実行してください）")
}

// report は固定結果を報告する。dryRun では drift を検出した時点でエラーを返す。
func report(drifted []string, dryRun bool, changed int) error {
	if !dryRun {
		log.Printf("✅ %d ファイルを固定しました", changed)

		return nil
	}
	if len(drifted) > 0 {
		sort.Strings(drifted)

		return xerrors.Wrap(errPinDrift,
			"make pin-images-resolve && make pin-images-apply してコミットしてください: "+
				strings.Join(drifted, ", "))
	}
	log.Printf("✅ 全 base image が lockfile 通りに固定されています")

	return nil
}

func writeLock(path string, lock map[string]string) error {
	keys := make([]string, 0, len(lock))
	for k := range lock {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Docker base image / docker compose image の pin 対象 digest（SSOT）。\n")
	b.WriteString("# make pin-images-resolve で解決・make pin-images-apply で Dockerfile / docker-compose へ反映する。\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%q = %q\n", k, lock[k])
	}
	return os.WriteFile(path, []byte(b.String()), filePerm)
}

// isIgnorableLockErr は、lockfile 読み込みエラーのうち無視して続行してよいものを判定する。
// nil（成功）と「ファイル不在」（初回 resolve）のみ true。それ以外（解釈できない行・キー重複・
// 権限エラー等）は fail-close 対象。
func isIgnorableLockErr(err error) bool {
	return err == nil || xerrors.Is(err, os.ErrNotExist)
}

// readLock は lockfile を image:tag→digest として読む。空行とコメント行以外で代入として解釈
// できない行、および既出キーの再定義はエラーにする。読み飛ばしや後勝ちの上書きは、そのエントリが
// 「存在しない」あるいは「行順で決まる」状態を警告なく作り、lockfile が SSOT として機能しなくなる。
func readLock(path string) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // path is constructed from cwd + literal filename
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	lock := map[string]string{}
	sc := bufio.NewScanner(f)
	for lineNo := 1; sc.Scan(); lineNo++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := lockRe.FindStringSubmatch(line)
		if m == nil {
			return nil, xerrors.Wrap(errLockInvalidLine,
				fmt.Sprintf("%d 行目: %q（make pin-images-resolve を実行するか該当行を削除してください）", lineNo, line))
		}
		if _, dup := lock[m[1]]; dup {
			return nil, xerrors.Wrap(errLockDuplicateKey,
				fmt.Sprintf("%d 行目: %q（make pin-images-resolve を実行するか重複行を削除してください）", lineNo, m[1]))
		}
		lock[m[1]] = m[2]
	}
	return lock, sc.Err()
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}

// uniq はソート済みスライスから隣接重複を除く。
func uniq(s []string) []string {
	out := s[:0]
	var prev string
	for i, v := range s {
		if i == 0 || v != prev {
			out = append(out, v)
		}
		prev = v
	}
	return out
}
