package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCanvasGrantRoutesDoNotConsumeLoginCriticalLimit 验证后台分组同步不会耗尽浏览器登录保护额度。
func TestCanvasGrantRoutesDoNotConsumeLoginCriticalLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousGlobalEnabled := common.GlobalApiRateLimitEnable
	previousCriticalEnabled := common.CriticalRateLimitEnable
	previousCriticalLimit := common.CriticalRateLimitNum
	previousCriticalDuration := common.CriticalRateLimitDuration
	previousRedisEnabled := common.RedisEnabled
	common.GlobalApiRateLimitEnable = false
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 1
	common.CriticalRateLimitDuration = 1200
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousGlobalEnabled
		common.CriticalRateLimitEnable = previousCriticalEnabled
		common.CriticalRateLimitNum = previousCriticalLimit
		common.CriticalRateLimitDuration = previousCriticalDuration
		common.RedisEnabled = previousRedisEnabled
	})
	t.Setenv("CANVAS_ACCOUNT_ENABLED", "true")
	t.Setenv("CANVAS_ACCOUNT_ISSUER", "http://127.0.0.1:13000")
	t.Setenv("CANVAS_ACCOUNT_CLIENT_ID", "canvas")
	t.Setenv("CANVAS_ACCOUNT_INSTANCE_ID", "router-test")
	t.Setenv("CANVAS_ACCOUNT_REDIRECT_URI", "http://localhost:5173/v1/auth/newapi/callback")

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	SetApiRouter(engine)
	remoteAddress := "192.0.2.251:12345"

	authorize := performCanvasAccountRouterRequest(engine, http.MethodGet, "/api/canvas/authorize", remoteAddress)
	assert.Equal(t, http.StatusBadRequest, authorize.Code)
	limited := performCanvasAccountRouterRequest(engine, http.MethodGet, "/api/canvas/authorize", remoteAddress)
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.Equal(t, "1200", limited.Header().Get("Retry-After"))

	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/canvas/account"},
		{http.MethodPut, "/api/canvas/groups/default"},
		{http.MethodPost, "/api/canvas/groups/default/rotate"},
		{http.MethodPost, "/api/canvas/revoke"},
	} {
		response := performCanvasAccountRouterRequest(engine, request.method, request.path, remoteAddress)
		assert.Equal(t, http.StatusUnauthorized, response.Code, "%s %s", request.method, request.path)
		assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	}
}

// performCanvasAccountRouterRequest 从同一来源地址执行隔离路由请求。
func performCanvasAccountRouterRequest(handler http.Handler, method, path, remoteAddress string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = remoteAddress
	handler.ServeHTTP(response, request)
	return response
}
