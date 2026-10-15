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

func TestCanvasVideoCancelAuthorization(t *testing.T) {
	initModelListColumnNames(t)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "video-cancel.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}, &model.CanvasReceipt{}))
	previousDB, previousRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	t.Setenv("CANVAS_BRIDGE_ENABLED", "true")
	user := &model.User{Username: "cancel-owner", AffCode: "cancel-owner", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "cancelownersynthetic", Status: common.TokenStatusEnabled, ExpiredTime: time.Now().Add(-time.Hour).Unix()}
	otherToken := &model.Token{UserId: user.Id, Key: "cancelothersynthetic", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, db.Create(otherToken).Error)
	task := &model.Task{UserId: user.Id, TaskID: "video-own", Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{TokenId: token.Id, Execution: &model.TaskExecutionSnapshot{RequestID: "create-own", RequestPath: "/v1/videos"}}}
	require.NoError(t, db.Create(task).Error)
	_, err = model.EnsureCanvasReceipt(&model.CanvasReceipt{TokenID: token.Id, UserID: user.Id, RequestID: "create-own", TaskID: task.TaskID, ModelName: "synthetic-model", QuotaPerUnit: "500000"})
	require.NoError(t, err)
	require.NoError(t, model.MarkCanvasReceiptAccounting(token.Id, user.Id, "create-own", true))
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.POST("/v1/canvas/tasks/:taskId/cancel", middleware.TokenAuthReadOnly(), CancelCanvasVideoTask)
	request := func(key, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/canvas/tasks/"+id+"/cancel", strings.NewReader(`{"reason":"user_cancelled"}`))
		req.Header.Set("Authorization", "Bearer sk-"+key)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.10:3210"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	missing := request(token.Key, "missing")
	foreign := request(otherToken.Key, task.TaskID)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
	assert.Equal(t, missing.Body.String(), foreign.Body.String())
	require.NoError(t, db.Model(token).Update("allow_ips", "198.51.100.0/24").Error)
	assert.Equal(t, http.StatusForbidden, request(token.Key, task.TaskID).Code)
	require.NoError(t, db.Model(token).Updates(map[string]any{"allow_ips": "", "status": common.TokenStatusDisabled}).Error)
	assert.Equal(t, http.StatusUnauthorized, request(token.Key, task.TaskID).Code)
	require.NoError(t, db.Model(token).Update("status", common.TokenStatusExpired).Error)
	response := request(token.Key, task.TaskID)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"refund_status":"refunded"`)
	assert.Contains(t, response.Body.String(), `"status":"failed"`)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Equal(t, http.StatusOK, request(token.Key, task.TaskID).Code)
	require.NoError(t, db.Model(task).Update("status", model.TaskStatusSuccess).Error)
	assert.Equal(t, http.StatusConflict, request(token.Key, task.TaskID).Code)
}
