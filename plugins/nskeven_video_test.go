package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const nskevenBaseURL = "https://newapi.nskeven.uk"

func loadNSKevenVideoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("nskeven-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "nskeven-video"})
	require.NoError(t, err)
	return plugin
}

func nskevenCall(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), name, args...)
	require.NoError(t, err)
	return value
}

func nskevenCallObject(t *testing.T, plugin *jsplugin.LoadedPlugin, name string, args ...any) map[string]any {
	t.Helper()
	value := nskevenCall(t, plugin, name, args...)
	var object map[string]any
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &object))
	return object
}

func nskevenSubmitContext(model string, body map[string]any) map[string]any {
	return map[string]any{
		"model": model, "upstreamModel": model, "baseUrl": nskevenBaseURL,
		"apiKey": "fixture-only", "publicTaskId": "abc123def456",
		"requestBody": body,
	}
}

func TestNSKevenVideoPluginContract(t *testing.T) {
	plugin := loadNSKevenVideoPlugin(t)
	assert.Empty(t, plugin.Meta.Models)
	assert.Equal(t, "channel", plugin.Meta.ModelRouting)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, []string{"task-submit-no-retry@1"}, plugin.Meta.RequiredCapabilities)
	assert.Equal(t, "per_task", plugin.Meta.FetchMode)
	assert.Equal(t, nskevenBaseURL, plugin.Meta.BaseURL)
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["count"].Description["en"])
	assert.Equal(t, "视频生成单价", plugin.Meta.UsageSchema["count"].Description["zh"])
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["seconds"].Description["en"])
	assert.Equal(t, []string{"480p", "720p", "1080p", "4k"}, plugin.Meta.UsageSchema["video_resolution"].Enum)
	require.NotEmpty(t, plugin.Meta.UsageExamples)
	for _, example := range plugin.Meta.UsageExamples {
		assert.Len(t, example.Facts, len(plugin.Meta.UsageSchema))
	}

	registry := jsplugin.NewRegistry()
	source, err := plugins.Source("nskeven-video")
	require.NoError(t, err)
	registered, err := registry.RegisterFactory(source, jsplugin.Options{Key: "nskeven-video"})
	require.NoError(t, err)
	binding, claimed := registry.Generation().LookupPluginEndpoint("POST", "/v1/videos", "nskeven-video")
	require.True(t, claimed)
	assert.Same(t, registered, binding.Plugin)
}

func TestNSKevenVideoBuildRequests(t *testing.T) {
	plugin := loadNSKevenVideoPlugin(t)

	doubaoBody := map[string]any{
		"prompt": "a cat walking", "seconds": "8", "resolution": "1080p", "aspect_ratio": "16:9",
		"images": []any{"https://cdn.example/ref.png"}, "generate_audio": true,
	}
	doubao := nskevenCallObject(t, plugin, "buildSubmitRequest", nskevenSubmitContext("doubao-seedance-2-5-260628", doubaoBody))
	assert.Equal(t, nskevenBaseURL+"/v1/video/generations", doubao["url"])
	assert.Equal(t, true, doubao["noRetry"])
	headers := doubao["headers"].(map[string]any)
	assert.Equal(t, "Bearer fixture-only", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])
	assert.NotContains(t, headers, "Idempotency-Key")
	assert.JSONEq(t, `{"model":"doubao-seedance-2-5-260628","prompt":"a cat walking","seconds":"8","metadata":{"resolution":"1080p","ratio":"16:9","generate_audio":true,"content":[{"type":"image_url","image_url":{"url":"https://cdn.example/ref.png"},"role":"reference_image"}]}}`, mustJSON(t, doubao["body"]))

	expressBody := map[string]any{
		"prompt": "pan across", "duration": 6, "aspect_ratio": "9:16",
		"images": []any{"https://cdn.example/ref.png"},
		"videos": []any{"https://cdn.example/ref.mp4"},
		"audios": []any{"https://cdn.example/ref.mp3"},
	}
	express := nskevenCallObject(t, plugin, "buildSubmitRequest", nskevenSubmitContext("sd-2-E5", expressBody))
	assert.JSONEq(t, `{"model":"sd-2-E5","prompt":"pan across","seconds":"6","resolution":"720p","aspect_ratio":"9:16","images":["https://cdn.example/ref.png"],"metadata":{"videos":["https://cdn.example/ref.mp4"],"audios":["https://cdn.example/ref.mp3"]}}`, mustJSON(t, express["body"]))

	flatBody := map[string]any{
		"prompt": "zoom in", "duration": 5, "images": []any{"https://cdn.example/ref.png"},
		"first_frame_url": "https://cdn.example/start.png",
	}
	flat := nskevenCallObject(t, plugin, "buildSubmitRequest", nskevenSubmitContext("even--seedance-2.0-720p", flatBody))
	assert.JSONEq(t, `{"model":"even--seedance-2.0-720p","prompt":"zoom in","seconds":"5","resolution":"720p","first_frame_url":"https://cdn.example/start.png","images":["https://cdn.example/ref.png"]}`, mustJSON(t, flat["body"]))

	aSeries := nskevenCallObject(t, plugin, "buildSubmitRequest", nskevenSubmitContext("even-sd2.5-A", map[string]any{
		"prompt": "use Image 1 and Video 1", "images": []any{"https://cdn.example/ref.png"},
		"videos": []any{"https://cdn.example/ref.mp4"},
	}))
	assert.Equal(t, "use @图1 and @视频1", aSeries["body"].(map[string]any)["prompt"])
	assert.Equal(t, "480p", aSeries["body"].(map[string]any)["resolution"])

	withBase := nskevenSubmitContext("sd-2-E5", map[string]any{"prompt": "x"})
	withBase["baseUrl"] = "https://newapi.nskeven.uk/v1/"
	assert.Equal(t, nskevenBaseURL+"/v1/video/generations", nskevenCallObject(t, plugin, "buildSubmitRequest", withBase)["url"])

	for _, tc := range []struct {
		name  string
		model string
		body  map[string]any
		want  string
	}{
		{"missing prompt", "sd-2-E5", map[string]any{}, "prompt is required"},
		{"negative prompt", "sd-2-E5", map[string]any{"prompt": "x", "negative_prompt": "blur"}, "negative_prompt is not supported"},
		{"bad seconds", "sd-2-E5", map[string]any{"prompt": "x", "seconds": true}, "seconds must be"},
		{"too large seconds", "sd-2-E5", map[string]any{"prompt": "x", "duration": 16}, "between 4 and 15"},
		{"doubao frame limit", "doubao-seedance-2-0-260128", map[string]any{"prompt": "x", "first_frame_url": "https://cdn.example/a.png", "last_frame_url": "https://cdn.example/b.png", "images": []any{"https://cdn.example/c.png"}}, "at most 2 images"},
		{"doubao frame with reference video", "doubao-seedance-2-0-260128", map[string]any{"prompt": "x", "first_frame_url": "https://cdn.example/a.png", "videos": []any{"https://cdn.example/v.mp4"}}, "cannot combine reference videos"},
		{"express image limit", "sd-2-E5", map[string]any{"prompt": "x", "images": []any{"https://cdn.example/1.png", "https://cdn.example/2.png", "https://cdn.example/3.png", "https://cdn.example/4.png", "https://cdn.example/5.png", "https://cdn.example/6.png", "https://cdn.example/7.png", "https://cdn.example/8.png", "https://cdn.example/9.png", "https://cdn.example/10.png"}}, "at most 9 reference images"},
		{"mixed media sources", "sd-2-E5", map[string]any{"prompt": "x", "images": []any{"https://cdn.example/a.png"}, "metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/b.png"}}}}}, "not both"},
		{"unsupported field", "sd-2-E5", map[string]any{"prompt": "x", "seedance": 2}, "unsupported video request field"},
		{"unsupported metadata field", "sd-2-E5", map[string]any{"prompt": "x", "metadata": map[string]any{"callback": "x"}}, "unsupported video metadata field"},
		{"n larger than one", "sd-2-E5", map[string]any{"prompt": "x", "n": 2}, "n only supports 1"},
		{"fixed resolution", "video-ultra-720p", map[string]any{"prompt": "x", "resolution": "1080p"}, "only supports resolution 720p"},
		{"flat resolution whitelist", "sd-2-E5", map[string]any{"prompt": "x", "resolution": "2k"}, "resolution must be one of"},
		{"watermark only doubao", "sd-2-E5", map[string]any{"prompt": "x", "watermark": true}, "watermark is supported only"},
		{"asset only doubao", "sd-2-E5", map[string]any{"prompt": "x", "images": []any{"asset://ast-123"}}, "asset:// media URLs"},
		{"private media URL", "sd-2-E5", map[string]any{"prompt": "x", "images": []any{"http://localhost/a.png"}}, "public HTTP(S) URL"},
		{"conflicting durations", "sd-2-E5", map[string]any{"prompt": "x", "seconds": "5", "duration": 6}, "duration conflicts with seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", nskevenSubmitContext(tc.model, tc.body))
			require.ErrorContains(t, err, tc.want)
		})
	}

	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"model": "sd-2-E5", "upstreamModel": "sd-2-E5", "baseUrl": "http://newapi.nskeven.uk",
		"apiKey": "fixture-only", "publicTaskId": "abc123def456", "requestBody": map[string]any{"prompt": "x"},
	})
	require.ErrorContains(t, err, "HTTPS API base")
}

func TestNSKevenVideoProtocolAndTaskResults(t *testing.T) {
	plugin := loadNSKevenVideoPlugin(t)

	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "sd-2-E5",
		"body": map[string]any{"kind": "json", "value": map[string]any{"model": "canvas-alias", "prompt": "a cat", "duration": 6}},
	})
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, decoded), &intent))
	assert.Equal(t, "submit", intent["kind"])
	assert.Equal(t, "canvas-alias", intent["model"])
	assert.Equal(t, "text_to_video", intent["action"])
	assert.Equal(t, "a cat", intent["requestBody"].(map[string]any)["prompt"])

	frozen, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "even--seedance-2.0-720p",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "canvas-alias", "prompt": "animate Image 1 with Video 1",
			"metadata": map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "animate Image 1 with Video 1"},
				map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
				map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
				map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}},
			}},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, frozen), &intent))
	assert.Equal(t, "reference_to_video", intent["action"])
	requestBody := intent["requestBody"].(map[string]any)
	assert.Equal(t, []any{"https://cdn.example/ref.png"}, requestBody["images"])
	assert.Equal(t, []any{"https://cdn.example/ref.mp4"}, requestBody["videos"])
	assert.Equal(t, []any{"https://cdn.example/ref.mp3"}, requestBody["audios"])

	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "canvas-alias", "upstreamModel": "sd-2-E5",
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{"animate"}}, "files": []any{map[string]any{"field": "image", "ref": "fixture"}}},
	})
	require.ErrorContains(t, err, "public HTTP(S)")

	submit := nskevenCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"id": "task_385412", "task_id": "task_385412", "object": "video", "status": "queued", "created_at": 1790224802},
	})
	assert.Equal(t, "task_385412", submit["taskId"])
	assert.NotContains(t, submit, "immediate")
	envelopeSubmit := nskevenCallObject(t, plugin, "parseSubmitResponse", map[string]any{"apiKey": "fixture-only"}, map[string]any{
		"statusCode": 200,
		"body":       map[string]any{"code": "success", "message": "", "data": map[string]any{"id": 883, "task_id": "task_385412", "status": "SUBMITTED"}},
	})
	assert.Equal(t, "task_385412", envelopeSubmit["taskId"])
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 500, "body": map[string]any{}})
	require.Error(t, err)
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{
		"statusCode": 400, "body": map[string]any{"error": map[string]any{"message": "failed with sk-secret-value", "type": "new_api_error", "code": ""}},
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
		{"queued", map[string]any{"task_id": "task_385412", "status": "QUEUED"}, map[string]any{"status": "QUEUED"}},
		{"in progress", map[string]any{"status": "IN_PROGRESS", "progress": "45%"}, map[string]any{"status": "IN_PROGRESS", "progress": "45%"}},
		{"success", map[string]any{"status": "SUCCESS", "result_url": "https://newapi.nskeven.uk/v1/videos/task_385412/content"}, map[string]any{"status": "SUCCESS", "progress": "100%"}},
		{"failure", map[string]any{"status": "FAILURE", "fail_reason": "素材缺失"}, map[string]any{"status": "FAILURE", "reason": "素材缺失"}},
		{"failure without reason", map[string]any{"status": "FAILURE"}, map[string]any{"status": "FAILURE", "reason": "Video generation failed"}},
		{"unknown", map[string]any{"status": "WHATEVER"}, map[string]any{"status": "UNKNOWN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := nskevenCallObject(t, plugin, "parseTaskResult", map[string]any{}, tc.body)
			for key, value := range tc.want {
				assert.Equal(t, value, result[key], key)
			}
		})
	}
	envelope := nskevenCallObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{
		"code": "success", "message": "", "data": map[string]any{"task_id": "task_385412", "status": "IN_PROGRESS", "progress": "45%", "data": map[string]any{"status": "SUCCEEDED"}},
	})
	assert.Equal(t, "IN_PROGRESS", envelope["status"])
	redacted := nskevenCallObject(t, plugin, "parseTaskResult", map[string]any{"apiKey": "sk-secret-value"}, map[string]any{"status": "FAILURE", "fail_reason": "token sk-secret-value rejected"})
	assert.NotContains(t, redacted["reason"], "sk-secret-value")

	query := nskevenCallObject(t, plugin, "buildQueryRequest", map[string]any{"baseUrl": nskevenBaseURL, "taskId": "task_385412", "apiKey": "fixture-only"})
	assert.Equal(t, nskevenBaseURL+"/v1/video/generations/task_385412", query["url"])
	assert.Equal(t, "GET", query["method"])

	artifact := nskevenCall(t, plugin, "listArtifacts", map[string]any{"status": "SUCCESS", "data": map[string]any{"task_id": "task_385412"}})
	assert.JSONEq(t, `[{"key":"video","type":"video","mimeType":"video/mp4"}]`, mustJSON(t, artifact))
	assert.JSONEq(t, `[]`, mustJSON(t, nskevenCall(t, plugin, "listArtifacts", map[string]any{"status": "IN_PROGRESS"})))

	content := nskevenCallObject(t, plugin, "buildContentRequest", map[string]any{
		"artifactKey": "video", "upstreamTaskId": "task_385412", "apiKey": "fixture-only",
		"baseUrl": nskevenBaseURL, "clientRequest": map[string]any{"method": "HEAD"},
	})
	assert.Equal(t, nskevenBaseURL+"/v1/videos/task_385412/content", content["url"])
	assert.Equal(t, "HEAD", content["method"])
	assert.NotContains(t, content, "credentialless")
	assert.Equal(t, "Bearer fixture-only", content["headers"].(map[string]any)["Authorization"])
	_, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "video", "upstreamTaskId": "../escape", "apiKey": "fixture-only"})
	require.Error(t, err)

	for status, want := range map[string]string{"QUEUED": "queued", "IN_PROGRESS": "in_progress", "SUCCESS": "completed", "FAILURE": "failed"} {
		rendered, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, map[string]any{"task_id": "task_385412", "status": status})
		require.NoError(t, err)
		var response map[string]any
		require.NoError(t, common.UnmarshalJsonStr(mustJSON(t, rendered), &response))
		assert.Equal(t, want, response["status"])
		assert.Equal(t, "video", response["object"])
	}
}

func TestNSKevenVideoUsage(t *testing.T) {
	plugin := loadNSKevenVideoPlugin(t)
	doubao := nskevenCallObject(t, plugin, "extractUsage", nskevenSubmitContext("doubao-seedance-2-0-260128", map[string]any{"prompt": "x"}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(4), "video_resolution": "720p"}, doubao)
	flat := nskevenCallObject(t, plugin, "extractUsage", nskevenSubmitContext("even--seedance-2.0-720p", map[string]any{"prompt": "x", "duration": 6, "resolution": "720p"}))
	assert.Equal(t, map[string]any{"count": float64(1), "seconds": float64(6), "video_resolution": "720p"}, flat)
	ratioContext := nskevenSubmitContext("doubao-seedance-2-0-260128", map[string]any{"prompt": "x"})
	ratioContext["usagePurpose"] = "billing_ratios"
	assert.Empty(t, nskevenCallObject(t, plugin, "extractUsage", ratioContext))
	assert.Empty(t, nskevenCallObject(t, plugin, "extractUsage", nskevenSubmitContext("doubao-seedance-2-0-260128", map[string]any{"prompt": "x", "negative_prompt": "blur"})))
	assert.Nil(t, nskevenCall(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, map[string]any{}))
}

func TestNSKevenVideoCanvasRequests(t *testing.T) {
	plugin := loadNSKevenVideoPlugin(t)
	for _, tc := range []struct {
		name   string
		model  string
		body   map[string]any
		want   string
		action string
	}{
		{
			name: "canvas generic first frame", model: "doubao-seedance-2-0-260128", action: "image_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate", "duration": 8, "seconds": "8", "resolution": "720p", "aspect_ratio": "16:9", "image": "https://cdn.example/start.png"},
			want: `{"model":"doubao-seedance-2-0-260128","prompt":"animate","seconds":"8","metadata":{"resolution":"720p","ratio":"16:9","content":[{"type":"image_url","image_url":{"url":"https://cdn.example/start.png"},"role":"first_frame"}]}}`,
		},
		{
			name: "canvas generic express", model: "sd-2-E5", action: "image_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate", "duration": 8, "seconds": "8", "resolution": "720p", "image": "https://cdn.example/start.png"},
			want: `{"model":"sd-2-E5","prompt":"animate","seconds":"8","resolution":"720p","first_frame_url":"https://cdn.example/start.png"}`,
		},
		{
			name: "canvas frozen references", model: "even--seedance-2.0-720p", action: "image_to_video",
			body: map[string]any{"model": "canvas-alias", "prompt": "animate Image 1", "metadata": map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "animate Image 1"},
				map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}},
			}}},
			want: `{"model":"even--seedance-2.0-720p","prompt":"animate Image 1","seconds":"5","resolution":"720p","images":["https://cdn.example/ref.png"]}`,
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
			ctx := nskevenSubmitContext(tc.model, intent["requestBody"].(map[string]any))
			ctx["model"] = "canvas-alias"
			request := nskevenCallObject(t, plugin, "buildSubmitRequest", ctx)
			assert.JSONEq(t, tc.want, mustJSON(t, request["body"]))
		})
	}
}
