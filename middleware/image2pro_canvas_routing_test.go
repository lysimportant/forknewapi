package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type image2ProCanvasBlockedTransport struct {
	requests atomic.Int32
}

func (transport *image2ProCanvasBlockedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.requests.Add(1)
	return nil, fmt.Errorf("network is disabled in the Image2Pro routing fixture")
}

func TestImage2ProCanvasImageTaskDistribution(t *testing.T) {
	const modelName = "完全无限制-Z-Image"
	const group = "image2pro-canvas-fixture"
	require.NoError(t, i18n.Init())
	previousDB, previousRegistry := model.DB, jsplugin.DefaultRegistry
	previousMemoryCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousAffinity := operation_setting.GetChannelAffinitySetting().Enabled
	previousTransport := http.DefaultTransport
	setupTaskPluginRouteDB(t)
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}))
	jsplugin.DefaultRegistry = jsplugin.NewRegistry()
	common.MemoryCacheEnabled, common.RedisEnabled = true, false
	operation_setting.GetChannelAffinitySetting().Enabled = false
	transport := &image2ProCanvasBlockedTransport{}
	http.DefaultTransport = transport
	t.Cleanup(func() {
		assert.Zero(t, transport.requests.Load(), "distribution must stop before an upstream request")
		http.DefaultTransport = previousTransport
		operation_setting.GetChannelAffinitySetting().Enabled = previousAffinity
		jsplugin.DefaultRegistry = previousRegistry
		// Clear this fixture's cache entries before restoring the previous database.
		model.DB = database.Where("1 = 0")
		model.InitChannelCache()
		model.DB = previousDB
		common.MemoryCacheEnabled, common.RedisEnabled = previousMemoryCache, previousRedis
		if previousMemoryCache && previousDB != nil &&
			previousDB.Migrator().HasTable(&model.Channel{}) && previousDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		assert.NoError(t, sqlDB.Close())
	})
	source, err := os.ReadFile(filepath.Join("..", "plugins", "tasks", "image2pro", "plugin.js"))
	require.NoError(t, err)
	plugin, err := jsplugin.DefaultRegistry.RegisterFactory(string(source), jsplugin.Options{Key: "image2pro"})
	require.NoError(t, err)
	require.Equal(t, "channel", plugin.Meta.ModelRouting)
	channel := &model.Channel{
		Name: "GPT_Image_4k", Type: constant.ChannelTypeTaskPlugin,
		Key: "fixture-only", Status: common.ChannelStatusEnabled,
		Models: modelName, Group: group,
	}
	channel.SetSetting(dto.ChannelSettings{
		TaskPluginKey: "image2pro", TaskPluginModelTypes: map[string]string{modelName: "image"},
	})
	require.NoError(t, database.Create(channel).Error)
	require.NoError(t, database.Create(&model.Ability{
		Group: group, Model: modelName, ChannelId: channel.Id, Enabled: true,
	}).Error)
	model.InitChannelCache()

	for _, tc := range []struct {
		name, path, group string
		wantStatus        int
	}{
		{name: "asynchronous image task selects Image2Pro", path: "/v1/images/tasks", group: group, wantStatus: http.StatusNoContent},
		{name: "synchronous image generation rejects task channel", path: "/v1/images/generations", group: group, wantStatus: http.StatusServiceUnavailable},
		{name: "asynchronous task rejects unauthorized group", path: "/v1/images/tasks", group: "another-fixture-group", wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			var requestContext *gin.Context
			router := gin.New()
			handlers := []gin.HandlerFunc{func(c *gin.Context) {
				requestContext = c
				common.SetContextKey(c, constant.ContextKeyUsingGroup, tc.group)
				common.SetContextKey(c, constant.ContextKeyUserGroup, tc.group)
				c.Next()
			}}
			if tc.path == "/v1/images/tasks" {
				handlers = append(handlers, PinTaskPluginEndpoint(), PrepareTaskPluginEndpoint())
			}
			handlers = append(handlers, Distribute(), func(c *gin.Context) {
				reached = true
				assert.Equal(t, modelName, c.GetString("original_model"))
				assert.Equal(t, channel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
				assert.Equal(t, constant.ChannelTypeTaskPlugin, common.GetContextKeyInt(c, constant.ContextKeyChannelType))
				assert.Equal(t, "GPT_Image_4k", common.GetContextKeyString(c, constant.ContextKeyChannelName))
				assert.Equal(t, "image2pro", c.GetString("expected_task_plugin_key"))
				assert.Equal(t, "image2pro", c.GetString("task_plugin_key"))
				pinned := c.MustGet(jsplugin.ContextKeyPinnedPlugin).(jsplugin.PinnedPlugin)
				assert.Same(t, plugin, pinned.Plugin)
				endpoint := c.MustGet(jsplugin.ContextKeyPinnedEndpoint).(jsplugin.PinnedEndpoint)
				assert.True(t, endpoint.ChannelScoped)
				assert.Equal(t, "openai_image_tasks", endpoint.Protocol)
				assert.Equal(t, modelName, endpoint.Model)
				c.Status(http.StatusNoContent)
			})
			router.POST(tc.path, handlers...)
			request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(
				`{"model":"`+modelName+`","prompt":"synthetic routing fixture"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assert.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			assert.Equal(t, tc.wantStatus == http.StatusNoContent, reached)
			if tc.wantStatus != http.StatusNoContent {
				assert.Zero(t, common.GetContextKeyInt(requestContext, constant.ContextKeyChannelId))
				assert.Contains(t, recorder.Body.String(), "model_not_found")
			}
		})
	}
}

func TestImage2ProCanvasCreationPathIsExact(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{path: "/v1/images/tasks", want: true},
		{path: "/v1/images/tasks/task_fixture"},
		{path: "/v1/images/tasks/"},
		{path: "/v1/images/tasks/arbitrary"},
		{path: "/v1/images/tasks-extra"},
		{path: "/vendor/images/tasks"},
		{path: "/v1/arbitrary"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.want, canvasCreationPath(tc.path))
		})
	}
}
