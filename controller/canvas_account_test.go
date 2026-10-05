package controller

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", "")
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
	require.NoError(t, model.DB.Model(&model.Option{}).Where(map[string]any{"key": "GroupRatio"}).Update("value", `{"default":9,"vip":7,"神秘分组":5}`).Error)
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

// TestCanvasControlledRotation 验证维护入口与读取入口严格区分并限制在当前 grant。
func TestCanvasControlledRotation(t *testing.T) {
	_, identity := setupCanvasAccountControllerTest(t)
	login := authorizeCanvasAccountForTest(t, identity)
	old := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, login.Grant.Token, "default", "initial"))
	tokenID, err := strconv.Atoi(old.TokenID)
	require.NoError(t, err)
	fingerprint := sha256.Sum256([]byte(old.Key))
	body, err := common.Marshal(canvasManagedGroupRequest{OperationID: "rotate-1", Rotation: &model.CanvasManagedTokenRotation{TokenID: tokenID, CredentialRevision: 1, KeyFingerprint: hex.EncodeToString(fingerprint[:])}})
	require.NoError(t, err)
	request := func(bearer, group string, rotate bool, value []byte) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/canvas/groups/"+url.PathEscape(group)+"/rotate", strings.NewReader(string(value)))
		c.Request.Header.Set("Authorization", "Bearer "+bearer)
		c.Params = gin.Params{{Key: "group", Value: group}}
		if rotate {
			PostCanvasRotateGroup(c)
		} else {
			PutCanvasManagedGroup(c)
		}
		return response
	}
	assert.Equal(t, http.StatusUnauthorized, request("invalid-grant", "default", true, body).Code)
	assert.Equal(t, http.StatusBadRequest, request(login.Grant.Token, "default", false, body).Code)
	assert.Equal(t, http.StatusBadRequest, request(login.Grant.Token, "default", true, []byte(`{"operation_id":"missing-version"}`)).Code)
	assert.Equal(t, http.StatusForbidden, request(login.Grant.Token, model.CanvasExcludedGroup, true, body).Code)
	rotated := decodeCanvasManagedGroupForTest(t, request(login.Grant.Token, "default", true, body))
	assert.Equal(t, old.TokenID, rotated.TokenID)
	assert.Equal(t, "2", rotated.CredentialRevision)
	assert.NotEqual(t, old.Key, rotated.Key)
	retried := decodeCanvasManagedGroupForTest(t, request(login.Grant.Token, "default", true, body))
	assert.Equal(t, rotated, retried)
	revoked := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/revoke", "{}", login.Grant.Token, PostCanvasRevoke)
	require.Equal(t, http.StatusOK, revoked.Code)
	assert.Equal(t, http.StatusUnauthorized, request(login.Grant.Token, "default", true, body).Code)
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

// canvasAccountMultiClientConfig 描述测试使用的附加 Canvas 客户端身份。
type canvasAccountMultiClientConfig struct {
	ClientID     string `json:"client_id"`
	InstanceID   string `json:"instance_id"`
	RedirectURI  string `json:"redirect_uri"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// canvasAccountMultiFlow 保存一次授权流程的 PKCE 与一次性令牌。
type canvasAccountMultiFlow struct {
	Config       canvasAccountMultiClientConfig
	Verifier     string
	RequestToken string
	Code         string
}

// canvasAccountMultiRequestTokenPattern 只提取授权页生成的不透明流程令牌。
var canvasAccountMultiRequestTokenPattern = regexp.MustCompile(`data-request-token="([A-Za-z0-9_-]+)"`)

// TestCanvasAccountMultipleClientsIsolation 验证旧客户端与附加实例各自维护授权和管理 Token。
func TestCanvasAccountMultipleClientsIsolation(t *testing.T) {
	user, identity := setupCanvasAccountControllerTest(t)
	local := canvasAccountMultiLocalConfig()
	canvasAccountSetAdditionalClients(t, local, canvasAccountMultiSiblingConfig())

	primary := authorizeCanvasAccountForTest(t, identity)
	localLogin := canvasAccountAuthorizeMultiClient(t, identity, local, "local-login")
	require.Equal(t, primary.Issuer, localLogin.Issuer)
	require.Equal(t, primary.User.ID, localLogin.User.ID)
	require.Equal(t, user.Id, canvasAccountGrantUserID(t, localLogin))
	assert.NotEqual(t, primary.Grant.ID, localLogin.Grant.ID)
	assert.NotEqual(t, primary.Grant.Token, localLogin.Grant.Token)

	primaryManaged := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, primary.Grant.Token, "default", "primary-default"))
	localManaged := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, localLogin.Grant.Token, "default", "local-default"))
	assert.NotEqual(t, primaryManaged.TokenID, localManaged.TokenID)

	relogged := canvasAccountAuthorizeMultiClient(t, identity, local, "local-relogin")
	assert.Equal(t, localLogin.Grant.ID, relogged.Grant.ID)
	assert.NotEqual(t, localLogin.Grant.Token, relogged.Grant.Token)
	assert.Equal(t, http.StatusUnauthorized, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", localLogin.Grant.Token, GetCanvasAccount).Code)
	assert.Equal(t, http.StatusOK, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", primary.Grant.Token, GetCanvasAccount).Code)

	localManagedAfterRelogin := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, relogged.Grant.Token, "default", "local-default"))
	assert.Equal(t, localManaged.TokenID, localManagedAfterRelogin.TokenID)
	primaryManagedAfterRelogin := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, primary.Grant.Token, "default", "primary-default"))
	assert.Equal(t, primaryManaged.TokenID, primaryManagedAfterRelogin.TokenID)

	revoked := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/revoke", `{}`, relogged.Grant.Token, PostCanvasRevoke)
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	revokedAgain := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/revoke", `{}`, relogged.Grant.Token, PostCanvasRevoke)
	require.Equal(t, http.StatusOK, revokedAgain.Code, revokedAgain.Body.String())
	assert.Equal(t, http.StatusUnauthorized, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", relogged.Grant.Token, GetCanvasAccount).Code)
	assert.Equal(t, http.StatusOK, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", primary.Grant.Token, GetCanvasAccount).Code)
	primaryManagedAfterRevoke := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, primary.Grant.Token, "default", "primary-default"))
	assert.Equal(t, primaryManaged.TokenID, primaryManagedAfterRevoke.TokenID)
}

// TestCanvasAccountMultipleClientsProtocolBoundaries 验证精确身份选择、PKCE、重放和过期边界。
func TestCanvasAccountMultipleClientsProtocolBoundaries(t *testing.T) {
	_, identity := setupCanvasAccountControllerTest(t)
	local := canvasAccountMultiLocalConfig()
	sibling := canvasAccountMultiSiblingConfig()
	canvasAccountSetAdditionalClients(t, local, sibling)

	t.Run("exact authorization identity", func(t *testing.T) {
		base := canvasAccountMultiAuthorizeQuery(local, "exact-pair", strings.Repeat("p", 64))
		for _, test := range []struct {
			name string
			edit func(url.Values)
		}{
			{name: "legacy client with local instance", edit: func(query url.Values) { query.Set("client_id", "canvas") }},
			{name: "sibling instance with local redirect", edit: func(query url.Values) { query.Set("instance_id", sibling.InstanceID) }},
			{name: "sibling redirect with local instance", edit: func(query url.Values) { query.Set("redirect_uri", sibling.RedirectURI) }},
		} {
			t.Run(test.name, func(t *testing.T) {
				query := canvasAccountCloneValues(base)
				test.edit(query)
				response := canvasAccountControllerRequest(http.MethodGet, "/api/canvas/authorize?"+query.Encode(), "", "", GetCanvasAuthorize)
				canvasAccountRequireFailure(t, response, http.StatusBadRequest, "canvas_authorization_invalid")
			})
		}

		duplicate := canvasAccountCloneValues(base)
		duplicate.Add("client_id", local.ClientID)
		response := canvasAccountControllerRequest(http.MethodGet, "/api/canvas/authorize?"+duplicate.Encode(), "", "", GetCanvasAuthorize)
		canvasAccountRequireFailure(t, response, http.StatusBadRequest, "canvas_authorization_invalid")
	})

	t.Run("request token keeps selected identity", func(t *testing.T) {
		flow := canvasAccountBeginMultiClient(t, local, "saved-flow", strings.Repeat("q", 64))
		canvasAccountSetAdditionalClients(t, sibling, local)
		canvasAccountApproveMultiClient(t, identity, &flow)
		login := canvasAccountExchangeMultiClient(t, flow, flow.Verifier, local)
		require.Equal(t, http.StatusOK, login.Code, login.Body.String())
	})

	t.Run("token identity cannot cross configured pairs", func(t *testing.T) {
		for _, candidate := range []canvasAccountMultiClientConfig{
			{ClientID: local.ClientID, InstanceID: local.InstanceID, RedirectURI: sibling.RedirectURI, ClientSecret: local.ClientSecret},
			{ClientID: sibling.ClientID, InstanceID: sibling.InstanceID, RedirectURI: local.RedirectURI, ClientSecret: sibling.ClientSecret},
			{ClientID: local.ClientID, InstanceID: local.InstanceID, RedirectURI: local.RedirectURI, ClientSecret: "wrong-secret"},
		} {
			flow := canvasAccountBeginMultiClient(t, local, "token-client", strings.Repeat("r", 64))
			canvasAccountApproveMultiClient(t, identity, &flow)
			response := canvasAccountExchangeMultiClient(t, flow, flow.Verifier, candidate)
			canvasAccountRequireFailure(t, response, http.StatusUnauthorized, "canvas_client_invalid")
		}

		for _, candidate := range []canvasAccountMultiClientConfig{
			{ClientID: "canvas", InstanceID: "controller-test", RedirectURI: "http://localhost:5173/v1/auth/newapi/callback"},
			{ClientID: local.ClientID, InstanceID: sibling.InstanceID, RedirectURI: sibling.RedirectURI, ClientSecret: sibling.ClientSecret},
		} {
			flow := canvasAccountBeginMultiClient(t, local, "token-flow", strings.Repeat("r", 64))
			canvasAccountApproveMultiClient(t, identity, &flow)
			response := canvasAccountExchangeMultiClient(t, flow, flow.Verifier, candidate)
			canvasAccountRequireFailure(t, response, http.StatusBadRequest, "canvas_authorization_invalid")
		}
		flow := canvasAccountBeginMultiClient(t, local, "token-valid", strings.Repeat("r", 64))
		canvasAccountApproveMultiClient(t, identity, &flow)
		valid := canvasAccountExchangeMultiClient(t, flow, flow.Verifier, local)
		require.Equal(t, http.StatusOK, valid.Code, valid.Body.String())
	})

	t.Run("pkce replay and expiry remain enforced", func(t *testing.T) {
		flow := canvasAccountBeginMultiClient(t, sibling, "pkce", strings.Repeat("s", 64))
		canvasAccountApproveMultiClient(t, identity, &flow)
		wrongVerifier := canvasAccountExchangeMultiClient(t, flow, strings.Repeat("t", 64), sibling)
		canvasAccountRequireFailure(t, wrongVerifier, http.StatusBadRequest, "canvas_authorization_invalid")
		valid := canvasAccountExchangeMultiClient(t, flow, flow.Verifier, sibling)
		require.Equal(t, http.StatusOK, valid.Code, valid.Body.String())
		replayed := canvasAccountExchangeMultiClient(t, flow, flow.Verifier, sibling)
		canvasAccountRequireFailure(t, replayed, http.StatusBadRequest, "canvas_authorization_invalid")

		expired := canvasAccountBeginMultiClient(t, sibling, "expired", strings.Repeat("u", 64))
		canvasAccountApproveMultiClient(t, identity, &expired)
		record, err := model.GetAuthFlow(expired.Code, model.AuthFlowMatch{
			Purpose: model.AuthFlowPurposeCanvasCode, Provider: sibling.ClientID, Intent: sibling.InstanceID,
		})
		require.NoError(t, err)
		require.NoError(t, model.DB.Model(&model.AuthFlow{}).Where("id = ?", record.Id).Update("expires_at", time.Now().Add(-time.Second)).Error)
		canvasAccountRequireFailure(t, canvasAccountExchangeMultiClient(t, expired, expired.Verifier, sibling), http.StatusBadRequest, "canvas_authorization_invalid")
	})
}

// TestCanvasAccountRemovedAdditionalClient 验证移除实例会立即封锁未完成流程和既有 grant。
func TestCanvasAccountRemovedAdditionalClient(t *testing.T) {
	_, identity := setupCanvasAccountControllerTest(t)
	local := canvasAccountMultiLocalConfig()
	canvasAccountSetAdditionalClients(t, local)
	primary := authorizeCanvasAccountForTest(t, identity)

	pendingApproval := canvasAccountBeginMultiClient(t, local, "pending-approval", strings.Repeat("v", 64))
	pendingRedemption := canvasAccountBeginMultiClient(t, local, "pending-redemption", strings.Repeat("w", 64))
	canvasAccountApproveMultiClient(t, identity, &pendingRedemption)
	localLogin := canvasAccountAuthorizeMultiClient(t, identity, local, "active-local")
	localManaged := decodeCanvasManagedGroupForTest(t, canvasManagedGroupRequestForTest(t, localLogin.Grant.Token, "default", "removed-local"))
	require.NotEmpty(t, localManaged.TokenID)

	canvasAccountRemoveAdditionalClients(t)

	approvalBody, err := common.Marshal(canvasAuthorizeApproval{RequestToken: pendingApproval.RequestToken})
	require.NoError(t, err)
	approval := securityEnrollmentRequest(http.MethodPost, "/api/canvas/authorize", string(approvalBody), "", identity, PostCanvasAuthorize)
	canvasAccountRequireFailure(t, approval, http.StatusBadRequest, "canvas_authorization_invalid")

	redemption := canvasAccountExchangeMultiClient(t, pendingRedemption, pendingRedemption.Verifier, local)
	canvasAccountRequireFailure(t, redemption, http.StatusUnauthorized, "canvas_client_invalid")
	canvasAccountRequireFailure(t, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", localLogin.Grant.Token, GetCanvasAccount), http.StatusUnauthorized, "canvas_authorization_invalid")
	canvasAccountRequireFailure(t, canvasManagedGroupRequestForTest(t, localLogin.Grant.Token, "default", "removed-local"), http.StatusUnauthorized, "canvas_authorization_invalid")

	revoked := canvasAccountControllerRequest(http.MethodPost, "/api/canvas/revoke", `{}`, localLogin.Grant.Token, PostCanvasRevoke)
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	canvasAccountSetAdditionalClients(t, local)
	canvasAccountRequireFailure(t, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", localLogin.Grant.Token, GetCanvasAccount), http.StatusUnauthorized, "canvas_authorization_invalid")
	assert.Equal(t, http.StatusOK, canvasAccountControllerRequest(http.MethodGet, "/api/canvas/account", "", primary.Grant.Token, GetCanvasAccount).Code)
}

// canvasAccountMultiLocalConfig 返回带客户端密钥的本机附加实例。
func canvasAccountMultiLocalConfig() canvasAccountMultiClientConfig {
	return canvasAccountMultiClientConfig{
		ClientID: "canvas-local", InstanceID: "desktop-local",
		RedirectURI: "http://localhost:5180/v1/auth/newapi/callback", ClientSecret: "local-test-secret",
	}
}

// canvasAccountMultiSiblingConfig 返回同一客户端 ID 下的公开 PKCE 附加实例。
func canvasAccountMultiSiblingConfig() canvasAccountMultiClientConfig {
	return canvasAccountMultiClientConfig{
		ClientID: "canvas-local", InstanceID: "desktop-sibling",
		RedirectURI: "http://127.0.0.1:5181/v1/auth/newapi/callback",
	}
}

// canvasAccountSetAdditionalClients 写入测试使用的附加客户端 JSON。
func canvasAccountSetAdditionalClients(t *testing.T, configs ...canvasAccountMultiClientConfig) {
	t.Helper()
	if configs == nil {
		configs = []canvasAccountMultiClientConfig{}
	}
	encoded, err := common.Marshal(configs)
	require.NoError(t, err)
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", string(encoded))
}

// canvasAccountRemoveAdditionalClients 模拟部署移除全部附加客户端环境变量。
func canvasAccountRemoveAdditionalClients(t *testing.T) {
	t.Helper()
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", "")
}

// canvasAccountAuthorizeMultiClient 完成附加客户端的授权、批准与兑换。
func canvasAccountAuthorizeMultiClient(t *testing.T, identity service.AuthIdentity, config canvasAccountMultiClientConfig, state string) canvasAccountTestLogin {
	t.Helper()
	flow := canvasAccountBeginMultiClient(t, config, state, strings.Repeat("m", 64))
	canvasAccountApproveMultiClient(t, identity, &flow)
	response := canvasAccountExchangeMultiClient(t, flow, flow.Verifier, config)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var decoded canvasAccountTestResponse[canvasAccountTestLogin]
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &decoded))
	require.True(t, decoded.Success)
	return decoded.Data
}

// canvasAccountBeginMultiClient 发起授权并提取不透明 request token。
func canvasAccountBeginMultiClient(t *testing.T, config canvasAccountMultiClientConfig, state, verifier string) canvasAccountMultiFlow {
	t.Helper()
	query := canvasAccountMultiAuthorizeQuery(config, state, verifier)
	response := canvasAccountControllerRequest(http.MethodGet, "/api/canvas/authorize?"+query.Encode(), "", "", GetCanvasAuthorize)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	match := canvasAccountMultiRequestTokenPattern.FindStringSubmatch(response.Body.String())
	require.Len(t, match, 2)
	return canvasAccountMultiFlow{Config: config, Verifier: verifier, RequestToken: match[1]}
}

// canvasAccountApproveMultiClient 使用已有浏览器会话批准 request token。
func canvasAccountApproveMultiClient(t *testing.T, identity service.AuthIdentity, flow *canvasAccountMultiFlow) {
	t.Helper()
	body, err := common.Marshal(canvasAuthorizeApproval{RequestToken: flow.RequestToken})
	require.NoError(t, err)
	response := securityEnrollmentRequest(http.MethodPost, "/api/canvas/authorize", string(body), "", identity, PostCanvasAuthorize)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var decoded canvasAccountTestResponse[struct {
		RedirectURI string `json:"redirect_uri"`
	}]
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &decoded))
	require.True(t, decoded.Success)
	redirect, err := url.Parse(decoded.Data.RedirectURI)
	require.NoError(t, err)
	require.Equal(t, flow.Config.RedirectURI, redirect.Scheme+"://"+redirect.Host+redirect.Path)
	flow.Code = redirect.Query().Get("code")
	require.NotEmpty(t, flow.Code)
}

// canvasAccountExchangeMultiClient 以指定身份字段和 PKCE verifier 兑换授权码。
func canvasAccountExchangeMultiClient(t *testing.T, flow canvasAccountMultiFlow, verifier string, config canvasAccountMultiClientConfig) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(canvasTokenExchangeRequest{
		Code: flow.Code, CodeVerifier: verifier, ClientID: config.ClientID, InstanceID: config.InstanceID,
		RedirectURI: config.RedirectURI, ClientSecret: config.ClientSecret,
	})
	require.NoError(t, err)
	return canvasAccountControllerRequest(http.MethodPost, "/api/canvas/token", string(body), "", PostCanvasToken)
}

// canvasAccountMultiAuthorizeQuery 构造固定 S256 PKCE 授权查询。
func canvasAccountMultiAuthorizeQuery(config canvasAccountMultiClientConfig, state, verifier string) url.Values {
	challenge := sha256.Sum256([]byte(verifier))
	return url.Values{
		"client_id": {config.ClientID}, "instance_id": {config.InstanceID},
		"redirect_uri": {config.RedirectURI}, "state": {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
}

// canvasAccountCloneValues 深拷贝授权查询，避免表格用例相互污染。
func canvasAccountCloneValues(source url.Values) url.Values {
	cloned := make(url.Values, len(source))
	for key, values := range source {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

// canvasAccountRequireFailure 校验统一错误包装中的状态码和公开错误码。
func canvasAccountRequireFailure(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, response.Code, response.Body.String())
	var decoded canvasAccountTestResponse[any]
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &decoded))
	assert.False(t, decoded.Success)
	assert.Equal(t, code, decoded.Code)
}

// canvasAccountGrantUserID 将授权响应中的用户 ID 转为测试用户主键。
func canvasAccountGrantUserID(t *testing.T, login canvasAccountTestLogin) int {
	t.Helper()
	var userID int
	_, err := fmt.Sscan(login.User.ID, &userID)
	require.NoError(t, err)
	return userID
}
