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
import { toIntlLocale } from '@/i18n/languages'

import rawSiteSEOManifest from '../../public/site-seo.json'

/** 站点 SEO 清单中的导航链接。 */
export type SiteSEOLink = {
  href: string
  label: string
}

/** 单个公开页面的搜索元数据与无脚本摘要。 */
export type SiteSEOPage = {
  title: string
  description: string
  heading: string
  paragraphs: string[]
  links: SiteSEOLink[]
}

/** 前后端共同读取的站点 SEO 清单。 */
export type SiteSEOManifest = {
  origin: string
  siteName: string
  description: string
  image: string
  imageAlt: string
  pages: Record<string, SiteSEOPage>
}

/** 客户端为当前路径生成的完整 SEO 状态。 */
export type ResolvedSiteSEO = {
  title: string
  description: string
  robots: 'index, follow' | 'noindex, nofollow'
  canonical: string | null
  image: string
  imageAlt: string
  siteName: string
  schema: Record<string, unknown> | null
  index: boolean
}

/** 首页或关于页的管理员自定义内容解析状态。 */
export type PublicContentSEOState = 'default' | 'custom' | 'unresolved'

/** 前后端共享的只读站点 SEO 清单。 */
export const SITE_SEO_MANIFEST = rawSiteSEOManifest as SiteSEOManifest

/** 明确允许搜索引擎收录的前端路径。 */
export const PUBLIC_SEO_PATHS = ['/', '/about', '/pricing'] as const

/** 将路由路径规范化为清单使用的稳定形式。 */
function normalizePath(pathname: string): string {
  if (!pathname || pathname === '/') return '/'
  return pathname.replace(/\/+$/, '') || '/'
}

/** 将清单图片路径限制在站点规范域名内。 */
function resolveImageURL(image: string): string {
  if (image.startsWith('/')) return `${SITE_SEO_MANIFEST.origin}${image}`
  if (image.startsWith(`${SITE_SEO_MANIFEST.origin}/`)) return image
  return `${SITE_SEO_MANIFEST.origin}/mansui-social.png`
}

/** 根据页面路径返回 Schema.org 页面类型。 */
function resolveSchemaType(pathname: string): string {
  if (pathname === '/') return 'WebSite'
  if (pathname === '/about') return 'AboutPage'
  return 'WebPage'
}

/**
 * 根据当前路径计算公开页或私有页的元数据。
 *
 * @param pathname 当前浏览器路径，不包含查询参数。
 * @param privateTitle 私有页可使用的租户显示名称；公开页始终使用共享清单。
 * @param pricingPublic 价格页是否公开且无需登录。
 * @param publicContentState 首页或关于页的管理员内容解析状态；未决时使用中性回退。
 * @returns 可直接写入 document.head 的规范化元数据。
 */
export function resolveSiteSEO(
  pathname: string,
  privateTitle?: string,
  pricingPublic = true,
  publicContentState: PublicContentSEOState = 'default'
): ResolvedSiteSEO {
  const routePath = normalizePath(pathname)
  const page = SITE_SEO_MANIFEST.pages[routePath]
  const index =
    PUBLIC_SEO_PATHS.includes(routePath as (typeof PUBLIC_SEO_PATHS)[number]) &&
    (routePath !== '/pricing' || pricingPublic)
  const image = resolveImageURL(SITE_SEO_MANIFEST.image)

  if (!index || !page) {
    return {
      title: privateTitle?.trim() || SITE_SEO_MANIFEST.siteName,
      description: SITE_SEO_MANIFEST.description,
      robots: 'noindex, nofollow',
      canonical: null,
      image,
      imageAlt: SITE_SEO_MANIFEST.imageAlt,
      siteName: SITE_SEO_MANIFEST.siteName,
      schema: null,
      index: false,
    }
  }

  const useNeutralContentMetadata =
    (routePath === '/' || routePath === '/about') &&
    publicContentState !== 'default'
  const title = useNeutralContentMetadata
    ? SITE_SEO_MANIFEST.pages['/'].title
    : page.title
  const description = useNeutralContentMetadata
    ? SITE_SEO_MANIFEST.description
    : page.description
  const canonical = `${SITE_SEO_MANIFEST.origin}${routePath}`
  const schema = {
    '@context': 'https://schema.org',
    '@type': resolveSchemaType(routePath),
    name: title,
    description,
    url: canonical,
    image: {
      '@type': 'ImageObject',
      url: image,
      caption: SITE_SEO_MANIFEST.imageAlt,
    },
    isPartOf: {
      '@type': 'WebSite',
      name: SITE_SEO_MANIFEST.siteName,
      url: `${SITE_SEO_MANIFEST.origin}/`,
    },
  }

  return {
    title,
    description,
    robots: 'index, follow',
    canonical,
    image,
    imageAlt: SITE_SEO_MANIFEST.imageAlt,
    siteName: SITE_SEO_MANIFEST.siteName,
    schema,
    index: true,
  }
}

/**
 * 判断当前文档是否已包含与指定公开页面匹配的服务端元数据。
 *
 * @param documentRoot 当前页面的 Document。
 * @param pathname 待匹配的公开页面路径。
 * @returns canonical 与 robots 都符合公开页面契约时返回 true。
 */
export function hasServerPublicPageSEO(
  documentRoot: Document,
  pathname: string
): boolean {
  const routePath = normalizePath(pathname)
  if (
    !PUBLIC_SEO_PATHS.includes(routePath as (typeof PUBLIC_SEO_PATHS)[number])
  ) {
    return false
  }
  const canonical = documentRoot
    .querySelector<HTMLLinkElement>('link[rel="canonical"]')
    ?.getAttribute('href')
  const robots = documentRoot
    .querySelector<HTMLMetaElement>('meta[name="robots"]')
    ?.getAttribute('content')
  return (
    canonical === `${SITE_SEO_MANIFEST.origin}${routePath}` &&
    robots === 'index, follow'
  )
}

/** 设置唯一的 meta 元素，并清除同名的旧副本。 */
function setMeta(
  documentRoot: Document,
  attribute: 'name' | 'property',
  key: string,
  content: string
): void {
  const elements = [
    ...documentRoot.head.querySelectorAll<HTMLMetaElement>(
      `meta[${attribute}="${key}"]`
    ),
  ]
  const element = elements.shift() ?? documentRoot.createElement('meta')
  element.setAttribute(attribute, key)
  element.setAttribute('content', content)
  if (!element.isConnected) documentRoot.head.append(element)
  elements.forEach((duplicate) => duplicate.remove())
}

/** 删除指定名称或属性的全部 meta 元素。 */
function removeMeta(
  documentRoot: Document,
  attribute: 'name' | 'property',
  key: string
): void {
  documentRoot.head
    .querySelectorAll(`meta[${attribute}="${key}"]`)
    .forEach((element) => element.remove())
}

/** 设置唯一的 canonical 链接。 */
function setCanonical(documentRoot: Document, href: string): void {
  const elements = [
    ...documentRoot.head.querySelectorAll<HTMLLinkElement>(
      'link[rel="canonical"]'
    ),
  ]
  const element = elements.shift() ?? documentRoot.createElement('link')
  element.setAttribute('rel', 'canonical')
  element.setAttribute('href', href)
  if (!element.isConnected) documentRoot.head.append(element)
  elements.forEach((duplicate) => duplicate.remove())
}

/** 删除公开页专用的开放图谱、规范链接和结构化数据。 */
function removePublicMetadata(documentRoot: Document): void {
  documentRoot.head
    .querySelectorAll('link[rel="canonical"]')
    .forEach((element) => element.remove())
  for (const property of [
    'og:type',
    'og:site_name',
    'og:title',
    'og:description',
    'og:url',
    'og:image',
    'og:image:alt',
  ]) {
    removeMeta(documentRoot, 'property', property)
  }
  for (const name of [
    'twitter:card',
    'twitter:title',
    'twitter:description',
    'twitter:image',
    'twitter:image:alt',
  ]) {
    removeMeta(documentRoot, 'name', name)
  }
  documentRoot.head
    .querySelectorAll('script#site-jsonld')
    .forEach((element) => element.remove())
}

/**
 * 将路由 SEO 状态原位同步到 DOM，确保 SPA 导航不会残留上一页的公开元数据。
 *
 * @param documentRoot 当前页面的 Document。
 * @param pathname 当前路由路径。
 * @param language i18next 当前语言代码。
 * @param privateTitle 私有页使用的租户显示名称。
 * @param pricingPublic 价格页是否公开且无需登录。
 * @param publicContentState 首页或关于页的管理员内容解析状态。
 */
export function applySiteSEO(
  documentRoot: Document,
  pathname: string,
  language: string | undefined,
  privateTitle?: string,
  pricingPublic = true,
  publicContentState: PublicContentSEOState = 'default'
): void {
  const routePath = normalizePath(pathname)
  if (
    publicContentState === 'unresolved' &&
    (routePath === '/' || routePath === '/about') &&
    hasServerPublicPageSEO(documentRoot, routePath)
  ) {
    // 首次加载时，服务端已经按当前管理员内容生成了准确元数据。
    documentRoot.documentElement.lang = toIntlLocale(language) ?? 'en'
    return
  }

  const resolved = resolveSiteSEO(
    pathname,
    privateTitle,
    pricingPublic,
    publicContentState
  )
  const titles = [...documentRoot.head.querySelectorAll('title')]
  const title = titles.shift() ?? documentRoot.createElement('title')
  title.textContent = resolved.title
  if (!title.isConnected) documentRoot.head.append(title)
  titles.forEach((duplicate) => duplicate.remove())

  documentRoot.documentElement.lang = toIntlLocale(language) ?? 'en'
  setMeta(documentRoot, 'name', 'title', resolved.title)
  setMeta(documentRoot, 'name', 'description', resolved.description)
  setMeta(documentRoot, 'name', 'robots', resolved.robots)

  if (!resolved.index || !resolved.canonical || !resolved.schema) {
    removePublicMetadata(documentRoot)
    return
  }

  setCanonical(documentRoot, resolved.canonical)
  setMeta(documentRoot, 'property', 'og:type', 'website')
  setMeta(documentRoot, 'property', 'og:site_name', resolved.siteName)
  setMeta(documentRoot, 'property', 'og:title', resolved.title)
  setMeta(documentRoot, 'property', 'og:description', resolved.description)
  setMeta(documentRoot, 'property', 'og:url', resolved.canonical)
  setMeta(documentRoot, 'property', 'og:image', resolved.image)
  setMeta(documentRoot, 'property', 'og:image:alt', resolved.imageAlt)
  setMeta(documentRoot, 'name', 'twitter:card', 'summary_large_image')
  setMeta(documentRoot, 'name', 'twitter:title', resolved.title)
  setMeta(documentRoot, 'name', 'twitter:description', resolved.description)
  setMeta(documentRoot, 'name', 'twitter:image', resolved.image)
  setMeta(documentRoot, 'name', 'twitter:image:alt', resolved.imageAlt)

  const scripts = [
    ...documentRoot.head.querySelectorAll<HTMLScriptElement>(
      'script#site-jsonld'
    ),
  ]
  const script = scripts.shift() ?? documentRoot.createElement('script')
  script.id = 'site-jsonld'
  script.type = 'application/ld+json'
  script.textContent = JSON.stringify(resolved.schema).replaceAll(
    '<',
    '\\u003c'
  )
  if (!script.isConnected) documentRoot.head.append(script)
  scripts.forEach((duplicate) => duplicate.remove())
}

/** 在 React 提交后移除服务端语义摘要与仅供首屏使用的首页模式标记。 */
export function removeServerSEOContent(documentRoot: Document): void {
  documentRoot.querySelector('#public-seo-content')?.remove()
  documentRoot.querySelector('meta[name="mansui-home-mode"]')?.remove()
}
