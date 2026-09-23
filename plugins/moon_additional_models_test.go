package plugins_test

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMoonAdditionalModelsContracts 在本地 JS 运行时校验新增模型的路由、请求与用量，不调用 Moon 上游。
func TestMoonAdditionalModelsContracts(t *testing.T) {
	source, err := plugins.Source("moon")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)

	callObject := func(t *testing.T, hook string, args ...any) map[string]any {
		t.Helper()
		value, callErr := plugin.Engine.Call(t.Context(), hook, args...)
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var result map[string]any
		require.NoError(t, common.Unmarshal(encoded, &result))
		return result
	}
	submit := func(t *testing.T, model string, body map[string]any) map[string]any {
		t.Helper()
		return callObject(t, "buildSubmitRequest", map[string]any{
			"baseUrl": "https://moon.example/proxy/v1/", "apiKey": "fixture", "publicTaskId": "task-additional",
			"model": "local-alias", "upstreamModel": model, "requestBody": body,
		})
	}

	models := []struct {
		id, billing, resolution string
		seconds                 int
	}{
		{"seedance-2-5-official", "token", "1080p", 30},
		{"seedance2.0-9-3-3-PT", "second", "480p", 15},
		{"seedance2.5-30-10-10-PT", "second", "720p", 30},
		{"seedance2.0-fast-PT", "second", "720p", 15},
		{"sd2mini", "count", "480p", 15},
		{"sd2-930-face", "count", "720p", 30},
		{"sd2.5-30-10-face", "count", "720p", 30},
		{"sd2-930-fast", "second", "720p", 15},
		{"sd2.5-30-10-10-480", "second", "480p", 30},
		{"sd2.5-30-10-10", "second", "720p", 30},
		{"sd2-930-no-face", "count", "720p", 15},
		{"sd2.5-30-10-10-per-request", "count", "720p", 30},
	}
	t.Run("十二个精确ID注册并路由到Moon", func(t *testing.T) {
		require.Len(t, models, 12)
		require.NotNil(t, plugin.Meta.ModelDiscovery)
		assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
		for _, tc := range models {
			t.Run(tc.id, func(t *testing.T) {
				assert.Contains(t, plugin.Meta.Models, tc.id)
				canonical, found := registry.Generation().CanonicalModel(tc.id)
				require.True(t, found)
				assert.Equal(t, tc.id, canonical)
				for _, path := range []string{"/v1/videos", "/v1/responses"} {
					candidates := registry.Generation().LookupEndpointCandidates("POST", path, tc.id)
					require.Len(t, candidates, 1, path)
					assert.Equal(t, "moon", candidates[0].Plugin.Meta.Key)
				}
				schema, _ := plugin.Meta.UsageForModel(tc.id)
				require.Contains(t, schema, "resolution")
				for _, field := range []string{"image_input_count", "video_input_count"} {
					assert.Equal(t, "count", schema[field].Unit)
					assert.NotEmpty(t, schema[field].Description["en"])
					assert.NotEmpty(t, schema[field].Description["zh"])
				}
				switch tc.billing {
				case "token":
					assert.Equal(t, "token", schema["tokens"].Unit)
				case "second":
					assert.Equal(t, "second", schema["seconds"].Unit)
				case "count":
					assert.Equal(t, "count", schema["video_count"].Unit)
				}
			})
		}
		assert.NotContains(t, plugin.Meta.Models, "seedance2.0-fast-pt")
	})

	t.Run("十二个模型解码并提交原样模型ID", func(t *testing.T) {
		for _, tc := range models {
			t.Run(tc.id, func(t *testing.T) {
				body := map[string]any{"model": tc.id, "prompt": "ocean", "resolution": tc.resolution, "ratio": "16:9"}
				if strings.HasPrefix(tc.id, "sd2") {
					body["seconds"] = tc.seconds
				} else {
					body["duration"] = tc.seconds
				}
				value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": tc.id, "body": map[string]any{"kind": "json", "value": body},
				})
				require.NoError(t, callErr)
				encoded, marshalErr := common.Marshal(value)
				require.NoError(t, marshalErr)
				var decoded struct {
					Action      string         `json:"action"`
					RequestBody map[string]any `json:"requestBody"`
				}
				require.NoError(t, common.Unmarshal(encoded, &decoded))
				assert.Equal(t, "text_to_video", decoded.Action)
				request := submit(t, tc.id, decoded.RequestBody)
				assert.Equal(t, "POST", request["method"])
				assert.Equal(t, "https://moon.example/proxy/v1/videos", request["url"])
				assert.Equal(t, true, request["noRetry"])
				headers, ok := request["headers"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "task-additional", headers["Idempotency-Key"])
				assert.Equal(t, "application/json", headers["Content-Type"])
				upstreamBody, ok := request["body"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.id, upstreamBody["model"])
				assert.Equal(t, "ocean", upstreamBody["prompt"])
				assert.Equal(t, tc.resolution, upstreamBody["resolution"])
				assert.Equal(t, "16:9", upstreamBody["ratio"])
				seconds := upstreamBody["duration"]
				if seconds == nil {
					seconds = upstreamBody["seconds"]
				}
				assert.Equal(t, float64(tc.seconds), seconds)
				assert.NotContains(t, upstreamBody, "price")
				assert.NotContains(t, upstreamBody, "api_key")
			})
		}
		query := callObject(t, "buildQueryRequest", map[string]any{
			"baseUrl": "https://moon.example/proxy/v1", "apiKey": "fixture", "taskId": "task/a?b",
		})
		assert.Equal(t, "GET", query["method"])
		assert.Equal(t, "https://moon.example/proxy/v1/videos/task%2Fa%3Fb", query["url"])
	})

	t.Run("用量区分Token按秒与按次且不采信上游积分", func(t *testing.T) {
		for _, tc := range models {
			t.Run(tc.id, func(t *testing.T) {
				body := map[string]any{"prompt": "ocean", "duration": tc.seconds, "resolution": tc.resolution}
				facts := callObject(t, "extractUsage", map[string]any{
					"upstreamModel": tc.id, "usagePurpose": "facts", "requestBody": body,
				})
				assert.Equal(t, tc.resolution, facts["resolution"])
				assert.NotContains(t, facts, "credits")
				switch tc.billing {
				case "token":
					assert.Equal(t, float64(1458000), facts["tokens"])
					assert.Equal(t, "none", facts["video_input"])
					assert.NotContains(t, facts, "seconds")
				case "second":
					assert.Equal(t, float64(tc.seconds), facts["seconds"])
					assert.NotContains(t, facts, "tokens")
				case "count":
					assert.Equal(t, float64(1), facts["video_count"])
					assert.NotContains(t, facts, "tokens")
				}
				ratios, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
					"upstreamModel": tc.id, "usagePurpose": "billing_ratios", "requestBody": body,
				})
				require.NoError(t, callErr)
				assert.Nil(t, ratios)
			})
		}
		actual := callObject(t, "extractUsageOnComplete", map[string]any{"upstreamModel": "seedance-2-5-official"},
			map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"total_tokens": 321}, "status": "completed"})
		assert.Equal(t, map[string]any{"tokens": float64(321)}, actual)
		pending := callObject(t, "parseTaskResult", map[string]any{"upstreamModel": "seedance-2-5-official"},
			map[string]any{"status": "completed"})
		assert.Equal(t, "IN_PROGRESS", pending["status"])
	})

	t.Run("官转视频参考只计输出秒数且Fast禁视频", func(t *testing.T) {
		body := map[string]any{"prompt": "@Video1 camera move", "duration": 5, "resolution": "720p",
			"reference_videos": []any{map[string]any{"url": "https://cdn.example/clip.mp4", "durationSeconds": 5}},
		}
		request := submit(t, "seedance2.5-30-10-10-PT", body)
		upstreamBody, ok := request["body"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "seedance2.5-30-10-10-PT", upstreamBody["model"])
		assert.Equal(t, []any{map[string]any{"url": "https://cdn.example/clip.mp4", "durationSeconds": float64(5)}}, upstreamBody["reference_videos"])
		facts := callObject(t, "extractUsage", map[string]any{
			"upstreamModel": "seedance2.5-30-10-10-PT", "usagePurpose": "facts", "requestBody": body,
		})
		assert.Equal(t, float64(5), facts["seconds"])
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"upstreamModel": "seedance2.0-fast-PT", "baseUrl": "https://moon.example/v1", "apiKey": "fixture",
			"publicTaskId": "task-fast", "requestBody": body,
		})
		require.Error(t, err)
	})

	t.Run("PT content 视频时长原样转发且只计输出秒数", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "text", "text": "@Video1 camera move"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/scene.jpg"}, "role": "reference_image"},
			map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/clip.mp4"}, "role": "reference_video", "durationSeconds": 5},
		}
		body := map[string]any{"content": content, "duration": 5, "resolution": "720p"}
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "seedance2.5-30-10-10-PT", "body": map[string]any{"kind": "json", "value": body},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var decoded struct {
			Action      string         `json:"action"`
			RequestBody map[string]any `json:"requestBody"`
		}
		require.NoError(t, common.Unmarshal(encoded, &decoded))
		assert.Equal(t, "reference_to_video", decoded.Action)
		request := submit(t, "seedance2.5-30-10-10-PT", decoded.RequestBody)
		upstreamBody, ok := request["body"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "seedance2.5-30-10-10-PT", upstreamBody["model"])
		items, ok := upstreamBody["content"].([]any)
		require.True(t, ok)
		require.Len(t, items, 3)
		video, ok := items[2].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "reference_video", video["role"])
		assert.Equal(t, float64(5), video["durationSeconds"])
		facts := callObject(t, "extractUsage", map[string]any{
			"upstreamModel": "seedance2.5-30-10-10-PT", "usagePurpose": "facts", "requestBody": decoded.RequestBody,
		})
		assert.Equal(t, float64(5), facts["seconds"])
		_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"upstreamModel": "seedance2.0-fast-PT", "baseUrl": "https://moon.example/v1", "apiKey": "fixture",
			"publicTaskId": "task-fast-content", "requestBody": body,
		})
		require.Error(t, callErr)
	})

	t.Run("底价允许的素材按原顺序转发", func(t *testing.T) {
		images := []any{"https://cdn.example/first.jpg", "https://cdn.example/second.jpg"}
		body := map[string]any{"prompt": "@图片1 @图片2", "seconds": 4, "resolution": "480p",
			"images": images,
		}
		request := submit(t, "sd2.5-30-10-10-480", body)
		upstreamBody, ok := request["body"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, []any{images[0], images[1]}, upstreamBody["images"])
		assert.Equal(t, "reference_to_video", request["action"])
	})

	t.Run("Seedance下界、自动时长与纯音频可提交", func(t *testing.T) {
		for _, tc := range []struct {
			model      string
			body       map[string]any
			wantAmount float64
			usageField string
		}{
			{"seedance-2-5-official", map[string]any{"prompt": "ocean", "duration": 4, "resolution": "720p"}, 86400, "tokens"},
			{"seedance-2-5-official", map[string]any{"prompt": "ocean", "duration": 30, "resolution": "1080p"}, 1458000, "tokens"},
			{"seedance2.0-fast-PT", map[string]any{"prompt": "ocean", "duration": 5, "resolution": "480p"}, 5, "seconds"},
			{"seedance2.5-30-10-10-PT", map[string]any{"prompt": "ocean", "duration": 5, "resolution": "480p"}, 5, "seconds"},
		} {
			t.Run(tc.model, func(t *testing.T) {
				request := submit(t, tc.model, tc.body)
				upstreamBody, ok := request["body"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.model, upstreamBody["model"])
				facts := callObject(t, "extractUsage", map[string]any{
					"upstreamModel": tc.model, "usagePurpose": "facts", "requestBody": tc.body,
				})
				assert.Equal(t, tc.wantAmount, facts[tc.usageField])
			})
		}
		autoDurationBody := map[string]any{
			"prompt": "ocean", "duration": -1, "resolution": "720p",
			"audios": []any{map[string]any{"url": "https://cdn.example/voice.mp3", "role": "reference_audio"}},
		}
		request := submit(t, "seedance-2-5-official", autoDurationBody)
		assert.Equal(t, "reference_to_video", request["action"])
		upstreamBody, ok := request["body"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(-1), upstreamBody["duration"])
		assert.Equal(t, autoDurationBody["audios"], upstreamBody["audios"])
		assert.NotContains(t, upstreamBody, "videos")
		facts := callObject(t, "extractUsage", map[string]any{
			"upstreamModel": "seedance-2-5-official", "usagePurpose": "facts",
			"requestBody": autoDurationBody,
		})
		assert.Equal(t, float64(648000), facts["tokens"])
		assert.Equal(t, "720p", facts["resolution"])
		assert.Equal(t, "none", facts["video_input"])
	})

	t.Run("八款底价模型分辨率和时长分档", func(t *testing.T) {
		cases := []struct {
			model, resolution, usageField string
			minSeconds, maxSeconds        int
		}{
			{"sd2mini", "480p", "video_count", 5, 15},
			{"sd2mini", "720p", "video_count", 5, 12},
			{"sd2-930-face", "720p", "video_count", 4, 30},
			{"sd2.5-30-10-face", "720p", "video_count", 4, 30},
			{"sd2-930-fast", "720p", "seconds", 5, 15},
			{"sd2.5-30-10-10-480", "480p", "seconds", 4, 30},
			{"sd2.5-30-10-10", "720p", "seconds", 4, 30},
			{"sd2-930-no-face", "720p", "video_count", 4, 15},
			{"sd2.5-30-10-10-per-request", "720p", "video_count", 5, 30},
		}
		for _, tc := range cases {
			t.Run(tc.model+"/"+tc.resolution, func(t *testing.T) {
				for _, seconds := range []int{tc.minSeconds, tc.maxSeconds} {
					body := map[string]any{"prompt": "ocean", "seconds": seconds, "resolution": tc.resolution}
					request := submit(t, tc.model, body)
					upstreamBody, ok := request["body"].(map[string]any)
					require.True(t, ok)
					assert.Equal(t, float64(seconds), upstreamBody["seconds"])
					facts := callObject(t, "extractUsage", map[string]any{
						"upstreamModel": tc.model, "usagePurpose": "facts", "requestBody": body,
					})
					want := float64(seconds)
					if tc.usageField == "video_count" {
						want = 1
					}
					assert.Equal(t, want, facts[tc.usageField])
					assert.Equal(t, tc.resolution, facts["resolution"])
				}
				for _, seconds := range []int{tc.minSeconds - 1, tc.maxSeconds + 1} {
					_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
						"upstreamModel": tc.model, "baseUrl": "https://moon.example/v1", "apiKey": "fixture", "publicTaskId": "task-invalid",
						"requestBody": map[string]any{"prompt": "ocean", "seconds": seconds, "resolution": tc.resolution},
					})
					require.Error(t, callErr, "%s %s %d seconds", tc.model, tc.resolution, seconds)
				}
				if tc.model == "sd2mini" {
					return
				}
				otherResolution := "480p"
				if tc.resolution == "480p" {
					otherResolution = "720p"
				}
				_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"upstreamModel": tc.model, "baseUrl": "https://moon.example/v1", "apiKey": "fixture", "publicTaskId": "task-invalid",
					"requestBody": map[string]any{"prompt": "ocean", "seconds": tc.minSeconds, "resolution": otherResolution},
				})
				require.Error(t, callErr, "%s does not support %s", tc.model, otherResolution)
			})
		}
	})

	t.Run("关键输入越界必须本地拒绝", func(t *testing.T) {
		cases := []struct {
			name, model string
			body        map[string]any
		}{
			{"2.5 Token不支持480p", "seedance-2-5-official", map[string]any{"resolution": "480p"}},
			{"2.5 Token最多30秒", "seedance-2-5-official", map[string]any{"duration": 31}},
			{"PT不接受4秒", "seedance2.0-9-3-3-PT", map[string]any{"duration": 4}},
			{"2.0 PT最多15秒", "seedance2.0-9-3-3-PT", map[string]any{"duration": 16}},
			{"2.5 PT最多30秒", "seedance2.5-30-10-10-PT", map[string]any{"duration": 31}},
			{"PT只支持480p与720p", "seedance2.5-30-10-10-PT", map[string]any{"resolution": "1080p"}},
			{"PT参考视频必须有时长", "seedance2.0-9-3-3-PT", map[string]any{"reference_videos": []any{map[string]any{"url": "https://cdn.example/clip.mp4"}}}},
			{"Fast PT禁视频参考", "seedance2.0-fast-PT", map[string]any{"reference_videos": []any{map[string]any{"url": "https://cdn.example/clip.mp4", "durationSeconds": 5}}}},
			{"Fast PT提示词最多4000字", "seedance2.0-fast-PT", map[string]any{"prompt": strings.Repeat("x", 4001)}},
			{"Face不接受视频", "sd2-930-face", map[string]any{"videos": []any{"https://cdn.example/clip.mp4"}}},
			{"2.5 Face不接受视频", "sd2.5-30-10-face", map[string]any{"videos": []any{"https://cdn.example/clip.mp4"}}},
			{"底价不支持21比9", "sd2.5-30-10-10", map[string]any{"ratio": "21:9"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				body := map[string]any{"prompt": "ocean", "duration": 5, "resolution": "720p"}
				if tc.model == "sd2.5-30-10-10-480" {
					body["resolution"] = "480p"
				}
				for key, value := range tc.body {
					body[key] = value
				}
				_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"upstreamModel": tc.model, "baseUrl": "https://moon.example/v1", "apiKey": "fixture",
					"publicTaskId": "task-invalid", "requestBody": body,
				})
				require.Error(t, callErr)
			})
		}
	})

	t.Run("既有模型基础计费事实", func(t *testing.T) {
		for _, tc := range []struct {
			model string
			body  map[string]any
			want  map[string]any
		}{
			{"seedance-2-0-mini-official", map[string]any{"prompt": "ocean", "duration": 5}, map[string]any{"tokens": float64(108000), "resolution": "720p", "video_input": "none", "image_input_count": float64(0), "video_input_count": float64(0)}},
			{"wan3.0-video", map[string]any{"prompt": "ocean", "duration": 5}, map[string]any{"seconds": float64(5), "resolution": "720P", "image_input_count": float64(0), "video_input_count": float64(0)}},
			{"grok-v1.5-video", map[string]any{"prompt": "ocean", "seconds": 6}, map[string]any{"seconds": float64(6), "resolution": "720p", "image_input_count": float64(0), "video_input_count": float64(0)}},
			{"minimax-h3", map[string]any{"prompt": "ocean", "workflow_id": "text-to-video", "seconds": 5, "size": "1376x768"}, map[string]any{"seconds": float64(5), "resolution": "768p", "image_input_count": float64(0), "video_input_count": float64(0)}},
		} {
			t.Run(tc.model, func(t *testing.T) {
				assert.Equal(t, tc.want, callObject(t, "extractUsage", map[string]any{
					"upstreamModel": tc.model, "usagePurpose": "facts", "requestBody": tc.body,
				}))
			})
		}
	})

	t.Run("引用素材按请求条目直接计数", func(t *testing.T) {
		image := "https://cdn.example/ref.jpg"
		video := "https://cdn.example/ref.mp4"
		for _, tc := range []struct {
			name, model string
			body        map[string]any
			images      float64
			videos      float64
		}{
			{"Seedance content", "seedance-2-5-official", map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "ocean"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": image}, "role": "reference_image"},
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": video}, "role": "reference_video"},
			}}, 1, 1},
			{"Seedance 首尾帧", "seedance-2-5-official", map[string]any{"prompt": "ocean", "ratio": "adaptive", "first_frame": image, "last_frame": image}, 2, 0},
			{"Wan 文件 ID 和文档", "wan3.0-video", map[string]any{
				"prompt": "ocean", "reference_images": []any{map[string]any{"file_id": "file-fixture"}, map[string]any{"url": image}},
				"reference_videos": []any{map[string]any{"url": video, "duration": 3}},
				"input":            map[string]any{"media": []any{map[string]any{"type": "link", "url": "https://example.com/article"}}},
			}, 2, 1},
			{"PT content", "seedance2.5-30-10-10-PT", map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "ocean"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": image}, "role": "reference_image"},
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": video}, "role": "reference_video", "durationSeconds": 5},
			}}, 1, 1},
			{"PT materials", "seedance2.0-9-3-3-PT", map[string]any{"prompt": "ocean", "materials": []any{
				map[string]any{"type": "image", "url": image},
				map[string]any{"type": "video", "url": video, "durationSeconds": 5},
			}}, 1, 1},
			{"底价别名重复引用不去重", "sd2.5-30-10-10", map[string]any{"prompt": "ocean", "images": []any{image}, "image_refs": []any{image}, "videos": []any{video}, "video_refs": []any{video}}, 2, 2},
			{"H3 单数和复数素材", "minimax-h3", map[string]any{"prompt": "ocean", "workflow_id": "multi-reference", "seconds": 5, "size": "1376x768", "images": []any{image, image}, "input_reference": image, "reference_video": video}, 3, 1},
			{"Grok 仅图片", "grok-v1.5-video", map[string]any{"prompt": "ocean", "reference_images": []any{map[string]any{"url": image}, map[string]any{"url": image}}}, 2, 0},
			{"纯文生视频零输入", "sd2-930-fast", map[string]any{"prompt": "ocean"}, 0, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				schema, _ := plugin.Meta.UsageForModel(tc.model)
				assert.Equal(t, "count", schema["image_input_count"].Unit)
				assert.Equal(t, "count", schema["video_input_count"].Unit)
				facts := callObject(t, "extractUsage", map[string]any{
					"upstreamModel": tc.model, "usagePurpose": "facts", "requestBody": tc.body,
				})
				assert.Equal(t, tc.images, facts["image_input_count"])
				assert.Equal(t, tc.videos, facts["video_input_count"])
			})
		}
	})

	t.Run("图片和视频单价仅在价格表达式启用时叠加", func(t *testing.T) {
		facts := callObject(t, "extractUsage", map[string]any{
			"upstreamModel": "sd2.5-30-10-10", "usagePurpose": "facts", "requestBody": map[string]any{
				"prompt": "ocean", "seconds": 5,
				"images": []any{"https://cdn.example/a.jpg", "https://cdn.example/b.jpg"},
				"videos": []any{"https://cdn.example/a.mp4", "https://cdn.example/b.mp4"},
			},
		})
		for _, tc := range []struct {
			expression string
			quota      int
		}{
			{`tier("base", u("seconds") * 0.02)`, 50000},
			{`tier("base", u("seconds") * 0.02 + u("image_input_count") * 0.10 + u("video_input_count") * 0.30)`, 450000},
		} {
			snapshot := &billingexpr.BillingSnapshot{
				ExprString: tc.expression, ExprHash: billingexpr.ExprHashString(tc.expression),
				GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: facts,
			}
			reserved, _, err := service.EvaluateTaskCompletionUsage(snapshot, nil)
			require.NoError(t, err)
			completed, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete",
				map[string]any{"upstreamModel": "sd2.5-30-10-10"}, map[string]any{"status": "SUCCESS"},
				map[string]any{"billing": map[string]any{"seconds": 999, "charged_credits": 999}})
			require.NoError(t, callErr)
			assert.Nil(t, completed)
			settled, finalFacts, err := service.EvaluateTaskCompletionUsage(snapshot, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.quota, reserved.ActualQuotaAfterGroup)
			assert.Equal(t, tc.quota, settled.ActualQuotaAfterGroup)
			assert.Equal(t, facts, finalFacts)
		}
	})

	t.Run("实际Token结算保留请求素材数", func(t *testing.T) {
		facts := callObject(t, "extractUsage", map[string]any{
			"upstreamModel": "seedance-2-5-official", "usagePurpose": "facts", "requestBody": map[string]any{
				"prompt": "ocean", "duration": 5,
				"images": []any{map[string]any{"url": "https://cdn.example/ref.jpg", "role": "reference_image"}},
				"videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video"}},
			},
		})
		assert.Equal(t, float64(756000), facts["tokens"])
		expression := `tier("base", u("tokens") * 70 / 1000000 + u("image_input_count") * 0.10 + u("video_input_count") * 0.30)`
		snapshot := &billingexpr.BillingSnapshot{
			ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
			GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: facts,
		}
		reserved, _, err := service.EvaluateTaskCompletionUsage(snapshot, nil)
		require.NoError(t, err)
		assert.Equal(t, 26660000, reserved.ActualQuotaAfterGroup)
		actual := callObject(t, "extractUsageOnComplete", map[string]any{"upstreamModel": "seedance-2-5-official"},
			map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"total_tokens": 100000}})
		settled, finalFacts, err := service.EvaluateTaskCompletionUsage(snapshot, actual)
		require.NoError(t, err)
		assert.Equal(t, 3700000, settled.ActualQuotaAfterGroup)
		assert.Equal(t, float64(100000), finalFacts["tokens"])
		assert.Equal(t, float64(1), finalFacts["image_input_count"])
		assert.Equal(t, float64(1), finalFacts["video_input_count"])
		assert.Equal(t, float64(756000), snapshot.UsageFacts["tokens"])
	})
}
