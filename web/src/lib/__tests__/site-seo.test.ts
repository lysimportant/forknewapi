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
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import {
  DEFAULT_LOGO,
  DEFAULT_SYSTEM_NAME,
  normalizeSystemLogo,
  normalizeSystemName,
} from '@/lib/constants'
import {
  applySiteSEO,
  hasServerPublicPageSEO,
  SITE_SEO_MANIFEST,
} from '@/lib/site-seo'
import {
  mapStatusDataToConfig,
  readCachedStatus,
  STATUS_STORAGE_KEY,
} from '@/lib/status-query'
import { useSystemConfigStore } from '@/stores/system-config-store'

const originalSystemConfigStorage =
  useSystemConfigStore.persist.getOptions().storage

type SystemConfigStorage = NonNullable<
  ReturnType<typeof useSystemConfigStore.persist.getOptions>['storage']
>

beforeEach(() => {
  useSystemConfigStore.persist.setOptions({
    storage: originalSystemConfigStorage,
  })
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  document.head.innerHTML = '<title>stale title</title>'
  document.body.innerHTML = ''
  window.localStorage.clear()
})

afterEach(() => {
  useSystemConfigStore.persist.setOptions({
    storage: originalSystemConfigStorage,
  })
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  document.head.innerHTML = ''
  document.body.innerHTML = ''
  window.localStorage.clear()
})

it('SPA 公开页导航会替换唯一元数据、规范链接和结构化数据', () => {
  applySiteSEO(document, '/', 'zhCN', DEFAULT_SYSTEM_NAME)

  expect(document.title).toBe('ManSuiAI - AI 聚合平台')
  expect(document.documentElement.lang).toBe('zh-CN')
  expect(document.querySelector('link[rel="canonical"]')).toHaveAttribute(
    'href',
    'https://api.lolicon.beer/'
  )
  expect(document.querySelector('meta[name="robots"]')).toHaveAttribute(
    'content',
    'index, follow'
  )

  applySiteSEO(document, '/about/', 'ja', DEFAULT_SYSTEM_NAME)

  expect(document.title).toBe('关于 ManSuiAI - AI 聚合平台')
  expect(document.documentElement.lang).toBe('ja')
  expect(document.querySelector('link[rel="canonical"]')).toHaveAttribute(
    'href',
    'https://api.lolicon.beer/about'
  )
  expect(document.querySelectorAll('title')).toHaveLength(1)
  expect(document.querySelectorAll('meta[name="description"]')).toHaveLength(1)
  expect(document.querySelectorAll('meta[property="og:title"]')).toHaveLength(1)
  expect(document.querySelector('meta[property="og:locale"]')).toHaveAttribute(
    'content',
    'zh_CN'
  )
  expect(
    document.querySelector('meta[property="og:image:secure_url"]')
  ).toHaveAttribute('content', 'https://api.lolicon.beer/mansui-social.png')
  expect(
    document.querySelector('meta[property="og:image:type"]')
  ).toHaveAttribute('content', 'image/png')
  expect(
    document.querySelector('meta[property="og:image:width"]')
  ).toHaveAttribute('content', '1200')
  expect(
    document.querySelector('meta[property="og:image:height"]')
  ).toHaveAttribute('content', '630')
  expect(document.querySelectorAll('script#site-jsonld')).toHaveLength(1)
  expect(
    JSON.parse(
      document.querySelector<HTMLScriptElement>('#site-jsonld')?.textContent ??
        '{}'
    )
  ).toMatchObject({
    '@type': 'AboutPage',
    url: 'https://api.lolicon.beer/about',
  })
})

it('私有页和非公开价格页会 noindex 并清除上一公开页元数据', () => {
  applySiteSEO(document, '/', 'en', DEFAULT_SYSTEM_NAME)
  applySiteSEO(document, '/pricing', 'en', DEFAULT_SYSTEM_NAME, false)

  expect(document.title).toBe(DEFAULT_SYSTEM_NAME)
  expect(document.querySelector('meta[name="robots"]')).toHaveAttribute(
    'content',
    'noindex, nofollow'
  )
  expect(document.querySelector('link[rel="canonical"]')).toBeNull()
  expect(document.querySelector('meta[property^="og:"]')).toBeNull()
  expect(document.querySelector('meta[name^="twitter:"]')).toBeNull()
  expect(document.querySelector('#site-jsonld')).toBeNull()

  applySiteSEO(document, '/dashboard', 'fr', 'Custom Gateway')
  expect(document.title).toBe('Custom Gateway')
  expect(document.documentElement.lang).toBe('fr')
  expect(document.querySelector('meta[name="robots"]')).toHaveAttribute(
    'content',
    'noindex, nofollow'
  )
})

it.each(['/', '/about'])(
  '公开页 %s 描述大肥鱼头像并提供可读的分享图片说明',
  (pathname) => {
    applySiteSEO(document, pathname, 'zhCN', DEFAULT_SYSTEM_NAME)

    expect(
      document
        .querySelector('meta[name="description"]')
        ?.getAttribute('content')
    ).toContain('大肥鱼')
    for (const selector of [
      'meta[property="og:image:alt"]',
      'meta[name="twitter:image:alt"]',
    ]) {
      expect(document.querySelector(selector)).toHaveAttribute(
        'content',
        expect.stringContaining('DeepSeek')
      )
      expect(document.querySelector(selector)).toHaveAttribute(
        'content',
        expect.stringContaining('大肥鱼')
      )
    }
    expect(
      JSON.parse(document.querySelector('#site-jsonld')?.textContent ?? '{}')
    ).toMatchObject({
      image: {
        '@type': 'ImageObject',
        url: 'https://api.lolicon.beer/mansui-social.png',
        caption: expect.stringContaining('大肥鱼'),
      },
    })

    applySiteSEO(document, '/dashboard', 'zhCN', DEFAULT_SYSTEM_NAME)
    expect(document.querySelector('meta[property="og:image:alt"]')).toBeNull()
    expect(document.querySelector('meta[name="twitter:image:alt"]')).toBeNull()
  }
)

it('状态未返回时可识别并保留后端生成的公开价格页元数据', () => {
  applySiteSEO(document, '/pricing', 'en', DEFAULT_SYSTEM_NAME, true)
  expect(hasServerPublicPageSEO(document, '/pricing')).toBe(true)

  applySiteSEO(document, '/pricing', 'en', DEFAULT_SYSTEM_NAME, false)
  expect(hasServerPublicPageSEO(document, '/pricing')).toBe(false)
})

it.each([
  ['/', 'WebSite'],
  ['/about', 'AboutPage'],
] as const)(
  '管理员自定义内容使 %s 使用中性站点元数据',
  (pathname, schemaType) => {
    applySiteSEO(
      document,
      pathname,
      'zhCN',
      DEFAULT_SYSTEM_NAME,
      true,
      'custom'
    )

    expect(document.title).toBe(SITE_SEO_MANIFEST.pages['/'].title)
    expect(document.querySelector('meta[name="description"]')).toHaveAttribute(
      'content',
      SITE_SEO_MANIFEST.description
    )
    expect(document.querySelector('meta[property="og:title"]')).toHaveAttribute(
      'content',
      SITE_SEO_MANIFEST.pages['/'].title
    )
    expect(
      JSON.parse(
        document.querySelector<HTMLScriptElement>('#site-jsonld')
          ?.textContent ?? '{}'
      )
    ).toMatchObject({
      '@type': schemaType,
      name: SITE_SEO_MANIFEST.pages['/'].title,
      description: SITE_SEO_MANIFEST.description,
    })
  }
)

it('SPA 导航会在默认内容与自定义内容之间切换元数据', () => {
  applySiteSEO(document, '/', 'en', DEFAULT_SYSTEM_NAME)
  expect(document.title).toBe(SITE_SEO_MANIFEST.pages['/'].title)

  applySiteSEO(document, '/about', 'en', DEFAULT_SYSTEM_NAME, true, 'custom')
  expect(document.title).toBe(SITE_SEO_MANIFEST.pages['/'].title)
  expect(document.querySelector('link[rel="canonical"]')).toHaveAttribute(
    'href',
    'https://api.lolicon.beer/about'
  )

  applySiteSEO(document, '/about', 'en', DEFAULT_SYSTEM_NAME)
  expect(document.title).toBe(SITE_SEO_MANIFEST.pages['/about'].title)
})

it('内容未决或加载失败时保留匹配的服务端元数据，并为 SPA 导航使用中性回退', () => {
  document.head.innerHTML = `
    <title>服务端自定义标题</title>
    <meta name="description" content="服务端自定义描述" />
    <meta name="robots" content="index, follow" />
    <link rel="canonical" href="https://api.lolicon.beer/about" />
  `

  applySiteSEO(
    document,
    '/about',
    'ja',
    DEFAULT_SYSTEM_NAME,
    true,
    'unresolved'
  )
  expect(document.title).toBe('服务端自定义标题')
  expect(document.querySelector('meta[name="description"]')).toHaveAttribute(
    'content',
    '服务端自定义描述'
  )
  expect(document.documentElement.lang).toBe('ja')

  applySiteSEO(document, '/', 'en', DEFAULT_SYSTEM_NAME, true, 'unresolved')
  expect(document.title).toBe(SITE_SEO_MANIFEST.pages['/'].title)
  expect(document.querySelector('meta[name="description"]')).toHaveAttribute(
    'content',
    SITE_SEO_MANIFEST.description
  )
  expect(document.querySelector('link[rel="canonical"]')).toHaveAttribute(
    'href',
    'https://api.lolicon.beer/'
  )
})

it.each(['New API', 'NewAPI', 'ManSui'])(
  '旧缓存品牌 %s 会迁移为部署默认品牌并保留其他状态',
  (legacyName) => {
    window.localStorage.setItem(
      STATUS_STORAGE_KEY,
      JSON.stringify({
        system_name: legacyName,
        logo: '/logo.png',
        version: 'cached-version',
      })
    )

    expect(readCachedStatus()).toMatchObject({
      system_name: DEFAULT_SYSTEM_NAME,
      logo: DEFAULT_LOGO,
      version: 'cached-version',
    })
    expect(
      mapStatusDataToConfig({ system_name: legacyName, logo: '/logo.png' })
    ).toMatchObject({ systemName: DEFAULT_SYSTEM_NAME, logo: DEFAULT_LOGO })
  }
)

it('管理员自定义品牌不会被部署默认值覆盖', () => {
  expect(normalizeSystemName('Custom Gateway')).toBe('Custom Gateway')
  expect(normalizeSystemLogo('https://example.com/custom-logo.png')).toBe(
    'https://example.com/custom-logo.png'
  )
})

it.each([
  ['首次访问', undefined],
  ['空持久化状态', null],
] as const)('%s 时系统配置保留默认状态和操作方法', async (_scenario, state) => {
  const storage = {
    getItem: vi.fn(() => (state === undefined ? null : { state, version: 0 })),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  } as unknown as SystemConfigStorage
  useSystemConfigStore.persist.setOptions({ storage })

  await useSystemConfigStore.persist.rehydrate()

  const current = useSystemConfigStore.getState()
  expect(current.config).toEqual({
    systemName: DEFAULT_SYSTEM_NAME,
    logo: DEFAULT_LOGO,
    currency: expect.objectContaining({ quotaDisplayType: 'USD' }),
  })
  expect(current.loadedLogoUrl).toBe(DEFAULT_LOGO)
  expect(current.setConfig).toBeTypeOf('function')
  expect(current.setLoadedLogoUrl).toBeTypeOf('function')
  expect(current.setLoading).toBeTypeOf('function')
})
