package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// responsesBillingObserver 记录协议适配器是否越过职责边界修改计费会话；不访问账户或数据库。
type responsesBillingObserver struct {
	// mutations 统计预扣、结算和退款次数，所有调用均属于不应发生的内部副作用。
	mutations int
}

// responsesHTTPRecorder 为 Gin 的旧 c.Stream 回调提供连接关闭通知；这些同步 HTTP 契约夹具保持客户端连接。
type responsesHTTPRecorder struct {
	*httptest.ResponseRecorder
	// closed 在当前夹具生命周期中不关闭；客户端取消行为由独立编码器测试覆盖。
	closed chan bool
}

// CloseNotify 满足 Gin 旧流接口，仅返回该测试客户端的连接通知通道。
func (w *responsesHTTPRecorder) CloseNotify() <-chan bool { return w.closed }

// Settle 记录结算尝试，实际金额由外层 ResponsesHelper 负责。
func (b *responsesBillingObserver) Settle(int) error { b.mutations++; return nil }

// Refund 记录退款尝试，不执行异步操作。
func (b *responsesBillingObserver) Refund(*gin.Context) { b.mutations++ }

// NeedsRefund 表示该测试会话尚未结算。
func (b *responsesBillingObserver) NeedsRefund() bool { return true }

// GetPreConsumedQuota 返回固定预扣额度，单位为内部 quota。
func (b *responsesBillingObserver) GetPreConsumedQuota() int { return 100 }

// Reserve 记录补预扣尝试，不修改账户。
func (b *responsesBillingObserver) Reserve(int) error { b.mutations++; return nil }

// responsesChannelContext 构造独立的 Responses 客户端请求和渠道信息，测试凭据仅为明确的占位值。
func responsesChannelContext(t *testing.T, channelType int, model, input string, stream bool) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *dto.OpenAIResponsesRequest) {
	t.Helper()
	request := &dto.OpenAIResponsesRequest{
		Model: model, Input: []byte(input), User: []byte(`"responses-fixture-user"`),
		Stream: common.GetPointer(stream), MaxOutputTokens: common.GetPointer(uint(64)),
	}
	if channelType == constant.ChannelTypePaLM {
		request.MaxOutputTokens = nil
	}
	body, err := common.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(&responsesHTTPRecorder{ResponseRecorder: recorder, closed: make(chan bool)})
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, "responses-channel-fixture")
	ctx.Set("bot_id", "fixture-bot")
	apiType, _ := common.ChannelType2APIType(channelType)
	info := &relaycommon.RelayInfo{
		Request: request, RelayFormat: types.RelayFormatOpenAIResponses,
		RelayMode: relayconstant.RelayModeResponses, RequestURLPath: "/v1/responses",
		OriginModelName: model, IsStream: stream, DisablePing: true,
		StartTime: time.Now(), ShouldIncludeUsage: true, Billing: &responsesBillingObserver{},
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{BuiltInTools: make(map[string]*relaycommon.BuildInToolInfo)},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: channelType, ApiType: apiType, ChannelBaseUrl: "http://127.0.0.1:1",
			ApiKey: "test-only-upstream-token", UpstreamModelName: model,
		},
	}
	info.SetEstimatePromptTokens(11)
	switch channelType {
	case constant.ChannelTypeTencent:
		info.ApiKey = "1|test-only-secret-id|test-only-secret-key"
	case constant.ChannelTypeAws:
		info.ApiKey = "test-only-access-key|test-only-secret-key|us-east-1"
	case constant.ChannelTypeXunfei:
		info.ApiKey = "fixture-app|test-only-secret|test-only-key"
	case constant.ChannelTypeBaidu:
		info.ApiKey = "test-only-client-id|test-only-client-secret"
	case constant.ChannelTypeZhipu, constant.ChannelTypeZhipu_v4:
		info.ApiKey = "test-only-key-id.test-only-secret"
	case constant.ChannelTypeVertexAi:
		info.ChannelOtherSettings.VertexKeyType = dto.VertexKeyTypeAPIKey
		info.ApiVersion = "us-central1"
	case constant.ChannelCloudflare:
		info.ApiVersion = "fixture-account"
	case constant.ChannelTypeCodex:
		info.ApiKey = `{"access_token":"test-only-access-token","account_id":"fixture-account"}`
	}
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, info.ApiKey)
	return ctx, recorder, info, request
}

// TestResponsesAllAPITypeRequestConversion 验证全部 APIType 经真实适配器产生可观察的上游请求，非文本适配器在出站前返回 400。
func TestResponsesAllAPITypeRequestConversion(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	// textPath 对应真实上游协议中的用户文本字段；这同时保护公共路由选择和供应商请求转换。
	tests := map[int]struct {
		name        string
		channelType int
		model       string
		textPath    string
		wantText    string
		reject      bool
	}{
		constant.APITypeOpenAI:         {"OpenAI", constant.ChannelTypeOpenAI, "test-model", "input", "hello", false},
		constant.APITypeAnthropic:      {"Anthropic", constant.ChannelTypeAnthropic, "claude-sonnet-4-20250514", "messages.0.content", "hello", false},
		constant.APITypePaLM:           {"PaLM", constant.ChannelTypePaLM, "chat-bison-001", "prompt.messages.0.content", "hello", false},
		constant.APITypeBaidu:          {"Baidu", constant.ChannelTypeBaidu, "ERNIE-Bot", "messages.0.content", "hello", false},
		constant.APITypeZhipu:          {"Zhipu", constant.ChannelTypeZhipu, "chatglm_std", "prompt.0.content", "hello", false},
		constant.APITypeAli:            {"Ali", constant.ChannelTypeAli, "qwen-plus", "input", "hello", false},
		constant.APITypeXunfei:         {"Xunfei", constant.ChannelTypeXunfei, "SparkDesk", "messages.0.content", "hello", false},
		constant.APITypeAIProxyLibrary: {"AIProxyLibrary", constant.ChannelTypeAIProxyLibrary, "test-model", "", "", true},
		constant.APITypeTencent:        {"Tencent", constant.ChannelTypeTencent, "hunyuan-lite", "Messages.0.Content", "hello", false},
		constant.APITypeGemini:         {"Gemini", constant.ChannelTypeGemini, "gemini-2.5-flash", "contents.0.parts.0.text", "hello", false},
		constant.APITypeZhipuV4:        {"ZhipuV4", constant.ChannelTypeZhipu_v4, "glm-4", "messages.0.content", "hello", false},
		constant.APITypeOllama:         {"Ollama", constant.ChannelTypeOllama, "llama3", "messages.0.content", "hello", false},
		constant.APITypePerplexity:     {"Perplexity", constant.ChannelTypePerplexity, "sonar", "input", "hello", false},
		constant.APITypeAws:            {"AWS Nova", constant.ChannelTypeAws, "amazon.nova-lite-v1:0", "messages.0.content.0.text", "hello", false},
		constant.APITypeCohere:         {"Cohere", constant.ChannelTypeCohere, "command-r", "message", "hello", false},
		constant.APITypeDify:           {"Dify", constant.ChannelTypeDify, "chat-model", "query", "USER: \nhello\n", false},
		constant.APITypeJina:           {"Jina", constant.ChannelTypeJina, "jina-clip-v1", "", "", true},
		constant.APITypeCloudflare:     {"Cloudflare", constant.ChannelCloudflare, "@cf/test-model", "input", "hello", false},
		constant.APITypeSiliconFlow:    {"SiliconFlow", constant.ChannelTypeSiliconFlow, "test-model", "messages.0.content", "hello", false},
		constant.APITypeVertexAi:       {"Vertex Gemini", constant.ChannelTypeVertexAi, "gemini-2.5-flash", "contents.0.parts.0.text", "hello", false},
		constant.APITypeMistral:        {"Mistral", constant.ChannelTypeMistral, "mistral-small-latest", "messages.0.content.0.text", "hello", false},
		constant.APITypeSub2API:        {"Sub2API", constant.ChannelTypeSub2API, "test-model", "input", "hello", false},
		constant.APITypeNewAPI:         {"NewAPI", constant.ChannelTypeNewAPI, "test-model", "input", "hello", false},
		constant.APITypeDeepSeek:       {"DeepSeek", constant.ChannelTypeDeepSeek, "deepseek-v4-flash-vision-exp", "input", "hello", false},
		constant.APITypeMokaAI:         {"MokaAI", constant.ChannelTypeMokaAI, "moka-ai/m3e-base", "", "", true},
		constant.APITypeVolcEngine:     {"VolcEngine", constant.ChannelTypeVolcEngine, "doubao-test", "input", "hello", false},
		constant.APITypeBaiduV2:        {"BaiduV2", constant.ChannelTypeBaiduV2, "ernie-4.5", "messages.0.content", "hello", false},
		constant.APITypeOpenRouter:     {"OpenRouter", constant.ChannelTypeOpenRouter, "test/model", "input", "hello", false},
		constant.APITypeXinference:     {"Xinference", constant.ChannelTypeXinference, "test-model", "input", "hello", false},
		constant.APITypeXai:            {"Xai", constant.ChannelTypeXai, "grok-test", "input", "hello", false},
		constant.APITypeCoze:           {"Coze", constant.ChannelTypeCoze, "chat-model", "additional_messages.0.content", "hello", false},
		constant.APITypeJimeng:         {"Jimeng", constant.ChannelTypeJimeng, "jimeng_high_aes_general_v21_L", "", "", true},
		constant.APITypeMoonshot:       {"Moonshot", constant.ChannelTypeMoonshot, "moonshot-v1-8k", "messages.0.content", "hello", false},
		constant.APITypeSubmodel:       {"Submodel", constant.ChannelTypeSubmodel, "test-model", "messages.0.content", "hello", false},
		constant.APITypeMiniMax:        {"MiniMax", constant.ChannelTypeMiniMax, "MiniMax-M2.5", "messages.0.content", "hello", false},
		constant.APITypeReplicate:      {"Replicate", constant.ChannelTypeReplicate, "owner/image-model", "", "", true},
		constant.APITypeCodex:          {"Codex", constant.ChannelTypeCodex, "gpt-test", "input", "hello", false},
		constant.APITypeAdvancedCustom: {"AdvancedCustom", constant.ChannelTypeAdvancedCustom, "test-model", "input", "hello", false},
	}
	for apiType := 0; apiType < constant.APITypeDummy; apiType++ {
		test, ok := tests[apiType]
		require.True(t, ok, "新增 APIType %d 必须提供实际请求转换或明确拒绝的契约用例", apiType)
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder, info, request := responsesChannelContext(t, test.channelType, test.model, `"hello"`, false)
			require.Equal(t, apiType, info.ApiType)
			if apiType == constant.APITypeAdvancedCustom {
				info.ChannelOtherSettings.AdvancedCustom = &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/responses"}}}
			}
			adaptor, apiErr := newResponsesAdaptor(info)
			if test.reject {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				assert.True(t, types.IsSkipRetryError(apiErr))
				assert.Nil(t, adaptor)
				assert.Empty(t, recorder.Body.String())
				return
			}
			require.Nil(t, apiErr)
			require.NotNil(t, adaptor)
			adaptor.Init(info)
			converted, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, *request)
			require.NoError(t, err)
			outbound, err := common.Marshal(converted)
			require.NoError(t, err)
			assert.Equal(t, test.wantText, gjson.GetBytes(outbound, test.textPath).String(), "上游请求: %s", outbound)
			assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
			assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
			assert.Equal(t, "/v1/responses", info.RequestURLPath)
			assert.Same(t, request, info.Request)
			assert.Empty(t, recorder.Body.String(), "请求转换不得写客户端或调用上游")
			assert.Zero(t, info.Billing.(*responsesBillingObserver).mutations)
		})
	}
}

// TestResponsesRejectTaskChannelFallback 保护非文本任务渠道不会因默认 OpenAI 映射而误发生成请求。
func TestResponsesRejectTaskChannelFallback(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus, constant.ChannelTypeSunoAPI, constant.ChannelTypeKling, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora} {
		t.Run(constant.ChannelTypeNames[channelType], func(t *testing.T) {
			_, _, info, _ := responsesChannelContext(t, channelType, "test-model", `"hello"`, false)
			adaptor, apiErr := newResponsesAdaptor(info)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.Nil(t, adaptor)
		})
	}
}

// TestResponsesPaLMRejectsOutputLimitBeforeRequest 保护 PaLM 无法表达的输出上限不会被静默丢弃或转换成 500。
func TestResponsesPaLMRejectsOutputLimitBeforeRequest(t *testing.T) {
	ctx, recorder, info, request := responsesChannelContext(t, constant.ChannelTypePaLM, "chat-bison-001", `"hello"`, false)
	request.MaxOutputTokens = common.GetPointer(uint(64))
	adaptor, apiErr := newResponsesAdaptor(info)
	require.Nil(t, apiErr)
	adaptor.Init(info)
	_, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, *request)
	require.Error(t, err)
	var convertedError *types.NewAPIError
	require.ErrorAs(t, err, &convertedError)
	assert.Equal(t, http.StatusBadRequest, convertedError.StatusCode)
	assert.True(t, types.IsSkipRetryError(convertedError))
	assert.Contains(t, err.Error(), "max")
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
	assert.Same(t, request, info.Request)
	assert.Empty(t, recorder.Body.String())
	assert.Zero(t, info.Billing.(*responsesBillingObserver).mutations)
}

// TestResponsesProviderHTTPContracts 验证真实本地 HTTP 出站、各供应商响应解码及最终 Responses 文本/工具/用量；整个过程不访问外网。
func TestResponsesProviderHTTPContracts(t *testing.T) {
	service.InitHttpClient()
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	for _, test := range []struct {
		name        string
		channelType int
		model       string
		path        string
		inputPath   string
		contentType string
		body        string
		stream      bool
		tool        string
		cache       int
		history     bool
		estimate    bool
	}{
		{
			name: "Claude 文本和工具及缓存计费", channelType: constant.ChannelTypeAnthropic, model: "claude-sonnet-4-20250514", path: "/v1/messages", inputPath: "messages.0.content", contentType: "application/json", tool: "read_file", cache: 5,
			body: `{"id":"msg_fixture","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"OK"},{"type":"tool_use","id":"call_fixture","name":"read_file","input":{"path":"README.md"}}],"stop_reason":"tool_use","usage":{"input_tokens":11,"output_tokens":3,"cache_read_input_tokens":5,"cache_creation_input_tokens":2}}`,
		},
		{
			name: "Cohere 原生逐行流", channelType: constant.ChannelTypeCohere, model: "command-r", path: "/v1/chat", inputPath: "message", contentType: "application/x-ndjson", stream: true,
			body: "{\"event_type\":\"text-generation\",\"text\":\"OK\",\"is_finished\":false}\n{\"event_type\":\"stream-end\",\"is_finished\":true,\"finish_reason\":\"COMPLETE\",\"response\":{\"meta\":{\"billed_units\":{\"input_tokens\":11,\"output_tokens\":3}}}}\n",
		},
		{
			name: "Ollama 原生逐行流和工具", channelType: constant.ChannelTypeOllama, model: "llama3", path: "/api/chat", inputPath: "messages.2.content", contentType: "application/x-ndjson", stream: true, tool: "read_file", history: true,
			body: "{\"model\":\"llama3\",\"message\":{\"role\":\"assistant\",\"content\":\"OK\",\"tool_calls\":[{\"function\":{\"name\":\"read_file\",\"arguments\":{\"path\":\"README.md\"}}}]},\"done\":false}\n{\"model\":\"llama3\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":11,\"eval_count\":3}\n",
		},
		{
			name: "PaLM 专用请求和JSON响应", channelType: constant.ChannelTypePaLM, model: "chat-bison-001", path: "/v1beta2/models/chat-bison-001:generateMessage", inputPath: "prompt.messages.0.content", contentType: "application/json", estimate: true,
			body: `{"candidates":[{"author":"1","content":"OK"}]}`,
		},
		{
			name: "Dify 应用响应", channelType: constant.ChannelTypeDify, model: "chat-model", path: "/v1/chat-messages", inputPath: "query", contentType: "application/json",
			body: `{"event":"message","conversation_id":"conversation_fixture","answer":"OK","metadata":{"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}}}`,
		},
		{
			name: "Mistral 工具历史和Chat SSE", channelType: constant.ChannelTypeMistral, model: "mistral-small-latest", path: "/v1/chat/completions", inputPath: "messages.2.content.0.text", contentType: "text/event-stream", stream: true, history: true,
			body: "data: {\"id\":\"chat_fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"mistral-small-latest\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chat_fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":3,\"total_tokens\":14}}\n\ndata: [DONE]\n\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 请求计数在服务器 goroutine 中递增；缓冲通道只承载一次实际出站的请求体。
			var calls atomic.Int32
			requests := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, test.path, r.URL.Path)
				body, err := io.ReadAll(r.Body)
				if !assert.NoError(t, err) {
					http.Error(w, "cannot read fixture request", http.StatusBadRequest)
					return
				}
				select {
				case requests <- body:
				default:
					t.Error("协议适配重复发送生成请求")
				}
				w.Header().Set("Content-Type", test.contentType)
				_, _ = io.WriteString(w, test.body)
			}))
			defer upstream.Close()
			input := `"hello"`
			if test.history {
				input = `[{"type":"function_call","call_id":"call_fixture","name":"read_file","arguments":"{\"path\":\"README.md\"}"},{"type":"function_call_output","call_id":"call_fixture","output":"project description"},{"role":"user","content":"hello"}]`
			}
			ctx, recorder, info, request := responsesChannelContext(t, test.channelType, test.model, input, test.stream)
			info.ChannelBaseUrl = upstream.URL
			originalRequest := ctx.Request
			originalWriter := ctx.Writer
			adaptor, apiErr := newResponsesAdaptor(info)
			require.Nil(t, apiErr)
			adaptor.Init(info)
			converted, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, *request)
			require.NoError(t, err)
			outbound, err := common.Marshal(converted)
			require.NoError(t, err)
			response, err := adaptor.DoRequest(ctx, info, bytes.NewReader(outbound))
			require.NoError(t, err)
			httpResponse, ok := response.(*http.Response)
			require.True(t, ok)
			defer httpResponse.Body.Close()
			require.Equal(t, http.StatusOK, httpResponse.StatusCode)
			usageValue, apiErr := adaptor.DoResponse(ctx, httpResponse, info)
			require.Nil(t, apiErr)
			usage, ok := usageValue.(*dto.Usage)
			require.True(t, ok)
			require.NotNil(t, usage)
			assert.Equal(t, int32(1), calls.Load())
			upstreamBody := <-requests
			outboundText := gjson.GetBytes(upstreamBody, test.inputPath).String()
			if test.channelType == constant.ChannelTypeDify {
				assert.Equal(t, "USER: \nhello\n", outboundText)
			} else {
				assert.Equal(t, "hello", outboundText)
			}
			if test.history {
				assert.Equal(t, "read_file", gjson.GetBytes(upstreamBody, "messages.0.tool_calls.0.function.name").String())
				if test.channelType == constant.ChannelTypeMistral {
					callID := gjson.GetBytes(upstreamBody, "messages.0.tool_calls.0.id").String()
					assert.Regexp(t, `^[a-zA-Z0-9]{9}$`, callID)
					assert.Equal(t, callID, gjson.GetBytes(upstreamBody, "messages.1.tool_call_id").String())
					assert.Equal(t, "project description", gjson.GetBytes(upstreamBody, "messages.1.content.0.text").String())
				} else {
					assert.Equal(t, "read_file", gjson.GetBytes(upstreamBody, "messages.1.tool_name").String())
					assert.Equal(t, "project description", gjson.GetBytes(upstreamBody, "messages.1.content").String())
				}
			}
			assert.Equal(t, 11, usage.PromptTokens)
			if test.estimate {
				assert.Positive(t, usage.CompletionTokens)
				assert.Equal(t, 11+usage.CompletionTokens, usage.TotalTokens)
			} else {
				assert.Equal(t, 3, usage.CompletionTokens)
			}
			assert.Equal(t, test.cache, usage.PromptTokensDetails.CachedTokens)
			if test.cache > 0 {
				assert.Equal(t, "anthropic", usage.UsageSemantic)
				require.NotNil(t, usage.BillingUsage, "内部 Chat 输出不得覆盖原始 Anthropic 计费元数据")
				assert.Equal(t, 2, usage.PromptTokensDetails.CachedCreationTokens)
			}
			assert.Zero(t, info.Billing.(*responsesBillingObserver).mutations, "适配器不得重复结算、预扣或退款")
			assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
			assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
			assert.Equal(t, "/v1/responses", info.RequestURLPath)
			assert.Equal(t, test.stream, info.IsStream)
			assert.Same(t, request, info.Request)
			assert.Same(t, originalRequest, ctx.Request)
			assert.Same(t, originalWriter, ctx.Writer)
			result := gjson.ParseBytes(recorder.Body.Bytes())
			if test.stream {
				assert.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
				assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed\n"))
				assert.NotContains(t, recorder.Body.String(), "data: [DONE]")
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if strings.HasPrefix(line, "data: ") {
						event := gjson.Parse(strings.TrimPrefix(line, "data: "))
						if event.Get("type").String() == "response.completed" {
							result = event.Get("response")
						}
					}
				}
			}
			assert.Equal(t, "response", result.Get("object").String())
			assert.Equal(t, "completed", result.Get("status").String())
			assert.Equal(t, "OK", result.Get(`output.#(type=="message").content.0.text`).String())
			assert.Equal(t, int64(usage.CompletionTokens), result.Get("usage.output_tokens").Int())
			clientInputTokens := int64(11)
			if test.cache > 0 {
				// Responses 输入总数包含缓存读取和创建；原始 Anthropic 计费用量仍保留非缓存输入 11。
				clientInputTokens = 18
			}
			assert.Equal(t, clientInputTokens, result.Get("usage.input_tokens").Int())
			assert.Equal(t, clientInputTokens+int64(usage.CompletionTokens), result.Get("usage.total_tokens").Int())
			if test.tool != "" {
				call := result.Get(`output.#(type=="function_call")`)
				assert.Equal(t, test.tool, call.Get("name").String())
				assert.NotEmpty(t, call.Get("call_id").String())
				assert.JSONEq(t, `{"path":"README.md"}`, call.Get("arguments").String())
			}
		})
	}
}
