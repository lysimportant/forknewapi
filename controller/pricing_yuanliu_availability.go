package controller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

// 源流目录缓存、排队和单次状态请求的时间及并发边界。
const (
	yuanliuCatalogTTL              = 90 * time.Second
	yuanliuCatalogFailureTTL       = 15 * time.Second
	yuanliuCatalogQueueTimeout     = 8 * time.Second
	yuanliuAvailabilityWaitTimeout = 8 * time.Second
	yuanliuCatalogConcurrency      = 8
)

var yuanliuCatalogFetchSlots = make(chan struct{}, yuanliuCatalogConcurrency)

// yuanliuAvailability 是一个可见模型的只读状态；checked_at 仅在目录成功时存在。
type yuanliuAvailability struct {
	Status    string  `json:"status"`
	CheckedAt *string `json:"checked_at"`
}

// yuanliuCatalogSnapshot 保存一个渠道的目录名称及抽样范围，不保存凭据或原始上游响应。
type yuanliuCatalogSnapshot struct {
	names               map[string]string
	checkedAt           time.Time
	expiresAt           time.Time
	signature           [32]byte
	multipleEnabledKeys bool
}

// yuanliuCatalogCache 按渠道配置隔离短期目录结果，合并同时到达的刷新请求。
type yuanliuCatalogCache struct {
	sync.Mutex
	group   singleflight.Group
	entries map[int]yuanliuCatalogSnapshot
}

var pricingYuanliuCatalogCache = yuanliuCatalogCache{entries: make(map[int]yuanliuCatalogSnapshot)}

// get 只返回尚在有效期内且配置一致的目录；等待超时返回空快照，刷新仍可完成并更新缓存。
func (cache *yuanliuCatalogCache) get(ctx context.Context, channel *model.Channel, plugin jsplugin.Meta, fetch func(*model.Channel) (map[string]string, error)) yuanliuCatalogSnapshot {
	config, err := common.Marshal(struct {
		Key               string
		BaseURL           *string
		Setting           *string
		OtherSettings     string
		HeaderOverride    *string
		Keys              []string
		IsMultiKey        bool
		MultiKeyStatuses  map[int]int
		PluginVersion     string
		PluginBaseURL     string
		ModelDiscovery    *jsplugin.ModelDiscovery
		SupportedModelIDs []string
	}{channel.Key, channel.BaseURL, channel.Setting, channel.OtherSettings, channel.HeaderOverride,
		channel.Keys, channel.ChannelInfo.IsMultiKey, channel.ChannelInfo.MultiKeyStatusList,
		plugin.Version, plugin.BaseURL, plugin.ModelDiscovery, plugin.Models})
	if err != nil {
		return yuanliuCatalogSnapshot{}
	}
	signature := sha256.Sum256(config)
	cache.Lock()
	cached, ok := cache.entries[channel.Id]
	cache.Unlock()
	if ok && cached.signature == signature && time.Now().Before(cached.expiresAt) {
		return cached
	}

	key := fmt.Sprintf("%d:%x", channel.Id, signature)
	resultChannel := cache.group.DoChan(key, func() (any, error) {
		cache.Lock()
		cached, ok := cache.entries[channel.Id]
		cache.Unlock()
		if ok && cached.signature == signature && time.Now().Before(cached.expiresAt) {
			return cached, nil
		}
		queueTimer := time.NewTimer(yuanliuCatalogQueueTimeout)
		var names map[string]string
		var fetchErr error
		select {
		case yuanliuCatalogFetchSlots <- struct{}{}:
			queueTimer.Stop()
			names, fetchErr = fetch(channel)
			<-yuanliuCatalogFetchSlots
		case <-queueTimer.C:
			fetchErr = errors.New("Yuanliu catalog fetch queue timed out")
		}
		now := time.Now().UTC()
		result := yuanliuCatalogSnapshot{signature: signature}
		if fetchErr == nil {
			result.names = names
			result.checkedAt = now
			result.expiresAt = now.Add(yuanliuCatalogTTL)
			if channel.ChannelInfo.IsMultiKey {
				enabledKeys := 0
				for index := range channel.GetKeys() {
					if status, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok && status != common.ChannelStatusEnabled {
						continue
					}
					enabledKeys++
					if enabledKeys > 1 {
						result.multipleEnabledKeys = true
						break
					}
				}
			}
		} else {
			result.expiresAt = now.Add(yuanliuCatalogFailureTTL)
		}
		cache.Lock()
		cache.entries[channel.Id] = result
		cache.Unlock()
		return result, nil
	})
	select {
	case result := <-resultChannel:
		return result.Val.(yuanliuCatalogSnapshot)
	case <-ctx.Done():
		return yuanliuCatalogSnapshot{}
	}
}

// fetchYuanliuModelNames 使用渠道已有的目录地址、密钥和代理设置进行一次受限的只读请求。
func fetchYuanliuModelNames(channel *model.Channel) (map[string]string, error) {
	plugin, ok := jsplugin.DefaultRegistry.Get("yuanliu")
	if !ok || plugin.Meta.ModelDiscovery == nil || plugin.Meta.ModelDiscovery.Protocol != "openai" {
		return nil, errors.New("Yuanliu model discovery is unavailable")
	}
	var settings dto.ChannelSettings
	if channel.Setting != nil && *channel.Setting != "" {
		if err := common.UnmarshalJsonStr(*channel.Setting, &settings); err != nil {
			return nil, errors.New("invalid channel settings")
		}
	}
	if settings.TaskPluginKey != "yuanliu" {
		return nil, errors.New("channel is not bound to Yuanliu")
	}
	baseURL := strings.TrimSpace(channel.GetBaseURL())
	if baseURL == "" {
		baseURL = plugin.Meta.BaseURL
	}
	requestURL, err := taskPluginModelDiscoveryURL(baseURL, *plugin.Meta.ModelDiscovery)
	if err != nil {
		return nil, err
	}
	credentialChannel := *channel
	credentialChannel.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
	key, _, apiErr := credentialChannel.GetNextEnabledKey()
	if apiErr != nil || strings.TrimSpace(key) == "" {
		return nil, errors.New("channel has no available key")
	}
	headers := GetAuthHeader(strings.TrimSpace(key))
	if err := applyFetchModelsHeaderOverrides(channel, key, headers); err != nil {
		return nil, errors.New("invalid channel headers")
	}
	baseClient, err := service.NewProxyHttpClient(settings.Proxy)
	if err != nil {
		return nil, errors.New("invalid channel proxy")
	}
	client := *baseClient
	if client.Timeout <= 0 || client.Timeout > 30*time.Second {
		client.Timeout = 30 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := getTaskPluginModelDiscoveryPage(ctx, &client, requestURL.String(), headers, 1<<20)
	if err != nil {
		return nil, errors.New("Yuanliu model discovery failed")
	}
	var response struct {
		Data *[]struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &response); err != nil || response.Data == nil || len(*response.Data) == 0 {
		return nil, errors.New("invalid Yuanliu model catalog")
	}
	declared := make(map[string]struct{}, len(plugin.Meta.Models))
	for _, id := range plugin.Meta.Models {
		declared[id] = struct{}{}
	}
	names := make(map[string]string)
	seen := make(map[string]struct{}, len(*response.Data))
	for _, item := range *response.Data {
		if item.ID == "" {
			return nil, errors.New("invalid Yuanliu model ID")
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, errors.New("duplicate Yuanliu model ID")
		}
		seen[item.ID] = struct{}{}
		if _, supported := declared[item.ID]; supported {
			names[item.ID] = item.Name
		}
	}
	return names, nil
}

// resolveYuanliuModel 只接受渠道映射最终指向插件精确模型 ID 的名称。
func resolveYuanliuModel(name string, mapping map[string]string, declared map[string]struct{}) string {
	visited := map[string]bool{name: true}
	for {
		next, ok := mapping[name]
		if !ok || next == "" || next == name {
			break
		}
		if visited[next] {
			return ""
		}
		visited[next] = true
		name = next
	}
	if _, ok := declared[name]; ok {
		return name
	}
	return ""
}

// aggregateYuanliuAvailability 将所有可路由渠道的结果合并；失败渠道不使售罄或缺失变成确定结论。
func aggregateYuanliuAvailability(targets map[int]string, snapshots map[int]yuanliuCatalogSnapshot) yuanliuAvailability {
	result := yuanliuAvailability{Status: "unknown"}
	if len(targets) == 0 {
		return result
	}
	unknown, soldOut := false, false
	var oldest time.Time
	for channelID, upstreamID := range targets {
		snapshot := snapshots[channelID]
		if snapshot.checkedAt.IsZero() {
			unknown = true
			continue
		}
		if oldest.IsZero() || snapshot.checkedAt.Before(oldest) {
			oldest = snapshot.checkedAt
		}
		// 多 Key 目录只抽查一个启用密钥，负向结果不能推断其他密钥。
		name, listed := snapshot.names[upstreamID]
		if !listed {
			if snapshot.multipleEnabledKeys {
				unknown = true
			}
			continue
		}
		if strings.TrimSpace(name) == "" {
			unknown = true
			continue
		}
		if strings.Contains(name, "售罄") || strings.Contains(name, "告罄") {
			if snapshot.multipleEnabledKeys {
				unknown = true
				continue
			}
			soldOut = true
			continue
		}
		checkedAt := snapshot.checkedAt.Format(time.RFC3339)
		return yuanliuAvailability{Status: "available", CheckedAt: &checkedAt}
	}
	if unknown {
		return result
	}
	if soldOut {
		result.Status = "sold_out"
	} else {
		result.Status = "disabled"
	}
	checkedAt := oldest.Format(time.RFC3339)
	result.CheckedAt = &checkedAt
	return result
}

// GetYuanliuAvailability 返回 GET /api/pricing/yuanliu-availability 的可见源流模型状态。
// 输入用户身份和角色来自定价页认证中间件，无查询参数；data.models 仅包含可通过启用渠道映射到插件精确 ID 的可见模型。
// status 为 available、sold_out、disabled 或 unknown；售罄仅按目录名称中的“售罄/告罄”判断。
// 多启用密钥渠道的抽样售罄或缺失返回 unknown；available 不保证生成成功。
// checked_at 是成功目录检查的 RFC3339 时间，未知时为 null。
// 数据库读取失败返回 503；上游目录失败或等待超过 8 秒仅使受影响模型为 unknown。
// 请求可能触发带渠道凭据的只读目录 GET，超时刷新会继续填充缓存；响应不含渠道信息或凭据。
func GetYuanliuAvailability(c *gin.Context) {
	plugin, ok := jsplugin.DefaultRegistry.Get("yuanliu")
	models := make(map[string]yuanliuAvailability)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"models": models}})
		return
	}
	group := ""
	if userID, exists := c.Get("id"); exists {
		if user, err := model.GetUserCache(userID.(int)); err == nil {
			group = user.Group
		}
	}
	usableGroups := service.GetUserUsableGroups(group)
	role := c.GetInt("role")
	for name := range usableGroups {
		if !service.IsGroupVisible(name, role) {
			delete(usableGroups, name)
		}
	}
	visible := filterPricingByUsableGroups(model.GetPricing(), usableGroups, role)
	visibleNames := make(map[string]struct{}, len(visible))
	for _, item := range visible {
		visibleNames[item.ModelName] = struct{}{}
	}
	abilities, err := model.GetAllEnableAbilityWithChannels()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Model availability is temporarily unavailable"})
		return
	}
	channelIDs := make(map[int]struct{})
	for _, ability := range abilities {
		if ability.ChannelType != constant.ChannelTypeTaskPlugin {
			continue
		}
		if _, ok := visibleNames[ability.Model]; !ok || !service.IsGroupVisible(ability.Group, role) {
			continue
		}
		if _, ok := usableGroups[ability.Group]; !ok && ability.Group != "all" {
			continue
		}
		channelIDs[ability.ChannelId] = struct{}{}
	}
	ids := make([]int, 0, len(channelIDs))
	for id := range channelIDs {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"models": models}})
		return
	}
	channels, err := model.GetChannelsByIds(ids)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Model availability is temporarily unavailable"})
		return
	}
	byID := make(map[int]*model.Channel, len(channels))
	for _, channel := range channels {
		if channel.Type != constant.ChannelTypeTaskPlugin || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		var settings dto.ChannelSettings
		if channel.Setting == nil || common.UnmarshalJsonStr(*channel.Setting, &settings) != nil || settings.TaskPluginKey != "yuanliu" {
			continue
		}
		byID[channel.Id] = channel
	}
	declared := make(map[string]struct{}, len(plugin.Meta.Models))
	for _, name := range plugin.Meta.Models {
		declared[name] = struct{}{}
	}
	targetsByModel := make(map[string]map[int]string)
	for _, ability := range abilities {
		channel := byID[ability.ChannelId]
		if channel == nil {
			continue
		}
		if _, ok := visibleNames[ability.Model]; !ok || !service.IsGroupVisible(ability.Group, role) {
			continue
		}
		if _, ok := usableGroups[ability.Group]; !ok && ability.Group != "all" {
			continue
		}
		upstreamID := resolveYuanliuModel(ability.Model, normalizeChannelModelMapping(channel), declared)
		if upstreamID == "" {
			continue
		}
		if targetsByModel[ability.Model] == nil {
			targetsByModel[ability.Model] = make(map[int]string)
		}
		targetsByModel[ability.Model][channel.Id] = upstreamID
	}
	snapshots := make(map[int]yuanliuCatalogSnapshot)
	var snapshotsLock sync.Mutex
	var wait sync.WaitGroup
	usedChannels := make(map[int]struct{})
	for _, targets := range targetsByModel {
		for channelID := range targets {
			usedChannels[channelID] = struct{}{}
		}
	}
	requestCtx, cancel := context.WithTimeout(c.Request.Context(), yuanliuAvailabilityWaitTimeout)
	defer cancel()
	work := make(chan int)
	for range min(yuanliuCatalogConcurrency, len(usedChannels)) {
		wait.Go(func() {
			for channelID := range work {
				channel := byID[channelID]
				snapshot := pricingYuanliuCatalogCache.get(requestCtx, channel, plugin.Meta, fetchYuanliuModelNames)
				snapshotsLock.Lock()
				snapshots[channel.Id] = snapshot
				snapshotsLock.Unlock()
			}
		})
	}
queue:
	for channelID := range usedChannels {
		select {
		case work <- channelID:
		case <-requestCtx.Done():
			break queue
		}
	}
	close(work)
	wait.Wait()
	for name, targets := range targetsByModel {
		models[name] = aggregateYuanliuAvailability(targets, snapshots)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"models": models}})
}
