package plugins_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSoraResponsesProtocol 验证 Sora 的 Responses 解码、输出和用量声明契约。
func TestSoraResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "sora",
		model:     "sora-2-pro",
		requestBody: map[string]any{
			"model":   "sora-2-pro",
			"input":   "waves at sunset",
			"seconds": 8,
			"size":    "1792x1024",
		},
		wantAction: "text_to_video",
		wantRequest: map[string]any{
			"model":   "sora-2-pro",
			"prompt":  "waves at sunset",
			"seconds": float64(8),
			"size":    "1792x1024",
		},
		wantUsageKeys:  []string{"seconds", "size"},
		wantVendorName: "sora",
	})
}

// TestSoraSizeHostContracts 通过真实 Go 宿主和本地模拟上游验证尺寸转发、预扣用量及完成用量，不调用付费接口。
func TestSoraSizeHostContracts(t *testing.T) {
	source, err := builtinplugins.Source("sora")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "sora"})
	require.NoError(t, err)
	// 原尺寸及顺序属于既有配置契约，新横竖尺寸只能追加。
	sizes := []string{"720x1280", "1280x720", "1792x1024", "1024x1792", "1920x1080", "1080x1920"}
	assert.Equal(t, sizes, plugin.Meta.UsageSchema["size"].Enum)
	service.InitHttpClient()

	for _, size := range sizes {
		t.Run(size, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v1/videos", r.URL.Path)
				var body map[string]any
				if !assert.NoError(t, common.DecodeJson(r.Body, &body)) {
					http.Error(w, "invalid JSON", http.StatusBadRequest)
					return
				}
				assert.Equal(t, map[string]any{"model": "sora-2-pro", "prompt": "waves at sunset", "seconds": float64(8), "size": size}, body)
				w.Header().Set("Content-Type", "application/json")
				_, writeErr := io.WriteString(w, `{"id":"sora-fixture","status":"queued"}`)
				assert.NoError(t, writeErr)
			}))
			defer server.Close()

			info := &relaycommon.RelayInfo{
				ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "fixture-only", UpstreamModelName: "sora-2-pro"},
				OriginModelName: "sora-2-pro",
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_sora_fixture"},
			}
			adaptor := taskplugin.New(plugin)
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Plugin: plugin, Protocol: "openai_responses", Model: "sora-2-pro"})
			c.Set(jsplugin.ContextKeyProtocolRequest, jsplugin.ProtocolRequestContext{
				Model: "sora-2-pro", Protocol: "openai_responses",
				RouteRequestContext: jsplugin.RouteRequestContext{Body: map[string]any{
					"kind": "json", "value": map[string]any{"model": "sora-2-pro", "input": "waves at sunset", "seconds": 8, "size": size},
				}},
			})
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			facts, err := adaptor.ExtractUsageFactsValidated(c, info)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"seconds": float64(8), "size": size}, facts)
			ratios, err := adaptor.EstimateBillingValidated(c, info)
			require.NoError(t, err)
			assert.Equal(t, map[string]float64{"seconds": 8}, ratios, "旧倍率计价仍仅乘秒数")

			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			resp, err := adaptor.DoRequest(c, info, body)
			require.NoError(t, err)
			defer resp.Body.Close()
			parsed, taskErr := adaptor.ParseResponse(c, resp, info)
			require.Nil(t, taskErr)
			require.NotNil(t, parsed)
			assert.Equal(t, "sora-fixture", parsed.UpstreamTaskID)

			// 完成时使用不同于提交的秒数，证明读的是完成响应而非请求副本。
			completion, err := common.Marshal(map[string]any{"status": "completed", "seconds": 12, "size": size})
			require.NoError(t, err)
			result, err := adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, completion)
			require.NoError(t, err)
			assert.Equal(t, "SUCCESS", result.Status)
			assert.Equal(t, map[string]any{"seconds": float64(12), "size": size}, result.UsageFacts)
		})
	}
}
