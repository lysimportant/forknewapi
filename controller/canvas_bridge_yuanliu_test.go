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

// TestCanvasYuanDynamicPricing 验证新型号按站点表达式选择按次或清晰度×秒数计价，不调用上游。
func TestCanvasYuanDynamicPricing(t *testing.T) {
	db := setupCanvasBridgeDB(t, "sqlite")
	transport := blockCanvasEstimateNetwork(t)
	plugin, found := jsplugin.DefaultRegistry.Generation().Get("yuanliu")
	require.True(t, found)
	require.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.Empty(t, plugin.Meta.Models)
	channel := canvasFixtureChannel(t, db, "default", "future-yuan-call", "future-yuan-seconds", "Yuan-Seedance-2.5-LW")
	require.NoError(t, db.Model(channel).Updates(map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "setting": `{"task_plugin_key":"yuanliu"}`,
		"model_mapping": `{"future-yuan-seconds":"Future-Upstream-v3","Yuan-Seedance-2.5-LW":"yl_lwaigc_mf_sd2_5_v2"}`,
	}).Error)
	model.InitChannelCache()
	withTieredBillingConfig(t,
		map[string]string{"future-yuan-call": "tiered_expr", "future-yuan-seconds": "tiered_expr", "Yuan-Seedance-2.5-LW": "tiered_expr"},
		map[string]string{
			"future-yuan-call":     `tier("base", u("count") * 0.2)`,
			"future-yuan-seconds":  `tier("base", u("seconds") * (u("resolution") == "1080p" ? 0.3 : 0.1))`,
			"Yuan-Seedance-2.5-LW": `u("seconds") * (u("resolution") == "720p" ? 0.2 : 0.3)`,
		},
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
	require.Len(t, catalog.Models, 3)
	for _, item := range catalog.Models {
		assert.True(t, item.Available, item.ID+": "+item.UnavailableReason)
		assert.Equal(t, "video", item.MediaType)
		assert.Equal(t, "newapi-video-v1", item.Contract)
		if item.ID != "Yuan-Seedance-2.5-LW" {
			assert.Equal(t, []string{"text"}, item.InputMediaTypes)
		}
	}
	for _, test := range []struct {
		name, resolution, quota string
	}{
		{"future-yuan-call", "1080p", "100000"},
		{"future-yuan-seconds", "1080p", "1500000"},
		{"future-yuan-seconds", "720p", "500000"},
		{"Yuan-Seedance-2.5-LW", "720p", "1000000"},
	} {
		t.Run(test.name+"/"+test.resolution, func(t *testing.T) {
			response := canvasBridgeRequest(t, router, http.MethodPost, "/v1/canvas/estimate", "", canvasEstimateRequest{
				Model: test.name, Contract: "newapi-video-v1", InputText: "A bird riding a bicycle",
				Parameters: map[string]any{"seconds": 10, "resolution": test.resolution},
			})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var estimate canvasEstimatePayload
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &estimate))
			assert.Equal(t, test.quota, estimate.EstimatedQuota)
		})
	}
	assert.Zero(t, transport.requests.Load(), "catalog and estimates must not call the provider")
}
