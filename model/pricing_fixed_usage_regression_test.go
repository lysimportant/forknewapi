package model

import (
	"encoding/json"
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

func TestPricingFixedExpressionKeepsItsOwnMetadataWhenTaskPluginsShareModel(t *testing.T) {
	const modelName = "pricing-fixed-usage-model"
	const fixedExpression = `tier("base", fixed(6))`
	const taskExpression = `tier("base", u("count") * 0.2)`
	const overrideExpression = `tier("override", u("count") * 0.4)`
	for _, testCase := range []struct {
		name             string
		providers        int
		expression       string
		pluginOverride   bool
		wantsUsageSchema bool
	}{
		{name: "single_provider_fixed_price", providers: 1, expression: fixedExpression},
		{name: "shared_providers_fixed_price", providers: 2, expression: fixedExpression},
		{name: "fixed_price_preserves_explicit_plugin_override", providers: 2, expression: fixedExpression, pluginOverride: true},
		{name: "task_usage_expression_keeps_schema", providers: 1, expression: taskExpression, wantsUsageSchema: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resetPricingEndpointTestTables(t)
			saved := config.GlobalConfig.ExportAllConfigs()
			t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
			for provider := 0; provider < testCase.providers; provider++ {
				pluginKey := fmt.Sprintf("pricing-fixed-usage-%d", provider)
				source := fmt.Sprintf(`
export const meta = {
 apiVersion:1,key:%q,name:"Fixed Usage Probe",version:"1.0.0",author:{name:"Test"},
 models:[%q],fetchMode:"per_task",
 usageSchema:{count:{type:"number",unit:"count",description:"Generated image count."}},
 usageExamples:[{label:"One image",facts:{count:1}}]
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

			channelType := constant.ChannelTypeOpenAI
			if testCase.wantsUsageSchema {
				channelType = constant.ChannelTypeTaskPlugin
			}
			insertPricingEndpointChannel(t, 950, channelType, dto.ChannelOtherSettings{})
			insertPricingEndpointAbility(t, 950, modelName)
			modes, err := json.Marshal(map[string]string{modelName: billing_setting.BillingModeTieredExpr})
			require.NoError(t, err)
			expressions, err := json.Marshal(map[string]string{modelName: testCase.expression})
			require.NoError(t, err)
			overrides := map[string]string{}
			if testCase.pluginOverride {
				overrides["pricing-fixed-usage-1::"+modelName] = overrideExpression
			}
			pluginExpressions, err := json.Marshal(overrides)
			require.NoError(t, err)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode":          string(modes),
				"billing_setting.billing_expr":          string(expressions),
				billing_setting.PluginBillingExprOption: string(pluginExpressions),
			}))
			InitChannelCache()
			InvalidatePricingCache()

			prices := pricingByModel(GetPricing())
			require.Contains(t, prices, modelName)
			price := prices[modelName]
			assert.Equal(t, billing_setting.BillingModeTieredExpr, price.BillingMode)
			assert.Equal(t, testCase.expression, price.BillingExpr)
			if testCase.wantsUsageSchema {
				assert.Equal(t, "count", price.BillingUsageSchema["count"].Unit)
				assert.Len(t, price.BillingUsageExamples, 1)
				return
			}

			assert.Empty(t, price.BillingUsageSchema, "a shared plugin model name must not turn a fixed request price into task-usage pricing")
			assert.Empty(t, price.BillingUsageExamples)
			if !testCase.pluginOverride {
				assert.Empty(t, price.BillingPluginVariants, "automatic task variants must not hide the configured fixed request expression")
				return
			}
			for _, variant := range price.BillingPluginVariants {
				if variant.PluginKey != "pricing-fixed-usage-1" {
					continue
				}
				assert.Equal(t, billing_setting.BillingModeTieredExpr, variant.BillingMode)
				assert.Equal(t, overrideExpression, variant.BillingExpr)
				assert.Equal(t, "count", variant.BillingUsageSchema["count"].Unit)
				assert.Len(t, variant.BillingUsageExamples, 1)
				return
			}
			t.Fatal("explicit task-plugin pricing override was omitted")
		})
	}
}
