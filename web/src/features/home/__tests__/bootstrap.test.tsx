/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { runInNewContext } from 'node:vm'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor } from '@testing-library/react'
import { AxiosError, type AxiosAdapter, type AxiosResponse } from 'axios'
import { StrictMode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { SiteSEO } from '@/components/site-seo'
import { ThemeProvider } from '@/context/theme-provider'
import { Home } from '@/features/home'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

type HomeReply =
  | { kind: 'success'; data: string }
  | { kind: 'failure' }
  | 'pending'

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

const originalAdapter = api.defaults.adapter
const cwdIndexHtmlPath = resolve(process.cwd(), 'index.html')
const indexHtmlPath = existsSync(cwdIndexHtmlPath)
  ? cwdIndexHtmlPath
  : resolve(process.cwd(), 'web/index.html')
const indexHtml = readFileSync(indexHtmlPath, 'utf8')
const themeBootstrap = indexHtml.match(
  /<script id="theme-bootstrap">([\s\S]*?)<\/script>/
)

if (!themeBootstrap) {
  throw new Error('theme-bootstrap script is missing from web/index.html')
}
const bootstrapSource = themeBootstrap[1]

const pendingReplies: Deferred<string>[] = []
const queryClients: QueryClient[] = []

/** 保存入口 DOM 和可控超时，供测试检查首屏状态及触发降级。 */
type ThemeBootstrapRun = {
  document: Document
  timeoutDelays: number[]
  flushTimeouts: () => void
}

/** 创建可控的网络 Promise，使 pending 状态可在断言后明确收敛。 */
function createDeferred<T>(): Deferred<T> {
  let resolvePromise: (value: T) => void = () => undefined
  let rejectPromise: (reason?: unknown) => void = () => undefined
  const promise = new Promise<T>((resolve, reject) => {
    resolvePromise = resolve
    rejectPromise = reject
  })
  return {
    promise,
    resolve: resolvePromise,
    reject: rejectPromise,
  }
}

/** 返回 Axios 适配器所需的最小响应，所有其他请求都使用稳定公共夹具。 */
function response<T>(
  config: Parameters<AxiosAdapter>[0],
  data: T
): AxiosResponse<T> {
  return {
    config,
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
  }
}

/** 只替换 Axios 网络边界，保留 Home、Hook、路由和 Query 的真实实现。 */
function installHomeAdapter(reply: HomeReply): {
  adapter: AxiosAdapter
  requests: string[]
  pending: Deferred<string> | null
} {
  const requests: string[] = []
  const pending = reply === 'pending' ? createDeferred<string>() : null
  if (pending) pendingReplies.push(pending)
  const adapter = vi.fn<AxiosAdapter>(async (config) => {
    const url = String(config.url ?? '')
    requests.push(url)

    if (url === '/api/home_page_content') {
      if (reply === 'pending') {
        if (!pending) throw new Error('pending response was not initialized')
        return response(config, { success: true, data: await pending.promise })
      }
      if (reply.kind === 'failure') {
        throw new AxiosError('Network Error', 'ERR_NETWORK', config)
      }
      return response(config, { success: true, data: reply.data })
    }

    if (url === '/api/notice') {
      return response(config, { success: true, data: '' })
    }
    if (url === '/api/status') {
      return response(config, {
        success: true,
        data: {
          system_name: 'Test Gateway',
          announcements_enabled: false,
        },
      })
    }
    return response(config, { success: true, data: null })
  })

  api.defaults.adapter = adapter
  return { adapter, requests, pending }
}

/** 为每个 Home 夹具注入服务端模式标记，并清除上一次路由留下的节点。 */
function prepareHomeDocument(mode: 'default' | 'custom'): void {
  document.documentElement.className = ''
  document.documentElement.removeAttribute('data-home-boot')
  document.querySelector('meta[name="mansui-home-mode"]')?.remove()
  const meta = document.createElement('meta')
  meta.name = 'mansui-home-mode'
  meta.content = mode
  document.head.append(meta)
  if (mode === 'default') {
    document.documentElement.dataset.homeBoot = 'default'
  }
}

/** 用真实路由和 QueryProvider 渲染 Home；状态和公告请求使用已确认夹具。 */
function renderHome(path = '/') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
    },
  })
  queryClients.push(queryClient)
  queryClient.setQueryData(['notice'], { success: true, data: '' })
  queryClient.setQueryData(['status'], {
    system_name: 'Test Gateway',
    announcements_enabled: false,
  })

  const rootRoute = createRootRoute()
  const routeTree = rootRoute.addChildren([
    createRoute({
      getParentRoute: () => rootRoute,
      path: '/',
      component: () => (
        <>
          <SiteSEO />
          <Home />
        </>
      ),
    }),
    ...['/about', '/dashboard', '/pricing', '/rankings'].map((routePath) =>
      createRoute({
        getParentRoute: () => rootRoute,
        path: routePath,
        component: () => null,
      })
    ),
  ])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  const view = render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <RouterProvider router={router} />
        </ThemeProvider>
      </QueryClientProvider>
    </StrictMode>
  )
  return { view, queryClient, router }
}

/** 按路径和主题偏好执行真实入口脚本，返回 DOM、毫秒延时记录及超时触发方法。 */
function runThemeBootstrap(options: {
  path: string
  cookie?: string
  systemDark: boolean
  homeMode?: 'default' | 'custom'
}): ThemeBootstrapRun {
  const documentRoot = new DOMParser().parseFromString(indexHtml, 'text/html')
  const timeoutCallbacks: Array<() => void> = []
  const timeoutDelays: number[] = []
  const setTimeout = (callback: () => void, delay = 0): number => {
    timeoutCallbacks.push(callback)
    timeoutDelays.push(delay)
    return timeoutCallbacks.length
  }
  Object.defineProperty(documentRoot, 'cookie', {
    value: options.cookie ? `vite-ui-theme=${options.cookie}` : '',
  })
  if (options.homeMode) {
    const meta = documentRoot.createElement('meta')
    meta.name = 'mansui-home-mode'
    meta.content = options.homeMode
    documentRoot.head.append(meta)
  }
  runInNewContext(bootstrapSource, {
    document: documentRoot,
    window: {
      location: { pathname: options.path },
      matchMedia: () => ({ matches: options.systemDark }),
      setTimeout,
    },
    setTimeout,
  })
  return {
    document: documentRoot,
    timeoutDelays,
    flushTimeouts: () => {
      for (const callback of timeoutCallbacks.splice(0)) callback()
    },
  }
}

beforeEach(() => {
  window.localStorage.clear()
  window.sessionStorage.clear()
  useAuthStore.getState().auth.reset()
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
})

afterEach(async () => {
  for (const pending of pendingReplies.splice(0)) {
    pending.resolve('')
    await pending.promise
  }
  for (const client of queryClients.splice(0)) client.clear()
  api.defaults.adapter = originalAdapter
  document.querySelector('meta[name="mansui-home-mode"]')?.remove()
  document.documentElement.removeAttribute('data-home-boot')
  vi.restoreAllMocks()
})

describe('Home 首屏主题衔接', () => {
  test('默认首页请求 pending 时保留深色壳并显示共享 LoadingState', async () => {
    prepareHomeDocument('default')
    const network = installHomeAdapter('pending')
    const { view, queryClient } = renderHome()

    await waitFor(() =>
      expect(network.requests).toContain('/api/home_page_content')
    )
    expect(screen.getByText('Loading...')).toBeVisible()
    expect(
      screen.getByText('Loading...').closest('.mansui-landing')
    ).not.toBeNull()
    expect(document.documentElement.dataset.homeBoot).toBe('default')
    network.pending?.resolve('')
    await screen.findByRole('heading', {
      level: 1,
      name: 'ManSuiAI - AI aggregation platform',
    })
    view.unmount()
    queryClient.clear()
  }, 15_000)

  test('默认首页响应完成后渲染正式落地页并清理首屏标记', async () => {
    prepareHomeDocument('default')
    installHomeAdapter({ kind: 'success', data: '' })
    const { view, queryClient } = renderHome()

    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'ManSuiAI - AI aggregation platform',
      })
    ).toBeVisible()
    expect(document.documentElement).not.toHaveAttribute('data-home-boot')
    expect(document.querySelector('meta[name="mansui-home-mode"]')).toBeNull()
    expect(view.container.querySelector('.mansui-landing')).not.toBeNull()

    view.unmount()
    queryClient.clear()
  }, 15_000)

  test('自定义首页响应完成后显示内容并保持普通主题壳', async () => {
    prepareHomeDocument('custom')
    const network = installHomeAdapter({
      kind: 'success',
      data: 'Custom home content',
    })
    const { view, queryClient } = renderHome()

    expect(await screen.findByText('Custom home content')).toBeVisible()
    expect(view.container.querySelector('.mansui-landing')).toBeNull()
    expect(document.documentElement).not.toHaveAttribute('data-home-boot')
    expect(document.querySelector('meta[name="mansui-home-mode"]')).toBeNull()
    expect(
      screen.queryByRole('heading', {
        level: 1,
        name: 'ManSuiAI - AI aggregation platform',
      })
    ).not.toBeInTheDocument()
    expect(network.requests).toContain('/api/home_page_content')

    view.unmount()
    queryClient.clear()
  }, 15_000)

  test('默认首页请求失败后回退落地页并清理首屏标记', async () => {
    prepareHomeDocument('default')
    const network = installHomeAdapter({ kind: 'failure' })
    const { view, queryClient } = renderHome()

    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'ManSuiAI - AI aggregation platform',
      })
    ).toBeVisible()
    expect(document.documentElement).not.toHaveAttribute('data-home-boot')
    expect(document.querySelector('meta[name="mansui-home-mode"]')).toBeNull()
    expect(view.container.querySelector('.mansui-landing')).not.toBeNull()
    expect(network.requests).toContain('/api/home_page_content')

    view.unmount()
    queryClient.clear()
  }, 15_000)

  test('离开首页路由时清理仍在等待的首屏标记', async () => {
    prepareHomeDocument('default')
    const network = installHomeAdapter('pending')
    const { view, queryClient, router } = renderHome()

    await waitFor(() =>
      expect(network.requests).toContain('/api/home_page_content')
    )
    await act(() => router.navigate({ to: '/about' }))
    await waitFor(() =>
      expect(document.documentElement).not.toHaveAttribute('data-home-boot')
    )

    network.pending?.resolve('')
    view.unmount()
    queryClient.clear()
  }, 15_000)
})

describe('theme-bootstrap 入口脚本', () => {
  test.each([
    ['dark cookie', 'dark', false, 'dark'],
    ['light cookie', 'light', true, 'light'],
    ['system dark', undefined, true, 'dark'],
    ['system light', undefined, false, 'light'],
    ['invalid cookie falls back to system', 'invalid', true, 'dark'],
  ] as const)(
    '%s 恢复 html 主题类',
    (_name, cookie, systemDark, expectedClass) => {
      const documentRoot = runThemeBootstrap({
        path: '/',
        cookie,
        systemDark,
        homeMode: 'custom',
      }).document
      expect(documentRoot.documentElement.className).toBe(expectedClass)
      expect(documentRoot.documentElement.dataset.homeBoot).toBeUndefined()
    }
  )

  test.each([
    ['custom home', '/', 'custom'],
    ['non-home route', '/about', 'default'],
  ] as const)('%s 不强制默认首页深色标记', (_name, path, homeMode) => {
    const documentRoot = runThemeBootstrap({
      path,
      systemDark: false,
      homeMode,
    }).document
    expect(documentRoot.documentElement.className).toBe('light')
    expect(documentRoot.documentElement.dataset.homeBoot).toBeUndefined()
  })

  test('默认首页且系统深色时设置 data-home-boot=default', () => {
    const { document: documentRoot } = runThemeBootstrap({
      path: '/',
      systemDark: true,
      homeMode: 'default',
    })
    expect(documentRoot.documentElement.className).toBe('dark')
    expect(documentRoot.documentElement.dataset.homeBoot).toBe('default')
  })

  test('真实入口包含无 SEO 文案的站点图标遮罩并保留 SEO 正文入口', () => {
    const documentRoot = new DOMParser().parseFromString(indexHtml, 'text/html')
    const overlay = documentRoot.querySelector('#home-boot-overlay')

    expect(overlay).not.toBeNull()
    expect(overlay?.querySelector('img')?.getAttribute('src')).toBe(
      '/mansui-icon.png'
    )
    expect(overlay?.textContent?.trim()).toBe('')
    expect(indexHtml).toContain('<!--site-seo-content-->')
  })

  test('无脚本加载时首页遮罩保持未激活', () => {
    const documentRoot = new DOMParser().parseFromString(indexHtml, 'text/html')

    expect(documentRoot.documentElement.dataset.homeBoot).toBeUndefined()
    expect(documentRoot.querySelector('#home-boot-overlay')).not.toBeNull()
  })

  test('默认首页入口计时器到期后撤销遮罩标记', () => {
    const {
      document: documentRoot,
      timeoutDelays,
      flushTimeouts,
    } = runThemeBootstrap({
      path: '/',
      systemDark: true,
      homeMode: 'default',
    })

    expect(timeoutDelays).toContain(15_000)
    expect(documentRoot.documentElement.dataset.homeBoot).toBe('default')
    flushTimeouts()
    expect(documentRoot.documentElement.dataset.homeBoot).toBeUndefined()
  })

  test.each([
    ['custom home', '/', 'custom'],
    ['non-home route', '/about', 'default'],
  ] as const)('%s 不启动首页遮罩超时', (_name, path, homeMode) => {
    const { timeoutDelays } = runThemeBootstrap({
      path,
      systemDark: false,
      homeMode,
    })

    expect(timeoutDelays).toEqual([])
  })
})
