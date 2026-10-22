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
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func image2ProDynamicPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("image2pro")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	return plugin
}

func image2ProDynamicObject(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var object map[string]any
	require.NoError(t, common.Unmarshal(encoded, &object))
	return object
}

func image2ProDynamicDecode(t *testing.T, plugin *jsplugin.LoadedPlugin, protocol, client, upstream string, body map[string]any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
		"model": client, "upstreamModel": upstream,
		"body": map[string]any{"kind": "json", "value": body},
	})
	require.NoError(t, err)
	intent := image2ProDynamicObject(t, value)
	assert.Equal(t, "submit", intent["kind"])
	assert.Equal(t, client, intent["model"])
	return intent
}

// TestImage2ProDynamicImageLifecycle exercises a raw catalog ID through the real host adapter.
func TestImage2ProDynamicImageLifecycle(t *testing.T) {
	plugin := image2ProDynamicPlugin(t)
	const client, upstream = "future-image-v7", "future-image-v7"
	assert.NotContains(t, plugin.Meta.Models, upstream, "fixture must remain outside the plugin's suggested models")
	intent := image2ProDynamicDecode(t, plugin, "openai_image_tasks", client, upstream, map[string]any{
		"model": client, "prompt": "A lighthouse at dusk", "n": 2, "size": "2048x2048",
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: client,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://synthetic.example/v1", ApiKey: "synthetic-fixture", UpstreamModelName: upstream,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_dynamic_image_fixture"},
	}
	adaptor := taskplugin.New(plugin)
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/tasks", nil)
	c.Set("task_request", intent["requestBody"])
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	facts, err := adaptor.ExtractUsageFactsValidated(c, info)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"count": float64(2)}, facts)
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	var sent map[string]any
	require.NoError(t, common.DecodeJson(body, &sent))
	assert.Equal(t, upstream, sent["model"], "upstream receives the exact catalog ID")
	assert.Equal(t, "A lighthouse at dusk", sent["prompt"])
	assert.Equal(t, float64(2), sent["n"])
	assert.Len(t, sent, 4, "internal routing state must not leak to the provider")
	response := &http.Response{StatusCode: http.StatusAccepted, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
		`{"id":"future-image-job:71","status":"queued","n":2}`,
	))}
	parsed, taskErr := adaptor.ParseResponse(c, response, info)
	require.Nil(t, taskErr)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "future-image-job:71", parsed.UpstreamTaskID)
	task := &model.Task{
		Properties:  model.Properties{OriginModelName: client, UpstreamModelName: upstream},
		PrivateData: model.TaskPrivateData{PluginState: parsed.PluginState},
	}
	state := image2ProDynamicObject(t, jsonValueForDynamicTest(t, parsed.PluginState))
	// Query with only the raw model identity verifies persisted image state is sufficient.
	queryValue, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
		"model": upstream, "upstreamModel": upstream, "taskId": parsed.UpstreamTaskID, "state": state,
		"baseUrl": "https://synthetic.example/v1", "apiKey": "synthetic-fixture",
	})
	require.NoError(t, err)
	query := image2ProDynamicObject(t, queryValue)
	assert.Equal(t, "https://synthetic.example/v1/images/tasks/future-image-job%3A71", query["url"])
	assert.Equal(t, http.MethodGet, query["method"])
	completed, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK}, []byte(
		`{"status":"completed","n":99,"data":[{"url":"https://cdn.example/first.png"},{"url":"https://cdn.example/second.png"}]}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", completed.Status)
	assert.Equal(t, "https://cdn.example/first.png", completed.Url)
	assert.Equal(t, map[string]any{"count": float64(2)}, completed.UsageFacts, "billing uses frozen request count rather than provider count")
	assert.JSONEq(t, `{"kind":"image","n":2}`, string(completed.PluginState))
	// Persist the returned completion state, then retry with only the raw catalog identity.
	task.Properties.OriginModelName = upstream
	task.PrivateData.PluginState = completed.PluginState
	completedState := jsonValueForDynamicTest(t, completed.PluginState)
	queryValue, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
		"model": upstream, "upstreamModel": upstream, "taskId": parsed.UpstreamTaskID, "state": completedState,
		"baseUrl": "https://synthetic.example/v1", "apiKey": "synthetic-fixture",
	})
	require.NoError(t, err)
	assert.Equal(t, query, image2ProDynamicObject(t, queryValue))
	repeated, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK}, []byte(
		`{"status":"completed","n":99,"data":[{"url":"https://cdn.example/first.png"}]}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", repeated.Status)
	assert.Equal(t, completed.UsageFacts, repeated.UsageFacts, "a repeated completion preserves the frozen image count")
	assert.JSONEq(t, string(completed.PluginState), string(repeated.PluginState))
}

func jsonValueForDynamicTest(t *testing.T, data []byte) any {
	t.Helper()
	var value any
	require.NoError(t, common.Unmarshal(data, &value))
	return value
}

// TestImage2ProDynamicVideoAudio verifies a raw video ID and media references without network calls.
func TestImage2ProDynamicVideoAudio(t *testing.T) {
	plugin := image2ProDynamicPlugin(t)
	const client, upstream = "future-video-v9", "future-video-v9"
	assert.NotContains(t, plugin.Meta.Models, upstream)
	intent := image2ProDynamicDecode(t, plugin, "openai_video", client, upstream, map[string]any{
		"model": client, "prompt": "Waves matching the soundtrack", "duration": 22,
		"audios": []any{"https://cdn.example/soundtrack.mp3", map[string]any{"mediaId": "audio-fixture:123"}},
	})
	ctx := map[string]any{
		"model": client, "upstreamModel": upstream, "requestBody": intent["requestBody"],
		"baseUrl": "https://synthetic.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_dynamic_video_fixture",
	}
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	descriptor := image2ProDynamicObject(t, value)
	assert.Equal(t, "https://synthetic.example/v1/videos", descriptor["url"])
	assert.Equal(t, true, descriptor["noRetry"])
	sent := descriptor["body"].(map[string]any)
	assert.Equal(t, upstream, sent["model"])
	assert.Equal(t, float64(22), sent["duration"], "new models must not inherit Seedance or Flash duration ceilings")
	assert.Equal(t, []any{"https://cdn.example/soundtrack.mp3", map[string]any{"mediaId": "audio-fixture:123"}}, sent["audios"])
	assert.NotContains(t, sent, "content")
	usage, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"seconds": float64(22)}, image2ProDynamicObject(t, usage))
	parsedValue, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{
		"statusCode": http.StatusOK, "body": map[string]any{"id": "abcdef1234567890", "status": "processing"},
	})
	require.NoError(t, err)
	parsed := image2ProDynamicObject(t, parsedValue)
	queryValue, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
		"model": client, "upstreamModel": upstream, "taskId": parsed["taskId"], "state": parsed["state"],
		"baseUrl": "https://synthetic.example", "apiKey": "synthetic-fixture",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://synthetic.example/v1/videos/abcdef1234567890", image2ProDynamicObject(t, queryValue)["url"])
	completed, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{
		"model": client, "upstreamModel": upstream, "state": parsed["state"],
	}, map[string]any{"status": "SUCCESS"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"seconds": float64(22)}, image2ProDynamicObject(t, completed))
}

func TestImage2ProDynamicLegacyAliases(t *testing.T) {
	plugin := image2ProDynamicPlugin(t)
	t.Run("image", func(t *testing.T) {
		const client, upstream = "Image2Pro-Image-future-image-v7", "future-image-v7"
		intent := image2ProDynamicDecode(t, plugin, "openai_image_tasks", client, upstream, map[string]any{
			"model": client, "prompt": "A lighthouse at dusk", "n": 2,
		})
		request := image2ProDynamicObject(t, intent["requestBody"])
		assert.Equal(t, upstream, request["model"])
		assert.Equal(t, true, request["__imageTask"])
	})
	t.Run("video", func(t *testing.T) {
		const client, upstream = "Image2Pro-Video-future-video-v9", "future-video-v9"
		intent := image2ProDynamicDecode(t, plugin, "openai_video", client, upstream, map[string]any{
			"model": client, "prompt": "Waves matching the soundtrack", "duration": 22,
		})
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": client, "upstreamModel": upstream, "requestBody": intent["requestBody"],
			"baseUrl": "https://synthetic.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_legacy_video_fixture",
		})
		require.NoError(t, err)
		sent := image2ProDynamicObject(t, image2ProDynamicObject(t, value)["body"])
		assert.Equal(t, upstream, sent["model"])
		assert.Equal(t, float64(22), sent["duration"])
	})
}

func TestImage2ProDynamicAudioRejections(t *testing.T) {
	plugin := image2ProDynamicPlugin(t)
	for _, tc := range []struct {
		name   string
		audios []any
	}{
		{"duplicate URL", []any{"https://cdn.example/a.mp3", "https://cdn.example/a.mp3"}},
		{"duplicate media ID", []any{map[string]any{"mediaId": "audio-1"}, map[string]any{"mediaId": "audio-1"}}},
		{"HTTP", []any{"http://cdn.example/a.mp3"}},
		{"data URL", []any{"data:audio/mpeg;base64,AAAA"}},
		{"credentials", []any{"https://user:synthetic@cdn.example/a.mp3"}},
		{"local file URL", []any{"file:///tmp/synthetic.mp3"}},
		{"local path", []any{"D:\\synthetic\\audio.mp3"}},
		{"file placeholder", []any{map[string]any{"__fileRef": "synthetic-audio", "encoding": "dataUrl", "mimeType": "audio/mpeg"}}},
		{"too many", []any{"https://cdn.example/1.mp3", "https://cdn.example/2.mp3", "https://cdn.example/3.mp3", "https://cdn.example/4.mp3"}},
		{"mixed media ID and URL", []any{map[string]any{"mediaId": "audio-1", "url": "https://cdn.example/1.mp3"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": "new-video", "upstreamModel": "new-video", "body": map[string]any{
					"kind": "json", "value": map[string]any{"model": "new-video", "prompt": "ocean", "duration": 5, "audios": tc.audios},
				},
			})
			require.Error(t, err)
		})
	}
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "new-video", "upstreamModel": "new-video", "body": map[string]any{
			"kind": "multipart", "fields": map[string]any{"model": []string{"new-video"}, "prompt": []string{"ocean"}, "duration": []string{"5"}},
			"files": []any{map[string]any{"field": "audios", "ref": "synthetic-audio", "filename": "fixture.mp3", "mimeType": "audio/mpeg", "size": 32}},
		},
	})
	require.Error(t, err, "audio uploads require the provider upload API and cannot be sent as inline image-style references")
}

func TestImage2ProDynamicTypeMisuse(t *testing.T) {
	plugin := image2ProDynamicPlugin(t)
	for _, tc := range []struct{ protocol, client, upstream string }{
		{"openai_image_tasks", "Seedance2.0", "Seedance2.0"},
		{"openai_image_tasks", "Image2Pro-Video-new-video", "new-video"},
		{"openai_video", "gpt-image-2.0", "gpt-image-2.0"},
		{"openai_video", "Image2Pro-Image-new-image", "new-image"},
		{"openai_responses", "gpt-image-2.0", "gpt-image-2.0"},
		{"openai_responses", "Image2Pro-Image-new-image", "new-image"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			request := map[string]any{"model": tc.client, "prompt": "ocean", "duration": 5}
			if tc.protocol == "openai_image_tasks" {
				delete(request, "duration")
			}
			if tc.protocol == "openai_responses" {
				delete(request, "prompt")
				request["input"] = "ocean"
			}
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{tc.protocol, "decodeRequest"}, map[string]any{
				"model": tc.client, "upstreamModel": tc.upstream, "body": map[string]any{"kind": "json", "value": request},
			})
			require.Error(t, err, "known media types and legacy aliases must reject the other protocol")
		})
	}
}

func TestImage2ProKnownImageModelsUseImageProtocol(t *testing.T) {
	plugin := image2ProDynamicPlugin(t)
	for _, model := range plugin.Meta.Models {
		assert.Contains(t, []string{"image", "video"}, plugin.Meta.ModelMediaTypes[model], "known model %s must declare its compatible media type", model)
	}
	assert.Equal(t, "video", plugin.Meta.ModelMediaTypes["Seedance2.0 0.9r"])
	for _, model := range []string{"Nano-banana-Pro", "无限制-Seedream-5-Pro-2K"} {
		t.Run(model, func(t *testing.T) {
			assert.Equal(t, "image", plugin.Meta.ModelMediaTypes[model])
			intent := image2ProDynamicDecode(t, plugin, "openai_image_tasks", model, model, map[string]any{
				"model": model, "prompt": "A quiet coastal town",
			})
			request := image2ProDynamicObject(t, intent["requestBody"])
			assert.Equal(t, true, request["__imageTask"])

			for _, protocol := range []string{"openai_video", "openai_responses"} {
				body := map[string]any{"model": model, "prompt": "A quiet coastal town", "duration": 5}
				if protocol == "openai_responses" {
					delete(body, "prompt")
					body["input"] = "A quiet coastal town"
				}
				_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
					"model": model, "upstreamModel": model, "body": map[string]any{"kind": "json", "value": body},
				})
				require.Error(t, err, "image model must not be accepted by video protocol %s", protocol)
			}
		})
	}
}
