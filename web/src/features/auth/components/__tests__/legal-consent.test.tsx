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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { LegalConsent } from '../legal-consent'
import { TermsFooter } from '../terms-footer'

/** 仅替换 HTTP 边界，弹窗、正文和勾选行为使用实际组件。 */
const originalAdapter = api.defaults.adapter
const agreementTitle = 'API Service, Privacy and Usage Responsibility Agreement'

/** 渲染默认未同意的登录协议区域，并保留表单提交监测。 */
function renderConsent() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const onCheckedChange = vi.fn()
  const onSubmit = vi.fn((event) => event.preventDefault())
  const view = render(
    <QueryClientProvider client={queryClient}>
      <form onSubmit={onSubmit}>
        <LegalConsent
          status={{
            legal_consent: { version: '2026-09-16', required: true },
            user_agreement_enabled: true,
            privacy_policy_enabled: true,
          }}
          checked={false}
          onCheckedChange={onCheckedChange}
        />
      </form>
    </QueryClientProvider>
  )
  return { ...view, queryClient, onCheckedChange, onSubmit }
}

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

it('点击协议名称在当前表单打开正文弹窗，不跳转、提交或改变同意状态，Esc 后焦点回到入口', async () => {
  const requests: string[] = []
  api.defaults.adapter = (async (config) => {
    requests.push(config.url ?? '')
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: {
        success: true,
        data: '## Service scope\n\nRead the complete terms here.',
      },
    }
  }) satisfies AxiosAdapter
  const user = userEvent.setup()
  const view = renderConsent()
  const initialUrl = window.location.href
  expect(requests).toEqual([])
  const trigger = screen.getByRole('button', { name: agreementTitle })
  await user.click(trigger)
  const dialog = await screen.findByRole('dialog', { name: agreementTitle })
  expect(
    await within(dialog).findByRole('heading', { name: 'Service scope' })
  ).toBeVisible()
  expect(requests).toEqual(['/api/api-service-agreement'])
  expect(window.location.href).toBe(initialUrl)
  expect(view.onSubmit).not.toHaveBeenCalled()
  expect(view.onCheckedChange).not.toHaveBeenCalled()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  await waitFor(() => expect(trigger).toHaveFocus())
  expect(screen.getByRole('checkbox')).not.toBeChecked()
  view.unmount()
  view.queryClient.clear()
})

it.each([
  ['User Agreement', '/api/user-agreement'],
  ['Privacy Policy', '/api/privacy-policy'],
])('相关文档 %s 在弹窗显示对应正文并可关闭', async (title, url) => {
  api.defaults.adapter = (async (config) => {
    expect(config.url).toBe(url)
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: { success: true, data: `${title} full text` },
    }
  }) satisfies AxiosAdapter
  const user = userEvent.setup()
  const view = renderConsent()
  await user.click(screen.getByRole('button', { name: title }))
  const dialog = await screen.findByRole('dialog', { name: title })
  expect(await within(dialog).findByText(`${title} full text`)).toBeVisible()
  await user.click(within(dialog).getByText('Close', { selector: 'button' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(view.onCheckedChange).not.toHaveBeenCalled()
  view.unmount()
  view.queryClient.clear()
})

it('正文加载失败显示可重试的错误，重试后显示正文', async () => {
  let failed = true
  api.defaults.adapter = (async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: failed
      ? { success: false }
      : { success: true, data: 'Recovered agreement text' },
  })) satisfies AxiosAdapter
  const user = userEvent.setup()
  const view = renderConsent()
  await user.click(screen.getByRole('button', { name: agreementTitle }))
  await screen.findByText('The agreement text is currently unavailable.')
  failed = false
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Recovered agreement text')).toBeVisible()
  view.unmount()
  view.queryClient.clear()
})

it('注册页文档入口只供阅读，不声明注册即同意，也不跳转页面', async () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  api.defaults.adapter = (async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: { success: true, data: 'User terms' },
  })) satisfies AxiosAdapter
  const view = render(
    <QueryClientProvider client={queryClient}>
      <TermsFooter
        variant='sign-up'
        status={{ user_agreement_enabled: true }}
      />
    </QueryClientProvider>
  )
  expect(screen.queryByText(/By creating an account/)).not.toBeInTheDocument()
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'User Agreement' }))
  expect(await screen.findByText('User terms')).toBeVisible()
  view.unmount()
  queryClient.clear()
})
