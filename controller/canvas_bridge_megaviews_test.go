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

// TestCanvasCatalogShowsPricedMegaViewsVideoModels 验证 MegaAi 渠道模式插件在 Canvas 目录中
// 发布统一的参考素材能力，并仍按渠道配置决定可用性与计费。
func TestCanvasCatalogShowsPricedMegaViewsVideoModels(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	transport := blockCanvasEstimateNetwork(t)
	plugin, found := jsplugin.DefaultRegistry.Generation().Get("megaviews-video")
	require.True(t, found)
	require.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.Empty(t, plugin.Meta.Models)

	channel := canvasFixtureChannel(t, db, "default", "megaai-priced-model", "megaai-unpriced-model")
	require.NoError(t, db.Model(channel).Updates(map[string]any{
		"type":          constant.ChannelTypeTaskPlugin,
		"setting":       `{"task_plugin_key":"megaviews-video"}`,
		"model_mapping": `{"megaai-priced-model":"seedance-2.5-m3","megaai-unpriced-model":"minimax-h3-f"}`,
	}).Error)
	model.InitChannelCache()
	withTieredBillingConfig(t,
		map[string]string{"megaai-priced-model": "tiered_expr"},
		map[string]string{"megaai-priced-model": `u("count") * 0.2`},
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
		if item.ID == "megaai-unpriced-model" {
			assert.False(t, item.Available)
			assert.Equal(t, "missing_pricing", item.UnavailableReason)
			assert.Equal(t, []string{"text"}, item.InputMediaTypes)
			assert.NotContains(t, item.Capabilities, "videoReferences")
			continue
		}
		assert.True(t, item.Available, item.ID+": "+item.UnavailableReason)
		assert.Equal(t, "video", item.MediaType)
		assert.Equal(t, "newapi-video-v1", item.Contract)
		assert.Equal(t, []string{"text", "image", "video", "audio"}, item.InputMediaTypes)
		assert.Equal(t, []any{"text", "image", "video", "audio"}, item.Capabilities["mentionMediaTypes"])
		assert.Equal(t, map[string]any{
			"version": float64(1), "adapter": "seedance-video", "maxImages": float64(30),
			"maxVideos": float64(10), "maxAudios": float64(10), "tokenSyntax": "numbered-media",
		}, item.Capabilities["videoReferences"])
	}

	for _, test := range []struct {
		mediaType, role string
		limit           int
	}{
		{"image", "reference_image", 30},
		{"video", "reference_video", 10},
		{"audio", "reference_audio", 10},
	} {
		media := make([]canvasInputMedium, test.limit+1)
		for index := range media {
			media[index] = canvasInputMedium{Type: test.mediaType, Role: test.role}
		}
		for _, count := range []int{test.limit, test.limit + 1} {
			response = canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{
				Model: "megaai-priced-model", Contract: "newapi-video-v1", InputText: "Animate the references",
				InputMedia: media[:count],
			})
			if count == test.limit {
				assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
			} else {
				assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), `"code":"invalid_estimate"`)
			}
		}
	}
	assert.Zero(t, transport.requests.Load(), "catalog and estimates must not call the provider")
}
