package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

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

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		usage.CompletionTokenDetails = responsesResponse.Usage.CompletionTokenDetails
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.CacheWriteTokens = responsesResponse.Usage.InputTokensDetails.CacheWriteTokens
		}
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
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
				if inputReported && incoming.InputTokens >= 0 && (incoming.InputTokens > 0 || isTerminal) {
					usage.PromptTokens = incoming.InputTokens
					inputUsageKnown = true
				}
				if outputReported && incoming.OutputTokens >= 0 && (incoming.OutputTokens > 0 || isTerminal) {
					usage.CompletionTokens = incoming.OutputTokens
					outputUsageKnown = true
				}
				if inputZero {
					usage.PromptTokensDetails = dto.InputTokenDetails{}
				} else if incoming.InputTokensDetails != nil {
					usage.PromptTokensDetails.CachedTokens = incoming.InputTokensDetails.CachedTokens
					usage.PromptTokensDetails.CacheWriteTokens = incoming.InputTokensDetails.CacheWriteTokens
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
			} else {
				// 图片调用的既有计费标记依赖终态输出中的类型、质量和尺寸。
				for _, output := range event.Get("response.output").Array() {
					if output.Get("type").Str == dto.ResponsesOutputTypeImageGenerationCall {
						c.Set("image_generation_call", true)
						c.Set("image_generation_call_quality", output.Get("quality").String())
						c.Set("image_generation_call_size", output.Get("size").String())
						break
					}
				}
			}
		} else if eventType.Str == "response.output_text.delta" {
			if delta := event.Get("delta"); delta.Type == gjson.String {
				responseTextBuilder.WriteString(delta.Str)
			}
		} else if eventType.Str == dto.ResponsesOutputTypeItemDone && event.Get("item.type").Str == dto.BuildInCallWebSearchCall {
			if info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
				if webSearchTool := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; webSearchTool != nil {
					webSearchTool.CallCount++
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
