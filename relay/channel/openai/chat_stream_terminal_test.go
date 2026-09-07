package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestOaiStreamHandlerPreservesTerminalChoice 验证关闭 include_usage 时仍转发携带用量的终止选择与最后工具分片，仅省略独立用量帧。
func TestOaiStreamHandlerPreservesTerminalChoice(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	for _, test := range []struct {
		name         string
		choices      string
		includeUsage bool
		wantLast     bool
	}{
		{"stop", `[{"index":0,"delta":{},"finish_reason":"stop"}]`, false, true},
		{"tool-terminal", `[{"index":0,"delta":{},"finish_reason":"tool_calls"}]`, false, true},
		{"final-tool-fragment", `[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]`, false, true},
		{"length", `[{"index":0,"delta":{},"finish_reason":"length"}]`, false, true},
		{"content-filter", `[{"index":0,"delta":{},"finish_reason":"content_filter"}]`, false, true},
		{"omit-only-usage", `[]`, false, false},
		{"include-only-usage", `[]`, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			last := `{"id":"last","object":"chat.completion.chunk","choices":` + test.choices + `,"usage":{"prompt_tokens":374,"completion_tokens":78,"total_tokens":452}}`
			body := "data: {\"id\":\"first\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"}}]}\n\ndata: " + last + "\n\ndata: [DONE]\n\n"
			ctx, recorder, response, info := newResponsesChatTestContext(t, body, true)
			info.RelayMode = relayconstant.RelayModeChatCompletions
			info.ShouldIncludeUsage = test.includeUsage
			usage, apiErr := OaiStreamHandler(ctx, info, response)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 374, usage.PromptTokens)
			assert.Equal(t, 78, usage.CompletionTokens)
			assert.Equal(t, 452, usage.TotalTokens)
			var finalData string
			for _, frame := range strings.Split(recorder.Body.String(), "\n\n") {
				data := strings.TrimPrefix(frame, "data: ")
				if gjson.Get(data, "id").String() == "last" {
					finalData = data
				}
			}
			if test.wantLast {
				require.NotEmpty(t, finalData)
				assert.JSONEq(t, last, finalData)
			} else {
				assert.Empty(t, finalData)
			}
			assert.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
		})
	}
}
