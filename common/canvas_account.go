package common

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode"

	"github.com/tidwall/gjson"
)

const (
	// CanvasAccountScopeIdentityRead 允许 Canvas 读取当前账号的稳定身份。
	CanvasAccountScopeIdentityRead = "identity:read"
	// CanvasAccountScopeGroupsRead 允许 Canvas 读取当前账号的可用分组。
	CanvasAccountScopeGroupsRead = "groups:read"
	// CanvasAccountScopeTokensManage 允许 Canvas 创建或恢复本实例的管理 Token。
	CanvasAccountScopeTokensManage         = "tokens:manage"
	canvasAccountAdditionalClientsMaxSize  = 64 * 1024
	canvasAccountAdditionalClientsMaxCount = 32
	canvasAccountRedirectURIMaxLength      = 2048
	canvasAccountClientSecretMaxLength     = 4096
)

var errCanvasAccountConfiguration = errors.New("canvas account configuration is invalid")

// CanvasAccountConfig 保存单个受信 Canvas 实例的固定 OAuth 风格客户端配置。
// ClientSecret 为空时按公开 PKCE 客户端处理；其余字段必须精确匹配请求。
type CanvasAccountConfig struct {
	Issuer       string
	ClientID     string
	InstanceID   string
	RedirectURI  string
	ClientSecret string
}

// CanvasAccountEnabled 判断是否开放 Canvas 账号接入端点；默认关闭。
func CanvasAccountEnabled() bool {
	return GetEnvOrDefaultBool("CANVAS_ACCOUNT_ENABLED", false)
}

// GetCanvasAccountConfig 读取并校验固定 issuer、客户端、实例与回调地址。
//
// 返回错误表示部署配置不完整，或 URL 不是安全的无凭据、无查询参数的绝对地址。
// 非回环地址必须使用 HTTPS，避免授权码和 grant 经明文链路传输。
func GetCanvasAccountConfig() (CanvasAccountConfig, error) {
	config := CanvasAccountConfig{
		Issuer:       strings.TrimRight(strings.TrimSpace(GetEnvOrDefaultString("CANVAS_ACCOUNT_ISSUER", "")), "/"),
		ClientID:     strings.TrimSpace(GetEnvOrDefaultString("CANVAS_ACCOUNT_CLIENT_ID", "")),
		InstanceID:   strings.TrimSpace(GetEnvOrDefaultString("CANVAS_ACCOUNT_INSTANCE_ID", "")),
		RedirectURI:  strings.TrimSpace(GetEnvOrDefaultString("CANVAS_ACCOUNT_REDIRECT_URI", "")),
		ClientSecret: GetEnvOrDefaultString("CANVAS_ACCOUNT_CLIENT_SECRET", ""),
	}
	if config.ClientID == "" || len(config.ClientID) > 64 || config.InstanceID == "" || len(config.InstanceID) > 128 {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	issuer, err := url.Parse(config.Issuer)
	if err != nil || !canvasAccountAbsoluteURL(issuer) || (issuer.Path != "" && issuer.Path != "/") {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	redirect, err := url.Parse(config.RedirectURI)
	if err != nil || !canvasAccountAbsoluteURL(redirect) {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	return config, nil
}

// GetCanvasAccountConfigs 读取主 Canvas 客户端及同一 issuer 下的附加客户端。
//
// 附加客户端来自 CANVAS_ACCOUNT_ADDITIONAL_CLIENTS，必须是最多 32 项的严格 JSON 数组。
// 身份与回调字段缺失、类型错误、重复或不安全时，整组配置失效；secret 保持原始字节语义。
func GetCanvasAccountConfigs() ([]CanvasAccountConfig, error) {
	primary, err := GetCanvasAccountConfig()
	if err != nil {
		return nil, err
	}

	raw := GetEnvOrDefaultString("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", "")
	if raw == "" {
		return []CanvasAccountConfig{primary}, nil
	}
	if len(raw) > canvasAccountAdditionalClientsMaxSize || !gjson.Valid(raw) {
		return nil, errCanvasAccountConfiguration
	}

	additional := gjson.Parse(raw)
	if !additional.IsArray() {
		return nil, errCanvasAccountConfiguration
	}
	entries := additional.Array()
	if len(entries) > canvasAccountAdditionalClientsMaxCount {
		return nil, errCanvasAccountConfiguration
	}

	configs := make([]CanvasAccountConfig, 0, len(entries)+1)
	configs = append(configs, primary)
	type clientPair struct {
		clientID   string
		instanceID string
	}
	pairs := map[clientPair]struct{}{{clientID: primary.ClientID, instanceID: primary.InstanceID}: {}}
	for _, entry := range entries {
		config, err := parseCanvasAccountAdditionalConfig(primary.Issuer, entry)
		if err != nil {
			return nil, errCanvasAccountConfiguration
		}
		pair := clientPair{clientID: config.ClientID, instanceID: config.InstanceID}
		if _, exists := pairs[pair]; exists {
			return nil, errCanvasAccountConfiguration
		}
		pairs[pair] = struct{}{}
		configs = append(configs, config)
	}
	return configs, nil
}

// FindCanvasAccountConfig 按未经规范化的 client_id 与 instance_id 精确查找配置。
// 返回 false 表示不存在完全相同的客户端与实例组合。
func FindCanvasAccountConfig(configs []CanvasAccountConfig, clientID, instanceID string) (CanvasAccountConfig, bool) {
	for _, config := range configs {
		if config.ClientID == clientID && config.InstanceID == instanceID {
			return config, true
		}
	}
	return CanvasAccountConfig{}, false
}

// CanvasAccountScopes 返回固定授权范围的新切片，调用方可安全修改。
func CanvasAccountScopes() []string {
	return []string{
		CanvasAccountScopeIdentityRead,
		CanvasAccountScopeGroupsRead,
		CanvasAccountScopeTokensManage,
	}
}

// canvasAccountAbsoluteURL 限定无凭据、查询和片段的 HTTPS 地址；本机回环允许 HTTP。
func canvasAccountAbsoluteURL(value *url.URL) bool {
	if value == nil || value.Hostname() == "" || value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return false
	}
	return value.Scheme == "https" || (value.Scheme == "http" &&
		(strings.EqualFold(value.Hostname(), "localhost") || net.ParseIP(value.Hostname()).IsLoopback()))
}

// parseCanvasAccountAdditionalConfig 校验一个附加客户端对象并继承主配置 issuer。
func parseCanvasAccountAdditionalConfig(issuer string, entry gjson.Result) (CanvasAccountConfig, error) {
	if !entry.IsObject() {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}

	config := CanvasAccountConfig{Issuer: issuer}
	seen := make(map[string]struct{}, 4)
	valid := true
	entry.ForEach(func(key, value gjson.Result) bool {
		name := key.String()
		if _, exists := seen[name]; exists || value.Type != gjson.String {
			valid = false
			return false
		}
		seen[name] = struct{}{}
		switch name {
		case "client_id":
			config.ClientID = value.String()
		case "instance_id":
			config.InstanceID = value.String()
		case "redirect_uri":
			config.RedirectURI = value.String()
		case "client_secret":
			config.ClientSecret = value.String()
		default:
			valid = false
			return false
		}
		return true
	})
	if !valid || len(seen) < 3 {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	if _, exists := seen["client_id"]; !exists {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	if _, exists := seen["instance_id"]; !exists {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	if _, exists := seen["redirect_uri"]; !exists {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	if !canvasAccountAdditionalValue(config.ClientID, 64, true) ||
		!canvasAccountAdditionalValue(config.InstanceID, 128, true) ||
		!canvasAccountAdditionalValue(config.RedirectURI, canvasAccountRedirectURIMaxLength, true) ||
		len(config.ClientSecret) > canvasAccountClientSecretMaxLength {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	redirect, err := url.Parse(config.RedirectURI)
	if err != nil || !canvasAccountAbsoluteURL(redirect) {
		return CanvasAccountConfig{}, errCanvasAccountConfiguration
	}
	return config, nil
}

// canvasAccountAdditionalValue 限制附加配置值的边界与不可见字符。
func canvasAccountAdditionalValue(value string, maxLength int, required bool) bool {
	if (required && value == "") || len(value) > maxLength || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return !strings.Contains(value, "*")
}
