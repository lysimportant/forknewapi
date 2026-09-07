package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := relayconvert.NormalizeResponsesUsage(responsesResponse.Usage)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return usage, nil
}

// OaiResponsesStreamHandler 转发原生 Responses 事件，并提取结算所需的用量与工具调用。
// 已开流的协议或传输失败通过 StreamStatus 记录，返回已有用量与 nil 错误，
// 让调用层按实际消耗结算，避免重新请求上游、整笔退款或向 SSE 追加 JSON。
func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	// 输入、输出分别判断是否已有实际计数；中间零值与空 usage 不阻止缺失项估算。
	usage := &dto.Usage{}
	inputUsageKnown, outputUsageKnown := false, false
	// 终态显式零值覆盖历史快照，并同步清除对应的缓存、推理及计费明细。
	terminalInputZero, terminalOutputZero := false, false
	var responseTextBuilder strings.Builder
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	// terminalSeen 与网络 EOF 分开记录；上游 EOF 不代表 Responses 已完成。
	terminalSeen := false
	// writeFailed 防止下游已经不可写时再次补发错误事件。
	writeFailed := false
	// streamErr 保存本地协议解析失败，供 scanner 退出后输出单个错误事件。
	var streamErr error

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if !gjson.Valid(data) {
			streamErr = fmt.Errorf("invalid JSON in upstream Responses stream")
			sr.Stop(streamErr)
			return
		}
		event := gjson.Parse(data)
		eventType := event.Get("type")
		if eventType.Type != gjson.String || eventType.Str == "" || strings.ContainsAny(eventType.Str, "\r\n") {
			streamErr = fmt.Errorf("missing or invalid type in upstream Responses stream event")
			sr.Stop(streamErr)
			return
		}
		isTerminal := false
		switch eventType.Str {
		case "response.completed", "response.done", "error", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			isTerminal = true
		}

		// 只解析用量，避免扩展工具的 delta、result 或 content 类型阻断原始事件。
		if rawUsage := event.Get("response.usage"); rawUsage.Exists() && rawUsage.Type != gjson.Null {
			var incomingUsage dto.Usage
			if err := common.UnmarshalJsonStr(rawUsage.Raw, &incomingUsage); err != nil {
				sr.Error(fmt.Errorf("invalid Responses usage: %w", err))
			} else {
				// Responses 使用 output_tokens_details，共享 DTO 的字段名属于 Chat Completions。
				if details := rawUsage.Get("output_tokens_details"); details.Exists() && details.Type != gjson.Null {
					if err := common.UnmarshalJsonStr(details.Raw, &incomingUsage.CompletionTokenDetails); err != nil {
						sr.Error(fmt.Errorf("invalid Responses output token details: %w", err))
					}
				}
				usage = dto.MergeUsageNonZero(usage, relayconvert.NormalizeResponsesUsage(&incomingUsage))
				if rawUsage.Get("input_tokens").Type == gjson.Number && incomingUsage.InputTokens >= 0 {
					inputUsageKnown = inputUsageKnown || incomingUsage.InputTokens > 0 || isTerminal
					terminalInputZero = isTerminal && incomingUsage.InputTokens == 0
				}
				if rawUsage.Get("output_tokens").Type == gjson.Number && incomingUsage.OutputTokens >= 0 {
					outputUsageKnown = outputUsageKnown || incomingUsage.OutputTokens > 0 || isTerminal
					terminalOutputZero = isTerminal && incomingUsage.OutputTokens == 0
				}
			}
		}

		// 先记录上游已经产生的文本与工具消耗，下游写失败不能抹掉这些计费依据。
		responseStatus := event.Get("response.status")
		var failedTerminal bool
		switch eventType.Str {
		case "response.completed", "response.done":
			terminalSeen = true
			failedTerminal = relaycommon.IsNonBillableResponsesStatus([]byte(responseStatus.Raw))
			if !failedTerminal {
				for index, output := range event.Get("response.output").Array() {
					observeResponsesImageOutput(imageCounter, output, &index)
				}
				imageCounter.Commit(info)
			}
		case "error", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			terminalSeen = true
			failedTerminal = true
		case "response.output_text.delta":
			if delta := event.Get("delta"); delta.Type == gjson.String {
				responseTextBuilder.WriteString(delta.Str)
			}
		case dto.ResponsesOutputTypeItemDone:
			item := event.Get("item")
			switch item.Get("type").Str {
			case dto.BuildInCallWebSearchCall, dto.BuildInCallFileSearchCall, dto.BuildInCallFunctionCall:
				info.CountBillableToolCall(item.Get("type").Str, item.Get("name").Str)
			case dto.ResponsesOutputTypeImageGenerationCall:
				var outputIndex *int
				if rawIndex := event.Get("output_index"); rawIndex.Type == gjson.Number {
					var index int
					if err := common.UnmarshalJsonStr(rawIndex.Raw, &index); err == nil {
						outputIndex = &index
					}
				}
				observeResponsesImageOutput(imageCounter, item, outputIndex)
			}
		}
		var terminalErr error
		if failedTerminal {
			imageCounter.Reset()
			imageCounter.Commit(info)
			var message, code string
			for _, path := range []string{"response.error.message", "error.message", "message", "response.incomplete_details.reason"} {
				if message = event.Get(path).String(); message != "" {
					break
				}
			}
			for _, path := range []string{"response.error.code", "error.code", "code"} {
				if code = event.Get(path).String(); code != "" {
					break
				}
			}
			terminalErr = fmt.Errorf("upstream Responses %s (status=%s, code=%s): %s", eventType.Str, responseStatus.String(), code, message)
		}
		if err := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType.Str}, data); err != nil {
			writeFailed = true
			sr.Stop(err)
			return
		}
		if terminalErr != nil {
			sr.Stop(terminalErr)
		} else if terminalSeen {
			sr.Done()
		}
	})

	if !terminalSeen {
		imageCounter.Reset()
		imageCounter.Commit(info)
		if !writeFailed && c.Request.Context().Err() == nil {
			if streamErr == nil {
				streamErr = fmt.Errorf("upstream Responses stream ended without a terminal event (%s)", info.StreamStatus.EndReason)
				info.StreamStatus.RecordError(streamErr.Error())
			}
			logger.LogError(c, streamErr.Error())
			// Scanner 已退出，不再与 ping 并发写；以单个流内错误结束，保留部分用量结算。
			failure := dto.ResponsesStreamResponse{Type: "error", Code: "upstream_stream_error", Message: streamErr.Error()}
			data, err := common.Marshal(failure)
			if err == nil {
				helper.ExtendWriteDeadline(c)
				err = helper.ResponseChunkData(c, failure, string(data))
			}
			if err != nil {
				info.StreamStatus.RecordError(err.Error())
				logger.LogError(c, "failed to send Responses stream error: "+err.Error())
			}
		}
	}

	if terminalInputZero || terminalOutputZero {
		applyResponsesTerminalZeroUsage(usage, terminalInputZero, terminalOutputZero)
	}
	// 原始计费快照也可能携带顶层缺失的真实计数，优先采用它而不是生成估算。
	if canonical, ok := usage.BillingUsage.CanonicalUsage(); ok {
		if !inputUsageKnown {
			inputTokens := max(canonical.InputTokens, canonical.PromptTokens)
			if inputTokens > 0 {
				usage.PromptTokens, usage.InputTokens = inputTokens, inputTokens
				inputUsageKnown = true
			}
		}
		if !outputUsageKnown && canonical.CompletionTokens > 0 {
			usage.CompletionTokens, usage.OutputTokens = canonical.CompletionTokens, canonical.CompletionTokens
			outputUsageKnown = true
		}
	}
	if !outputUsageKnown {
		if text := responseTextBuilder.String(); text != "" {
			usage.CompletionTokens = service.CountTextToken(text, info.UpstreamModelName)
			usage.OutputTokens = usage.CompletionTokens
		}
		usage.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(usage.BillingUsage, usage.CompletionTokens)
	}
	if !inputUsageKnown && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
		usage.InputTokens = usage.PromptTokens
		// CanonicalUsage 优先于顶层用量结算，输入估算也必须写入原始方言的快照。
		if snapshot := dto.CloneBillingUsage(usage.BillingUsage); snapshot != nil {
			switch {
			case snapshot.OpenAIUsage != nil:
				snapshot.OpenAIUsage.PromptTokens = usage.PromptTokens
				snapshot.OpenAIUsage.InputTokens = usage.PromptTokens
				snapshot.OpenAIUsage.TotalTokens = usage.PromptTokens + max(snapshot.OpenAIUsage.CompletionTokens, snapshot.OpenAIUsage.OutputTokens)
			case snapshot.ClaudeUsage != nil:
				snapshot.ClaudeUsage.InputTokens = usage.PromptTokens
			case snapshot.GeminiUsageMetadata != nil:
				snapshot.GeminiUsageMetadata.PromptTokenCount = usage.PromptTokens
				snapshot.GeminiUsageMetadata.TotalTokenCount = usage.PromptTokens + snapshot.GeminiUsageMetadata.CandidatesTokenCount + snapshot.GeminiUsageMetadata.ThoughtsTokenCount
			}
			snapshot.Estimated = true
			usage.BillingUsage = snapshot
		}
	}
	if total := usage.PromptTokens + usage.CompletionTokens; total > usage.TotalTokens {
		usage.TotalTokens = total
	}
	return usage, nil
}

// applyResponsesTerminalZeroUsage 将 Responses 终态的显式零计数应用到用量及原始计费快照。
// 只清除对应输入或输出侧，保留另一侧实际计数和原始供应商语义；全零时移除过期快照。
func applyResponsesTerminalZeroUsage(usage *dto.Usage, inputZero, outputZero bool) {
	usage.BillingUsage = dto.CloneBillingUsage(usage.BillingUsage)
	openAIUsages := []*dto.Usage{usage}
	if usage.BillingUsage != nil && usage.BillingUsage.OpenAIUsage != nil {
		openAIUsages = append(openAIUsages, usage.BillingUsage.OpenAIUsage)
	}
	for _, current := range openAIUsages {
		if inputZero {
			current.PromptTokens, current.InputTokens, current.PromptCacheHitTokens = 0, 0, 0
			current.PromptTokensDetails = dto.InputTokenDetails{}
			current.InputTokensDetails = nil
			current.ClaudeCacheCreation5mTokens, current.ClaudeCacheCreation1hTokens = 0, 0
		}
		if outputZero {
			current.CompletionTokens, current.OutputTokens = 0, 0
			current.CompletionTokenDetails = dto.OutputTokenDetails{}
		}
		current.TotalTokens = max(current.PromptTokens, current.InputTokens) + max(current.CompletionTokens, current.OutputTokens)
	}
	if inputZero && outputZero {
		usage.BillingUsage = nil
		return
	}
	if usage.BillingUsage == nil {
		return
	}
	if current := usage.BillingUsage.ClaudeUsage; current != nil {
		if inputZero {
			current.InputTokens, current.CacheReadInputTokens, current.CacheCreationInputTokens = 0, 0, 0
			current.ClaudeCacheCreation5mTokens, current.ClaudeCacheCreation1hTokens = 0, 0
			current.CacheCreation = nil
		}
		if outputZero {
			current.OutputTokens = 0
		}
	}
	if current := usage.BillingUsage.GeminiUsageMetadata; current != nil {
		if inputZero {
			current.PromptTokenCount, current.ToolUsePromptTokenCount, current.CachedContentTokenCount = 0, 0, 0
			current.PromptTokensDetails, current.ToolUsePromptTokensDetails = nil, nil
		}
		if outputZero {
			current.CandidatesTokenCount, current.ThoughtsTokenCount = 0, 0
			current.CandidatesTokensDetails = nil
		}
		current.TotalTokenCount = current.PromptTokenCount + current.ToolUsePromptTokenCount + current.CandidatesTokenCount + current.ThoughtsTokenCount
	}
}

// observeResponsesImageOutput 仅提取图片计费依赖的字段；其他工具及扩展输出保持原样转发。
// outputIndex 可为空；存在时参与现有图片去重，不改变图片调用的计费上限。
func observeResponsesImageOutput(counter *relaycommon.ImageGenerationCallCounter, item gjson.Result, outputIndex *int) {
	if item.Get("type").Str != dto.ResponsesOutputTypeImageGenerationCall || item.Get("result").Type != gjson.String {
		return
	}
	counter.Observe(&dto.ResponsesOutput{
		Type: dto.ResponsesOutputTypeImageGenerationCall, ID: item.Get("id").Str,
		CallId: item.Get("call_id").Str, Status: item.Get("status").Str, Result: item.Get("result").Str,
	}, outputIndex)
}
