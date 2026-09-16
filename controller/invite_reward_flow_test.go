package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestTransferAffQuotaEnforcesFreezeAndIdempotency 覆盖提现接口的对外契约：
// 缺少幂等标识、奖励仍在冻结期、正常入账，以及同一 requestId 重放只记一次。
func TestTransferAffQuotaEnforcesFreezeAndIdempotency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.InviteReward{}, &model.InviteRewardWithdrawal{}))
	require.NoError(t, db.Exec("DELETE FROM invite_reward_withdrawals").Error)
	require.NoError(t, db.Exec("DELETE FROM invite_rewards").Error)

	payment := operation_setting.GetPaymentSetting()
	previousConfirmed, previousVersion := payment.ComplianceConfirmed, payment.ComplianceTermsVersion
	previousQuotaPerUnit := common.QuotaPerUnit
	previousFreeze := common.InviteRewardFreezeHours
	t.Cleanup(func() {
		payment.ComplianceConfirmed, payment.ComplianceTermsVersion = previousConfirmed, previousVersion
		common.QuotaPerUnit = previousQuotaPerUnit
		common.InviteRewardFreezeHours = previousFreeze
	})
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	common.QuotaPerUnit = 500000
	common.InviteRewardFreezeHours = 48

	const quotaPerUnit = 500000
	// 用户名单一，重复运行时先清掉上一次的残留，保证用例可独立重复执行。
	require.NoError(t, db.Where("username = ?", "invite-withdraw-user").Delete(&model.User{}).Error)
	user := &model.User{
		Username: "invite-withdraw-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AffCode: "iwd1", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)

	claimed, err := model.ClaimInviteReward(
		db, user.Id, fmt.Sprintf("inviter:%d", user.Id), model.InviteRewardKindInviter, quotaPerUnit, common.GetTimestamp(),
	)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"aff_quota": quotaPerUnit, "aff_history": quotaPerUnit,
	}).Error)

	transfer := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/user/aff_transfer", bytes.NewReader([]byte(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("id", user.Id)
		TransferAffQuota(c)
		return recorder
	}

	// 缺少幂等标识：明确要求升级客户端，且不扣账。
	missingKey := transfer(fmt.Sprintf(`{"quota":%d}`, quotaPerUnit))
	assert.Contains(t, missingKey.Body.String(), "invite_reward_idempotency_key_missing")
	assert.Equal(t, quotaPerUnit, reloadInviteRewardUser(t, db, user.Id).AffQuota, "被拒绝的请求不得扣减邀请额度")
	assert.Zero(t, reloadInviteRewardUser(t, db, user.Id).Quota, "被拒绝的请求不得入账")

	// 冻结期内提现被拒绝，并返回最近可提现时间。
	frozen := transfer(fmt.Sprintf(`{"quota":%d,"idempotency_key":"frozen-attempt"}`, quotaPerUnit))
	assert.Contains(t, frozen.Body.String(), "invite_reward_frozen")
	assert.Zero(t, reloadInviteRewardUser(t, db, user.Id).Quota, "冻结期内的提现不得入账")

	// 把奖励产生时间前移到冻结期之前，模拟奖励到期。
	require.NoError(t, db.Model(&model.InviteReward{}).Where("user_id = ?", user.Id).
		Update("created_at", common.GetTimestamp()-int64(common.InviteRewardFreezeHours)*3600).Error)

	success := transfer(fmt.Sprintf(`{"quota":%d,"idempotency_key":"withdraw-once"}`, quotaPerUnit))
	assert.Contains(t, success.Body.String(), `"success":true`)
	afterWithdraw := reloadInviteRewardUser(t, db, user.Id)
	assert.Equal(t, quotaPerUnit, afterWithdraw.Quota)
	assert.Zero(t, afterWithdraw.AffQuota)

	// 同一 requestId 重放：返回首次结果，不重复入账。
	replay := transfer(fmt.Sprintf(`{"quota":%d,"idempotency_key":"withdraw-once"}`, quotaPerUnit))
	assert.Contains(t, replay.Body.String(), `"success":true`)
	afterReplay := reloadInviteRewardUser(t, db, user.Id)
	assert.Equal(t, quotaPerUnit, afterReplay.Quota, "重放不得重复记账")
	assert.Zero(t, afterReplay.AffQuota)

	var withdrawalCount int64
	require.NoError(t, db.Model(&model.InviteRewardWithdrawal{}).Where("user_id = ?", user.Id).Count(&withdrawalCount).Error)
	assert.Equal(t, int64(1), withdrawalCount)
}

// reloadInviteRewardUser 读取用户最新余额，避免使用过期快照断言。
func reloadInviteRewardUser(t *testing.T, db *gorm.DB, userId int) *model.User {
	t.Helper()
	var user model.User
	require.NoError(t, db.Where("id = ?", userId).First(&user).Error)
	return &user
}
