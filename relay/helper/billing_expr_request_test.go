package helper

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResolveIncomingBillingExprRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	body := []byte(`{"service_tier":"fast"}`)
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	ctx.Set(common.KeyRequestBody, body)

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	require.Equal(t, body, input.Body)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
}

func TestBuildBillingExprRequestInputFromRequest(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:  "gemini-3.1-pro-preview",
		Stream: lo.ToPtr(true),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
		MaxTokens: lo.ToPtr(uint(3000)),
	}

	input, err := BuildBillingExprRequestInputFromRequest(request, map[string]string{
		"Content-Type": "application/json",
		"X-Test":       "1",
	})
	require.NoError(t, err)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
	require.Equal(t, "1", input.Headers["X-Test"])
	require.True(t, gjson.GetBytes(input.Body, "stream").Bool())
	require.Equal(t, "user", gjson.GetBytes(input.Body, "messages.0.role").String())
	require.Equal(t, float64(3000), gjson.GetBytes(input.Body, "max_tokens").Float())
}

func TestResolveBillingEffortFromBodyAndSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("chat reasoning_effort", func(t *testing.T) {
		input := resolveEffortInput(t, &relaycommon.RelayInfo{OriginModelName: "gpt-5"}, []byte(`{"reasoning_effort":"MAX"}`))
		assert.Equal(t, "max", input.Effort)
	})

	t.Run("responses reasoning.effort", func(t *testing.T) {
		input := resolveEffortInput(t, &relaycommon.RelayInfo{OriginModelName: "gpt-5"}, []byte(`{"reasoning":{"effort":"high"}}`))
		assert.Equal(t, "high", input.Effort)
	})

	t.Run("model suffix overrides body", func(t *testing.T) {
		input := resolveEffortInput(t, &relaycommon.RelayInfo{
			OriginModelName: "gpt-5@effort:max",
			ReasoningEffort: "high",
		}, []byte(`{"reasoning_effort":"high"}`))
		assert.Equal(t, "max", input.Effort)
	})

	t.Run("legacy gpt-5-high suffix", func(t *testing.T) {
		input := resolveEffortInput(t, &relaycommon.RelayInfo{OriginModelName: "gpt-5-high"}, []byte(`{"model":"gpt-5-high"}`))
		assert.Equal(t, "high", input.Effort)
	})

	t.Run("codex-max is a real model id", func(t *testing.T) {
		input := resolveEffortInput(t, &relaycommon.RelayInfo{OriginModelName: "gpt-5.1-codex-max"}, []byte(`{"model":"gpt-5.1-codex-max"}`))
		assert.Empty(t, input.Effort)
	})

	t.Run("thinking-on does not invent high", func(t *testing.T) {
		input := resolveEffortInput(t, &relaycommon.RelayInfo{OriginModelName: "qwen3-max@thinking:on"}, []byte(`{}`))
		assert.Empty(t, input.Effort)
	})
}

func TestBuildBillingExprRequestInputFromRequestResolvesEffort(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:           "gpt-5",
		ReasoningEffort: "xhigh",
	}
	input, err := BuildBillingExprRequestInputFromRequest(request, nil)
	require.NoError(t, err)
	assert.Equal(t, "xhigh", input.Effort)
}

func resolveEffortInput(t *testing.T, info *relaycommon.RelayInfo, body []byte) billingexpr.RequestInput {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.KeyRequestBody, body)
	if info.RequestHeaders == nil {
		info.RequestHeaders = map[string]string{"Content-Type": "application/json"}
	}
	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	return input
}
