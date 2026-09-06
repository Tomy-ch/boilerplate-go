// Package dbslot は、複数の git worktree が単一共有 Postgres を per-worktree のデータベース
// （wt<N>_local / wt<N>_test）として貸し借りする DB スロットプールのコアロジックを提供します。
// リース（ロックディレクトリ）管理・stale 回収・DB 作成・compose 起動を担い、cmd/db-slot.go から
// 実依存を配線して呼び出します。詳細は docs/maintenance/db-worktree-pool.md。
package dbslot

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"go-boilerplate/pkg/xerrors"
)

const (
	dirPerm  = 0o700 // レジストリ / ロックディレクトリ（ユーザー専有）
	filePerm = 0o600 // meta / lock ファイル（ユーザー専有）
)

// errPoolDirSymlink は、レジストリのプールディレクトリが symlink だった場合のエラーです。
var errPoolDirSymlink = xerrors.New("pool dir is a symlink; set GOBP_DB_POOL_DIR to a safe path")

// reclaimSeq は、回収時の一時名を一意にするためのカウンタ（プロセス内 goroutine 間の衝突回避）。
var reclaimSeq atomic.Int64

// Meta は、スロットのリース保持者情報です。
type Meta struct {
	Owner     string // リース保持 worktree の絶対パス
	Heartbeat time.Time
	Branch    string
	Slot      int
}

// Registry は、ホスト上のロックディレクトリでスロットのリースを管理します。
type Registry struct {
	dir      string
	owner    string // 自 worktree の絶対パス
	branch   string
	ttl      time.Duration // heartbeat 失効までの猶予
	maxSlots int
	now      func() time.Time // テストで差し替え可能
}

// NewRegistry は Registry を生成します。now が nil の場合は time.Now を使います。
func NewRegistry(dir, owner, branch string, ttl time.Duration, maxSlots int, now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{dir: dir, owner: owner, branch: branch, ttl: ttl, maxSlots: maxSlots, now: now}
}

// Lock は、acquire 走査を全プロセス・goroutine 横断で直列化する flock を取得します（戻り値で解放）。
// fresh mkdir と reclaim rename の隙間で起きる二重取得を防ぐため。EnsureDir 後に呼ぶこと。
func (r *Registry) Lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(r.dir, ".scan.lock"), os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to open scan lock")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, xerrors.Wrap(err, "failed to acquire scan lock")
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// EnsureDir は、レジストリを用意します。symlink は先読み攻撃対策として拒否します。
func (r *Registry) EnsureDir() error {
	if fi, err := os.Lstat(r.dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return xerrors.Wrap(errPoolDirSymlink, fmt.Sprintf("%q", r.dir))
	}
	if err := os.MkdirAll(r.dir, dirPerm); err != nil {
		return xerrors.Wrap(err, "failed to create pool dir")
	}
	_ = os.Chmod(r.dir, dirPerm)
	return nil
}

// MaxSlots は、スロット数を返します。
func (r *Registry) MaxSlots() int { return r.maxSlots }

// TryAcquireFresh は、未使用スロットを os.Mkdir の原子性で取得します（成功で true）。
func (r *Registry) TryAcquireFresh(slot int) bool {
	return os.Mkdir(r.lockDir(slot), dirPerm) == nil
}

// TryReclaim は、stale スロットを rename で原子的に奪って作り直します（勝者は1つ、負けは false）。
// rename の source は一度しか成功しないのが要点。rm→mkdir は互いの mkdir を消して全員成功しうる。
func (r *Registry) TryReclaim(slot int) bool {
	d := r.lockDir(slot)
	tmp := fmt.Sprintf("%s.claim.%d.%d", d, os.Getpid(), reclaimSeq.Add(1))
	if os.Rename(d, tmp) != nil {
		return false // 先取りされた
	}
	_ = os.RemoveAll(tmp)
	return os.Mkdir(d, dirPerm) == nil
}

// Exists は、スロットのロックが存在するかを返します。
func (r *Registry) Exists(slot int) bool {
	fi, err := os.Stat(r.lockDir(slot))
	return err == nil && fi.IsDir()
}

// WriteMeta は、スロットの meta を書き出します。
func (r *Registry) WriteMeta(slot int) error {
	m := fmt.Sprintf("owner=%s\nheartbeat=%d\nbranch=%s\nslot=%d\n",
		r.owner, r.now().Unix(), r.branch, slot)
	if err := os.WriteFile(r.metaPath(slot), []byte(m), filePerm); err != nil {
		return xerrors.Wrap(err, "failed to write slot meta")
	}
	return nil
}

// ReadMeta は、スロットの meta を読み取ります（存在しなければ ok=false）。
func (r *Registry) ReadMeta(slot int) (Meta, bool) {
	f, err := os.Open(r.metaPath(slot))
	if err != nil {
		return Meta{}, false
	}
	defer func() { _ = f.Close() }()

	m := Meta{Slot: slot}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "owner":
			m.Owner = v
		case "branch":
			m.Branch = v
		case "heartbeat":
			if sec, err := strconv.ParseInt(v, 10, 64); err == nil {
				m.Heartbeat = time.Unix(sec, 0)
			}
		}
	}
	if sc.Err() != nil {
		return Meta{}, false
	}
	return m, true
}

// OwnedBySelf は、スロットが自 worktree の保持かを返します。
func (r *Registry) OwnedBySelf(slot int) bool {
	m, ok := r.ReadMeta(slot)
	return ok && m.Owner == r.owner
}

// HeldByOther は、meta を読み取れて、かつ保持者が自分でないことを返します。
// 読み取れなかった場合は false です。「他 worktree が保持している」と「判定できなかった」を
// 同じ真に畳むと、レジストリを一度読めなかっただけで自分の後始末を取り下げることになります。
func (r *Registry) HeldByOther(slot int) bool {
	m, ok := r.ReadMeta(slot)
	return ok && m.Owner != r.owner
}

// IsStale は、他 worktree のリースが heartbeat TTL を超過しているかを返します（自分の保持は常に非 stale）。
func (r *Registry) IsStale(slot int) bool {
	m, ok := r.ReadMeta(slot)
	if !ok {
		return true
	}
	if m.Owner == r.owner {
		return false
	}
	if m.Heartbeat.IsZero() {
		return true
	}
	return r.now().Sub(m.Heartbeat) > r.ttl
}

// Release は、スロットのロックを解放します（自分の保持のときのみ）。解放したかを返します。
// 保持を確認できないロックは残します。meta を書く前の取得直後と区別できず、消せば他 worktree の
// 取得を壊すためです。残っても meta の無いロックは stale として次の走査が回収します。
func (r *Registry) Release(slot int) bool {
	if !r.OwnedBySelf(slot) {
		return false
	}
	_ = os.RemoveAll(r.lockDir(slot))

	return true
}

// AgeSeconds は、スロットの heartbeat からの経過秒数を返します（meta 無しは -1）。
func (r *Registry) AgeSeconds(slot int) int64 {
	m, ok := r.ReadMeta(slot)
	if !ok || m.Heartbeat.IsZero() {
		return -1
	}
	return int64(r.now().Sub(m.Heartbeat).Seconds())
}

func (r *Registry) lockDir(slot int) string {
	return filepath.Join(r.dir, fmt.Sprintf("slot-%d.lock", slot))
}

func (r *Registry) metaPath(slot int) string {
	return filepath.Join(r.lockDir(slot), "meta")
}
