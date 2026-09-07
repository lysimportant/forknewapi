package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponsesChatCapabilitiesRejectsLoss 验证已知不保真的工具、工具历史和多模态请求在渠道请求前被拒绝。
func TestResponsesChatCapabilitiesRejectsLoss(t *testing.T) {
	for _, apiType := range []int{
		constant.APITypePaLM, constant.APITypeBaidu, constant.APITypeZhipu, constant.APITypeTencent,
		constant.APITypeXunfei, constant.APITypeCohere, constant.APITypeCoze,
	} {
		t.Run(GetAdaptor(apiType).GetChannelName(), func(t *testing.T) {
			err := validateResponsesChatCapabilities(apiType, &dto.GeneralOpenAIRequest{
				Tools: []dto.ToolCallRequest{{Type: "function", Function: dto.FunctionRequest{Name: "lookup"}}},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tools")
			err = validateResponsesChatCapabilities(apiType, &dto.GeneralOpenAIRequest{
				Messages: []dto.Message{{Role: "tool", ToolCallId: "call_1", Content: "OK"}},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tool history")
			err = validateResponsesChatCapabilities(apiType, &dto.GeneralOpenAIRequest{
				Messages: []dto.Message{{Role: "user", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL, ImageUrl: "https://example.test/image.png"}}}},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "multimodal")
		})
	}
	for _, test := range []struct {
		name    string
		apiType int
		request dto.GeneralOpenAIRequest
		want    string
	}{
		{"Coze助手历史", constant.APITypeCoze, dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "assistant", Content: "earlier"}}}, "non-user"},
		{"Dify工具定义", constant.APITypeDify, dto.GeneralOpenAIRequest{Tools: []dto.ToolCallRequest{{Type: "function"}}}, "tools"},
		{"Dify助手图片", constant.APITypeDify, dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "assistant", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL}}}}}, "user images"},
		{"Dify用户文件", constant.APITypeDify, dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "user", Content: []dto.MediaContent{{Type: dto.ContentTypeFile}}}}}, "user images"},
		{"Ollama音频", constant.APITypeOllama, dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "user", Content: []dto.MediaContent{{Type: dto.ContentTypeInputAudio}}}}}, "multimodal"},
		{"Ollama工具结果无名称", constant.APITypeOllama, dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "tool", ToolCallId: "call_1", Content: "OK"}}}, "name"},
		{"Nova工具", constant.APITypeAws, dto.GeneralOpenAIRequest{Model: "amazon.nova-pro-v1:0", Tools: []dto.ToolCallRequest{{Type: "function"}}}, "tools"},
		{"Nova图片", constant.APITypeAws, dto.GeneralOpenAIRequest{Model: "us.amazon.nova-lite-v1:0", Messages: []dto.Message{{Role: "user", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL}}}}}, "multimodal"},
		{"Claude系统图片", constant.APITypeAnthropic, dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "system", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL}}}}}, "system instructions"},
		{"AWSClaude系统图片", constant.APITypeAws, dto.GeneralOpenAIRequest{Model: "anthropic.claude-3-haiku", Messages: []dto.Message{{Role: "system", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL}}}}}, "system instructions"},
		{"VertexClaude系统图片", constant.APITypeVertexAi, dto.GeneralOpenAIRequest{Model: "claude-sonnet", Messages: []dto.Message{{Role: "system", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL}}}}}, "system instructions"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateResponsesChatCapabilities(test.apiType, &test.request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

// TestResponsesChatCapabilitiesAllowsPreservedRequests 防止预检按渠道或模型名称笼统拒绝普通文本及已映射的图片和函数能力。
func TestResponsesChatCapabilitiesAllowsPreservedRequests(t *testing.T) {
	for _, apiType := range []int{
		constant.APITypeAnthropic, constant.APITypePaLM, constant.APITypeBaidu, constant.APITypeZhipu,
		constant.APITypeXunfei, constant.APITypeTencent, constant.APITypeZhipuV4, constant.APITypeOllama,
		constant.APITypeAws, constant.APITypeCohere, constant.APITypeDify, constant.APITypeSiliconFlow,
		constant.APITypeVertexAi, constant.APITypeMistral, constant.APITypeBaiduV2, constant.APITypeCoze,
		constant.APITypeMoonshot, constant.APITypeSubmodel, constant.APITypeMiniMax,
	} {
		t.Run(GetAdaptor(apiType).GetChannelName(), func(t *testing.T) {
			require.NoError(t, validateResponsesChatCapabilities(apiType, &dto.GeneralOpenAIRequest{
				Model: "model-test", Messages: []dto.Message{{Role: "user", Content: "hello"}},
			}))
		})
	}
	for _, apiType := range []int{
		constant.APITypeAnthropic, constant.APITypeZhipuV4, constant.APITypeOllama, constant.APITypeAws,
		constant.APITypeSiliconFlow, constant.APITypeVertexAi, constant.APITypeMistral, constant.APITypeBaiduV2,
		constant.APITypeMoonshot, constant.APITypeSubmodel, constant.APITypeMiniMax,
	} {
		request := &dto.GeneralOpenAIRequest{
			Model: "claude-test", Tools: []dto.ToolCallRequest{{Type: "function", Function: dto.FunctionRequest{Name: "lookup"}}},
			Messages: []dto.Message{{Role: "user", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL, ImageUrl: "https://example.test/image.png"}}}},
		}
		require.NoError(t, validateResponsesChatCapabilities(apiType, request))
	}
	require.NoError(t, validateResponsesChatCapabilities(constant.APITypeDify, &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{{Role: "user", Content: []dto.MediaContent{{Type: dto.ContentTypeImageURL, ImageUrl: "https://example.test/image.png"}}}},
	}))
	require.NoError(t, validateResponsesChatCapabilities(constant.APITypeOllama, &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{{Role: "tool", ToolCallId: "call_1", Name: common.GetPointer("lookup"), Content: "OK"}},
	}))
}
