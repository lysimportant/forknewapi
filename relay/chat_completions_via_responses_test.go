package relay

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/model_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIProtocolBridgeChatOverridesGlobalResponsesPolicy(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.ChatCompletionsToResponsesPolicy
	t.Cleanup(func() { settings.ChatCompletionsToResponsesPolicy = original })
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{".*"},
	}

	info := &relaycommon.RelayInfo{
		OriginModelName: "deepseek-v4.1-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:     constant.APITypeOpenAI,
			ChannelType: constant.ChannelTypeOpenAI,
			ChannelId:   1,
			ChannelSetting: dto.ChannelSettings{
				OpenAIProtocolBridge: dto.OpenAIProtocolBridgeChat,
			},
		},
	}

	assert.False(t, shouldUseResponsesForChat(info))
	info.ChannelSetting.OpenAIProtocolBridge = dto.OpenAIProtocolBridgeNative
	assert.True(t, shouldUseResponsesForChat(info))
	info.ChannelSetting.OpenAIProtocolBridge = dto.OpenAIProtocolBridgeResponses
	assert.True(t, shouldUseResponsesForChat(info))
}

func TestOpenAIProtocolBridgeIsChannelScoped(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
			ChannelSetting: dto.ChannelSettings{
				OpenAIProtocolBridge: dto.OpenAIProtocolBridgeResponses,
			},
		},
	}

	assert.True(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeResponses))
	assert.False(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeChat))
	assert.False(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeNative))

	info.ApiType = constant.APITypeOpenRouter
	assert.False(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeResponses))
}

func TestIsResponsesEventStreamContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain", contentType: "text/event-stream", want: true},
		{name: "mixed case with charset", contentType: "Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponsesEventStreamContentType(tt.contentType))
		})
	}
}

func TestRecalcQuotaFromRatiosIgnoresInvalidMultipliers(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: hosttypes.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"duration": 3,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.True(t, ok)
	assert.Equal(t, 150, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestRecalcQuotaFromRatiosRejectsAllInvalidAdjustedRatios(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: hosttypes.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.False(t, ok)
	assert.Equal(t, 0, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestTextRequestViaResponsesConvertsClaudeDirectly(t *testing.T) {
	type capturedRequest struct {
		path string
		body []byte
	}
	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured <- capturedRequest{path: r.URL.Path, body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1",
			"object":"response",
			"status":"completed",
			"model":"gpt-5.6-sol",
			"output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],
			"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}
		}`))
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		RelayMode:              relayconstant.RelayModeChatCompletions,
		RelayFormat:            relaytypes.RelayFormatClaude,
		OriginModelName:        "gpt-5.6-sol",
		RequestConversionChain: []relaytypes.RelayFormat{relaytypes.RelayFormatClaude},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "gpt-5.6-sol",
		},
	}
	adaptor := &openaichannel.Adaptor{}
	adaptor.Init(info)
	request := &dto.ClaudeRequest{
		Model:    "gpt-5.6-sol",
		Thinking: &dto.Thinking{Type: "adaptive", Display: "summarized"},
		Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
	}

	usage, apiErr := textRequestViaResponses(c, info, adaptor, request)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.Equal(t, []relaytypes.RelayFormat{relaytypes.RelayFormatClaude, relaytypes.RelayFormatOpenAIResponses}, info.RequestConversionChain)

	upstream := <-captured
	assert.Equal(t, "/v1/responses", upstream.path)
	var upstreamBody map[string]any
	require.NoError(t, common.Unmarshal(upstream.body, &upstreamBody))
	assert.NotContains(t, upstreamBody, "messages")
	reasoning, ok := upstreamBody["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", reasoning["effort"])
	assert.Equal(t, "detailed", reasoning["summary"])

	var response dto.ClaudeResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Content, 1)
	assert.Equal(t, "ok", response.Content[0].GetText())
}

func TestApplySystemPromptIfNeededSkipsToolLoadingMessages(t *testing.T) {
	tools := json.RawMessage(`[{"type":"function","function":{"name":"get_current_time","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]`)
	toolLoading := dto.Message{Role: "system", Tools: tools}
	user := dto.Message{Role: "user", Content: "What time is it in Beijing?"}

	tests := []struct {
		name         string
		messages     []dto.Message
		wantMessages []dto.Message
		wantOverride bool
	}{
		{
			name:     "tool loading message alone is not a system prompt",
			messages: []dto.Message{toolLoading, user},
			wantMessages: []dto.Message{
				{Role: "system", Content: "Answer in English."},
				toolLoading,
				user,
			},
		},
		{
			name:     "override targets the real system prompt only",
			messages: []dto.Message{toolLoading, {Role: "system", Content: "You are Kimi."}, user},
			wantMessages: []dto.Message{
				toolLoading,
				{Role: "system", Content: "Answer in English.\nYou are Kimi."},
				user,
			},
			wantOverride: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelSetting: dto.ChannelSettings{
						SystemPrompt:         "Answer in English.",
						SystemPromptOverride: true,
					},
				},
			}
			request := &dto.GeneralOpenAIRequest{
				Model:    "kimi-k3",
				Messages: append([]dto.Message(nil), tt.messages...),
			}

			applySystemPromptIfNeeded(c, info, request)

			require.Len(t, request.Messages, len(tt.wantMessages))
			for i, want := range tt.wantMessages {
				got := request.Messages[i]
				assert.Equal(t, want.Role, got.Role, "message %d role", i)
				assert.Equal(t, want.Content, got.Content, "message %d content", i)
				if len(want.Tools) > 0 {
					assert.JSONEq(t, string(want.Tools), string(got.Tools), "message %d tools", i)
				} else {
					assert.Empty(t, got.Tools, "message %d tools", i)
				}
			}
			_, overrideSet := common.GetContextKey(c, constant.ContextKeySystemPromptOverride)
			assert.Equal(t, tt.wantOverride, overrideSet)
		})
	}
}

// TestGroupOpenAIProtocolBridgeUsesCurrentRoutingGroup 验证分组覆盖按本次实际路由组读取，且不会修改共享渠道或旧的全局策略。
func TestGroupOpenAIProtocolBridgeUsesCurrentRoutingGroup(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original, err := config.ConfigToMap(settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(settings, original)) })
	require.NoError(t, config.UpdateConfigFromMap(settings, map[string]string{
		"group_openai_protocol_bridge": `{"openclaw":"chat","native-responses":"responses","inherit":""}`,
	}))
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{
		Enabled: true, AllChannels: true, ModelPatterns: []string{".*"},
	}
	channel := &relaycommon.ChannelMeta{
		ApiType: constant.APITypeOpenAI, ChannelType: constant.ChannelTypeOpenAI, ChannelId: 1,
		ChannelSetting: dto.ChannelSettings{OpenAIProtocolBridge: dto.OpenAIProtocolBridgeResponses},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: channel, RelayMode: relayconstant.RelayModeChatCompletions,
		OriginModelName: "model-test", UserGroup: "native-responses", TokenGroup: "auto",
	}
	for _, group := range []string{"openclaw", "other", "openclaw", "inherit", "", "auto"} {
		info.UsingGroup = group
		assert.Equal(t, group != "openclaw", shouldUseResponsesForChat(info), "using group %q", group)
		assert.Equal(t, dto.OpenAIProtocolBridgeResponses, channel.ChannelSetting.OpenAIProtocolBridge)
	}
	channel.ChannelSetting.OpenAIProtocolBridge = dto.OpenAIProtocolBridgeChat
	info.UsingGroup = "native-responses"
	assert.True(t, shouldUseResponsesForChat(info), "明确分组策略应覆盖相反的渠道策略")
	info.RelayMode = relayconstant.RelayModeResponses
	assert.True(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeResponses))
	assert.False(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeChat))

	// 删除条目后必须恢复渠道行为，不能因配置解码合并 map 而留下旧覆盖。
	require.NoError(t, config.UpdateConfigFromMap(settings, map[string]string{"group_openai_protocol_bridge": "{}"}))
	assert.True(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeChat))
}

// TestGroupOpenAIProtocolBridgePreservesOtherRoutes 验证分组覆盖不改变其他供应商、非目标路由及未初始化的请求。
func TestGroupOpenAIProtocolBridgePreservesOtherRoutes(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original, err := config.ConfigToMap(settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(settings, original)) })
	require.NoError(t, config.UpdateConfigFromMap(settings, map[string]string{
		"group_openai_protocol_bridge": `{"openclaw":"chat"}`,
	}))
	info := &relaycommon.RelayInfo{
		UsingGroup: "openclaw", RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeOpenAI},
	}
	assert.True(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeChat))
	info.ApiType = constant.APITypeOpenRouter
	assert.False(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeChat))
	info.ApiType = constant.APITypeOpenAI
	for _, mode := range []int{relayconstant.RelayModeResponsesCompact, relayconstant.RelayModeCompletions, relayconstant.RelayModeEmbeddings} {
		info.RelayMode = mode
		assert.False(t, useOpenAIProtocolBridge(info, dto.OpenAIProtocolBridgeChat), "relay mode %d", mode)
	}
	assert.False(t, useOpenAIProtocolBridge(nil, dto.OpenAIProtocolBridgeChat))
	assert.False(t, useOpenAIProtocolBridge(&relaycommon.RelayInfo{}, dto.OpenAIProtocolBridgeChat))
}
