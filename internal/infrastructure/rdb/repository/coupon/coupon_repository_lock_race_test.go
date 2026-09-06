package coupon

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/config"
	domaincoupon "go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/infrastructure/system"
	"go-boilerplate/internal/logging"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/tx"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// raceBlockedGracePeriod は、引き換え役が返却役のロック解放を待たされていることを確認するために待つ時間です。
// 高負荷でこの時間内に問い合わせが届かない場合も「まだ完了していない」側に倒れるため、負荷は偽陽性を生みません。
// 待たされた事実は、後続役がロックを取れた時刻が先行役の解放より後であることでも確かめます。
// 最終状態の一致だけを見ると、ロックが外れていても後続役の往復がこの時間を超えれば同じ観測になり、
// 退行を見逃します。所要時間そのものを閾値と比べないのは、後続役の計測開始が先行役の待機開始より
// 遅れるぶん、ロックが効いていても閾値を下回るためです。
const raceBlockedGracePeriod = 300 * time.Millisecond

var (
	// errRollbackRaceTx は、引き換え役の tx を成否に関わらずロールバックさせるための番兵です。
	errRollbackRaceTx = xerrors.New("rollback race tx")
	// errCouponRestored は、引き換え役が読み出したクーポンが未使用へ戻っていたことを表す番兵です。
	errCouponRestored = xerrors.New("coupon restored")
)

// Test_lockByIDSerializesRestoreAgainstRedeem は、返却（キャンセル）と引き換え（購入確定）が同じクーポン行へ
// 同時に到達したとき、LockByID が両者を直列化し、待たされた側が返却後の状態を観測することを検証します。
//
// tx を 2 本同時に生かす必要があるため testkit.WithinTx（tx 1 本 + ロールバック）では表現できません。
// 返却役をコミットさせる必要から、検証用クーポンは後始末で物理削除します。
func Test_lockByIDSerializesRestoreAgainstRedeem(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	// 検証用クーポンの作成から 2 本の tx の完了までを、他パッケージの CASCADE TRUNCATE から守る。
	testkit.HoldSuiteSerialization(t, testDB)

	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}

	newTxManager := func() tx.Manager {
		return driver.NewTransactionManager(
			testDB,
			config.NewDatabaseConfig(config.MockConfigForTest(t)),
			logging.NewTestLogger(t),
			system.NewSleeper(),
		)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	targetID := uuidtestkit.NewTestFromSalt(t, "race-restore-coupon")

	// 共有シードを使うと他テストへ波及するため、この検証専用の使用済みクーポンを立てて後始末で物理削除する。
	_, insertErr := driver.New(ctx, testDB).Exec(ctx,
		`INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, expires_at, used_at, issued_at)
		 VALUES ($1, $2, 2, '0.10', 1, $3, $4, $5)`,
		targetID, mustParse(t, seedBobUserID), now.Add(30*24*time.Hour), now, now.Add(-24*time.Hour))
	require.NoError(t, insertErr)
	t.Cleanup(func() {
		_, cleanupErr := driver.New(ctx, testDB).Exec(ctx, "DELETE FROM coupons WHERE id = $1", targetID)
		require.NoError(t, cleanupErr)
	})

	restoreLocked := make(chan struct{})
	redeemDone := make(chan struct{})
	redeemResult := make(chan error, 1)
	redeemAcquiredAt := make(chan time.Time, 1)
	var restoreReleasedAt time.Time

	// 引き換え役: 返却役がクーポン行を押さえている間にロックへ入り、返却の確定まで待たされる。
	go func() {
		defer close(redeemDone)
		<-restoreLocked
		redeemResult <- newTxManager().Do(ctx, func(txCtx context.Context) error {
			redeeming, lockErr := repo.LockByID(txCtx, targetID)
			redeemAcquiredAt <- time.Now()
			if lockErr != nil {
				return xerrors.Join(errRollbackRaceTx, lockErr)
			}
			// ロックを取れた時点の状態が未使用であることが、直列化の成立を示す。
			if !redeeming.IsUsed() {
				return xerrors.Join(errRollbackRaceTx, errCouponRestored)
			}
			return errRollbackRaceTx
		})
	}()

	// 返却役: クーポン行を排他ロックしてから未使用への差し戻しを確定させる。
	require.NoError(t, newTxManager().Do(ctx, func(txCtx context.Context) error {
		restoring, lockErr := repo.LockByID(txCtx, targetID)
		if lockErr != nil {
			return lockErr
		}
		close(restoreLocked)

		// ロックを握ったまま、引き換え役が素通りしないことを確かめる。
		select {
		case <-redeemDone:
			t.Error("引き換え役が返却役のロックを待たずに完了した")
		case <-time.After(raceBlockedGracePeriod):
		}
		restoreReleasedAt = time.Now()

		restored, restoreErr := restoring.Restore(time.Now().UTC())
		if restoreErr != nil {
			return restoreErr
		}
		if !restored {
			return domaincoupon.ErrNotUsed
		}

		return repo.UpdateUnused(txCtx, targetID)
	}))

	<-redeemDone
	// 返却が確定した以上、待たされていた引き換えが読み出すクーポンは未使用になっている。
	require.ErrorIs(t, <-redeemResult, errCouponRestored)
	// 未使用を観測できたのがロック待ちの結果であることを、取得時刻の前後で裏づける。
	assert.True(t, (<-redeemAcquiredAt).After(restoreReleasedAt))
}
