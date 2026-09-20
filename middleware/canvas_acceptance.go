package middleware

import (
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// canvasExecutionHeader 是 Canvas 冻结执行授权的唯一请求头名称。
const canvasExecutionHeader = "x-canvas-execution"

var errCanvasExecutionHeaderInvalid = errors.New("canvas execution header is invalid")

// canvasExecutionPayload 描述 Canvas 在提交时冻结的九项授权事实。
type canvasExecutionPayload struct {
	Version            int      `json:"version"`
	Issuer             string   `json:"issuer"`
	UserID             string   `json:"user_id"`
	InstanceID         string   `json:"instance_id"`
	GrantID            string   `json:"grant_id"`
	TokenID            string   `json:"token_id"`
	ExpectedGroup      string   `json:"expected_group"`
	PermissionRevision string   `json:"permission_revision"`
	AutoGroups         []string `json:"auto_groups"`
}

// validateCanvasExecutionAcceptance 在渠道和实际分组确定后、预扣与供应商请求前，
// 对 Canvas 管理 Token 复核单头冻结授权。非管理 Token 保持原有中继合同。
func validateCanvasExecutionAcceptance(c *gin.Context, channel *model.Channel, modelName string) bool {
	if c.Request.Method != http.MethodPost {
		return true
	}
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenID <= 0 {
		return true
	}
	payload, headerErr := parseCanvasExecutionHeader(c.Request.Header.Values(canvasExecutionHeader))
	if !canvasCreationPath(c.Request.URL.Path) && headerErr == nil {
		headerErr = errCanvasExecutionHeaderInvalid
	}
	actualGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if actualGroup == "auto" {
		actualGroup = common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	}
	channelID := 0
	if channel != nil {
		channelID = channel.Id
	}
	config, configErr := common.GetCanvasAccountConfig()
	if !common.CanvasAccountEnabled() || configErr != nil {
		config = common.CanvasAccountConfig{}
		if headerErr == nil {
			headerErr = errCanvasExecutionHeaderInvalid
		}
	}
	managed, _, err := model.ValidateCanvasAcceptance(model.CanvasAcceptanceInput{
		TokenID: tokenID,
		Issuer:  payload.Issuer, ExpectedIssuer: config.Issuer, ExpectedUserID: payload.UserID,
		GrantID: payload.GrantID, InstanceID: payload.InstanceID,
		ExpectedTokenID: payload.TokenID, ExpectedPolicyGroup: payload.ExpectedGroup,
		ExpectedPermissionRevision: payload.PermissionRevision, ExpectedAutoGroups: payload.AutoGroups,
		ActualGroup: actualGroup, Model: modelName, ChannelID: channelID,
	})
	if !managed {
		return true
	}
	if headerErr != nil {
		abortWithOpenAiMessage(c, http.StatusBadRequest, "Canvas 执行授权头无效", types.ErrorCode("canvas_execution_invalid"))
		return false
	}
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, model.ErrCanvasGrantInvalid):
		abortWithOpenAiMessage(c, http.StatusUnauthorized, "Canvas 授权无效或已过期", types.ErrorCode("canvas_authorization_invalid"))
	case errors.Is(err, model.ErrCanvasPermissionRevisionConflict):
		abortWithOpenAiMessage(c, http.StatusConflict, "Canvas 分组或模型权限已变化，请刷新后重试", types.ErrorCode("canvas_permission_changed"))
	case errors.Is(err, model.ErrCanvasManagedTokenChanged):
		abortWithOpenAiMessage(c, http.StatusConflict, "Canvas 管理 Token 已被修改，请重新授权", types.ErrorCode("canvas_managed_token_changed"))
	case errors.Is(err, model.ErrCanvasGroupUnavailable):
		abortWithOpenAiMessage(c, http.StatusForbidden, "当前账号无权使用所选分组或模型", types.ErrorCode("canvas_group_unavailable"))
	case errors.Is(err, model.ErrCanvasAcceptanceMismatch), errors.Is(err, model.ErrCanvasGroupExcluded), errors.Is(err, model.ErrCanvasAutoGroupEmpty):
		abortWithOpenAiMessage(c, http.StatusForbidden, "Canvas 执行身份与当前请求不一致", types.ErrorCode("canvas_execution_mismatch"))
	default:
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "Canvas 执行授权暂不可用", types.ErrorCode("canvas_authorization_unavailable"))
	}
	return false
}

// parseCanvasExecutionHeader 严格解析单个无填充 canonical base64url JSON 头。
func parseCanvasExecutionHeader(values []string) (canvasExecutionPayload, error) {
	if len(values) != 1 || values[0] == "" || len(values[0]) > 64*1024 || strings.ContainsAny(values[0], "= \t\r\n") {
		return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(values[0])
	if err != nil || len(decoded) == 0 || base64.RawURLEncoding.EncodeToString(decoded) != values[0] {
		return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
	}
	if !gjson.ValidBytes(decoded) {
		return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
	}
	allowedFields := map[string]struct{}{
		"version": {}, "issuer": {}, "user_id": {}, "instance_id": {}, "grant_id": {},
		"token_id": {}, "expected_group": {}, "permission_revision": {}, "auto_groups": {},
	}
	seenFields := make(map[string]struct{}, len(allowedFields))
	fieldCount := 0
	validFields := gjson.ParseBytes(decoded).IsObject()
	gjson.ParseBytes(decoded).ForEach(func(key, _ gjson.Result) bool {
		fieldCount++
		name := key.String()
		if _, ok := allowedFields[name]; !ok {
			validFields = false
			return false
		}
		if _, duplicate := seenFields[name]; duplicate {
			validFields = false
			return false
		}
		seenFields[name] = struct{}{}
		return true
	})
	if !validFields || fieldCount != len(allowedFields) || len(seenFields) != len(allowedFields) {
		return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
	}
	var payload canvasExecutionPayload
	if err := common.Unmarshal(decoded, &payload); err != nil || payload.Version != 1 ||
		!canvasHeaderValueValid(payload.Issuer, 2048) || !canvasHeaderValueValid(payload.UserID, 64) ||
		!canvasHeaderValueValid(payload.InstanceID, 128) || !canvasHeaderValueValid(payload.GrantID, 64) ||
		!canvasHeaderValueValid(payload.TokenID, 32) || !canvasHeaderValueValid(payload.ExpectedGroup, 191) ||
		!canvasHeaderValueValid(payload.PermissionRevision, 32) || len(payload.AutoGroups) > 1024 {
		return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
	}
	if !canonicalPositiveInteger(payload.TokenID) || !canonicalPositiveInteger(payload.PermissionRevision) {
		return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
	}
	seen := make(map[string]struct{}, len(payload.AutoGroups))
	for _, group := range payload.AutoGroups {
		if !canvasHeaderValueValid(group, 191) || group == model.CanvasExcludedGroup {
			return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
		}
		if _, ok := seen[group]; ok {
			return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
		}
		seen[group] = struct{}{}
	}
	if payload.ExpectedGroup == "auto" {
		if len(payload.AutoGroups) == 0 {
			return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
		}
	} else if len(payload.AutoGroups) != 0 || payload.ExpectedGroup == model.CanvasExcludedGroup {
		return canvasExecutionPayload{}, errCanvasExecutionHeaderInvalid
	}
	return payload, nil
}

// canvasCreationPath 判断路径是否允许管理 Token 发起新的供应商 POST。
func canvasCreationPath(path string) bool {
	return slices.Contains([]string{
		"/v1/chat/completions",
		"/v1/images/generations",
		"/v1/images/edits",
		"/v1/audio/speech",
		"/v1/videos",
	}, path)
}

// canvasHeaderValueValid 校验头字段的边界、首尾空白和控制字符。
func canvasHeaderValueValid(value string, maxLength int) bool {
	return value != "" && len(value) <= maxLength && strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

// canonicalPositiveInteger 判断字符串是否为无前导零的正十进制整数。
func canonicalPositiveInteger(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}
