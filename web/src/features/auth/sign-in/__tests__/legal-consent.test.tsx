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
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { UserAuthForm } from '../components/user-auth-form'

/** 保存请求适配器，测试只替换网络边界并在结束时恢复。 */
const originalAdapter = api.defaults.adapter

/** 服务端当前生效的协议版本，由 /api/status 的 legal_consent 返回。 */
const CONSENT_VERSION = '2026-09-16'

/** 协议版本未知时的提示，与 legal-consent.tsx 使用同一 key。 */
const AGREEMENT_UNAVAILABLE =
  'The agreement requirement could not be loaded. Check your connection and reload the page.'

async function renderSignIn(status: Record<string, unknown>) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {
    password_login_enabled: true,
    legal_consent: { version: CONSENT_VERSION, required: true },
    ...status,
  })
  const rootRoute = createRootRoute()
  const routeTree = rootRoute.addChildren([
    createRoute({
      getParentRoute: () => rootRoute,
      path: '/',
      component: UserAuthForm,
    }),
    createRoute({
      getParentRoute: () => rootRoute,
      path: '/forgot-password',
      component: () => null,
    }),
    createRoute({
      getParentRoute: () => rootRoute,
      path: '/otp',
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

async function fillCredentials(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Username or Email'), 'enrollment-user')
  await user.type(screen.getByLabelText('Password'), 'enrollment-password')
}

function signInButton(): HTMLElement {
  return screen.getByRole('button', { name: 'Sign in' })
}

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('登录页默认未勾选协议，点击登录先提示同意且不发请求', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  const adapter = vi.fn<AxiosAdapter>()
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignIn({})
  const user = userEvent.setup()

  expect(screen.getByRole('checkbox')).not.toBeChecked()
  expect(signInButton()).toBeEnabled()
  await user.click(signInButton())
  expect(errorToast).toHaveBeenCalledWith(
    'Please read and agree to the agreement before signing in.'
  )
  expect(adapter).not.toHaveBeenCalled()

  view.unmount()
  queryClient.clear()
})

it('勾选后取消协议，再次点击登录仍提示同意且不发请求', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  const adapter = vi.fn<AxiosAdapter>()
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignIn({})
  const user = userEvent.setup()

  await user.click(screen.getByRole('checkbox'))
  expect(screen.getByRole('checkbox')).toBeChecked()
  expect(signInButton()).toBeEnabled()

  await user.click(screen.getByRole('checkbox'))
  expect(screen.getByRole('checkbox')).not.toBeChecked()
  expect(signInButton()).toBeEnabled()
  await user.click(signInButton())
  expect(errorToast).toHaveBeenCalledWith(
    'Please read and agree to the agreement before signing in.'
  )
  expect(adapter).not.toHaveBeenCalled()

  view.unmount()
  queryClient.clear()
})

it('协议要求未知时点击登录提示重新加载且不发请求', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  const adapter = vi.fn<AxiosAdapter>()
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignIn({ legal_consent: undefined })

  expect(screen.getByText(AGREEMENT_UNAVAILABLE)).toBeInTheDocument()
  expect(signInButton()).toBeEnabled()
  await userEvent.setup().click(signInButton())
  expect(errorToast).toHaveBeenCalledWith(AGREEMENT_UNAVAILABLE)
  expect(adapter).not.toHaveBeenCalled()

  view.unmount()
  queryClient.clear()
})

it('未勾选协议时回车不会发起登录请求', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  const adapter = vi.fn<AxiosAdapter>(async (config) => ({
    config,
    data: { success: false, message: 'unused' },
    status: 200,
    statusText: 'OK',
    headers: {},
  }))
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignIn({})
  const user = userEvent.setup()

  await fillCredentials(user)
  await user.keyboard('{Enter}')

  expect(errorToast).toHaveBeenCalledWith(
    'Please read and agree to the agreement before signing in.'
  )
  expect(adapter).not.toHaveBeenCalled()

  view.unmount()
  queryClient.clear()
})

it('勾选协议后回车提交，登录请求携带 consent 与 consent_version', async () => {
  const adapter = vi.fn<AxiosAdapter>(async (config) => ({
    config,
    data: { success: false, message: 'invalid credentials' },
    status: 200,
    statusText: 'OK',
    headers: {},
  }))
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignIn({})
  const user = userEvent.setup()

  await fillCredentials(user)
  await user.click(screen.getByRole('checkbox'))
  await user.keyboard('{Enter}')

  await waitFor(() => expect(adapter).toHaveBeenCalledTimes(1))
  const body = JSON.parse(String(adapter.mock.calls[0][0].data)) as Record<
    string,
    unknown
  >
  expect(body.username).toBe('enrollment-user')
  expect(body.consent).toBe(true)
  expect(body.consent_version).toBe(CONSENT_VERSION)

  view.unmount()
  queryClient.clear()
})

it('未勾选协议时第三方与微信登录提示同意且不发请求', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  const adapter = vi.fn<AxiosAdapter>()
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignIn({
    github_oauth: true,
    github_client_id: 'client-id',
    wechat_login: true,
  })
  const user = userEvent.setup()

  const githubButton = await screen.findByRole('button', {
    name: /Continue with GitHub/,
  })
  expect(githubButton).toBeEnabled()
  await user.click(githubButton)
  await user.click(screen.getByRole('button', { name: /Continue with WeChat/ }))
  expect(errorToast).toHaveBeenCalledTimes(2)
  expect(errorToast).toHaveBeenLastCalledWith(
    'Please read and agree to the agreement before signing in.'
  )
  expect(adapter).not.toHaveBeenCalled()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

  await user.click(screen.getByRole('checkbox'))
  expect(githubButton).toBeEnabled()

  view.unmount()
  queryClient.clear()
})

it('设备支持 Passkey 但未同意协议时，点击仅提示且不请求认证', async () => {
  vi.stubGlobal('PublicKeyCredential', class {})
  const errorToast = vi.spyOn(toast, 'error')
  const adapter = vi.fn<AxiosAdapter>()
  api.defaults.adapter = adapter
  const { view, queryClient } = await renderSignIn({ passkey_login: true })
  const passkeyButton = screen.getByRole('button', {
    name: 'Sign in with Passkey',
  })
  await waitFor(() => expect(passkeyButton).toBeEnabled())

  await userEvent.setup().click(passkeyButton)
  expect(errorToast).toHaveBeenCalledWith(
    'Please read and agree to the agreement before signing in.'
  )
  expect(adapter).not.toHaveBeenCalled()

  view.unmount()
  queryClient.clear()
})

it('协议已更新时提示刷新页面重新同意，而不是通用失败', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  api.defaults.adapter = async (config) => ({
    config,
    data: {
      success: false,
      code: 'legal_consent_outdated',
      message: '协议已更新，请刷新页面后重新阅读并勾选同意',
    },
    status: 200,
    statusText: 'OK',
    headers: {},
  })
  const { view, queryClient } = await renderSignIn({})
  const user = userEvent.setup()

  await fillCredentials(user)
  await user.click(screen.getByRole('checkbox'))
  await user.click(signInButton())

  await waitFor(() =>
    expect(errorToast).toHaveBeenCalledWith(
      expect.stringMatching(/agreement has been updated/i)
    )
  )
  expect(errorToast).toHaveBeenCalledWith(
    expect.stringMatching(/refresh the page/i)
  )
  expect(errorToast).not.toHaveBeenCalledWith(
    expect.stringContaining('协议已更新')
  )

  view.unmount()
  queryClient.clear()
})
