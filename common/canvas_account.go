package common

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

const (
	// CanvasAccountScopeIdentityRead 允许 Canvas 读取当前账号的稳定身份。
	CanvasAccountScopeIdentityRead = "identity:read"
	// CanvasAccountScopeGroupsRead 允许 Canvas 读取当前账号的可用分组。
	CanvasAccountScopeGroupsRead = "groups:read"
	// CanvasAccountScopeTokensManage 允许 Canvas 创建或恢复本实例的管理 Token。
	CanvasAccountScopeTokensManage = "tokens:manage"
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
