package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestCanvasReceiptReadAuthorization 覆盖 ASVS 5.0.0 的 8.2.1、8.2.2、8.2.3 和 8.3.1。
// 回执令牌可过期或耗尽，但禁用、跨令牌/用户及 IP 限制仍必须拒绝。
func TestCanvasReceiptReadAuthorization(t *testing.T) {
	previousDB, previousRedis := model.DB, common.RedisEnabled
	initModelListColumnNames(t)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "receipt-auth.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.CanvasReceipt{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		require.NoError(t, sqlDB.Close())
	})
	t.Setenv("CANVAS_BRIDGE_ENABLED", "true")
	user := &model.User{Username: "receiptowner", AffCode: "receiptowner", Status: common.UserStatusEnabled}
	otherUser := &model.User{Username: "receiptother", AffCode: "receiptother", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(otherUser).Error)
	token := &model.Token{UserId: user.Id, Key: "receipttestowner", Status: common.TokenStatusEnabled, RemainQuota: 0, ExpiredTime: time.Now().Add(-time.Hour).Unix()}
	otherToken := &model.Token{UserId: user.Id, Key: "receipttestsametenant", Status: common.TokenStatusEnabled}
	foreignToken := &model.Token{UserId: otherUser.Id, Key: "receipttestother", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, db.Create(otherToken).Error)
	require.NoError(t, db.Create(foreignToken).Error)
	_, err = model.EnsureCanvasReceipt(&model.CanvasReceipt{TokenID: token.Id, UserID: user.Id, RequestID: "create-original", TaskID: "video1", ModelName: "test-model", Group: "default", QuotaPerUnit: "500000"})
	require.NoError(t, err)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/v1/canvas/receipts/:requestId", middleware.TokenAuthReadOnly(), GetCanvasReceipt)
	request := func(key, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/canvas/receipts/"+id, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer sk-"+key)
		}
		req.Header.Set("X-Oneapi-Request-Id", "poll-must-not-be-used")
		req.Header.Set("X-Forwarded-For", "198.51.100.10")
		req.RemoteAddr = "192.0.2.10:3210"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	response := request(token.Key, "create-original")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
	var payload map[string]any
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "create-original", payload["request_id"])
	assert.Equal(t, "pending", payload["status"])
	assert.Equal(t, "0", payload["quota"])
	for _, field := range []string{"user_id", "token_id", "token_key", "data", "settled_at"} {
		assert.NotContains(t, payload, field)
	}
	assert.NotContains(t, response.Body.String(), token.Key)
	assert.Equal(t, http.StatusUnauthorized, request("", "create-original").Code)
	assert.Equal(t, http.StatusUnauthorized, request("invalid", "create-original").Code)
	missing := request(token.Key, "absent")
	for _, key := range []string{otherToken.Key, foreignToken.Key} {
		foreign := request(key, "create-original")
		assert.Equal(t, http.StatusNotFound, foreign.Code)
		assert.Equal(t, missing.Body.String(), foreign.Body.String())
	}
	assert.Equal(t, http.StatusBadRequest, request(token.Key, strings.Repeat("a", 65)).Code)
	require.NoError(t, db.Model(token).Update("status", common.TokenStatusExhausted).Error)
	assert.Equal(t, http.StatusOK, request(token.Key, "create-original").Code)
	require.NoError(t, db.Model(token).Update("status", common.TokenStatusExpired).Error)
	assert.Equal(t, http.StatusOK, request(token.Key, "create-original").Code)
	require.NoError(t, db.Model(token).Update("status", common.TokenStatusDisabled).Error)
	assert.Equal(t, http.StatusUnauthorized, request(token.Key, "create-original").Code)
	require.NoError(t, db.Model(token).Updates(map[string]any{"status": common.TokenStatusEnabled, "allow_ips": "198.51.100.0/24"}).Error)
	assert.Equal(t, http.StatusForbidden, request(token.Key, "create-original").Code, "untrusted forwarded IP must not bypass the allowlist")
	require.NoError(t, db.Model(token).Update("allow_ips", "192.0.2.0/24").Error)
	assert.Equal(t, http.StatusOK, request(token.Key, "create-original").Code)
	require.NoError(t, db.Model(user).Update("status", common.UserStatusDisabled).Error)
	assert.Equal(t, http.StatusForbidden, request(token.Key, "create-original").Code)
	require.NoError(t, db.Model(user).Update("status", common.UserStatusEnabled).Error)
	require.NoError(t, model.MarkCanvasReceiptAccounting(token.Id, user.Id, "create-original", true))
	require.NoError(t, model.FinalizeCanvasReceipt(token.Id, user.Id, "create-original", model.CanvasReceiptSettled, 123))
	response = request(token.Key, "create-original")
	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "123", payload["quota"])
	assert.Equal(t, "500000", payload["quota_per_unit"])
	_, err = time.Parse(time.RFC3339Nano, payload["settled_at"].(string))
	require.NoError(t, err)
	t.Setenv("CANVAS_BRIDGE_ENABLED", "false")
	assert.Equal(t, http.StatusNotFound, request(token.Key, "create-original").Code)
}
