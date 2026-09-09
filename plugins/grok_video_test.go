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

// TestGrokVideoContracts 在真实插件运行时验证公开视频协议、按秒用量及失败边界，不调用供应商。
func TestGrokVideoContracts(t *testing.T) {
	source, err := plugins.Source("grok-video")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	for _, modelName := range []string{"grok-imagine-video-1.5", "grok-imagine-video-1.5.1", "grok-imagine-video"} {
		for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
			binding, found := registry.Generation().LookupEndpoint("POST", endpoint, modelName)
			require.True(t, found, "%s 必须能从 %s 找到插件", modelName, endpoint)
			assert.Same(t, plugin, binding.Plugin)
		}
	}

	t.Run("按次模型精确注册", func(t *testing.T) {
		for _, endpoint := range []string{"/v1/videos", "/v1/responses"} {
			binding, found := registry.Generation().LookupEndpoint("POST", endpoint, "grok-imagine-video-1.5（按次）")
			if assert.True(t, found, "按次模型必须能从 %s 找到插件", endpoint) {
				assert.Same(t, plugin, binding.Plugin)
			}
		}
	})

	// 测试表的期望值独立描述上游线协议及计费契约，不复制生产转换逻辑。
	tests := []struct {
		name      string
		hook      string
		path      []string
		args      []any
		want      string
		wantError string
	}{
		{name: "完整视频地址创建请求不重复路径", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example/v1/videos", "apiKey": "fixture-only", "upstreamModel": "grok-imagine-video-1.5", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10}}}, want: `{"url":"https://upstream.example/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture-only","Content-Type":"application/json"},"body":{"model":"grok-imagine-video-1.5","prompt":"ocean","seconds":10}}`},
		{name: "完整视频地址末尾斜杠不重复路径", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example/v1/videos/", "apiKey": "fixture-only", "upstreamModel": "grok-imagine-video-1.5", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10}}}, want: `{"url":"https://upstream.example/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture-only","Content-Type":"application/json"},"body":{"model":"grok-imagine-video-1.5","prompt":"ocean","seconds":10}}`},
		{name: "按次上游模型字符串原样保留", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example", "apiKey": "fixture-only", "upstreamModel": "grok-imagine-video-1.5（按次）", "requestBody": map[string]any{"model": "client-alias", "prompt": "ocean", "seconds": 10}}}, want: `{"url":"https://upstream.example/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture-only","Content-Type":"application/json"},"body":{"model":"grok-imagine-video-1.5（按次）","prompt":"ocean","seconds":10}}`},
		{name: "真实提交响应原样保留任务数据", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"id": "task_fixture", "task_id": "task_fixture", "object": "video", "model": "grok-imagine-video-1.5", "status": "queued", "progress": 0, "created_at": 1788926514, "seconds": "10"}}}, want: `{"taskId":"task_fixture","taskData":{"id":"task_fixture","task_id":"task_fixture","object":"video","model":"grok-imagine-video-1.5","status":"queued","progress":0,"created_at":1788926514,"seconds":"10"}}`},
		{name: "拒绝HTML字符串提交响应", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": "<!DOCTYPE html><html><body>Bad Gateway</body></html>"}}, wantError: "JSON object"},
		{name: "视频解码保留清晰度及别名", hook: "protocols", path: []string{"openai_video", "decodeRequest"}, args: []any{map[string]any{"model": "my-grok", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": "4k"}}}}, want: `{"kind":"submit","model":"my-grok","action":"text_to_video","requestBody":{"model":"my-grok","prompt":"ocean","seconds":10,"resolution":"4k"}}`},
		{name: "根地址创建请求使用渠道映射模型", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example", "apiKey": "fixture-only", "upstreamModel": "grok-imagine-video-1.5", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": "720p"}}}, want: `{"url":"https://upstream.example/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture-only","Content-Type":"application/json"},"body":{"model":"grok-imagine-video-1.5","prompt":"ocean","seconds":10,"resolution":"720p"}}`},
		{name: "版本路径查询不会重复且编码任务ID", hook: "buildQueryRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example/proxy/v1/", "apiKey": "fixture-only", "taskId": "id/with?path"}}, want: `{"url":"https://upstream.example/proxy/v1/videos/id%2Fwith%3Fpath","method":"GET","headers":{"Authorization":"Bearer fixture-only"}}`},
		{name: "提交响应保留上游ID", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"request_id": "vendor-id", "status": "queued"}}}, want: `{"taskId":"vendor-id","taskData":{"request_id":"vendor-id","status":"queued"}}`},
		{name: "错误响应不能提交成功", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"id": "invalid", "error": "rejected"}}}, wantError: "rejected"},
		{name: "未识别包装不能伪造任务ID", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"data": map[string]any{"id": "nested"}}}}, wantError: "id is missing"},
		{name: "十秒四K报告秒数及档位", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": "4K"}}}, want: `{"seconds":10,"resolution":"4k"}`},
		{name: "像素尺寸与清晰度一致可正确计档", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "size": "3840x2160", "resolution": "4k"}}}, want: `{"seconds":10,"resolution":"4k"}`},
		{name: "不猜测未列于官方文档的2K档", hook: "buildSubmitRequest", args: []any{map[string]any{"requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": "2k"}}}, wantError: "resolution must be"},
		{name: "竖屏像素尺寸不按缺省档收费", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "size": "720x1280"}}}, want: `{"seconds":10,"resolution":"720p"}`},
		{name: "默认规格不猜测供应商清晰度", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10}}}, want: `{"seconds":10,"resolution":"unspecified"}`},
		{name: "旧倍率分支不得重复乘价", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "billing_ratios", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": "1080p"}}}, want: `{}`},
		{name: "完成时拒绝上游非法时长", hook: "extractUsageOnComplete", args: []any{nil, map[string]any{"status": "SUCCESS"}, map[string]any{"seconds": 99999999, "count": 99999999}}, wantError: "upstream seconds"},
		{name: "失败没有成功用量", hook: "extractUsageOnComplete", args: []any{nil, map[string]any{"status": "FAILURE"}}, want: `null`},
		{name: "处理状态保留有效进度", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "in_progress", "progress": 40}}, want: `{"status":"IN_PROGRESS","progress":"40%"}`},
		{name: "真实done响应明确成功及完整进度", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "done", "video": map[string]any{"url": "/v1/videos/task_fixture/content"}}}, want: `{"status":"SUCCESS","progress":"100%"}`},
		{name: "当前任务相对内容地址使用渠道鉴权", hook: "buildContentRequest", args: []any{map[string]any{"artifactKey": "video", "baseUrl": "https://upstream.example/v1/videos", "apiKey": "fixture-only", "upstreamTaskId": "task_fixture", "data": map[string]any{"status": "done", "video": map[string]any{"url": "/v1/videos/task_fixture/content"}}, "clientRequest": map[string]any{"method": "GET"}}}, want: `{"url":"https://upstream.example/v1/videos/task_fixture/content","method":"GET","headers":{"Authorization":"Bearer fixture-only"}}`},
		{name: "成功明确终态", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "completed"}}, want: `{"status":"SUCCESS","progress":"100%"}`},
		{name: "过期进入失败退款路径", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "expired"}}, want: `{"status":"FAILURE","reason":"upstream video generation failed"}`},
		{name: "未知状态不无限处理中", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "unexpected"}}, want: `{"status":"UNKNOWN","reason":"unrecognized video status"}`},
		{name: "成功声明视频制品", hook: "listArtifacts", args: []any{map[string]any{"status": "SUCCESS"}}, want: `[{"key":"video","type":"video"}]`},
		{name: "失败不能下载制品", hook: "listArtifacts", args: []any{map[string]any{"status": "FAILURE"}}, want: `[]`},
		{name: "标准内容接口使用渠道鉴权", hook: "buildContentRequest", args: []any{map[string]any{"artifactKey": "video", "baseUrl": "https://upstream.example/v1", "apiKey": "fixture-only", "upstreamTaskId": "vendor/id", "data": map[string]any{}, "clientRequest": map[string]any{"method": "HEAD"}}}, want: `{"url":"https://upstream.example/v1/videos/vendor%2Fid/content","method":"HEAD","headers":{"Authorization":"Bearer fixture-only"}}`},
		{name: "CDN下载不泄露渠道凭据", hook: "buildContentRequest", args: []any{map[string]any{"artifactKey": "video", "apiKey": "fixture-only", "data": map[string]any{"video_url": "https://cdn.example/video.mp4"}, "clientRequest": map[string]any{"method": "GET"}}}, want: `{"url":"https://cdn.example/video.mp4","method":"GET","credentialless":true}`},
		{name: "无效制品地址明确失败", hook: "buildContentRequest", args: []any{map[string]any{"artifactKey": "video", "data": map[string]any{"url": "file:///tmp/test"}}}, wantError: "invalid video artifact URL"},
		{name: "Responses工具不得静默丢弃", hook: "protocols", path: []string{"openai_responses", "decodeRequest"}, args: []any{map[string]any{"model": "grok-imagine-video-1.5", "body": map[string]any{"kind": "json", "value": map[string]any{"input": "ocean", "tools": []any{}}}}}, wantError: "does not support tools"},
		{name: "Responses重复成功不重发视频", hook: "protocols", path: []string{"openai_responses", "renderEvents"}, args: []any{nil, map[string]any{"status": "SUCCESS"}, map[string]any{"status": "SUCCESS"}}, want: `{"events":[],"state":{"status":"SUCCESS","progress":""},"done":true}`},
		{name: "multipart文件引用原样传宿主", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example/v1", "apiKey": "fixture-only", "upstreamModel": "grok-imagine-video-1.5", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10}, "files": []any{map[string]any{"field": "input_reference", "ref": "opaque-1", "filename": "frame.png"}}}}, want: `{"url":"https://upstream.example/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture-only"},"bodyType":"multipart","parts":[{"name":"prompt","value":"ocean"},{"name":"seconds","value":10},{"name":"model","value":"grok-imagine-video-1.5"},{"name":"input_reference","fileRef":"opaque-1","filename":"frame.png"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result any
			var callErr error
			if len(test.path) > 0 {
				result, callErr = plugin.Engine.CallPath(t.Context(), test.hook, test.path, test.args...)
			} else {
				result, callErr = plugin.Engine.Call(t.Context(), test.hook, test.args...)
			}
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

	// 只接受当前任务的标准内容路径，其他相对地址不能携带渠道凭据。
	for _, artifactURL := range []string{"/video.mp4", "video.mp4", "../content", "//cdn.example/video.mp4", "/v1/videos/other_task/content", "/v1/videos/task_fixture/content?download=1", "/v1/videos/task_fixture/content/../other"} {
		t.Run("拒绝非当前任务标准内容相对地址_"+artifactURL, func(t *testing.T) {
			_, callErr := plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{
				"artifactKey":    "video",
				"baseUrl":        "https://upstream.example/v1/videos",
				"apiKey":         "fixture-only",
				"upstreamTaskId": "task_fixture",
				"data":           map[string]any{"video": map[string]any{"url": artifactURL}},
				"clientRequest":  map[string]any{"method": "GET"},
			})
			require.ErrorContains(t, callErr, "invalid video artifact URL")
		})
	}

	for _, resolution := range []string{"480p", "720p", "1080p", "4k"} {
		t.Run("声明并接受清晰度_"+resolution, func(t *testing.T) {
			assert.Contains(t, plugin.Meta.UsageSchema["resolution"].Enum, resolution)
			facts, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": resolution}, "usagePurpose": "facts"})
			require.NoError(t, callErr)
			encoded, encodeErr := common.Marshal(facts)
			require.NoError(t, encodeErr)
			assert.JSONEq(t, `{"seconds":10,"resolution":"`+resolution+`"}`, string(encoded))
		})
	}
	for _, invalid := range []map[string]any{{}, {"seconds": nil}, {"seconds": 0}, {"seconds": -1}, {"duration": 3601}, {"duration": "18446744073686646784"}, {"seconds": false}, {"seconds": ""}, {"seconds": 6, "duration": 8}, {"n": 2}, {"count": 0}, {"batch_size": 2}, {"resolution": "8k"}, {"resolution": "480p", "size": "4k"}, {"resolution": "720p", "size": "3840x2160"}, {"size": "8k"}, {"size": "unknown"}, {"size": "7680x4320"}} {
		t.Run("拒绝非法生成参数", func(t *testing.T) {
			if len(invalid) > 0 {
				if _, seconds := invalid["seconds"]; !seconds {
					if _, duration := invalid["duration"]; !duration {
						invalid["seconds"] = 10
					}
				}
			}
			invalid["prompt"] = "ocean"
			_, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"requestBody": invalid, "usagePurpose": "facts"})
			require.Error(t, callErr)
			_, callErr = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "grok-imagine-video-1.5", "body": map[string]any{"kind": "json", "value": invalid}})
			require.Error(t, callErr)
			invalid["input"] = "ocean"
			_, callErr = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_responses", "decodeRequest"}, map[string]any{"model": "grok-imagine-video-1.5", "body": map[string]any{"kind": "json", "value": invalid}})
			require.Error(t, callErr)
		})
	}
}

// TestGrokVideoSecondsBilling 验证按秒分档表达式、完成用量覆盖和透传参数边界，不调用付费上游。
func TestGrokVideoSecondsBilling(t *testing.T) {
	source, err := plugins.Source("grok-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{})
	require.NoError(t, err)
	assert.Contains(t, plugin.Meta.UsageSchema, "seconds")
	assert.NotContains(t, plugin.Meta.UsageSchema, "count")
	// 单价仅为测试夹具，不代表供应商报价；缺省档必须与显式清晰度独立计费。
	const expression = `u("resolution") == "unspecified" ? tier("unspecified", u("seconds") * 0.1) : u("resolution") == "480p" ? tier("480p", u("seconds") * 0.2) : u("resolution") == "720p" ? tier("720p", u("seconds") * 0.3) : u("resolution") == "1080p" ? tier("1080p", u("seconds") * 0.4) : tier("4k", u("seconds") * 0.5)`
	for _, test := range []struct {
		resolution string
		price      float64
		quota10    int
	}{
		{"unspecified", 0.1, 500000}, {"480p", 0.2, 1000000}, {"720p", 0.3, 1500000}, {"1080p", 0.4, 2000000}, {"4k", 0.5, 2500000},
	} {
		t.Run(test.resolution, func(t *testing.T) {
			for _, seconds := range []int{6, 10} {
				body := map[string]any{"prompt": "ocean", "seconds": seconds}
				if test.resolution != "unspecified" {
					body["resolution"] = test.resolution
				}
				facts, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"requestBody": body, "usagePurpose": "facts"})
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(facts)
				require.NoError(t, encodeErr)
				var usage map[string]any
				require.NoError(t, common.Unmarshal(encoded, &usage))
				cost, trace, runErr := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: usage})
				require.NoError(t, runErr)
				assert.InDelta(t, float64(seconds)*test.price, cost, 1e-12)
				assert.Equal(t, test.resolution, trace.MatchedTier)
				completion, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, map[string]any{"status": "SUCCESS", "data": map[string]any{"seconds": 99}}, map[string]any{"duration": "10", "resolution": "480p"})
				require.NoError(t, callErr)
				encoded, encodeErr = common.Marshal(completion)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, `{"seconds":10}`, string(encoded))
				// 宿主按键覆盖完成秒数，冻结的请求清晰度不得被上游改写。
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
		{name: "负时长", body: map[string]any{"duration": -1}, invalid: true},
		{name: "超上限", body: map[string]any{"seconds": 3601}, invalid: true},
		{name: "溢出字符串", body: map[string]any{"duration": "18446744073686646784"}, invalid: true},
		{name: "非有限数", body: map[string]any{"seconds": "Infinity"}, invalid: true},
		{name: "布尔时长", body: map[string]any{"seconds": false}, invalid: true},
		{name: "空时长", body: map[string]any{"seconds": nil}, invalid: true},
		{name: "空白时长", body: map[string]any{"duration": " "}, invalid: true},
		{name: "时长冲突", body: map[string]any{"seconds": 6, "duration": 8}, invalid: true},
		{name: "小数秒", body: map[string]any{"seconds": 0.5}, want: `{"seconds":0.5}`},
		{name: "时长别名", body: map[string]any{"duration": "16"}, want: `{"seconds":16}`},
		{name: "时长上限", body: map[string]any{"seconds": 3600}, want: `{"seconds":3600}`},
		{name: "两字段等值", body: map[string]any{"seconds": 6, "duration": "6"}, want: `{"seconds":6}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.body["prompt"] = "ocean"
			ctx := map[string]any{"requestBody": test.body, "usagePurpose": "facts", "baseUrl": "https://upstream.example", "upstreamModel": "grok-imagine-video-1.5"}
			for _, hook := range []string{"extractUsage", "buildSubmitRequest"} {
				result, callErr := plugin.Engine.Call(t.Context(), hook, ctx)
				if test.invalid {
					require.Error(t, callErr, hook)
				} else {
					require.NoError(t, callErr, hook)
					if hook == "extractUsage" {
						encoded, encodeErr := common.Marshal(result)
						require.NoError(t, encodeErr)
						assert.JSONEq(t, test.want[:len(test.want)-1]+`,"resolution":"unspecified"}`, string(encoded))
					}
				}
			}
			test.body["input"] = "ocean"
			for _, protocol := range []string{"openai_video", "openai_responses"} {
				_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{"model": "grok-imagine-video-1.5", "body": map[string]any{"kind": "json", "value": test.body}})
				if test.invalid {
					require.Error(t, callErr, protocol)
				} else {
					require.NoError(t, callErr, protocol)
				}
			}
			completion, callErr := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, map[string]any{"status": "SUCCESS"}, test.body)
			if test.name == "缺少时长" {
				require.NoError(t, callErr)
				assert.Nil(t, completion)
			} else if test.invalid {
				require.Error(t, callErr)
			} else {
				require.NoError(t, callErr)
				encoded, encodeErr := common.Marshal(completion)
				require.NoError(t, encodeErr)
				assert.JSONEq(t, test.want, string(encoded))
			}
			completion, callErr = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, map[string]any{"status": "FAILURE"}, test.body)
			require.NoError(t, callErr)
			assert.Nil(t, completion)
		})
	}
	for _, container := range []string{"metadata", "parameters"} {
		for _, key := range []string{"seconds", "duration", "resolution", "size", "n", "count", "batch_size"} {
			for _, stringify := range []bool{false, true} {
				t.Run(container+"_"+key+map[bool]string{false: "_对象", true: "_JSON字符串"}[stringify], func(t *testing.T) {
					var extra any = map[string]any{key: 1}
					if stringify {
						encoded, encodeErr := common.Marshal(extra)
						require.NoError(t, encodeErr)
						extra = string(encoded)
					}
					body := map[string]any{"prompt": "ocean", "input": "ocean", "seconds": 6, container: extra}
					for _, hook := range []string{"extractUsage", "buildSubmitRequest"} {
						_, callErr := plugin.Engine.Call(t.Context(), hook, map[string]any{"requestBody": body, "usagePurpose": "facts", "baseUrl": "https://upstream.example"})
						require.Error(t, callErr, hook)
					}
					for _, protocol := range []string{"openai_video", "openai_responses"} {
						_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{"model": "grok-imagine-video-1.5", "body": map[string]any{"kind": "json", "value": body}})
						require.Error(t, callErr, protocol)
					}
				})
			}
		}
	}
}
