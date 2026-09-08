package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newResponsesStreamContractFixture 建立只访问本机的 HTTP 上游与真实 Responses 处理上下文；body 是确定的响应字节。
// isStream 决定响应类型，测试结束时关闭上游并恢复全局流超时及 Gin 模式，不使用外部服务或凭据。
func newResponsesStreamContractFixture(t *testing.T, body string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/responses", r.URL.Path)
		if isStream {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(upstream.Close)
	response, err := upstream.Client().Post(upstream.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"deepseek-v4-flash-vision-exp","input":"fixture"}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	require.Equal(t, http.StatusOK, response.StatusCode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-stream-contract")
	info := &relaycommon.RelayInfo{
		OriginModelName: "deepseek-v4-flash-vision-exp",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-v4-flash-vision-exp",
		},
		RelayFormat: types.RelayFormatOpenAIResponses,
		IsStream:    isStream,
		DisablePing: true,
	}
	info.SetEstimatePromptTokens(99)
	return c, recorder, response, info
}

// TestResponsesStreamContractTerminalFailure 验证失败和未完成终态原样到达客户端，其真实用量及异常状态进入结算和消费日志。
func TestResponsesStreamContractTerminalFailure(t *testing.T) {
	for _, test := range []struct {
		name  string
		event string
	}{
		{
			name:  "failed",
			event: `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"server_error","message":"upstream failed"},"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4}}}}`,
		},
		{
			name:  "incomplete",
			event: `{"type":"response.incomplete","response":{"id":"resp_incomplete","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4}}}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, recorder, response, info := newResponsesStreamContractFixture(t, "data: "+test.event+"\n\n", true)
			usage, apiErr := OaiResponsesStreamHandler(c, info, response)
			require.Nil(t, apiErr, "已发送的终态通过 SSE 传达，不能再次触发普通 JSON 错误或请求重试")
			require.NotNil(t, usage)
			assert.Contains(t, recorder.Body.String(), "data: "+test.event+"\n\n")
			assert.NotContains(t, recorder.Body.String(), "event: response.completed")
			assert.Equal(t, 12, usage.PromptTokens, "终态明确报告的输入用量不能归零")
			assert.Equal(t, 7, usage.CompletionTokens, "终态明确报告的输出用量不能归零")
			assert.Equal(t, 19, usage.TotalTokens)
			assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)

			other := service.GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
			streamLog, ok := other.Snapshot()["stream_status"].(map[string]interface{})
			require.True(t, ok, "消费日志必须包含流状态")
			assert.Equal(t, "error", streamLog["status"], "失败或未完成终态不能在消费日志中显示为成功")
		})
	}
}

// TestResponsesStreamContractReasoningUsage 验证标准 output_tokens_details.reasoning_tokens 进入用量明细，且不重复增加总输出 token。
func TestResponsesStreamContractReasoningUsage(t *testing.T) {
	const completed = `{"id":"resp_reasoning","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":7,"total_tokens":19,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":5}}}`
	for _, isStream := range []bool{false, true} {
		name := "nonstream"
		body := completed
		if isStream {
			name = "stream"
			body = "data: {\"type\":\"response.completed\",\"response\":" + completed + "}\n\n"
		}
		t.Run(name, func(t *testing.T) {
			c, recorder, response, info := newResponsesStreamContractFixture(t, body, isStream)
			var usage *dto.Usage
			var apiErr *types.NewAPIError
			if isStream {
				usage, apiErr = OaiResponsesStreamHandler(c, info, response)
			} else {
				usage, apiErr = OaiResponsesHandler(c, info, response)
			}
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Contains(t, recorder.Body.String(), `"output_tokens_details":{"reasoning_tokens":5}`)
			assert.Equal(t, 5, usage.CompletionTokenDetails.ReasoningTokens, "客户端可见的推理 token 不能在网关用量明细中丢失")
			assert.Equal(t, 12, usage.PromptTokens)
			assert.Equal(t, 7, usage.CompletionTokens, "output_tokens 已包含推理 token")
			assert.Equal(t, 19, usage.TotalTokens)
		})
	}
}

// TestResponsesStreamContractOpaqueExtension 验证未知供应商事件原样透传，不把扩展字段的类型冲突误记为正常文本流错误。
// 此处的对象 delta 是供应商私有事件字段，并不声称 OpenAI 标准文本或工具参数 delta 支持对象。
func TestResponsesStreamContractOpaqueExtension(t *testing.T) {
	const extension = `{"type":"response.provider_metadata","delta":{"trace":"opaque-test-value"},"sequence_number":1}`
	const textDelta = `{"type":"response.output_text.delta","delta":"OK","sequence_number":2}`
	const completed = `{"type":"response.completed","response":{"id":"resp_extension","status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}},"sequence_number":3}`
	body := "data: " + extension + "\n\ndata: " + textDelta + "\n\ndata: " + completed + "\n\n"
	c, recorder, response, info := newResponsesStreamContractFixture(t, body, true)
	usage, apiErr := OaiResponsesStreamHandler(c, info, response)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Contains(t, recorder.Body.String(), "data: "+extension+"\n\n", "未知事件不需要转换，但应保留原始数据")
	assert.Contains(t, recorder.Body.String(), "data: "+textDelta+"\n\n")
	assert.Contains(t, recorder.Body.String(), "data: "+completed+"\n\n")
	assert.Equal(t, 3, usage.TotalTokens)
	require.NotNil(t, info.StreamStatus)
	assert.False(t, info.StreamStatus.HasErrors(), "供应商私有字段不应污染已完成标准响应的流状态")
}

// TestResponsesStreamContractMissingTerminal 验证普通 EOF 不能代替 Responses 终态；客户端与消费日志都应看见截断。
func TestResponsesStreamContractMissingTerminal(t *testing.T) {
	const textDelta = `{"type":"response.output_text.delta","delta":"partial answer"}`
	c, recorder, response, info := newResponsesStreamContractFixture(t, "data: "+textDelta+"\n\n", true)
	_, apiErr := OaiResponsesStreamHandler(c, info, response)
	require.Nil(t, apiErr, "开流后不返回会触发普通 JSON 错误或重试的错误")
	assert.Contains(t, recorder.Body.String(), "data: "+textDelta+"\n\n")
	assert.Contains(t, recorder.Body.String(), "event: error\n", "缺少终态的截断必须向客户端显式报告")
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error\n"), "截断只补发一个流内错误")
	assert.NotContains(t, recorder.Body.String(), "event: response.completed")
	other := service.GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
	streamLog, ok := other.Snapshot()["stream_status"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "error", streamLog["status"], "截断的回复不能显示为正常结束")
}

// TestResponsesStreamContractExplicitZeroUsage 验证终态明确的零值覆盖历史用量，并清除对应侧明细而不被文本估算替换。
func TestResponsesStreamContractExplicitZeroUsage(t *testing.T) {
	for _, test := range []struct {
		name          string
		input, output int
		cache, think  int
	}{
		{"全部为零", 0, 0, 0, 0},
		{"仅输入为零", 0, 7, 0, 5},
		{"仅输出为零", 12, 0, 4, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			const progress = `{"type":"response.in_progress","response":{"usage":{"input_tokens":12,"output_tokens":7,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":5}}}}`
			terminal := fmt.Sprintf(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}}`, test.input, test.output, test.input+test.output)
			body := "data: " + progress + "\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"visible text\"}\n\ndata: " + terminal + "\n\n"
			c, _, response, info := newResponsesStreamContractFixture(t, body, true)
			usage, apiErr := OaiResponsesStreamHandler(c, info, response)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, test.input, usage.PromptTokens)
			assert.Equal(t, test.output, usage.CompletionTokens)
			assert.Equal(t, test.input+test.output, usage.TotalTokens)
			assert.Equal(t, test.cache, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, test.think, usage.CompletionTokenDetails.ReasoningTokens)
		})
	}
}

// TestResponsesStreamContractPartialUsage 验证空用量不冻结估算，且缺失的输出计数不覆盖已报告的输入计数。
func TestResponsesStreamContractPartialUsage(t *testing.T) {
	for _, test := range []struct {
		name, usage string
		wantInput   int
	}{
		{"空用量", `{}`, 99},
		{"仅输入用量", `{"input_tokens":8}`, 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":" + test.usage + "}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"visible text\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{}}}\n\n"
			c, _, response, info := newResponsesStreamContractFixture(t, body, true)
			usage, apiErr := OaiResponsesStreamHandler(c, info, response)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, test.wantInput, usage.PromptTokens)
			assert.Positive(t, usage.CompletionTokens)
		})
	}
}

// responsesContractFailWriter 模拟下游写错误或短写，writes 用于证明故障后不会继续补发事件。
type responsesContractFailWriter struct {
	gin.ResponseWriter
	writes int
	short  bool
}

// Write 按用例返回明确的传输错误或没有错误值的短写。
func (w *responsesContractFailWriter) Write([]byte) (int, error) {
	w.writes++
	if w.short {
		return 1, nil
	}
	return 0, errors.New("downstream write failed")
}

// WriteString 让字符串写入经过同一故障边界。
func (w *responsesContractFailWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// TestResponsesStreamContractWriteFailure 验证下游错误和短写立即停止，只保留已有用量并记录流异常。
func TestResponsesStreamContractWriteFailure(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprintf("short=%t", short), func(t *testing.T) {
			const body = "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":7}}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"must not write\"}\n\n"
			c, _, response, info := newResponsesStreamContractFixture(t, body, true)
			writer := &responsesContractFailWriter{ResponseWriter: c.Writer, short: short}
			c.Writer = writer
			usage, apiErr := OaiResponsesStreamHandler(c, info, response)
			require.Nil(t, apiErr, "开流后不能返回触发重试或普通 JSON 错误的错误值")
			require.NotNil(t, usage)
			assert.Equal(t, 12, usage.PromptTokens)
			assert.Equal(t, 7, usage.CompletionTokens)
			assert.Equal(t, 1, writer.writes, "传输失败后不得继续写数据或补发错误")
			require.NotNil(t, info.StreamStatus)
			assert.True(t, info.StreamStatus.HasErrors())
		})
	}
}

// TestResponsesStreamContractCompletedClosesUpstream 验证 completed 即结束并关闭上游，无需额外 DONE 或远端 EOF。
func TestResponsesStreamContractCompletedClosesUpstream(t *testing.T) {
	const terminal = `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+terminal+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, upstream.URL+"/v1/responses", nil)
	require.NoError(t, err)
	response, err := upstream.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	c, recorder, unused, info := newResponsesStreamContractFixture(t, "", true)
	require.NoError(t, unused.Body.Close())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = OaiResponsesStreamHandler(c, info, response)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("收到 completed 后仍在等待上游关闭连接")
	}
	assert.Contains(t, recorder.Body.String(), "data: "+terminal+"\n\n")
	require.NotNil(t, info.StreamStatus)
	assert.False(t, info.StreamStatus.HasErrors())
}

// TestResponsesStreamContractUsageMetadata 保护 rc35 用量来源、成本与多模态缓存明细，同时保留原生用量不创建转换快照的契约。
func TestResponsesStreamContractUsageMetadata(t *testing.T) {
	const event = `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":20,"output_tokens":7,"usage_semantic":"openai","usage_source":"upstream","cost":0.25,"input_tokens_details":{"cached_tokens":4,"cache_write_tokens":2,"cached_creation_tokens":3,"text_tokens":10,"image_tokens":6,"audio_tokens":4},"output_tokens_details":{"reasoning_tokens":5}}}}`
	c, recorder, response, info := newResponsesStreamContractFixture(t, "data: "+event+"\n\n", true)
	usage, apiErr := OaiResponsesStreamHandler(c, info, response)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Contains(t, recorder.Body.String(), event)
	assert.Equal(t, "openai", usage.UsageSemantic)
	assert.Equal(t, "upstream", usage.UsageSource)
	assert.Equal(t, 0.25, usage.Cost)
	assert.Nil(t, usage.BillingUsage)
	assert.Equal(t, 20, usage.InputTokens)
	assert.Equal(t, 7, usage.OutputTokens)
	assert.Equal(t, dto.InputTokenDetails{CachedTokens: 4, CacheWriteTokens: 2, CachedCreationTokens: 3, TextTokens: 10, ImageTokens: 6, AudioTokens: 4}, usage.PromptTokensDetails)
	assert.Equal(t, 5, usage.CompletionTokenDetails.ReasoningTokens)
	assert.Equal(t, 27, usage.TotalTokens)
}
