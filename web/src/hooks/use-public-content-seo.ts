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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { applySiteSEO, type PublicContentSEOState } from '@/lib/site-seo'
import { useSystemConfigStore } from '@/stores/system-config-store'

/**
 * 使用页面已有的内容请求结果同步首页或关于页 SEO，不额外发起请求。
 *
 * @param pathname 首页或关于页路径。
 * @param hasCustomContent `undefined` 表示请求未完成或失败，应保留匹配的服务端元数据。
 * @returns 无返回值。
 */
export function usePublicContentSEO(
  pathname: '/' | '/about',
  hasCustomContent: boolean | undefined
): void {
  const { i18n } = useTranslation()
  const systemName = useSystemConfigStore((state) => state.config.systemName)

  useEffect(() => {
    let publicContentState: PublicContentSEOState = 'unresolved'
    if (hasCustomContent !== undefined) {
      publicContentState = hasCustomContent ? 'custom' : 'default'
    }
    applySiteSEO(
      document,
      pathname,
      i18n.resolvedLanguage || i18n.language,
      systemName,
      true,
      publicContentState
    )
  }, [
    hasCustomContent,
    i18n.language,
    i18n.resolvedLanguage,
    pathname,
    systemName,
  ])
}
