package testkit

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/config"
	"go-boilerplate/internal/logging"

	"go-boilerplate/internal/infrastructure/rdb/driver"
	mock_driver "go-boilerplate/internal/infrastructure/rdb/driver/mock"
	"go-boilerplate/internal/infrastructure/system"
	mock_tx "go-boilerplate/internal/usecase/boundary/tx/mock"
	"go-boilerplate/pkg/xerrors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

// lockTimeoutProbeHold は、待機側の lock_timeout を確実に超える保持時間です。
// waiterLockTimeout は、待機側が張る lock_timeout（無効化されなければここで 55P03 になる）です。
const (
	lockTimeoutProbeHold = 300 * time.Millisecond
	waiterLockTimeout    = 50 * time.Millisecond
)

// errBeginFailed は db.Begin を、errExecFailed は Exec を、それぞれ失敗させるモックが返すエラーです。
var (
	errBeginFailed = xerrors.New("begin failed")
	errExecFailed  = xerrors.New("exec failed")
)

func TestNewTestDB(t *testing.T) {
	t.Parallel()
	db := NewTestDB(t)
	// 返る DB が生きている（接続可能）ことを検証する。
	require.NoError(t, db.Ping(context.Background()))
}

func TestNewTestTransactionRunner(t *testing.T) {
	t.Parallel()
	runner := NewTestTransactionRunner(t)
	// 公開 API 経由で WithinTx がコールバックを実行する（実トランザクションを開始しロールバックする）ことを検証する。
	ran := false
	runner.WithinTx(func(context.Context) { ran = true })
	assert.True(t, ran)
}

func Test_testTxRunner_WithinTx(t *testing.T) {
	t.Parallel()
	cfg := config.MockConfigForTest(t)
	dbCfg := config.NewDatabaseConfig(cfg)
	osCfg := config.NewOperatingSystemConfig(cfg)
	dbConnCfg := config.NewDBConnectionConfig(cfg)

	testLogger := logging.NewTestLogger(t)

	db, err := driver.NewDB(dbCfg, osCfg, dbConnCfg)
	require.NoError(t, err)
	innerTxm := driver.NewTransactionManager(db, dbCfg, testLogger, system.NewSleeper())

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("実行時にエラーが発生しない場合、正常に終了すること", func(t *testing.T) {
			t.Parallel()
			txm := &testTxRunner{
				inner: innerTxm,
				db:    db,
				t:     t,
			}
			txm.WithinTx(func(_ context.Context) {})
		})

		t.Run("Doがロールバックsentinel以外のnilを返す場合、NoError検証まで到達すること", func(t *testing.T) {
			t.Parallel()
			// inner.Do が nil を返すと、rollback sentinel 判定を外れて require.NoError の検証経路に到達する。
			ctrl := gomock.NewController(t)
			manager := mock_tx.NewMockManager(ctrl)
			manager.EXPECT().Do(gomock.Any(), gomock.Any()).Return(nil)

			txm := &testTxRunner{
				inner: manager,
				t:     t,
			}
			txm.WithinTx(func(_ context.Context) {})
		})
	})
}

func Test_testTxRunner_WithinTxE(t *testing.T) {
	t.Parallel()
	cfg := config.MockConfigForTest(t)
	dbCfg := config.NewDatabaseConfig(cfg)
	osCfg := config.NewOperatingSystemConfig(cfg)
	dbConnCfg := config.NewDBConnectionConfig(cfg)

	testLogger := logging.NewTestLogger(t)

	db, err := driver.NewDB(dbCfg, osCfg, dbConnCfg)
	require.NoError(t, err)
	innerTxm := driver.NewTransactionManager(db, dbCfg, testLogger, system.NewSleeper())

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("fnがnilを返す場合、トランザクションはロールバックされ検証は通ること", func(t *testing.T) {
			t.Parallel()
			txm := &testTxRunner{inner: innerTxm, db: db, t: t}

			ran := false
			txm.WithinTxE(func(_ context.Context) error {
				ran = true
				return nil
			})
			assert.True(t, ran)
		})

		t.Run("fnがdeadlockを返す場合、トランザクションごと再試行されること", func(t *testing.T) {
			t.Parallel()
			// 本 issue の主題。deadlock は pgerror.IsRetryableTxError がリトライ可能と宣言しており、
			// エラーを返しさえすればトランザクションマネージャーが tx ごと再試行する。
			// require で即死させるとこの経路へ到達しない（runtime.Goexit で戻り値が失われる）。
			txm := &testTxRunner{inner: innerTxm, db: db, t: t}

			attempts := 0
			txm.WithinTxE(func(_ context.Context) error {
				attempts++
				if attempts < 2 {
					// 40P01 = deadlock_detected（pgerror.IsRetryableTxError がリトライ可能と判定する）。
					return &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
				}
				return nil
			})
			assert.Equal(t, 2, attempts)
		})
	})
}

func Test_holdSuiteSerialization(t *testing.T) {
	t.Parallel()

	db := NewTestDB(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("占有中はtxLockを保持し、解放関数を呼ぶと手放す", func(t *testing.T) {
			t.Parallel()
			requireTxLockAvailable(t, "先行するテストが txLock を解放していない")

			release, err := holdSuiteSerialization(context.Background(), db)
			require.NoError(t, err)
			require.NotNil(t, release)

			assert.False(t, txLock.TryLock(), "占有中に txLock を取れてしまう")

			release()
			requireTxLockAvailable(t, "解放関数を呼んでも txLock が解放されていない")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("トランザクションを開始できない場合、txLockを保持したままにしない", func(t *testing.T) {
			t.Parallel()
			requireTxLockAvailable(t, "先行するテストが txLock を解放していない")

			mockDB := mock_driver.NewMockDatabaseDriver(gomock.NewController(t))
			mockDB.EXPECT().Begin(gomock.Any()).Return(nil, errBeginFailed)

			release, err := holdSuiteSerialization(context.Background(), mockDB)
			require.ErrorIs(t, err, errBeginFailed)
			assert.Nil(t, release)

			requireTxLockAvailable(t, "占有に失敗したのに txLock を保持したままになっている")
		})

		t.Run("直列化の文を実行できない場合、holderを終了しtxLockを保持したままにしない", func(t *testing.T) {
			t.Parallel()
			requireTxLockAvailable(t, "先行するテストが txLock を解放していない")

			// holder は開いたまま渡す。閉じた tx を渡すと Rollback の有無が観測できず、
			// 後始末を落とす退行を見逃す。文の失敗はキャンセル済み context で決定的に起こす。
			holder, err := db.Begin(context.Background())
			require.NoError(t, err)
			t.Cleanup(func() { _ = holder.Rollback(context.Background()) })

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			mockDB := mock_driver.NewMockDatabaseDriver(gomock.NewController(t))
			mockDB.EXPECT().Begin(gomock.Any()).Return(holder, nil)

			release, err := holdSuiteSerialization(ctx, mockDB)
			require.ErrorIs(t, err, context.Canceled)
			assert.Nil(t, release)

			_, execErr := holder.Exec(context.Background(), "SELECT 1")
			require.ErrorIs(t, execErr, pgx.ErrTxClosed, "holder が終了しておらず、トランザクションが開いたまま残る")

			requireTxLockAvailable(t, "占有に失敗したのに txLock を保持したままになっている")
		})
	})
}

// requireTxLockAvailable は、txLock が取得可能になるまで待ちます。
// 同居する並列テストが一時的に握るため即時とは限らず、解放漏れだけが時間切れになります。
//
// 占有の前後どちらでも呼んでください。前で呼ばないと、解放漏れが起きたとき後続が
// txLock.Lock() で止まる窓が広がります。並列サブテストの出力は親が終わるまで出ないため、
// 止まった側に巻き込まれると失敗の内容が消えます。
//
// ロックの強制解除はしません。解放漏れが再発すれば掴みに行った側は止まったままで、
// パッケージは go test のタイムアウトを待ちます。
func requireTxLockAvailable(t *testing.T, msg string) {
	t.Helper()

	require.Eventually(t, func() bool {
		if !txLock.TryLock() {
			return false
		}
		txLock.Unlock()
		return true
	}, 15*time.Second, 50*time.Millisecond, msg)
}

func TestHoldSuiteSerialization(t *testing.T) {
	t.Parallel()

	db := NewTestDB(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("占有中は他セッションが同じキーを取得できない", func(t *testing.T) {
			t.Parallel()
			HoldSuiteSerialization(t, db)

			// 直列化の実体は advisory lock の排他性なので、別セッション（接続プール直）から
			// 同じキーを取れないことで検証する。取れてしまうなら並行実行を止められていない。
			ctx := context.Background()
			var acquired bool
			row := driver.New(ctx, db).QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", txAdvisoryLockKey)
			require.NoError(t, row.Scan(&acquired))
			assert.False(t, acquired)
		})

		t.Run("占有したテストが終わると解放される", func(t *testing.T) {
			t.Parallel()

			// 解放は t.Cleanup で行われるため、占有を子テストへ閉じ込めて完了させ、
			// 親から解放後の状態を観測する。解放されないとスイート全体が止まる。
			//nolint:paralleltest // Cleanup を親の観測より先に走らせるため逐次実行する
			t.Run("占有", func(t *testing.T) {
				HoldSuiteSerialization(t, db)
			})

			ctx := context.Background()
			probe, err := db.Begin(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = probe.Rollback(ctx) })

			// 同じキーは他の並列テストの WithinTx も一時的に握るため、取得できるまで待つ。
			// 解放されないなら永久に取得できず、ここで打ち切られる。
			// probe 自身の解放漏れを避けるため、セッション単位ではなく tx 単位で取得する。
			require.Eventually(t, func() bool {
				var acquired bool
				if err := probe.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", txAdvisoryLockKey).Scan(&acquired); err != nil {
					return false
				}
				return acquired
			}, 15*time.Second, 50*time.Millisecond, "占有したテストが終わっても解放されない")
		})
	})
}

func Test_lockSuiteSerialization(t *testing.T) {
	t.Parallel()

	db := NewTestDB(t)
	runner := NewTestTransactionRunner(t)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("同一トランザクションからの再取得は待たずに成立する", func(t *testing.T) {
			t.Parallel()
			// WithinTx が既に同じキーを保持している。advisory lock は同一トランザクション内で再入可能。
			runner.WithinTx(func(ctx context.Context) {
				require.NoError(t, lockSuiteSerialization(ctx, driver.New(ctx, db)))
			})
		})

		t.Run("他トランザクションが保持中でも呼び出し側のlock_timeoutで打ち切られない", func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// 保持側。WithinTx はこの直列化に参加するため使えず、専用の tx で握る。
			// 解放済みの tx への Rollback は ErrTxClosed を返すだけなので、後始末は無条件で行う。
			holder, err := db.Begin(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = holder.Rollback(ctx) })

			// このキーは全パッケージが共有するため、保持側自身も接続既定の lock_timeout(10s)
			// で打ち切られうる。検証対象と同じ理由でここでも無効化する。
			_, err = holder.Exec(ctx, "SET LOCAL lock_timeout = 0")
			require.NoError(t, err)
			_, err = holder.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", txAdvisoryLockKey)
			require.NoError(t, err)

			type waitResult struct {
				err     error
				elapsed time.Duration
			}
			waiterPID := make(chan int32, 1)
			waiterDone := make(chan waitResult, 1)
			go func() {
				waiter, beginErr := db.Begin(ctx)
				if beginErr != nil {
					waiterDone <- waitResult{err: beginErr}
					return
				}
				defer func() { _ = waiter.Rollback(ctx) }()

				var pid int32
				if scanErr := waiter.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); scanErr != nil {
					waiterDone <- waitResult{err: scanErr}
					return
				}
				if _, execErr := waiter.Exec(ctx, "SET LOCAL lock_timeout = '50ms'"); execErr != nil {
					waiterDone <- waitResult{err: execErr}
					return
				}

				waiterPID <- pid
				startedAt := time.Now()
				waiterDone <- waitResult{err: lockSuiteSerialization(ctx, waiter), elapsed: time.Since(startedAt)}
			}()

			var pid int32
			select {
			case pid = <-waiterPID:
			case got := <-waiterDone:
				require.NoError(t, got.err, "待機側がロック待ちへ入る前に失敗した")
			}

			// 待機側がロック待ちへ入る前に解放すると、lock_timeout が効いたままでも
			// 素通りして偽陽性で通る。待ちに入ったことを確認してから解放する。
			requireAdvisoryLockWait(t, db, pid)
			time.Sleep(lockTimeoutProbeHold)
			require.NoError(t, holder.Rollback(ctx))

			got := <-waiterDone
			require.NoError(t, got.err, "無効化されていなければ lock_timeout(50ms) で 55P03 になる")
			assert.Greater(t, got.elapsed, waiterLockTimeout, "解放を待たずに取得しており、待ち合わせを検証できていない")
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("lock_timeoutの無効化に失敗した場合、advisory lockを取りに行かない", func(t *testing.T) {
			t.Parallel()

			q := mock_driver.NewMockDBTX(gomock.NewController(t))
			// 1 文目が失敗したら 2 文目は発行しない。EXPECT を 1 つだけ置くことがその検証になる。
			q.EXPECT().Exec(gomock.Any(), "SET LOCAL lock_timeout = 0").Return(pgconn.CommandTag{}, errExecFailed)

			require.ErrorIs(t, lockSuiteSerialization(context.Background(), q), errExecFailed)
		})

		t.Run("advisory lockの取得に失敗した場合、そのエラーを返す", func(t *testing.T) {
			t.Parallel()

			q := mock_driver.NewMockDBTX(gomock.NewController(t))
			q.EXPECT().Exec(gomock.Any(), "SET LOCAL lock_timeout = 0").Return(pgconn.CommandTag{}, nil)
			q.EXPECT().Exec(gomock.Any(), "SELECT pg_advisory_xact_lock($1)", txAdvisoryLockKey).
				Return(pgconn.CommandTag{}, errExecFailed)

			require.ErrorIs(t, lockSuiteSerialization(context.Background(), q), errExecFailed)
		})
	})
}

// requireAdvisoryLockWait は、pid のセッションがスイート直列化キーの advisory lock 待ちへ
// 入るまで待ちます。このキーは全パッケージが共有するため、pid で絞らないと無関係な
// セッションの待ちを自分のものと取り違えます。
func requireAdvisoryLockWait(t *testing.T, db driver.DatabaseDriver, pid int32) {
	t.Helper()

	ctx := context.Background()
	require.Eventually(t, func() bool {
		var waiting bool
		if err := driver.New(ctx, db).QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND NOT granted
				AND pid = $1 AND (classid::bigint << 32) + objid::bigint = $2 AND objsubid = 1)`,
			pid, txAdvisoryLockKey).Scan(&waiting); err != nil {
			return false
		}
		return waiting
	}, 10*time.Second, 20*time.Millisecond, "advisory lock 待ちに入らなかった")
}

func Test_getTestDB(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("接続可能なドライバを返す", func(t *testing.T) {
			t.Parallel()

			db := getTestDB(t)

			require.NotNil(t, db)
			require.NoError(t, db.Ping(context.Background()))
		})

		t.Run("複数回呼び出しても同一のドライバを共有する", func(t *testing.T) {
			t.Parallel()

			first := getTestDB(t)
			second := getTestDB(t)

			assert.Same(t, first, second)
		})
	})
}
