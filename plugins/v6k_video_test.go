package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const v6kBaseURL = "https://www.v6k.top"

func loadV6KVideoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("v6k-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "v6k-video"})
	require.NoError(t, err)
	return plugin
}

func v6kCall(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), name, args...)
	require.NoError(t, err)
	return value
}

func v6kCallObject(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) map[string]any {
	t.Helper()
	value := v6kCall(t, plugin, name, args...)
	var object map[string]any
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &object))
	return object
}

func v6kSubmitContext(model string, body map[string]any) map[string]any {
	return map[string]any{
		"model": model, "upstreamModel": model, "baseUrl": v6kBaseURL,
		"apiKey": "fixture-only", "publicTaskId": "pub-task-1",
		"requestBody": body,
	}
}

func TestV6KVideoPluginContract(t *testing.T) {
	plugin := loadV6KVideoPlugin(t)
	assert.Empty(t, plugin.Meta.Models)
	assert.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, []string{"task-submit-no-retry@1"}, plugin.Meta.RequiredCapabilities)
	assert.Equal(t, "per_task", plugin.Meta.FetchMode)
	assert.Equal(t, v6kBaseURL, plugin.Meta.BaseURL)
	assert.Equal(t, []string{"www.v6k.top:443"}, plugin.Meta.AllowedHosts)
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["count"].Description["en"])
	assert.Equal(t, "视频生成单价", plugin.Meta.UsageSchema["count"].Description["zh"])
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.Equal(t, "Input image unit price", plugin.Meta.UsageSchema["input_images"].Description["en"])
	assert.Equal(t, "张", plugin.Meta.UsageSchema["input_images"].UnitLabel["zh"])
	assert.Equal(t, "count", plugin.Meta.UsageSchema["input_videos"].Unit)
	assert.Equal(t, "输入音频单价", plugin.Meta.UsageSchema["input_audios"].Description["zh"])
	assert.Equal(t, []string{"480p", "720p", "1080p", "4k"}, plugin.Meta.UsageSchema["video_resolution"].Enum)
	assert.Equal(t, "输出视频分辨率", plugin.Meta.UsageSchema["video_resolution"].Description["zh"])
	require.NotEmpty(t, plugin.Meta.UsageExamples)
	for _, example := range plugin.Meta.UsageExamples {
		assert.Len(t, example.Facts, len(plugin.Meta.UsageSchema))
	}

	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("v6k-video")
	require.NoError(t, err)
	registered, err := registry.RegisterFactory(source, jsplugin.Options{Key: "v6k-video"})
	require.NoError(t, err)
	binding, claimed := registry.Generation().LookupPluginEndpoint("POST", "/v1/videos", "v6k-video")
	require.True(t, claimed)
	assert.Same(t, registered, binding.Plugin)
}

func TestV6KVideoBuildRequests(t *testing.T) {
	plugin := loadV6KVideoPlugin(t)

	generic := v6kCallObject(t, plugin, "buildSubmitRequest", v6kSubmitContext("seedance-2.0-mini-720p", map[string]any{
		"model": "canvas-alias", "prompt": "a cat walking", "duration": 5,
		"resolution": "720p", "aspect_ratio": "16:9", "size": "1280x720", "quality": "high",
		"image": "https://cdn.example/start.png",
	}))
	assert.Equal(t, v6kBaseURL+"/v1/videos", generic["url"])
	assert.Equal(t, true, generic["noRetry"])
	headers := generic["headers"].(map[string]any)
	assert.Equal(t, "Bearer fixture-only", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])
	assert.Equal(t, "pub-task-1", headers["Idempotency-Key"])
	assert.JSONEq(t, `{"model":"seedance-2.0-mini-720p","prompt":"a cat walking","seconds":5,"ratio":"16:9","resolution":"720p","size":"1280x720","input_reference":"https://cdn.example/start.png"}`, mustJSON(t, generic["body"]))

	frozen := v6kCallObject(t, plugin, "buildSubmitRequest", v6kSubmitContext("TTseadanco2.5-720YT", map[string]any{
		"model": "canvas-alias", "prompt": "use Image 1 with Video 1",
		"metadata": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "use Image 1 with Video 1"},
			map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "https://cdn.example/first.png"}},
			map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
			map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
			map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}},
		}},
	}))
	assert.JSONEq(t, `{"model":"TTseadanco2.5-720YT","prompt":"use Image 1 with Video 1","input_reference":"https://cdn.example/first.png","images":["https://cdn.example/ref.png"],"videos":["https://cdn.example/ref.mp4"],"audios":["https://cdn.example/ref.mp3"]}`, mustJSON(t, frozen["body"]))

	aliases := v6kCallObject(t, plugin, "buildSubmitRequest", v6kSubmitContext("dola2.5", map[string]any{
		"prompt": "pan across", "seconds": "8",
		"reference_image_urls": []any{"https://cdn.example/a.png"},
		"reference_video_urls": []any{"https://cdn.example/v.mp4"},
		"reference_audio_urls": []any{"https://cdn.example/a.mp3"},
		"metadata":             map[string]any{"style": "anime"},
	}))
	assert.JSONEq(t, `{"model":"dola2.5","prompt":"pan across","seconds":8,"images":["https://cdn.example/a.png"],"videos":["https://cdn.example/v.mp4"],"audios":["https://cdn.example/a.mp3"],"metadata":{"style":"anime"}}`, mustJSON(t, aliases["body"]))

	withObjects := v6kCallObject(t, plugin, "buildSubmitRequest", v6kSubmitContext("grok--1.5", map[string]any{
		"prompt": "animate", "images": []any{map[string]any{"url": "https://cdn.example/a.png", "duration_seconds": 3}},
	}))
	assert.JSONEq(t, `{"model":"grok--1.5","prompt":"animate","images":[{"url":"https://cdn.example/a.png","duration_seconds":3}]}`, mustJSON(t, withObjects["body"]))

	withBase := v6kSubmitContext("dola2.5", map[string]any{"prompt": "x"})
	withBase["baseUrl"] = "https://www.v6k.top/v1/"
	assert.Equal(t, v6kBaseURL+"/v1/videos", v6kCallObject(t, plugin, "buildSubmitRequest", withBase)["url"])

	for _, tc := range []struct {
		name  string
		model string
		body  map[string]any
		want  string
	}{
		{"missing prompt", "dola2.5", map[string]any{}, "prompt is required"},
		{"unsupported field", "dola2.5", map[string]any{"prompt": "x", "seedance": 2}, "unsupported video request field"},
		{"negative prompt", "dola2.5", map[string]any{"prompt": "x", "negative_prompt": "blur"}, "negative_prompt is not supported"},
		{"last frame", "dola2.5", map[string]any{"prompt": "x", "last_frame_url": "https://cdn.example/b.png"}, "last frame is not supported"},
		{"conflicting durations", "dola2.5", map[string]any{"prompt": "x", "seconds": "5", "duration": 6}, "duration conflicts with seconds"},
		{"bad seconds type", "dola2.5", map[string]any{"prompt": "x", "seconds": true}, "seconds must be an integer"},
		{"zero seconds", "dola2.5", map[string]any{"prompt": "x", "seconds": 0}, "between 1 and 3600"},
		{"too large seconds", "dola2.5", map[string]any{"prompt": "x", "duration": 4000}, "between 1 and 3600"},
		{"bad ratio", "dola2.5", map[string]any{"prompt": "x", "ratio": "16x9"}, "ratio like 16:9"},
		{"bad size", "dola2.5", map[string]any{"prompt": "x", "size": "1280*720"}, "pixel size"},
		{"bad quality", "dola2.5", map[string]any{"prompt": "x", "quality": 2}, "quality must be a string"},
		{"n larger than one", "dola2.5", map[string]any{"prompt": "x", "n": 2}, "n must be 1"},
		{"private media URL", "dola2.5", map[string]any{"prompt": "x", "images": []any{"http://localhost/a.png"}}, "public HTTP(S) URL"},
		{"media item field", "dola2.5", map[string]any{"prompt": "x", "images": []any{map[string]any{"url": "https://cdn.example/a.png", "role": "first_frame"}}}, "unsupported images item field"},
		{"metadata duration override", "dola2.5", map[string]any{"prompt": "x", "metadata": map[string]any{"seconds": 5}}, "metadata must not override seconds"},
		{"content text mismatch", "dola2.5", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "other"}}}}, "must match prompt"},
		{"content last frame", "dola2.5", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "role": "last_frame", "image_url": map[string]any{"url": "https://cdn.example/b.png"}}}}}, "last frame is not supported"},
		{"content unknown role", "dola2.5", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "role": "style_reference", "image_url": map[string]any{"url": "https://cdn.example/a.png"}}}}}, "unsupported metadata.content image role"},
		{"unsupported content type", "dola2.5", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "file_url"}}}}, "unsupported metadata.content item type"},
		{"alias conflict", "dola2.5", map[string]any{"prompt": "x", "images": []any{"https://cdn.example/a.png"}, "reference_image_urls": []any{"https://cdn.example/b.png"}}, "conflicts with images"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", v6kSubmitContext(tc.model, tc.body))
			require.ErrorContains(t, err, tc.want)
		})
	}

	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"model": "dola2.5", "upstreamModel": "dola2.5", "baseUrl": "http://www.v6k.top",
		"apiKey": "fixture-only", "publicTaskId": "pub-task-1", "requestBody": map[string]any{"prompt": "x"},
	})
	require.ErrorContains(t, err, "HTTPS API base")

	missingKey := v6kSubmitContext("dola2.5", map[string]any{"prompt": "x"})
	missingKey["apiKey"] = ""
	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", missingKey)
	require.ErrorContains(t, err, "channel API key is required")
}

func TestV6KVideoProtocolAndTaskResults(t *testing.T) {
	plugin := loadV6KVideoPlugin(t)

	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "dola2.5",
		"body": map[string]any{"kind": "json", "value": map[string]any{"model": "canvas-alias", "prompt": "a cat", "duration": 6}},
	})
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, decoded), &intent))
	assert.Equal(t, "submit", intent["kind"])
	assert.Equal(t, "canvas-alias", intent["model"])
	assert.Equal(t, "text_to_video", intent["action"])
	requestBody := intent["requestBody"].(map[string]any)
	assert.Equal(t, "a cat", requestBody["prompt"])
	assert.Equal(t, float64(6), requestBody["seconds"])

	imageDecoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "grok--1.5",
		"body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "animate", "image": "https://cdn.example/s.png"}},
	})
	require.NoError(t, err)
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, imageDecoded), &intent))
	assert.Equal(t, "image_to_video", intent["action"])
	assert.Equal(t, "https://cdn.example/s.png", intent["requestBody"].(map[string]any)["input_reference"])

	frozenDecoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "TTseadanco2.5-720YT",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "canvas-alias", "prompt": "animate Image 1",
			"metadata": map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "animate Image 1"},
				map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
				map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
			}},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, frozenDecoded), &intent))
	assert.Equal(t, "reference_to_video", intent["action"])

	multipartDecoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "dola2.5",
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{
			"prompt":  []string{"animate"},
			"seconds": []string{"5"},
			"images":  []string{`["https://cdn.example/a.png"]`},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, multipartDecoded), &intent))
	multipartBody := intent["requestBody"].(map[string]any)
	assert.Equal(t, float64(5), multipartBody["seconds"])
	assert.Equal(t, []any{"https://cdn.example/a.png"}, multipartBody["images"])

	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "dola2.5",
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{"animate"}}, "files": []any{map[string]any{"field": "image", "ref": "fixture"}}},
	})
	require.ErrorContains(t, err, "public HTTP(S)")

	submit := v6kCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"id": "task_abc123", "object": "video", "model": "dola2.5", "status": "queued", "progress": 0, "created_at": 1790224802},
	})
	assert.Equal(t, "task_abc123", submit["taskId"])
	assert.NotContains(t, submit, "immediate")

	failedSubmit := v6kCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"id": "task_abc123", "status": "failed", "error": map[string]any{"code": "invalid_request", "message": "rejected sk-secret-value"}},
	})
	failedImmediate := failedSubmit["immediate"].(map[string]any)
	assert.Equal(t, "FAILURE", failedImmediate["status"])
	assert.NotContains(t, failedImmediate["reason"], "sk-secret-value")

	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 500, "body": map[string]any{}})
	require.Error(t, err)
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{
		"statusCode": 400, "body": map[string]any{"error": map[string]any{"message": "bad key sk-secret-value", "type": "new_api_error", "code": "invalid_request"}},
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
		{"pending", map[string]any{"id": "task_abc123", "status": "pending"}, map[string]any{"status": "QUEUED"}},
		{"in progress number", map[string]any{"id": "task_abc123", "status": "in_progress", "progress": 42}, map[string]any{"status": "IN_PROGRESS", "progress": "42%"}},
		{"in progress string", map[string]any{"id": "task_abc123", "status": "in_progress", "progress": "45%"}, map[string]any{"status": "IN_PROGRESS", "progress": "45%"}},
		{"completed", map[string]any{"id": "task_abc123", "status": "completed", "progress": 100}, map[string]any{"status": "SUCCESS", "progress": "100%"}},
		{"failed with message", map[string]any{"id": "task_abc123", "status": "failed", "error": map[string]any{"code": "upstream_failed", "message": "provider failed"}}, map[string]any{"status": "FAILURE", "reason": "provider failed"}},
		{"failed without message", map[string]any{"id": "task_abc123", "status": "failed"}, map[string]any{"status": "FAILURE", "reason": "Video generation failed"}},
		{"unknown status", map[string]any{"id": "task_abc123", "status": "unknown"}, map[string]any{"status": "UNKNOWN"}},
		{"unrecognized status", map[string]any{"id": "task_abc123", "status": "whatever"}, map[string]any{"status": "UNKNOWN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := v6kCallObject(t, plugin, "parseTaskResult", map[string]any{}, tc.body)
			for key, value := range tc.want {
				assert.Equal(t, value, result[key], key)
			}
		})
	}
	envelope := v6kCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{"data": map[string]any{"id": "task_abc123", "status": "in_progress", "progress": 12}})
	assert.Equal(t, "IN_PROGRESS", envelope["status"])
	redacted := v6kCallObject(t, plugin, "parseTaskResult", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{"status": "failed", "error": map[string]any{"message": "token sk-secret-value rejected"}})
	assert.NotContains(t, redacted["reason"], "sk-secret-value")

	query := v6kCallObject(t, plugin, "buildQueryRequest", map[string]any{"baseUrl": v6kBaseURL, "taskId": "task_abc123", "apiKey": "fixture-only"})
	assert.Equal(t, v6kBaseURL+"/v1/videos/task_abc123", query["url"])
	assert.Equal(t, "GET", query["method"])
	_, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": v6kBaseURL, "taskId": "../escape", "apiKey": "fixture-only"})
	require.ErrorContains(t, err, "task id is invalid")

	artifact := v6kCall(t, plugin, "listArtifacts", map[string]any{"status": "SUCCESS", "data": map[string]any{"id": "task_abc123"}})
	assert.JSONEq(t, `[{"key":"video","type":"video","mimeType":"video/mp4"}]`, mustJSON(t, artifact))
	assert.JSONEq(t, `[]`, mustJSON(t, v6kCall(t, plugin, "listArtifacts", map[string]any{"status": "IN_PROGRESS"})))

	content := v6kCallObject(t, plugin, "buildContentRequest", map[string]any{
		"artifactKey": "video", "upstreamTaskId": "task_abc123", "apiKey": "fixture-only",
		"baseUrl": v6kBaseURL, "clientRequest": map[string]any{"method": "HEAD"},
	})
	assert.Equal(t, v6kBaseURL+"/v1/videos/task_abc123/content", content["url"])
	assert.Equal(t, "HEAD", content["method"])
	assert.NotContains(t, content, "credentialless")
	assert.Equal(t, "Bearer fixture-only", content["headers"].(map[string]any)["Authorization"])
	_, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "image", "upstreamTaskId": "task_abc123", "apiKey": "fixture-only"})
	require.ErrorContains(t, err, "artifact_not_found")
	_, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "video", "upstreamTaskId": "../escape", "apiKey": "fixture-only"})
	require.Error(t, err)

	for status, want := range map[string]string{"QUEUED": "queued", "IN_PROGRESS": "in_progress", "SUCCESS": "completed", "FAILURE": "failed"} {
		rendered, renderErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, map[string]any{"task_id": "task_abc123", "status": status})
		require.NoError(t, renderErr)
		var response map[string]any
		require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, rendered), &response))
		assert.Equal(t, want, response["status"])
		assert.Equal(t, "video", response["object"])
	}
}

func TestV6KVideoUsage(t *testing.T) {
	plugin := loadV6KVideoPlugin(t)
	withSeconds := v6kCallObject(t, plugin, "extractUsage", v6kSubmitContext("dola2.5", map[string]any{"prompt": "x", "duration": 6}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0)}, withSeconds)
	withoutSeconds := v6kCallObject(t, plugin, "extractUsage", v6kSubmitContext("dola2.5", map[string]any{"prompt": "x"}))
	assert.Equal(t, map[string]any{"count": float64(1), "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0)}, withoutSeconds)
	withResolution := v6kCallObject(t, plugin, "extractUsage", v6kSubmitContext("dola2.5", map[string]any{
		"prompt": "x", "duration": 6, "resolution": "720P",
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "video_resolution": "720p", "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0)}, withResolution)
	withSizeOnly := v6kCallObject(t, plugin, "extractUsage", v6kSubmitContext("dola2.5", map[string]any{
		"prompt": "x", "size": "1920x1080",
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "video_resolution": "1080p", "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0)}, withSizeOnly)
	unknownResolution := v6kCallObject(t, plugin, "extractUsage", v6kSubmitContext("dola2.5", map[string]any{
		"prompt": "x", "resolution": "999p",
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "input_images": float64(0), "input_videos": float64(0), "input_audios": float64(0)}, unknownResolution)
	references := v6kCallObject(t, plugin, "extractUsage", v6kSubmitContext("dola2.5", map[string]any{
		"prompt": "x", "duration": 6, "image": "https://cdn.example/start.png",
		"reference_image_urls": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"},
		"reference_video_urls": []any{"https://cdn.example/v.mp4"},
		"reference_audio_urls": []any{"https://cdn.example/a.mp3"},
	}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "input_images": float64(2), "input_videos": float64(1), "input_audios": float64(1)}, references, "首帧是生成条件，不计入参考图片")
	ratioContext := v6kSubmitContext("dola2.5", map[string]any{"prompt": "x"})
	ratioContext["usagePurpose"] = "billing_ratios"
	assert.Empty(t, v6kCallObject(t, plugin, "extractUsage", ratioContext))
	assert.Empty(t, v6kCallObject(t, plugin, "extractUsage", v6kSubmitContext("dola2.5", map[string]any{"prompt": "x", "negative_prompt": "blur"})))
	assert.Nil(t, v6kCall(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, map[string]any{}))
}

func TestV6KVideoCanvasRequests(t *testing.T) {
	plugin := loadV6KVideoPlugin(t)
	for _, tc := range []struct {
		name   string
		model  string
		body   map[string]any
		want   string
		action string
	}{
		{
			name: "canvas generic first frame", model: "grok--1.5", action: "image_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate", "duration": 5, "seconds": "5", "resolution": "720p", "aspect_ratio": "16:9", "image": "https://cdn.example/start.png"},
			want: `{"model":"grok--1.5","prompt":"animate","seconds":5,"ratio":"16:9","resolution":"720p","input_reference":"https://cdn.example/start.png"}`,
		},
		{
			name: "canvas frozen references", model: "TTseadanco2.5-720YT", action: "image_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate Image 1", "metadata": map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "animate Image 1"},
				map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
			}}},
			want: `{"model":"TTseadanco2.5-720YT","prompt":"animate Image 1","images":["https://cdn.example/ref.png"]}`,
		},
		{
			name: "canvas frozen first frame", model: "dola2.0", action: "image_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate", "metadata": map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "animate"},
				map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "https://cdn.example/first.png"}},
			}}},
			want: `{"model":"dola2.0","prompt":"animate","input_reference":"https://cdn.example/first.png"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": "canvas-alias", "upstreamModel": tc.model, "body": map[string]any{"kind": "json", "value": tc.body},
			})
			require.NoError(t, err)
			var intent map[string]any
			require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, decoded), &intent))
			assert.Equal(t, "canvas-alias", intent["model"])
			assert.Equal(t, tc.action, intent["action"])
			ctx := v6kSubmitContext(tc.model, intent["requestBody"].(map[string]any))
			ctx["model"] = "canvas-alias"
			request := v6kCallObject(t, plugin, "buildSubmitRequest", ctx)
			assert.JSONEq(t, tc.want, mustJSON(t, request["body"]))
		})
	}
}
