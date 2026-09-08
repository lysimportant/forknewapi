package relay

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// validateResponsesChatCapabilities 在发送请求前拒绝现有渠道适配器必然丢失的工具与媒体字段。
// 校验仅依据代码中的转换路径，不预测具体上游模型的能力，也不修改请求或发起网络访问。
func validateResponsesChatCapabilities(apiType int, request *dto.GeneralOpenAIRequest) error {
	if request == nil {
		return errors.New("Responses Chat request is nil")
	}
	// textOnly 与 toolsUnsupported 对应明确只生成字符串消息、未映射工具结构的适配器。
	textOnly, toolsUnsupported := false, false
	claudeFormat := apiType == constant.APITypeAnthropic
	switch apiType {
	case constant.APITypePaLM, constant.APITypeBaidu, constant.APITypeZhipu,
		constant.APITypeTencent, constant.APITypeXunfei, constant.APITypeCohere, constant.APITypeCoze:
		textOnly, toolsUnsupported = true, true
	case constant.APITypeDify:
		toolsUnsupported = true
	case constant.APITypeAws:
		// 与 aws.isNovaModel 的分派条件一致；该 Nova DTO 只有 Text，没有工具或媒体字段。
		textOnly = strings.Contains(request.Model, "nova-")
		toolsUnsupported = textOnly
		claudeFormat = !textOnly
	case constant.APITypeVertexAi:
		// 与 Vertex 初始化中的 Claude 分派保持一致，其余 Vertex 协议由各自适配器校验。
		claudeFormat = strings.HasPrefix(request.Model, "claude")
	}
	if toolsUnsupported && (len(request.Tools) > 0 || request.ToolChoice != nil && request.ToolChoice != "none" && request.ToolChoice != "auto") {
		return fmt.Errorf("Responses Chat conversion for api type %d cannot preserve tools or tool_choice", apiType)
	}
	for _, message := range request.Messages {
		if toolsUnsupported && (len(message.ToolCalls) > 0 || message.ToolCallId != "" || message.Role == "tool" || message.Role == "function") {
			return fmt.Errorf("Responses Chat conversion for api type %d cannot preserve tool history", apiType)
		}
		if apiType == constant.APITypeCoze && message.Role != "user" {
			return errors.New("Responses Chat conversion for Coze cannot preserve non-user message history")
		}
		if apiType == constant.APITypeOllama && message.Role == "tool" && message.ToolCallId != "" && (message.Name == nil || *message.Name == "") {
			return errors.New("Responses Chat conversion for Ollama requires tool result name to preserve call_id association")
		}
		if message.Content == nil || message.IsStringContent() {
			continue
		}
		// 只读取原始内容的类型；ParseContent 会忽略不认识或不完整的媒体项，不能用于防丢失预检。
		var parts []struct {
			Type string `json:"type"`
		}
		contentJSON, err := common.Marshal(message.Content)
		if err != nil {
			return errors.New("Responses Chat conversion cannot encode message content")
		}
		if err := common.Unmarshal(contentJSON, &parts); err != nil {
			return errors.New("Responses Chat conversion requires text or typed message content")
		}
		for _, part := range parts {
			if part.Type == dto.ContentTypeText {
				continue
			}
			if textOnly {
				return fmt.Errorf("Responses Chat conversion for api type %d cannot preserve multimodal content %q", apiType, part.Type)
			}
			if apiType == constant.APITypeDify && (message.Role != "user" || part.Type != dto.ContentTypeImageURL) {
				return errors.New("Responses Chat conversion for Dify supports multimodal content only as user images")
			}
			if apiType == constant.APITypeOllama && part.Type != dto.ContentTypeImageURL {
				return fmt.Errorf("Responses Chat conversion for Ollama cannot preserve multimodal content %q", part.Type)
			}
			if claudeFormat && (message.Role == "system" || message.Role == "developer") {
				return errors.New("Responses Chat conversion to Claude cannot preserve multimodal system instructions")
			}
		}
	}
	return nil
}
