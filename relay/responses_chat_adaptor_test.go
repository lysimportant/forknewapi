package relay

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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestResponsesChatFallbackHTTP 验证六种 Chat 渠道的真实 HTTP 路径、输出上限和工具回传，并检查两种 Responses 返回格式。
func TestResponsesChatFallbackHTTP(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	for _, test := range []struct {
		name, path  string
		channelType int
	}{
		{name: "SiliconFlow", channelType: constant.ChannelTypeSiliconFlow, path: "/v1/chat/completions"},
		{name: "Mistral", channelType: constant.ChannelTypeMistral, path: "/v1/chat/completions"},
		{name: "Moonshot", channelType: constant.ChannelTypeMoonshot, path: "/v1/chat/completions"},
		{name: "MiniMax", channelType: constant.ChannelTypeMiniMax, path: "/v1/text/chatcompletion_v2"},
		{name: "Submodel", channelType: constant.ChannelTypeSubmodel, path: "/v1/chat/completions"},
		{name: "BaiduV2", channelType: constant.ChannelTypeBaiduV2, path: "/v2/chat/completions"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", test.name, stream), func(t *testing.T) {
				requests := make(chan []byte, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if !assert.NoError(t, err) {
						http.Error(w, "cannot read request", http.StatusBadRequest)
						return
					}
					assert.Equal(t, test.path, r.URL.Path)
					assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
					requests <- body
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"id\":\"chat_1\",\"model\":\"chat-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"}}]}\n\n")
						_, _ = io.WriteString(w, "data: {\"id\":\"chat_1\",\"model\":\"chat-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"chat_1","model":"chat-model","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`)
				}))
				defer server.Close()
				body := fmt.Sprintf(`{"model":"chat-model","input":[{"type":"function_call","call_id":"call00001","name":"read_file","arguments":"{\"path\":\"README.md\"}"},{"type":"function_call_output","call_id":"call00001","output":"Project overview"}],"tools":[{"type":"function","name":"read_file","parameters":{"type":"object"}}],"max_output_tokens":73,"stream":%t,"stream_options":{"include_obfuscation":false}}`, stream)
				c, recorder, info, request := responsesFallbackContext(t, test.channelType, server.URL, body)
				adaptor, apiErr := newResponsesAdaptor(info)
				require.Nil(t, apiErr)
				adaptor.Init(info)

				converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
				require.NoError(t, err)
				outbound, err := common.Marshal(converted)
				require.NoError(t, err)
				response, err := adaptor.DoRequest(c, info, bytes.NewReader(outbound))
				require.NoError(t, err)
				upstreamResponse, ok := response.(*http.Response)
				require.True(t, ok)
				defer upstreamResponse.Body.Close()
				usageResult, apiErr := adaptor.DoResponse(c, upstreamResponse, info)
				require.Nil(t, apiErr)
				usage, ok := usageResult.(*dto.Usage)
				require.True(t, ok)
				assert.Equal(t, 12, usage.PromptTokens)
				assert.Equal(t, 3, usage.CompletionTokens)
				assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
				assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
				assert.Equal(t, "/v1/responses", info.RequestURLPath)
				assert.Equal(t, []types.RelayFormat{types.RelayFormatOpenAIResponses, types.RelayFormatOpenAI}, info.RequestConversionChain)
				captured := <-requests
				assert.False(t, gjson.GetBytes(captured, "input").Exists())
				assert.Equal(t, int64(73), gjson.GetBytes(captured, "max_tokens").Int())
				assert.False(t, gjson.GetBytes(captured, "max_completion_tokens").Exists())
				assert.Equal(t, "call00001", gjson.GetBytes(captured, "messages.0.tool_calls.0.id").String())
				assert.Equal(t, "read_file", gjson.GetBytes(captured, "tools.0.function.name").String())
				assert.Equal(t, "call00001", gjson.GetBytes(captured, "messages.1.tool_call_id").String())
				assert.Contains(t, gjson.GetBytes(captured, "messages.1.content").Raw, "Project overview")
				assert.False(t, gjson.GetBytes(captured, "stream_options.include_obfuscation").Exists())
				if stream && info.SupportStreamOptions {
					assert.True(t, gjson.GetBytes(captured, "stream_options.include_usage").Bool())
				} else {
					assert.False(t, gjson.GetBytes(captured, "stream_options").Exists())
				}
				if stream {
					assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed\n"))
					assert.Contains(t, recorder.Body.String(), "event: response.output_text.delta\n")
				} else {
					assert.Equal(t, "response", gjson.Get(recorder.Body.String(), "object").String())
					assert.Equal(t, "OK", gjson.Get(recorder.Body.String(), "output.0.content.0.text").String())
				}
				assert.Empty(t, requests)
			})
		}
	}
}

// TestResponsesChatFallbackRejectsStatefulInput 验证 Chat 无法执行的服务端状态在访问上游前以 400 拒绝。
func TestResponsesChatFallbackRejectsStatefulInput(t *testing.T) {
	for _, field := range []string{`"previous_response_id":"resp_previous"`, `"conversation":"conv_previous"`, `"prompt":{"id":"prompt_1"}`, `"context_management":[{"type":"compaction"}]`} {
		t.Run(field, func(t *testing.T) {
			requests := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- struct{}{}
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			c, _, info, request := responsesFallbackContext(t, constant.ChannelTypeSiliconFlow, server.URL, `{"model":"chat-model","input":"hello",`+field+`}`)
			adaptor, apiErr := newResponsesAdaptor(info)
			require.Nil(t, apiErr)
			adaptor.Init(info)
			_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
			require.Error(t, err)
			apiErr = newConvertRequestFailedError(c, info, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Empty(t, requests)
		})
	}
}

// TestResponsesChatFallbackRejectsLossyInput 防止 Codex 历史和不支持的工具被静默删除或转换为空消息。
func TestResponsesChatFallbackRejectsLossyInput(t *testing.T) {
	for _, test := range []struct {
		name, fields, errorPath string
	}{
		{name: "custom tool call", fields: `"input":[{"type":"custom_tool_call","call_id":"call00001","name":"apply_patch","input":"patch content"}]`, errorPath: "input[0].type"},
		{name: "custom tool result", fields: `"input":[{"type":"custom_tool_call_output","call_id":"call00001","output":"Patch applied"}]`, errorPath: "input[0].type"},
		{name: "encrypted reasoning", fields: `"input":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque-state"}]`, errorPath: "input[0].type"},
		{name: "reasoning summary", fields: `"input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"Read the project"}]}]`, errorPath: "input[0].type"},
		{name: "item reference", fields: `"input":[{"type":"item_reference","id":"msg_previous"}]`, errorPath: "input[0].type"},
		{name: "hosted tool result", fields: `"input":[{"type":"computer_call_output","call_id":"call00001","output":{"type":"computer_screenshot","image_url":"data:image/png;base64,test"}}]`, errorPath: "input[0].type"},
		{name: "custom tool definition", fields: `"input":"hello","tools":[{"type":"custom","name":"apply_patch","format":{"type":"text"}}]`, errorPath: "tools[0]"},
		{name: "hosted tool definition", fields: `"input":"hello","tools":[{"type":"file_search","vector_store_ids":["vs_1"]}]`, errorPath: "tools[0]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _, info, request := responsesFallbackContext(t, constant.ChannelTypeSiliconFlow, "https://unused.example", `{"model":"chat-model",`+test.fields+`}`)
			adaptor, apiErr := newResponsesAdaptor(info)
			require.Nil(t, apiErr)
			adaptor.Init(info)
			_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.errorPath)
			apiErr = newConvertRequestFailedError(c, info, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		})
	}
}

// TestResponsesChatFallbackMistralRejectsDroppedOptions 验证渠道重建 DTO 后不能丢弃已有 Chat 表达的参数，显式零值同样受保护。
func TestResponsesChatFallbackMistralRejectsDroppedOptions(t *testing.T) {
	for _, test := range []struct {
		fields, dropped string
	}{
		{fields: `"text":{"format":{"type":"json_object"}}`, dropped: "response_format"},
		{fields: `"parallel_tool_calls":false`, dropped: "parallel_tool_calls"},
		{fields: `"reasoning":{"effort":"none"}`, dropped: "reasoning_effort"},
		{fields: `"top_logprobs":0`, dropped: "top_logprobs"},
		{fields: `"frequency_penalty":0`, dropped: "frequency_penalty"},
		{fields: `"presence_penalty":0`, dropped: "presence_penalty"},
		{fields: `"store":false`, dropped: "store"},
		{fields: `"user":"private-test-value"`, dropped: "user"},
		{fields: `"metadata":{"label":"private-test-value"}`, dropped: "metadata"},
		{fields: `"safety_identifier":"private-test-value"`, dropped: "safety_identifier"},
		{fields: `"prompt_cache_key":"private-test-value"`, dropped: "prompt_cache_key"},
		{fields: `"prompt_cache_retention":"24h"`, dropped: "prompt_cache_retention"},
		{fields: `"service_tier":"flex"`, dropped: "service_tier"},
		{fields: `"enable_thinking":false`, dropped: "enable_thinking"},
		{fields: `"tools":[{"type":"web_search_preview"}]`, dropped: "web_search_options"},
		{fields: `"user":"private-test-value","store":false,"parallel_tool_calls":false,"top_logprobs":0`, dropped: "parallel_tool_calls, store, top_logprobs, user"},
	} {
		t.Run(test.dropped, func(t *testing.T) {
			c, _, info, request := responsesFallbackContext(t, constant.ChannelTypeMistral, "https://unused.example", `{"model":"chat-model","input":"hello",`+test.fields+`}`)
			adaptor, apiErr := newResponsesAdaptor(info)
			require.Nil(t, apiErr)
			adaptor.Init(info)
			_, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.dropped)
			assert.NotContains(t, err.Error(), "private-test-value")
			apiErr = newConvertRequestFailedError(c, info, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
			assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
			assert.Equal(t, "/v1/responses", info.RequestURLPath)
		})
	}
}

// TestResponsesChatFallbackMistralPreservesBasicOptions 验证受支持的基础请求和显式零值仍可正常发送。
func TestResponsesChatFallbackMistralPreservesBasicOptions(t *testing.T) {
	c, _, info, request := responsesFallbackContext(t, constant.ChannelTypeMistral, "https://unused.example", `{"model":"chat-model","input":"hello","stream":false,"temperature":0,"top_p":0,"max_output_tokens":0,"tool_choice":"none"}`)
	adaptor, apiErr := newResponsesAdaptor(info)
	require.Nil(t, apiErr)
	adaptor.Init(info)
	converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.Equal(t, "false", gjson.GetBytes(body, "stream").Raw)
	for _, key := range []string{"temperature", "top_p", "max_tokens"} {
		assert.Equal(t, "0", gjson.GetBytes(body, key).Raw, key)
	}
	assert.Equal(t, "none", gjson.GetBytes(body, "tool_choice").String())
	assert.Contains(t, gjson.GetBytes(body, "messages.0.content").Raw, "hello")
}

// TestResponsesChatFallbackRejectsPassThrough 验证原始 Responses 请求体不会误发到 Chat 路径。
func TestResponsesChatFallbackRejectsPassThrough(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previous := settings.PassThroughRequestEnabled
	t.Cleanup(func() { settings.PassThroughRequestEnabled = previous })
	for _, global := range []bool{false, true} {
		_, _, info, _ := responsesFallbackContext(t, constant.ChannelTypeSiliconFlow, "https://unused.example", `{"model":"chat-model","input":"hello"}`)
		settings.PassThroughRequestEnabled = global
		info.ChannelSetting.PassThroughBodyEnabled = !global
		_, apiErr := newResponsesAdaptor(info)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	}
}

// TestResponsesNativeChannelsKeepTheirAdaptor 验证已具备原生能力的渠道不会进入 Chat 兼容路径。
func TestResponsesNativeChannelsKeepTheirAdaptor(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeDeepSeek, constant.ChannelTypeSub2API} {
		c, _, info, request := responsesFallbackContext(t, channelType, "https://unused.example", `{"model":"deepseek-v4-flash-vision-exp","input":"hello","max_output_tokens":73}`)
		info.ChannelSetting.PassThroughBodyEnabled = true
		adaptor, apiErr := newResponsesAdaptor(info)
		require.Nil(t, apiErr)
		adaptor.Init(info)
		converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.Equal(t, "hello", gjson.GetBytes(body, "input").String())
		assert.Equal(t, int64(73), gjson.GetBytes(body, "max_output_tokens").Int())
		assert.False(t, gjson.GetBytes(body, "messages").Exists())
	}
}

// responsesFallbackContext 创建无数据库、无真实密钥的 Responses 测试上下文，并使用渠道既有的 StreamOptions 能力设置。
func responsesFallbackContext(t *testing.T, channelType int, baseURL, body string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *dto.OpenAIResponsesRequest) {
	t.Helper()
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal([]byte(body), &request))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, request.Model)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIResponses, RelayMode: relayconstant.RelayModeResponses,
		RequestURLPath: "/v1/responses", OriginModelName: request.Model,
		Request: &request, IsStream: request.IsStream(c.Request), DisablePing: true,
	}
	info.InitChannelMeta(c)
	info.ChannelBaseUrl = baseURL
	info.ApiKey = "test-token"
	info.InitRequestConversionChain()
	return c, recorder, info, &request
}
