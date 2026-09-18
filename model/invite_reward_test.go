package model

import (
	"errors"
	"fmt"
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
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &InviteReward{}, &InviteRewardWithdrawal{}, &TopUp{}, &Redemption{}))
	for _, table := range []string{"invite_reward_withdrawals", "invite_rewards", "top_ups", "redemptions", "logs", "users"} {
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
// 只计一次邀请人数；旧邀请人金额配置不再生效，受邀人赠送不改变冻结账本。
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
			assert.Zero(t, inviterAfter.AffQuota)
			assert.Zero(t, inviterAfter.AffHistoryQuota)
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
			assert.Zero(t, rewards[0].Quota)
			assert.Zero(t, rewards[0].RemainingQuota)
			inviterSummary, err := GetInviteRewardSummary(inviter.Id)
			require.NoError(t, err)
			assert.Empty(t, inviterSummary.Rewards, "零额度邀请计数不得展示为奖励")

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

// TestInviteRewardRegistrationPersistsInviter 验证普通及 OAuth 用户创建接口均持久化邀请关系，
// 注册只计人数，随后首次充值能通过已保存的直接邀请人发放返佣。
func TestInviteRewardRegistrationPersistsInviter(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			for _, mode := range []string{"password", "oauth"} {
				t.Run(mode, func(t *testing.T) {
					inviter := newInviteRewardTestUser(t, 0, 0)
					invitee := &User{Username: "join-" + common.GetRandomString(8), Status: common.UserStatusEnabled}
					if mode == "password" {
						invitee.Password = "Example-password-123!"
						require.NoError(t, invitee.Insert(inviter.Id))
					} else {
						require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return invitee.InsertWithTx(tx, inviter.Id) }))
						invitee.FinalizeOAuthUserCreation(inviter.Id)
					}
					assert.Equal(t, inviter.Id, reloadInviteRewardTestUser(t, invitee.Id).InviterId)
					afterRegister := reloadInviteRewardTestUser(t, inviter.Id)
					assert.Equal(t, 1, afterRegister.AffCount)
					assert.Zero(t, afterRegister.AffQuota)
					code := &Redemption{Key: common.GetRandomString(32), Name: mode, Quota: 10000, Status: common.RedemptionCodeStatusEnabled}
					require.NoError(t, code.Insert())
					_, err := Redeem(code.Key, invitee.Id)
					require.NoError(t, err)
					afterTopUp := reloadInviteRewardTestUser(t, inviter.Id)
					assert.Equal(t, 1000, afterTopUp.AffQuota)
					assert.Equal(t, 1, afterTopUp.AffCount)
				})
			}
		})
	}
}

// TestInviteRewardTopUpCreditsOnce 覆盖所有在线支付、补单和兑换码的真实到账路径：
// 按到账余额返佣、重复请求不重复记账、冻结期不可由旧配置取消，并验证精确解冻边界。
func TestInviteRewardTopUpCreditsOnce(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			common.InviteRewardFreezeHours = 0
			for _, provider := range []string{PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake, "manual", "redemption"} {
				t.Run(provider, func(t *testing.T) {
					inviter := newInviteRewardTestUser(t, 0, 0)
					invitee := newInviteRewardTestUser(t, 0, 0)
					require.NoError(t, DB.Model(invitee).Update("inviter_id", inviter.Id).Error)
					require.NoError(t, invitee.grantInviteRewards(inviter.Id))
					initialQuota := reloadInviteRewardTestUser(t, invitee.Id).Quota
					order := &TopUp{UserId: invitee.Id, Amount: 10, Money: 2, TradeNo: "reward-" + common.GetRandomString(16), PaymentProvider: provider, PaymentMethod: "test", Status: common.TopUpStatusPending}
					wantQuota := 5_000_000
					if provider == PaymentProviderStripe {
						wantQuota = 1_000_000
					}
					if provider == PaymentProviderCreem {
						order.Amount = int64(wantQuota)
					}
					if provider == "manual" {
						order.PaymentProvider = PaymentProviderEpay
					}
					var settle func() error
					var sourceKey string
					if provider == "redemption" {
						wantQuota = 5_000_009
						code := &Redemption{Key: common.GetRandomString(32), Name: "referral", Quota: wantQuota, Status: common.RedemptionCodeStatusEnabled}
						require.NoError(t, code.Insert())
						sourceKey = fmt.Sprintf("redemption:%d", code.Id)
						settle = func() error { _, err := Redeem(code.Key, invitee.Id); return err }
					} else {
						require.NoError(t, order.Insert())
						sourceKey = fmt.Sprintf("topup:%d", order.Id)
						settle = func() error {
							switch provider {
							case PaymentProviderEpay:
								_, err := RechargeEpay(order.TradeNo, "", "")
								return err
							case PaymentProviderStripe:
								return Recharge(order.TradeNo, "customer", "")
							case PaymentProviderCreem:
								return RechargeCreem(order.TradeNo, "", "", "")
							case PaymentProviderWaffo:
								return RechargeWaffo(order.TradeNo, "")
							case PaymentProviderWaffoPancake:
								return RechargeWaffoPancake(order.TradeNo)
							default:
								return ManualCompleteTopUp(order.TradeNo, "")
							}
						}
					}
					require.NoError(t, settle())
					// 供应商重复回调允许返回原有的已完成错误，但绝不能再次入账。
					_ = settle()
					assert.Equal(t, initialQuota+wantQuota, reloadInviteRewardTestUser(t, invitee.Id).Quota)
					inviterAfter := reloadInviteRewardTestUser(t, inviter.Id)
					assert.Equal(t, wantQuota/10, inviterAfter.AffQuota)
					assert.Equal(t, wantQuota/10, inviterAfter.AffHistoryQuota)
					assert.Equal(t, 1, inviterAfter.AffCount, "充值不增加邀请人数")
					assert.Zero(t, inviterAfter.Quota, "冻结奖励不能直接进入可消费余额")
					var rewards []InviteReward
					require.NoError(t, DB.Where("user_id = ? AND quota > 0", inviter.Id).Find(&rewards).Error)
					require.Len(t, rewards, 1)
					assert.Equal(t, sourceKey, rewards[0].SourceKey)
					assert.Equal(t, InviteRewardKindTopUp, rewards[0].Kind)
					assert.Equal(t, rewards[0].CreatedAt+172800, rewards[0].AvailableAt)
					summary, err := GetInviteRewardSummary(inviter.Id)
					require.NoError(t, err)
					assert.Equal(t, wantQuota/10, summary.FrozenQuota)
					assert.Zero(t, summary.WithdrawableQuota)
					common.QuotaPerUnit = 1
					_, err = WithdrawInviteRewardsAt(inviter.Id, "too-early", wantQuota/10, rewards[0].AvailableAt-1)
					var frozenErr *InviteRewardFrozenError
					require.ErrorAs(t, err, &frozenErr)
					withdrawn, err := WithdrawInviteRewardsAt(inviter.Id, "mature", wantQuota/10, rewards[0].AvailableAt)
					require.NoError(t, err)
					assert.Equal(t, wantQuota/10, withdrawn)
					assert.Equal(t, withdrawn, reloadInviteRewardTestUser(t, inviter.Id).Quota)
					common.QuotaPerUnit = inviteRewardTestQuotaPerUnit
				})
			}
		})
	}
}

// TestInviteRewardTopUpExclusions 验证缺失、禁用、自邀请及未确认条款不会产生返佣，
// 同时保护最小额度向下取整，不因零返佣拒绝合法充值。
func TestInviteRewardTopUpExclusions(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			for _, reason := range []string{"none", "deleted", "disabled", "self", "compliance", "rounding"} {
				t.Run(reason, func(t *testing.T) {
					inviter := newInviteRewardTestUser(t, 0, 0)
					invitee := newInviteRewardTestUser(t, 0, 0)
					inviterID := inviter.Id
					switch reason {
					case "none":
						inviterID = 0
					case "deleted":
						require.NoError(t, DB.Delete(inviter).Error)
					case "disabled":
						require.NoError(t, DB.Model(inviter).Update("status", common.UserStatusDisabled).Error)
					case "self":
						inviterID = invitee.Id
					case "compliance":
						operation_setting.GetPaymentSetting().ComplianceConfirmed = false
						t.Cleanup(func() { operation_setting.GetPaymentSetting().ComplianceConfirmed = true })
					}
					require.NoError(t, DB.Model(invitee).Update("inviter_id", inviterID).Error)
					quota := 500000
					if reason == "rounding" {
						quota = 9
					}
					code := &Redemption{Key: common.GetRandomString(32), Name: reason, Quota: quota, Status: common.RedemptionCodeStatusEnabled}
					require.NoError(t, code.Insert())
					got, err := Redeem(code.Key, invitee.Id)
					require.NoError(t, err)
					assert.Equal(t, quota, got)
					assert.Equal(t, quota, reloadInviteRewardTestUser(t, invitee.Id).Quota)
					var count int64
					require.NoError(t, DB.Model(&InviteReward{}).Where("source_key = ?", fmt.Sprintf("redemption:%d", code.Id)).Count(&count).Error)
					assert.Zero(t, count)
				})
			}
		})
	}
}

// TestInviteRewardTopUpRollback 验证邀请人余额或累计返佣超限时，订单、兑换码、充值与返佣全部回滚。
func TestInviteRewardTopUpRollback(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			for _, field := range []string{"aff_quota", "aff_history"} {
				t.Run(field, func(t *testing.T) {
					inviter := newInviteRewardTestUser(t, 0, 0)
					invitee := newInviteRewardTestUser(t, 0, 0)
					require.NoError(t, DB.Model(inviter).Update(field, common.MaxWalletQuota).Error)
					require.NoError(t, DB.Model(invitee).Update("inviter_id", inviter.Id).Error)
					order := &TopUp{UserId: invitee.Id, Amount: 10, TradeNo: common.GetRandomString(16), PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
					require.NoError(t, order.Insert())
					_, err := RechargeEpay(order.TradeNo, "", "")
					require.ErrorIs(t, err, ErrWalletQuotaLimitExceeded)
					assert.Equal(t, common.TopUpStatusPending, GetTopUpById(order.Id).Status)
					code := &Redemption{Key: common.GetRandomString(32), Name: "rollback", Quota: 1000, Status: common.RedemptionCodeStatusEnabled}
					require.NoError(t, code.Insert())
					_, err = Redeem(code.Key, invitee.Id)
					require.ErrorIs(t, err, ErrRedeemFailed)
					require.NoError(t, DB.First(code, code.Id).Error)
					assert.Equal(t, common.RedemptionCodeStatusEnabled, code.Status)
					assert.Zero(t, code.UsedUserId)
					assert.Zero(t, reloadInviteRewardTestUser(t, invitee.Id).Quota)
					var count int64
					require.NoError(t, DB.Model(&InviteReward{}).Where("user_id = ?", inviter.Id).Count(&count).Error)
					assert.Zero(t, count)
				})
			}
		})
	}
}

// TestInviteRewardMultipleTopUpsAndConcurrentReplay 验证同一邀请用户多笔充值逐单返佣，
// 并发重复回调只返一次，统计人数不随订单数增加。
func TestInviteRewardMultipleTopUpsAndConcurrentReplay(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			common.QuotaForInvitee = 100
			inviter := newInviteRewardTestUser(t, 0, 0)
			invitee := newInviteRewardTestUser(t, 0, 0)
			require.NoError(t, DB.Model(invitee).Update("inviter_id", inviter.Id).Error)
			initialQuota := reloadInviteRewardTestUser(t, invitee.Id).Quota
			for _, amount := range []int64{10, 20} {
				order := &TopUp{UserId: invitee.Id, Amount: amount, TradeNo: common.GetRandomString(16), PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
				require.NoError(t, order.Insert())
				// 模拟新账号已经创建、注册收尾尚未结束时的首次充值和重复注册回调。
				results := make([]error, 4)
				var wg sync.WaitGroup
				for i := range results {
					wg.Go(func() {
						if i < 2 {
							_, results[i] = RechargeEpay(order.TradeNo, "", "")
						} else {
							results[i] = invitee.grantInviteRewards(inviter.Id)
						}
					})
				}
				wg.Wait()
				for _, err := range results {
					require.NoError(t, err)
				}
			}
			assert.Equal(t, initialQuota+15_000_100, reloadInviteRewardTestUser(t, invitee.Id).Quota)
			after := reloadInviteRewardTestUser(t, inviter.Id)
			assert.Equal(t, 1_500_000, after.AffQuota)
			assert.Equal(t, 1_500_000, after.AffHistoryQuota)
			assert.Equal(t, 1, after.AffCount)
			var count int64
			require.NoError(t, DB.Model(&InviteReward{}).Where("user_id = ? AND quota > 0", inviter.Id).Count(&count).Error)
			assert.Equal(t, int64(2), count)
		})
	}
}

// TestInviteRewardSummaryIncludesBeyondDetailPage 保护超过最近 50 笔明细后的冻结和已提现汇总，
// 并确认重启结转不把零额度邀请计数或充值返佣当作历史可提余额。
func TestInviteRewardSummaryIncludesBeyondDetailPage(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			openInviteRewardTestDB(t, dialect)
			user := newInviteRewardTestUser(t, 0, 0)
			now := common.GetTimestamp()
			for i := range 51 {
				seedInviteReward(t, user.Id, InviteRewardKindTopUp, 1000, now-int64(i))
			}
			summary, err := GetInviteRewardSummary(user.Id)
			require.NoError(t, err)
			assert.Len(t, summary.Rewards, 50)
			assert.Equal(t, 51000, summary.FrozenQuota)
			assert.Zero(t, summary.WithdrawableQuota)
			assert.Equal(t, now-50+172800, summary.NextAvailableAt)
			require.NoError(t, InitializeInviteRewardLedger())
			require.NoError(t, InitializeInviteRewardLedger())
			afterRestart, err := GetInviteRewardSummary(user.Id)
			require.NoError(t, err)
			assert.Equal(t, summary.FrozenQuota, afterRestart.FrozenQuota)
			common.QuotaPerUnit = 1
			_, err = WithdrawInviteRewardsAt(user.Id, "oldest-only", 1000, now-50+172800)
			require.NoError(t, err)
			afterWithdraw, err := GetInviteRewardSummary(user.Id)
			require.NoError(t, err)
			assert.Equal(t, 1000, afterWithdraw.WithdrawnQuota)
			assert.Equal(t, 50000, afterWithdraw.FrozenQuota)
			assert.Zero(t, afterWithdraw.WithdrawableQuota)
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
// 旧注册奖励立即可提，新充值返佣仍固定冻结 48 小时。
func TestInviteRewardFreezeHoursZeroDisablesFreeze(t *testing.T) {
	openInviteRewardTestDB(t, "sqlite")
	common.InviteRewardFreezeHours = 0
	user := newInviteRewardTestUser(t, 0, 0)
	seedInviteReward(t, user.Id, InviteRewardKindInviter, inviteRewardTestQuotaPerUnit, common.GetTimestamp())

	summary, err := GetInviteRewardSummary(user.Id)
	require.NoError(t, err)
	assert.Equal(t, inviteRewardTestQuotaPerUnit, summary.WithdrawableQuota)
	assert.Zero(t, summary.FrozenQuota)
	assert.Equal(t, 48*60*60, summary.FreezeSeconds)
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
