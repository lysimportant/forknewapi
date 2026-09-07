package relay

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// newResponsesAdaptor 为所有文本渠道选择 Responses 实现；已有原生或专用实现优先，其余复用渠道的 Chat 适配。
// 非文本渠道与原始请求体透传冲突在调用上游前返回不可重试的 400，不进行失败后的协议探测或重发。
func newResponsesAdaptor(info *relaycommon.RelayInfo) (channel.Adaptor, *types.NewAPIError) {
	switch info.ChannelType {
	case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus, constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora:
		return nil, responsesChatRequestError(fmt.Errorf("channel type %d does not support text Responses", info.ChannelType))
	}
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, responsesChatRequestError(fmt.Errorf("api type %d has no text adaptor", info.ApiType))
	}
	switch info.ApiType {
	case constant.APITypeJina, constant.APITypeMokaAI, constant.APITypeJimeng, constant.APITypeReplicate:
		return nil, responsesChatRequestError(fmt.Errorf("channel %s does not support text Responses", adaptor.GetChannelName()))
	case constant.APITypeOpenAI, constant.APITypeAli, constant.APITypeGemini, constant.APITypePerplexity,
		constant.APITypeCloudflare, constant.APITypeDeepSeek, constant.APITypeVolcEngine,
		constant.APITypeOpenRouter, constant.APITypeXinference, constant.APITypeXai,
		constant.APITypeCodex, constant.APITypeAdvancedCustom:
		return adaptor, nil
	default:
		if info.RelayMode != relayconstant.RelayModeResponses {
			return nil, responsesChatRequestError(fmt.Errorf("channel %s does not support Responses compaction", adaptor.GetChannelName()))
		}
		if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
			return nil, responsesChatRequestError(fmt.Errorf("Responses requires Chat conversion for channel %s; disable request body pass-through", adaptor.GetChannelName()))
		}
		return &responsesChatAdaptor{Adaptor: adaptor}, nil
	}
}

// responsesChatRequestError 保留转换错误上下文，并阻止不可能成功的客户端请求重试其他渠道。
func responsesChatRequestError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// responsesChatAdaptor 组合供应商请求和响应适配器，仅在传输阶段临时采用 Chat 上下文。
// 不复制 RelayInfo，不调用 TextHelper，计费仍由外层 ResponsesHelper 执行一次。
type responsesChatAdaptor struct {
	channel.Adaptor
	// request 保存公共转换后的 Chat 请求，供供应商 SDK 和响应处理器读取。
	request *dto.GeneralOpenAIRequest
	// writer 在请求保活和响应阶段共享，输出始终遵循客户端 Responses 协议。
	writer *responsesChatWriter
}

// useChatContext 临时切换供应商所读取的协议、路径和请求；返回恢复函数，保留同一上下文中的用量及渠道元数据。
func (a *responsesChatAdaptor) useChatContext(c *gin.Context, info *relaycommon.RelayInfo) func() {
	format, mode, path, request := info.RelayFormat, info.RelayMode, info.RequestURLPath, info.Request
	baseURL := info.ChannelBaseUrl
	info.RelayFormat, info.RelayMode, info.RequestURLPath = types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, "/v1/chat/completions"
	switch info.ApiType {
	case constant.APITypeSiliconFlow, constant.APITypeMistral, constant.APITypeSubmodel:
		// 这些适配器直接追加标准 Chat 路径；仅桥接阶段去除重复的版本段，不修改渠道存储值。
		info.ChannelBaseUrl = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	}
	if a.request != nil {
		info.Request = a.request
	}
	var originalRequest *http.Request
	if c != nil && c.Request != nil {
		originalRequest = c.Request
		c.Request = c.Request.Clone(c.Request.Context())
		c.Request.URL.Path, c.Request.URL.RawPath = info.RequestURLPath, ""
		c.Request.RequestURI = c.Request.URL.RequestURI()
	}
	return func() {
		info.RelayFormat, info.RelayMode, info.RequestURLPath, info.Request = format, mode, path, request
		info.ChannelBaseUrl = baseURL
		if originalRequest != nil {
			c.Request = originalRequest
		}
	}
}

// Init 让供应商按文本模式初始化内部状态，完成后恢复客户端协议。
func (a *responsesChatAdaptor) Init(info *relaycommon.RelayInfo) {
	restore := a.useChatContext(nil, info)
	defer restore()
	a.Adaptor.Init(info)
}

// GetRequestURL 使用供应商原有的 Chat 地址规则，完成后恢复 Responses 路径。
func (a *responsesChatAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	restore := a.useChatContext(nil, info)
	defer restore()
	return a.Adaptor.GetRequestURL(info)
}

// ConvertOpenAIResponsesRequest 保留文本、图片及函数调用历史；无法映射的状态与工具在出站前明确拒绝。
func (a *responsesChatAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if info.ApiType == constant.APITypePaLM && request.MaxOutputTokens != nil {
		return nil, responsesChatRequestError(fmt.Errorf("PaLM generateMessage cannot enforce max_output_tokens"))
	}
	info.InitRequestConversionChain()
	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, &request)
	if err != nil {
		return nil, responsesChatRequestError(err)
	}
	chat, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, responsesChatRequestError(fmt.Errorf("expected Chat request, got %T", result.Value))
	}
	for _, tool := range chat.Tools {
		if tool.Type != "function" {
			return nil, responsesChatRequestError(fmt.Errorf("Responses Chat conversion cannot preserve tool type %q", tool.Type))
		}
	}
	// Ollama 使用 tool_name 关联工具结果；依据已校验的 call_id 补齐名称，不让并行结果失去身份。
	toolNames := make(map[string]string)
	for index, message := range chat.Messages {
		for _, call := range message.ParseToolCalls() {
			if call.Type != "" && call.Type != "function" {
				return nil, responsesChatRequestError(fmt.Errorf("Responses Chat conversion cannot preserve history tool type %q", call.Type))
			}
			toolNames[call.ID] = call.Function.Name
		}
		if info.ApiType == constant.APITypeOllama && message.Role == "tool" && message.Name == nil {
			if name := toolNames[message.ToolCallId]; name != "" {
				chat.Messages[index].Name = &name
			}
		}
	}
	chat.MaxTokens, chat.MaxCompletionTokens = request.MaxOutputTokens, nil
	chat.StreamOptions = nil
	if info.IsStream && info.SupportStreamOptions {
		chat.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	if err := validateResponsesChatCapabilities(info.ApiType, chat); err != nil {
		return nil, responsesChatRequestError(err)
	}
	a.request = chat
	restore := a.useChatContext(c, info)
	defer restore()
	converted, err := a.Adaptor.ConvertOpenAIRequest(c, info, chat)
	if err != nil {
		return nil, responsesChatRequestError(err)
	}
	return converted, nil
}

// prepareWriter 保持客户端上下文的原始输出对象，防止供应商写入包装器时递归调用自身。
func (a *responsesChatAdaptor) prepareWriter(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if a.writer != nil {
		return nil
	}
	client := c.Copy()
	client.Writer = c.Writer
	var err *types.NewAPIError
	a.writer, err = newResponsesChatWriter(client, info)
	return err
}

// DoRequest 只执行供应商原有请求流程；SDK 延迟请求与需要轮询的供应商保留自身语义。
func (a *responsesChatAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if err := a.prepareWriter(c, info); err != nil {
		return nil, err
	}
	restore := a.useChatContext(c, info)
	defer restore()
	originalWriter := c.Writer
	c.Writer = a.writer
	defer func() { c.Writer = originalWriter }()
	return a.Adaptor.DoRequest(c, info, body)
}

// DoResponse 先由供应商把实际协议标准化为 Chat，再同步编码为 Responses；返回供应商原始用量供外层统一结算。
func (a *responsesChatAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if err := a.prepareWriter(c, info); err != nil {
		return nil, err
	}
	restore := a.useChatContext(c, info)
	defer restore()
	originalWriter := c.Writer
	defer func() { c.Writer = originalWriter }()
	c.Writer = a.writer
	usage, upstreamErr := a.Adaptor.DoResponse(c, resp, info)
	c.Writer = originalWriter
	restore()
	usageDTO, _ := usage.(*dto.Usage)
	if usageDTO == nil {
		usageDTO = a.writer.UsageFallback()
	}
	if err := a.writer.Finish(usageDTO, upstreamErr); err != nil {
		return usageDTO, err
	}
	return usageDTO, nil
}
