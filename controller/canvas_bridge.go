package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// canvasCatalogModel 只公开当前 Key 可访问的精确模型 ID、已确认调用合同和价格版本。
type canvasCatalogModel struct {
	ID                string         `json:"id"`
	Name              string         `json:"name,omitempty"`
	Description       string         `json:"description,omitempty"`
	MediaType         string         `json:"media_type,omitempty"`
	Contract          string         `json:"contract,omitempty"`
	Capabilities      map[string]any `json:"capabilities,omitempty"`
	Limitations       map[string]any `json:"limitations,omitempty"`
	InputMediaTypes   []string       `json:"input_media_types,omitempty"`
	Available         bool           `json:"available"`
	UnavailableReason string         `json:"unavailable_reason,omitempty"`
	PricingVersion    string         `json:"pricing_version"`
}

// canvasEstimateRequest 仅接受调用合同和标量参数；不接收任意 URL、凭据或计费表达式。
type canvasEstimateRequest struct {
	Model        string         `json:"model"`
	Contract     string         `json:"contract"`
	Parameters   map[string]any `json:"parameters"`
	InputText    string         `json:"input_text"`
	InputPending bool           `json:"input_pending"`
}

// canvasCandidate 保存授权范围内的一条路由；Channel 从数据库读取时排除渠道凭据。
type canvasCandidate struct {
	Group         string
	Channel       *model.Channel
	OtherSettings dto.ChannelOtherSettings
	Plugin        *jsplugin.LoadedPlugin
	Generation    *jsplugin.RoutingGeneration
	UpstreamModel string
}

// canvasProfile 表示明确端点对应的 Canvas 合同；不通过模型名称推断媒体能力。
type canvasProfile struct {
	MediaType string
	Contract  string
	Path      string
}

// canvasEstimateFailure 区分可修正的参数错误和价格配置错误，消息不包含表达式或渠道配置。
type canvasEstimateFailure struct {
	Status  int
	Code    string
	Message string
}

// Error 返回面向调用方的有界错误说明。
func (failure *canvasEstimateFailure) Error() string { return failure.Message }

// canvasProfiles 是宿主已实现且 Canvas 可直接调用的固定路径，不允许自定义请求地址。
var canvasProfiles = []canvasProfile{
	{MediaType: "text", Contract: "openai-chat-completions", Path: "/v1/chat/completions"},
	{MediaType: "image", Contract: "openai-images", Path: "/v1/images/generations"},
	{MediaType: "video", Contract: "newapi-video-v1", Path: "/v1/videos"},
	{MediaType: "audio", Contract: "openai-audio", Path: "/v1/audio/speech"},
}

// canvasBridgeError 返回稳定错误码，避免将数据库、插件或价格表达式内容传给调用方。
func canvasBridgeError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

// canvasBillingUnits 校验站点换算设置；额度单位和美元兑人民币汇率均以十进制字符串输出。
func canvasBillingUnits() (string, string, error) {
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) ||
		operation_setting.USDExchangeRate <= 0 || math.IsNaN(operation_setting.USDExchangeRate) || math.IsInf(operation_setting.USDExchangeRate, 0) {
		return "", "", errors.New("invalid billing conversion settings")
	}
	return strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64), strconv.FormatFloat(operation_setting.USDExchangeRate, 'f', -1, 64), nil
}

// canvasAuthorizedModels 读取当前用户组、Key 分组和模型限制的交集；查询失败时拒绝返回部分目录。
func canvasAuthorizedModels(c *gin.Context) (modelListGroups, map[string][]canvasCandidate, map[string]model.Model, error) {
	groups, err := getModelListGroups(c)
	if err != nil {
		return groups, nil, nil, err
	}
	byModel := make(map[string][]canvasCandidate)
	metadata := make(map[string]model.Model)
	if len(groups.ownerGroups) == 0 {
		return groups, byModel, metadata, nil
	}
	var abilities []model.Ability
	if err := model.DB.Where(map[string]any{"group": groups.ownerGroups, "enabled": true}).Order("channel_id").Find(&abilities).Error; err != nil {
		return groups, nil, nil, err
	}
	limitEnabled := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	limitValue, _ := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
	limits, _ := limitValue.(map[string]bool)
	pin, pinned, _ := service.GetChannelConstraints(c).ResolvedPin()
	channelIDs := make([]int, 0)
	filtered := abilities[:0]
	for _, ability := range abilities {
		if pinned && ability.ChannelId != pin.ChannelId {
			continue
		}
		if limitEnabled && !limits[ability.Model] && !limits[ratio_setting.RoutingMatchModelName(ability.Model)] {
			continue
		}
		filtered = append(filtered, ability)
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	if len(channelIDs) == 0 {
		return groups, byModel, metadata, nil
	}
	var channels []model.Channel
	if err := model.DB.Select("id", "type", "status", "model_mapping", "setting", "settings").Where("id IN ? AND status = ?", channelIDs, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return groups, nil, nil, err
	}
	byChannel := make(map[int]*model.Channel, len(channels))
	for i := range channels {
		byChannel[channels[i].Id] = &channels[i]
	}
	generation := jsplugin.DefaultRegistry.Generation()
	for _, group := range groups.ownerGroups {
		for _, ability := range filtered {
			channel := byChannel[ability.ChannelId]
			if ability.Group != group || channel == nil {
				continue
			}
			candidate := canvasCandidate{Group: group, Channel: channel, UpstreamModel: ability.Model, Generation: generation}
			var channelSetting dto.ChannelSettings
			if channel.Setting != nil && *channel.Setting != "" {
				if err := common.UnmarshalJsonStr(*channel.Setting, &channelSetting); err != nil {
					return groups, nil, nil, errors.New("invalid channel settings")
				}
			}
			if channel.OtherSettings != "" {
				if err := common.UnmarshalJsonStr(channel.OtherSettings, &candidate.OtherSettings); err != nil {
					return groups, nil, nil, errors.New("invalid channel settings")
				}
			}
			if channel.ModelMapping != nil && *channel.ModelMapping != "" {
				mappedContext := c.Copy()
				mappedContext.Set("model_mapping", *channel.ModelMapping)
				mappedInfo := &relaycommon.RelayInfo{OriginModelName: ability.Model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ability.Model}}
				if err := helper.ModelMappedHelper(mappedContext, mappedInfo, nil); err != nil {
					return groups, nil, nil, errors.New("invalid channel model mapping")
				}
				candidate.UpstreamModel = mappedInfo.UpstreamModelName
			}
			if channel.Type == constant.ChannelTypeTaskPlugin {
				candidate.Plugin, _ = generation.Get(channelSetting.TaskPluginKey)
			} else {
				candidate.Plugin, _ = generation.GetByChannelType(channel.Type)
				if candidate.Plugin != nil && !slices.Contains(candidate.Plugin.Meta.Models, candidate.UpstreamModel) {
					candidate.Plugin = nil
				}
			}
			byModel[ability.Model] = append(byModel[ability.Model], candidate)
		}
	}
	names := make([]string, 0, len(byModel))
	for name := range byModel {
		names = append(names, name)
	}
	if len(names) != 0 {
		var records []model.Model
		if err := model.DB.Where("model_name IN ? AND name_rule = ?", names, model.NameRuleExact).Find(&records).Error; err != nil {
			return groups, nil, nil, err
		}
		for _, record := range records {
			metadata[record.ModelName] = record
		}
	}
	return groups, byModel, metadata, nil
}

// supports 判断实际可用渠道是否接受固定合同；插件必须精确声明模型和视频协议。
func (candidate canvasCandidate) supports(name string, profile canvasProfile) bool {
	if candidate.Channel.Type == constant.ChannelTypeAdvancedCustom {
		config := candidate.OtherSettings.AdvancedCustom
		if config == nil || !config.SupportsPathForModel(profile.Path, name) {
			return false
		}
	}
	if candidate.Plugin == nil {
		return candidate.Channel.Type != constant.ChannelTypeTaskPlugin && profile.MediaType != "video"
	}
	if profile.Contract != "newapi-video-v1" || !slices.Contains(candidate.Plugin.Meta.Models, candidate.UpstreamModel) {
		return false
	}
	// 真实视频入口先固定全局模型别名；仅检查当前渠道映射会误开放跨渠道冲突的别名。
	lookupModel := name
	if declared, ok := candidate.Generation.CanonicalModel(name); ok {
		lookupModel = declared
	} else if target, ok := model.ResolveTaskModelAlias(candidate.Generation, name); ok {
		if target.Declared == "" {
			return false
		}
		lookupModel = target.Declared
	}
	return slices.ContainsFunc(candidate.Generation.LookupEndpointCandidates(http.MethodPost, profile.Path, lookupModel), func(binding jsplugin.ProtocolBinding) bool {
		return binding.Plugin == candidate.Plugin && binding.Protocol == "openai_video"
	})
}

// canvasDeclaredProfiles 只解析管理员明确填写的标准端点；歧义端点保持待确认状态。
func canvasDeclaredProfiles(metadata model.Model, candidates []canvasCandidate, name string) []canvasProfile {
	paths := make(map[string]bool)
	var endpoints map[string]any
	if common.UnmarshalJsonStr(metadata.Endpoints, &endpoints) == nil {
		for _, value := range endpoints {
			switch endpoint := value.(type) {
			case string:
				paths[endpoint] = true
			case map[string]any:
				method, _ := endpoint["method"].(string)
				path, _ := endpoint["path"].(string)
				if method == "" || strings.EqualFold(method, http.MethodPost) {
					paths[path] = true
				}
			}
		}
	}
	profiles := make([]canvasProfile, 0)
	for _, profile := range canvasProfiles {
		for _, candidate := range candidates {
			if candidate.supports(name, profile) && (paths[profile.Path] || profile.Contract == "newapi-video-v1") {
				profiles = append(profiles, profile)
				break
			}
		}
	}
	return profiles
}

// canvasPricingConfigurationVersion 使用运行中价格配置的内容哈希；表达式仅参与哈希，不向 Canvas 公开。
func canvasPricingConfigurationVersion() string {
	return model.ModelPricingVersion(model.PricingValues{
		"mode": billing_setting.GetBillingModeCopy(), "expression": billing_setting.GetBillingExprCopy(),
		"plugin_expression": billing_setting.GetPluginBillingExprCopy(), "price": ratio_setting.GetModelPriceCopy(),
		"ratio": ratio_setting.GetModelRatioCopy(), "completion": ratio_setting.GetCompletionRatioCopy(),
		"image": ratio_setting.GetImageRatioCopy(), "audio": ratio_setting.GetAudioRatioCopy(),
		"audio_completion": ratio_setting.GetAudioCompletionRatioCopy(), "cache": ratio_setting.GetCacheRatioCopy(),
		"create_cache": ratio_setting.GetCreateCacheRatioCopy(), "quota_per_unit": common.QuotaPerUnit,
		"usd_to_cny": operation_setting.USDExchangeRate, "preconsume": common.PreConsumedQuota,
	})
}

// canvasModelPricingVersion 将可用渠道、插件版本和用户组倍率纳入版本，价格变化后旧估算自然失效。
func canvasModelPricingVersion(base, name, userGroup string, metadata model.Model, candidates []canvasCandidate) string {
	routes := make([]any, 0, len(candidates))
	for _, candidate := range candidates {
		ratio, special := ratio_setting.GetGroupGroupRatio(userGroup, candidate.Group)
		if !special {
			ratio = ratio_setting.GetGroupRatio(candidate.Group)
		}
		route := map[string]any{"group": candidate.Group, "ratio": ratio, "channel": candidate.Channel.Id, "type": candidate.Channel.Type, "upstream_model": candidate.UpstreamModel}
		if candidate.Plugin != nil {
			route["plugin"] = candidate.Plugin.Meta.Key
			route["version"] = candidate.Plugin.Meta.Version
		}
		routes = append(routes, route)
	}
	return model.ModelPricingVersion(model.PricingValues{"configuration": base, "model": name, "endpoints": metadata.Endpoints, "routes": routes})
}

// priced 判断该路由是否具有宿主可执行的计费配置，不把自用模式的隐式默认值当成已定价。
func (candidate canvasCandidate) priced(name string) bool {
	if candidate.Plugin != nil {
		if expression, exists := billing_setting.ResolveTaskBillingExpr(candidate.Plugin.Meta.Key, name, candidate.UpstreamModel); exists {
			schema, _ := candidate.Plugin.Meta.UsageForModel(candidate.UpstreamModel)
			return billing_setting.TaskExprCompatible(expression, schema)
		}
	} else {
		name = helper.ResolveBillingModelName(name)
	}
	if _, configured := ratio_setting.GetModelPrice(name, false); configured || ratio_setting.HasConfiguredModelRatio(name) {
		return true
	}
	expression, exists := billing_setting.GetBillingExpr(name)
	return billing_setting.GetBillingMode(name) == billing_setting.BillingModeTieredExpr && exists && strings.TrimSpace(expression) != ""
}

// GetCanvasCatalog 返回 Key 授权目录、明确调用合同和人民币换算信息；无钱包或上游请求副作用。
func GetCanvasCatalog(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !common.CanvasBridgeEnabled() {
		canvasBridgeError(c, http.StatusNotFound, "canvas_bridge_disabled", "Canvas 计费桥接尚未启用")
		return
	}
	quotaUnit, exchangeRate, err := canvasBillingUnits()
	if err != nil {
		canvasBridgeError(c, http.StatusServiceUnavailable, "invalid_billing_settings", "额度单位或人民币汇率配置无效，请联系管理员")
		return
	}
	groups, byModel, metadata, err := canvasAuthorizedModels(c)
	if err != nil {
		canvasBridgeError(c, http.StatusServiceUnavailable, "catalog_unavailable", "模型目录暂时不可用，请检查渠道配置后重试")
		return
	}
	names := make([]string, 0, len(byModel))
	for name := range byModel {
		names = append(names, name)
	}
	slices.Sort(names)
	baseVersion := canvasPricingConfigurationVersion()
	models := make([]canvasCatalogModel, 0, len(names))
	for _, name := range names {
		meta := metadata[name]
		candidates := byModel[name]
		item := canvasCatalogModel{ID: name, Name: name, Description: meta.Description, PricingVersion: canvasModelPricingVersion(baseVersion, name, groups.userGroup, meta, candidates)}
		profiles := canvasDeclaredProfiles(meta, candidates, name)
		if !canvasModelIDValid(name) {
			item.UnavailableReason = "invalid_model_id"
		} else if meta.Id != 0 && meta.Status != 1 {
			item.UnavailableReason = "model_disabled"
		} else if len(profiles) != 1 {
			item.UnavailableReason = "missing_profile"
		} else {
			profile := profiles[0]
			item.MediaType, item.Contract = profile.MediaType, profile.Contract
			item.InputMediaTypes = []string{"text"}
			item.Capabilities = map[string]any{"mediaTypes": []string{profile.MediaType}, "mentionMediaTypes": []string{"text"}}
			item.Available = slices.ContainsFunc(candidates, func(candidate canvasCandidate) bool {
				return candidate.supports(name, profile) && candidate.priced(name)
			})
			if !item.Available {
				item.UnavailableReason = "missing_pricing"
			}
		}
		models = append(models, item)
	}
	c.JSON(http.StatusOK, gin.H{"version": 1, "currency": "CNY", "quota_per_unit": quotaUnit, "usd_to_cny": exchangeRate, "models": models})
}

// canvasModelIDValid 与回执持久化一致限制为 512 UTF-8 字节，不截断或改写模型身份。
func canvasModelIDValid(name string) bool {
	return name != "" && len(name) <= 512 && strings.TrimSpace(name) == name &&
		!strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// canvasEstimateBody 限制 JSON 大小和字段，保留模型 ID 原文，未知输入仅用于明确标识的估算。
func canvasEstimateBody(c *gin.Context) (canvasEstimateRequest, canvasProfile, error) {
	var request canvasEstimateRequest
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return request, canvasProfile{}, errors.New("Content-Type 必须为 application/json")
	}
	const maxEstimateBytes = 256 * 1024
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxEstimateBytes+1))
	if err != nil || len(data) > maxEstimateBytes {
		return request, canvasProfile{}, errors.New("预估请求读取失败或超过 256 KiB")
	}
	var fields map[string]any
	if err := common.Unmarshal(data, &fields); err != nil || fields == nil {
		return request, canvasProfile{}, errors.New("预估请求必须是有效 JSON 对象")
	}
	for field := range fields {
		if !slices.Contains([]string{"model", "contract", "parameters", "input_text", "input_pending"}, field) {
			return request, canvasProfile{}, errors.New("预估请求包含不支持的字段")
		}
	}
	if err := common.Unmarshal(data, &request); err != nil || !canvasModelIDValid(request.Model) {
		return request, canvasProfile{}, errors.New("模型 ID 或预估字段类型无效")
	}
	for _, profile := range canvasProfiles {
		if profile.Contract == request.Contract {
			return request, profile, nil
		}
	}
	return request, canvasProfile{}, errors.New("该 Canvas 调用合同暂不支持价格预估")
}

// canvasCanonicalBody 校验合同的标量参数白名单；数量和时长在进入原计费引擎前限制范围。
func canvasCanonicalBody(request canvasEstimateRequest, profile canvasProfile) (map[string]any, error) {
	allowed := map[string][]string{
		"text":  {"max_tokens", "max_completion_tokens", "temperature", "top_p", "reasoning_effort", "seed"},
		"image": {"n", "size", "quality", "aspect_ratio", "style", "background", "output_format", "watermark"},
		"video": {"seconds", "duration", "size", "resolution", "aspect_ratio", "quality", "seed", "generate_audio", "audio", "watermark", "negative_prompt"},
		"audio": {"voice", "speed", "response_format", "instructions"},
	}
	body := map[string]any{"model": request.Model}
	for key, value := range request.Parameters {
		if !slices.Contains(allowed[profile.MediaType], key) {
			return nil, errors.New("当前调用合同不支持所提供的参数，请移除未声明参数")
		}
		switch typed := value.(type) {
		case string:
			if len(typed) > 4096 {
				return nil, errors.New("单个参数字符串不能超过 4096 字节")
			}
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) {
				return nil, errors.New("数值参数必须为有限数")
			}
		case bool:
		default:
			return nil, errors.New("参数只接受字符串、数字或布尔值，不接受对象、数组和 null")
		}
		switch key {
		case "max_tokens", "max_completion_tokens", "seed", "temperature", "top_p", "speed":
			number, ok := value.(float64)
			if !ok || (slices.Contains([]string{"max_tokens", "max_completion_tokens", "seed"}, key) && math.Trunc(number) != number) {
				return nil, fmt.Errorf("%s 的数值类型无效", key)
			}
		case "generate_audio", "audio", "watermark":
			if _, ok := value.(bool); !ok {
				return nil, fmt.Errorf("%s 必须为布尔值", key)
			}
		case "seconds", "duration", "n":
		default:
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("%s 必须为字符串", key)
			}
		}
		if key == "seconds" || key == "duration" || key == "n" {
			number, ok := value.(float64)
			if text, isString := value.(string); isString && key != "n" {
				var err error
				number, err = strconv.ParseFloat(text, 64)
				ok = err == nil
			}
			limit := float64(relaycommon.MaxTaskDurationSeconds)
			if key == "n" {
				limit = dto.MaxImageN
			}
			if !ok || math.IsNaN(number) || number < 1 || number > limit || math.Trunc(number) != number {
				return nil, fmt.Errorf("%s 必须是 1 到 %.0f 之间的整数", key, limit)
			}
		}
		body[key] = value
	}
	switch profile.MediaType {
	case "text":
		body["messages"] = []any{map[string]any{"role": "user", "content": request.InputText}}
	case "audio":
		body["input"] = request.InputText
	default:
		body["prompt"] = request.InputText
	}
	return body, nil
}

// estimateQuota 复用原请求校验和预估计费引擎；不预扣、不创建任务、不构造上游请求。
func (candidate canvasCandidate) estimateQuota(c *gin.Context, groups modelListGroups, request canvasEstimateRequest, profile canvasProfile, body map[string]any) (int, error) {
	encoded, err := common.Marshal(body)
	if err != nil {
		return 0, err
	}
	estimateContext := c.Copy()
	delete(estimateContext.Keys, common.KeyBodyStorage)
	delete(estimateContext.Keys, common.KeyRequestBody)
	delete(estimateContext.Keys, "auto_group")
	estimateContext.Request, err = http.NewRequestWithContext(c.Request.Context(), http.MethodPost, profile.Path, bytes.NewReader(encoded))
	if err != nil {
		return 0, err
	}
	estimateContext.Request.Header.Set("Content-Type", "application/json")
	estimateContext.Set(string(constant.ContextKeyChannelType), candidate.Channel.Type)
	if candidate.Channel.ModelMapping != nil {
		estimateContext.Set("model_mapping", *candidate.Channel.ModelMapping)
	}
	defer common.CleanupBodyStorage(estimateContext)
	info := &relaycommon.RelayInfo{UserId: c.GetInt("id"), TokenId: c.GetInt("token_id"), UserGroup: groups.userGroup, UsingGroup: candidate.Group,
		OriginModelName: request.Model, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: candidate.Channel.Type, ChannelId: candidate.Channel.Id, UpstreamModelName: candidate.UpstreamModel},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	if profile.MediaType == "video" {
		plugin := candidate.Plugin
		route := jsplugin.RouteRequestContext{Path: profile.Path, Method: http.MethodPost, Body: map[string]any{"kind": "json", "value": body}, RequestBody: body}
		decoded, err := plugin.Engine.CallPath(c.Request.Context(), "protocols", []string{"openai_video", "decodeRequest"}, jsplugin.ProtocolRequestContext{
			RouteRequestContext: route, Protocol: "openai_video", Operation: "create", Model: request.Model, UpstreamModel: candidate.UpstreamModel,
		}.JSValue())
		if err != nil {
			return 0, &canvasEstimateFailure{http.StatusUnprocessableEntity, "plugin_parameters_rejected", "当前视频插件不接受这些参数或缺少输入，请检查时长、尺寸和生成输入"}
		}
		resolved, ok := decoded.(map[string]any)
		if !ok || resolved["kind"] != "submit" || resolved["model"] != request.Model {
			return 0, &canvasEstimateFailure{http.StatusUnprocessableEntity, "plugin_profile_invalid", "视频插件返回的调用合同无效，请联系管理员检查插件"}
		}
		if normalized, exists := resolved["requestBody"]; exists {
			route.RequestBody = normalized
		}
		info.Action, _ = resolved["action"].(string)
		estimateContext.Set(jsplugin.ContextKeyRouteRequest, route)
		estimateContext.Set("task_request", route.RequestBody)
		adaptor := taskplugin.New(plugin)
		adaptor.Init(info)
		// 不提供渠道密钥；OAuth 插件会在凭据解析阶段退出，预估不会申请令牌。
		expression, exists := billing_setting.ResolveTaskBillingExpr(plugin.Meta.Key, request.Model, candidate.UpstreamModel)
		if exists || billing_setting.GetBillingMode(request.Model) == billing_setting.BillingModeTieredExpr {
			schema, _ := plugin.Meta.UsageForModel(candidate.UpstreamModel)
			if !exists || !billing_setting.TaskExprCompatible(expression, schema) {
				return 0, &canvasEstimateFailure{http.StatusUnprocessableEntity, "missing_pricing", "当前插件缺少可执行的价格配置，请联系管理员"}
			}
			facts, err := adaptor.ExtractUsageFactsValidated(estimateContext, info)
			if err != nil {
				return 0, &canvasEstimateFailure{http.StatusUnprocessableEntity, "plugin_usage_invalid", "视频插件无法计算有效用量，请检查生成参数和输入"}
			}
			cost, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
			if err != nil || cost < 0 {
				return 0, &canvasEstimateFailure{http.StatusUnprocessableEntity, "pricing_evaluation_failed", "当前参数无法计算价格，请联系管理员检查计费配置"}
			}
			return common.QuotaRoundStrict(cost * common.QuotaPerUnit * helper.HandleGroupRatio(estimateContext, info).GroupRatio)
		}
		price, err := helper.ModelPriceHelperPerCall(estimateContext, info)
		if err != nil {
			return 0, err
		}
		ratios, err := adaptor.EstimateBillingValidated(estimateContext, info)
		if err != nil {
			return 0, err
		}
		for key, ratio := range ratios {
			price.AddOtherRatio(key, ratio)
		}
		if slices.Contains(constant.TaskPricePatches, request.Model) {
			return price.Quota, nil
		}
		return common.QuotaFromFloatStrict(price.ApplyOtherRatiosToFloat(float64(price.Quota)))
	}
	var relayRequest dto.Request
	switch profile.MediaType {
	case "text":
		relayRequest, err = helper.GetAndValidateTextRequest(estimateContext, relayconstant.RelayModeChatCompletions)
	case "image":
		relayRequest, err = helper.GetAndValidOpenAIImageRequest(estimateContext, relayconstant.RelayModeImagesGenerations)
	case "audio":
		relayRequest, err = helper.GetAndValidAudioRequest(estimateContext, relayconstant.RelayModeAudioSpeech)
	}
	if err != nil {
		return 0, &canvasEstimateFailure{http.StatusBadRequest, "invalid_parameters", "参数未通过原生接口校验，请检查生成数量、最大输出 token 和参数类型"}
	}
	info.Request = relayRequest
	input, err := helper.BuildBillingExprRequestInputFromRequest(relayRequest, nil)
	if err != nil {
		return 0, err
	}
	// 真实中继冻结的是入站 body；Images DTO 的重新序列化会省略 aspect_ratio 等扩展字段。
	input.Body = encoded
	info.BillingRequestInput = &input
	meta := relayRequest.GetTokenCountMeta()
	promptTokens := service.EstimateTokenByModel(request.Model, request.InputText)
	if profile.MediaType == "audio" && meta.TokenType == "text_number" {
		promptTokens = len([]rune(request.InputText))
	}
	price, err := helper.ModelPriceHelper(estimateContext, info, promptTokens, meta)
	return price.QuotaToPreConsume, err
}

// EstimateCanvasPrice 使用当前 Key 可用路由的原计费引擎预估，返回其中最高估算；这不是最大费用保证或锁价。
func EstimateCanvasPrice(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !common.CanvasBridgeEnabled() {
		canvasBridgeError(c, http.StatusNotFound, "canvas_bridge_disabled", "Canvas 计费桥接尚未启用")
		return
	}
	request, profile, err := canvasEstimateBody(c)
	if err != nil {
		canvasBridgeError(c, http.StatusBadRequest, "invalid_estimate", err.Error())
		return
	}
	body, err := canvasCanonicalBody(request, profile)
	if err != nil {
		canvasBridgeError(c, http.StatusBadRequest, "invalid_estimate", err.Error())
		return
	}
	quotaUnit, exchangeRate, err := canvasBillingUnits()
	if err != nil {
		canvasBridgeError(c, http.StatusServiceUnavailable, "invalid_billing_settings", "额度单位或人民币汇率配置无效，请联系管理员")
		return
	}
	groups, byModel, metadata, err := canvasAuthorizedModels(c)
	if err != nil {
		canvasBridgeError(c, http.StatusServiceUnavailable, "catalog_unavailable", "模型目录暂时不可用，请检查渠道配置后重试")
		return
	}
	candidates := byModel[request.Model]
	meta := metadata[request.Model]
	if len(candidates) == 0 || (meta.Id != 0 && meta.Status != 1) {
		canvasBridgeError(c, http.StatusForbidden, "model_not_allowed", "当前 Key 无权使用该模型或模型已停用")
		return
	}
	estimatedQuota, selectedGroup := -1, ""
	failure := &canvasEstimateFailure{http.StatusUnprocessableEntity, "estimate_unavailable", "当前 Key 下没有同时支持该调用合同和价格的渠道，请检查模型授权、合同与上游定价"}
	for _, candidate := range candidates {
		if !candidate.supports(request.Model, profile) || !candidate.priced(request.Model) {
			continue
		}
		quota, err := candidate.estimateQuota(c, groups, request, profile, body)
		if err != nil {
			var estimateFailure *canvasEstimateFailure
			var quotaClamp *common.QuotaClamp
			if errors.As(err, &estimateFailure) {
				failure = estimateFailure
			} else if errors.As(err, &quotaClamp) {
				failure = &canvasEstimateFailure{http.StatusBadRequest, "quota_out_of_range", "本次预估额度超过宿主安全范围，请减少生成数量或时长"}
			} else {
				failure = &canvasEstimateFailure{http.StatusUnprocessableEntity, "pricing_evaluation_failed", "当前参数无法计算价格，请联系管理员检查计费配置"}
			}
			continue
		}
		if quota > estimatedQuota {
			estimatedQuota, selectedGroup = quota, candidate.Group
		}
	}
	if estimatedQuota < 0 {
		canvasBridgeError(c, failure.Status, failure.Code, failure.Message)
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": 1, "model": request.Model, "group": selectedGroup,
		"pricing_version": canvasModelPricingVersion(canvasPricingConfigurationVersion(), request.Model, groups.userGroup, meta, candidates),
		"estimated_quota": strconv.Itoa(estimatedQuota), "quota_per_unit": quotaUnit, "usd_to_cny": exchangeRate,
		"expires_at": time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339), "estimate_only": true})
}
