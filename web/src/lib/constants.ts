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
/**
 * Application-wide constants
 */

/** 租户未自定义名称时显示的站点品牌。 */
export const DEFAULT_SYSTEM_NAME = 'ManSuiAI - AI 聚合平台'

/** 租户未自定义图标时显示的站点品牌图标。 */
export const DEFAULT_LOGO = '/mansui-icon.png'

/**
 * 将空值和旧版项目默认名称映射为当前租户品牌，同时保留管理员自定义名称。
 *
 * @param value 状态接口或持久化缓存中的系统名称。
 * @returns 可直接展示的系统名称。
 */
export function normalizeSystemName(value?: string | null): string {
  const name = value?.trim() ?? ''
  if (!name || /^(new api|newapi|mansui)$/i.test(name)) {
    return DEFAULT_SYSTEM_NAME
  }
  return name
}

/**
 * 将空值和旧版项目默认图标映射为当前租户品牌，同时保留管理员自定义图标。
 *
 * @param value 状态接口或持久化缓存中的图标地址。
 * @returns 可直接加载的图标地址。
 */
export function normalizeSystemLogo(value?: string | null): string {
  const logo = value?.trim() ?? ''
  if (!logo || logo === '/logo.png') return DEFAULT_LOGO
  return logo
}
// 与后端 general_setting.docs_link 的默认值保持一致，避免空值产生死链
export const DEFAULT_DOCS_LINK = 'https://docs.newapi.pro'

// LocalStorage Keys
export const STORAGE_KEYS = {
  SYSTEM_NAME: 'system_name',
  LOGO: 'logo',
  FOOTER_HTML: 'footer_html',
} as const
