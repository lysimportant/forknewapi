package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponsesRoutesRequireAuthentication 验证两种 Base URL 的 Responses 入口均走鉴权，不能落入网页或重定向。
func TestResponsesRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	engine.NoRoute(func(c *gin.Context) { c.Data(http.StatusOK, "text/html", []byte("dashboard")) })

	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/responses"},
		{http.MethodPost, "/responses"},
		{http.MethodPost, "/v1/responses/compact"},
		{http.MethodPost, "/responses/compact"},
		{http.MethodGet, "/v1/responses/resp_test"},
		{http.MethodGet, "/responses/resp_test"},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(`{"model":"deepseek-v4-flash-vision-exp","input":"test"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
			assert.Empty(t, recorder.Header().Get("Location"))
		})
	}
}

// TestResponsesRoutesPreserveRequest 验证别名规范化保留原始 JSON、查询编码及 retrieve 参数，且只执行一次全局中间件。
func TestResponsesRoutesPreserveRequest(t *testing.T) {
	for _, test := range []struct {
		method        string
		canonicalPath string
		path          string
		wantPath      string
		wantRawPath   string
		wantParam     string
		wantMode      int
	}{
		{http.MethodPost, "/v1/responses", "/responses", "/v1/responses", "", "", relayconstant.RelayModeResponses},
		{http.MethodPost, "/v1/responses/compact", "/responses/compact", "/v1/responses/compact", "", "", relayconstant.RelayModeResponsesCompact},
		{http.MethodGet, "/v1/responses/:response_id", "/responses/resp_%74est", "/v1/responses/resp_test", "/v1/responses/resp_%74est", "resp_test", relayconstant.RelayModeResponses},
		{http.MethodPost, "/v1/responses", "/v1/responses", "/v1/responses", "", "", relayconstant.RelayModeResponses},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			const query = "tag=a%2Fb&tag=two+words&literal=%252F"
			const body = ` {"model":"test","input":"你好","store":false,"extra":{"zero":0}} `
			globalCalls := 0
			engine := gin.New()
			engine.Use(func(c *gin.Context) { globalCalls++; c.Next() })
			registerResponsesRoute(engine, test.method, test.canonicalPath, func(c *gin.Context) {
				assert.Equal(t, test.wantPath, c.Request.URL.Path)
				assert.Equal(t, test.wantRawPath, c.Request.URL.RawPath)
				assert.Equal(t, query, c.Request.URL.RawQuery)
				assert.Equal(t, c.Request.URL.EscapedPath()+"?"+query, c.Request.RequestURI)
				assert.Equal(t, []string{"a/b", "two words"}, c.QueryArray("tag"))
				assert.Equal(t, test.wantParam, c.Param("response_id"))
				assert.Equal(t, test.wantMode, relayconstant.Path2RelayMode(c.Request.URL.Path))
				actualBody, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, body, string(actualBody))
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path+"?"+query, strings.NewReader(body)))
			require.Equal(t, http.StatusNoContent, recorder.Code)
			assert.Equal(t, 1, globalCalls)
			assert.Empty(t, recorder.Header().Get("Location"))
		})
	}
}

// TestResponsesAliasRetainsPluginDispatch 通过真实鉴权与协议中间件确认根路径仍命中插件，并保留插件可见的路径、参数及请求体。
func TestResponsesAliasRetainsPluginDispatch(t *testing.T) {
	setupRelayRouterTestDB(t)
	wasRateLimitEnabled := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = false
	t.Cleanup(func() { setting.ModelRequestRateLimitEnabled = wasRateLimitEnabled })
	user := model.User{Username: "responses-alias-user", Status: common.UserStatusEnabled, Group: "default", Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: user.Id, Key: "responsesaliastest", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	_, err := jsplugin.DefaultRegistry.Register(`
export const meta = {
  apiVersion: 1, key: "responses-alias-fixture", name: "Responses alias fixture", version: "1.0.0",
  author: {name: "Test"}, models: ["responses-alias-model"], fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["sync"]}],
};
export function buildSubmitRequest() { throw new Error("unexpected upstream request"); }
export function parseSubmitResponse() { return {taskId: "unused"}; }
export function buildQueryRequest() { throw new Error("unexpected upstream request"); }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) {
    if (ctx.path !== "/v1/responses" || ctx.method !== "POST") throw new Error("path changed");
    if (ctx.query.tag[0] !== "a/b" || ctx.query.tag[1] !== "two words") throw new Error("query changed");
    if (ctx.body.kind !== "json" || ctx.body.value.input !== "你好" || ctx.body.value.store !== false || ctx.body.value.extra.zero !== 0) throw new Error("body changed");
    throw new Error("responses_alias_decoder_reached");
  },
  renderFinal: function() { return {}; },
}};
`, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, jsplugin.DefaultRegistry.Unregister("responses-alias-fixture")) })
	engine := gin.New()
	SetRelayRouter(engine)
	SetTaskPluginProtocolRouter(engine)

	for _, path := range []string{"/v1/responses", "/responses"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path+"?tag=a%2Fb&tag=two+words", strings.NewReader(`{"model":"responses-alias-model","input":"你好","store":false,"extra":{"zero":0}}`))
			request.Header.Set("Authorization", "Bearer responsesaliastest")
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), "responses_alias_decoder_reached")
			assert.Empty(t, recorder.Header().Get("Location"))
		})
	}
}
