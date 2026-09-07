package openai

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newNativeResponsesStreamTest 创建独立的原生 Responses 流上下文；测试不请求外部服务。
func newNativeResponsesStreamTest(t *testing.T, events ...string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	var body strings.Builder
	for _, event := range events {
		body.WriteString("data: " + event + "\n\n")
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "deepseek-v4-flash-vision-exp",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-v4-flash-vision-exp",
		},
		IsStream: true, DisablePing: true,
	}
	info.SetEstimatePromptTokens(99)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
	return c, recorder, resp, info
}

// TestNativeResponsesStreamTerminalUsage 验证失败终态保留上游用量并只发送一次，供调用层继续结算。
func TestNativeResponsesStreamTerminalUsage(t *testing.T) {
	for _, test := range []struct {
		name, event, reason string
	}{
		{"failed", `{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"provider unavailable"},"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}}}`, "provider unavailable"},
		{"incomplete", `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}}}`, "max_output_tokens"},
		{"error", `{"type":"error","code":"server_error","message":"provider unavailable","response":{"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}}}`, "provider unavailable"},
		{"cancelled", `{"type":"response.cancelled","response":{"status":"cancelled","usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}}}`, "cancelled"},
		{"done with failed status", `{"type":"response.done","response":{"status":"failed","error":{"code":"server_error","message":"provider unavailable"},"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":3}}}}`, "provider unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder, resp, info := newNativeResponsesStreamTest(t, test.event, `{"type":"response.output_text.delta","delta":"must not follow terminal"}`)
			usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr, "流内错误由 SSE 承载，不得触发退款或额外 JSON")
			require.NotNil(t, usage)
			assert.Equal(t, 12, usage.PromptTokens)
			assert.Equal(t, 7, usage.CompletionTokens)
			assert.Equal(t, 19, usage.TotalTokens)
			assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 3, usage.CompletionTokenDetails.ReasoningTokens)
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "data: "))
			assert.Contains(t, recorder.Body.String(), test.event)
			require.NotNil(t, info.StreamStatus)
			assert.True(t, info.StreamStatus.HasErrors())
			require.NotEmpty(t, info.StreamStatus.Errors)
			assert.Contains(t, info.StreamStatus.Errors[0].Message, test.reason)
		})
	}
}

// TestNativeResponsesStreamCompletedClosesUpstream 验证 completed 即结束，不依赖额外 DONE 或上游关闭连接。
func TestNativeResponsesStreamCompletedClosesUpstream(t *testing.T) {
	event := `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`
	c, recorder, resp, info := newNativeResponsesStreamTest(t)
	body := &blockingBody{chunk: []byte("data: " + event + "\n\n"), closed: make(chan struct{})}
	resp.Body = body
	t.Cleanup(func() { _ = body.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = OaiResponsesStreamHandler(c, info, resp)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = body.Close()
		<-done
		t.Fatal("收到 completed 后仍等待上游关闭")
	}
	assert.Contains(t, recorder.Body.String(), event)
	assert.False(t, info.StreamStatus.HasErrors())
	select {
	case <-body.closed:
	default:
		t.Fatal("完成后未关闭上游响应体")
	}
}

// TestNativeResponsesStreamUnexpectedEOF 验证缺终态的 EOF 显式报错，并保留已知用量。
func TestNativeResponsesStreamUnexpectedEOF(t *testing.T) {
	c, recorder, resp, info := newNativeResponsesStreamTest(t,
		`{"type":"response.in_progress","response":{"usage":{"input_tokens":8,"output_tokens":2,"total_tokens":10}}}`,
	)
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, 10, usage.TotalTokens)
	assert.True(t, info.StreamStatus.HasErrors())
	assert.Contains(t, recorder.Body.String(), "event: error\n")
	assert.NotContains(t, recorder.Body.String(), "event: response.completed")
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error\n"))
}

// TestNativeResponsesStreamPreservesExtensionEvents 验证扩展工具字段不因共享 DTO 的字段类型限制被丢弃。
func TestNativeResponsesStreamPreservesExtensionEvents(t *testing.T) {
	extension := `{"type":"response.custom_tool_call_input.delta","delta":{"arguments":"hello"},"sequence_number":1}`
	item := `{"type":"response.output_item.done","item":{"type":"custom_tool_call","result":{"text":"result"},"content":"extension-content"}}`
	terminal := `{"type":"response.completed","response":{"status":"completed","output":[{"type":"custom_tool_call","result":{"text":"result"},"content":"extension-content"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`
	c, recorder, resp, info := newNativeResponsesStreamTest(t, extension, item, terminal)
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.False(t, info.StreamStatus.HasErrors())
	for _, event := range []string{extension, item, terminal} {
		assert.Contains(t, recorder.Body.String(), "data: "+event+"\n\n")
	}
}

// TestNativeResponsesStreamExplicitZeroUsage 验证上游明确的零用量不会被文本估算覆盖。
func TestNativeResponsesStreamExplicitZeroUsage(t *testing.T) {
	c, _, resp, info := newNativeResponsesStreamTest(t,
		`{"type":"response.output_text.delta","delta":"some visible text"}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
	)
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Zero(t, usage.PromptTokens)
	assert.Zero(t, usage.CompletionTokens)
	assert.Zero(t, usage.TotalTokens)
}

// nativeResponsesFailWriter 在数据写入时模拟下游故障，并记录尝试次数。
type nativeResponsesFailWriter struct {
	gin.ResponseWriter
	// writes 记录故障后是否仍有额外写入。
	writes int
}

// Write 模拟明确的网络写入失败，不接受任何字节。
func (w *nativeResponsesFailWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("downstream write failed")
}

// WriteString 保证字符串写入也经过同一个失败边界。
func (w *nativeResponsesFailWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// TestNativeResponsesStreamWriteFailureStops 验证下游写失败后不继续发送，同时保留已经解析的上游用量。
func TestNativeResponsesStreamWriteFailureStops(t *testing.T) {
	c, _, resp, info := newNativeResponsesStreamTest(t,
		`{"type":"response.in_progress","response":{"usage":{"input_tokens":8,"output_tokens":2,"total_tokens":10}}}`,
		`{"type":"response.output_text.delta","delta":"must not write"}`,
	)
	writer := &nativeResponsesFailWriter{ResponseWriter: c.Writer}
	c.Writer = writer
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 10, usage.TotalTokens)
	assert.Equal(t, 1, writer.writes)
	assert.True(t, info.StreamStatus.HasErrors())
}

// TestNativeResponsesStreamMissingUsageEstimatesText 保留无上游 usage 时的现有文本估算契约。
func TestNativeResponsesStreamMissingUsageEstimatesText(t *testing.T) {
	c, _, resp, info := newNativeResponsesStreamTest(t,
		`{"type":"response.output_text.delta","delta":"some visible text"}`,
		`{"type":"response.completed","response":{"status":"completed"}}`,
	)
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, 99, usage.PromptTokens)
	assert.Positive(t, usage.CompletionTokens)
	assert.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	assert.Equal(t, 0, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
}

// TestNativeResponsesStreamPartialUsageFallback 验证输入和输出分别回退，中间零值或空对象不能冻结输出估算。
func TestNativeResponsesStreamPartialUsageFallback(t *testing.T) {
	for _, test := range []struct {
		name, initial, terminal string
		wantInput               int
	}{
		{"input then completed", `{"input_tokens":8}`, `{"type":"response.completed","response":{"status":"completed"}}`, 8},
		{"input then EOF", `{"input_tokens":8}`, "", 8},
		{"intermediate zero output", `{"input_tokens":8,"output_tokens":0}`, `{"type":"response.completed","response":{"status":"completed"}}`, 8},
		{"empty terminal usage", `{}`, `{"type":"response.completed","response":{"status":"completed","usage":{}}}`, 99},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := []string{
				`{"type":"response.in_progress","response":{"usage":` + test.initial + `}}`,
				`{"type":"response.output_text.delta","delta":"some visible text"}`,
			}
			if test.terminal != "" {
				events = append(events, test.terminal)
			}
			c, _, resp, info := newNativeResponsesStreamTest(t, events...)
			usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			wantOutput := service.CountTextToken("some visible text", info.UpstreamModelName)
			assert.Equal(t, test.wantInput, usage.PromptTokens)
			assert.Equal(t, wantOutput, usage.CompletionTokens)
			assert.Equal(t, test.wantInput+wantOutput, usage.TotalTokens)
		})
	}
}

// TestNativeResponsesStreamEstimatedUsageUpdatesBillingSnapshot 验证回退用量同步至结算实际读取的原始快照。
func TestNativeResponsesStreamEstimatedUsageUpdatesBillingSnapshot(t *testing.T) {
	for _, test := range []struct {
		name, initial, source string
		wantInput, wantCached int
	}{
		{"OpenAI input snapshot", `{"input_tokens":8,"input_tokens_details":{"cached_tokens":2},"billing_usage":{"source":"oai_responses","semantic":"openai","openai_usage":{"input_tokens":8,"input_tokens_details":{"cached_tokens":2}}}}`, dto.BillingUsageSourceOAIResponses, 8, 2},
		{"Claude input snapshot", `{"input_tokens":8,"input_tokens_details":{"cached_tokens":2},"billing_usage":{"source":"claude_messages","semantic":"anthropic","claude_usage":{"input_tokens":6,"cache_read_input_tokens":2}}}`, dto.BillingUsageSourceClaudeMessages, 6, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _, resp, info := newNativeResponsesStreamTest(t,
				`{"type":"response.in_progress","response":{"usage":`+test.initial+`}}`,
				`{"type":"response.output_text.delta","delta":"some visible text"}`,
				`{"type":"response.completed","response":{"status":"completed"}}`,
			)
			usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage.BillingUsage)
			assert.Equal(t, test.source, usage.BillingUsage.Source)
			assert.True(t, usage.BillingUsage.Estimated)
			canonical, ok := usage.BillingUsage.CanonicalUsage()
			require.True(t, ok)
			assert.Equal(t, test.wantInput, canonical.PromptTokens)
			assert.Equal(t, test.wantCached, canonical.PromptTokensDetails.CachedTokens)
			assert.Equal(t, service.CountTextToken("some visible text", info.UpstreamModelName), canonical.CompletionTokens)
			assert.Equal(t, usage.CompletionTokens, canonical.CompletionTokens)
		})
	}
}

// TestNativeResponsesStreamActualBillingUsagePrecedesEstimates 验证真实快照输出优先于文本估算，缺失输入估算也同步结算快照。
func TestNativeResponsesStreamActualBillingUsagePrecedesEstimates(t *testing.T) {
	for _, test := range []struct {
		name, initial string
		wantInput     int
		wantEstimated bool
	}{
		{"actual output", `{"input_tokens":8,"billing_usage":{"source":"oai_responses","semantic":"openai","openai_usage":{"input_tokens":8,"output_tokens":7,"total_tokens":15}}}`, 8, false},
		{"missing input", `{"output_tokens":7,"billing_usage":{"source":"oai_responses","semantic":"openai","openai_usage":{"output_tokens":7}}}`, 99, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _, resp, info := newNativeResponsesStreamTest(t,
				`{"type":"response.in_progress","response":{"usage":`+test.initial+`}}`,
				`{"type":"response.output_text.delta","delta":"some visible text"}`,
				`{"type":"response.completed","response":{"status":"completed"}}`,
			)
			usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			assert.Equal(t, test.wantInput, usage.PromptTokens)
			assert.Equal(t, 7, usage.CompletionTokens)
			canonical, ok := usage.BillingUsage.CanonicalUsage()
			require.True(t, ok)
			assert.Equal(t, test.wantInput, canonical.PromptTokens)
			assert.Equal(t, 7, canonical.CompletionTokens)
			assert.Equal(t, test.wantInput+7, canonical.TotalTokens)
			assert.Equal(t, test.wantEstimated, usage.BillingUsage.Estimated)
		})
	}
}

// TestNativeResponsesStreamTerminalZeroOverridesPriorUsage 验证终态零按字段覆盖旧计数，旧快照不能重新参与计费。
func TestNativeResponsesStreamTerminalZeroOverridesPriorUsage(t *testing.T) {
	for _, test := range []struct {
		name, terminalUsage   string
		wantInput, wantOutput int
	}{
		{"all zero", `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, 0, 0},
		{"input zero", `{"input_tokens":0}`, 0, 3},
		{"output zero", `{"output_tokens":0}`, 8, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _, resp, info := newNativeResponsesStreamTest(t,
				`{"type":"response.in_progress","response":{"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1},"billing_usage":{"source":"oai_responses","semantic":"openai","openai_usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11,"input_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}}}}}}`,
				`{"type":"response.output_text.delta","delta":"some visible text"}`,
				`{"type":"response.completed","response":{"status":"completed","usage":`+test.terminalUsage+`}}`,
			)
			usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			assert.Equal(t, test.wantInput, usage.PromptTokens)
			assert.Equal(t, test.wantOutput, usage.CompletionTokens)
			assert.Equal(t, test.wantInput+test.wantOutput, usage.TotalTokens)
			if test.wantInput == 0 {
				assert.Zero(t, usage.PromptTokensDetails.CachedTokens)
			}
			if test.wantOutput == 0 {
				assert.Zero(t, usage.CompletionTokenDetails.ReasoningTokens)
			}
			canonical, ok := usage.BillingUsage.CanonicalUsage()
			if test.wantInput+test.wantOutput == 0 {
				assert.False(t, ok, "全零终态不能被历史快照覆盖")
			} else {
				require.True(t, ok)
				assert.Equal(t, test.wantInput, canonical.PromptTokens)
				assert.Equal(t, test.wantOutput, canonical.CompletionTokens)
				assert.Equal(t, test.wantInput+test.wantOutput, canonical.TotalTokens)
			}
		})
	}
}
