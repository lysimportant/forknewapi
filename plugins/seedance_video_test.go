package plugins_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeedanceVideoPluginContracts 在真实 JS 引擎中验证模型映射、统一字段、异步状态和内容代理合同。
func TestSeedanceVideoPluginContracts(t *testing.T) {
	source, err := plugins.Source("seedance-video")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, "seedance-video", plugin.Meta.Key)
	assert.Empty(t, plugin.Meta.Models)
	assert.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, "count", plugin.Meta.UsageSchema["count"].Unit)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["count"].Description["en"])

	alias := "我的 Seedance 渠道名"
	upstream := "Xseadanco2.0"
	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": alias, "upstreamModel": upstream,
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": alias, "prompt": "A slow camera move", "duration": 6, "ratio": "16:9",
			"generate_audio": false, "images": []any{"https://cdn.example/start.png"},
		}},
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(decoded)
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.Unmarshal(encoded, &intent))
	assert.Equal(t, alias, intent["model"])
	assert.Equal(t, alias, intent["requestBody"].(map[string]any)["model"])

	descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"model": alias, "upstreamModel": upstream, "apiKey": "synthetic-key", "publicTaskId": "task-seedance-1",
		"baseUrl": "https://channel.example", "requestBody": intent["requestBody"],
	})
	require.NoError(t, err)
	encoded, err = common.Marshal(descriptor)
	require.NoError(t, err)
	var request map[string]any
	require.NoError(t, common.Unmarshal(encoded, &request))
	assert.Equal(t, "https://channel.example/v1/videos", request["url"])
	assert.Equal(t, true, request["noRetry"])
	assert.Equal(t, "task-seedance-1", request["headers"].(map[string]any)["Idempotency-Key"])
	sent := request["body"].(map[string]any)
	assert.Equal(t, upstream, sent["model"])
	assert.Equal(t, false, sent["generate_audio"])
	assert.Equal(t, []any{"https://cdn.example/start.png"}, sent["images"])

	usage, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
		"model": alias, "upstreamModel": upstream, "requestBody": intent["requestBody"], "usagePurpose": "facts",
	})
	require.NoError(t, err)
	encoded, err = common.Marshal(usage)
	require.NoError(t, err)
	assert.JSONEq(t, `{"count":1}`, string(encoded))

	t.Run("状态和内容地址", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			body   map[string]any
			status string
		}{
			{name: "processing", body: map[string]any{"status": "processing"}, status: "IN_PROGRESS"},
			{name: "succeeded", body: map[string]any{"status": "succeeded", "result_url": "https://cdn.example/video.mp4"}, status: "SUCCESS"},
			{name: "failed", body: map[string]any{"status": "failed", "error_code": "quota", "error_detail": "insufficient"}, status: "FAILURE"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"apiKey": "synthetic-key"}, tc.body)
				require.NoError(t, callErr)
				encoded, marshalErr := common.Marshal(value)
				require.NoError(t, marshalErr)
				var result map[string]any
				require.NoError(t, common.Unmarshal(encoded, &result))
				assert.Equal(t, tc.status, result["status"])
				if tc.status == "FAILURE" {
					assert.Equal(t, "quota: insufficient", result["reason"])
				}
			})
		}

		value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{"status": "succeeded"})
		require.NoError(t, callErr)
		encoded, err := common.Marshal(value)
		require.NoError(t, err)
		var unknown map[string]any
		require.NoError(t, common.Unmarshal(encoded, &unknown))
		assert.Equal(t, "UNKNOWN", unknown["status"])

		value, callErr = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{
			"artifactKey": "video", "data": map[string]any{"status": "succeeded", "video_url": "https://cdn.example/video.mp4"},
			"clientRequest": map[string]any{"method": "GET"},
		})
		require.NoError(t, callErr)
		encoded, err = common.Marshal(value)
		require.NoError(t, err)
		var content map[string]any
		require.NoError(t, common.Unmarshal(encoded, &content))
		assert.Equal(t, "https://cdn.example/video.mp4", content["url"])
		assert.Equal(t, true, content["credentialless"])
		assert.NotContains(t, content, "headers")
	})

	for _, bad := range []map[string]any{
		{"prompt": "x", "extra": map[string]any{"vendor": "value"}},
		{"prompt": "x", "duration": 3601},
		{"prompt": "x", "duration": 0},
		{"prompt": "x", "duration": -1},
		{"prompt": "x", "duration": 6, "seconds": 7},
		{"prompt": "x", "end_frame": "https://cdn.example/end.png"},
		{"prompt": "x", "generate_audio": "false"},
		{"prompt": "x", "images": []any{"data:image/png;base64,abc"}},
	} {
		_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": upstream, "upstreamModel": upstream, "apiKey": "synthetic-key", "publicTaskId": "task-bad",
			"baseUrl": "https://channel.example", "requestBody": bad,
		})
		assert.Error(t, callErr)
	}

	t.Run("纯文本multipart和文件拒绝", func(t *testing.T) {
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "future-provider-name", "body": map[string]any{"kind": "multipart", "fields": map[string]any{
				"model": []string{"future-provider-name"}, "prompt": []string{"A slow camera move"},
				"seconds": []string{"6"}, "generate_audio": []string{"false"}, "images": []string{`["https://cdn.example/start.png"]`},
			}},
		})
		require.NoError(t, callErr)
		intent := value.(map[string]any)
		body := intent["requestBody"].(map[string]any)
		assert.EqualValues(t, 6, body["duration"])
		assert.Equal(t, false, body["generate_audio"])
		assert.Equal(t, "image_to_video", intent["action"])
		_, callErr = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": upstream, "body": map[string]any{"kind": "multipart", "files": []any{map[string]any{"ref": "file-1"}}},
		})
		require.ErrorContains(t, callErr, "/v1/media")
	})

	t.Run("创建失败缺字段和终态", func(t *testing.T) {
		for _, bad := range []any{nil, "not JSON", map[string]any{"code": 0, "success": true}, map[string]any{"code": 1, "success": false, "task_id": "vid-1"}} {
			_, callErr := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": bad})
			assert.Error(t, callErr)
		}
		value, callErr := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": map[string]any{
			"code": 0, "success": true, "task_id": "vid-1", "status": "succeeded", "download_url": "https://cdn.example/video.mp4?download=1",
		}})
		require.NoError(t, callErr)
		parsed := value.(map[string]any)
		assert.Equal(t, "vid-1", parsed["taskId"])
		assert.Equal(t, "SUCCESS", parsed["immediate"].(map[string]any)["status"])
		value, callErr = plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"apiKey": "synthetic-key"}, map[string]any{
			"status": "failed", "error_code": "vendor_failure", "error_detail": "synthetic-key Bearer sensitive-token sk-fixturetoken",
		})
		require.NoError(t, callErr)
		reason := value.(map[string]any)["reason"].(string)
		assert.Contains(t, reason, "vendor_failure")
		assert.NotContains(t, reason, "synthetic-key")
		assert.NotContains(t, reason, "sensitive-token")
		assert.NotContains(t, reason, "sk-fixturetoken")
	})
}

// TestSeedanceVideoHostFlow 用本地上游验证映射名称、创建幂等、完成用量及无凭据内容描述，不调用收费接口。
func TestSeedanceVideoHostFlow(t *testing.T) {
	source, err := plugins.Source("seedance-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	service.InitHttpClient()
	for _, upstream := range []string{"Xseadanco2.0", "kksedanco2.5", "future-provider-name"} {
		t.Run(upstream, func(t *testing.T) {
			posts, queries := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer fixture-only", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "POST /gateway/v1/videos":
					posts++
					assert.Equal(t, "task_seedance_fixture", r.Header.Get("Idempotency-Key"))
					var body map[string]any
					if !assert.NoError(t, common.DecodeJson(r.Body, &body)) {
						return
					}
					assert.Equal(t, upstream, body["model"])
					assert.Equal(t, false, body["generate_audio"])
					_, writeErr := io.WriteString(w, `{"code":0,"success":true,"task_id":"vid-fixture","status":"processing","next_poll_seconds":10}`)
					assert.NoError(t, writeErr)
				case "GET /gateway/v1/tasks/vid-fixture":
					queries++
					_, writeErr := io.WriteString(w, `{"task_id":"vid-fixture","status":"succeeded","video_url":"https://cdn.example/video.mp4"}`)
					assert.NoError(t, writeErr)
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()
			info := &relaycommon.RelayInfo{
				OriginModelName: "我的视频模型", ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL + "/gateway/v1", ApiKey: "fixture-only", UpstreamModelName: upstream},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_seedance_fixture"},
			}
			adaptor := taskplugin.New(plugin)
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			c.Set("resolved_task_model", info.OriginModelName)
			c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Plugin: plugin, Protocol: "openai_video", Model: info.OriginModelName, ChannelScoped: true})
			c.Set(jsplugin.ContextKeyProtocolRequest, jsplugin.ProtocolRequestContext{
				Model: info.OriginModelName, Protocol: "openai_video", RouteRequestContext: jsplugin.RouteRequestContext{Body: map[string]any{
					"kind": "json", "value": map[string]any{"model": info.OriginModelName, "prompt": "A calm ocean wave", "duration": 6, "generate_audio": false},
				}},
			})
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			assert.True(t, adaptor.SkipSubmitRetry())
			facts, err := adaptor.ExtractUsageFactsValidated(c, info)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"count": float64(1)}, facts)
			ratios, err := adaptor.EstimateBillingValidated(c, info)
			require.NoError(t, err)
			assert.Empty(t, ratios)
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			resp, err := adaptor.DoRequest(c, info, body)
			require.NoError(t, err)
			defer resp.Body.Close()
			parsed, taskErr := adaptor.ParseResponse(c, resp, info)
			require.Nil(t, taskErr)
			require.NotNil(t, parsed)
			task := &model.Task{TaskID: info.PublicTaskID, Properties: model.Properties{OriginModelName: info.OriginModelName, UpstreamModelName: upstream}, PrivateData: model.TaskPrivateData{UpstreamTaskID: parsed.UpstreamTaskID}}
			queryResp, err := adaptor.FetchTask(info.ChannelBaseUrl, info.ApiKey, task, "")
			require.NoError(t, err)
			defer queryResp.Body.Close()
			completion, err := io.ReadAll(queryResp.Body)
			require.NoError(t, err)
			result, err := adaptor.ParseTaskResult(task, queryResp, completion)
			require.NoError(t, err)
			assert.Equal(t, "SUCCESS", result.Status)
			assert.Equal(t, map[string]any{"count": float64(1)}, result.UsageFacts)
			task.Status, task.Data = model.TaskStatusSuccess, completion
			artifacts, err := adaptor.ListArtifacts(task)
			require.NoError(t, err)
			require.Len(t, artifacts, 1)
			content, err := adaptor.BuildContentRequest(task, artifacts[0].Key, channel.TaskArtifactClientRequest{Method: http.MethodGet})
			require.NoError(t, err)
			assert.True(t, content.Credentialless)
			assert.Empty(t, content.Headers)
			assert.Equal(t, "https://cdn.example/video.mp4", content.URL)
			failed, err := adaptor.ParseTaskResult(task, queryResp, []byte(`{"status":"failed","error_code":"generation_failed","error_detail":"provider failure"}`))
			require.NoError(t, err)
			assert.Equal(t, "FAILURE", failed.Status)
			assert.Empty(t, failed.UsageFacts)
			assert.Equal(t, 1, posts)
			assert.Equal(t, 1, queries)
			invalid := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true}`))}
			_, taskErr = adaptor.ParseResponse(c, invalid, info)
			require.NotNil(t, taskErr)
			assert.True(t, taskErr.NoRetry)
		})
	}
}
