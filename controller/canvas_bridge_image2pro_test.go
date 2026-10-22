package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
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
		modes[name], expressions[name] = "tiered_expr", `u("count") * 0.2`
	}
	expressions["future-video"], expressions["mapped-video"] = `u("seconds") * 0.2`, `u("seconds") * 0.2`
	withTieredBillingConfig(t, modes, expressions)
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
	byID := make(map[string]canvasCatalogModel)
	for _, item := range catalog.Models {
		byID[item.ID] = item
	}
	require.Len(t, byID, 8)
	for _, name := range []string{"Nano-banana-Pro", "无限制-Seedream-5-Pro-2K", "future-picture", "mapped-picture", "unknown-raw-model"} {
		item := byID[name]
		assert.False(t, item.Available, name)
		assert.Equal(t, "missing_profile", item.UnavailableReason, name)
		assert.Empty(t, item.Contract, name)
		if name != "unknown-raw-model" {
			assert.Equal(t, "image", item.MediaType, name)
		}
	}
	for _, name := range []string{"Seedance2.0", "future-video", "mapped-video"} {
		item := byID[name]
		assert.True(t, item.Available, name+": "+item.UnavailableReason)
		assert.Equal(t, "video", item.MediaType, name)
		assert.Equal(t, "newapi-video-v1", item.Contract, name)
	}
	assert.Zero(t, transport.requests.Load())
}
