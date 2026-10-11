package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetasoH3VideoContracts 使用真实任务插件宿主验证秘塔合同，不访问上游或使用凭据。
func TestMetasoH3VideoContracts(t *testing.T) {
	source, err := plugins.Source("metaso-h3-video")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)

	assert.Equal(t, "https://metaso.cn/api/openai", plugin.Meta.BaseURL)
	assert.Equal(t, []string{"Minimax-H3"}, plugin.Meta.Models)
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)
	binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", "Minimax-H3")
	require.True(t, found)
	assert.Same(t, plugin, binding.Plugin)
	_, found = registry.Generation().LookupEndpoint("POST", "/v1/videos", "MiniMax-H3")
	assert.False(t, found, "模型 ID 大小写不同，不应自动变成别名")
	_, found = registry.Generation().LookupEndpoint("POST", "/v1/responses", "Minimax-H3")
	assert.False(t, found, "秘塔文档只确认视频协议")

	textBody := map[string]any{"model": "Minimax-H3", "prompt": "A sunflower swaying gently", "seconds": 5, "size": "1280x720"}
	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "body": map[string]any{"kind": "json", "value": textBody},
	})
	require.NoError(t, err)
	decodedBytes, err := common.Marshal(decoded)
	require.NoError(t, err)
	var command map[string]any
	require.NoError(t, common.Unmarshal(decodedBytes, &command))
	assert.Equal(t, "text_to_video", command["action"])

	request := metasoHookObject(t, plugin, "buildSubmitRequest", map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "baseUrl": "https://metaso.cn/api/openai", "apiKey": "fixture-only", "requestBody": textBody,
	})
	assert.Equal(t, "https://metaso.cn/api/openai/v1/videos", request["url"])
	assert.Equal(t, true, request["noRetry"])
	assert.Equal(t, map[string]any{"model": "Minimax-H3", "prompt": "A sunflower swaying gently", "seconds": float64(5), "size": "1280x720"}, request["body"])

	imageBody := map[string]any{
		"model": "Minimax-H3", "prompt": "Animate this flower", "seconds": 6, "size": "720x1280",
		"input_reference": map[string]any{"image_url": "https://cdn.example/flower.png"},
	}
	imageRequest := metasoHookObject(t, plugin, "buildSubmitRequest", map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "baseUrl": "https://metaso.cn/api/openai", "apiKey": "fixture-only", "requestBody": imageBody,
	})
	assert.Equal(t, "image_to_video", commandAction(t, plugin, imageBody))
	assert.Equal(t, map[string]any{"model": "Minimax-H3", "prompt": "Animate this flower", "seconds": float64(6), "size": "720x1280", "input_reference": map[string]any{"image_url": "https://cdn.example/flower.png"}}, imageRequest["body"])

	metadataBody := map[string]any{
		"model": "Minimax-H3", "prompt": "", "seconds": 5, "size": "1792x1024",
		"metadata": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "Animate the supplied image"},
			map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/reference.png"}},
		}},
	}
	metadataRequest := metasoHookObject(t, plugin, "buildSubmitRequest", map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "baseUrl": "https://metaso.cn/api/openai", "apiKey": "fixture-only", "requestBody": metadataBody,
	})
	assert.Equal(t, map[string]any{"model": "Minimax-H3", "prompt": "Animate the supplied image", "seconds": float64(5), "size": "1792x1024", "input_reference": map[string]any{"image_url": "https://cdn.example/reference.png"}}, metadataRequest["body"])

	usage := metasoHookObject(t, plugin, "extractUsage", map[string]any{"upstreamModel": "Minimax-H3", "requestBody": imageBody})
	assert.Equal(t, map[string]any{"seconds": float64(6), "size": "720x1280", "input_images": float64(1), "input_videos": float64(0), "input_audios": float64(0)}, usage)
	assert.Empty(t, metasoHookObject(t, plugin, "extractUsage", map[string]any{"upstreamModel": "Minimax-H3", "usagePurpose": "billing_ratios", "requestBody": imageBody}))

	multimodalBody := map[string]any{
		"model": "Minimax-H3", "prompt": "Animate all supplied references", "seconds": 8, "size": "1792x1024", "mode": "omni_reference",
		"images": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"},
		"videos": []any{"https://cdn.example/reference.mp4"}, "audios": []any{"https://cdn.example/reference.mp3"},
	}
	multimodalRequest := metasoHookObject(t, plugin, "buildSubmitRequest", map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "baseUrl": "https://metaso.cn/api/openai", "apiKey": "fixture-only", "requestBody": multimodalBody,
	})
	assert.Equal(t, map[string]any{
		"model": "Minimax-H3", "prompt": "Animate all supplied references", "seconds": float64(8), "size": "1792x1024", "mode": "omni_reference",
		"images": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"}, "videos": []any{"https://cdn.example/reference.mp4"}, "audios": []any{"https://cdn.example/reference.mp3"},
	}, multimodalRequest["body"])
	multimodalUsage := metasoHookObject(t, plugin, "extractUsage", map[string]any{"upstreamModel": "Minimax-H3", "requestBody": multimodalBody})
	assert.Equal(t, map[string]any{"seconds": float64(8), "size": "1792x1024", "input_images": float64(2), "input_videos": float64(1), "input_audios": float64(1)}, multimodalUsage)

	query := metasoHookObject(t, plugin, "buildQueryRequest", map[string]any{"baseUrl": "https://metaso.cn/api/openai", "apiKey": "fixture-only", "taskId": "task/id?x"})
	assert.Equal(t, "https://metaso.cn/api/openai/v1/videos/task%2Fid%3Fx", query["url"])
	content := metasoHookObject(t, plugin, "buildContentRequest", map[string]any{"baseUrl": "https://metaso.cn/api/openai", "apiKey": "fixture-only", "taskId": "task/id?x", "artifactKey": "video", "clientRequest": map[string]any{"method": "GET"}})
	assert.Equal(t, "https://metaso.cn/api/openai/v1/videos/task%2Fid%3Fx/content", content["url"])
	assert.Equal(t, "GET", content["method"])

	for name, body := range map[string]map[string]any{
		"unsupported size": {"prompt": "flower", "seconds": 5, "size": "480p"},
		"zero seconds":     {"prompt": "flower", "seconds": 0},
		"private image":    {"prompt": "flower", "seconds": 5, "input_reference": "data:image/png;base64,abc"},
		"too many images":  {"prompt": "flower", "seconds": 5, "images": make([]any, 10)},
		"too many videos":  {"prompt": "flower", "seconds": 5, "videos": make([]any, 4)},
		"too many audios":  {"prompt": "flower", "seconds": 5, "audios": make([]any, 4)},
	} {
		t.Run(name, func(t *testing.T) {
			_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"upstreamModel": "Minimax-H3", "requestBody": body, "baseUrl": "https://metaso.cn/api/openai"})
			require.Error(t, callErr)
		})
	}
	boundaryBody := map[string]any{
		"prompt": "Animate all supplied references", "seconds": 5,
		"images": make([]any, 9), "videos": make([]any, 3), "audios": make([]any, 3),
	}
	for _, field := range []string{"images", "videos", "audios"} {
		items := boundaryBody[field].([]any)
		for index := range items {
			items[index] = "https://cdn.example/" + field + "/" + string(rune('a'+index))
		}
	}
	boundaryRequest := metasoHookObject(t, plugin, "buildSubmitRequest", map[string]any{
		"upstreamModel": "Minimax-H3", "baseUrl": "https://metaso.cn/api/openai", "apiKey": "fixture-only", "requestBody": boundaryBody,
	})
	assert.Equal(t, float64(9), float64(len(boundaryRequest["body"].(map[string]any)["images"].([]any))))
	assert.Equal(t, float64(3), float64(len(boundaryRequest["body"].(map[string]any)["videos"].([]any))))
	assert.Equal(t, float64(3), float64(len(boundaryRequest["body"].(map[string]any)["audios"].([]any))))
}

// metasoHookObject 将插件 Hook 返回值转换为便于断言的 JSON 对象。
func metasoHookObject(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

// commandAction 解码指定请求并返回插件选择的生成动作。
func commandAction(t *testing.T, plugin *jsplugin.LoadedPlugin, body map[string]any) string {
	t.Helper()
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "Minimax-H3", "upstreamModel": "Minimax-H3", "body": map[string]any{"kind": "json", "value": body},
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result["action"].(string)
}
