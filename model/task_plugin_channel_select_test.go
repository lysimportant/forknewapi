package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

func TestTaskPluginChannelSelectionFiltersBothCachePaths(t *testing.T) {
	truncateTables(t)
	priority := int64(0)
	weight := uint(1)
	baseURL := "https://example.com"
	alphaSetting := `{"task_plugin_key":"alpha"}`
	betaSetting := `{"task_plugin_key":"beta"}`
	channels := []Channel{
		{Id: 900001, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: "alpha", Models: "shared", Group: "default", Priority: &priority, Weight: &weight, BaseURL: &baseURL, Setting: &alphaSetting},
		{Id: 900002, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: "beta", Models: "shared", Group: "default", Priority: &priority, Weight: &weight, BaseURL: &baseURL, Setting: &betaSetting},
		{Id: 900003, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "ordinary", Models: "ordinary", Group: "default", Priority: &priority, Weight: &weight},
		{Id: 900004, Type: constant.ChannelTypeKling, Status: common.ChannelStatusEnabled, Name: "legacy-alpha", Models: "legacy", Group: "default", Priority: &priority, Weight: &weight},
		{Id: 900005, Type: constant.ChannelTypeJimeng, Status: common.ChannelStatusEnabled, Name: "legacy-beta", Models: "legacy", Group: "default", Priority: &priority, Weight: &weight},
	}
	for i := range channels {
		require.NoError(t, channels[i].Insert())
	}

	selected, err := GetChannel("default", "shared", 0, identityFilters("alpha", nil))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "alpha", selected.Name)
	selected, err = GetChannel("default", "shared", 0, identityFilters("", nil))
	require.NoError(t, err)
	assert.Nil(t, selected)
	selected, err = GetChannel("default", "ordinary", 0, identityFilters("", nil))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "ordinary", selected.Name)
	selected, err = GetChannel("default", "legacy", 0, identityFilters("legacy-alpha", []int{constant.ChannelTypeKling}))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "legacy-alpha", selected.Name)
	selected, err = GetChannel("default", "legacy", 0, identityFilters("legacy-alpha", []int{constant.ChannelTypeKling, constant.ChannelTypeJimeng}))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Contains(t, []string{"legacy-alpha", "legacy-beta"}, selected.Name)
}

func identityFilters(key string, channelTypes []int) []dto.ChannelFilter {
	return []dto.ChannelFilter{{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          key,
		TaskPluginChannelTypes: channelTypes,
	}}
}

func TestSharedPluginKeysFilterBothChannelSources(t *testing.T) {
	truncateTables(t)
	originalMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })
	filters := []dto.ChannelFilter{{Kind: dto.FilterTaskPluginIdentity, TaskPluginKey: "alpha", TaskPluginKeys: []string{"alpha", "beta"}}}
	var abilities []Ability
	for index, key := range []string{"alpha", "beta", "unrelated"} {
		setting := `{"task_plugin_key":"` + key + `"}`
		channel := Channel{Id: 910001 + index, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: key, Models: "shared", Group: "default", Setting: &setting}
		require.NoError(t, channel.Insert())
		abilities = append(abilities, Ability{ChannelId: channel.Id, Model: "shared", Group: "default", Enabled: true})
		matches, _ := ChannelSatisfiesFilters(&channel, "shared", filters)
		assert.Equal(t, index < 2, matches)
	}
	assert.Equal(t, abilities[:2], filterAbilitiesByConstraints(abilities, "shared", filters))
	InitChannelCache()
	channelSyncLock.RLock()
	kept, emptied := filterCandidateIDs([]int{910001, 910002, 910003}, "shared", filters)
	channelSyncLock.RUnlock()
	assert.Equal(t, []int{910001, 910002}, kept)
	assert.Empty(t, emptied)
}

// TestTaskModelAliasChannelRouting 验证渠道授权模型、映射歧义和只读设置解析；
// SQLite 使用临时文件，MySQL/PostgreSQL 仅连接显式配置的 TEST_* 隔离实例。
func TestTaskModelAliasChannelRouting(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, spec := range []struct{ key, models, routing string }{
		{"alias-channel-alpha", `[]`, `modelRouting:"channel",`},
		{"alias-channel-beta", `[]`, `modelRouting:"channel",`},
		{"alias-declared", `["global-model","declared-a","declared-b"]`, ""},
		{"alias-shared-declared", `["global-model"]`, ""},
	} {
		source := fmt.Sprintf(`
export const meta = {
 apiVersion:1,key:%q,name:%q,version:"1.0.0",author:{name:"Test"},
 models:%s,%s fetchMode:"per_task",protocols:["openai_video"],
 usageSchema:{videos:{type:"number",unit:"count",description:{en:"Video generation unit price",zh:"视频生成单价"}}}
};
export const protocols = {openai_video:{
 decodeRequest(ctx){return {kind:"submit",model:ctx.model,requestBody:ctx.body.value};},
 render(ctx,task){return task;}
}};
export function buildSubmitRequest(){return {};}
export function parseSubmitResponse(){return {};}
export function buildQueryRequest(){return {};}
export function parseTaskResult(){return {};}
export function listArtifacts(){return [];}
export function buildContentRequest(){return {};}
`, spec.key, spec.key, spec.models, spec.routing)
		_, err := registry.Register(source, jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("TEST_" + strings.ToUpper(dialect) + "_DSN")
			if dialect == "sqlite" {
				dsn = "local"
				previousPath := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "task-alias.db")
				t.Cleanup(func() { common.SQLitePath = previousPath })
			}
			if dsn == "" {
				t.Skip("isolated test database DSN is not configured")
			}
			t.Setenv("TASK_ALIAS_TEST_DSN", dsn)
			db, dbType, err := chooseDB("TASK_ALIAS_TEST_DSN", false)
			require.NoError(t, err)
			previousType := common.MainDatabaseType()
			common.SetMainDatabaseType(dbType)
			initCol()
			t.Cleanup(func() { common.SetMainDatabaseType(previousType); initCol() })
			db.Config.NamingStrategy = schema.NamingStrategy{TablePrefix: fmt.Sprintf("task_alias_test_%d_", time.Now().UnixNano())}
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousView := DB, taskAliasViewPtr.Load()
			DB = db
			taskAliasViewPtr.Store(nil)
			t.Cleanup(func() {
				assert.NoError(t, db.Migrator().DropTable(&Channel{}))
				DB = previousDB
				taskAliasViewPtr.Store(previousView)
				assert.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Channel{}))
			versionQuery := "SELECT VERSION()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s %s", dialect, version)

			alphaSetting := `{"task_plugin_key":"alias-channel-alpha"}`
			betaSetting := `{"task_plugin_key":"alias-channel-beta"}`
			declaredSetting := `{"task_plugin_key":"alias-declared"}`
			unknownSetting := `{"task_plugin_key":"unavailable-plugin"}`
			invalidSetting := `{"task_plugin_key":`
			alphaMapping := `{"alias-model":"upstream-a","chain-alias":"middle","middle":"upstream-b","multi-target":"upstream-a","cycle-a":"cycle-b","cycle-b":"cycle-a","self-model":"self-model","not-exposed":"upstream-c"}`
			secondMapping := `{"multi-target":"upstream-b"}`
			declaredMapping := `{"legacy-alias":"declared-a","legacy-ambiguous":"declared-a","static-conflict":"declared-a"}`
			otherDeclaredMapping := `{"legacy-ambiguous":"declared-b"}`
			invalidMapping := `{`
			channels := []Channel{
				{Id: 1, Models: "raw-model,alias-model,chain-alias,multi-target,cycle-a,cycle-b,self-model,global-model,Case-Model", Setting: &alphaSetting, ModelMapping: &alphaMapping},
				{Id: 2, Models: "multi-target,conflict,case-model", Setting: &alphaSetting, ModelMapping: &secondMapping},
				{Id: 3, Models: "CONFLICT,static-conflict", Setting: &betaSetting},
				{Id: 4, Models: "unbound-model"},
				{Id: 5, Models: "disabled-model", Setting: &alphaSetting, Status: common.ChannelStatusManuallyDisabled},
				{Id: 6, Models: "wrong-type", Setting: &alphaSetting, Type: constant.ChannelTypeOpenAI},
				{Id: 7, Models: "invalid-setting", Setting: &invalidSetting},
				{Id: 8, Models: "legacy-alias,legacy-ambiguous,static-unmapped,static-conflict", Setting: &declaredSetting, ModelMapping: &declaredMapping},
				{Id: 9, Models: "legacy-ambiguous", ModelMapping: &otherDeclaredMapping},
				{Id: 10, Models: "unknown-plugin-model", Setting: &unknownSetting},
				{Id: 11, Models: "invalid-mapping", Setting: &alphaSetting, ModelMapping: &invalidMapping},
			}
			for i := range channels {
				if channels[i].Type == 0 {
					channels[i].Type = constant.ChannelTypeTaskPlugin
				}
				if channels[i].Status == 0 {
					channels[i].Status = common.ChannelStatusEnabled
				}
				channels[i].Name = fmt.Sprintf("channel-%d", channels[i].Id)
				channels[i].Key = "synthetic-channel-key"
				require.NoError(t, db.Create(&channels[i]).Error)
			}
			for _, name := range []string{"raw-model", "alias-model", "chain-alias", "multi-target", "self-model", "Case-Model", "case-model"} {
				target, found := ResolveTaskModelAlias(generation, name)
				require.True(t, found, name)
				assert.Equal(t, TaskAliasTarget{Alias: name, PluginKey: "alias-channel-alpha", ChannelScoped: true}, target)
				_, found = ResolveTaskModelAlias(generation, strings.ToUpper(name))
				assert.False(t, found, name)
			}
			for _, name := range []string{"conflict", "CONFLICT", "static-conflict", "cycle-a", "cycle-b", "global-model", "not-exposed", "middle", "upstream-a", "unbound-model", "disabled-model", "wrong-type", "invalid-setting", "static-unmapped", "unknown-plugin-model", "invalid-mapping"} {
				_, found := ResolveTaskModelAlias(generation, name)
				assert.False(t, found, name)
			}
			for _, spec := range []struct{ name, declared string }{{"legacy-alias", "declared-a"}, {"legacy-ambiguous", ""}} {
				target, found := ResolveTaskModelAlias(generation, spec.name)
				require.True(t, found, spec.name)
				assert.Equal(t, TaskAliasTarget{Alias: spec.name, Declared: spec.declared, PluginKey: "alias-declared"}, target)
				folded, found := ResolveTaskModelAlias(generation, strings.ToUpper(spec.name))
				require.True(t, found, spec.name)
				assert.Equal(t, target, folded)
			}
			var saved Channel
			require.NoError(t, db.First(&saved, 7).Error)
			assert.Equal(t, channels[6].Name, saved.Name)
			assert.Equal(t, channels[6].Key, saved.Key)
			assert.Equal(t, channels[6].Setting, saved.Setting)
			assert.Equal(t, channels[6].Models, saved.Models)
			require.NoError(t, db.AutoMigrate(&Option{}))
			t.Cleanup(func() { assert.NoError(t, db.Migrator().DropTable(&Option{})) })
			previousRegistry := jsplugin.DefaultRegistry
			jsplugin.DefaultRegistry = registry
			t.Cleanup(func() { jsplugin.DefaultRegistry = previousRegistry })
			snapshot, err := GetModelPricingSnapshot([]string{"raw-model", "alias-model", "declared-a", "global-model", "ordinary-model"})
			require.NoError(t, err)
			require.Len(t, snapshot.Entries, 5)
			for _, entry := range snapshot.Entries {
				switch entry.ModelName {
				case "raw-model", "alias-model":
					require.Contains(t, entry.UsageSchema, "videos", entry.ModelName)
					assert.Equal(t, "number", entry.UsageSchema["videos"].Type)
					assert.Equal(t, "count", entry.UsageSchema["videos"].Unit)
					assert.Equal(t, "视频生成单价", entry.UsageSchema["videos"].Description["zh"])
					assert.Equal(t, "alias-channel-alpha", entry.TaskPluginKey)
				case "declared-a":
					assert.Equal(t, "alias-declared", entry.TaskPluginKey)
				case "global-model":
					assert.Empty(t, entry.TaskPluginKey)
					require.Len(t, entry.PluginVariants, 2)
				case "ordinary-model":
					assert.Empty(t, entry.TaskPluginKey)
					assert.Empty(t, entry.UsageSchema)
				}
			}
		})
	}
}

func TestNewAPIChannelServesExtendedTaskPlugins(t *testing.T) {
	truncateTables(t)
	priority := int64(0)
	weight := uint(1)
	baseURL := "https://gateway.example"
	extended := `{"task_extend_plugin_keys":["alpha","gamma"]}`
	single := `{"task_plugin_key":"delta"}`
	channels := []Channel{
		{Id: 920001, Type: constant.ChannelTypeNewAPI, Status: common.ChannelStatusEnabled, Name: "gateway", Models: "shared,chat", Group: "default", Priority: &priority, Weight: &weight, BaseURL: &baseURL, Setting: &extended},
		{Id: 920002, Type: constant.ChannelTypeNewAPI, Status: common.ChannelStatusEnabled, Name: "gateway-single", Models: "shared", Group: "default", Priority: &priority, Weight: &weight, BaseURL: &baseURL, Setting: &single},
	}
	for i := range channels {
		require.NoError(t, channels[i].Insert())
	}

	selected, err := GetChannel("default", "shared", 0, identityFilters("alpha", nil))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "gateway", selected.Name)
	selected, err = GetChannel("default", "shared", 0, identityFilters("delta", nil))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "gateway-single", selected.Name, "a single task_plugin_key still binds a New API channel")
	selected, err = GetChannel("default", "shared", 0, identityFilters("beta", nil))
	require.NoError(t, err)
	assert.Nil(t, selected, "unbound plugins never reach the gateway channel")
	shared := []dto.ChannelFilter{{Kind: dto.FilterTaskPluginIdentity, TaskPluginKey: "beta", TaskPluginKeys: []string{"beta", "gamma"}}}
	selected, err = GetChannel("default", "shared", 0, shared)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "gateway", selected.Name, "a bound shared-model candidate admits the channel")
	selected, err = GetChannel("default", "chat", 0, identityFilters("", nil))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "gateway", selected.Name, "requests without a pinned plugin keep using the gateway")
}

func TestModelModifierNamesRouteWithAndWithoutMemoryCache(t *testing.T) {
	truncateTables(t)
	originalMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })
	channel := Channel{Id: 930001, Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Name: "deepseek", Models: "deepseek-flash", Group: "default"}
	require.NoError(t, channel.Insert())

	for _, memoryCache := range []bool{false, true} {
		common.MemoryCacheEnabled = memoryCache
		InitChannelCache()
		selected, err := GetRandomSatisfiedChannel("default", "deepseek-flash@effort:max", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, selected, "memory cache %v", memoryCache)
		assert.Equal(t, "deepseek", selected.Name)
		selected, err = GetRandomSatisfiedChannel("default", "deepseek-pro@effort:max", 0, nil)
		require.NoError(t, err)
		assert.Nil(t, selected, "memory cache %v", memoryCache)
	}
}
