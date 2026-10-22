package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasImage2ProCatalogUsesModelMediaTypes(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	transport := blockCanvasEstimateNetwork(t)
	plugin, found := jsplugin.DefaultRegistry.Generation().Get("image2pro")
	require.True(t, found)
	require.Equal(t, "channel", plugin.Meta.ModelRouting)
	channel := canvasFixtureChannel(t, db, "default", "Nano-banana-Pro", "无限制-Seedream-5-Pro-2K", "Seedance2.0",
		"future-picture", "future-video", "unknown-raw-model", "mapped-picture", "mapped-video")
	require.NoError(t, db.Model(channel).Updates(map[string]any{
		"type":          constant.ChannelTypeTaskPlugin,
		"setting":       `{"task_plugin_key":"image2pro","task_plugin_model_types":{"future-picture":"image","future-video":"video","provider-picture":"image","provider-video":"video"}}`,
		"model_mapping": `{"mapped-picture":"provider-picture","mapped-video":"provider-video"}`,
	}).Error)
	model.InitChannelCache()
	modes, expressions := make(map[string]string), make(map[string]string)
	for _, name := range channel.GetModels() {
		modes[name], expressions[name] = "tiered_expr", `tier("base", u("count") * 0.2 + u("seconds") * 0)`
	}
	expressions["future-video"], expressions["mapped-video"] = `u("seconds") * 0.2`, `u("seconds") * 0.2`
	withTieredBillingConfig(t, modes, expressions)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{billing_setting.PluginBillingExprOption: `{}`}))
	router := canvasImage2ProRouter()
	response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var catalog struct {
		Models []canvasCatalogModel `json:"models"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
	byID := make(map[string]canvasCatalogModel)
	for _, item := range catalog.Models {
		byID[item.ID] = item
	}
	require.Len(t, byID, 8)
	for _, name := range []string{"Nano-banana-Pro", "无限制-Seedream-5-Pro-2K", "future-picture", "mapped-picture"} {
		item := byID[name]
		assert.True(t, item.Available, name+": "+item.UnavailableReason)
		assert.Equal(t, "image", item.MediaType, name)
		assert.Equal(t, "image2pro-image-task-v1", item.Contract, name)
		assert.Equal(t, []string{"text", "image"}, item.InputMediaTypes, name)
		assert.NotEqual(t, "public-url", item.Capabilities["referenceImages"], name)
	}
	assert.False(t, byID["unknown-raw-model"].Available)
	assert.Equal(t, "missing_profile", byID["unknown-raw-model"].UnavailableReason)
	assert.Empty(t, byID["unknown-raw-model"].Contract)
	for _, name := range []string{"Seedance2.0", "future-video", "mapped-video"} {
		item := byID[name]
		assert.True(t, item.Available, name+": "+item.UnavailableReason)
		assert.Equal(t, "video", item.MediaType, name)
		assert.Equal(t, "newapi-video-v1", item.Contract, name)
	}
	assert.Zero(t, transport.requests.Load())
}

func canvasImage2ProRouter() *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		c.Next()
	})
	router.GET("/v1/canvas/catalog", GetCanvasCatalog)
	router.POST("/v1/canvas/estimate", EstimateCanvasPrice)
	return router
}

func TestCanvasImage2ProEstimateCountsGeneratedImages(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	transport := blockCanvasEstimateNetwork(t)
	channel := canvasFixtureChannel(t, db, "default", "Nano-banana-Pro", "future-picture", "future-override", "mapped-picture", "mapped-override", "Seedance2.0", "unknown-raw-model")
	require.NoError(t, db.Model(channel).Updates(map[string]any{
		"type":          constant.ChannelTypeTaskPlugin,
		"setting":       `{"task_plugin_key":"image2pro","task_plugin_model_types":{"future-picture":"image","future-override":"image","provider-picture":"image","provider-override":"image"}}`,
		"model_mapping": `{"mapped-picture":"provider-picture","mapped-override":"provider-override"}`,
	}).Error)
	model.InitChannelCache()
	withTieredBillingConfig(t,
		map[string]string{"Nano-banana-Pro": "tiered_expr", "future-picture": "tiered_expr", "future-override": "tiered_expr", "provider-picture": "tiered_expr", "mapped-override": "tiered_expr", "Seedance2.0": "tiered_expr", "unknown-raw-model": "tiered_expr"},
		map[string]string{
			"Nano-banana-Pro":   `tier("base", u("count") * 3.5 + u("seconds") * 0)`,
			"future-picture":    `tier("base", u("count") * 0.2 + u("seconds") * 0)`,
			"future-override":   `u("count") * 9`,
			"provider-picture":  `u("count") * 0.3`,
			"mapped-override":   `u("count") * 9`,
			"Seedance2.0":       `u("seconds") * 0.2`,
			"unknown-raw-model": `u("count") * 0.2`,
		},
	)
	overrides, err := common.Marshal(map[string]string{
		billing_setting.PluginBillingExprKey("image2pro", "future-override"):   `tier("override", u("count") * 0.4 + u("seconds") * 0)`,
		billing_setting.PluginBillingExprKey("image2pro", "provider-override"): `tier("override", u("count") * 0.6 + u("seconds") * 0)`,
	})
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{billing_setting.PluginBillingExprOption: string(overrides)}))
	router := canvasImage2ProRouter()
	for _, test := range []struct {
		name, model, quota string
		parameters         map[string]any
		media              []canvasInputMedium
	}{
		{name: "known-default-one", model: "Nano-banana-Pro", quota: "1750000"},
		{name: "known-two", model: "Nano-banana-Pro", quota: "3500000", parameters: map[string]any{"n": 2, "size": "1024x1024", "aspect_ratio": "1:1"}},
		{name: "dynamic-four", model: "future-picture", quota: "400000", parameters: map[string]any{"n": 4}},
		{name: "public-plugin-price", model: "future-override", quota: "600000", parameters: map[string]any{"n": 3}},
		{name: "mapped-model-price", model: "mapped-picture", quota: "300000", parameters: map[string]any{"n": 2}},
		{name: "mapped-plugin-price", model: "mapped-override", quota: "1200000", parameters: map[string]any{"n": 4}},
		{name: "references-do-not-add-generated-count", model: "future-picture", quota: "200000", parameters: map[string]any{"n": 2}, media: canvasReferenceInputs(5, 0, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{
				Model: test.model, Contract: "image2pro-image-task-v1", InputText: "A bird riding a bicycle", Parameters: test.parameters, InputMedia: test.media,
			})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var estimate canvasEstimatePayload
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &estimate))
			assert.Equal(t, test.quota, estimate.EstimatedQuota)
			assert.Equal(t, "500000", estimate.QuotaPerUnit)
			assert.True(t, estimate.EstimateOnly)
		})
	}
	for _, test := range []struct {
		name                    string
		model, contract, prompt string
		parameters              map[string]any
		media                   any
	}{
		{name: "zero-count", parameters: map[string]any{"n": 0}},
		{name: "too-many-images", parameters: map[string]any{"n": 5}},
		{name: "fractional-count", parameters: map[string]any{"n": 1.5}},
		{name: "string-count", parameters: map[string]any{"n": "2"}},
		{name: "video-parameter", parameters: map[string]any{"seconds": 10}},
		{name: "undeclared-quality", parameters: map[string]any{"quality": "high"}},
		{name: "undeclared-output-format", parameters: map[string]any{"response_format": "url"}},
		{name: "resource-url-parameter", parameters: map[string]any{"url": "https://example.invalid/reference.png"}},
		{name: "empty-prompt", prompt: " "},
		{name: "video-as-image", model: "Seedance2.0"},
		{name: "unknown-media-type", model: "unknown-raw-model"},
		{name: "image-as-video", contract: "newapi-video-v1", parameters: map[string]any{"seconds": 5}},
		{name: "audio-reference", media: canvasReferenceInputs(0, 0, 1)},
		{name: "video-reference", media: canvasReferenceInputs(0, 1, 0)},
		{name: "first-frame", media: []canvasInputMedium{{Type: "image", Role: "first_frame"}}},
		{name: "reference-url", media: []any{map[string]any{"type": "image", "role": "reference_image", "url": "https://example.invalid/reference.png"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			name, contract, prompt := test.model, test.contract, test.prompt
			if name == "" {
				name = "future-picture"
			}
			if contract == "" {
				contract = "image2pro-image-task-v1"
			}
			if prompt == "" {
				prompt = "A bird riding a bicycle"
			}
			body := map[string]any{"model": name, "contract": contract, "input_text": prompt, "parameters": test.parameters}
			if test.media != nil {
				body["input_media"] = test.media
			}
			response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", body)
			assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, response.Code, response.Body.String())
			var failure struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &failure))
			assert.Contains(t, []string{"invalid_estimate", "plugin_parameters_rejected", "estimate_unavailable"}, failure.Error.Code,
				"invalid inputs must be rejected before billing evaluation: "+response.Body.String())
		})
	}
	assert.Zero(t, transport.requests.Load(), "catalog and estimates must not call the provider")
}

func TestCanvasImage2ProExplicitEndpointsFailClosed(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	transport := blockCanvasEstimateNetwork(t)
	channel := canvasFixtureChannel(t, db, "default", "future-picture")
	require.NoError(t, db.Model(channel).Updates(map[string]any{
		"type":    constant.ChannelTypeTaskPlugin,
		"setting": `{"task_plugin_key":"image2pro","task_plugin_model_types":{"future-picture":"image"}}`,
	}).Error)
	model.InitChannelCache()
	withTieredBillingConfig(t, map[string]string{"future-picture": "tiered_expr"}, map[string]string{"future-picture": `u("count") * 0.2`})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{billing_setting.PluginBillingExprOption: `{}`}))
	metadata := &model.Model{ModelName: "future-picture", Status: 1, NameRule: model.NameRuleExact}
	require.NoError(t, db.Create(metadata).Error)
	router := canvasImage2ProRouter()
	for _, test := range []struct {
		name, endpoints string
		available       bool
	}{
		{name: "absent-infers", available: true},
		{name: "empty-array-infers", endpoints: `[]`, available: true},
		{name: "empty-object-infers", endpoints: `{}`, available: true},
		{name: "explicit-task-path", endpoints: `{"image-task":"/v1/images/tasks"}`, available: true},
		{name: "explicit-post-task", endpoints: `{"image-task":{"path":"/v1/images/tasks","method":"POST"}}`, available: true},
		{name: "synchronous-image-is-different", endpoints: `["image-generation"]`},
		{name: "explicit-chat", endpoints: `{"openai":"/v1/chat/completions"}`},
		{name: "explicit-get-task", endpoints: `{"image-task":{"path":"/v1/images/tasks","method":"GET"}}`},
		{name: "invalid-declaration", endpoints: `{invalid`},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, db.Model(metadata).Update("endpoints", test.endpoints).Error)
			response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var catalog struct {
				Models []canvasCatalogModel `json:"models"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.Equal(t, test.available, catalog.Models[0].Available, response.Body.String())
			if test.available {
				assert.Equal(t, "image2pro-image-task-v1", catalog.Models[0].Contract)
			} else {
				assert.Equal(t, "missing_profile", catalog.Models[0].UnavailableReason)
			}
			response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{
				Model: "future-picture", Contract: "image2pro-image-task-v1", InputText: "A bird riding a bicycle",
			})
			if test.available {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			} else {
				assert.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
			}
		})
	}
	assert.Zero(t, transport.requests.Load())
}
