package plugins_test

import (
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const globalstoreBaseURL = "https://saas.globalstore.ai/kyyReactApiServer"

// globalstoreImageModels 与 globalstoreVideoModels 必须与插件 meta.models 逐字一致。
// 唯一例外是 gpt-image-2：该名字是本 fork 的内置模型，插件对外加 globalstore/ 前缀以避免
// 抢占内置计费与路由判定，上游仍按原名下发（见插件里的 UPSTREAM_MODEL_NAMES）。
var globalstoreImageModels = []string{
	"banana2", "bananapro", "banana_c1", "banana_c2",
	"globalstore/gpt-image-2", "gpt-image-2.5", "seedream-5.0", "seedream_5.0Pro",
}

var globalstoreVideoModels = []string{
	"sd_2.0_discount", "sd_2.0_fast_discount", "sd_2.0_mini_discount", "sd_2.5_discount",
	"sd_2.0_special", "sd_2.0_fast_special", "sd_2.0_mini_special", "sd_2.5_special",
	"seedance_2.0_nsfw_c1", "grok_video", "grok_1.5", "omni_720P",
	"veo3.1-fast", "wan3.0", "wan3.0-Prime",
}

var globalstoreUsageKeys = []string{
	"audio_mode", "count", "images", "input_audios", "input_images",
	"input_videos", "quality", "resolution", "seconds",
}

func loadGlobalstorePlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("globalstore")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "globalstore"})
	require.NoError(t, err)
	return plugin
}

// globalstoreSubmitContext 构造 buildSubmitRequest/extractUsage 需要的提交上下文。
func globalstoreSubmitContext(model string, body map[string]any) map[string]any {
	return map[string]any{
		"model": model, "upstreamModel": model,
		"baseUrl": globalstoreBaseURL, "apiKey": "fixture-only", "requestBody": body,
	}
}

func globalstoreValue(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	return value
}

func globalstoreObject(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(globalstoreValue(t, plugin, hook, args...))
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func globalstoreJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func globalstoreProtocol(t *testing.T, plugin *jsplugin.LoadedPlugin, protocol string, ctx map[string]any, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, append([]any{ctx}, args...)...)
	require.NoError(t, err)
	encoded, marshalErr := common.Marshal(value)
	require.NoError(t, marshalErr)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func globalstoreAction(t *testing.T, plugin *jsplugin.LoadedPlugin, protocol string, ctx map[string]any) string {
	t.Helper()
	return globalstoreProtocol(t, plugin, protocol, ctx)["action"].(string)
}

// TestGlobalstoreContracts 使用真实插件宿主核实描述符、模型隔离与计费字段。
func TestGlobalstoreContracts(t *testing.T) {
	source, err := plugins.Source("globalstore")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "globalstore"})
	require.NoError(t, err)

	assert.Equal(t, "globalstore", plugin.Meta.Key)
	assert.Equal(t, globalstoreBaseURL, plugin.Meta.BaseURL)
	assert.Equal(t, "1.0.0", plugin.Meta.Version)
	assert.Equal(t, []string{"saas.globalstore.ai:443"}, plugin.Meta.AllowedHosts)
	assert.Equal(t, []string{"task-submit-no-retry@1"}, plugin.Meta.RequiredCapabilities)
	assert.Equal(t, "per_task", plugin.Meta.FetchMode)
	assert.Equal(t, "https://docs.globalstore.ai/zh", plugin.Meta.Website)

	declared := append([]string(nil), plugin.Meta.Models...)
	sort.Strings(declared)
	expected := append(append([]string(nil), globalstoreImageModels...), globalstoreVideoModels...)
	sort.Strings(expected)
	assert.Equal(t, expected, declared, "描述符必须覆盖文档的全部 23 个模型")

	require.Len(t, plugin.Meta.Protocols, 2)
	assert.Equal(t, "openai_video", plugin.Meta.Protocols[0].Name)
	assert.Equal(t, globalstoreVideoModels, plugin.Meta.Protocols[0].Models)
	assert.Empty(t, plugin.Meta.Protocols[0].Supports, "标准视频入口不声明模式")
	assert.Equal(t, "openai_responses", plugin.Meta.Protocols[1].Name)
	assert.Equal(t, globalstoreImageModels, plugin.Meta.Protocols[1].Models)
	assert.Equal(t, []string{"background"}, plugin.Meta.Protocols[1].Supports, "图片模型只有异步后台模式")

	t.Run("binds image models to background Responses only", func(t *testing.T) {
		for _, model := range globalstoreImageModels {
			binding, found := registry.Generation().LookupEndpoint("POST", "/v1/responses", model)
			require.True(t, found, model)
			assert.Same(t, plugin, binding.Plugin)
			assert.Equal(t, "openai_responses", binding.Protocol)
			_, videoFound := registry.Generation().LookupEndpoint("POST", "/v1/videos", model)
			assert.False(t, videoFound, model)
		}
	})

	t.Run("binds video models to the video endpoint only", func(t *testing.T) {
		for _, model := range globalstoreVideoModels {
			binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", model)
			require.True(t, found, model)
			assert.Same(t, plugin, binding.Plugin)
			assert.Equal(t, "openai_video", binding.Protocol)
			_, responsesFound := registry.Generation().LookupEndpoint("POST", "/v1/responses", model)
			assert.False(t, responsesFound, model)
		}
	})

	t.Run("declares every usage fact with a description", func(t *testing.T) {
		keys := make([]string, 0, len(plugin.Meta.UsageSchema))
		for key, schema := range plugin.Meta.UsageSchema {
			keys = append(keys, key)
			assert.NotEmpty(t, schema.Description, key)
		}
		sort.Strings(keys)
		assert.Equal(t, globalstoreUsageKeys, keys)
	})
}

// TestGlobalstoreVideoSubmit 核实上游请求体、地址归一化与参数校验。
func TestGlobalstoreVideoSubmit(t *testing.T) {
	plugin := loadGlobalstorePlugin(t)

	t.Run("builds the documented task request", func(t *testing.T) {
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 5}))
		assert.Equal(t, globalstoreBaseURL+"/v2/model-center/tasks", request["url"])
		assert.Equal(t, "POST", request["method"])
		assert.Equal(t, true, request["noRetry"], "提交结果未知时禁止宿主自动重试")
		headers := request["headers"].(map[string]any)
		assert.Equal(t, "Bearer fixture-only", headers["Authorization"])
		assert.Equal(t, "application/json", headers["Content-Type"])
		assert.JSONEq(t, `{"model":"sd_2.0_discount","prompt":"flower","duration":5,"resolution":"4k","aspect_ratio":"16:9","generate_audio":true}`, globalstoreJSON(t, request["body"]))
	})

	t.Run("forwards frames, references and switches", func(t *testing.T) {
		body := map[string]any{
			"prompt": "flower", "seconds": 10, "resolution": "1080p", "aspect_ratio": "9:16",
			"first_image":      "https://cdn.example/first.png",
			"last_image":       []any{"https://cdn.example/last.png"},
			"reference_images": []any{"https://cdn.example/ref.png"},
			"reference_videos": []any{"https://cdn.example/ref.mp4"},
			"reference_audios": []any{"https://cdn.example/ref.mp3"},
			"seed":             7, "generate_audio": false, "aigc_watermark": false,
		}
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("sd_2.5_special", body))
		assert.JSONEq(t, `{"model":"sd_2.5_special","prompt":"flower","duration":10,"resolution":"1080p","aspect_ratio":"9:16","first_image":["https://cdn.example/first.png"],"last_image":["https://cdn.example/last.png"],"reference_images":["https://cdn.example/ref.png"],"reference_videos":["https://cdn.example/ref.mp4"],"reference_audios":["https://cdn.example/ref.mp3"],"seed":7,"generate_audio":false}`, globalstoreJSON(t, request["body"]))
	})

	t.Run("accepts metadata and content arrays", func(t *testing.T) {
		body := map[string]any{
			"prompt": "flower", "seconds": 5,
			"metadata": map[string]any{"resolution": "720p", "content": []any{
				map[string]any{"type": "text", "text": "flower"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/first.png"}, "role": "first_frame"},
			}},
		}
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("sd_2.0_special", body))
		assert.JSONEq(t, `{"model":"sd_2.0_special","prompt":"flower","duration":5,"resolution":"720p","aspect_ratio":"16:9","first_image":["https://cdn.example/first.png"],"generate_audio":true}`, globalstoreJSON(t, request["body"]))
	})

	t.Run("derives resolution and ratio from pixel sizes", func(t *testing.T) {
		for _, test := range []struct{ size, resolution, ratio string }{
			{"1280x720", "720p", "16:9"}, {"720x1280", "720p", "9:16"},
			{"1920x1080", "1080p", "16:9"}, {"1080x1920", "1080p", "9:16"},
		} {
			request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 5, "size": test.size}))
			upstream := request["body"].(map[string]any)
			assert.Equal(t, test.resolution, upstream["resolution"], test.size)
			assert.Equal(t, test.ratio, upstream["aspect_ratio"], test.size)
		}
	})

	t.Run("omits the default ratio for models without one", func(t *testing.T) {
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("grok_video", map[string]any{"prompt": "flower"}))
		assert.JSONEq(t, `{"model":"grok_video","prompt":"flower","duration":8,"resolution":"720p"}`, globalstoreJSON(t, request["body"]))
	})

	t.Run("keeps documented defaults", func(t *testing.T) {
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("veo3.1-fast", map[string]any{"prompt": "flower"}))
		assert.JSONEq(t, `{"model":"veo3.1-fast","prompt":"flower","duration":8,"resolution":"720p","aspect_ratio":"16:9","generate_audio":true}`, globalstoreJSON(t, request["body"]))
	})

	t.Run("normalizes the channel base URL", func(t *testing.T) {
		for _, test := range []struct{ base, want string }{
			{"", globalstoreBaseURL},
			{globalstoreBaseURL, globalstoreBaseURL},
			{globalstoreBaseURL + "/", globalstoreBaseURL},
			{globalstoreBaseURL + "/v2", globalstoreBaseURL},
			{globalstoreBaseURL + "/v2/model-center", globalstoreBaseURL},
			{globalstoreBaseURL + "/v2/model-center/tasks", globalstoreBaseURL},
			{"https://proxy.example/prefix", "https://proxy.example/prefix"},
			{"https://proxy.example/prefix/v2", "https://proxy.example/prefix"},
		} {
			t.Run("base="+test.base, func(t *testing.T) {
				ctx := globalstoreSubmitContext("sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 5})
				ctx["baseUrl"] = test.base
				request := globalstoreObject(t, plugin, "buildSubmitRequest", ctx)
				assert.Equal(t, test.want+"/v2/model-center/tasks", request["url"])
				if test.base != "" {
					require.NoError(t, jsplugin.ValidateRequestURL(request["url"].(string), test.base, plugin.Meta.AllowedHosts))
				}
				ctx["taskId"] = "task/id?x"
				query := globalstoreObject(t, plugin, "buildQueryRequest", ctx)
				assert.Equal(t, test.want+"/v2/model-center/tasks/task%2Fid%3Fx", query["url"])
				assert.Equal(t, "GET", query["method"])
			})
		}
		for _, base := range []string{"https://user:pass@example.com", "https://example.com/#fragment", "ftp://example.com"} {
			ctx := globalstoreSubmitContext("sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 5})
			ctx["baseUrl"] = base
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.Error(t, err, base)
		}
	})
}

func TestGlobalstoreVideoActions(t *testing.T) {
	plugin := loadGlobalstorePlugin(t)
	for _, test := range []struct {
		name   string
		body   map[string]any
		action string
	}{
		{"text to video", map[string]any{"prompt": "flower", "seconds": 5}, "text_to_video"},
		{"first frame", map[string]any{"prompt": "flower", "seconds": 5, "first_image": "https://cdn.example/a.png"}, "image_to_video"},
		{"single reference", map[string]any{"prompt": "flower", "seconds": 5, "reference_images": []any{"https://cdn.example/a.png"}}, "image_to_video"},
		{"multiple references", map[string]any{"prompt": "flower", "seconds": 5, "reference_images": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"}}, "reference_to_video"},
		{"reference video", map[string]any{"prompt": "flower", "seconds": 5, "reference_videos": []any{"https://cdn.example/v.mp4"}}, "reference_to_video"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := map[string]any{
				"model": "sd_2.0_special", "upstreamModel": "sd_2.0_special",
				"body": map[string]any{"kind": "json", "value": test.body},
			}
			decoded := globalstoreProtocol(t, plugin, "openai_video", ctx)
			assert.Equal(t, "submit", decoded["kind"])
			assert.Equal(t, "sd_2.0_special", decoded["model"])
			requestBody := decoded["requestBody"].(map[string]any)
			assert.Equal(t, "sd_2.0_special", requestBody["model"], "上游模型名不依赖客户端模型名")
		})
	}

	t.Run("decodes form bodies", func(t *testing.T) {
		ctx := map[string]any{
			"model": "sd_2.0_special", "upstreamModel": "sd_2.0_special",
			"body": map[string]any{"kind": "form", "fields": map[string]any{
				"prompt": []string{"flower"}, "seconds": []string{"5"}, "resolution": []string{"720p"},
			}},
		}
		decoded := globalstoreProtocol(t, plugin, "openai_video", ctx)
		assert.Equal(t, "text_to_video", decoded["action"])
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("sd_2.0_special", decoded["requestBody"].(map[string]any)))
		assert.JSONEq(t, `{"model":"sd_2.0_special","prompt":"flower","duration":5,"resolution":"720p","aspect_ratio":"16:9","generate_audio":true}`, globalstoreJSON(t, request["body"]))
	})

	t.Run("rejects file uploads", func(t *testing.T) {
		_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "sd_2.0_special", "upstreamModel": "sd_2.0_special",
			"body": map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{"flower"}}, "files": []any{map[string]any{"field": "image"}}},
		})
		require.ErrorContains(t, err, "public media URLs")
	})
}

func TestGlobalstoreVideoValidation(t *testing.T) {
	plugin := loadGlobalstorePlugin(t)
	for name, test := range map[string]struct {
		model string
		body  map[string]any
	}{
		"unknown model":                {"nope", map[string]any{"prompt": "flower"}},
		"missing prompt":               {"sd_2.0_discount", map[string]any{"seconds": 5}},
		"blank prompt":                 {"sd_2.0_discount", map[string]any{"prompt": "  ", "seconds": 5}},
		"duration below minimum":       {"sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 4}},
		"duration above maximum":       {"sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 16}},
		"duration not an integer":      {"sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 5.5}},
		"fixed duration mismatch":      {"veo3.1-fast", map[string]any{"prompt": "flower", "seconds": 5}},
		"unsupported resolution":       {"sd_2.0_discount", map[string]any{"prompt": "flower", "resolution": "2k"}},
		"unsupported ratio":            {"sd_2.0_discount", map[string]any{"prompt": "flower", "ratio": "5:7"}},
		"unknown pixel size":           {"sd_2.0_discount", map[string]any{"prompt": "flower", "size": "640x360"}},
		"size and resolution":          {"sd_2.0_discount", map[string]any{"prompt": "flower", "size": "1280x720", "resolution": "720p"}},
		"frames on a model without":    {"grok_video", map[string]any{"prompt": "flower", "first_image": "https://cdn.example/a.png"}},
		"frames on a pixel-only model": {"veo3.1-fast", map[string]any{"prompt": "flower", "last_image": "https://cdn.example/a.png"}},
		"seed on a model without":      {"wan3.0", map[string]any{"prompt": "flower", "seed": 1}},
		"audio on a model without":     {"wan3.0", map[string]any{"prompt": "flower", "generate_audio": true}},
		"non boolean audio":            {"sd_2.0_discount", map[string]any{"prompt": "flower", "generate_audio": "yes"}},
		"private media URL":            {"sd_2.0_discount", map[string]any{"prompt": "flower", "first_image": "data:image/png;base64,abc"}},
		"duration conflict":            {"sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 5, "duration": 6}},
		"ratio conflict":               {"sd_2.0_discount", map[string]any{"prompt": "flower", "ratio": "16:9", "aspect_ratio": "9:16"}},
		"metadata conflict":            {"sd_2.0_discount", map[string]any{"prompt": "flower", "seconds": 5, "metadata": map[string]any{"seconds": 6}}},
		"invalid metadata":             {"sd_2.0_discount", map[string]any{"prompt": "flower", "metadata": "not-json"}},
		"invalid content":              {"sd_2.0_discount", map[string]any{"prompt": "flower", "content": []any{true}}},
		"unsupported content type":     {"sd_2.0_discount", map[string]any{"prompt": "flower", "content": []any{map[string]any{"type": "file_url", "url": "https://cdn.example/a.pdf"}}}},
		"unsupported content role":     {"sd_2.0_special", map[string]any{"prompt": "flower", "content": []any{map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/v.mp4"}, "role": "first_frame"}}}},
		"too many first frames":        {"sd_2.0_special", map[string]any{"prompt": "flower", "first_image": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", globalstoreSubmitContext(test.model, test.body))
			require.Error(t, err)
		})
	}

	t.Run("enforces per model reference limits", func(t *testing.T) {
		urls := make([]any, 7)
		for index := range urls {
			urls[index] = "https://cdn.example/reference.png"
		}
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", globalstoreSubmitContext("banana_c2", map[string]any{"prompt": "flower", "reference_images": urls}))
		require.ErrorContains(t, err, "reference_images cannot exceed 6")

		urls = urls[:6]
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("banana_c2", map[string]any{"prompt": "flower", "reference_images": urls}))
		upstream := request["body"].(map[string]any)
		assert.Len(t, upstream["reference_images"], 6)
	})

	t.Run("rejects images on a model without image inputs", func(t *testing.T) {
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", globalstoreSubmitContext("veo3.1-fast", map[string]any{"prompt": "flower", "reference_images": []any{"https://cdn.example/a.png"}}))
		require.ErrorContains(t, err, "does not accept image inputs")
	})
}

func TestGlobalstoreTaskResults(t *testing.T) {
	plugin := loadGlobalstorePlugin(t)

	t.Run("parses the submit response", func(t *testing.T) {
		response := globalstoreObject(t, plugin, "parseSubmitResponse", map[string]any{}, map[string]any{
			"statusCode": 202, "body": map[string]any{"id": "gs-task-1", "object": "video", "status": "queued"},
		})
		assert.Equal(t, "gs-task-1", response["taskId"])

		for name, call := range map[string]map[string]any{
			"http error":   {"statusCode": 500, "body": map[string]any{"id": "gs-task-1"}},
			"error object": {"statusCode": 200, "body": map[string]any{"error": map[string]any{"message": "denied"}, "id": "gs-task-1"}},
			"missing id":   {"statusCode": 200, "body": map[string]any{"object": "video"}},
			"non object":   {"statusCode": 200, "body": "nope"},
		} {
			t.Run(name, func(t *testing.T) {
				_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, call)
				require.Error(t, err)
			})
		}
	})

	t.Run("maps task statuses", func(t *testing.T) {
		for _, test := range []struct {
			name     string
			body     map[string]any
			status   string
			reason   string
			url      string
			progress string
		}{
			{name: "queued", body: map[string]any{"status": "queued", "object": "video"}, status: "QUEUED"},
			{name: "processing", body: map[string]any{"status": "processing", "object": "video", "progress": 42.6}, status: "IN_PROGRESS", progress: "42%"},
			{name: "completed", body: map[string]any{"status": "completed", "object": "video", "video_url": "https://cdn.example/v.mp4"}, status: "SUCCESS", url: "https://cdn.example/v.mp4", progress: "100%"},
			{name: "completed image", body: map[string]any{"status": "completed", "object": "image", "image_url": "https://cdn.example/i.png"}, status: "SUCCESS", url: "https://cdn.example/i.png", progress: "100%"},
			{name: "failed with message", body: map[string]any{"status": "failed", "object": "video", "error": map[string]any{"message": "content rejected"}}, status: "FAILURE", reason: "content rejected"},
			{name: "failed without message", body: map[string]any{"status": "failed", "object": "video"}, status: "FAILURE", reason: "upstream task failed"},
			{name: "unknown status", body: map[string]any{"status": "paused", "object": "video"}, status: "UNKNOWN", reason: "unrecognized task status"},
			{name: "error without status", body: map[string]any{"error": "denied", "object": "video"}, status: "FAILURE", reason: "denied"},
		} {
			t.Run(test.name, func(t *testing.T) {
				result := globalstoreObject(t, plugin, "parseTaskResult", map[string]any{"upstreamModel": "sd_2.0_discount"}, test.body)
				assert.Equal(t, test.status, result["status"])
				if test.reason != "" {
					assert.Equal(t, test.reason, result["reason"])
				}
				if test.url != "" {
					assert.Equal(t, test.url, result["url"])
				}
				if test.progress != "" {
					assert.Equal(t, test.progress, result["progress"])
				}
			})
		}
	})

	t.Run("rejects a completed task without a result URL", func(t *testing.T) {
		_, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"upstreamModel": "sd_2.0_discount"}, map[string]any{"status": "completed", "object": "video"})
		require.ErrorContains(t, err, "without a result URL")
	})

	t.Run("keeps missing tasks terminal but retryable errors raised", func(t *testing.T) {
		result := globalstoreObject(t, plugin, "parseTaskResult", map[string]any{"upstreamModel": "sd_2.0_discount"}, map[string]any{}, map[string]any{"statusCode": 404})
		assert.Equal(t, "FAILURE", result["status"])
		for _, statusCode := range []int{408, 429, 500, 503} {
			_, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"upstreamModel": "sd_2.0_discount"}, map[string]any{}, map[string]any{"statusCode": statusCode})
			require.ErrorContains(t, err, "upstream task HTTP status", "查询临时错误不能把已受理任务判为失败")
		}
		unknown := globalstoreObject(t, plugin, "parseTaskResult", map[string]any{"upstreamModel": "sd_2.0_discount"}, "not-an-object")
		assert.Equal(t, "UNKNOWN", unknown["status"])
	})
}

// TestGlobalstoreImageResponses 核实图片模型只走后台 Responses 且不泄漏上游地址。
func TestGlobalstoreImageResponses(t *testing.T) {
	plugin := loadGlobalstorePlugin(t)

	resolve := func(t *testing.T, value map[string]any) map[string]any {
		t.Helper()
		return globalstoreProtocol(t, plugin, "openai_responses", map[string]any{
			"model": "banana2", "upstreamModel": "banana2",
			"body": map[string]any{"kind": "json", "value": value},
		})
	}

	t.Run("decodes a text request", func(t *testing.T) {
		decoded := resolve(t, map[string]any{"model": "banana2", "input": "a cat on a roof"})
		assert.Equal(t, "submit", decoded["kind"])
		assert.Equal(t, "text_to_image", decoded["action"])
		request := decoded["requestBody"].(map[string]any)
		assert.Equal(t, "a cat on a roof", request["prompt"])
		assert.Equal(t, "banana2", request["model"])
	})

	t.Run("decodes a message with reference images", func(t *testing.T) {
		decoded := resolve(t, map[string]any{"model": "banana2", "input": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "a cat"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/a.png"},
			},
		}}})
		assert.Equal(t, "image_to_image", decoded["action"])
		request := decoded["requestBody"].(map[string]any)
		assert.Equal(t, "a cat", request["prompt"])
		assert.Equal(t, []any{"https://cdn.example/a.png"}, request["reference_images"])
	})

	t.Run("builds the upstream image request", func(t *testing.T) {
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("banana2", map[string]any{"prompt": "a cat", "aspect_ratio": "21:9", "resolution": "4k"}))
		assert.Equal(t, globalstoreBaseURL+"/v2/model-center/tasks", request["url"])
		assert.JSONEq(t, `{"model":"banana2","prompt":"a cat","aspect_ratio":"21:9","resolution":"4k"}`, globalstoreJSON(t, request["body"]))

		sized := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("banana2", map[string]any{"prompt": "a cat", "size": "1024x1024"}))
		assert.JSONEq(t, `{"model":"banana2","prompt":"a cat","size":"1024x1024"}`, globalstoreJSON(t, sized["body"]))

		quality := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("gpt-image-2.5", map[string]any{"prompt": "a cat"}))
		assert.JSONEq(t, `{"model":"gpt-image-2.5","prompt":"a cat","aspect_ratio":"1:1","resolution":"1K","quality":"low"}`, globalstoreJSON(t, quality["body"]))

		// gpt-image-2 在本 fork 是内置模型，插件对外用 globalstore/ 前缀，上游按原名下发。
		renamed := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("globalstore/gpt-image-2", map[string]any{"prompt": "a cat"}))
		assert.JSONEq(t, `{"model":"gpt-image-2","prompt":"a cat","aspect_ratio":"1:1","resolution":"1K","quality":"medium"}`, globalstoreJSON(t, renamed["body"]))
	})

	t.Run("uses the documented per model default ratio", func(t *testing.T) {
		request := globalstoreObject(t, plugin, "buildSubmitRequest", globalstoreSubmitContext("banana_c1", map[string]any{"prompt": "a cat"}))
		assert.JSONEq(t, `{"model":"banana_c1","prompt":"a cat","aspect_ratio":"21:9","resolution":"1K","quality":"standard"}`, globalstoreJSON(t, request["body"]))
	})

	t.Run("rejects unsupported Responses fields", func(t *testing.T) {
		for name, value := range map[string]map[string]any{
			"tools":             {"input": "a cat", "tools": []any{}},
			"unsupported field": {"input": "a cat", "seed": 1},
			"multiple messages": {"input": []any{map[string]any{"role": "user", "content": "a"}, map[string]any{"role": "user", "content": "b"}}},
			"system role":       {"input": []any{map[string]any{"role": "system", "content": "a"}}},
			"audio content":     {"input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_audio", "input_audio": "x"}}}}},
			"prompt conflict":   {"input": "a cat", "prompt": "a dog"},
		} {
			t.Run(name, func(t *testing.T) {
				_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{
					"model": "banana2", "upstreamModel": "banana2",
					"body": map[string]any{"kind": "json", "value": value},
				})
				require.Error(t, err)
			})
		}
	})

	t.Run("rejects image parameters the model does not support", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"n greater than one": {"prompt": "a cat", "n": 2},
			"response format":    {"prompt": "a cat", "response_format": "b64_json"},
			"unsupported size":   {"prompt": "a cat", "size": "1280x720"},
			"unsupported ratio":  {"prompt": "a cat", "ratio": "5:7"},
			"first frame":        {"prompt": "a cat", "first_image": "https://cdn.example/a.png"},
			"reference video":    {"prompt": "a cat", "reference_videos": []any{"https://cdn.example/v.mp4"}},
			"unknown quality":    {"prompt": "a cat", "quality": "ultra"},
		} {
			model := "banana2"
			if name == "unsupported size" || name == "unknown quality" {
				model = "banana_c1"
			}
			t.Run(name, func(t *testing.T) {
				_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", globalstoreSubmitContext(model, body))
				require.Error(t, err)
			})
		}
	})

	t.Run("renders only the host artifact link", func(t *testing.T) {
		value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "renderFinal"}, map[string]any{
			"requestBody": map[string]any{"model": "banana2"},
			"artifacts": map[string]any{"image": map[string]any{
				"key": "image", "type": "image", "mimeType": "image/png",
				"url": "https://gateway.example/v1/tasks/t/image/content?a=1&b=2",
			}},
		})
		require.NoError(t, err)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var rendered map[string]any
		require.NoError(t, common.Unmarshal(encoded, &rendered))
		output := rendered["output"].([]any)
		require.Len(t, output, 1)
		content := output[0].(map[string]any)["content"].([]any)
		text := content[0].(map[string]any)["text"].(string)
		assert.Contains(t, text, "gateway.example")
		assert.Contains(t, text, "&amp;", "HTML 属性必须转义")
		assert.Equal(t, map[string]any{"vendor": "globalstore"}, rendered["metadata"])

		_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "renderFinal"}, map[string]any{
			"requestBody": map[string]any{"model": "banana2"},
		})
		require.ErrorContains(t, err, "artifact is unavailable")
	})
}

func TestGlobalstoreArtifactsAndUsage(t *testing.T) {
	plugin := loadGlobalstorePlugin(t)

	t.Run("lists one artifact for a completed task", func(t *testing.T) {
		video := globalstoreValue(t, plugin, "listArtifacts", map[string]any{
			"status": "SUCCESS", "state": map[string]any{"kind": "video"},
			"data": map[string]any{"status": "completed", "video_url": "https://cdn.example/v.mp4"},
		})
		assert.JSONEq(t, `[{"key":"video","type":"video"}]`, globalstoreJSON(t, video))

		image := globalstoreValue(t, plugin, "listArtifacts", map[string]any{
			"status": "SUCCESS", "state": map[string]any{"kind": "image"},
			"data": map[string]any{"status": "completed", "image_url": "https://cdn.example/i.png"},
		})
		assert.JSONEq(t, `[{"key":"image","type":"image"}]`, globalstoreJSON(t, image))

		for name, ctx := range map[string]map[string]any{
			"in progress": {"status": "IN_PROGRESS", "data": map[string]any{"video_url": "https://cdn.example/v.mp4"}},
			"failed":      {"status": "FAILURE", "data": map[string]any{"video_url": "https://cdn.example/v.mp4"}},
			"no url":      {"status": "SUCCESS", "state": map[string]any{"kind": "video"}, "data": map[string]any{"status": "completed"}},
		} {
			t.Run(name, func(t *testing.T) {
				value := globalstoreValue(t, plugin, "listArtifacts", ctx)
				assert.JSONEq(t, `[]`, globalstoreJSON(t, value))
			})
		}
	})

	t.Run("builds a credentialless content request", func(t *testing.T) {
		ctx := map[string]any{
			"artifactKey": "video", "state": map[string]any{"kind": "video"},
			"data":          map[string]any{"video_url": "https://cdn.example/v.mp4?signature=upstream"},
			"clientRequest": map[string]any{"method": "GET"}, "apiKey": "fixture-only",
		}
		request := globalstoreObject(t, plugin, "buildContentRequest", ctx)
		assert.Equal(t, map[string]any{"url": "https://cdn.example/v.mp4?signature=upstream", "method": "GET", "credentialless": true}, request)
		ctx["artifactKey"] = "image"
		_, err := plugin.Engine.Call(t.Context(), "buildContentRequest", ctx)
		require.ErrorContains(t, err, "artifact_not_found")
	})

	t.Run("reports the declared usage facts", func(t *testing.T) {
		facts := globalstoreObject(t, plugin, "extractUsage", globalstoreSubmitContext("sd_2.5_special", map[string]any{
			"prompt": "flower", "seconds": 10, "resolution": "1080p",
			"first_image": "https://cdn.example/a.png", "reference_images": []any{"https://cdn.example/b.png"},
			"reference_videos": []any{"https://cdn.example/v.mp4"}, "generate_audio": false,
		}))
		keys := make([]string, 0, len(facts))
		for key := range facts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		assert.Equal(t, globalstoreUsageKeys, keys, "计费事实必须与描述符声明的字段一致")
		assert.Equal(t, float64(10), facts["seconds"])
		assert.Equal(t, float64(0), facts["images"])
		assert.Equal(t, float64(2), facts["input_images"])
		assert.Equal(t, float64(1), facts["input_videos"])
		assert.Equal(t, "1080p", facts["resolution"])
		assert.Equal(t, "disabled", facts["audio_mode"])

		imageFacts := globalstoreObject(t, plugin, "extractUsage", globalstoreSubmitContext("banana2", map[string]any{"prompt": "a cat", "reference_images": []any{"https://cdn.example/a.png"}}))
		assert.Equal(t, float64(0), imageFacts["seconds"])
		assert.Equal(t, float64(1), imageFacts["images"])
		assert.Equal(t, float64(1), imageFacts["input_images"])
		assert.Equal(t, "1k", imageFacts["resolution"])
		assert.Equal(t, "unspecified", imageFacts["quality"])
		assert.Equal(t, "unspecified", imageFacts["audio_mode"])

		ctx := globalstoreSubmitContext("banana2", map[string]any{"prompt": "a cat"})
		ctx["usagePurpose"] = "billing_ratios"
		ratios := globalstoreObject(t, plugin, "extractUsage", ctx)
		assert.Empty(t, ratios, "倍率估算阶段不重复计费")

		pixel := globalstoreObject(t, plugin, "extractUsage", globalstoreSubmitContext("banana2", map[string]any{"prompt": "a cat", "size": "1024x1024"}))
		assert.Equal(t, "unspecified", pixel["resolution"], "像素尺寸无法可靠映射时归为 unspecified")
	})

	t.Run("only settles completed tasks", func(t *testing.T) {
		completed := globalstoreObject(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS", "state": map[string]any{"kind": "video"}, "url": "https://cdn.example/v.mp4"}, map[string]any{"status": "completed", "object": "video", "video_url": "https://cdn.example/v.mp4"})
		assert.Equal(t, map[string]any{"count": float64(1)}, completed)

		image := globalstoreObject(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS", "state": map[string]any{"kind": "image"}, "url": "https://cdn.example/i.png"}, map[string]any{"status": "completed", "object": "image", "image_url": "https://cdn.example/i.png"})
		assert.Equal(t, map[string]any{"count": float64(1), "images": float64(1)}, image)

		fromBody := globalstoreObject(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{"status": "SUCCESS"}, map[string]any{"status": "completed", "object": "image", "image_url": "https://cdn.example/i.png"})
		assert.Equal(t, map[string]any{"count": float64(1), "images": float64(1)}, fromBody, "缺少持久化状态时退回上游响应体判断媒体类型")

		for name, call := range map[string]struct {
			result map[string]any
			body   map[string]any
		}{
			"in progress": {map[string]any{"status": "IN_PROGRESS"}, map[string]any{"status": "processing"}},
			"failed":      {map[string]any{"status": "FAILURE"}, map[string]any{"status": "failed"}},
		} {
			t.Run(name, func(t *testing.T) {
				value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, call.result, call.body)
				require.NoError(t, err)
				assert.Nil(t, value)
			})
		}
	})
}
