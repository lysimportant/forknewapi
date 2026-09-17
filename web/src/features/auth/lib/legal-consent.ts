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
import type { SystemStatus } from '../types'

/** 服务端返回的协议“已更新”业务错误码。 */
export const LEGAL_CONSENT_OUTDATED_CODE = 'legal_consent_outdated'

/** 服务端返回的协议“未同意”业务错误码。 */
export const LEGAL_CONSENT_REQUIRED_CODE = 'legal_consent_required'

export type LegalConsentRequirement = {
  /** 当前生效的协议版本，登录时必须原样回传。 */
  version: string
  /** 是否已从 /api/status 读取到协议版本；未知时不得放开任何登录入口 */
  known: boolean
}

/**
 * 读取 /api/status 中的协议同意要求。useStatus() 暴露的是 status.data，
 * 但本地缓存与测试桩可能给出嵌套结构，因此两种形状都兼容，与
 * user_agreement_enabled 的读取方式保持一致。
 */
export function getLegalConsentRequirement(
  status: SystemStatus | null
): LegalConsentRequirement {
  const raw = status?.legal_consent ?? status?.data?.legal_consent
  const version = typeof raw?.version === 'string' ? raw.version.trim() : ''
  return {
    version,
    known: version.length > 0,
  }
}

/**
 * 勾选状态与协议版本同时满足才允许提交。服务端无条件校验同意标记与版本，
 * 因此版本未知（状态加载中或请求失败）时返回 false，避免在协议要求未知的
 * 情况下短暂放开登录。
 */
export function isLegalConsentSatisfied(
  requirement: LegalConsentRequirement,
  checked: boolean
): boolean {
  return requirement.known && checked
}
