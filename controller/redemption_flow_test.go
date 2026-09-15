package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedemptionCreateAndRedeemIncreasesQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Redemption{}))

	payment := operation_setting.GetPaymentSetting()
	previousConfirmed := payment.ComplianceConfirmed
	previousVersion := payment.ComplianceTermsVersion
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		payment.ComplianceConfirmed = previousConfirmed
		payment.ComplianceTermsVersion = previousVersion
	})

	admin := &model.User{
		Username: "redeem-admin",
		Password: "password",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Quota:    0,
		AffCode:  "adm1",
	}
	user := &model.User{
		Username: "redeem-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Quota:    100,
		AffCode:  "usr1",
	}
	require.NoError(t, db.Create(admin).Error)
	require.NoError(t, db.Create(user).Error)

	createBody, err := json.Marshal(map[string]any{
		"name":         "e2e-code",
		"quota":        500,
		"count":        1,
		"expired_time": 0,
	})
	require.NoError(t, err)
	createRecorder := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRecorder)
	createCtx.Request = httptest.NewRequest(http.MethodPost, "/api/redemption", bytes.NewReader(createBody))
	createCtx.Request.Header.Set("Content-Type", "application/json")
	createCtx.Set("id", admin.Id)
	AddRedemption(createCtx)
	require.Equal(t, http.StatusOK, createRecorder.Code)

	var createResp struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(createRecorder.Body.Bytes(), &createResp))
	require.True(t, createResp.Success, createResp.Message)
	require.Len(t, createResp.Data, 1)
	key := createResp.Data[0]
	require.NotEmpty(t, key)

	redeemBody, err := json.Marshal(map[string]string{"key": key})
	require.NoError(t, err)
	redeemRecorder := httptest.NewRecorder()
	redeemCtx, _ := gin.CreateTestContext(redeemRecorder)
	redeemCtx.Request = httptest.NewRequest(http.MethodPost, "/api/user/topup", bytes.NewReader(redeemBody))
	redeemCtx.Request.Header.Set("Content-Type", "application/json")
	redeemCtx.Set("id", user.Id)
	TopUp(redeemCtx)
	require.Equal(t, http.StatusOK, redeemRecorder.Code)

	var redeemResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    int    `json:"data"`
	}
	require.NoError(t, json.Unmarshal(redeemRecorder.Body.Bytes(), &redeemResp))
	require.True(t, redeemResp.Success, redeemResp.Message)
	assert.Equal(t, 500, redeemResp.Data)

	var updated model.User
	require.NoError(t, db.First(&updated, "id = ?", user.Id).Error)
	assert.Equal(t, 600, updated.Quota)

	var redemption model.Redemption
	require.NoError(t, db.First(&redemption, "`key` = ?", key).Error)
	assert.Equal(t, common.RedemptionCodeStatusUsed, redemption.Status)
	assert.Equal(t, user.Id, redemption.UsedUserId)
}
