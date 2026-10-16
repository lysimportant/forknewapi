package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCanvasGlobalStoreSeedreamPricingAndChannel verifies the exact reported ID
// through TokenAuth and the catalog. A price cannot add an image protocol to a
// video task plugin; the native image channel must be both routed and priced.
func TestCanvasGlobalStoreSeedreamPricingAndChannel(t *testing.T) {
	const modelName = "seedream_5.0Pro"
	for _, test := range []struct {
		name        string
		channelType int
		priced      bool
		available   bool
		reason      string
	}{
		{"priced video plugin", constant.ChannelTypeTaskPlugin, true, false, "missing_profile"},
		{"priced native image channel", constant.ChannelTypeGlobalStore, true, true, ""},
		{"unpriced native image channel", constant.ChannelTypeGlobalStore, false, false, "missing_pricing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupCanvasBridgeDB(t, "sqlite")
			user := &model.User{
				Username: "seedream-catalog-fixture", Password: "placeholder",
				Status: common.UserStatusEnabled, Group: "default", Quota: 900000,
				AffCode: "seedreamfixture",
			}
			require.NoError(t, db.Create(user).Error)
			token := &model.Token{
				UserId: user.Id, Key: strings.Repeat("s", 48),
				Status: common.TokenStatusEnabled, RemainQuota: 700000, ExpiredTime: -1,
			}
			require.NoError(t, db.Create(token).Error)
			channel := canvasFixtureChannel(t, db, "default", modelName)
			require.NoError(t, db.Model(channel).Updates(map[string]any{
				"type": test.channelType, "setting": `{"task_plugin_key":"globalstore"}`,
			}).Error)
			if test.priced {
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"seedream_5.0Pro":0.05}`))
			}
			router := gin.New()
			router.GET("/v1/canvas/catalog", middleware.TokenAuth(), GetCanvasCatalog)
			response := canvasBridgeRequest(t, router, http.MethodGet, "/v1/canvas/catalog", token.Key, nil)
			require.Equal(t, http.StatusOK, response.Code)
			var catalog struct {
				Models []canvasCatalogModel `json:"models"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			item := catalog.Models[0]
			assert.Equal(t, modelName, item.ID)
			assert.Equal(t, test.available, item.Available)
			assert.Equal(t, test.reason, item.UnavailableReason)
			if test.channelType == constant.ChannelTypeGlobalStore {
				assert.Equal(t, "image", item.MediaType)
				assert.Equal(t, "openai-images", item.Contract)
				assert.Equal(t, "public-url", item.Capabilities["referenceImages"])
			} else {
				assert.Empty(t, item.MediaType)
				assert.Empty(t, item.Contract)
			}
		})
	}
}
