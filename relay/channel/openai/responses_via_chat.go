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
)

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var chatResp dto.OpenAITextResponse
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responseID := helper.GetResponseID(c); responseID != "" {
		chatResp.Id = responseID
	}
	convertResult, err := service.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, &chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responsesResp, ok := convertResult.Value.(*dto.OpenAIResponsesResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI responses response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	usage := convertResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = relayconvert.UsageFromChatUsage(usage)
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

// OaiChatToResponsesStreamHandler 将 Chat SSE 转为 Responses，保留结束原因之后的用量尾包。
// 开流后的失败通过协议错误事件与 StreamStatus 记录，并返回已产生用量与 nil 错误，
// 避免调用层重新请求上游、整笔退款或向 SSE 追加 JSON；初始化失败仍返回外层错误。
func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseID := helper.GetResponseID(c)
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{
		ID:                 responseID,
		Model:              info.UpstreamModelName,
		EmitSequenceNumber: true,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	// finishSeen 与网络 EOF 分开记录；读到结束原因后继续接收最终用量。
	finishSeen := false
	// writeFailed 防止向已断开的下游补发错误事件。
	writeFailed := false
	// streamErr 使用普通 error，避免将带类型的 nil 传入 StreamResult.Stop。
	var streamErr error

	// sendEvents 统一发送普通、失败与最终事件，并保留实际的写入错误。
	sendEvents := func(results []relayconvert.ResponseResult) error {
		for _, result := range results {
			if err := c.Request.Context().Err(); err != nil {
				return err
			}
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok {
				return fmt.Errorf("expected OAI responses stream event, got %T", result.Value)
			}
			if event.Type == "response.incomplete" {
				message := "upstream Chat response incomplete"
				if response := event.Payload.Response; response != nil && response.IncompleteDetails != nil {
					message += ": " + response.IncompleteDetails.Reason
				}
				info.StreamStatus.RecordError(message)
				logger.LogError(c, message)
			}
			data, err := common.Marshal(event.Payload)
			if err != nil {
				return fmt.Errorf("failed to marshal Responses stream event: %w", err)
			}
			helper.ExtendWriteDeadline(c)
			if err := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, string(data)); err != nil {
				writeFailed = true
				return fmt.Errorf("failed to write Responses stream event: %w", err)
			}
		}
		return nil
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		// 同时读取 Chat 内容与动态错误，支持缺少 type 但含有 message/code 的错误包。
		var chunk struct {
			dto.ChatCompletionsStreamResponse
			Error any `json:"error"`
		}
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			streamErr = fmt.Errorf("failed to unmarshal chat stream response: %w", err)
			sr.Stop(streamErr)
			return
		}

		// 在转换或下游写入之前保存已发生的用量，后续失败不能清空结算依据。
		if chunk.Usage != nil {
			state.SetUsage(dto.MergeUsageNonZero(state.Usage(), relayconvert.UsageFromChatUsage(chunk.Usage)))
		}
		if oaiError := dto.GetOpenAIError(chunk.Error); oaiError != nil {
			streamErr = fmt.Errorf("upstream Chat stream error (type=%s, code=%v): %s", oaiError.Type, oaiError.Code, oaiError.Message)
			sr.Stop(streamErr)
			return
		}

		results, err := service.ConvertStreamResponseChunk(c, info, state, &chunk.ChatCompletionsStreamResponse)
		if err != nil {
			streamErr = fmt.Errorf("failed to convert chat stream response: %w", err)
			sr.Stop(streamErr)
			return
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
				finishSeen = true
			}
		}
		if streamErr = sendEvents(results); streamErr != nil {
			sr.Stop(streamErr)
		}
	})

	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	state.SetUsage(usage)

	if writeFailed || c.Request.Context().Err() != nil {
		return usage, nil
	}
	if streamErr == nil && (!info.StreamStatus.IsNormalEnd() || info.StreamStatus.HasErrors()) {
		streamErr = fmt.Errorf("upstream Chat stream ended abnormally (%s)", info.StreamStatus.Summary())
		info.StreamStatus.RecordError(streamErr.Error())
	}
	if streamErr == nil && !finishSeen {
		streamErr = fmt.Errorf("upstream Chat stream ended without finish_reason (%s)", info.StreamStatus.EndReason)
		info.StreamStatus.RecordError(streamErr.Error())
	}

	var finalResults []relayconvert.ResponseResult
	if streamErr == nil {
		finalResults, streamErr = service.FinalizeStreamResponse(c, info, state)
		if streamErr != nil {
			info.StreamStatus.RecordError(streamErr.Error())
		}
	}
	if streamErr != nil {
		logger.LogError(c, streamErr.Error())
		// Scanner 已退出，错误终态仅生成一次，并与最终事件使用同一序号分配器。
		finalResults, _ = state.FailResponsesStream("server_error", streamErr.Error(), "")
	}
	if err := sendEvents(finalResults); err != nil {
		info.StreamStatus.RecordError(err.Error())
		logger.LogError(c, err.Error())
	}

	return usage, nil
}
