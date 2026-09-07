package openai_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/advancedcustom"
	"github.com/QuantumNous/new-api/relay/channel/deepseek"
	"github.com/QuantumNous/new-api/relay/channel/newapi"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/sub2api"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestNativeResponsesHTTPToolRoundTrip 验证真实 HTTP 传输中的原生 Responses 两轮工具调用，保护请求保真、事件完整性及终态用量。
func TestNativeResponsesHTTPToolRoundTrip(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })

	for _, test := range []struct {
		name        string
		channelType int
		adaptor     channel.Adaptor
		baseSuffix  string
	}{
		{name: "OpenAI 兼容渠道", channelType: constant.ChannelTypeOpenAI, adaptor: &openai.Adaptor{}},
		{name: "高级自定义原生路由", channelType: constant.ChannelTypeAdvancedCustom, adaptor: &advancedcustom.Adaptor{}},
		{name: "Sub2 渠道对照", channelType: constant.ChannelTypeSub2API, adaptor: &sub2api.Adaptor{}},
		{name: "NewAPI 原生渠道", channelType: constant.ChannelTypeNewAPI, adaptor: &newapi.Adaptor{}},
		{name: "DeepSeek 专用渠道", channelType: constant.ChannelTypeDeepSeek, adaptor: &deepseek.Adaptor{}, baseSuffix: "/v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 仅在模拟上游 goroutine 写入，请求结束后由测试 goroutine 读取。
			requests := make(chan []byte, 2)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if !assert.NoError(t, err) {
					http.Error(w, "cannot read request", http.StatusBadRequest)
					return
				}
				assert.Equal(t, "/v1/responses", r.URL.Path)
				assert.Equal(t, "Bearer test-only-upstream-token", r.Header.Get("Authorization"))
				assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
				requests <- body
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Codex-Turn-State", "test-turn-state")
				if strings.Contains(string(body), `"function_call_output"`) {
					_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n")
				} else {
					_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"README.md\\\"}\"}}\n\n")
				}
				_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":20,\"output_tokens\":4,\"total_tokens\":24,\"input_tokens_details\":{\"cached_tokens\":3}}}}\n\n")
			}))
			defer upstream.Close()

			for turn, input := range []string{
				`[{"role":"user","content":[{"type":"input_text","text":"Read README.md"}]}]`,
				`[{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"README.md\"}"},{"type":"function_call_output","call_id":"call_1","output":"Project overview"}]`,
			} {
				body := fmt.Sprintf(`{"model":"deepseek-v4-flash-vision-exp","input":%s,"tools":[{"type":"function","name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}],"reasoning":{"effort":"max","summary":"auto"},"store":false,"stream":true,"include":["reasoning.encrypted_content"]}`, input)
				var request dto.OpenAIResponsesRequest
				require.NoError(t, common.Unmarshal([]byte(body), &request))
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				ctx.Set(common.RequestIdKey, "native-responses-roundtrip")
				// 使用独立渠道元信息，使每轮适配状态及用量互不共享。
				info := &relaycommon.RelayInfo{
					OriginModelName: request.Model,
					Request:         &request,
					RelayFormat:     types.RelayFormatOpenAIResponses,
					RelayMode:       relayconstant.RelayModeResponses,
					RequestURLPath:  "/v1/responses",
					IsStream:        true,
					DisablePing:     true,
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelType:       test.channelType,
						ChannelBaseUrl:    upstream.URL + test.baseSuffix,
						ApiKey:            "test-only-upstream-token",
						UpstreamModelName: request.Model,
						ChannelOtherSettings: dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{
							Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/responses", Converter: relayconvert.ConverterNone}},
						}},
					},
				}
				test.adaptor.Init(info)
				converted, err := test.adaptor.ConvertOpenAIResponsesRequest(ctx, info, request)
				require.NoError(t, err)
				outbound, err := common.Marshal(converted)
				require.NoError(t, err)
				response, err := test.adaptor.DoRequest(ctx, info, bytes.NewReader(outbound))
				require.NoError(t, err)
				httpResponse, ok := response.(*http.Response)
				require.True(t, ok)
				defer httpResponse.Body.Close()
				require.Equal(t, http.StatusOK, httpResponse.StatusCode)
				usage, apiErr := test.adaptor.DoResponse(ctx, httpResponse, info)
				require.Nil(t, apiErr)
				assert.JSONEq(t, body, string(<-requests))
				require.IsType(t, &dto.Usage{}, usage)
				assert.Equal(t, 20, usage.(*dto.Usage).PromptTokens)
				assert.Equal(t, 4, usage.(*dto.Usage).CompletionTokens)
				assert.Equal(t, 3, usage.(*dto.Usage).PromptTokensDetails.CachedTokens)
				require.NotNil(t, info.StreamStatus)
				assert.False(t, info.StreamStatus.HasErrors())
				assert.Equal(t, "test-turn-state", recorder.Header().Get("X-Codex-Turn-State"))
				assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed\n"))
				if turn == 0 {
					for _, line := range strings.Split(recorder.Body.String(), "\n") {
						if strings.HasPrefix(line, "data: ") && gjson.Get(line[6:], "type").String() == "response.output_item.done" {
							assert.Equal(t, "call_1", gjson.Get(line[6:], "item.call_id").String())
						}
					}
					assert.Contains(t, recorder.Body.String(), `"call_id":"call_1"`)
				} else {
					assert.Contains(t, recorder.Body.String(), `"delta":"OK"`)
				}
			}
			assert.Empty(t, requests, "每轮只应发送一次上游请求")
		})
	}
}
