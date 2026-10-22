package model

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type pricingImage2ProBlockedTransport struct{ requests atomic.Int64 }

func (transport *pricingImage2ProBlockedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.requests.Add(1)
	return nil, fmt.Errorf("network is disabled in the Image2Pro pricing fixture")
}

// This uses the shipped plugin source in a private registry, rather than a mock
// or importing plugins (whose init registers every factory globally).
func setupPricingImage2ProFixture(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "image2pro-pricing.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &Model{}, &Vendor{}, &Option{}))
	previousDB, previousRegistry, previousMemoryCache := DB, jsplugin.DefaultRegistry, common.MemoryCacheEnabled
	previousConfig := config.GlobalConfig.ExportAllConfigs()
	previousTransport := http.DefaultTransport
	transport := &pricingImage2ProBlockedTransport{}
	http.DefaultTransport = transport
	DB = db
	jsplugin.DefaultRegistry = jsplugin.NewRegistry()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		assert.Zero(t, transport.requests.Load(), "pricing metadata must not contact the upstream")
		http.DefaultTransport = previousTransport
		require.NoError(t, config.GlobalConfig.LoadFromDB(previousConfig))
		DB, jsplugin.DefaultRegistry, common.MemoryCacheEnabled = previousDB, previousRegistry, previousMemoryCache
		InitChannelCache()
		InvalidatePricingCache()
		assert.NoError(t, sqlDB.Close())
	})
	source, err := os.ReadFile(filepath.Join("..", "plugins", "tasks", "image2pro", "plugin.js"))
	require.NoError(t, err)
	plugin, err := jsplugin.DefaultRegistry.RegisterFactory(string(source), jsplugin.Options{Key: "image2pro"})
	require.NoError(t, err)
	require.Equal(t, "channel", plugin.Meta.ModelRouting)
	return plugin
}

func TestPricingImage2ProImageModelsKeepSchemaAndConfiguredPrices(t *testing.T) {
	plugin := setupPricingImage2ProFixture(t)
	const dynamicImage = "future-image2pro-picture"
	const mappedImage = "mapped-image2pro-picture"
	names := []string{"Nano-banana-Pro", "无限制-Seedream-5-Pro-2K", dynamicImage, mappedImage}
	channel := &Channel{
		Id: 1, Type: constant.ChannelTypeTaskPlugin, Key: "synthetic", Status: common.ChannelStatusEnabled,
		Name: "Image2Pro pricing fixture", Group: "default", Models: strings.Join(names, ","),
	}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "image2pro", TaskPluginModelTypes: map[string]string{dynamicImage: "image"}})
	mapping := `{"mapped-image2pro-picture":"Nano-banana-Pro"}`
	channel.ModelMapping = &mapping
	require.NoError(t, DB.Create(channel).Error)
	for _, name := range names {
		insertPricingEndpointAbility(t, channel.Id, name)
	}
	InitChannelCache()

	// Preserve the exact legacy expression, including its zero-cost seconds term.
	// Correcting the schema must not silently rewrite an administrator's prices.
	const expression = `tier("base", u("count") * 3.5 + u("seconds") * 0)`
	const override = `tier("image2pro", u("count") * 4.25 + u("seconds") * 0)`
	modes, expressions, overrides := map[string]string{}, map[string]string{}, map[string]string{}
	for _, name := range names {
		modes[name] = billing_setting.BillingModeTieredExpr
		expressions[name] = expression
		overrides[billing_setting.PluginBillingExprKey("image2pro", name)] = override
	}
	encode := func(value any) string {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		return string(encoded)
	}
	options := map[string]string{
		"billing_setting.billing_mode":          encode(modes),
		"billing_setting.billing_expr":          encode(expressions),
		billing_setting.PluginBillingExprOption: `{}`,
	}
	for key, value := range options {
		require.NoError(t, DB.Create(&Option{Key: key, Value: value}).Error)
	}
	require.NoError(t, config.GlobalConfig.LoadFromDB(options))
	InvalidatePricingCache()

	expectedSchema, _ := plugin.Meta.UsageForModel("Nano-banana-Pro")
	require.Contains(t, expectedSchema, "count")
	require.NotContains(t, expectedSchema, "seconds")
	prices := pricingByModel(GetPricing())
	snapshot, err := GetModelPricingSnapshot(names)
	require.NoError(t, err)
	entries := make(map[string]ModelPricingEntry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.ModelName] = entry
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			price, exists := prices[name]
			require.True(t, exists)
			assert.Equal(t, billing_setting.BillingModeTieredExpr, price.BillingMode)
			assert.Equal(t, expression, price.BillingExpr, "existing prices must be retained verbatim")
			assert.Equal(t, expectedSchema, price.BillingUsageSchema)
			assert.NotContains(t, price.BillingUsageSchema, "seconds", "image prices must not expose video duration")
			for _, example := range price.BillingUsageExamples {
				assert.NotContains(t, example.Facts, "seconds")
			}
			assert.Empty(t, price.BillingPluginVariants)
			assert.Equal(t, expectedSchema, entries[name].UsageSchema, "configuration editor must use the same image schema")
			assert.Equal(t, "image2pro", entries[name].TaskPluginKey)
			assert.Equal(t, expression, entries[name].Configured["billing_setting.billing_expr"])
			assert.NoError(t, ValidateModelPricing(name, PricingValues{
				"billing_setting.billing_mode": billing_setting.BillingModeTieredExpr,
				"billing_setting.billing_expr": expression,
			}), "the narrower image display profile must still accept legacy saved prices")
			assert.ErrorContains(t, ValidateModelPricing(name, PricingValues{
				"billing_setting.billing_expr": `tier("invalid", u("unknown_image2pro_usage") * 0)`,
			}), "not declared", "compatibility must not accept unknown usage fields")
		})
	}

	// Explicit per-plugin prices must survive channel-scoped model resolution.
	options[billing_setting.PluginBillingExprOption] = encode(overrides)
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", billing_setting.PluginBillingExprOption).Update("value", options[billing_setting.PluginBillingExprOption]).Error)
	require.NoError(t, config.GlobalConfig.LoadFromDB(options))
	InvalidatePricingCache()
	prices = pricingByModel(GetPricing())
	snapshot, err = GetModelPricingSnapshot(names)
	require.NoError(t, err)
	for _, entry := range snapshot.Entries {
		entries[entry.ModelName] = entry
	}
	for _, name := range names {
		t.Run(name+"/plugin_override", func(t *testing.T) {
			price := prices[name]
			assert.Equal(t, expression, price.BillingExpr)
			require.Len(t, price.BillingPluginVariants, 1)
			variant := price.BillingPluginVariants[0]
			assert.Equal(t, "image2pro", variant.PluginKey)
			assert.Equal(t, override, variant.BillingExpr)
			assert.Equal(t, billing_setting.BillingModeTieredExpr, variant.BillingMode)
			assert.Equal(t, expectedSchema, variant.BillingUsageSchema)
			assert.NotContains(t, variant.BillingUsageSchema, "seconds")
			require.Len(t, entries[name].PluginVariants, 1)
			configured := entries[name].PluginVariants[0]
			assert.Equal(t, "image2pro", configured.PluginKey)
			assert.Equal(t, override, configured.Configured)
			assert.Equal(t, override, configured.Effective)
			assert.True(t, configured.Compatible)
			assert.False(t, configured.Stale)
			assert.Equal(t, expectedSchema, configured.UsageSchema)
			assert.NoError(t, ValidateModelPricing(name, PricingValues{
				"billing_setting.billing_mode":          billing_setting.BillingModeTieredExpr,
				"billing_setting.billing_expr":          expression,
				billing_setting.PluginBillingExprOption: map[string]any{"image2pro": override},
			}), "channel-authorized models must accept plugin-specific legacy prices")
			assert.ErrorContains(t, ValidateModelPricing(name, PricingValues{
				billing_setting.PluginBillingExprOption: map[string]any{"image2pro": `tier("invalid", u("unknown_image2pro_usage") * 0)`},
			}), "not declared")
		})
	}
	assert.ErrorContains(t, ValidateModelPricing("unauthorized-image2pro-picture", PricingValues{
		billing_setting.PluginBillingExprOption: map[string]any{"image2pro": override},
	}), "does not declare this model", "plugin overrides must still require channel model authorization")
}

func TestPricingImage2ProVideoUnknownAndTypeConflictsKeepFallback(t *testing.T) {
	plugin := setupPricingImage2ProFixture(t)
	const dynamicVideo = "future-image2pro-video"
	const unknown = "unknown-image2pro-media"
	const conflicting = "conflicting-image2pro-media"
	const mappedImage = "mapped-dynamic-image2pro-picture"
	const upstreamImage = "future-image2pro-mapped-upstream"
	names := []string{"Seedance2.0 0.9r", "无限制-Flash-MAX-Video", dynamicVideo, unknown, conflicting, mappedImage}
	channel := &Channel{
		Id: 1, Type: constant.ChannelTypeTaskPlugin, Key: "synthetic", Status: common.ChannelStatusEnabled,
		Name: "Image2Pro mixed pricing fixture", Group: "default", Models: strings.Join(names, ","),
	}
	channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "image2pro", TaskPluginModelTypes: map[string]string{
		dynamicVideo: "video", conflicting: "image", upstreamImage: "image",
	}})
	// Type metadata belongs to the final upstream model, including chained aliases.
	mapping := `{"mapped-dynamic-image2pro-picture":"image2pro-intermediate","image2pro-intermediate":"future-image2pro-mapped-upstream"}`
	channel.ModelMapping = &mapping
	require.NoError(t, DB.Create(channel).Error)
	for _, name := range names {
		insertPricingEndpointAbility(t, channel.Id, name)
	}
	otherChannel := &Channel{
		Id: 2, Type: constant.ChannelTypeTaskPlugin, Key: "synthetic", Status: common.ChannelStatusEnabled,
		Name: "Image2Pro conflicting pricing fixture", Group: "default", Models: conflicting,
	}
	otherChannel.SetSetting(dto.ChannelSettings{TaskPluginKey: "image2pro", TaskPluginModelTypes: map[string]string{conflicting: "video"}})
	require.NoError(t, DB.Create(otherChannel).Error)
	insertPricingEndpointAbility(t, otherChannel.Id, conflicting)
	InitChannelCache()

	const expression = `tier("legacy", u("count") * 3.5 + u("seconds") * 0)`
	modes, expressions := map[string]string{}, map[string]string{}
	for _, name := range names {
		modes[name], expressions[name] = billing_setting.BillingModeTieredExpr, expression
	}
	encodedModes, err := json.Marshal(modes)
	require.NoError(t, err)
	encodedExpressions, err := json.Marshal(expressions)
	require.NoError(t, err)
	options := map[string]string{
		"billing_setting.billing_mode":          string(encodedModes),
		"billing_setting.billing_expr":          string(encodedExpressions),
		billing_setting.PluginBillingExprOption: `{}`,
	}
	for key, value := range options {
		require.NoError(t, DB.Create(&Option{Key: key, Value: value}).Error)
	}
	require.NoError(t, config.GlobalConfig.LoadFromDB(options))
	InvalidatePricingCache()
	prices := pricingByModel(GetPricing())
	snapshot, err := GetModelPricingSnapshot(names)
	require.NoError(t, err)
	entries := make(map[string]ModelPricingEntry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.ModelName] = entry
	}
	seedanceSchema, _ := plugin.Meta.UsageForModel("Seedance2.0 0.9r")
	flashSchema, _ := plugin.Meta.UsageForModel("无限制-Flash-MAX-Video")
	imageSchema, _ := plugin.Meta.UsageForModel("Nano-banana-Pro")
	require.NotContains(t, seedanceSchema, "seconds")
	require.Contains(t, flashSchema, "seconds")
	require.NotContains(t, flashSchema, "count")
	for _, tc := range []struct {
		name   string
		schema map[string]jsplugin.UsageFieldSchema
	}{
		{"Seedance2.0 0.9r", seedanceSchema},
		{"无限制-Flash-MAX-Video", flashSchema},
		{dynamicVideo, plugin.Meta.UsageSchema},
		{unknown, plugin.Meta.UsageSchema},
		{conflicting, plugin.Meta.UsageSchema},
		{mappedImage, imageSchema},
	} {
		t.Run(tc.name, func(t *testing.T) {
			price, exists := prices[tc.name]
			require.True(t, exists)
			assert.Equal(t, tc.schema, price.BillingUsageSchema)
			assert.Equal(t, tc.schema, entries[tc.name].UsageSchema)
			assert.Equal(t, expression, price.BillingExpr)
			assert.Equal(t, expression, entries[tc.name].Configured["billing_setting.billing_expr"])
			target, ok := ResolveTaskModelAlias(jsplugin.DefaultRegistry.Generation(), tc.name)
			require.True(t, ok)
			assert.True(t, target.ChannelScoped)
			assert.Equal(t, "image2pro", target.PluginKey)
			assert.Empty(t, target.Declared, "display metadata must not change the channel routing target")
		})
	}
}
