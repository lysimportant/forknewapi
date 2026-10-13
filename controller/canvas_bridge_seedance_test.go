package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasCatalogShowsPricedSeedanceChannelModels(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	transport := blockCanvasEstimateNetwork(t)
	plugin, found := jsplugin.DefaultRegistry.Generation().Get("seedance-video")
	require.True(t, found)
	require.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.Empty(t, plugin.Meta.Models)

	channel := canvasFixtureChannel(t, db, "default", "LLseadanco2.5", "arbitrary-future-model")
	require.NoError(t, db.Model(channel).Updates(map[string]any{
		"type":          constant.ChannelTypeTaskPlugin,
		"setting":       `{"task_plugin_key":"seedance-video"}`,
		"model_mapping": `{"arbitrary-future-model":"provider-video-v3"}`,
	}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "LLseadanco2.5", Status: 1}).Error)
	model.InitChannelCache()
	withTieredBillingConfig(t,
		map[string]string{"LLseadanco2.5": "tiered_expr", "arbitrary-future-model": "tiered_expr"},
		map[string]string{"LLseadanco2.5": `u("count") * 0.2`, "arbitrary-future-model": `u("count") * 0.3`},
	)

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
	require.Len(t, catalog.Models, 2)
	for _, item := range catalog.Models {
		assert.True(t, item.Available, item.ID+": "+item.UnavailableReason)
		assert.Equal(t, "video", item.MediaType)
		assert.Equal(t, "newapi-video-v1", item.Contract)
		assert.Equal(t, []string{"text"}, item.InputMediaTypes)
	}

	for _, test := range []struct{ name, quota string }{
		{"LLseadanco2.5", "100000"},
		{"arbitrary-future-model", "150000"},
	} {
		response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{
			Model: test.name, Contract: "newapi-video-v1", InputText: "A bird riding a bicycle",
		})
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Contains(t, response.Body.String(), `"estimated_quota":"`+test.quota+`"`)
	}
	assert.Zero(t, transport.requests.Load(), "catalog and estimates must not call the provider")
}

func TestCanvasCatalogRejectsSeedanceNameClaimedByAnotherPlugin(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	channel := canvasFixtureChannel(t, db, "default", "shared-channel-video")
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "seedance-video"})
	require.NoError(t, db.Model(channel).Updates(map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "setting": *channel.Setting,
	}).Error)
	other := canvasFixtureChannel(t, db, "secret", "shared-channel-video")
	other.SetSetting(dto.ChannelSettings{TaskPluginKey: "yuanliu"})
	require.NoError(t, db.Model(other).Updates(map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "setting": *other.Setting,
		"model_mapping": `{"shared-channel-video":"seedance-2.5-guanfang-anmiao"}`,
	}).Error)
	model.InitChannelCache()
	withTieredBillingConfig(t, map[string]string{"shared-channel-video": "tiered_expr"},
		map[string]string{"shared-channel-video": `u("count") * 0.2`})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		c.Next()
	})
	router.GET("/v1/canvas/catalog", GetCanvasCatalog)
	response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", "", nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var catalog struct {
		Models []canvasCatalogModel `json:"models"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
	require.Len(t, catalog.Models, 1)
	assert.Equal(t, "shared-channel-video", catalog.Models[0].ID)
	assert.False(t, catalog.Models[0].Available)
	assert.Equal(t, "missing_profile", catalog.Models[0].UnavailableReason)
}
