package helper

import (
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitreasoning "github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/setting/model_setting"
	hostreasoning "github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func ResolveIncomingBillingExprRequestInput(c *gin.Context, info *relaycommon.RelayInfo) (billingexpr.RequestInput, error) {
	if info != nil && info.BillingRequestInput != nil {
		input := cloneRequestInput(*info.BillingRequestInput)
		merged := cloneStringMap(info.RequestHeaders)
		maps.Copy(merged, input.Headers)
		input.Headers = merged
		return input, nil
	}

	input := billingexpr.RequestInput{}
	if info != nil {
		input.Headers = cloneStringMap(info.RequestHeaders)
	}

	bodyBytes, err := readIncomingBillingExprBody(c)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	input.Effort = resolveBillingEffort(info, bodyBytes)
	return input, nil
}

// ResolveImageBillingRequestInput freezes only the validated scalar image
// parameters needed by pricing. Image files, prompts and base64 payloads are
// deliberately excluded, including for multipart edits.
func ResolveImageBillingRequestInput(c *gin.Context, info *relaycommon.RelayInfo, input billingexpr.RequestInput) (billingexpr.RequestInput, error) {
	request, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return input, nil
	}
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	if info.ChannelMeta != nil {
		channelType = info.ChannelType
	}
	count, err := request.ImageCount(channelType == constant.ChannelTypeAli)
	if err != nil {
		return input, err
	}
	topLevelCount, err := request.ImageCount(false)
	if err != nil {
		return input, err
	}
	body := map[string]any{"model": request.Model, "n": topLevelCount, "size": request.Size, "quality": request.Quality}
	if request.BillingParameters != nil {
		body["parameters"] = request.BillingParameters
	}
	encoded, err := common.Marshal(body)
	if err != nil {
		return input, err
	}
	input.Body = encoded
	input.ImageCount = &count
	return input, nil
}

func BuildBillingExprRequestInputFromRequest(request dto.Request, headers map[string]string) (billingexpr.RequestInput, error) {
	input := billingexpr.RequestInput{
		Headers: cloneStringMap(headers),
	}
	if request == nil {
		return input, nil
	}

	bodyBytes, err := common.Marshal(request)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	input.Effort = resolveBillingEffort(nil, bodyBytes)
	return input, nil
}

func readIncomingBillingExprBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil || !isJSONContentType(c.Request.Header.Get("Content-Type")) {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

func cloneRequestInput(src billingexpr.RequestInput) billingexpr.RequestInput {
	input := billingexpr.RequestInput{
		Headers: cloneStringMap(src.Headers),
		Effort:  src.Effort,
	}
	if src.ImageCount != nil {
		count := *src.ImageCount
		input.ImageCount = &count
	}
	if len(src.Body) > 0 {
		input.Body = append([]byte(nil), src.Body...)
	}
	return input
}

func isJSONContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "application/json")
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		if strings.TrimSpace(key) == "" {
			continue
		}
		dst[key] = value
	}
	return dst
}

// resolveBillingEffort 解析计费用推理档位。
// 模型名上的显式 @effort / 旧后缀优先于请求体，与转换层“后缀覆盖请求字段”一致。
// 不把 thinking:on 或 budget 推断成 high，避免未声明档位被误加价。
func resolveBillingEffort(info *relaycommon.RelayInfo, body []byte) string {
	modelName := ""
	if info != nil {
		modelName = strings.TrimSpace(info.GetOriginModelName())
	}
	if modelName == "" && len(body) > 0 {
		modelName = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	if effort := explicitEffortFromModelName(modelName); effort != "" {
		return effort
	}
	if info != nil {
		if effort := normalizeBillingEffortValue(info.GetReasoningEffort()); effort != "" {
			return effort
		}
	}
	return effortFromRequestBody(body)
}

func explicitEffortFromModelName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || model_setting.ShouldPreserveThinkingSuffix(modelName) {
		return ""
	}
	spec := kitreasoning.ParseModelModifiers(modelName)
	if model_setting.ShouldPreserveThinkingSuffix(spec.Base) {
		return ""
	}
	last := ""
	for _, modifier := range spec.Modifiers {
		if modifier.Key == "effort" {
			last = modifier.Value
		}
	}
	if effort, err := kitreasoning.ParseEffort(last); err == nil && effort != "" {
		return string(effort)
	}
	_, intent, found, err := hostreasoning.ParseLegacyModelSuffix(
		spec.Base,
		model_setting.GetClaudeSettings().ThinkingAdapterEnabled,
		model_setting.GetGeminiSettings().ThinkingAdapterEnabled,
	)
	if err != nil || !found {
		return ""
	}
	return string(intent.Effort)
}

func effortFromRequestBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	for _, path := range []string{
		"reasoning_effort",
		"reasoning.effort",
		"output_config.effort",
		"generationConfig.thinkingConfig.thinkingLevel",
		"generation_config.thinking_config.thinking_level",
	} {
		value := gjson.GetBytes(body, path)
		if value.Type != gjson.String {
			continue
		}
		if effort := normalizeBillingEffortValue(value.String()); effort != "" {
			return effort
		}
	}
	return ""
}

func normalizeBillingEffortValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if effort, err := kitreasoning.ParseEffort(raw); err == nil && effort != "" {
		return string(effort)
	}
	return strings.ToLower(raw)
}
