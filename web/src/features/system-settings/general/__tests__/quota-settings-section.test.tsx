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
import type { AxiosAdapter } from 'axios'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/http-client'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { DocsLinkSection } from '../../maintenance/docs-link-section'
import { QuotaSettingsSection } from '../quota-settings-section'

/** 额度表单只保存计费字段，文档链接由公告页独立管理。 */
const defaultValues = {
  QuotaForNewUser: 0,
  PreConsumedQuota: 0,
  QuotaForInviter: 0,
  QuotaForInvitee: 0,
  TopUpLink: '',
  quota_setting: { enable_free_model_pre_consume: true },
}

/** 隔离请求传输，保留实际表单校验、路由和保存钩子。 */
const transport = vi.fn<AxiosAdapter>()
const originalAdapter = api.defaults.adapter
let client: QueryClient

/** 提供与设置页一致的保存操作容器。 */
function QuotaPage() {
  const [actionsContainer, setActionsContainer] =
    useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setActionsContainer} />
      <SettingsPageProvider actionsContainer={actionsContainer}>
        <QuotaSettingsSection defaultValues={defaultValues} />
      </SettingsPageProvider>
    </>
  )
}

/** 装载额度设置和公告文档设置，实际验证客户端导航。 */
async function renderQuotaPage() {
  const root = createRootRoute()
  const quota = createRoute({
    getParentRoute: () => root,
    path: '/system-settings/billing/quota',
    component: QuotaPage,
  })
  const notice = createRoute({
    getParentRoute: () => root,
    path: '/system-settings/site/$section',
    component: () => (
      <DocsLinkSection defaultValue='https://docs.example.com' />
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([quota, notice]),
    history: createMemoryHistory({
      initialEntries: ['/system-settings/billing/quota'],
    }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  transport.mockImplementation(async (config) => ({
    data: { success: true, message: '' },
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }))
  api.defaults.adapter = transport
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  client.clear()
})

test('额度页不再编辑文档链接，公告入口可导航到唯一编辑区域', async () => {
  const user = userEvent.setup()
  const router = await renderQuotaPage()
  expect(
    screen.queryByRole('textbox', { name: 'Documentation Link' })
  ).not.toBeInTheDocument()
  const link = screen.getByRole('link', { name: 'System Notice' })
  expect(link).toHaveAttribute('href', '/system-settings/site/notice')
  await user.click(link)
  expect(
    await screen.findByRole('textbox', { name: 'Documentation Link' })
  ).toHaveValue('https://docs.example.com')
  expect(router.state.location.pathname).toBe('/system-settings/site/notice')
  expect(transport).not.toHaveBeenCalled()
})

test('没有文档链接默认值时仍能保存额度，且只提交修改的计费字段', async () => {
  const user = userEvent.setup()
  await renderQuotaPage()
  const quota = screen.getByRole('spinbutton', { name: 'New User Quota' })
  await user.clear(quota)
  await user.type(quota, '250000')
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(transport).toHaveBeenCalledTimes(1))
  expect(JSON.parse(String(transport.mock.calls[0][0].data))).toEqual({
    key: 'QuotaForNewUser',
    value: 250000,
  })
})
