package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go-boilerplate/internal/cli/dbslot"

	"github.com/spf13/cobra"
)

const (
	defaultPoolMaxSlots      = 12
	defaultPoolTTLSeconds    = 1800
	defaultPoolAPIBasePort   = 8080
	defaultPoolMockBasePort  = 2010
	defaultPoolDlvBasePort   = 2345
	defaultPoolPprofBasePort = 6060
	defaultPoolPGPort        = 5432
)

// newDBSlotCommand は、worktree 並列開発用の DB スロットプールを操作するコマンドを生成します。
// 詳細は docs/maintenance/db-worktree-pool.md。
func newDBSlotCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db-slot",
		Short: "worktree 並列開発用の DB スロットプールを操作します（acquire/release/heartbeat/status）。",
	}
	cmd.AddCommand(
		newDBSlotSubCommand("acquire", "空きスロットをリースし共有 DB に自 worktree の DB を用意します。",
			func(ctx context.Context, p *dbslot.Pool) error { return p.Acquire(ctx) }),
		newDBSlotSubCommand("release", "保持中のスロットを解放します（serve コンテナ停止・DB は warm 保持）。",
			func(ctx context.Context, p *dbslot.Pool) error { return p.Release(ctx) }),
		newDBSlotSubCommand("heartbeat", "保持中スロットの heartbeat を更新します。",
			func(_ context.Context, p *dbslot.Pool) error { return p.Heartbeat() }),
		newDBSlotStatusCommand(),
		// env の標準出力は make が eval するため出力先を分ける。require-owner の案内は stderr。
		newDBSlotResolverCommand("env", "スロットから導かれる値を KEY=VALUE で出力します（make が eval します）。",
			os.Stdout, func(ctx context.Context, r *dbslot.Resolver) error { return r.PrintEnv(ctx) }),
		newDBSlotResolverCommand("require-owner", "自分が所有するデータベースがあることを検証します。",
			os.Stderr, func(ctx context.Context, r *dbslot.Resolver) error { return r.RequireOwner(ctx) }),
	)
	return cmd
}

// newDBSlotStatusCommand は、スロットの占有状況に続けて自 checkout の解決済みの値を表示します。
func newDBSlotStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "スロットの占有状況と、自 checkout の解決済みの値を表示します。",
		RunE: func(c *cobra.Command, _ []string) error {
			pool, err := newSlotPool()
			if err != nil {
				return err
			}
			if err := pool.Status(); err != nil {
				return err
			}
			resolver, err := newSlotResolver(os.Stdout)
			if err != nil {
				return err
			}
			return resolver.PrintValues(c.Context())
		},
	}
}

func newDBSlotResolverCommand(use, short string, out io.Writer, run func(context.Context, *dbslot.Resolver) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(c *cobra.Command, _ []string) error {
			resolver, err := newSlotResolver(out)
			if err != nil {
				return err
			}
			return run(c.Context(), resolver)
		},
	}
}

func newDBSlotSubCommand(use, short string, run func(context.Context, *dbslot.Pool) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(c *cobra.Command, _ []string) error {
			pool, err := newSlotPool()
			if err != nil {
				return err
			}
			return run(c.Context(), pool)
		},
	}
}

// newSlotPool は、実依存（ホスト実行の registry / pgx admin / docker compose）を配線して Pool を生成します。
func newSlotPool() (*dbslot.Pool, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	reg := dbslot.NewRegistry(
		poolDir(),
		root,
		gitBranch(),
		time.Duration(envInt("GOBP_DB_POOL_TTL", defaultPoolTTLSeconds))*time.Second,
		envInt("GOBP_DB_POOL_MAX", defaultPoolMaxSlots),
		nil,
	)
	admin := dbslot.NewPgxAdmin(
		envStr("GOBP_DB_POOL_PGHOST", "localhost"),
		envInt("GOBP_DB_POOL_PGPORT", defaultPoolPGPort),
		envStr("GOBP_DB_POOL_PGUSER", "postgres"),
		envStr("GOBP_DB_POOL_PGPASSWORD", "postgres-password"),
		envStr("GOBP_DB_POOL_PGMAINTDB", "postgres"),
	)
	cfg, err := slotConfig(root)
	if err != nil {
		return nil, err
	}

	return dbslot.NewPool(reg, admin, dbslot.ExecCompose{}, cfg, os.Stdout, os.Stderr), nil
}

// newSlotResolver は、スロットから導かれる値の解決器を実依存（ホストの git）で配線して生成します。
func newSlotResolver(out io.Writer) (*dbslot.Resolver, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg, err := slotConfig(root)
	if err != nil {
		return nil, err
	}

	return dbslot.NewResolver(cfg, nil, out), nil
}

func slotConfig(root string) (dbslot.Config, error) {
	cfg := dbslot.Config{
		Root:          root,
		SharedProject: envStr("GOBP_DB_SHARED_PROJECT", "gobp-shared"),
		APIBasePort:   envInt("GOBP_API_POOL_BASE", defaultPoolAPIBasePort),
		MockAuthBase:  envInt("GOBP_MOCK_AUTH_POOL_BASE", defaultPoolMockBasePort),
		DlvBase:       envInt("GOBP_DLV_POOL_BASE", defaultPoolDlvBasePort),
		PprofBase:     envInt("GOBP_PPROF_POOL_BASE", defaultPoolPprofBasePort),
		APPEnv:        os.Getenv("APP_ENV"),
	}

	base, err := dbslot.LoadRealtimeBase()
	if err != nil {
		return dbslot.Config{}, err
	}

	cfg.Realtime = base

	return cfg, nil
}

func poolDir() string {
	if v := os.Getenv("GOBP_DB_POOL_DIR"); v != "" {
		return v
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		home, _ := os.UserHomeDir()
		cache = home + "/.cache"
	}
	return cache + "/gobp-db-pool"
}

func gitBranch() string {
	out, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "-"
	}
	return strings.TrimSpace(string(out))
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
