package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const megaViewsBaseURL = "https://megaviews.top"

func loadMegaViewsVideoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("megaviews-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "megaviews-video"})
	require.NoError(t, err)
	return plugin
}

func megaViewsCall(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), name, args...)
	require.NoError(t, err)
	return value
}

func megaViewsCallObject(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) map[string]any {
	t.Helper()
	value := megaViewsCall(t, plugin, name, args...)
	var object map[string]any
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &object))
	return object
}

func megaViewsSubmitContext(model string, body map[string]any) map[string]any {
	return map[string]any{
		"model": model, "upstreamModel": model, "baseUrl": megaViewsBaseURL,
		"apiKey": "fixture-only", "publicTaskId": "pub-task-1",
		"requestBody": body,
	}
}

func TestMegaViewsVideoPluginContract(t *testing.T) {
	plugin := loadMegaViewsVideoPlugin(t)
	assert.Empty(t, plugin.Meta.Models)
	assert.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, []string{"task-submit-no-retry@1"}, plugin.Meta.RequiredCapabilities)
	assert.Equal(t, "per_task", plugin.Meta.FetchMode)
	assert.Equal(t, megaViewsBaseURL, plugin.Meta.BaseURL)
	assert.Equal(t, "https://megaviews.top/docs", plugin.Meta.Website)
	assert.Equal(t, []string{"megaviews.top:443", "www.megaviews.top:443", "newapi.megaviews.top:443", "newapi.megabyai.cc:443"}, plugin.Meta.AllowedHosts)
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["count"].Description["en"])
	assert.Equal(t, "视频生成单价", plugin.Meta.UsageSchema["seconds"].Description["zh"])
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.Equal(t, "条", plugin.Meta.UsageSchema["count"].UnitLabel["zh"])
	assert.Equal(t, []string{"480p", "720p", "768p", "1080p", "4k"}, plugin.Meta.UsageSchema["video_resolution"].Enum)
	assert.Equal(t, "输出视频分辨率", plugin.Meta.UsageSchema["video_resolution"].Description["zh"])
	require.NotEmpty(t, plugin.Meta.UsageExamples)
	for _, example := range plugin.Meta.UsageExamples {
		assert.Len(t, example.Facts, len(plugin.Meta.UsageSchema))
	}

	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("megaviews-video")
	require.NoError(t, err)
	registered, err := registry.RegisterFactory(source, jsplugin.Options{Key: "megaviews-video"})
	require.NoError(t, err)
	binding, claimed := registry.Generation().LookupPluginEndpoint("POST", "/v1/videos", "megaviews-video")
	require.True(t, claimed)
	assert.Same(t, registered, binding.Plugin)
}

func TestMegaViewsVideoBuildRequests(t *testing.T) {
	plugin := loadMegaViewsVideoPlugin(t)

	openAI := megaViewsCallObject(t, plugin, "buildSubmitRequest", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{
		"model": "canvas-alias", "prompt": "a cat walking", "seconds": "5", "size": "1280x720", "seed": 7,
	}))
	assert.Equal(t, megaViewsBaseURL+"/v1/videos", openAI["url"])
	assert.Equal(t, "POST", openAI["method"])
	assert.Equal(t, true, openAI["noRetry"])
	headers := openAI["headers"].(map[string]any)
	assert.Equal(t, "Bearer fixture-only", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])
	assert.JSONEq(t, `{"model":"seedance-2.5-m3","prompt":"a cat walking","duration":5,"ratio":"16:9","resolution":"720p","seed":7}`, mustJSON(t, openAI["body"]))

	canvas := megaViewsCallObject(t, plugin, "buildSubmitRequest", megaViewsSubmitContext("wan3.0-video", map[string]any{
		"model": "canvas-alias", "prompt": "animate Image 1", "resolution": "720p", "aspect_ratio": "16:9", "size": "1280x720",
		"quality": "high", "negative_prompt": "blur", "generate_audio": true,
		"metadata": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "animate Image 1"},
			map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "https://cdn.example/first.png"}},
			map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
			map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
			map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}},
		}},
	}))
	assert.JSONEq(t, `{"model":"wan3.0-video","prompt":"animate Image 1","ratio":"16:9","resolution":"720p","referenceImages":["https://cdn.example/first.png","https://cdn.example/ref.png"],"referenceVideos":["https://cdn.example/ref.mp4"],"referenceAudios":["https://cdn.example/ref.mp3"],"metadata":{"negative_prompt":"blur","quality":"high","generate_audio":true}}`, mustJSON(t, canvas["body"]))

	withBase := megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x"})
	withBase["baseUrl"] = "https://newapi.megaviews.top/v1/"
	assert.Equal(t, "https://newapi.megaviews.top/v1/videos", megaViewsCallObject(t, plugin, "buildSubmitRequest", withBase)["url"])

	withSeedZero := megaViewsCallObject(t, plugin, "buildSubmitRequest", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x", "seed": 0}))
	assert.Equal(t, float64(0), withSeedZero["body"].(map[string]any)["seed"].(float64))

	for _, tc := range []struct {
		name string
		body map[string]any
		want string
	}{
		{"missing prompt", map[string]any{}, "prompt is required"},
		{"unsupported field", map[string]any{"prompt": "x", "fps": 24}, "unsupported video request field: fps"},
		{"last frame", map[string]any{"prompt": "x", "last_frame": "https://cdn.example/b.png"}, "last frame is not supported"},
		{"conflicting durations", map[string]any{"prompt": "x", "seconds": "5", "duration": 6}, "seconds conflicts with duration"},
		{"bad seconds type", map[string]any{"prompt": "x", "seconds": true}, "seconds must be an integer"},
		{"zero seconds", map[string]any{"prompt": "x", "seconds": 0}, "between 1 and 3600"},
		{"too large seconds", map[string]any{"prompt": "x", "duration": 4000}, "between 1 and 3600"},
		{"bad ratio", map[string]any{"prompt": "x", "ratio": "16x9"}, "ratio like 16:9"},
		{"bad size", map[string]any{"prompt": "x", "size": "1280*720"}, "pixel size"},
		{"ratio conflicts with size", map[string]any{"prompt": "x", "ratio": "9:16", "size": "1280x720"}, "ratio conflicts with size"},
		{"resolution conflicts with size", map[string]any{"prompt": "x", "resolution": "1080p", "size": "1280x720"}, "resolution conflicts with size"},
		{"bad seed", map[string]any{"prompt": "x", "seed": 1.5}, "between 0 and 2147483647"},
		{"seed too large", map[string]any{"prompt": "x", "seed": 2147483648}, "between 0 and 2147483647"},
		{"n larger than one", map[string]any{"prompt": "x", "n": 2}, "n must be 1"},
		{"n boolean", map[string]any{"prompt": "x", "n": true}, "n must be 1"},
		{"private media URL", map[string]any{"prompt": "x", "images": []any{"http://localhost/a.png"}}, "public HTTP(S) media URL"},
		{"media item field", map[string]any{"prompt": "x", "images": []any{map[string]any{"url": "https://cdn.example/a.png", "role": "first_frame"}}}, "unsupported images item field"},
		{"metadata duration override", map[string]any{"prompt": "x", "metadata": map[string]any{"seconds": 5}}, "metadata must not override seconds"},
		{"content text mismatch", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "other"}}}}, "must match prompt"},
		{"content last frame", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "role": "last_frame", "image_url": map[string]any{"url": "https://cdn.example/b.png"}}}}}, "last frame is not supported"},
		{"content unknown role", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "role": "style_reference", "image_url": map[string]any{"url": "https://cdn.example/a.png"}}}}}, "unsupported metadata.content image role"},
		{"unsupported content type", map[string]any{"prompt": "x", "metadata": map[string]any{"content": []any{map[string]any{"type": "file_url"}}}}, "unsupported metadata.content item type"},
		{"alias conflict", map[string]any{"prompt": "x", "images": []any{"https://cdn.example/a.png"}, "referenceImages": []any{"https://cdn.example/b.png"}}, "conflicts with images"},
		{"bad generate_audio", map[string]any{"prompt": "x", "generate_audio": "yes"}, "generate_audio must be a boolean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", megaViewsSubmitContext("seedance-2.5-m3", tc.body))
			require.ErrorContains(t, err, tc.want)
		})
	}

	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"model": "seedance-2.5-m3", "upstreamModel": "seedance-2.5-m3", "baseUrl": "http://megaviews.top",
		"apiKey": "fixture-only", "publicTaskId": "pub-task-1", "requestBody": map[string]any{"prompt": "x"},
	})
	require.ErrorContains(t, err, "HTTPS API base")

	missingKey := megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x"})
	missingKey["apiKey"] = ""
	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", missingKey)
	require.ErrorContains(t, err, "channel API key is required")
}

func TestMegaViewsVideoProtocolAndTaskResults(t *testing.T) {
	plugin := loadMegaViewsVideoPlugin(t)

	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "seedance-2.5-m3",
		"body": map[string]any{"kind": "json", "value": map[string]any{"model": "canvas-alias", "prompt": "a cat", "duration": 6, "resolution": "1080P"}},
	})
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, decoded), &intent))
	assert.Equal(t, "submit", intent["kind"])
	assert.Equal(t, "canvas-alias", intent["model"])
	assert.Equal(t, "text_to_video", intent["action"])
	requestBody := intent["requestBody"].(map[string]any)
	assert.Equal(t, "a cat", requestBody["prompt"])
	assert.Equal(t, float64(6), requestBody["duration"])
	assert.Equal(t, "1080p", requestBody["resolution"])

	imageDecoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "minimax-h3-f",
		"body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "animate", "input_reference": "https://cdn.example/s.png"}},
	})
	require.NoError(t, err)
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, imageDecoded), &intent))
	assert.Equal(t, "image_to_video", intent["action"])
	assert.Equal(t, []any{"https://cdn.example/s.png"}, intent["requestBody"].(map[string]any)["referenceImages"])

	multiRef, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "wan3.0-video",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"prompt": "animate", "images": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, multiRef), &intent))
	assert.Equal(t, "reference_to_video", intent["action"])

	multipartDecoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "seedance-2.5-m3",
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{
			"prompt":          []string{"animate"},
			"duration":        []string{"5"},
			"referenceImages": []string{`["https://cdn.example/a.png"]`},
			"metadata":        []string{`{"negative_prompt":"blur"}`},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, multipartDecoded), &intent))
	multipartBody := intent["requestBody"].(map[string]any)
	assert.Equal(t, float64(5), multipartBody["duration"])
	assert.Equal(t, []any{"https://cdn.example/a.png"}, multipartBody["referenceImages"])
	assert.Equal(t, map[string]any{"negative_prompt": "blur"}, multipartBody["metadata"])

	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "seedance-2.5-m3",
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{"animate"}}, "files": []any{map[string]any{"field": "input_reference", "ref": "fixture"}}},
	})
	require.ErrorContains(t, err, "public HTTP(S)")

	submit := megaViewsCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"id": "task_abc123", "task_id": "task_abc123", "status": "queued"},
	})
	assert.Equal(t, "task_abc123", submit["taskId"])
	assert.NotContains(t, submit, "immediate")

	wrapped := megaViewsCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"code": 0, "data": map[string]any{"id": "task_wrap1", "status": "in_progress", "progress": 10}},
	})
	assert.Equal(t, "task_wrap1", wrapped["taskId"])
	assert.NotContains(t, wrapped, "immediate")

	immediate := megaViewsCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"id": "task_done", "status": "completed"},
	})
	assert.Equal(t, "SUCCESS", immediate["immediate"].(map[string]any)["status"])

	failedSubmit := megaViewsCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"id": "task_bad", "status": "failed", "error": map[string]any{"code": "invalid_request", "message": "rejected sk-secret-value"}},
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
		{"in progress string", map[string]any{"id": "task_abc123", "status": "processing", "progress": "45%"}, map[string]any{"status": "IN_PROGRESS", "progress": "45%"}},
		{"completed", map[string]any{"id": "task_abc123", "status": "completed", "progress": 100}, map[string]any{"status": "SUCCESS", "progress": "100%"}},
		{"succeeded", map[string]any{"id": "task_abc123", "status": "succeeded"}, map[string]any{"status": "SUCCESS", "progress": "100%"}},
		{"failed with message", map[string]any{"id": "task_abc123", "status": "failed", "error": map[string]any{"code": "upstream_failed", "message": "provider failed"}}, map[string]any{"status": "FAILURE", "reason": "provider failed"}},
		{"failed without message", map[string]any{"id": "task_abc123", "status": "failed"}, map[string]any{"status": "FAILURE", "reason": "Video generation failed"}},
		{"unknown status", map[string]any{"id": "task_abc123", "status": "unknown"}, map[string]any{"status": "UNKNOWN"}},
		{"unrecognized status", map[string]any{"id": "task_abc123", "status": "whatever"}, map[string]any{"status": "UNKNOWN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := megaViewsCallObject(t, plugin, "parseTaskResult", map[string]any{}, tc.body)
			for key, value := range tc.want {
				assert.Equal(t, value, result[key], key)
			}
		})
	}
	envelope := megaViewsCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{"data": map[string]any{"id": "task_abc123", "status": "in_progress", "progress": 12}})
	assert.Equal(t, "IN_PROGRESS", envelope["status"])
	redacted := megaViewsCallObject(t, plugin, "parseTaskResult", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{"status": "failed", "error": map[string]any{"message": "token sk-secret-value rejected"}})
	assert.NotContains(t, redacted["reason"], "sk-secret-value")

	query := megaViewsCallObject(t, plugin, "buildQueryRequest", map[string]any{"baseUrl": megaViewsBaseURL, "taskId": "task_abc123", "apiKey": "fixture-only"})
	assert.Equal(t, megaViewsBaseURL+"/v1/videos/task_abc123", query["url"])
	assert.Equal(t, "GET", query["method"])
	_, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": megaViewsBaseURL, "taskId": "../escape", "apiKey": "fixture-only"})
	require.ErrorContains(t, err, "task id is invalid")

	artifact := megaViewsCall(t, plugin, "listArtifacts", map[string]any{"status": "SUCCESS", "data": map[string]any{"id": "task_abc123"}})
	assert.JSONEq(t, `[{"key":"video","type":"video","mimeType":"video/mp4"}]`, mustJSON(t, artifact))
	assert.JSONEq(t, `[]`, mustJSON(t, megaViewsCall(t, plugin, "listArtifacts", map[string]any{"status": "IN_PROGRESS"})))

	content := megaViewsCallObject(t, plugin, "buildContentRequest", map[string]any{
		"artifactKey": "video", "upstreamTaskId": "task_abc123", "apiKey": "fixture-only",
		"baseUrl": megaViewsBaseURL, "clientRequest": map[string]any{"method": "HEAD"},
	})
	assert.Equal(t, megaViewsBaseURL+"/v1/videos/task_abc123/content", content["url"])
	assert.Equal(t, "HEAD", content["method"])
	assert.Equal(t, "Bearer fixture-only", content["headers"].(map[string]any)["Authorization"])
	fromData := megaViewsCallObject(t, plugin, "buildContentRequest", map[string]any{
		"artifactKey": "video", "apiKey": "fixture-only", "baseUrl": megaViewsBaseURL,
		"data": map[string]any{"task_id": "task_from_data"},
	})
	assert.Equal(t, megaViewsBaseURL+"/v1/videos/task_from_data/content", fromData["url"])
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
}

func TestMegaViewsVideoUsage(t *testing.T) {
	plugin := loadMegaViewsVideoPlugin(t)

	withSeconds := megaViewsCallObject(t, plugin, "extractUsage", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x", "duration": 6}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6)}, withSeconds)
	withAlias := megaViewsCallObject(t, plugin, "extractUsage", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x", "seconds": "5"}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(5)}, withAlias)
	withResolution := megaViewsCallObject(t, plugin, "extractUsage", megaViewsSubmitContext("minimax-h3-f", map[string]any{"prompt": "x", "duration": 6, "resolution": "768P"}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "video_resolution": "768p"}, withResolution)
	withSizeOnly := megaViewsCallObject(t, plugin, "extractUsage", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x", "size": "1920x1080"}))
	assert.Equal(t, map[string]any{"count": float64(1), "video_resolution": "1080p"}, withSizeOnly)
	unknownResolution := megaViewsCallObject(t, plugin, "extractUsage", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x", "resolution": "999p"}))
	assert.Equal(t, map[string]any{"count": float64(1)}, unknownResolution)
	assert.Equal(t, map[string]any{"count": float64(1)}, megaViewsCallObject(t, plugin, "extractUsage", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x"})))
	ratioContext := megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x", "duration": 6})
	ratioContext["usagePurpose"] = "billing_ratios"
	assert.Empty(t, megaViewsCallObject(t, plugin, "extractUsage", ratioContext))
	assert.Empty(t, megaViewsCallObject(t, plugin, "extractUsage", megaViewsSubmitContext("seedance-2.5-m3", map[string]any{"prompt": "x", "generate_audio": "yes"})))
	assert.Nil(t, megaViewsCall(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "FAILURE"}, map[string]any{"billing_seconds": 4}))
	assert.Nil(t, megaViewsCall(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, map[string]any{}))
	assert.Nil(t, megaViewsCall(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, map[string]any{"billing_seconds": 0}))
	assert.Nil(t, megaViewsCall(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, map[string]any{"billing_seconds": 4.5}))
	for _, tc := range []struct {
		name string
		data map[string]any
		want map[string]any
	}{
		{"billing seconds include references", map[string]any{"status": "completed", "billing_seconds": 9, "generated_seconds": 6, "reference_video_seconds": 3}, map[string]any{"count": float64(1), "seconds": float64(9)}},
		{"generated seconds fallback", map[string]any{"status": "completed", "generated_seconds": 6}, map[string]any{"count": float64(1), "seconds": float64(6)}},
		{"numeric string seconds", map[string]any{"status": "completed", "billing_seconds": "8"}, map[string]any{"count": float64(1), "seconds": float64(8)}},
		{"wrapped data", map[string]any{"data": map[string]any{"status": "completed", "billing_seconds": 5}}, map[string]any{"count": float64(1), "seconds": float64(5)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := megaViewsCallObject(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, tc.data)
			assert.Equal(t, tc.want, facts)
		})
	}
}

func TestMegaViewsVideoCanvasRequests(t *testing.T) {
	plugin := loadMegaViewsVideoPlugin(t)
	for _, tc := range []struct {
		name   string
		model  string
		body   map[string]any
		action string
		want   string
	}{
		{
			name: "canvas first frame", model: "seedance-2.5-m3", action: "image_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate", "duration": 5, "seconds": "5", "resolution": "720p", "aspect_ratio": "16:9", "size": "1280x720", "image": "https://cdn.example/start.png"},
			want: `{"model":"seedance-2.5-m3","prompt":"animate","duration":5,"ratio":"16:9","resolution":"720p","referenceImages":["https://cdn.example/start.png"]}`,
		},
		{
			name: "canvas references", model: "minimax-h3-f", action: "reference_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate Image 1", "metadata": map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "animate Image 1"},
				map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
				map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
			}}},
			want: `{"model":"minimax-h3-f","prompt":"animate Image 1","referenceImages":["https://cdn.example/ref.png"],"referenceVideos":["https://cdn.example/ref.mp4"]}`,
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
			ctx := megaViewsSubmitContext(tc.model, intent["requestBody"].(map[string]any))
			ctx["model"] = "canvas-alias"
			request := megaViewsCallObject(t, plugin, "buildSubmitRequest", ctx)
			assert.JSONEq(t, tc.want, mustJSON(t, request["body"]))
		})
	}
}
