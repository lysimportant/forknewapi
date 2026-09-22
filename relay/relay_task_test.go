package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTaskSubmissionAcceptanceAndSingleAttempt 验证异步受理与发送后失败的重试边界，不访问真实供应商。
func TestTaskSubmissionAcceptanceAndSingleAttempt(t *testing.T) {
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = savedFreePreConsume
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"declared-model":0}`))
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	service.InitHttpClient()
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		noRetry     bool
		wantNoRetry bool
		wantError   bool
		disconnect  bool
	}{
		{name: "异步202保留任务编号", status: 202, body: `{"id":"accepted-video"}`, noRetry: true},
		{name: "已有200响应仍可用", status: 200, body: `{"id":"accepted-video"}`},
		{name: "服务端失败禁止重复提交", status: 503, body: `{"error":"submission unknown"}`, noRetry: true, wantNoRetry: true, wantError: true},
		{name: "受理后缺少编号禁止重复提交", status: 202, body: `{}`, wantNoRetry: true, wantError: true},
		{name: "受理后JSON损坏禁止重复提交", status: 202, body: `{`, wantNoRetry: true, wantError: true},
		{name: "响应断开禁止重复提交", noRetry: true, wantNoRetry: true, wantError: true, disconnect: true},
		{name: "既有插件保留重试行为", status: 503, body: `{"error":"unavailable"}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/submit", r.URL.Path)
				assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))
				if tc.disconnect {
					conn, _, err := w.(http.Hijacker).Hijack()
					require.NoError(t, err)
					require.NoError(t, conn.Close())
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, err := io.WriteString(w, tc.body)
				require.NoError(t, err)
			}))
			defer upstream.Close()
			source := `
export const meta = {apiVersion:1,key:"single-submit",name:"Single submit",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task",requiredCapabilities:["task-submit-no-retry@1"]};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/submit",method:"POST",headers:{"Idempotency-Key":ctx.publicTaskId},body:{model:ctx.upstreamModel},noRetry:NO_RETRY};}
export function parseSubmitResponse(ctx,resp){if(!resp.body.id)throw new Error("missing task id");return {taskId:resp.body.id,taskData:resp.body};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query"};}
export function parseTaskResult(){return {status:"QUEUED"};}
`
			noRetryValue := "false"
			if tc.noRetry {
				noRetryValue = "true"
			}
			source = strings.Replace(source, "NO_RETRY", noRetryValue, 1)
			c, info := newTaskSubmitContext(t, "declared-model", "")
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			pinMappingOrderPlugin(t, c, source)
			info.OriginModelName = "declared-model"
			info.UserGroup, info.UsingGroup = "default", "default"
			result, taskErr := RelayTaskSubmit(c, info)
			assert.Equal(t, 1, requests)
			if tc.wantError {
				require.NotNil(t, taskErr)
				assert.Equal(t, tc.wantNoRetry, taskErr.NoRetry)
				assert.Nil(t, result)
				return
			}
			require.Nil(t, taskErr, "%+v", taskErr)
			require.NotNil(t, result)
			assert.Equal(t, "accepted-video", result.UpstreamTaskID)
		})
	}
}

// TestTaskSubmitNoRetryRequiresCapability 阻止未声明宿主能力的插件静默依赖重试保护。
func TestTaskSubmitNoRetryRequiresCapability(t *testing.T) {
	source := strings.Replace(mappingOrderSubmitPlugin, `action:"text_to_video"`, `noRetry:true,action:"text_to_video"`, 1)
	c, info := newTaskSubmitContext(t, "declared-model", "")
	pinMappingOrderPlugin(t, c, source)
	info.OriginModelName = "declared-model"
	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "noRetry requires task-submit-no-retry@1")
}

func TestTaskModel2DtoNormalizesLegacyAction(t *testing.T) {
	task := &model.Task{Action: "firstTailGenerate"}

	dtoTask := TaskModel2Dto(task)

	assert.Equal(t, constant.TaskActionFirstTailToVideo, dtoTask.Action)
	assert.Equal(t, "firstTailGenerate", task.Action)
}

const mappingOrderSubmitPlugin = `
export const meta = {apiVersion:1,key:"maporder",name:"Map Order",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

const mappingOrderRewritePlugin = `
export const meta = {apiVersion:1,key:"maporder-rw",name:"Map Order RW",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel}, rewriteModel:"rewritten"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func pinMappingOrderPlugin(t *testing.T, c *gin.Context, source string) {
	t.Helper()
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
}

func newTaskSubmitContext(t *testing.T, originalModel, mapping string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, originalModel)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.example")
	if mapping != "" {
		c.Set("model_mapping", mapping)
	}
	c.Set("task_request", map[string]any{"prompt": "p"})
	return c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
}

func TestRelayTaskSubmitMapsBeforeValidateWhenOriginSet(t *testing.T) {
	const mapping = `{"alias-model":"mid-model","mid-model":"declared-model"}`

	c, info := newTaskSubmitContext(t, "alias-model", mapping)
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "alias-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDeclaredNameWithoutMappingIsUnchanged(t *testing.T) {
	c, info := newTaskSubmitContext(t, "declared-model", "")
	pinMappingOrderPlugin(t, c, mappingOrderSubmitPlugin)
	info.OriginModelName = "declared-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "declared-model", info.OriginModelName)
	assert.Equal(t, "declared-model", info.UpstreamModelName)
	assert.False(t, info.IsModelMapped)
}

func TestRelayTaskSubmitDoesNotApplyMappingTwice(t *testing.T) {
	c, info := newTaskSubmitContext(t, "alias-model", `{"alias-model":"declared-model"}`)
	pinMappingOrderPlugin(t, c, mappingOrderRewritePlugin)
	info.OriginModelName = "alias-model"

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, "rewritten", info.UpstreamModelName, "late mapping would overwrite rewriteModel with the chain tail")
	assert.Equal(t, "alias-model", info.OriginModelName)
}

func TestRelayTaskSubmitEmptyOriginKeepsLateMapping(t *testing.T) {
	plugin, err := pluginruntime.NewRegistry().Register(mappingOrderSubmitPlugin, pluginruntime.Options{})
	require.NoError(t, err)
	synthesized := service.CoverTaskActionToModelName(constant.TaskPlatform(plugin.Meta.Key), "text_to_video")
	c, info := newTaskSubmitContext(t, "pre-validate-upstream",
		`{"pre-validate-upstream":"should-not-apply-early","`+synthesized+`":"legacy-tail"}`)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	info.OriginModelName = ""

	_, taskErr := RelayTaskSubmit(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
	assert.Equal(t, synthesized, info.OriginModelName)
	assert.Equal(t, "legacy-tail", info.UpstreamModelName)
	assert.True(t, info.IsModelMapped)
}

const billingFallbackPlugin = `
export const meta = {apiVersion:1,key:"bill-fallback",name:"Bill Fallback",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) {
  return {url: ctx.baseUrl+"/submit", method:"POST", body:{upstreamModel: ctx.upstreamModel, model: ctx.model}, action:"text_to_video"};
}
export function parseSubmitResponse(){return {taskId:"1"};}
export function buildQueryRequest(){return {url:"https://provider.example"};}
export function parseTaskResult(){return {status:"SUCCESS"};}
`

func saveBillingConfig(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
}

func TestRelayTaskSubmitAliasBillingIdentityAndExprFallback(t *testing.T) {
	const mapping = `{"alias-model":"declared-model"}`
	const aliasExpr = `tier("alias", 2)`
	const tailExpr = `tier("tail", 3)`
	const legacyExpr = `tier("legacy", u("old_units") * 2)`

	tests := []struct {
		name       string
		modes      map[string]string
		exprs      map[string]string
		wantTiered bool
		wantExpr   string
		source     string
	}{
		{
			name:       "alias own tiered wins",
			modes:      map[string]string{"alias-model": "tiered_expr", "declared-model": "tiered_expr"},
			exprs:      map[string]string{"alias-model": aliasExpr, "declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   aliasExpr,
		},
		{
			name:       "fallback uses tail expr",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": tailExpr},
			wantTiered: true,
			wantExpr:   tailExpr,
		},
		{
			name:       "stored expression keeps running after schema narrows",
			modes:      map[string]string{"declared-model": "tiered_expr"},
			exprs:      map[string]string{"declared-model": legacyExpr},
			wantTiered: true,
			wantExpr:   legacyExpr,
			source: strings.Replace(billingFallbackPlugin, `fetchMode:"per_task"`, `fetchMode:"per_task", usageSchema:{old_units:{type:"number",unit:"count"}}, usageProfiles:[{models:["declared-model"],schema:{seconds:{type:"number",unit:"second"}}}]`, 1) + `
export function extractUsage(){return {old_units:2};}
`,
		},
		{
			name:       "neither tiered uses ordinary pricing",
			wantTiered: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			saveBillingConfig(t)
			if len(testCase.modes) > 0 {
				modeJSON, marshalErr := common.Marshal(testCase.modes)
				require.NoError(t, marshalErr)
				exprJSON, marshalErr := common.Marshal(testCase.exprs)
				require.NoError(t, marshalErr)
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
					"billing_setting.billing_mode": string(modeJSON),
					"billing_setting.billing_expr": string(exprJSON),
				}))
				if testCase.wantExpr == aliasExpr {
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("alias-model"))
				} else {
					require.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("alias-model"))
					require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("declared-model"))
				}
			}

			c, info := newTaskSubmitContext(t, "alias-model", mapping)
			c.Set("group", "default")
			info.UserGroup = "default"
			info.UsingGroup = "default"
			source := testCase.source
			if source == "" {
				source = billingFallbackPlugin
			}
			pinMappingOrderPlugin(t, c, source)
			info.OriginModelName = "alias-model"

			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, "alias-model", info.OriginModelName)
			assert.Equal(t, "declared-model", info.UpstreamModelName)
			assert.True(t, info.IsModelMapped)

			task := model.InitTask(constant.TaskPlatform("bill-fallback"), info)
			assert.Equal(t, "alias-model", task.Properties.OriginModelName)
			assert.Equal(t, "declared-model", task.Properties.UpstreamModelName)

			if testCase.wantTiered {
				require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
				assert.Equal(t, "alias-model", info.TieredBillingSnapshot.ModelName)
				assert.Equal(t, testCase.wantExpr, info.TieredBillingSnapshot.ExprString)
				assert.Equal(t, billingexpr.ExprHashString(testCase.wantExpr), info.TieredBillingSnapshot.ExprHash)
				assert.NotEqual(t, "model_price_error", taskErr.Code)
				if testCase.wantExpr == legacyExpr {
					assert.Equal(t, 4*common.QuotaPerUnit, info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup)
				}
			} else {
				assert.Nil(t, info.TieredBillingSnapshot)
				assert.Equal(t, "model_price_error", taskErr.Code)
			}
		})
	}
}

func TestSharedTaskBillingExpressionSelectionAndFrozenSettlement(t *testing.T) {
	const baseExpr = `tier("base", u("seconds") * 2)`
	const alphaExpr = `tier("alpha", u("seconds") * 3)`
	const betaExpr = `tier("beta", u("credits") * 5)`
	const aliasExpr = `tier("alias", u("credits") * 7)`
	for _, tc := range []struct {
		name, plugin, model, mapping, modelExpr, mode, wantExpr string
		variants                                                map[string]string
		wantPriceError                                          bool
	}{
		{name: "executing plugin override", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-alpha::declared-model": alphaExpr, "billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "override ignores model mode", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "ratio", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "model expression fallback", plugin: "billing-alpha", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantExpr: baseExpr},
		{name: "alias override precedes mapped override", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr, "billing-beta::alias-model": aliasExpr}, wantExpr: aliasExpr},
		{name: "mapped override precedes model fallback", plugin: "billing-beta", model: "alias-model", mapping: `{"alias-model":"declared-model"}`, modelExpr: baseExpr, mode: "tiered_expr", variants: map[string]string{"billing-beta::declared-model": betaExpr}, wantExpr: betaExpr},
		{name: "unconfigured plugin cannot use another schema", plugin: "billing-beta", model: "declared-model", modelExpr: baseExpr, mode: "tiered_expr", wantPriceError: true},
		{name: "missing usage in skipped branch remains incompatible", plugin: "billing-beta", model: "declared-model", modelExpr: `true ? tier("free", 0) : tier("missing", u("seconds"))`, mode: "tiered_expr", wantPriceError: true},
		{name: "fixed pricing is still rejected", plugin: "billing-beta", model: "declared-model", variants: map[string]string{"billing-beta::declared-model": `tier("fixed", fixed(1))`}, wantPriceError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveBillingConfig(t)
			registry := pluginruntime.NewRegistry()
			for _, spec := range []struct{ key, field, unit string }{{"billing-alpha", "seconds", "second"}, {"billing-beta", "credits", "credit"}} {
				source := strings.ReplaceAll(billingFallbackPlugin, "bill-fallback", spec.key)
				source = strings.Replace(source, `fetchMode:"per_task"`, `fetchMode:"per_task",usageSchema:{`+spec.field+`:{type:"number",unit:"`+spec.unit+`"}}`, 1)
				source += `export function extractUsage(){return {` + spec.field + `:2};}`
				_, err := registry.Register(source, pluginruntime.Options{})
				require.NoError(t, err)
			}
			variants := tc.variants
			if variants == nil {
				variants = map[string]string{}
			}
			rawVariants, err := common.Marshal(variants)
			require.NoError(t, err)
			modes, err := common.Marshal(map[string]string{"declared-model": tc.mode})
			require.NoError(t, err)
			expressions, err := common.Marshal(map[string]string{"declared-model": tc.modelExpr})
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				billing_setting.PluginBillingExprOption: string(rawVariants), "billing_setting.billing_mode": string(modes), "billing_setting.billing_expr": string(expressions),
			}))
			c, info := newTaskSubmitContext(t, tc.model, tc.mapping)
			c.Set("group", "default")
			c.Set("task_plugin_key", tc.plugin)
			info.UserGroup = "default"
			info.UsingGroup = "default"
			info.OriginModelName = tc.model
			plugin, ok := registry.Generation().Get(tc.plugin)
			require.True(t, ok)
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
			_, taskErr := RelayTaskSubmit(c, info)
			require.NotNil(t, taskErr) // This fixture stops at reservation, before upstream submission.
			if tc.wantPriceError {
				assert.Equal(t, "model_price_error", taskErr.Code)
				assert.Nil(t, info.TieredBillingSnapshot)
				return
			}
			require.NotNil(t, info.TieredBillingSnapshot, "submission error: %+v", taskErr)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
			assert.NotEqual(t, "model_price_error", taskErr.Code)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{billing_setting.PluginBillingExprOption: `{}`, "billing_setting.billing_expr": `{}`}))
			field := "seconds"
			if tc.plugin == "billing-beta" {
				field = "credits"
			}
			result, usage, err := service.EvaluateTaskCompletionUsage(info.TieredBillingSnapshot, map[string]any{field: float64(4)})
			require.NoError(t, err)
			assert.Equal(t, float64(4), usage[field])
			assert.Equal(t, float64(2), info.TieredBillingSnapshot.UsageFacts[field])
			assert.Equal(t, 2*info.TieredBillingSnapshot.EstimatedQuotaAfterGroup, result.ActualQuotaAfterGroup)
			assert.Equal(t, tc.wantExpr, info.TieredBillingSnapshot.ExprString)
		})
	}
}
