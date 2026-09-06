package user

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/config"
	"go-boilerplate/internal/domain/user"
	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/testkit"
	"go-boilerplate/internal/infrastructure/system"
	"go-boilerplate/internal/logging"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/pkg/uuid"
	uuidtestkit "go-boilerplate/pkg/uuid/testkit"
	"go-boilerplate/pkg/xerrors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// raceBlockedGracePeriod は、購入役が退会役のロック解放を待たされていることを確認するために待つ時間です。
// 高負荷でこの時間内に問い合わせが届かない場合も「まだ完了していない」側に倒れるため、負荷は偽陽性を生みません。
// 待たされた事実は、後続役がロックを取れた時刻が先行役の解放より後であることでも確かめます。
// 最終状態の一致だけを見ると、ロックが外れていても後続役の往復がこの時間を超えれば同じ観測になり、
// 退行を見逃します。所要時間そのものを閾値と比べないのは、後続役の計測開始が先行役の待機開始より
// 遅れるぶん、ロックが効いていても閾値を下回るためです。
const raceBlockedGracePeriod = 300 * time.Millisecond

var (
	// errRollbackRaceTx は、購入役の tx を成否に関わらずロールバックさせるための番兵です。
	errRollbackRaceTx = xerrors.New("rollback race tx")
	// errPurchaserWithdrawn は、購入役が読み出した購入者が退会済みだったことを表す番兵です。
	errPurchaserWithdrawn = xerrors.New("purchaser withdrawn")
)

// Test_lockSerializesWithdrawalAgainstPurchase は、#766 の競合順序を実 DB の 2 トランザクションで再現し、
// 退会（排他ロック）が確定した時点で購入の在籍ガード（共有ロック）が拒否へ転じることを検証します。
//
// tx を 2 本同時に生かす必要があるため testkit.WithinTx（tx 1 本 + ロールバック）では表現できません。
// 退会役をコミットさせる必要から、検証用ユーザーは後始末で物理削除します。
func Test_lockSerializesWithdrawalAgainstPurchase(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	// 検証用ユーザーの作成から 2 本の tx の完了までを、他パッケージの CASCADE TRUNCATE から守る。
	// 部分的にしか守らないと、作成直後の行が消えてロック競合の手前で落ちる。
	testkit.HoldSuiteSerialization(t, testDB)

	tracer := observability.NewMockInfraLayerTracer(t)
	repo := &repository{tracer: tracer, db: testDB}
	lockRepo := &lockRepository{tracer: tracer, db: testDB}

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
	targetID := uuidtestkit.NewTestFromSalt(t, "race-withdrawal-user")
	prefectureID, err := uuid.Parse("a03aaec4-3bd6-4bfb-8e47-2fbfa026d344")
	require.NoError(t, err)

	target, err := user.New(targetID, user.Attributes{
		FirstName:    "Race",
		LastName:     "Target",
		Email:        "race.target@example.com",
		Phone:        "7777777777",
		PrefectureID: prefectureID,
		City:         "新宿区",
		Street:       "9-9-9",
		PostalCode:   "160-0009",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)

	// 共有シードを退会させると他テストへ波及するため、この検証専用のユーザーを立てて後始末で物理削除する。
	require.NoError(t, repo.Create(ctx, target))
	t.Cleanup(func() {
		_, cleanupErr := driver.New(ctx, testDB).Exec(ctx, "DELETE FROM users WHERE id = $1", targetID)
		require.NoError(t, cleanupErr)
	})

	withdrawalLocked := make(chan struct{})
	guardDone := make(chan struct{})
	guardResult := make(chan error, 1)
	guardAcquiredAt := make(chan time.Time, 1)
	var withdrawalReleasedAt time.Time

	// 購入役: 退会役がユーザー行を押さえている間に在籍ガードへ入り、退会の確定まで待たされる。
	go func() {
		defer close(guardDone)
		<-withdrawalLocked
		guardResult <- newTxManager().Do(ctx, func(txCtx context.Context) error {
			purchaser, guardErr := lockRepo.LockShareByID(txCtx, targetID)
			guardAcquiredAt <- time.Now()
			if guardErr != nil {
				return xerrors.Join(errRollbackRaceTx, guardErr)
			}
			// 共有ロックを取れた時点の状態が退会済みであることが、直列化の成立を示す。
			if !purchaser.IsActive() {
				return xerrors.Join(errRollbackRaceTx, errPurchaserWithdrawn)
			}
			return errRollbackRaceTx
		})
	}()

	// 退会役: ユーザー行を排他ロックしてから論理削除を確定させる。
	require.NoError(t, newTxManager().Do(ctx, func(txCtx context.Context) error {
		withdrawing, lockErr := lockRepo.LockByID(txCtx, targetID)
		if lockErr != nil {
			return lockErr
		}
		close(withdrawalLocked)

		// ロックを握ったまま、購入役が素通りしないことを確かめる。
		select {
		case <-guardDone:
			t.Error("購入役が退会役のロックを待たずに完了した")
		case <-time.After(raceBlockedGracePeriod):
		}
		withdrawalReleasedAt = time.Now()

		if markErr := withdrawing.MarkAsDeleted(time.Now().UTC()); markErr != nil {
			return markErr
		}
		return repo.Update(txCtx, withdrawing)
	}))

	<-guardDone
	// 退会が確定した以上、待たされていた購入が読み出す購入者は退会済みになっている。
	require.ErrorIs(t, <-guardResult, errPurchaserWithdrawn)
	// 退会済みを観測できたのがロック待ちの結果であることを、取得時刻の前後で裏づける。
	assert.True(t, (<-guardAcquiredAt).After(withdrawalReleasedAt))
}
