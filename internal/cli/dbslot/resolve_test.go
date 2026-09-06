package dbslot

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-boilerplate/pkg/xerrors"
)

// slotFileContent は、スロット 3 を保持した状態の .gobp-db-slot です。
const slotFileContent = "SLOT=3\nDB_NAME_LOCAL=wt3_local\nDB_NAME_TEST=wt3_test\n" +
	"API_HOST_PORT=8083\nMOCK_AUTH_HOST_PORT=2013\nCOMPOSE_PROJECT_NAME=gobp-shared\nSERVE_PROJECT=gobp-wt-3\n"

// errGitFailed は、git コマンドが失敗したことを表すテスト用のエラー。
var errGitFailed = xerrors.New("exit status 128")

// probeStub は、git の 4 通りの応答（不在 / 非リポジトリ / 主 checkout / リンク worktree）を作ります。
type probeStub struct {
	lookErr     error
	dirs        string
	dirsErr     error
	hasGitEntry bool
}

func (s probeStub) probe() *GitProbe {
	return &GitProbe{
		LookGit:     func() error { return s.lookErr },
		GitDirs:     func(context.Context, string) (string, error) { return s.dirs, s.dirsErr },
		HasGitEntry: func(string) bool { return s.hasGitEntry },
	}
}

// newResolver は、宣言どおりのスロットをリースしている Resolver と出力バッファを返します。
func newResolver(t *testing.T, root string, stub probeStub) (*Resolver, *bytes.Buffer) {
	t.Helper()

	return newResolverWithLease(t, root, stub, &LeaseProbe{OwnedBySelf: func(int) bool { return true }})
}

// newResolverWithLease は、リース所有権の判定点を差し替えた Resolver と出力バッファを返します。
func newResolverWithLease(t *testing.T, root string, stub probeStub, lease *LeaseProbe) (*Resolver, *bytes.Buffer) {
	t.Helper()

	var out bytes.Buffer

	cfg := Config{
		Root: root, SharedProject: "gobp-shared", APIBasePort: 8080, MockAuthBase: 2010,
		Realtime: RealtimeBase{
			TableSuffix: "local",
			QueuePrefix: "realtime-local",
			Topic:       "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local",
		},
	}

	return NewResolver(cfg, stub.probe(), lease, &out), &out
}

// writeSlot は、root に .gobp-db-slot を書き出します。
func writeSlot(t *testing.T, root, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(root, ".gobp-db-slot"), []byte(content), filePerm))
}

func TestResolver_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("git-dir と git-common-dir が一致すれば主 checkout とみなす", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirs: ".git\n.git\n"})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitMainCheckout, got.Git)
		})

		t.Run("相対パスと絶対パスで返っても同一なら主 checkout とみなす", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			r, _ := newResolver(t, root, probeStub{dirs: ".git\n" + filepath.Join(root, ".git") + "\n"})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitMainCheckout, got.Git)
		})

		t.Run("git-dir と git-common-dir が食い違えばリンク worktree とみなす", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirs: "/repo/.git/worktrees/wt1\n/repo/.git\n"})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitLinkedWorktree, got.Git)
			assert.Equal(t, "--no-recreate", got.InfraNoRecreate)
		})

		t.Run("git 実行ファイルが無ければ判定せず素通りさせる", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{lookErr: errGitFailed, hasGitEntry: true})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitUnavailable, got.Git)
			assert.Empty(t, got.InfraNoRecreate)
		})

		t.Run("git はあるがリポジトリでなければ素通りさせる", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirsErr: errGitFailed, hasGitEntry: false})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitNoRepository, got.Git)
		})

		t.Run("スロット未取得なら既定の local / test を所有データベースとする", func(t *testing.T) {
			t.Parallel()

			root := filepath.Join(t.TempDir(), "go-boilerplate")
			require.NoError(t, os.Mkdir(root, dirPerm))
			r, _ := newResolver(t, root, probeStub{dirs: ".git\n.git\n"})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.False(t, got.SlotHeld)
			assert.Equal(t, "local", got.DBLocal)
			assert.Equal(t, "test", got.DBTest)
			assert.Equal(t, "gobp-app-go-boilerplate", got.AppProject)
			assert.Equal(t, "http://localhost:2010/default", got.AuthIssuer)
			assert.Equal(t, "local", got.RealtimeTableSuffix, "スロットが無ければ埋め込み env の基底名のまま")
			assert.Equal(t, "realtime-local", got.RealtimeQueuePrefix)
			assert.Equal(t, "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local", got.RealtimeTopic)
		})

		t.Run("スロット取得済みならスロットの値を所有データベース・プロジェクトとする", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)
			r, _ := newResolver(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.True(t, got.SlotHeld)
			assert.Equal(t, "wt3_local", got.DBLocal)
			assert.Equal(t, "wt3_test", got.DBTest)
			assert.Equal(t, "gobp-wt-3", got.AppProject)
			assert.Equal(t, "http://localhost:2013/default", got.AuthIssuer)
		})

		t.Run("値はスロット番号から導き、スロット定義が並べる DB 名やポートは採用しない", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, "SLOT=3\nDB_NAME_LOCAL=wt9_local\nDB_NAME_TEST=wt9_test\n"+
				"MOCK_AUTH_HOST_PORT=2019\nSERVE_PROJECT=gobp-wt-9\n")
			r, _ := newResolver(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.True(t, got.SlotHeld)
			assert.Equal(t, "wt3_local", got.DBLocal)
			assert.Equal(t, "wt3_test", got.DBTest)
			assert.Equal(t, "gobp-wt-3", got.AppProject)
			assert.Equal(t, "http://localhost:2013/default", got.AuthIssuer)
		})

		t.Run("Realtime の資源名をスロットごとの名前空間へ落とす", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)
			r, _ := newResolver(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, "local_wt3", got.RealtimeTableSuffix, "table の suffix だけ区切りが `_`")
			assert.Equal(t, "realtime-local-wt3", got.RealtimeQueuePrefix)
			assert.Equal(t, "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local-wt3", got.RealtimeTopic)
		})

		t.Run("リースを他 worktree に回収されていればスロット定義を無視して既定へ落とす", func(t *testing.T) {
			t.Parallel()

			root := filepath.Join(t.TempDir(), "go-boilerplate")
			require.NoError(t, os.Mkdir(root, dirPerm))
			writeSlot(t, root, slotFileContent)
			r, _ := newResolverWithLease(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"},
				&LeaseProbe{OwnedBySelf: func(int) bool { return false }})

			got, err := r.Resolve(t.Context())
			require.NoError(t, err)
			assert.False(t, got.SlotHeld)
			assert.Equal(t, "local", got.DBLocal)
			assert.Equal(t, "test", got.DBTest)
			assert.Equal(t, "gobp-app-go-boilerplate", got.AppProject)
			assert.Equal(t, "http://localhost:2010/default", got.AuthIssuer)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("リポジトリなのに git が失敗したら判定不能として止める", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirsErr: errGitFailed, hasGitEntry: true})

			_, err := r.Resolve(t.Context())
			require.ErrorIs(t, err, errGitLayoutUnreadable)
		})

		t.Run("git の出力が想定外の形なら判定不能として止める", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirs: ".git\n"})

			_, err := r.Resolve(t.Context())
			require.ErrorIs(t, err, errGitLayoutUnreadable)
		})
	})
}

func TestResolver_RequireOwner(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("主 checkout は所有者として素通りさせる", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirs: ".git\n.git\n"})

			require.NoError(t, r.RequireOwner(t.Context()))
		})

		t.Run("ツールランナー内（git 不在）は素通りさせる", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{lookErr: errGitFailed, hasGitEntry: true})

			require.NoError(t, r.RequireOwner(t.Context()))
		})

		t.Run("スロット取得済みのリンク worktree は素通りさせる", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)
			r, _ := newResolver(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			require.NoError(t, r.RequireOwner(t.Context()))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("スロット未取得のリンク worktree は取得方法を案内して失敗させる", func(t *testing.T) {
			t.Parallel()

			r, out := newResolver(t, t.TempDir(), probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			require.ErrorIs(t, r.RequireOwner(t.Context()), errNoDatabaseOwner)
			assert.Contains(t, out.String(), "所有するデータベースがありません")
			assert.Contains(t, out.String(), "make slot-acquire")
		})

		t.Run("回収済みのスロット定義が残っていても所有者とみなさない", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)
			r, out := newResolverWithLease(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"},
				&LeaseProbe{OwnedBySelf: func(int) bool { return false }})

			require.ErrorIs(t, r.RequireOwner(t.Context()), errNoDatabaseOwner)
			assert.Contains(t, out.String(), "回収されている")
		})

		t.Run("リースの判定点が無ければ所有者とみなさない", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)
			r, _ := newResolverWithLease(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"}, nil)

			require.ErrorIs(t, r.RequireOwner(t.Context()), errNoDatabaseOwner)
		})

		t.Run("スロット定義に DB 名が無ければ所有者とみなさない", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, "SLOT=3\n")
			r, out := newResolver(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			require.ErrorIs(t, r.RequireOwner(t.Context()), errNoDatabaseOwner)
			assert.Contains(t, out.String(), "make slot-acquire")
		})

		t.Run("リポジトリなのに git が失敗したら素通りさせず止める", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirsErr: errGitFailed, hasGitEntry: true})

			require.ErrorIs(t, r.RequireOwner(t.Context()), errGitLayoutUnreadable)
		})
	})
}

func TestResolver_PrintEnv(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("make が eval できる KEY=VALUE を出力する", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)
			r, out := newResolver(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			require.NoError(t, r.PrintEnv(t.Context()))
			assert.Contains(t, out.String(), "DB_LOCAL='wt3_local'\n")
			assert.Contains(t, out.String(), "DB_TEST='wt3_test'\n")
			assert.Contains(t, out.String(), "APP_PROJECT='gobp-wt-3'\n")
			assert.Contains(t, out.String(), "AUTH_ISSUER='http://localhost:2013/default'\n")
			assert.Contains(t, out.String(), "INFRA_NO_RECREATE='--no-recreate'\n")
			assert.Contains(t, out.String(), "GOBP_GIT_CONTEXT='linked-worktree'\n")
		})

		t.Run("単一 checkout では --no-recreate を渡さない", func(t *testing.T) {
			t.Parallel()

			r, out := newResolver(t, t.TempDir(), probeStub{dirs: ".git\n.git\n"})

			require.NoError(t, r.PrintEnv(t.Context()))
			assert.Contains(t, out.String(), "INFRA_NO_RECREATE=''\n")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("判定不能なら何も出力せず失敗させる", func(t *testing.T) {
			t.Parallel()

			r, out := newResolver(t, t.TempDir(), probeStub{dirsErr: errGitFailed, hasGitEntry: true})

			require.ErrorIs(t, r.PrintEnv(t.Context()), errGitLayoutUnreadable)
			assert.Empty(t, out.String())
		})
	})
}

func TestResolver_PrintValues(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("どのデータベースとプロジェクトを叩くのかを表示する", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)
			r, out := newResolver(t, root, probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			require.NoError(t, r.PrintValues(t.Context()))
			assert.Contains(t, out.String(), "git context       : linked-worktree")
			assert.Contains(t, out.String(), "slot held         : yes")
			assert.Contains(t, out.String(), "DB_LOCAL          : wt3_local")
			assert.Contains(t, out.String(), "APP_PROJECT       : gobp-wt-3")
			assert.Contains(t, out.String(), "INFRA_NO_RECREATE : --no-recreate")
		})

		t.Run("渡さない値は空欄にせず明示する", func(t *testing.T) {
			t.Parallel()

			r, out := newResolver(t, t.TempDir(), probeStub{dirs: ".git\n.git\n"})

			require.NoError(t, r.PrintValues(t.Context()))
			assert.Contains(t, out.String(), "INFRA_NO_RECREATE : （渡さない）")
			assert.Contains(t, out.String(), "slot held         : no")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("判定不能なら何も出力せず失敗させる", func(t *testing.T) {
			t.Parallel()

			r, out := newResolver(t, t.TempDir(), probeStub{dirsErr: errGitFailed, hasGitEntry: true})

			require.ErrorIs(t, r.PrintValues(t.Context()), errGitLayoutUnreadable)
			assert.Empty(t, out.String())
		})
	})
}

func TestNewResolver(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("probe を渡さなければホストの git を使う実依存で組み立てる", func(t *testing.T) {
			t.Parallel()

			r := NewResolver(Config{Root: t.TempDir()}, nil, &LeaseProbe{OwnedBySelf: func(int) bool { return true }}, io.Discard)

			assert.NotNil(t, r.probe.LookGit)
			assert.NotNil(t, r.probe.GitDirs)
			assert.NotNil(t, r.probe.HasGitEntry)
		})
	})
}

func Test_hasGitEntry(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("祖先に .git があれば真を返す", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), dirPerm))
			nested := filepath.Join(root, "a", "b")
			require.NoError(t, os.MkdirAll(nested, dirPerm))

			assert.True(t, hasGitEntry(nested))
		})

		t.Run("リンク worktree の .git はファイルでも真を返す", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /repo/.git/worktrees/wt1\n"), filePerm))

			assert.True(t, hasGitEntry(root))
		})

		t.Run("祖先に .git が無ければ偽を返す", func(t *testing.T) {
			t.Parallel()

			// t.TempDir() は /tmp 配下（git 管理外）なので、根まで遡っても .git は見つからない。
			assert.False(t, hasGitEntry(t.TempDir()))
		})
	})
}

func TestGitContext_String(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("git 不在は git-unavailable を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "git-unavailable", GitUnavailable.String())
		})

		t.Run("非リポジトリは no-repository を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "no-repository", GitNoRepository.String())
		})

		t.Run("主 checkout は main-checkout を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "main-checkout", GitMainCheckout.String())
		})

		t.Run("リンク worktree は linked-worktree を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "linked-worktree", GitLinkedWorktree.String())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("未定義の値は unknown へ落とし、数値を露出させない", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "unknown", GitContext(99).String())
		})
	})
}

func TestRenderEnv(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("make が eval できる KEY='VALUE' を 1 行 1 変数で並べる", func(t *testing.T) {
			t.Parallel()

			got := RenderEnv(Values{
				Git:                 GitLinkedWorktree,
				DBLocal:             "wt3_local",
				DBTest:              "wt3_test",
				AppProject:          "gobp-wt-3",
				AuthIssuer:          "http://localhost:2013/default",
				InfraNoRecreate:     noRecreateFlag,
				RealtimeTableSuffix: "local_wt3",
				RealtimeQueuePrefix: "realtime-local-wt3",
				RealtimeTopic:       "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local-wt3",
			})

			assert.Equal(t, "GOBP_GIT_CONTEXT='linked-worktree'\n"+
				"DB_LOCAL='wt3_local'\n"+
				"DB_TEST='wt3_test'\n"+
				"APP_PROJECT='gobp-wt-3'\n"+
				"AUTH_ISSUER='http://localhost:2013/default'\n"+
				"INFRA_NO_RECREATE='--no-recreate'\n"+
				"REALTIME_TABLE_SUFFIX='local_wt3'\n"+
				"REALTIME_QUEUE_PREFIX='realtime-local-wt3'\n"+
				"REALTIME_TOPIC='arn:aws:sns:us-east-1:000000000000:realtime-fanout-local-wt3'\n", got)
		})

		t.Run("空の値もキーごと出力し、変数の欠落にしない", func(t *testing.T) {
			t.Parallel()

			got := RenderEnv(Values{Git: GitMainCheckout})

			assert.Contains(t, got, "INFRA_NO_RECREATE=''\n")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値に含まれる単引用符をエスケープし、eval を壊させない", func(t *testing.T) {
			t.Parallel()

			got := RenderEnv(Values{DBLocal: "it's"})

			assert.Contains(t, got, `DB_LOCAL='it'\''s'`)
		})
	})
}

func TestRenderValues(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("解決済みの値を項目ごとに並べる", func(t *testing.T) {
			t.Parallel()

			got := RenderValues(Values{
				Git:                 GitLinkedWorktree,
				SlotHeld:            true,
				DBLocal:             "wt3_local",
				DBTest:              "wt3_test",
				AppProject:          "gobp-wt-3",
				AuthIssuer:          "http://localhost:2013/default",
				InfraNoRecreate:     noRecreateFlag,
				RealtimeTableSuffix: "local_wt3",
				RealtimeQueuePrefix: "realtime-local-wt3",
				RealtimeTopic:       "arn:aws:sns:us-east-1:000000000000:realtime-fanout-local-wt3",
			})

			assert.Contains(t, got, "git context       : linked-worktree")
			assert.Contains(t, got, "slot held         : yes")
			assert.Contains(t, got, "DB_LOCAL          : wt3_local")
			assert.Contains(t, got, "DB_TEST           : wt3_test")
			assert.Contains(t, got, "APP_PROJECT       : gobp-wt-3")
			assert.Contains(t, got, "AUTH_ISSUER       : http://localhost:2013/default")
			assert.Contains(t, got, "INFRA_NO_RECREATE : --no-recreate")
			assert.Contains(t, got, "REALTIME_TABLE    : local_wt3")
			assert.Contains(t, got, "REALTIME_QUEUE    : realtime-local-wt3")
			assert.Contains(t, got, "REALTIME_TOPIC    : arn:aws:sns:us-east-1:000000000000:realtime-fanout-local-wt3")
		})

		t.Run("渡さないフラグは空欄ではなく明示の文言で示す", func(t *testing.T) {
			t.Parallel()

			got := RenderValues(Values{Git: GitMainCheckout})

			assert.Contains(t, got, "INFRA_NO_RECREATE : （渡さない）")
			assert.Contains(t, got, "slot held         : no")
		})
	})
}

func TestResolver_absDir(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("相対パスは root 起点の絶対パスへ揃える", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			r, _ := newResolver(t, root, probeStub{})

			assert.Equal(t, filepath.Join(root, ".git"), r.absDir(".git"))
		})

		t.Run("絶対パスはそのまま扱う", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{})

			assert.Equal(t, filepath.FromSlash("/repo/.git"), r.absDir(filepath.FromSlash("/repo/.git")))
		})

		t.Run("同じ場所を指す表記揺れを同一の値へ畳む", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			r, _ := newResolver(t, root, probeStub{})

			assert.Equal(t, r.absDir(".git"), r.absDir("./a/../.git"))
		})
	})
}

func TestResolver_inspectGit(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("git 実行ファイルが無ければ GitUnavailable を返し失敗させない", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{lookErr: errGitFailed, hasGitEntry: true})

			got, err := r.inspectGit(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitUnavailable, got)
		})

		t.Run("git が失敗し .git も無ければ GitNoRepository を返し失敗させない", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirsErr: errGitFailed, hasGitEntry: false})

			got, err := r.inspectGit(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitNoRepository, got)
		})

		t.Run("git-dir と git-common-dir が一致すれば主 checkout と判定する", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirs: ".git\n.git\n"})

			got, err := r.inspectGit(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitMainCheckout, got)
		})

		t.Run("git-dir と git-common-dir が食い違えばリンク worktree と判定する", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirs: "/repo/.git/worktrees/wt3\n/repo/.git\n"})

			got, err := r.inspectGit(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitLinkedWorktree, got)
		})

		t.Run("相対と絶対が混ざっても同一箇所なら主 checkout と判定する", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			r, _ := newResolver(t, root, probeStub{dirs: ".git\n" + filepath.Join(root, ".git") + "\n"})

			got, err := r.inspectGit(t.Context())
			require.NoError(t, err)
			assert.Equal(t, GitMainCheckout, got)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("git が失敗しても .git があれば素通りさせず失敗させる", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirsErr: errGitFailed, hasGitEntry: true})

			_, err := r.inspectGit(t.Context())
			require.ErrorIs(t, err, errGitLayoutUnreadable)
		})

		t.Run("git の出力が 2 フィールドでなければ失敗させる", func(t *testing.T) {
			t.Parallel()

			r, _ := newResolver(t, t.TempDir(), probeStub{dirs: ".git\n"})

			_, err := r.inspectGit(t.Context())
			require.ErrorIs(t, err, errGitLayoutUnreadable)
		})
	})
}

func Test_realGitProbe(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("3 つの依存をすべて結線して返す", func(t *testing.T) {
			t.Parallel()

			p := realGitProbe()

			assert.NotNil(t, p.LookGit)
			assert.NotNil(t, p.GitDirs)
			assert.NotNil(t, p.HasGitEntry)
		})

		t.Run("HasGitEntry は .git の実在を見る実装を指す", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			p := realGitProbe()

			assert.False(t, p.HasGitEntry(root))
			require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), dirPerm))
			assert.True(t, p.HasGitEntry(root))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("GitDirs は非リポジトリでエラーを返す", func(t *testing.T) {
			t.Parallel()

			_, err := realGitProbe().GitDirs(t.Context(), t.TempDir())
			require.Error(t, err)
		})
	})
}

func Test_readSlotFile(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("KEY=VALUE を読み取る", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, slotFileContent)

			got := readSlotFile(filepath.Join(root, ".gobp-db-slot"))

			assert.Equal(t, "wt3_local", got["DB_NAME_LOCAL"])
			assert.Equal(t, "wt3_test", got["DB_NAME_TEST"])
			assert.Equal(t, "gobp-wt-3", got["SERVE_PROJECT"])
			assert.Equal(t, "2013", got["MOCK_AUTH_HOST_PORT"])
		})

		t.Run("値に含まれる = は最初の 1 つだけで分割する", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, "AUTH_ISSUER=http://localhost:2013/?a=b\n")

			got := readSlotFile(filepath.Join(root, ".gobp-db-slot"))

			assert.Equal(t, "http://localhost:2013/?a=b", got["AUTH_ISSUER"])
		})

		t.Run("= を含まない行は読み飛ばす", func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeSlot(t, root, "# comment\n\nSLOT=3\n")

			got := readSlotFile(filepath.Join(root, ".gobp-db-slot"))

			assert.Len(t, got, 1)
			assert.Equal(t, "3", got["SLOT"])
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("ファイルが無ければ空の map を返し、スロット未取得として扱わせる", func(t *testing.T) {
			t.Parallel()

			got := readSlotFile(filepath.Join(t.TempDir(), ".gobp-db-slot"))

			assert.Empty(t, got)
			assert.NotNil(t, got)
		})
	})
}

func Test_realtimeName(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("スロットがあれば区切りとスロット番号を継ぐ", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "local_wt3", realtimeName("local", "3", slotInfixTable))
			assert.Equal(t, "realtime-local-wt3", realtimeName("realtime-local", "3", slotInfixName))
		})

		t.Run("スロットが無ければ基底名のままにする", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "local", realtimeName("local", "", slotInfixTable),
				"スロット未取得の checkout は埋め込み env の名前をそのまま使う")
		})

		t.Run("基底が空ならスロット番号を継がない", func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, realtimeName("", "3", slotInfixName),
				"空の topic は env の契約。継ぐと `-wt3` という契約でも名前でもない値になる")
		})
	})
}

func Test_orDefault(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("値があればその値を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "wt3_local", orDefault("wt3_local", defaultDBLocal))
		})

		t.Run("空なら既定値を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, defaultDBLocal, orDefault("", defaultDBLocal))
		})
	})
}

func Test_yesNo(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("真は yes を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "yes", yesNo(true))
		})

		t.Run("偽は no を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "no", yesNo(false))
		})
	})
}
