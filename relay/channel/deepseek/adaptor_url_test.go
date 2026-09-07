package deepseek

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeepSeekRequestURL 验证 Responses 自动补齐版本前缀，并保护 FIM、Chat 与 Claude 专用路径。
func TestDeepSeekRequestURL(t *testing.T) {
	for _, test := range []struct {
		name   string
		base   string
		format types.RelayFormat
		mode   int
		want   string
	}{
		{"Responses根域名", "https://api.deepseek.com", types.RelayFormatOpenAIResponses, relayconstant.RelayModeResponses, "https://api.deepseek.com/v1/responses"},
		{"Responses带版本", "https://upstream.test/v1/", types.RelayFormatOpenAIResponses, relayconstant.RelayModeResponses, "https://upstream.test/v1/responses"},
		{"Chat", "https://api.deepseek.com", types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, "https://api.deepseek.com/v1/chat/completions"},
		{"FIM", "https://api.deepseek.com", types.RelayFormatOpenAI, relayconstant.RelayModeCompletions, "https://api.deepseek.com/beta/completions"},
		{"FIM已有beta", "https://api.deepseek.com/beta", types.RelayFormatOpenAI, relayconstant.RelayModeCompletions, "https://api.deepseek.com/beta/completions"},
		{"Claude", "https://api.deepseek.com", types.RelayFormatClaude, relayconstant.RelayModeChatCompletions, "https://api.deepseek.com/anthropic/v1/messages"},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayFormat: test.format,
				RelayMode:   test.mode,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:    constant.ChannelTypeDeepSeek,
					ChannelBaseUrl: test.base,
				},
			}
			got, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
