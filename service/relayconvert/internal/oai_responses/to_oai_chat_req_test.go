package oairesponses

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	oaichat "github.com/QuantumNous/new-api/service/relayconvert/internal/oai_chat"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesRequestToChatCompletionsRequestInstructionsAndScalarInput(t *testing.T) {
	stream := true
	temperature := 0.0
	topP := 0.9
	maxOutputTokens := uint(128)
	parallelToolCalls := true

	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:                "gpt-test",
		Instructions:         mustRawMessage(t, "system rules"),
		Input:                mustRawMessage(t, "hello"),
		Stream:               &stream,
		StreamOptions:        &dto.StreamOptions{IncludeUsage: true},
		MaxOutputTokens:      &maxOutputTokens,
		Temperature:          &temperature,
		TopP:                 &topP,
		User:                 mustRawMessage(t, "user-1"),
		Store:                mustRawMessage(t, false),
		Metadata:             mustRawMessage(t, map[string]any{"trace": "abc"}),
		ParallelToolCalls:    mustRawMessage(t, parallelToolCalls),
		PromptCacheKey:       mustRawMessage(t, "cache-key"),
		PromptCacheRetention: mustRawMessage(t, "24h"),
		Reasoning:            &dto.Reasoning{Effort: "medium"},
	})
	require.NoError(t, err)

	assert.Equal(t, "gpt-test", got.Model)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, dto.Message{Role: "system", Content: "system rules"}, got.Messages[0])
	assert.Equal(t, dto.Message{Role: "user", Content: "hello"}, got.Messages[1])
	assert.Same(t, &stream, got.Stream)
	require.NotNil(t, got.StreamOptions)
	assert.True(t, got.StreamOptions.IncludeUsage)
	assert.Equal(t, maxOutputTokens, lo.FromPtr(got.MaxCompletionTokens))
	assert.Equal(t, 0.0, lo.FromPtr(got.Temperature))
	assert.Equal(t, 0.9, lo.FromPtr(got.TopP))
	assert.True(t, lo.FromPtr(got.ParallelTooCalls))
	assert.Equal(t, "cache-key", got.PromptCacheKey)
	assert.Equal(t, "medium", got.ReasoningEffort)
	assert.Equal(t, `"user-1"`, string(got.User))
	assert.Equal(t, `false`, string(got.Store))
	assert.Equal(t, "abc", gjson.GetBytes(got.Metadata, "trace").String())
}

// TestResponsesRequestToChatCompletionsRequestMultimodalInput 保护多模态输入内容及图片 detail 的转换语义。
func TestResponsesRequestToChatCompletionsRequestMultimodalInput(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "look"},
					{"type": "input_image", "image_url": "https://example.test/a.png", "detail": "low"},
					{"type": "input_file", "file_id": "file_1", "filename": "a.txt"},
					{"type": "input_audio", "input_audio": map[string]any{"data": "abc", "format": "wav"}},
					{"type": "input_video", "video_url": map[string]any{"url": "https://example.test/v.mp4"}},
				},
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	assert.Equal(t, "user", got.Messages[0].Role)
	parts := got.Messages[0].ParseContent()
	require.Len(t, parts, 5)
	assert.Equal(t, dto.ContentTypeText, parts[0].Type)
	assert.Equal(t, "look", parts[0].Text)
	assert.Equal(t, dto.ContentTypeImageURL, parts[1].Type)
	assert.Equal(t, "https://example.test/a.png", parts[1].GetImageMedia().Url)
	assert.Equal(t, "low", parts[1].GetImageMedia().Detail)
	assert.Equal(t, dto.ContentTypeFile, parts[2].Type)
	assert.Equal(t, "file_1", parts[2].GetFile().FileId)
	assert.Equal(t, dto.ContentTypeInputAudio, parts[3].Type)
	assert.Equal(t, "wav", parts[3].GetInputAudio().Format)
	assert.Equal(t, dto.ContentTypeVideoUrl, parts[4].Type)
	assert.Equal(t, "https://example.test/v.mp4", parts[4].GetVideoUrl().Url)
}

func TestResponsesRequestToChatCompletionsRequestAssistantTextAndFunctionCallCoexist(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": "I will call."},
				},
			},
			{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "lookup",
				"arguments": map[string]any{"q": "x"},
			},
			{
				"type":    "function_call_output",
				"call_id": "call_1",
				"output":  map[string]any{"ok": true},
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 2)
	assert.Equal(t, "assistant", got.Messages[0].Role)
	assert.Equal(t, "I will call.", got.Messages[0].StringContent())
	toolCalls := got.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0].ID)
	assert.Equal(t, "function", toolCalls[0].Type)
	assert.Equal(t, "lookup", toolCalls[0].Function.Name)
	assert.JSONEq(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	assert.Equal(t, "tool", got.Messages[1].Role)
	assert.Equal(t, "call_1", got.Messages[1].ToolCallId)
	assert.JSONEq(t, `{"ok":true}`, got.Messages[1].StringContent())
}

func TestResponsesRequestToChatCompletionsRequestOnlyFunctionCallCreatesAssistant(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "lookup",
				"arguments": `{"q":"x"}`,
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	assert.Equal(t, "assistant", got.Messages[0].Role)
	assert.Nil(t, got.Messages[0].Content)
	toolCalls := got.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
}

func TestResponsesRequestToChatCompletionsRequestToolsToolChoiceAndTextFormat(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, "hello"),
		Tools: mustRawMessage(t, []map[string]any{
			{
				"type":        "function",
				"name":        "lookup",
				"description": "Lookup data",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"q": map[string]any{"type": "string"},
					},
				},
			},
		}),
		ToolChoice: mustRawMessage(t, map[string]any{
			"type": "function",
			"name": "lookup",
		}),
		Text: mustRawMessage(t, map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "answer",
				"schema": map[string]any{"type": "object"},
				"strict": true,
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Tools, 1)
	assert.Equal(t, "function", got.Tools[0].Type)
	assert.Equal(t, "lookup", got.Tools[0].Function.Name)
	assert.Equal(t, "Lookup data", got.Tools[0].Function.Description)
	assert.Equal(t, "object", got.Tools[0].Function.Parameters.(map[string]any)["type"])
	assert.Equal(t, map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "lookup",
		},
	}, got.ToolChoice)
	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_schema", got.ResponseFormat.Type)
	assert.Equal(t, "answer", gjson.GetBytes(got.ResponseFormat.JsonSchema, "name").String())
	assert.True(t, gjson.GetBytes(got.ResponseFormat.JsonSchema, "strict").Bool())
}

func TestResponsesRequestToChatCompletionsRequestCustomToolCallPreservesRawShape(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"type":    "custom_tool_call",
				"call_id": "call_custom",
				"name":    "apply_patch",
				"input":   "patch body",
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	toolCalls := got.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, dto.CustomType, toolCalls[0].Type)
	assert.Equal(t, "call_custom", toolCalls[0].ID)
	assert.Equal(t, "apply_patch", toolCalls[0].Function.Name)
	assert.Equal(t, "patch body", toolCalls[0].Function.Arguments)
	assert.Equal(t, "custom_tool_call", gjson.GetBytes(toolCalls[0].Custom, "type").String())
	assert.Equal(t, "patch body", gjson.GetBytes(toolCalls[0].Custom, "input").String())
}

func TestResponsesRequestToChatCompletionsRequestRejectsStatefulFields(t *testing.T) {
	tests := []struct {
		name string
		req  *dto.OpenAIResponsesRequest
		want string
	}{
		{
			name: "conversation",
			req:  &dto.OpenAIResponsesRequest{Model: "gpt-test", Conversation: mustRawMessage(t, "conv_1")},
			want: "conversation",
		},
		{
			name: "previous response",
			req:  &dto.OpenAIResponsesRequest{Model: "gpt-test", PreviousResponseID: "resp_1"},
			want: "previous_response_id",
		},
		{
			name: "prompt",
			req:  &dto.OpenAIResponsesRequest{Model: "gpt-test", Prompt: mustRawMessage(t, map[string]any{"id": "pmpt_1"})},
			want: "prompt",
		},
		{
			name: "context management",
			req:  &dto.OpenAIResponsesRequest{Model: "gpt-test", ContextManagement: mustRawMessage(t, map[string]any{"type": "auto"})},
			want: "context_management",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResponsesRequestToChatCompletionsRequest(tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Contains(t, err.Error(), "stateful fields")
		})
	}
}

// TestResponsesRequestToChatRejectsUnrepresentableInput 防止状态引用、密文或未知项目被静默转换为空消息。
func TestResponsesRequestToChatRejectsUnrepresentableInput(t *testing.T) {
	for _, test := range []struct {
		name string
		item map[string]any
		want string
	}{
		{"未知项目", map[string]any{"type": "future_item", "content": "sensitive-payload"}, "future_item"},
		{"状态引用", map[string]any{"type": "item_reference", "id": "sensitive-payload"}, "item_reference"},
		{"推理密文", map[string]any{"type": "reasoning", "encrypted_content": "sensitive-payload"}, "reasoning"},
		{"消息密文", map[string]any{"type": "message", "role": "assistant", "content": "hello", "encrypted_content": "sensitive-payload"}, "encrypted_content"},
		{"自定义工具结果", map[string]any{"type": "custom_tool_call_output", "call_id": "call_1", "output": "sensitive-payload"}, "custom_tool_call_output"},
		{"缺失摘要", map[string]any{"type": "reasoning"}, "summary"},
		{"未知推理内容", map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "reasoning_text", "text": "sensitive-payload"}}}, "content"},
		{"摘要类型错误", map[string]any{"type": "reasoning", "summary": "sensitive-payload"}, "summary"},
		{"未知摘要项目", map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "future_summary", "text": "sensitive-payload"}}}, "summary_text"},
		{"摘要文本类型错误", map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": map[string]any{"content": "sensitive-payload"}}}}, "summary_text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
				Model: "gpt-test", Input: mustRawMessage(t, []map[string]any{test.item}),
			})
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), test.want)
			assert.NotContains(t, err.Error(), "sensitive-payload")
		})
	}
}

// TestResponsesReasoningSummaryToolHistory 保留可表达的推理摘要，使其与同一轮文字、函数调用及后续工具结果一起回传。
func TestResponsesReasoningSummaryToolHistory(t *testing.T) {
	for _, textFirst := range []bool{false, true} {
		message := map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "I will look it up."}}}
		reasoning := map[string]any{"type": "reasoning", "summary": []any{
			map[string]any{"type": "summary_text", "text": "First thought."},
			map[string]any{"type": "summary_text", "text": "Second thought."},
		}}
		items := []map[string]any{reasoning, message}
		if textFirst {
			items = []map[string]any{message, reasoning}
		}
		items = append(items,
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "OK"},
		)
		chat, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{Model: "model-test", Input: mustRawMessage(t, items)})
		require.NoError(t, err)
		require.Len(t, chat.Messages, 2)
		assert.Equal(t, "assistant", chat.Messages[0].Role)
		assert.Equal(t, "I will look it up.", chat.Messages[0].StringContent())
		assert.Equal(t, "First thought.\n\nSecond thought.", chat.Messages[0].GetReasoningContent())
		require.Len(t, chat.Messages[0].ParseToolCalls(), 1)
		assert.Equal(t, "call_1", chat.Messages[0].ParseToolCalls()[0].ID)
		assert.Equal(t, "call_1", chat.Messages[1].ToolCallId)
		assert.Equal(t, "OK", chat.Messages[1].StringContent())
	}
}

// TestResponsesRequestToChatFunctionHistory 验证工具调用标识与结果关联，不强制补齐截取历史末尾的调用结果。
func TestResponsesRequestToChatFunctionHistory(t *testing.T) {
	call := map[string]any{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": "{}"}
	output := map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "OK"}
	for _, test := range []struct {
		name  string
		items []map[string]any
	}{
		{"调用缺少call_id", []map[string]any{{"type": "function_call", "name": "lookup", "arguments": "{}"}}},
		{"项目id不能代替call_id", []map[string]any{{"type": "function_call", "id": "fc_1", "name": "lookup", "arguments": "{}"}}},
		{"空白call_id", []map[string]any{{"type": "function_call", "call_id": "  ", "name": "lookup", "arguments": "{}"}}},
		{"结果缺少call_id", []map[string]any{call, {"type": "function_call_output", "output": "OK"}}},
		{"重复调用", []map[string]any{call, call}},
		{"结果无对应调用", []map[string]any{{"type": "function_call_output", "call_id": "unknown", "output": "OK"}}},
		{"重复结果", []map[string]any{call, output, output}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
				Model: "gpt-test", Input: mustRawMessage(t, test.items),
			})
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), "call_id")
		})
	}

	t.Run("并行调用可按不同顺序返回", func(t *testing.T) {
		got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
			Model: "gpt-test", Input: mustRawMessage(t, []map[string]any{
				call,
				{"type": "function_call", "call_id": "call_2", "name": "lookup", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call_2", "output": "second"},
				output,
			}),
		})
		require.NoError(t, err)
		require.Len(t, got.Messages, 3)
		assert.Len(t, got.Messages[0].ParseToolCalls(), 2)
		assert.Equal(t, "call_2", got.Messages[1].ToolCallId)
		assert.Equal(t, "call_1", got.Messages[2].ToolCallId)
	})
}

// TestResponsesRequestToChatFunctionStrict 验证工具 strict 缺省、true、显式 false 的出站 JSON 契约。
func TestResponsesRequestToChatFunctionStrict(t *testing.T) {
	for _, test := range []struct {
		name    string
		strict  any
		present bool
		wantErr bool
	}{
		{"省略", nil, false, false},
		{"显式true", true, true, false},
		{"显式false", false, true, false},
		{"错误类型", "false", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object"}}
			if test.present {
				tool["strict"] = test.strict
			}
			got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
				Model: "gpt-test", Input: mustRawMessage(t, "hello"), Tools: mustRawMessage(t, []map[string]any{tool}),
			})
			if test.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "strict")
				return
			}
			require.NoError(t, err)
			raw, err := common.Marshal(got)
			require.NoError(t, err)
			strict := gjson.GetBytes(raw, "tools.0.function.strict")
			assert.Equal(t, test.present, strict.Exists())
			if test.present {
				assert.Equal(t, test.strict, strict.Bool())
			}
		})
	}
}

// TestResponsesChatFunctionHistoryToClaudeAndGemini 保护经过 Chat 公共格式后二次转换的工具调用、参数与结果关联。
func TestResponsesChatFunctionHistoryToClaudeAndGemini(t *testing.T) {
	chat, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "model-test",
		Input: mustRawMessage(t, []map[string]any{
			{"role": "user", "content": "lookup"},
			{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": `{"q":"value"}`},
			{"type": "function_call_output", "call_id": "call_1", "output": `{"ok":true}`},
		}),
	})
	require.NoError(t, err)
	claude, err := oaichat.OpenAIChatRequestToClaudeMessages(nil, *chat)
	require.NoError(t, err)
	rawClaude, err := common.Marshal(claude)
	require.NoError(t, err)
	assert.Equal(t, "call_1", gjson.GetBytes(rawClaude, `messages.1.content.#(type=="tool_use").id`).String())
	assert.Equal(t, "value", gjson.GetBytes(rawClaude, `messages.1.content.#(type=="tool_use").input.q`).String())
	assert.Equal(t, "call_1", gjson.GetBytes(rawClaude, "messages.2.content.0.tool_use_id").String())
	assert.JSONEq(t, `{"ok":true}`, gjson.GetBytes(rawClaude, "messages.2.content.0.content").String())

	gemini, err := oaichat.OpenAIChatRequestToGeminiGenerateContent(nil, *chat, nil)
	require.NoError(t, err)
	rawGemini, err := common.Marshal(gemini)
	require.NoError(t, err)
	assert.Equal(t, "lookup", gjson.GetBytes(rawGemini, "contents.1.parts.0.functionCall.name").String())
	assert.Equal(t, "value", gjson.GetBytes(rawGemini, "contents.1.parts.0.functionCall.args.q").String())
	assert.Equal(t, "lookup", gjson.GetBytes(rawGemini, "contents.2.parts.0.functionResponse.name").String())
	assert.True(t, gjson.GetBytes(rawGemini, "contents.2.parts.0.functionResponse.response.ok").Bool())
}

// mustRawMessage 将测试输入编码为项目 DTO 使用的 JSON，编码失败时终止当前测试。
func mustRawMessage(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}
