package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGrokVideoContracts 在真实插件运行时验证公开视频协议、按次用量及失败边界，不调用供应商。
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

	// 测试表的期望值独立描述上游线协议及计费契约，不复制生产转换逻辑。
	tests := []struct {
		name      string
		hook      string
		path      []string
		args      []any
		want      string
		wantError string
	}{
		{name: "视频解码保留清晰度及别名", hook: "protocols", path: []string{"openai_video", "decodeRequest"}, args: []any{map[string]any{"model": "my-grok", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "ocean", "resolution": "4k", "seconds": 10}}}}, want: `{"kind":"submit","model":"my-grok","action":"text_to_video","requestBody":{"model":"my-grok","prompt":"ocean","resolution":"4k","seconds":10}}`},
		{name: "根地址创建请求使用渠道映射模型", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example", "apiKey": "fixture-only", "upstreamModel": "grok-imagine-video-1.5", "requestBody": map[string]any{"prompt": "ocean", "resolution": "720p"}}}, want: `{"url":"https://upstream.example/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture-only","Content-Type":"application/json"},"body":{"model":"grok-imagine-video-1.5","prompt":"ocean","resolution":"720p"}}`},
		{name: "版本路径查询不会重复且编码任务ID", hook: "buildQueryRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example/proxy/v1/", "apiKey": "fixture-only", "taskId": "id/with?path"}}, want: `{"url":"https://upstream.example/proxy/v1/videos/id%2Fwith%3Fpath","method":"GET","headers":{"Authorization":"Bearer fixture-only"}}`},
		{name: "提交响应保留上游ID", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"request_id": "vendor-id", "status": "queued"}}}, want: `{"taskId":"vendor-id","taskData":{"request_id":"vendor-id","status":"queued"}}`},
		{name: "错误响应不能提交成功", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"id": "invalid", "error": "rejected"}}}, wantError: "rejected"},
		{name: "未识别包装不能伪造任务ID", hook: "parseSubmitResponse", args: []any{nil, map[string]any{"body": map[string]any{"data": map[string]any{"id": "nested"}}}}, wantError: "id is missing"},
		{name: "十秒四K只报告一次", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": "4K"}}}, want: `{"count":1,"resolution":"4k"}`},
		{name: "像素尺寸与清晰度一致可正确计档", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "size": "3840x2160", "resolution": "4k"}}}, want: `{"count":1,"resolution":"4k"}`},
		{name: "不猜测未列于官方文档的2K档", hook: "buildSubmitRequest", args: []any{map[string]any{"requestBody": map[string]any{"prompt": "ocean", "resolution": "2k"}}}, wantError: "resolution must be"},
		{name: "竖屏像素尺寸不按缺省档收费", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean", "size": "720x1280"}}}, want: `{"count":1,"resolution":"720p"}`},
		{name: "默认规格不猜测供应商清晰度", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "facts", "requestBody": map[string]any{"prompt": "ocean"}}}, want: `{"count":1,"resolution":"unspecified"}`},
		{name: "旧倍率分支不得重复乘价", hook: "extractUsage", args: []any{map[string]any{"usagePurpose": "billing_ratios", "requestBody": map[string]any{"prompt": "ocean", "seconds": 10, "resolution": "1080p"}}}, want: `{}`},
		{name: "完成时忽略上游任意时长计数", hook: "extractUsageOnComplete", args: []any{nil, map[string]any{"status": "SUCCESS"}, map[string]any{"seconds": 99999999, "count": 99999999}}, want: `{"count":1}`},
		{name: "失败没有成功用量", hook: "extractUsageOnComplete", args: []any{nil, map[string]any{"status": "FAILURE"}}, want: `null`},
		{name: "处理状态保留有效进度", hook: "parseTaskResult", args: []any{nil, map[string]any{"status": "in_progress", "progress": 40}}, want: `{"status":"IN_PROGRESS","progress":"40%"}`},
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
		{name: "multipart文件引用原样传宿主", hook: "buildSubmitRequest", args: []any{map[string]any{"baseUrl": "https://upstream.example/v1", "apiKey": "fixture-only", "upstreamModel": "grok-imagine-video-1.5", "requestBody": map[string]any{"prompt": "ocean"}, "files": []any{map[string]any{"field": "input_reference", "ref": "opaque-1", "filename": "frame.png"}}}}, want: `{"url":"https://upstream.example/v1/videos","method":"POST","headers":{"Authorization":"Bearer fixture-only"},"bodyType":"multipart","parts":[{"name":"prompt","value":"ocean"},{"name":"model","value":"grok-imagine-video-1.5"},{"name":"input_reference","fileRef":"opaque-1","filename":"frame.png"}]}`},
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

	for _, resolution := range []string{"480p", "720p", "1080p", "4k"} {
		t.Run("声明并接受清晰度_"+resolution, func(t *testing.T) {
			assert.Contains(t, plugin.Meta.UsageSchema["resolution"].Enum, resolution)
			facts, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"requestBody": map[string]any{"prompt": "ocean", "resolution": resolution}, "usagePurpose": "facts"})
			require.NoError(t, callErr)
			encoded, encodeErr := common.Marshal(facts)
			require.NoError(t, encodeErr)
			assert.JSONEq(t, `{"count":1,"resolution":"`+resolution+`"}`, string(encoded))
		})
	}
	for _, invalid := range []map[string]any{{"seconds": 0}, {"seconds": -1}, {"duration": 3601}, {"duration": "18446744073686646784"}, {"seconds": false}, {"seconds": ""}, {"seconds": 6, "duration": 8}, {"n": 2}, {"count": 0}, {"batch_size": 2}, {"resolution": "8k"}, {"resolution": "480p", "size": "4k"}, {"resolution": "720p", "size": "3840x2160"}, {"size": "8k"}, {"size": "unknown"}, {"size": "7680x4320"}} {
		t.Run("拒绝非法生成参数", func(t *testing.T) {
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

