package controller

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registerModelDiscoveryTestPlugin 注册独立插件，测试结束时恢复注册表。
func registerModelDiscoveryTestPlugin(t *testing.T, manifestFields string) string {
	t.Helper()
	const key = "model-discovery-test"
	source := fmt.Sprintf(`
export const meta = {apiVersion: 1, key: %q, name: "Discovery", version: "1.0.0", author: {name: "Test"}, models: ["wan3.0-video", "minimax-h3", "seedance-2-0-official"], fetchMode: "per_task"%s};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
`, key, manifestFields)
	_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = jsplugin.DefaultRegistry.Unregister(key) })
	return key
}

// taskPluginDiscoveryResponse 保留模型来源和未适配模型，便于验证公开接口合同。
type taskPluginDiscoveryResponse struct {
	Success           bool     `json:"success"`
	Message           string   `json:"message"`
	Data              []string `json:"data"`
	Source            string   `json:"source"`
	UnsupportedModels []string `json:"unsupported_models"`
}

// previewTaskPluginModels 调用渠道草稿的模型获取入口，不保存渠道或执行付费请求。
func previewTaskPluginModels(t *testing.T, request map[string]any) taskPluginDiscoveryResponse {
	t.Helper()
	body, err := common.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	FetchModels(ctx)
	var response taskPluginDiscoveryResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestFetchTaskPluginModelsUsesDeclaredCatalogWithoutDiscovery(t *testing.T) {
	key := registerModelDiscoveryTestPlugin(t, "")
	response := previewTaskPluginModels(t, map[string]any{"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key})
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "plugin", response.Source)
	assert.Equal(t, []string{"wan3.0-video", "minimax-h3", "seedance-2-0-official"}, response.Data)
	assert.Empty(t, response.UnsupportedModels)

	unknown := previewTaskPluginModels(t, map[string]any{"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": "unknown-plugin"})
	assert.False(t, unknown.Success)
	assert.Contains(t, unknown.Message, "not registered")
}

func TestFetchTaskPluginModelsCreatePreviewFiltersCatalogAndJoinsBasePath(t *testing.T) {
	for _, basePath := range []string{"", "/v1", "/proxy/v1"} {
		t.Run("base="+basePath, func(t *testing.T) {
			key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "openai", path: "/v1/models"}`)
			received := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Clone(r.Context())
				_, _ = w.Write([]byte(`{"data":[{"id":"wan3.0-video"},{"id":"seedance-2.5"},{"id":"minimax-h3"},{"id":"MiniMax-H3"},{"id":"wan3.0-video"},{"id":""}]}`))
			}))
			defer server.Close()
			response := previewTaskPluginModels(t, map[string]any{
				"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
				"base_url": server.URL + basePath, "key": "preview-key\nunused-key",
				"header_override": `{"Authorization":"Token {api_key}","X-Discovery":"preview","Host":"models.example.test"}`,
			})
			require.True(t, response.Success, response.Message)
			require.Equal(t, "upstream", response.Source)
			assert.Equal(t, []string{"wan3.0-video", "minimax-h3"}, response.Data)
			assert.Equal(t, []string{"seedance-2.5", "MiniMax-H3"}, response.UnsupportedModels)
			request := <-received
			wantPath := "/v1/models"
			if basePath == "/proxy/v1" {
				wantPath = "/proxy/v1/models"
			}
			assert.Equal(t, wantPath, request.URL.Path)
			assert.Equal(t, "Token preview-key", request.Header.Get("Authorization"))
			assert.Equal(t, "preview", request.Header.Get("X-Discovery"))
			assert.Equal(t, "models.example.test", request.Host)
		})
	}
}

func TestFetchTaskPluginModelsUsesDefaultBaseURLAndProxy(t *testing.T) {
	requests := make(chan *http.Request, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		_, _ = w.Write([]byte(`{"data":[{"id":"wan3.0-video"}]}`))
	}))
	defer proxy.Close()
	key := registerModelDiscoveryTestPlugin(t, `, baseUrl: "http://models.example.test/api/v3", modelDiscovery: {protocol: "openai", path: "/api/v3/models"}`)
	response := previewTaskPluginModels(t, map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
		"key": "proxy-key", "proxy": proxy.URL,
	})
	require.True(t, response.Success, response.Message)
	request := <-requests
	assert.Equal(t, "http://models.example.test/api/v3/models", request.URL.String())
	assert.Equal(t, "Bearer proxy-key", request.Header.Get("Authorization"))
}

func TestFetchTaskPluginModelsSavedChannelUsesEnabledKey(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "openai", path: "/v1/models"}`)
	receivedKey := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"wan3.0-video"},{"id":"seedance-2.5"}]}`))
	}))
	defer server.Close()
	channel := &model.Channel{
		Type: constant.ChannelTypeTaskPlugin, Name: "saved discovery", BaseURL: &server.URL,
		Key: "disabled-key\nenabled-saved-key", Models: "wan3.0-video",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 1, MultiKeyStatusList: map[int]int{
			0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusEnabled,
		}},
	}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: key})
	require.NoError(t, db.Create(channel).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/fetch_models/"+fmt.Sprint(channel.Id), nil)
	FetchUpstreamModels(ctx)
	var response taskPluginDiscoveryResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "upstream", response.Source)
	assert.Equal(t, []string{"wan3.0-video"}, response.Data)
	assert.Equal(t, []string{"seedance-2.5"}, response.UnsupportedModels)
	assert.Equal(t, "Bearer enabled-saved-key", <-receivedKey)
	assert.NotContains(t, recorder.Body.String(), "saved-key")
	reloaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, channel.ChannelInfo, reloaded.ChannelInfo)
}

func TestFetchTaskPluginModelsEditPreviewDoesNotPersistDraft(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "openai", path: "/v1/models"}`)
	received := make(chan http.Header, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		_, _ = w.Write([]byte(`{"data":[{"id":"wan3.0-video"}]}`))
	}))
	defer server.Close()
	savedBase := "http://127.0.0.1:1"
	savedHeaders := `{"X-Saved":"saved-header"}`
	channel := &model.Channel{
		Type: constant.ChannelTypeTaskPlugin, Name: "saved preview", Key: "disabled-key\nenabled-saved-key",
		BaseURL: &savedBase, HeaderOverride: &savedHeaders, Models: "local-model",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyStatusList: map[int]int{
			0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusEnabled,
		}},
	}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "previous-plugin", Proxy: "http://127.0.0.1:1"})
	require.NoError(t, db.Create(channel).Error)
	request := map[string]any{
		"channel_id": channel.Id, "type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
		"base_url": server.URL, "header_override": "", "proxy": "",
	}
	response := previewTaskPluginModels(t, request)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, []string{"wan3.0-video"}, response.Data)
	headers := <-received
	assert.Equal(t, "Bearer enabled-saved-key", headers.Get("Authorization"))
	assert.Empty(t, headers.Get("X-Saved"))

	request["key"] = "draft-key\nunused-key"
	request["header_override"] = `{"Authorization":"Token {api_key}"}`
	response = previewTaskPluginModels(t, request)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "Token draft-key", (<-received).Get("Authorization"))
	assert.NotContains(t, response.Message, "key")
	reloaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, channel.Key, reloaded.Key)
	assert.Equal(t, channel.GetBaseURL(), reloaded.GetBaseURL())
	assert.Equal(t, channel.Setting, reloaded.Setting)
	assert.Equal(t, channel.HeaderOverride, reloaded.HeaderOverride)
	assert.Equal(t, channel.ChannelInfo, reloaded.ChannelInfo)
	assert.Equal(t, channel.Models, reloaded.Models)

	channel.ChannelInfo.MultiKeyStatusList[1] = common.ChannelStatusManuallyDisabled
	require.NoError(t, db.Model(channel).Update("channel_info", channel.ChannelInfo).Error)
	delete(request, "key")
	response = previewTaskPluginModels(t, request)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "no enabled keys")
	assert.Empty(t, received)

	ordinary := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "ordinary channel", Key: "ordinary-secret"}
	require.NoError(t, db.Create(ordinary).Error)
	request["channel_id"] = ordinary.Id
	response = previewTaskPluginModels(t, request)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "not a task plugin channel")
	assert.Empty(t, received)
}

func TestFailedTaskPluginDetectionPreservesChannelModels(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		wantError string
	}{
		{name: "empty upstream catalog", body: `{"data":[]}`, wantError: "no valid model IDs"},
		{name: "no supported models", body: `{"data":[{"id":"future-model"}]}`, wantError: "no supported model IDs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "openai", path: "/v1/models"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			channel := &model.Channel{Type: constant.ChannelTypeTaskPlugin, Name: test.name, Key: "sync-key", BaseURL: &server.URL, Models: "wan3.0-video,minimax-h3"}
			channel.SetSetting(dto.ChannelSettings{TaskPluginKey: key})
			settings := dto.ChannelOtherSettings{UpstreamModelUpdateCheckEnabled: true, UpstreamModelUpdateAutoSyncEnabled: true}
			channel.SetOtherSettings(settings)
			require.NoError(t, db.Create(channel).Error)
			modelsChanged, autoAdded, err := checkAndPersistChannelUpstreamModelUpdates(channel, &settings, true, true)
			require.ErrorContains(t, err, test.wantError)
			assert.False(t, modelsChanged)
			assert.Zero(t, autoAdded)
			reloaded, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			assert.Equal(t, "wan3.0-video,minimax-h3", reloaded.Models)
			assert.Empty(t, reloaded.GetOtherSettings().UpstreamModelUpdateLastDetectedModels)
			assert.Empty(t, reloaded.GetOtherSettings().UpstreamModelUpdateLastRemovedModels)
			if test.name == "no supported models" {
				response := previewTaskPluginModels(t, map[string]any{"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key, "key": "preview-key", "base_url": server.URL})
				require.True(t, response.Success, response.Message)
				assert.Equal(t, "upstream", response.Source)
				assert.Empty(t, response.Data)
				assert.Equal(t, []string{"future-model"}, response.UnsupportedModels)
			}
		})
	}
}

func TestFetchTaskPluginModelsRejectsInvalidUpstreamWithoutFallback(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantError string
	}{
		{name: "HTTP error", status: http.StatusUnauthorized, body: `{"data":[{"id":"wan3.0-video"}]}`, wantError: "status code: 401"},
		{name: "missing data", body: `{"error":"denied"}`, wantError: "data is required"},
		{name: "null data", body: `{"data":null}`, wantError: "data is required"},
		{name: "wrong type", body: `{"data":{}}`, wantError: "invalid OpenAI Models response"},
		{name: "malformed", body: `{"data":`, wantError: "invalid OpenAI Models response"},
		{name: "oversized", body: strings.Repeat(" ", (1<<20)+1), wantError: "1 MiB"},
		{name: "empty catalog", body: `{"data":[]}`, wantError: "no valid model IDs"},
		{name: "blank IDs", body: `{"data":[{"id":" "}]}`, wantError: "no valid model IDs"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "openai", path: "/v1/models"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			response := previewTaskPluginModels(t, map[string]any{
				"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
				"base_url": server.URL, "key": "upstream-key",
			})
			if test.wantError != "" {
				assert.False(t, response.Success)
				assert.Contains(t, response.Message, test.wantError)
				assert.Empty(t, response.Data)
				return
			}
			require.True(t, response.Success, response.Message)
			assert.Equal(t, "upstream", response.Source)
			assert.Equal(t, []string{}, response.Data)
		})
	}
}

func TestFetchTaskPluginModelsRedactsInvalidAddressesAndUpstreamErrors(t *testing.T) {
	key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "openai", path: "/v1/models"}`)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"preview-secret custom-header-secret proxy-secret"}`))
	}))
	defer server.Close()
	for _, test := range []struct {
		name         string
		baseURL      string
		proxy        string
		wantError    string
		wantRequests int32
	}{
		{name: "base URL credentials", baseURL: "https://user:preview-secret@models.example.test", wantError: "Base URL"},
		{name: "base URL query", baseURL: server.URL + "/v1?api_key=preview-secret", wantError: "Base URL"},
		{name: "malformed base URL", baseURL: "https://models.example.test/%preview-secret", wantError: "Base URL"},
		{name: "malformed proxy", baseURL: server.URL, proxy: "http://user:proxy-secret@%", wantError: "代理配置无效"},
		{name: "upstream error body", baseURL: server.URL, wantError: "status code: 401", wantRequests: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests.Store(0)
			response := previewTaskPluginModels(t, map[string]any{
				"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
				"base_url": test.baseURL, "key": "preview-secret", "proxy": test.proxy,
				"header_override": `{"X-Discovery":"custom-header-secret"}`,
			})
			assert.False(t, response.Success)
			assert.Contains(t, response.Message, test.wantError)
			assert.NotContains(t, response.Message, "preview-secret")
			assert.NotContains(t, response.Message, "custom-header-secret")
			assert.NotContains(t, response.Message, "proxy-secret")
			assert.Equal(t, test.wantRequests, requests.Load())
		})
	}
}

func TestFetchTaskPluginModelsRejectsRedirectWithoutLeakingHeaders(t *testing.T) {
	key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "openai", path: "/v1/models"}`)
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"wan3.0-video"}]}`))
	}))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/models", http.StatusFound)
	}))
	defer server.Close()
	sharedClient, err := service.NewProxyHttpClient("")
	require.NoError(t, err)
	originalTimeout := sharedClient.Timeout
	originalRedirectPolicy := fmt.Sprintf("%p", sharedClient.CheckRedirect)
	response := previewTaskPluginModels(t, map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
		"base_url": server.URL, "key": "must-stay-on-origin",
		"header_override": `{"X-Secret":"custom-header-secret"}`,
	})
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "status code: 302")
	assert.Zero(t, redirected.Load())
	assert.Equal(t, originalTimeout, sharedClient.Timeout)
	assert.Equal(t, originalRedirectPolicy, fmt.Sprintf("%p", sharedClient.CheckRedirect))
	assert.NotContains(t, response.Message, "must-stay-on-origin")
	assert.NotContains(t, response.Message, "custom-header-secret")
}

func TestFetchTaskPluginGeminiModelsPaginatesAndPreservesExactIDs(t *testing.T) {
	key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "gemini", path: "/v1beta/models"}`)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, "/v1beta/models", r.URL.Path)
		assert.Equal(t, "gemini-key", r.Header.Get("x-goog-api-key"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.URL.Query().Get("key"))
		if r.URL.Query().Get("pageToken") == "next-page" {
			_, _ = w.Write([]byte(`{"nextPageToken":"last-page"}`))
			return
		}
		if r.URL.Query().Get("pageToken") == "last-page" {
			_, _ = w.Write([]byte(`{"models":[{"name":"models/minimax-h3"}],"nextPageToken":"terminal-page"}`))
			return
		}
		if r.URL.Query().Get("pageToken") == "terminal-page" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/wan3.0-video"},{"name":"models/MiniMax-H3"}],"nextPageToken":"next-page"}`))
	}))
	defer server.Close()
	response := previewTaskPluginModels(t, map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
		"base_url": server.URL + "/v1beta", "key": "gemini-key",
	})
	require.True(t, response.Success, response.Message)
	assert.Equal(t, []string{"wan3.0-video", "minimax-h3"}, response.Data)
	assert.Equal(t, []string{"MiniMax-H3"}, response.UnsupportedModels)
	assert.Equal(t, int32(4), requests.Load())
}

func TestFetchTaskPluginGeminiModelsRejectsIncompletePagination(t *testing.T) {
	tests := []struct {
		name      string
		bodies    []string
		wantError string
	}{
		{name: "invalid models type", bodies: []string{`{"models":{},"nextPageToken":"next"}`}, wantError: "invalid Gemini Models response"},
		{name: "empty catalog", bodies: []string{`{"models":[]}`}, wantError: "no valid model IDs"},
		{name: "repeated page token", bodies: []string{`{"models":[{"name":"models/wan3.0-video"}],"nextPageToken":"same"}`}, wantError: "repeated page token"},
		{name: "combined response size", bodies: []string{`{"models":[{"name":"models/wan3.0-video"}],"nextPageToken":"next"}` + strings.Repeat(" ", 1<<19)}, wantError: "1 MiB"},
		{name: "empty entire catalog through pages", bodies: []string{
			`{"models":[],"nextPageToken":"next"}`, `{"models":[]}`,
		}, wantError: "no valid model IDs"},
		{name: "repeated model on later page", bodies: []string{
			`{"models":[{"name":"models/wan3.0-video"}],"nextPageToken":"next"}`, `{"models":[{"name":"models/wan3.0-video"}]}`,
		}, wantError: "repeated model ID"},
		{name: "blank model name", bodies: []string{`{"models":[{"name":"models/"}]}`}, wantError: "empty or repeated model ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "gemini", path: "/v1beta/models"}`)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				index := min(int(requests.Add(1))-1, len(test.bodies)-1)
				_, _ = w.Write([]byte(test.bodies[index]))
			}))
			defer server.Close()
			response := previewTaskPluginModels(t, map[string]any{
				"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
				"base_url": server.URL, "key": "gemini-key",
			})
			assert.False(t, response.Success)
			assert.Contains(t, response.Message, test.wantError)
			assert.Empty(t, response.Data)
		})
	}
}

func TestFetchTaskPluginGeminiModelsRejectsMoreThanHundredPages(t *testing.T) {
	key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "gemini", path: "/v1beta/models"}`)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page := requests.Add(1)
		_, _ = fmt.Fprintf(w, `{"models":[],"nextPageToken":"page-%d"}`, page)
	}))
	defer server.Close()
	response := previewTaskPluginModels(t, map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
		"base_url": server.URL, "key": "gemini-key",
	})
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "100")
	assert.Empty(t, response.Data)
	assert.Equal(t, int32(100), requests.Load())
}

func TestFetchTaskPluginBailianModelsUsesNativePathAndPaginates(t *testing.T) {
	key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "bailian", path: "/api/v1/models"}`)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, "/api/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer bailian-key", r.Header.Get("Authorization"))
		assert.Equal(t, "20", r.URL.Query().Get("page_size"))
		if r.URL.Query().Get("page_no") == "2" {
			_, _ = w.Write([]byte(`{"success":true,"output":{"total":21,"page_no":2,"page_size":20,"models":[{"model":"minimax-h3"}]}}`))
			return
		}
		models := []map[string]string{{"model": "wan3.0-video"}}
		for i := range 19 {
			models = append(models, map[string]string{"model": fmt.Sprintf("new-model-%d", i)})
		}
		body, err := common.Marshal(map[string]any{
			"success": true,
			"output":  map[string]any{"total": 21, "page_no": 1, "page_size": 20, "models": models},
		})
		if assert.NoError(t, err) {
			_, _ = w.Write(body)
		}
	}))
	defer server.Close()
	response := previewTaskPluginModels(t, map[string]any{
		"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
		"base_url": server.URL + "/compatible-mode/v1", "key": "bailian-key",
	})
	require.True(t, response.Success, response.Message)
	assert.Equal(t, []string{"wan3.0-video", "minimax-h3"}, response.Data)
	assert.Len(t, response.UnsupportedModels, 19)
	assert.Equal(t, int32(2), requests.Load())
}

func TestFetchTaskPluginBailianModelsValidatesResponse(t *testing.T) {
	tests := []struct {
		name      string
		bodies    []string
		wantError string
	}{
		{name: "empty catalog", bodies: []string{`{"success":true,"output":{"total":0,"page_no":1,"page_size":20,"models":[]}}`}, wantError: "no valid model IDs"},
		{name: "failed response", bodies: []string{`{"success":false,"output":{"total":0,"page_no":1,"page_size":20,"models":[]}}`}, wantError: "invalid Bailian Models response"},
		{name: "missing models", bodies: []string{`{"success":true,"output":{"total":0,"page_no":1,"page_size":20}}`}, wantError: "output.models"},
		{name: "missing total", bodies: []string{`{"success":true,"output":{"page_no":1,"page_size":20,"models":[]}}`}, wantError: "pagination"},
		{name: "early empty page", bodies: []string{`{"success":true,"output":{"total":3,"page_no":1,"page_size":20,"models":[]}}`}, wantError: "model count"},
		{name: "wrong page", bodies: []string{`{"success":true,"output":{"total":1,"page_no":2,"page_size":20,"models":[{"model":"wan3.0-video"}]}}`}, wantError: "pagination"},
		{name: "blank model", bodies: []string{`{"success":true,"output":{"total":1,"page_no":1,"page_size":20,"models":[{"model":" "}]}}`}, wantError: "empty or repeated model ID"},
		{name: "repeated model on later page", bodies: []string{
			`{"success":true,"output":{"total":2,"page_no":1,"page_size":20,"models":[{"model":"wan3.0-video"}]}}`,
			`{"success":true,"output":{"total":2,"page_no":2,"page_size":20,"models":[{"model":"wan3.0-video"}]}}`,
		}, wantError: "repeated model ID"},
		{name: "total changed", bodies: []string{
			`{"success":true,"output":{"total":2,"page_no":1,"page_size":20,"models":[{"model":"wan3.0-video"}]}}`,
			`{"success":true,"output":{"total":1,"page_no":2,"page_size":20,"models":[{"model":"minimax-h3"}]}}`,
		}, wantError: "total changed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := registerModelDiscoveryTestPlugin(t, `, modelDiscovery: {protocol: "bailian", path: "/api/v1/models"}`)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				index := min(int(requests.Add(1))-1, len(test.bodies)-1)
				_, _ = w.Write([]byte(test.bodies[index]))
			}))
			defer server.Close()
			response := previewTaskPluginModels(t, map[string]any{
				"type": constant.ChannelTypeTaskPlugin, "task_plugin_key": key,
				"base_url": server.URL, "key": "bailian-key",
			})
			assert.False(t, response.Success)
			assert.Contains(t, response.Message, test.wantError)
			assert.Empty(t, response.Data)
		})
	}
}

func newAdvancedCustomModelListChannel(baseURL string, key string, upstreamPath string, auth *dto.AdvancedCustomRouteAuth) *model.Channel {
	config := &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{
			{
				IncomingPath: dto.AdvancedCustomModelListPath,
				UpstreamPath: upstreamPath,
				Converter:    "none",
				Auth:         auth,
			},
		},
	}
	channel := &model.Channel{
		Type:    constant.ChannelTypeAdvancedCustom,
		Key:     key,
		BaseURL: &baseURL,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: config})
	return channel
}

func TestParseOpenAIModelIDsStrictResponseContract(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		want      []string
		wantError string
	}{
		{name: "malformed JSON", body: `{"data":`, wantError: "invalid OpenAI Models response"},
		{name: "missing data", body: `{"object":"list"}`, wantError: "data is required"},
		{name: "null data", body: `{"data":null}`, wantError: "data is required"},
		{name: "empty data", body: `{"data":[]}`, wantError: "no valid model IDs"},
		{name: "all IDs empty", body: `{"data":[{"id":""},{"id":"   "}]}`, wantError: "no valid model IDs"},
		{
			name: "filters empty IDs and normalizes valid IDs",
			body: `{"data":[{"id":" gpt-4.1 "},{"id":""},{"id":"gpt-4.1"},{"id":"o3"}]}`,
			want: []string{"gpt-4.1", "o3"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			models, err := parseOpenAIModelIDs([]byte(test.body))
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, models)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, models)
		})
	}
}

func TestFetchAdvancedCustomModelsAppliesHeaderOverrideAfterRouteAuth(t *testing.T) {
	type receivedRequest struct {
		Headers http.Header
		Host    string
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- receivedRequest{Headers: r.Header.Clone(), Host: r.Host}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1"}]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "secret-key", "/provider/models", &dto.AdvancedCustomRouteAuth{
		Type:  dto.AdvancedCustomAuthTypeHeader,
		Name:  "X-Route-Key",
		Value: "route-{api_key}",
	})
	headerOverride := `{
		"X-Route-Key":"global-{api_key}",
		"X-Static":"static-value",
		"X-Client":"{client_header:X-Client}",
		"Host":"models.example.test",
		"*":""
	}`
	channel.HeaderOverride = &headerOverride

	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-4.1"}, models)

	request := <-received
	require.Equal(t, "global-secret-key", request.Headers.Get("X-Route-Key"))
	require.Equal(t, "static-value", request.Headers.Get("X-Static"))
	require.Empty(t, request.Headers.Get("X-Client"))
	require.Equal(t, "models.example.test", request.Host)
}

func TestFetchAdvancedCustomModelsUsesEnabledSavedMultiKey(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1-mini"}]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "disabled-key\nenabled-key", "/v1/models", nil)
	channel.ChannelInfo = model.ChannelInfo{
		IsMultiKey: true,
		MultiKeyStatusList: map[int]int{
			0: common.ChannelStatusManuallyDisabled,
			1: common.ChannelStatusEnabled,
		},
	}

	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-4.1-mini"}, models)
	require.Equal(t, "Bearer enabled-key", <-authorization)
}

func TestFetchAdvancedCustomModelsRejectsNonOKResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"data":[{"id":"must-not-be-used"}]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "secret-key", "/v1/models", nil)
	models, err := fetchChannelUpstreamModelIDs(channel)
	require.ErrorContains(t, err, "status code: 502")
	require.Nil(t, models)
}

func TestFetchAdvancedCustomModelsRedactsQueryKeyFromTransportErrors(t *testing.T) {
	const secret = "secret key/+"
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	channel := newAdvancedCustomModelListChannel(baseURL, secret, "/v1/models", &dto.AdvancedCustomRouteAuth{
		Type:  dto.AdvancedCustomAuthTypeQuery,
		Name:  "custom-token",
		Value: "prefix-{api_key}",
	})

	_, err := fetchChannelUpstreamModelIDs(channel)
	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
	require.NotContains(t, err.Error(), "custom-token")
	require.NotContains(t, err.Error(), "prefix-")

	direct := sanitizeFetchModelsError(&url.Error{
		Op:  http.MethodGet,
		URL: baseURL + "/v1/models?custom-token=prefix-" + url.QueryEscape(secret),
		Err: errors.New("connection refused"),
	}, secret)
	require.EqualError(t, direct, "connection refused")

	queryValue := "prefix-" + secret
	queryError := sanitizeAdvancedCustomRequestError(
		errors.New("dial "+queryValue+": connection refused"),
		queryValue,
		baseURL+"/v1/models?custom-token="+url.QueryEscape(queryValue),
	)
	require.NotContains(t, queryError.Error(), queryValue)
	require.EqualError(t, queryError, "dial [REDACTED]: connection refused")
}

func TestFetchOrdinaryOpenAIModelsKeepsExistingEmptyDataBehavior(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list"}`))
	}))
	defer server.Close()

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "ordinary-key",
		BaseURL: &baseURL,
	}
	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestFetchModelsAdvancedCustomCreatePreview(t *testing.T) {
	receivedAuthorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthorization <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"preview-model"}]}`))
	}))
	defer server.Close()

	config := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
		IncomingPath: dto.AdvancedCustomModelListPath,
		UpstreamPath: "/preview/models",
		Converter:    "none",
	}}}
	configBytes, err := common.Marshal(config)
	require.NoError(t, err)
	rawConfig := string(configBytes)
	baseURL := server.URL
	emptyProxy := ""
	req := fetchModelsRequest{
		BaseURL:        &baseURL,
		Type:           constant.ChannelTypeAdvancedCustom,
		Key:            "create-preview-key",
		AdvancedCustom: &rawConfig,
		Proxy:          &emptyProxy,
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	FetchModels(ctx)

	var response struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	require.Equal(t, []string{"preview-model"}, response.Data)
	require.Equal(t, "Bearer create-preview-key", <-receivedAuthorization)
}

func TestFetchModelsAdvancedCustomEditPreviewUsesSavedKeyAndExplicitClears(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	receivedHeaders := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders <- r.Header.Clone()
		_, _ = w.Write([]byte(`{"data":[{"id":"edited-preview-model"}]}`))
	}))
	defer server.Close()

	savedChannel := newAdvancedCustomModelListChannel("http://127.0.0.1:1", "disabled-saved-key\nenabled-saved-key", "/saved/models", nil)
	savedChannel.Name = "saved advanced channel"
	savedChannel.Models = "old-model"
	savedChannel.ChannelInfo = model.ChannelInfo{
		IsMultiKey: true,
		MultiKeyStatusList: map[int]int{
			0: common.ChannelStatusManuallyDisabled,
			1: common.ChannelStatusEnabled,
		},
	}
	savedHeaderOverride := `{"X-Saved":"must-not-be-sent"}`
	savedChannel.HeaderOverride = &savedHeaderOverride
	savedChannel.SetSetting(dto.ChannelSettings{Proxy: "http://127.0.0.1:1"})
	require.NoError(t, db.Create(savedChannel).Error)

	preserved, err := buildAdvancedCustomModelPreviewChannel(fetchModelsRequest{ChannelID: savedChannel.Id})
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:1", preserved.GetBaseURL())
	require.Equal(t, savedHeaderOverride, *preserved.HeaderOverride)
	require.Equal(t, "http://127.0.0.1:1", preserved.GetSetting().Proxy)

	previewConfig := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
		IncomingPath: dto.AdvancedCustomModelListPath,
		UpstreamPath: "/edited/models",
		Converter:    "none",
	}}}
	configBytes, err := common.Marshal(previewConfig)
	require.NoError(t, err)
	rawConfig := string(configBytes)
	baseURL := server.URL
	explicitEmpty := ""
	req := fetchModelsRequest{
		ChannelID:      savedChannel.Id,
		BaseURL:        &baseURL,
		Type:           constant.ChannelTypeAdvancedCustom,
		Key:            "request-key-must-be-ignored",
		AdvancedCustom: &rawConfig,
		HeaderOverride: &explicitEmpty,
		Proxy:          &explicitEmpty,
	}
	cleared, err := buildAdvancedCustomModelPreviewChannel(fetchModelsRequest{
		ChannelID:      savedChannel.Id,
		BaseURL:        &explicitEmpty,
		AdvancedCustom: &rawConfig,
		HeaderOverride: &explicitEmpty,
		Proxy:          &explicitEmpty,
	})
	require.NoError(t, err)
	require.NotNil(t, cleared.BaseURL)
	require.Empty(t, *cleared.BaseURL)
	require.NotNil(t, cleared.HeaderOverride)
	require.Empty(t, *cleared.HeaderOverride)
	require.Empty(t, cleared.GetSetting().Proxy)

	body, err := common.Marshal(req)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	FetchModels(ctx)

	var response struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	require.Equal(t, []string{"edited-preview-model"}, response.Data)
	require.NotContains(t, recorder.Body.String(), "enabled-saved-key")
	require.NotContains(t, recorder.Body.String(), "request-key-must-be-ignored")

	headers := <-receivedHeaders
	require.Equal(t, "Bearer enabled-saved-key", headers.Get("Authorization"))
	require.Empty(t, headers.Get("X-Saved"))
}

func TestFailedAdvancedCustomDetectionDoesNotStageFullRemoval(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "secret-key", "/v1/models", nil)
	channel.Name = "empty discovery response"
	channel.Models = "gpt-4.1,o3"
	settings := channel.GetOtherSettings()
	settings.UpstreamModelUpdateCheckEnabled = true
	settings.UpstreamModelUpdateAutoSyncEnabled = true
	channel.SetOtherSettings(settings)
	require.NoError(t, db.Create(channel).Error)

	modelsChanged, autoAdded, err := checkAndPersistChannelUpstreamModelUpdates(channel, &settings, true, true)
	require.ErrorContains(t, err, "no valid model IDs")
	require.False(t, modelsChanged)
	require.Zero(t, autoAdded)
	require.Empty(t, settings.UpstreamModelUpdateLastDetectedModels)
	require.Empty(t, settings.UpstreamModelUpdateLastRemovedModels)

	reloaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	persistedSettings := reloaded.GetOtherSettings()
	require.Empty(t, persistedSettings.UpstreamModelUpdateLastDetectedModels)
	require.Empty(t, persistedSettings.UpstreamModelUpdateLastRemovedModels)
	require.Equal(t, "gpt-4.1,o3", reloaded.Models)
}

func TestFetchModelsUsesSharedChannelFetchBehavior(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "first-key" {
			t.Errorf("unexpected x-api-key header: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":" claude-sonnet "},{"id":"claude-sonnet"}]}`))
	}))
	t.Cleanup(server.Close)

	body, err := common.Marshal(map[string]any{
		"base_url": server.URL,
		"type":     constant.ChannelTypeAnthropic,
		"key":      "first-key\nsecond-key",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"success":true,"message":"","data":["claude-sonnet"]}`, recorder.Body.String())
}

func TestFetchNewAPIModelsUsesOpenAIContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer new-api-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data":[{"id":"gpt-5"},{"id":" gpt-5-mini "}]}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeNewAPI,
		Key:     "new-api-key",
		BaseURL: &baseURL,
	}

	models, err := fetchChannelUpstreamModelIDs(channel)

	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5", "gpt-5-mini"}, models)
}

func TestNormalizeModelNames(t *testing.T) {
	result := normalizeModelNames([]string{
		" gpt-4o ",
		"",
		"gpt-4o",
		"gpt-4.1",
		"   ",
	})

	require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, result)
}

func TestMergeModelNames(t *testing.T) {
	result := mergeModelNames(
		[]string{"gpt-4o", "gpt-4.1"},
		[]string{"gpt-4.1", " gpt-4.1-mini ", "gpt-4o"},
	)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1", "gpt-4.1-mini"}, result)
}

func TestSubtractModelNames(t *testing.T) {
	result := subtractModelNames(
		[]string{"gpt-4o", "gpt-4.1", "gpt-4.1-mini"},
		[]string{"gpt-4.1", "not-exists"},
	)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1-mini"}, result)
}

func TestIntersectModelNames(t *testing.T) {
	result := intersectModelNames(
		[]string{"gpt-4o", "gpt-4.1", "gpt-4.1", "not-exists"},
		[]string{"gpt-4.1", "gpt-4o-mini", "gpt-4o"},
	)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, result)
}

func TestApplySelectedModelChanges(t *testing.T) {
	t.Run("add and remove together", func(t *testing.T) {
		result := applySelectedModelChanges(
			[]string{"gpt-4o", "gpt-4.1", "claude-3"},
			[]string{"gpt-4.1-mini"},
			[]string{"claude-3"},
		)

		require.Equal(t, []string{"gpt-4o", "gpt-4.1", "gpt-4.1-mini"}, result)
	})

	t.Run("add wins when conflict with remove", func(t *testing.T) {
		result := applySelectedModelChanges(
			[]string{"gpt-4o"},
			[]string{"gpt-4.1"},
			[]string{"gpt-4.1"},
		)

		require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, result)
	})
}

func TestCollectPendingApplyUpstreamModelChanges(t *testing.T) {
	settings := dto.ChannelOtherSettings{
		UpstreamModelUpdateLastDetectedModels: []string{" gpt-4o ", "gpt-4o", "gpt-4.1"},
		UpstreamModelUpdateLastRemovedModels:  []string{" old-model ", "", "old-model"},
	}

	pendingAddModels, pendingRemoveModels := collectPendingApplyUpstreamModelChanges(settings)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, pendingAddModels)
	require.Equal(t, []string{"old-model"}, pendingRemoveModels)
}

func TestNormalizeChannelModelMapping(t *testing.T) {
	modelMapping := `{
		" alias-model ": " upstream-model ",
		"": "invalid",
		"invalid-target": ""
	}`
	channel := &model.Channel{
		ModelMapping: &modelMapping,
	}

	result := normalizeChannelModelMapping(channel)
	require.Equal(t, map[string]string{
		"alias-model": "upstream-model",
	}, result)
}

func TestCollectPendingUpstreamModelChangesFromModels_WithModelMapping(t *testing.T) {
	pendingAddModels, pendingRemoveModels := collectPendingUpstreamModelChangesFromModels(
		[]string{"alias-model", "gpt-4o", "stale-model"},
		[]string{"gpt-4o", "gpt-4.1", "mapped-target"},
		[]string{"gpt-4.1"},
		map[string]string{
			"alias-model": "mapped-target",
		},
	)

	require.Equal(t, []string{}, pendingAddModels)
	require.Equal(t, []string{"stale-model"}, pendingRemoveModels)
}

func TestCollectPendingUpstreamModelChangesFromModels_WithIgnoredRegexPatterns(t *testing.T) {
	pendingAddModels, pendingRemoveModels := collectPendingUpstreamModelChangesFromModels(
		[]string{"gpt-4o"},
		[]string{"gpt-4o", "claude-3-5-sonnet", "sora-video", "gpt-4.1"},
		[]string{"regex:^sora-.*$", "gpt-4.1"},
		nil,
	)

	require.Equal(t, []string{"claude-3-5-sonnet"}, pendingAddModels)
	require.Equal(t, []string{}, pendingRemoveModels)
}

func TestBuildUpstreamModelUpdateTaskNotificationContent_OmitOverflowDetails(t *testing.T) {
	channelSummaries := make([]upstreamModelUpdateChannelSummary, 0, 12)
	for i := range 12 {
		channelSummaries = append(channelSummaries, upstreamModelUpdateChannelSummary{
			ChannelName: "channel-" + string(rune('A'+i)),
			AddCount:    i + 1,
			RemoveCount: i,
		})
	}

	content := buildUpstreamModelUpdateTaskNotificationContent(
		24,
		12,
		56,
		21,
		9,
		[]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		channelSummaries,
		[]string{
			"gpt-4.1", "gpt-4.1-mini", "o3", "o4-mini", "gemini-2.5-pro", "claude-3.7-sonnet",
			"qwen-max", "deepseek-r1", "llama-3.3-70b", "mistral-large", "command-r-plus", "doubao-pro-32k",
			"hunyuan-large",
		},
		[]string{
			"gpt-3.5-turbo", "claude-2.1", "gemini-1.5-pro", "mixtral-8x7b", "qwen-plus", "glm-4",
			"yi-large", "moonshot-v1", "doubao-lite",
		},
	)

	require.Contains(t, content, "其余 4 个渠道已省略")
	require.Contains(t, content, "其余 1 个已省略")
	require.Contains(t, content, "失败渠道 ID（展示 10/12）")
	require.Contains(t, content, "其余 2 个已省略")
}

func TestShouldSendUpstreamModelUpdateNotification(t *testing.T) {
	channelUpstreamModelUpdateNotifyState.Lock()
	channelUpstreamModelUpdateNotifyState.lastNotifiedAt = 0
	channelUpstreamModelUpdateNotifyState.lastChangedChannels = 0
	channelUpstreamModelUpdateNotifyState.lastFailedChannels = 0
	channelUpstreamModelUpdateNotifyState.Unlock()

	baseTime := int64(2000000)

	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime, 6, 0))
	require.False(t, shouldSendUpstreamModelUpdateNotification(baseTime+3600, 6, 0))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+3600, 7, 0))
	require.False(t, shouldSendUpstreamModelUpdateNotification(baseTime+7200, 7, 0))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+8000, 0, 3))
	require.False(t, shouldSendUpstreamModelUpdateNotification(baseTime+9000, 0, 3))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+10000, 0, 4))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+90000, 7, 0))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+90001, 0, 0))
}

func TestDetectAllChannelUpstreamModelUpdatesRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeModelUpdate, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/upstream-models/detect-all", nil)

	DetectAllChannelUpstreamModelUpdates(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有模型更新任务正在运行或等待中")
}
