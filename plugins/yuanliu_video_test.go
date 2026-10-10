package plugins_test

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadYuanliuPlugin 在真实 Sobek 宿主注册源流插件，不访问上游或使用真实凭据。
func loadYuanliuPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := plugins.Source("yuanliu")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	return plugin
}

// yuanliuHookObject 读取真实引擎的 JSON 对象返回值，供协议和用量断言共用。
func yuanliuHookObject(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	result, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, common.Unmarshal(encoded, &value))
	return value
}

// TestYuanliuModelContracts 核对已适配模型、别名路由、计费单位及目录快照边界。
func TestYuanliuModelContracts(t *testing.T) {
	plugin := loadYuanliuPlugin(t)
	assert.Equal(t, "1.1.0", plugin.Meta.Version)
	assert.Equal(t, "https://test.yuanliuai.tsyzai.com/openapi/v1", plugin.Meta.BaseURL)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/openapi/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Contains(t, plugin.Meta.RequiredCapabilities, "task-submit-no-retry@1")
	assert.Equal(t, []jsplugin.ProtocolClaim{{Name: "openai_video"}}, plugin.Meta.Protocols)

	models := []struct {
		id, alias         string
		min, max, images  int
		videos, audios    int
		prompt            int
		perSecond         bool
		defaultResolution string
		adaptive          string
	}{
		{"seedance-2.5-guanfang-anmiao", "Yuan-Seedance-2.5-Official", 4, 30, 30, 0, 10, 16000, true, "480p", "adaptive"},
		{"yl_g7zy_seedance_v2_0_std", "Yuan-Seedance-2.0-LJ", 4, 15, 9, 0, 0, 16000, false, "720p", "auto"},
		{"yl_g7zy_seedance_v2_0_std_full", "Yuan-Seedance-2.0-LJ-Full", 5, 15, 9, 3, 3, 16000, false, "720p", "auto"},
		{"yl_g7zy_seedance_v2_5", "Yuan-Seedance-2.5-LJ", 4, 30, 30, 0, 0, 16000, false, "720p", "auto"},
		{"yl_g7zy_seedance_v2_5_full", "Yuan-Seedance-2.5-LJ-Full", 5, 30, 30, 10, 10, 16000, true, "720p", "auto"},
		{"yl_seedance-2-0_ba0687ff09f2", "Yuan-Seedance-2.0-HD", 5, 15, 9, 0, 0, 10000, false, "720p", "adaptive"},
		{"yl_seedance-2-5_6caffaca7390", "Yuan-Seedance-2.5-HD", 4, 30, 30, 0, 0, 16000, false, "720p", "adaptive"},
		{"yl_seedance-2-5_0fab2f1b1f10", "Yuan-Seedance-2.5-HD-Full", 10, 30, 30, 10, 10, 16000, false, "720p", "adaptive"},
		{"yl_seedance-2-5_750271498003", "Yuan-Seedance-2.5-HD-PerSecond", 10, 30, 30, 10, 10, 16000, true, "720p", "adaptive"},
		{"yl_api_hmstudio_seedance_v2_5_101010_7d58bbb217e6", "Yuan-Seedance-2.5-YS-Full", 4, 30, 10, 10, 10, 16000, false, "720p", ""},
		{"yl_api_hmstudio_seedance_v2_5_dc729300ff39", "Yuan-Seedance-2.5-YS", 4, 30, 10, 0, 0, 16000, false, "720p", ""},
		{"yl_video-30_76dbb7993f8e", "Yuan-Seedance-2.5-YL1", 30, 30, 9, 0, 0, 8000, false, "720p", ""},
		{"yl_api_hmstudio_seedance_v2_0_514a65db713b", "Yuan-Seedance-2.0-YS", 4, 15, 9, 0, 0, 16000, false, "720p", ""},
		{"yl_lwaigc_mf_sd2_5_v2", "Yuan-Seedance-2.5-LW", 4, 30, 30, 0, 10, 16000, true, "720p", ""},
	}
	wantIDs := make([]string, 0, len(models))
	for _, model := range models {
		wantIDs = append(wantIDs, model.id)
		t.Run(model.alias, func(t *testing.T) {
			registry := jsplugin.NewRegistry()
			source, err := plugins.Source("yuanliu")
			require.NoError(t, err)
			registered, err := registry.RegisterFactory(source, jsplugin.Options{})
			require.NoError(t, err)
			assert.Equal(t, model.id, registered.Meta.ModelAliases[model.alias])
			binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", model.id)
			require.True(t, found, model.id)
			assert.Same(t, registered, binding.Plugin)
			_, globallyBoundAlias := registry.Generation().LookupEndpoint("POST", "/v1/videos", model.alias)
			assert.False(t, globallyBoundAlias, "展示别名仅通过渠道 model_mapping 进入插件")
			_, responses := registry.Generation().LookupEndpoint("POST", "/v1/responses", model.id)
			assert.False(t, responses)

			body := map[string]any{"model": model.alias, "prompt": "ocean", "seconds": model.min}
			decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": model.alias, "upstreamModel": model.id,
				"body": map[string]any{"kind": "json", "value": body},
			})
			require.NoError(t, err)
			encoded, err := common.Marshal(decoded)
			require.NoError(t, err)
			var command struct {
				Model       string         `json:"model"`
				Action      string         `json:"action"`
				RequestBody map[string]any `json:"requestBody"`
			}
			require.NoError(t, common.Unmarshal(encoded, &command))
			assert.Equal(t, model.alias, command.Model)
			assert.Equal(t, "text_to_video", command.Action)
			ctx := map[string]any{
				"model": model.alias, "upstreamModel": model.id, "publicTaskId": "gateway-task",
				"baseUrl": "https://yuanliu.example/openapi/v1/", "apiKey": "fixture-only",
				"requestBody": command.RequestBody,
			}
			request := yuanliuHookObject(t, plugin, "buildSubmitRequest", ctx)
			assert.Equal(t, "https://yuanliu.example/openapi/v1/videos", request["url"])
			assert.Equal(t, true, request["noRetry"])
			payload := request["body"].(map[string]any)
			assert.Equal(t, model.id, payload["model"])
			assert.Equal(t, "gateway-task", payload["client_request_id"])
			assert.Equal(t, float64(model.min), payload["duration"])
			assert.Equal(t, model.defaultResolution, payload["resolution"])
			assert.NotContains(t, payload, "seconds")
			usage := yuanliuHookObject(t, plugin, "extractUsage", ctx)
			if model.perSecond {
				assert.Equal(t, map[string]any{"seconds": float64(model.min), "resolution": model.defaultResolution}, usage)
			} else {
				assert.Equal(t, map[string]any{"count": float64(1)}, usage)
			}
			ctx["usagePurpose"] = "billing_ratios"
			assert.Empty(t, yuanliuHookObject(t, plugin, "extractUsage", ctx))
			ctx["usagePurpose"] = "facts"
			completion := yuanliuHookObject(t, plugin, "extractUsageOnComplete", map[string]any{
				"upstreamModel": model.id, "state": map[string]any{"duration": model.min, "resolution": model.defaultResolution},
			}, map[string]any{"status": "SUCCESS"}, map[string]any{"seconds": model.max, "billing": map[string]any{"charged": 9876}})
			if model.perSecond {
				assert.Equal(t, map[string]any{"seconds": float64(model.max), "resolution": model.defaultResolution}, completion)
			} else {
				assert.Equal(t, map[string]any{"count": float64(1)}, completion)
			}

			for _, duration := range []int{model.min - 1, model.max + 1} {
				invalid := map[string]any{"model": model.alias, "prompt": "ocean", "duration": duration}
				_, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"upstreamModel": model.id, "requestBody": invalid})
				require.ErrorContains(t, err, "duration")
			}
			refs := make([]string, model.images)
			for i := range refs {
				refs[i] = "https://cdn.example/image.png"
			}
			valid := map[string]any{"prompt": strings.Repeat("a", model.prompt), "duration": model.min, "images": refs}
			assert.NotEmpty(t, yuanliuHookObject(t, plugin, "extractUsage", map[string]any{"upstreamModel": model.id, "requestBody": valid}))
			for _, media := range []struct {
				field string
				limit int
			}{{"videos", model.videos}, {"audios", model.audios}} {
				mediaURLs := make([]string, media.limit)
				for i := range mediaURLs {
					mediaURLs[i] = "https://cdn.example/reference"
				}
				request := map[string]any{"prompt": "ocean", "duration": model.min, media.field: mediaURLs}
				if model.id == "yl_lwaigc_mf_sd2_5_v2" && media.field == "audios" {
					request["images"] = []string{"https://cdn.example/image.png"}
				}
				assert.NotEmpty(t, yuanliuHookObject(t, plugin, "extractUsage", map[string]any{"upstreamModel": model.id, "requestBody": request}))
				request[media.field] = append(mediaURLs, "https://cdn.example/extra")
				_, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"upstreamModel": model.id, "requestBody": request})
				require.ErrorContains(t, err, "too many reference "+media.field)
			}
			for _, ratio := range []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9"} {
				ctx := map[string]any{"upstreamModel": model.id, "requestBody": map[string]any{"prompt": "ocean", "duration": model.min, "aspect_ratio": ratio}}
				if model.id == "yl_lwaigc_mf_sd2_5_v2" && ratio != "16:9" && ratio != "9:16" && ratio != "1:1" {
					_, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
					require.ErrorContains(t, err, "aspect_ratio")
				} else {
					assert.NotEmpty(t, yuanliuHookObject(t, plugin, "extractUsage", ctx))
				}
			}
			if model.id == "seedance-2.5-guanfang-anmiao" {
				for _, resolution := range []string{"480p", "720p", "1080p"} {
					assert.NotEmpty(t, yuanliuHookObject(t, plugin, "extractUsage", map[string]any{"upstreamModel": model.id, "requestBody": map[string]any{"prompt": "ocean", "duration": model.min, "resolution": resolution}}))
				}
			}
			for _, invalid := range []map[string]any{
				{"prompt": "ocean", "duration": model.min, "images": append(refs, "https://cdn.example/extra.png")},
				{"prompt": strings.Repeat("a", model.prompt+1), "duration": model.min},
				{"prompt": "ocean", "duration": model.min, "resolution": "4k"},
			} {
				_, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"upstreamModel": model.id, "requestBody": invalid})
				require.Error(t, err)
			}
			if model.adaptive != "" {
				assert.NotEmpty(t, yuanliuHookObject(t, plugin, "extractUsage", map[string]any{"upstreamModel": model.id, "requestBody": map[string]any{"prompt": "ocean", "duration": model.min, "ratio": model.adaptive}}))
			}
		})
	}
	assert.ElementsMatch(t, wantIDs, plugin.Meta.Models)
}

// TestYuanliuReferenceContracts 验证 URL 顺序、Canvas 普通角色桥接和所有拒绝边界在计费前生效。
func TestYuanliuReferenceContracts(t *testing.T) {
	plugin := loadYuanliuPlugin(t)
	const model = "yl_g7zy_seedance_v2_0_std_full"
	const prompt = "@Image1 meets @Image4 with @Video1 and @Audio1"
	metadata := map[string]any{
		"resolution": "720p", "ratio": "auto", "omni_reference_task_type": "reference",
		"content": []any{
			map[string]any{"type": "text", "text": prompt},
			map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "https://cdn.example/audio.mp3"}, "role": "reference_audio"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/last.png"}, "role": "reference_image"},
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/video.mp4"}, "role": "reference_video"},
		},
	}
	body := map[string]any{
		"model": "Yuan-Seedance-2.0-LJ-Full", "prompt": prompt, "seconds": "5", "duration": 5,
		"images":          []string{"https://cdn.example/first.png", "https://cdn.example/first.png"},
		"input_reference": map[string]any{"image_url": "https://cdn.example/middle.png"}, "metadata": metadata,
	}
	decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "Yuan-Seedance-2.0-LJ-Full", "upstreamModel": model, "body": map[string]any{"kind": "json", "value": body},
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(decoded)
	require.NoError(t, err)
	var command map[string]any
	require.NoError(t, common.Unmarshal(encoded, &command))
	assert.Equal(t, "reference_to_video", command["action"])
	request := yuanliuHookObject(t, plugin, "buildSubmitRequest", map[string]any{
		"model": "Yuan-Seedance-2.0-LJ-Full", "upstreamModel": model, "publicTaskId": "stable-task",
		"baseUrl": "https://yuanliu.example/openapi/v1", "apiKey": "fixture-only", "requestBody": command["requestBody"],
	})
	payload := request["body"].(map[string]any)
	assert.Equal(t, []any{"https://cdn.example/first.png", "https://cdn.example/first.png", "https://cdn.example/middle.png", "https://cdn.example/last.png"}, payload["images"])
	assert.Equal(t, []any{"https://cdn.example/video.mp4"}, payload["videos"])
	assert.Equal(t, []any{"https://cdn.example/audio.mp3"}, payload["audios"])
	assert.Equal(t, prompt, payload["prompt"])
	assert.Equal(t, "auto", payload["aspect_ratio"])
	assert.NotContains(t, payload, "metadata")

	for _, reference := range []any{"https://cdn.example/input.png", map[string]any{"url": "https://cdn.example/input.png"}, map[string]any{"image_url": map[string]any{"url": "https://cdn.example/input.png"}}} {
		value := yuanliuHookObject(t, plugin, "buildSubmitRequest", map[string]any{
			"upstreamModel": model, "publicTaskId": "stable-task", "baseUrl": "https://yuanliu.example/openapi/v1", "apiKey": "fixture-only",
			"requestBody": map[string]any{"prompt": "ocean", "duration": 5, "input_reference": reference},
		})
		assert.Equal(t, []any{"https://cdn.example/input.png"}, value["body"].(map[string]any)["images"])
	}
	multipart, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": model,
		"body": map[string]any{"kind": "multipart", "fields": map[string]any{
			"model": []string{model}, "prompt": []string{"ocean"}, "seconds": []string{"5"},
			"images": []string{"https://cdn.example/one.png", "https://cdn.example/two.png"},
			"videos": []string{`["https://cdn.example/clip.mp4"]`},
		}},
	})
	require.NoError(t, err)
	encoded, err = common.Marshal(multipart)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &command))
	assert.Equal(t, []any{"https://cdn.example/one.png", "https://cdn.example/two.png"}, command["requestBody"].(map[string]any)["images"])
	assert.Equal(t, []any{"https://cdn.example/clip.mp4"}, command["requestBody"].(map[string]any)["videos"])

	invalid := []struct {
		name, model string
		fields      map[string]any
	}{
		{"缺少时长", model, map[string]any{"seconds": nil}},
		{"巨大时长", model, map[string]any{"seconds": "18446744073686646784"}},
		{"自动时长", model, map[string]any{"seconds": -1}},
		{"小数时长", model, map[string]any{"seconds": 5.5}},
		{"科学记数时长", model, map[string]any{"seconds": "5e0"}},
		{"布尔时长", model, map[string]any{"seconds": true}},
		{"时长冲突", model, map[string]any{"duration": 6}},
		{"离散时长", "yl_seedance-2-0_ba0687ff09f2", map[string]any{"seconds": 6}},
		{"固定时长", "yl_video-30_76dbb7993f8e", map[string]any{"seconds": 29}},
		{"分辨率冲突", model, map[string]any{"resolution": "720p", "size": "1080p"}},
		{"像素尺寸未知", model, map[string]any{"size": "1280x720"}},
		{"画幅冲突", model, map[string]any{"ratio": "auto", "aspect_ratio": "16:9"}},
		{"YS不支持自适应", "yl_api_hmstudio_seedance_v2_5_dc729300ff39", map[string]any{"ratio": "adaptive"}},
		{"普通模型不支持视频", "yl_g7zy_seedance_v2_0_std", map[string]any{"videos": []string{"https://cdn.example/video.mp4"}}},
		{"普通模型不支持音频", "yl_g7zy_seedance_v2_5", map[string]any{"audios": []string{"https://cdn.example/audio.mp3"}}},
		{"官方模型不支持视频", "seedance-2.5-guanfang-anmiao", map[string]any{"videos": []string{"https://cdn.example/video.mp4"}}},
		{"LW音频必须配图", "yl_lwaigc_mf_sd2_5_v2", map[string]any{"audios": []string{"https://cdn.example/audio.mp3"}}},
		{"LW嵌套音频必须配图", "yl_lwaigc_mf_sd2_5_v2", map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "https://cdn.example/audio.mp3"}, "role": "reference_audio"}}}}},
		{"LW固定720p", "yl_lwaigc_mf_sd2_5_v2", map[string]any{"resolution": "1080p"}},
		{"LW不支持自适应", "yl_lwaigc_mf_sd2_5_v2", map[string]any{"ratio": "adaptive"}},
		{"LW首帧字段不能降级", "yl_lwaigc_mf_sd2_5_v2", map[string]any{"image": "https://cdn.example/image.png"}},
		{"LW普通参考不能夹带帧角色", "yl_lwaigc_mf_sd2_5_v2", map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/image.png", "role": "first_frame"}}}},
		{"LW普通参考URL不能绕过", "yl_lwaigc_mf_sd2_5_v2", map[string]any{"reference_images": []any{map[string]any{"url": "data:image/png;base64,aW1hZ2U="}}}},
		{"未知模型", "seedance-unknown", map[string]any{}},
		{"未知顶层字段", model, map[string]any{"generate_audio": false}},
		{"嵌套计费绕过", model, map[string]any{"metadata": map[string]any{"duration": 99999999}}},
		{"重复prompt不同", model, map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "different"}}}}},
		{"首帧不能降级", model, map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/image.png"}, "role": "first_frame"}}}}},
		{"尾帧不能降级", model, map[string]any{"input_reference": map[string]any{"url": "https://cdn.example/image.png", "role": "last_frame"}}},
		{"视频角色不匹配", model, map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/video.mp4"}, "role": "reference_image"}}}}},
		{"未知内容类型", model, map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "draft_task"}}}}},
		{"编辑模式未适配", model, map[string]any{"mode": "video_edit"}},
		{"编辑metadata未适配", model, map[string]any{"metadata": map[string]any{"omni_reference_task_type": "edit"}}},
		{"空参考模式", model, map[string]any{"mode": "omni_reference"}},
		{"文本模式含素材", model, map[string]any{"mode": "text_to_video", "images": []string{"https://cdn.example/image.png"}}},
		{"参考编号越界", model, map[string]any{"prompt": "@Image1"}},
		{"参考编号零", model, map[string]any{"prompt": "@Image0", "images": []string{"https://cdn.example/image.png"}}},
		{"对象数组未确认", model, map[string]any{"images": []any{map[string]any{"url": "https://cdn.example/image.png"}}}},
		{"Base64未确认", model, map[string]any{"images": []string{"data:image/png;base64,aW1hZ2U="}}},
		{"URL内凭据", model, map[string]any{"images": []string{"https://user:secret@cdn.example/image.png"}}},
		{"URL反斜杠", model, map[string]any{"images": []string{"https://cdn.example\\evil.example/image.png"}}},
		{"URL换行", model, map[string]any{"images": []string{"https://cdn.example/image.png\n"}}},
		{"URL非空白控制字符", model, map[string]any{"images": []string{"https://cdn.example/image\x01.png"}}},
		{"URL删除控制字符", model, map[string]any{"images": []string{"https://cdn.example/image\x7f.png"}}},
		{"不接受外部幂等号", model, map[string]any{"client_request_id": "client-key"}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			body := map[string]any{"model": test.model, "prompt": "ocean", "seconds": 5}
			for key, value := range test.fields {
				body[key] = value
			}
			ctx := map[string]any{"upstreamModel": test.model, "requestBody": body, "publicTaskId": "stable-task", "baseUrl": "https://yuanliu.example/openapi/v1"}
			for _, hook := range []string{"buildSubmitRequest", "extractUsage"} {
				_, err := plugin.Engine.Call(t.Context(), hook, ctx)
				require.Error(t, err, hook)
			}
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": test.model, "body": map[string]any{"kind": "json", "value": body}})
			require.Error(t, err)
		})
	}
	for _, fields := range []map[string]any{
		{"prompt": []string{"one", "two"}, "seconds": []string{"5"}},
		{"prompt": []string{"ocean"}, "seconds": []string{"5"}, "metadata": []string{"{bad"}},
	} {
		_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": model, "body": map[string]any{"kind": "multipart", "fields": fields}})
		require.Error(t, err)
	}
	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": model, "body": map[string]any{"kind": "multipart", "files": []any{map[string]any{"ref": "request_file:input_reference"}}}})
	require.ErrorContains(t, err, "file uploads are unsupported")
}

// TestYuanliuCanvasGenericReferences 验证未登记新型号的画布通用请求，保留普通图音参考和显式重复顺序。
func TestYuanliuCanvasGenericReferences(t *testing.T) {
	plugin := loadYuanliuPlugin(t)
	const upstream = "yl_lwaigc_mf_sd2_5_v2"
	for _, model := range []string{upstream, "Yuan-Seedance-2.5-LW"} {
		t.Run(model, func(t *testing.T) {
			body := map[string]any{
				"model": model, "prompt": "Animate @Image2 with @Audio1", "duration": 4, "seconds": "4", "resolution": "720p", "aspect_ratio": "9:16",
				"reference_images": []any{
					map[string]any{"url": "https://cdn.example/second.png"},
					map[string]any{"url": "https://cdn.example/first.png"},
					map[string]any{"url": "https://cdn.example/first.png"},
				},
				"metadata": map[string]any{"content": []any{
					map[string]any{"type": "audio_url", "role": "audioTrack", "audio_url": map[string]any{"url": "https://cdn.example/audio.mp3"}},
					map[string]any{"type": "audio_url", "role": "content", "audio_url": map[string]any{"url": "https://cdn.example/audio.mp3"}},
				}},
			}
			decoded, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": model, "upstreamModel": upstream, "body": map[string]any{"kind": "json", "value": body},
			})
			require.NoError(t, err)
			encoded, err := common.Marshal(decoded)
			require.NoError(t, err)
			var command struct {
				Model       string         `json:"model"`
				Action      string         `json:"action"`
				RequestBody map[string]any `json:"requestBody"`
			}
			require.NoError(t, common.Unmarshal(encoded, &command))
			assert.Equal(t, model, command.Model)
			assert.Equal(t, "reference_to_video", command.Action)
			request := yuanliuHookObject(t, plugin, "buildSubmitRequest", map[string]any{
				"model": model, "upstreamModel": upstream, "publicTaskId": "generic-canvas-task", "requestBody": command.RequestBody,
				"baseUrl": "https://yuanliu.example/openapi/v1", "apiKey": "fixture-only",
			})
			payload := request["body"].(map[string]any)
			assert.Equal(t, upstream, payload["model"])
			assert.Equal(t, "Animate @Image2 with @Audio1", payload["prompt"])
			assert.Equal(t, []any{"https://cdn.example/second.png", "https://cdn.example/first.png", "https://cdn.example/first.png"}, payload["images"])
			assert.Equal(t, []any{"https://cdn.example/audio.mp3", "https://cdn.example/audio.mp3"}, payload["audios"])
			assert.NotContains(t, payload, "reference_images")
			assert.NotContains(t, payload, "metadata")
			assert.Equal(t, map[string]any{"seconds": float64(4), "resolution": "720p"}, yuanliuHookObject(t, plugin, "extractUsage", map[string]any{
				"upstreamModel": upstream, "requestBody": command.RequestBody,
			}))
		})
	}
}

// TestYuanliuTaskLifecycle 验证任务状态、幂等终态、冻结用量恢复和不泄露密钥的同渠道下载。
func TestYuanliuTaskLifecycle(t *testing.T) {
	plugin := loadYuanliuPlugin(t)
	ctx := map[string]any{"upstreamModel": "seedance-2.5-guanfang-anmiao", "apiKey": "fixture-only", "requestBody": map[string]any{"prompt": "ocean", "duration": 4, "resolution": "720p"}}
	for _, statusCode := range []int{200, 201} {
		parsed := yuanliuHookObject(t, plugin, "parseSubmitResponse", ctx, map[string]any{"statusCode": statusCode, "body": map[string]any{"id": "3", "task_id": "3", "status": "queued"}})
		assert.Equal(t, "3", parsed["taskId"])
		assert.Equal(t, map[string]any{"duration": float64(4), "resolution": "720p"}, parsed["state"])
	}
	for _, status := range []string{"completed", "failed"} {
		body := map[string]any{"id": "3", "status": status}
		if status == "failed" {
			body["error"] = map[string]any{"message": "content rejected"}
		}
		parsed := yuanliuHookObject(t, plugin, "parseSubmitResponse", ctx, map[string]any{"statusCode": 200, "body": body})
		expected := "SUCCESS"
		if status == "failed" {
			expected = "FAILURE"
		}
		assert.Equal(t, expected, parsed["immediate"].(map[string]any)["status"])
	}
	for _, response := range []map[string]any{
		{"statusCode": 401, "body": map[string]any{"error": map[string]any{"message": "fixture-only"}}},
		{"statusCode": 402, "body": map[string]any{"error": map[string]any{"message": "balance"}}},
		{"body": map[string]any{"id": 3, "status": "queued"}},
		{"body": map[string]any{"id": "3", "task_id": "4", "status": "queued"}},
		{"body": map[string]any{"id": "3", "status": "review"}},
	} {
		_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, response)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "fixture-only")
	}
	for _, test := range []struct{ upstream, want string }{
		{"queued", "QUEUED"}, {"in_progress", "IN_PROGRESS"}, {"unknown", "IN_PROGRESS"}, {"completed", "SUCCESS"}, {"failed", "FAILURE"}, {"review", "UNKNOWN"}, {"__proto__", "UNKNOWN"}, {"", "UNKNOWN"},
	} {
		t.Run(test.upstream, func(t *testing.T) {
			result := yuanliuHookObject(t, plugin, "parseTaskResult", ctx, map[string]any{"status": test.upstream, "progress": 40, "error": map[string]any{"message": "素材拒绝 fixture-only Bearer sk-test"}})
			assert.Equal(t, test.want, result["status"])
			if test.want == "FAILURE" {
				assert.Equal(t, "素材拒绝 [redacted] [redacted]", result["reason"])
			}
		})
	}
	task := map[string]any{"upstreamModel": "seedance-2.5-guanfang-anmiao", "state": map[string]any{"duration": 4, "resolution": "720p"}}
	assert.Equal(t, map[string]any{"seconds": float64(4), "resolution": "720p"}, yuanliuHookObject(t, plugin, "extractUsageOnComplete", task, map[string]any{"status": "SUCCESS"}, map[string]any{"billing": map[string]any{"charged": 99}}))
	for _, invalid := range []map[string]any{{"seconds": 99999999}, {"seconds": "18446744073686646784"}, {"seconds": 4.5}, {"seconds": 0}, {"seconds": 4, "duration": 5}, {"size": "4k"}, {"resolution": "480p", "size": "720p"}} {
		_, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", task, map[string]any{"status": "SUCCESS"}, invalid)
		require.Error(t, err)
	}
	result, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", task, map[string]any{"status": "FAILURE"}, map[string]any{"seconds": 99999999})
	require.NoError(t, err)
	assert.Nil(t, result)
	query := yuanliuHookObject(t, plugin, "buildQueryRequest", map[string]any{"baseUrl": "https://yuanliu.example/openapi/v1/", "apiKey": "fixture-only", "taskId": "id/with?path"})
	assert.Equal(t, "https://yuanliu.example/openapi/v1/videos/id%2Fwith%3Fpath", query["url"])
	for _, method := range []string{"GET", "HEAD"} {
		content := yuanliuHookObject(t, plugin, "buildContentRequest", map[string]any{
			"baseUrl": "https://yuanliu.example/openapi/v1/", "apiKey": "fixture-only", "artifactKey": "video", "upstreamTaskId": "id/with?path",
			"data": map[string]any{"url": "https://evil.example/collect?secret=fixture-only"}, "clientRequest": map[string]any{"method": method},
		})
		assert.Equal(t, "https://yuanliu.example/openapi/v1/videos/id%2Fwith%3Fpath/content", content["url"])
		assert.Equal(t, "GET", content["method"])
		assert.Equal(t, map[string]any{"Authorization": "Bearer fixture-only"}, content["headers"])
		assert.NotContains(t, content, "credentialless")
	}
	_, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "audio"})
	require.ErrorContains(t, err, "artifact_not_found")
	for _, credentials := range []any{nil, "", "fixture-only\nInjected: true"} {
		_, err := plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "video", "baseUrl": "https://yuanliu.example/openapi/v1", "apiKey": credentials})
		require.ErrorContains(t, err, "API key is required")
		assert.NotContains(t, err.Error(), "fixture-only")
	}
	for _, baseURL := range []string{"https://user:pass@yuanliu.example/openapi/v1", "https://yuanliu.example/openapi/v1?token=hidden", "https://yuanliu.example\\evil.example/openapi/v1"} {
		_, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": baseURL, "apiKey": "fixture-only", "taskId": "3"})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "hidden")
	}
	result, err = plugin.Engine.Call(t.Context(), "listArtifacts", map[string]any{"status": "IN_PROGRESS"})
	require.NoError(t, err)
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, string(encoded))
	result, err = plugin.Engine.Call(t.Context(), "listArtifacts", map[string]any{"status": "SUCCESS"})
	require.NoError(t, err)
	encoded, err = common.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"key":"video","type":"video","mimeType":"video/mp4"}]`, string(encoded))
	result, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, nil, map[string]any{
		"task_id": "public-task", "status": "SUCCESS", "properties": map[string]any{"origin_model_name": "Yuan-Seedance-2.5-Official"},
		"data": map[string]any{"id": "upstream-private", "seconds": "4", "size": "720p", "url": "https://cdn.example/video.mp4", "billing": map[string]any{"charged": 99}},
	})
	require.NoError(t, err)
	encoded, err = common.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"public-task","object":"video","model":"Yuan-Seedance-2.5-Official","status":"completed","seconds":"4","size":"720p"}`, string(encoded))
}
