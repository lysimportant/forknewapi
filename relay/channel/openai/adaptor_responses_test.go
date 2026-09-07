package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertOpenAIResponsesRequestReasoningCompatibility 验证原生 DeepSeek V4 的推理力度、模型映射与显式修饰符，并保护 OpenAI 和跨协议转换的既有语义。
func TestConvertOpenAIResponsesRequestReasoningCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name        string
		model       string
		mappedModel string
		effort      string
		fromChat    bool
		wantModel   string
		wantEffort  string
	}{
		{name: "DeepSeek vision 原生 max", model: "deepseek-v4-flash-vision-exp", effort: "max", wantModel: "deepseek-v4-flash-vision-exp", wantEffort: "max"},
		{name: "DeepSeek pro 原生 none", model: "deepseek-v4-pro", effort: "none", wantModel: "deepseek-v4-pro", wantEffort: "none"},
		{name: "DeepSeek 缺省 reasoning", model: "deepseek-v4-pro", wantModel: "deepseek-v4-pro"},
		{name: "DeepSeek 命名空间", model: "provider/deepseek-v4-pro", effort: "max", wantModel: "provider/deepseek-v4-pro", wantEffort: "max"},
		{name: "映射到 DeepSeek", model: "coding-model", mappedModel: "deepseek-v4-pro", effort: "max", wantModel: "deepseek-v4-pro", wantEffort: "max"},
		{name: "DeepSeek 映射到 OpenAI", model: "deepseek-v4-pro", mappedModel: "gpt-5", effort: "max", wantModel: "gpt-5", wantEffort: "xhigh"},
		{name: "映射模型显式修饰符覆盖请求", model: "coding-model", mappedModel: "deepseek-v4-pro@effort:max", effort: "none", wantModel: "deepseek-v4-pro", wantEffort: "max"},
		{name: "原始模型显式修饰符", model: "deepseek-v4-pro@effort:max", effort: "none", wantModel: "deepseek-v4-pro", wantEffort: "max"},
		{name: "DeepSeek 裸后缀保持模型标识", model: "deepseek-v4-pro-max", effort: "max", wantModel: "deepseek-v4-pro-max", wantEffort: "max"},
		{name: "OpenAI 显式 max 继续投影", model: "gpt-5", effort: "max", wantModel: "gpt-5", wantEffort: "xhigh"},
		{name: "OpenAI 推理后缀", model: "gpt-5-high", wantModel: "gpt-5", wantEffort: "high"},
		{name: "Chat 转 Responses 继续投影", model: "deepseek-v4-pro", effort: "max", fromChat: true, wantModel: "deepseek-v4-pro", wantEffort: "xhigh"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &dto.OpenAIResponsesRequest{
				Model: test.model,
				Input: json.RawMessage(`[{"type":"function_call_output","call_id":"call_1","output":"done"}]`),
				Tools: json.RawMessage(`[{"type":"function","name":"read_file","parameters":{"type":"object"}}]`),
				Store: json.RawMessage(`false`),
			}
			if test.effort != "" {
				request.Reasoning = &dto.Reasoning{Effort: test.effort, Summary: "auto"}
			}
			info := &relaycommon.RelayInfo{
				OriginModelName: test.model,
				Request:         request,
				RelayFormat:     types.RelayFormatOpenAIResponses,
				RelayMode:       relayconstant.RelayModeResponses,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeOpenAI,
					UpstreamModelName: test.model,
				},
			}
			if test.fromChat {
				info.RelayFormat = types.RelayFormatOpenAI
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if test.mappedModel != "" {
				mapping, err := common.Marshal(map[string]string{test.model: test.mappedModel})
				require.NoError(t, err)
				ctx.Set("model_mapping", string(mapping))
			}
			outbound, err := common.DeepCopy(request)
			require.NoError(t, err)
			require.NoError(t, helper.ModelMappedHelper(ctx, info, outbound))
			require.NoError(t, helper.ApplyReasoningModelSuffix(ctx, info, outbound))

			converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(ctx, info, *outbound)

			require.NoError(t, err)
			got, ok := converted.(dto.OpenAIResponsesRequest)
			require.True(t, ok)
			assert.Equal(t, test.wantModel, got.Model)
			assert.Equal(t, test.wantModel, info.UpstreamModelName)
			assert.Equal(t, test.wantEffort, info.ReasoningEffort)
			if test.wantEffort == "" {
				assert.Nil(t, got.Reasoning)
			} else {
				require.NotNil(t, got.Reasoning)
				assert.Equal(t, test.wantEffort, got.Reasoning.Effort)
				if test.effort != "" {
					assert.Equal(t, "auto", got.Reasoning.Summary)
				}
			}
			assert.JSONEq(t, string(request.Input), string(got.Input))
			assert.JSONEq(t, string(request.Tools), string(got.Tools))
			assert.JSONEq(t, `false`, string(got.Store))
		})
	}
}
