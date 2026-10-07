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

// TestImage2ProVideoContracts 在真实 JS 引擎中验证公开协议、素材外发和按秒用量，不调用付费上游。
func TestImage2ProVideoContracts(t *testing.T) {
	source, err := plugins.Source("image2pro")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	models := []string{"无限制-Flash-中配-Video", "无限制-Flash-MAX-Video", "Seedance2.0 0.9r"}
	assert.Equal(t, models, plugin.Meta.Models)
	require.NotNil(t, plugin.Meta.ModelDiscovery)
	assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
	assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
	assert.Equal(t, "https://api.image2pro.top/v1", plugin.Meta.BaseURL)
	require.Len(t, plugin.Meta.UsageSchema, 1)
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.Equal(t, "Video generation unit price", plugin.Meta.UsageSchema["seconds"].Description["en"])
	assert.Equal(t, "视频生成单价", plugin.Meta.UsageSchema["seconds"].Description["zh"])

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			for _, protocol := range []string{"openai_video", "openai_responses"} {
				t.Run(protocol, func(t *testing.T) {
					body := map[string]any{"model": "my-video", "seconds": "5", "aspect_ratio": "16:9"}
					if protocol == "openai_video" {
						body["prompt"] = "An ocean at dawn"
					} else {
						body["input"] = "An ocean at dawn"
					}
					decoded, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
						"model": "my-video", "upstreamModel": model,
						"body": map[string]any{"kind": "json", "value": body},
					})
					require.NoError(t, callErr)
					var intent map[string]any
					encoded, marshalErr := common.Marshal(decoded)
					require.NoError(t, marshalErr)
					require.NoError(t, common.Unmarshal(encoded, &intent))
					assert.Equal(t, "my-video", intent["model"])
					request := intent["requestBody"]
					for _, base := range []string{"https://upstream.example", "https://upstream.example/v1/"} {
						value, buildErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
							"model": "my-video", "upstreamModel": model, "apiKey": "synthetic-fixture",
							"baseUrl": base, "publicTaskId": "task_image2pro_fixture", "requestBody": request,
						})
						require.NoError(t, buildErr)
						var descriptor map[string]any
						encoded, marshalErr = common.Marshal(value)
						require.NoError(t, marshalErr)
						require.NoError(t, common.Unmarshal(encoded, &descriptor))
						assert.Equal(t, "https://upstream.example/v1/videos", descriptor["url"])
						assert.Equal(t, true, descriptor["noRetry"])
						sent := descriptor["body"].(map[string]any)
						assert.Equal(t, model, sent["model"])
						assert.Equal(t, float64(5), sent["duration"])
						assert.Equal(t, "16:9", sent["ratio"])
						assert.NotContains(t, sent, "seconds")
						assert.NotContains(t, sent, "metadata")
					}
					usage, usageErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
						"model": "my-video", "upstreamModel": model, "requestBody": request, "usagePurpose": "facts",
					})
					require.NoError(t, usageErr)
					encoded, marshalErr = common.Marshal(usage)
					require.NoError(t, marshalErr)
					assert.JSONEq(t, `{"seconds":5}`, string(encoded))
				})
			}
		})
	}

	t.Run("只路由声明的三个精确模型", func(t *testing.T) {
		for _, model := range models {
			for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
				binding, found := registry.Generation().LookupEndpoint("POST", endpoint, model)
				require.True(t, found)
				assert.Same(t, plugin, binding.Plugin)
			}
		}
		_, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", "Seedance2.0")
		assert.False(t, found, "目录中的其他模型不能自动开放生成")
	})

	t.Run("普通参考对象转换为有序字符串且保留签名与重复", func(t *testing.T) {
		url := "https://cdn.example/image.png?signature=a%2Bb&expires=123"
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": models[2], "body": map[string]any{"kind": "json", "value": map[string]any{
				"model": models[2], "duration": 6, "images": []any{map[string]any{"url": url, "role": "reference_image"}, url, "data:image/png;base64,aW1hZ2U="},
			}},
		})
		require.NoError(t, callErr)
		var intent map[string]any
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, common.Unmarshal(encoded, &intent))
		req := intent["requestBody"].(map[string]any)
		assert.Equal(t, []any{url, url, "data:image/png;base64,aW1hZ2U="}, req["images"])
		assert.Equal(t, "image_to_video", intent["action"])
	})

	t.Run("同名上传字段使用独立文件引用", func(t *testing.T) {
		files := []any{
			map[string]any{"ref": "request_file:images", "field": "images", "filename": "first.png", "mimeType": "image/png", "size": 4},
			map[string]any{"ref": "request_file_index:1:images", "field": "images", "filename": "second.png", "mimeType": "image/png", "size": 6},
		}
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": models[1], "body": map[string]any{"kind": "multipart", "fields": map[string]any{
				"model": []string{models[1]}, "seconds": []string{"5"},
			}, "files": files},
		})
		require.NoError(t, callErr)
		var intent map[string]any
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, common.Unmarshal(encoded, &intent))
		value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": models[1], "upstreamModel": models[1], "requestBody": intent["requestBody"], "files": files,
			"apiKey": "fixture", "baseUrl": "https://upstream.example/v1", "publicTaskId": "task_upload_fixture",
		})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		images := descriptor["body"].(map[string]any)["images"].([]any)
		require.Len(t, images, 2)
		assert.Equal(t, "request_file:images", images[0].(map[string]any)["__fileRef"])
		assert.Equal(t, "request_file_index:1:images", images[1].(map[string]any)["__fileRef"])
		assert.Equal(t, "dataUrl", images[0].(map[string]any)["encoding"])
	})

	t.Run("音视频兼容数组保留顺序角色与签名", func(t *testing.T) {
		video := "https://cdn.example/ref.mp4?signature=a%2Bb"
		audio := "https://cdn.example/ref.mp3?signature=c%2Bd"
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": models[2], "body": map[string]any{"kind": "json", "value": map[string]any{
				"prompt": "An ocean at dawn", "duration": 5,
				"reference_videos": []any{map[string]any{"url": video, "role": "reference_video"}, video, "data:video/mp4;base64,dmlkZW8="},
				"reference_audios": []any{map[string]any{"url": audio, "role": "reference_audio"}, audio, "data:audio/mpeg;base64,YXVkaW8="},
			}},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var intent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &intent))
		request := intent["requestBody"].(map[string]any)
		assert.Equal(t, []any{video, video, "data:video/mp4;base64,dmlkZW8="}, request["videos"])
		assert.Equal(t, []any{audio, audio, "data:audio/mpeg;base64,YXVkaW8="}, request["audios"])
		assert.NotContains(t, request, "reference_audios")
		assert.NotContains(t, request, "reference_videos")
	})

	t.Run("混合文件按媒体类型构造上传占位符", func(t *testing.T) {
		files := []any{
			map[string]any{"ref": "request_file:images", "field": "images", "filename": "ref.png", "mimeType": "image/png", "size": 5},
			map[string]any{"ref": "request_file:videos", "field": "videos", "filename": "ref.mp4", "mimeType": "video/mp4", "size": 6},
			map[string]any{"ref": "request_file:audios", "field": "audios", "filename": "ref.mp3", "mimeType": "audio/mpeg", "size": 7},
		}
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": models[2], "body": map[string]any{"kind": "multipart", "fields": map[string]any{
				"model": []string{models[2]}, "seconds": []string{"5"}, "prompt": []string{"An ocean at dawn"},
			}, "files": files},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var intent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &intent))
		request := intent["requestBody"].(map[string]any)
		for _, spec := range []struct{ field, mime string }{{"images", "image/png"}, {"videos", "video/mp4"}, {"audios", "audio/mpeg"}} {
			refs := request[spec.field].([]any)
			require.Len(t, refs, 1)
			placeholder := refs[0].(map[string]any)
			assert.Equal(t, "request_file:"+spec.field, placeholder["__fileRef"])
			assert.Equal(t, "dataUrl", placeholder["encoding"])
			assert.Equal(t, spec.mime, placeholder["mimeType"])
		}
	})

	for _, spec := range []struct {
		name string
		body map[string]any
	}{
		{"缺少计费时长", map[string]any{"prompt": "ocean"}},
		{"零时长", map[string]any{"prompt": "ocean", "seconds": 0}},
		{"负时长", map[string]any{"prompt": "ocean", "seconds": -1}},
		{"超大时长", map[string]any{"prompt": "ocean", "seconds": "18446744073686646784"}},
		{"布尔时长", map[string]any{"prompt": "ocean", "seconds": true}},
		{"冲突时长", map[string]any{"prompt": "ocean", "seconds": 5, "duration": 6}},
		{"空生成输入", map[string]any{"seconds": 5}},
		{"超长提示词", map[string]any{"prompt": strings.Repeat("a", 30001), "seconds": 5}},
		{"图片别名冲突", map[string]any{"seconds": 5, "input_image": "https://cdn.example/one.png", "images": []string{"https://cdn.example/two.png"}}},
		{"首帧不可冒充普通图片", map[string]any{"seconds": 5, "images": []any{map[string]any{"url": "https://cdn.example/one.png", "role": "first_frame"}}}},
		{"音频超量不可静默截断", map[string]any{"prompt": "ocean", "seconds": 5, "audios": []string{"https://cdn.example/1.mp3", "https://cdn.example/2.mp3", "https://cdn.example/3.mp3", "https://cdn.example/4.mp3"}}},
		{"视频超量不可静默截断", map[string]any{"prompt": "ocean", "seconds": 5, "videos": []string{"https://cdn.example/1.mp4", "https://cdn.example/2.mp4", "https://cdn.example/3.mp4", "https://cdn.example/4.mp4"}}},
		{"隐藏时长不可绕过", map[string]any{"prompt": "ocean", "seconds": 5, "metadata": map[string]any{"duration": 3601}}},
		{"图片超量不可静默截断", map[string]any{"seconds": 5, "images": []string{"https://cdn.example/1.png", "https://cdn.example/2.png", "https://cdn.example/3.png", "https://cdn.example/4.png", "https://cdn.example/5.png", "https://cdn.example/6.png", "https://cdn.example/7.png", "https://cdn.example/8.png", "https://cdn.example/9.png", "https://cdn.example/10.png"}}},
	} {
		t.Run(spec.name, func(t *testing.T) {
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": models[0], "body": map[string]any{"kind": "json", "value": spec.body},
			})
			require.Error(t, callErr)
		})
	}

	t.Run("受理响应冻结秒数而非供应商积分", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{
			"requestBody": map[string]any{"duration": 5, "prompt": "ocean"},
		}, map[string]any{"statusCode": 202, "body": map[string]any{"id": "a1b2c3d4e5f6789012345678", "status": "processing", "progress": 0}})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var parsed map[string]any
		require.NoError(t, common.Unmarshal(encoded, &parsed))
		assert.Equal(t, "a1b2c3d4e5f6789012345678", parsed["taskId"])
		usage, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"state": parsed["state"]}, map[string]any{"status": "SUCCESS"}, map[string]any{"credits": 99})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(usage)
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"seconds":5}`, string(encoded))
	})

	t.Run("轮询状态与下载不泄露凭据", func(t *testing.T) {
		for _, spec := range []struct{ upstream, want string }{{"processing", "IN_PROGRESS"}, {"completed", "SUCCESS"}, {"failed", "FAILURE"}, {"mystery", "UNKNOWN"}, {"constructor", "UNKNOWN"}, {"__proto__", "UNKNOWN"}} {
			value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"state": map[string]any{"seconds": 5}}, map[string]any{
				"status": spec.upstream, "progress": 45, "url": "https://cdn.example/result.mp4", "error": "provider failure",
			})
			require.NoError(t, callErr)
			encoded, marshalErr := common.Marshal(value)
			require.NoError(t, marshalErr)
			var parsed map[string]any
			require.NoError(t, common.Unmarshal(encoded, &parsed))
			assert.Equal(t, spec.want, parsed["status"])
		}
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
		_, callErr := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": map[string]any{"id": "invalid/task"}})
		require.Error(t, callErr)
	})
}
