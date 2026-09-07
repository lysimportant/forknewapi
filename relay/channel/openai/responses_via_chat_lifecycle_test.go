package openai

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChatResponsesBridgeConsumesUsageAfterFinish 验证结束原因之后的 usage-only 尾包仍进入最终 Responses 与结算。
func TestChatResponsesBridgeConsumesUsageAfterFinish(t *testing.T) {
	for _, test := range []struct{ name, delta, finish, terminal string }{
		{"text stop", `{"content":"hello"}`, "stop", "response.completed"},
		{"function call", `{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]}`, "tool_calls", "response.completed"},
		{"output limit", `{"content":"hello"}`, "length", "response.incomplete"},
		{"content filter", `{"content":"hello"}`, "content_filter", "response.incomplete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder, resp, info := newNativeResponsesStreamTest(t,
				`{"choices":[{"index":0,"delta":`+test.delta+`,"finish_reason":null}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"`+test.finish+`"}]}`,
				`{"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11,"prompt_tokens_details":{"cached_tokens":2}}}`,
				`[DONE]`,
			)
			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 8, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			assert.Equal(t, 11, usage.TotalTokens)
			canonical, ok := usage.BillingUsage.CanonicalUsage()
			require.True(t, ok)
			assert.Equal(t, 2, canonical.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: "+test.terminal+"\n"))
			assert.Contains(t, recorder.Body.String(), `"input_tokens":8`)
			assert.Contains(t, recorder.Body.String(), `"output_tokens":3`)
			assert.NotContains(t, recorder.Body.String(), "event: response.failed\n")
			assert.Equal(t, test.terminal == "response.incomplete", info.StreamStatus.HasErrors())
			if test.finish == "tool_calls" {
				assert.Contains(t, recorder.Body.String(), "event: response.function_call_arguments.done\n")
			}
		})
	}
}

// TestChatResponsesBridgePreservesSparseUsage 验证缺少 total_tokens 的实际计数在失败结算时仍保留。
func TestChatResponsesBridgePreservesSparseUsage(t *testing.T) {
	for _, test := range []struct {
		name, usage         string
		prompt, output, sum int
	}{
		{"prompt only", `{"prompt_tokens":8}`, 8, 0, 8},
		{"output only", `{"completion_tokens":3}`, 0, 3, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder, resp, info := newNativeResponsesStreamTest(t,
				`{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}],"usage":`+test.usage+`}`,
			)
			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, test.prompt, usage.PromptTokens)
			assert.Equal(t, test.output, usage.CompletionTokens)
			assert.Equal(t, test.sum, usage.TotalTokens)
			canonical, ok := usage.BillingUsage.CanonicalUsage()
			require.True(t, ok)
			assert.Equal(t, test.prompt, canonical.PromptTokens)
			assert.Equal(t, test.output, canonical.CompletionTokens)
			assert.Contains(t, recorder.Body.String(), "event: response.failed\n")
		})
	}
}

// TestChatResponsesBridgeConversionFailureKeepsLatestUsage 验证工具调用标识冲突时保留当前包的新用量。
func TestChatResponsesBridgeConversionFailureKeepsLatestUsage(t *testing.T) {
	c, recorder, resp, info := newNativeResponsesStreamTest(t,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup"}}]},"finish_reason":null}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"lookup"}}]},"finish_reason":null}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`,
	)
	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.TotalTokens)
	assert.Contains(t, recorder.Body.String(), `"output_tokens":3`)
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error\n"))
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
	assert.NotContains(t, recorder.Body.String(), "event: response.completed\n")
	require.True(t, info.StreamStatus.HasErrors())
	assert.Contains(t, info.StreamStatus.Errors[0].Message, "changed id")
}

// TestChatResponsesBridgeRejectsFailedStreams 验证上游错误、解析失败及缺失 finish_reason 不能被改写为成功终态。
func TestChatResponsesBridgeRejectsFailedStreams(t *testing.T) {
	for _, test := range []struct{ name, ending, reason string }{
		{"provider error", `{"error":{"type":"server_error","code":"overloaded","message":"provider overloaded"}}`, "provider overloaded"},
		{"provider error without type", `{"error":{"code":"overloaded","message":"provider overloaded"}}`, "provider overloaded"},
		{"malformed chunk", `{"choices":`, "unmarshal"},
		{"EOF without finish", "", "finish_reason"},
		{"DONE without finish", `[DONE]`, "finish_reason"},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := []string{`{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`}
			if test.ending != "" {
				events = append(events, test.ending)
			}
			c, recorder, resp, info := newNativeResponsesStreamTest(t, events...)
			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr, "流内失败保留用量，不触发再次请求或全额退款")
			require.NotNil(t, usage)
			assert.Equal(t, 11, usage.TotalTokens)
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error\n"))
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
			assert.NotContains(t, recorder.Body.String(), "event: response.completed\n")
			assert.Contains(t, recorder.Body.String(), `"input_tokens":8`)
			assert.Contains(t, recorder.Body.String(), `"output_tokens":3`)
			require.True(t, info.StreamStatus.HasErrors())
			assert.Contains(t, info.StreamStatus.Errors[0].Message, test.reason)
		})
	}
}

// responsesBridgeReadFailure 在已有流数据之后模拟网络读取失败。
type responsesBridgeReadFailure struct{}

// Read 返回固定的传输错误，不产生额外的上游数据。
func (responsesBridgeReadFailure) Read([]byte) (int, error) {
	return 0, errors.New("upstream read failed")
}

// TestChatResponsesBridgeNetworkFailures 验证读取失败和超时不能借助 Finalize 伪造 completed。
func TestChatResponsesBridgeNetworkFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		timeout bool
	}{
		{name: "read failure"},
		{name: "timeout", timeout: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := `{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`
			c, recorder, resp, info := newNativeResponsesStreamTest(t, event)
			if test.timeout {
				constant.StreamingTimeout = 1
				body := &blockingBody{chunk: []byte("data: " + event + "\n\n"), closed: make(chan struct{})}
				resp.Body = body
				t.Cleanup(func() { _ = body.Close() })
			} else {
				resp.Body = io.NopCloser(io.MultiReader(resp.Body, responsesBridgeReadFailure{}))
			}
			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			assert.Equal(t, 11, usage.TotalTokens)
			assert.NotContains(t, recorder.Body.String(), "event: response.completed\n")
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
			assert.True(t, info.StreamStatus.HasErrors())
		})
	}
}

// responsesBridgeWriteFailure 先发送一个 SSE 事件，再模拟下游写入错误。
type responsesBridgeWriteFailure struct {
	gin.ResponseWriter
	// writes 记录是否在首次故障后仍尝试追加事件。
	writes int
	// failEvent 非空时仅在写入指定事件及其后发生故障。
	failEvent string
	// failures 记录故障后的写入次数，验证不会再次尝试补写。
	failures int
}

// Write 在指定事件或第二次写入后返回网络错误。
func (w *responsesBridgeWriteFailure) Write(data []byte) (int, error) {
	w.writes++
	if w.failures > 0 || (w.failEvent == "" && w.writes > 1) || (w.failEvent != "" && strings.HasPrefix(string(data), "event: "+w.failEvent+"\n")) {
		w.failures++
		return 0, errors.New("downstream write failed")
	}
	return w.ResponseWriter.Write(data)
}

// WriteString 保证字符串写入经过相同故障边界。
func (w *responsesBridgeWriteFailure) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// TestChatResponsesBridgeWriteFailureKeepsUsage 验证已发送流的写失败不升级为会触发重试退款的外层错误。
func TestChatResponsesBridgeWriteFailureKeepsUsage(t *testing.T) {
	for _, test := range []struct{ name, failEvent, finish string }{
		{"after created", "", "null"},
		{"completed event", "response.completed", `"stop"`},
		{"failed event", "response.failed", "null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder, resp, info := newNativeResponsesStreamTest(t,
				`{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":`+test.finish+`}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`,
			)
			writer := &responsesBridgeWriteFailure{ResponseWriter: c.Writer, failEvent: test.failEvent}
			c.Writer = writer
			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 11, usage.TotalTokens)
			assert.Equal(t, 1, writer.failures)
			assert.True(t, info.StreamStatus.HasErrors())
			assert.NotContains(t, recorder.Body.String(), "event: response.completed\n")
		})
	}
}

// responsesBridgeCancelWriter 在响应内容已送达后模拟客户端取消请求。
type responsesBridgeCancelWriter struct {
	gin.ResponseWriter
	// cancel 触发请求上下文取消，以便 scanner 关闭阻塞中的上游读取。
	cancel context.CancelFunc
}

// Write 转发事件后在首个正文增量到达时取消请求。
func (w *responsesBridgeCancelWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if strings.HasPrefix(string(data), "event: response.output_text.delta\n") {
		w.cancel()
	}
	return n, err
}

// WriteString 使字符串写入使用同一取消边界。
func (w *responsesBridgeCancelWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// TestChatResponsesBridgeClientCancellation 验证取消请求关闭上游并保留已观察用量，不追加终态。
func TestChatResponsesBridgeClientCancellation(t *testing.T) {
	event := `{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`
	c, recorder, resp, info := newNativeResponsesStreamTest(t)
	ctx, cancel := context.WithCancel(c.Request.Context())
	t.Cleanup(cancel)
	c.Request = c.Request.WithContext(ctx)
	c.Writer = &responsesBridgeCancelWriter{ResponseWriter: c.Writer, cancel: cancel}
	body := &blockingBody{chunk: []byte("data: " + event + "\n\n"), closed: make(chan struct{})}
	resp.Body = body
	t.Cleanup(func() { _ = body.Close() })
	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.TotalTokens)
	assert.Contains(t, recorder.Body.String(), "event: response.output_text.delta\n")
	assert.NotContains(t, recorder.Body.String(), "event: response.completed\n")
	assert.NotContains(t, recorder.Body.String(), "event: response.failed\n")
	assert.NotContains(t, recorder.Body.String(), "event: error\n")
	select {
	case <-body.closed:
	default:
		require.FailNow(t, "客户端取消后上游响应体未关闭")
	}
}
