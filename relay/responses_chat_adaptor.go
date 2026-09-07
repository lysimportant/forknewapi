package relay

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

// newResponsesAdaptor 优先使用原生 Responses；仅对已确认 Chat 契约的渠道启用转换。
// 原始请求体透传不能与 Chat 转换组合，冲突时在请求上游前返回不可重试的 400。
func newResponsesAdaptor(info *relaycommon.RelayInfo) (channel.Adaptor, *types.NewAPIError) {
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	if info.RelayMode != relayconstant.RelayModeResponses {
		return adaptor, nil
	}
	switch info.ApiType {
	case constant.APITypeSiliconFlow, constant.APITypeMistral, constant.APITypeMoonshot,
		constant.APITypeMiniMax, constant.APITypeSubmodel, constant.APITypeBaiduV2:
		if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("Responses requires Chat conversion for channel %s; request body pass-through must be disabled", adaptor.GetChannelName()),
				types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry(),
			)
		}
		return &responsesChatAdaptor{Adaptor: adaptor}, nil
	default:
		return adaptor, nil
	}
}

// responsesChatAdaptor 复用渠道的 Chat 请求处理与公共 Responses 响应编码，不进行失败后重发。
type responsesChatAdaptor struct {
	// Adaptor 保留渠道自身的鉴权、模型约束及请求地址规则。
	channel.Adaptor
}

// ConvertOpenAIResponsesRequest 将无服务端状态依赖的 Responses 请求转换为渠道可执行的 Chat 请求。
// 转换损失或不支持的输入在请求上游前返回 400，渠道转换错误保留原始上下文。
func (a *responsesChatAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if common.GetJsonType(request.Input) == "array" {
		// 转换器会把未知历史项当成普通消息；显式阻止工具结果、推理密文或状态引用被丢弃。
		var items []struct {
			// Type 是 Responses 历史项的协议类型；空值保留普通消息的简写形式。
			Type string `json:"type"`
		}
		if err := common.Unmarshal(request.Input, &items); err != nil {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("invalid Responses input: %w", err), types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		for index, item := range items {
			switch strings.TrimSpace(item.Type) {
			case "", "message", "function_call", "function_call_output":
			default:
				return nil, types.NewErrorWithStatusCode(
					fmt.Errorf("Responses to Chat conversion cannot preserve input[%d].type %q; use a native Responses channel", index, item.Type),
					types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry(),
				)
			}
		}
	}
	info.InitRequestConversionChain()
	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, &request)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	// 自动兼容不能继承默认 allow 而隐式删除工具；仅在当前请求检查诊断，不修改渠道配置。
	if err := types.RejectConversionLoss(types.ConversionLossPolicyStrict, result.Diagnostics); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	chatRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected Chat request after Responses conversion, got %T", result.Value)
	}
	// 这组渠道使用 max_tokens；不能将 Responses 上限留在可能不被识别的 max_completion_tokens 中。
	chatRequest.MaxTokens = request.MaxOutputTokens
	chatRequest.MaxCompletionTokens = nil
	chatRequest.StreamOptions = nil
	if info.IsStream && info.SupportStreamOptions {
		// Responses 需要终态用量；不将其专属 include_obfuscation 发给 Chat 上游。
		chatRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	// Mistral 会重建 DTO；记录已存在的顶层参数，防止后续渠道转换删除显式 false、0 或其他选项。
	var requestedFields map[string]any
	if info.ApiType == constant.APITypeMistral {
		requestedJSON, err := common.Marshal(chatRequest)
		if err != nil {
			return nil, fmt.Errorf("cannot encode Chat request before Mistral conversion: %w", err)
		}
		if err := common.Unmarshal(requestedJSON, &requestedFields); err != nil {
			return nil, fmt.Errorf("cannot inspect Chat request before Mistral conversion: %w", err)
		}
	}

	// 仅传输阶段切换三个字段，不复制含同步状态及计费信息的 RelayInfo。
	originalFormat, originalMode, originalPath := info.RelayFormat, info.RelayMode, info.RequestURLPath
	info.RelayFormat, info.RelayMode, info.RequestURLPath = types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, "/v1/chat/completions"
	defer func() {
		info.RelayFormat, info.RelayMode, info.RequestURLPath = originalFormat, originalMode, originalPath
	}()
	converted, err := a.Adaptor.ConvertOpenAIRequest(c, info, chatRequest)
	if err != nil {
		return nil, err
	}
	if requestedFields != nil {
		convertedJSON, err := common.Marshal(converted)
		if err != nil {
			return nil, fmt.Errorf("cannot encode Mistral request after conversion: %w", err)
		}
		var convertedFields map[string]any
		if err := common.Unmarshal(convertedJSON, &convertedFields); err != nil {
			return nil, fmt.Errorf("cannot inspect Mistral request after conversion: %w", err)
		}
		var missingFields []string
		for field := range requestedFields {
			if _, exists := convertedFields[field]; !exists {
				missingFields = append(missingFields, field)
			}
		}
		if len(missingFields) > 0 {
			// 仅暴露稳定排序后的参数名称，错误中不包含用户标识、元数据或参数值。
			sort.Strings(missingFields)
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("Mistral Responses to Chat conversion cannot preserve parameters: %s; remove these options or use a native Responses channel", strings.Join(missingFields, ", ")),
				types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry(),
			)
		}
	}
	return converted, nil
}

// DoRequest 使用渠道的 Chat 端点发送一次请求，返回前恢复客户端 Responses 协议上下文。
func (a *responsesChatAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	originalFormat, originalMode, originalPath := info.RelayFormat, info.RelayMode, info.RequestURLPath
	info.RelayFormat, info.RelayMode, info.RequestURLPath = types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, "/v1/chat/completions"
	defer func() {
		info.RelayFormat, info.RelayMode, info.RequestURLPath = originalFormat, originalMode, originalPath
	}()
	return a.Adaptor.DoRequest(c, info, requestBody)
}

// DoResponse 将 Chat 上游结果编码为客户端 Responses 格式，并返回原有计费流水所需的用量。
func (a *responsesChatAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info.IsStream {
		return openai.OaiChatToResponsesStreamHandler(c, info, resp)
	}
	return openai.OaiChatToResponsesHandler(c, info, resp)
}
