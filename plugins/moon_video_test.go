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

// TestMoonVideoContracts 通过真实 JS 运行时验证供应商协议、计费用量和渠道别名，不发起付费请求。
func TestMoonVideoContracts(t *testing.T) {
	source, err := plugins.Source("moon")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	const tokenModel = "doubao-seedance-2-0-mini-260615"
	const fastModel = "doubao-seedance-2-0-fast-260128"
	const wanModel = "wan3.0-video"
	t.Run("Moon与官方H3精确ID独立路由", func(t *testing.T) {
		h3Source, sourceErr := plugins.Source("hailuo")
		require.NoError(t, sourceErr)
		h3Registry := jsplugin.NewRegistry()
		_, registerErr := h3Registry.RegisterFactory(h3Source, jsplugin.Options{})
		require.NoError(t, registerErr)
		canonical, found := h3Registry.Generation().CanonicalModel("MINIMAX-H3")
		require.True(t, found)
		assert.Equal(t, "MiniMax-H3", canonical)
		_, registerErr = h3Registry.RegisterFactory(source, jsplugin.Options{})
		require.NoError(t, registerErr)
		for _, spec := range []struct{ model, plugin string }{{"minimax-h3", "moon"}, {"MiniMax-H3", "hailuo"}} {
			canonical, found = h3Registry.Generation().CanonicalModel(spec.model)
			require.True(t, found)
			assert.Equal(t, spec.model, canonical)
			for _, path := range []string{"/v1/videos", "/v1/responses"} {
				candidates := h3Registry.Generation().LookupEndpointCandidates("POST", path, canonical)
				require.Len(t, candidates, 1)
				assert.Equal(t, spec.plugin, candidates[0].Plugin.Meta.Key)
			}
		}
		canonical, found = h3Registry.Generation().CanonicalModel("MINIMAX-H3")
		assert.False(t, found)
		assert.Empty(t, canonical)
	})
	for _, spec := range []struct{ model, other string }{{wanModel, "alibaba"}, {tokenModel, "doubao"}} {
		otherSource, sourceErr := plugins.Source(spec.other)
		require.NoError(t, sourceErr)
		_, registerErr := registry.RegisterFactory(otherSource, jsplugin.Options{})
		require.NoError(t, registerErr)
		for _, path := range []string{"/v1/videos", "/v1/responses"} {
			candidates := registry.Generation().LookupEndpointCandidates("POST", path, spec.model)
			keys := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				keys = append(keys, candidate.Plugin.Meta.Key)
			}
			assert.ElementsMatch(t, []string{spec.other, "moon"}, keys)
		}
	}
	for _, tc := range []struct {
		name string
		hook string
		path []string
		args []any
		want string
	}{
		{name: "版本地址与显式零值保留", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://moon.example/proxy/v1/", "apiKey": "fixture", "publicTaskId": "task_unique", "upstreamModel": tokenModel, "requestBody": map[string]any{"prompt": "ocean", "seconds": 5, "generate_audio": false, "watermark": false, "seed": 0}}}, want: `{"url":"https://moon.example/proxy/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture","Content-Type":"application/json","Idempotency-Key":"task_unique"},"body":{"model":"doubao-seedance-2-0-mini-260615","prompt":"ocean","duration":5,"resolution":"720p","ratio":"16:9","generate_audio":false,"watermark":false,"seed":0},"noRetry":true,"action":"text_to_video"}`},
		{name: "Wan按秒事实兼容百炼价格", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": wanModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": 2, "resolution": "480p"}}}, want: `{"seconds":2,"resolution":"480P","image_input_count":0,"video_input_count":0}`},
		{name: "新模型预留估算", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": tokenModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": 5}}}, want: `{"tokens":108000,"resolution":"720p","video_input":"none","image_input_count":0,"video_input_count":0}`},
		{name: "自动时长与参考视频有界预留", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": tokenModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": -1, "videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video"}}}}}, want: `{"tokens":648000,"resolution":"720p","video_input":"video","image_input_count":0,"video_input_count":1}`},
		{name: "轮询ID编码且不重复v1", hook: "buildQueryRequest", args: []any{map[string]any{"baseUrl": "https://moon.example/v1", "apiKey": "fixture", "taskId": "task/a?b"}}, want: `{"url":"https://moon.example/v1/videos/task%2Fa%3Fb","method":"GET","headers":{"Authorization":"Bearer fixture"}}`},
		{name: "实际零Token不回退预估", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": tokenModel}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"total_tokens": 0}}}, want: `{"tokens":0}`},
		{name: "实际Token覆盖估算", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": tokenModel}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"total_tokens": 100000}}}, want: `{"tokens":100000}`},
		{name: "Wan不套用其他供应商用量字段", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": wanModel}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"input_video_duration": 20, "output_video_duration": 5}}}, want: `null`},
		{name: "Wan输出及参考视频共同计秒", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": wanModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": 5, "resolution": "720p", "reference_videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "duration": 3.5}}}}}, want: `{"seconds":8.5,"resolution":"720P","image_input_count":0,"video_input_count":1}`},
		{name: "H3正方形按照公布尺寸计费", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": "minimax-h3", "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "seconds": 5, "workflow_id": "text-to-video", "size": "1024x1024"}}}, want: `{"seconds":5,"resolution":"768p","image_input_count":0,"video_input_count":0}`},
		{name: "H3实际秒数与分辨率结算", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": "minimax-h3"}, map[string]any{"status": "SUCCESS"}, map[string]any{"billing": map[string]any{"seconds": 6, "resolution": "768p", "charged_credits": 1.08}}}, want: `{"seconds":6,"resolution":"768p"}`},
		{name: "CDN下载不带渠道密钥", hook: "buildContentRequest", args: []any{map[string]any{"artifactKey": "video", "baseUrl": "https://moon.example/v1", "apiKey": "fixture", "upstreamTaskId": "private", "clientRequest": map[string]any{"method": "HEAD"}, "data": map[string]any{"status": "completed", "data": []any{map[string]any{"url": "https://cdn.example/video.mp4"}}}}}, want: `{"url":"https://cdn.example/video.mp4","method":"HEAD","credentialless":true}`},
		{name: "缺直链时走原任务内容接口", hook: "buildContentRequest", args: []any{map[string]any{"artifactKey": "video", "baseUrl": "https://moon.example", "apiKey": "fixture", "upstreamTaskId": "task/a", "clientRequest": map[string]any{"method": "GET"}, "data": map[string]any{}}}, want: `{"url":"https://moon.example/v1/videos/task%2Fa/content","method":"GET","headers":{"Authorization":"Bearer fixture"}}`},
		{name: "进行中没有可下载制品", hook: "listArtifacts", args: []any{map[string]any{"status": "IN_PROGRESS"}}, want: `[]`},
		{name: "别名解码保留用户模型", hook: "protocols", path: []string{"openai_responses", "decodeRequest"}, args: []any{map[string]any{"model": "my-wan", "upstreamModel": wanModel, "body": map[string]any{"kind": "json", "value": map[string]any{"model": "my-wan", "input": "ocean"}}}}, want: `{"kind":"submit","model":"my-wan","action":"text_to_video","requestBody":{"model":"my-wan","prompt":"ocean"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value any
			var callErr error
			if len(tc.path) > 0 {
				value, callErr = plugin.Engine.CallPath(t.Context(), tc.hook, tc.path, tc.args...)
			} else {
				value, callErr = plugin.Engine.Call(t.Context(), tc.hook, tc.args...)
			}
			require.NoError(t, callErr)
			encoded, encodeErr := common.Marshal(value)
			require.NoError(t, encodeErr)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
	t.Run("Canvas元数据解码为Moon顶层合同", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			model  string
			body   map[string]any
			want   string
			action string
		}{
			{
				name:  "Wan空媒体",
				model: wanModel,
				body: map[string]any{
					"model": wanModel, "prompt": "ocean", "duration": 5, "seconds": "5", "resolution": "720P", "ratio": "16:9",
					"metadata": map[string]any{"input": map[string]any{"media": []any{}}},
				},
				want: `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9"}`,
			},
			{
				name:  "Seedance Fast文本",
				model: fastModel,
				body: map[string]any{
					"model": fastModel, "prompt": "ocean", "duration": 5, "seconds": "5",
					"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}}, "resolution": "720p", "ratio": "16:9"},
				},
				want: `{"model":"doubao-seedance-2-0-fast-260128","content":[{"type":"text","text":"ocean"}],"duration":5,"resolution":"720p","ratio":"16:9"}`,
			},
			{
				name:  "Seedance Mini文本",
				model: tokenModel,
				body: map[string]any{
					"model": tokenModel, "prompt": "ocean", "duration": 5, "seconds": "5",
					"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}}, "resolution": "720p", "ratio": "16:9"},
				},
				want: `{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"ocean"}],"duration":5,"resolution":"720p","ratio":"16:9"}`,
			},
			{
				name: "Wan首尾帧", model: wanModel, action: "reference_to_video",
				body: map[string]any{"model": wanModel, "prompt": "ocean", "duration": 5, "seconds": "5", "resolution": "720P", "ratio": "16:9",
					"metadata": map[string]any{"input": map[string]any{"media": []any{
						map[string]any{"type": "first_frame", "url": "https://cdn.example/first.jpg"},
						map[string]any{"type": "last_frame", "url": "https://cdn.example/last.jpg"},
					}}}},
				want: `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","reference_images":[{"url":"https://cdn.example/first.jpg","role":"first_frame"},{"url":"https://cdn.example/last.jpg","role":"last_frame"}]}`,
			},
			{
				name: "Wan参考时长附加元数据", model: wanModel, action: "reference_to_video",
				body: map[string]any{"model": wanModel, "prompt": "ocean", "duration": 5,
					"metadata": map[string]any{"reference_video_durations": []any{3.5, 4}, "input": map[string]any{"media": []any{
						map[string]any{"type": "reference_video", "url": "https://cdn.example/one.mp4"},
						map[string]any{"type": "reference_image", "url": "https://cdn.example/ref.jpg"},
						map[string]any{"type": "reference_video", "url": "https://cdn.example/two.mp4"},
					}}}},
				want: `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","reference_images":[{"url":"https://cdn.example/ref.jpg"}],"reference_videos":[{"url":"https://cdn.example/one.mp4","duration":3.5},{"url":"https://cdn.example/two.mp4","duration":4}]}`,
			},
			{
				name: "Seedance全能参考", model: "seedance-2-0-official", action: "reference_to_video",
				body: map[string]any{"model": "seedance-2-0-official", "prompt": "ocean", "seconds": "5", "metadata": map[string]any{
					"resolution": "1080p", "ratio": "adaptive", "omni_reference_task_type": "reference",
					"content": []any{
						map[string]any{"type": "text", "text": "ocean"},
						map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.jpg"}},
						map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
						map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}},
					}}},
				want: `{"model":"seedance-2-0-official","duration":5,"resolution":"1080p","ratio":"adaptive","omni_reference_task_type":"auto","content":[{"type":"text","text":"ocean"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://cdn.example/ref.jpg"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example/ref.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://cdn.example/ref.mp3"}}]}`,
			},
			{
				name: "H3全能参考", model: "minimax-h3", action: "reference_to_video",
				body: map[string]any{"model": "minimax-h3", "prompt": "ocean", "duration": 5, "seconds": "5", "metadata": map[string]any{
					"resolution": "768P", "ratio": "16:9", "content": []any{
						map[string]any{"type": "text", "text": "ocean"},
						map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.jpg"}},
						map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}},
						map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}},
					}}},
				want: `{"model":"minimax-h3","prompt":"ocean","seconds":5,"workflow_id":"multi-reference","size":"1376x768","images":["https://cdn.example/ref.jpg"],"reference_videos":["https://cdn.example/ref.mp4"],"reference_audios":["https://cdn.example/ref.mp3"]}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": tc.model,
					"body":  map[string]any{"kind": "json", "value": tc.body},
				})
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(value)
				require.NoError(t, encodeErr)
				var decoded struct {
					Action      string         `json:"action"`
					RequestBody map[string]any `json:"requestBody"`
				}
				require.NoError(t, common.Unmarshal(encoded, &decoded))
				action := tc.action
				if action == "" {
					action = "text_to_video"
				}
				assert.Equal(t, action, decoded.Action)
				assert.NotContains(t, decoded.RequestBody, "metadata")

				value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"baseUrl": "https://moon.example/v1", "apiKey": "fixture", "publicTaskId": "task_unique",
					"upstreamModel": tc.model, "requestBody": decoded.RequestBody,
				})
				require.NoError(t, callErr)
				encoded, encodeErr = common.Marshal(value)
				require.NoError(t, encodeErr)
				var request struct {
					Body map[string]any `json:"body"`
				}
				require.NoError(t, common.Unmarshal(encoded, &request))
				encoded, encodeErr = common.Marshal(request.Body)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, tc.want, string(encoded))
			})
		}
	})
	t.Run("Moon原生参考模式保留已确认字段", func(t *testing.T) {
		for _, tc := range []struct {
			name, model string
			body        map[string]any
			want        string
		}{
			{
				name: "Wan上传文件与URL", model: wanModel,
				body: map[string]any{"prompt": "ocean", "duration": 5, "resolution": "720p", "prompt_extend": false,
					"reference_images": []any{map[string]any{"file_id": "file-fixture"}},
					"reference_videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "duration": 3.5}},
					"reference_audios": []any{map[string]any{"url": "https://cdn.example/ref.mp3"}}},
				want: `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","prompt_extend":false,"reference_images":[{"file_id":"file-fixture"}],"reference_videos":[{"url":"https://cdn.example/ref.mp4","duration":3.5}],"reference_audios":[{"url":"https://cdn.example/ref.mp3"}]}`,
			},
			{
				name: "Seedance视频编辑", model: "seedance-2-0-fast-official",
				body: map[string]any{"prompt": "ocean", "duration": -1, "ratio": "adaptive", "omni_reference_task_type": "edit",
					"videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video"}}},
				want: `{"model":"seedance-2-0-fast-official","prompt":"ocean","duration":-1,"resolution":"720p","ratio":"adaptive","omni_reference_task_type":"edit","videos":[{"url":"https://cdn.example/ref.mp4","role":"reference_video"}]}`,
			},
			{
				name: "H3首尾帧超分", model: "minimax-h3",
				body: map[string]any{"prompt": "ocean", "seconds": 6, "workflow_id": "cf-fl2v", "size": "2K", "aspect_ratio": "16:9", "mode": "first_last_frame", "prompt_enhance": false,
					"images": []any{"https://cdn.example/first.jpg", "https://cdn.example/last.jpg"}},
				want: `{"model":"minimax-h3","prompt":"ocean","seconds":6,"workflow_id":"cf-fl2v","size":"2K","aspect_ratio":"16:9","mode":"first_last_frame","prompt_enhance":false,"images":["https://cdn.example/first.jpg","https://cdn.example/last.jpg"]}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"upstreamModel": tc.model, "baseUrl": "https://moon.example/v1", "apiKey": "fixture", "publicTaskId": "task-reference", "requestBody": tc.body})
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(value)
				require.NoError(t, encodeErr)
				var request struct {
					Body    map[string]any `json:"body"`
					NoRetry bool           `json:"noRetry"`
				}
				require.NoError(t, common.Unmarshal(encoded, &request))
				assert.True(t, request.NoRetry)
				encoded, encodeErr = common.Marshal(request.Body)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, tc.want, string(encoded))
			})
		}
	})
	t.Run("Responses图片参考按模型转换", func(t *testing.T) {
		for _, tc := range []struct {
			model  string
			fields map[string]any
			want   string
		}{
			{model: wanModel, fields: map[string]any{"duration": 5, "reference_videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "duration": 3}}},
				want: `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","reference_images":[{"url":"https://cdn.example/ref.jpg"}],"reference_videos":[{"url":"https://cdn.example/ref.mp4","duration":3}]}`},
			{model: "seedance-2-0-official", fields: map[string]any{"duration": 5},
				want: `{"model":"seedance-2-0-official","prompt":"ocean","duration":5,"resolution":"720p","ratio":"16:9","images":[{"url":"https://cdn.example/ref.jpg","role":"reference_image"}]}`},
			{model: "minimax-h3", fields: map[string]any{"seconds": 5, "workflow_id": "multi-reference", "size": "1024x1024"},
				want: `{"model":"minimax-h3","prompt":"ocean","seconds":5,"workflow_id":"multi-reference","size":"1024x1024","images":["https://cdn.example/ref.jpg"]}`},
		} {
			t.Run(tc.model, func(t *testing.T) {
				body := map[string]any{"model": tc.model, "input": []any{map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "input_text", "text": "ocean"},
					map[string]any{"type": "input_image", "image_url": "https://cdn.example/ref.jpg"},
				}}}}
				for key, value := range tc.fields {
					body[key] = value
				}
				value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{"model": tc.model, "body": map[string]any{"kind": "json", "value": body}})
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(value)
				require.NoError(t, encodeErr)
				var decoded struct {
					Action      string         `json:"action"`
					RequestBody map[string]any `json:"requestBody"`
				}
				require.NoError(t, common.Unmarshal(encoded, &decoded))
				assert.Equal(t, "reference_to_video", decoded.Action)
				value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"upstreamModel": tc.model, "requestBody": decoded.RequestBody, "baseUrl": "https://moon.example/v1", "apiKey": "fixture", "publicTaskId": "task-responses"})
				require.NoError(t, callErr)
				encoded, encodeErr = common.Marshal(value)
				require.NoError(t, encodeErr)
				var request struct {
					Body map[string]any `json:"body"`
				}
				require.NoError(t, common.Unmarshal(encoded, &request))
				encoded, encodeErr = common.Marshal(request.Body)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, tc.want, string(encoded))
			})
		}
	})
	t.Run("解码请求通过计费枚举并保留上游语义", func(t *testing.T) {
		for _, protocol := range []string{"openai_video", "openai_responses"} {
			for _, tc := range []struct {
				name, model, resolution, canonical string
				duration                           int
			}{
				{name: "Wan大小写与别名", model: wanModel, resolution: "480p", canonical: "480P", duration: 2},
				{name: "Token大小写", model: tokenModel, resolution: "720P", canonical: "720p", duration: 5},
				{name: "Token自动时长", model: tokenModel, resolution: "720p", canonical: "720p", duration: -1},
			} {
				t.Run(protocol+"/"+tc.name, func(t *testing.T) {
					body := map[string]any{"model": "video-alias", "duration": tc.duration, "resolution": tc.resolution}
					if protocol == "openai_responses" {
						body["input"] = "ocean"
					} else {
						body["prompt"] = "ocean"
					}
					value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{"model": "video-alias", "upstreamModel": tc.model, "body": map[string]any{"kind": "json", "value": body}})
					require.NoError(t, callErr)
					encoded, encodeErr := common.Marshal(value)
					require.NoError(t, encodeErr)
					var decoded struct {
						Model       string         `json:"model"`
						RequestBody map[string]any `json:"requestBody"`
					}
					require.NoError(t, common.Unmarshal(encoded, &decoded))
					assert.Equal(t, "video-alias", decoded.Model)
					schema, _ := plugin.Meta.UsageForModel(tc.model)
					assert.Equal(t, tc.canonical, decoded.RequestBody["resolution"])
					assert.Contains(t, schema["resolution"].Enum, decoded.RequestBody["resolution"])
					if tc.duration == -1 {
						assert.NotContains(t, decoded.RequestBody, "duration")
						assert.NotContains(t, decoded.RequestBody, "seconds")
					}
					value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"baseUrl": "https://moon.example/v1", "apiKey": "fixture", "publicTaskId": "task_unique", "upstreamModel": tc.model, "requestBody": decoded.RequestBody})
					require.NoError(t, callErr)
					encoded, encodeErr = common.Marshal(value)
					require.NoError(t, encodeErr)
					var request struct {
						Body map[string]any `json:"body"`
					}
					require.NoError(t, common.Unmarshal(encoded, &request))
					assert.Equal(t, tc.model, request.Body["model"])
					assert.EqualValues(t, tc.duration, request.Body["duration"])
					assert.NotContains(t, request.Body, "auto_duration")
				})
			}
		}
	})
	t.Run("待结算与无有效用量不得成功", func(t *testing.T) {
		for _, body := range []map[string]any{
			{"status": "submitting_unknown"}, {"status": "usage_pending"}, {"status": "commit_pending"},
			{"status": "completed"}, {"status": "completed", "usage": map[string]any{"total_tokens": -1}},
			{"status": "completed", "usage": map[string]any{"total_tokens": 2147483648}},
			{"status": "completed", "usage": map[string]any{"total_tokens": "100"}},
			{"status": "completed", "usage": map[string]any{"total_tokens": 1.5}},
		} {
			body["data"] = []any{map[string]any{"url": "https://cdn.example/not-settled.mp4"}}
			value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"upstreamModel": tokenModel}, body)
			require.NoError(t, callErr)
			encoded, encodeErr := common.Marshal(value)
			require.NoError(t, encodeErr)
			var result map[string]any
			require.NoError(t, common.Unmarshal(encoded, &result))
			assert.Equal(t, "IN_PROGRESS", result["status"], "%s", encoded)
			assert.NotContains(t, result, "url")
		}
	})
	t.Run("成功实际用量参与最终扣费", func(t *testing.T) {
		value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"upstreamModel": tokenModel}, map[string]any{"status": "completed", "usage": map[string]any{"total_tokens": 0}, "data": []any{map[string]any{"url": "https://cdn.example/done.mp4"}}})
		require.NoError(t, callErr)
		encoded, encodeErr := common.Marshal(value)
		require.NoError(t, encodeErr)
		assert.JSONEq(t, `{"status":"SUCCESS","progress":"100%","url":"https://cdn.example/done.mp4"}`, string(encoded))
		for _, tokens := range []float64{0, 100000} {
			result, runErr := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{
				ExprString: `u("tokens") / 1000000`, ExprHash: billingexpr.ExprHashString(`u("tokens") / 1000000`), GroupRatio: 1,
				QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true,
			}, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: map[string]any{"tokens": tokens, "resolution": "720p", "video_input": "none"}})
			require.NoError(t, runErr)
			assert.Equal(t, int(tokens/2), result.ActualQuotaAfterGroup)
		}
	})
	for _, tc := range []struct {
		name, model string
		fields      map[string]any
	}{
		{name: "超长视频", model: tokenModel, fields: map[string]any{"duration": 16}},
		{name: "布尔时长", model: tokenModel, fields: map[string]any{"duration": true}},
		{name: "空时长", model: tokenModel, fields: map[string]any{"duration": nil}},
		{name: "小数时长", model: tokenModel, fields: map[string]any{"duration": 4.5}},
		{name: "时长冲突", model: tokenModel, fields: map[string]any{"seconds": 5, "duration": 6}},
		{name: "布尔参数字符串", model: tokenModel, fields: map[string]any{"generate_audio": "false"}},
		{name: "无限分辨率", model: tokenModel, fields: map[string]any{"resolution": "4k"}},
		{name: "元数据绕过", model: tokenModel, fields: map[string]any{"metadata": map[string]any{"duration": 100000}}},
		{name: "Wan参考视频缺少时长", model: wanModel, fields: map[string]any{"metadata": map[string]any{"input": map[string]any{"media": []any{map[string]any{"type": "reference_video", "url": "https://cdn.example/ref.mp4"}}}}}},
		{name: "Wan参考视频附加时长数量不符", model: wanModel, fields: map[string]any{"metadata": map[string]any{"reference_video_durations": []any{3, 4}, "input": map[string]any{"media": []any{map[string]any{"type": "reference_video", "url": "https://cdn.example/ref.mp4"}}}}}},
		{name: "Wan元数据未知字段", model: wanModel, fields: map[string]any{"metadata": map[string]any{"input": map[string]any{"media": []any{}, "negative_prompt": "blur"}}}},
		{name: "Seedance元数据Base64素材", model: tokenModel, fields: map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}, map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "data:image/png;base64,aW1hZ2U="}}}, "resolution": "720p", "ratio": "16:9"}}},
		{name: "Seedance元数据未知模式", model: tokenModel, fields: map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}}, "resolution": "720p", "ratio": "16:9", "omni_reference_task_type": "invalid"}}},
		{name: "Seedance提示冲突", model: tokenModel, fields: map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "desert"}}, "resolution": "720p", "ratio": "16:9"}}},
		{name: "Seedance分辨率冲突", model: tokenModel, fields: map[string]any{"resolution": "480p", "metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}}, "resolution": "720p", "ratio": "16:9"}}},
		{name: "Seedance比例同义字段冲突", model: tokenModel, fields: map[string]any{"aspect_ratio": "9:16", "metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}}, "resolution": "720p", "ratio": "16:9"}}},
		{name: "内部时长标记不可直传", model: tokenModel, fields: map[string]any{"auto_duration": true}},
		{name: "提示覆盖参数", model: tokenModel, fields: map[string]any{"prompt": "ocean --duration 9999"}},
		{name: "参考片段未知字段", model: tokenModel, fields: map[string]any{"videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video", "duration": 1}}}},
		{name: "仅音频", model: tokenModel, fields: map[string]any{"audios": []any{map[string]any{"url": "https://cdn.example/ref.mp3", "role": "reference_audio"}}}},
		{name: "编辑时长必须自动", model: tokenModel, fields: map[string]any{"omni_reference_task_type": "edit", "duration": 5, "ratio": "adaptive", "videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video"}}}},
		{name: "Wan未验证参考输入", model: wanModel, fields: map[string]any{"images": []any{map[string]any{"url": "https://cdn.example/ref.png", "role": "reference_image"}}}},
		{name: "Wan不能自动时长", model: wanModel, fields: map[string]any{"duration": -1}},
		{name: "Wan未知参数", model: wanModel, fields: map[string]any{"generate_audio": false}},
		{name: "Wan参考视频总时长越界", model: wanModel, fields: map[string]any{"reference_videos": []any{map[string]any{"url": "https://cdn.example/one.mp4", "duration": 8}, map[string]any{"url": "https://cdn.example/two.mp4", "duration": 8}}}},
		{name: "Wan引用地址与文件互斥", model: wanModel, fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/ref.jpg", "file_id": "file-fixture"}}}},
		{name: "H3量化不能参考视频", model: "minimax-h3", fields: map[string]any{"workflow_id": "lh-multi-reference", "size": "1376x768", "seconds": 6, "images": []any{"https://cdn.example/ref.jpg"}, "reference_video": "https://cdn.example/ref.mp4"}},
		{name: "H3量化时长越界", model: "minimax-h3", fields: map[string]any{"workflow_id": "lh-multi-reference", "size": "1376x768", "seconds": 11, "images": []any{"https://cdn.example/ref.jpg"}}},
		{name: "H3超分缺少比例", model: "minimax-h3", fields: map[string]any{"workflow_id": "cf-multi-reference", "size": "4K", "seconds": 6, "images": []any{"https://cdn.example/ref.jpg"}}},
		{name: "H3首尾帧超数量", model: "minimax-h3", fields: map[string]any{"workflow_id": "fl2v", "size": "1376x768", "seconds": 6, "images": []any{"https://cdn.example/one.jpg", "https://cdn.example/two.jpg", "https://cdn.example/three.jpg"}}},
		{name: "H3单复数视频字段互斥", model: "minimax-h3", fields: map[string]any{"workflow_id": "multi-reference", "size": "1376x768", "seconds": 6, "reference_video": "https://cdn.example/one.mp4", "reference_videos": []any{"https://cdn.example/two.mp4"}}},
		{name: "H3不能借官方ID路由", model: "MiniMax-H3", fields: map[string]any{"workflow_id": "text-to-video", "size": "1376x768", "seconds": 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"model": tc.model, "prompt": "ocean"}
			for k, v := range tc.fields {
				body[k] = v
			}
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": tc.model, "body": map[string]any{"kind": "json", "value": body}})
			require.Error(t, callErr)
		})
	}
	t.Run("Responses不静默丢弃工具与元数据", func(t *testing.T) {
		for _, field := range []string{"tools", "metadata", "instructions", "previous_response_id"} {
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{"model": tokenModel, "body": map[string]any{"kind": "json", "value": map[string]any{"input": "ocean", field: map[string]any{}}}})
			require.Error(t, callErr, field)
		}
	})
}

// TestMoonLatestVideoContracts 验证公开文档中的新增模型、输入边界与结算门槛，不调用生成上游。
func TestMoonLatestVideoContracts(t *testing.T) {
	source, err := plugins.Source("moon")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	const grokModel = "grok-v1.5-video"
	t.Run("Grok精确模型提供次数和秒数定价", func(t *testing.T) {
		assert.Contains(t, plugin.Meta.Models, grokModel)
		require.NotNil(t, plugin.Meta.ModelDiscovery)
		assert.Equal(t, "openai", plugin.Meta.ModelDiscovery.Protocol)
		assert.Equal(t, "/v1/models", plugin.Meta.ModelDiscovery.Path)
		for _, path := range []string{"/v1/videos", "/v1/responses"} {
			candidates := registry.Generation().LookupEndpointCandidates("POST", path, grokModel)
			require.Len(t, candidates, 1)
			assert.Equal(t, "moon", candidates[0].Plugin.Meta.Key)
			assert.Empty(t, registry.Generation().LookupEndpointCandidates("POST", path, "grok-imagine-video-1.5"))
		}
		schema, _ := plugin.Meta.UsageForModel(grokModel)
		require.Len(t, schema, 4)
		assert.Equal(t, "number", schema["video_count"].Type)
		assert.Equal(t, "count", schema["video_count"].Unit)
		assert.Equal(t, "number", schema["seconds"].Type)
		assert.Equal(t, "second", schema["seconds"].Unit)
	})
	t.Run("Grok规范请求保留幂等且禁止提交重试", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			fields map[string]any
			want   string
		}{
			{name: "默认文生视频", want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"720p","aspect_ratio":"16:9"}`},
			{name: "别名及整数字符串", fields: map[string]any{"duration": "15", "resolution": "1080p", "ratio": "1:1"}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":15,"size":"1080p","aspect_ratio":"1:1"}`},
			{name: "一致的同义参数", fields: map[string]any{"duration": 4, "seconds": "4", "size": "720p", "resolution": "720p", "ratio": "4:3", "aspect_ratio": "4:3"}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":4,"size":"720p","aspect_ratio":"4:3"}`},
			{name: "横向720精确尺寸", fields: map[string]any{"size": "1280x720"}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"720p","aspect_ratio":"16:9"}`},
			{name: "竖向720精确尺寸", fields: map[string]any{"size": "720x1280", "ratio": "9:16"}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"720p","aspect_ratio":"9:16"}`},
			{name: "横向1080精确尺寸", fields: map[string]any{"size": "1920x1080", "resolution": "1080p"}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"1080p","aspect_ratio":"16:9"}`},
			{name: "竖向1080精确尺寸", fields: map[string]any{"size": "1080x1920", "aspect_ratio": "9:16"}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"1080p","aspect_ratio":"9:16"}`},
			{name: "单图入口", fields: map[string]any{"input_reference": "https://cdn.example/ref.jpg", "ratio": "3:4"}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"720p","aspect_ratio":"3:4","input_reference":"https://cdn.example/ref.jpg"}`},
			{name: "默认参考图角色", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/ref.jpg"}}}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"720p","aspect_ratio":"16:9","reference_images":[{"url":"https://cdn.example/ref.jpg","role":"reference_image"}]}`},
			{name: "单张首帧", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/first.jpg", "role": "first_frame"}}}, want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"720p","aspect_ratio":"16:9","reference_images":[{"url":"https://cdn.example/first.jpg","role":"first_frame"}]}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body := map[string]any{"prompt": "ocean"}
				for key, value := range tc.fields {
					body[key] = value
				}
				value, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"upstreamModel": grokModel, "model": "moon-grok-alias", "baseUrl": "https://moon.example/proxy/v1/",
					"apiKey": "fixture", "publicTaskId": "task-grok", "requestBody": body,
				})
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(value)
				require.NoError(t, encodeErr)
				var request struct {
					URL     string            `json:"url"`
					Method  string            `json:"method"`
					Headers map[string]string `json:"headers"`
					Body    map[string]any    `json:"body"`
					NoRetry bool              `json:"noRetry"`
				}
				require.NoError(t, common.Unmarshal(encoded, &request))
				assert.Equal(t, "https://moon.example/proxy/v1/videos", request.URL)
				assert.Equal(t, "POST", request.Method)
				assert.Equal(t, "Bearer fixture", request.Headers["Authorization"])
				assert.Equal(t, "task-grok", request.Headers["Idempotency-Key"])
				assert.True(t, request.NoRetry)
				encoded, encodeErr = common.Marshal(request.Body)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, tc.want, string(encoded))
			})
		}
	})
	t.Run("Grok协议输入映射到原生图片参考", func(t *testing.T) {
		for _, tc := range []struct {
			name, protocol string
			body           map[string]any
			want           string
		}{
			{
				name: "Responses文本图片", protocol: "openai_responses",
				body: map[string]any{"duration": 8, "resolution": "1080p", "ratio": "4:3", "input": []any{map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "input_text", "text": "ocean"},
					map[string]any{"type": "input_image", "image_url": "https://cdn.example/ref.jpg"},
				}}}},
				want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":8,"size":"1080p","aspect_ratio":"4:3","reference_images":[{"url":"https://cdn.example/ref.jpg","role":"reference_image"}]}`,
			},
			{
				name: "Canvas参考图", protocol: "openai_video",
				body: map[string]any{"prompt": "ocean", "seconds": "5", "metadata": map[string]any{"resolution": "1080p", "ratio": "3:4", "content": []any{
					map[string]any{"type": "text", "text": "ocean"},
					map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://cdn.example/ref.jpg"}},
				}}},
				want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":5,"size":"1080p","aspect_ratio":"3:4","reference_images":[{"url":"https://cdn.example/ref.jpg","role":"reference_image"}]}`,
			},
			{
				name: "Canvas首帧", protocol: "openai_video",
				body: map[string]any{"metadata": map[string]any{"resolution": "720p", "ratio": "16:9", "content": []any{
					map[string]any{"type": "text", "text": "ocean"},
					map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "https://cdn.example/first.jpg"}},
				}}},
				want: `{"model":"grok-v1.5-video","prompt":"ocean","seconds":6,"size":"720p","aspect_ratio":"16:9","reference_images":[{"url":"https://cdn.example/first.jpg","role":"first_frame"}]}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{tc.protocol, "decodeRequest"}, map[string]any{
					"model": "moon-grok-alias", "upstreamModel": grokModel, "body": map[string]any{"kind": "json", "value": tc.body},
				})
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(value)
				require.NoError(t, encodeErr)
				var decoded struct {
					Model       string         `json:"model"`
					Action      string         `json:"action"`
					RequestBody map[string]any `json:"requestBody"`
				}
				require.NoError(t, common.Unmarshal(encoded, &decoded))
				assert.Equal(t, "moon-grok-alias", decoded.Model)
				assert.Equal(t, "reference_to_video", decoded.Action)
				assert.NotContains(t, decoded.RequestBody, "metadata")
				value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"upstreamModel": grokModel, "baseUrl": "https://moon.example/v1", "apiKey": "fixture",
					"publicTaskId": "task-grok", "requestBody": decoded.RequestBody,
				})
				require.NoError(t, callErr)
				encoded, encodeErr = common.Marshal(value)
				require.NoError(t, encodeErr)
				var request struct {
					Body map[string]any `json:"body"`
				}
				require.NoError(t, common.Unmarshal(encoded, &request))
				encoded, encodeErr = common.Marshal(request.Body)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, tc.want, string(encoded))
			})
		}
	})
	t.Run("Grok按次结算并等待真实成片", func(t *testing.T) {
		for _, tc := range []struct {
			name, hook string
			args       []any
			want       string
		}{
			{name: "默认请求一次六秒", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": grokModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean"}}}, want: `{"video_count":1,"seconds":6,"image_input_count":0,"video_input_count":0}`},
			{name: "高分辨率长视频一次十五秒", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": grokModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": 15, "size": "1080p"}}}, want: `{"video_count":1,"seconds":15,"image_input_count":0,"video_input_count":0}`},
			{name: "成功无需Token用量", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": grokModel}, map[string]any{"status": "SUCCESS"}, map[string]any{"status": "completed", "url": "https://cdn.example/done.mp4"}}, want: `{"video_count":1}`},
			{name: "失败不发布结算事实", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": grokModel}, map[string]any{"status": "FAILURE"}, map[string]any{"status": "failed"}}, want: `null`},
			{name: "顶层原链接完成", hook: "parseTaskResult", args: []any{map[string]any{"upstreamModel": grokModel}, map[string]any{"status": "completed", "url": "https://cdn.example/done.mp4"}}, want: `{"status":"SUCCESS","progress":"100%","url":"https://cdn.example/done.mp4"}`},
			{name: "元数据原链接完成", hook: "parseTaskResult", args: []any{map[string]any{"upstreamModel": grokModel}, map[string]any{"status": "completed", "metadata": map[string]any{"url": "https://cdn.example/done.mp4"}}}, want: `{"status":"SUCCESS","progress":"100%","url":"https://cdn.example/done.mp4"}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				value, callErr := plugin.Engine.Call(t.Context(), tc.hook, tc.args...)
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(value)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, tc.want, string(encoded))
			})
		}
		for _, body := range []map[string]any{
			{"status": "completed"},
			{"status": "completed", "url": "javascript:invalid"},
			{"status": "submitting_unknown", "url": "https://cdn.example/not-settled.mp4"},
			{"status": "commit_pending", "url": "https://cdn.example/not-settled.mp4"},
			{"status": "refund_pending"},
		} {
			value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"upstreamModel": grokModel}, body)
			require.NoError(t, callErr)
			encoded, encodeErr := common.Marshal(value)
			require.NoError(t, encodeErr)
			var result map[string]any
			require.NoError(t, common.Unmarshal(encoded, &result))
			assert.Equal(t, "IN_PROGRESS", result["status"], "%s", encoded)
			assert.NotContains(t, result, "url")
		}
	})
	t.Run("Grok按秒与按次售价独立于上游账单", func(t *testing.T) {
		completed, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete",
			map[string]any{"upstreamModel": grokModel}, map[string]any{"status": "SUCCESS"},
			map[string]any{"seconds": 999, "billing": map[string]any{"seconds": 0, "charged_credits": 12345}})
		require.NoError(t, callErr)
		encoded, encodeErr := common.Marshal(completed)
		require.NoError(t, encodeErr)
		var completionFacts map[string]any
		require.NoError(t, common.Unmarshal(encoded, &completionFacts))
		assert.JSONEq(t, `{"video_count":1}`, string(encoded))

		for _, tc := range []struct {
			name, protocol string
			fields         map[string]any
			seconds        float64
		}{
			{name: "默认六秒", protocol: "openai_video", seconds: 6},
			{name: "整数字符串四秒", protocol: "openai_video", fields: map[string]any{"seconds": "4"}, seconds: 4},
			{name: "duration别名十五秒", protocol: "openai_responses", fields: map[string]any{"duration": "15"}, seconds: 15},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body := map[string]any{"prompt": "ocean"}
				for key, value := range tc.fields {
					body[key] = value
				}
				if tc.protocol == "openai_responses" {
					delete(body, "prompt")
					body["input"] = "ocean"
				}
				decodedValue, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{tc.protocol, "decodeRequest"},
					map[string]any{"model": grokModel, "body": map[string]any{"kind": "json", "value": body}})
				require.NoError(t, err)
				encoded, err := common.Marshal(decodedValue)
				require.NoError(t, err)
				var decoded struct {
					RequestBody map[string]any `json:"requestBody"`
				}
				require.NoError(t, common.Unmarshal(encoded, &decoded))
				assert.Equal(t, tc.seconds, decoded.RequestBody["seconds"])
				assert.NotContains(t, decoded.RequestBody, "duration")
				value, err := plugin.Engine.Call(t.Context(), "extractUsage",
					map[string]any{"upstreamModel": grokModel, "usagePurpose": "facts", "requestBody": decoded.RequestBody})
				require.NoError(t, err)
				encoded, err = common.Marshal(value)
				require.NoError(t, err)
				var facts map[string]any
				require.NoError(t, common.Unmarshal(encoded, &facts))
				assert.Equal(t, tc.seconds, facts["seconds"])
				assert.Equal(t, float64(1), facts["video_count"])

				for _, price := range []struct {
					expression string
					quota      int
				}{
					{expression: `u("seconds") * 0.02`, quota: int(tc.seconds) * 10000},
					{expression: `u("video_count") * 0.2`, quota: 100000},
				} {
					snapshot := &billingexpr.BillingSnapshot{
						ExprString: price.expression, ExprHash: billingexpr.ExprHashString(price.expression),
						GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: facts,
					}
					before, _, err := service.EvaluateTaskCompletionUsage(snapshot, nil)
					require.NoError(t, err)
					after, settled, err := service.EvaluateTaskCompletionUsage(snapshot, completionFacts)
					require.NoError(t, err)
					assert.Equal(t, price.quota, before.ActualQuotaAfterGroup)
					assert.Equal(t, price.quota, after.ActualQuotaAfterGroup)
					assert.Equal(t, tc.seconds, settled["seconds"])
					assert.Equal(t, facts, snapshot.UsageFacts)
				}
			})
		}
		expression := `u("video_count") * 0.2`
		legacy := &billingexpr.BillingSnapshot{
			ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
			GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true,
			UsageFacts: map[string]any{"video_count": float64(1)},
		}
		result, facts, err := service.EvaluateTaskCompletionUsage(legacy, completionFacts)
		require.NoError(t, err)
		assert.Equal(t, 100000, result.ActualQuotaAfterGroup)
		assert.NotContains(t, facts, "seconds")
	})
	t.Run("Grok参考图数量边界", func(t *testing.T) {
		for _, count := range []int{7, 8} {
			images := make([]any, count)
			for i := range images {
				images[i] = map[string]any{"url": "https://cdn.example/ref.jpg"}
			}
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": grokModel, "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "ocean", "reference_images": images}},
			})
			if count == 7 {
				require.NoError(t, callErr)
			} else {
				require.Error(t, callErr)
			}
		}
	})
	t.Run("Grok拒绝越界及未支持素材", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			fields map[string]any
		}{
			{name: "不支持自动时长", fields: map[string]any{"duration": -1}},
			{name: "超短时长", fields: map[string]any{"duration": 3}},
			{name: "超长时长", fields: map[string]any{"duration": 16}},
			{name: "小数时长", fields: map[string]any{"duration": 4.5}},
			{name: "空时长", fields: map[string]any{"seconds": nil}},
			{name: "布尔时长", fields: map[string]any{"seconds": true}},
			{name: "冲突时长", fields: map[string]any{"seconds": 4, "duration": 5}},
			{name: "错误分辨率", fields: map[string]any{"resolution": "480p"}},
			{name: "分辨率不能接受精确尺寸", fields: map[string]any{"resolution": "1280x720"}},
			{name: "分辨率别名冲突", fields: map[string]any{"resolution": "720p", "size": "1080p"}},
			{name: "尺寸比例冲突", fields: map[string]any{"size": "1280x720", "ratio": "1:1"}},
			{name: "未声明尺寸", fields: map[string]any{"size": "1024x1024"}},
			{name: "未知比例", fields: map[string]any{"ratio": "21:9"}},
			{name: "比例别名冲突", fields: map[string]any{"ratio": "1:1", "aspect_ratio": "16:9"}},
			{name: "空提示", fields: map[string]any{"prompt": " "}},
			{name: "超长提示", fields: map[string]any{"prompt": strings.Repeat("a", 32001)}},
			{name: "未知参数", fields: map[string]any{"generate_audio": true}},
			{name: "视频参考", fields: map[string]any{"reference_videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4"}}}},
			{name: "音频参考", fields: map[string]any{"reference_audios": []any{map[string]any{"url": "https://cdn.example/ref.mp3"}}}},
			{name: "空参考图片", fields: map[string]any{"reference_images": []any{}}},
			{name: "图片上传文件", fields: map[string]any{"reference_images": []any{map[string]any{"file_id": "file-fixture"}}}},
			{name: "图片内联数据", fields: map[string]any{"input_reference": "data:image/png;base64,aW1hZ2U="}},
			{name: "图片对象未知字段", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/ref.jpg", "strength": 1}}}},
			{name: "首帧最多一张", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/a.jpg", "role": "first_frame"}, map[string]any{"url": "https://cdn.example/b.jpg", "role": "first_frame"}}}},
			{name: "首帧不能混用参考图", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/a.jpg", "role": "first_frame"}, map[string]any{"url": "https://cdn.example/b.jpg", "role": "reference_image"}}}},
			{name: "不支持尾帧", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/ref.jpg", "role": "last_frame"}}}},
			{name: "图片入口互斥", fields: map[string]any{"input_reference": "https://cdn.example/a.jpg", "reference_images": []any{map[string]any{"url": "https://cdn.example/b.jpg"}}}},
			{name: "Canvas尾帧", fields: map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}, map[string]any{"type": "image_url", "role": "last_frame", "image_url": map[string]any{"url": "https://cdn.example/ref.jpg"}}}}}},
			{name: "Canvas视频", fields: map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}, map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}}}}}},
			{name: "Canvas音频", fields: map[string]any{"metadata": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ocean"}, map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://cdn.example/ref.mp3"}}}}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body := map[string]any{"prompt": "ocean"}
				for key, value := range tc.fields {
					body[key] = value
				}
				_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": grokModel, "body": map[string]any{"kind": "json", "value": body},
				})
				require.Error(t, callErr)
			})
		}
	})
	t.Run("各模型新增参考输入规范化并保持计费", func(t *testing.T) {
		for _, tc := range []struct {
			name, model string
			body        map[string]any
			want, usage string
		}{
			{
				name: "Seedance字符串图片别名", model: "seedance-2-0-mini-official",
				body:  map[string]any{"prompt": "ocean", "image_urls": []any{"https://cdn.example/ref.jpg"}},
				want:  `{"model":"seedance-2-0-mini-official","prompt":"ocean","duration":5,"resolution":"720p","ratio":"16:9","images":[{"url":"https://cdn.example/ref.jpg","role":"reference_image"}]}`,
				usage: `{"tokens":108000,"resolution":"720p","video_input":"none","image_input_count":1,"video_input_count":0}`,
			},
			{
				name: "Seedance对象图片别名", model: "seedance-2-0-fast-official",
				body: map[string]any{"prompt": "ocean", "image_urls": []any{map[string]any{"url": "https://cdn.example/ref.jpg", "role": "reference_image"}}},
				want: `{"model":"seedance-2-0-fast-official","prompt":"ocean","duration":5,"resolution":"720p","ratio":"16:9","images":[{"url":"https://cdn.example/ref.jpg","role":"reference_image"}]}`,
			},
			{
				name: "Wan文档与普通参考图", model: "wan3.0-video",
				body: map[string]any{"prompt": "ocean", "duration": 5, "input": map[string]any{"media": []any{map[string]any{"type": "file", "url": "https://cdn.example/story.pdf"}}},
					"reference_images": []any{map[string]any{"url": "https://cdn.example/ref.jpg"}}},
				want:  `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","input":{"media":[{"type":"file","url":"https://cdn.example/story.pdf"}]},"reference_images":[{"url":"https://cdn.example/ref.jpg"}]}`,
				usage: `{"seconds":5,"resolution":"720P","image_input_count":1,"video_input_count":0}`,
			},
			{
				name: "Wan网页参考", model: "wan3.0-video-prime",
				body: map[string]any{"prompt": "ocean", "duration": 5, "input": map[string]any{"media": []any{map[string]any{"type": "link", "url": "https://example.com/story"}}}},
				want: `{"model":"wan3.0-video-prime","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","input":{"media":[{"type":"link","url":"https://example.com/story"}]}}`,
			},
			{
				name: "Wan单独尾帧保留既有行为", model: "wan3.0-video",
				body: map[string]any{"prompt": "ocean", "duration": 5, "reference_images": []any{map[string]any{"url": "https://cdn.example/last.jpg", "role": "last_frame"}}},
				want: `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","reference_images":[{"url":"https://cdn.example/last.jpg","role":"last_frame"}]}`,
			},
			{
				name: "WanCanvas分离文档与计费素材", model: "wan3.0-video",
				body: map[string]any{"prompt": "ocean", "duration": 5, "metadata": map[string]any{
					"reference_video_durations": []any{3}, "input": map[string]any{"media": []any{
						map[string]any{"type": "link", "url": "https://example.com/story"},
						map[string]any{"type": "reference_image", "url": "https://cdn.example/ref.jpg"},
						map[string]any{"type": "reference_video", "url": "https://cdn.example/ref.mp4"},
					}}}},
				want:  `{"model":"wan3.0-video","prompt":"ocean","duration":5,"resolution":"720p","aspect_ratio":"16:9","input":{"media":[{"type":"link","url":"https://example.com/story"}]},"reference_images":[{"url":"https://cdn.example/ref.jpg"}],"reference_videos":[{"url":"https://cdn.example/ref.mp4","duration":3}]}`,
				usage: `{"seconds":8,"resolution":"720P","image_input_count":1,"video_input_count":1}`,
			},
			{
				name: "H3量化允许480p", model: "minimax-h3",
				body:  map[string]any{"prompt": "ocean", "seconds": 10, "workflow_id": "lh-multi-reference", "size": "864x480", "images": []any{"https://cdn.example/ref.jpg"}},
				want:  `{"model":"minimax-h3","prompt":"ocean","seconds":10,"workflow_id":"lh-multi-reference","size":"864x480","images":["https://cdn.example/ref.jpg"]}`,
				usage: `{"seconds":10,"resolution":"480p","image_input_count":1,"video_input_count":0}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": tc.model, "body": map[string]any{"kind": "json", "value": tc.body},
				})
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(value)
				require.NoError(t, encodeErr)
				var decoded struct {
					Action      string         `json:"action"`
					RequestBody map[string]any `json:"requestBody"`
				}
				require.NoError(t, common.Unmarshal(encoded, &decoded))
				assert.Equal(t, "reference_to_video", decoded.Action)
				value, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"upstreamModel": tc.model, "baseUrl": "https://moon.example/v1", "apiKey": "fixture",
					"publicTaskId": "task-references", "requestBody": decoded.RequestBody,
				})
				require.NoError(t, callErr)
				encoded, encodeErr = common.Marshal(value)
				require.NoError(t, encodeErr)
				var request struct {
					Body map[string]any `json:"body"`
				}
				require.NoError(t, common.Unmarshal(encoded, &request))
				encoded, encodeErr = common.Marshal(request.Body)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, tc.want, string(encoded))
				if tc.usage != "" {
					value, callErr = plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
						"upstreamModel": tc.model, "usagePurpose": "facts", "requestBody": decoded.RequestBody,
					})
					require.NoError(t, callErr)
					encoded, encodeErr = common.Marshal(value)
					require.NoError(t, encodeErr)
					assert.JSONEq(t, tc.usage, string(encoded))
				}
			})
		}
	})
	t.Run("Wan文档额外于十二个图片音视频素材", func(t *testing.T) {
		images := make([]any, 10)
		for i := range images {
			images[i] = map[string]any{"url": "https://cdn.example/ref.jpg"}
		}
		_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "wan3.0-video", "body": map[string]any{"kind": "json", "value": map[string]any{
				"prompt": "ocean", "input": map[string]any{"media": []any{map[string]any{"type": "file", "url": "https://cdn.example/story.pdf"}}},
				"reference_images": images, "reference_videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "duration": 3}},
				"reference_audios": []any{map[string]any{"url": "https://cdn.example/ref.mp3"}},
			}},
		})
		require.NoError(t, callErr)
	})
	t.Run("新增别名和参考模式不绕过模型限制", func(t *testing.T) {
		for _, tc := range []struct {
			name, model string
			fields      map[string]any
		}{
			{name: "Seedance图片字段互斥", model: "seedance-2-0-official", fields: map[string]any{"images": []any{map[string]any{"url": "https://cdn.example/a.jpg", "role": "reference_image"}}, "image_urls": []any{"https://cdn.example/b.jpg"}}},
			{name: "Seedance别名与content互斥", model: "seedance-2-0-official", fields: map[string]any{"prompt": nil, "content": []any{map[string]any{"type": "text", "text": "ocean"}}, "image_urls": []any{"https://cdn.example/ref.jpg"}}},
			{name: "Seedance别名禁止内联图片", model: "seedance-2-0-official", fields: map[string]any{"image_urls": []any{"data:image/png;base64,aW1hZ2U="}}},
			{name: "Seedance别名禁止文件ID", model: "seedance-2-0-official", fields: map[string]any{"image_urls": []any{map[string]any{"file_id": "file-fixture"}}}},
			{name: "Seedance别名禁止未知字段", model: "seedance-2-0-official", fields: map[string]any{"image_urls": []any{map[string]any{"url": "https://cdn.example/ref.jpg", "strength": 1}}}},
			{name: "Wan文档与网页互斥", model: "wan3.0-video", fields: map[string]any{"input": map[string]any{"media": []any{map[string]any{"type": "file", "url": "https://cdn.example/story.pdf"}, map[string]any{"type": "link", "url": "https://example.com/story"}}}}},
			{name: "Wan未知文档类型", model: "wan3.0-video", fields: map[string]any{"input": map[string]any{"media": []any{map[string]any{"type": "document", "url": "https://cdn.example/story.pdf"}}}}},
			{name: "Wan文档禁止内联数据", model: "wan3.0-video", fields: map[string]any{"input": map[string]any{"media": []any{map[string]any{"type": "file", "url": "data:application/pdf;base64,cGRm"}}}}},
			{name: "Wan文档禁止未知字段", model: "wan3.0-video", fields: map[string]any{"input": map[string]any{"media": []any{map[string]any{"type": "file", "url": "https://cdn.example/story.pdf", "file_id": "file-fixture"}}}}},
			{name: "Wan首帧不可混普通参考图", model: "wan3.0-video", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/first.jpg", "role": "first_frame"}, map[string]any{"url": "https://cdn.example/ref.jpg"}}}},
			{name: "Wan首帧不可混参考音频", model: "wan3.0-video", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/first.jpg", "role": "first_frame"}}, "reference_audios": []any{map[string]any{"url": "https://cdn.example/ref.mp3"}}}},
			{name: "Wan首帧不可混参考视频", model: "wan3.0-video", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/first.jpg", "role": "first_frame"}}, "reference_videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "duration": 3}}}},
			{name: "Wan首帧不可混文档", model: "wan3.0-video", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/first.jpg", "role": "first_frame"}}, "input": map[string]any{"media": []any{map[string]any{"type": "file", "url": "https://cdn.example/story.pdf"}}}}},
			{name: "Wan首帧不能重复", model: "wan3.0-video", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/a.jpg", "role": "first_frame"}, map[string]any{"url": "https://cdn.example/b.jpg", "role": "first_frame"}}}},
			{name: "Wan尾帧不能重复", model: "wan3.0-video", fields: map[string]any{"reference_images": []any{map[string]any{"url": "https://cdn.example/a.jpg", "role": "last_frame"}, map[string]any{"url": "https://cdn.example/b.jpg", "role": "last_frame"}}}},
			{name: "WanCanvas首帧与网页互斥", model: "wan3.0-video", fields: map[string]any{"metadata": map[string]any{"input": map[string]any{"media": []any{map[string]any{"type": "first_frame", "url": "https://cdn.example/first.jpg"}, map[string]any{"type": "link", "url": "https://example.com/story"}}}}}},
			{name: "H3量化拒绝1080p", model: "minimax-h3", fields: map[string]any{"seconds": 6, "workflow_id": "lh-multi-reference", "size": "1920x1088", "images": []any{"https://cdn.example/ref.jpg"}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body := map[string]any{"prompt": "ocean"}
				for key, value := range tc.fields {
					body[key] = value
				}
				if body["prompt"] == nil {
					delete(body, "prompt")
				}
				_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": tc.model, "body": map[string]any{"kind": "json", "value": body},
				})
				require.Error(t, callErr)
			})
		}
	})
	t.Run("Seedance图片别名计入九图与总素材上限", func(t *testing.T) {
		for _, count := range []int{9, 10} {
			images := make([]any, count)
			for i := range images {
				images[i] = "https://cdn.example/ref.jpg"
			}
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": "seedance-2-0-official", "body": map[string]any{"kind": "json", "value": map[string]any{
					"prompt": "ocean", "image_urls": images,
					"videos": []any{map[string]any{"url": "https://cdn.example/one.mp4", "role": "reference_video"}, map[string]any{"url": "https://cdn.example/two.mp4", "role": "reference_video"}, map[string]any{"url": "https://cdn.example/three.mp4", "role": "reference_video"}},
					"audios": []any{map[string]any{"url": "https://cdn.example/one.mp3", "role": "reference_audio"}, map[string]any{"url": "https://cdn.example/two.mp3", "role": "reference_audio"}, map[string]any{"url": "https://cdn.example/three.mp3", "role": "reference_audio"}},
				}},
			})
			if count == 9 {
				require.NoError(t, callErr)
			} else {
				require.Error(t, callErr)
			}
		}
	})
}
