package dbslot

import (
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRegistry(t *testing.T, owner string, now time.Time) *Registry {
	t.Helper()
	r := NewRegistry(t.TempDir(), owner, "branch", 30*time.Minute, 8, func() time.Time { return now })
	require.NoError(t, r.EnsureDir())
	return r
}

func TestRegistry_TryAcquireFresh(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未使用スロットは最初の1回だけ取得でき、2回目は失敗する", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/a", time.Unix(1000, 0))

			assert.True(t, r.TryAcquireFresh(1))
			assert.False(t, r.TryAcquireFresh(1))
		})

		t.Run("並行取得でも成功は1つだけ（二重リースしない）", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/a", time.Unix(1000, 0))

			const n = 20
			var wg sync.WaitGroup
			var mu sync.Mutex
			wins := 0
			wg.Add(n)
			for range n {
				go func() {
					defer wg.Done()
					if r.TryAcquireFresh(1) {
						mu.Lock()
						wins++
						mu.Unlock()
					}
				}()
			}
			wg.Wait()

			assert.Equal(t, 1, wins)
		})
	})
}

func TestRegistry_IsStale(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("自 worktree の保持スロットは stale 扱いしない", func(t *testing.T) {
			t.Parallel()

			now := time.Unix(10_000, 0)
			r := newTestRegistry(t, "/w/self", now)
			require.True(t, r.TryAcquireFresh(1))
			require.NoError(t, r.WriteMeta(1))

			// TTL を大きく超えて時間が進んでも、owner が自分なら stale ではない。
			r2 := NewRegistry(r.dir, "/w/self", "b", 30*time.Minute, 8, func() time.Time { return now.Add(time.Hour) })
			assert.False(t, r2.IsStale(1))
		})

		t.Run("他 worktree の保持スロットは heartbeat が TTL 超過なら stale", func(t *testing.T) {
			t.Parallel()

			now := time.Unix(10_000, 0)
			owner := newTestRegistry(t, "/w/other", now)
			require.True(t, owner.TryAcquireFresh(1))
			require.NoError(t, owner.WriteMeta(1))

			// 別 worktree から見て、TTL(30分)以内は非 stale、超過で stale。
			viewer := NewRegistry(owner.dir, "/w/me", "b", 30*time.Minute, 8, func() time.Time { return now.Add(29 * time.Minute) })
			assert.False(t, viewer.IsStale(1))

			viewerLate := NewRegistry(owner.dir, "/w/me", "b", 30*time.Minute, 8, func() time.Time { return now.Add(31 * time.Minute) })
			assert.True(t, viewerLate.IsStale(1))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("meta が無いスロットは stale 扱い", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/a", time.Unix(1000, 0))
			assert.True(t, r.IsStale(5))
		})

		t.Run("heartbeat 欠落 meta（他 owner）は stale 扱い", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/me", time.Unix(1000, 0))
			require.True(t, r.TryAcquireFresh(1))
			// heartbeat を持たない他 owner の meta を直接書く。
			require.NoError(t, os.WriteFile(r.metaPath(1), []byte("owner=/w/other\nbranch=b\nslot=1\n"), 0o600))

			assert.True(t, r.IsStale(1))
		})
	})
}

func TestRegistry_TryReclaim(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("stale スロットを並行回収しても占有権を得るのは1つだけ", func(t *testing.T) {
			t.Parallel()

			now := time.Unix(10_000, 0)
			// 他 worktree が古い heartbeat で保持中の状態を作る。
			owner := newTestRegistry(t, "/w/other", now)
			require.True(t, owner.TryAcquireFresh(1))
			require.NoError(t, owner.WriteMeta(1))

			late := now.Add(time.Hour)
			const n = 20
			var wg sync.WaitGroup
			var mu sync.Mutex
			wins := 0
			wg.Add(n)
			for i := range n {
				id := i
				go func() {
					defer wg.Done()
					// 各 goroutine は別 worktree（別 owner）としてオーケストレータと同じ scan lock + claim +
					// WriteMeta を行う。flock で直列化され、最初の1つだけが占有権を得る。
					ownerPath := "/w/me/" + strconv.Itoa(id)
					r := NewRegistry(owner.dir, ownerPath, "b", 30*time.Minute, 8, func() time.Time { return late })
					unlock, err := r.Lock()
					if err != nil {
						return
					}
					defer unlock()
					if r.IsStale(1) && r.TryReclaim(1) {
						_ = r.WriteMeta(1)
						mu.Lock()
						wins++
						mu.Unlock()
					}
				}()
			}
			wg.Wait()

			assert.Equal(t, 1, wins)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("対象スロットが存在しなければ false（先取り/消失）", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/a", time.Unix(1000, 0))
			assert.False(t, r.TryReclaim(7))
		})
	})
}

func TestRegistry_Release(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("自分の保持スロットのみ解放できる", func(t *testing.T) {
			t.Parallel()

			now := time.Unix(1000, 0)
			r := newTestRegistry(t, "/w/self", now)
			require.True(t, r.TryAcquireFresh(1))
			require.NoError(t, r.WriteMeta(1))

			// 別 owner からは解放されない。
			other := NewRegistry(r.dir, "/w/other", "b", 30*time.Minute, 8, func() time.Time { return now })
			assert.False(t, other.Release(1))
			assert.True(t, r.Exists(1))

			// 自分なら解放される。
			assert.True(t, r.Release(1))
			assert.False(t, r.Exists(1))
		})
	})
}

func TestRegistry_Exists(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("取得済みスロットは true、未取得スロットは false", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/a", time.Unix(1000, 0))
			require.True(t, r.TryAcquireFresh(1))

			assert.True(t, r.Exists(1))
			assert.False(t, r.Exists(2))
		})

		t.Run("解放後は false になる", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/a", time.Unix(1000, 0))
			require.True(t, r.TryAcquireFresh(1))
			require.NoError(t, r.WriteMeta(1))
			require.True(t, r.Release(1))

			assert.False(t, r.Exists(1))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロックと同名のファイルはディレクトリでないため false", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/a", time.Unix(1000, 0))
			require.NoError(t, os.WriteFile(r.lockDir(1), []byte("not a dir"), 0o600))

			assert.False(t, r.Exists(1))
		})
	})
}

func TestRegistry_MaxSlots(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時に渡したスロット数を返す", func(t *testing.T) {
			t.Parallel()

			r := NewRegistry("/pool", "/w/a", "b", time.Minute, 4, nil)

			assert.Equal(t, 4, r.MaxSlots())
		})
	})
}

func TestRegistry_OwnedBySelf(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("自分が meta を書いたスロットは true", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/self", time.Unix(1000, 0))
			require.True(t, r.TryAcquireFresh(1))
			require.NoError(t, r.WriteMeta(1))

			assert.True(t, r.OwnedBySelf(1))
		})

		t.Run("別 worktree が meta を書いたスロットは false", func(t *testing.T) {
			t.Parallel()

			now := time.Unix(1000, 0)
			owner := newTestRegistry(t, "/w/other", now)
			require.True(t, owner.TryAcquireFresh(1))
			require.NoError(t, owner.WriteMeta(1))

			viewer := NewRegistry(owner.dir, "/w/me", "b", 30*time.Minute, 8, func() time.Time { return now })
			assert.False(t, viewer.OwnedBySelf(1))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("meta が無いスロットは false", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/self", time.Unix(1000, 0))
			require.True(t, r.TryAcquireFresh(1))

			assert.False(t, r.OwnedBySelf(1))
		})
	})
}

func TestRegistry_HeldByOther(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("別 worktree が meta を書いたスロットは true", func(t *testing.T) {
			t.Parallel()

			now := time.Unix(1000, 0)
			owner := newTestRegistry(t, "/w/other", now)
			require.True(t, owner.TryAcquireFresh(1))
			require.NoError(t, owner.WriteMeta(1))

			viewer := NewRegistry(owner.dir, "/w/me", "b", 30*time.Minute, 8, func() time.Time { return now })
			assert.True(t, viewer.HeldByOther(1))
		})

		t.Run("自分が meta を書いたスロットは false", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/self", time.Unix(1000, 0))
			require.True(t, r.TryAcquireFresh(1))
			require.NoError(t, r.WriteMeta(1))

			assert.False(t, r.HeldByOther(1))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("meta を読めないスロットは他 worktree の保持とみなさない", func(t *testing.T) {
			t.Parallel()

			r := newTestRegistry(t, "/w/self", time.Unix(1000, 0))
			require.True(t, r.TryAcquireFresh(1))

			assert.False(t, r.HeldByOther(1))
		})
	})
}

func TestRegistry_lockDir(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("プールディレクトリ直下の slot-<N>.lock を指す", func(t *testing.T) {
			t.Parallel()

			r := NewRegistry("/pool", "/w/a", "b", time.Minute, 8, nil)

			assert.Equal(t, "/pool/slot-3.lock", r.lockDir(3))
		})
	})
}

func TestRegistry_metaPath(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ロックディレクトリ内の meta を指す", func(t *testing.T) {
			t.Parallel()

			r := NewRegistry("/pool", "/w/a", "b", time.Minute, 8, nil)

			assert.Equal(t, "/pool/slot-3.lock/meta", r.metaPath(3))
		})
	})
}
