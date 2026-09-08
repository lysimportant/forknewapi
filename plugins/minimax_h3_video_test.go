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

// TestMiniMaxH3VideoContracts 使用真实宿主引擎验证第三方视频协议与按秒用量，不调用付费上游。
func TestMiniMaxH3VideoContracts(t *testing.T) {
	source, err := plugins.Source("minimax-h3-video")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	hailuoSource, err := plugins.Source("hailuo")
	require.NoError(t, err)
	hailuo, err := registry.RegisterFactory(hailuoSource, jsplugin.Options{})
	require.NoError(t, err)

	for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
		binding, found := registry.Generation().LookupEndpoint("POST", endpoint, "MiniMax-H3")
		require.True(t, found)
		assert.Same(t, hailuo, binding.Plugin, "原生 MiniMax-H3 必须仍归属 hailuo")
	}

	// 档位来自已确认的真实模型名称；不根据插件实现生成测试期望。
	models := []struct {
		name       string
		resolution string
		size       string
		conflict   string
	}{
		{"minimax_h3-1080p", "1080P", "1920x1080", "768P"},
		{"minimax_h3-2K", "2K", "1440x2560", "1080P"},
		{"minimax_h3-768p", "768P", "1366x768", "2K"},
	}
	for _, model := range models {
		t.Run(model.name, func(t *testing.T) {
			for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
				binding, found := registry.Generation().LookupEndpoint("POST", endpoint, model.name)
				require.True(t, found)
				assert.Same(t, plugin, binding.Plugin)
			}
			assert.Contains(t, plugin.Meta.UsageSchema["resolution"].Enum, model.resolution)
			for _, protocol := range []string{"openai_video", "openai_responses"} {
				t.Run(protocol, func(t *testing.T) {
					for _, explicit := range []bool{false, true} {
						body := map[string]any{"model": "channel-alias", "seconds": 6}
						if protocol == "openai_video" {
							body["prompt"] = "ocean"
						} else {
							body["input"] = "ocean"
						}
						if explicit {
							body["resolution"] = model.resolution
							body["size"] = model.size
						}
						decoded, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
							"model": "channel-alias", "upstreamModel": model.name,
							"body": map[string]any{"kind": "json", "value": body},
						})
						require.NoError(t, callErr)
						encoded, encodeErr := common.Marshal(decoded)
						require.NoError(t, encodeErr)
						var command struct {
							Kind        string         `json:"kind"`
							Model       string         `json:"model"`
							Action      string         `json:"action"`
							RequestBody map[string]any `json:"requestBody"`
						}
						require.NoError(t, common.Unmarshal(encoded, &command))
						assert.Equal(t, "submit", command.Kind)
						assert.Equal(t, "channel-alias", command.Model)
						assert.Equal(t, "text_to_video", command.Action)
						ctx := map[string]any{
							"baseUrl": "https://upstream.example", "apiKey": "fixture-only",
							"model": command.Model, "upstreamModel": model.name, "requestBody": command.RequestBody,
							"usagePurpose": "facts",
						}
						request, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
						require.NoError(t, callErr)
						encoded, encodeErr = common.Marshal(request)
						require.NoError(t, encodeErr)
						wantBody := map[string]any{"model": model.name, "prompt": "ocean", "seconds": 6}
						if explicit {
							wantBody["resolution"] = model.resolution
							wantBody["size"] = model.size
						}
						want, encodeErr := common.Marshal(map[string]any{
							"url": "https://upstream.example/v1/videos", "method": "POST",
							"headers": map[string]any{"Authorization": "Bearer fixture-only", "Content-Type": "application/json"},
							"body":    wantBody,
						})
						require.NoError(t, encodeErr)
						assert.JSONEq(t, string(want), string(encoded), "模型原样透传；未指定清晰度时不得补参数")
						facts, callErr := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
						require.NoError(t, callErr)
						encoded, encodeErr = common.Marshal(facts)
						require.NoError(t, encodeErr)
						var usage map[string]any
						require.NoError(t, common.Unmarshal(encoded, &usage))
						assert.Equal(t, model.resolution, usage["resolution"])
						assert.Equal(t, float64(6), usage["seconds"])
						assert.NotContains(t, usage, "count")
					}
				})
			}
			for _, field := range []string{"resolution", "size"} {
				t.Run("拒绝冲突_"+field, func(t *testing.T) {
					body := map[string]any{"model": model.name, "prompt": "ocean", "seconds": 6, field: model.conflict}
					ctx := map[string]any{"model": "channel-alias", "upstreamModel": model.name, "requestBody": body, "usagePurpose": "facts", "baseUrl": "https://upstream.example"}
					for _, hook := range []string{"buildSubmitRequest", "extractUsage"} {
						_, callErr := plugin.Engine.Call(t.Context(), hook, ctx)
						require.ErrorContains(t, callErr, "conflict", hook)
					}
					body["input"] = "ocean"
					for _, protocol := range []string{"openai_video", "openai_responses"} {
						_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
							"model": "channel-alias", "upstreamModel": model.name, "body": map[string]any{"kind": "json", "value": body},
						})
						require.ErrorContains(t, callErr, "conflict", protocol)
					}
				})
			}
		})
	}

	// 查询及终态断言固定 OpenAI 线协议，防止误转为 MiniMax 原生接口或将失败识别为成功。
	tests := []struct {
		name      string
		hook      string
		args      []any
		want      string
		wantError string
	}{
		{name: "查询保留版本前缀并编码ID", hook: "buildQueryRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example/proxy/v1/", "apiKey": "fixture-only", "taskId": "id/with?path"}}, want: `{"url":"https://upstream.example/proxy/v1/videos/id%2Fwith%3Fpath","method":"GET","headers":{"Authorization":"Bearer fixture-only"}}`},
		{name: "提交保留上游ID", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"id": "vendor-id", "status": "queued"}}}, want: `{"taskId":"vendor-id","taskData":{"id":"vendor-id","status":"queued"}}`},
		{name: "上游错误拒绝提交", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"id": "invalid", "error": "rejected"}}}, wantError: "rejected"},
		{name: "排队", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "queued"}}, want: `{"status":"QUEUED"}`},
		{name: "处理中进度", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "in_progress", "progress": 40}}, want: `{"status":"IN_PROGRESS","progress":"40%"}`},
		{name: "完成", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "completed"}}, want: `{"status":"SUCCESS","progress":"100%"}`},
		{name: "失败", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "failed"}}, want: `{"status":"FAILURE","reason":"upstream video generation failed"}`},
		{name: "过期", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "expired"}}, want: `{"status":"FAILURE","reason":"upstream video generation failed"}`},
		{name: "未知状态", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "unexpected"}}, want: `{"status":"UNKNOWN","reason":"unrecognized video status"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, callErr := plugin.Engine.Call(t.Context(), test.hook, test.args...)
			if test.wantError != "" {
				require.ErrorContains(t, callErr, test.wantError)
				return
			}
			require.NoError(t, callErr)
			encoded, encodeErr := common.Marshal(result)
			require.NoError(t, encodeErr)
			assert.JSONEq(t, test.want, string(encoded))
		})
	}
}

// TestMiniMaxH3VideoSecondsBilling 验证三档独立每秒价格、宿主额度换算和完成时长覆盖；价格仅为测试夹具。
func TestMiniMaxH3VideoSecondsBilling(t *testing.T) {
	source, err := plugins.Source("minimax-h3-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, "number", plugin.Meta.UsageSchema["seconds"].Type)
	assert.Equal(t, "second", plugin.Meta.UsageSchema["seconds"].Unit)
	assert.NotContains(t, plugin.Meta.UsageSchema, "count")
	// 测试单价分别为 0.10、0.20、0.40 美元/秒，不代表供应商商业报价。
	const expression = `u("resolution") == "768P" ? tier("768P", u("seconds") * 0.1) : u("resolution") == "1080P" ? tier("1080P", u("seconds") * 0.2) : tier("2K", u("seconds") * 0.4)`
	for _, test := range []struct {
		model      string
		resolution string
		cost6      float64
		cost10     float64
		quota10    int
	}{
		{"minimax_h3-768p", "768P", 0.6, 1, 500000},
		{"minimax_h3-1080p", "1080P", 1.2, 2, 1000000},
		{"minimax_h3-2K", "2K", 2.4, 4, 2000000},
	} {
		t.Run(test.model, func(t *testing.T) {
			for _, seconds := range []int{6, 10} {
				ctx := map[string]any{"upstreamModel": test.model, "usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "seconds": seconds}}
				facts, callErr := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(facts)
				require.NoError(t, encodeErr)
				var usage map[string]any
				require.NoError(t, common.Unmarshal(encoded, &usage))
				cost, trace, runErr := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: usage})
				require.NoError(t, runErr)
				wantCost := test.cost6
				if seconds == 10 {
					wantCost = test.cost10
				}
				assert.InDelta(t, wantCost, cost, 1e-12)
				assert.Equal(t, test.resolution, trace.MatchedTier)
				ctx["usagePurpose"] = "billing_ratios"
				ratios, callErr := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
				require.NoError(t, callErr)
				encoded, encodeErr = common.Marshal(ratios)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, `{}`, string(encoded))

				completion, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, map[string]any{"status": "SUCCESS"}, map[string]any{"duration": "10"})
				require.NoError(t, callErr)
				encoded, encodeErr = common.Marshal(completion)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, `{"seconds":10}`, string(encoded))
				// 宿主按键覆盖完成事实，保留冻结请求的清晰度档位。
				require.NoError(t, common.Unmarshal(encoded, &usage))
				settled, settleErr := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{
					ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1,
					QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true,
				}, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: usage})
				require.NoError(t, settleErr)
				assert.Equal(t, test.quota10, settled.ActualQuotaAfterGroup)
			}
		})
	}

	for _, test := range []struct {
		name    string
		body    map[string]any
		want    string
		invalid bool
	}{
		{name: "缺少时长", body: map[string]any{}, invalid: true},
		{name: "零秒", body: map[string]any{"seconds": 0}, invalid: true},
		{name: "负数", body: map[string]any{"duration": -1}, invalid: true},
		{name: "超限", body: map[string]any{"seconds": 3601}, invalid: true},
		{name: "溢出字符串", body: map[string]any{"duration": "18446744073686646784"}, invalid: true},
		{name: "布尔值", body: map[string]any{"seconds": false}, invalid: true},
		{name: "空字符串", body: map[string]any{"duration": ""}, invalid: true},
		{name: "空值", body: map[string]any{"seconds": nil}, invalid: true},
		{name: "时长冲突", body: map[string]any{"seconds": 6, "duration": 10}, invalid: true},
		{name: "低于原生H3下限仍接受", body: map[string]any{"seconds": 1}, want: `{"seconds":1,"resolution":"1080P"}`},
		{name: "高于原生H3上限仍接受", body: map[string]any{"duration": "16"}, want: `{"seconds":16,"resolution":"1080P"}`},
		{name: "宿主上限", body: map[string]any{"seconds": 3600}, want: `{"seconds":3600,"resolution":"1080P"}`},
		{name: "两字段等值", body: map[string]any{"seconds": 6, "duration": "6"}, want: `{"seconds":6,"resolution":"1080P"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.body["prompt"] = "ocean"
			ctx := map[string]any{"upstreamModel": "minimax_h3-1080p", "usagePurpose": "facts", "requestBody": test.body, "baseUrl": "https://upstream.example"}
			facts, callErr := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			if test.invalid {
				require.Error(t, callErr)
				_, callErr = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
				require.Error(t, callErr)
				test.body["input"] = "ocean"
				for _, protocol := range []string{"openai_video", "openai_responses"} {
					_, callErr = plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
						"model": "minimax_h3-1080p", "body": map[string]any{"kind": "json", "value": test.body},
					})
					require.Error(t, callErr, protocol)
				}
			} else {
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(facts)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, test.want, string(encoded))
			}
			completion, completionErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, map[string]any{"status": "SUCCESS"}, test.body)
			if test.name == "缺少时长" {
				require.NoError(t, completionErr)
				assert.Nil(t, completion, "无完成时长时保留预扣事实")
			} else if test.invalid {
				require.Error(t, completionErr)
			} else {
				require.NoError(t, completionErr)
				require.NotNil(t, completion)
			}
			completion, completionErr = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, map[string]any{"status": "FAILURE"}, test.body)
			require.NoError(t, completionErr)
			assert.Nil(t, completion)
		})
	}
	t.Run("metadata不得绕过时长校验", func(t *testing.T) {
		_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"upstreamModel": "minimax_h3-1080p", "baseUrl": "https://upstream.example",
			"requestBody": map[string]any{"prompt": "ocean", "seconds": 6, "metadata": `{"duration":18446744073686646784}`},
		})
		require.ErrorContains(t, callErr, "metadata")
	})
}
