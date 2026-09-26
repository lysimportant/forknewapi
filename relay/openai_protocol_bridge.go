package relay

import (
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

// useOpenAIProtocolBridge 判断当前渠道是否显式启用了指定的 OpenAI 协议桥接模式。
// 空值保持原生转发，只有 APITypeOpenAI 渠道会启用该兼容路径。
func useOpenAIProtocolBridge(info *relaycommon.RelayInfo, bridge dto.OpenAIProtocolBridge) bool {
	return bridge != dto.OpenAIProtocolBridgeNative &&
		info != nil && info.ApiType == constant.APITypeOpenAI && info.ChannelSetting.OpenAIProtocolBridge == bridge
}

// shouldUseResponsesForChat 决定 Chat Completions 是否转成 Responses。
// 明确启用 Chat 桥接时优先保持 Chat 原生路径，避免全局兼容策略把请求再次送入有问题的 Responses 上游。
func shouldUseResponsesForChat(info *relaycommon.RelayInfo) bool {
	if useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeChat) {
		return false
	}
	return service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) ||
		useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeResponses)
}
