package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const metasoBaseURL = "https://b.metaso.cn/api/minimax"

func loadMetasoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("metaso-h3-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	return plugin
}

func metasoSubmitContext(body map[string]any) map[string]any {
	return map[string]any{"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "baseUrl": metasoBaseURL, "apiKey": "fixture-only", "requestBody": body}
}

func metasoMedia(kind, role, url string) map[string]any {
	return map[string]any{"type": kind, kind: map[string]any{"url": url}, "role": role}
}

// TestMetasoH3VideoContracts 使用真实插件宿主核实协议和模型隔离，不访问上游。
func TestMetasoH3VideoContracts(t *testing.T) {
	source, err := plugins.Source("metaso-h3-video")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, metasoBaseURL, plugin.Meta.BaseURL)
	assert.Equal(t, "1.2.0", plugin.Meta.Version)
	assert.Equal(t, "number", plugin.Meta.UsageSchema["input_video_seconds"].Type)
	assert.Equal(t, "second", plugin.Meta.UsageSchema["input_video_seconds"].Unit)
	assert.Equal(t, []string{"Minimax-H3"}, plugin.Meta.Models)
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)
	assert.Equal(t, []string{"480p", "768p", "1080p", "2k"}, plugin.Meta.UsageSchema["size"].Enum)
	binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", "Minimax-H3")
	require.True(t, found)
	assert.Same(t, plugin, binding.Plugin)
	_, found = registry.Generation().LookupEndpoint("POST", "/v1/videos", "MiniMax-H3")
	assert.False(t, found, "保留秘塔对外模型名，不抢占官方模型")
	_, found = registry.Generation().LookupEndpoint("POST", "/v1/responses", "Minimax-H3")
	assert.False(t, found)

	body := map[string]any{"model": "Minimax-H3", "prompt": "flower", "seconds": 5, "size": "480p"}
	request := metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(body))
	assert.Equal(t, metasoBaseURL+"/v2/video_generation", request["url"])
	assert.Equal(t, "POST", request["method"])
	assert.Equal(t, true, request["noRetry"])
	assert.Equal(t, "Bearer fixture-only", request["headers"].(map[string]any)["Authorization"])
	assert.JSONEq(t, `{"model":"MiniMax-H3","content":[{"type":"text","text":"flower"}],"duration":5,"resolution":"480p","ratio":"16:9"}`, metasoJSON(t, request["body"]))
	assert.Equal(t, "text_to_video", commandAction(t, plugin, body))
	hailuoSource, err := plugins.Source("hailuo")
	require.NoError(t, err)
	hailuo, err := registry.RegisterFactory(hailuoSource, jsplugin.Options{})
	require.NoError(t, err)
	binding, found = registry.Generation().LookupEndpoint("POST", "/v1/videos", "MiniMax-H3")
	require.True(t, found)
	assert.Same(t, hailuo, binding.Plugin)
}

// TestMetasoH3NativeExample 确认文档的 content、角色、比例与 false 原样进入上游请求。
func TestMetasoH3NativeExample(t *testing.T) {
	plugin := loadMetasoPlugin(t)
	const example = `{"model":"MiniMax-H3","content":[{"type":"text","text":"Pull focus to the people in the background and add more steam to the ramen bowl."},{"type":"image_url","image_url":{"url":"https://cdn.example/ramen.png"},"role":"first_frame"}],"resolution":"2K","duration":5,"ratio":"adaptive","fast_mode":false}`
	var body map[string]any
	require.NoError(t, common.UnmarshalJsonStr(example, &body))
	assert.Equal(t, "image_to_video", commandAction(t, plugin, body))
	request := metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(body))
	assert.JSONEq(t, example, metasoJSON(t, request["body"]))
	assert.JSONEq(t, `{"seconds":5,"size":"2k","input_images":1,"input_videos":0,"input_video_seconds":0,"input_audios":0}`, metasoJSON(t, metasoHookObject(t, plugin, "extractUsage", metasoSubmitContext(body))))
	body["fast_mode"] = true
	request = metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(body))
	assert.Equal(t, true, request["body"].(map[string]any)["fast_mode"])
	ctx := metasoSubmitContext(body)
	ctx["usagePurpose"] = "billing_ratios"
	assert.Empty(t, metasoHookObject(t, plugin, "extractUsage", ctx))
}

func TestMetasoH3BaseURLs(t *testing.T) {
	plugin := loadMetasoPlugin(t)
	for _, test := range []struct{ base, want string }{
		{"", metasoBaseURL}, {metasoBaseURL + "/", metasoBaseURL},
		{metasoBaseURL + "/v2", metasoBaseURL}, {metasoBaseURL + "/v2/video_generation", metasoBaseURL},
		{"https://metaso.cn/api/openai", metasoBaseURL}, {"https://metaso.cn/api/openai/v1/", metasoBaseURL},
		{"https://metaso.cn/api/openai/v1/videos", metasoBaseURL},
		{"https://proxy.example/prefix", "https://proxy.example/prefix"}, {"https://proxy.example/prefix/v1/videos", "https://proxy.example/prefix"},
	} {
		t.Run(test.base, func(t *testing.T) {
			ctx := metasoSubmitContext(map[string]any{"prompt": "flower", "seconds": 5})
			ctx["baseUrl"] = test.base
			request := metasoHookObject(t, plugin, "buildSubmitRequest", ctx)
			assert.Equal(t, test.want+"/v2/video_generation", request["url"])
			if test.base != "" {
				require.NoError(t, jsplugin.ValidateRequestURL(request["url"].(string), test.base, plugin.Meta.AllowedHosts))
			}
			ctx["taskId"] = "task/id?x"
			query := metasoHookObject(t, plugin, "buildQueryRequest", ctx)
			assert.Equal(t, test.want+"/v2/query/video_generation/task%2Fid%3Fx", query["url"])
			assert.Equal(t, "GET", query["method"])
		})
	}
}

func TestMetasoH3LegacyAndCanvasInputs(t *testing.T) {
	plugin := loadMetasoPlugin(t)
	for _, test := range []struct {
		name         string
		body         map[string]any
		want, action string
	}{
		{
			name:   "legacy single image",
			body:   map[string]any{"prompt": "flower", "seconds": 6, "size": "768p", "input_reference": map[string]any{"image_url": "https://cdn.example/first.png"}},
			want:   `{"model":"MiniMax-H3","content":[{"type":"text","text":"flower"},{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"}],"duration":6,"resolution":"768P","ratio":"adaptive"}`,
			action: "image_to_video",
		},
		{
			name: "Canvas mixed frames and references keep roles",
			body: map[string]any{"prompt": "flower", "seconds": 5, "size": "1080p", "aspect_ratio": "9:16", "metadata": map[string]any{
				"fast_mode": false, "aigc_watermark": false, "content": []any{
					metasoMedia("image_url", "first_frame", "https://cdn.example/first.png"), metasoMedia("image_url", "last_frame", "https://cdn.example/last.png"), metasoMedia("image_url", "reference_image", "https://cdn.example/reference.png"),
				},
			}},
			want:   `{"model":"MiniMax-H3","content":[{"type":"text","text":"flower"},{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"},{"type":"image_url","image_url":{"url":"https://cdn.example/reference.png"},"role":"reference_image"}],"duration":5,"resolution":"1080p","ratio":"9:16","fast_mode":false,"aigc_watermark":false}`,
			action: "reference_to_video",
		},
		{
			name:   "one image with video and audio is not dropped",
			body:   map[string]any{"prompt": "flower", "seconds": 8, "size": "2k", "mode": "omni_reference", "images": []any{"https://cdn.example/image.png"}, "videos": []any{"https://cdn.example/video.mp4"}, "audios": []any{"https://cdn.example/audio.mp3"}},
			want:   `{"model":"MiniMax-H3","content":[{"type":"text","text":"flower"},{"type":"image_url","image_url":{"url":"https://cdn.example/image.png"},"role":"reference_image"},{"type":"video_url","video_url":{"url":"https://cdn.example/video.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://cdn.example/audio.mp3"},"role":"reference_audio"}],"duration":8,"resolution":"2K","ratio":"adaptive"}`,
			action: "reference_to_video",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(test.body))
			assert.JSONEq(t, test.want, metasoJSON(t, request["body"]))
			assert.Equal(t, test.action, commandAction(t, plugin, test.body))
		})
	}
	for _, test := range []struct{ size, resolution, ratio string }{
		{"480p", "480p", "16:9"}, {"768P", "768P", "16:9"}, {"1080p", "1080p", "16:9"}, {"2K", "2K", "16:9"},
		{"1280x720", "768P", "16:9"}, {"720x1280", "768P", "9:16"}, {"1792x1024", "2K", "16:9"}, {"1024x1792", "2K", "9:16"},
	} {
		t.Run(test.size, func(t *testing.T) {
			request := metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(map[string]any{"prompt": "flower", "seconds": 5, "size": test.size}))
			body := request["body"].(map[string]any)
			assert.Equal(t, test.resolution, body["resolution"])
			assert.Equal(t, test.ratio, body["ratio"])
		})
	}
	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "body": map[string]any{"kind": "form", "fields": map[string]any{
			"content": []string{`[{"type":"text","text":"flower"}]`}, "duration": []string{"5"}, "fast_mode": []string{"false"}, "metadata": []string{`{"ratio":"9:16"}`},
		}},
	})
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.UnmarshalJsonStr(metasoJSON(t, decoded), &intent))
	request := metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(intent["requestBody"].(map[string]any)))
	assert.Equal(t, false, request["body"].(map[string]any)["fast_mode"])
	assert.Equal(t, "9:16", request["body"].(map[string]any)["ratio"])
}

func TestMetasoH3Validation(t *testing.T) {
	plugin := loadMetasoPlugin(t)
	for name, fields := range map[string]map[string]any{
		"unsupported size": {"size": "720p"}, "too short": {"seconds": 3}, "too long": {"seconds": 31}, "fraction": {"seconds": 5.5},
		"high resolution long video": {"seconds": 16, "size": "2K"}, "duration conflict": {"duration": 6},
		"resolution conflict": {"size": "768p", "resolution": "2K"}, "bad boolean": {"fast_mode": 0}, "null boolean": {"fast_mode": nil},
		"metadata conflict": {"fast_mode": false, "metadata": map[string]any{"fast_mode": true}},
		"invalid metadata":  {"metadata": "not-json"}, "metadata array": {"metadata": []any{}},
		"invalid content": {"content": "not-json"}, "invalid item": {"content": []any{true}},
		"invalid role":          {"content": []any{metasoMedia("video_url", "first_frame", "https://cdn.example/video.mp4")}},
		"duplicate first frame": {"content": []any{metasoMedia("image_url", "first_frame", "https://cdn.example/a.png"), metasoMedia("image_url", "first_frame", "https://cdn.example/b.png")}},
		"nonpublic image":       {"image": "data:image/png;base64,abc"}, "invalid ratio": {"ratio": "5:7"}, "adaptive without visual": {"ratio": "adaptive"},
		"ratio conflict": {"ratio": "16:9", "aspect_ratio": "9:16"}, "no text": {"prompt": ""},
	} {
		t.Run(name, func(t *testing.T) {
			body := map[string]any{"prompt": "flower", "seconds": 5}
			for key, value := range fields {
				body[key] = value
			}
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", metasoSubmitContext(body))
			require.Error(t, err)
		})
	}
	for _, base := range []string{"https://metaso.cn/api/openai?x=1", "https://user:pass@example.com", "https://example.com/#fragment"} {
		ctx := metasoSubmitContext(map[string]any{"prompt": "flower", "seconds": 5})
		ctx["baseUrl"] = base
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
		require.Error(t, err)
	}
	for _, size := range []string{"480p", "768P"} {
		request := metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(map[string]any{"prompt": "flower", "seconds": 30, "size": size}))
		assert.Equal(t, float64(30), request["body"].(map[string]any)["duration"])
	}
	for _, kind := range []struct {
		field string
		count int
	}{{"images", 9}, {"videos", 3}, {"audios", 3}} {
		body := map[string]any{"prompt": "flower", "seconds": 5, "mode": "omni_reference"}
		values := make([]any, kind.count+1)
		for index := range values {
			values[index] = "https://cdn.example/reference"
		}
		body[kind.field] = values[:kind.count]
		metasoHookObject(t, plugin, "buildSubmitRequest", metasoSubmitContext(body))
		body[kind.field] = values
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", metasoSubmitContext(body))
		require.Error(t, err, kind.field)
	}
}

func TestMetasoH3SubmitAndQueryResults(t *testing.T) {
	plugin := loadMetasoPlugin(t)
	response := metasoHookObject(t, plugin, "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": map[string]any{"task_id": "native-task"}})
	assert.Equal(t, "native-task", response["taskId"])
	for _, body := range []map[string]any{
		{"request_id": "not-a-task"}, {"id": "not-native"}, {"task_id": " "},
		{"task_id": "invalid", "error": map[string]any{"message": "denied"}},
		{"task_id": "invalid", "base_resp": map[string]any{"status_code": 1004, "status_msg": "denied"}},
	} {
		_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": body})
		require.Error(t, err)
	}
	_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 500, "body": map[string]any{"task_id": "not-accepted"}})
	require.Error(t, err)
	for _, test := range []struct{ body, status, reason string }{
		{`{"task":{"status":"queued"}}`, "QUEUED", ""}, {`{"task":{"status":"running"}}`, "IN_PROGRESS", ""},
		{`{"task":{"status":"succeeded","content":{"url":"https://cdn.example/result.mp4"}}}`, "SUCCESS", ""},
		{`{"task":{"status":"failed","error":{"message":"content rejected"}}}`, "FAILURE", "content rejected"},
		{`{"task":{"status":"cancelled"}}`, "FAILURE", "upstream video generation failed"},
		{`{"task":{"status":"new-status"}}`, "UNKNOWN", "unrecognized video status"},
		{`{"error":{"http_code":"401","message":"denied"}}`, "FAILURE", "denied"},
		{`{"base_resp":{"status_code":1004,"status_msg":"denied"}}`, "FAILURE", "denied"},
	} {
		t.Run(test.body, func(t *testing.T) {
			var body map[string]any
			require.NoError(t, common.UnmarshalJsonStr(test.body, &body))
			result := metasoHookObject(t, plugin, "parseTaskResult", map[string]any{}, body)
			assert.Equal(t, test.status, result["status"])
			if test.reason != "" {
				assert.Equal(t, test.reason, result["reason"])
			}
			if test.status == "SUCCESS" {
				assert.Equal(t, "https://cdn.example/result.mp4", result["url"])
			}
		})
	}
	for _, code := range []int{408, 429, 500, 503} {
		_, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{"error": map[string]any{"http_code": code, "message": "retry later"}})
		require.ErrorContains(t, err, "retry later", "查询临时错误不能将已受理任务判为失败")
	}
	_, err = plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{"task": map[string]any{"status": "succeeded"}})
	require.Error(t, err, "缺失结果地址不能提前报告成功")
}

func TestMetasoH3ArtifactsAndCompletionUsage(t *testing.T) {
	plugin := loadMetasoPlugin(t)
	body := map[string]any{"task": map[string]any{
		"id": "native-task", "status": "succeeded", "resolution": "2K", "duration": 5,
		"content": map[string]any{"url": "https://cdn.example/result.mp4"},
		"usage":   map[string]any{"output_seconds": 5.2, "input_image_count": 0, "input_seconds": 7.5, "input_audio_seconds": 6},
	}}
	for _, data := range []any{body, map[string]any{"data": map[string]any{"task_id": "native-task", "data": body}}} {
		ctx := map[string]any{"status": "SUCCESS", "data": data, "artifactKey": "video", "clientRequest": map[string]any{"method": "GET"}, "apiKey": "fixture-only"}
		artifacts, err := plugin.Engine.Call(t.Context(), "listArtifacts", ctx)
		require.NoError(t, err)
		assert.JSONEq(t, `[{"key":"video","type":"video","mimeType":"video/mp4"}]`, metasoJSON(t, artifacts))
		request := metasoHookObject(t, plugin, "buildContentRequest", ctx)
		assert.Equal(t, map[string]any{"url": "https://cdn.example/result.mp4", "method": "GET", "credentialless": true}, request, "CDN 请求不携带渠道密钥")
	}
	_, err := plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "video", "data": map[string]any{}})
	require.ErrorContains(t, err, "artifact_not_found")
	usage := metasoHookObject(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, body)
	assert.Equal(t, map[string]any{"seconds": 5.2, "size": "2k", "input_images": float64(0), "input_video_seconds": 7.5}, usage, "输入视频按上游秒数结算，音频时长不计入计费事实")
	for _, test := range []struct {
		name         string
		inputSeconds any
	}{{"negative", -1.0}, {"zero", 0.0}, {"oversized", 3601.0}, {"not a number", "7.5"}} {
		t.Run(test.name, func(t *testing.T) {
			invalidBody := map[string]any{"task": map[string]any{"resolution": "480p", "usage": map[string]any{"output_seconds": 5, "input_seconds": test.inputSeconds, "input_audio_seconds": 6}}}
			facts := metasoHookObject(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, invalidBody)
			assert.Equal(t, map[string]any{"seconds": float64(5), "size": "480p"}, facts, "无效输入视频秒数不产生计费事实")
		})
	}
	invalid := map[string]any{"task": map[string]any{"resolution": "unknown", "usage": map[string]any{"output_seconds": -1, "input_image_count": 10}}}
	for _, test := range []struct {
		status string
		body   any
	}{{"SUCCESS", invalid}, {"SUCCESS", map[string]any{"task": map[string]any{}}}, {"IN_PROGRESS", body}} {
		value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{"status": test.status}, test.body)
		require.NoError(t, err)
		assert.Nil(t, value)
	}
}

func metasoJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func metasoHookObject(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.UnmarshalJsonStr(metasoJSON(t, value), &result))
	return result
}

func commandAction(t *testing.T, plugin *jsplugin.LoadedPlugin, body map[string]any) string {
	t.Helper()
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "body": map[string]any{"kind": "json", "value": body},
	})
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.UnmarshalJsonStr(metasoJSON(t, value), &result))
	return result["action"].(string)
}
