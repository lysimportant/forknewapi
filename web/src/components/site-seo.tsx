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
import { useRouterState } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { toIntlLocale } from '@/i18n/languages'
import { getModuleAccessFromStatus } from '@/lib/nav-modules'
import {
  applySiteSEO,
  hasServerPublicPageSEO,
  removeServerSEOContent,
} from '@/lib/site-seo'
import { useSystemConfigStore } from '@/stores/system-config-store'

/** 在每次 SPA 路由或界面语言变化后同步站点 SEO 元数据。 */
export function SiteSEO() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const { i18n } = useTranslation()
  const { status } = useStatus()
  const systemName = useSystemConfigStore((state) => state.config.systemName)
  const pricingAccess = getModuleAccessFromStatus(
    status as Record<string, unknown> | null,
    'pricing'
  )
  const pricingPublic = status
    ? pricingAccess.enabled && !pricingAccess.requireAuth
    : undefined

  useEffect(() => {
    const language = i18n.resolvedLanguage || i18n.language
    const routePath = pathname.replace(/\/+$/, '') || '/'
    const publicContentState =
      routePath === '/' || routePath === '/about' ? 'unresolved' : 'default'
    if (
      routePath === '/pricing' &&
      pricingPublic === undefined &&
      hasServerPublicPageSEO(document, routePath)
    ) {
      // 首次加载时保留后端已经按真实导航配置生成的公开价格页元数据。
      document.documentElement.lang = toIntlLocale(language) ?? 'en'
      return
    }

    applySiteSEO(
      document,
      pathname,
      language,
      systemName,
      pricingPublic ?? false,
      publicContentState
    )
  }, [
    i18n.language,
    i18n.resolvedLanguage,
    pathname,
    pricingPublic,
    systemName,
  ])

  useEffect(() => {
    removeServerSEOContent(document)
  }, [])

  return null
}
