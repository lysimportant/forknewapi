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

import { api } from '@/lib/api'

import { About } from '../index'

/** 保存请求适配器，测试只替换网络边界并在结束时恢复。 */
const originalAdapter = api.defaults.adapter

/** 关于页在所有内容模式下保留项目、作者和许可证归属。 */
const ATTRIBUTION_LINKS = [
  ['NewAPI', 'https://github.com/QuantumNous/new-api'],
  ['QuantumNous', 'https://github.com/QuantumNous'],
  ['One API', 'https://github.com/songquanpeng/one-api'],
  ['JustSong', 'https://github.com/songquanpeng'],
  [
    'AGPL v3.0 License',
    'https://github.com/QuantumNous/new-api/blob/main/LICENSE',
  ],
] as const

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

/** 每个归属链接都必须可见、保持原目标并隔离新窗口。 */
function expectProjectAttribution(): void {
  for (const [name, href] of ATTRIBUTION_LINKS) {
    const link = screen.getByRole('link', { name })
    expect(link).toBeVisible()
    expect(link).toHaveAttribute('href', href)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  }
}

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})

it.each(['network', 'business'])(
  '%s 加载失败时显示错误与重试并保留完整归属，而不是空状态',
  async (failure) => {
    if (failure === 'network') {
      api.defaults.adapter = async (config) => {
        throw new AxiosError('Network Error', 'ERR_NETWORK', config)
      }
    } else {
      stubAboutResponse({ success: false, message: 'Unavailable' })
    }
    const { view, queryClient } = renderAbout()

    expect(
      await screen.findByText('Failed to load the about page')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
    expect(
      screen.queryByText('No site introduction yet')
    ).not.toBeInTheDocument()
    expectProjectAttribution()

    view.unmount()
    queryClient.clear()
  }
)

it('未配置正文时显示站点原有充值、远程协助和群联系方式', async () => {
  stubAboutResponse({ success: true, message: '', data: '' })
  const { view, queryClient } = renderAbout()

  expect(
    await screen.findByRole('heading', { name: 'Top-up assistance' })
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Remote setup assistance' })
  ).toBeVisible()
  expect(
    screen.getByRole('heading', { name: 'Questions and support' })
  ).toBeVisible()
  expect(screen.getByText('hkcustom0928')).toBeVisible()
  expect(screen.getByText('811481586')).toBeVisible()
  expect(
    screen.getByText(
      'Need help connecting Codex or ChatGPT? Contact the administrator for remote setup assistance.'
    )
  ).toBeVisible()
  expect(
    screen.queryByText('Get started in three steps')
  ).not.toBeInTheDocument()
  expect(screen.queryByText('No site introduction yet')).not.toBeInTheDocument()
  expectProjectAttribution()

  view.unmount()
  queryClient.clear()
})

it('原有运营正文使用优化版式展示，管理员账号和 QQ 群号可复制', async () => {
  const originalContent =
    '充值可以找管理：hkcustom0928 不会接入Codex ChatGPT的也可以找管理远程帮忙; 有问题请联系管理员； Q扣群：811481586'
  stubAboutResponse({ success: true, message: '', data: originalContent })
  const user = userEvent.setup()
  const { view, queryClient } = renderAbout()

  await screen.findByRole('heading', { name: 'Top-up assistance' })
  expect(screen.queryByText(originalContent)).not.toBeInTheDocument()
  expect(screen.getAllByText('hkcustom0928')).toHaveLength(1)
  expect(screen.getAllByText('811481586')).toHaveLength(1)
  await user.click(
    screen.getByRole('button', { name: 'Copy administrator account' })
  )
  expect(await navigator.clipboard.readText()).toBe('hkcustom0928')
  await user.click(screen.getByRole('button', { name: 'Copy QQ group number' }))
  expect(await navigator.clipboard.readText()).toBe('811481586')
  expectProjectAttribution()

  view.unmount()
  queryClient.clear()
})

it('隐私摘要保留不售卖承诺并明确必要的上游处理及其适用政策', async () => {
  stubAboutResponse({ success: true, message: '', data: '' })
  const { view, queryClient } = renderAbout()
  expect(
    await screen.findByText(
      'Personal information, account details, prompts, and model replies are not sold or rented. Content is sent to the selected upstream model service only to process your request; its data handling and retention policies apply.'
    )
  ).toBeVisible()
  expect(
    screen.queryByText(
      'Personal information, account details, prompts, and model replies are not sold, rented, or provided to others.'
    )
  ).not.toBeInTheDocument()

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
  expectProjectAttribution()

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
  expectProjectAttribution()

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

  await waitFor(() => {
    const roots = [...view.container.querySelectorAll('div')].map(
      (element) => element.shadowRoot
    )
    expect(
      roots.some((root) => root?.textContent?.includes('Custom HTML body'))
    ).toBe(true)
  })
  expectProjectAttribution()
  expect(screen.queryByText('Site introduction')).not.toBeInTheDocument()
  expect(screen.queryByText('Unified API access')).not.toBeInTheDocument()
  expect(view.container.querySelector('iframe')).toBeNull()
  expect(screen.queryByText('Custom HTML body')).not.toBeInTheDocument()

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
  expectProjectAttribution()

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

  expect(await screen.findByText('Recovered introduction.')).toBeInTheDocument()

  view.unmount()
  queryClient.clear()
})
