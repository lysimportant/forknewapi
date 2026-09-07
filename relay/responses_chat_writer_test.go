package relay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// responsesWriterFixture 为同步渠道输出提供真实 Gin writer，保留独立计费上下文，不访问数据库或外部上游。
func responsesWriterFixture(t *testing.T, stream bool) (*responsesChatWriter, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	c.Set(common.RequestIdKey, "chat-writer-fixture")
	info := &relaycommon.RelayInfo{IsStream: stream, RelayFormat: types.RelayFormatOpenAIResponses, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "chat-model"}}
	w, apiErr := newResponsesChatWriter(c, info)
	require.Nil(t, apiErr)
	return w, recorder, info
}

// responsesWriterEvents 解析实际下发的 SSE，同时验证事件名、JSON 类型和连续序号契约。
func responsesWriterEvents(t *testing.T, body string) []gjson.Result {
	t.Helper()
	var events []gjson.Result
	for _, frame := range strings.Split(body, "\n\n") {
		if strings.TrimSpace(frame) == "" || strings.HasPrefix(frame, ":") {
			continue
		}
		lines := strings.Split(frame, "\n")
		require.Len(t, lines, 2)
		require.True(t, strings.HasPrefix(lines[1], "data: "))
		data := strings.TrimPrefix(lines[1], "data: ")
		require.True(t, gjson.Valid(data))
		event := gjson.Parse(data)
		assert.Equal(t, "event: "+event.Get("type").String(), lines[0])
		require.True(t, event.Get("sequence_number").Exists())
		assert.Equal(t, int64(len(events)), event.Get("sequence_number").Int())
		events = append(events, event)
	}
	return events
}

// TestResponsesChatWriterText 验证分片、CRLF、多行 data、尾部用量与完整文本生命周期，终态必须等待渠道结算用量。
func TestResponsesChatWriterText(t *testing.T) {
	w, recorder, _ := responsesWriterFixture(t, true)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Content-Length", "999")
	w.Header().Set("X-Codex-Turn-State", "fixture-state")
	for _, part := range []string{"da", "ta: {\"object\":\"chat.completion.chunk\",\r\n", "data: \"choices\":[{\"index\":0,\"delta\":{\"content\":\"你好\"}}]}\r", "\n\r\n"} {
		_, err := w.WriteString(part)
		require.NoError(t, err)
	}
	assert.Contains(t, recorder.Body.String(), "你好", "首段文本必须即时下发")
	assert.NotContains(t, recorder.Body.String(), "response.completed")
	_, err := w.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":99,\"completion_tokens\":88,\"total_tokens\":187}}\n\ndata: [DONE]\n\n")
	require.NoError(t, err)
	assert.NotContains(t, recorder.Body.String(), "response.completed")
	usage := &dto.Usage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 2}, CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokens: 1}}
	usage.BillingUsage = dto.NewOpenAIChatBillingUsage(usage)
	billingSnapshot := usage.BillingUsage
	require.Nil(t, w.Finish(usage, nil))
	require.Same(t, billingSnapshot, usage.BillingUsage)
	assert.Equal(t, 7, usage.PromptTokens)
	events := responsesWriterEvents(t, recorder.Body.String())
	var kinds []string
	for _, event := range events {
		kinds = append(kinds, event.Get("type").String())
		if event.Get("type").String() == "response.output_text.done" {
			assert.Equal(t, "你好", event.Get("text").String())
		}
	}
	assert.Equal(t, []string{"response.created", "response.in_progress", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"}, kinds)
	last := events[len(events)-1]
	assert.Equal(t, int64(7), last.Get("response.usage.input_tokens").Int())
	assert.Equal(t, int64(3), last.Get("response.usage.output_tokens").Int())
	assert.Equal(t, int64(2), last.Get("response.usage.input_tokens_details.cached_tokens").Int())
	assert.Equal(t, int64(1), last.Get("response.usage.output_tokens_details.reasoning_tokens").Int())
	assert.JSONEq(t, `{"input_tokens":7,"output_tokens":3,"total_tokens":10,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1}}`, last.Get("response.usage").Raw)
	assert.Equal(t, "fixture-state", recorder.Header().Get("X-Codex-Turn-State"))
	assert.Empty(t, recorder.Header().Get("Content-Length"))
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	before := recorder.Body.String()
	require.Nil(t, w.Finish(usage, nil))
	assert.Equal(t, before, recorder.Body.String(), "终态只能生成一次")
}

// TestResponsesChatWriterTool 验证函数名称与参数分片、稳定调用 ID 以及 arguments.done 中的完整 JSON 字符串。
func TestResponsesChatWriterTool(t *testing.T) {
	w, recorder, _ := responsesWriterFixture(t, true)
	for _, chunk := range []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_","arguments":"{\"path\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"file","arguments":"\"README.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
	} {
		_, err := w.WriteString("data: " + chunk + "\n\n")
		require.NoError(t, err)
	}
	require.Nil(t, w.Finish(&dto.Usage{PromptTokens: 2, CompletionTokens: 4, TotalTokens: 6}, nil))
	events := responsesWriterEvents(t, recorder.Body.String())
	for _, event := range events {
		switch event.Get("type").String() {
		case "response.output_item.added":
			assert.Equal(t, "in_progress", event.Get("item.status").String())
			assert.Equal(t, "call_1", event.Get("item.call_id").String())
			assert.Equal(t, "", event.Get("item.arguments").String())
		case "response.function_call_arguments.done":
			assert.Equal(t, `{"path":"README.md"}`, event.Get("arguments").String())
			assert.Equal(t, "call_1", event.Get("item_id").String())
		case "response.completed":
			assert.Equal(t, "read_file", event.Get("response.output.0.name").String())
			assert.Equal(t, "call_1", event.Get("response.output.0.call_id").String())
		}
	}
}

// TestResponsesChatWriterBufferedJSON 验证 SDK 一次性 JSON 对流式客户端仍输出完整 SSE，并共享非流式 JSON 的用量与推理字段。
func TestResponsesChatWriterBufferedJSON(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sdk-json-to-sse"
		}
		t.Run(name, func(t *testing.T) {
			w, recorder, _ := responsesWriterFixture(t, stream)
			w.Header().Set("Content-Type", "application/json")
			body := `{"object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"OK","reasoning_content":"think"},"finish_reason":"length"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
			_, err := w.WriteString(body[:30])
			require.NoError(t, err)
			_, err = w.WriteString(body[30:])
			require.NoError(t, err)
			w.Flush()
			assert.Empty(t, recorder.Body.String(), "JSON 校验前不能提交下游")
			require.Nil(t, w.Finish(&dto.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}, nil))
			response := gjson.Parse(recorder.Body.String())
			if stream {
				events := responsesWriterEvents(t, recorder.Body.String())
				require.NotEmpty(t, events)
				assert.Equal(t, "response.incomplete", events[len(events)-1].Get("type").String())
				response = events[len(events)-1].Get("response")
			}
			assert.Equal(t, "incomplete", response.Get("status").String())
			assert.Equal(t, "max_output_tokens", response.Get("incomplete_details.reason").String())
			assert.Equal(t, int64(5), response.Get("usage.input_tokens").Int())
			var reasoning gjson.Result
			response.Get("output").ForEach(func(_, item gjson.Result) bool {
				if item.Get("type").String() == "reasoning" {
					reasoning = item
				}
				return true
			})
			assert.Equal(t, "think", reasoning.Get("summary.0.text").String())
			assert.False(t, reasoning.Get("content").Exists())
		})
	}
}

// TestResponsesChatWriterFailures 保护开流后的截断、JSON 错误、上游异常和超时，禁止伪造 completed 或重复写错误。
func TestResponsesChatWriterFailures(t *testing.T) {
	for _, name := range []string{"missing-finish", "malformed", "upstream-error", "timeout", "unknown-tool", "incomplete-frame"} {
		t.Run(name, func(t *testing.T) {
			w, recorder, info := responsesWriterFixture(t, true)
			_, err := w.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n")
			require.NoError(t, err)
			var upstreamErr *types.NewAPIError
			switch name {
			case "malformed":
				_, err = w.WriteString("data: {invalid}\n\n")
				require.Error(t, err)
			case "upstream-error":
				upstreamErr = types.NewError(errors.New("upstream stopped"), types.ErrorCodeBadResponse)
			case "timeout":
				info.StreamStatus = relaycommon.NewStreamStatus()
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
			case "unknown-tool":
				_, err = w.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"type\":\"custom\"}]}}]}\n\n")
				require.Error(t, err)
			case "incomplete-frame":
				_, err = w.WriteString("data: {\"choices\":")
				require.NoError(t, err)
			}
			usage := &dto.Usage{PromptTokens: 6, CompletionTokens: 2, TotalTokens: 8}
			require.Nil(t, w.Finish(usage, upstreamErr))
			require.Nil(t, w.Finish(usage, upstreamErr))
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error\n"))
			assert.NotContains(t, recorder.Body.String(), "response.completed")
			require.NotNil(t, info.StreamStatus)
			assert.True(t, info.StreamStatus.HasErrors())
			assert.Equal(t, 8, usage.TotalTokens)
			responsesWriterEvents(t, recorder.Body.String())
		})
	}
}

// responsesFailingClient 模拟客户端短写；计数用于确认 Finish 不会继续写错误帧。
type responsesFailingClient struct {
	gin.ResponseWriter
	writes int
}

// Write 返回真实可观测的短写，不修改测试之外的连接。
func (w *responsesFailingClient) Write(data []byte) (int, error) {
	w.writes++
	return len(data) - 1, nil
}

// TestResponsesChatWriterDisconnected 确保短写和取消保留已发生用量且不追加错误或返回可重试错误。
func TestResponsesChatWriterDisconnected(t *testing.T) {
	t.Run("short-write", func(t *testing.T) {
		w, _, info := responsesWriterFixture(t, true)
		client := &responsesFailingClient{ResponseWriter: w.ResponseWriter}
		w.ResponseWriter, w.client.Writer = client, client
		_, err := w.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"}}]}\n\n")
		require.ErrorIs(t, err, io.ErrShortWrite)
		require.Nil(t, w.Finish(&dto.Usage{TotalTokens: 3}, nil))
		assert.Equal(t, 1, client.writes)
		require.NotNil(t, info.StreamStatus)
		assert.True(t, info.StreamStatus.HasErrors())
	})
	t.Run("cancel", func(t *testing.T) {
		w, recorder, _ := responsesWriterFixture(t, true)
		ctx, cancel := context.WithCancel(w.client.Request.Context())
		w.client.Request = w.client.Request.WithContext(ctx)
		_, err := w.WriteString("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"}}]}\n\n")
		require.NoError(t, err)
		before := recorder.Body.String()
		cancel()
		require.Nil(t, w.Finish(&dto.Usage{TotalTokens: 3}, nil))
		assert.Equal(t, before, recorder.Body.String())
	})
}

// TestResponsesChatWriterRejectsInvalidJSON 在尚未向客户端写入时返回明确错误，不把空响应、HTML 或未知工具伪装成成功。
func TestResponsesChatWriterRejectsInvalidJSON(t *testing.T) {
	for _, body := range []string{`<html>error</html>`, `{"choices":[]}`, `{"error":{"message":"provider error"}}`, `{"choices":[{"index":0,"message":{"content":"OK"}}]}`, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"x","type":"custom"}]},"finish_reason":"tool_calls"}]}`} {
		t.Run(body, func(t *testing.T) {
			w, recorder, _ := responsesWriterFixture(t, false)
			_, err := w.WriteString(body)
			require.NoError(t, err)
			apiErr := w.Finish(nil, nil)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			assert.Empty(t, recorder.Body.String())
		})
	}
}

// TestResponsesChatWriterUsageFallback 验证供应商返回 nil 时使用已报告用量，显式零值不被估算覆盖；没有用量时保留部分生成费用。
func TestResponsesChatWriterUsageFallback(t *testing.T) {
	for _, test := range []struct {
		name       string
		usage      string
		wantPrompt int
		wantOutput int
	}{
		{"reported", `,"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}`, 8, 4},
		{"explicit-zero", `,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`, 0, 0},
		{"estimated", "", 7, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			w, _, info := responsesWriterFixture(t, true)
			info.SetEstimatePromptTokens(7)
			_, err := w.WriteString(`data: {"choices":[{"index":0,"delta":{"content":"partial generated text"}}]` + test.usage + "}\n\n")
			require.NoError(t, err)
			usage := w.UsageFallback()
			require.NotNil(t, usage)
			assert.Equal(t, test.wantPrompt, usage.PromptTokens)
			if test.wantOutput < 0 {
				assert.Positive(t, usage.CompletionTokens)
			} else {
				assert.Equal(t, test.wantOutput, usage.CompletionTokens)
			}
		})
	}
	t.Run("buffered-json", func(t *testing.T) {
		w, _, _ := responsesWriterFixture(t, true)
		_, err := w.WriteString(`{"choices":[{"index":0,"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}}`)
		require.NoError(t, err)
		usage := w.UsageFallback()
		require.NotNil(t, usage)
		assert.Equal(t, 12, usage.TotalTokens)
		require.Nil(t, w.Finish(usage, nil))
	})
}

// TestResponsesChatWriterClaudeUsage 仅为客户端转换 Anthropic 缓存输入语义，计费对象仍保持非缓存输入及原始快照。
func TestResponsesChatWriterClaudeUsage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			w, recorder, _ := responsesWriterFixture(t, stream)
			_, err := w.WriteString(`{"choices":[{"index":0,"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":18,"completion_tokens":3,"total_tokens":21}}`)
			require.NoError(t, err)
			usage := &dto.Usage{UsageSemantic: dto.BillingUsageSemanticAnthropic, UsageSource: dto.BillingUsageSourceClaudeMessages, PromptTokens: 11, CompletionTokens: 3, TotalTokens: 14, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 5, CachedCreationTokens: 2}, ClaudeCacheCreation5mTokens: 2}
			usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{InputTokens: 11, OutputTokens: 3, CacheReadInputTokens: 5, CacheCreationInputTokens: 2})
			snapshot := usage.BillingUsage
			require.Nil(t, w.Finish(usage, nil))
			assert.Equal(t, 11, usage.PromptTokens)
			assert.Equal(t, 14, usage.TotalTokens)
			require.Same(t, snapshot, usage.BillingUsage)
			response := gjson.Parse(recorder.Body.String())
			if stream {
				events := responsesWriterEvents(t, recorder.Body.String())
				response = events[len(events)-1].Get("response")
			}
			assert.Equal(t, int64(18), response.Get("usage.input_tokens").Int())
			assert.Equal(t, int64(21), response.Get("usage.total_tokens").Int())
			assert.JSONEq(t, `{"input_tokens":18,"output_tokens":3,"total_tokens":21,"input_tokens_details":{"cached_tokens":5,"cache_write_tokens":2},"output_tokens_details":{"reasoning_tokens":0}}`, response.Get("usage").Raw)
		})
	}
}

// TestResponsesChatWriterOpenAIUsageTerminal 复现 SiliconFlow 等 Chat 渠道在默认不请求独立用量帧时，工具终态与 usage 同帧的完整输出链路。
func TestResponsesChatWriterOpenAIUsageTerminal(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	w, recorder, info := responsesWriterFixture(t, true)
	info.DisablePing = true
	info.RelayFormat = types.RelayFormatOpenAI
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.ChannelType = constant.ChannelTypeSiliconFlow
	ctx := w.client.Copy()
	ctx.Writer = w
	body := "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_fixture","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"README.md\""}}]}}]}` + "\n\ndata: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":374,"completion_tokens":78,"total_tokens":452,"prompt_tokens_details":{"cached_tokens":100,"cache_write_tokens":12},"completion_tokens_details":{"reasoning_tokens":42}}}` + "\n\ndata: [DONE]\n\n"
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
	usage, apiErr := openai.OaiStreamHandler(ctx, info, response)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 374, usage.PromptTokens)
	assert.Equal(t, 78, usage.CompletionTokens)
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.RelayMode = relayconstant.RelayModeResponses
	require.Nil(t, w.Finish(usage, nil))
	events := responsesWriterEvents(t, recorder.Body.String())
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	assert.Equal(t, "response.completed", last.Get("type").String())
	assert.Equal(t, "read_file", last.Get("response.output.0.name").String())
	assert.Equal(t, "call_fixture", last.Get("response.output.0.call_id").String())
	assert.Equal(t, `{"path":"README.md"}`, last.Get("response.output.0.arguments").String())
	assert.JSONEq(t, `{"input_tokens":374,"output_tokens":78,"total_tokens":452,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":12},"output_tokens_details":{"reasoning_tokens":42}}`, last.Get("response.usage").Raw)
	assert.False(t, info.StreamStatus.HasErrors())
}

// TestResponsesChatWriterPreRequestPing 确保真正响应前的请求保活不提前开流，上游请求失败仍可返回正确 HTTP 错误。
func TestResponsesChatWriterPreRequestPing(t *testing.T) {
	w, recorder, _ := responsesWriterFixture(t, true)
	_, err := w.WriteString(": PING\n\n")
	require.NoError(t, err)
	w.Flush()
	assert.Empty(t, recorder.Body.String())
	assert.False(t, w.ResponseWriter.Written())
	upstreamErr := types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeDoRequestFailed, http.StatusServiceUnavailable)
	require.Same(t, upstreamErr, w.Finish(nil, upstreamErr))
	assert.Empty(t, recorder.Body.String())
}
