package model

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// TaskAliasTarget 表示跨渠道聚合后的模型别名或渠道授权模型。
// Declared 为空表示渠道模式或同插件存在多个映射尾目标；ChannelScoped 仅允许
// 显式绑定插件的类型 61 渠道在选中后解析自己的映射，不依赖全局模型声明。
type TaskAliasTarget struct {
	Alias         string
	Declared      string
	PluginKey     string
	ChannelScoped bool
}

type taskAliasView struct {
	generation   uint64
	expiresAt    time.Time
	byExact      map[string]TaskAliasTarget
	byFold       map[string]TaskAliasTarget
	usageByExact map[string]taskAliasUsage
}

type taskAliasUsage struct {
	schema   map[string]jsplugin.UsageFieldSchema
	examples []jsplugin.UsageExample
}

// taskPricingUsage resolves display metadata without changing the routing
// target. Channel-scoped targets intentionally have no global Declared model.
func taskPricingUsage(g *jsplugin.RoutingGeneration, name string) (*jsplugin.LoadedPlugin, taskAliasUsage, bool) {
	if plugin, ok := g.GetByModel(name); ok {
		schema, examples := plugin.Meta.UsageForModel(name)
		return plugin, taskAliasUsage{schema, examples}, true
	}
	target, ok := ResolveTaskModelAlias(g, name)
	if !ok {
		return nil, taskAliasUsage{}, false
	}
	plugin, ok := g.Get(target.PluginKey)
	if !ok {
		return nil, taskAliasUsage{}, false
	}
	if target.ChannelScoped {
		if view := loadFreshTaskAliasView(g); view != nil {
			if usage, found := view.usageByExact[name]; found {
				return plugin, usage, true
			}
		}
	}
	schema, examples := plugin.Meta.UsageForModel(target.Declared)
	return plugin, taskAliasUsage{schema, examples}, true
}

func taskPricingPlugins(g *jsplugin.RoutingGeneration, name string) []*jsplugin.LoadedPlugin {
	providers := g.PluginsByModel(name)
	if len(providers) > 0 {
		return providers
	}
	if target, ok := ResolveTaskModelAlias(g, name); ok && target.ChannelScoped {
		if plugin, ok := g.Get(target.PluginKey); ok {
			return []*jsplugin.LoadedPlugin{plugin}
		}
	}
	return nil
}

func taskPricingProviderUsage(g *jsplugin.RoutingGeneration, name string, provider *jsplugin.LoadedPlugin) taskAliasUsage {
	if plugin, usage, ok := taskPricingUsage(g, name); ok && plugin.Meta.Key == provider.Meta.Key {
		return usage
	}
	schema, examples := provider.Meta.UsageForModel(name)
	return taskAliasUsage{schema, examples}
}

// Keep the channel plugin's existing billing fields for saved expressions,
// including harmless legacy fields such as seconds*0, while allowing fields
// introduced by its selected profile. This must not rewrite stored prices.
func taskPricingValidationSchema(g *jsplugin.RoutingGeneration, name string, provider *jsplugin.LoadedPlugin) map[string]jsplugin.UsageFieldSchema {
	if target, ok := ResolveTaskModelAlias(g, name); ok && target.ChannelScoped && target.PluginKey == provider.Meta.Key {
		schema := jsplugin.CloneUsageSchema(provider.Meta.UsageSchema)
		if schema == nil {
			schema = make(map[string]jsplugin.UsageFieldSchema)
		}
		usage := taskPricingProviderUsage(g, name, provider)
		for key, field := range usage.schema {
			schema[key] = field
		}
		return schema
	}
	schema, _ := provider.Meta.UsageForModel(name)
	return schema
}

const taskAliasViewTTL = 60 * time.Second

var (
	taskAliasViewPtr   atomic.Pointer[taskAliasView]
	taskAliasRebuildMu sync.Mutex
)

// ResolveTaskModelAlias 返回 name 对应的模型别名或渠道授权模型。
// 渠道模式仅匹配已保存的精确名称，旧别名保持 ASCII 大小写折叠。
// g 为请求固定的路由代次；代次不一致或缓存过期时从已启用渠道重建，未找到返回 false。
func ResolveTaskModelAlias(g *jsplugin.RoutingGeneration, name string) (TaskAliasTarget, bool) {
	if g == nil || name == "" {
		return TaskAliasTarget{}, false
	}
	view := loadFreshTaskAliasView(g)
	if view == nil {
		return TaskAliasTarget{}, false
	}
	if target, ok := view.byExact[name]; ok {
		return target, true
	}
	target, ok := view.byFold[jsplugin.ASCIIFold(name)]
	return target, ok
}

func loadFreshTaskAliasView(g *jsplugin.RoutingGeneration) *taskAliasView {
	view := taskAliasViewPtr.Load()
	if taskAliasViewFresh(view, g.Number) {
		return view
	}
	taskAliasRebuildMu.Lock()
	defer taskAliasRebuildMu.Unlock()
	view = taskAliasViewPtr.Load()
	if taskAliasViewFresh(view, g.Number) {
		return view
	}
	rebuilt := buildTaskAliasView(g)
	taskAliasViewPtr.Store(rebuilt)
	return rebuilt
}

func taskAliasViewFresh(view *taskAliasView, generation uint64) bool {
	return view != nil && view.generation == generation && time.Now().Before(view.expiresAt)
}

func rebuildTaskAliasView() {
	taskAliasRebuildMu.Lock()
	defer taskAliasRebuildMu.Unlock()
	taskAliasViewPtr.Store(buildTaskAliasView(jsplugin.DefaultRegistry.Generation()))
}

type taskAliasDraft struct {
	spellings     []string
	byPlugin      map[string]map[string]struct{}
	channelScoped bool
	usageByExact  map[string][]taskAliasUsage
}

func buildTaskAliasView(generation *jsplugin.RoutingGeneration) *taskAliasView {
	genNum := uint64(0)
	if generation != nil {
		genNum = generation.Number
	}
	view := &taskAliasView{
		generation:   genNum,
		expiresAt:    time.Now().Add(taskAliasViewTTL),
		byExact:      make(map[string]TaskAliasTarget),
		byFold:       make(map[string]TaskAliasTarget),
		usageByExact: make(map[string]taskAliasUsage),
	}
	if DB == nil {
		return view
	}

	var channels []Channel
	err := DB.Select("id", "type", "models", "model_mapping", "setting").
		Where("status = ?", common.ChannelStatusEnabled).
		Find(&channels).Error
	if err != nil {
		common.SysError(fmt.Sprintf("rebuild task alias view: %s", err.Error()))
		return view
	}

	drafts := make(map[string]*taskAliasDraft)
	for i := range channels {
		channel := &channels[i]
		var channelPlugin *jsplugin.LoadedPlugin
		var setting dto.ChannelSettings
		if channel.Type == constant.ChannelTypeTaskPlugin && channel.Setting != nil && *channel.Setting != "" {
			// GetSetting 在解析失败时保存渠道；这里只加载部分列，必须保持只读。
			if err := common.UnmarshalJsonStr(*channel.Setting, &setting); err != nil {
				common.SysError(fmt.Sprintf("task alias view: channel %d invalid setting JSON (%T)", channel.Id, err))
				continue
			}
			if plugin, ok := generation.Get(setting.TaskPluginKey); ok && plugin.Meta.ModelRouting == "channel" {
				channelPlugin = plugin
			}
		}
		mappingJSON := channel.GetModelMapping()
		modelMap := make(map[string]string)
		if mappingJSON != "" && mappingJSON != "{}" {
			if err := common.UnmarshalJsonStr(mappingJSON, &modelMap); err != nil {
				common.SysError(fmt.Sprintf("task alias view: channel %d model_mapping: %s", channel.Id, err.Error()))
				continue
			}
		}
		inModels := make(map[string]struct{})
		for _, modelName := range channel.GetModels() {
			inModels[modelName] = struct{}{}
		}
		for alias := range inModels {
			if alias == "" {
				continue
			}
			if _, declared := generation.CanonicalModel(alias); declared {
				continue
			}
			tail, cyclic := followChannelModelMapping(modelMap, alias)
			if cyclic {
				common.SysError(fmt.Sprintf("task alias mapping cycle dropped: channel=%d key=%q", channel.Id, alias))
				continue
			}
			plugin := channelPlugin
			declared := ""
			if plugin == nil {
				if modelMap[alias] == "" {
					continue
				}
				var ok bool
				declared, ok = generation.CanonicalModel(tail)
				if !ok {
					continue
				}
				plugin, ok = generation.GetByModel(declared)
				if !ok {
					continue
				}
			}
			fold := jsplugin.ASCIIFold(alias)
			draft := drafts[fold]
			if draft == nil {
				draft = &taskAliasDraft{byPlugin: make(map[string]map[string]struct{}), usageByExact: make(map[string][]taskAliasUsage)}
				drafts[fold] = draft
			}
			draft.spellings = append(draft.spellings, alias)
			draft.channelScoped = draft.channelScoped || channelPlugin != nil
			declareds := draft.byPlugin[plugin.Meta.Key]
			if declareds == nil {
				declareds = make(map[string]struct{})
				draft.byPlugin[plugin.Meta.Key] = declareds
			}
			declareds[declared] = struct{}{}
			if channelPlugin != nil {
				modelType := TaskPluginModelType(channelPlugin, setting, tail)
				schema, examples := channelPlugin.Meta.UsageForModelType(tail, modelType)
				draft.usageByExact[alias] = append(draft.usageByExact[alias], taskAliasUsage{schema, examples})
			}
		}
	}

	for _, draft := range drafts {
		alias := draft.spellings[0]
		for _, spelling := range draft.spellings[1:] {
			if spelling < alias {
				alias = spelling
			}
		}
		if len(draft.byPlugin) != 1 {
			pluginKeys := make([]string, 0, len(draft.byPlugin))
			for key := range draft.byPlugin {
				pluginKeys = append(pluginKeys, key)
			}
			common.SysLog(fmt.Sprintf("task model alias %q dropped: maps to multiple plugins %v", alias, pluginKeys))
			continue
		}
		var pluginKey, declared string
		for key, declareds := range draft.byPlugin {
			pluginKey = key
			if len(declareds) == 1 {
				for name := range declareds {
					declared = name
				}
			}
		}
		target := TaskAliasTarget{
			Alias:         alias,
			Declared:      declared,
			PluginKey:     pluginKey,
			ChannelScoped: draft.channelScoped,
		}
		if draft.channelScoped {
			for _, spelling := range draft.spellings {
				target.Alias = spelling
				view.byExact[spelling] = target
				if usages := draft.usageByExact[spelling]; len(usages) > 0 {
					usage := usages[0]
					for _, other := range usages[1:] {
						if !reflect.DeepEqual(usage, other) {
							plugin, _ := generation.Get(pluginKey)
							usage = taskAliasUsage{plugin.Meta.UsageSchema, plugin.Meta.UsageExamples}
							break
						}
					}
					view.usageByExact[spelling] = usage
				}
			}
		} else {
			view.byFold[jsplugin.ASCIIFold(alias)] = target
		}
	}
	return view
}

// followChannelModelMapping 按 ModelMappedHelper 的规则解析单个渠道的映射。
// 自映射停在当前名称，其他循环返回 cyclic=true，调用方必须拒绝该模型。
func followChannelModelMapping(modelMap map[string]string, start string) (string, bool) {
	current := start
	visited := map[string]bool{current: true}
	for {
		mapped, exists := modelMap[current]
		if !exists || mapped == "" {
			return current, false
		}
		if visited[mapped] {
			if mapped == current {
				return current, false
			}
			return "", true
		}
		visited[mapped] = true
		current = mapped
	}
}
