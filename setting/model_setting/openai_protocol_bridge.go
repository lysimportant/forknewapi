package model_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// GroupOpenAIProtocolBridgeOptionKey 是实际分组到 OpenAI 上游文本协议覆盖表的系统配置键。
const GroupOpenAIProtocolBridgeOptionKey = "global.group_openai_protocol_bridge"

// ValidateGroupOpenAIProtocolBridgeJSON 校验分组协议覆盖表，拒绝无效分组、保留组 auto 及未知协议。
// value 必须是 JSON 对象字符串；空对象或空字符串值表示不覆盖既有策略，校验不修改内存配置。
func ValidateGroupOpenAIProtocolBridgeJSON(value string) error {
	var groups map[string]*dto.OpenAIProtocolBridge
	if err := common.UnmarshalJsonStr(value, &groups); err != nil {
		return fmt.Errorf("invalid %s: %w", GroupOpenAIProtocolBridgeOptionKey, err)
	}
	if groups == nil {
		return fmt.Errorf("%s must be a JSON object", GroupOpenAIProtocolBridgeOptionKey)
	}
	for group, bridge := range groups {
		if group == "" || group != strings.TrimSpace(group) || group == "auto" {
			return fmt.Errorf("%s: invalid group %q; use an actual routing group, not auto", GroupOpenAIProtocolBridgeOptionKey, group)
		}
		if bridge == nil {
			return fmt.Errorf("%s: protocol for group %q must be a string", GroupOpenAIProtocolBridgeOptionKey, group)
		}
		switch *bridge {
		case dto.OpenAIProtocolBridgeNative, dto.OpenAIProtocolBridgeChat, dto.OpenAIProtocolBridgeResponses:
		default:
			return fmt.Errorf("%s: invalid protocol %q for group %q", GroupOpenAIProtocolBridgeOptionKey, *bridge, group)
		}
	}
	return nil
}
