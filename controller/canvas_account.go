package controller

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	// canvasAuthorizationTTL 是授权请求和授权码的有效期。
	canvasAuthorizationTTL = 5 * time.Minute
	// canvasGrantTTL 是 Canvas grant 的固定有效期。
	canvasGrantTTL = 30 * 24 * time.Hour
)

var (
	canvasPKCEChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	canvasPKCEVerifierPattern  = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
	canvasAuthorizePage        = template.Must(template.New("canvas-authorize").Parse(canvasAuthorizePageHTML))
)

// canvasAuthorizationPayload 保存请求到授权码之间必须保持一致的浏览器授权事实。
type canvasAuthorizationPayload struct {
	ClientID      string                    `json:"client_id"`
	InstanceID    string                    `json:"instance_id"`
	RedirectURI   string                    `json:"redirect_uri"`
	State         string                    `json:"state"`
	CodeChallenge string                    `json:"code_challenge"`
	Identity      model.AuthSessionIdentity `json:"identity"`
}

// canvasAuthorizeApproval 是已登录浏览器连接受信 Canvas 时提交的请求。
type canvasAuthorizeApproval struct {
	RequestToken string `json:"request_token"`
}

// canvasTokenExchangeRequest 描述后端兑换一次性授权码所需字段。
type canvasTokenExchangeRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	ClientID     string `json:"client_id"`
	InstanceID   string `json:"instance_id"`
	RedirectURI  string `json:"redirect_uri"`
	ClientSecret string `json:"client_secret"`
}

// canvasManagedGroupRequest 提供幂等管理 Token 操作标识。
type canvasManagedGroupRequest struct {
	OperationID string `json:"operation_id"`
}

// canvasAccountUser 是返回给受信 Canvas 的最小用户身份投影。
type canvasAccountUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Status      string `json:"status"`
}

// canvasAuthorizePageData 保存授权页模板所需的非凭据数据。
type canvasAuthorizePageData struct {
	Nonce        string
	RequestToken string
	CancelURI    string
	// SelectAccount 控制授权页是否显示显式选号提示。
	SelectAccount bool
}

// GetCanvasAuthorize 校验固定客户端、回调地址与 S256 PKCE 后自动连接同源登录身份。
// 页面通过现有 Refresh Cookie 恢复短期登录令牌；长期授权和浏览器 Access Token 不进入 URL，
// 一次性授权码只进入已校验的固定回调查询参数。
func GetCanvasAuthorize(c *gin.Context) {
	config, ok := canvasAccountConfiguration(c)
	if !ok {
		return
	}
	query := c.Request.URL.Query()
	allowed := map[string]bool{
		"client_id": true, "instance_id": true, "redirect_uri": true,
		"state": true, "code_challenge": true, "code_challenge_method": true, "prompt": true,
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 || key == "prompt" && values[0] != "select_account" {
			canvasAccountJSONError(c, http.StatusBadRequest, "canvas_authorization_invalid", "Canvas 授权请求无效")
			return
		}
	}
	selectAccount := query.Get("prompt") == "select_account"
	clientID, clientOK := canvasSingleQueryValue(query, "client_id", 64)
	instanceID, instanceOK := canvasSingleQueryValue(query, "instance_id", 128)
	redirectURI, redirectOK := canvasSingleQueryValue(query, "redirect_uri", 2048)
	state, stateOK := canvasSingleQueryValue(query, "state", 512)
	challenge, challengeOK := canvasSingleQueryValue(query, "code_challenge", 64)
	method, methodOK := canvasSingleQueryValue(query, "code_challenge_method", 16)
	decodedChallenge, challengeErr := base64.RawURLEncoding.Strict().DecodeString(challenge)
	if !clientOK || !instanceOK || !redirectOK || !stateOK || !challengeOK || !methodOK ||
		clientID != config.ClientID || instanceID != config.InstanceID || redirectURI != config.RedirectURI ||
		method != "S256" || !canvasPKCEChallengePattern.MatchString(challenge) || challengeErr != nil || len(decodedChallenge) != sha256.Size {
		canvasAccountJSONError(c, http.StatusBadRequest, "canvas_authorization_invalid", "Canvas 授权请求无效")
		return
	}
	payload, err := common.Marshal(canvasAuthorizationPayload{
		ClientID: clientID, InstanceID: instanceID, RedirectURI: redirectURI,
		State: state, CodeChallenge: challenge,
	})
	if err != nil {
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_authorization_unavailable", "Canvas 授权暂不可用")
		return
	}
	requestToken, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeCanvasAuthorize,
		Provider:  clientID,
		Intent:    instanceID,
		Payload:   string(payload),
		ExpiresAt: time.Now().Add(canvasAuthorizationTTL),
	})
	if err != nil {
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_authorization_unavailable", "Canvas 授权暂不可用")
		return
	}
	cancelURI, err := canvasAuthorizationRedirect(redirectURI, map[string]string{"error": "access_denied", "state": state})
	if err != nil {
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_authorization_unavailable", "Canvas 授权暂不可用")
		return
	}
	nonce, err := canvasRandomToken(18)
	if err != nil {
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_authorization_unavailable", "Canvas 授权暂不可用")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+nonce+"'; script-src 'nonce-"+nonce+"'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	if err := canvasAuthorizePage.Execute(c.Writer, canvasAuthorizePageData{
		Nonce: nonce, RequestToken: requestToken, CancelURI: cancelURI, SelectAccount: selectAccount,
	}); err != nil {
		c.Abort()
	}
}

// PostCanvasAuthorize 为部署配置中受信的 Canvas 签发五分钟单次登录码。
// PAT 不能替代浏览器会话；会话版本会冻结到授权码并在兑换时再次校验。
func PostCanvasAuthorize(c *gin.Context) {
	config, ok := canvasAccountConfiguration(c)
	if !ok {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		canvasAccountJSONError(c, http.StatusForbidden, "canvas_session_required", "请使用浏览器登录会话批准 Canvas 授权")
		return
	}
	var request canvasAuthorizeApproval
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || len(request.RequestToken) < 32 || len(request.RequestToken) > 256 {
		canvasAccountJSONError(c, http.StatusBadRequest, "canvas_authorization_invalid", "Canvas 授权请求无效")
		return
	}
	var redirectURI string
	_, err := model.ConsumeAuthFlowWithAction(request.RequestToken, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeCanvasAuthorize,
		Provider: config.ClientID,
		Intent:   config.InstanceID,
	}, func(tx *gorm.DB, flow *model.AuthFlow) error {
		var payload canvasAuthorizationPayload
		if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
			return model.ErrAuthFlowInvalid
		}
		if payload.ClientID != config.ClientID || payload.InstanceID != config.InstanceID || payload.RedirectURI != config.RedirectURI || payload.State == "" || payload.CodeChallenge == "" {
			return model.ErrAuthFlowInvalid
		}
		payload.Identity = model.AuthSessionIdentity{
			UserID: identity.UserID, SessionID: identity.SessionID,
			UserAuthVersion: identity.UserAuthVersion, SessionVersion: identity.SessionVersion,
		}
		if err := model.ValidateAuthSessionWithTx(tx, payload.Identity); err != nil {
			return err
		}
		encoded, err := common.Marshal(payload)
		if err != nil {
			return err
		}
		code, _, err := model.CreateAuthFlowWithTx(tx, model.AuthFlowCreate{
			Purpose:   model.AuthFlowPurposeCanvasCode,
			Provider:  config.ClientID,
			Intent:    config.InstanceID,
			UserId:    identity.UserID,
			SessionId: identity.SessionID,
			Payload:   string(encoded),
			ExpiresAt: time.Now().Add(canvasAuthorizationTTL),
		})
		if err != nil {
			return err
		}
		redirectURI, err = canvasAuthorizationRedirect(payload.RedirectURI, map[string]string{"code": code, "state": payload.State})
		return err
	})
	if err != nil {
		canvasAccountFlowError(c, err)
		return
	}
	recordUserSecurityAudit(c, identity.UserID, "canvas.authorize", map[string]any{
		"success": true, "client_id": config.ClientID, "instance_id": config.InstanceID,
	})
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"redirect_uri": redirectURI}})
}

// PostCanvasToken 原子消费授权码、校验 PKCE 和会话版本，并返回一次 grant Bearer。
// 数据库只保存 grant Bearer 的用途隔离 HMAC；响应未知时客户端必须重新发起授权。
func PostCanvasToken(c *gin.Context) {
	config, ok := canvasAccountConfiguration(c)
	if !ok {
		return
	}
	var request canvasTokenExchangeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !canvasPKCEVerifierPattern.MatchString(request.CodeVerifier) || len(request.Code) < 32 || len(request.Code) > 4096 {
		canvasAccountJSONError(c, http.StatusBadRequest, "canvas_token_request_invalid", "Canvas 授权码兑换请求无效")
		return
	}
	if request.ClientID != config.ClientID || request.InstanceID != config.InstanceID || request.RedirectURI != config.RedirectURI || !canvasClientSecretMatches(config.ClientSecret, request.ClientSecret) {
		canvasAccountJSONError(c, http.StatusUnauthorized, "canvas_client_invalid", "Canvas 客户端认证失败")
		return
	}
	rawGrant, err := canvasRandomToken(48)
	if err != nil {
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_token_unavailable", "Canvas 授权暂不可用")
		return
	}
	var grant *model.CanvasGrant
	var user model.User
	_, err = model.ConsumeAuthFlowWithAction(request.Code, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeCanvasCode,
		Provider: config.ClientID,
		Intent:   config.InstanceID,
	}, func(tx *gorm.DB, flow *model.AuthFlow) error {
		var payload canvasAuthorizationPayload
		if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
			return model.ErrAuthFlowInvalid
		}
		if payload.ClientID != request.ClientID || payload.InstanceID != request.InstanceID || payload.RedirectURI != request.RedirectURI || payload.Identity.UserID != flow.UserId || payload.Identity.SessionID != flow.SessionId {
			return model.ErrAuthFlowInvalid
		}
		actualChallenge := sha256.Sum256([]byte(request.CodeVerifier))
		encodedChallenge := base64.RawURLEncoding.EncodeToString(actualChallenge[:])
		if subtle.ConstantTimeCompare([]byte(encodedChallenge), []byte(payload.CodeChallenge)) != 1 {
			return model.ErrAuthFlowInvalid
		}
		if err := model.ValidateAuthSessionWithTx(tx, payload.Identity); err != nil {
			return err
		}
		var err error
		grant, err = model.UpsertCanvasGrantWithTx(
			tx, request.ClientID, request.InstanceID, payload.Identity.UserID,
			model.CanvasGrantTokenHash(rawGrant), strings.Join(common.CanvasAccountScopes(), " "),
			time.Now().Add(canvasGrantTTL).Unix(),
		)
		if err != nil {
			return err
		}
		return tx.First(&user, payload.Identity.UserID).Error
	})
	if err != nil {
		canvasAccountFlowError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"issuer": config.Issuer,
		"user":   canvasAccountUserProjection(&user),
		"grant": gin.H{
			"id": grant.ID, "token": rawGrant,
			"expires_at": time.Unix(grant.ExpiresAt, 0).UTC().Format(time.RFC3339),
			"scopes":     common.CanvasAccountScopes(),
		},
	}})
}

// GetCanvasAccount 返回 grant 绑定用户和当前全部可接入分组，不读取或返回派生 Key。
func GetCanvasAccount(c *gin.Context) {
	if _, ok := canvasAccountConfiguration(c); !ok {
		return
	}
	grant, user, ok := canvasGrantAuthority(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"user": canvasAccountUserProjection(user), "grant_id": grant.ID,
		"groups": canvasAccountGroups(user),
	}})
}

// PutCanvasManagedGroup 为 grant 的指定可用分组幂等创建或恢复唯一管理 Token。
// 普通分组固定路由；auto 保存排除后的非空显式范围且关闭跨组重试。
func PutCanvasManagedGroup(c *gin.Context) {
	if _, ok := canvasAccountConfiguration(c); !ok {
		return
	}
	grant, user, ok := canvasGrantAuthority(c)
	if !ok {
		return
	}
	group := c.Param("group")
	if group == "" || len(group) > 191 || strings.TrimSpace(group) != group {
		canvasAccountJSONError(c, http.StatusBadRequest, "canvas_group_invalid", "Canvas 分组请求无效")
		return
	}
	if group == model.CanvasExcludedGroup {
		canvasAccountModelError(c, model.ErrCanvasGroupExcluded)
		return
	}
	if !slices.Contains(canvasAccountGroups(user), group) {
		canvasAccountModelError(c, model.ErrCanvasGroupUnavailable)
		return
	}
	var request canvasManagedGroupRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		canvasAccountJSONError(c, http.StatusBadRequest, "canvas_operation_invalid", "Canvas 分组操作无效")
		return
	}
	autoGroups := []string(nil)
	if group == "auto" {
		autoGroups = canvasAccountAutoGroups(user)
	}
	authority, err := model.EnsureCanvasManagedToken(model.CanvasManagedTokenInput{
		GrantID: grant.ID, GroupID: group, OperationID: request.OperationID, AutoGroups: autoGroups,
	})
	if err != nil {
		canvasAccountModelError(c, err)
		return
	}
	if authority.Grant.ID != grant.ID || authority.User.Id != user.Id {
		canvasAccountModelError(c, model.ErrCanvasAcceptanceMismatch)
		return
	}
	storedAutoGroups, err := authority.Token.GetAutoGroups()
	if err != nil {
		canvasAccountModelError(c, model.ErrCanvasManagedTokenChanged)
		return
	}
	if storedAutoGroups == nil {
		storedAutoGroups = []string{}
	}
	recordUserSecurityAudit(c, user.Id, "canvas.token.manage", map[string]any{
		"success": true, "grant_id": grant.ID, "group": group, "token_id": authority.Token.Id,
	})
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"token_id": strconv.Itoa(authority.Token.Id), "key": authority.Token.GetFullKey(),
		"group": authority.Managed.GroupID, "status": "active",
		"credential_revision": strconv.FormatInt(authority.Managed.CredentialRevision, 10),
		"permission_revision": strconv.FormatInt(authority.Managed.PermissionRevision, 10),
		"auto_groups":         storedAutoGroups,
	}})
}

// PostCanvasRevoke 撤销当前 grant 并禁用且失效其明确归属的全部派生 Token。
func PostCanvasRevoke(c *gin.Context) {
	if _, ok := canvasAccountConfiguration(c); !ok {
		return
	}
	raw, ok := canvasGrantBearer(c)
	if !ok {
		return
	}
	grant, err := model.RevokeCanvasGrant(raw)
	if err != nil {
		canvasAccountModelError(c, err)
		return
	}
	recordUserSecurityAudit(c, grant.UserID, "canvas.revoke", map[string]any{
		"success": true, "grant_id": grant.ID, "instance_id": grant.InstanceID,
	})
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"revoked": true}})
}

// canvasAccountConfiguration 读取启用状态和固定客户端配置，并写入统一错误响应。
func canvasAccountConfiguration(c *gin.Context) (common.CanvasAccountConfig, bool) {
	if !common.CanvasAccountEnabled() {
		canvasAccountJSONError(c, http.StatusNotFound, "canvas_account_disabled", "Canvas 账号接入未启用")
		return common.CanvasAccountConfig{}, false
	}
	config, err := common.GetCanvasAccountConfig()
	if err != nil {
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_account_misconfigured", "Canvas 账号接入配置无效")
		return common.CanvasAccountConfig{}, false
	}
	return config, true
}

// canvasGrantAuthority 从 Bearer 读取当前有效 grant 及其权威用户。
func canvasGrantAuthority(c *gin.Context) (*model.CanvasGrant, *model.User, bool) {
	raw, ok := canvasGrantBearer(c)
	if !ok {
		return nil, nil, false
	}
	grant, user, err := model.GetCanvasGrantByToken(raw)
	if err != nil {
		canvasAccountModelError(c, err)
		return nil, nil, false
	}
	return grant, user, true
}

// canvasGrantBearer 提取格式和长度合法的单一 Bearer 凭据。
func canvasGrantBearer(c *gin.Context) (string, bool) {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) < 16 || len(parts[1]) > 512 {
		canvasAccountJSONError(c, http.StatusUnauthorized, "canvas_authorization_invalid", "Canvas 授权无效或已过期")
		return "", false
	}
	return parts[1], true
}

// canvasAccountGroups 返回用户当前可接入且已精确排除禁用项的有序分组。
func canvasAccountGroups(user *model.User) []string {
	usable := service.GetUserUsableGroups(user.Group)
	groups := make([]string, 0, len(usable))
	for group := range ratio_setting.GetGroupRatioCopy() {
		if group == model.CanvasExcludedGroup {
			continue
		}
		if _, ok := usable[group]; ok {
			groups = append(groups, group)
		}
	}
	if _, ok := usable["auto"]; ok {
		groups = append(groups, "auto")
	}
	sort.Strings(groups)
	return groups
}

// canvasAccountAutoGroups 生成受用户权限和站点上限约束的显式 Auto 范围。
func canvasAccountAutoGroups(user *model.User) []string {
	groups := setting.GetAutoGroups()
	filtered := make([]string, 0, len(groups))
	for _, group := range groups {
		if group != model.CanvasExcludedGroup {
			filtered = append(filtered, group)
		}
	}
	return service.FilterUserTokenAutoGroups(user.Group, filtered)
}

// canvasAccountUserProjection 将内部用户转换为不含敏感字段的外部身份。
func canvasAccountUserProjection(user *model.User) canvasAccountUser {
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	return canvasAccountUser{
		ID: strconv.Itoa(user.Id), DisplayName: displayName, Email: user.Email, Status: "active",
	}
}

// canvasSingleQueryValue 读取唯一且不含控制字符的查询参数。
func canvasSingleQueryValue(values url.Values, key string, maxLength int) (string, bool) {
	items := values[key]
	if len(items) != 1 || items[0] == "" || len(items[0]) > maxLength || strings.ContainsFunc(items[0], unicode.IsControl) {
		return "", false
	}
	return items[0], true
}

// canvasAuthorizationRedirect 在固定回调地址上附加授权结果字段。
func canvasAuthorizationRedirect(raw string, fields map[string]string) (string, error) {
	target, err := url.Parse(raw)
	if err != nil || !target.IsAbs() {
		return "", errors.New("invalid canvas redirect URI")
	}
	query := target.Query()
	for key, value := range fields {
		query.Set(key, value)
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

// canvasRandomToken 生成指定随机字节数的无填充 base64url token。
func canvasRandomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// canvasClientSecretMatches 以固定长度摘要比较可选客户端 secret。
func canvasClientSecretMatches(expected, actual string) bool {
	if expected == "" {
		return actual == ""
	}
	expectedHash := sha256.Sum256([]byte(expected))
	actualHash := sha256.Sum256([]byte(actual))
	return subtle.ConstantTimeCompare(expectedHash[:], actualHash[:]) == 1
}

// canvasAccountFlowError 将一次性流程错误映射为稳定的公开响应。
func canvasAccountFlowError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrAuthFlowInvalid), errors.Is(err, model.ErrAuthFlowExpired), errors.Is(err, model.ErrAuthFlowConsumed):
		canvasAccountJSONError(c, http.StatusBadRequest, "canvas_authorization_invalid", "Canvas 授权已失效，请重新开始")
	case errors.Is(err, model.ErrUserSessionInactive):
		canvasAccountJSONError(c, http.StatusUnauthorized, "canvas_session_invalid", "登录会话已失效，请重新登录")
	default:
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_authorization_unavailable", "Canvas 授权暂不可用")
	}
}

// canvasAccountModelError 将账号模型错误映射为稳定的公开响应。
func canvasAccountModelError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrCanvasGrantInvalid):
		canvasAccountJSONError(c, http.StatusUnauthorized, "canvas_authorization_invalid", "Canvas 授权无效或已过期")
	case errors.Is(err, model.ErrCanvasGroupExcluded):
		canvasAccountJSONError(c, http.StatusForbidden, "canvas_group_excluded", "该分组不参与 Canvas 接入")
	case errors.Is(err, model.ErrCanvasGroupUnavailable):
		canvasAccountJSONError(c, http.StatusForbidden, "canvas_group_unavailable", "当前账号无权使用该分组或模型")
	case errors.Is(err, model.ErrCanvasAutoGroupEmpty):
		canvasAccountJSONError(c, http.StatusConflict, "canvas_auto_group_empty", "自动路由当前没有可用实际分组")
	case errors.Is(err, model.ErrCanvasIdempotencyConflict):
		canvasAccountJSONError(c, http.StatusConflict, "canvas_operation_conflict", "operation_id 与原请求不一致")
	case errors.Is(err, model.ErrCanvasManagedTokenChanged):
		canvasAccountJSONError(c, http.StatusConflict, "canvas_managed_token_changed", "Canvas 管理 Token 已被修改，请重新授权")
	case errors.Is(err, model.ErrCanvasTokenLimitReached):
		canvasAccountJSONError(c, http.StatusConflict, "canvas_token_limit_reached", "当前账号的 Token 数量已达上限")
	case errors.Is(err, model.ErrCanvasPermissionRevisionConflict):
		canvasAccountJSONError(c, http.StatusConflict, "canvas_permission_changed", "分组或模型权限已变化，请刷新后重试")
	case errors.Is(err, model.ErrCanvasAcceptanceMismatch):
		canvasAccountJSONError(c, http.StatusForbidden, "canvas_execution_mismatch", "Canvas 执行身份与当前请求不一致")
	default:
		canvasAccountJSONError(c, http.StatusServiceUnavailable, "canvas_account_unavailable", "Canvas 账号服务暂不可用")
	}
}

// canvasAccountJSONError 返回禁止缓存的统一 Canvas JSON 错误。
func canvasAccountJSONError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(status, gin.H{"success": false, "code": code, "message": message})
}

// canvasAuthorizePageHTML 恢复浏览器会话并自动连接固定 Canvas，仅显式换号展示账号选择。
const canvasAuthorizePageHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>登录 Canvas</title>
  <style nonce="{{.Nonce}}">
    :root { color-scheme: light dark; font-family: Inter, "Segoe UI", "Microsoft YaHei", sans-serif; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #f3f5f7; color: #17202a; }
    main { width: min(460px, calc(100vw - 32px)); border: 1px solid #dce1e6; border-radius: 8px; background: #fff; padding: 28px; box-shadow: 0 16px 40px rgba(24, 32, 42, .10); }
    h1 { margin: 0 0 10px; font-size: 24px; letter-spacing: 0; }
    p { margin: 0 0 20px; color: #52606d; line-height: 1.65; }
    dl { margin: 0 0 24px; padding: 16px 0; border-block: 1px solid #e6e9ed; }
    div.row { display: grid; grid-template-columns: 88px 1fr; gap: 12px; padding: 5px 0; }
    dt { color: #6b7280; } dd { margin: 0; overflow-wrap: anywhere; }
    #status[data-error="true"] { color: #b42318; }
    .actions { display: flex; justify-content: flex-end; gap: 10px; }
    button { min-height: 40px; border-radius: 6px; border: 1px solid #cfd5dc; padding: 0 16px; font: inherit; cursor: pointer; background: #fff; color: #17202a; }
    button.primary { border-color: #16784a; background: #16784a; color: #fff; }
    button:disabled { cursor: wait; opacity: .55; }
    @media (prefers-color-scheme: dark) {
      body { background: #11161b; color: #edf2f7; } main { background: #1a2027; border-color: #343c45; box-shadow: none; }
      p, dt { color: #aeb8c4; } dl { border-color: #343c45; } button { background: #20272f; border-color: #46515d; color: #edf2f7; }
    }
  </style>
</head>
<body>
  <main id="authorization" data-request-token="{{.RequestToken}}" data-cancel-uri="{{.CancelURI}}">
    <h1>{{if .SelectAccount}}选择登录账号{{else}}正在登录画布{{end}}</h1>
    <p>登录后自动同步可用分组和模型，生成费用由当前 New API 账号结算。</p>
    {{if .SelectAccount}}<p id="account-choice">继续使用当前账号，或换一个账号登录。</p>{{end}}
    <dl>
      <div class="row"><dt>当前账号</dt><dd id="account">正在确认登录状态</dd></div>
    </dl>
    <p id="status" role="status" aria-live="polite">正在恢复登录会话</p>
    <div class="actions">
      {{if .SelectAccount}}<button id="switch-account" type="button" disabled>换一个账号</button>{{end}}
      <button id="cancel" type="button">取消</button>
      {{if .SelectAccount}}<button id="continue" class="primary" type="button" disabled>继续登录</button>{{end}}
    </div>
  </main>
  <script nonce="{{.Nonce}}">
    (() => {
      const root = document.getElementById('authorization');
      const account = document.getElementById('account');
      const status = document.getElementById('status');
      const continueLogin = document.getElementById('continue');
      const cancel = document.getElementById('cancel');
      const switchAccount = document.getElementById('switch-account');
      let accessToken = '';
      let sessionID = '';
      let connecting = false;
      const showError = (message) => { status.textContent = message; status.dataset.error = 'true'; };
      // 已选择新账号或尚未登录时，完成登录就直接回画布，不再重复展示选择页。
      const signIn = () => {
        const target = new URL(window.location.href);
        target.searchParams.delete('prompt');
        window.location.replace('/sign-in?redirect=' + encodeURIComponent(target.pathname + target.search));
      };
      cancel.addEventListener('click', () => window.location.assign(root.dataset.cancelUri));
      switchAccount?.addEventListener('click', async () => {
        if (!accessToken || !sessionID) return;
        switchAccount.disabled = true;
        continueLogin.disabled = true;
        cancel.disabled = true;
        status.dataset.error = 'false';
        status.textContent = '正在退出当前账号';
        try {
          const response = await fetch('/api/user/auth/logout', {
            method: 'POST', credentials: 'same-origin',
            headers: { 'accept': 'application/json', 'authorization': 'Bearer ' + accessToken, 'x-auth-session': sessionID },
          });
          if (!response.ok) throw new Error('logout rejected');
          signIn();
        } catch {
          showError('账号切换未完成，请稍后重试');
          switchAccount.disabled = false;
          continueLogin.disabled = false;
          cancel.disabled = false;
        }
      });
      // 仅连接服务端严格白名单中的一体化实例；身份来自当前有效浏览器会话。
      const connectCanvas = async () => {
        if (!accessToken || connecting) return;
        connecting = true;
        if (switchAccount) switchAccount.disabled = true;
        if (continueLogin) continueLogin.disabled = true;
        cancel.disabled = true;
        status.dataset.error = 'false';
        status.textContent = '正在进入画布并同步分组';
        try {
          const response = await fetch('/api/canvas/authorize', {
            method: 'POST', credentials: 'same-origin',
            headers: { 'accept': 'application/json', 'content-type': 'application/json', 'authorization': 'Bearer ' + accessToken },
            body: JSON.stringify({ request_token: root.dataset.requestToken }),
          });
          const body = await response.json();
          if (!response.ok || body.success !== true || !body.data?.redirect_uri) throw new Error('authorization rejected');
          window.location.assign(body.data.redirect_uri);
        } catch {
          showError('登录未完成，请返回画布后重试');
          cancel.disabled = false;
        }
      };
      continueLogin?.addEventListener('click', connectCanvas);
      (async () => {
        try {
          const response = await fetch('/api/user/auth/refresh', { method: 'POST', credentials: 'same-origin', headers: { 'accept': 'application/json' } });
          if (response.status === 401 || response.status === 403) {
            signIn();
            return;
          }
          const body = await response.json();
          if (!response.ok || body.success !== true || !body.data?.access_token || !body.data?.user || !body.data?.session?.sid) throw new Error('session unavailable');
          accessToken = body.data.access_token;
          sessionID = body.data.session.sid;
          account.textContent = body.data.user.display_name || body.data.user.username || String(body.data.user.id);
          if (continueLogin) {
            status.textContent = '请选择登录账号';
            switchAccount.disabled = false;
            continueLogin.disabled = false;
          } else {
            await connectCanvas();
          }
        } catch {
          showError('暂时无法确认登录状态，请稍后重试');
        }
      })();
    })();
  </script>
</body>
</html>`
