package controller

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// canvasAccountTestLogin 描述测试所需的授权兑换结果。
type canvasAccountTestLogin struct {
	Issuer string `json:"issuer"`
	User   struct {
		ID string `json:"id"`
	} `json:"user"`
	Grant struct {
		ID        string   `json:"id"`
		Token     string   `json:"token"`
		ExpiresAt string   `json:"expires_at"`
		Scopes    []string `json:"scopes"`
	} `json:"grant"`
}

// canvasAccountTestManagedToken 描述测试所需的管理 Token 响应。
type canvasAccountTestManagedToken struct {
	TokenID            string   `json:"token_id"`
	Key                string   `json:"key"`
	Group              string   `json:"group"`
	CredentialRevision string   `json:"credential_revision"`
	PermissionRevision string   `json:"permission_revision"`
	AutoGroups         []string `json:"auto_groups"`
}

// canvasAccountTestResponse 表示 Canvas 账号接口的统一测试响应。
type canvasAccountTestResponse[T any] struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Data    T      `json:"data"`
}

// setupCanvasAccountControllerTest 初始化隔离数据库、用户、分组和渠道。
func setupCanvasAccountControllerTest(t *testing.T) (*model.User, service.AuthIdentity) {
	t.Helper()
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Token{}, &model.Channel{}, &model.Ability{},
		&model.CanvasGrant{}, &model.CanvasManagedToken{}, &model.CanvasIdempotencyOperation{},
	))

	t.Setenv("CANVAS_ACCOUNT_ENABLED", "true")
	t.Setenv("CANVAS_ACCOUNT_ISSUER", "http://127.0.0.1:13000")
	t.Setenv("CANVAS_ACCOUNT_CLIENT_ID", "canvas")
	t.Setenv("CANVAS_ACCOUNT_INSTANCE_ID", "controller-test")
	t.Setenv("CANVAS_ACCOUNT_REDIRECT_URI", "http://localhost:5173/v1/auth/newapi/callback")
	t.Setenv("CANVAS_ACCOUNT_CLIENT_SECRET", "")

	previousUsable := setting.UserUsableGroups2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousAuto := setting.AutoGroups2JsonString()
	previousMax := setting.GetMaxTokenAutoGroups()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAuto))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(previousMax)))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","神秘分组":"Excluded","auto":"Auto"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2,"神秘分组":3}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["神秘分组","default","vip"]`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("5"))

	for index, group := range []string{"default", "vip", model.CanvasExcludedGroup} {
		channel := model.Channel{Id: 8100 + index, Name: "canvas-" + group, Type: 1, Status: common.ChannelStatusEnabled, Models: "canvas-test-model", Group: group}
		require.NoError(t, model.DB.Create(&channel).Error)
		require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: "canvas-test-model", ChannelId: channel.Id, Enabled: true}).Error)
	}
	return user, identity
}

// authorizeCanvasAccountForTest 完成一次授权、兑换并验证授权码不可重放。
func authorizeCanvasAccountForTest(t *testing.T, identity service.AuthIdentity) canvasAccountTestLogin {
	t.Helper()
	verifier := strings.Repeat("a", 64)
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"client_id": {"canvas"}, "instance_id": {"controller-test"},
		"redirect_uri": {"http://localhost:5173/v1/auth/newapi/callback"},
		"state":        {"canvas-state"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	authorize := canvasAccountControllerRequest(http.MethodGet, "/api/canvas/authorize?"+query.Encode(), "", "", GetCanvasAuthorize)
	require.Equal(t, http.StatusOK, authorize.Code, authorize.Body.String())
	match := regexp.MustCompile(`data-request-token="([A-Za-z0-9_-]+)"`).FindStringSubmatch(authorize.Body.String())
	require.Len(t, match, 2)

	approvalBody, err := common.Marshal(canvasAuthorizeApproval{RequestToken: match[1]})
	require.NoError(t, err)
	approved := securityEnrollmentRequest(http.MethodPost, "/api/canvas/authorize", string(approvalBody), "", identity, PostCanvasAuthorize)
	require.Equal(t, http.StatusOK, approved.Code, approved.Body.String())
	var approval canvasAccountTestResponse[struct {
		RedirectURI string `json:"redirect_uri"`
	}]
	require.NoError(t, common.Unmarshal(approved.Body.Bytes(), &approval))
	require.True(t, approval.Success)
	redirect, err := url.Parse(approval.Data.RedirectURI)
	require.NoError(t, err)
	require.Equal(t, "canvas-state", redirect.Query().Get("state"))
	code := redirect.Query().Get("code")
	require.NotEmpty(t, code)

	exchangeBody, err := common.Marshal(canvasTokenExchangeRequest{
		Code: code, CodeVerifier: verifier, ClientID: "canvas", InstanceID: "controller-test",
		RedirectURI: "http://localhost:5173/v1/auth/newapi/callback",
	})
	require.NoError(t, err)
	exchanged := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/token", string(exchangeBody), "", PostCanvasToken)
	require.Equal(t, http.StatusOK, exchanged.Code, exchanged.Body.String())
	var response canvasAccountTestResponse[canvasAccountTestLogin]
	require.NoError(t, common.Unmarshal(exchanged.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotEmpty(t, response.Data.Grant.Token)

	replayed := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/token", string(exchangeBody), "", PostCanvasToken)
	assert.Equal(t, http.StatusBadRequest, replayed.Code)
	return response.Data
}

// TestCanvasAccountTransportConfiguration 禁止非回环明文配置，保留本机验收入口。
func TestCanvasAccountTransportConfiguration(t *testing.T) {
	t.Setenv("CANVAS_ACCOUNT_CLIENT_ID", "canvas")
	t.Setenv("CANVAS_ACCOUNT_INSTANCE_ID", "transport-test")
	for _, test := range []struct {
		name, issuer, redirect string
		valid                  bool
	}{
		{"https", "https://api.example.com", "https://canvas.example.com/callback", true},
		{"local", "http://127.0.0.1:13000", "http://localhost:5173/callback", true},
		{"ipv6", "http://[::1]:13000", "http://[::1]:5173/callback", true},
		{"remote issuer", "http://api.example.com", "https://canvas.example.com/callback", false},
		{"remote callback", "https://api.example.com", "http://canvas.example.com/callback", false},
		{"lookalike localhost", "http://localhost.example.com", "https://canvas.example.com/callback", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CANVAS_ACCOUNT_ISSUER", test.issuer)
			t.Setenv("CANVAS_ACCOUNT_REDIRECT_URI", test.redirect)
			_, err := common.GetCanvasAccountConfig()
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestCanvasAccountAuthorizePrompt 覆盖一体化登录与显式换号的参数边界。
func TestCanvasAccountAuthorizePrompt(t *testing.T) {
	_, _ = setupCanvasAccountControllerTest(t)
	challenge := sha256.Sum256([]byte(strings.Repeat("a", 64)))
	query := url.Values{
		"client_id":             {"canvas"},
		"instance_id":           {"controller-test"},
		"redirect_uri":          {"http://localhost:5173/v1/auth/newapi/callback"},
		"state":                 {"canvas-prompt-state"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}.Encode()

	for _, test := range []struct {
		name          string
		promptQuery   string
		status        int
		selectAccount bool
	}{
		{name: "without prompt", status: http.StatusOK},
		{name: "select account", promptQuery: "&prompt=select_account", status: http.StatusOK, selectAccount: true},
		{name: "unsupported prompt", promptQuery: "&prompt=consent", status: http.StatusBadRequest},
		{name: "empty prompt", promptQuery: "&prompt=", status: http.StatusBadRequest},
		{name: "duplicate prompt", promptQuery: "&prompt=select_account&prompt=select_account", status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := canvasAccountControllerRequest(
				http.MethodGet,
				"/api/canvas/authorize?"+query+test.promptQuery,
				"",
				"",
				GetCanvasAuthorize,
			)
			require.Equal(t, test.status, response.Code, response.Body.String())
			if test.status != http.StatusOK {
				var body canvasAccountTestResponse[struct{}]
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				assert.False(t, body.Success)
				assert.Equal(t, "canvas_authorization_invalid", body.Code)
				return
			}

			body := response.Body.String()
			assert.NotContains(t, body, `id="approve"`)
			assert.Equal(t, test.selectAccount, strings.Contains(body, `<button id="switch-account"`))
			assert.Equal(t, test.selectAccount, strings.Contains(body, `<button id="continue"`))
			assert.Equal(t, test.selectAccount, strings.Contains(body, `id="account-choice"`))
			assert.Contains(t, body, "登录后自动同步可用分组和模型")
			assert.Contains(t, body, "await connectCanvas();")
			assert.Contains(t, body, "target.searchParams.delete('prompt')")
		})
	}
}

// TestCanvasAccountAuthorizationManagementAndRecovery 覆盖账号授权、管理、撤销和恢复合同。
func TestCanvasAccountAuthorizationManagementAndRecovery(t *testing.T) {
	user, identity := setupCanvasAccountControllerTest(t)
	login := authorizeCanvasAccountForTest(t, identity)
	assert.Equal(t, "http://127.0.0.1:13000", login.Issuer)
	assert.Equal(t, strconv.Itoa(user.Id), login.User.ID)
	assert.ElementsMatch(t, common.CanvasAccountScopes(), login.Grant.Scopes)

	account := canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", login.Grant.Token, GetCanvasAccount)
	require.Equal(t, http.StatusOK, account.Code, account.Body.String())
	var state canvasAccountTestResponse[struct {
		GrantID string   `json:"grant_id"`
		Groups  []string `json:"groups"`
	}]
	require.NoError(t, common.Unmarshal(account.Body.Bytes(), &state))
	assert.Equal(t, login.Grant.ID, state.Data.GrantID)
	assert.Equal(t, []string{"auto", "default", "vip"}, state.Data.Groups)

	excluded := canvasManagedGroupRequestForTest(t, login.Grant.Token, model.CanvasExcludedGroup, "excluded-operation")
	assert.Equal(t, http.StatusForbidden, excluded.Code)
	var tokenCount int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenCount).Error)
	assert.Zero(t, tokenCount)

	created := canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation")
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	defaultToken := decodeCanvasManagedGroupForTest(t, created)
	assert.Equal(t, "1", defaultToken.CredentialRevision)
	assert.Equal(t, "1", defaultToken.PermissionRevision)
	assert.NotNil(t, defaultToken.AutoGroups)
	assert.Empty(t, defaultToken.AutoGroups)

	retried := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	assert.Equal(t, defaultToken.TokenID, retried.TokenID)
	assert.Equal(t, defaultToken.Key, retried.Key)
	conflict := canvasManagedGroupRequestForTest(t, login.Grant.Token, "vip", "default-operation")
	assert.Equal(t, http.StatusConflict, conflict.Code)

	require.NoError(t, model.DB.Create(&model.Option{Key: "GroupRatio", Value: `{"default":1,"vip":2,"神秘分组":3}`}).Error)
	permissionBaseline := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "GroupRatio").Update("value", `{"default":9,"vip":7,"神秘分组":5}`).Error)
	priceChanged := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	assert.Equal(t, permissionBaseline.PermissionRevision, priceChanged.PermissionRevision)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 8100).Update("balance", 123.45).Error)
	balanceChanged := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	assert.Equal(t, permissionBaseline.PermissionRevision, balanceChanged.PermissionRevision)

	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 8100).Update("enabled", false).Error)
	abilityDisabled := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	assert.Greater(t, canvasPermissionRevisionForTest(t, abilityDisabled), canvasPermissionRevisionForTest(t, balanceChanged))
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 8100).Update("enabled", true).Error)
	abilityEnabled := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	assert.Greater(t, canvasPermissionRevisionForTest(t, abilityEnabled), canvasPermissionRevisionForTest(t, abilityDisabled))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 8100).Update("status", common.ChannelStatusManuallyDisabled).Error)
	channelDisabled := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	assert.Greater(t, canvasPermissionRevisionForTest(t, channelDisabled), canvasPermissionRevisionForTest(t, abilityEnabled))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 8100).Update("status", common.ChannelStatusEnabled).Error)
	channelEnabled := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-operation"))
	assert.Greater(t, canvasPermissionRevisionForTest(t, channelEnabled), canvasPermissionRevisionForTest(t, channelDisabled))

	auto := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "auto", "auto-operation"))
	assert.Equal(t, []string{"default", "vip"}, auto.AutoGroups)
	assert.NotContains(t, auto.AutoGroups, model.CanvasExcludedGroup)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip"]`))
	updatedAuto := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "auto", "auto-operation"))
	assert.Equal(t, auto.TokenID, updatedAuto.TokenID)
	assert.Equal(t, auto.Key, updatedAuto.Key)
	assert.Equal(t, []string{"vip"}, updatedAuto.AutoGroups)
	assert.Greater(t, canvasPermissionRevisionForTest(t, updatedAuto), canvasPermissionRevisionForTest(t, auto))

	var autoRecord model.Token
	require.NoError(t, model.DB.First(&autoRecord, "id = ?", updatedAuto.TokenID).Error)
	require.NoError(t, autoRecord.SetAutoGroups([]string{"default"}))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", autoRecord.Id).Update("auto_groups", autoRecord.AutoGroups).Error)
	tampered := canvasManagedGroupRequestForTest(t, login.Grant.Token, "auto", "auto-operation")
	assert.Equal(t, http.StatusConflict, tampered.Code)

	revoked := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/revoke", `{}`, login.Grant.Token, PostCanvasRevoke)
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	assert.Equal(t, http.StatusUnauthorized, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", login.Grant.Token, GetCanvasAccount).Code)
	var defaultRecord model.Token
	require.NoError(t, model.DB.First(&defaultRecord, "id = ?", defaultToken.TokenID).Error)
	assert.Equal(t, common.TokenStatusDisabled, defaultRecord.Status)

	reauthorized := authorizeCanvasAccountForTest(t, identity)
	assert.Equal(t, login.Grant.ID, reauthorized.Grant.ID)
	assert.NotEqual(t, login.Grant.Token, reauthorized.Grant.Token)
	restored := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, reauthorized.Grant.Token, "default", "default-operation"))
	assert.Equal(t, defaultToken.TokenID, restored.TokenID)
	assert.Equal(t, defaultToken.Key, restored.Key)
	assert.Greater(t, canvasPermissionRevisionForTest(t, restored), canvasPermissionRevisionForTest(t, channelEnabled))
	require.NoError(t, model.DB.First(&defaultRecord, "id = ?", defaultToken.TokenID).Error)
	assert.Equal(t, common.TokenStatusEnabled, defaultRecord.Status)

	autoAfterReauthorization := canvasManagedGroupRequestForTest(t, reauthorized.Grant.Token, "auto", "auto-operation")
	assert.Equal(t, http.StatusConflict, autoAfterReauthorization.Code)
	require.NoError(t, model.DB.First(&autoRecord, "id = ?", updatedAuto.TokenID).Error)
	assert.Equal(t, common.TokenStatusDisabled, autoRecord.Status)
	var autoManaged model.CanvasManagedToken
	require.NoError(t, model.DB.First(&autoManaged, "token_id = ?", autoRecord.Id).Error)
	assert.Equal(t, model.CanvasManagedTokenStatusChanged, autoManaged.Status)
}

// TestCanvasAccountPartialGroups 验证令牌数量达限和 auto 空范围不会破坏已有分组。
func TestCanvasAccountPartialGroups(t *testing.T) {
	_, identity := setupCanvasAccountControllerTest(t)
	login := authorizeCanvasAccountForTest(t, identity)
	existing := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-limited"))
	previousLimit := operation_setting.GetMaxUserTokens()
	operation_setting.GetTokenSetting().MaxUserTokens = 1
	t.Cleanup(func() { operation_setting.GetTokenSetting().MaxUserTokens = previousLimit })
	limited := canvasManagedGroupRequestForTest(t, login.Grant.Token, "vip", "vip-limited")
	assert.Equal(t, http.StatusConflict, limited.Code)
	assert.Contains(t, limited.Body.String(), "canvas_token_limit_reached")
	reused := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "default-limited"))
	assert.Equal(t, existing.TokenID, reused.TokenID)
	assert.Equal(t, existing.Key, reused.Key)
	operation_setting.GetTokenSetting().MaxUserTokens = previousLimit
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["神秘分组"]`))
	empty := canvasManagedGroupRequestForTest(t, login.Grant.Token, "auto", "empty-auto")
	assert.Equal(t, http.StatusConflict, empty.Code)
	assert.Contains(t, empty.Body.String(), "canvas_auto_group_empty")
	var tokens []model.Token
	require.NoError(t, model.DB.Find(&tokens).Error)
	require.Len(t, tokens, 1)
	assert.False(t, tokens[0].CrossGroupRetry)
	assert.Equal(t, "default", tokens[0].Group)
}

// TestCanvasAccountManualExpiryIsNotRestored 验证人工改期不会被同步或重新授权抵消。
func TestCanvasAccountManualExpiryIsNotRestored(t *testing.T) {
	for _, test := range []struct {
		name         string
		expiry       int64
		revokeBefore bool
	}{
		{name: "already-expired", expiry: time.Now().Unix() - 1},
		{name: "shorter-lifetime", expiry: time.Now().Unix() + 3600},
		{name: "unlimited-lifetime", expiry: -1},
		{name: "changed-after-revoke", expiry: time.Now().Unix() - 1, revokeBefore: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, identity := setupCanvasAccountControllerTest(t)
			login := authorizeCanvasAccountForTest(t, identity)
			created := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "manual-expiry"))
			if test.revokeBefore {
				revoked := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/revoke", `{}`, login.Grant.Token, PostCanvasRevoke)
				require.Equal(t, http.StatusOK, revoked.Code)
			}
			var token model.Token
			require.NoError(t, model.DB.First(&token, "id = ?", created.TokenID).Error)
			token.ExpiredTime = test.expiry
			require.NoError(t, token.Update())
			if !test.revokeBefore {
				response := canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "manual-expiry")
				assert.Equal(t, http.StatusConflict, response.Code)
				var failure canvasAccountTestResponse[any]
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &failure))
				assert.Equal(t, "canvas_managed_token_changed", failure.Code)
			}
			reauthorized := authorizeCanvasAccountForTest(t, identity)
			response := canvasManagedGroupRequestForTest(t, reauthorized.Grant.Token, "default", "manual-expiry")
			assert.Equal(t, http.StatusConflict, response.Code)
			require.NoError(t, model.DB.First(&token, "id = ?", created.TokenID).Error)
			assert.Equal(t, test.expiry, token.ExpiredTime)
			var count int64
			require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
		})
	}
}

// canvasManagedGroupRequestForTest 向管理分组处理器发送隔离请求。
func canvasManagedGroupRequestForTest(t *testing.T, grant, group, operation string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(canvasManagedGroupRequest{OperationID: operation})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/canvas/groups/"+url.PathEscape(group), strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer "+grant)
	c.Params = gin.Params{{Key: "group", Value: group}}
	PutCanvasManagedGroup(c)
	return response
}

// decodeCanvasManagedGroupForTest 校验并解析成功的管理 Token 响应。
func decodeCanvasManagedGroupForTest(t *testing.T, response *httptest.ResponseRecorder) canvasAccountTestManagedToken {
	t.Helper()
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var decoded canvasAccountTestResponse[canvasAccountTestManagedToken]
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &decoded))
	require.True(t, decoded.Success)
	return decoded.Data
}

// canvasPermissionRevisionForTest 将接口中的十进制权限修订转换为可比较整数。
func canvasPermissionRevisionForTest(t *testing.T, token canvasAccountTestManagedToken) int64 {
	t.Helper()
	revision, err := strconv.ParseInt(token.PermissionRevision, 10, 64)
	require.NoError(t, err)
	return revision
}

// canvasAccountControllerRequest 构造无需启动 HTTP 服务的处理器请求。
func canvasAccountControllerRequest(method, path, body, bearer string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		c.Request.Header.Set("Authorization", "Bearer "+bearer)
	}
	handler(c)
	return response
}
