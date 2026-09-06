// Package testkit は RDB のテスト用インスタンスを提供します。
package testkit

import (
	"context"
	"sync"
	"testing"

	"go-boilerplate/internal/config"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/system"
	"go-boilerplate/internal/logging"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/require"
)

// txAdvisoryLockKey は、テスト tx を DB 単位で全プロセス横断に直列化する advisory lock キー。
// go test はパッケージ毎に別プロセスで走り、プロセス内 txLock だけでは別プロセスの CASCADE TRUNCATE
// 同士が deadlock しうるため補う。
const txAdvisoryLockKey = 8_246_913

var errRollbackForTest = xerrors.New("rollback for test")

var (
	testDB  driver.DatabaseDriver
	errInit error
	dbOnce  sync.Once
	// txLock は、テスト用のトランザクションマネージャーでトランザクションを開始する際のロックです。
	// これにより、テストが並行して実行される場合でも、トランザクションの競合を防止します。
	txLock sync.Mutex
)

// TransactionRunner は、テスト用に関数をトランザクション内で実行するインターフェースです。
type TransactionRunner interface {
	WithinTx(fn func(ctx context.Context))
	WithinTxE(fn func(ctx context.Context) error)
}

// testTxRunner はテスト用のトランザクションランナーを表します。
type testTxRunner struct {
	inner tx.Manager
	db    driver.DatabaseDriver
	t     *testing.T
}

// NewTestDB は、テスト用の共有データベースドライバー（シングルトン）を取得します。
func NewTestDB(t *testing.T) driver.DatabaseDriver {
	t.Helper()
	return getTestDB(t)
}

// NewTestTransactionRunner は、テスト用のトランザクションランナーを生成します。
func NewTestTransactionRunner(t *testing.T) TransactionRunner {
	t.Helper()
	testLogger := logging.NewTestLogger(t)

	db := getTestDB(t)
	dbCfg := config.NewDatabaseConfig(config.MockConfigForTest(t))
	innerTxm := driver.NewTransactionManager(db, dbCfg, testLogger, system.NewSleeper())

	runner := &testTxRunner{
		inner: innerTxm,
		db:    db,
		t:     t,
	}

	return runner
}

// WithinTx は、テスト用のトランザクションマネージャーでトランザクションを開始し、引数で渡されたfnを実行し、最後にロールバックします。
//
// 使用例:
//
//	txm.WithinTx(func(ctx context.Context) {
//	  // トランザクション内の処理
//	})
func (t *testTxRunner) WithinTx(fn func(ctx context.Context)) {
	t.t.Helper()

	t.WithinTxE(func(ctx context.Context) error {
		fn(ctx)
		return nil
	})
}

// WithinTxE は、fn がエラーを返せる WithinTx です。fn が返したエラーはトランザクションマネージャーへ
// 伝わるため、deadlock（40P01）や serialization failure（40001）は tx ごと再試行されます。
//
// WithinTx との違いは、fn が失敗をどう表明するかだけです。fn の中で require を使うと testify は
// runtime.Goexit で goroutine を落とし、トランザクションマネージャーは戻り値を受け取れないため、
// リトライ可能と宣言済みのエラーであっても再試行されません。一時障害を再試行させたい文
// （テーブル全体をロックする CASCADE TRUNCATE など）は、require ではなくエラーを返してください。
//
// fn が nil を返した場合もトランザクションはロールバックされます（テストは DB を汚さない）。
func (t *testTxRunner) WithinTxE(fn func(ctx context.Context) error) {
	t.t.Helper()

	txLock.Lock()
	defer txLock.Unlock()

	baseCtx := context.Background()

	err := t.inner.Do(baseCtx, func(txCtx context.Context) error {
		if lockErr := lockSuiteSerialization(txCtx, driver.New(txCtx, t.db)); lockErr != nil {
			return lockErr
		}
		if fnErr := fn(txCtx); fnErr != nil {
			return fnErr
		}
		return errRollbackForTest
	})

	if xerrors.Is(err, errRollbackForTest) {
		return
	}
	require.NoError(t.t, err)
}

// HoldSuiteSerialization は、呼び出したテストが終わるまでスイート全体の直列化を占有します。
// 占有している間、他パッケージのテスト（別プロセス）が張る CASCADE TRUNCATE は走りません。
// 解放は t.Cleanup で行われます。
//
// 占有は専用のトランザクションで行い、テスト自身のトランザクションはこの直列化に参加しません。
// 参加させると自分の占有を待つことになり、進みません。
//
// 通常のテストは WithinTx が同じ直列化を内部で行うため、これを呼ぶ必要はありません。呼ぶのは、
// トランザクションを 2 本同時に生かす検証（ロック競合の再現など）のように、WithinTx の
// 「1 本 + ロールバック」では表現できないテストに限られます。
func HoldSuiteSerialization(t *testing.T, db driver.DatabaseDriver) {
	t.Helper()

	release, err := holdSuiteSerialization(context.Background(), db)
	require.NoError(t, err)
	t.Cleanup(release)
}

// holdSuiteSerialization は、直列化を占有し、解放する関数を返します。
// 占有に失敗した場合は何も保持せずにエラーだけを返します。
//
// 取得と解放をここへ閉じてください。呼び出し側で占有と後始末の登録が別々の文に分かれると、
// その間で失敗したときに解放されない経路ができます。
func holdSuiteSerialization(ctx context.Context, db driver.DatabaseDriver) (func(), error) {
	txLock.Lock()

	holder, err := db.Begin(ctx)
	if err != nil {
		txLock.Unlock()
		return nil, err
	}

	if err := lockSuiteSerialization(ctx, holder); err != nil {
		_ = holder.Rollback(ctx)
		txLock.Unlock()
		return nil, err
	}

	return func() {
		_ = holder.Rollback(ctx)
		txLock.Unlock()
	}, nil
}

// lockSuiteSerialization は、q が属するトランザクションでスイート直列化用の advisory lock を取ります。
func lockSuiteSerialization(ctx context.Context, q driver.DBTX) error {
	// advisory lock 待ちは lock_timeout(10s) の対象で、高負荷で超過すると 55P03（非リトライ）で
	// 落ちる。取得の間だけ無効化する（直列化ゆえテーブルロック競合は起きず待ち詰まりの懸念はない）。
	if _, err := q.Exec(ctx, "SET LOCAL lock_timeout = 0"); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", txAdvisoryLockKey); err != nil {
		return err
	}
	return nil
}

// getTestDB は、テスト用の共有データベースドライバーを返します（初回呼び出し時のみ生成）。
func getTestDB(t *testing.T) driver.DatabaseDriver {
	t.Helper()

	dbOnce.Do(func() {
		cfg := config.MockConfigForTest(t)
		dbCfg := config.NewDatabaseConfig(cfg)
		osCfg := config.NewOperatingSystemConfig(cfg)
		dbConnCfg := config.NewDBConnectionConfig(cfg)

		testDB, errInit = driver.NewDB(dbCfg, osCfg, dbConnCfg)
	})

	require.NoError(t, errInit)
	require.NotNil(t, testDB)

	return testDB
}
