package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPricingFixedValidationSeparatesModelAndPluginExpressions(t *testing.T) {
	const modelName = "model-pricing-fixed-probe"
	const aliasName = "model-pricing-fixed-alias"
	const fixedExpression = `tier("base", fixed(3))`
	const taskExpression = `tier("images", u("count") * 0.2)`
	for _, testCase := range []struct {
		name             string
		providers        int
		alias            bool
		expression       string
		pluginExpression string
		wantError        string
	}{
		{name: "same_name_plugin_does_not_reject_request_price", providers: 1, expression: fixedExpression},
		{name: "shared_name_plugins_do_not_reject_request_price", providers: 2, expression: fixedExpression},
		{name: "mapped_alias_does_not_reject_request_price", providers: 1, alias: true, expression: fixedExpression},
		{name: "explicit_task_override_is_preserved", providers: 2, expression: fixedExpression, pluginExpression: taskExpression},
		{name: "task_usage_still_uses_declared_schema", providers: 1, expression: taskExpression},
		{name: "alias_task_usage_still_uses_declared_schema", providers: 1, alias: true, expression: taskExpression},
		{name: "negative_fixed_price_is_rejected", providers: 1, expression: `tier("base", fixed(-3))`, wantError: "non-negative"},
		{name: "nonliteral_fixed_price_is_rejected", providers: 1, expression: `tier("base", fixed(p))`, wantError: "numeric literal"},
		{name: "token_scaled_fixed_price_is_rejected", providers: 1, expression: `tier("base", fixed(3)) * p`, wantError: "fixed pricing requires"},
		{name: "undeclared_model_usage_is_rejected", providers: 1, expression: `tier("base", u("missing") * 3)`, wantError: "not declared"},
		{name: "undeclared_alias_usage_is_rejected", providers: 1, alias: true, expression: `tier("base", u("missing") * 3)`, wantError: "not declared"},
		{name: "fixed_plugin_override_is_rejected", providers: 1, expression: fixedExpression, pluginExpression: fixedExpression, wantError: "fixed pricing is not supported for task usage expressions"},
		{name: "undeclared_plugin_override_usage_is_rejected", providers: 1, expression: fixedExpression, pluginExpression: `u("missing")`, wantError: "not declared"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resetPricingEndpointTestTables(t)
			saved := config.GlobalConfig.ExportAllConfigs()
			t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode":          `{}`,
				"billing_setting.billing_expr":          `{}`,
				billing_setting.PluginBillingExprOption: `{}`,
			}))
			for provider := 0; provider < testCase.providers; provider++ {
				pluginKey := fmt.Sprintf("model-pricing-fixed-provider-%d", provider)
				source := fmt.Sprintf(`
export const meta = {
 apiVersion:1,key:%q,name:"Fixed Validation Probe",version:"1.0.0",author:{name:"Test"},
 models:[%q],fetchMode:"per_task",usageSchema:{count:{type:"number",unit:"count"}}
};
export function buildSubmitRequest(){return {};}
export function parseSubmitResponse(){return {};}
export function buildQueryRequest(){return {};}
export function parseTaskResult(){return {};}
`, pluginKey, modelName)
				_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
				require.NoError(t, err)
				t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister(pluginKey) })
			}

			name := modelName
			if testCase.alias {
				name = aliasName
				mapping := fmt.Sprintf(`{%q:%q}`, aliasName, modelName)
				require.NoError(t, DB.Create(&Channel{
					Id: 960, Type: constant.ChannelTypeTaskPlugin, Status: 1, Name: "fixed-validation-alias",
					Models: aliasName + "," + modelName, ModelMapping: &mapping,
				}).Error)
				insertPricingEndpointAbility(t, 960, aliasName)
				insertPricingEndpointAbility(t, 960, modelName)
				InitChannelCache()
				target, found := ResolveTaskModelAlias(jsplugin.DefaultRegistry.Generation(), aliasName)
				require.True(t, found, "the test must exercise the mapped task-alias validation path")
				require.Equal(t, modelName, target.Declared)
			} else {
				insertPricingEndpointChannel(t, 960, constant.ChannelTypeOpenAI, dto.ChannelOtherSettings{})
				insertPricingEndpointAbility(t, 960, modelName)
			}

			draft := PricingValues{
				"billing_setting.billing_mode": billing_setting.BillingModeTieredExpr,
				"billing_setting.billing_expr": testCase.expression,
			}
			if testCase.pluginExpression != "" {
				draft[billing_setting.PluginBillingExprOption] = map[string]any{
					"model-pricing-fixed-provider-0": testCase.pluginExpression,
				}
			}
			err := ValidateModelPricing(name, draft)
			if testCase.wantError != "" {
				require.ErrorContains(t, err, testCase.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testCase.expression, draft["billing_setting.billing_expr"])
			if testCase.pluginExpression != "" {
				assert.Equal(t, map[string]any{
					"model-pricing-fixed-provider-0": testCase.pluginExpression,
				}, draft[billing_setting.PluginBillingExprOption], "model validation must preserve explicit plugin overrides")
			}
		})
	}
}
