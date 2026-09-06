//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE
package dbslot

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	"go-boilerplate/pkg/xerrors"
)

// appProfile は、app 層のサービス（api_server / mock_auth_server）が属する compose の profile です。
// 値は .makefiles/docker/compose.mk の COMPOSE_APP と一致していなければなりません
// （渡さない down が空振りする理由は README.md「Compose」）。
const appProfile = "development"

// Compose は、docker compose 操作を抽象化します（テストでフェイク可能）。
type Compose interface {
	// UpSharedDB は、共有 DB コンテナを固定プロジェクトで起動し healthcheck 完了まで待ちます。
	UpSharedDB(ctx context.Context, project string) error
	// DownServe は、per-worktree の serve プロジェクト（app コンテナ）を停止・削除します。
	DownServe(ctx context.Context, project string) error
	// RunningContainers は、指定プロジェクトで稼働中のコンテナ数を返します。
	RunningContainers(ctx context.Context, project string) (int, error)
}

// ExecCompose は、docker compose をホストで実行する Compose 実装です。
// 出力（進捗ログ）は stderr へ流します。
type ExecCompose struct{}

// UpSharedDB は `docker compose --profile database up -d --wait --no-recreate database` を実行します。
// --no-recreate を外すと、他の checkout が使っている稼働中コンテナを作り直します
// （経緯と config-hash の仕組みは docs/maintenance/db-worktree-pool.md）。
func (c ExecCompose) UpSharedDB(ctx context.Context, project string) error {
	return c.run(ctx, project, "--profile", "database", "up", "-d", "--wait", "--no-recreate", "database")
}

// DownServe は、app 層の compose プロジェクトを down します。
// docker compose down はコンテナ不在でも exit 0 のため、返るエラーは docker 未起動・権限不足などの実失敗のみです。
// app 層の override（docker-compose.attach.yaml）は重ねません（理由は README.md「Compose」）。
func (c ExecCompose) DownServe(ctx context.Context, project string) error {
	return c.run(ctx, project, "--profile", appProfile, "-f", "docker-compose.yaml", "down")
}

// RunningContainers は `docker compose ps -q --status running` の出力行数を返します。
// プロジェクトが存在しない場合は 0 を返します（未 serve と稼働中を区別できれば足りるため）。
func (c ExecCompose) RunningContainers(ctx context.Context, project string) (int, error) {
	out, err := c.output(ctx, project, "ps", "-q", "--status", "running")
	if err != nil {
		return 0, err
	}

	return len(strings.Fields(out)), nil
}

func (ExecCompose) run(ctx context.Context, project string, args ...string) error {
	if err := newComposeCmd(ctx, project, os.Stderr, args...).Run(); err != nil {
		return xerrors.Wrap(err, "docker compose failed")
	}
	return nil
}

func (ExecCompose) output(ctx context.Context, project string, args ...string) (string, error) {
	var buf bytes.Buffer
	if err := newComposeCmd(ctx, project, &buf, args...).Run(); err != nil {
		return "", xerrors.Wrap(err, "docker compose failed")
	}
	return buf.String(), nil
}

// newComposeCmd は、固定プロジェクトを環境変数で与えた docker compose コマンドを組み立てます。
func newComposeCmd(ctx context.Context, project string, stdout io.Writer, args ...string) *exec.Cmd {
	// 引数は本パッケージ内の固定文字列のみ（ユーザー入力を渡さない）。
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...) //nolint:gosec // 固定引数
	cmd.Env = append(os.Environ(), "COMPOSE_PROJECT_NAME="+project)
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr

	return cmd
}
