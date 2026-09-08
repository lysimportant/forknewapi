package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKlingResponsesProtocol 验证Responses模式解码、用量声明与输出保持兼容。
func TestKlingResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "kling",
		model:     "kling-v2-master",
		requestBody: map[string]any{
			"model": "kling-v2-master",
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "camera orbit"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/frame.png"},
			}}},
			"seconds": 10,
			"metadata": map[string]any{
				"mode": "pro",
			},
		},
		wantAction: "image_to_video",
		wantRequest: map[string]any{
			"model":    "kling-v2-master",
			"prompt":   "camera orbit",
			"image":    "https://cdn.example/frame.png",
			"duration": float64(10),
			"metadata": map[string]any{"mode": "pro"},
		},
		wantUsageKeys:  []string{"units", "resolution"},
		wantVendorName: "kling",
	})
}

// TestKlingResolutionUsage 验证真实插件引擎按请求模式提供清晰度，原单位费率与完成覆盖契约保持不变。
func TestKlingResolutionUsage(t *testing.T) {
	source, err := builtinplugins.Source("kling")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "kling"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name       string
		model      string
		request    map[string]any
		resolution string
		units      float64
	}{
		{"默认std", "kling-v1", map[string]any{}, "720P", 1},
		{"请求pro", "kling-v1", map[string]any{"mode": "pro"}, "1080P", 3.5},
		{"元数据std", "kling-v1-6", map[string]any{"metadata": map[string]any{"mode": "std"}}, "720P", 2},
		{"元数据pro十秒", "kling-v1-6", map[string]any{"metadata": map[string]any{"mode": "pro", "duration": 10}}, "1080P", 7},
		{"master默认pro", "kling-v2-master", map[string]any{}, "1080P", 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"upstreamModel": tc.model, "requestBody": tc.request})
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			var facts map[string]any
			require.NoError(t, common.Unmarshal(encoded, &facts))
			assert.Equal(t, map[string]any{"units": tc.units, "resolution": tc.resolution}, facts)
			ratio, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"usagePurpose": "billing_ratios", "upstreamModel": tc.model, "requestBody": tc.request})
			require.NoError(t, err)
			assert.Nil(t, ratio)
		})
	}
	_, err = plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"upstreamModel": "kling-v2-master", "requestBody": map[string]any{"mode": "std"}})
	require.ErrorContains(t, err, "does not support mode std")
	value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{}, map[string]any{"data": map[string]any{"final_unit_deduction": "6.25"}})
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var completion map[string]any
	require.NoError(t, common.Unmarshal(encoded, &completion))
	assert.Equal(t, map[string]any{"units": 6.25}, completion, "完成用量不得用推测的清晰度覆盖预扣事实")
}
