package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShumaiH3VideoContract 验证 SUB 统一入口的 480p 文生视频合同，不调用真实上游。
func TestShumaiH3VideoContract(t *testing.T) {
	source, err := builtinplugins.Source("shumai-h3-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "shumai-h3-video"})
	require.NoError(t, err)

	assert.Equal(t, "https://shumai.siphot.com", plugin.Meta.BaseURL)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, []string{"h3"}, plugin.Meta.Models)

	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model":         "h3",
		"upstreamModel": "h3",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "h3", "mode": "text", "prompt": "A sunflower swaying gently in a field under warm sunlight", "duration": 5, "resolution": "480p",
		}},
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(decoded)
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.Unmarshal(encoded, &intent))
	assert.Equal(t, "submit", intent["kind"])
	assert.Equal(t, "text_to_video", intent["action"])

	requestBody := intent["requestBody"].(map[string]any)
	descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl": "https://shumai.siphot.com", "apiKey": "fixture-only", "upstreamModel": "h3", "requestBody": requestBody,
	})
	require.NoError(t, err)
	encoded, err = common.Marshal(descriptor)
	require.NoError(t, err)
	assert.JSONEq(t, `{"url":"https://shumai.siphot.com/v1/videos/generations","method":"POST","headers":{"Authorization":"Bearer fixture-only","Content-Type":"application/json","Accept":"application/json"},"body":{"model":"h3","mode":"text","duration":5,"resolution":"480p竖","prompt":"A sunflower swaying gently in a field under warm sunlight"},"noRetry":true}`, string(encoded))

	facts, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"upstreamModel": "h3", "requestBody": requestBody, "usagePurpose": "facts"})
	require.NoError(t, err)
	encoded, err = common.Marshal(facts)
	require.NoError(t, err)
	assert.JSONEq(t, `{"seconds":5,"resolution":"480p","mode":"text","input_images":0,"input_audios":0}`, string(encoded))
}

// TestShumaiH3VideoModesAndBounds 验证媒体模式、分辨率边界和任务状态映射。
func TestShumaiH3VideoModesAndBounds(t *testing.T) {
	source, err := builtinplugins.Source("shumai-h3-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "shumai-h3-video"})
	require.NoError(t, err)

	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"baseUrl": "https://shumai.siphot.com", "upstreamModel": "h3", "requestBody": map[string]any{
		"mode": "reference", "prompt": "sunflower", "duration": 10, "resolution": "1080p", "images": []any{"https://example.com/flower.png"},
	}})
	require.NoError(t, err)
	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"baseUrl": "https://shumai.siphot.com", "upstreamModel": "h3", "requestBody": map[string]any{
		"mode": "text", "prompt": "sunflower", "duration": 16, "resolution": "480p",
	}})
	require.Error(t, err)
	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"baseUrl": "https://shumai.siphot.com", "upstreamModel": "h3", "requestBody": map[string]any{
		"mode": "text", "prompt": "sunflower", "duration": 5, "resolution": "1080p",
	}})
	require.Error(t, err)

	for _, test := range []struct {
		name string
		body map[string]any
		want string
	}{
		{
			name: "reference images are billable",
			body: map[string]any{
				"mode": "reference", "prompt": "sunflower", "duration": 10, "resolution": "1080p",
				"images": []any{"https://example.com/a.png", "https://example.com/b.png", "https://example.com/c.png"},
			},
			want: `{"seconds":10,"resolution":"1080p","mode":"reference","input_images":3,"input_audios":0}`,
		},
		{
			name: "image-audio counts both media lists",
			body: map[string]any{
				"mode": "image-audio", "prompt": "sunflower", "duration": 10, "resolution": "1080p",
				"images": []any{"https://example.com/a.png", "https://example.com/b.png"},
				"audios": []any{"https://example.com/a.mp3", "https://example.com/b.mp3"},
			},
			want: `{"seconds":10,"resolution":"1080p","mode":"image-audio","input_images":2,"input_audios":2}`,
		},
		{
			name: "first and last frames are not references",
			body: map[string]any{
				"mode": "frames", "prompt": "sunflower", "duration": 10, "resolution": "768p",
				"images": []any{"https://example.com/first.png", "https://example.com/last.png"},
			},
			want: `{"seconds":10,"resolution":"768p","mode":"frames","input_images":0,"input_audios":0}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"upstreamModel": "h3", "requestBody": test.body, "usagePurpose": "facts"})
			require.NoError(t, callErr)
			encoded, marshalErr := common.Marshal(facts)
			require.NoError(t, marshalErr)
			assert.JSONEq(t, test.want, string(encoded))
		})
	}

	for _, test := range []struct {
		status string
		want   string
	}{
		{status: "queued", want: "QUEUED"},
		{status: "in_progress", want: "IN_PROGRESS"},
		{status: "completed", want: "SUCCESS"},
		{status: "failed", want: "FAILURE"},
	} {
		t.Run(test.status, func(t *testing.T) {
			value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", nil, map[string]any{"status": test.status})
			require.NoError(t, callErr)
			encoded, marshalErr := common.Marshal(value)
			require.NoError(t, marshalErr)
			var result map[string]any
			require.NoError(t, common.Unmarshal(encoded, &result))
			assert.Equal(t, test.want, result["status"])
		})
	}
}
