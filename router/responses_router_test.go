package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponsesRoutesRequireAuthentication 通过实际路由确认根路径和版本化入口均要求令牌，不落入网页或重定向。
func TestResponsesRoutesRequireAuthentication(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	engine := gin.New()
	SetRelayRouter(engine)
	engine.NoRoute(func(c *gin.Context) { c.Data(http.StatusOK, "text/html", []byte("dashboard")) })
	for _, path := range []string{"/responses", "/v1/responses", "/responses/compact", "/v1/responses/compact"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"deepseek-v4-flash-vision-exp","input":"test"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
			assert.Empty(t, recorder.Header().Get("Location"))
		})
	}
}

// TestResponsesAliasPreservesRequest 保护路径规范化后的模式识别、请求体、查询编码和请求方法，避免压缩路由按普通请求计费。
func TestResponsesAliasPreservesRequest(t *testing.T) {
	for _, test := range []struct {
		path string
		mode int
	}{
		{"/responses", relayconstant.RelayModeResponses},
		{"/responses/compact", relayconstant.RelayModeResponsesCompact},
	} {
		t.Run(test.path, func(t *testing.T) {
			const query = "tag=a%2Fb&tag=two+words&literal=%252F"
			const body = ` {"model":"test","input":"你好","store":false,"extra":{"zero":0}} `
			globalCalls := 0
			engine := gin.New()
			engine.Use(func(c *gin.Context) { globalCalls++; c.Next() })
			engine.POST(test.path, normalizeResponsesAlias, func(c *gin.Context) {
				assert.Equal(t, http.MethodPost, c.Request.Method)
				assert.Equal(t, "/v1"+test.path, c.Request.URL.Path)
				assert.Equal(t, query, c.Request.URL.RawQuery)
				assert.Equal(t, "/v1"+test.path+"?"+query, c.Request.RequestURI)
				assert.Equal(t, []string{"a/b", "two words"}, c.QueryArray("tag"))
				assert.Equal(t, test.mode, relayconstant.Path2RelayMode(c.Request.URL.Path))
				actualBody, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, body, string(actualBody))
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, test.path+"?"+query, strings.NewReader(body)))
			require.Equal(t, http.StatusNoContent, recorder.Code)
			assert.Equal(t, 1, globalCalls)
			assert.Empty(t, recorder.Header().Get("Location"))
		})
	}
}
