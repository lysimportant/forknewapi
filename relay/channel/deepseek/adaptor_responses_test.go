package deepseek

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponsesReasoning 保证原生 max 不变，并将映射后的 V4 后缀同步到上游模型和推理用量上下文。
func TestResponsesReasoning(t *testing.T) {
	for _, test := range []struct {
		name          string
		model         string
		upstreamModel string
		effort        string
		wantModel     string
		wantEffort    string
	}{
		{"保留原生 max", "deepseek-v4-flash-vision-exp", "deepseek-v4-flash-vision-exp", "max", "deepseek-v4-flash-vision-exp", "max"},
		{"max 后缀", "deepseek-v4-flash-max", "deepseek-v4-flash-max", "", "deepseek-v4-flash", "max"},
		{"映射后禁用思考", "mapped-model", "deepseek-v4-flash-none", "max", "deepseek-v4-flash", "none"},
		{"非 V4 模型不剥后缀", "custom-max", "custom-max", "", "custom-max", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := dto.OpenAIResponsesRequest{Model: test.model}
			if test.effort != "" {
				request.Reasoning = &dto.Reasoning{Effort: test.effort, Summary: "auto"}
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: test.upstreamModel}}
			result, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
			require.NoError(t, err)
			require.IsType(t, dto.OpenAIResponsesRequest{}, result)
			converted := result.(dto.OpenAIResponsesRequest)
			assert.Equal(t, test.wantModel, converted.Model)
			assert.Equal(t, test.wantModel, info.UpstreamModelName)
			assert.Equal(t, test.wantEffort, info.ReasoningEffort)
			if test.wantEffort != "" {
				require.NotNil(t, converted.Reasoning)
				assert.Equal(t, test.wantEffort, converted.Reasoning.Effort)
			}
			if test.effort != "" {
				assert.Equal(t, "auto", converted.Reasoning.Summary)
			}
		})
	}
}
