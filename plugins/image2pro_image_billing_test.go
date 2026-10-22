package plugins_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the actual precharge and completion evaluators: schema compatibility
// alone cannot detect the missing seconds fact used by legacy image expressions.
func TestImage2ProImageLegacyExpressionBilling(t *testing.T) {
	plugin := image2ProDynamicPlugin(t)
	const expression = `tier("base", u("count") * 3.5 + u("seconds") * 0)`
	_, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{
		Usage: map[string]any{"count": float64(2)},
	})
	require.Error(t, err, "the regression fixture must require an explicit seconds fact")
	for _, tc := range []struct {
		client, upstream string
		n                int
	}{
		{"完全无限制-Z-Image", "完全无限制-Z-Image", 2},
		{"Nano-banana-Pro", "Nano-banana-Pro", 1},
		{"future-image-v7", "future-image-v7", 4},
		{"Image2Pro-Image-future-image-v7", "future-image-v7", 3},
	} {
		t.Run(tc.client, func(t *testing.T) {
			intent := image2ProDynamicDecode(t, plugin, "openai_image_tasks", tc.client, tc.upstream, map[string]any{
				"model": tc.client, "prompt": "A lighthouse at dusk", "n": tc.n,
			})
			info := &relaycommon.RelayInfo{
				OriginModelName: tc.client,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: "https://synthetic.example/v1", ApiKey: "synthetic-fixture", UpstreamModelName: tc.upstream,
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_image_legacy_billing_fixture"},
			}
			adaptor := taskplugin.New(plugin)
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/tasks", nil)
			c.Set("task_request", intent["requestBody"])
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			facts, err := adaptor.ExtractUsageFactsValidated(c, info)
			require.NoError(t, err)
			expected := map[string]any{"count": float64(tc.n), "seconds": float64(0)}
			assert.Equal(t, expected, facts)
			ratios, err := adaptor.EstimateBillingValidated(c, info)
			require.NoError(t, err)
			assert.Empty(t, ratios, "usage facts must not also multiply legacy billing ratios")
			cost, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
			require.NoError(t, err)
			assert.InDelta(t, float64(tc.n)*3.5, cost, 1e-10)
			for _, status := range []string{"queued", "completed"} {
				t.Run(status, func(t *testing.T) {
					response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
						`{"id":"image-billing-fixture:71","status":"` + status + `","n":99,"data":[{"url":"https://cdn.example/result.png"}]}`,
					))}
					parsed, taskErr := adaptor.ParseResponse(c, response, info)
					require.Nil(t, taskErr)
					require.NoError(t, response.Body.Close())
					assert.JSONEq(t, fmt.Sprintf(`{"kind":"image","n":%d}`, tc.n), string(parsed.PluginState))
					if status == "completed" {
						require.NotNil(t, parsed.Immediate)
						assert.Equal(t, expected, parsed.Immediate.UsageFacts)
					}
					task := &model.Task{
						Properties:  model.Properties{OriginModelName: tc.client, UpstreamModelName: tc.upstream},
						PrivateData: model.TaskPrivateData{PluginState: parsed.PluginState},
					}
					completed, err := adaptor.ParseTaskResult(task, response, []byte(
						`{"status":"completed","n":99,"data":[{"url":"https://cdn.example/result.png"}]}`,
					))
					require.NoError(t, err)
					assert.Equal(t, "SUCCESS", completed.Status)
					assert.Equal(t, expected, completed.UsageFacts, "settlement must use the frozen request count")
					for _, snapshotFacts := range []map[string]any{facts, {"count": float64(tc.n)}} {
						snapshotBefore := image2ProDynamicObject(t, snapshotFacts)
						snapshot := &billingexpr.BillingSnapshot{
							ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 2,
							QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: snapshotFacts,
						}
						settled, merged, err := service.EvaluateTaskCompletionUsage(snapshot, completed.UsageFacts)
						require.NoError(t, err)
						assert.Equal(t, expected, merged, "completion supplies seconds for older image snapshots too")
						assert.Equal(t, common.QuotaRound(cost*1000000), settled.ActualQuotaAfterGroup)
						assert.Nil(t, settled.Clamp)
						assert.Equal(t, snapshotBefore, snapshot.UsageFacts, "settlement must preserve the frozen snapshot")
					}
					failed, err := adaptor.ParseTaskResult(task, response, []byte(`{"status":"failed"}`))
					require.NoError(t, err)
					assert.Empty(t, failed.UsageFacts)
				})
			}
		})
	}
}
