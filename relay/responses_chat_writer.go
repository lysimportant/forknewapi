package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// responsesChatWriter 同步将渠道已经标准化的 Chat 输出转换为 Responses；只缓存一个 SSE 事件及最终响应所需状态。
// ResponseWriter 是客户端真实输出，Header/状态另行保存在包装器中，防止原渠道提前发送 Chat 头部。
type responsesChatWriter struct {
	gin.ResponseWriter
	client *gin.Context
	info   *relaycommon.RelayInfo
	header http.Header
	status int
	size   int
	// stream 固定客户端所选协议，避免 SDK 临时切换 RelayInfo.IsStream 影响最终响应格式。
	stream bool

	// mu 串行化渠道数据、保活和最终结算输出，不额外启动扫描器或 goroutine。
	mu sync.Mutex
	// pending 仅保存尚未结束的一行（非流式时保存完整 JSON）；eventData 保存当前 SSE 的 data 行。
	pending     []byte
	eventData   []byte
	comment     bool
	state       *relayconvert.ResponseStreamState
	sequence    int
	finished    bool
	finishSeen  bool
	doneSeen    bool
	started     bool
	writeFailed bool
	err         error
	// jsonMode 支持客户端要求 SSE、但渠道 SDK 一次性输出标准 Chat JSON 的情况。
	jsonMode bool
	chatSeen bool
	// toolIDs/toolNames 为分片提供身份信息的工具保留稳定客户端身份。
	toolIDs   map[int]string
	toolNames map[int]string
	// observedUsage 保留标准 Chat 已报告的用量；即使为零也不能替换成估算。
	observedUsage *dto.Usage
	// usageText 仅用于供应商异常返回 nil 用量时估算已生成的文本、推理和工具参数。
	usageText strings.Builder
	// items 保存当前输出项的身份和完成数据，用于补齐旧转换器遗漏的 content_part 与 done 字段。
	items map[string]map[string]any
}

// newResponsesChatWriter 建立 Responses 编码状态；参数缺失或转换器不可用时在请求上游前返回错误。
func newResponsesChatWriter(c *gin.Context, info *relaycommon.RelayInfo) (*responsesChatWriter, *types.NewAPIError) {
	if c == nil || c.Writer == nil || c.Request == nil || info == nil || info.ChannelMeta == nil {
		return nil, types.NewErrorWithStatusCode(errors.New("invalid Responses Chat writer context"), types.ErrorCodeBadResponse, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	w := &responsesChatWriter{ResponseWriter: c.Writer, client: c, info: info, header: c.Writer.Header().Clone(), status: http.StatusOK, size: -1, stream: info.IsStream, items: make(map[string]map[string]any), toolIDs: make(map[int]string), toolNames: make(map[int]string)}
	if info.IsStream {
		state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{ID: helper.GetResponseID(c), Model: info.UpstreamModelName})
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}
		w.state = state
	}
	return w, nil
}

// Header 返回渠道输出的临时头部，转换完成前不污染客户端响应。
func (w *responsesChatWriter) Header() http.Header { return w.header }

// WriteHeader 记录渠道状态，不提前提交原始协议的头部。
func (w *responsesChatWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size < 0 && code > 0 {
		w.status = code
	}
}

// WriteHeaderNow 仅标记渠道开始输出；真实客户端头部由 Responses 编码控制。
func (w *responsesChatWriter) WriteHeaderNow() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size < 0 {
		w.size = 0
	}
}

// Status 返回渠道声明的 HTTP 状态。
func (w *responsesChatWriter) Status() int { w.mu.Lock(); defer w.mu.Unlock(); return w.status }

// Size 返回渠道已写入的 Chat 字节数，未开始时为 -1。
func (w *responsesChatWriter) Size() int { w.mu.Lock(); defer w.mu.Unlock(); return w.size }

// Written 表示渠道是否已经提交 Chat 输出，并不表示最终 Responses 已完成。
func (w *responsesChatWriter) Written() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.size >= 0 }

// Unwrap 允许 http.ResponseController 继续设置真实客户端连接的写入期限。
func (w *responsesChatWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteString 与 Write 使用同一解析入口，避免供应商绕过协议转换。
func (w *responsesChatWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// Write 接收任意分片的标准 Chat JSON 或 SSE；流式缓存按现有扫描器上限约束，不缓存整个事件流。
func (w *responsesChatWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return 0, io.ErrClosedPipe
	}
	if w.err != nil {
		return 0, w.err
	}
	if err := w.client.Request.Context().Err(); err != nil {
		w.err = err
		return 0, err
	}
	if w.size < 0 {
		w.size = 0
	}
	w.size += len(data)
	if w.stream && !w.chatSeen && len(w.eventData) == 0 && len(bytes.TrimSpace(w.pending)) == 0 && bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		w.jsonMode = true
	}
	if !w.stream || w.jsonMode {
		if len(w.pending)+len(data) > helper.DefaultMaxScannerBufferSize {
			w.err = errors.New("Chat JSON response exceeds response buffer limit")
			return 0, w.err
		}
		w.pending = append(w.pending, data...)
		return len(data), nil
	}
	remaining := data
	for len(remaining) > 0 {
		index := bytes.IndexByte(remaining, '\n')
		if index < 0 {
			index = len(remaining)
		}
		if len(w.pending)+len(w.eventData)+index > helper.DefaultMaxScannerBufferSize {
			w.err = errors.New("Chat SSE event exceeds response buffer limit")
			return 0, w.err
		}
		w.pending = append(w.pending, remaining[:index]...)
		if index == len(remaining) {
			break
		}
		line := strings.TrimSuffix(string(w.pending), "\r")
		w.pending = w.pending[:0]
		remaining = remaining[index+1:]
		if err := w.consumeLine(line); err != nil {
			w.err = err
			return 0, err
		}
	}
	return len(data), nil
}

// Flush 只刷新已经发出的 Responses，避免供应商 Flush 提前提交尚未校验的 JSON。
func (w *responsesChatWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started && !w.writeFailed && w.client.Request.Context().Err() == nil {
		if err := helper.FlushWriter(w.client); err != nil {
			w.err = err
			w.writeFailed = true
		}
	}
}

// consumeLine 按 SSE 帧处理多行 data 和 CRLF；注释仅作为保活，其他控制字段不进入 Chat JSON。
func (w *responsesChatWriter) consumeLine(line string) error {
	if line == "" {
		if len(w.eventData) != 0 {
			data := strings.TrimSuffix(string(w.eventData), "\n")
			w.eventData = w.eventData[:0]
			w.comment = false
			return w.consumeChatChunk(data)
		}
		if w.comment && !w.doneSeen {
			w.comment = false
			if !w.started {
				return nil
			}
			return w.writeClient([]byte(": PING\n\n"), true)
		}
		return nil
	}
	if strings.HasPrefix(line, ":") {
		w.comment = true
		return nil
	}
	field, value, _ := strings.Cut(line, ":")
	value = strings.TrimPrefix(value, " ")
	switch field {
	case "data":
		w.eventData = append(w.eventData, value...)
		w.eventData = append(w.eventData, '\n')
	case "event", "id", "retry":
	default:
		return fmt.Errorf("invalid Chat SSE field %q", field)
	}
	return nil
}

// consumeChatChunk 校验标准 Chat 数据后调用公共转换器；不把 EOF 或 [DONE] 当作 finish_reason。
func (w *responsesChatWriter) consumeChatChunk(data string) error {
	if data == "[DONE]" {
		w.doneSeen = true
		return nil
	}
	if w.doneSeen {
		return errors.New("Chat stream contains data after [DONE]")
	}
	w.chatSeen = true
	var chunk struct {
		dto.ChatCompletionsStreamResponse
		Error any `json:"error"`
	}
	if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
		return fmt.Errorf("invalid Chat stream JSON: %w", err)
	}
	if chunk.Usage != nil && (gjson.Get(data, "usage.prompt_tokens").Exists() || gjson.Get(data, "usage.completion_tokens").Exists() || gjson.Get(data, "usage.total_tokens").Exists()) {
		w.observedUsage = chunk.Usage
	}
	if chunk.Error != nil {
		return fmt.Errorf("upstream Chat stream error: %v", dto.GetOpenAIError(chunk.Error))
	}
	if chunk.Object != "" && chunk.Object != "chat.completion.chunk" {
		return fmt.Errorf("unsupported Chat stream object %q", chunk.Object)
	}
	if gjson.Get(data, "type").Exists() {
		return errors.New("expected Chat chunk, received another event protocol")
	}
	if len(chunk.Choices) == 0 && chunk.Usage == nil {
		return errors.New("Chat stream chunk contains neither choices nor usage")
	}
	if len(chunk.Choices) > 1 {
		return errors.New("Responses Chat conversion requires one completion choice")
	}
	for choiceIndex := range chunk.Choices {
		choice := &chunk.Choices[choiceIndex]
		if choice.Index != 0 {
			return errors.New("Responses Chat conversion requires completion choice index 0")
		}
		if w.finishSeen && (choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" || len(choice.Delta.ToolCalls) != 0) {
			return errors.New("Chat stream contains content after finish_reason")
		}
		w.usageText.WriteString(choice.Delta.GetContentString())
		w.usageText.WriteString(choice.Delta.GetReasoningContent())
		for toolIndex := range choice.Delta.ToolCalls {
			tool := &choice.Delta.ToolCalls[toolIndex]
			if tool.Type != nil && common.Interface2String(tool.Type) != "" && common.Interface2String(tool.Type) != "function" {
				return fmt.Errorf("unsupported Chat tool type %v", tool.Type)
			}
			w.usageText.WriteString(tool.Function.Arguments)
			index := 0
			if tool.Index != nil {
				index = *tool.Index
			}
			if index < 0 {
				return errors.New("Chat tool index cannot be negative")
			}
			if w.toolIDs[index] == "" {
				w.toolIDs[index] = tool.ID
				if tool.ID == "" {
					w.toolIDs[index] = fmt.Sprintf("%s_call_%d", helper.GetResponseID(w.client), index)
				}
			}
			tool.ID = w.toolIDs[index]
			if tool.Function.Name != "" && tool.Function.Name != w.toolNames[index] {
				w.toolNames[index] += tool.Function.Name
			}
			tool.Function.Name = w.toolNames[index]
		}
		if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
			// Cohere 等旧渠道把长度终态写成 max_tokens；在公共 Chat 边界规范化为 length。
			if *choice.FinishReason == "max_tokens" {
				reason := "length"
				choice.FinishReason = &reason
			}
			switch *choice.FinishReason {
			case "stop", "tool_calls", "length", "content_filter":
			default:
				return fmt.Errorf("unsupported Chat finish_reason %q", *choice.FinishReason)
			}
			w.finishSeen = true
			if *choice.FinishReason == "tool_calls" && len(w.toolIDs) == 0 {
				return errors.New("Chat tool_calls finish_reason contains no tool calls")
			}
			for _, name := range w.toolNames {
				if name == "" {
					return errors.New("Chat function tool is missing its name")
				}
			}
		}
	}
	var unknownDelta string
	gjson.Get(data, "choices.0.delta").ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "content", "reasoning", "reasoning_content", "role", "tool_calls":
		default:
			if value.Type != gjson.Null {
				unknownDelta = key.String()
				return false
			}
		}
		return true
	})
	if unknownDelta != "" {
		return fmt.Errorf("unsupported Chat delta field %q", unknownDelta)
	}
	results, err := relayconvert.ConvertStreamResponseChunk(w.client, w.info, w.state, &chunk.ChatCompletionsStreamResponse)
	if err != nil {
		return err
	}
	return w.sendResults(results)
}

// sendResults 补齐旧转换器的 Responses 内容生命周期，并为每个客户端事件分配严格递增的序号。
func (w *responsesChatWriter) sendResults(results []relayconvert.ResponseResult) error {
	// 同一批的 item.done 包含完整内容，可补到先出现的 text/arguments.done 中。
	for _, result := range results {
		if event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent); ok && event.Payload.Item != nil {
			itemBytes, err := common.Marshal(event.Payload.Item)
			if err != nil {
				return err
			}
			var item map[string]any
			if err := common.Unmarshal(itemBytes, &item); err != nil {
				return err
			}
			if item["type"] == "reasoning" {
				item["summary"] = item["content"]
				delete(item, "content")
			}
			w.items[event.Payload.Item.ID] = item
		}
	}
	for _, result := range results {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			return fmt.Errorf("unexpected Responses converter result %T", result.Value)
		}
		raw, err := common.Marshal(event.Payload)
		if err != nil {
			return err
		}
		var payload map[string]any
		if err := common.Unmarshal(raw, &payload); err != nil {
			return err
		}
		if item, ok := payload["item"].(map[string]any); ok && item["type"] == "reasoning" {
			item["summary"] = item["content"]
			delete(item, "content")
		}
		switch event.Type {
		case "response.output_text.done":
			payload["text"] = ""
			if content, ok := w.items[event.Payload.ItemID]["content"].([]any); ok && len(content) > 0 {
				if part, ok := content[0].(map[string]any); ok {
					payload["text"] = part["text"]
				}
			}
		case "response.function_call_arguments.done":
			payload["arguments"] = w.items[event.Payload.ItemID]["arguments"]
			payload["name"] = w.items[event.Payload.ItemID]["name"]
		case "response.reasoning_summary_text.done":
			if event.Payload.Part != nil {
				payload["text"] = event.Payload.Part.Text
			}
			delete(payload, "part")
		}
		if response, ok := payload["response"].(map[string]any); ok {
			normalizeResponsesChatOutput(response)
		}
		if err := w.sendEvent(event.Type, payload); err != nil {
			return err
		}
		switch event.Type {
		case "response.created":
			if err := w.sendEvent("response.in_progress", map[string]any{"response": payload["response"]}); err != nil {
				return err
			}
		case "response.output_item.added":
			item := event.Payload.Item
			if item != nil && (item.Type == "message" || item.Type == "reasoning") {
				kind, indexKey := "response.content_part.added", "content_index"
				part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}}
				if item.Type == "reasoning" {
					kind, indexKey = "response.reasoning_summary_part.added", "summary_index"
					part = map[string]any{"type": "summary_text", "text": ""}
				}
				if err := w.sendEvent(kind, map[string]any{"item_id": item.ID, "output_index": payload["output_index"], indexKey: 0, "part": part}); err != nil {
					return err
				}
			}
		case "response.output_text.done", "response.reasoning_summary_text.done":
			kind, indexKey := "response.content_part.done", "content_index"
			part := map[string]any{"type": "output_text", "text": payload["text"], "annotations": []any{}}
			if event.Type == "response.reasoning_summary_text.done" {
				kind, indexKey = "response.reasoning_summary_part.done", "summary_index"
				part = map[string]any{"type": "summary_text", "text": payload["text"]}
			}
			if err := w.sendEvent(kind, map[string]any{"item_id": payload["item_id"], "output_index": payload["output_index"], indexKey: 0, "part": part}); err != nil {
				return err
			}
		case "response.incomplete":
			w.recordError("Chat response incomplete")
		}
	}
	return nil
}

// sendEvent 写出一个完整 SSE 帧，不向客户端发送 Chat 的 [DONE]。
func (w *responsesChatWriter) sendEvent(kind string, payload map[string]any) error {
	payload["type"], payload["sequence_number"] = kind, w.sequence
	w.sequence++
	data, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	return w.writeClient([]byte("event: "+kind+"\ndata: "+string(data)+"\n\n"), true)
}

// writeClient 首次写入时复制安全头部，完整转发一个 Responses 帧或最终 JSON，并捕获短写与刷新失败。
func (w *responsesChatWriter) writeClient(data []byte, stream bool) error {
	if err := w.client.Request.Context().Err(); err != nil {
		w.writeFailed = true
		return err
	}
	if !w.started {
		for key, values := range w.header {
			if !service.ShouldCopyUpstreamHeader(w.client, key, values) || strings.EqualFold(key, "Content-Type") || strings.EqualFold(key, "Content-Encoding") || strings.EqualFold(key, "Transfer-Encoding") {
				continue
			}
			w.ResponseWriter.Header()[key] = append([]string(nil), values...)
		}
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.Header().Del("Content-Encoding")
		w.ResponseWriter.Header().Set("Content-Type", "application/json")
		if stream {
			w.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
			w.ResponseWriter.Header().Set("Cache-Control", "no-cache")
			w.ResponseWriter.Header().Set("X-Accel-Buffering", "no")
		}
		w.ResponseWriter.WriteHeader(http.StatusOK)
		w.started = true
	}
	n, err := w.ResponseWriter.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = helper.FlushWriter(w.client)
	}
	if err != nil {
		w.writeFailed = true
	}
	return err
}

// recordError 保留渠道扫描器的状态对象，把转换及写入失败附加到既有消费日志上下文。
func (w *responsesChatWriter) recordError(message string) {
	if w.info.StreamStatus == nil {
		w.info.StreamStatus = relaycommon.NewStreamStatus()
	}
	w.info.StreamStatus.RecordError(message)
}

// normalizeResponsesChatOutput 将旧共享 DTO 编码为标准 Responses 摘要和用量，不向客户端暴露 Chat 字段或内部计费快照。
func normalizeResponsesChatOutput(response map[string]any) {
	if output, ok := response["output"].([]any); ok {
		for _, rawItem := range output {
			if item, ok := rawItem.(map[string]any); ok && item["type"] == "reasoning" {
				item["summary"] = item["content"]
				delete(item, "content")
			}
		}
	}
	if usage, ok := response["usage"].(map[string]any); ok {
		inputDetails := map[string]any{"cached_tokens": 0}
		if details, ok := usage["input_tokens_details"].(map[string]any); ok {
			if cached, exists := details["cached_tokens"]; exists {
				inputDetails["cached_tokens"] = cached
			}
			if written, exists := details["cache_write_tokens"]; exists {
				inputDetails["cache_write_tokens"] = written
			}
		}
		outputDetails := map[string]any{"reasoning_tokens": 0}
		details, ok := usage["output_tokens_details"].(map[string]any)
		if !ok {
			details, _ = usage["completion_tokens_details"].(map[string]any)
		}
		if reasoning, exists := details["reasoning_tokens"]; exists {
			outputDetails["reasoning_tokens"] = reasoning
		}
		response["usage"] = map[string]any{
			"input_tokens": usage["input_tokens"], "input_tokens_details": inputDetails,
			"output_tokens": usage["output_tokens"], "output_tokens_details": outputDetails,
			"total_tokens": usage["total_tokens"],
		}
	}
}

// responsesClientUsage 仅把 Anthropic 的非缓存输入语义转换为客户端总输入，保留原用量对象及计费快照。
func responsesClientUsage(usage *dto.Usage) *dto.Usage {
	if usage != nil && usage.UsageSemantic == dto.BillingUsageSemanticAnthropic {
		return relayconvert.UsageFromClaudeUsage(usage)
	}
	return usage
}

// UsageFallback 仅供原渠道返回 nil 用量时调用；优先已报告用量（含零），否则估算已生成内容，不执行扣费。
// 尚未输出任何内容时返回零用量，避免仅保活或请求前错误产生输入费用。
func (w *responsesChatWriter) UsageFallback() *dto.Usage {
	w.mu.Lock()
	defer w.mu.Unlock()
	text := w.usageText.String()
	if w.observedUsage == nil && len(w.pending) > 0 && (!w.stream || w.jsonMode) {
		var response dto.OpenAITextResponse
		if common.Unmarshal(w.pending, &response) == nil && response.Error == nil {
			if gjson.GetBytes(w.pending, "usage.prompt_tokens").Exists() || gjson.GetBytes(w.pending, "usage.completion_tokens").Exists() || gjson.GetBytes(w.pending, "usage.total_tokens").Exists() {
				w.observedUsage = &response.Usage
			}
			if len(response.Choices) == 1 {
				message := response.Choices[0].Message
				text += message.StringContent() + message.GetReasoningContent()
				for _, tool := range message.ParseToolCalls() {
					text += tool.Function.Arguments
				}
			}
		}
	}
	if w.observedUsage != nil {
		usage := *w.observedUsage
		usage.BillingUsage = dto.CloneBillingUsage(usage.BillingUsage)
		if usage.BillingUsage == nil {
			usage.BillingUsage = dto.NewOpenAIChatBillingUsage(&usage)
		}
		return &usage
	}
	if text == "" {
		return &dto.Usage{}
	}
	return service.ResponseText2Usage(w.client, text, w.info.UpstreamModelName, w.info.GetEstimatePromptTokens())
}

// convertBufferedChatResponse 校验一次性 Chat JSON，并依据客户端模式写入 JSON 或转为流状态；不修改原渠道用量。
func (w *responsesChatWriter) convertBufferedChatResponse(usage *dto.Usage) error {
	var response dto.OpenAITextResponse
	if err := common.Unmarshal(w.pending, &response); err != nil {
		return fmt.Errorf("invalid Chat JSON response: %w", err)
	}
	if gjson.GetBytes(w.pending, "usage.prompt_tokens").Exists() || gjson.GetBytes(w.pending, "usage.completion_tokens").Exists() || gjson.GetBytes(w.pending, "usage.total_tokens").Exists() {
		w.observedUsage = &response.Usage
	}
	if response.Error != nil {
		return fmt.Errorf("upstream Chat error: %v", dto.GetOpenAIError(response.Error))
	}
	if response.Object != "" && response.Object != "chat.completion" {
		return fmt.Errorf("unsupported Chat response object %q", response.Object)
	}
	if len(response.Choices) != 1 || response.Choices[0].Index != 0 {
		return errors.New("Responses Chat conversion requires one completion choice at index 0")
	}
	choice := response.Choices[0]
	if choice.FinishReason == "max_tokens" {
		choice.FinishReason = "length"
		response.Choices[0].FinishReason = "length"
	}
	switch choice.FinishReason {
	case "stop", "tool_calls", "length", "content_filter":
	default:
		return fmt.Errorf("unsupported Chat finish_reason %q", choice.FinishReason)
	}
	for _, field := range []string{"refusal", "audio", "function_call"} {
		if value := gjson.GetBytes(w.pending, "choices.0.message."+field); value.Exists() && value.Type != gjson.Null {
			return fmt.Errorf("unsupported Chat message field %q", field)
		}
	}
	if content := gjson.GetBytes(w.pending, "choices.0.message.content"); content.Exists() && content.Type != gjson.Null && content.Type != gjson.String {
		return errors.New("unsupported Chat message content type")
	}
	for _, tool := range choice.Message.ParseToolCalls() {
		if tool.Type != "" && tool.Type != "function" {
			return fmt.Errorf("unsupported Chat tool type %q", tool.Type)
		}
		if tool.Function.Name == "" {
			return errors.New("Chat function tool is missing its name")
		}
	}
	if choice.FinishReason == "tool_calls" && len(choice.Message.ParseToolCalls()) == 0 {
		return errors.New("Chat tool_calls finish_reason contains no tool calls")
	}
	if usage != nil {
		response.Usage = *responsesClientUsage(usage)
	}
	w.pending = nil
	if w.stream {
		text, reasoning := choice.Message.StringContent(), choice.Message.GetReasoningContent()
		chunk := dto.ChatCompletionsStreamResponse{Id: response.Id, Model: response.Model, Object: "chat.completion.chunk", Usage: &response.Usage, Choices: []dto.ChatCompletionsStreamResponseChoice{{Index: 0, FinishReason: &choice.FinishReason, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &text, ReasoningContent: &reasoning}}}}
		for index, tool := range choice.Message.ParseToolCalls() {
			chunk.Choices[0].Delta.ToolCalls = append(chunk.Choices[0].Delta.ToolCalls, dto.ToolCallResponse{Index: &index, ID: tool.ID, Type: "function", Function: dto.FunctionResponse{Name: tool.Function.Name, Arguments: tool.Function.Arguments}})
		}
		data, err := common.Marshal(chunk)
		if err != nil {
			return err
		}
		return w.consumeChatChunk(string(data))
	}
	converted, _, err := relayconvert.ChatCompletionsResponseToResponsesResponse(&response, helper.GetResponseID(w.client))
	if err != nil {
		return err
	}
	data, err := common.Marshal(converted)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return err
	}
	normalizeResponsesChatOutput(payload)
	data, err = common.Marshal(payload)
	if err != nil {
		return err
	}
	return w.writeClient(data, false)
}

// Finish 在渠道处理结束后生成唯一终态；使用原渠道用量，不估算、不结算、不重发请求。
// 已开流、写失败或客户端取消只记录流状态并返回 nil，防止调用层再次输出 JSON、退款或重试。
func (w *responsesChatWriter) Finish(usage *dto.Usage, upstreamErr *types.NewAPIError) *types.NewAPIError {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return nil
	}
	w.finished = true
	if upstreamErr != nil && w.err == nil {
		w.err = upstreamErr
	}
	if err := w.client.Request.Context().Err(); err != nil {
		w.recordError(err.Error())
		return nil
	}
	if w.writeFailed {
		if w.err != nil {
			w.recordError(w.err.Error())
		}
		return nil
	}
	if w.status < 200 || w.status >= 300 {
		if w.err == nil {
			w.err = fmt.Errorf("Chat adaptor returned HTTP %d", w.status)
		}
	}
	if w.err == nil && (!w.stream || w.jsonMode) {
		w.err = w.convertBufferedChatResponse(usage)
	}
	if w.stream {
		if w.err == nil && (len(bytes.TrimSpace(w.pending)) != 0 || len(w.eventData) != 0) {
			w.err = errors.New("Chat stream ended with an incomplete SSE frame")
		}
		if w.err == nil && !w.finishSeen {
			w.err = errors.New("Chat stream ended without finish_reason")
		}
		if status := w.info.StreamStatus; w.err == nil && status != nil && (status.HasErrors() || (status.EndReason != relaycommon.StreamEndReasonNone && !status.IsNormalEnd())) {
			w.err = fmt.Errorf("Chat stream ended abnormally: %s", status.Summary())
		}
		if w.err == nil {
			if usage != nil {
				w.state.SetUsage(responsesClientUsage(usage))
			}
			results, err := relayconvert.FinalizeStreamResponse(w.client, w.info, w.state)
			if err == nil {
				err = w.sendResults(results)
			}
			w.err = err
		}
	}
	if w.err == nil {
		return nil
	}
	w.recordError(w.err.Error())
	if w.writeFailed {
		return nil
	}
	if w.stream && (w.started || w.ResponseWriter.Written()) {
		if err := w.sendEvent("error", map[string]any{"code": "upstream_error", "message": w.err.Error(), "param": nil}); err != nil {
			w.recordError(err.Error())
		}
		return nil
	}
	if upstreamErr != nil {
		return upstreamErr
	}
	return types.NewErrorWithStatusCode(w.err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}
