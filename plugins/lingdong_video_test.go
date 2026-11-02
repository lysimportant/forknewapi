package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const lingdongBaseURL = "https://www.lingdongapi.com"

func loadLingdongVideoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("lingdong-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "lingdong-video"})
	require.NoError(t, err)
	return plugin
}

func lingdongCall(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), name, args...)
	require.NoError(t, err)
	return value
}

func lingdongCallObject(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) map[string]any {
	t.Helper()
	value := lingdongCall(t, plugin, name, args...)
	var object map[string]any
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &object))
	return object
}

func lingdongSubmitContext(model string, body map[string]any) map[string]any {
	return map[string]any{
		"model": model, "upstreamModel": model, "baseUrl": lingdongBaseURL,
		"apiKey": "fixture-only", "publicTaskId": "pub-task-1",
		"requestBody": body,
	}
}

func TestLingdongVideoPluginContract(t *testing.T) {
	plugin := loadLingdongVideoPlugin(t)
	assert.Empty(t, plugin.Meta.Models)
	assert.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, []string{"task-submit-no-retry@1"}, plugin.Meta.RequiredCapabilities)
	assert.Equal(t, "per_task", plugin.Meta.FetchMode)
	assert.Equal(t, lingdongBaseURL, plugin.Meta.BaseURL)
	assert.Equal(t, []string{"www.lingdongapi.com:443", "lingdongapi.com:443"}, plugin.Meta.AllowedHosts)
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)
	assert.Equal(t, "灵动视频", plugin.Meta.Name)

	assert.Equal(t, "count", plugin.Meta.UsageSchema["count"].Unit)
	assert.Equal(t, "条", plugin.Meta.UsageSchema["count"].UnitLabel["zh"])
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["count"].Description["en"])
	assert.Equal(t, "视频生成单价", plugin.Meta.UsageSchema["seconds"].Description["zh"])
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.Equal(t, "张", plugin.Meta.UsageSchema["input_images"].UnitLabel["zh"])
	assert.Equal(t, "Input image unit price", plugin.Meta.UsageSchema["input_images"].Description["en"])
	assert.Equal(t, "count", plugin.Meta.UsageSchema["input_videos"].Unit)
	assert.Equal(t, "输入音频单价", plugin.Meta.UsageSchema["input_audios"].Description["zh"])
	assert.Equal(t, "second", plugin.Meta.UsageSchema["input_video_seconds"].Unit)
	assert.Equal(t, "Input video unit price", plugin.Meta.UsageSchema["input_video_seconds"].Description["en"])
	assert.Equal(t, []string{"480p", "720p", "1080p"}, plugin.Meta.UsageSchema["video_resolution"].Enum)
	assert.Equal(t, "输出视频分辨率", plugin.Meta.UsageSchema["video_resolution"].Description["zh"])
	require.NotEmpty(t, plugin.Meta.UsageExamples)
	for _, example := range plugin.Meta.UsageExamples {
		assert.Len(t, example.Facts, len(plugin.Meta.UsageSchema))
	}

	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("lingdong-video")
	require.NoError(t, err)
	registered, err := registry.RegisterFactory(source, jsplugin.Options{Key: "lingdong-video"})
	require.NoError(t, err)
	binding, claimed := registry.Generation().LookupPluginEndpoint("POST", "/v1/videos", "lingdong-video")
	require.True(t, claimed)
	assert.Same(t, registered, binding.Plugin)
}

func TestLingdongVideoBuildSubmitRequest(t *testing.T) {
	plugin := loadLingdongVideoPlugin(t)

	generic := lingdongCallObject(t, plugin, "buildSubmitRequest", lingdongSubmitContext("cvk-2.5-720", map[string]any{
		"model": "canvas-alias", "prompt": "a cat walking", "duration": 5,
		"ratio": "16:9", "resolution": "720p", "watermark": false,
	}))
	assert.Equal(t, lingdongBaseURL+"/v1/video/generations", generic["url"])
	assert.Equal(t, "POST", generic["method"])
	assert.Equal(t, true, generic["noRetry"])
	headers := generic["headers"].(map[string]any)
	assert.Equal(t, "Bearer fixture-only", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])
	assert.Equal(t, "application/json", headers["Accept"])
	assert.Equal(t, "pub-task-1", headers["X-Idempotency-Key"])
	assert.JSONEq(t, `{"model":"cvk-2.5-720","prompt":"a cat walking","duration":5,"ratio":"16:9","resolution":"720p","watermark":false}`, mustJSON(t, generic["body"]))

	references := lingdongCallObject(t, plugin, "buildSubmitRequest", lingdongSubmitContext("cvk-2.5-720", map[string]any{
		"model": "canvas-alias", "prompt": "use Image 1 with Video 1", "seconds": 6,
		"metadata": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "use Image 1 with Video 1"},
			map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "https://cdn.example/first.png"}},
			map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
			map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
			map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}},
		}},
	}))
	assert.JSONEq(t, `{"model":"cvk-2.5-720","prompt":"use Image 1 with Video 1","images":["https://cdn.example/first.png","https://cdn.example/ref.png"],"videos":["https://cdn.example/ref.mp4"],"audios":["https://cdn.example/ref.mp3"],"duration":6}`, mustJSON(t, references["body"]))

	aliases := lingdongCallObject(t, plugin, "buildSubmitRequest", lingdongSubmitContext("minimax-h3-1", map[string]any{
		"prompt": "pan across", "duration": "8", "aspect_ratio": "9:16", "n": 1,
		"reference_images": []any{"https://cdn.example/a.png"},
		"reference_videos": []any{map[string]any{"url": "https://cdn.example/v.mp4", "duration_seconds": 4}},
		"reference_audios": []any{"https://cdn.example/a.mp3"},
	}))
	assert.JSONEq(t, `{"model":"minimax-h3-1","prompt":"pan across","images":["https://cdn.example/a.png"],"videos":["https://cdn.example/v.mp4"],"audios":["https://cdn.example/a.mp3"],"duration":8,"ratio":"9:16"}`, mustJSON(t, aliases["body"]))

	withSize := lingdongCallObject(t, plugin, "buildSubmitRequest", lingdongSubmitContext("cvk", map[string]any{
		"prompt": "x", "size": "1920x1080", "watermark": "true",
	}))
	assert.JSONEq(t, `{"model":"cvk","prompt":"x","ratio":"16:9","resolution":"1080p","watermark":true}`, mustJSON(t, withSize["body"]))

	withBase := lingdongSubmitContext("cvk", map[string]any{"prompt": "x"})
	withBase["baseUrl"] = "https://www.lingdongapi.com/v1/"
	assert.Equal(t, lingdongBaseURL+"/v1/video/generations", lingdongCallObject(t, plugin, "buildSubmitRequest", withBase)["url"])

	for _, tc := range []struct {
		name string
		body map[string]any
		want string
	}{
		{"missing prompt", map[string]any{}, "prompt is required"},
		{"unsupported field", map[string]any{"prompt": "x", "seed": 2}, "unsupported video request field"},
		{"bad seconds type", map[string]any{"prompt": "x", "seconds": true}, "seconds must be an integer"},
		{"zero seconds", map[string]any{"prompt": "x", "seconds": 0}, "between 1 and 3600"},
		{"too large duration", map[string]any{"prompt": "x", "duration": 4000}, "between 1 and 3600"},
		{"conflicting durations", map[string]any{"prompt": "x", "seconds": "5", "duration": 6}, "duration conflicts with seconds"},
		{"bad ratio", map[string]any{"prompt": "x", "ratio": "16x9"}, "ratio like 16:9"},
		{"bad size", map[string]any{"prompt": "x", "size": "1280*720"}, "pixel size"},
		{"ratio against size", map[string]any{"prompt": "x", "ratio": "9:16", "size": "1280x720"}, "ratio conflicts with size"},
		{"resolution against size", map[string]any{"prompt": "x", "resolution": "1080p", "size": "1280x720"}, "resolution conflicts with size"},
		{"n larger than one", map[string]any{"prompt": "x", "n": 2}, "n must be 1"},
		{"bad watermark", map[string]any{"prompt": "x", "watermark": "yes"}, "watermark must be a boolean"},
		{"last frame", map[string]any{"prompt": "x", "last_frame_url": "https://cdn.example/b.png"}, "last frame is not supported"},
		{"private media URL", map[string]any{"prompt": "x", "images": []any{"http://localhost/a.png"}}, "public HTTP(S) URL"},
		{"media item field", map[string]any{"prompt": "x", "images": []any{map[string]any{"url": "https://cdn.example/a.png", "role": "first_frame"}}}, "unsupported images item field"},
		{"alias conflict", map[string]any{"prompt": "x", "images": []any{"https://cdn.example/a.png"}, "reference_images": []any{"https://cdn.example/b.png"}}, "conflicts with images"},
		{"metadata field", map[string]any{"prompt": "x", "metadata": map[string]any{"style": "anime"}}, "unsupported metadata field: style"},
		{"metadata ratio conflict", map[string]any{"prompt": "x", "ratio": "16:9", "metadata": map[string]any{"ratio": "9:16"}}, "metadata.ratio conflicts with ratio"},
		{"content text mismatch", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "other"}}}}, "must match prompt"},
		{"content last frame", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "role": "last_frame", "image_url": map[string]any{"url": "https://cdn.example/b.png"}}}}}, "last frame is not supported"},
		{"content unknown role", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "role": "style_reference", "image_url": map[string]any{"url": "https://cdn.example/a.png"}}}}}, "unsupported metadata.content image role"},
		{"unsupported content type", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "file_url"}}}}, "unsupported metadata.content item type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", lingdongSubmitContext("cvk", tc.body))
			require.ErrorContains(t, err, tc.want)
		})
	}

	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"model": "cvk", "upstreamModel": "cvk", "baseUrl": "http://www.lingdongapi.com",
		"apiKey": "fixture-only", "publicTaskId": "pub-task-1", "requestBody": map[string]any{"prompt": "x"},
	})
	require.ErrorContains(t, err, "channel base URL must be an HTTPS URL")

	missingKey := lingdongSubmitContext("cvk", map[string]any{"prompt": "x"})
	missingKey["apiKey"] = ""
	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", missingKey)
	require.ErrorContains(t, err, "channel API key is required")
}

func TestLingdongVideoProtocolAndTaskResults(t *testing.T) {
	plugin := loadLingdongVideoPlugin(t)

	decode := func(body map[string]any) map[string]any {
		decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "canvas-alias", "upstreamModel": "cvk-2.5-720",
			"body": map[string]any{"kind": "json", "value": body},
		})
		require.NoError(t, err)
		var intent map[string]any
		require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, decoded), &intent))
		return intent
	}

	text := decode(map[string]any{"prompt": "a cat", "duration": 6})
	assert.Equal(t, "submit", text["kind"])
	assert.Equal(t, "canvas-alias", text["model"])
	assert.Equal(t, "text_to_video", text["action"])
	textBody := text["requestBody"].(map[string]any)
	assert.Equal(t, "a cat", textBody["prompt"])
	assert.Equal(t, float64(6), textBody["duration"])

	image := decode(map[string]any{"prompt": "animate", "image": "https://cdn.example/s.png"})
	assert.Equal(t, "image_to_video", image["action"])
	assert.Equal(t, "https://cdn.example/s.png", image["requestBody"].(map[string]any)["input_reference"])

	references := decode(map[string]any{"prompt": "animate Image 1", "metadata": map[string]any{"content": []any{
		map[string]any{"type": "text", "text": "animate Image 1"},
		map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
		map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
	}}})
	assert.Equal(t, "reference_to_video", references["action"])

	multipart, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "cvk-2.5-720",
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{
			"prompt":   []string{"animate"},
			"duration": []string{"5"},
			"images":   []string{`["https://cdn.example/a.png"]`},
		}},
	})
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, multipart), &intent))
	multipartBody := intent["requestBody"].(map[string]any)
	assert.Equal(t, float64(5), multipartBody["duration"])
	assert.Equal(t, []any{"https://cdn.example/a.png"}, multipartBody["images"])

	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "cvk-2.5-720",
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{"animate"}}, "files": []any{map[string]any{"field": "image", "ref": "fixture"}}},
	})
	require.ErrorContains(t, err, "public HTTP(S)")

	submit := lingdongCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"created_at": 1793607338, "id": "task_abc123", "model": "cvk-2.5-720", "status": "queued", "task_id": "task_abc123"},
	})
	assert.Equal(t, "task_abc123", submit["taskId"])
	assert.NotContains(t, submit, "immediate")

	completedSubmit := lingdongCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"id": "task_abc123", "status": "completed", "progress": 100},
	})
	immediate := completedSubmit["immediate"].(map[string]any)
	assert.Equal(t, "SUCCESS", immediate["status"])

	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 503, "body": map[string]any{}})
	require.ErrorContains(t, err, "Lingdong video submission failed")
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{
		"statusCode": 200, "body": map[string]any{"error": map[string]any{"message": "bad key sk-secret-value", "code": "lingdong_auth"}},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sk-secret-value")
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{
		"statusCode": 200, "body": map[string]any{"id": "../escape", "status": "queued"},
	})
	require.ErrorContains(t, err, "task id is invalid")

	for _, tc := range []struct {
		name string
		body map[string]any
		want map[string]any
	}{
		{"queued", map[string]any{"id": "task_abc123", "status": "queued"}, map[string]any{"status": "QUEUED"}},
		{"processing", map[string]any{"id": "task_abc123", "status": "processing", "progress": 45}, map[string]any{"status": "IN_PROGRESS", "progress": "45%"}},
		{"completed", map[string]any{"id": "task_abc123", "status": "completed", "progress": 100}, map[string]any{"status": "SUCCESS", "progress": "100%"}},
		{"failed with message", map[string]any{"id": "task_abc123", "status": "failed", "error": map[string]any{"code": "generation_failed", "message": "provider failed"}}, map[string]any{"status": "FAILURE", "reason": "provider failed"}},
		{"failed without message", map[string]any{"id": "task_abc123", "status": "failed"}, map[string]any{"status": "FAILURE", "reason": "Video generation failed"}},
		{"unknown status", map[string]any{"id": "task_abc123", "status": "unknown"}, map[string]any{"status": "UNKNOWN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := lingdongCallObject(t, plugin, "parseTaskResult", map[string]any{}, tc.body)
			for key, value := range tc.want {
				assert.Equal(t, value, result[key], key)
			}
		})
	}

	// 平台查询响应可能同时携带噪声文本（如 input 字段），判定只能以 status 为准。
	synthetic := lingdongCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{
		"id": "task_abc123", "status": "processing", "progress": 45, "input": "生成失败：请重新提交任务",
	})
	assert.Equal(t, "IN_PROGRESS", synthetic["status"])
	assert.Equal(t, "45%", synthetic["progress"])
	completedSynthetic := lingdongCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{
		"id": "task_abc123", "status": "completed", "progress": 100, "input": "生成失败：请重新提交任务",
	})
	assert.Equal(t, "SUCCESS", completedSynthetic["status"])

	envelope := lingdongCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{"data": map[string]any{"id": "task_abc123", "status": "in_progress", "progress": 12}})
	assert.Equal(t, "IN_PROGRESS", envelope["status"])
	redacted := lingdongCallObject(t, plugin, "parseTaskResult", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{"status": "failed", "error": map[string]any{"message": "token sk-secret-value rejected"}})
	assert.NotContains(t, redacted["reason"], "sk-secret-value")

	query := lingdongCallObject(t, plugin, "buildQueryRequest", map[string]any{"baseUrl": lingdongBaseURL, "taskId": "task_abc123", "apiKey": "fixture-only"})
	assert.Equal(t, lingdongBaseURL+"/v1/video/generations/task_abc123", query["url"])
	assert.Equal(t, "GET", query["method"])
	_, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": lingdongBaseURL, "taskId": "../escape", "apiKey": "fixture-only"})
	require.ErrorContains(t, err, "task id is invalid")

	artifact := lingdongCall(t, plugin, "listArtifacts", map[string]any{"status": "SUCCESS"})
	assert.JSONEq(t, `[{"key":"video","type":"video","mimeType":"video/mp4"}]`, mustJSON(t, artifact))
	assert.JSONEq(t, `[]`, mustJSON(t, lingdongCall(t, plugin, "listArtifacts", map[string]any{"status": "IN_PROGRESS"})))

	content := lingdongCallObject(t, plugin, "buildContentRequest", map[string]any{
		"artifactKey": "video", "upstreamTaskId": "task_abc123", "apiKey": "fixture-only",
		"baseUrl": lingdongBaseURL, "clientRequest": map[string]any{"method": "HEAD"},
	})
	assert.Equal(t, lingdongBaseURL+"/v1/videos/task_abc123/content", content["url"])
	assert.Equal(t, "HEAD", content["method"])
	assert.NotContains(t, content, "credentialless")
	assert.Equal(t, "Bearer fixture-only", content["headers"].(map[string]any)["Authorization"])
	_, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "image", "upstreamTaskId": "task_abc123", "apiKey": "fixture-only"})
	require.ErrorContains(t, err, "artifact_not_found")

	for status, want := range map[string]string{"QUEUED": "queued", "IN_PROGRESS": "in_progress", "SUCCESS": "completed", "FAILURE": "failed"} {
		rendered, renderErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, map[string]any{"task_id": "task_abc123", "status": status})
		require.NoError(t, renderErr)
		var response map[string]any
		require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, rendered), &response))
		assert.Equal(t, want, response["status"])
		assert.Equal(t, "video", response["object"])
	}
	rendered, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, map[string]any{
		"task_id": "task_abc123", "status": "FAILURE", "properties": map[string]any{"origin_model_name": "cvk-2.5-720"},
	})
	require.NoError(t, err)
	var renderedResponse map[string]any
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, rendered), &renderedResponse))
	assert.Equal(t, "cvk-2.5-720", renderedResponse["model"])
	assert.Equal(t, "video_generation_failed", renderedResponse["error"].(map[string]any)["code"])
}

func TestLingdongVideoUsage(t *testing.T) {
	plugin := loadLingdongVideoPlugin(t)

	withSeconds := lingdongCallObject(t, plugin, "extractUsage", lingdongSubmitContext("cvk-2.5-720", map[string]any{"prompt": "x", "duration": 6}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0), "input_video_seconds": float64(0)}, withSeconds)

	withoutSeconds := lingdongCallObject(t, plugin, "extractUsage", lingdongSubmitContext("cvk", map[string]any{"prompt": "x"}))
	assert.Equal(t, map[string]any{"count": float64(1), "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0), "input_video_seconds": float64(0)}, withoutSeconds)

	withResolution := lingdongCallObject(t, plugin, "extractUsage", lingdongSubmitContext("cvk-2.5-720", map[string]any{
		"prompt": "x", "duration": 6, "resolution": "720P",
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "video_resolution": "720p", "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0), "input_video_seconds": float64(0)}, withResolution)

	withSizeOnly := lingdongCallObject(t, plugin, "extractUsage", lingdongSubmitContext("cvk-2.5-1080", map[string]any{
		"prompt": "x", "size": "1920x1080",
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "video_resolution": "1080p", "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0), "input_video_seconds": float64(0)}, withSizeOnly)

	unknownResolution := lingdongCallObject(t, plugin, "extractUsage", lingdongSubmitContext("cvk", map[string]any{
		"prompt": "x", "resolution": "999p",
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0), "input_video_seconds": float64(0)}, unknownResolution)

	references := lingdongCallObject(t, plugin, "extractUsage", lingdongSubmitContext("cvk-2.5-720", map[string]any{
		"prompt": "x", "duration": 6, "image": "https://cdn.example/start.png",
		"reference_images": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"},
		"reference_videos": []any{map[string]any{"url": "https://cdn.example/v.mp4", "duration_seconds": 5}},
		"reference_audios": []any{"https://cdn.example/a.mp3"},
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "input_images": float64(2), "input_videos": float64(1), "input_audios": float64(1), "input_video_seconds": float64(5)}, references, "首帧是生成条件，不计入参考图片")

	ratioContext := lingdongSubmitContext("cvk", map[string]any{"prompt": "x"})
	ratioContext["usagePurpose"] = "billing_ratios"
	assert.Empty(t, lingdongCallObject(t, plugin, "extractUsage", ratioContext))
	assert.Empty(t, lingdongCallObject(t, plugin, "extractUsage", lingdongSubmitContext("cvk", map[string]any{"prompt": "x", "seed": 1})))
	assert.Nil(t, lingdongCall(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration": 6}))
}
