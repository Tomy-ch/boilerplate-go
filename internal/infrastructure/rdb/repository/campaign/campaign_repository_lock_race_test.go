package campaign

import (
	"context"
	"testing"
	"time"

	"go-boilerplate/internal/config"
	domaincampaign "go-boilerplate/internal/domain/campaign"
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

// raceBlockedGracePeriod は、後続の受け取り役が先行役のロック解放を待たされていることを確認するために待つ時間です。
// 高負荷でこの時間内に問い合わせが届かない場合も「まだ完了していない」側に倒れるため、負荷は偽陽性を生みません。
// 待たされた事実は、後続役がロックを取れた時刻が先行役の解放より後であることでも確かめます。
const raceBlockedGracePeriod = 300 * time.Millisecond

var (
	// errRollbackRaceTx は、後続役の tx を成否に関わらずロールバックさせるための番兵です。
	errRollbackRaceTx = xerrors.New("rollback race tx")
	// errLimitObserved は、後続役が上限に達した状態を観測したことを表す番兵です。
	errLimitObserved = xerrors.New("limit observed")
)

// raceRowIDs は、後始末で物理削除する検証用の行の識別子です。同型の識別子が並ぶため構造体で受けます
// （基準は docs/rules.md の Function Signature Rules）。
type raceRowIDs struct {
	// CampaignID は、検証用キャンペーンの ID です。
	CampaignID uuid.UUID
	// CouponID は、受け取り記録が参照する検証用クーポンの ID です。
	CouponID uuid.UUID
}

// newRaceTxManager は、レース検証用に独立したトランザクションマネージャを返します。
func newRaceTxManager(t *testing.T, testDB driver.DatabaseDriver) tx.Manager {
	t.Helper()

	return driver.NewTransactionManager(
		testDB,
		config.NewDatabaseConfig(config.MockConfigForTest(t)),
		logging.NewTestLogger(t),
		system.NewSleeper(),
	)
}

// Test_lockByCodeSerializesConcurrentClaimsAgainstTotalLimit は、総枚数上限の最後の 1 枚へ 2 つの受け取りが
// 同時に到達したとき、LockByCode が両者を直列化し、待たされた側が「上限に達した」状態を観測することを検証します。
//
// これが DoD「総枚数上限が並行引き換えの下で守られる」の機械的な証明です。判定の前にロックを取らなければ、
// 両者とも「まだ 1 枚残っている」を読んで 2 枚配ってしまいます
// （ADR-0036 (ordered-pessimistic-row-locks) の決定 2）。
//
// tx を 2 本同時に生かす必要があるため testkit.WithinTx（tx 1 本 + ロールバック）では表現できません。
// 先行役をコミットさせる必要から、検証用の行は後始末で物理削除します。
func Test_lockByCodeSerializesConcurrentClaimsAgainstTotalLimit(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	// 検証用キャンペーンの作成から 2 本の tx の完了までを、他パッケージの CASCADE TRUNCATE から守る。
	testkit.HoldSuiteSerialization(t, testDB)

	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}
	ctx := context.Background()

	// 残り 1 枚のキャンペーンを立てる。総枚数上限 1 枚に対して発行済み 0 枚。
	target := newRaceCampaign(t, 1, 1)
	require.NoError(t, repo.Create(ctx, target))
	couponID := insertTestCoupon(ctx, t, testDB)
	t.Cleanup(func() {
		cleanupRaceRows(ctx, t, testDB, raceRowIDs{CampaignID: target.ID(), CouponID: couponID})
	})

	firstLocked := make(chan struct{})
	secondDone := make(chan struct{})
	secondResult := make(chan error, 1)
	secondAcquiredAt := make(chan time.Time, 1)
	var firstReleasedAt time.Time

	// 入力は goroutine の外で組み立てる。testify のアサーションはテスト本体の goroutine でしか使えない。
	secondParams := raceClaimParams(t, mustParse(t, seedAliceUserID), couponID, 0)

	// 後続役: 先行役がキャンペーン行を押さえている間にロックへ入り、先行役の確定まで待たされる。
	go func() {
		defer close(secondDone)
		<-firstLocked
		secondResult <- newRaceTxManager(t, testDB).Do(ctx, func(txCtx context.Context) error {
			locked, lockErr := repo.LockByCode(txCtx, target.Code())
			secondAcquiredAt <- time.Now()
			if lockErr != nil {
				return xerrors.Join(errRollbackRaceTx, lockErr)
			}
			// ロックを取れた時点で上限に達していることが、直列化の成立を示す。
			if _, claimErr := locked.Claim(secondParams); claimErr != nil {
				return xerrors.Join(errRollbackRaceTx, errLimitObserved, claimErr)
			}

			return errRollbackRaceTx
		})
	}()

	// 先行役: キャンペーン行を排他ロックしてから、最後の 1 枚の発行を確定させる。
	require.NoError(t, newRaceTxManager(t, testDB).Do(ctx, func(txCtx context.Context) error {
		locked, lockErr := repo.LockByCode(txCtx, target.Code())
		if lockErr != nil {
			return lockErr
		}
		close(firstLocked)

		// ロックを握ったまま、後続役が素通りしないことを確かめる。
		select {
		case <-secondDone:
			t.Error("後続の受け取り役が先行役のロックを待たずに完了した")
		case <-time.After(raceBlockedGracePeriod):
		}
		firstReleasedAt = time.Now()

		record, claimErr := locked.Claim(raceClaimParams(t, mustParse(t, seedAliceUserID), couponID, 0))
		if claimErr != nil {
			return claimErr
		}

		return repo.RecordClaim(txCtx, record)
	}))

	<-secondDone
	// 先行役が最後の 1 枚を配った以上、待たされていた後続役は上限到達を観測する。
	secondErr := <-secondResult
	require.ErrorIs(t, secondErr, errLimitObserved)
	require.ErrorIs(t, secondErr, domaincampaign.ErrTotalLimitReached)
	// 上限到達を観測できたのがロック待ちの結果であることを、取得時刻の前後で裏づける。
	assert.True(t, (<-secondAcquiredAt).After(firstReleasedAt))
}

// Test_lockByCodeSerializesConcurrentClaimsAgainstPerUserLimit は、同じ利用者による 2 つの受け取りが
// 1 人あたり上限の最後の 1 枚へ同時に到達したとき、LockByCode が両者を直列化し、待たされた側が
// 「その利用者は上限に達した」状態を観測することを検証します。
//
// これが DoD「1 人あたり上限が並行引き換えの下で守られる」の機械的な証明です。総枚数上限と違い、
// 判定に必要な枚数は別表（campaign_claims）を数えて得ますが、数えるのはキャンペーン行の
// ロックを取ったあとであるため、同じ直列化に乗ります。
func Test_lockByCodeSerializesConcurrentClaimsAgainstPerUserLimit(t *testing.T) {
	t.Parallel()

	testDB := testkit.NewTestDB(t)
	testkit.HoldSuiteSerialization(t, testDB)

	repo := &repository{db: testDB, tracer: observability.NewMockInfraLayerTracer(t)}
	ctx := context.Background()
	userID := mustParse(t, seedAliceUserID)

	// 総枚数には余裕があり、1 人あたり上限だけが 1 枚のキャンペーンを立てる。
	target := newRaceCampaign(t, 10, 1)
	require.NoError(t, repo.Create(ctx, target))
	couponID := insertTestCoupon(ctx, t, testDB)
	t.Cleanup(func() {
		cleanupRaceRows(ctx, t, testDB, raceRowIDs{CampaignID: target.ID(), CouponID: couponID})
	})

	firstLocked := make(chan struct{})
	secondDone := make(chan struct{})
	secondResult := make(chan error, 1)
	secondAcquiredAt := make(chan time.Time, 1)
	var firstReleasedAt time.Time

	// 入力は goroutine の外で組み立てる（既受け取り枚数だけはロック下で数え直す）。
	secondParams := raceClaimParams(t, userID, couponID, 0)

	go func() {
		defer close(secondDone)
		<-firstLocked
		secondResult <- newRaceTxManager(t, testDB).Do(ctx, func(txCtx context.Context) error {
			locked, lockErr := repo.LockByCode(txCtx, target.Code())
			secondAcquiredAt <- time.Now()
			if lockErr != nil {
				return xerrors.Join(errRollbackRaceTx, lockErr)
			}

			// ロックを取ったあとで数えるため、先行役が挿入した記録が見える。
			claimed, countErr := repo.CountClaims(txCtx, domaincampaign.ClaimCountParams{
				CampaignID: target.ID(),
				UserID:     userID,
			})
			if countErr != nil {
				return xerrors.Join(errRollbackRaceTx, countErr)
			}
			params := secondParams
			params.ClaimedByUser = claimed
			if _, claimErr := locked.Claim(params); claimErr != nil {
				return xerrors.Join(errRollbackRaceTx, errLimitObserved, claimErr)
			}

			return errRollbackRaceTx
		})
	}()

	require.NoError(t, newRaceTxManager(t, testDB).Do(ctx, func(txCtx context.Context) error {
		locked, lockErr := repo.LockByCode(txCtx, target.Code())
		if lockErr != nil {
			return lockErr
		}
		close(firstLocked)

		select {
		case <-secondDone:
			t.Error("後続の受け取り役が先行役のロックを待たずに完了した")
		case <-time.After(raceBlockedGracePeriod):
		}
		firstReleasedAt = time.Now()

		claimed, countErr := repo.CountClaims(txCtx, domaincampaign.ClaimCountParams{
			CampaignID: target.ID(),
			UserID:     userID,
		})
		if countErr != nil {
			return countErr
		}
		record, claimErr := locked.Claim(raceClaimParams(t, userID, couponID, claimed))
		if claimErr != nil {
			return claimErr
		}

		return repo.RecordClaim(txCtx, record)
	}))

	<-secondDone
	secondErr := <-secondResult
	require.ErrorIs(t, secondErr, errLimitObserved)
	require.ErrorIs(t, secondErr, domaincampaign.ErrPerUserLimitReached)
	assert.True(t, (<-secondAcquiredAt).After(firstReleasedAt))
}

// raceClaimParams は、受け取り 1 件の入力を組み立てます。受け取り日時は常に現在時刻で、
// 検証したいのは上限と直列化であって期間ではありません。
func raceClaimParams(t *testing.T, userID, couponID uuid.UUID, claimedByUser int) domaincampaign.ClaimParams {
	t.Helper()

	return domaincampaign.ClaimParams{
		ClaimedAt:     time.Now().UTC(),
		ClaimedByUser: claimedByUser,
		ClaimID:       newID(t),
		UserID:        userID,
		CouponID:      couponID,
	}
}

// newRaceCampaign は、上限だけを指定した配布中のキャンペーンを組み立てます。
func newRaceCampaign(t *testing.T, totalLimit, perUserLimit int) *domaincampaign.Campaign {
	t.Helper()

	base := newTestCampaign(t, nil)
	c, err := domaincampaign.New(base.ID(), domaincampaign.Attributes{
		Code:         base.Code(),
		Template:     base.Template(),
		StartsAt:     base.StartsAt(),
		EndsAt:       base.EndsAt(),
		TotalLimit:   totalLimit,
		PerUserLimit: perUserLimit,
	})
	require.NoError(t, err)

	return c
}

// cleanupRaceRows は、コミットして残した検証用の行を物理削除します。
func cleanupRaceRows(
	ctx context.Context, t *testing.T, testDB driver.DatabaseDriver, ids raceRowIDs,
) {
	t.Helper()

	db := driver.New(ctx, testDB)
	_, err := db.Exec(ctx, "DELETE FROM campaign_claims WHERE campaign_id = $1", ids.CampaignID)
	require.NoError(t, err)
	_, err = db.Exec(ctx, "DELETE FROM campaigns WHERE id = $1", ids.CampaignID)
	require.NoError(t, err)
	_, err = db.Exec(ctx, "DELETE FROM coupons WHERE id = $1", ids.CouponID)
	require.NoError(t, err)
}
