package product

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/config"
	domainproduct "go-boilerplate/internal/domain/product"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/infrastructure/system"
	"go-boilerplate/internal/logging"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readCompletionGracePeriod は、ロックを待たされていないことを確認するために読み取りへ与える時間です。
// 高負荷でこの時間内に完了しない場合は「待たされた」側へ倒れるため、偽陰性の方向にだけ振れます。
const (
	readCompletionGracePeriod = 3 * time.Second

	// raceBlockedGracePeriod は、後続役が先行役のロック解放を待たされていることを確認するために待つ時間です。
	// 待たされた事実は所要時間でも確かめます。最終状態の一致だけを見ると、ロックが外れていても
	// 後続役の往復がこの時間を超えれば同じ観測になり、退行を見逃します。
	// 高負荷でこの時間内に完了しない場合も「まだ完了していない」側へ倒れるため、負荷は偽陽性を生みません。
	raceBlockedGracePeriod = 300 * time.Millisecond
)

// errRollbackHolderTx は、ロック保持役の tx をロールバックさせるための番兵です。
var errRollbackHolderTx = xerrors.New("rollback holder tx")

// Test_findByIDsDoesNotWaitForRowLocks は、FindByIDs が悲観ロックを取らないことを実 DB の 2 トランザクションで
// 検証します。カートの再評価は表示のたびに走るため、ここでロックを取ると表示が購入と商品行を奪い合います。
//
// tx を 2 本同時に生かす必要があるため testkit.WithinTx（tx 1 本 + ロールバック）では表現できません。
// 保持役へ実際にロックを取らせる必要から、検証用商品は後始末で物理削除します。
func Test_findByIDsDoesNotWaitForRowLocks(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	// 検証用商品の作成から 2 本の tx の完了までを、他パッケージの CASCADE TRUNCATE から守る。
	testkit.HoldSuiteSerialization(t, testDB)

	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}

	newTxManager := func() tx.Manager {
		return driver.NewTransactionManager(
			testDB,
			config.NewDatabaseConfig(config.MockConfigForTest(t)),
			logging.NewTestLogger(t),
			system.NewSleeper(),
		)
	}

	ctx := context.Background()
	targetID := "cccccccc-0000-4000-8003-000000000001"
	targetUUID := mustParse(t, targetID)
	published := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	insertProduct(ctx, t, driver.New(ctx, testDB), targetID,
		probeKeyword+"-FINDS-RACE", nil, 1999, statusInStock, categoryElectronics, &published)
	t.Cleanup(func() {
		_, cleanupErr := driver.New(ctx, testDB).Exec(ctx, "DELETE FROM products WHERE id = $1", targetID)
		require.NoError(t, cleanupErr)
	})

	var (
		lockHeld  = make(chan struct{})
		release   = make(chan struct{})
		readDone  = make(chan struct{})
		holderEnd = make(chan struct{})
	)

	var (
		read    domainproduct.Products
		readErr error
	)

	// 保持役: 商品行を排他ロックし、読み取り役の完了を確認するまで保持し続ける。
	go func() {
		defer close(holderEnd)
		_ = newTxManager().Do(ctx, func(txCtx context.Context) error {
			if _, lockErr := repo.LockByID(txCtx, targetUUID); lockErr != nil {
				return xerrors.Join(errRollbackHolderTx, lockErr)
			}
			close(lockHeld)
			<-release
			return errRollbackHolderTx
		})
	}()

	<-lockHeld

	// 読み取り役の表明はテスト本体で行う。goroutine 内の require はテストを正しく停止できない。
	go func() {
		defer close(readDone)
		read, readErr = repo.FindByIDs(ctx, []uuid.UUID{targetUUID})
	}()

	select {
	case <-readDone:
		require.NoError(t, readErr)
		assert.Len(t, read, 1)
	case <-time.After(readCompletionGracePeriod):
		t.Error("FindByIDs が行ロックの解放を待たされた")
	}

	close(release)
	<-holderEnd
}

// Test_lockByIDsSerializesConcurrentUpdates は、LockByIDs が同一商品への並行取得を直列化することを
// 実 DB の 2 トランザクションで検証します。FindByIDs との違いはロックを取るかどうかだけであり、
// 共通の契約テスト（assertBatchProductFetchContract）ではその差を表現できません。
//
// tx を 2 本同時に生かす必要があるため testkit.WithinTx（tx 1 本 + ロールバック）では表現できません。
func Test_lockByIDsSerializesConcurrentUpdates(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	// 検証用商品の作成から 2 本の tx の完了までを、他パッケージの CASCADE TRUNCATE から守る。
	testkit.HoldSuiteSerialization(t, testDB)

	repo := &repository{tracer: observability.NewMockInfraLayerTracer(t), db: testDB}

	newTxManager := func() tx.Manager {
		return driver.NewTransactionManager(
			testDB,
			config.NewDatabaseConfig(config.MockConfigForTest(t)),
			logging.NewTestLogger(t),
			system.NewSleeper(),
		)
	}

	ctx := context.Background()
	targetID := "cccccccc-0000-4000-8004-000000000001"
	targetUUID := mustParse(t, targetID)
	published := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)

	insertProduct(ctx, t, driver.New(ctx, testDB), targetID,
		probeKeyword+"-LOCKS-RACE", nil, 1999, statusInStock, categoryElectronics, &published)
	t.Cleanup(func() {
		_, cleanupErr := driver.New(ctx, testDB).Exec(ctx, "DELETE FROM products WHERE id = $1", targetID)
		require.NoError(t, cleanupErr)
	})

	var (
		lockHeld         = make(chan struct{})
		secondDone       = make(chan struct{})
		holderEnd        = make(chan struct{})
		secondBlockedFor = make(chan time.Duration, 1)
	)

	// 後続役: 先行役が商品行を押さえている間にロックを取りに行き、解放まで待たされる。
	go func() {
		defer close(secondDone)
		<-lockHeld
		_ = newTxManager().Do(ctx, func(txCtx context.Context) error {
			startedAt := time.Now()
			_, lockErr := repo.LockByIDs(txCtx, []uuid.UUID{targetUUID})
			secondBlockedFor <- time.Since(startedAt)
			if lockErr != nil {
				return xerrors.Join(errRollbackHolderTx, lockErr)
			}

			return errRollbackHolderTx
		})
	}()

	// 先行役: 商品行を排他ロックし、後続役が待たされていることを確認してから解放する。
	go func() {
		defer close(holderEnd)
		_ = newTxManager().Do(ctx, func(txCtx context.Context) error {
			if _, lockErr := repo.LockByIDs(txCtx, []uuid.UUID{targetUUID}); lockErr != nil {
				return xerrors.Join(errRollbackHolderTx, lockErr)
			}
			close(lockHeld)

			select {
			case <-secondDone:
				t.Error("後続役が先行役のロックを待たずに完了した")
			case <-time.After(raceBlockedGracePeriod):
			}
			return errRollbackHolderTx
		})
	}()

	<-holderEnd
	<-secondDone
	// 後続役が完了したのがロック待ちの結果であることを、待機時間で裏づける。
	assert.GreaterOrEqual(t, <-secondBlockedFor, raceBlockedGracePeriod)
}
