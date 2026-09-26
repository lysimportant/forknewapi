package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestResponsesValidationHTTPStatus 验证真实 Relay HTTP 入口拒绝非法客户端输入时返回 400，保留体积超限的 413，且不请求上游或改变余额。
func TestResponsesValidationHTTPStatus(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMode := common.RedisEnabled, gin.Mode()
	previousRetries, previousMaxBody := common.RetryTimes, constant.MaxRequestBodyMB
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedis
		gin.SetMode(previousMode)
		common.RetryTimes, constant.MaxRequestBodyMB = previousRetries, previousMaxBody
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	common.RetryTimes = 3
	constant.MaxRequestBodyMB = 1

	user := model.User{Username: "validation-user", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, UsedQuota: 17}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "test-only-validation-token", Status: common.TokenStatusEnabled, RemainQuota: 500000, UsedQuota: 9, Group: "default"}
	require.NoError(t, db.Create(&token).Error)
	// 两个计数区分已进入真实控制器和实际出站请求，失败重试不能隐藏在成功返回之后。
	var upstreamCalls, relayCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Name: "validation-fixture", Key: "test-only-upstream-token", BaseURL: &upstream.URL, Models: "deepseek-v4-flash-vision-exp", Group: "default"}
	require.NoError(t, db.Create(&channel).Error)

	engine := gin.New()
	engine.Use(middleware.BodyStorageCleanup())
	engine.Use(func(c *gin.Context) {
		c.Set(common.RequestIdKey, "responses-validation-test")
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
		common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
		common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		if err := middleware.SetupContextForSelectedChannel(c, &channel, "deepseek-v4-flash-vision-exp"); !assert.Nil(t, err) {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	for _, endpoint := range []struct {
		path   string
		format types.RelayFormat
	}{
		{"/responses", types.RelayFormatOpenAIResponses},
		{"/v1/responses", types.RelayFormatOpenAIResponses},
		{"/responses/compact", types.RelayFormatOpenAIResponsesCompaction},
		{"/v1/responses/compact", types.RelayFormatOpenAIResponsesCompaction},
	} {
		engine.POST(endpoint.path, func(c *gin.Context) {
			relayCalls.Add(1)
			Relay(c, endpoint.format)
		})
	}
	relayServer := httptest.NewServer(engine)
	t.Cleanup(relayServer.Close)
	oversized := `{"model":"deepseek-v4-flash-vision-exp","input":"` + strings.Repeat("x", 1<<20) + `"}`
	for _, test := range []struct {
		name, path, body, message string
		status                    int
		code                      string
	}{
		{"输出上限越界", "/responses", `{"model":"deepseek-v4-flash-vision-exp","input":"test","max_output_tokens":2147483647}`, "max_output_tokens is invalid", http.StatusBadRequest, "invalid_request"},
		{"缺少模型", "/v1/responses", `{"input":"test"}`, "model is required", http.StatusBadRequest, "invalid_request"},
		{"缺少输入", "/responses", `{"model":"deepseek-v4-flash-vision-exp"}`, "input is required", http.StatusBadRequest, "invalid_request"},
		{"输出上限类型错误", "/v1/responses", `{"model":"deepseek-v4-flash-vision-exp","input":"test","max_output_tokens":"invalid"}`, "max_output_tokens", http.StatusBadRequest, "invalid_request"},
		{"非法JSON", "/responses", `{"model":`, "", http.StatusBadRequest, "invalid_request"},
		{"压缩缺少模型", "/responses/compact", `{"input":[]}`, "model is required", http.StatusBadRequest, "invalid_request"},
		{"压缩非法JSON", "/v1/responses/compact", `{"model":`, "", http.StatusBadRequest, "invalid_request"},
		{"请求体超限", "/responses", oversized, "request body", http.StatusRequestEntityTooLarge, "read_request_body_failed"},
		{"压缩请求体超限", "/v1/responses/compact", oversized, "request body", http.StatusRequestEntityTooLarge, "read_request_body_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			beforeCalls := relayCalls.Load()
			response, err := relayServer.Client().Post(relayServer.URL+test.path, "application/json", strings.NewReader(test.body))
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			assert.Equal(t, test.status, response.StatusCode, string(body))
			assert.Contains(t, response.Header.Get("Content-Type"), "application/json")
			assert.Equal(t, test.code, gjson.GetBytes(body, "error.code").String())
			assert.Contains(t, gjson.GetBytes(body, "error.message").String(), test.message)
			assert.Contains(t, gjson.GetBytes(body, "error.message").String(), "responses-validation-test")
			assert.Equal(t, beforeCalls+1, relayCalls.Load())
			assert.Zero(t, upstreamCalls.Load(), "非法请求必须在任何上游调用及重试之前返回")

			var actualUser model.User
			var actualToken model.Token
			require.NoError(t, db.First(&actualUser, user.Id).Error)
			require.NoError(t, db.First(&actualToken, token.Id).Error)
			assert.Equal(t, user.Quota, actualUser.Quota)
			assert.Equal(t, user.UsedQuota, actualUser.UsedQuota)
			assert.Equal(t, token.RemainQuota, actualToken.RemainQuota)
			assert.Equal(t, token.UsedQuota, actualToken.UsedQuota)
			var consumeLogs int64
			require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&consumeLogs).Error)
			assert.Zero(t, consumeLogs, "客户端参数错误不能产生消费日志")
		})
	}
}

// groupBridgeHTTPChatResponse 是本地 Chat 上游的固定成功响应，用量单位为 token。
const groupBridgeHTTPChatResponse = `{"id":"chatcmpl_bridge_http","object":"chat.completion","created":1,"model":"group-bridge-http-model","choices":[{"index":0,"message":{"role":"assistant","content":"Hello bridge"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`

// groupBridgeHTTPResponsesResponse 是与 Chat 样例内容和用量等价的本地 Responses 响应。
const groupBridgeHTTPResponsesResponse = `{"id":"resp_bridge_http","object":"response","created_at":1,"status":"completed","model":"group-bridge-http-model","output":[{"id":"msg_bridge_http","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello bridge","annotations":[]}]}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}`

// groupBridgeHTTPRequest 记录真实出站路径、请求体和测试渠道标识；authorization 仅含夹具占位密钥。
type groupBridgeHTTPRequest struct {
	path          string
	body          []byte
	authorization string
}

// newGroupBridgeHTTPFixture 沿用真实 Relay HTTP 夹具，以 auto 的已选实际组驱动同一 OpenAI 渠道。
// handler 为 nil 时返回固定 JSON，否则由调用方提供本地上游响应；返回客户端服务和出站记录。
// 备用渠道仅供观察错误重试，配置、内存数据库和服务均在测试结束时恢复或关闭，不访问外部 API。
func newGroupBridgeHTTPFixture(t *testing.T, channelSettings dto.ChannelSettings, handler http.HandlerFunc) (*httptest.Server, <-chan groupBridgeHTTPRequest) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	previousConsume, previousBatch := common.LogConsumeEnabled, common.BatchUpdateEnabled
	previousMode, previousRetries := gin.Mode(), common.RetryTimes
	previousCount, previousForceUsage := constant.CountToken, constant.ForceStreamOption
	previousRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	quotaSettings := operation_setting.GetQuotaSetting()
	previousFreePreconsume := quotaSettings.EnableFreeModelPreConsume
	previousPrices := ratio_setting.ModelPrice2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousGroups := setting.UserUsableGroups2JSONString()
	settings := model_setting.GetGlobalSettings()
	previousConfig, err := config.ConfigToMap(settings)
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
		common.LogConsumeEnabled, common.BatchUpdateEnabled = previousConsume, previousBatch
		gin.SetMode(previousMode)
		common.RetryTimes = previousRetries
		constant.CountToken, constant.ForceStreamOption = previousCount, previousForceUsage
		operation_setting.AutomaticRetryStatusCodeRanges = previousRetryRanges
		quotaSettings.EnableFreeModelPreConsume = previousFreePreconsume
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
		require.NoError(t, config.UpdateConfigFromMap(settings, previousConfig))
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	common.MemoryCacheEnabled, common.LogConsumeEnabled, common.BatchUpdateEnabled = false, false, false
	common.RetryTimes = 2
	constant.CountToken, constant.ForceStreamOption = false, true
	quotaSettings.EnableFreeModelPreConsume = false
	// 即使管理员允许重试 400，桥接与透传的本地冲突仍必须阻止备用渠道出站。
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 400, End: 400}, {Start: 500, End: 500}}
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"group-bridge-http-model":0}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"openclaw":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","openclaw":"OpenClaw"}`))
	require.NoError(t, config.UpdateConfigFromMap(settings, map[string]string{
		"group_openai_protocol_bridge":         "{}",
		"pass_through_request_enabled":         "false",
		"chat_completions_to_responses_policy": `{"enabled":false}`,
	}))
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}

	requests := make(chan groupBridgeHTTPRequest, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			http.Error(w, "cannot read fixture request", http.StatusBadRequest)
			return
		}
		requests <- groupBridgeHTTPRequest{path: r.URL.Path, body: body, authorization: r.Header.Get("Authorization")}
		if handler != nil {
			handler(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, err = io.WriteString(w, groupBridgeHTTPChatResponse)
		case "/v1/responses":
			_, err = io.WriteString(w, groupBridgeHTTPResponsesResponse)
		default:
			http.Error(w, "unexpected upstream path", http.StatusNotFound)
		}
		assert.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)
	user := model.User{Username: "group-bridge-http-user", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "test-only-group-bridge-token", Status: common.TokenStatusEnabled, Group: "auto", UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	encodedSettings, err := common.Marshal(channelSettings)
	require.NoError(t, err)
	channelSettingsJSON := string(encodedSettings)
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "group-bridge-primary", Key: "test-only-primary-key", BaseURL: &upstream.URL, Models: "group-bridge-http-model", Group: "openclaw,default", Setting: &channelSettingsJSON}
	require.NoError(t, db.Create(&channel).Error)
	// 初次渠道由夹具固定；若 SkipRetry 失效，下一次选择必然命中不受 OpenAI 组覆盖影响的备用渠道。
	fallback := model.Channel{Type: constant.ChannelTypeOpenRouter, Status: common.ChannelStatusEnabled, Name: "group-bridge-retry-witness", Key: "test-only-fallback-key", BaseURL: &upstream.URL, Models: channel.Models, Group: channel.Group}
	require.NoError(t, db.Create(&fallback).Error)
	for _, group := range []string{"openclaw", "default"} {
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: channel.Models, ChannelId: fallback.Id, Enabled: true, Weight: 100}).Error)
	}

	engine := gin.New()
	engine.Use(middleware.BodyStorageCleanup())
	engine.Use(func(c *gin.Context) {
		group := c.GetHeader("X-Test-Group")
		if group != "openclaw" && group != "default" {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Set(common.RequestIdKey, "group-bridge-http-test")
		common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
		common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
		common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
		common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
		common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, []string{group})
		if apiErr := middleware.SetupContextForSelectedChannel(c, &channel, channel.Models); !assert.Nil(t, apiErr) {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	engine.POST("/v1/chat/completions", func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
	engine.POST("/v1/responses", func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIResponses) })
	server := httptest.NewServer(engine)
	server.Client().Timeout = 10 * time.Second
	t.Cleanup(server.Close)
	return server, requests
}

// TestGroupOpenAIProtocolBridgeHTTPRouting 验证同一渠道的两个实际组独立选上游协议，客户端路径、响应协议及用量保持不变。
func TestGroupOpenAIProtocolBridgeHTTPRouting(t *testing.T) {
	for _, policy := range []struct {
		groupMode, channelMode dto.OpenAIProtocolBridge
		groupPath, channelPath string
	}{
		{dto.OpenAIProtocolBridgeChat, dto.OpenAIProtocolBridgeResponses, "/v1/chat/completions", "/v1/responses"},
		{dto.OpenAIProtocolBridgeResponses, dto.OpenAIProtocolBridgeChat, "/v1/responses", "/v1/chat/completions"},
	} {
		t.Run(string(policy.groupMode), func(t *testing.T) {
			server, requests := newGroupBridgeHTTPFixture(t, dto.ChannelSettings{OpenAIProtocolBridge: policy.channelMode}, nil)
			encoded, err := common.Marshal(map[string]dto.OpenAIProtocolBridge{"openclaw": policy.groupMode})
			require.NoError(t, err)
			require.NoError(t, config.UpdateConfigFromMap(model_setting.GetGlobalSettings(), map[string]string{"group_openai_protocol_bridge": string(encoded)}))
			for _, group := range []string{"openclaw", "default"} {
				for _, endpoint := range []struct {
					path, body string
				}{
					{"/v1/chat/completions", `{"model":"group-bridge-http-model","messages":[{"role":"user","content":"Say hello"}]}`},
					{"/v1/responses", `{"model":"group-bridge-http-model","input":"Say hello"}`},
				} {
					t.Run(group+endpoint.path, func(t *testing.T) {
						request, err := http.NewRequest(http.MethodPost, server.URL+endpoint.path, strings.NewReader(endpoint.body))
						require.NoError(t, err)
						request.Header.Set("Content-Type", "application/json")
						request.Header.Set("X-Test-Group", group)
						response, err := server.Client().Do(request)
						require.NoError(t, err)
						body, err := io.ReadAll(response.Body)
						require.NoError(t, response.Body.Close())
						require.NoError(t, err)
						require.Equal(t, http.StatusOK, response.StatusCode, string(body))
						assert.Contains(t, response.Header.Get("Content-Type"), "application/json")
						require.Len(t, requests, 1, "每个请求只能调用一次上游，不得协议探测或重发")
						upstream := <-requests
						expectedPath := policy.channelPath
						if group == "openclaw" {
							expectedPath = policy.groupPath
						}
						assert.Equal(t, expectedPath, upstream.path)
						assert.Equal(t, "Bearer test-only-primary-key", upstream.authorization)
						assert.Equal(t, "group-bridge-http-model", gjson.GetBytes(upstream.body, "model").String())
						assert.Equal(t, expectedPath == "/v1/chat/completions", gjson.GetBytes(upstream.body, "messages").Exists())
						assert.Equal(t, expectedPath == "/v1/responses", gjson.GetBytes(upstream.body, "input").Exists())
						result := gjson.ParseBytes(body)
						if endpoint.path == "/v1/chat/completions" {
							assert.Equal(t, "chat.completion", result.Get("object").String())
							assert.Equal(t, "Hello bridge", result.Get("choices.0.message.content").String())
							assert.Equal(t, "stop", result.Get("choices.0.finish_reason").String())
							assert.Equal(t, int64(7), result.Get("usage.prompt_tokens").Int())
							assert.Equal(t, int64(3), result.Get("usage.completion_tokens").Int())
							assert.False(t, result.Get("output").Exists())
						} else {
							assert.Equal(t, "response", result.Get("object").String())
							assert.Equal(t, "completed", result.Get("status").String())
							assert.Equal(t, "Hello bridge", result.Get(`output.#(type=="message").content.0.text`).String())
							assert.Equal(t, int64(7), result.Get("usage.input_tokens").Int())
							assert.Equal(t, int64(3), result.Get("usage.output_tokens").Int())
							assert.False(t, result.Get("choices").Exists())
						}
						assert.Equal(t, int64(10), result.Get("usage.total_tokens").Int())
					})
				}
			}
		})
	}
}

// TestGroupOpenAIProtocolBridgeHTTPStream 验证 Responses 客户端经组级 Chat 上游收到完整文本、工具、事件类型、单一终态和用量。
func TestGroupOpenAIProtocolBridgeHTTPStream(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	server, requests := newGroupBridgeHTTPFixture(t, dto.ChannelSettings{OpenAIProtocolBridge: dto.OpenAIProtocolBridgeResponses}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"group-bridge-http-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello "}}]}`,
			`{"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"group-bridge-http-model","choices":[{"index":0,"delta":{"content":"bridge","tool_calls":[{"index":0,"id":"call_weather","type":"function","function":{"name":"get_weather","arguments":"{\"city\":"}}]}}]}`,
			`{"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"group-bridge-http-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`{"id":"chatcmpl_stream","object":"chat.completion.chunk","created":1,"model":"group-bridge-http-model","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
			`[DONE]`,
		} {
			_, err := io.WriteString(w, "data: "+chunk+"\n\n")
			if !assert.NoError(t, err) {
				return
			}
			w.(http.Flusher).Flush()
		}
	})
	require.NoError(t, config.UpdateConfigFromMap(model_setting.GetGlobalSettings(), map[string]string{"group_openai_protocol_bridge": `{"openclaw":"chat"}`}))
	requestBody := `{"model":"group-bridge-http-model","input":"Tell me the weather","stream":true,"max_output_tokens":16,"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(requestBody))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Group", "openclaw")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	assert.Contains(t, response.Header.Get("Content-Type"), "text/event-stream")
	require.Len(t, requests, 1)
	upstream := <-requests
	assert.Equal(t, "/v1/chat/completions", upstream.path)
	assert.True(t, gjson.GetBytes(upstream.body, "stream").Bool())
	assert.Equal(t, "get_weather", gjson.GetBytes(upstream.body, "tools.0.function.name").String())
	assert.False(t, gjson.GetBytes(upstream.body, "input").Exists())
	assert.NotContains(t, string(body), "data: [DONE]", "Responses 客户端不能收到 Chat 结束标记")
	var text, arguments strings.Builder
	var kinds []string
	var terminal gjson.Result
	completed := 0
	for frame := range strings.SplitSeq(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(frame) == "" || strings.HasPrefix(frame, ":") {
			continue
		}
		lines := strings.Split(frame, "\n")
		require.Len(t, lines, 2, frame)
		require.True(t, strings.HasPrefix(lines[0], "event: "), frame)
		require.True(t, strings.HasPrefix(lines[1], "data: "), frame)
		data := strings.TrimPrefix(lines[1], "data: ")
		require.True(t, gjson.Valid(data), frame)
		event := gjson.Parse(data)
		kind := event.Get("type").String()
		assert.Equal(t, strings.TrimPrefix(lines[0], "event: "), kind)
		kinds = append(kinds, kind)
		switch kind {
		case "response.output_text.delta":
			text.WriteString(event.Get("delta").String())
		case "response.function_call_arguments.delta":
			arguments.WriteString(event.Get("delta").String())
		case "response.function_call_arguments.done":
			assert.JSONEq(t, `{"city":"Paris"}`, event.Get("arguments").String())
		case "response.completed":
			completed++
			terminal = event.Get("response")
		}
	}
	require.NotEmpty(t, kinds)
	assert.Equal(t, "response.created", kinds[0])
	assert.Equal(t, "response.completed", kinds[len(kinds)-1])
	assert.Contains(t, kinds, "response.output_text.done")
	assert.Contains(t, kinds, "response.function_call_arguments.done")
	assert.Equal(t, "Hello bridge", text.String())
	assert.JSONEq(t, `{"city":"Paris"}`, arguments.String())
	assert.Equal(t, 1, completed)
	assert.NotContains(t, kinds, "response.failed")
	assert.NotContains(t, kinds, "response.incomplete")
	assert.Len(t, terminal.Get("output").Array(), 2)
	assert.Equal(t, "response", terminal.Get("object").String())
	assert.Equal(t, "completed", terminal.Get("status").String())
	assert.Equal(t, "Hello bridge", terminal.Get(`output.#(type=="message").content.0.text`).String())
	tool := terminal.Get(`output.#(type=="function_call")`)
	assert.Equal(t, "call_weather", tool.Get("call_id").String())
	assert.Equal(t, "get_weather", tool.Get("name").String())
	assert.JSONEq(t, `{"city":"Paris"}`, tool.Get("arguments").String())
	assert.Equal(t, int64(7), terminal.Get("usage.input_tokens").Int())
	assert.Equal(t, int64(3), terminal.Get("usage.output_tokens").Int())
	assert.Equal(t, int64(10), terminal.Get("usage.total_tokens").Int())
}

// TestGroupOpenAIProtocolBridgeHTTPPassthrough 验证两种透传开关只拒绝需要转换的请求；可重试 400 策略也不能绕过本地冲突。
func TestGroupOpenAIProtocolBridgeHTTPPassthrough(t *testing.T) {
	for _, source := range []string{"global", "channel"} {
		for _, endpoint := range []struct {
			name, path, body, errorCode string
			conversion, native          dto.OpenAIProtocolBridge
		}{
			{"chat", "/v1/chat/completions", `{"model":"group-bridge-http-model","messages":[{"role":"user","content":"Say hello"}],"metadata":{"fixture":"keep"}}`, "invalid_request", dto.OpenAIProtocolBridgeResponses, dto.OpenAIProtocolBridgeChat},
			{"responses", "/v1/responses", `{"model":"group-bridge-http-model","input":"Say hello","metadata":{"fixture":"keep"}}`, "convert_request_failed", dto.OpenAIProtocolBridgeChat, dto.OpenAIProtocolBridgeResponses},
		} {
			for _, convert := range []bool{true, false} {
				mode, channelMode, outcome := endpoint.native, endpoint.conversion, "native"
				if convert {
					mode, channelMode, outcome = endpoint.conversion, endpoint.native, "conflict"
				}
				t.Run(source+"/"+endpoint.name+"/"+outcome, func(t *testing.T) {
					server, requests := newGroupBridgeHTTPFixture(t, dto.ChannelSettings{OpenAIProtocolBridge: channelMode, PassThroughBodyEnabled: source == "channel"}, nil)
					settings := model_setting.GetGlobalSettings()
					settings.PassThroughRequestEnabled = source == "global"
					encoded, err := common.Marshal(map[string]dto.OpenAIProtocolBridge{"openclaw": mode})
					require.NoError(t, err)
					require.NoError(t, config.UpdateConfigFromMap(settings, map[string]string{"group_openai_protocol_bridge": string(encoded)}))
					request, err := http.NewRequest(http.MethodPost, server.URL+endpoint.path, strings.NewReader(endpoint.body))
					require.NoError(t, err)
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("X-Test-Group", "openclaw")
					response, err := server.Client().Do(request)
					require.NoError(t, err)
					body, err := io.ReadAll(response.Body)
					require.NoError(t, response.Body.Close())
					require.NoError(t, err)
					assert.Contains(t, response.Header.Get("Content-Type"), "application/json")
					if convert {
						assert.Equal(t, http.StatusBadRequest, response.StatusCode, string(body))
						assert.Equal(t, endpoint.errorCode, gjson.GetBytes(body, "error.code").String())
						assert.Contains(t, gjson.GetBytes(body, "error.message").String(), "request body pass-through")
						assert.Empty(t, requests, "冲突不能访问主渠道，也不能重试到可用的备用渠道")
						return
					}
					require.Equal(t, http.StatusOK, response.StatusCode, string(body))
					require.Len(t, requests, 1)
					upstream := <-requests
					assert.Equal(t, endpoint.path, upstream.path)
					assert.Equal(t, "Bearer test-only-primary-key", upstream.authorization)
					assert.Equal(t, endpoint.body, string(upstream.body), "同协议透传必须保留原始请求体")
				})
			}
		}
	}
	t.Run("retry-witness", func(t *testing.T) {
		server, requests := newGroupBridgeHTTPFixture(t, dto.ChannelSettings{}, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") == "Bearer test-only-primary-key" {
				w.WriteHeader(http.StatusBadRequest)
				_, err := io.WriteString(w, `{"error":{"message":"retry witness","type":"invalid_request_error","code":"fixture_retry"}}`)
				assert.NoError(t, err)
				return
			}
			_, err := io.WriteString(w, groupBridgeHTTPChatResponse)
			assert.NoError(t, err)
		})
		request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"group-bridge-http-model","messages":[{"role":"user","content":"Say hello"}]}`))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Group", "openclaw")
		response, err := server.Client().Do(request)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode, string(body))
		require.Len(t, requests, 2, "先证明普通上游 400 确实能够重试，避免零请求断言掩盖失效的备用路由")
		assert.Equal(t, "Bearer test-only-primary-key", (<-requests).authorization)
		assert.Equal(t, "Bearer test-only-fallback-key", (<-requests).authorization)
	})
}
