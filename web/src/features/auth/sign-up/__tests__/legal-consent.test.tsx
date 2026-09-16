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
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { SignUpForm } from '../components/sign-up-form'
import { api } from '@/lib/api'

/** 保存请求适配器，测试只替换网络边界并在结束时恢复。 */
const originalAdapter = api.defaults.adapter

const CONSENT_VERSION = '2026-09-16'
const REGISTER_PASSWORD = 'enrollment-password'

async function renderSignUp(status: Record<string, unknown> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {
    legal_consent: { version: CONSENT_VERSION, required: true },
    ...status,
  })
  const rootRoute = createRootRoute()
  const routeTree = rootRoute.addChildren([
    createRoute({
      getParentRoute: () => rootRoute,
      path: '/',
      component: SignUpForm,
    }),
    createRoute({
      getParentRoute: () => rootRoute,
      path: '/sign-in',
      component: () => null,
    }),
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
  // RouterProvider 首次渲染是异步的，先等待表单挂载再使用同步查询。
  await screen.findByRole('checkbox')
  return { view, queryClient }
}

function createAccountButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Create account' })
}

async function fillRegistration(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Username'), 'enrollment-user')
  await user.type(screen.getByLabelText('Password'), REGISTER_PASSWORD)
  await user.type(screen.getByLabelText('Confirm password'), REGISTER_PASSWORD)
}

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})

it('注册页默认未勾选协议且提交按钮禁用', async () => {
  const { view, queryClient } = await renderSignUp({
    oauth_register_enabled: false,
  })

  expect(screen.getByRole('checkbox')).not.toBeChecked()
  expect(createAccountButton()).toBeDisabled()

  view.unmount()
  queryClient.clear()
})

it('勾选协议后启用注册，取消勾选后再次禁用', async () => {
  const { view, queryClient } = await renderSignUp({
    oauth_register_enabled: false,
  })
  const user = userEvent.setup()

  await user.click(screen.getByRole('checkbox'))
  expect(createAccountButton()).toBeEnabled()

  await user.click(screen.getByRole('checkbox'))
  expect(createAccountButton()).toBeDisabled()

  view.unmount()
  queryClient.clear()
})

it('未勾选协议时回车不会发起注册请求', async () => {
  const adapter = vi.fn<AxiosAdapter>(async (config) => ({
    config,
    data: { success: false, message: 'unused' },
    status: 200,
    statusText: 'OK',
    headers: {},
  }))
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignUp({
    oauth_register_enabled: false,
  })
  const user = userEvent.setup()

  await fillRegistration(user)
  await user.keyboard('{Enter}')

  expect(adapter).not.toHaveBeenCalled()

  view.unmount()
  queryClient.clear()
})

it('勾选协议后注册成功，请求携带 consent 与 consent_version', async () => {
  const adapter = vi.fn<AxiosAdapter>(async (config) => ({
    config,
    data: { success: true, message: 'ok' },
    status: 200,
    statusText: 'OK',
    headers: {},
  }))
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignUp({
    oauth_register_enabled: false,
  })
  const user = userEvent.setup()

  await fillRegistration(user)
  await user.click(screen.getByRole('checkbox'))
  await user.click(createAccountButton())

  await waitFor(() => expect(adapter).toHaveBeenCalledTimes(1))
  const call = adapter.mock.calls[0][0]
  expect(String(call.url)).toContain('/api/user/register')
  const body = JSON.parse(String(call.data)) as Record<string, unknown>
  expect(body.username).toBe('enrollment-user')
  expect(body.password).toBe(REGISTER_PASSWORD)
  expect(body.consent).toBe(true)
  expect(body.consent_version).toBe(CONSENT_VERSION)

  view.unmount()
  queryClient.clear()
})
