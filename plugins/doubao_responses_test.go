package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoubaoResponsesProtocol 验证豆包视频的 Responses 解码和输出契约。
func TestDoubaoResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "doubao",
		model:     "doubao-seedance-2-0-260128",
		requestBody: map[string]any{
			"model": "doubao-seedance-2-0-260128",
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "a running fox"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/frame.png"},
			}}},
			"seconds": 6,
			"size":    "1920x1080",
		},
		wantAction: "image_to_video",
		wantRequest: map[string]any{
			"model":   "doubao-seedance-2-0-260128",
			"prompt":  "a running fox",
			"images":  []any{"https://cdn.example/frame.png"},
			"seconds": float64(6),
			"metadata": map[string]any{
				"resolution": "1080p",
			},
		},
		wantUsageKeys:  []string{"resolution", "tokens", "video_input"},
		wantVendorName: "doubao",
	})
}

// TestDoubaoAutomaticDurationHostContract 通过真实宿主链路验证自动时长预扣、上游还原和完成结算。
func TestDoubaoAutomaticDurationHostContract(t *testing.T) {
	source, err := builtinplugins.Source("doubao")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "doubao"})
	require.NoError(t, err)

	const (
		clientModel   = "seedance-edit-alias"
		upstreamModel = "doubao-seedance-2-5-260628"
	)
	video := map[string]any{
		"type":      "video_url",
		"video_url": map[string]any{"url": "https://cdn.example/reference.mp4"},
	}
	protocolRequest := jsplugin.ProtocolRequestContext{
		Protocol:      "openai_video",
		Operation:     "create",
		Model:         clientModel,
		UpstreamModel: upstreamModel,
		RouteRequestContext: jsplugin.RouteRequestContext{
			Method: http.MethodPost,
			Path:   "/v1/videos",
			Body: map[string]any{
				"kind": "json",
				"value": map[string]any{
					"model":    clientModel,
					"prompt":   "extend the camera movement",
					"duration": -1,
					"metadata": map[string]any{
						"resolution": "720p",
						"content":    []any{video},
					},
				},
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: clientModel,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://ark.example",
			ApiKey:            "fixture-only",
			UpstreamModelName: upstreamModel,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_doubao_auto"},
	}
	adaptor := taskplugin.New(plugin)
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Plugin: plugin, Protocol: "openai_video", Model: clientModel, MappedModel: upstreamModel})
	c.Set(jsplugin.ContextKeyProtocolRequest, protocolRequest)

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	requestValue, exists := c.Get("task_request")
	require.True(t, exists)
	request, ok := requestValue.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, request["auto_duration"])
	assert.NotContains(t, request, "seconds")
	assert.NotContains(t, request, "duration")
	metadata, ok := request["metadata"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, metadata, "duration")

	facts, err := adaptor.ExtractUsageFactsValidated(c, info)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"tokens": float64(648000), "resolution": "720p", "video_input": "video"}, facts)
	ratios, err := adaptor.EstimateBillingValidated(c, info)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"video_input_ratio": 0.6}, ratios)

	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	var upstreamBody map[string]any
	require.NoError(t, common.DecodeJson(reader, &upstreamBody))
	assert.Equal(t, upstreamModel, upstreamBody["model"])
	assert.Equal(t, float64(-1), upstreamBody["duration"])
	assert.NotContains(t, upstreamBody, "auto_duration")
	content, ok := upstreamBody["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	assert.Equal(t, video, content[0])
	assert.Equal(t, map[string]any{"type": "text", "text": "extend the camera movement"}, content[1])

	completion, err := common.Marshal(map[string]any{
		"status":  "succeeded",
		"content": map[string]any{"video_url": "https://cdn.example/result.mp4", "resolution": "720p"},
		"usage":   map[string]any{"completion_tokens": 12345},
	})
	require.NoError(t, err)
	task := &model.Task{
		TaskID:     "task_doubao_auto",
		Properties: model.Properties{OriginModelName: clientModel, UpstreamModelName: upstreamModel},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "vendor-task",
		},
	}
	result, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, completion)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", result.Status)
	assert.Equal(t, map[string]any{"tokens": float64(12345), "resolution": "720p"}, result.UsageFacts)
	assert.NotEqual(t, facts["tokens"], result.UsageFacts["tokens"])
}

// TestDoubaoDurationNormalization 验证三个入口共享的时长规范化、冲突拒绝和模型边界。
func TestDoubaoDurationNormalization(t *testing.T) {
	source, err := builtinplugins.Source("doubao")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "doubao"})
	require.NoError(t, err)

	decodeJSON := func(t *testing.T, protocol, model, upstreamModel string, body map[string]any) map[string]any {
		t.Helper()
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
			"model": model, "upstreamModel": upstreamModel,
			"body": map[string]any{"kind": "json", "value": body},
		})
		require.NoError(t, callErr)
		command, ok := value.(map[string]any)
		require.True(t, ok)
		request, ok := command["requestBody"].(map[string]any)
		require.True(t, ok)
		return request
	}
	decodeError := func(t *testing.T, protocol string, body map[string]any) error {
		t.Helper()
		_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
			"model": "doubao-seedance-2-5-260628",
			"body":  map[string]any{"kind": "json", "value": body},
		})
		return callErr
	}

	t.Run("等值字段规范为 seconds", func(t *testing.T) {
		for _, protocol := range []string{"openai_video", "openai_responses"} {
			body := map[string]any{
				"model": "doubao-seedance-2-5-260628", "seconds": 6, "duration": "6",
				"metadata": map[string]any{"duration": 6, "content": []any{}},
			}
			if protocol == "openai_video" {
				body["prompt"] = "ocean"
			} else {
				body["input"] = "ocean"
			}
			request := decodeJSON(t, protocol, "doubao-seedance-2-5-260628", "doubao-seedance-2-5-260628", body)
			assert.EqualValues(t, 6, request["seconds"])
			assert.NotContains(t, request, "duration")
			metadata := request["metadata"].(map[string]any)
			assert.NotContains(t, metadata, "duration")
		}
	})

	t.Run("冲突和非法时长被拒绝", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]any
		}{
			{"冲突", map[string]any{"seconds": 6, "duration": 7}},
			{"零值", map[string]any{"seconds": 0}},
			{"其他负数", map[string]any{"duration": -2}},
			{"小数", map[string]any{"metadata": map[string]any{"duration": 1.5}}},
			{"超过上限", map[string]any{"duration": 3601}},
			{"错误类型", map[string]any{"duration": true}},
			{"客户端顶层标记", map[string]any{"auto_duration": true}},
			{"客户端 metadata 标记", map[string]any{"metadata": map[string]any{"auto_duration": true}}},
		}
		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				test.body["model"] = "doubao-seedance-2-5-260628"
				test.body["prompt"] = "ocean"
				require.Error(t, decodeError(t, "openai_video", test.body))
			})
		}
	})

	t.Run("multipart 和 Responses 保留内部标记", func(t *testing.T) {
		multipartValue, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "doubao-seedance-2-5-260628",
			"body": map[string]any{
				"kind": "multipart",
				"fields": map[string]any{
					"model":    []any{"doubao-seedance-2-5-260628"},
					"prompt":   []any{"ocean"},
					"duration": []any{"-1"},
					"metadata": []any{`{"resolution":"720p"}`},
				},
				"files": []any{},
			},
		})
		require.NoError(t, callErr)
		multipartCommand := multipartValue.(map[string]any)
		multipartRequest := multipartCommand["requestBody"].(map[string]any)
		assert.Equal(t, true, multipartRequest["auto_duration"])
		assert.NotContains(t, multipartRequest, "duration")

		responsesRequest := decodeJSON(t, "openai_responses", "doubao-seedance-2-5-260628", "doubao-seedance-2-5-260628", map[string]any{
			"model": "doubao-seedance-2-5-260628", "input": "ocean", "duration": -1,
		})
		assert.Equal(t, true, responsesRequest["auto_duration"])
		assert.NotContains(t, responsesRequest, "duration")

		nativeValue, callErr := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{
			"body": map[string]any{"kind": "json", "value": map[string]any{
				"model": "doubao-seedance-2-5-260628", "duration": -1,
				"content": []any{map[string]any{"type": "text", "text": "ocean"}},
			}},
		})
		require.NoError(t, callErr)
		nativeRequest := nativeValue.(map[string]any)["requestBody"].(map[string]any)
		assert.Equal(t, true, nativeRequest["auto_duration"])
		assert.NotContains(t, nativeRequest["metadata"].(map[string]any), "duration")
	})

	t.Run("2.0 模型预留十五秒", func(t *testing.T) {
		for _, upstreamModel := range []string{
			"doubao-seedance-2-0-260128",
			"doubao-seedance-2-0-fast-260128",
			"doubao-seedance-2-0-mini-260615",
		} {
			request := decodeJSON(t, "openai_video", "channel-alias", upstreamModel, map[string]any{
				"model": "channel-alias", "prompt": "ocean", "duration": -1, "metadata": map[string]any{"resolution": "720p"},
			})
			factsValue, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
				"model": "channel-alias", "upstreamModel": upstreamModel, "usagePurpose": "facts", "requestBody": request,
			})
			require.NoError(t, callErr)
			facts := factsValue.(map[string]any)
			assert.EqualValues(t, 324000, facts["tokens"])
		}
	})

	t.Run("2.5 缺省时长预留三十秒", func(t *testing.T) {
		request := decodeJSON(t, "openai_video", "channel-alias", "doubao-seedance-2-5-260628", map[string]any{
			"model": "channel-alias", "prompt": "ocean", "metadata": map[string]any{"resolution": "720p"},
		})
		factsValue, callErr := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
			"model": "channel-alias", "upstreamModel": "doubao-seedance-2-5-260628", "usagePurpose": "facts", "requestBody": request,
		})
		require.NoError(t, callErr)
		facts := factsValue.(map[string]any)
		assert.EqualValues(t, 648000, facts["tokens"])
	})

	t.Run("不支持自动时长的上游模型被拒绝", func(t *testing.T) {
		request := decodeJSON(t, "openai_video", "channel-alias", "doubao-seedance-1-5-pro-251215", map[string]any{
			"model": "channel-alias", "prompt": "ocean", "duration": -1,
		})
		_, callErr := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "channel-alias", "upstreamModel": "doubao-seedance-1-5-pro-251215", "baseUrl": "https://ark.example", "requestBody": request,
		})
		require.ErrorContains(t, callErr, "not supported")
	})

	t.Run("重复解码不修改原请求", func(t *testing.T) {
		body := map[string]any{
			"model": "channel-alias", "prompt": "ocean", "duration": -1,
			"metadata": map[string]any{"duration": -1, "content": []any{}},
		}
		first := decodeJSON(t, "openai_video", "channel-alias", "doubao-seedance-2-5-260628", body)
		second := decodeJSON(t, "openai_video", "channel-alias", "doubao-seedance-2-5-260628", body)
		assert.Equal(t, first, second)
		assert.Equal(t, -1, body["duration"])
		assert.Equal(t, -1, body["metadata"].(map[string]any)["duration"])
		assert.NotContains(t, body, "auto_duration")
	})
}
