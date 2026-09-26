package relay

import (
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// useOpenAIProtocolBridge 判断本次 OpenAI 请求是否显式启用了指定的上游协议。
// Chat/Responses 请求优先读取实际路由分组；缺省沿用渠道配置，其他供应商不启用桥接。
func useOpenAIProtocolBridge(info *relaycommon.RelayInfo, bridge dto.OpenAIProtocolBridge) bool {
	if bridge == dto.OpenAIProtocolBridgeNative || info == nil || info.ChannelMeta == nil || info.ApiType != constant.APITypeOpenAI {
		return false
	}
	selected := info.ChannelSetting.OpenAIProtocolBridge
	if (info.RelayMode != relayconstant.RelayModeChatCompletions && info.RelayMode != relayconstant.RelayModeResponses) || info.UsingGroup == "" || info.UsingGroup == "auto" {
		return selected == bridge
	}
	// auto 重试可能换组，每次选择协议时读取实际分组，不改写共享渠道配置。
	groups := model_setting.GetGlobalSettings().GroupOpenAIProtocolBridge
	if groups != nil {
		if groupBridge, ok := groups.Get(info.UsingGroup); ok && groupBridge != dto.OpenAIProtocolBridgeNative {
			selected = groupBridge
		}
	}
	return selected == bridge
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
