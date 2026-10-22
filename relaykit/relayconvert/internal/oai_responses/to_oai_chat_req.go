package oairesponses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const (
	responsesInputTypeFunctionCall       = "function_call"
	responsesInputTypeFunctionCallOutput = "function_call_output"
	responsesInputTypeCustomToolCall     = "custom_tool_call"
	responsesInputTypeCustomToolOutput   = "custom_tool_call_output"
)

const (
	ResponsesInputTypeFunctionCall       = responsesInputTypeFunctionCall
	ResponsesInputTypeFunctionCallOutput = responsesInputTypeFunctionCallOutput
	ResponsesInputTypeCustomToolCall     = responsesInputTypeCustomToolCall
	ResponsesInputTypeCustomToolOutput   = responsesInputTypeCustomToolOutput
)

// ResponsesRequestToChatCompletionsRequest 将无状态 Responses 请求转换为 Chat 请求。
// 不可表达的输入项目、密文及无效工具结果关联返回错误；不修改原请求，不替上游执行或补全工具调用。
func ResponsesRequestToChatCompletionsRequest(ctx context.Context, req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if err := validateResponsesRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}

	messages, err := responsesRequestMessagesToChat(ctx, req)
	if err != nil {
		return nil, err
	}

	tools, err := responsesRequestToolsToChat(req.Tools)
	if err != nil {
		return nil, err
	}

	toolChoice, err := responsesRequestToolChoiceToChat(req.ToolChoice)
	if err != nil {
		return nil, err
	}

	responseFormat, err := responsesRequestTextToChatResponseFormat(req.Text)
	if err != nil {
		return nil, err
	}

	out := &dto.GeneralOpenAIRequest{
		Model:                req.Model,
		Messages:             messages,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		MaxCompletionTokens:  req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		TopLogProbs:          req.TopLogProbs,
		ResponseFormat:       responseFormat,
		Tools:                tools,
		ToolChoice:           toolChoice,
		User:                 req.User,
		Store:                req.Store,
		Metadata:             req.Metadata,
		SafetyIdentifier:     req.SafetyIdentifier,
		PromptCacheRetention: req.PromptCacheRetention,
		EnableThinking:       req.EnableThinking,
		ThinkingBudget:       req.ThinkingBudget,
	}

	out.FrequencyPenalty, err = responsesRawFloat(req.FrequencyPenalty)
	if err != nil {
		return nil, fmt.Errorf("invalid frequency_penalty: %w", err)
	}
	out.PresencePenalty, err = responsesRawFloat(req.PresencePenalty)
	if err != nil {
		return nil, fmt.Errorf("invalid presence_penalty: %w", err)
	}

	reasoningIntent, diagnostics, err := reasoning.FromOpenAIResponses(req)
	if err != nil {
		return nil, reasoning.AsClientError(err)
	}
	convdiag.Add(ctx, diagnostics...)
	if err := reasoning.ApplyToOpenAIChat(out, reasoningIntent); err != nil {
		return nil, reasoning.AsClientError(err)
	}
	if req.ServiceTier != "" {
		out.ServiceTier, _ = kitutil.Marshal(req.ServiceTier)
	}
	if len(req.ParallelToolCalls) > 0 && kitutil.GetJsonType(req.ParallelToolCalls) == "boolean" {
		var parallelToolCalls bool
		if err := kitutil.Unmarshal(req.ParallelToolCalls, &parallelToolCalls); err == nil {
			out.ParallelTooCalls = &parallelToolCalls
		}
	}
	if len(req.PromptCacheKey) > 0 && kitutil.GetJsonType(req.PromptCacheKey) == "string" {
		var promptCacheKey string
		if err := kitutil.Unmarshal(req.PromptCacheKey, &promptCacheKey); err == nil {
			out.PromptCacheKey = promptCacheKey
		}
	}

	return out, nil
}

func validateResponsesRequestChatUnsupportedFields(req *dto.OpenAIResponsesRequest) error {
	unsupported := make([]string, 0, 4)
	if rawJSONPresent(req.Conversation) {
		unsupported = append(unsupported, "conversation")
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		unsupported = append(unsupported, "previous_response_id")
	}
	if rawJSONPresent(req.Prompt) {
		unsupported = append(unsupported, "prompt")
	}
	if rawJSONPresent(req.ContextManagement) {
		unsupported = append(unsupported, "context_management")
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("responses to chat conversion does not support stateful fields: %s", strings.Join(unsupported, ", "))
	}
	return nil
}

func ValidateRequestChatUnsupportedFields(req *dto.OpenAIResponsesRequest) error {
	return validateResponsesRequestChatUnsupportedFields(req)
}

// responsesRequestMessagesToChat 转换指令与输入历史，并在返回前校验当前请求内的工具调用结果关联。
func responsesRequestMessagesToChat(ctx context.Context, req *dto.OpenAIResponsesRequest) ([]dto.Message, error) {
	messages := make([]dto.Message, 0)
	if rawJSONPresent(req.Instructions) {
		instructions, err := responsesJSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			messages = append(messages, dto.Message{Role: "system", Content: instructions})
		}
	}

	if !rawJSONPresent(req.Input) {
		return messages, nil
	}

	switch kitutil.GetJsonType(req.Input) {
	case "string":
		input, err := responsesJSONString(req.Input)
		if err != nil {
			return nil, fmt.Errorf("invalid input string: %w", err)
		}
		messages = append(messages, dto.Message{Role: "user", Content: input})
		return messages, nil
	case "array":
		var items []map[string]any
		if err := kitutil.Unmarshal(req.Input, &items); err != nil {
			return nil, fmt.Errorf("invalid input array: %w", err)
		}
		// Chat Completions requires the tool messages answering one assistant tool_calls
		// batch to stay contiguous, so media hoisted out of tool output items is
		// held back and emitted as a single user message once the batch ends.
		var pendingMedia []any
		for _, item := range items {
			itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
			if len(pendingMedia) > 0 && itemType != responsesInputTypeFunctionCallOutput && itemType != responsesInputTypeCustomToolOutput {
				messages = append(messages, dto.Message{Role: "user", Content: pendingMedia})
				pendingMedia = nil
			}
			nextMessages, media, err := responsesInputItemToChatMessages(ctx, item, messages)
			if err != nil {
				return nil, err
			}
			messages = nextMessages
			pendingMedia = append(pendingMedia, media...)
		}
		if len(pendingMedia) > 0 {
			messages = append(messages, dto.Message{Role: "user", Content: pendingMedia})
		}
		if err := validateResponsesChatToolHistory(messages); err != nil {
			return nil, err
		}
		return messages, nil
	default:
		return nil, fmt.Errorf("unsupported responses input type %q", kitutil.GetJsonType(req.Input))
	}
}

// responsesInputItemToChatMessages appends the Chat messages for one Responses input item.
// The second result carries media content parts hoisted out of a tool output item,
// already in Chat shape; the caller decides where that user message lands.
func responsesInputItemToChatMessages(ctx context.Context, item map[string]any, messages []dto.Message) ([]dto.Message, []any, error) {
	itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
	if item["encrypted_content"] != nil {
		return nil, nil, fmt.Errorf("responses to chat conversion does not support encrypted_content in input item %q", itemType)
	}
	switch itemType {
	case "reasoning":
		if item["content"] != nil {
			return nil, nil, errors.New("responses to chat conversion does not support reasoning content outside summary")
		}
		summary, ok := item["summary"].([]any)
		if !ok {
			return nil, nil, errors.New("reasoning summary must be an array of summary_text items")
		}
		texts := make([]string, 0, len(summary))
		for _, rawPart := range summary {
			part, ok := rawPart.(map[string]any)
			if !ok || part["type"] != "summary_text" {
				return nil, nil, errors.New("reasoning summary supports only summary_text items")
			}
			text, ok := part["text"].(string)
			if !ok {
				return nil, nil, errors.New("reasoning summary_text must contain string text")
			}
			texts = append(texts, text)
		}
		if len(messages) == 0 || messages[len(messages)-1].Role != "assistant" {
			messages = append(messages, dto.Message{Role: "assistant"})
		}
		// 摘要与该轮随后出现的文本及工具调用属于同一条 assistant 消息。
		last := &messages[len(messages)-1]
		summaryText := strings.Join(texts, "\n\n")
		if previous := last.GetReasoningContent(); previous != "" {
			summaryText = previous + "\n\n" + summaryText
		}
		last.ReasoningContent = &summaryText
		return messages, nil, nil
	case responsesInputTypeFunctionCall:
		toolCall, err := responsesFunctionCallItemToChatToolCall(item)
		if err != nil {
			return nil, nil, err
		}
		return appendToolCallToLastAssistant(messages, toolCall), nil, nil
	case responsesInputTypeCustomToolCall:
		toolCall, err := responsesCustomToolCallItemToChatToolCall(item)
		if err != nil {
			return nil, nil, err
		}
		return appendToolCallToLastAssistant(messages, toolCall), nil, nil
	case responsesInputTypeFunctionCallOutput, responsesInputTypeCustomToolOutput:
		callID := strings.TrimSpace(kitutil.Interface2String(item["call_id"]))
		if callID == "" {
			return nil, nil, fmt.Errorf("%s item is missing call_id", itemType)
		}
		if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" && len(messages[len(messages)-1].ParseToolCalls()) == 0 {
			return nil, nil, fmt.Errorf("%s call_id has no matching earlier call", itemType)
		}
		content, media := responsesToolOutputToChat(ctx, item["output"])
		return append(messages, dto.Message{Role: "tool", ToolCallId: callID, Content: content}), media, nil
	case "", "message":
		// 简写消息可省略 type；其他项目不能作为普通消息丢失协议语义。
	default:
		return nil, nil, fmt.Errorf("responses to chat conversion does not support input item type %q", itemType)
	}

	role := strings.TrimSpace(kitutil.Interface2String(item["role"]))
	if role == "" {
		role = "user"
	}
	content, err := responsesInputContentToChatContent(ctx, item["content"])
	if err != nil {
		return nil, nil, err
	}
	if role == "assistant" && len(messages) > 0 {
		last := &messages[len(messages)-1]
		if last.Role == "assistant" && last.ReasoningContent != nil && last.Content == nil {
			last.Content = content
			return messages, nil, nil
		}
	}
	return append(messages, dto.Message{Role: role, Content: content}), nil, nil
}

// validateResponsesChatToolHistory 校验调用标识唯一且结果对应已出现的调用；允许并行结果乱序与末尾尚未返回的调用。
func validateResponsesChatToolHistory(messages []dto.Message) error {
	// answered 记录已出现的调用及其是否已有结果，仅在当前请求历史内使用。
	answered := make(map[string]bool)
	for _, message := range messages {
		for _, call := range message.ParseToolCalls() {
			if _, exists := answered[call.ID]; exists {
				return errors.New("responses input contains duplicate function call_id")
			}
			answered[call.ID] = false
		}
		if message.Role != "tool" {
			continue
		}
		alreadyAnswered, exists := answered[message.ToolCallId]
		if !exists {
			return errors.New("function_call_output call_id has no matching earlier call")
		}
		if alreadyAnswered {
			return errors.New("responses input contains duplicate function_call_output call_id")
		}
		answered[message.ToolCallId] = true
	}
	return nil
}

func responsesInputContentToChatContent(ctx context.Context, content any) (any, error) {
	if content == nil {
		return "", nil
	}

	switch value := content.(type) {
	case string:
		return value, nil
	case []any:
		return responsesContentPartsToChatContent(ctx, value)
	case []map[string]any:
		parts := make([]any, 0, len(value))
		for _, part := range value {
			parts = append(parts, part)
		}
		return responsesContentPartsToChatContent(ctx, parts)
	default:
		return content, nil
	}
}

func responsesContentPartsToChatContent(ctx context.Context, parts []any) (any, error) {
	chatParts := make([]any, 0, len(parts))
	var textOnly strings.Builder
	onlyText := true
	droppedFile := false

	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			onlyText = false
			chatParts = append(chatParts, rawPart)
			continue
		}

		partType := strings.TrimSpace(kitutil.Interface2String(part["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			text := kitutil.Interface2String(part["text"])
			textOnly.WriteString(text)
			chatParts = append(chatParts, map[string]any{
				"type": dto.ContentTypeText,
				"text": text,
			})
		case "input_image":
			onlyText = false
			chatParts = append(chatParts, map[string]any{
				"type":      dto.ContentTypeImageURL,
				"image_url": responsesImagePartToChatImageURL(part),
			})
		case "input_file":
			onlyText = false
			chatPart, reason := responsesFilePartToChat(ctx, part)
			if reason != "" {
				convdiag.Add(ctx, types.ConversionDiagnostic{
					Code:     "unsupported_media_type",
					Path:     "input.content",
					Message:  reason + "; the file was omitted",
					Severity: types.ConversionDiagnosticError,
				})
				droppedFile = true
				continue
			}
			chatParts = append(chatParts, chatPart)
		case "input_audio":
			onlyText = false
			chatParts = append(chatParts, map[string]any{
				"type":        dto.ContentTypeInputAudio,
				"input_audio": responsesPartPayload(part, "input_audio"),
			})
		case "input_video":
			onlyText = false
			chatParts = append(chatParts, map[string]any{
				"type":      dto.ContentTypeVideoUrl,
				"video_url": responsesVideoPartToChatVideoURL(part),
			})
		default:
			onlyText = false
			chatParts = append(chatParts, part)
		}
	}

	if onlyText {
		return textOnly.String(), nil
	}
	if len(chatParts) == 0 && droppedFile {
		// Chat rejects an empty content array; name what was omitted instead.
		return "[file]", nil
	}
	return chatParts, nil
}

// responsesFunctionCallItemToChatToolCall 将函数历史转换为 Chat 工具调用；call_id 与 name 不能为空，项目 id 不能替代 call_id。
func responsesFunctionCallItemToChatToolCall(item map[string]any) (dto.ToolCallRequest, error) {
	name := responsesCallName(item)
	if name == "" {
		return dto.ToolCallRequest{}, errors.New("function_call item is missing name")
	}
	callID := strings.TrimSpace(kitutil.Interface2String(item["call_id"]))
	if callID == "" {
		return dto.ToolCallRequest{}, errors.New("function_call item is missing call_id")
	}
	return dto.ToolCallRequest{
		ID:   callID,
		Type: "function",
		Function: dto.FunctionRequest{
			Name:      name,
			Arguments: responsesArgumentsString(item["arguments"]),
		},
	}, nil
}

// responsesCustomToolCallItemToChatToolCall encodes a custom tool call the same
// way its definition reaches Chat Completions: a function call whose arguments
// carry the raw input under the single "input" key.
func responsesCustomToolCallItemToChatToolCall(item map[string]any) (dto.ToolCallRequest, error) {
	name := responsesCallName(item)
	if name == "" {
		return dto.ToolCallRequest{}, errors.New("custom_tool_call item is missing name")
	}
	arguments, err := kitutil.Marshal(map[string]string{
		convmeta.CustomToolInputArgument: responsesArgumentsString(item["input"]),
	})
	if err != nil {
		return dto.ToolCallRequest{}, err
	}
	return dto.ToolCallRequest{
		ID:   responsesCallID(item),
		Type: "function",
		Function: dto.FunctionRequest{
			Name:      name,
			Arguments: string(arguments),
		},
	}, nil
}

func appendToolCallToLastAssistant(messages []dto.Message, toolCall dto.ToolCallRequest) []dto.Message {
	if len(messages) == 0 || messages[len(messages)-1].Role != "assistant" {
		messages = append(messages, dto.Message{Role: "assistant"})
	}

	idx := len(messages) - 1
	toolCalls := messages[idx].ParseToolCalls()
	toolCalls = append(toolCalls, toolCall)
	toolCallsRaw, _ := kitutil.Marshal(toolCalls)
	messages[idx].ToolCalls = toolCallsRaw
	return messages
}

// responsesRequestToolsToChat 转换工具定义，保留函数 strict 的缺省与显式布尔值；类型错误返回错误。
func responsesRequestToolsToChat(raw json.RawMessage) ([]dto.ToolCallRequest, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}

	var tools []map[string]any
	if err := kitutil.Unmarshal(raw, &tools); err != nil {
		return nil, fmt.Errorf("invalid tools: %w", err)
	}

	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		toolType := strings.TrimSpace(kitutil.Interface2String(tool["type"]))
		if toolType == "function" {
			var strict *bool
			if value, exists := tool["strict"]; exists && value != nil {
				boolean, ok := value.(bool)
				if !ok {
					return nil, errors.New("function tool strict must be a boolean")
				}
				strict = &boolean
			}
			out = append(out, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        strings.TrimSpace(kitutil.Interface2String(tool["name"])),
					Description: kitutil.Interface2String(tool["description"]),
					Parameters:  tool["parameters"],
					Strict:      strict,
				},
			})
			continue
		}

		rawTool, err := kitutil.Marshal(tool)
		if err != nil {
			return nil, err
		}
		out = append(out, dto.ToolCallRequest{
			Type:   toolType,
			Custom: rawTool,
		})
	}
	return out, nil
}

func responsesRequestToolChoiceToChat(raw json.RawMessage) (any, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}
	if kitutil.GetJsonType(raw) == "string" {
		var choice string
		if err := kitutil.Unmarshal(raw, &choice); err != nil {
			return nil, fmt.Errorf("invalid tool_choice: %w", err)
		}
		return choice, nil
	}

	var choice map[string]any
	if err := kitutil.Unmarshal(raw, &choice); err != nil {
		return nil, fmt.Errorf("invalid tool_choice: %w", err)
	}
	if kitutil.Interface2String(choice["type"]) == "function" {
		name := strings.TrimSpace(kitutil.Interface2String(choice["name"]))
		if name != "" {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}, nil
		}
	}
	return choice, nil
}

func RequestToolChoiceToChat(raw json.RawMessage) (any, error) {
	return responsesRequestToolChoiceToChat(raw)
}

func responsesRequestTextToChatResponseFormat(raw json.RawMessage) (*dto.ResponseFormat, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}

	var textConfig map[string]any
	if err := kitutil.Unmarshal(raw, &textConfig); err != nil {
		return nil, fmt.Errorf("invalid text config: %w", err)
	}
	format, ok := textConfig["format"].(map[string]any)
	if !ok {
		return nil, nil
	}

	formatType := strings.TrimSpace(kitutil.Interface2String(format["type"]))
	if formatType == "" {
		return nil, nil
	}

	out := &dto.ResponseFormat{Type: formatType}
	if formatType == "json_schema" {
		schemaRaw, err := kitutil.Marshal(format)
		if err != nil {
			return nil, err
		}
		out.JsonSchema = schemaRaw
	}
	return out, nil
}

func RequestTextToChatResponseFormat(raw json.RawMessage) (*dto.ResponseFormat, error) {
	return responsesRequestTextToChatResponseFormat(raw)
}

// responsesImagePartToChatImageURL 将图片地址规范为 Chat 图片对象，保留同层 detail 且不修改输入对象。
func responsesImagePartToChatImageURL(part map[string]any) any {
	if imageURL, ok := part["image_url"]; ok {
		imageObject := make(map[string]any)
		switch value := imageURL.(type) {
		case string:
			imageObject["url"] = value
		case map[string]any:
			for key, entry := range value {
				imageObject[key] = entry
			}
		default:
			return imageURL
		}
		if detail, exists := part["detail"]; exists {
			imageObject["detail"] = detail
		}
		return imageObject
	}
	imageURL := map[string]any{}
	for _, key := range []string{"url", "file_id", "detail"} {
		if value, ok := part[key]; ok {
			imageURL[key] = value
		}
	}
	if len(imageURL) == 0 {
		return part
	}
	return imageURL
}

// responsesFilePartToChat maps a Responses input_file onto Chat Completions,
// whose file part takes only PDF data or an uploaded file_id and no file URL.
// Files given by data or URL are loaded through the media resolver: PDF
// becomes a Chat file part with inline data and text files become a text
// part. A non-empty reason means Chat cannot carry the file.
func responsesFilePartToChat(ctx context.Context, part map[string]any) (map[string]any, string) {
	fields := part
	if nested, ok := part["file"].(map[string]any); ok {
		fields = nested
	}
	filename := strings.TrimSpace(kitutil.Interface2String(fields["filename"]))
	source := ContentPartToFileSource(part)
	if source == nil {
		fileID := strings.TrimSpace(kitutil.Interface2String(fields["file_id"]))
		if fileID == "" {
			return nil, "input_file carries no file_data, file_url, or file_id"
		}
		file := map[string]any{"file_id": fileID}
		if filename != "" {
			file["filename"] = filename
		}
		return map[string]any{"type": dto.ContentTypeFile, "file": file}, ""
	}
	base64Data, mimeType, err := relaymedia.ResolveBase64Data(ctx, source, "formatting Responses input file for Chat")
	if err != nil {
		return nil, fmt.Sprintf("input_file could not be loaded: %s", err.Error())
	}
	mediaType, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mimeType)), ";")
	mediaType = strings.TrimSpace(mediaType)
	switch {
	case mediaType == "application/pdf":
		if filename == "" {
			filename = "document.pdf"
		}
		return map[string]any{"type": dto.ContentTypeFile, "file": map[string]any{
			"filename":  filename,
			"file_data": "data:application/pdf;base64," + base64Data,
		}}, ""
	case relaymedia.IsTextMimeType(mediaType):
		text, ok := relaymedia.DecodeText(base64Data)
		if !ok {
			return nil, fmt.Sprintf("input_file of MIME type %q could not be decoded as UTF-8 text", mimeType)
		}
		return map[string]any{"type": dto.ContentTypeText, "text": text}, ""
	}
	return nil, fmt.Sprintf("OpenAI Chat Completions file parts accept only PDF, not %q", mimeType)
}

func responsesVideoPartToChatVideoURL(part map[string]any) any {
	if videoURL, ok := part["video_url"]; ok {
		if videoURLMap, ok := videoURL.(map[string]any); ok {
			if url := kitutil.Interface2String(videoURLMap["url"]); url != "" {
				return url
			}
		}
		return videoURL
	}
	if url := kitutil.Interface2String(part["url"]); url != "" {
		return url
	}
	return responsesPartPayload(part, "video_url")
}

func responsesPartPayload(part map[string]any, key string) any {
	if value, ok := part[key]; ok {
		return value
	}
	payload := make(map[string]any, len(part))
	for k, value := range part {
		if k == "type" {
			continue
		}
		payload[k] = value
	}
	return payload
}

func responsesCallID(item map[string]any) string {
	callID := strings.TrimSpace(kitutil.Interface2String(item["call_id"]))
	if callID != "" {
		return callID
	}
	return strings.TrimSpace(kitutil.Interface2String(item["id"]))
}

func CallID(item map[string]any) string {
	return responsesCallID(item)
}

// responsesCallName is the upstream function name of a function_call or
// custom_tool_call item: a call made in a tool namespace uses the name its
// flattened definition was sent with.
func responsesCallName(item map[string]any) string {
	name := strings.TrimSpace(kitutil.Interface2String(item["name"]))
	if name == "" {
		return ""
	}
	return convmeta.NamespacedToolName(strings.TrimSpace(kitutil.Interface2String(item["namespace"])), name)
}

func responsesArgumentsString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		raw, err := kitutil.Marshal(v)
		if err != nil {
			return kitutil.Interface2String(v)
		}
		return string(raw)
	}
}

func responseToolOutputToChatContent(value any) any {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		raw, err := kitutil.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(raw)
	}
}

// responsesToolOutputToChat maps a function_call_output payload onto Chat Completions, where a
// tool message may only carry text. A Responses content-part array keeps its text on the tool
// message and returns the media parts in Chat shape for the caller to hoist into a user message;
// stringifying them instead would hand base64 image data to the upstream text tokenizer. Any
// other payload shape (string, object, plain JSON array) keeps the historical stringified form.
func responsesToolOutputToChat(ctx context.Context, value any) (any, []any) {
	rawParts, ok := value.([]any)
	if !ok || len(rawParts) == 0 {
		return responseToolOutputToChatContent(value), nil
	}

	texts := make([]string, 0, len(rawParts))
	mediaParts := make([]any, 0, len(rawParts))
	labels := make([]string, 0, len(rawParts))
	for _, rawPart := range rawParts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			return responseToolOutputToChatContent(value), nil
		}
		partType := strings.TrimSpace(kitutil.Interface2String(part["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			if text := kitutil.Interface2String(part["text"]); text != "" {
				texts = append(texts, text)
			}
		case "input_image", "input_file", "input_audio", "input_video":
			mediaParts = append(mediaParts, part)
			label := "[" + strings.TrimPrefix(partType, "input_") + "]"
			if !slices.Contains(labels, label) {
				labels = append(labels, label)
			}
		default:
			return responseToolOutputToChatContent(value), nil
		}
	}
	if len(mediaParts) == 0 {
		return strings.Join(texts, "\n"), nil
	}

	converted, err := responsesContentPartsToChatContent(ctx, mediaParts)
	if err != nil {
		return responseToolOutputToChatContent(value), nil
	}
	media, _ := converted.([]any)
	if len(texts) == 0 {
		// Upstreams reject empty tool content; the media itself rides on the hoisted user message.
		return strings.Join(labels, " "), media
	}
	return strings.Join(texts, "\n"), media
}

func responsesRawFloat(raw json.RawMessage) (*float64, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}
	var value float64
	if err := kitutil.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func responsesJSONString(raw json.RawMessage) (string, error) {
	if kitutil.GetJsonType(raw) != "string" {
		return string(raw), nil
	}
	var value string
	if err := kitutil.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func rawJSONPresent(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	return kitutil.GetJsonType(raw) != "null"
}

func JSONString(raw json.RawMessage) (string, error) {
	return responsesJSONString(raw)
}

func RawJSONPresent(raw json.RawMessage) bool {
	return rawJSONPresent(raw)
}
