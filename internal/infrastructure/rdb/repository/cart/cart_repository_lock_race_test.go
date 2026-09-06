package cart

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/config"
	domaincart "go-boilerplate/internal/domain/cart"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/infrastructure/system"
	"go-boilerplate/internal/logging"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// raceBlockedGracePeriod は、後続役が先行役のロック解放を待たされていることを確認するために待つ時間です。
// 高負荷でこの時間内に問い合わせが届かない場合も「まだ完了していない」側に倒れるため、負荷は偽陽性を生みません。
// 待たされた事実は、後続役がロックを取れた時刻が先行役の解放より後であることでも確かめます。
// 最終状態の一致だけを見ると、ロックが外れていても後続役の往復がこの時間を超えれば同じ観測になり、
// 退行を見逃します。所要時間そのものを閾値と比べないのは、後続役の計測開始が先行役の待機開始より
// 遅れるぶん、ロックが効いていても閾値を下回るためです。
const raceBlockedGracePeriod = 300 * time.Millisecond

// errRollbackRaceTx は、後続役の tx を成否に関わらずロールバックさせるための番兵です。
var errRollbackRaceTx = xerrors.New("rollback race tx")

// Test_lockSerializesConcurrentCartUpdates は、LockByID が同一カートへの並行更新を直列化することを
// 実 DB の 2 トランザクションで検証します。
//
// tx を 2 本同時に生かす必要があるため testkit.WithinTx（tx 1 本 + ロールバック）では表現できません。
// 先行役をコミットさせる必要から、検証用カートは後始末で物理削除します。
func Test_lockSerializesConcurrentCartUpdates(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	// 検証用カートの作成から 2 本の tx の完了までを、他パッケージの CASCADE TRUNCATE から守る。
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
	target := newGuestCart(t, "rce")
	require.NoError(t, repo.Create(ctx, target))
	t.Cleanup(func() {
		_, cleanupErr := driver.New(ctx, testDB).Exec(ctx, "DELETE FROM carts WHERE id = $1", target.ID())
		require.NoError(t, cleanupErr)
	})

	firstLocked := make(chan struct{})
	secondDone := make(chan struct{})
	secondResult := make(chan *domaincart.Cart, 1)
	secondAcquiredAt := make(chan time.Time, 1)
	var firstReleasedAt time.Time

	// 後続役: 先行役がカート行を押さえている間にロックを取りに行き、確定まで待たされる。
	go func() {
		defer close(secondDone)
		<-firstLocked
		_ = newTxManager().Do(ctx, func(txCtx context.Context) error {
			locked, lockErr := repo.LockByID(txCtx, target.ID())
			secondAcquiredAt <- time.Now()
			if lockErr != nil {
				return xerrors.Join(errRollbackRaceTx, lockErr)
			}
			secondResult <- locked
			return errRollbackRaceTx
		})
	}()

	extended := baseTime.Add(72 * time.Hour)

	// 先行役: カート行を排他ロックしてから有効期限の延長を確定させる。
	require.NoError(t, newTxManager().Do(ctx, func(txCtx context.Context) error {
		locked, lockErr := repo.LockByID(txCtx, target.ID())
		if lockErr != nil {
			return lockErr
		}
		close(firstLocked)

		select {
		case <-secondDone:
			t.Error("後続役が先行役のロックを待たずに完了した")
		case <-time.After(raceBlockedGracePeriod):
		}
		firstReleasedAt = time.Now()

		locked.Touch(extended, 0)
		return repo.Update(txCtx, locked)
	}))

	<-secondDone
	// 先行役が確定した以上、待たされていた後続役が読み出すのは延長後のカートになる。
	require.Equal(t, extended.UTC(), (<-secondResult).ExpiresAt().UTC())
	// 延長後を観測できたのがロック待ちの結果であることを、取得時刻の前後で裏づける。
	assert.True(t, (<-secondAcquiredAt).After(firstReleasedAt))
}
