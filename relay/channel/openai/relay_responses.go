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

// OaiResponsesHandler 原样转发非流式 Responses，并将输入、输出、缓存及推理 token 映射到既有结算用量。
// 无法读取或解析上游响应时返回协议错误，不发送不完整的 JSON 响应。
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
	if responsesResponse.Usage != nil {
		// Responses 与 Chat 的输出明细字段名不同，映射时不能再次累加已包含在输出总量中的推理 token。
		if details := gjson.GetBytes(responseBody, "usage.output_tokens_details"); details.Exists() && details.Type != gjson.Null {
			if err := common.UnmarshalJsonStr(details.Raw, &responsesResponse.Usage.CompletionTokenDetails); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
		}
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

// OaiResponsesStreamHandler 原样转发原生 Responses 事件，提取用量并在协议终态主动结束上游连接。
// 开流后的失败通过 SSE 和 StreamStatus 传达，返回已有用量及 nil 错误，避免重复请求、整笔退款或追加普通 JSON。
func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	usage := &dto.Usage{}
	// 输入和输出分别记录是否有有效计数；终态显式零值同样是有效计数，不能被后续估算覆盖。
	inputUsageKnown, outputUsageKnown := false, false
	var responseTextBuilder strings.Builder
	// 协议终态独立于网络 EOF；写失败后禁止继续补发流内错误。
	terminalSeen, writeFailed := false, false
	var streamErr error
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	imageCommitted := false

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
		case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
			isTerminal = true
		}

		// 只解析计费字段；未知事件的 delta、content 等扩展字段不参与共享 DTO 的反序列化。
		if rawUsage := event.Get("response.usage"); rawUsage.Exists() && rawUsage.Type != gjson.Null {
			var incoming dto.Usage
			if !rawUsage.IsObject() {
				sr.Error(fmt.Errorf("upstream Responses usage must be an object"))
			} else if err := common.UnmarshalJsonStr(rawUsage.Raw, &incoming); err != nil {
				sr.Error(fmt.Errorf("invalid Responses usage: %w", err))
			} else {
				inputReported := rawUsage.Get("input_tokens").Type == gjson.Number
				outputReported := rawUsage.Get("output_tokens").Type == gjson.Number
				inputZero := isTerminal && inputReported && incoming.InputTokens == 0
				outputZero := isTerminal && outputReported && incoming.OutputTokens == 0
				if incoming.InputTokens < 0 || incoming.OutputTokens < 0 {
					sr.Error(fmt.Errorf("upstream Responses token counts must not be negative"))
				}
				// 保留 rc35 的用量来源、语义、成本及计费快照；随后显式处理终态零值，不能被非零合并吞掉。
				if incoming.InputTokens >= 0 && incoming.OutputTokens >= 0 {
					usage = dto.MergeUsageNonZero(usage, relayconvert.NormalizeResponsesUsage(&incoming))
				}
				if inputReported && incoming.InputTokens >= 0 && (incoming.InputTokens > 0 || isTerminal) {
					usage.PromptTokens = incoming.InputTokens
					usage.InputTokens = incoming.InputTokens
					inputUsageKnown = true
				}
				if outputReported && incoming.OutputTokens >= 0 && (incoming.OutputTokens > 0 || isTerminal) {
					usage.CompletionTokens = incoming.OutputTokens
					usage.OutputTokens = incoming.OutputTokens
					outputUsageKnown = true
				}
				if inputZero {
					usage.PromptTokensDetails = dto.InputTokenDetails{}
				} else if incoming.InputTokensDetails != nil {
					usage.PromptTokensDetails = relayconvert.NormalizeResponsesUsage(&incoming).PromptTokensDetails
				}
				if outputZero {
					usage.CompletionTokenDetails = dto.OutputTokenDetails{}
				} else if details := rawUsage.Get("output_tokens_details"); details.Exists() && details.Type != gjson.Null {
					var outputDetails dto.OutputTokenDetails
					if err := common.UnmarshalJsonStr(details.Raw, &outputDetails); err != nil {
						sr.Error(fmt.Errorf("invalid Responses output token details: %w", err))
					} else {
						usage.CompletionTokenDetails = outputDetails
					}
				}
			}
		}

		var terminalErr error
		if isTerminal {
			terminalSeen = true
			failed := eventType.Str != "response.completed" && eventType.Str != "response.done"
			switch event.Get("response.status").Str {
			case "failed", "incomplete", "cancelled", "canceled":
				failed = true
			}
			if failed {
				message := ""
				for _, path := range []string{"response.error.message", "error.message", "message", "response.incomplete_details.reason"} {
					if message = event.Get(path).String(); message != "" {
						break
					}
				}
				terminalErr = fmt.Errorf("upstream Responses %s: %s", eventType.Str, message)
			}
			if !imageCommitted {
				if failed {
					imageCounter.Reset()
				} else {
					for i, output := range event.Get("response.output").Array() {
						if output.Get("type").Str != dto.ResponsesOutputTypeImageGenerationCall {
							continue
						}
						var item dto.ResponsesOutput
						if err := common.UnmarshalJsonStr(output.Raw, &item); err != nil {
							sr.Error(fmt.Errorf("invalid Responses image output: %w", err))
						} else {
							imageCounter.Observe(&item, &i)
						}
					}
				}
				imageCounter.Commit(info)
				imageCommitted = true
			}
		} else if eventType.Str == "response.output_text.delta" {
			if delta := event.Get("delta"); delta.Type == gjson.String {
				responseTextBuilder.WriteString(delta.Str)
			}
		} else if eventType.Str == dto.ResponsesOutputTypeItemDone {
			switch itemType := event.Get("item.type").Str; itemType {
			case dto.BuildInCallWebSearchCall, dto.BuildInCallFileSearchCall, dto.BuildInCallFunctionCall:
				info.CountBillableToolCall(itemType, event.Get("item.name").Str)
			case dto.ResponsesOutputTypeImageGenerationCall:
				var item dto.ResponsesOutput
				if err := common.UnmarshalJsonStr(event.Get("item").Raw, &item); err != nil {
					sr.Error(fmt.Errorf("invalid Responses image output: %w", err))
				} else if !imageCommitted {
					var outputIndex *int
					if index := event.Get("output_index"); index.Type == gjson.Number {
						value := int(index.Int())
						outputIndex = &value
					}
					imageCounter.Observe(&item, outputIndex)
				}
			}
		}

		if err := writeNativeResponsesEvent(c, eventType.Str, data); err != nil {
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

	if !terminalSeen && !writeFailed && c.Request.Context().Err() == nil {
		if streamErr == nil {
			streamErr = fmt.Errorf("upstream Responses stream ended without a terminal event (%s)", info.StreamStatus.EndReason)
			info.StreamStatus.RecordError(streamErr.Error())
		}
		logger.LogError(c, streamErr.Error())
		// Scanner 已退出，补发单个流内错误不会与 ping 并发写入。
		failure, err := common.Marshal(map[string]string{
			"type": "error", "code": "upstream_stream_error", "message": streamErr.Error(),
		})
		if err == nil {
			helper.ExtendWriteDeadline(c)
			err = writeNativeResponsesEvent(c, "error", string(failure))
		}
		if err != nil {
			info.StreamStatus.RecordError(err.Error())
			logger.LogError(c, "failed to send Responses stream error: "+err.Error())
		}
	}
	if !outputUsageKnown && responseTextBuilder.Len() > 0 {
		usage.CompletionTokens = service.CountTextToken(responseTextBuilder.String(), info.UpstreamModelName)
	}
	if !inputUsageKnown && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if usage.BillingUsage != nil {
		usage.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(usage.BillingUsage, usage.CompletionTokens)
	}

	return usage, nil
}

// writeNativeResponsesEvent 一次写入完整的原生 SSE 帧，并返回写入、短写或刷新错误。
// eventType 必须是已校验的不含换行的事件名称；data 保留原 JSON，仅沿用现有回车转义。
func writeNativeResponsesEvent(c *gin.Context, eventType, data string) error {
	if err := c.Request.Context().Err(); err != nil {
		return fmt.Errorf("Responses client context ended: %w", err)
	}
	frame := []byte("event: " + eventType + "\ndata: " + strings.ReplaceAll(data, "\r", "\\r") + "\n\n")
	written, err := c.Writer.Write(frame)
	if err != nil {
		return fmt.Errorf("Responses stream write failed: %w", err)
	}
	if written != len(frame) {
		return fmt.Errorf("Responses stream write failed: %w", io.ErrShortWrite)
	}
	return helper.FlushWriter(c)
}
