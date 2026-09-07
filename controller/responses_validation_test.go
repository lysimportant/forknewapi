package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestResponsesValidationHTTPStatus 验证真实 Relay HTTP 入口拒绝非法客户端输入时返回 400，保留体积超限的 413，且不请求上游或改变余额。
func TestResponsesValidationHTTPStatus(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMode := common.RedisEnabled, gin.Mode()
	previousRetries, previousMaxBody := common.RetryTimes, constant.MaxRequestBodyMB
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedis
		gin.SetMode(previousMode)
		common.RetryTimes, constant.MaxRequestBodyMB = previousRetries, previousMaxBody
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	common.RetryTimes = 3
	constant.MaxRequestBodyMB = 1

	user := model.User{Username: "validation-user", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, UsedQuota: 17}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "test-only-validation-token", Status: common.TokenStatusEnabled, RemainQuota: 500000, UsedQuota: 9, Group: "default"}
	require.NoError(t, db.Create(&token).Error)
	// 两个计数区分已进入真实控制器和实际出站请求，失败重试不能隐藏在成功返回之后。
	var upstreamCalls, relayCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Name: "validation-fixture", Key: "test-only-upstream-token", BaseURL: &upstream.URL, Models: "deepseek-v4-flash-vision-exp", Group: "default"}
	require.NoError(t, db.Create(&channel).Error)

	engine := gin.New()
	engine.Use(middleware.BodyStorageCleanup())
	engine.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "responses-validation-test")
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
		common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
		common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		if err := middleware.SetupContextForSelectedChannel(c, &channel, "deepseek-v4-flash-vision-exp"); !assert.Nil(t, err) {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	for _, endpoint := range []struct {
		path   string
		format types.RelayFormat
	}{
		{"/responses", types.RelayFormatOpenAIResponses},
		{"/v1/responses", types.RelayFormatOpenAIResponses},
		{"/responses/compact", types.RelayFormatOpenAIResponsesCompaction},
		{"/v1/responses/compact", types.RelayFormatOpenAIResponsesCompaction},
	} {
		engine.POST(endpoint.path, func(c *gin.Context) {
			relayCalls.Add(1)
			Relay(c, endpoint.format)
		})
	}
	relayServer := httptest.NewServer(engine)
	t.Cleanup(relayServer.Close)
	oversized := `{"model":"deepseek-v4-flash-vision-exp","input":"` + strings.Repeat("x", 1<<20) + `"}`
	for _, test := range []struct {
		name, path, body, message string
		status                    int
		code                      string
	}{
		{"输出上限越界", "/responses", `{"model":"deepseek-v4-flash-vision-exp","input":"test","max_output_tokens":2147483647}`, "max_output_tokens is invalid", http.StatusBadRequest, "invalid_request"},
		{"缺少模型", "/v1/responses", `{"input":"test"}`, "model is required", http.StatusBadRequest, "invalid_request"},
		{"缺少输入", "/responses", `{"model":"deepseek-v4-flash-vision-exp"}`, "input is required", http.StatusBadRequest, "invalid_request"},
		{"输出上限类型错误", "/v1/responses", `{"model":"deepseek-v4-flash-vision-exp","input":"test","max_output_tokens":"invalid"}`, "max_output_tokens", http.StatusBadRequest, "invalid_request"},
		{"非法JSON", "/responses", `{"model":`, "", http.StatusBadRequest, "invalid_request"},
		{"压缩缺少模型", "/responses/compact", `{"input":[]}`, "model is required", http.StatusBadRequest, "invalid_request"},
		{"压缩非法JSON", "/v1/responses/compact", `{"model":`, "", http.StatusBadRequest, "invalid_request"},
		{"请求体超限", "/responses", oversized, "request body", http.StatusRequestEntityTooLarge, "read_request_body_failed"},
		{"压缩请求体超限", "/v1/responses/compact", oversized, "request body", http.StatusRequestEntityTooLarge, "read_request_body_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			beforeCalls := relayCalls.Load()
			response, err := relayServer.Client().Post(relayServer.URL+test.path, "application/json", strings.NewReader(test.body))
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			assert.Equal(t, test.status, response.StatusCode, string(body))
			assert.Contains(t, response.Header.Get("Content-Type"), "application/json")
			assert.Equal(t, test.code, gjson.GetBytes(body, "error.code").String())
			assert.Contains(t, gjson.GetBytes(body, "error.message").String(), test.message)
			assert.Contains(t, gjson.GetBytes(body, "error.message").String(), "responses-validation-test")
			assert.Equal(t, beforeCalls+1, relayCalls.Load())
			assert.Zero(t, upstreamCalls.Load(), "非法请求必须在任何上游调用及重试之前返回")

			var actualUser model.User
			var actualToken model.Token
			require.NoError(t, db.First(&actualUser, user.Id).Error)
			require.NoError(t, db.First(&actualToken, token.Id).Error)
			assert.Equal(t, user.Quota, actualUser.Quota)
			assert.Equal(t, user.UsedQuota, actualUser.UsedQuota)
			assert.Equal(t, token.RemainQuota, actualToken.RemainQuota)
			assert.Equal(t, token.UsedQuota, actualToken.UsedQuota)
			var consumeLogs int64
			require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&consumeLogs).Error)
			assert.Zero(t, consumeLogs, "客户端参数错误不能产生消费日志")
		})
	}
}
