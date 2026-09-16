/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { AuthBundle } from '@/stores/auth-store'

import type { LoginResult } from './secure-verification/types'

// ============================================================================
// API Payloads
// ============================================================================

export interface LoginPayload {
  username: string
  password: string
  turnstile?: string
  passwordEncryptionEnabled?: boolean
  /** 登录前是否已勾选《API 服务、隐私与使用责任协议》 */
  consent: boolean
  /** 已勾选协议的版本号，必须与 /api/status 的 legal_consent.version 一致 */
  consent_version: string
}

export interface TwoFAPayload {
  code: string
  flow_token: string
}

export interface RegisterPayload {
  username: string
  password: string
  email?: string
  verification_code?: string
  aff_code?: string
  turnstile?: string
  /** 注册与登录使用同一组协议确认字段 */
  consent: boolean
  consent_version: string
}

export interface PasswordResetPayload {
  email: string
  turnstile?: string
}

export interface EmailVerificationPayload {
  email: string
  turnstile?: string
}

export interface BindEmailPayload {
  flow_token: string
  new_code: string
  old_code?: string
}

// ============================================================================
// API Responses
// ============================================================================

export interface LoginResponse {
  success: boolean
  message: string
  data?: LoginResult
  /** 业务错误码，协议相关错误为 legal_consent_required / legal_consent_outdated */
  code?: string
  consent_required?: boolean
  consent_version?: string
}

export interface Login2FAResponse {
  success: boolean
  message: string
  data?: AuthBundle
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message: string
  data?: T
  /** 业务错误码，协议相关错误为 legal_consent_required / legal_consent_outdated */
  code?: string
  consent_required?: boolean
  consent_version?: string
}

// ============================================================================
// System Status
// ============================================================================

/**
 * 《API 服务、隐私与使用责任协议》的当前生效要求，由 /api/status 公开返回。
 * 协议为站点内置内容，因此 required 始终为 true，前端必须按版本回传同意标记。
 */
export interface LegalConsentStatus {
  /** 当前生效的协议版本，登录/注册必须原样回传 */
  version?: string
  /** 服务端是否强制要求同意 */
  required?: boolean
}

export interface SystemStatus {
  success?: boolean
  message?: string
  data?: {
    version?: string
    system_name?: string
    logo?: string
    github_oauth?: boolean
    github_client_id?: string
    discord_oauth?: boolean
    discord_client_id?: string
    oidc_enabled?: boolean
    oidc_authorization_endpoint?: string
    oidc_client_id?: string
    oidc_display_name?: string
    linuxdo_oauth?: boolean
    linuxdo_client_id?: string
    telegram_oauth?: boolean
    telegram_oauth_configured?: boolean
    telegram_bot_name?: string
    passkey_login?: boolean
    wechat_login?: boolean
    wechat_qrcode?: string
    wechat_qr_code?: string
    wechat_qrcode_image_url?: string
    wechat_qr_code_image_url?: string
    wechat_account_qrcode_image_url?: string
    WeChatAccountQRCodeImageURL?: string
    turnstile_check?: boolean
    turnstile_site_key?: string
    email_verification?: boolean
    self_use_mode_enabled?: boolean
    display_in_currency?: boolean
    display_token_stat_enabled?: boolean
    quota_per_unit?: number
    quota_display_type?: string
    usd_exchange_rate?: number
    custom_currency_symbol?: string
    custom_currency_exchange_rate?: number
    demo_site_enabled?: boolean
    user_agreement_enabled?: boolean
    privacy_policy_enabled?: boolean
    legal_consent?: LegalConsentStatus
    oauth_register_enabled?: boolean
    register_enabled?: boolean
    password_login_enabled?: boolean
    password_login_encryption_enabled?: boolean
    password_register_enabled?: boolean
    custom_oauth_providers?: CustomOAuthProviderInfo[]
    [key: string]: unknown
  }
  // Allow direct access to common properties
  version?: string
  system_name?: string
  logo?: string
  github_oauth?: boolean
  github_client_id?: string
  discord_oauth?: boolean
  discord_client_id?: string
  oidc_enabled?: boolean
  oidc_authorization_endpoint?: string
  oidc_client_id?: string
  oidc_display_name?: string
  linuxdo_oauth?: boolean
  linuxdo_client_id?: string
  telegram_oauth?: boolean
  telegram_oauth_configured?: boolean
  telegram_bot_name?: string
  passkey_login?: boolean
  wechat_login?: boolean
  wechat_qrcode?: string
  wechat_qr_code?: string
  wechat_qrcode_image_url?: string
  wechat_qr_code_image_url?: string
  wechat_account_qrcode_image_url?: string
  WeChatAccountQRCodeImageURL?: string
  turnstile_check?: boolean
  turnstile_site_key?: string
  email_verification?: boolean
  self_use_mode_enabled?: boolean
  display_in_currency?: boolean
  display_token_stat_enabled?: boolean
  quota_per_unit?: number
  quota_display_type?: string
  usd_exchange_rate?: number
  custom_currency_symbol?: string
  custom_currency_exchange_rate?: number
  demo_site_enabled?: boolean
  user_agreement_enabled?: boolean
  privacy_policy_enabled?: boolean
  legal_consent?: LegalConsentStatus
  oauth_register_enabled?: boolean
  register_enabled?: boolean
  password_login_enabled?: boolean
  password_login_encryption_enabled?: boolean
  password_register_enabled?: boolean
  custom_oauth_providers?: CustomOAuthProviderInfo[]
  [key: string]: unknown
}

// ============================================================================
// OAuth
// ============================================================================

export interface OAuthProvider {
  name: string
  type: 'github' | 'discord' | 'oidc' | 'linuxdo' | 'telegram' | 'wechat'
  enabled: boolean
  clientId?: string
  authEndpoint?: string
}

export interface CustomOAuthProviderInfo {
  id: number
  name: string
  slug: string
  icon: string
  client_id: string
  authorization_endpoint: string
  scopes: string
}

// ============================================================================
// Form Props
// ============================================================================

export interface AuthFormProps extends React.HTMLAttributes<HTMLFormElement> {
  redirectTo?: string
}
