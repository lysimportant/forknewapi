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
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route } from '@/routes/_authenticated/usage-logs/$section'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { UsageLogsProvider } from '../usage-logs-provider'

/** 渲染真实筛选栏与统计组件，使用空表隔离列表展示。 */
function FilterFixture() {
  const table = useReactTable({
    data: [],
    columns: [],
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <UsageLogsProvider>
      <CommonLogsFilterBar table={table} />
    </UsageLogsProvider>
  )
}

/** 使用指定 URL 和真实路由校验初始化筛选栏，仅模拟 API 边界。 */
async function renderFilter(initialEntry = '/usage-logs/common') {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/self/groups') {
      return {
        data: { success: true, data: { premium: { desc: '', ratio: 2 } } },
      }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['premium'] } }
    }
    return { data: { success: true, data: { quota: 0, rpm: 0, tpm: 0 } } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: FilterFixture,
    validateSearch: Route.options.validateSearch,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: [initialEntry] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('combobox', { name: 'Type' })
  return router
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.setUser(null)
})

it('marks only retired log types as deprecated while keeping historical filters selectable', async () => {
  const router = await renderFilter()
  await userEvent.click(screen.getByRole('combobox', { name: 'Type' }))
  for (const label of ['Manage', 'Login']) {
    expect(
      within(
        screen.getByRole('option', { name: new RegExp(`^${label}`) })
      ).getByText('Deprecated')
    ).toBeVisible()
  }
  for (const label of [
    'All Types',
    'Model mismatch',
    'Top-up',
    'Consume',
    'System',
    'Error',
    'Refund',
  ]) {
    expect(
      within(screen.getByRole('option', { name: label })).queryByText(
        'Deprecated'
      )
    ).not.toBeInTheDocument()
  }
  await userEvent.click(screen.getByRole('option', { name: /^Manage/ }))
  expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent(
    'Deprecated'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ type: ['3'], page: 1 })
  )
  await userEvent.click(screen.getByRole('combobox', { name: 'Type' }))
  await userEvent.click(screen.getByRole('option', { name: /^Login/ }))
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ type: ['7'], page: 1 })
  )
})

it.each([1, 10])(
  'applies model mismatch with the selected group on page one for role %s',
  async (role) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'viewer', role })
    const router = await renderFilter(
      '/usage-logs/common?page=3&type=%5B%222%22%5D&group=premium'
    )
    await userEvent.click(screen.getByRole('combobox', { name: 'Type' }))
    await userEvent.click(
      screen.getByRole('option', { name: 'Model mismatch' })
    )
    expect(router.state.location.search).toMatchObject({
      type: ['2'],
      group: 'premium',
      page: 3,
    })
    await userEvent.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        type: ['-1'],
        group: 'premium',
        page: 1,
      })
    )
    expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent(
      'Model mismatch'
    )
    await waitFor(() => {
      const statsPath = role === 10 ? '/api/log/stat' : '/api/log/self/stat'
      const request = vi
        .mocked(api.get)
        .mock.calls.map(([url]) => new URL(url, 'http://localhost'))
        .find(
          (url) =>
            url.pathname === statsPath && url.searchParams.get('type') === '-1'
        )
      expect(request?.searchParams.get('group')).toBe('premium')
    })
  }
)

it.each(['-1', '%5B%22-1%22%5D'])(
  'restores model mismatch from URL type=%s and resets type, group and page',
  async (type) => {
    const router = await renderFilter(
      `/usage-logs/common?page=3&group=premium&type=${type}`
    )
    expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent(
      'Model mismatch'
    )
    expect(screen.getByRole('combobox', { name: 'Group' })).toHaveValue(
      'premium'
    )
    await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        type: ['0'],
        page: 1,
      })
    )
    expect(router.state.location.search.group).toBeUndefined()
    expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent(
      'All Types'
    )
    expect(screen.getByRole('combobox', { name: 'Group' })).toHaveValue('')
  }
)
