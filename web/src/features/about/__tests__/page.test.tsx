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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type AxiosAdapter } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { About } from '../index'
import { api } from '@/lib/api'

/** 保存请求适配器，测试只替换网络边界并在结束时恢复。 */
const originalAdapter = api.defaults.adapter

/** 项目归属信息由 <Footer /> 提供，必须在所有分支中可见且指向项目仓库。 */
const ATTRIBUTION_URL = 'https://github.com/QuantumNous/new-api'

function stubAboutResponse(data: unknown) {
  const adapter = vi.fn<AxiosAdapter>(async (config) => ({
    config,
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
  }))
  api.defaults.adapter = adapter
  return adapter
}

function renderAbout() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {
    system_name: 'Example Gateway',
    register_enabled: true,
    docs_link: 'https://docs.example.com',
  })
  const rootRoute = createRootRoute()
  // PublicLayout 的 Header/Footer 会链接到导航目标，测试树必须包含这些路径。
  const stubPaths = [
    '/dashboard',
    '/pricing',
    '/rankings',
    '/about',
    '/sign-in',
    '/sign-up',
    '/api-service-agreement',
  ]
  const routeTree = rootRoute.addChildren([
    createRoute({
      getParentRoute: () => rootRoute,
      path: '/',
      component: About,
    }),
    ...stubPaths.map((path) =>
      createRoute({
        getParentRoute: () => rootRoute,
        path,
        component: () => null,
      })
    ),
  ])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { view, queryClient }
}

function attributionLink(): HTMLElement | null {
  return (
    screen
      .queryAllByRole('link', { name: 'New API' })
      .find((link) => link.getAttribute('href') === ATTRIBUTION_URL) ?? null
  )
}

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})

it('加载失败时显示错误状态与重试，而不是空状态', async () => {
  api.defaults.adapter = async (config) => {
    throw new AxiosError('Network Error', 'ERR_NETWORK', config)
  }
  const { view, queryClient } = renderAbout()

  expect(
    await screen.findByText('Failed to load the about page')
  ).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  expect(screen.queryByText('No site introduction yet')).not.toBeInTheDocument()
  expect(attributionLink()).not.toBeNull()

  view.unmount()
  queryClient.clear()
})

it('管理员未配置内容时保留完整介绍版式并说明未配置', async () => {
  stubAboutResponse({ success: true, message: '', data: '' })
  const { view, queryClient } = renderAbout()

  expect(
    await screen.findByText('Developer-facing API access service')
  ).toBeInTheDocument()
  expect(screen.getByText('Unified API access')).toBeInTheDocument()
  expect(screen.getByText('Get started in three steps')).toBeInTheDocument()
  expect(screen.getByText('Service scope and privacy')).toBeInTheDocument()
  expect(screen.getByText('No site introduction yet')).toBeInTheDocument()
  expect(attributionLink()).not.toBeNull()

  view.unmount()
  queryClient.clear()
})

it('单行文本作为站点介绍正文渲染并保留归属信息', async () => {
  stubAboutResponse({
    success: true,
    message: '',
    data: 'A single line about this gateway.',
  })
  const { view, queryClient } = renderAbout()

  expect(await screen.findByText('Site introduction')).toBeInTheDocument()
  expect(
    screen.getByText('A single line about this gateway.')
  ).toBeInTheDocument()
  expect(screen.queryByText('No site introduction yet')).not.toBeInTheDocument()
  expect(attributionLink()).not.toBeNull()

  view.unmount()
  queryClient.clear()
})

it('Markdown 内容渲染在站点介绍区域', async () => {
  stubAboutResponse({
    success: true,
    message: '',
    data: '# Gateway headline\n\nBody paragraph.',
  })
  const { view, queryClient } = renderAbout()

  expect(
    await screen.findByRole('heading', { name: 'Gateway headline' })
  ).toBeInTheDocument()
  expect(screen.getByText('Body paragraph.')).toBeInTheDocument()
  expect(attributionLink()).not.toBeNull()

  view.unmount()
  queryClient.clear()
})

it('隔离 HTML 模式不套用模板且保留归属信息', async () => {
  stubAboutResponse({
    success: true,
    message: '',
    data: '<div class="custom-about">Custom HTML body</div>',
  })
  const { view, queryClient } = renderAbout()

  await waitFor(() =>
    expect(attributionLink()).not.toBeNull()
  )
  expect(screen.queryByText('Site introduction')).not.toBeInTheDocument()
  expect(screen.queryByText('Unified API access')).not.toBeInTheDocument()
  expect(view.container.querySelector('iframe')).toBeNull()
  // 隔离变体挂载在 shadow root 中，正文不会进入页面 DOM。
  const hosts = [...view.container.querySelectorAll('div')]
  expect(hosts.some((element) => element.shadowRoot !== null)).toBe(true)

  view.unmount()
  queryClient.clear()
})

it('URL 模式保留 iframe sandbox 且保留归属信息', async () => {
  stubAboutResponse({
    success: true,
    message: '',
    data: 'https://example.com/about-page',
  })
  const { view, queryClient } = renderAbout()

  const frame = await screen.findByTitle('About')
  expect(frame).toHaveAttribute('src', 'https://example.com/about-page')
  expect(frame).toHaveAttribute(
    'sandbox',
    'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
  )
  expect(screen.queryByText('Site introduction')).not.toBeInTheDocument()
  expect(attributionLink()).not.toBeNull()

  view.unmount()
  queryClient.clear()
})

it('重试按钮在恢复后重新渲染介绍内容', async () => {
  let aboutFails = true
  api.defaults.adapter = async (config) => {
    // 只让 /api/about 失败：Header 会同时请求 /api/notice。
    if (aboutFails && String(config.url).includes('/api/about')) {
      aboutFails = false
      throw new AxiosError('Network Error', 'ERR_NETWORK', config)
    }
    return {
      config,
      data: { success: true, message: '', data: 'Recovered introduction.' },
      status: 200,
      statusText: 'OK',
      headers: {},
    }
  }
  const { view, queryClient } = renderAbout()
  const user = userEvent.setup()

  await screen.findByText('Failed to load the about page')
  await user.click(screen.getByRole('button', { name: 'Retry' }))

  expect(
    await screen.findByText('Recovered introduction.')
  ).toBeInTheDocument()

  view.unmount()
  queryClient.clear()
})
