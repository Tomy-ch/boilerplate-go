package dbslot

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"go-boilerplate/pkg/xerrors"
)

const (
	// GitUnavailable は、git 実行ファイルが無い状態です（ツールランナーのコンテナ内など）。
	GitUnavailable GitContext = iota
	// GitNoRepository は、git はあるが作業ディレクトリが git リポジトリでない状態です。
	GitNoRepository
	// GitMainCheckout は、主 checkout です（git-dir と git-common-dir が一致する）。
	GitMainCheckout
	// GitLinkedWorktree は、git worktree add で作ったリンク worktree です（両者が食い違う）。
	GitLinkedWorktree
)

const (
	// defaultDBLocal / defaultDBTest は、スロットを保持しない checkout（＝主 checkout）の
	// 所有データベースです。
	defaultDBLocal = "local"
	defaultDBTest  = "test"

	// noRecreateFlag は、共有インフラの稼働中コンテナを作り直させない compose のフラグです。
	noRecreateFlag = "--no-recreate"

	// gitDirsFields は `git rev-parse --git-dir --git-common-dir` が返す行数です。
	gitDirsFields = 2

	// mockAuthIssuerPath は、mock 認証サーバーが issuer を生やすパスです。値は
	// docker/mock-auth-server/config.json の issuerId と一致していなければなりません。
	mockAuthIssuerPath = "/default"

	// slotInfixTable / slotInfixName は、基底名とスロット番号の間に入る区切りです。
	// table だけ `_` なのは REALTIME_TABLE_SUFFIX の使用可能文字に合わせるためで、
	// queue と topic は `-` です。
	slotInfixTable = "_wt"
	slotInfixName  = "-wt"
)

var (
	// errGitLayoutUnreadable は、git リポジトリではあるのにその構成を読み取れなかったことを表します。
	// 「リンク worktree かどうか」を判定できないため、所有者判定はここで止まります。
	errGitLayoutUnreadable = xerrors.New("git repository exists but its layout could not be read")

	// errNoDatabaseOwner は、リンク worktree がスロットを取得しておらず所有データベースが無いことを表します。
	errNoDatabaseOwner = xerrors.New("this worktree owns no database; run `make slot-acquire`")
)

// GitContext は、この checkout がどの git 文脈にあるかを表します。
// 「所有データベースが無い状態を検出できるか」はこの区別で決まります。
type GitContext int

// GitProbe は、git 文脈の判定に使う外部依存の注入点です（テストで差し替えます）。
type GitProbe struct {
	// LookGit は、git 実行ファイルが使えるかを返します。
	LookGit func() error
	// GitDirs は、root で `git rev-parse --git-dir --git-common-dir` を実行した標準出力を返します。
	GitDirs func(ctx context.Context, root string) (string, error)
	// HasGitEntry は、root か その祖先に .git が存在するかを返します。
	HasGitEntry func(root string) bool
}

// LeaseProbe は、リース所有権の判定に使う外部依存の注入点です（テストで差し替えます）。
// .gobp-db-slot は解放されないまま残ることがあるので、ファイルの存在だけでは所有を名乗れません。
type LeaseProbe struct {
	// OwnedBySelf は、そのスロットのリースを自 worktree が保持しているかを返します。
	OwnedBySelf func(slot int) bool
}

// Values は、スロットから導かれる解決済みの値です。
// 一箇所で導いて env（make が eval する KEY=VALUE）と status（人間向けの表示）の両方へ流すため、
// 「make が使う値」と「デバッグで読む値」が食い違いません。
type Values struct {
	Git             GitContext
	SlotHeld        bool   // .gobp-db-slot が宣言するスロットのリースを実際に保持しているか
	DBLocal         string // 所有する local 系データベース
	DBTest          string // 所有する test 系データベース
	AppProject      string // app 層の compose プロジェクト名
	AuthIssuer      string // mock 認証サーバーのホスト公開 URL（トークンの iss）
	InfraNoRecreate string // 共有インフラへ渡す --no-recreate（不要なら空）

	// Realtime Delivery の資源名。共有の DynamoDB Local / GoAWS 上でスロット毎に名前空間を分けます
	// （docs/maintenance/db-worktree-pool.md「The Realtime Delivery emulators are shared instances」）。
	RealtimeTableSuffix string // table 名の末尾（realtime_event_log_<suffix> の suffix）
	RealtimeQueuePrefix string // serve instance ごとの queue 名の先頭
	RealtimeTopic       string // fan-out topic の ARN
}

// Resolver は、スロットから導かれる値の解決と所有者判定を担います。
// .gobp-db-slot を書いているのが Pool である以上、そこから導かれる値の所有者も同じ側に置きます。
type Resolver struct {
	cfg   Config
	probe GitProbe
	lease LeaseProbe
	out   io.Writer
}

// String は、GitContext の表示名を返します。
func (c GitContext) String() string {
	switch c {
	case GitUnavailable:
		return "git-unavailable"
	case GitNoRepository:
		return "no-repository"
	case GitMainCheckout:
		return "main-checkout"
	case GitLinkedWorktree:
		return "linked-worktree"
	default:
		return "unknown"
	}
}

// NewResolver は Resolver を生成します。probe が nil の場合はホストの git を使います。
// lease が nil のときはリースを確認できないため、どのスロットも保持していないものとして解決します
// （所有を騙るより、スロットの取り直しを案内して止まるほうが安全なため）。
func NewResolver(cfg Config, probe *GitProbe, lease *LeaseProbe, out io.Writer) *Resolver {
	p := realGitProbe()
	if probe != nil {
		p = *probe
	}

	var l LeaseProbe
	if lease != nil {
		l = *lease
	}

	return &Resolver{cfg: cfg, probe: p, lease: l, out: out}
}

// Resolve は、git 文脈と .gobp-db-slot から解決済みの値を組み立てます。
// git リポジトリなのに構成を読めなかった場合だけエラーにします（inspectGit を参照）。
func (r *Resolver) Resolve(ctx context.Context) (Values, error) {
	gitCtx, err := r.inspectGit(ctx)
	if err != nil {
		return Values{}, err
	}

	values := Values{
		Git:        gitCtx,
		DBLocal:    defaultDBLocal,
		DBTest:     defaultDBTest,
		AppProject: "gobp-app-" + filepath.Base(r.cfg.Root),
		AuthIssuer: mockAuthIssuer(r.cfg.MockAuthBase),

		RealtimeTableSuffix: r.cfg.Realtime.TableSuffix,
		RealtimeQueuePrefix: r.cfg.Realtime.QueuePrefix,
		RealtimeTopic:       r.cfg.Realtime.Topic,
	}

	// リースを保持しているスロットの値だけを採用する。導出はスロット番号から行い、
	// .gobp-db-slot が並べている DB 名やプロジェクト名は読まない。レジストリで裏が取れるのは
	// 番号だけで、他のフィールドは手編集や別 worktree のファイルの写しで食い違い得る。
	if slot, held := r.heldSlot(); held {
		num := strconv.Itoa(slot)
		values.SlotHeld = true
		values.DBLocal = dbLocal(slot)
		values.DBTest = dbTest(slot)
		values.AppProject = serveProject(slot)
		values.AuthIssuer = mockAuthIssuer(r.cfg.MockAuthBase + slot)
		values.RealtimeTableSuffix = realtimeName(r.cfg.Realtime.TableSuffix, num, slotInfixTable)
		values.RealtimeQueuePrefix = realtimeName(r.cfg.Realtime.QueuePrefix, num, slotInfixName)
		values.RealtimeTopic = realtimeName(r.cfg.Realtime.Topic, num, slotInfixName)
	}

	// 共有インフラを奪い合う相手が居るのはリンク worktree のときだけなので、単一 checkout では空にします。
	if gitCtx == GitLinkedWorktree {
		values.InfraNoRecreate = noRecreateFlag
	}

	return values, nil
}

// RequireOwner は、所有データベースを持たない状態（リンク worktree かつスロット未取得）を
// 検出してエラーにします。主 checkout・ツールランナー内・CI は素通りします。
//
// 判定は三値です。「git 実行ファイルが無い」「リポジトリでない」は素通り、
// 「リポジトリなのに読めない」は失敗（理由は docs/maintenance/db-worktree-pool.md）。
func (r *Resolver) RequireOwner(ctx context.Context) error {
	values, err := r.Resolve(ctx)
	if err != nil {
		return err
	}

	if values.Git != GitLinkedWorktree || values.SlotHeld {
		return nil
	}

	_, _ = fmt.Fprint(r.out, "❌ この worktree は DB スロットを取得していない（または取得したスロットが\n"+
		"   他 worktree に回収されている）ため、所有するデータベースがありません。\n"+
		"   1 つのデータベースを複数の checkout から触らないよう、既定の local / test へは\n"+
		"   フォールバックしません。\n"+
		"   → make slot-acquire でスロットを取得してください（make slot-status で空きを確認）。\n")

	return errNoDatabaseOwner
}

// PrintEnv は、make のレシピが eval して読む KEY=VALUE を出力します。
func (r *Resolver) PrintEnv(ctx context.Context) error {
	values, err := r.Resolve(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(r.out, RenderEnv(values))

	return nil
}

// PrintValues は、解決済みの値を人間向けに表示します。どの compose プロジェクトとどの
// データベースを叩くのかは、この経路が唯一の確認窓口になります。
func (r *Resolver) PrintValues(ctx context.Context) error {
	values, err := r.Resolve(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(r.out, RenderValues(values))

	return nil
}

// heldSlot は、.gobp-db-slot が宣言するスロットのうち、自 worktree がリースを保持しているものを返します。
// 宣言が壊れている（スロット番号や DB 名が読めない）場合と、リースを確認できない場合は保持なしとします。
func (r *Resolver) heldSlot() (int, bool) {
	slot := readSlotFile(filepath.Join(r.cfg.Root, ".gobp-db-slot"))
	if slot["DB_NAME_LOCAL"] == "" || r.lease.OwnedBySelf == nil {
		return 0, false
	}

	n, err := strconv.Atoi(slot["SLOT"])
	if err != nil {
		return 0, false
	}

	return n, r.lease.OwnedBySelf(n)
}

// inspectGit は、git 文脈を判定します。
// git を引けなかったときは、.git の有無で「リポジトリでない」と「リポジトリなのに読めない」を
// 分けます。git のエラーメッセージはロケールで変わるため、メッセージは判定材料にしません。
func (r *Resolver) inspectGit(ctx context.Context) (GitContext, error) {
	if err := r.probe.LookGit(); err != nil {
		return GitUnavailable, nil //nolint:nilerr // git が無いのは異常ではなく「判定できない文脈」であり、素通りが正しい
	}

	out, err := r.probe.GitDirs(ctx, r.cfg.Root)
	if err != nil {
		if r.probe.HasGitEntry(r.cfg.Root) {
			return 0, xerrors.Wrap(errGitLayoutUnreadable, err.Error())
		}

		return GitNoRepository, nil
	}

	fields := strings.Fields(out)
	if len(fields) != gitDirsFields {
		return 0, xerrors.Wrap(errGitLayoutUnreadable, "unexpected `git rev-parse` output: "+strconv.Quote(out))
	}

	if r.absDir(fields[0]) == r.absDir(fields[1]) {
		return GitMainCheckout, nil
	}

	return GitLinkedWorktree, nil
}

// absDir は、git が返したパス（cwd 相対のことがある）を比較できる形へ揃えます。
func (r *Resolver) absDir(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.cfg.Root, path)
	}

	return filepath.Clean(path)
}

// RenderEnv は、解決済みの値を make のレシピが eval できる KEY=VALUE へ整形します。
func RenderEnv(v Values) string {
	pairs := [][2]string{
		{"GOBP_GIT_CONTEXT", v.Git.String()},
		{"DB_LOCAL", v.DBLocal},
		{"DB_TEST", v.DBTest},
		{"APP_PROJECT", v.AppProject},
		{"AUTH_ISSUER", v.AuthIssuer},
		{"INFRA_NO_RECREATE", v.InfraNoRecreate},
		{"REALTIME_TABLE_SUFFIX", v.RealtimeTableSuffix},
		{"REALTIME_QUEUE_PREFIX", v.RealtimeQueuePrefix},
		{"REALTIME_TOPIC", v.RealtimeTopic},
	}

	var sb strings.Builder
	for _, kv := range pairs {
		fmt.Fprintf(&sb, "%s='%s'\n", kv[0], strings.ReplaceAll(kv[1], "'", `'\''`))
	}

	return sb.String()
}

// RenderValues は、解決済みの値を人間向けの一覧へ整形します。
func RenderValues(v Values) string {
	var sb strings.Builder

	sb.WriteString("\n--- この checkout の解決済みの値 ---\n")
	fmt.Fprintf(&sb, "git context       : %s\n", v.Git)
	fmt.Fprintf(&sb, "slot held         : %s\n", yesNo(v.SlotHeld))
	fmt.Fprintf(&sb, "DB_LOCAL          : %s\n", v.DBLocal)
	fmt.Fprintf(&sb, "DB_TEST           : %s\n", v.DBTest)
	fmt.Fprintf(&sb, "APP_PROJECT       : %s\n", v.AppProject)
	fmt.Fprintf(&sb, "AUTH_ISSUER       : %s\n", v.AuthIssuer)
	fmt.Fprintf(&sb, "INFRA_NO_RECREATE : %s\n", orDefault(v.InfraNoRecreate, "（渡さない）"))
	fmt.Fprintf(&sb, "REALTIME_TABLE    : %s\n", v.RealtimeTableSuffix)
	fmt.Fprintf(&sb, "REALTIME_QUEUE    : %s\n", v.RealtimeQueuePrefix)
	fmt.Fprintf(&sb, "REALTIME_TOPIC    : %s\n", v.RealtimeTopic)

	return sb.String()
}

// realGitProbe は、ホストの git を使う GitProbe を返します。
func realGitProbe() GitProbe {
	return GitProbe{
		LookGit: func() error {
			_, err := exec.LookPath("git")

			return err
		},
		GitDirs: func(ctx context.Context, root string) (string, error) {
			cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir", "--git-common-dir")
			cmd.Dir = root
			out, err := cmd.Output()

			return string(out), err
		},
		HasGitEntry: hasGitEntry,
	}
}

// hasGitEntry は、dir か その祖先に .git（ディレクトリ、またはリンク worktree の .git ファイル）が
// あるかを返します。
func hasGitEntry(dir string) bool {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}

		dir = parent
	}
}

// readSlotFile は、.gobp-db-slot の KEY=VALUE を読み取ります。
// ファイルが無い状態は「スロット未取得」という正常な状態なので、空の map を返します。
func readSlotFile(path string) map[string]string {
	values := make(map[string]string)

	b, err := os.ReadFile(path) //nolint:gosec // 読むのは自 worktree の .gobp-db-slot のみ
	if err != nil {
		return values
	}

	for line := range strings.SplitSeq(string(b), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}

		values[key] = value
	}

	return values
}

// mockAuthIssuer は、mock 認証サーバーのホスト公開ポートから issuer URL を組み立てます。
func mockAuthIssuer(port int) string {
	return "http://localhost:" + strconv.Itoa(port) + mockAuthIssuerPath
}

// realtimeName は、基底名にスロット番号を継いだ資源名を返します（基底の出所は Config.Realtime）。
//
// 基底が空なら継ぎません。空の topic は「fan-out を配線すると起動に失敗する」という env の契約で、
// そこへスロット番号だけを継ぐと `-wt2` のような、契約でも正しい名前でもない値になります。
func realtimeName(base, slot, infix string) string {
	if base == "" || slot == "" {
		return base
	}

	return base + infix + slot
}

// orDefault は、value が空なら def を返します。
func orDefault(value, def string) string {
	if value == "" {
		return def
	}

	return value
}

// yesNo は、真偽を表示用の文字列にします。
func yesNo(v bool) string {
	if v {
		return "yes"
	}

	return "no"
}
