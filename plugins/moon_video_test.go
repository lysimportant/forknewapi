package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
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
	const wanModel = "wan3.0-video"
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
		{name: "Wan按秒事实兼容百炼价格", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": wanModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": 2, "resolution": "480p"}}}, want: `{"seconds":2,"resolution":"480P"}`},
		{name: "新模型预留估算", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": tokenModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": 5}}}, want: `{"tokens":108000,"resolution":"720p","video_input":"none"}`},
		{name: "自动时长与参考视频有界预留", hook: "extractUsage", args: []any{map[string]any{"upstreamModel": tokenModel, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "duration": -1, "videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video"}}}}}, want: `{"tokens":648000,"resolution":"720p","video_input":"video"}`},
		{name: "轮询ID编码且不重复v1", hook: "buildQueryRequest", args: []any{map[string]any{"baseUrl": "https://moon.example/v1", "apiKey": "fixture", "taskId": "task/a?b"}}, want: `{"url":"https://moon.example/v1/videos/task%2Fa%3Fb","method":"GET","headers":{"Authorization":"Bearer fixture"}}`},
		{name: "实际零Token不回退预估", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": tokenModel}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"total_tokens": 0}}}, want: `{"tokens":0}`},
		{name: "实际Token覆盖估算", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": tokenModel}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"total_tokens": 100000}}}, want: `{"tokens":100000}`},
		{name: "Wan不套用其他供应商用量字段", hook: "extractUsageOnComplete", args: []any{map[string]any{"upstreamModel": wanModel}, map[string]any{"status": "SUCCESS"}, map[string]any{"usage": map[string]any{"input_video_duration": 20, "output_video_duration": 5}}}, want: `null`},
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
		{name: "内部时长标记不可直传", model: tokenModel, fields: map[string]any{"auto_duration": true}},
		{name: "提示覆盖参数", model: tokenModel, fields: map[string]any{"prompt": "ocean --duration 9999"}},
		{name: "参考片段未知字段", model: tokenModel, fields: map[string]any{"videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video", "duration": 1}}}},
		{name: "仅音频", model: tokenModel, fields: map[string]any{"audios": []any{map[string]any{"url": "https://cdn.example/ref.mp3", "role": "reference_audio"}}}},
		{name: "编辑时长必须自动", model: tokenModel, fields: map[string]any{"omni_reference_task_type": "edit", "duration": 5, "ratio": "adaptive", "videos": []any{map[string]any{"url": "https://cdn.example/ref.mp4", "role": "reference_video"}}}},
		{name: "Wan未验证参考输入", model: wanModel, fields: map[string]any{"images": []any{map[string]any{"url": "https://cdn.example/ref.png", "role": "reference_image"}}}},
		{name: "Wan不能自动时长", model: wanModel, fields: map[string]any{"duration": -1}},
		{name: "Wan未知参数", model: wanModel, fields: map[string]any{"generate_audio": false}},
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
