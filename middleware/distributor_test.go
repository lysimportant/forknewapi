package middleware

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelMatchesExpectedTaskPluginUsesGenericChannelSetting(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeTaskPlugin}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "generic-alpha"})

	assert.True(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, "generic-beta"))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, channel, ""))
}

func TestChannelMatchesExpectedTaskPluginUsesPinnedLegacyIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	alpha, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)
	pinnedGeneration := registry.Generation()

	require.NoError(t, registry.Unregister("legacy-alpha"))
	_, err = registry.Register(distributorTaskPluginSource("legacy-beta", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: pinnedGeneration,
		Plugin:     alpha,
	})
	channel := &model.Channel{Type: constant.ChannelTypeKling}

	assert.True(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, channel, "legacy-beta"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha"))
}

func TestChannelMatchesExpectedTaskPluginRejectsUnindexedLegacyChannel(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("legacy-alpha", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "legacy-alpha"))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: 0}, "legacy-alpha"))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeJimeng}, ""))
	assert.False(t, channelMatchesExpectedTaskPlugin(nil, &model.Channel{Type: constant.ChannelTypeKling}, "legacy-alpha"))

	c.Set("expected_task_plugin_key", "legacy-alpha")
	setupErr := SetupContextForSelectedChannel(c, &model.Channel{Type: constant.ChannelTypeJimeng}, "task-model")
	require.NotNil(t, setupErr)
	assert.Contains(t, setupErr.Error(), "does not match")
}

func TestSharedEndpointRebindsToSelectedLegacyProvider(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(distributorEndpointPluginSource("gemini-shared", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(distributorEndpointPluginSource("vertex-shared", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: candidates[0].Plugin})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})
	c.Set("expected_task_plugin_key", candidates[0].Plugin.Meta.Key)

	geminiChannel := &model.Channel{Id: 1, Type: constant.ChannelTypeGemini}
	vertexChannel := &model.Channel{Id: 2, Type: constant.ChannelTypeVertexAi}
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, candidates[0].Plugin.Meta.Key))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, vertexChannel, candidates[0].Plugin.Meta.Key))
	assert.False(t, channelMatchesExpectedTaskPlugin(c, &model.Channel{Type: constant.ChannelTypeKling}, candidates[0].Plugin.Meta.Key))

	require.Nil(t, SetupContextForSelectedChannel(c, vertexChannel, "task-model"))
	pinnedValue, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint)
	require.True(t, exists)
	pinned, ok := pinnedValue.(jsplugin.PinnedEndpoint)
	require.True(t, ok)
	assert.Equal(t, "vertex-shared", pinned.Plugin.Meta.Key)
	assert.Equal(t, "vertex-shared", c.GetString("expected_task_plugin_key"))
	assert.Equal(t, "vertex-shared", c.GetString("task_plugin_key"))
	assert.True(t, channelMatchesExpectedTaskPlugin(c, geminiChannel, "vertex-shared"), "a retry may select another declared provider")
}

func distributorTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType)
}

func distributorEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelType)
}

func TestTokenModelLimitAllowsLegacyAliasAndModifierVariant(t *testing.T) {
	aliasOnly := map[string]bool{"claude-3-7-sonnet-thinking": true}
	assert.True(t, tokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet-thinking"))
	assert.False(t, tokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet"))

	baseOnly := map[string]bool{"claude-3-7-sonnet": true}
	assert.True(t, tokenModelLimitAllows(baseOnly, "claude-3-7-sonnet@thinking:on"))
	assert.True(t, tokenModelLimitAllows(baseOnly, "claude-3-7-sonnet-thinking"))

	wildcard := map[string]bool{"gemini-2.5-flash-thinking-*": true}
	assert.True(t, tokenModelLimitAllows(wildcard, "gemini-2.5-flash-thinking-8192"))
}

func TestTokenModelLimitAllowsExemptAtNameByFullName(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := append([]string(nil), settings.ThinkingModelBlacklist...)
	t.Cleanup(func() { settings.ThinkingModelBlacklist = original })
	settings.ThinkingModelBlacklist = append(original, "re:.*@sha256:.*")

	fullOnly := map[string]bool{"opaque@sha256:deadbeef": true}
	assert.True(t, tokenModelLimitAllows(fullOnly, "opaque@sha256:deadbeef"))

	baseOnly := map[string]bool{"opaque": true}
	assert.False(t, tokenModelLimitAllows(baseOnly, "opaque@sha256:deadbeef"))
}

func TestNoAvailableChannelMessageNamesClaimingTaskPlugin(t *testing.T) {
	require.NoError(t, i18n.Init())
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("claimer", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	pinned, _ := gin.CreateTestContext(nil)
	pinned.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	pinned.Request.Header.Set("Accept-Language", "en")
	pinned.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
	message := noAvailableChannelMessage(pinned, "default", "kling-v1")
	assert.Contains(t, message, `"claimer"`)
	assert.Contains(t, message, "disable or override")
	assert.Contains(t, message, "kling-v1")

	plain, _ := gin.CreateTestContext(nil)
	plain.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plain.Request.Header.Set("Accept-Language", "en")
	generic := noAvailableChannelMessage(plain, "default", "gpt-4o")
	assert.NotContains(t, generic, "task plugin")
	assert.Contains(t, generic, "gpt-4o")
}

func TestSharedEndpointRebindsToSelectedType61Plugin(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"alpha", "beta"} {
		source := strings.Replace(distributorEndpointPluginSource(key, 0), "channelTypes: [0],", "", 1)
		_, err := registry.Register(source, jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: generation, Plugin: candidates[0].Plugin})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: candidates[0].Plugin, Protocol: candidates[0].Protocol, Operation: candidates[0].Operation, Model: "task-model", Candidates: candidates})
	c.Set("expected_task_plugin_key", "alpha")
	channel := &model.Channel{Id: 2, Type: constant.ChannelTypeTaskPlugin}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "unrelated"})
	assert.False(t, channelMatchesExpectedTaskPlugin(c, channel, "alpha"))
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "beta"})
	require.Nil(t, SetupContextForSelectedChannel(c, channel, "task-model"))
	assert.Equal(t, "beta", c.GetString("task_plugin_key"))
	assert.Equal(t, "beta", c.GetString("expected_task_plugin_key"))
	assert.Equal(t, "beta", c.MustGet(jsplugin.ContextKeyPinnedEndpoint).(jsplugin.PinnedEndpoint).Plugin.Meta.Key)
	require.NoError(t, i18n.Init())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	assert.Contains(t, noAvailableChannelMessage(c, "default", "task-model"), "alpha, beta")
}

// TestCanvasExecutionAcceptance 验证管理 Token 在下游计费和供应商处理器执行前完成冻结授权复核。
func TestCanvasExecutionAcceptance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "canvas-acceptance.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Option{},
		&model.CanvasGrant{}, &model.CanvasManagedToken{}, &model.CanvasIdempotencyOperation{},
	))
	model.DB = database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMain, previousLog)
		sqlDB, dbErr := database.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	t.Setenv("CANVAS_ACCOUNT_ENABLED", "true")
	t.Setenv("CANVAS_ACCOUNT_ISSUER", "http://127.0.0.1:13000")
	t.Setenv("CANVAS_ACCOUNT_CLIENT_ID", "canvas")
	t.Setenv("CANVAS_ACCOUNT_INSTANCE_ID", "middleware-test")
	t.Setenv("CANVAS_ACCOUNT_REDIRECT_URI", "http://localhost:5173/v1/auth/newapi/callback")

	now := time.Now().Unix()
	user := model.User{Username: "canvas-acceptance", Password: "test-password", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, database.Create(&user).Error)
	channel := model.Channel{Id: 9101, Name: "canvas-acceptance", Key: "test", Status: common.ChannelStatusEnabled, Models: "canvas-test-model", Group: "default"}
	require.NoError(t, database.Create(&channel).Error)
	require.NoError(t, database.Create(&model.Ability{Group: "default", Model: "canvas-test-model", ChannelId: channel.Id, Enabled: true}).Error)

	grant := model.CanvasGrant{
		ID: "11111111-1111-4111-8111-111111111111", ClientID: "canvas", InstanceID: "middleware-test", UserID: user.Id,
		TokenHash: strings.Repeat("a", 64), Scopes: strings.Join(common.CanvasAccountScopes(), " "), Status: model.CanvasGrantStatusActive,
		IssuedAt: now, ExpiresAt: now + 3600, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, database.Create(&grant).Error)

	ordinaryToken := model.Token{
		UserId: user.Id, Key: "canvas-ordinary", Status: common.TokenStatusEnabled, Name: "Canvas-middleware-default",
		CreatedTime: now, AccessedTime: now, ExpiredTime: grant.ExpiresAt, UnlimitedQuota: true, Group: "default",
	}
	require.NoError(t, database.Create(&ordinaryToken).Error)
	ordinaryManaged := model.CanvasManagedToken{
		GrantID: grant.ID, UserID: user.Id, GroupID: "default", TokenID: ordinaryToken.Id,
		CredentialRevision: 1, Status: model.CanvasManagedTokenStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, database.Create(&ordinaryManaged).Error)
	ordinaryAuthority, err := model.ReconcileCanvasManagedAuthority(ordinaryManaged.ID)
	require.NoError(t, err)

	autoToken := model.Token{
		UserId: user.Id, Key: "canvas-auto", Status: common.TokenStatusEnabled, Name: "Canvas-middleware-auto",
		CreatedTime: now, AccessedTime: now, ExpiredTime: grant.ExpiresAt, UnlimitedQuota: true, Group: "auto",
	}
	require.NoError(t, autoToken.SetAutoGroups([]string{"default", "vip"}))
	require.NoError(t, database.Create(&autoToken).Error)
	autoManaged := model.CanvasManagedToken{
		GrantID: grant.ID, UserID: user.Id, GroupID: "auto", TokenID: autoToken.Id,
		CredentialRevision: 1, AutoGroups: autoToken.AutoGroups, Status: model.CanvasManagedTokenStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, database.Create(&autoManaged).Error)
	autoAuthority, err := model.ReconcileCanvasManagedAuthority(autoManaged.ID)
	require.NoError(t, err)

	ordinaryPayload := canvasExecutionPayload{
		Version: 1, Issuer: "http://127.0.0.1:13000", UserID: strconv.Itoa(user.Id), InstanceID: grant.InstanceID,
		GrantID: grant.ID, TokenID: strconv.Itoa(ordinaryToken.Id), ExpectedGroup: "default",
		PermissionRevision: strconv.FormatInt(ordinaryAuthority.Managed.PermissionRevision, 10), AutoGroups: []string{},
	}
	autoPayload := canvasExecutionPayload{
		Version: 1, Issuer: "http://127.0.0.1:13000", UserID: strconv.Itoa(user.Id), InstanceID: grant.InstanceID,
		GrantID: grant.ID, TokenID: strconv.Itoa(autoToken.Id), ExpectedGroup: "auto",
		PermissionRevision: strconv.FormatInt(autoAuthority.Managed.PermissionRevision, 10), AutoGroups: []string{"default", "vip"},
	}
	duplicateIssuerHeader := encodeCanvasExecutionPayloadForTest(t, ordinaryPayload)
	decodedDuplicateSource, err := base64.RawURLEncoding.DecodeString(duplicateIssuerHeader)
	require.NoError(t, err)
	duplicateIssuerJSON := strings.Replace(string(decodedDuplicateSource), `"issuer":"http://127.0.0.1:13000"`, `"issuer":"http://127.0.0.1:13000","issuer":"http://127.0.0.1:13000"`, 1)
	require.NotEqual(t, string(decodedDuplicateSource), duplicateIssuerJSON)
	duplicateIssuerHeader = base64.RawURLEncoding.EncodeToString([]byte(duplicateIssuerJSON))

	tests := []struct {
		name       string
		method     string
		path       string
		tokenID    int
		usingGroup string
		header     string
		status     int
		downstream bool
	}{
		{name: "ordinary valid", method: http.MethodPost, path: "/v1/chat/completions", tokenID: ordinaryToken.Id, usingGroup: "default", header: encodeCanvasExecutionPayloadForTest(t, ordinaryPayload), status: http.StatusNoContent, downstream: true},
		{name: "auto valid", method: http.MethodPost, path: "/v1/images/generations", tokenID: autoToken.Id, usingGroup: "auto", header: encodeCanvasExecutionPayloadForTest(t, autoPayload), status: http.StatusNoContent, downstream: true},
		{name: "missing header", method: http.MethodPost, path: "/v1/chat/completions", tokenID: ordinaryToken.Id, usingGroup: "default", status: http.StatusBadRequest},
		{name: "padded header", method: http.MethodPost, path: "/v1/chat/completions", tokenID: ordinaryToken.Id, usingGroup: "default", header: encodeCanvasExecutionPayloadForTest(t, ordinaryPayload) + "=", status: http.StatusBadRequest},
		{name: "duplicate json field", method: http.MethodPost, path: "/v1/chat/completions", tokenID: ordinaryToken.Id, usingGroup: "default", header: duplicateIssuerHeader, status: http.StatusBadRequest},
		{name: "issuer mismatch", method: http.MethodPost, path: "/v1/chat/completions", tokenID: ordinaryToken.Id, usingGroup: "default", header: encodeCanvasExecutionPayloadForTest(t, mutateCanvasExecutionPayload(ordinaryPayload, func(payload *canvasExecutionPayload) { payload.Issuer = "http://example.invalid" })), status: http.StatusForbidden},
		{name: "user mismatch", method: http.MethodPost, path: "/v1/chat/completions", tokenID: ordinaryToken.Id, usingGroup: "default", header: encodeCanvasExecutionPayloadForTest(t, mutateCanvasExecutionPayload(ordinaryPayload, func(payload *canvasExecutionPayload) { payload.UserID = strconv.Itoa(user.Id + 1) })), status: http.StatusForbidden},
		{name: "stale permission revision", method: http.MethodPost, path: "/v1/chat/completions", tokenID: ordinaryToken.Id, usingGroup: "default", header: encodeCanvasExecutionPayloadForTest(t, mutateCanvasExecutionPayload(ordinaryPayload, func(payload *canvasExecutionPayload) {
			payload.PermissionRevision = strconv.FormatInt(ordinaryAuthority.Managed.PermissionRevision+1, 10)
		})), status: http.StatusConflict},
		{name: "auto scope mismatch", method: http.MethodPost, path: "/v1/images/generations", tokenID: autoToken.Id, usingGroup: "auto", header: encodeCanvasExecutionPayloadForTest(t, mutateCanvasExecutionPayload(autoPayload, func(payload *canvasExecutionPayload) { payload.AutoGroups = []string{"vip", "default"} })), status: http.StatusForbidden},
		{name: "managed post outside creation paths", method: http.MethodPost, path: "/v1/videos/task-1", tokenID: ordinaryToken.Id, usingGroup: "default", header: encodeCanvasExecutionPayloadForTest(t, ordinaryPayload), status: http.StatusBadRequest},
		{name: "video polling without header", method: http.MethodGet, path: "/v1/videos/task-1", tokenID: ordinaryToken.Id, usingGroup: "default", status: http.StatusNoContent, downstream: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(test.method, test.path, nil)
			if test.header != "" {
				c.Request.Header.Set(canvasExecutionHeader, test.header)
			}
			common.SetContextKey(c, constant.ContextKeyTokenId, test.tokenID)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, test.usingGroup)
			if test.usingGroup == "auto" {
				common.SetContextKey(c, constant.ContextKeyAutoGroup, "default")
			}
			downstream := validateCanvasExecutionAcceptance(c, &channel, "canvas-test-model")
			if downstream {
				c.Status(http.StatusNoContent)
				c.Writer.WriteHeaderNow()
			}
			assert.Equal(t, test.status, response.Code)
			assert.Equal(t, test.downstream, downstream)
		})
	}
}

// encodeCanvasExecutionPayloadForTest 生成与生产客户端相同的无填充 base64url JSON 头。
func encodeCanvasExecutionPayloadForTest(t *testing.T, payload canvasExecutionPayload) string {
	t.Helper()
	encoded, err := common.Marshal(payload)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// mutateCanvasExecutionPayload 复制基础载荷并应用单一测试变更。
func mutateCanvasExecutionPayload(payload canvasExecutionPayload, mutate func(*canvasExecutionPayload)) canvasExecutionPayload {
	mutate(&payload)
	return payload
}
