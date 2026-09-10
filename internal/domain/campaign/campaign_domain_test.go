package campaign

import (
	"testing"
	"time"

	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testStartsAt        = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	testEndsAt          = time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	testCouponExpiresAt = time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
)

func newTestUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.New()
	require.NoError(t, err)

	return id
}

func newTestDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	require.NoError(t, err)

	return d
}

func validTemplateAttrs(t *testing.T) TemplateAttributes {
	t.Helper()

	return TemplateAttributes{
		DiscountKindName: "rate",
		DiscountValue:    newTestDecimal(t, "0.10"),
		ScopeKindName:    "all",
		ExpiresAt:        testCouponExpiresAt,
	}
}

func newTestTemplate(t *testing.T) Template {
	t.Helper()
	tmpl, err := NewTemplate(validTemplateAttrs(t))
	require.NoError(t, err)

	return tmpl
}

func validCampaignArgs(t *testing.T) Attributes {
	t.Helper()

	code, err := NewCode("WELCOME-2026")
	require.NoError(t, err)

	return Attributes{
		Code:         code,
		Template:     newTestTemplate(t),
		StartsAt:     testStartsAt,
		EndsAt:       testEndsAt,
		TotalLimit:   10,
		PerUserLimit: 2,
	}
}

func newTestCampaign(t *testing.T) *Campaign {
	t.Helper()
	c, err := New(newTestUUID(t), validCampaignArgs(t))
	require.NoError(t, err)

	return c
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("全属性が有効な場合、1枚も配っておらず停止していないキャンペーンを生成する", func(t *testing.T) {
			t.Parallel()

			id := newTestUUID(t)
			attrs := validCampaignArgs(t)

			got, err := New(id, attrs)

			require.NoError(t, err)
			assert.Equal(t, id, got.ID())
			assert.Equal(t, attrs.Code, got.Code())
			assert.Equal(t, attrs.StartsAt, got.StartsAt())
			assert.Equal(t, attrs.EndsAt, got.EndsAt())
			assert.Equal(t, attrs.TotalLimit, got.TotalLimit())
			assert.Equal(t, attrs.PerUserLimit, got.PerUserLimit())
			assert.Zero(t, got.IssuedCount())
			assert.False(t, got.IsSuspended())
		})

		t.Run("1人あたり上限が総枚数上限と同じ場合を認める", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.PerUserLimit = attrs.TotalLimit

			_, err := New(newTestUUID(t), attrs)

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("IDが未設定の場合、ErrInvalidIDを返す", func(t *testing.T) {
			t.Parallel()

			_, err := New(uuid.UUID{}, validCampaignArgs(t))

			require.ErrorIs(t, err, ErrInvalidID)
		})

		t.Run("コードが未設定の場合、ErrInvalidCodeを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.Code = Code{}

			_, err := New(newTestUUID(t), attrs)

			require.ErrorIs(t, err, ErrInvalidCode)
		})

		t.Run("テンプレートが未設定の場合、ErrInvalidTemplateを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.Template = Template{}

			_, err := New(newTestUUID(t), attrs)

			require.ErrorIs(t, err, ErrInvalidTemplate)
		})
	})
}

func TestReconstruct(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("発行済み枚数と停止日時を復元する", func(t *testing.T) {
			t.Parallel()

			suspendedAt := testStartsAt.Add(time.Hour)

			got, err := Reconstruct(newTestUUID(t), validCampaignArgs(t), 3, &suspendedAt)

			require.NoError(t, err)
			assert.Equal(t, 3, got.IssuedCount())
			require.NotNil(t, got.SuspendedAt())
			assert.Equal(t, suspendedAt, *got.SuspendedAt())
		})

		t.Run("総枚数上限ちょうどまで配り終えた行を復元できる", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)

			got, err := Reconstruct(newTestUUID(t), attrs, attrs.TotalLimit, nil)

			require.NoError(t, err)
			assert.Equal(t, attrs.TotalLimit, got.IssuedCount())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("発行済み枚数が負の場合、ErrInvalidIssuedCountを返す", func(t *testing.T) {
			t.Parallel()

			_, err := Reconstruct(newTestUUID(t), validCampaignArgs(t), -1, nil)

			require.ErrorIs(t, err, ErrInvalidIssuedCount)
		})

		t.Run("発行済み枚数が総枚数上限を超える場合、ErrInvalidIssuedCountを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)

			_, err := Reconstruct(newTestUUID(t), attrs, attrs.TotalLimit+1, nil)

			require.ErrorIs(t, err, ErrInvalidIssuedCount)
		})
	})
}

func Test_validatePeriod(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("配布期間が順で、クーポンの有効期限が終了より後ならエラーにならない", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validatePeriod(validCampaignArgs(t)))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("開始日時がゼロ値の場合、ErrInvalidPeriodを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.StartsAt = time.Time{}

			require.ErrorIs(t, validatePeriod(attrs), ErrInvalidPeriod)
		})

		t.Run("終了日時がゼロ値の場合、ErrInvalidPeriodを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.EndsAt = time.Time{}

			require.ErrorIs(t, validatePeriod(attrs), ErrInvalidPeriod)
		})

		t.Run("終了日時が開始日時ちょうどの場合、ErrInvalidPeriodを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.EndsAt = attrs.StartsAt

			require.ErrorIs(t, validatePeriod(attrs), ErrInvalidPeriod)
		})

		t.Run("クーポンの有効期限が配布期間の終了ちょうどの場合、ErrInvalidCouponExpiresAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.EndsAt = attrs.Template.ExpiresAt()

			require.ErrorIs(t, validatePeriod(attrs), ErrInvalidCouponExpiresAt)
		})

		t.Run("クーポンの有効期限が配布期間の終了より前の場合、ErrInvalidCouponExpiresAtを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.EndsAt = attrs.Template.ExpiresAt().Add(time.Nanosecond)

			require.ErrorIs(t, validatePeriod(attrs), ErrInvalidCouponExpiresAt)
		})
	})
}

func Test_validateLimits(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("上限がどちらも正で、1人あたり上限が総枚数上限以下ならエラーにならない", func(t *testing.T) {
			t.Parallel()

			require.NoError(t, validateLimits(validCampaignArgs(t), 0))
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("総枚数上限が0の場合、ErrInvalidTotalLimitを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.TotalLimit = 0

			require.ErrorIs(t, validateLimits(attrs, 0), ErrInvalidTotalLimit)
		})

		t.Run("1人あたり上限が0の場合、ErrInvalidPerUserLimitを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.PerUserLimit = 0

			require.ErrorIs(t, validateLimits(attrs, 0), ErrInvalidPerUserLimit)
		})

		t.Run("1人あたり上限が総枚数上限を超える場合、ErrInvalidPerUserLimitを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			attrs.PerUserLimit = attrs.TotalLimit + 1

			require.ErrorIs(t, validateLimits(attrs, 0), ErrInvalidPerUserLimit)
		})
	})
}

func TestCampaign_IsDistributing(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("配布期間の開始ちょうどはtrueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, newTestCampaign(t).IsDistributing(testStartsAt))
		})

		t.Run("配布期間の開始より前はfalseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, newTestCampaign(t).IsDistributing(testStartsAt.Add(-time.Nanosecond)))
		})

		t.Run("配布期間の終了ちょうどはfalseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, newTestCampaign(t).IsDistributing(testEndsAt))
		})

		t.Run("配布期間の終了直前はtrueを返す", func(t *testing.T) {
			t.Parallel()

			assert.True(t, newTestCampaign(t).IsDistributing(testEndsAt.Add(-time.Nanosecond)))
		})

		t.Run("停止済みの場合は期間内でもfalseを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)
			require.NoError(t, c.Suspend(testStartsAt))

			assert.False(t, c.IsDistributing(testStartsAt))
		})

		t.Run("総枚数上限に達している場合は期間内でもfalseを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			c, err := Reconstruct(newTestUUID(t), attrs, attrs.TotalLimit, nil)
			require.NoError(t, err)

			assert.False(t, c.IsDistributing(testStartsAt))
		})
	})
}

func TestCampaign_Claim(t *testing.T) {
	t.Parallel()

	within := testStartsAt.Add(time.Hour)

	// claimAt は、受け取り日時と既受け取り枚数だけを変える入力を組み立てます。
	claimAt := func(t *testing.T, at time.Time, claimedByUser int) ClaimParams {
		t.Helper()

		return ClaimParams{
			ClaimedAt:     at,
			ClaimedByUser: claimedByUser,
			ClaimID:       newTestUUID(t),
			UserID:        newTestUUID(t),
			CouponID:      newTestUUID(t),
		}
	}

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("受け取りを受け付けて発行済み枚数を1増やす", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			got, err := c.Claim(claimAt(t, within, 0))

			require.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, 1, c.IssuedCount())
		})

		t.Run("受け取りの事実を返し、キャンペーンと受け取り日時が一致する", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)
			params := claimAt(t, within, 0)

			got, err := c.Claim(params)

			require.NoError(t, err)
			assert.Equal(t, params.ClaimID, got.ID())
			assert.Equal(t, c.ID(), got.CampaignID())
			assert.Equal(t, params.UserID, got.UserID())
			assert.Equal(t, params.CouponID, got.CouponID())
			assert.Equal(t, within, got.ClaimedAt())
		})

		t.Run("総枚数上限の最後の1枚を配れる", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			c, err := Reconstruct(newTestUUID(t), attrs, attrs.TotalLimit-1, nil)
			require.NoError(t, err)

			_, err = c.Claim(claimAt(t, within, 0))

			require.NoError(t, err)
			assert.Equal(t, attrs.TotalLimit, c.IssuedCount())
		})

		t.Run("1人あたり上限の最後の1枚を配れる", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			_, err := c.Claim(claimAt(t, within, c.PerUserLimit()-1))

			require.NoError(t, err)
			assert.Equal(t, 1, c.IssuedCount())
		})

		t.Run("配布期間の開始ちょうどに受け取れる", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			_, err := c.Claim(claimAt(t, testStartsAt, 0))

			require.NoError(t, err)
		})

		t.Run("配布期間の終了直前に受け取れる", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			_, err := c.Claim(claimAt(t, testEndsAt.Add(-time.Nanosecond), 0))

			require.NoError(t, err)
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止済みの場合はErrSuspendedを返し発行済み枚数を変えない", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)
			require.NoError(t, c.Suspend(within))

			got, err := c.Claim(claimAt(t, within, 0))

			require.ErrorIs(t, err, ErrSuspended)
			assert.Nil(t, got)
			assert.Zero(t, c.IssuedCount())
		})

		t.Run("配布期間の開始より前の場合はErrNotDistributingを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			got, err := c.Claim(claimAt(t, testStartsAt.Add(-time.Nanosecond), 0))

			require.ErrorIs(t, err, ErrNotDistributing)
			assert.Nil(t, got)
			assert.Zero(t, c.IssuedCount())
		})

		t.Run("配布期間の終了ちょうどの場合はErrNotDistributingを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			got, err := c.Claim(claimAt(t, testEndsAt, 0))

			require.ErrorIs(t, err, ErrNotDistributing)
			assert.Nil(t, got)
			assert.Zero(t, c.IssuedCount())
		})

		t.Run("総枚数上限に達している場合はErrTotalLimitReachedを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			c, err := Reconstruct(newTestUUID(t), attrs, attrs.TotalLimit, nil)
			require.NoError(t, err)

			got, err := c.Claim(claimAt(t, within, 0))

			require.ErrorIs(t, err, ErrTotalLimitReached)
			assert.Nil(t, got)
			assert.Equal(t, attrs.TotalLimit, c.IssuedCount())
		})

		t.Run("1人あたり上限に達している場合はErrPerUserLimitReachedを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			got, err := c.Claim(claimAt(t, within, c.PerUserLimit()))

			require.ErrorIs(t, err, ErrPerUserLimitReached)
			assert.Nil(t, got)
			assert.Zero(t, c.IssuedCount())
		})

		t.Run("総枚数上限は1人あたり上限より先に判定する", func(t *testing.T) {
			t.Parallel()

			// どちらの上限にも達している状態で、キャンペーン全体の理由が優先されることを固定する。
			attrs := validCampaignArgs(t)
			c, err := Reconstruct(newTestUUID(t), attrs, attrs.TotalLimit, nil)
			require.NoError(t, err)

			_, err = c.Claim(claimAt(t, within, attrs.PerUserLimit))

			require.ErrorIs(t, err, ErrTotalLimitReached)
		})

		t.Run("既受け取り枚数が負の場合はErrInvalidClaimedByUserを返す", func(t *testing.T) {
			t.Parallel()

			// 上限の最終防衛を集約の外へ出さないための防御。呼び出し元は COUNT の結果を渡すため
			// 通常は非負だが、ドメインはその値を無条件には信じない。
			c := newTestCampaign(t)

			got, err := c.Claim(claimAt(t, within, -1))

			require.ErrorIs(t, err, ErrInvalidClaimedByUser)
			assert.Nil(t, got)
			assert.Zero(t, c.IssuedCount())
		})

		t.Run("受け取り記録の識別子が未設定の場合は状態を変えない", func(t *testing.T) {
			t.Parallel()

			// 記録を生めないなら発行済み枚数も増えない（遷移と記録が同時に成立する）。
			c := newTestCampaign(t)
			params := claimAt(t, within, 0)
			params.CouponID = uuid.UUID{}

			got, err := c.Claim(params)

			require.ErrorIs(t, err, ErrInvalidCouponID)
			assert.Nil(t, got)
			assert.Zero(t, c.IssuedCount())
		})
	})
}

func TestCampaign_Suspend(t *testing.T) {
	t.Parallel()

	within := testStartsAt.Add(time.Hour)

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止日時を刻む", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)

			require.NoError(t, c.Suspend(within))

			assert.True(t, c.IsSuspended())
			require.NotNil(t, c.SuspendedAt())
			assert.Equal(t, within, *c.SuspendedAt())
		})
	})

	t.Run("異常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止済みの場合はErrAlreadySuspendedを返し停止日時を変えない", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)
			require.NoError(t, c.Suspend(within))

			err := c.Suspend(within.Add(time.Hour))

			require.ErrorIs(t, err, ErrAlreadySuspended)
			require.NotNil(t, c.SuspendedAt())
			assert.Equal(t, within, *c.SuspendedAt())
		})
	})
}

func TestCampaign_SuspendedAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止していない場合はnilを返す", func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, newTestCampaign(t).SuspendedAt())
		})

		t.Run("返した値を書き換えてもキャンペーンの停止日時は変わらない", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)
			require.NoError(t, c.Suspend(testStartsAt))

			got := c.SuspendedAt()
			require.NotNil(t, got)
			*got = testEndsAt

			require.NotNil(t, c.SuspendedAt())
			assert.Equal(t, testStartsAt, *c.SuspendedAt())
		})
	})
}

func TestCampaign_ID(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時に渡したIDを返す", func(t *testing.T) {
			t.Parallel()

			id := newTestUUID(t)
			c, err := New(id, validCampaignArgs(t))
			require.NoError(t, err)

			assert.Equal(t, id, c.ID())
		})
	})
}

func TestCampaign_Code(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時に渡したコードを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, attrs.Code, c.Code())
		})
	})
}

func TestCampaign_Template(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成時に渡したテンプレートを返す", func(t *testing.T) {
			t.Parallel()

			attrs := validCampaignArgs(t)
			c, err := New(newTestUUID(t), attrs)
			require.NoError(t, err)

			assert.Equal(t, attrs.Template, c.Template())
		})
	})
}

func TestCampaign_StartsAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("配布期間の開始日時を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testStartsAt, newTestCampaign(t).StartsAt())
		})
	})
}

func TestCampaign_EndsAt(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("配布期間の終了日時を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testEndsAt, newTestCampaign(t).EndsAt())
		})
	})
}

func TestCampaign_TotalLimit(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("総枚数上限を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 10, newTestCampaign(t).TotalLimit())
		})
	})
}

func TestCampaign_PerUserLimit(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("1人あたり上限を返す", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, 2, newTestCampaign(t).PerUserLimit())
		})
	})
}

func TestCampaign_IssuedCount(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("生成直後は0を返す", func(t *testing.T) {
			t.Parallel()

			assert.Zero(t, newTestCampaign(t).IssuedCount())
		})
	})
}

func TestCampaign_IsSuspended(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("停止していない場合はfalseを返す", func(t *testing.T) {
			t.Parallel()

			assert.False(t, newTestCampaign(t).IsSuspended())
		})

		t.Run("停止済みの場合はtrueを返す", func(t *testing.T) {
			t.Parallel()

			c := newTestCampaign(t)
			require.NoError(t, c.Suspend(testStartsAt))

			assert.True(t, c.IsSuspended())
		})
	})
}

func Test_newCampaign(t *testing.T) {
	t.Parallel()

	t.Run("正常系", func(t *testing.T) {
		t.Parallel()

		t.Run("渡した停止日時を書き換えてもキャンペーンの状態は変わらない", func(t *testing.T) {
			t.Parallel()

			suspendedAt := testStartsAt
			c, err := newCampaign(newTestUUID(t), validCampaignArgs(t), 0, &suspendedAt)
			require.NoError(t, err)

			suspendedAt = testEndsAt

			require.NotNil(t, c.SuspendedAt())
			assert.Equal(t, testStartsAt, *c.SuspendedAt())
		})
	})
}
