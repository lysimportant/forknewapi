package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestResponsesHelperChatRouting 验证真实入口完成模型映射、Chat 转换和参数覆盖，再保留上游拒绝上下文。
// 上游故意返回 422，以隔离请求路由契约，避免依赖数据库或钱包结算。
func TestResponsesHelperChatRouting(t *testing.T) {
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"provider_validation","message":"provider rejected test input"}}`)
	}))
	defer server.Close()
	c, _, info, _ := responsesFallbackContext(t, constant.ChannelTypeSiliconFlow, server.URL, `{"model":"client-alias","input":"hello","max_output_tokens":73}`)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-token")
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"max_tokens": 51})
	c.Set("model_mapping", `{"client-alias":"upstream-model"}`)

	apiErr := ResponsesHelper(c, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "provider rejected test input")
	require.Len(t, requests, 1)
	body := <-requests
	assert.Equal(t, "upstream-model", gjson.GetBytes(body, "model").String())
	assert.Equal(t, "hello", gjson.GetBytes(body, "messages.0.content").String())
	assert.Equal(t, int64(51), gjson.GetBytes(body, "max_tokens").Int())
	assert.False(t, gjson.GetBytes(body, "input").Exists())
}
