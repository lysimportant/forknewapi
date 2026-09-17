package model

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 邀请奖励账本的回归测试。同一组用例在三个数据库上执行：SQLite 使用临时磁盘文件，
// MySQL / PostgreSQL 通过 TEST_MYSQL_DSN / TEST_POSTGRES_DSN 指向真实实例，
// DSN 缺失时才跳过（跳过不算通过）。
const (
	inviteRewardTestQuotaPerUnit = 500000
	inviteRewardTestFreeze       = int64(48 * 60 * 60)
)

type inviteRewardTestDB struct {
	db *gorm.DB
}

// openInviteRewardTestDB 为指定方言建立独立连接并切换到账本所需的表结构。
func openInviteRewardTestDB(t *testing.T, dialect string) *inviteRewardTestDB {
	t.Helper()
	if dialect == "sqlite" {
		previousPath := common.SQLitePath
		common.SQLitePath = filepath.Join(t.TempDir(), "invite-reward.db")
		t.Cleanup(func() { common.SQLitePath = previousPath })
	}

	dsn := "local"
	switch dialect {
	case "mysql":
		dsn = strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
	case "postgres":
		dsn = strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
	}
	t.Setenv("INVITE_REWARD_TEST_DSN", dsn)

	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { common.SetDatabaseTypes(previousMain, previousLog) })

	db, dbType, err := chooseDB("INVITE_REWARD_TEST_DSN", false)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if dialect == "sqlite" {
		// SQLite 单写者模型：限制为一个连接，使并发提现串行化，而不是触发
		// database is locked 的驱动级错误。
		sqlDB.SetMaxOpenConns(1)
	}

	previousDB, previousLOGDB := DB, LOG_DB
	DB, LOG_DB = db, db
	t.Cleanup(func() { DB, LOG_DB = previousDB, previousLOGDB })

	previousFreeze := common.InviteRewardFreezeHours
	previousQuotaPerUnit := common.QuotaPerUnit
	previousQuotaForInviter, previousQuotaForInvitee := common.QuotaForInviter, common.QuotaForInvitee
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance, previousTermsVersion := paymentSetting.ComplianceConfirmed, paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		common.InviteRewardFreezeHours = previousFreeze
		common.QuotaPerUnit = previousQuotaPerUnit
		common.QuotaForInviter, common.QuotaForInvitee = previousQuotaForInviter, previousQuotaForInvitee
		paymentSetting.ComplianceConfirmed = previousCompliance
		paymentSetting.ComplianceTermsVersion = previousTermsVersion
	})
	common.InviteRewardFreezeHours = 48
	common.QuotaPerUnit = inviteRewardTestQuotaPerUnit
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	common.SetMainDatabaseType(dbType)
	common.SetLogDatabaseType(dbType)
	initCol()
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &InviteReward{}, &InviteRewardWithdrawal{}))
	for _, table := range []string{"invite_reward_withdrawals", "invite_rewards", "logs", "users"} {
		require.NoError(t, db.Exec("DELETE FROM "+table).Error)
	}

	return &inviteRewardTestDB{db: db}
}

func newInviteRewardTestUser(t *testing.T, quota, affQuota int) *User {
	t.Helper()
	user := &User{
		Username:    "invite-user-" + common.GetRandomString(8),
		AffCode:     common.GetRandomString(4),
		Status:      common.UserStatusEnabled,
		Quota:       quota,
		AffQuota:    affQuota,
		AuthVersion: 1,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

// seedInviteReward 直接写入账本记录并同步汇总字段，用于构造确定的产生时间与冻结状态。
// 走的是与生产发放相同的 ClaimInviteReward + 汇总更新组合。
func seedInviteReward(t *testing.T, userId int, kind string, quota int, createdAt int64) *InviteReward {
	t.Helper()
	sourceKey := inviteRewardSourceKey(kind, userId) + "-" + common.GetRandomString(6)
	claimed, err := ClaimInviteReward(DB, userId, sourceKey, kind, quota, createdAt)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
		"aff_quota":   gorm.Expr("aff_quota + ?", quota),
		"aff_history": gorm.Expr("aff_history + ?", quota),
	}).Error)
	var reward InviteReward
	require.NoError(t, DB.Where("user_id = ? AND source_key = ?", userId, sourceKey).First(&reward).Error)
	return &reward
}

func reloadInviteRewardTestUser(t *testing.T, userId int) *User {
	t.Helper()
	var user User
	require.NoError(t, DB.Where("id = ?", userId).First(&user).Error)
	return &user
}

// restoreInviteReward 把账本与汇总恢复到“未提取”状态，用于在一个用例里串联多次
// 失败尝试后的成功路径；失败的提现必须已经完整回滚，这里只重置被成功路径消耗的状态。
func restoreInviteReward(t *testing.T, userId int, rewardId int, quota int) {
	t.Helper()
	require.NoError(t, DB.Model(&InviteReward{}).Where("id = ?", rewardId).
		Update("remaining_quota", quota).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userId).
		Updates(map[string]interface{}{"quota": 0, "aff_quota": quota}).Error)
}

// TestInviteRewardGrantIsIdempotentPerSource 覆盖普通注册、OAuth 注册与重复回调
// 只能产生一份奖励，且受邀人赠送不顺带改变冻结账本。
func TestInviteRewardGrantIsIdempotentPerSource(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			common.QuotaForInviter = 250
			common.QuotaForInvitee = 100

			inviter := newInviteRewardTestUser(t, 0, 0)
			invitee := newInviteRewardTestUser(t, 0, 0)

			require.NoError(t, invitee.grantInviteRewards(inviter.Id))
			// 重复回调 / 重复注册流程：第二次调用必须是幂等空操作。
			require.NoError(t, invitee.grantInviteRewards(inviter.Id))

			inviterAfter := reloadInviteRewardTestUser(t, inviter.Id)
			assert.Equal(t, 250, inviterAfter.AffQuota)
			assert.Equal(t, 250, inviterAfter.AffHistoryQuota)
			assert.Equal(t, 1, inviterAfter.AffCount)

			inviteeAfter := reloadInviteRewardTestUser(t, invitee.Id)
			assert.Equal(t, 100, inviteeAfter.Quota, "受邀人赠送直接计入余额")
			assert.Zero(t, inviteeAfter.AffQuota, "受邀人赠送不进入邀请人冻结账本")
			inviteeSummary, err := GetInviteRewardSummary(invitee.Id)
			require.NoError(t, err)
			assert.Zero(t, inviteeSummary.FrozenQuota)
			assert.Zero(t, inviteeSummary.WithdrawableQuota)
			assert.Empty(t, inviteeSummary.Rewards, "受邀人赠送来源不展示为邀请人奖励")

			var rewards []InviteReward
			require.NoError(t, DB.Where("user_id = ?", inviter.Id).Find(&rewards).Error)
			require.Len(t, rewards, 1)
			assert.Equal(t, InviteRewardKindInviter, rewards[0].Kind)
			assert.Equal(t, 250, rewards[0].Quota)
			assert.Equal(t, 250, rewards[0].RemainingQuota)

			// 汇总与账本必须一致：剩余额度之和等于 aff_quota。
			var remaining int
			require.NoError(t, DB.Model(&InviteReward{}).Where("user_id = ?", inviter.Id).
				Select("COALESCE(SUM(remaining_quota), 0)").Scan(&remaining).Error)
			assert.Equal(t, inviterAfter.AffQuota, remaining)

			// 受邀人赠送同样按来源去重，不会重复加款。
			var inviteeRewards []InviteReward
			require.NoError(t, DB.Where("user_id = ?", invitee.Id).Find(&inviteeRewards).Error)
			require.Len(t, inviteeRewards, 1)
			assert.Equal(t, InviteRewardKindInvitee, inviteeRewards[0].Kind)
		})
	}
}

// TestInviteRewardFreezeBoundary 覆盖 47:59:59 拒绝、48:00:00 允许，以及改变本地时间
// 不能提前提现（服务端时间才是唯一判据）。
func TestInviteRewardFreezeBoundary(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			user := newInviteRewardTestUser(t, inviteRewardTestQuotaPerUnit, 0)
			createdAt := int64(1_800_000_000)
			seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, createdAt)
			claimed, err := ClaimInviteReward(DB, user.Id, inviteRewardSourceKey(InviteRewardKindInvitee, user.Id), InviteRewardKindInvitee, inviteRewardTestQuotaPerUnit, createdAt-inviteRewardTestFreeze)
			require.NoError(t, err)
			require.True(t, claimed)

			beforeDeadline := createdAt + inviteRewardTestFreeze - 1
			_, err = WithdrawInviteRewardsAt(user.Id, "boundary-early", inviteRewardTestQuotaPerUnit, beforeDeadline)
			var frozen *InviteRewardFrozenError
			require.Error(t, err)
			require.True(t, errors.As(err, &frozen), "冻结期内的提现必须报告最近可提现时间")
			assert.Equal(t, createdAt+inviteRewardTestFreeze, frozen.NextAvailableAt)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, reloadInviteRewardTestUser(t, user.Id).Quota, "被拒绝的提现不得改变已经入账的赠送余额")

			atDeadline := createdAt + inviteRewardTestFreeze
			quota, err := WithdrawInviteRewardsAt(user.Id, "boundary-exact", inviteRewardTestQuotaPerUnit, atDeadline)
			require.NoError(t, err)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, quota)

			after := reloadInviteRewardTestUser(t, user.Id)
			assert.Equal(t, 2*inviteRewardTestQuotaPerUnit, after.Quota)
			assert.Zero(t, after.AffQuota)

			// 到期本身不会自动划转：未点击提现时余额保持不变。
			var reward InviteReward
			require.NoError(t, DB.Where("user_id = ? AND kind = ?", user.Id, InviteRewardKindInviter).First(&reward).Error)
			assert.Zero(t, reward.RemainingQuota)
			var gifted InviteReward
			require.NoError(t, DB.Where("user_id = ? AND kind = ?", user.Id, InviteRewardKindInvitee).First(&gifted).Error)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, gifted.RemainingQuota, "提现不得消耗赠送去重记录")
		})
	}
}

// TestInviteRewardWithdrawAllocatesPerReward 覆盖多笔不同时间分别解冻、
// A 提现不影响 B，以及固定扣减顺序（到期时间、记录 ID 升序）。
func TestInviteRewardWithdrawAllocatesPerReward(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			user := newInviteRewardTestUser(t, 0, 0)
			// 以服务端当前时间为基准，使两笔奖励都处于冻结期，再由显式 now 推进时间。
			base := common.GetTimestamp()
			// A 先到期，B 后到期；金额相同以便观察扣减顺序。
			rewardA := seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, base)
			rewardB := seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, base+7200)

			// 只提取一笔：必须命中先到期的 A。
			_, err := WithdrawInviteRewardsAt(user.Id, "allocate-a", inviteRewardTestQuotaPerUnit, base+inviteRewardTestFreeze+1)
			require.NoError(t, err)

			var gotA, gotB InviteReward
			require.NoError(t, DB.Where("id = ?", rewardA.Id).First(&gotA).Error)
			require.NoError(t, DB.Where("id = ?", rewardB.Id).First(&gotB).Error)
			assert.Zero(t, gotA.RemainingQuota, "固定顺序应先扣减先到期的 A")
			assert.Equal(t, inviteRewardTestQuotaPerUnit, gotB.RemainingQuota, "A 的提现不得影响仍冻结的 B")

			afterFirst := reloadInviteRewardTestUser(t, user.Id)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, afterFirst.Quota)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, afterFirst.AffQuota)

			summary, err := GetInviteRewardSummary(user.Id)
			require.NoError(t, err)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, summary.FrozenQuota)
			assert.Zero(t, summary.WithdrawableQuota)
			assert.Equal(t, base+7200+inviteRewardTestFreeze, summary.NextAvailableAt)

			// B 到期后可以继续提取，冻结与可提金额随之更新。
			_, err = WithdrawInviteRewardsAt(user.Id, "allocate-b", inviteRewardTestQuotaPerUnit, base+7200+inviteRewardTestFreeze)
			require.NoError(t, err)
			final := reloadInviteRewardTestUser(t, user.Id)
			assert.Equal(t, 2*inviteRewardTestQuotaPerUnit, final.Quota)
			assert.Zero(t, final.AffQuota)

			summary, err = GetInviteRewardSummary(user.Id)
			require.NoError(t, err)
			assert.Zero(t, summary.FrozenQuota)
			assert.Zero(t, summary.WithdrawableQuota)
			assert.Equal(t, 2*inviteRewardTestQuotaPerUnit, summary.WithdrawnQuota)
		})
	}
}

// TestInviteRewardWithdrawIdempotentAndRejectsInvalidInput 覆盖重复提交、请求重放、
// 两个会话并发只入账一次，以及越权、零、负数、超可提额和钱包上限。
func TestInviteRewardWithdrawIdempotentAndRejectsInvalidInput(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			// 余额只差 1 就触顶：任何入账都会越过钱包上限。
			user := newInviteRewardTestUser(t, common.MaxWalletQuota-1, 0)
			other := newInviteRewardTestUser(t, 0, 0)
			base := common.GetTimestamp() - inviteRewardTestFreeze
			reward := seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, base)
			mature := base + inviteRewardTestFreeze

			// 缺少幂等标识：明确拒绝且不扣账。
			_, err := WithdrawInviteRewards(user.Id, "   ", inviteRewardTestQuotaPerUnit)
			assert.ErrorIs(t, err, ErrInviteRewardIdempotencyKeyMissing)

			// 零与负数在最小额度校验之前就被拒绝。
			_, err = WithdrawInviteRewardsAt(user.Id, "invalid-zero", 0, mature)
			assert.ErrorIs(t, err, ErrInviteRewardQuotaNotPositive)
			_, err = WithdrawInviteRewardsAt(user.Id, "invalid-negative", -inviteRewardTestQuotaPerUnit, mature)
			assert.ErrorIs(t, err, ErrInviteRewardQuotaNotPositive)

			// 低于最低额度（1 个基础货币单位）被拒绝。
			_, err = WithdrawInviteRewardsAt(user.Id, "invalid-minimum", inviteRewardTestQuotaPerUnit-1, mature)
			require.Error(t, err)

			// 超出可提额度被拒绝，且账本保持不变。
			_, err = WithdrawInviteRewardsAt(user.Id, "invalid-overdraw", 2*inviteRewardTestQuotaPerUnit, mature)
			assert.ErrorIs(t, err, ErrInviteRewardQuotaInsufficient)

			// 钱包上限：入账会越过上限，整笔回滚。
			_, err = WithdrawInviteRewardsAt(user.Id, "invalid-wallet-cap", inviteRewardTestQuotaPerUnit, mature)
			assert.ErrorIs(t, err, ErrWalletQuotaLimitExceeded)
			require.NoError(t, DB.Model(&InviteReward{}).Where("id = ?", reward.Id).First(reward).Error)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, reward.RemainingQuota, "失败的提现不得扣减账本")

			// 另一个用户的账本不受影响，也不能被本次操作读取。
			otherSummary, err := GetInviteRewardSummary(other.Id)
			require.NoError(t, err)
			assert.Zero(t, otherSummary.AffQuota)
			assert.Empty(t, otherSummary.Rewards)

			require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 0).Error)
			restoreInviteReward(t, user.Id, reward.Id, inviteRewardTestQuotaPerUnit)

			// 首次成功入账。
			first, err := WithdrawInviteRewards(user.Id, "replay-key", inviteRewardTestQuotaPerUnit)
			require.NoError(t, err)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, first)

			// 相同 requestId 重放：返回相同结果且不重复记款。
			replay, err := WithdrawInviteRewards(user.Id, "replay-key", inviteRewardTestQuotaPerUnit)
			require.NoError(t, err)
			assert.Equal(t, first, replay)

			afterReplay := reloadInviteRewardTestUser(t, user.Id)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, afterReplay.Quota)
			assert.Zero(t, afterReplay.AffQuota)

			var withdrawalCount int64
			require.NoError(t, DB.Model(&InviteRewardWithdrawal{}).Where("user_id = ?", user.Id).Count(&withdrawalCount).Error)
			assert.Equal(t, int64(1), withdrawalCount)
		})
	}
}

// TestInviteRewardConcurrentWithdrawCreditsOnce 覆盖两个会话并发提交同一笔奖励：
// 只允许一次有效入账，账本不会被扣成负数。
func TestInviteRewardConcurrentWithdrawCreditsOnce(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			user := newInviteRewardTestUser(t, 0, 0)
			base := common.GetTimestamp() - inviteRewardTestFreeze
			reward := seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, base)
			mature := base + inviteRewardTestFreeze

			start := make(chan struct{})
			var wg sync.WaitGroup
			results := make([]error, 2)
			for i := range results {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					<-start
					_, results[index] = WithdrawInviteRewardsAt(
						user.Id,
						"concurrent-"+common.GetRandomString(8),
						inviteRewardTestQuotaPerUnit,
						mature,
					)
				}(i)
			}
			close(start)
			wg.Wait()

			succeeded := 0
			for _, err := range results {
				if err == nil {
					succeeded++
				}
			}
			assert.Equal(t, 1, succeeded, "并发提现只能有一次成功")

			final := reloadInviteRewardTestUser(t, user.Id)
			assert.Equal(t, inviteRewardTestQuotaPerUnit, final.Quota, "余额只能增加一次")
			assert.Zero(t, final.AffQuota)

			var got InviteReward
			require.NoError(t, DB.Where("id = ?", reward.Id).First(&got).Error)
			assert.Zero(t, got.RemainingQuota)
			assert.GreaterOrEqual(t, got.RemainingQuota, 0, "账本不得被扣成负数")

			var count int64
			require.NoError(t, DB.Model(&InviteRewardWithdrawal{}).Where("user_id = ?", user.Id).Count(&count).Error)
			assert.Equal(t, int64(1), count)
		})
	}
}

// TestInitializeInviteRewardLedgerIsIdempotent 覆盖历史结转：只生成一笔、重复启动
// 不生成第二笔、剩余可提金额与迁移前汇总一致。
func TestInitializeInviteRewardLedgerIsIdempotent(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			legacy := newInviteRewardTestUser(t, 0, 3*inviteRewardTestQuotaPerUnit)
			zero := newInviteRewardTestUser(t, 0, 0)
			frozen := newInviteRewardTestUser(t, 0, 0)
			seedInviteReward(t, frozen.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, common.GetTimestamp())

			require.NoError(t, InitializeInviteRewardLedger())
			require.NoError(t, InitializeInviteRewardLedger())

			var carryovers []InviteReward
			require.NoError(t, DB.Where("user_id = ?", legacy.Id).Find(&carryovers).Error)
			require.Len(t, carryovers, 1)
			assert.Equal(t, InviteRewardKindHistory, carryovers[0].Kind)
			assert.Equal(t, 3*inviteRewardTestQuotaPerUnit, carryovers[0].RemainingQuota)

			var zeroCarryovers int64
			require.NoError(t, DB.Model(&InviteReward{}).Where("user_id = ?", zero.Id).Count(&zeroCarryovers).Error)
			assert.Zero(t, zeroCarryovers, "余额为零的用户无需结转")
			var frozenRewards []InviteReward
			require.NoError(t, DB.Where("user_id = ?", frozen.Id).Find(&frozenRewards).Error)
			require.Len(t, frozenRewards, 1, "已有冻结账本不得再次结转为可立即提现的历史余额")
			assert.Equal(t, InviteRewardKindInviter, frozenRewards[0].Kind)
			_, err := WithdrawInviteRewardsAt(frozen.Id, "restart-preserves-freeze", inviteRewardTestQuotaPerUnit, common.GetTimestamp())
			var frozenErr *InviteRewardFrozenError
			require.ErrorAs(t, err, &frozenErr)

			// 历史结转是可提现例外：产生即可提取，不受 48 小时冻结约束。
			summary, err := GetInviteRewardSummary(legacy.Id)
			require.NoError(t, err)
			assert.Equal(t, 3*inviteRewardTestQuotaPerUnit, summary.WithdrawableQuota)
			assert.Zero(t, summary.FrozenQuota)

			// 对账：迁移前后剩余推荐余额一致。
			var remaining int
			require.NoError(t, DB.Model(&InviteReward{}).Where("user_id = ?", legacy.Id).
				Select("COALESCE(SUM(remaining_quota), 0)").Scan(&remaining).Error)
			assert.Equal(t, legacy.AffQuota, remaining)
		})
	}
}

// TestInviteRewardFreezeHoursZeroDisablesFreeze 覆盖管理员把冻结时长配置为 0 的情况：
// 新奖励立即可提，不影响历史结转语义。
func TestInviteRewardFreezeHoursZeroDisablesFreeze(t *testing.T) {
	openInviteRewardTestDB(t, "sqlite")
	common.InviteRewardFreezeHours = 0
	user := newInviteRewardTestUser(t, 0, 0)
	seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, common.GetTimestamp())

	summary, err := GetInviteRewardSummary(user.Id)
	require.NoError(t, err)
	assert.Equal(t, inviteRewardTestQuotaPerUnit, summary.WithdrawableQuota)
	assert.Zero(t, summary.FrozenQuota)
	assert.Zero(t, summary.FreezeSeconds)
}

// TestInviteRewardLedgerSchemaIsStable 覆盖真实数据库上的账本表结构：
// 首次迁移建表，第二次迁移不得产生任何 DDL，历史数据保持可读，唯一索引真实生效。
func TestInviteRewardLedgerSchemaIsStable(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			handle := openInviteRewardTestDB(t, dialect)
			recorder := &migrationSQLRecorder{}
			db := handle.db.Session(&gorm.Session{Logger: recorder})
			t.Cleanup(func() { _ = handle.db.Migrator().DropTable(&InviteRewardWithdrawal{}, &InviteReward{}) })

			require.NoError(t, db.AutoMigrate(&InviteReward{}, &InviteRewardWithdrawal{}))
			for _, column := range []string{"user_id", "source_key", "kind", "quota", "remaining_quota", "created_at", "available_at"} {
				assert.True(t, db.Migrator().HasColumn(&InviteReward{}, column), "invite_rewards.%s 必须存在", column)
			}
			for _, column := range []string{"user_id", "idempotency_key", "quota", "allocations", "created_at"} {
				assert.True(t, db.Migrator().HasColumn(&InviteRewardWithdrawal{}, column), "invite_reward_withdrawals.%s 必须存在", column)
			}
			assert.True(t, db.Migrator().HasIndex(&InviteReward{}, "uk_invite_rewards_source"))
			assert.True(t, db.Migrator().HasIndex(&InviteRewardWithdrawal{}, "uk_invite_reward_withdrawals_request"))

			user := newInviteRewardTestUser(t, 0, 0)
			seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, common.GetTimestamp())

			// 第二次迁移必须是空操作，且不得改动既有数据。
			recorder.reset()
			require.NoError(t, db.AutoMigrate(&InviteReward{}, &InviteRewardWithdrawal{}))
			assert.Empty(t, recorder.schemaMutations(), "重复迁移不得重复执行 DDL")

			var readings []int
			require.NoError(t, db.Model(&InviteReward{}).Where("user_id = ?", user.Id).Pluck("quota", &readings).Error)
			assert.Equal(t, []int{inviteRewardTestQuotaPerUnit}, readings)
		})
	}
}
