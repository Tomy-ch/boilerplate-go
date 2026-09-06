//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE
package dbslot

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"go-boilerplate/pkg/xerrors"

	"github.com/jackc/pgx/v5"
)

// DBAdmin は、共有 DB に対する管理操作を抽象化します（テストでフェイク可能）。
type DBAdmin interface {
	// EnsureDatabase は、データベースが無ければ作成します（冪等）。
	EnsureDatabase(ctx context.Context, name string) error
	// SetupDatabase は、init スクリプトが local/test に施すのと同じ拡張を対象 DB に設定します。
	SetupDatabase(ctx context.Context, name string) error
	// ActiveConnections は、指定データベース群への稼働中接続数を返します。
	ActiveConnections(ctx context.Context, names ...string) (int, error)
}

// PgxAdmin は、pgx でホストから共有 DB へ接続する DBAdmin 実装です。
// CREATE DATABASE / pg_stat_activity は maintenance DB へ、拡張は対象 DB へ接続します。
type PgxAdmin struct {
	host, user, password, maintenanceDB string
	port                                int
}

// NewPgxAdmin は PgxAdmin を生成します。
func NewPgxAdmin(host string, port int, user, password, maintenanceDB string) *PgxAdmin {
	return &PgxAdmin{host: host, port: port, user: user, password: password, maintenanceDB: maintenanceDB}
}

// EnsureDatabase は、対象 DB が無ければ CREATE DATABASE します。
func (a *PgxAdmin) EnsureDatabase(ctx context.Context, name string) error {
	conn, err := a.connect(ctx, a.maintenanceDB)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	var exists bool
	if err := conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", name).Scan(&exists); err != nil {
		return xerrors.Wrap(err, "failed to check database existence")
	}
	if exists {
		return nil
	}
	// CREATE DATABASE は識別子をパラメータ化できないため安全に引用する。
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return xerrors.Wrap(err, "failed to create database "+name)
	}
	return nil
}

// SetupDatabase は、対象 DB に pg_trgm 拡張を設定します
// （timezone を設定しない理由は README.md「DB admin」）。
func (a *PgxAdmin) SetupDatabase(ctx context.Context, name string) error {
	conn, err := a.connect(ctx, name)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_trgm"); err != nil {
		return xerrors.Wrap(err, "failed to create extension on "+name)
	}
	return nil
}

// ActiveConnections は、指定 DB 群への稼働中接続数を返します（stale 回収前の破壊防止ガードに使用）。
func (a *PgxAdmin) ActiveConnections(ctx context.Context, names ...string) (int, error) {
	if len(names) == 0 {
		return 0, nil
	}
	conn, err := a.connect(ctx, a.maintenanceDB)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close(ctx) }()

	var count int
	if err := conn.QueryRow(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE datname = ANY($1)", names).Scan(&count); err != nil {
		return 0, xerrors.Wrap(err, "failed to count active connections")
	}
	return count, nil
}

func (a *PgxAdmin) dsn(dbName string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		a.user, a.password, net.JoinHostPort(a.host, strconv.Itoa(a.port)), dbName)
}

func (a *PgxAdmin) connect(ctx context.Context, dbName string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, a.dsn(dbName))
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to connect to "+dbName)
	}
	return conn, nil
}
