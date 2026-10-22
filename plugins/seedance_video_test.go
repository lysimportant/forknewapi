package plugins_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
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
	require.Len(t, plugin.Meta.UsageExamples, 4)
	for i, label := range []string{"480p", "720p", "1080p", "2K"} {
		assert.Equal(t, label, plugin.Meta.UsageExamples[i].Label)
	}

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

	t.Run("画布字段转换为文档字段", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			fields map[string]any
			want   string
		}{
			{
				name: "通用首尾帧",
				fields: map[string]any{
					"image": map[string]any{"url": "https://cdn.example/start.png"}, "last_frame": "https://cdn.example/end.png",
				},
				want: `{"model":"Xseadanco2.0","prompt":"ocean","start_frame":"https://cdn.example/start.png","end_frame":"https://cdn.example/end.png"}`,
			},
			{
				name: "全能参考保留每类素材顺序与重复引用",
				fields: map[string]any{
					"reference_images": []any{map[string]any{"url": "https://cdn.example/a.png"}, map[string]any{"url": "https://cdn.example/a.png"}},
					"reference_videos": []any{"https://cdn.example/one.mp4"},
					"negative_prompt":  "blur",
					"metadata": map[string]any{"content": []any{
						map[string]any{"type": "video_url", "role": "content", "video_url": map[string]any{"url": "https://cdn.example/two.mp4"}},
						map[string]any{"type": "audio_url", "role": "audioTrack", "audio_url": map[string]any{"url": "https://cdn.example/ref.wav"}},
					}},
				},
				want: `{"model":"Xseadanco2.0","prompt":"ocean","negative_prompt":"blur","images":["https://cdn.example/a.png","https://cdn.example/a.png"],"reference_videos":["https://cdn.example/one.mp4","https://cdn.example/two.mp4"],"reference_audios":["https://cdn.example/ref.wav"]}`,
			},
			{
				name: "结构化媒体和参数",
				fields: map[string]any{
					"seconds": "30", "generate_audio": false,
					"metadata": map[string]any{"resolution": "720p", "ratio": "16:9", "omni_reference_task_type": "reference", "content": []any{
						map[string]any{"type": "text", "text": "ocean"},
						map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
						map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "https://cdn.example/start.png"}},
						map[string]any{"type": "image_url", "role": "last_frame", "image_url": map[string]any{"url": "https://cdn.example/end.png"}},
						map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
						map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.wav"}},
					}},
				},
				want: `{"model":"Xseadanco2.0","prompt":"ocean","duration":30,"resolution":"720p","ratio":"16:9","generate_audio":false,"images":["https://cdn.example/ref.png"],"start_frame":"https://cdn.example/start.png","end_frame":"https://cdn.example/end.png","reference_videos":["https://cdn.example/ref.mp4"],"reference_audios":["https://cdn.example/ref.wav"]}`,
			},
			{
				name: "图片数组显式帧角色",
				fields: map[string]any{"reference_images": []any{
					map[string]any{"url": "https://cdn.example/start.png", "role": "first_frame"},
					map[string]any{"url": "https://cdn.example/end.png", "role": "last_frame"},
					map[string]any{"url": "https://cdn.example/ref.png", "role": "reference_image"},
				}},
				want: `{"model":"Xseadanco2.0","prompt":"ocean","start_frame":"https://cdn.example/start.png","end_frame":"https://cdn.example/end.png","images":["https://cdn.example/ref.png"]}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.fields["prompt"] = "ocean"
				value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": alias, "upstreamModel": upstream, "body": map[string]any{"kind": "json", "value": tc.fields},
				})
				require.NoError(t, callErr)
				intent := value.(map[string]any)
				value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"model": alias, "upstreamModel": upstream, "baseUrl": "https://channel.example", "publicTaskId": "task-canvas-fixture",
					"requestBody": intent["requestBody"],
				})
				require.NoError(t, callErr)
				encoded, err := common.Marshal(value.(map[string]any)["body"])
				require.NoError(t, err)
				assert.JSONEq(t, tc.want, string(encoded))
			})
		}
	})

	t.Run("画布别名不能绕过校验或静默丢弃参数", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			fields map[string]any
		}{
			{"参考图未知字段", map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/ref.png", "extra": "ignored"}}}},
			{"参考图无效地址", map[string]any{"reference_images": []any{map[string]any{"url": "data:image/png;base64,aW1hZ2U="}}}},
			{"首帧字段冲突", map[string]any{"image": "https://cdn.example/a.png", "start_frame": "https://cdn.example/b.png"}},
			{"尾帧需要首帧", map[string]any{"last_frame": "https://cdn.example/end.png"}},
			{"嵌套参数冲突", map[string]any{"aspect_ratio": "16:9", "metadata": map[string]any{"ratio": "9:16"}}},
			{"未知嵌套字段", map[string]any{"metadata": map[string]any{"extra": true}}},
			{"编辑不能降级为参考", map[string]any{"metadata": map[string]any{"omni_reference_task_type": "edit"}}},
			{"文字不能被忽略", map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "different prompt"}}}}},
			{"音频不能冒充帧", map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "audio_url", "role": "first_frame", "audio_url": map[string]any{"url": "https://cdn.example/ref.wav"}}}}}},
			{"嵌套地址需要HTTPS", map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "video_url", "video_url": map[string]any{"url": "http://cdn.example/ref.mp4"}}}}}},
			{"嵌套媒体未知字段", map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/ref.png", "file_id": "ignored"}}}}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.fields["prompt"] = "ocean"
				_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": upstream, "body": map[string]any{"kind": "json", "value": tc.fields},
				})
				assert.Error(t, callErr)
			})
		}
	})

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

// TestSeedanceVideoBillingSchemes 验证两种售价、清晰度归一化、缺失计量拒绝和冻结事实结算，不调用收费接口。
func TestSeedanceVideoBillingSchemes(t *testing.T) {
	source, err := plugins.Source("seedance-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.Equal(t, []string{"480p", "720p", "1080p", "2k"}, plugin.Meta.UsageSchema["video_resolution"].Enum)

	const perCall = `u("count") * 0.35`
	const perSecond = `u("video_resolution") == "480p" ? tier("480p", u("seconds") * 0.01) : u("video_resolution") == "720p" ? tier("720p", u("seconds") * 0.02) : u("video_resolution") == "1080p" ? tier("1080p", u("seconds") * 0.03) : tier("2k", u("seconds") * 0.04)`
	const zeroPrice = `u("video_resolution") == "480p" ? tier("480p", u("seconds") * 0) : u("video_resolution") == "720p" ? tier("720p", u("seconds") * 0) : u("video_resolution") == "1080p" ? tier("1080p", u("seconds") * 0) : tier("2k", u("seconds") * 0)`
	for _, tc := range []struct {
		name       string
		fields     map[string]any
		resolution string
		cost       float64
	}{
		{name: "480p", fields: map[string]any{"duration": 2.5, "resolution": "480p"}, resolution: "480p", cost: 0.025},
		{name: "720p", fields: map[string]any{"duration": 2.5, "resolution": "720p"}, resolution: "720p", cost: 0.05},
		{name: "1080p", fields: map[string]any{"duration": 2.5, "resolution": "1080p"}, resolution: "1080p", cost: 0.075},
		{name: "2K", fields: map[string]any{"duration": 2.5, "resolution": "2K"}, resolution: "2k", cost: 0.1},
		{name: "480入口别名", fields: map[string]any{"seconds": "2.5", "size": "480"}, resolution: "480p", cost: 0.025},
		{name: "720无后缀", fields: map[string]any{"duration": 2.5, "resolution": "720"}, resolution: "720p", cost: 0.05},
		{name: "1080无后缀", fields: map[string]any{"duration": 2.5, "resolution": "1080"}, resolution: "1080p", cost: 0.075},
		{name: "2K去空白", fields: map[string]any{"duration": 2.5, "resolution": " 2K "}, resolution: "2k", cost: 0.1},
		{name: "旧按次无计量"},
		{name: "缺时长", fields: map[string]any{"resolution": "1080p"}},
		{name: "缺清晰度", fields: map[string]any{"duration": 2.5}},
		{name: "未知清晰度仍按次", fields: map[string]any{"duration": 2.5, "resolution": "540p"}},
		{name: "不猜测像素尺寸", fields: map[string]any{"duration": 2.5, "size": "2048x1080"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{"model": "我的视频模型", "prompt": "A calm ocean wave"}
			for field, value := range tc.fields {
				request[field] = value
			}
			info := &relaycommon.RelayInfo{
				OriginModelName: "我的视频模型", ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://channel.example", UpstreamModelName: "Xseadanco2.0"},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_billing_fixture"},
			}
			adaptor := taskplugin.New(plugin)
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			c.Set("resolved_task_model", info.OriginModelName)
			c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Plugin: plugin, Protocol: "openai_video", Model: info.OriginModelName, ChannelScoped: true})
			c.Set(jsplugin.ContextKeyProtocolRequest, jsplugin.ProtocolRequestContext{
				Model: info.OriginModelName, Protocol: "openai_video", RouteRequestContext: jsplugin.RouteRequestContext{Body: map[string]any{"kind": "json", "value": request}},
			})
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			facts, err := adaptor.ExtractUsageFactsValidated(c, info)
			require.NoError(t, err)
			expectedFacts := map[string]any{"count": float64(1)}
			if tc.resolution != "" {
				expectedFacts["seconds"] = 2.5
				expectedFacts["video_resolution"] = tc.resolution
			}
			assert.Equal(t, expectedFacts, facts)
			ratios, err := adaptor.EstimateBillingValidated(c, info)
			require.NoError(t, err)
			assert.Empty(t, ratios)
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			var upstream map[string]any
			require.NoError(t, common.DecodeJson(body, &upstream))
			if value, exists := tc.fields["resolution"]; exists {
				assert.Equal(t, value, upstream["resolution"])
			} else if value, exists := tc.fields["size"]; exists {
				assert.Equal(t, value, upstream["resolution"])
			}
			cost, _, err := billingexpr.RunExprWithRequest(perCall, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
			require.NoError(t, err)
			assert.Equal(t, 0.35, cost)
			expression := perCall
			for _, candidate := range []string{perSecond, zeroPrice} {
				cost, _, err = billingexpr.RunExprWithRequest(candidate, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
				if tc.resolution == "" {
					require.Error(t, err, "缺失计量在零价分支也必须拒绝")
					continue
				}
				require.NoError(t, err)
				if candidate == perSecond {
					assert.InDelta(t, tc.cost, cost, 1e-10)
					expression = candidate
				} else {
					assert.Zero(t, cost)
				}
			}
			// 完成响应的未确认时长不进入计量；结算沿用快照中的请求时长、清晰度和原售价。
			snapshot := &billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 2, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: facts}
			task := &model.Task{Properties: model.Properties{OriginModelName: info.OriginModelName, UpstreamModelName: "Xseadanco2.0"}}
			response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
			completed, err := adaptor.ParseTaskResult(task, response, []byte(`{"status":"succeeded","video_url":"https://cdn.example/video.mp4","duration":9999,"resolution":"540p"}`))
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"count": float64(1)}, completed.UsageFacts)
			settled, merged, err := service.EvaluateTaskCompletionUsage(snapshot, completed.UsageFacts)
			require.NoError(t, err)
			assert.Equal(t, expectedFacts, merged)
			assert.Equal(t, expectedFacts, snapshot.UsageFacts)
			wantCost := 0.35
			if tc.resolution != "" {
				wantCost = tc.cost
			}
			assert.InDelta(t, wantCost*500000, settled.ActualQuotaBeforeGroup, 1e-7)
			assert.Equal(t, common.QuotaRound(wantCost*1000000), settled.ActualQuotaAfterGroup)
			assert.Nil(t, settled.Clamp)
		})
	}
}

// TestSeedanceVideoHostFlow 用本地上游验证映射名称、创建幂等、完成用量及无凭据内容描述，不调用收费接口。
func TestSeedanceVideoHostFlow(t *testing.T) {
	source, err := plugins.Source("seedance-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	service.InitHttpClient()
	for _, upstream := range []string{"Cseadanco2.5K", "LLseadanco2.5X", "future-provider-name"} {
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
					assert.Equal(t, map[string]any{
						"model": upstream, "prompt": "A calm ocean wave", "duration": float64(30), "resolution": "720p", "ratio": "16:9",
						"generate_audio": false, "images": []any{"https://cdn.example/ref.png"},
					}, body)
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
					"kind": "json", "value": map[string]any{
						"model": info.OriginModelName, "prompt": "A calm ocean wave", "duration": 30, "seconds": "30", "resolution": "720p", "aspect_ratio": "16:9", "generate_audio": false,
						"reference_images": []any{map[string]any{"url": "https://cdn.example/ref.png"}},
					},
				}},
			})
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			assert.True(t, adaptor.SkipSubmitRetry())
			facts, err := adaptor.ExtractUsageFactsValidated(c, info)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(30), "video_resolution": "720p"}, facts)
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
			content, err := adaptor.BuildContentRequest(t.Context(), task, artifacts[0].Key, channel.TaskArtifactClientRequest{Method: http.MethodGet})
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
