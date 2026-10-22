package plugins_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImage2ProVideoContracts 在真实 JS 引擎中验证公开模型目录、创建字段和任务恢复，不调用付费上游。
func TestImage2ProVideoContracts(t *testing.T) {
	source, err := plugins.Source("image2pro")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)

	models := []struct {
		client, upstream string
		maximum          int
	}{
		{client: "Seedance2.0 0.9r", upstream: "Seedance2.0", maximum: 15},
		{client: "无限制-Flash-MAX-Video", upstream: "Flash视频-MAX", maximum: 12},
		{client: "Seedance2.0", upstream: "Seedance2.0", maximum: 15},
		{client: "Flash视频-MAX", upstream: "Flash视频-MAX", maximum: 12},
	}
	assert.Equal(t, []string{
		"Seedance2.0 0.9r", "无限制-Flash-MAX-Video", "Seedance2.0", "Flash视频-MAX",
		"无限制-Seedream-5-Pro", "无限制-Seedream-5-Pro-2K", "无限制-Qwen-2.1NEW", "完全无限制-Z-Image",
		"无限制-NovelAI-V5", "无限制-NovelAI-V4.5", "gpt-image-2.5-sunburst", "gpt-image-2.5-flare",
		"gpt-image-2.0", "gpt-image-2.0-原生4K", "gpt-image-2.5-不超分", "gpt-image-2.0-不超分",
		"Nano-banana-2.1", "Nano-banana-Pro", "Nano-banana-2", "Grok-image-2.0", "Flash会员专属",
		"Kolors-极速生图", "无限制-Z-Image",
	}, plugin.Meta.Models)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, "channel", plugin.Meta.ModelRouting)
	assert.Equal(t, []string{"image", "video"}, plugin.Meta.ModelDiscovery.ModelTypes)
	assert.Nil(t, plugin.Meta.ModelDiscovery.TypePrefixes)
	assert.Equal(t, "https://api.image2pro.top/v1", plugin.Meta.BaseURL)
	assert.Contains(t, plugin.Meta.UsageSchema, "count")
	assert.Contains(t, plugin.Meta.UsageSchema, "seconds")
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["seconds"].Description["en"])
	assert.Equal(t, "视频生成单价", plugin.Meta.UsageSchema["seconds"].Description["zh"])

	for _, model := range models {
		t.Run(model.client, func(t *testing.T) {
			for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
				binding, found := registry.Generation().LookupPluginEndpoint("POST", endpoint, plugin.Meta.Key)
				require.True(t, found)
				assert.Same(t, plugin, binding.Plugin)
			}

			for _, protocol := range []string{"openai_video", "openai_responses"} {
				t.Run(protocol, func(t *testing.T) {
					body := map[string]any{"model": model.client, "seconds": "5", "aspect_ratio": "16:9"}
					if protocol == "openai_video" {
						body["prompt"] = "An ocean at dawn"
					} else {
						body["input"] = "An ocean at dawn"
					}
					decoded, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
						"model": model.client,
						"body":  map[string]any{"kind": "json", "value": body},
					})
					require.NoError(t, callErr)
					encoded, marshalErr := common.Marshal(decoded)
					require.NoError(t, marshalErr)
					var intent map[string]any
					require.NoError(t, common.Unmarshal(encoded, &intent))
					assert.Equal(t, model.client, intent["model"])

					for _, base := range []string{"https://upstream.example", "https://upstream.example/v1/"} {
						value, buildErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
							"model": model.client, "apiKey": "synthetic-fixture", "baseUrl": base,
							"publicTaskId": "task_image2pro_fixture", "requestBody": intent["requestBody"],
						})
						require.NoError(t, buildErr)
						encoded, marshalErr = common.Marshal(value)
						require.NoError(t, marshalErr)
						var descriptor map[string]any
						require.NoError(t, common.Unmarshal(encoded, &descriptor))
						assert.Equal(t, "https://upstream.example/v1/videos", descriptor["url"])
						assert.Equal(t, true, descriptor["noRetry"])
						assert.Equal(t, "task_image2pro_fixture", descriptor["headers"].(map[string]any)["Idempotency-Key"])
						sentJSON, marshalErr := common.Marshal(descriptor["body"])
						require.NoError(t, marshalErr)
						assert.JSONEq(t, `{"model":"`+model.upstream+`","duration":5,"ratio":"16:9","prompt":"An ocean at dawn"}`, string(sentJSON))
					}

					usage, usageErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
						"model": model.client, "requestBody": intent["requestBody"], "usagePurpose": "facts",
					})
					require.NoError(t, usageErr)
					encoded, marshalErr = common.Marshal(usage)
					require.NoError(t, marshalErr)
					if model.upstream == "Seedance2.0" {
						assert.JSONEq(t, `{"count":1}`, string(encoded))
					} else {
						assert.JSONEq(t, `{"seconds":5}`, string(encoded))
					}
				})
			}

			for _, duration := range []int{4, model.maximum} {
				_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"model": model.client, "requestBody": map[string]any{"prompt": "ocean", "duration": duration},
					"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_duration_fixture",
				})
				require.NoError(t, callErr)
			}
		})
	}

	t.Run("渠道自定义别名仍发送真实模型ID", func(t *testing.T) {
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "canvas-video", "upstreamModel": "Seedance2.0 0.9r", "body": map[string]any{"kind": "json", "value": map[string]any{
				"model": "canvas-video", "duration": 5, "prompt": "ocean",
			}},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var intent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &intent))
		assert.Equal(t, "canvas-video", intent["model"])

		value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "canvas-video", "upstreamModel": "Seedance2.0 0.9r", "requestBody": intent["requestBody"],
			"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_alias_fixture",
		})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		assert.Equal(t, "Seedance2.0", descriptor["body"].(map[string]any)["model"])
	})

	for _, model := range []string{"Seedance2.0-old", "无限制-Flash-中配-Video", "MiniMax-H3", "MiniMax-H3-Max"} {
		for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
			_, found := registry.Generation().LookupEndpoint("POST", endpoint, model)
			assert.False(t, found, "目录中的其他模型不能自动开放生成")
		}
	}

	t.Run("单图与多图使用公开字段", func(t *testing.T) {
		for _, spec := range []struct {
			name   string
			images []string
			field  string
		}{
			{name: "单图", images: []string{"https://cdn.example/one.png?signature=a%2Bb"}, field: "input_image"},
			{name: "多图", images: []string{"https://cdn.example/one.png", "data:image/png;base64,aW1hZ2U="}, field: "images"},
		} {
			t.Run(spec.name, func(t *testing.T) {
				value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"model": "Seedance2.0 0.9r", "requestBody": map[string]any{"duration": 5, "prompt": "ocean", "images": spec.images},
					"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_image_fixture",
				})
				require.NoError(t, callErr)
				encoded, marshalErr := common.Marshal(value)
				require.NoError(t, marshalErr)
				var descriptor map[string]any
				require.NoError(t, common.Unmarshal(encoded, &descriptor))
				sent := descriptor["body"].(map[string]any)
				assert.NotContains(t, sent, "content")
				if spec.field == "input_image" {
					assert.Equal(t, spec.images[0], sent[spec.field])
					assert.NotContains(t, sent, "images")
				} else {
					assert.Equal(t, []any{spec.images[0], spec.images[1]}, sent[spec.field])
					assert.NotContains(t, sent, "input_image")
				}
			})
		}
	})

	t.Run("兼容content在发送前转换为公开字段", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "text", "text": "first line\n"},
			map[string]any{"type": "text", "text": "second line"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}, "role": "reference_image"},
		}
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "无限制-Flash-MAX-Video", "requestBody": map[string]any{"duration": 5, "content": content},
			"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_content_fixture",
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		sent := descriptor["body"].(map[string]any)
		assert.Equal(t, "Flash视频-MAX", sent["model"])
		assert.Equal(t, "first line\nsecond line", sent["prompt"])
		assert.Equal(t, "https://cdn.example/ref.png", sent["input_image"])
		assert.NotContains(t, sent, "content")
	})

	t.Run("同名图片上传使用独立文件引用", func(t *testing.T) {
		files := []any{
			map[string]any{"ref": "request_file:images", "field": "images", "filename": "first.png", "mimeType": "image/png", "size": 4},
			map[string]any{"ref": "request_file_index:1:images", "field": "images", "filename": "second.png", "mimeType": "image/png", "size": 6},
		}
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "Seedance2.0 0.9r", "body": map[string]any{"kind": "multipart", "fields": map[string]any{
				"model": []string{"Seedance2.0 0.9r"}, "seconds": []string{"5"},
			}, "files": files},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var intent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &intent))
		value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "Seedance2.0 0.9r", "requestBody": intent["requestBody"], "files": files,
			"apiKey": "fixture", "baseUrl": "https://upstream.example/v1", "publicTaskId": "task_upload_fixture",
		})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		images := descriptor["body"].(map[string]any)["images"].([]any)
		require.Len(t, images, 2)
		for index, reference := range []string{"request_file:images", "request_file_index:1:images"} {
			placeholder := images[index].(map[string]any)
			assert.Equal(t, reference, placeholder["__fileRef"])
			assert.Equal(t, "dataUrl", placeholder["encoding"])
			assert.Equal(t, "image/png", placeholder["mimeType"])
		}
	})

	t.Run("公开目录限制在发送前执行", func(t *testing.T) {
		flashImages := make([]string, 6)
		seedanceImages := make([]string, 10)
		for index := range flashImages {
			flashImages[index] = "https://cdn.example/flash-" + string(rune('a'+index)) + ".png"
		}
		for index := range seedanceImages {
			seedanceImages[index] = "https://cdn.example/seedance-" + string(rune('a'+index)) + ".png"
		}
		cases := []struct {
			name, model string
			body        map[string]any
			message     string
		}{
			{name: "Flash最多五图", model: "无限制-Flash-MAX-Video", body: map[string]any{"duration": 5, "images": flashImages}, message: "at most 5 reference images"},
			{name: "Seedance最多九图", model: "Seedance2.0 0.9r", body: map[string]any{"duration": 5, "images": seedanceImages}, message: "at most 9 reference images"},
			{name: "Flash固定比例", model: "无限制-Flash-MAX-Video", body: map[string]any{"duration": 5, "prompt": "ocean", "ratio": "adaptive"}, message: "requires a fixed ratio"},
			{name: "音频要求HTTPS", model: "无限制-Flash-MAX-Video", body: map[string]any{"duration": 5, "prompt": "ocean", "audios": []string{"http://cdn.example/ref.mp3"}}, message: "require an HTTPS URL or mediaId"},
			{name: "视频未公开", model: "Seedance2.0 0.9r", body: map[string]any{"duration": 5, "prompt": "ocean", "videos": []string{"https://cdn.example/ref.mp4"}}, message: "does not support video references"},
			{name: "首帧角色未公开", model: "无限制-Flash-MAX-Video", body: map[string]any{"duration": 5, "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}, "role": "first_frame"}}}, message: "unsupported Image2Pro Flash-MAX image role"},
			{name: "Seedance扩展开关未公开", model: "Seedance2.0 0.9r", body: map[string]any{"duration": 5, "prompt": "ocean", "watermark": false}, message: "does not support Seedance generation flags"},
			{name: "未公开分辨率不静默丢弃", model: "Seedance2.0 0.9r", body: map[string]any{"duration": 5, "prompt": "ocean", "resolution": "1080p"}, message: "does not expose resolution"},
			{name: "Flash时长上限", model: "无限制-Flash-MAX-Video", body: map[string]any{"duration": 13, "prompt": "ocean"}, message: "integer from 4 to 12"},
			{name: "Seedance时长上限", model: "Seedance2.0 0.9r", body: map[string]any{"duration": 16, "prompt": "ocean"}, message: "integer from 4 to 15"},
			{name: "提示词上限", model: "Seedance2.0 0.9r", body: map[string]any{"duration": 5, "prompt": strings.Repeat("a", 30001)}, message: "must not exceed 30000"},
		}
		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"model": test.model, "requestBody": test.body, "baseUrl": "https://upstream.example",
					"apiKey": "synthetic-fixture", "publicTaskId": "task_invalid_fixture",
				})
				require.ErrorContains(t, callErr, test.message)
			})
		}
	})

	for _, test := range []struct{ name, url string }{
		{name: "内嵌凭据", url: "https://fixture-user:fixture-password@cdn.example/ref.png"},
		{name: "控制字符", url: "https://cdn.example/ref.png?value=\x7f"},
		{name: "空白", url: "https://cdn.example/ref image.png"},
		{name: "反斜杠", url: `https://cdn.example\ref.png`},
		{name: "非HTTP协议", url: "ftp://cdn.example/ref.png"},
	} {
		t.Run("拒绝不安全地址_"+test.name, func(t *testing.T) {
			_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"model": "Seedance2.0 0.9r", "requestBody": map[string]any{"duration": 5, "images": []string{test.url}},
				"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_unsafe_fixture",
			})
			require.ErrorContains(t, callErr, "references must be HTTP(S) URLs")
			_, callErr = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{
				"artifactKey": "video", "data": map[string]any{"url": test.url}, "clientRequest": map[string]any{"method": "GET"},
			})
			require.ErrorContains(t, callErr, "video URL is unavailable")
		})
	}

	t.Run("幂等键只放请求头", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "Seedance2.0 0.9r", "requestBody": map[string]any{
				"prompt": "ocean", "duration": 5, "clientRequestId": "seedance-stable-0001",
			}, "baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_idempotency_fixture",
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		assert.Equal(t, "seedance-stable-0001", descriptor["headers"].(map[string]any)["Idempotency-Key"])
		assert.NotContains(t, descriptor["body"].(map[string]any), "client_request_id")
		assert.NotContains(t, descriptor["body"].(map[string]any), "clientRequestId")
	})

	t.Run("轮询状态与冻结用量保持兼容", func(t *testing.T) {
		query, callErr := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
			"taskId": "a1b2c3d4e5f6789012345678", "apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example/v1",
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(query)
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"url":"https://upstream.example/v1/videos/a1b2c3d4e5f6789012345678","method":"GET","headers":{"Authorization":"Bearer synthetic-fixture"}}`, string(encoded))

		value, callErr := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{
			"requestBody": map[string]any{"duration": 12.5, "prompt": "old task"},
		}, map[string]any{"statusCode": 202, "body": map[string]any{"id": "a1b2c3d4e5f6789012345678", "status": "processing", "progress": 0}})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		var parsed map[string]any
		require.NoError(t, common.Unmarshal(encoded, &parsed))
		usage, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"state": parsed["state"]}, map[string]any{"status": "SUCCESS"}, map[string]any{"duration": 99})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(usage)
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"seconds":12.5}`, string(encoded))

		for _, status := range []struct{ upstream, want string }{{"processing", "IN_PROGRESS"}, {"completed", "SUCCESS"}, {"failed", "FAILURE"}, {"mystery", "UNKNOWN"}, {"constructor", "UNKNOWN"}, {"__proto__", "UNKNOWN"}} {
			value, callErr = plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"state": map[string]any{"seconds": 5}}, map[string]any{
				"status": status.upstream, "progress": 45, "url": "https://cdn.example/result.mp4", "error": "provider failure",
			})
			require.NoError(t, callErr)
			encoded, marshalErr = common.Marshal(value)
			require.NoError(t, marshalErr)
			require.NoError(t, common.Unmarshal(encoded, &parsed))
			assert.Equal(t, status.want, parsed["status"])
		}
	})

	t.Run("成片下载不携带渠道凭据", func(t *testing.T) {
		for _, method := range []string{"GET", "HEAD"} {
			value, callErr := plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{
				"artifactKey": "video", "apiKey": "credential-must-not-be-sent", "clientRequest": map[string]any{"method": method},
				"data": map[string]any{"status": "completed", "url": "https://cdn.example/result.mp4?signature=x%2By"},
			})
			require.NoError(t, callErr)
			encoded, marshalErr := common.Marshal(value)
			require.NoError(t, marshalErr)
			assert.JSONEq(t, `{"url":"https://cdn.example/result.mp4?signature=x%2By","method":"`+method+`","credentialless":true}`, string(encoded))
			assert.NotContains(t, string(encoded), "credential-must-not-be-sent")
		}
	})
}

// TestImage2ProBillingLifecycle 验证宿主预扣、提交状态、轮询/即时完成与旧计费快照，全部使用合成响应。
func TestImage2ProBillingLifecycle(t *testing.T) {
	source, err := plugins.Source("image2pro")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, "2.4.0", plugin.Meta.Version)
	for _, tc := range []struct {
		name, upstream, unit string
		maximum              int
	}{
		{"Seedance2.0 0.9r", "Seedance2.0", "count", 15},
		{"Seedance2.0", "Seedance2.0", "count", 15},
		{"无限制-Flash-MAX-Video", "Flash视频-MAX", "seconds", 12},
		{"Flash视频-MAX", "Flash视频-MAX", "seconds", 12},
		{"custom-seedance-alias", "Seedance2.0", "count", 15},
	} {
		for _, duration := range []int{4, 5, tc.maximum} {
			t.Run(fmt.Sprintf("%s/%ds", tc.name, duration), func(t *testing.T) {
				schema, _ := plugin.Meta.UsageForModel(tc.upstream)
				require.Len(t, schema, 1)
				if tc.unit == "count" {
					assert.Equal(t, "count", schema["count"].Unit)
					assert.Equal(t, "次", schema["count"].UnitLabel["zh"])
				} else {
					assert.Equal(t, "second", schema["seconds"].Unit)
				}
				info := &relaycommon.RelayInfo{
					OriginModelName: tc.name,
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelBaseUrl: "https://upstream.example", ApiKey: "synthetic-fixture", UpstreamModelName: tc.upstream,
					},
					TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_image2pro_billing_fixture"},
				}
				adaptor := taskplugin.New(plugin)
				adaptor.Init(info)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
				c.Set("task_request", map[string]any{"model": tc.name, "prompt": "ocean", "duration": duration})
				require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
				facts, err := adaptor.ExtractUsageFactsValidated(c, info)
				require.NoError(t, err)
				units := float64(duration)
				if tc.unit == "count" {
					units = 1
				}
				expected := map[string]any{tc.unit: units}
				assert.Equal(t, expected, facts)
				ratios, err := adaptor.EstimateBillingValidated(c, info)
				require.NoError(t, err)
				assert.Empty(t, ratios, "用量不重复叠加到旧倍率")
				body, err := adaptor.BuildRequestBody(c, info)
				require.NoError(t, err)
				var sent map[string]any
				require.NoError(t, common.DecodeJson(body, &sent))
				assert.Equal(t, float64(duration), sent["duration"], "时长仍作为真实生成参数")
				assert.Equal(t, tc.upstream, sent["model"])
				expression := `u("` + tc.unit + `") * 0.2`
				require.NoError(t, billing_setting.SmokeTestTaskExpr(expression, schema))
				cost, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
				require.NoError(t, err)
				assert.InDelta(t, units*0.2, cost, 1e-10)
				if tc.unit == "count" {
					legacyExpression := `u("seconds") * 0.2`
					assert.False(t, billing_setting.TaskExprCompatible(legacyExpression, schema))
					require.Error(t, billing_setting.SmokeTestTaskExpr(legacyExpression, schema))
					_, _, err = billingexpr.RunExprWithRequest(legacyExpression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
					require.Error(t, err, "旧按秒表达式不能悄悄得到零费用")
				}
				snapshot := &billingexpr.BillingSnapshot{
					ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 2,
					QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: facts,
				}
				for _, status := range []string{"processing", "completed"} {
					response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
						`{"id":"a1b2c3d4e5f6789012345678","status":"` + status + `","url":"https://cdn.example/result.mp4","duration":999}`,
					))}
					parsed, taskErr := adaptor.ParseResponse(c, response, info)
					require.Nil(t, taskErr)
					require.NoError(t, response.Body.Close())
					if tc.unit == "count" {
						assert.JSONEq(t, fmt.Sprintf(`{"seconds":%d,"billingUnit":"count"}`, duration), string(parsed.PluginState))
					} else {
						assert.JSONEq(t, fmt.Sprintf(`{"seconds":%d}`, duration), string(parsed.PluginState))
					}
					task := &model.Task{
						Properties:  model.Properties{OriginModelName: tc.name, UpstreamModelName: tc.upstream},
						PrivateData: model.TaskPrivateData{PluginState: parsed.PluginState},
					}
					completed, err := adaptor.ParseTaskResult(task, response, []byte(`{"status":"completed","url":"https://cdn.example/result.mp4","duration":999}`))
					require.NoError(t, err)
					assert.Equal(t, "SUCCESS", completed.Status)
					assert.Equal(t, expected, completed.UsageFacts)
					if status == "completed" {
						require.NotNil(t, parsed.Immediate)
						assert.Equal(t, expected, parsed.Immediate.UsageFacts)
					}
					settled, merged, err := service.EvaluateTaskCompletionUsage(snapshot, completed.UsageFacts)
					require.NoError(t, err)
					assert.Equal(t, expected, merged)
					assert.Equal(t, common.QuotaRound(cost*1000000), settled.ActualQuotaAfterGroup)
					assert.Nil(t, settled.Clamp)
					failed, err := adaptor.ParseTaskResult(task, response, []byte(`{"status":"failed"}`))
					require.NoError(t, err)
					assert.Empty(t, failed.UsageFacts)
				}
			})
		}
	}

	t.Run("旧任务保留原冻结秒数与价格", func(t *testing.T) {
		adaptor := taskplugin.New(plugin)
		adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://upstream.example"}})
		for _, state := range []string{`{"seconds":12.5}`, `{"seconds":12.5,"billingUnit":"count"}`} {
			task := &model.Task{
				Properties:  model.Properties{OriginModelName: "Seedance2.0 0.9r", UpstreamModelName: "Seedance2.0"},
				PrivateData: model.TaskPrivateData{PluginState: []byte(state)},
			}
			response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
			completed, err := adaptor.ParseTaskResult(task, response, []byte(`{"status":"completed","url":"https://cdn.example/result.mp4"}`))
			require.NoError(t, err)
			if state == `{"seconds":12.5}` {
				assert.Equal(t, map[string]any{"seconds": 12.5}, completed.UsageFacts)
			}
			expression := `u("seconds") * 0.2`
			snapshot := &billingexpr.BillingSnapshot{
				ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1,
				QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: map[string]any{"seconds": 12.5},
			}
			settled, merged, err := service.EvaluateTaskCompletionUsage(snapshot, completed.UsageFacts)
			require.NoError(t, err)
			assert.Equal(t, 12.5, merged["seconds"])
			assert.Equal(t, 1250000, settled.ActualQuotaAfterGroup)
			assert.Equal(t, map[string]any{"seconds": 12.5}, snapshot.UsageFacts)
		}
	})
}

// TestImage2ProImageTaskContracts validates the asynchronous image protocol with synthetic data only.
func TestImage2ProImageTaskContracts(t *testing.T) {
	imageModels := []string{
		"无限制-Seedream-5-Pro", "无限制-Seedream-5-Pro-2K", "无限制-Qwen-2.1NEW", "完全无限制-Z-Image",
		"无限制-NovelAI-V5", "无限制-NovelAI-V4.5", "gpt-image-2.5-sunburst", "gpt-image-2.5-flare",
		"gpt-image-2.0", "gpt-image-2.0-原生4K", "gpt-image-2.5-不超分", "gpt-image-2.0-不超分",
		"Nano-banana-2.1", "Nano-banana-Pro", "Nano-banana-2", "Grok-image-2.0", "Flash会员专属",
		"Kolors-极速生图", "无限制-Z-Image",
	}
	videoModels := []string{"Seedance2.0 0.9r", "无限制-Flash-MAX-Video", "Seedance2.0", "Flash视频-MAX"}
	source, err := plugins.Source("image2pro")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, append(append([]string{}, videoModels...), imageModels...), plugin.Meta.Models)
	require.Len(t, plugin.Meta.Protocols, 3)
	assert.Nil(t, plugin.Meta.Protocols[0].Models)
	assert.Nil(t, plugin.Meta.Protocols[1].Models)
	assert.Nil(t, plugin.Meta.Protocols[2].Models)
	for _, modelName := range imageModels {
		binding, found := registry.Generation().LookupPluginEndpoint(http.MethodPost, "/v1/images/tasks", plugin.Meta.Key)
		require.True(t, found, "image model %q must route to async image tasks", modelName)
		assert.Same(t, plugin, binding.Plugin)
	}
	for _, modelName := range videoModels {
		binding, found := registry.Generation().LookupPluginEndpoint(http.MethodPost, "/v1/videos", plugin.Meta.Key)
		require.True(t, found, "video model %q must route to videos", modelName)
		assert.Same(t, plugin, binding.Plugin)
	}

	modelName := imageModels[8]
	requestBody := map[string]any{
		"model": modelName, "prompt": "A quiet coastal town", "n": 2, "size": "1024x1024",
		"aspect_ratio": "1:1", "ratio": "1:1", "resolution": "2k", "image_size": "1024",
		"input_image": "https://cdn.example/input.png", "reference_image": "https://cdn.example/reference.png",
		"image": "https://cdn.example/image.png", "images": []string{"https://cdn.example/a.png", "https://cdn.example/b.png"},
		"mask": "https://cdn.example/mask.png", "response_format": "url",
	}
	decodedValue, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image_tasks", "decodeRequest"}, map[string]any{
		"model": modelName, "body": map[string]any{"kind": "json", "value": requestBody},
	})
	require.NoError(t, err)
	decodedBytes, err := common.Marshal(decodedValue)
	require.NoError(t, err)
	var intent map[string]any
	require.NoError(t, common.Unmarshal(decodedBytes, &intent))
	assert.Equal(t, "submit", intent["kind"])
	assert.Equal(t, modelName, intent["model"])
	requestJSON, err := common.Marshal(requestBody)
	require.NoError(t, err)
	intentRequestJSON, err := common.Marshal(intent["requestBody"])
	require.NoError(t, err)
	var normalizedRequest map[string]any
	require.NoError(t, common.Unmarshal(intentRequestJSON, &normalizedRequest))
	assert.Equal(t, true, normalizedRequest["__imageTask"])
	delete(normalizedRequest, "__imageTask")
	intentRequestJSON, err = common.Marshal(normalizedRequest)
	require.NoError(t, err)
	assert.JSONEq(t, string(requestJSON), string(intentRequestJSON))

	build := func(taskID string) (map[string]any, error) {
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": modelName, "upstreamModel": modelName, "requestBody": intent["requestBody"],
			"baseUrl": "https://upstream.example/v1", "apiKey": "synthetic-fixture", "publicTaskId": taskID,
		})
		if callErr != nil {
			return nil, callErr
		}
		encoded, marshalErr := common.Marshal(value)
		if marshalErr != nil {
			return nil, marshalErr
		}
		var descriptor map[string]any
		marshalErr = common.Unmarshal(encoded, &descriptor)
		return descriptor, marshalErr
	}
	descriptor, err := build("task_public-123")
	require.NoError(t, err)
	assert.Equal(t, "https://upstream.example/v1/images/tasks", descriptor["url"])
	assert.Equal(t, http.MethodPost, descriptor["method"])
	assert.Equal(t, true, descriptor["noRetry"])
	assert.Equal(t, "task_public-123", descriptor["headers"].(map[string]any)["Idempotency-Key"])
	assert.Equal(t, "Bearer synthetic-fixture", descriptor["headers"].(map[string]any)["Authorization"])
	descriptorBodyJSON, err := common.Marshal(descriptor["body"])
	require.NoError(t, err)
	assert.JSONEq(t, string(requestJSON), string(descriptorBodyJSON))
	for _, forbidden := range []string{"quality", "background", "output_format", "output_compression", "stream", "client_request_id"} {
		_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image_tasks", "decodeRequest"}, map[string]any{
			"model": modelName, "body": map[string]any{"kind": "json", "value": map[string]any{"model": modelName, "prompt": "a picture", forbidden: "invalid"}},
		})
		require.Error(t, err, "forbidden field %q must be rejected", forbidden)
	}
	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image_tasks", "decodeRequest"}, map[string]any{
		"model": modelName, "body": map[string]any{"kind": "multipart", "fields": map[string]any{}},
	})
	require.Error(t, err, "multipart image requests are unsupported")
	for _, invalid := range []map[string]any{
		{"model": modelName, "prompt": "a picture", "n": 0},
		{"model": modelName, "prompt": "a picture", "n": 5},
		{"model": modelName, "prompt": "a picture", "n": 1.5},
		{"model": modelName, "prompt": "a picture", "response_format": "b64_json"},
		{"model": modelName, "prompt": "   "},
		{"prompt": "a picture"},
	} {
		_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image_tasks", "decodeRequest"}, map[string]any{
			"model": modelName, "body": map[string]any{"kind": "json", "value": invalid},
		})
		require.Error(t, err, "invalid request %#v must be rejected", invalid)
	}
	_, err = build("bad/task-id")
	require.Error(t, err, "unsafe public task ID cannot be used as an idempotency key")

	usageValue, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
		"model": modelName, "upstreamModel": modelName, "requestBody": intent["requestBody"], "usagePurpose": "facts",
	})
	require.NoError(t, err)
	usageBytes, err := common.Marshal(usageValue)
	require.NoError(t, err)
	assert.JSONEq(t, `{"count":2}`, string(usageBytes))
	schema, _ := plugin.Meta.UsageForModel(modelName)
	require.Contains(t, schema, "count")
	assert.Equal(t, "count", schema["count"].Unit)
	completeUsage, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{
		"model": modelName, "upstreamModel": modelName, "state": map[string]any{"n": 2},
	}, map[string]any{"status": "SUCCESS"})
	require.NoError(t, err)
	usageBytes, err = common.Marshal(completeUsage)
	require.NoError(t, err)
	assert.JSONEq(t, `{"count":2}`, string(usageBytes))

	created := map[string]any{
		"id": "upstream-job:123", "object": "image.task", "model": modelName, "status": "queued",
		"created_at": 1700000000, "updated_at": 1700000001, "expires_at": 1700003600, "n": 2,
		"accounting_status": "pending",
	}
	parsedValue, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{
		"model": modelName, "upstreamModel": modelName, "requestBody": intent["requestBody"],
	}, map[string]any{"statusCode": http.StatusAccepted, "body": created})
	require.NoError(t, err)
	parsedBytes, err := common.Marshal(parsedValue)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, common.Unmarshal(parsedBytes, &parsed))
	assert.Equal(t, "upstream-job:123", parsed["taskId"])
	createdJSON, err := common.Marshal(created)
	require.NoError(t, err)
	parsedTaskDataJSON, err := common.Marshal(parsed["taskData"])
	require.NoError(t, err)
	assert.JSONEq(t, string(createdJSON), string(parsedTaskDataJSON))
	assert.Equal(t, map[string]any{"kind": "image", "n": float64(2)}, parsed["state"])

	queryValue, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
		"model": modelName, "upstreamModel": modelName, "taskId": "upstream-job:123",
		"apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example/v1",
	})
	require.NoError(t, err)
	queryBytes, err := common.Marshal(queryValue)
	require.NoError(t, err)
	assert.JSONEq(t, `{"url":"https://upstream.example/v1/images/tasks/upstream-job%3A123","method":"GET","headers":{"Authorization":"Bearer synthetic-fixture"}}`, string(queryBytes))
	for _, taskID := range []string{"../escape", "a/b", "id?query", "id#fragment", ".."} {
		_, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
			"model": modelName, "upstreamModel": modelName, "taskId": taskID,
			"apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example",
		})
		require.Error(t, err, "unsafe task ID %q must be rejected", taskID)
	}

	for _, tc := range []struct{ upstream, want string }{
		{"queued", "QUEUED"}, {"processing", "IN_PROGRESS"}, {"completed", "SUCCESS"}, {"failed", "FAILURE"}, {"unknown", "UNKNOWN"},
	} {
		body := map[string]any{"status": tc.upstream, "n": 2}
		if tc.upstream == "completed" {
			body["data"] = []any{map[string]any{"url": "https://cdn.example/a.png"}, map[string]any{"url": "https://cdn.example/b.png"}}
		}
		if tc.upstream == "failed" {
			body["error"] = map[string]any{"message": "provider secret must not leak"}
		}
		value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{
			"model": modelName, "upstreamModel": modelName, "state": map[string]any{"n": 2},
		}, body)
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var result map[string]any
		require.NoError(t, common.Unmarshal(encoded, &result))
		assert.Equal(t, tc.want, result["status"])
		assert.NotContains(t, string(encoded), "provider secret")
	}
	for _, unsafeURL := range []string{"file:///etc/passwd", "https://user:password@cdn.example/a.png", "https://cdn.example/a b.png"} {
		value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"model": modelName}, map[string]any{
			"status": "completed", "data": []any{map[string]any{"url": unsafeURL}},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var result map[string]any
		require.NoError(t, common.Unmarshal(encoded, &result))
		assert.Equal(t, "UNKNOWN", result["status"])
	}

	view := map[string]any{
		"task_id": "newapi-public-task", "status": "SUCCESS", "created_at": 1700000000, "updated_at": 1700000020,
		"data": map[string]any{
			"id": "upstream-job:123", "model": modelName, "status": "completed", "n": 2,
			"data":  []any{map[string]any{"url": "https://cdn.example/a.png"}, map[string]any{"url": "https://cdn.example/b.png"}},
			"error": map[string]any{"message": "must not leak"},
		},
	}
	renderedValue, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image_tasks", "render"},
		map[string]any{"protocol": "openai_image_tasks", "operation": "retrieve"}, view)
	require.NoError(t, err)
	renderedBytes, err := common.Marshal(renderedValue)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"newapi-public-task","object":"image.task","model":"`+modelName+`","status":"completed","created_at":1700000000,"updated_at":1700000020,"n":2,"data":[{"url":"https://cdn.example/a.png"},{"url":"https://cdn.example/b.png"}]}`, string(renderedBytes))
	assert.NotContains(t, string(renderedBytes), "upstream-job")
	assert.NotContains(t, string(renderedBytes), "must not leak")

	failedValue, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image_tasks", "render"},
		map[string]any{"protocol": "openai_image_tasks", "operation": "retrieve"}, map[string]any{
			"task_id": "newapi-public-task", "status": "FAILURE", "created_at": 1700000000,
			"data": map[string]any{"status": "failed", "error": "provider credential leak"},
		})
	require.NoError(t, err)
	failedBytes, err := common.Marshal(failedValue)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"newapi-public-task","object":"image.task","model":"","status":"failed","created_at":1700000000,"error":{"code":"image_generation_failed","message":"Image2Pro image generation failed"}}`, string(failedBytes))
	assert.NotContains(t, string(failedBytes), "provider credential")
}
