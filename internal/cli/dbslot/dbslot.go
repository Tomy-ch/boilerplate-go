package dbslot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go-boilerplate/internal/config"
	"go-boilerplate/pkg/xerrors"
)

var (
	// errNoFreeSlot は、空きスロットが無くリースできなかった場合のエラーです。
	errNoFreeSlot = xerrors.New("no free slot; release one or raise GOBP_DB_POOL_MAX")
	// errDeployEnvRefused は、deploy 系 env で DB スロット操作を拒否した場合のエラーです。
	errDeployEnvRefused = xerrors.New("db-slot refuses to run outside a local/ci/test environment")
)

// Config は、Pool の振る舞いを決めるパラメータです。
type Config struct {
	Root          string // 自 worktree の絶対パス（.gobp-db-slot の出力先・owner）
	SharedProject string // 共有インフラの固定 compose プロジェクト（例: gobp-shared）
	APIBasePort   int    // API_HOST_PORT のベース（スロット N = base+N）
	MockAuthBase  int    // MOCK_AUTH_HOST_PORT のベース
	DlvBase       int    // DLV_HOST_PORT のベース
	PprofBase     int    // PPROF_HOST_PORT のベース
	APPEnv        string // 実行環境ラベル（deploy 系ガードに使用）
}

// Pool は、リース・DB 管理・compose 起動を統合するオーケストレータです。
type Pool struct {
	reg   *Registry
	admin DBAdmin
	comp  Compose
	cfg   Config
	out   io.Writer // 標準出力（.gobp-db-slot の内容・status 表）
	logw  io.Writer // 進捗ログ（stderr）
}

// NewPool は Pool を生成します。
func NewPool(reg *Registry, admin DBAdmin, comp Compose, cfg Config, out, logw io.Writer) *Pool {
	return &Pool{reg: reg, admin: admin, comp: comp, cfg: cfg, out: out, logw: logw}
}

// Acquire は、空きスロットをリースし共有 DB 上に自 worktree の DB を用意して .gobp-db-slot を書き出します。
func (p *Pool) Acquire(ctx context.Context) error {
	if err := p.ensureLocalEnv(); err != nil {
		return err
	}
	if err := p.reg.EnsureDir(); err != nil {
		return err
	}
	if err := p.comp.UpSharedDB(ctx, p.cfg.SharedProject); err != nil {
		return err
	}

	// 既に自分が保持するスロットがあれば heartbeat 更新して再利用（冪等）。
	if slot, ok := p.heldSlot(); ok && p.reg.OwnedBySelf(slot) {
		return p.finishAcquire(ctx, slot, "reuse")
	}

	// flock で走査を直列化し二重リースを防ぐ。
	unlock, err := p.reg.Lock()
	if err != nil {
		return err
	}
	defer unlock()

	for slot := 1; slot <= p.reg.MaxSlots(); slot++ {
		switch {
		case p.reg.TryAcquireFresh(slot):
		case p.reg.IsStale(slot):
			// heartbeat は make serve 時にしか打たれないため、起動しっぱなしの app を持つスロットも
			// TTL 超過で stale になる。DB を作り直す前に serve 中でないことを確かめる。
			busy, err := p.slotInUse(ctx, slot)
			if err != nil {
				return err
			}
			if busy {
				continue
			}
			if !p.reg.TryReclaim(slot) {
				p.logf("slot %d reclaim lost to another process, skip", slot)
				continue
			}
			p.logf("reclaim stale slot %d", slot)
		default:
			continue
		}
		return p.finishAcquire(ctx, slot, "acquired")
	}
	return xerrors.Wrap(errNoFreeSlot, fmt.Sprintf("all %d slots in use", p.reg.MaxSlots()))
}

// Release は、serve コンテナを停止し、リースと .gobp-db-slot を解放します（DB は warm 保持）。
func (p *Pool) Release(ctx context.Context) error {
	slot, ok := p.heldSlot()
	if !ok {
		p.logf("no slot held by this worktree")
		return nil
	}
	// リースが stale 回収で他 worktree へ渡っていれば、そのスロットの app コンテナは現保持者のもの。
	// down すれば他人の serve を落とすため compose には触れず、失効した .gobp-db-slot だけを片付ける。
	if !p.reg.OwnedBySelf(slot) {
		_ = os.Remove(p.slotFilePath())
		p.logf("slot %d is held by another worktree; dropped the stale slot file only", slot)
		return nil
	}
	// serve した app コンテナを停止する（放置すると再割当て・reinit 後の DB を孤児が掴む）。
	// 停止失敗（docker 未起動・権限不足など）は孤児コンテナ検知のためログへ可視化し、リース解放自体は続行する。
	if err := p.comp.DownServe(ctx, serveProject(slot)); err != nil {
		p.logf("failed to stop serve containers for slot %d: %v", slot, err)
	}
	p.reg.Release(slot)
	_ = os.Remove(p.slotFilePath())
	p.logf("released slot %d (databases left warm for reuse)", slot)
	return nil
}

// Heartbeat は、自 worktree の保持スロットの heartbeat を更新します。
func (p *Pool) Heartbeat() error {
	slot, ok := p.heldSlot()
	if !ok || !p.reg.OwnedBySelf(slot) {
		return nil
	}
	return p.reg.WriteMeta(slot)
}

// Status は、スロットの占有状況を表示します。
func (p *Pool) Status() error {
	if err := p.ensureLocalEnv(); err != nil {
		return err
	}
	if err := p.reg.EnsureDir(); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(p.out, "%-5s %-14s %-14s %-6s %-9s %-9s %s\n",
		"SLOT", "DB_LOCAL", "DB_TEST", "API", "STATE", "AGE", "OWNER")
	for slot := 1; slot <= p.reg.MaxSlots(); slot++ {
		state, age, owner := "free", "-", "-"
		if p.reg.Exists(slot) {
			m, _ := p.reg.ReadMeta(slot)
			owner = m.Owner
			age = strconv.FormatInt(p.reg.AgeSeconds(slot), 10)
			if p.reg.IsStale(slot) {
				state = "stale"
			} else {
				state = "in-use"
			}
		}
		_, _ = fmt.Fprintf(p.out, "%-5d %-14s %-14s %-6d %-9s %-9s %s\n",
			slot, dbLocal(slot), dbTest(slot), p.cfg.APIBasePort+slot, state, age, owner)
	}
	return nil
}

func (p *Pool) logf(format string, a ...any) {
	_, _ = fmt.Fprintf(p.logw, "[db-slot] "+format+"\n", a...)
}

func (p *Pool) slotFilePath() string { return filepath.Join(p.cfg.Root, ".gobp-db-slot") }

// ensureLocalEnv は、deploy 系 env（dev/stg/prd）での実行を拒否します（DB を作成/破棄する dev/test 専用
// ツールのため。APP_ENV 未設定はローカル開発とみなし許可）。
func (p *Pool) ensureLocalEnv() error {
	if p.cfg.APPEnv != "" && !config.IsLocalClassEnv(p.cfg.APPEnv) {
		return xerrors.Wrap(errDeployEnvRefused, fmt.Sprintf("APP_ENV=%q", p.cfg.APPEnv))
	}
	return nil
}

// slotInUse は、stale なスロットが実際にはまだ使われているかを返します。
// app コンテナの稼働と DB への接続をそれぞれ確認します。接続プールはアイドルで空になるため、
// 接続数だけでは serve 中の worktree を見落とします。
func (p *Pool) slotInUse(ctx context.Context, slot int) (bool, error) {
	running, err := p.comp.RunningContainers(ctx, serveProject(slot))
	if err != nil {
		return false, err
	}
	if running > 0 {
		p.logf("slot %d is stale but %s has running containers, skip", slot, serveProject(slot))
		return true, nil
	}

	conns, err := p.admin.ActiveConnections(ctx, dbLocal(slot), dbTest(slot))
	if err != nil {
		return false, err
	}
	if conns > 0 {
		p.logf("slot %d is stale but has active DB connections, skip", slot)
		return true, nil
	}

	return false, nil
}

// finishAcquire は、取得済みスロットに meta / DB / .gobp-db-slot を確定させます。
func (p *Pool) finishAcquire(ctx context.Context, slot int, verb string) error {
	if err := p.reg.WriteMeta(slot); err != nil {
		return err
	}
	if err := p.ensureSlotDBs(ctx, slot); err != nil {
		return err
	}
	if err := p.writeSlotFile(slot); err != nil {
		return err
	}
	p.logf("%s slot %d (db %s / %s)", verb, slot, dbLocal(slot), dbTest(slot))
	return p.printSlotFile()
}

// ensureSlotDBs は、wt<N>_local / wt<N>_test を作成し拡張を設定します。
func (p *Pool) ensureSlotDBs(ctx context.Context, slot int) error {
	for _, name := range []string{dbLocal(slot), dbTest(slot)} {
		if err := p.admin.EnsureDatabase(ctx, name); err != nil {
			return err
		}
		if err := p.admin.SetupDatabase(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// writeSlotFile は、.gobp-db-slot（make が -include で読む KEY=VALUE）を書き出します。
// app コンテナのホスト公開ポートは全てスロット番号で相対化し、並列 serve が衝突しないようにします。
func (p *Pool) writeSlotFile(slot int) error {
	content := fmt.Sprintf(
		"SLOT=%d\nDB_NAME_LOCAL=%s\nDB_NAME_TEST=%s\n"+
			"API_HOST_PORT=%d\nMOCK_AUTH_HOST_PORT=%d\nDLV_HOST_PORT=%d\nPPROF_HOST_PORT=%d\n"+
			"COMPOSE_PROJECT_NAME=%s\nSERVE_PROJECT=%s\n",
		slot, dbLocal(slot), dbTest(slot),
		p.cfg.APIBasePort+slot, p.cfg.MockAuthBase+slot, p.cfg.DlvBase+slot, p.cfg.PprofBase+slot,
		p.cfg.SharedProject, serveProject(slot),
	)
	if err := os.WriteFile(p.slotFilePath(), []byte(content), filePerm); err != nil {
		return xerrors.Wrap(err, "failed to write .gobp-db-slot")
	}
	return nil
}

func (p *Pool) printSlotFile() error {
	b, err := os.ReadFile(p.slotFilePath())
	if err != nil {
		return xerrors.Wrap(err, "failed to read .gobp-db-slot")
	}
	_, _ = p.out.Write(b)
	return nil
}

// heldSlot は、.gobp-db-slot に記録された保持スロット番号を返します。
func (p *Pool) heldSlot() (int, bool) {
	b, err := os.ReadFile(p.slotFilePath())
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "SLOT="); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			return n, err == nil
		}
	}
	return 0, false
}

func dbLocal(slot int) string      { return fmt.Sprintf("wt%d_local", slot) }
func dbTest(slot int) string       { return fmt.Sprintf("wt%d_test", slot) }
func serveProject(slot int) string { return fmt.Sprintf("gobp-wt-%d", slot) }
