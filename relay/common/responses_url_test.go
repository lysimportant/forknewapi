package common_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/deepseek"
	"github.com/QuantumNous/new-api/relay/channel/newapi"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/sub2api"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponsesUpstreamURL 验证 Responses 的可选版本前缀，并保护其他协议与供应商专用路径。
func TestResponsesUpstreamURL(t *testing.T) {
	for _, test := range []struct {
		name        string
		baseURL     string
		requestURL  string
		channelType int
		want        string
	}{
		{"根域名与无版本入口", "https://upstream.test", "/responses", constant.ChannelTypeOpenAI, "https://upstream.test/v1/responses"},
		{"已带版本", "https://upstream.test/v1", "/v1/responses", constant.ChannelTypeNewAPI, "https://upstream.test/v1/responses"},
		{"尾部斜杠", "https://upstream.test/v1/", "/responses", constant.ChannelTypeSub2API, "https://upstream.test/v1/responses"},
		{"自定义前缀", "https://upstream.test/proxy", "/v1/responses", constant.ChannelTypeOpenAI, "https://upstream.test/proxy/v1/responses"},
		{"前缀含版本", "https://upstream.test/proxy/v1/", "/v1/responses", constant.ChannelTypeDeepSeek, "https://upstream.test/proxy/v1/responses"},
		{"压缩与查询串", "https://upstream.test/v1", "/responses/compact?trace=a%2Fb", constant.ChannelTypeOpenAI, "https://upstream.test/v1/responses/compact?trace=a%2Fb"},
		{"版本化压缩", "https://upstream.test/v1", "/v1/responses/compact", constant.ChannelTypeNewAPI, "https://upstream.test/v1/responses/compact"},
		{"其他版本前缀不误删", "https://upstream.test/v10", "/v1/responses", constant.ChannelTypeOpenAI, "https://upstream.test/v10/v1/responses"},
		{"Chat保持原契约", "https://upstream.test/prefix", "/v1/chat/completions", constant.ChannelTypeOpenAI, "https://upstream.test/prefix/v1/chat/completions"},
		{"Responses资源子路由不改写", "https://upstream.test", "/responses/resp_1", constant.ChannelTypeOpenAI, "https://upstream.test/responses/resp_1"},
		{"高级自定义不改写", "https://upstream.test/proxy", "/responses", constant.ChannelTypeAdvancedCustom, "https://upstream.test/proxy/responses"},
		{"其他供应商不改写", "https://upstream.test/api/v3", "/responses", constant.ChannelTypeVolcEngine, "https://upstream.test/api/v3/responses"},
		{"Azure保持专用路径", "https://instance.openai.azure.com", "/openai/v1/responses?api-version=preview", constant.ChannelTypeAzure, "https://instance.openai.azure.com/openai/v1/responses?api-version=preview"},
		{"Cloudflare保持去版本行为", "https://gateway.ai.cloudflare.com/v1/account/gateway/openai", "/v1/responses", constant.ChannelTypeOpenAI, "https://gateway.ai.cloudflare.com/v1/account/gateway/openai/responses"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, relaycommon.GetFullRequestURL(test.baseURL, test.requestURL, test.channelType))
		})
	}
}

// TestResponsesUpstreamHTTPPaths 验证真实出站地址与鉴权，确认错误响应不会触发地址探测重发。
func TestResponsesUpstreamHTTPPaths(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	for _, provider := range []struct {
		name        string
		channelType int
		adaptor     channel.Adaptor
	}{
		{"OpenAI", constant.ChannelTypeOpenAI, &openai.Adaptor{}},
		{"NewAPI", constant.ChannelTypeNewAPI, &newapi.Adaptor{}},
		{"Sub2API", constant.ChannelTypeSub2API, &sub2api.Adaptor{}},
		{"DeepSeek", constant.ChannelTypeDeepSeek, &deepseek.Adaptor{}},
	} {
		for _, endpoint := range []struct {
			baseSuffix string
			incoming   string
			wantPath   string
			mode       int
			status     int
		}{
			{"", "/responses", "/v1/responses", relayconstant.RelayModeResponses, http.StatusOK},
			{"/v1", "/v1/responses", "/v1/responses", relayconstant.RelayModeResponses, http.StatusOK},
			{"/v1/", "/responses", "/v1/responses", relayconstant.RelayModeResponses, http.StatusOK},
			{"/proxy", "/v1/responses", "/proxy/v1/responses", relayconstant.RelayModeResponses, http.StatusOK},
			{"/proxy/v1/", "/v1/responses", "/proxy/v1/responses", relayconstant.RelayModeResponses, http.StatusBadRequest},
			{"/v1", "/responses/compact", "/v1/responses/compact", relayconstant.RelayModeResponsesCompact, http.StatusOK},
		} {
			if provider.channelType == constant.ChannelTypeDeepSeek && endpoint.mode == relayconstant.RelayModeResponsesCompact {
				continue // DeepSeek 现有能力不包含 compact，URL 规范化不扩大协议支持范围。
			}
			t.Run(provider.name+endpoint.baseSuffix+endpoint.incoming, func(t *testing.T) {
				// 计数只用于确认每次调用恰好发送一次 HTTP 请求。
				var requestCount atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requestCount.Add(1)
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, endpoint.wantPath, r.URL.Path)
					assert.Equal(t, "Bearer test-only-responses-token", r.Header.Get("Authorization"))
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					assert.JSONEq(t, `{"model":"deepseek-v4-flash-vision-exp","input":"OK","stream":false}`, string(body))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(endpoint.status)
					_, _ = io.WriteString(w, `{"id":"resp_test","status":"completed","output":[]}`)
				}))
				defer upstream.Close()
				body := `{"model":"deepseek-v4-flash-vision-exp","input":"OK","stream":false}`
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest(http.MethodPost, endpoint.incoming, strings.NewReader(body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				info := &relaycommon.RelayInfo{
					RelayFormat:    types.RelayFormatOpenAIResponses,
					RelayMode:      endpoint.mode,
					RequestURLPath: endpoint.incoming,
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelType:    provider.channelType,
						ChannelBaseUrl: upstream.URL + endpoint.baseSuffix,
						ApiKey:         "test-only-responses-token",
					},
				}
				provider.adaptor.Init(info)
				result, err := provider.adaptor.DoRequest(ctx, info, strings.NewReader(body))
				require.NoError(t, err)
				require.IsType(t, &http.Response{}, result)
				response := result.(*http.Response)
				require.NoError(t, response.Body.Close())
				assert.Equal(t, endpoint.status, response.StatusCode)
				assert.EqualValues(t, 1, requestCount.Load())
				assert.Equal(t, upstream.URL+endpoint.baseSuffix, info.ChannelBaseUrl, "不能改写共享渠道配置")
			})
		}
	}
}
