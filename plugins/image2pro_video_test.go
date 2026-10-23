package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const image2ProBaseURL = "https://api.image2pro.top/v1"

func loadImage2ProVideoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("image2pro-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "image2pro-video"})
	require.NoError(t, err)
	return plugin
}

func image2ProCallObject(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) map[string]any {
	t.Helper()
	value := image2ProCall(t, plugin, name, args...)
	var object map[string]any
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &object))
	return object
}

func image2ProCall(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), name, args...)
	require.NoError(t, err)
	return value
}

func image2ProSubmitContext(model string, body map[string]any) map[string]any {
	return map[string]any{
		"model": model, "upstreamModel": model, "baseUrl": image2ProBaseURL,
		"apiKey": "fixture-only", "publicTaskId": "abcdef0123456789",
		"requestBody": body,
	}
}

func TestImage2ProVideoPluginContract(t *testing.T) {
	plugin := loadImage2ProVideoPlugin(t)
	assert.Equal(t, []string{"Seedance2.0", "无限制-MAX-Video"}, plugin.Meta.Models)
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["count"].Description["en"])
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["seconds"].Description["en"])
	assert.Empty(t, plugin.Meta.UsageProfiles[0].Schema["seconds"].Description["en"] == "")

	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("image2pro-video")
	require.NoError(t, err)
	registered, err := registry.RegisterFactory(source, jsplugin.Options{Key: "image2pro-video"})
	require.NoError(t, err)
	for _, model := range plugin.Meta.Models {
		binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", model)
		if !found {
			binding, found = jsplugin.DefaultRegistry.Generation().LookupEndpoint("POST", "/v1/videos", model)
		}
		require.True(t, found, model)
		assert.NotNil(t, binding)
		assert.NotNil(t, registered)
	}
	_, found := registry.Generation().LookupEndpoint("POST", "/v1/responses", "Seedance2.0")
	assert.False(t, found)
}

func TestImage2ProVideoBuildRequests(t *testing.T) {
	plugin := loadImage2ProVideoPlugin(t)
	maxBody := map[string]any{
		"prompt": "a slow camera move", "duration": 8, "ratio": "16:9", "size": "720P",
		"first_frame": "https://cdn.example/start.png", "last_frame": "https://cdn.example/end.png", "seed": 7,
	}
	request := image2ProCallObject(t, plugin, "buildSubmitRequest", image2ProSubmitContext("无限制-MAX-Video", maxBody))
	assert.Equal(t, image2ProBaseURL+"/videos", request["url"])
	assert.Equal(t, true, request["noRetry"])
	headers := request["headers"].(map[string]any)
	assert.Equal(t, "Bearer fixture-only", headers["Authorization"])
	assert.Equal(t, "abcdef0123456789", headers["Idempotency-Key"])
	assert.JSONEq(t, `{"model":"无限制-MAX-Video","prompt":"a slow camera move","duration":8,"ratio":"16:9","mode":"keyframe","first_frame":"https://cdn.example/start.png","last_frame":"https://cdn.example/end.png","seed":7,"size":"720P","client_request_id":"abcdef0123456789"}`, mustJSON(t, request["body"]))

	seedanceBody := map[string]any{"prompt": "a city at night", "duration": 6, "first_frame": "https://cdn.example/start.png"}
	seedance := image2ProCallObject(t, plugin, "buildSubmitRequest", image2ProSubmitContext("Seedance2.0", seedanceBody))
	assert.JSONEq(t, `{"model":"Seedance2.0","prompt":"a city at night","duration":6,"images":["https://cdn.example/start.png"],"client_request_id":"abcdef0123456789"}`, mustJSON(t, seedance["body"]))

	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", image2ProSubmitContext("无限制-MAX-Video", map[string]any{"prompt": "x", "duration": 8, "first_frame": "https://cdn.example/a.png", "images": []any{"https://cdn.example/b.png"}}))
	require.ErrorContains(t, err, "cannot be mixed")
	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", image2ProSubmitContext("Seedance2.0", map[string]any{"prompt": "x", "last_frame": "https://cdn.example/a.png"}))
	require.ErrorContains(t, err, "last_frame")
	for _, n := range []any{true, []any{1}, 0, 2, 1.5, 1e20} {
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", image2ProSubmitContext("Seedance2.0", map[string]any{"prompt": "x", "n": n}))
		require.ErrorContains(t, err, "n only supports")
	}
}

func TestImage2ProVideoProtocolAndTaskResults(t *testing.T) {
	plugin := loadImage2ProVideoPlugin(t)
	decodedValue, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "Seedance2.0", "upstreamModel": "Seedance2.0",
		"body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "a cat", "duration": 5}},
	})
	require.NoError(t, err)
	decodedJSON, err := common.Marshal(decodedValue)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(decodedJSON, &decoded))
	assert.Equal(t, "submit", decoded["kind"])
	assert.Equal(t, "text_to_video", decoded["action"])

	for _, status := range []string{"processing", "completed", "failed"} {
		body := map[string]any{"id": "abcdef0123456789", "status": status}
		if status == "completed" {
			body["url"] = "https://cdn.example/video.mp4"
		}
		if status == "failed" {
			body["error"] = map[string]any{"message": "Bearer sk-secret-value"}
		}
		result := image2ProCallObject(t, plugin, "parseTaskResult", map[string]any{"apiKey": "sk-secret-value"}, body)
		want := map[string]string{"processing": "IN_PROGRESS", "completed": "SUCCESS", "failed": "FAILURE"}[status]
		assert.Equal(t, want, result["status"])
		if reason, ok := result["reason"].(string); ok {
			assert.NotContains(t, reason, "sk-secret-value")
		}
	}
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 202, "body": map[string]any{"id": "short", "status": "processing"}})
	require.Error(t, err)
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 202, "body": map[string]any{"id": "abcdef0123456789", "status": "completed"}})
	require.ErrorContains(t, err, "completed video URL")

	query := image2ProCallObject(t, plugin, "buildQueryRequest", map[string]any{"baseUrl": image2ProBaseURL, "taskId": "abcdef0123456789"})
	assert.Equal(t, image2ProBaseURL+"/videos/abcdef0123456789", query["url"])
	artifact := image2ProCall(t, plugin, "listArtifacts", map[string]any{"status": "SUCCESS", "data": map[string]any{"url": "https://cdn.example/video.mp4"}})
	assert.JSONEq(t, `[{"key":"video","type":"video","mimeType":"video/mp4"}]`, mustJSON(t, artifact))
	content := image2ProCallObject(t, plugin, "buildContentRequest", map[string]any{"artifactKey": "video", "data": map[string]any{"url": "https://cdn.example/video.mp4"}, "clientRequest": map[string]any{"method": "GET"}, "apiKey": "fixture-only"})
	assert.Equal(t, true, content["credentialless"])
	assert.NotContains(t, content, "headers")
	for _, url := range []string{"http://cdn.example/video.mp4", "https://", "https://user:password@cdn.example/video.mp4", "https://cdn.example/video.mp4 bad"} {
		result := image2ProCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{"status": "completed", "url": url})
		assert.Equal(t, "UNKNOWN", result["status"])
	}
}

func TestImage2ProVideoUsage(t *testing.T) {
	plugin := loadImage2ProVideoPlugin(t)
	max := image2ProCallObject(t, plugin, "extractUsage", image2ProSubmitContext("无限制-MAX-Video", map[string]any{"prompt": "x", "duration": 8}))
	assert.Equal(t, map[string]any{"seconds": float64(8)}, max)
	seedance := image2ProCallObject(t, plugin, "extractUsage", image2ProSubmitContext("Seedance2.0", map[string]any{"prompt": "x"}))
	assert.Equal(t, map[string]any{"count": float64(1)}, seedance)
	ratioContext := image2ProSubmitContext("Seedance2.0", map[string]any{"prompt": "x"})
	ratioContext["usagePurpose"] = "billing_ratios"
	assert.Empty(t, image2ProCallObject(t, plugin, "extractUsage", ratioContext))
	for _, duration := range []any{nil, true, []any{8}, "", 0, 3, 13, 8.5, 1e20} {
		_, err := plugin.Engine.Call(t.Context(), "extractUsage", image2ProSubmitContext("无限制-MAX-Video", map[string]any{"prompt": "x", "duration": duration}))
		require.Error(t, err, "duration: %v", duration)
	}
	_, err := plugin.Engine.Call(t.Context(), "extractUsage", image2ProSubmitContext("无限制-MAX-Video", map[string]any{"prompt": "x"}))
	require.Error(t, err)
	for _, duration := range []any{nil, true, false, []any{8}, map[string]any{"duration": 8}, "", " \t ", 0, -1, 3, 13, 8.5, 1e20} {
		assert.Nil(t, image2ProCall(t, plugin, "extractUsageOnComplete", map[string]any{"state": map[string]any{"model": "无限制-MAX-Video"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration": duration}))
		rendered, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, map[string]any{"task_id": "task_fixture", "status": "SUCCESS", "data": map[string]any{"duration": duration}})
		require.NoError(t, err)
		var response map[string]any
		require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, rendered), &response))
		assert.NotContains(t, response, "duration")
	}
	for _, duration := range []any{6, "6", " 6 "} {
		assert.Equal(t, map[string]any{"seconds": float64(6)}, image2ProCallObject(t, plugin, "extractUsageOnComplete", map[string]any{"state": map[string]any{"model": "无限制-MAX-Video"}}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration": duration}))
		rendered, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, map[string]any{"task_id": "task_fixture", "status": "SUCCESS", "data": map[string]any{"duration": duration}})
		require.NoError(t, err)
		var response map[string]any
		require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, rendered), &response))
		assert.Equal(t, float64(6), response["duration"])
	}
	assert.Nil(t, image2ProCall(t, plugin, "extractUsageOnComplete", map[string]any{"state": map[string]any{"model": "Seedance2.0"}}, map[string]any{"status": "FAILURE"}, map[string]any{}))
}

func TestImage2ProCanvasVideoRequests(t *testing.T) {
	plugin := loadImage2ProVideoPlugin(t)
	for _, tc := range []struct {
		name string
		body map[string]any
		want string
	}{
		{"canvas first frame", map[string]any{"prompt": "animate", "duration": 8, "seconds": "8", "resolution": "720p", "aspect_ratio": "16:9", "image": "https://cdn.example/start.png"}, `{"model":"无限制-MAX-Video","prompt":"animate","duration":8,"ratio":"16:9","size":"720P","mode":"keyframe","first_frame":"https://cdn.example/start.png","client_request_id":"abcdef0123456789"}`},
		{"metadata references", map[string]any{"prompt": "animate", "seconds": "8", "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}, "role": "reference_image"}, map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}, "role": "reference_audio"}}}}, `{"model":"无限制-MAX-Video","prompt":"animate","duration":8,"mode":"reference","images":["https://cdn.example/ref.png"],"audios":["https://cdn.example/ref.mp3"],"client_request_id":"abcdef0123456789"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "canvas-alias", "upstreamModel": "无限制-MAX-Video", "body": map[string]any{"kind": "json", "value": tc.body}})
			require.NoError(t, err)
			encoded, err := common.Marshal(decoded)
			require.NoError(t, err)
			var intent map[string]any
			require.NoError(t, common.Unmarshal(encoded, &intent))
			assert.Equal(t, "canvas-alias", intent["model"])
			assert.Equal(t, "image_to_video", intent["action"])
			ctx := image2ProSubmitContext("无限制-MAX-Video", intent["requestBody"].(map[string]any))
			ctx["model"] = "canvas-alias"
			request := image2ProCallObject(t, plugin, "buildSubmitRequest", ctx)
			assert.JSONEq(t, tc.want, mustJSON(t, request["body"]))
		})
	}
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "Seedance2.0", "body": map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{"animate"}, "seconds": []string{"6"}}, "files": []any{map[string]any{"field": "image", "ref": "fixture"}}}})
	require.ErrorContains(t, err, "public HTTPS")
}

func TestImage2ProGatewayUpstreamLifecycle(t *testing.T) {
	plugin := loadImage2ProVideoPlugin(t)
	ctx := image2ProSubmitContext("无限制-MAX-Video", map[string]any{"prompt": "animate", "seconds": "8"})
	ctx["upstream"] = map[string]any{"kind": "new_api"}
	ctx["baseUrl"] = "https://gateway.example"
	request := image2ProCallObject(t, plugin, "buildSubmitRequest", ctx)
	assert.NotContains(t, request["body"].(map[string]any), "client_request_id")
	assert.Equal(t, "https://gateway.example/v1/videos", request["url"])
	for _, status := range []string{"queued", "in_progress", "completed", "failed"} {
		body := map[string]any{"id": "task_gateway_fixture", "status": status, "duration": 8}
		parsed := image2ProCallObject(t, plugin, "parseSubmitResponse", ctx, map[string]any{"statusCode": 202, "body": body})
		assert.Equal(t, "task_gateway_fixture", parsed["taskId"])
		if status == "completed" || status == "failed" {
			assert.Contains(t, parsed, "immediate")
		}
	}
	ctx["taskId"] = "task_gateway_fixture"
	query := image2ProCallObject(t, plugin, "buildQueryRequest", ctx)
	assert.Equal(t, "https://gateway.example/v1/videos/task_gateway_fixture", query["url"])
	ctx["upstreamTaskId"] = "task_gateway_fixture"
	ctx["artifactKey"] = "video"
	ctx["clientRequest"] = map[string]any{"method": "HEAD"}
	content := image2ProCallObject(t, plugin, "buildContentRequest", ctx)
	assert.Equal(t, "https://gateway.example/v1/videos/task_gateway_fixture/content", content["url"])
	assert.Equal(t, "HEAD", content["method"])
	assert.Equal(t, "Bearer fixture-only", content["headers"].(map[string]any)["Authorization"])
	assert.NotContains(t, content, "credentialless")
	ctx["upstreamTaskId"] = "../escape"
	_, err := plugin.Engine.Call(t.Context(), "buildContentRequest", ctx)
	require.Error(t, err)
	for _, status := range []string{"queued", "in_progress", "unrecognized"} {
		assert.Equal(t, "UNKNOWN", image2ProCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{"status": status})["status"])
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
