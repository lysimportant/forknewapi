package plugins_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
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
	const seedanceModel = "Seedance2.0 0.9r"
	models := []string{seedanceModel, "无限制-Flash-MAX-Video"}
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
						sentJSON, marshalErr := common.Marshal(sent)
						require.NoError(t, marshalErr)
						assert.JSONEq(t, `{"model":"`+model+`","duration":5,"resolution":"720p","ratio":"16:9","content":[{"type":"text","text":"An ocean at dawn"}]}`, string(sentJSON))
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

	t.Run("Seedance 查询沿用 Image2Pro 视频路径", func(t *testing.T) {
		model := seedanceModel
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": model, "body": map[string]any{"kind": "json", "value": map[string]any{"model": model, "prompt": "An ocean at dawn", "duration": 5, "ratio": "16:9"}},
		})
		require.NoError(t, callErr)
		var intent map[string]any
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, common.Unmarshal(encoded, &intent))
		request := intent["requestBody"].(map[string]any)
		assert.Equal(t, float64(5), request["duration"])

		submission, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": model, "upstreamModel": model, "apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example/v1",
			"publicTaskId": "task_seedance_fixture", "requestBody": request,
		})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(submission)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		assert.Equal(t, "https://upstream.example/v1/videos", descriptor["url"])
		sent := descriptor["body"].(map[string]any)
		assert.Equal(t, float64(5), sent["duration"])
		assert.Equal(t, "720p", sent["resolution"])
		assert.NotContains(t, sent, "size")

		query, callErr := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
			"taskId": "a1b2c3d4e5f6789012345678", "apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example/v1",
		})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(query)
		require.NoError(t, marshalErr)
		var queryDescriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &queryDescriptor))
		assert.Equal(t, "https://upstream.example/v1/videos/a1b2c3d4e5f6789012345678", queryDescriptor["url"])
		assert.Equal(t, "GET", queryDescriptor["method"])
		assert.Equal(t, "Bearer synthetic-fixture", queryDescriptor["headers"].(map[string]any)["Authorization"])
	})

	for _, spec := range []struct {
		name       string
		content    []any
		seconds    int
		resolution string
		ratio      string
	}{
		{"文生最低时长", []any{map[string]any{"type": "text", "text": "An ocean at dawn"}}, 4, "480p", "16:9"},
		{"首帧无需提示词", []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,aW1hZ2U="}, "role": "first_frame"}}, 5, "720p", "adaptive"},
		{"首尾帧保留角色", []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/first.png"}, "role": "first_frame"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/last.png"}, "role": "last_frame"},
		}, 15, "4k", "21:9"},
		{"纯视频参考与空文本", []any{
			map[string]any{"type": "text", "text": ""},
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4?signature=a%2Bb"}, "role": "reference_video"},
		}, 6, "1080p", "9:16"},
		{"全模态参考保持跨媒体顺序", []any{
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}, "role": "reference_video"},
			map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "data:audio/wav;base64,YXVkaW8="}, "role": "reference_audio"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}, "role": "reference_image"},
		}, 7, "720p", "1:1"},
	} {
		t.Run("官方 content_"+spec.name, func(t *testing.T) {
			body := map[string]any{
				"model": "my-seedance", "content": spec.content, "duration": spec.seconds, "resolution": spec.resolution, "ratio": spec.ratio,
				"generate_audio": false, "watermark": false, "return_last_frame": false,
			}
			value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": "my-seedance", "upstreamModel": seedanceModel, "body": map[string]any{"kind": "json", "value": body},
			})
			require.NoError(t, callErr)
			encoded, marshalErr := common.Marshal(value)
			require.NoError(t, marshalErr)
			var intent map[string]any
			require.NoError(t, common.Unmarshal(encoded, &intent))
			value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"model": "my-seedance", "upstreamModel": seedanceModel, "requestBody": intent["requestBody"],
				"baseUrl": "https://upstream.example/v1", "apiKey": "synthetic-fixture", "publicTaskId": "task_official_content",
			})
			require.NoError(t, callErr)
			encoded, marshalErr = common.Marshal(value)
			require.NoError(t, marshalErr)
			var descriptor map[string]any
			require.NoError(t, common.Unmarshal(encoded, &descriptor))
			body["model"] = seedanceModel
			wantJSON, marshalErr := common.Marshal(body)
			require.NoError(t, marshalErr)
			sentJSON, marshalErr := common.Marshal(descriptor["body"])
			require.NoError(t, marshalErr)
			assert.JSONEq(t, string(wantJSON), string(sentJSON))
			assert.Equal(t, true, descriptor["noRetry"])
		})
	}

	t.Run("九图三视频三音频均保留", func(t *testing.T) {
		content := []any{}
		for _, spec := range []struct {
			kind, role, url string
			count           int
		}{
			{"image_url", "reference_image", "https://cdn.example/ref.png", 9},
			{"video_url", "reference_video", "https://cdn.example/ref.mp4", 3},
			{"audio_url", "reference_audio", "https://cdn.example/ref.mp3", 3},
		} {
			for range spec.count {
				content = append(content, map[string]any{"type": spec.kind, spec.kind: map[string]any{"url": spec.url}, "role": spec.role})
			}
		}
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": seedanceModel, "requestBody": map[string]any{"model": seedanceModel, "content": content, "duration": 5},
			"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_reference_limits",
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		assert.Equal(t, content, descriptor["body"].(map[string]any)["content"])
	})

	t.Run("视频文件上传不可冒充官方视频URL", func(t *testing.T) {
		_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": seedanceModel, "body": map[string]any{"kind": "multipart", "fields": map[string]any{"seconds": []string{"5"}}, "files": []any{
				map[string]any{"ref": "request_file:videos", "field": "videos", "filename": "ref.mp4", "mimeType": "video/mp4", "size": 6},
			}},
		})
		require.ErrorContains(t, callErr, "video uploads are unsupported")
	})

	t.Run("客户端幂等键只放请求头", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": seedanceModel, "requestBody": map[string]any{"prompt": "An ocean at dawn", "duration": 5, "clientRequestId": "seedance-stable-0001"},
			"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_idempotency_fixture",
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

	for _, model := range []string{"无限制-Flash-中配-Video"} {
		t.Run("Flash 拒绝创建但保留旧任务恢复_"+model, func(t *testing.T) {
			for _, protocol := range []string{"openai_video", "openai_responses"} {
				for _, clientModel := range []string{model, "my-video"} {
					body := map[string]any{"model": clientModel, "seconds": 5}
					if protocol == "openai_video" {
						body["prompt"] = "An ocean at dawn"
					} else {
						body["input"] = "An ocean at dawn"
					}
					_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
						"model": clientModel, "upstreamModel": model, "body": map[string]any{"kind": "json", "value": body},
					})
					require.ErrorContains(t, callErr, "unsupported Image2Pro model")
				}
			}
			for _, clientModel := range []string{model, "my-video"} {
				_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"model": clientModel, "upstreamModel": model, "requestBody": map[string]any{"model": clientModel, "duration": 5, "prompt": "ocean"},
					"baseUrl": "https://upstream.example/v1", "apiKey": "synthetic-fixture", "publicTaskId": "task_flash_rejected",
				})
				require.ErrorContains(t, callErr, "unsupported Image2Pro model")
			}

			// 收窄的是新建合同；持久化任务仍按原供应商编号查询，并按已冻结秒数结算。
			task := map[string]any{
				"model": "my-video", "upstreamModel": model, "taskId": "a1b2c3d4e5f6789012345678",
				"baseUrl": "https://upstream.example/v1", "apiKey": "synthetic-fixture", "state": map[string]any{"seconds": 12.5},
			}
			value, callErr := plugin.Engine.Call(t.Context(), "buildQueryRequest", task)
			require.NoError(t, callErr)
			encoded, marshalErr := common.Marshal(value)
			require.NoError(t, marshalErr)
			assert.JSONEq(t, `{"url":"https://upstream.example/v1/videos/a1b2c3d4e5f6789012345678","method":"GET","headers":{"Authorization":"Bearer synthetic-fixture"}}`, string(encoded))

			value, callErr = plugin.Engine.Call(t.Context(), "parseTaskResult", task, map[string]any{
				"status": "completed", "url": "https://cdn.example/result.mp4",
			})
			require.NoError(t, callErr)
			encoded, marshalErr = common.Marshal(value)
			require.NoError(t, marshalErr)
			assert.JSONEq(t, `{"status":"SUCCESS","progress":"100%","url":"https://cdn.example/result.mp4"}`, string(encoded))

			value, callErr = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", task, map[string]any{"status": "SUCCESS"})
			require.NoError(t, callErr)
			encoded, marshalErr = common.Marshal(value)
			require.NoError(t, marshalErr)
			assert.JSONEq(t, `{"seconds":12.5}`, string(encoded))
		})
	}

	t.Run("只路由声明的精确模型", func(t *testing.T) {
		for _, model := range models {
			for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
				binding, found := registry.Generation().LookupEndpoint("POST", endpoint, model)
				require.True(t, found)
				assert.Same(t, plugin, binding.Plugin)
			}
		}
		for _, model := range []string{"Seedance2.0", "无限制-Flash-中配-Video", "MiniMax-H3", "MiniMax-H3-Max"} {
			for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
				_, found := registry.Generation().LookupEndpoint("POST", endpoint, model)
				assert.False(t, found, "目录中的其他模型不能自动开放生成")
			}
		}
	})

	t.Run("旧图片入口转换为官方 content 且保留签名与重复", func(t *testing.T) {
		url := "https://cdn.example/image.png?signature=a%2Bb&expires=123"
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": seedanceModel, "body": map[string]any{"kind": "json", "value": map[string]any{
				"model": seedanceModel, "duration": 6, "images": []any{map[string]any{"url": url, "role": "reference_image"}, url, "data:image/png;base64,aW1hZ2U="},
			}},
		})
		require.NoError(t, callErr)
		var intent map[string]any
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, common.Unmarshal(encoded, &intent))
		req := intent["requestBody"].(map[string]any)
		assert.Equal(t, []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}, "role": "reference_image"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}, "role": "reference_image"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,aW1hZ2U="}, "role": "reference_image"},
		}, req["content"])
		assert.Equal(t, "image_to_video", intent["action"])
	})

	t.Run("FullHD图片DataURL在默认引擎期限内解码并构造提交", func(t *testing.T) {
		frame := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
		for y := range frame.Rect.Dy() {
			for x := range frame.Rect.Dx() {
				offset := frame.PixOffset(x, y)
				frame.Pix[offset] = byte(x)
				frame.Pix[offset+1] = byte(y)
				frame.Pix[offset+2] = byte(x ^ y)
				frame.Pix[offset+3] = 255
			}
		}
		// 无压缩 PNG 保留 Full-HD 素材的实际体积，避免纯色压缩后退化成短字符串用例。
		var imageBytes bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.NoCompression}
		require.NoError(t, encoder.Encode(&imageBytes, frame))
		config, decodeErr := png.DecodeConfig(bytes.NewReader(imageBytes.Bytes()))
		require.NoError(t, decodeErr)
		assert.Equal(t, 1920, config.Width)
		assert.Equal(t, 1080, config.Height)
		dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes.Bytes())

		// 注册保持 Options{}；两个模型及协议均使用真实 Sobek 默认期限，不设置机器相关的毫秒阈值。
		for _, model := range models {
			for _, protocol := range []string{"openai_video", "openai_responses"} {
				t.Run(model+"_"+protocol, func(t *testing.T) {
					body := map[string]any{"model": model, "seconds": 5}
					imageIndex := 0
					if model != seedanceModel {
						imageIndex = 1
						if protocol == "openai_video" {
							body["prompt"] = "An ocean at dawn"
						}
					}
					if protocol == "openai_video" {
						body["images"] = []string{dataURL}
					} else {
						input := []any{}
						if model != seedanceModel {
							input = append(input, map[string]any{"type": "input_text", "text": "An ocean at dawn"})
						}
						input = append(input, map[string]any{"type": "input_image", "image_url": dataURL})
						body["input"] = []any{map[string]any{"role": "user", "content": input}}
					}
					value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
						"model": model, "body": map[string]any{"kind": "json", "value": body},
					})
					require.NoError(t, callErr)
					encoded, marshalErr := common.Marshal(value)
					require.NoError(t, marshalErr)
					var intent map[string]any
					require.NoError(t, common.Unmarshal(encoded, &intent))
					assert.Equal(t, "image_to_video", intent["action"])
					request := intent["requestBody"].(map[string]any)
					content := request["content"].([]any)
					require.Len(t, content, imageIndex+1)
					assert.True(t, content[imageIndex].(map[string]any)["image_url"].(map[string]any)["url"] == dataURL, "解码必须完整保留 Data URL")

					value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
						"model": model, "upstreamModel": model, "requestBody": request,
						"apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example/v1", "publicTaskId": "task_full_hd_fixture",
					})
					require.NoError(t, callErr)
					encoded, marshalErr = common.Marshal(value)
					require.NoError(t, marshalErr)
					var descriptor map[string]any
					require.NoError(t, common.Unmarshal(encoded, &descriptor))
					assert.Equal(t, "https://upstream.example/v1/videos", descriptor["url"])
					assert.Equal(t, true, descriptor["noRetry"])
					sent := descriptor["body"].(map[string]any)
					content = sent["content"].([]any)
					require.Len(t, content, imageIndex+1)
					assert.True(t, content[imageIndex].(map[string]any)["image_url"].(map[string]any)["url"] == dataURL, "提交构造不得裁切或替换 Data URL")
				})
			}
		}
	})

	for _, spec := range []struct{ name, url string }{
		{"内嵌凭据", "https://fixture-user:fixture-password@cdn.example/ref.png"},
		{"控制字符", "https://cdn.example/ref.png?value=\x7f"},
		{"空白", "https://cdn.example/ref image.png"},
		{"反斜杠", `https://cdn.example\ref.png`},
		{"非HTTP协议", "ftp://cdn.example/ref.png"},
	} {
		t.Run("拒绝不安全地址_"+spec.name, func(t *testing.T) {
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": seedanceModel, "body": map[string]any{"kind": "json", "value": map[string]any{
					"seconds": 5, "images": []string{spec.url},
				}},
			})
			require.ErrorContains(t, callErr, "references must be HTTP(S) URLs")
			_, callErr = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{
				"artifactKey": "video", "data": map[string]any{"url": spec.url}, "clientRequest": map[string]any{"method": "GET"},
			})
			require.ErrorContains(t, callErr, "video URL is unavailable")
		})
	}

	t.Run("同名上传字段使用独立文件引用", func(t *testing.T) {
		files := []any{
			map[string]any{"ref": "request_file:images", "field": "images", "filename": "first.png", "mimeType": "image/png", "size": 4},
			map[string]any{"ref": "request_file_index:1:images", "field": "images", "filename": "second.png", "mimeType": "image/png", "size": 6},
		}
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": seedanceModel, "body": map[string]any{"kind": "multipart", "fields": map[string]any{
				"model": []string{seedanceModel}, "seconds": []string{"5"},
			}, "files": files},
		})
		require.NoError(t, callErr)
		var intent map[string]any
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, common.Unmarshal(encoded, &intent))
		value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": seedanceModel, "upstreamModel": seedanceModel, "requestBody": intent["requestBody"], "files": files,
			"apiKey": "fixture", "baseUrl": "https://upstream.example/v1", "publicTaskId": "task_upload_fixture",
		})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		images := descriptor["body"].(map[string]any)["content"].([]any)
		require.Len(t, images, 2)
		first := images[0].(map[string]any)["image_url"].(map[string]any)["url"].(map[string]any)
		second := images[1].(map[string]any)["image_url"].(map[string]any)["url"].(map[string]any)
		assert.Equal(t, "request_file:images", first["__fileRef"])
		assert.Equal(t, "request_file_index:1:images", second["__fileRef"])
		assert.Equal(t, "dataUrl", first["encoding"])
	})

	t.Run("音视频旧入口转换官方 content 且保留顺序角色与签名", func(t *testing.T) {
		video := "https://cdn.example/ref.mp4?signature=a%2Bb"
		audio := "https://cdn.example/ref.mp3?signature=c%2Bd"
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": seedanceModel, "body": map[string]any{"kind": "json", "value": map[string]any{
				"prompt": "An ocean at dawn", "duration": 5,
				"reference_videos": []any{map[string]any{"url": video, "role": "reference_video"}, video},
				"reference_audios": []any{map[string]any{"url": audio, "role": "reference_audio"}, audio, "data:audio/mpeg;base64,YXVkaW8="},
			}},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var intent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &intent))
		request := intent["requestBody"].(map[string]any)
		assert.Equal(t, []any{
			map[string]any{"type": "text", "text": "An ocean at dawn"},
			map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": audio}, "role": "reference_audio"},
			map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": audio}, "role": "reference_audio"},
			map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "data:audio/mpeg;base64,YXVkaW8="}, "role": "reference_audio"},
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": video}, "role": "reference_video"},
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": video}, "role": "reference_video"},
		}, request["content"])
		assert.NotContains(t, request, "reference_audios")
		assert.NotContains(t, request, "reference_videos")
	})

	t.Run("图片音频文件和视频URL转换为官方 content", func(t *testing.T) {
		files := []any{
			map[string]any{"ref": "request_file:images", "field": "images", "filename": "ref.png", "mimeType": "image/png", "size": 5},
			map[string]any{"ref": "request_file:audios", "field": "audios", "filename": "ref.mp3", "mimeType": "audio/mpeg", "size": 7},
		}
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": seedanceModel, "body": map[string]any{"kind": "multipart", "fields": map[string]any{
				"model": []string{seedanceModel}, "seconds": []string{"5"}, "videos": []string{"https://cdn.example/ref.mp4"}, "generate_audio": []string{"false"},
			}, "files": files},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var intent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &intent))
		request := intent["requestBody"].(map[string]any)
		assert.Equal(t, false, request["generate_audio"])
		content := request["content"].([]any)
		require.Len(t, content, 3)
		for index, spec := range []struct{ field, itemType, mime string }{{"images", "image_url", "image/png"}, {"audios", "audio_url", "audio/mpeg"}} {
			item := content[index].(map[string]any)
			placeholder := item[spec.itemType].(map[string]any)["url"].(map[string]any)
			assert.Equal(t, "request_file:"+spec.field, placeholder["__fileRef"])
			assert.Equal(t, "dataUrl", placeholder["encoding"])
			assert.Equal(t, spec.mime, placeholder["mimeType"])
		}
		assert.Equal(t, map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}, "role": "reference_video"}, content[2])
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
		{"未知图片角色", map[string]any{"seconds": 5, "images": []any{map[string]any{"url": "https://cdn.example/one.png", "role": "unknown"}}}},
		{"音频超量不可静默截断", map[string]any{"prompt": "ocean", "seconds": 5, "audios": []string{"https://cdn.example/1.mp3", "https://cdn.example/2.mp3", "https://cdn.example/3.mp3", "https://cdn.example/4.mp3"}}},
		{"视频超量不可静默截断", map[string]any{"prompt": "ocean", "seconds": 5, "videos": []string{"https://cdn.example/1.mp4", "https://cdn.example/2.mp4", "https://cdn.example/3.mp4", "https://cdn.example/4.mp4"}}},
		{"隐藏时长不可绕过", map[string]any{"prompt": "ocean", "seconds": 5, "metadata": map[string]any{"duration": 3601}}},
		{"content 与 prompt 冲突", map[string]any{"prompt": "ocean", "seconds": 5, "content": []any{map[string]any{"type": "text", "text": "ocean"}}}},
		{"不支持的分辨率", map[string]any{"prompt": "ocean", "seconds": 5, "resolution": "2k"}},
		{"不支持的比例", map[string]any{"prompt": "ocean", "seconds": 5, "ratio": "2:3"}},
		{"小数秒", map[string]any{"prompt": "ocean", "seconds": 5.5}},
		{"小于四秒", map[string]any{"prompt": "ocean", "seconds": 3}},
		{"大于十五秒", map[string]any{"prompt": "ocean", "seconds": 16}},
		{"字符串布尔值", map[string]any{"prompt": "ocean", "seconds": 5, "watermark": "false"}},
		{"数字布尔值", map[string]any{"prompt": "ocean", "seconds": 5, "generate_audio": 0}},
		{"非布尔尾帧开关", map[string]any{"prompt": "ocean", "seconds": 5, "return_last_frame": nil}},
		{"内联时长绕过", map[string]any{"prompt": "ocean --dur 30", "seconds": 5}},
		{"视频DataURL", map[string]any{"prompt": "ocean", "seconds": 5, "videos": []string{"data:video/mp4;base64,dmlkZW8="}}},
		{"纯音频参考", map[string]any{"prompt": "ocean", "seconds": 5, "audios": []string{"https://cdn.example/audio.mp3"}}},
		{"未适配高级回调", map[string]any{"prompt": "ocean", "seconds": 5, "callback_url": "https://callback.example"}},
		{"未适配样片模式", map[string]any{"prompt": "ocean", "seconds": 5, "draft": true}},
		{"未适配编辑模式", map[string]any{"prompt": "ocean", "seconds": 5, "omni_reference_task_type": "edit"}},
		{"非2.0种子字段", map[string]any{"prompt": "ocean", "seconds": 5, "seed": 0}},
		{"仅空文本", map[string]any{"seconds": 5, "content": []any{map[string]any{"type": "text", "text": ""}}}},
		{"官方content与旧引用冲突", map[string]any{"seconds": 5, "content": []any{map[string]any{"type": "text", "text": "ocean"}}, "images": []string{"https://cdn.example/ref.png"}}},
		{"尾帧缺少首帧", map[string]any{"seconds": 5, "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/last.png"}, "role": "last_frame"}}}},
		{"首帧与参考图混用", map[string]any{"seconds": 5, "content": []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/first.png"}, "role": "first_frame"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/ref.png"}, "role": "reference_image"},
		}}},
		{"首帧与视频混用", map[string]any{"seconds": 5, "content": []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/first.png"}, "role": "first_frame"},
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}, "role": "reference_video"},
		}}},
		{"官方引用嵌套多余字段", map[string]any{"seconds": 5, "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/ref.png", "strength": 0.5}, "role": "reference_image"}}}},
		{"官方视频错误角色", map[string]any{"seconds": 5, "content": []any{map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}, "role": "reference_image"}}}},
		{"图片超量不可静默截断", map[string]any{"seconds": 5, "images": []string{"https://cdn.example/1.png", "https://cdn.example/2.png", "https://cdn.example/3.png", "https://cdn.example/4.png", "https://cdn.example/5.png", "https://cdn.example/6.png", "https://cdn.example/7.png", "https://cdn.example/8.png", "https://cdn.example/9.png", "https://cdn.example/10.png"}}},
	} {
		t.Run(spec.name, func(t *testing.T) {
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": seedanceModel, "body": map[string]any{"kind": "json", "value": spec.body},
			})
			require.Error(t, callErr)
			_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"model": seedanceModel, "requestBody": spec.body,
				"baseUrl": "https://upstream.example", "apiKey": "synthetic-fixture", "publicTaskId": "task_invalid_fixture",
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

// TestImage2ProH3VideoContracts 验证精确 H3 别名的官方输入、网关输出边界和旧任务恢复，不发送上游请求。
func TestImage2ProH3VideoContracts(t *testing.T) {
	source, err := plugins.Source("image2pro")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	const model = "无限制-Flash-MAX-Video"
	text := map[string]any{"type": "text", "text": "A singer follows the reference voice"}
	imageURL := "https://cdn.example/ref.png?signature=a%2Bb"
	for _, spec := range []struct {
		name, ratio, action string
		content             []any
	}{
		{"纯文本默认固定比例", "16:9", "text_to_video", []any{text}},
		{"H3参数样式文本保持字面值", "16:9", "text_to_video", []any{map[string]any{"type": "text", "text": "Display --duration 12 and --seed 42 as captions"}}},
		{"首帧默认自适应", "adaptive", "image_to_video", []any{text, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "first_frame"}}},
		{"首尾帧保留顺序", "adaptive", "image_to_video", []any{text,
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "first_frame"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "last_frame"}}},
		{"纯音频参考默认自适应", "adaptive", "reference_to_video", []any{text, map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "data:audio/wav;base64,YXVkaW8="}, "role": "reference_audio"}}},
		{"全模态跨媒体顺序与重复保留", "adaptive", "reference_to_video", []any{text,
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": "data:video/mp4;base64,dmlkZW8="}, "role": "reference_video"},
			map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "https://cdn.example/voice.mp3"}, "role": "reference_audio"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "reference_image"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "reference_image"}}},
	} {
		t.Run(spec.name, func(t *testing.T) {
			for _, duration := range []int{4, 12} {
				value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": "my-h3", "upstreamModel": model, "body": map[string]any{"kind": "json", "value": map[string]any{
						"model": "my-h3", "duration": duration, "content": spec.content, "client_request_id": "h3-stable-0001",
					}},
				})
				require.NoError(t, callErr)
				encoded, marshalErr := common.Marshal(value)
				require.NoError(t, marshalErr)
				var intent map[string]any
				require.NoError(t, common.Unmarshal(encoded, &intent))
				assert.Equal(t, "my-h3", intent["model"])
				assert.Equal(t, spec.action, intent["action"])
				value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"model": "my-h3", "upstreamModel": model, "requestBody": intent["requestBody"],
					"apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example/v1", "publicTaskId": "task_h3_fixture",
				})
				require.NoError(t, callErr)
				encoded, marshalErr = common.Marshal(value)
				require.NoError(t, marshalErr)
				var descriptor map[string]any
				require.NoError(t, common.Unmarshal(encoded, &descriptor))
				assert.Equal(t, "https://upstream.example/v1/videos", descriptor["url"])
				assert.Equal(t, true, descriptor["noRetry"])
				assert.Equal(t, "h3-stable-0001", descriptor["headers"].(map[string]any)["Idempotency-Key"])
				sent := descriptor["body"].(map[string]any)
				assert.Equal(t, model, sent["model"])
				assert.Equal(t, "720p", sent["resolution"])
				assert.Equal(t, float64(duration), sent["duration"])
				assert.Equal(t, spec.ratio, sent["ratio"])
				assert.Equal(t, spec.content, sent["content"])
				assert.NotContains(t, sent, "client_request_id")
				assert.NotContains(t, sent, "clientRequestId")
			}
		})
	}

	t.Run("multipart按MIME和单文件上限内联", func(t *testing.T) {
		files := []any{
			map[string]any{"ref": "request_file:images", "field": "images", "filename": "ref.heic", "mimeType": "image/heic", "size": 6},
			map[string]any{"ref": "request_file:videos", "field": "videos", "filename": "ref.mp4", "mimeType": "application/octet-stream", "size": 9},
			map[string]any{"ref": "request_file:audios", "field": "audios", "filename": "voice.wav", "mimeType": "audio/wav", "size": 12},
		}
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": model, "body": map[string]any{"kind": "multipart", "fields": map[string]any{
				"model": []string{model}, "duration": []string{"5"}, "prompt": []string{"A singer follows the reference voice"},
			}, "files": files},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var intent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &intent))
		value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": model, "requestBody": intent["requestBody"], "files": files, "apiKey": "synthetic-fixture",
			"baseUrl": "https://upstream.example", "publicTaskId": "task_h3_multipart",
		})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		var descriptor map[string]any
		require.NoError(t, common.Unmarshal(encoded, &descriptor))
		content := descriptor["body"].(map[string]any)["content"].([]any)
		for index, spec := range []struct {
			kind, ref, mime string
			maximum         int
		}{
			{"image_url", "request_file:images", "image/heic", 30 * 1024 * 1024},
			{"audio_url", "request_file:audios", "audio/wav", 15 * 1024 * 1024},
			{"video_url", "request_file:videos", "video/mp4", 50 * 1024 * 1024},
		} {
			placeholder := content[index+1].(map[string]any)[spec.kind].(map[string]any)["url"].(map[string]any)
			assert.Equal(t, spec.ref, placeholder["__fileRef"])
			assert.Equal(t, spec.mime, placeholder["mimeType"])
			assert.Equal(t, float64(spec.maximum), placeholder["maxBytes"])
		}
	})

	for _, spec := range []struct {
		name, field, mime string
		size              int
	}{
		{"图片单文件超限", "images", "image/png", 30*1024*1024 + 1},
		{"视频单文件超限", "videos", "video/mp4", 50*1024*1024 + 1},
		{"音频单文件超限", "audios", "audio/wav", 15*1024*1024 + 1},
		{"GIF不可继承Seedance格式", "images", "image/gif", 5},
		{"MOV不可内联", "videos", "video/quicktime", 5},
	} {
		t.Run(spec.name, func(t *testing.T) {
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": model, "body": map[string]any{"kind": "multipart", "fields": map[string]any{"duration": []string{"5"}, "prompt": []string{"ocean"}}, "files": []any{
					map[string]any{"ref": "request_file:" + spec.field, "field": spec.field, "filename": "ref.bin", "mimeType": spec.mime, "size": spec.size},
				}},
			})
			require.Error(t, callErr)
		})
	}

	t.Run("Base64膨胀计入64MB最终请求而非只算文件字节", func(t *testing.T) {
		for _, size := range []int{48*1024*1024 - 4096, 48 * 1024 * 1024} {
			files := []any{map[string]any{"ref": "request_file:videos", "field": "videos", "mimeType": "video/mp4", "size": size}}
			_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"model": model, "requestBody": map[string]any{"duration": 5, "prompt": "海边🎵", "videos": []any{
					map[string]any{"__fileRef": "request_file:videos", "encoding": "dataUrl", "mimeType": "video/mp4"},
				}}, "files": files, "apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example", "publicTaskId": "task_h3_body_limit",
			})
			if size < 48*1024*1024 {
				require.NoError(t, callErr)
			} else {
				require.ErrorContains(t, callErr, "64 MB")
			}
		}
	})

	t.Run("UTF8与宿主JSON转义计入最终正文", func(t *testing.T) {
		stub, marshalErr := common.Marshal(map[string]any{
			"model": model, "duration": 5, "resolution": "720p", "ratio": "adaptive", "content": []any{
				map[string]any{"type": "text", "text": "aaaa"},
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "data:video/mp4;base64,"}, "role": "reference_video"},
			},
		})
		require.NoError(t, marshalErr)
		// 让 ASCII 正文距上限不足四字节；Unicode 或宿主转义的额外字节会跨过正文上限。
		size := ((64*1024*1024 - len(stub)) / 4) * 3
		files := []any{map[string]any{"ref": "request_file:videos", "field": "videos", "mimeType": "video/mp4", "size": size}}
		for _, prompt := range []string{"aaaa", "海边🎵", "<>&", "a\u2028\u2029"} {
			_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"model": model, "requestBody": map[string]any{"duration": 5, "prompt": prompt, "videos": []any{
					map[string]any{"__fileRef": "request_file:videos", "encoding": "dataUrl", "mimeType": "video/mp4"},
				}}, "files": files, "apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example", "publicTaskId": "task_h3_utf8_limit",
			})
			if prompt == "aaaa" {
				require.NoError(t, callErr)
			} else {
				require.ErrorContains(t, callErr, "64 MB")
			}
		}
	})

	t.Run("九图三视频三音频上限与纯音频固定比例", func(t *testing.T) {
		content := []any{text}
		for _, spec := range []struct {
			kind, role, url string
			count           int
		}{
			{"image_url", "reference_image", "https://cdn.example/ref.heic", 9},
			{"video_url", "reference_video", "https://cdn.example/ref.mov", 3},
			{"audio_url", "reference_audio", "https://cdn.example/ref.mp3", 3},
		} {
			for range spec.count {
				content = append(content, map[string]any{"type": spec.kind, spec.kind: map[string]any{"url": spec.url}, "role": spec.role})
			}
		}
		for _, references := range [][]any{content, {text, map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}, "role": "reference_audio"}}} {
			value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"model": model, "requestBody": map[string]any{"duration": 5, "ratio": "1:1", "content": references},
				"apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example", "publicTaskId": "task_h3_reference_limits",
			})
			require.NoError(t, callErr)
			encoded, marshalErr := common.Marshal(value)
			require.NoError(t, marshalErr)
			var descriptor map[string]any
			require.NoError(t, common.Unmarshal(encoded, &descriptor))
			assert.Equal(t, "1:1", descriptor["body"].(map[string]any)["ratio"])
			assert.Equal(t, references, descriptor["body"].(map[string]any)["content"])
		}
	})

	for _, spec := range []struct {
		name string
		body map[string]any
	}{
		{"小于四秒", map[string]any{"prompt": "ocean", "duration": 3}},
		{"大于十二秒", map[string]any{"prompt": "ocean", "duration": 13}},
		{"小数秒", map[string]any{"prompt": "ocean", "duration": 5.5}},
		{"2K不可替代720p", map[string]any{"prompt": "ocean", "duration": 5, "resolution": "2K"}},
		{"768P不可替代720p", map[string]any{"prompt": "ocean", "duration": 5, "resolution": "768P"}},
		{"文生不可自适应", map[string]any{"prompt": "ocean", "duration": 5, "ratio": "adaptive"}},
		{"帧不可固定比例", map[string]any{"duration": 5, "ratio": "16:9", "content": []any{text, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "first_frame"}}}},
		{"首帧必须提示词", map[string]any{"duration": 5, "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "first_frame"}}}},
		{"每条文本不超过7000字符", map[string]any{"duration": 5, "prompt": strings.Repeat("a", 7001)}},
		{"尾帧仍需首帧", map[string]any{"duration": 5, "content": []any{text, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}, "role": "last_frame"}}}},
		{"GIFDataURL不可继承", map[string]any{"duration": 5, "prompt": "ocean", "images": []string{"data:image/gif;base64,aW1hZ2U="}}},
		{"MOVDataURL不可内联", map[string]any{"duration": 5, "prompt": "ocean", "videos": []string{"data:video/quicktime;base64,dmlkZW8="}}},
		{"不可继承Seedance音频开关", map[string]any{"duration": 5, "prompt": "ocean", "generate_audio": false}},
		{"不可继承Seedance水印开关", map[string]any{"duration": 5, "prompt": "ocean", "watermark": false}},
		{"不可继承Seedance尾帧开关", map[string]any{"duration": 5, "prompt": "ocean", "return_last_frame": false}},
		{"水印未适配", map[string]any{"duration": 5, "prompt": "ocean", "aigc_watermark": false}},
		{"回调未适配", map[string]any{"duration": 5, "prompt": "ocean", "callback_url": "https://callback.example"}},
	} {
		t.Run(spec.name, func(t *testing.T) {
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": model, "body": map[string]any{"kind": "json", "value": spec.body},
			})
			require.Error(t, callErr)
			_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"model": model, "requestBody": spec.body, "apiKey": "synthetic-fixture",
				"baseUrl": "https://upstream.example", "publicTaskId": "task_h3_invalid",
			})
			require.Error(t, callErr)
		})
	}

	t.Run("恢复旧MAX任务时不套用新的整秒上限", func(t *testing.T) {
		task := map[string]any{"model": "my-h3", "upstreamModel": model, "taskId": "a1b2c3d4e5f6789012345678", "state": map[string]any{"seconds": 12.5}, "apiKey": "synthetic-fixture", "baseUrl": "https://upstream.example/v1"}
		value, callErr := plugin.Engine.Call(t.Context(), "buildQueryRequest", task)
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"url":"https://upstream.example/v1/videos/a1b2c3d4e5f6789012345678","method":"GET","headers":{"Authorization":"Bearer synthetic-fixture"}}`, string(encoded))
		value, callErr = plugin.Engine.Call(t.Context(), "parseTaskResult", task, map[string]any{"status": "completed", "url": "https://cdn.example/result.mp4"})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"status":"SUCCESS","progress":"100%","url":"https://cdn.example/result.mp4"}`, string(encoded))
		value, callErr = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", task, map[string]any{"status": "SUCCESS"}, map[string]any{"duration": 99})
		require.NoError(t, callErr)
		encoded, marshalErr = common.Marshal(value)
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"seconds":12.5}`, string(encoded))
	})
}
