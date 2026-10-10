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
	assert.Equal(t, []string{"Seedance2.0 0.9r", "无限制-Flash-MAX-Video", "Seedance2.0", "Flash视频-MAX"}, plugin.Meta.Models)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, "https://api.image2pro.top/v1", plugin.Meta.BaseURL)
	require.Len(t, plugin.Meta.UsageSchema, 1)
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["seconds"].Description["en"])
	assert.Equal(t, "视频生成单价", plugin.Meta.UsageSchema["seconds"].Description["zh"])

	for _, model := range models {
		t.Run(model.client, func(t *testing.T) {
			for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
				binding, found := registry.Generation().LookupEndpoint("POST", endpoint, model.client)
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
					assert.JSONEq(t, `{"seconds":5}`, string(encoded))
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
			{name: "音频未公开", model: "无限制-Flash-MAX-Video", body: map[string]any{"duration": 5, "prompt": "ocean", "audios": []string{"https://cdn.example/ref.mp3"}}, message: "supports image references only"},
			{name: "视频未公开", model: "Seedance2.0 0.9r", body: map[string]any{"duration": 5, "prompt": "ocean", "videos": []string{"https://cdn.example/ref.mp4"}}, message: "supports image references only"},
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
