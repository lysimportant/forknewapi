package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// setupCanvasBridgeDB 创建隔离数据库表并恢复全局配置；三库使用同一实际授权和计费测试。
func setupCanvasBridgeDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	var dialector gorm.Dialector
	switch dialect {
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			if os.Getenv("CANVAS_REQUIRE_DATABASE_MATRIX") == "true" {
				t.Fatal("TEST_MYSQL_DSN is required")
			}
			t.Skip("TEST_MYSQL_DSN is required for the external database matrix")
		}
		require.Contains(t, dsn, "canvas_catalog_test")
		dialector = mysql.Open(dsn)
		common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			if os.Getenv("CANVAS_REQUIRE_DATABASE_MATRIX") == "true" {
				t.Fatal("TEST_POSTGRES_DSN is required")
			}
			t.Skip("TEST_POSTGRES_DSN is required for the external database matrix")
		}
		require.Contains(t, dsn, "canvas_catalog_test")
		dialector = postgres.Open(dsn)
		common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	default:
		dialector = sqlite.Open(filepath.Join(t.TempDir(), "catalog.sqlite"))
		common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	}
	previousMaster, previousPath := common.IsMasterNode, common.SQLitePath
	common.IsMasterNode = false
	t.Setenv("LOG_SQL_DSN", "")
	switch dialect {
	case "mysql":
		t.Setenv("SQL_DSN", os.Getenv("TEST_MYSQL_DSN"))
	case "postgres":
		t.Setenv("SQL_DSN", os.Getenv("TEST_POSTGRES_DSN"))
	default:
		t.Setenv("SQL_DSN", "local")
		common.SQLitePath = filepath.Join(t.TempDir(), "initialization.sqlite")
	}
	require.NoError(t, model.InitDB())
	initializedSQL, err := model.DB.DB()
	require.NoError(t, err)
	require.NoError(t, initializedSQL.Close())
	common.IsMasterNode, common.SQLitePath = previousMaster, previousPath
	prefix := fmt.Sprintf("canvas_catalog_%d_", time.Now().UnixNano())
	db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		for _, table := range []any{&model.Token{}, &model.Ability{}, &model.Channel{}, &model.Model{}, &model.User{}} {
			require.NoError(t, db.Migrator().DropTable(table))
		}
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled = previousRedis
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	// 随机前缀只清理本次创建的表，避免删除同一测试库的其他验收数据。
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Token{}))
	t.Setenv("CANVAS_BRIDGE_ENABLED", "true")
	previousPrice := ratio_setting.ModelPrice2JSONString()
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousSpecial := ratio_setting.GroupGroupRatio2JSONString()
	previousUnits, previousRate := common.QuotaPerUnit, operation_setting.USDExchangeRate
	previousSelfUse := operation_setting.SelfUseModeEnabled
	previousUsable := setting.GetUserUsableGroupsCopy()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrice))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousSpecial))
		encoded, err := common.Marshal(previousUsable)
		require.NoError(t, err)
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(encoded)))
		common.QuotaPerUnit, operation_setting.USDExchangeRate = previousUnits, previousRate
		operation_setting.SelfUseModeEnabled = previousSelfUse
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2,"secret":9}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"vip":1.5}}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","auto":"Auto"}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"Canvas-Text":0.02,"canvas-text":0.03,"canvas-image":0.1,"gpt-image-auto":0.1,"canvas-audio":0.02,"canvas-unprofiled":0.02,"canvas-hidden":0.02,"canvas-other-group":0.02,"canvas-free":0,"canvas-video":0.1}`))
	common.QuotaPerUnit, operation_setting.USDExchangeRate = 500000, 7.3
	operation_setting.SelfUseModeEnabled = false
	require.NoError(t, i18n.Init())
	return db
}

// canvasFixtureChannel 为给定精确模型建立真实渠道和能力行，不使用渠道缓存或模拟查询。
func canvasFixtureChannel(t *testing.T, db *gorm.DB, group string, names ...string) *model.Channel {
	t.Helper()
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "synthetic-channel-secret", Status: common.ChannelStatusEnabled, Name: "Canvas fixture", Models: strings.Join(names, ","), Group: group}
	require.NoError(t, db.Create(channel).Error)
	for _, name := range names {
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: name, ChannelId: channel.Id, Enabled: true}).Error)
	}
	return channel
}

// canvasBridgeRequest 通过真实 TokenAuth 执行目录或预估，返回原始响应以验证不泄密。
func canvasBridgeRequest(t *testing.T, router http.Handler, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := common.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Authorization", "Bearer sk-"+key)
	}
	request.RemoteAddr = "192.0.2.9:7890"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.NotContains(t, response.Body.String(), "synthetic-channel-secret")
	if key != "" {
		assert.NotContains(t, response.Body.String(), key)
	}
	return response
}

// TestCanvasCatalogProfilesFromAuthorizedCandidates 验证目录只从当前 Key 的候选渠道推导，并保留显式元数据的失败关闭语义。
func TestCanvasCatalogProfilesFromAuthorizedCandidates(t *testing.T) {
	textCandidate := canvasCandidate{Channel: &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI}}
	generation := jsplugin.DefaultRegistry.Generation()
	sharedTaskModelTextCandidate := canvasCandidate{Channel: &model.Channel{Id: 5, Type: constant.ChannelTypeOpenAI}, Generation: generation}
	videoPlugin, found := generation.Get("hailuo")
	require.True(t, found)
	videoCandidate := canvasCandidate{
		Channel: &model.Channel{Id: 4, Type: constant.ChannelTypeTaskPlugin}, Plugin: videoPlugin,
		Generation: generation, UpstreamModel: "MiniMax-H3",
	}
	advancedImageCandidate := canvasCandidate{
		Channel: &model.Channel{Id: 2, Type: constant.ChannelTypeAdvancedCustom},
		OtherSettings: dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
			{IncomingPath: "/v1/images/generations", Models: []string{"advanced-image", "mixed-model"}},
		}}},
	}
	for _, test := range []struct {
		name       string
		modelName  string
		metadata   model.Model
		candidates []canvasCandidate
		contracts  []string
	}{
		{name: "openai text without metadata", modelName: "plain-model", candidates: []canvasCandidate{textCandidate}, contracts: []string{"openai-chat-completions"}},
		{name: "openai image without metadata", modelName: "gpt-image-auto", candidates: []canvasCandidate{textCandidate}, contracts: []string{"openai-images"}},
		{name: "empty metadata object", modelName: "plain-model", metadata: model.Model{Endpoints: `{}`}, candidates: []canvasCandidate{textCandidate}, contracts: []string{"openai-chat-completions"}},
		{name: "explicit endpoint type array", modelName: "plain-model", metadata: model.Model{Endpoints: `["image-generation"]`}, candidates: []canvasCandidate{textCandidate}, contracts: []string{"openai-images"}},
		{name: "explicit image overrides inferred text", modelName: "plain-model", metadata: model.Model{Endpoints: `{"image-generation":"/v1/images/generations"}`}, candidates: []canvasCandidate{textCandidate}, contracts: []string{"openai-images"}},
		{name: "explicit text resolves candidate ambiguity", modelName: "mixed-model", metadata: model.Model{Endpoints: `{"openai":"/v1/chat/completions"}`}, candidates: []canvasCandidate{textCandidate, advancedImageCandidate}, contracts: []string{"openai-chat-completions"}},
		{name: "explicit ambiguity", modelName: "plain-model", metadata: model.Model{Endpoints: `{"openai":"/v1/chat/completions","image-generation":"/v1/images/generations"}`}, candidates: []canvasCandidate{textCandidate}, contracts: []string{"openai-chat-completions", "openai-images"}},
		{name: "invalid metadata", modelName: "plain-model", metadata: model.Model{Endpoints: `{invalid`}, candidates: []canvasCandidate{textCandidate}},
		{name: "partially invalid endpoint array", modelName: "plain-model", metadata: model.Model{Endpoints: `["openai",1]`}, candidates: []canvasCandidate{textCandidate}},
		{name: "partially invalid endpoint object", modelName: "plain-model", metadata: model.Model{Endpoints: `{"openai":"/v1/chat/completions","broken":1}`}, candidates: []canvasCandidate{textCandidate}},
		{name: "invalid method type", modelName: "plain-model", metadata: model.Model{Endpoints: `{"openai":{"path":"/v1/chat/completions","method":1}}`}, candidates: []canvasCandidate{textCandidate}},
		{name: "explicit get method", modelName: "plain-model", metadata: model.Model{Endpoints: `{"openai":{"path":"/v1/chat/completions","method":"GET"}}`}, candidates: []canvasCandidate{textCandidate}},
		{name: "unsupported explicit responses protocol", modelName: "plain-model", metadata: model.Model{Endpoints: `{"openai-response":"/v1/responses"}`}, candidates: []canvasCandidate{textCandidate}},
		{name: "video without metadata", modelName: "MiniMax-H3", candidates: []canvasCandidate{videoCandidate}, contracts: []string{"newapi-video-v1"}},
		{name: "explicit video endpoint type array", modelName: "MiniMax-H3", metadata: model.Model{Endpoints: `["openai-video"]`}, candidates: []canvasCandidate{videoCandidate}, contracts: []string{"newapi-video-v1"}},
		{name: "explicit text does not relabel video candidate", modelName: "MiniMax-H3", metadata: model.Model{Endpoints: `{"openai":"/v1/chat/completions"}`}, candidates: []canvasCandidate{videoCandidate}},
		{name: "explicit text resolves text and video candidates", modelName: "MiniMax-H3", metadata: model.Model{Endpoints: `{"openai":"/v1/chat/completions"}`}, candidates: []canvasCandidate{textCandidate, videoCandidate}, contracts: []string{"openai-chat-completions"}},
		{name: "unrelated channel may expose a shared task model as text", modelName: "MiniMax-H3", candidates: []canvasCandidate{sharedTaskModelTextCandidate}, contracts: []string{"openai-chat-completions"}},
		{name: "advanced custom image", modelName: "advanced-image", candidates: []canvasCandidate{advancedImageCandidate}, contracts: []string{"openai-images"}},
		{name: "advanced custom missing config", modelName: "advanced-image", candidates: []canvasCandidate{{Channel: &model.Channel{Id: 3, Type: constant.ChannelTypeAdvancedCustom}}}},
		{name: "advanced custom model mismatch", modelName: "other-model", candidates: []canvasCandidate{advancedImageCandidate}},
		{name: "candidate ambiguity", modelName: "mixed-model", candidates: []canvasCandidate{textCandidate, advancedImageCandidate}, contracts: []string{"openai-chat-completions", "openai-images"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			profiles := canvasCatalogProfiles(test.metadata, test.candidates, test.modelName)
			contracts := make([]string, 0, len(profiles))
			for _, profile := range profiles {
				contracts = append(contracts, profile.Contract)
			}
			if test.contracts == nil {
				assert.Empty(t, contracts)
				return
			}
			assert.Equal(t, test.contracts, contracts)
		})
	}
}

// TestCanvasModelPricingVersionTracksAdvancedCustomEndpoints 验证高级自定义路由改变目录合同时会刷新价格版本。
func TestCanvasModelPricingVersionTracksAdvancedCustomEndpoints(t *testing.T) {
	candidate := canvasCandidate{
		Group: "default", UpstreamModel: "advanced-model",
		Channel: &model.Channel{Id: 1, Type: constant.ChannelTypeAdvancedCustom},
		OtherSettings: dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
			{IncomingPath: "/v1/chat/completions", Models: []string{"advanced-model"}},
		}}},
	}
	textVersion := canvasModelPricingVersion("base", "advanced-model", "default", model.Model{}, []canvasCandidate{candidate})
	candidate.OtherSettings.AdvancedCustom.Routes[0].IncomingPath = "/v1/images/generations"
	imageVersion := canvasModelPricingVersion("base", "advanced-model", "default", model.Model{}, []canvasCandidate{candidate})
	assert.NotEqual(t, textVersion, imageVersion)
	candidate.PluginManaged = true
	pluginManagedVersion := canvasModelPricingVersion("base", "advanced-model", "default", model.Model{}, []canvasCandidate{candidate})
	assert.NotEqual(t, imageVersion, pluginManagedVersion)
}

// canvasEstimatePayload 只解析面向 Canvas 的额度和元数据，不复算表达式或操纵钱包。
type canvasEstimatePayload struct {
	EstimatedQuota string `json:"estimated_quota"`
	QuotaPerUnit   string `json:"quota_per_unit"`
	USDToCNY       string `json:"usd_to_cny"`
	PricingVersion string `json:"pricing_version"`
	Group          string `json:"group"`
	EstimateOnly   bool   `json:"estimate_only"`
	ExpiresAt      string `json:"expires_at"`
}

// TestCanvasBridgeDatabaseMatrix 验证三库相同的权限、目录、估算和不扣余额合同。
func TestCanvasBridgeDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupCanvasBridgeDB(t, dialect)
			user := &model.User{Username: "canvas-owner", Password: "placeholder", Status: common.UserStatusEnabled, Group: "default", Quota: 900000, AffCode: "canvasowner"}
			require.NoError(t, db.Create(user).Error)
			token := &model.Token{UserId: user.Id, Key: strings.Repeat("b", 48), Status: common.TokenStatusEnabled, RemainQuota: 700000, ExpiredTime: -1}
			require.NoError(t, db.Create(token).Error)
			canvasFixtureChannel(t, db, "default", "Canvas-Text", "canvas-image", "gpt-image-auto", "canvas-audio", "canvas-unprofiled", "canvas-free", "canvas-unpriced", "canvas-hidden")
			canvasFixtureChannel(t, db, "default", "canvas-text")
			canvasFixtureChannel(t, db, "secret", "canvas-other-group")
			canvasFixtureChannel(t, db, "vip", "Canvas-Text")
			longModel, oversizedModel := strings.Repeat("中", 100), strings.Repeat("中", 171)
			canvasFixtureChannel(t, db, "default", longModel, oversizedModel)
			prices := ratio_setting.GetModelPriceCopy()
			prices[longModel], prices[oversizedModel] = 0.02, 0.02
			encodedPrices, err := common.Marshal(prices)
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(encodedPrices)))
			for _, meta := range []model.Model{
				{ModelName: longModel, Status: 1, Endpoints: `{"openai":"/v1/chat/completions"}`},
				{ModelName: "Canvas-Text", Status: 1, Endpoints: `{"openai":"/v1/chat/completions"}`},
				{ModelName: "canvas-image", Status: 1, Endpoints: `{"image":"/v1/images/generations"}`},
				{ModelName: "canvas-audio", Status: 1, Endpoints: `{"audio":"/v1/audio/speech"}`},
				{ModelName: "canvas-unprofiled", Status: 1, Endpoints: `{"openai":"/v1/chat/completions","image":"/v1/images/generations"}`},
				{ModelName: "canvas-unpriced", Status: 1, Endpoints: `{"openai":"/v1/chat/completions"}`},
				{ModelName: "canvas-hidden", Status: 2, Endpoints: `{"openai":"/v1/chat/completions"}`},
			} {
				require.NoError(t, db.Create(&meta).Error)
			}
			router := gin.New()
			router.GET("/v1/canvas/catalog", middleware.TokenAuth(), GetCanvasCatalog)
			router.POST("/v1/canvas/estimate", middleware.TokenAuth(), EstimateCanvasPrice)
			assert.Equal(t, http.StatusUnauthorized, canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil).Code)
			assert.Equal(t, http.StatusUnauthorized, canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", strings.Repeat("c", 48), nil).Code)
			response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", token.Key, nil)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			var catalog struct {
				Currency string               `json:"currency"`
				Models   []canvasCatalogModel `json:"models"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			assert.Equal(t, "CNY", catalog.Currency)
			items := make(map[string]canvasCatalogModel)
			for _, item := range catalog.Models {
				items[item.ID] = item
			}
			assert.Len(t, items, 11)
			assert.True(t, items[longModel].Available)
			assert.False(t, items[oversizedModel].Available)
			assert.Equal(t, "invalid_model_id", items[oversizedModel].UnavailableReason)
			assert.True(t, items["Canvas-Text"].Available)
			assert.True(t, items["canvas-text"].Available)
			assert.Equal(t, "openai-chat-completions", items["canvas-text"].Contract)
			assert.Equal(t, "openai-images", items["canvas-image"].Contract)
			assert.True(t, items["gpt-image-auto"].Available)
			assert.Equal(t, "image", items["gpt-image-auto"].MediaType)
			assert.Equal(t, "openai-images", items["gpt-image-auto"].Contract)
			assert.Equal(t, "openai-audio", items["canvas-audio"].Contract)
			assert.Equal(t, "missing_profile", items["canvas-unprofiled"].UnavailableReason)
			assert.Equal(t, "missing_pricing", items["canvas-unpriced"].UnavailableReason)
			assert.Equal(t, "model_disabled", items["canvas-hidden"].UnavailableReason)
			assert.NotContains(t, items, "canvas-other-group")
			var firstVersion string
			for _, test := range []struct {
				name, contract, quota string
				parameters            map[string]any
			}{
				{"Canvas-Text", "openai-chat-completions", "10000", map[string]any{"max_tokens": 100}},
				{longModel, "openai-chat-completions", "10000", nil},
				{"canvas-text", "openai-chat-completions", "15000", nil},
				{"canvas-unprofiled", "openai-chat-completions", "10000", nil},
				{"canvas-image", "openai-images", "100000", map[string]any{"n": 2}},
				{"canvas-audio", "openai-audio", "10000", map[string]any{"voice": "alloy"}},
				{"canvas-free", "openai-chat-completions", "0", nil},
			} {
				response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", token.Key, canvasEstimateRequest{Model: test.name, Contract: test.contract, Parameters: test.parameters, InputText: "Write a short story", InputPending: true})
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var result canvasEstimatePayload
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
				assert.Equal(t, test.quota, result.EstimatedQuota, test.name)
				assert.Equal(t, "500000", result.QuotaPerUnit)
				assert.Equal(t, "7.3", result.USDToCNY)
				assert.True(t, result.EstimateOnly)
				_, err := time.Parse(time.RFC3339, result.ExpiresAt)
				require.NoError(t, err)
				if test.name == "Canvas-Text" {
					firstVersion = result.PricingVersion
					assert.Equal(t, items["Canvas-Text"].PricingVersion, firstVersion)
				}
			}
			for _, test := range []struct {
				body   any
				status int
			}{
				{canvasEstimateRequest{Model: "canvas-other-group", Contract: "openai-chat-completions"}, http.StatusForbidden},
				{canvasEstimateRequest{Model: oversizedModel, Contract: "openai-chat-completions"}, http.StatusBadRequest},
				{canvasEstimateRequest{Model: "invalid\nmodel", Contract: "openai-chat-completions"}, http.StatusBadRequest},
				{canvasEstimateRequest{Model: "Canvas-Text", Contract: "unsupported"}, http.StatusBadRequest},
				{canvasEstimateRequest{Model: "Canvas-Text", Contract: "openai-chat-completions", Parameters: map[string]any{"url": "https://example.com"}}, http.StatusBadRequest},
				{canvasEstimateRequest{Model: "canvas-image", Contract: "openai-images", Parameters: map[string]any{"n": 129}}, http.StatusBadRequest},
				{canvasEstimateRequest{Model: "Canvas-Text", Contract: "openai-chat-completions", Parameters: map[string]any{"max_tokens": 18446744073709551615.0}}, http.StatusBadRequest},
				{canvasEstimateRequest{Model: "Canvas-Text", Contract: "openai-chat-completions", Parameters: map[string]any{"temperature": true}}, http.StatusBadRequest},
				{map[string]any{"model": "Canvas-Text", "contract": "openai-chat-completions", "price": 0}, http.StatusBadRequest},
				{map[string]any{"model": "Canvas-Text", "contract": "openai-chat-completions", "input_media": []any{}}, http.StatusBadRequest},
			} {
				response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", token.Key, test.body)
				assert.Equal(t, test.status, response.Code, response.Body.String())
			}
			for _, change := range []map[string]any{
				{"status": common.TokenStatusDisabled},
				{"status": common.TokenStatusEnabled, "expired_time": time.Now().Unix() - 60},
				{"expired_time": -1, "remain_quota": 0},
			} {
				require.NoError(t, db.Model(token).Updates(change).Error)
				assert.Equal(t, http.StatusUnauthorized, canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", token.Key, nil).Code)
			}
			require.NoError(t, db.Model(token).Updates(map[string]any{"remain_quota": 700000, "status": common.TokenStatusEnabled, "allow_ips": "198.51.100.0/24"}).Error)
			assert.Equal(t, http.StatusForbidden, canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", token.Key, nil).Code)
			require.NoError(t, db.Model(token).Update("allow_ips", "").Error)
			require.NoError(t, db.Model(token).Updates(map[string]any{"group": "auto", "auto_groups": `["vip"]`, "model_limits_enabled": true, "model_limits": "Canvas-Text"}).Error)
			response = canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", token.Key, nil)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.Equal(t, "Canvas-Text", catalog.Models[0].ID)
			response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", token.Key, canvasEstimateRequest{Model: "Canvas-Text", Contract: "openai-chat-completions"})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var special canvasEstimatePayload
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &special))
			assert.Equal(t, "15000", special.EstimatedQuota)
			assert.Equal(t, "vip", special.Group)
			assert.NotEqual(t, firstVersion, special.PricingVersion)
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"Canvas-Text":0.04}`))
			response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", token.Key, canvasEstimateRequest{Model: "Canvas-Text", Contract: "openai-chat-completions"})
			var changed canvasEstimatePayload
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &changed))
			assert.Equal(t, "30000", changed.EstimatedQuota)
			assert.NotEqual(t, special.PricingVersion, changed.PricingVersion)
			var savedUser model.User
			var savedToken model.Token
			require.NoError(t, db.First(&savedUser, user.Id).Error)
			require.NoError(t, db.First(&savedToken, token.Id).Error)
			assert.Equal(t, 900000, savedUser.Quota)
			assert.Equal(t, 700000, savedToken.RemainQuota)
			assert.Zero(t, savedToken.UsedQuota)
			t.Setenv("CANVAS_BRIDGE_ENABLED", "false")
			assert.Equal(t, http.StatusNotFound, canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", token.Key, nil).Code)
		})
	}
}

// TestCanvasBridgeExpressionAndAlias 使用原价格引擎验证推理后缀、固定请求价和原始图片参数。
func TestCanvasBridgeExpressionAndAlias(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	canvasFixtureChannel(t, db, "default", "Canvas-Text@effort:high", "canvas-expression", "canvas-image-expression", "canvas-unpriced")
	for _, metadata := range []model.Model{
		{ModelName: "Canvas-Text@effort:high", Status: 1, Endpoints: `{"openai":"/v1/chat/completions"}`},
		{ModelName: "canvas-unpriced", Status: 1, Endpoints: `{"openai":"/v1/chat/completions"}`},
	} {
		require.NoError(t, db.Create(&metadata).Error)
	}
	withTieredBillingConfig(t, map[string]string{"canvas-expression": "tiered_expr", "canvas-image-expression": "tiered_expr"}, map[string]string{
		"canvas-expression":       `tier("request", fixed(0.03))`,
		"canvas-image-expression": `param("aspect_ratio") == "16:9" ? tier("wide", fixed(0.04)) : tier("normal", fixed(0.02))`,
	})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		c.Next()
	})
	router.GET("/v1/canvas/catalog", GetCanvasCatalog)
	router.POST("/v1/canvas/estimate", EstimateCanvasPrice)
	operation_setting.SelfUseModeEnabled = true
	response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var catalog struct {
		Models []canvasCatalogModel `json:"models"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
	for _, item := range catalog.Models {
		if item.ID == "Canvas-Text@effort:high" {
			assert.True(t, item.Available)
		}
		if item.ID == "canvas-unpriced" {
			assert.Equal(t, "missing_pricing", item.UnavailableReason)
		}
	}
	for _, test := range []struct {
		model, contract, quota string
		parameters             map[string]any
	}{
		{"Canvas-Text@effort:high", "openai-chat-completions", "10000", nil},
		{"canvas-expression", "openai-chat-completions", "15000", nil},
		{"canvas-image-expression", "openai-images", "20000", map[string]any{"aspect_ratio": "16:9"}},
	} {
		response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{Model: test.model, Contract: test.contract, Parameters: test.parameters, InputPending: true})
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result canvasEstimatePayload
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.Equal(t, test.quota, result.EstimatedQuota)
	}
	channel := canvasFixtureChannel(t, db, "default", "invalid-channel-settings")
	require.NoError(t, db.Model(channel).Update("settings", "{malformed").Error)
	response = canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	var saved model.Channel
	require.NoError(t, db.First(&saved, channel.Id).Error)
	assert.Equal(t, "{malformed", saved.OtherSettings, "a read-only catalog must never repair or overwrite channel configuration")
}

// TestCanvasBridgePluginPricing 验证视频只执行解码和原用量引擎，不调用提交、查询或内容网络 hook。
func TestCanvasBridgePluginPricing(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	source := `
export const meta = {apiVersion:1,key:"canvas-bridge-test",name:"Canvas test",version:"1.0.0",author:{name:"Test"},models:["canvas-video","canvas-not-video"],fetchMode:"per_task",protocols:[{name:"openai_video",models:["canvas-video"]}],usageSchema:{seconds:{type:"number",unit:"second",description:{en:"Video unit price",zh:"视频单价"}}}};
export const protocols = {openai_video:{decodeRequest(ctx){ const body=ctx.body.value; if (body.seconds===13) throw new Error("unsupported duration"); return {kind:"submit",model:ctx.model,action:"generate",requestBody:body}; },render(){return {};}}};
export function extractUsage(ctx){return {seconds:Number(ctx.requestBody.seconds || 4)};}
export function buildSubmitRequest(){throw new Error("must never submit");}
export function parseSubmitResponse(){throw new Error("must never submit");}
export function buildQueryRequest(){throw new Error("must never query");}
export function parseTaskResult(){throw new Error("must never query");}
export function listArtifacts(){throw new Error("must never query");}
export function buildContentRequest(){throw new Error("must never query");}
`
	_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("canvas-bridge-test") })
	channel := canvasFixtureChannel(t, db, "default", "canvas-video", "canvas-not-video")
	require.NoError(t, db.Model(channel).Updates(map[string]any{"type": constant.ChannelTypeTaskPlugin, "setting": `{"task_plugin_key":"canvas-bridge-test"}`}).Error)
	withTieredBillingConfig(t, map[string]string{"canvas-video": "tiered_expr"}, map[string]string{"canvas-video": `u("seconds") * 0.2`})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		c.Next()
	})
	router.GET("/v1/canvas/catalog", GetCanvasCatalog)
	router.POST("/v1/canvas/estimate", EstimateCanvasPrice)
	response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"contract":"newapi-video-v1"`)
	assert.Contains(t, response.Body.String(), `"unavailable_reason":"missing_profile"`)
	assert.NotContains(t, response.Body.String(), `u(\"seconds\")`)
	for _, test := range []struct {
		seconds int
		status  int
		quota   string
	}{{5, http.StatusOK, "500000"}, {13, http.StatusUnprocessableEntity, ""}, {3601, http.StatusBadRequest, ""}, {-1, http.StatusBadRequest, ""}} {
		response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{Model: "canvas-video", Contract: "newapi-video-v1", Parameters: map[string]any{"seconds": test.seconds}, InputText: "A bird riding a bicycle"})
		require.Equal(t, test.status, response.Code, response.Body.String())
		if test.status == http.StatusOK {
			assert.Contains(t, response.Body.String(), fmt.Sprintf(`"estimated_quota":"%s"`, test.quota))
		}
	}
}

// TestCanvasBridgeVideoAliasContract 验证目录和估算仅开放真实视频入口能够固定的别名，隐藏组的冲突同样生效。
func TestCanvasBridgeVideoAliasContract(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupCanvasBridgeDB(t, dialect)
			source := `
export const meta = {apiVersion:1,key:"canvas-alias-test",name:"Canvas alias test",version:"1.0.0",author:{name:"Test"},models:["canvas-video-one","canvas-video-two"],fetchMode:"per_task",protocols:["openai_video"]};
export const protocols = {openai_video:{decodeRequest(ctx){return {kind:"submit",model:ctx.model,action:"text_to_video",requestBody:{prompt:ctx.body.value.prompt,normalized:true}};},render(){return {};}}};
export function buildSubmitRequest(ctx){if (!ctx.requestBody.normalized) throw new Error("protocol decoder required"); return {url:ctx.baseUrl+"/videos",method:"POST",body:ctx.requestBody};}
export function parseSubmitResponse(){throw new Error("must never submit");}
export function buildQueryRequest(){throw new Error("must never query");}
export function parseTaskResult(){throw new Error("must never query");}
export function listArtifacts(){throw new Error("must never query");}
export function buildContentRequest(){throw new Error("must never query");}
`
			plugin, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, jsplugin.DefaultRegistry.Unregister("canvas-alias-test")) })
			channel := canvasFixtureChannel(t, db, "default", "canvas-video-alias", "canvas-video-conflict")
			require.NoError(t, db.Model(channel).Updates(map[string]any{"type": constant.ChannelTypeTaskPlugin, "setting": `{"task_plugin_key":"canvas-alias-test"}`, "model_mapping": `{"canvas-video-alias":"canvas-video-one","canvas-video-conflict":"canvas-video-one"}`}).Error)
			other := canvasFixtureChannel(t, db, "secret", "canvas-video-conflict")
			require.NoError(t, db.Model(other).Updates(map[string]any{"type": constant.ChannelTypeTaskPlugin, "setting": `{"task_plugin_key":"canvas-alias-test"}`, "model_mapping": `{"canvas-video-conflict":"canvas-video-two"}`}).Error)
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"canvas-video-alias":0.1,"canvas-video-conflict":0.1}`))
			router := gin.New()
			router.Use(func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				c.Next()
			})
			router.GET("/v1/canvas/catalog", GetCanvasCatalog)
			router.POST("/v1/canvas/estimate", EstimateCanvasPrice)
			router.POST("/v1/videos", middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), func(c *gin.Context) {
				var body map[string]any
				require.NoError(t, common.UnmarshalBodyReusable(c, &body))
				name, _ := body["model"].(string)
				info := &relaycommon.RelayInfo{OriginModelName: name, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "canvas-video-one", ChannelBaseUrl: "https://canvas.example"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
				adaptor := taskplugin.New(plugin)
				adaptor.Init(info)
				if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
					c.JSON(taskErr.StatusCode, gin.H{"code": taskErr.Code})
					return
				}
				c.Status(http.StatusNoContent)
			})
			for _, test := range []struct {
				name   string
				status int
			}{{"canvas-video-alias", http.StatusNoContent}, {"canvas-video-conflict", http.StatusBadRequest}} {
				response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/videos", "", map[string]any{"model": test.name, "prompt": "A bird riding a bicycle"})
				require.Equal(t, test.status, response.Code, response.Body.String())
			}
			response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var catalog struct {
				Models []canvasCatalogModel `json:"models"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 2)
			assert.Equal(t, "canvas-video-alias", catalog.Models[0].ID)
			assert.True(t, catalog.Models[0].Available)
			assert.Equal(t, "canvas-video-conflict", catalog.Models[1].ID)
			assert.False(t, catalog.Models[1].Available)
			assert.Equal(t, "missing_profile", catalog.Models[1].UnavailableReason)
			for _, test := range []struct {
				name   string
				status int
			}{{"canvas-video-alias", http.StatusOK}, {"canvas-video-conflict", http.StatusUnprocessableEntity}} {
				response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{Model: test.name, Contract: "newapi-video-v1", Parameters: map[string]any{"seconds": 5}, InputText: "A bird riding a bicycle"})
				assert.Equal(t, test.status, response.Code, response.Body.String())
			}
		})
	}
}

// TestCanvasBridgeHailuoMappedH3 验证官方协议的 h3 上游名称保留对外模型及价格，目录、预算和视频入口一致。
// 使用临时 SQLite 和内置插件，只构造请求描述，不访问上游或扣减账户余额。
func TestCanvasBridgeHailuoMappedH3(t *testing.T) {
	for _, test := range []struct {
		channelType int
		upstream    string
	}{
		{constant.ChannelTypeMiniMax, "MiniMax-H3"},
		{constant.ChannelTypeMiniMax, "h3"},
		{constant.ChannelTypeTaskPlugin, "MiniMax-H3"},
		{constant.ChannelTypeTaskPlugin, "h3"},
	} {
		t.Run(fmt.Sprintf("%d/%s", test.channelType, test.upstream), func(t *testing.T) {
			db := setupCanvasBridgeDB(t, "sqlite")
			channel := canvasFixtureChannel(t, db, "default", "MiniMax-H3")
			require.NoError(t, db.Model(channel).Updates(map[string]any{
				"type": test.channelType, "setting": `{"task_plugin_key":"hailuo"}`,
				"model_mapping": fmt.Sprintf(`{"MiniMax-H3":%q}`, test.upstream),
			}).Error)
			withTieredBillingConfig(t, map[string]string{"MiniMax-H3": "tiered_expr"}, map[string]string{
				"MiniMax-H3": `u("seconds") * 0.2 + u("input_images") * 0.001 + u("input_video_seconds") * 0.002`,
			})
			plugin, found := jsplugin.DefaultRegistry.Generation().Get("hailuo")
			require.True(t, found)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				c.Next()
			})
			router.GET("/v1/canvas/catalog", GetCanvasCatalog)
			router.POST("/v1/canvas/estimate", EstimateCanvasPrice)
			router.POST("/v1/videos", middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), func(c *gin.Context) {
				info := &relaycommon.RelayInfo{OriginModelName: "MiniMax-H3", ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: test.upstream, ChannelBaseUrl: "https://canvas.example",
				}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
				adaptor := taskplugin.New(plugin)
				adaptor.Init(info)
				if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
					c.JSON(taskErr.StatusCode, gin.H{"code": taskErr.Code})
					return
				}
				c.Status(http.StatusNoContent)
			})
			response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
			require.Equal(t, http.StatusOK, response.Code)
			var catalog struct {
				Models []canvasCatalogModel `json:"models"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.Equal(t, "MiniMax-H3", catalog.Models[0].ID)
			assert.True(t, catalog.Models[0].Available)
			assert.Equal(t, "newapi-video-v1", catalog.Models[0].Contract)
			assert.Empty(t, catalog.Models[0].UnavailableReason)
			assert.Equal(t, []string{"text", "image", "video", "audio"}, catalog.Models[0].InputMediaTypes)
			assert.Equal(t, []any{"text", "image", "video", "audio"}, catalog.Models[0].Capabilities["mentionMediaTypes"])
			response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{
				Model: "MiniMax-H3", Contract: "newapi-video-v1",
				Parameters: map[string]any{"seconds": 5, "resolution": "768P"}, InputText: "A still landscape",
			})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var estimate canvasEstimatePayload
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &estimate))
			assert.Equal(t, "500000", estimate.EstimatedQuota)
			image := map[string]any{"type": "image", "role": "reference_image"}
			video := map[string]any{"type": "video", "role": "reference_video"}
			audio := map[string]any{"type": "audio", "role": "reference_audio"}
			firstFrame := map[string]any{"type": "image", "role": "first_frame"}
			lastFrame := map[string]any{"type": "image", "role": "last_frame"}
			for _, mediaTest := range []struct {
				name   string
				media  any
				status int
				quota  string
			}{
				{"empty", []any{}, http.StatusOK, "500000"},
				{"one-image", []any{image}, http.StatusOK, "500500"},
				{"two-images", []any{image, image}, http.StatusOK, "501000"},
				{"two-frames", []any{firstFrame, lastFrame}, http.StatusOK, "501000"},
				{"video-reservation", []any{video}, http.StatusOK, "515000"},
				{"maximum-media", []any{image, image, image, image, image, image, image, image, image, video, video, video, audio, audio, audio}, http.StatusOK, "519500"},
				{"null", nil, http.StatusBadRequest, ""},
				{"object", image, http.StatusBadRequest, ""},
				{"negative", -1, http.StatusBadRequest, ""},
				{"scalar-item", []any{1}, http.StatusBadRequest, ""},
				{"null-item", []any{nil}, http.StatusBadRequest, ""},
				{"unknown-type", []any{map[string]any{"type": "file", "role": "reference_image"}}, http.StatusBadRequest, ""},
				{"unknown-role", []any{map[string]any{"type": "image", "role": "middle_frame"}}, http.StatusBadRequest, ""},
				{"mismatched-role", []any{map[string]any{"type": "video", "role": "reference_image"}}, http.StatusBadRequest, ""},
				{"url-forbidden", []any{map[string]any{"type": "image", "role": "reference_image", "url": "https://secret.example/input.png"}}, http.StatusBadRequest, ""},
				{"missing-role", []any{map[string]any{"type": "image"}}, http.StatusBadRequest, ""},
				{"wrong-role-type", []any{map[string]any{"type": "image", "role": 1}}, http.StatusBadRequest, ""},
				{"too-many-images", []any{image, image, image, image, image, image, image, image, image, image}, http.StatusBadRequest, ""},
				{"too-many-videos", []any{video, video, video, video}, http.StatusBadRequest, ""},
				{"too-many-audios", []any{audio, audio, audio, audio}, http.StatusBadRequest, ""},
				{"too-many-items", []any{image, image, image, image, image, image, image, image, image, video, video, video, audio, audio, audio, audio}, http.StatusBadRequest, ""},
				{"duplicate-first-frame", []any{firstFrame, firstFrame}, http.StatusBadRequest, ""},
				{"duplicate-last-frame", []any{lastFrame, lastFrame}, http.StatusBadRequest, ""},
				{"mixed-frame-reference", []any{firstFrame, image}, http.StatusBadRequest, ""},
			} {
				t.Run(mediaTest.name, func(t *testing.T) {
					response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", map[string]any{
						"model": "MiniMax-H3", "contract": "newapi-video-v1",
						"parameters": map[string]any{"seconds": 5, "resolution": "768P"},
						"input_text": "A still landscape", "input_media": mediaTest.media,
					})
					require.Equal(t, mediaTest.status, response.Code, response.Body.String())
					if mediaTest.status == http.StatusOK {
						var estimate canvasEstimatePayload
						require.NoError(t, common.Unmarshal(response.Body.Bytes(), &estimate))
						assert.Equal(t, mediaTest.quota, estimate.EstimatedQuota)
					}
				})
			}
			response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/videos", "", map[string]any{
				"model": "MiniMax-H3", "prompt": "A still landscape", "seconds": 5, "resolution": "768P",
			})
			require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
			require.NoError(t, db.Model(channel).Update("model_mapping", `{"MiniMax-H3":"unconfirmed-h3"}`).Error)
			response = canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
			require.Equal(t, http.StatusOK, response.Code)
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.False(t, catalog.Models[0].Available)
			assert.Equal(t, "missing_profile", catalog.Models[0].UnavailableReason)
			response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", map[string]any{
				"model": "MiniMax-H3", "contract": "newapi-video-v1", "parameters": map[string]any{"seconds": 5},
				"input_text": "A still landscape", "input_media": []any{image},
			})
			assert.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), `"code":"media_input_unsupported"`)
		})
	}
	t.Run("unadapted-public-alias", func(t *testing.T) {
		db := setupCanvasBridgeDB(t, "sqlite")
		channel := canvasFixtureChannel(t, db, "default", "canvas-h3-alias")
		require.NoError(t, db.Model(channel).Updates(map[string]any{
			"type": constant.ChannelTypeTaskPlugin, "setting": `{"task_plugin_key":"hailuo"}`,
			"model_mapping": `{"canvas-h3-alias":"h3"}`,
		}).Error)
		withTieredBillingConfig(t, map[string]string{"canvas-h3-alias": "tiered_expr"}, map[string]string{
			"canvas-h3-alias": `u("seconds") * 0.2 + u("input_images") * 0.001`,
		})
		router := gin.New()
		router.Use(func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
			c.Next()
		})
		router.GET("/v1/canvas/catalog", GetCanvasCatalog)
		router.POST("/v1/canvas/estimate", EstimateCanvasPrice)
		response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
		require.Equal(t, http.StatusOK, response.Code)
		var catalog struct {
			Models []canvasCatalogModel `json:"models"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
		require.Len(t, catalog.Models, 1)
		assert.Equal(t, []string{"text"}, catalog.Models[0].InputMediaTypes)
		response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", map[string]any{
			"model": "canvas-h3-alias", "contract": "newapi-video-v1", "parameters": map[string]any{"seconds": 5},
			"input_text": "Animate this image", "input_media": []any{map[string]any{"type": "image", "role": "reference_image"}},
		})
		assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
		assert.Contains(t, response.Body.String(), `"code":"media_input_unsupported"`)
	})
	t.Run("mixed-routes", func(t *testing.T) {
		db := setupCanvasBridgeDB(t, "sqlite")
		source := `
export const meta = {apiVersion:1,key:"canvas-h3-text-only",name:"Canvas H3 text fixture",version:"1.0.0",author:{name:"Test"},models:["MiniMax-H3"],fetchMode:"per_task",protocols:["openai_video"],usageSchema:{seconds:{type:"number",unit:"second"},resolution:{enum:["768P"]},input_images:{type:"number",unit:"count"},input_video_seconds:{type:"number",unit:"second"}}};
export const protocols = {openai_video:{decodeRequest(ctx){return {kind:"submit",model:ctx.model,action:"text_to_video",requestBody:ctx.body.value};},render(){return {};}}};
export function extractUsage(){return {seconds:5,resolution:"768P",input_images:0,input_video_seconds:0};}
export function buildSubmitRequest(){throw new Error("must never submit");}
export function parseSubmitResponse(){throw new Error("must never submit");}
export function buildQueryRequest(){throw new Error("must never query");}
export function parseTaskResult(){throw new Error("must never query");}
export function listArtifacts(){throw new Error("must never query");}
export function buildContentRequest(){throw new Error("must never query");}
`
		_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, jsplugin.DefaultRegistry.Unregister("canvas-h3-text-only")) })
		for _, pluginKey := range []string{"hailuo", "canvas-h3-text-only"} {
			channel := canvasFixtureChannel(t, db, "default", "MiniMax-H3")
			require.NoError(t, db.Model(channel).Updates(map[string]any{
				"type": constant.ChannelTypeTaskPlugin, "setting": fmt.Sprintf(`{"task_plugin_key":%q}`, pluginKey),
			}).Error)
		}
		withTieredBillingConfig(t, map[string]string{"MiniMax-H3": "tiered_expr"}, map[string]string{
			"MiniMax-H3": `u("seconds") * 0.2 + u("input_images") * 0.001 + u("input_video_seconds") * 0.002`,
		})
		router := gin.New()
		router.Use(func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
			c.Next()
		})
		router.GET("/v1/canvas/catalog", GetCanvasCatalog)
		router.POST("/v1/canvas/estimate", EstimateCanvasPrice)
		response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var catalog struct {
			Models []canvasCatalogModel `json:"models"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
		require.Len(t, catalog.Models, 1)
		assert.True(t, catalog.Models[0].Available)
		assert.Equal(t, []string{"text"}, catalog.Models[0].InputMediaTypes)
		response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", map[string]any{
			"model": "MiniMax-H3", "contract": "newapi-video-v1", "parameters": map[string]any{"seconds": 5},
			"input_text": "A still landscape", "input_media": []any{map[string]any{"type": "image", "role": "reference_image"}},
		})
		assert.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
		assert.Contains(t, response.Body.String(), `"code":"media_input_unsupported"`)
	})
}
