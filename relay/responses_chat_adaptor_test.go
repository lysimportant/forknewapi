package relay

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestResponsesChatAdaptorRejectsUnsupportedInputs 验证无法保真的状态、工具及输入在公共入口返回不可重试的 400。
func TestResponsesChatAdaptorRejectsUnsupportedInputs(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
		want string
	}{
		{"状态引用", `{"model":"model-test","input":"hello","previous_response_id":"resp_1"}`, "previous_response_id"},
		{"会话状态", `{"model":"model-test","input":"hello","conversation":"conv_1"}`, "conversation"},
		{"远程提示模板", `{"model":"model-test","input":"hello","prompt":{"id":"pmpt_1"}}`, "prompt"},
		{"上下文管理", `{"model":"model-test","input":"hello","context_management":{"type":"auto"}}`, "context_management"},
		{"内置工具", `{"model":"model-test","input":"hello","tools":[{"type":"web_search_preview"}]}`, "tool type"},
		{"自定义工具", `{"model":"model-test","input":"hello","tools":[{"type":"custom","name":"apply_patch"}]}`, "tool type"},
		{"自定义调用历史", `{"model":"model-test","input":[{"type":"custom_tool_call","call_id":"call_1","name":"apply_patch","input":"patch"}]}`, "history tool type"},
		{"未知输入项目", `{"model":"model-test","input":[{"type":"future_item","content":"value"}]}`, "future_item"},
		{"推理密文", `{"model":"model-test","input":[{"type":"reasoning","encrypted_content":"test-encrypted-content"}]}`, "encrypted_content"},
		{"不可表达自定义结果", `{"model":"model-test","input":[{"type":"custom_tool_call_output","call_id":"call_1","output":"result"}]}`, "custom_tool_call_output"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder, info, _ := responsesChannelContext(t, constant.ChannelTypeSiliconFlow, "model-test", `"hello"`, false)
			var request dto.OpenAIResponsesRequest
			require.NoError(t, common.Unmarshal([]byte(test.json), &request))
			info.Request = &request
			originalHTTP := ctx.Request
			adaptor, apiErr := newResponsesAdaptor(info)
			require.Nil(t, apiErr)
			adaptor.Init(info)
			converted, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, request)
			require.Error(t, err)
			var conversionErr *types.NewAPIError
			require.ErrorAs(t, err, &conversionErr)
			assert.Equal(t, http.StatusBadRequest, conversionErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(conversionErr))
			assert.Contains(t, err.Error(), test.want)
			assert.Nil(t, converted)
			assert.Empty(t, recorder.Body.String())
			assert.Same(t, originalHTTP, ctx.Request)
			assert.Same(t, &request, info.Request)
			assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
		})
	}
}

// TestResponsesChatAdaptorRejectsPassThrough 验证全局与渠道透传冲突在选择适配器阶段即返回 400。
func TestResponsesChatAdaptorRejectsPassThrough(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalGlobal := settings.PassThroughRequestEnabled
	t.Cleanup(func() { settings.PassThroughRequestEnabled = originalGlobal })
	for _, global := range []bool{false, true} {
		settings.PassThroughRequestEnabled = global
		_, _, info, _ := responsesChannelContext(t, constant.ChannelTypeSiliconFlow, "model-test", `"hello"`, false)
		info.ChannelSetting.PassThroughBodyEnabled = !global
		adaptor, apiErr := newResponsesAdaptor(info)
		require.NotNil(t, apiErr)
		assert.Nil(t, adaptor)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.Contains(t, apiErr.Error(), "pass-through")
	}
}

// TestResponsesChatAdaptorPreservesOptionalScalars 保护标准 Chat 出站显式零、false 和缺省字段，以及流选项按渠道能力生成。
func TestResponsesChatAdaptorPreservesOptionalScalars(t *testing.T) {
	for _, test := range []struct {
		name    string
		present bool
		stream  bool
	}{
		{"缺省", false, false},
		{"显式零与false", true, false},
		{"流式保留零", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _, info, request := responsesChannelContext(t, constant.ChannelTypeSiliconFlow, "model-test", `"hello"`, test.stream)
			request.Stream, request.MaxOutputTokens = nil, nil
			if test.present {
				request.Stream = common.GetPointer(test.stream)
				request.MaxOutputTokens = common.GetPointer(uint(0))
				request.Temperature, request.TopP = common.GetPointer(0.0), common.GetPointer(0.0)
				request.Store, request.ParallelToolCalls = []byte("false"), []byte("false")
			}
			info.SupportStreamOptions = true
			originalHTTP := ctx.Request
			adaptor, apiErr := newResponsesAdaptor(info)
			require.Nil(t, apiErr)
			adaptor.Init(info)
			converted, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, *request)
			require.NoError(t, err)
			raw, err := common.Marshal(converted)
			require.NoError(t, err)
			for _, field := range []string{"stream", "max_tokens", "temperature", "top_p", "store", "parallel_tool_calls"} {
				assert.Equal(t, test.present, gjson.GetBytes(raw, field).Exists(), field)
			}
			if test.present {
				assert.Equal(t, test.stream, gjson.GetBytes(raw, "stream").Bool())
				assert.Zero(t, gjson.GetBytes(raw, "max_tokens").Uint())
				assert.Zero(t, gjson.GetBytes(raw, "temperature").Float())
				assert.Zero(t, gjson.GetBytes(raw, "top_p").Float())
				assert.False(t, gjson.GetBytes(raw, "store").Bool())
				assert.False(t, gjson.GetBytes(raw, "parallel_tool_calls").Bool())
			}
			assert.False(t, gjson.GetBytes(raw, "max_completion_tokens").Exists())
			assert.Equal(t, test.stream, gjson.GetBytes(raw, "stream_options.include_usage").Exists())
			assert.Same(t, originalHTTP, ctx.Request)
			assert.Same(t, request, info.Request)
			assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
			assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
			assert.Equal(t, "/v1/responses", info.RequestURLPath)
		})
	}
}

// TestResponsesChatAdaptorStandardChatURLs 验证标准 Chat 渠道根地址、可选 v1 和代理前缀，以及读取 URL 后渠道上下文完整恢复。
func TestResponsesChatAdaptorStandardChatURLs(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeSiliconFlow, constant.ChannelTypeMistral, constant.ChannelTypeSubmodel} {
		for _, test := range []struct{ base, want string }{
			{"https://upstream.test", "https://upstream.test/v1/chat/completions"},
			{"https://upstream.test/v1/", "https://upstream.test/v1/chat/completions"},
			{"https://upstream.test/proxy/v1/", "https://upstream.test/proxy/v1/chat/completions"},
		} {
			_, _, info, request := responsesChannelContext(t, channelType, "model-test", `"hello"`, false)
			info.ChannelBaseUrl = test.base
			adaptor, apiErr := newResponsesAdaptor(info)
			require.Nil(t, apiErr)
			adaptor.Init(info)
			got, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.base, info.ChannelBaseUrl)
			assert.Equal(t, "/v1/responses", info.RequestURLPath)
			assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
			assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
			assert.Same(t, request, info.Request)
		}
	}
}

// responsesFailingContextAdaptor 观察供应商转换期间的上下文，并模拟明确转换失败以验证恢复路径。
type responsesFailingContextAdaptor struct {
	channel.Adaptor
	// observed 记录供应商是否收到完整 Chat 上下文，避免仅检查返回后的最终状态。
	observed bool
}

// ConvertOpenAIRequest 验证临时 Chat 语义，保留渠道元信息修改并返回测试错误；不发起上游请求。
func (a *responsesFailingContextAdaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	a.observed = info.RelayFormat == types.RelayFormatOpenAI && info.RelayMode == relayconstant.RelayModeChatCompletions &&
		info.RequestURLPath == "/v1/chat/completions" && c.Request.URL.Path == "/v1/chat/completions" && info.Request == request
	info.UpstreamModelName = "provider-resolved-model"
	return nil, errors.New("test provider rejected conversion")
}

// TestResponsesChatAdaptorRestoresContextAfterProviderError 保护异常路径恢复客户端请求，同时保留供应商解析出的渠道状态。
func TestResponsesChatAdaptorRestoresContextAfterProviderError(t *testing.T) {
	ctx, _, info, request := responsesChannelContext(t, constant.ChannelTypeSiliconFlow, "model-test", `"hello"`, false)
	originalHTTP := ctx.Request
	probe := &responsesFailingContextAdaptor{}
	adaptor := &responsesChatAdaptor{Adaptor: probe}
	_, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, *request)
	require.Error(t, err)
	assert.True(t, probe.observed)
	assert.Same(t, originalHTTP, ctx.Request)
	assert.Same(t, request, info.Request)
	assert.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.RelayFormat)
	assert.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
	assert.Equal(t, "/v1/responses", info.RequestURLPath)
	assert.Equal(t, "provider-resolved-model", info.UpstreamModelName)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
}

// TestResponsesChatAdaptorReasoningToolRoundTrip 验证实际公共响应编码器产出的完整 output 可携带工具结果回传，推理和调用标识不丢失。
func TestResponsesChatAdaptorReasoningToolRoundTrip(t *testing.T) {
	ctx, recorder, info, request := responsesChannelContext(t, constant.ChannelTypeSiliconFlow, "model-test", `"hello"`, false)
	adaptor, apiErr := newResponsesAdaptor(info)
	require.Nil(t, apiErr)
	adaptor.Init(info)
	_, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, *request)
	require.NoError(t, err)
	chatBody := `{"id":"chatcmpl_test","object":"chat.completion","model":"model-test","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"Need to read the file.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"README.md\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(chatBody))}
	_, apiErr = adaptor.DoResponse(ctx, response, info)
	require.Nil(t, apiErr)
	output := gjson.Get(recorder.Body.String(), "output")
	require.True(t, output.IsArray())
	assert.Equal(t, "Need to read the file.", gjson.Get(output.Raw, `#(type=="reasoning").summary.0.text`).String())
	var history []map[string]any
	require.NoError(t, common.Unmarshal([]byte(output.Raw), &history))
	history = append(history, map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "Project overview"})
	nextInput, err := common.Marshal(history)
	require.NoError(t, err)
	ctx, _, info, request = responsesChannelContext(t, constant.ChannelTypeSiliconFlow, "model-test", string(nextInput), false)
	adaptor, apiErr = newResponsesAdaptor(info)
	require.Nil(t, apiErr)
	adaptor.Init(info)
	converted, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, *request)
	require.NoError(t, err)
	chat, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, chat.Messages, 2)
	assert.Equal(t, "Need to read the file.", chat.Messages[0].GetReasoningContent())
	require.Len(t, chat.Messages[0].ParseToolCalls(), 1)
	assert.Equal(t, "call_1", chat.Messages[0].ParseToolCalls()[0].ID)
	assert.Equal(t, "call_1", chat.Messages[1].ToolCallId)
	assert.Equal(t, "Project overview", chat.Messages[1].StringContent())
}
