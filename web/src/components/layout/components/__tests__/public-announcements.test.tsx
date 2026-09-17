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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import { api } from '@/lib/api'
import { useNotificationStore } from '@/stores/notification-store'

import { PublicHeader } from '../public-header'

/** 每个用例独立创建网络边界与查询缓存，直接运行公共导航的公告交互。 */
let client: QueryClient

/** 通知面板的滚动组件会查询动画；JSDOM 不运行动画，仅在本测试提供空列表。 */
const animationDescriptor = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'getAnimations'
)

beforeAll(() => {
  Object.defineProperty(Element.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (animationDescriptor) {
    Object.defineProperty(
      Element.prototype,
      'getAnimations',
      animationDescriptor
    )
  } else {
    Reflect.deleteProperty(Element.prototype, 'getAnimations')
  }
})

/** 渲染与首页、关于页相同的公共导航，不模拟公告钩子或弹窗。 */
function renderPublicHeader(path: '/' | '/about') {
  const rootRoute = createRootRoute({
    component: () => (
      <PublicHeader
        showLanguageSwitcher={false}
        showThemeSwitch={false}
        showAuthButtons={false}
      />
    ),
  })
  const routeTree = rootRoute.addChildren([
    createRoute({ getParentRoute: () => rootRoute, path: '/' }),
    createRoute({ getParentRoute: () => rootRoute, path: '/about' }),
  ])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [path] }),
  })

  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

beforeEach(() => {
  window.localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return {
        data: {
          data: {
            announcements_enabled: true,
            announcements: [{ id: 1, content: 'Public entry announcement' }],
          },
        },
      }
    }
    if (url === '/api/notice') {
      return { data: { success: true, data: 'Legacy top-up contact notice' } }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  client.clear()
  window.localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
})

describe('公共页面公告入口', () => {
  it.each(['/', '/about'] as const)(
    '直接进入 %s 时自动展示公告 Dialog',
    async (path) => {
      renderPublicHeader(path)

      const dialog = await screen.findByRole('dialog', {
        name: 'System Announcements',
      })
      expect(
        within(dialog).getByText('Public entry announcement')
      ).toBeVisible()
      expect(
        within(dialog).queryByText('Legacy top-up contact notice')
      ).not.toBeInTheDocument()
    }
  )

  it('今日关闭抑制自动弹窗，但通知入口仍可手动查看全部公告', async () => {
    const user = userEvent.setup()
    useNotificationStore.getState().closeAnnouncementsForToday()
    renderPublicHeader('/about')

    const trigger = await screen.findByRole('button', { name: 'Notifications' })
    await waitFor(() => expect(client.isFetching()).toBe(0))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await user.click(trigger)
    await user.click(
      await screen.findByRole('button', { name: 'View All Announcements' })
    )

    expect(
      await screen.findByRole('dialog', { name: 'System Announcements' })
    ).toBeVisible()
  })
})
