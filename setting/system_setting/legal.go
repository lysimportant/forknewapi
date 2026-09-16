package system_setting

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// CurrentLegalConsentVersion 是《API 服务、隐私与使用责任协议》当前生效的版本号。
//
// 登录与注册流程必须携带该版本号，服务端据此拒绝缺失、未同意和过期版本的请求。
// 修订协议正文时必须同步递增版本号，否则已经确认旧版正文的客户端会被错误放行。
const CurrentLegalConsentVersion = "2026-09-16"

var (
	// ErrLegalConsentRequired 表示请求缺少协议同意标记或版本号。
	ErrLegalConsentRequired = errors.New("legal consent is required")
	// ErrLegalConsentOutdated 表示客户端确认的版本与当前生效版本不一致。
	ErrLegalConsentOutdated = errors.New("legal consent version is outdated")
)

type LegalSettings struct {
	UserAgreement string `json:"user_agreement"`
	PrivacyPolicy string `json:"privacy_policy"`
	// APIServiceAgreement 是登录必须勾选的《API 服务、隐私与使用责任协议》正文。
	// 留空时使用内置正文，因此协议始终可阅读且同意始终强制，不依赖管理员配置。
	APIServiceAgreement string `json:"api_service_agreement"`
}

var defaultLegalSettings = LegalSettings{
	UserAgreement: "",
	PrivacyPolicy: "",
}

func init() {
	config.GlobalConfig.Register("legal", &defaultLegalSettings)
}

func GetLegalSettings() *LegalSettings {
	return &defaultLegalSettings
}

// ConsentRequirement 描述当前生效的协议同意要求。
type ConsentRequirement struct {
	// Version 是当前生效的协议版本，客户端确认后必须原样回传。
	Version string `json:"version"`
	// Required 为 true 表示登录与注册必须携带匹配版本的同意标记。
	Required bool `json:"required"`
}

// CurrentConsentRequirement 返回当前协议版本。协议为本站内置内容并提供默认正文，
// 因此同意要求始终生效；Required 保留字段语义，便于前端按状态渲染而不再推断。
func CurrentConsentRequirement() ConsentRequirement {
	return ConsentRequirement{Version: CurrentLegalConsentVersion, Required: true}
}

// GetAPIServiceAgreement 返回《API 服务、隐私与使用责任协议》正文。
// 管理员未配置时返回内置默认正文，保证协议始终可阅读。
func GetAPIServiceAgreement() string {
	if content := strings.TrimSpace(defaultLegalSettings.APIServiceAgreement); content != "" {
		return content
	}
	return DefaultAPIServiceAgreement
}

// ValidateLegalConsent 校验客户端提交的同意状态。
// agreed 必须为 true，version 必须与当前生效版本一致，否则返回对应错误。
func ValidateLegalConsent(agreed bool, version string) error {
	if !agreed {
		return ErrLegalConsentRequired
	}
	if strings.TrimSpace(version) != CurrentLegalConsentVersion {
		return ErrLegalConsentOutdated
	}
	return nil
}
