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
  act,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type AxiosAdapter } from 'axios'
import type { PropsWithChildren } from 'react'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'

import { updateSystemOption } from '../../api'
import { FAQSection } from '../../content/faq-section'
import { useUpdateOption } from '../use-update-option'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

/** 仅替换外部传输层；真实保存 API、请求及响应拦截器均参与测试。 */
const transport = vi.fn<AxiosAdapter>()
/** 保存测试前的传输配置，结束时恢复，避免影响其他测试。 */
const originalAdapter = api.defaults.adapter
/** 每个用例独立设置的服务端业务响应。 */
let responseBody: { success: boolean; message: string; code?: string }

/** 创建隔离查询缓存，保存请求通过模拟传输层，不接触真实 API。 */
function setup() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  /** 为被测钩子提供独立缓存，避免污染其他用例。 */
  function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
  return {
    ...renderHook(() => useUpdateOption(), { wrapper: Wrapper }),
    invalidate,
  }
}

beforeEach(() => {
  responseBody = { success: true, message: '' }
  transport.mockReset()
  transport.mockImplementation(async (config) => ({
    data: responseBody,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }))
  api.defaults.adapter = transport
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
})

describe('保存配置兼容官方刷新与定制提示', () => {
  test('连续成功保存使用同一个提示 ID，布尔值仍转换为字符串', async () => {
    const { result } = setup()
    await act(async () => {
      await result.current.mutateAsync({ key: 'Notice', value: true })
      await result.current.mutateAsync({ key: 'Notice', value: false })
    })
    expect(JSON.parse(transport.mock.calls[0][0].data)).toEqual({
      key: 'Notice',
      value: 'true',
    })
    expect(JSON.parse(transport.mock.calls[1][0].data)).toEqual({
      key: 'Notice',
      value: 'false',
    })
    expect(toast.success).toHaveBeenCalledTimes(2)
    expect(
      vi.mocked(toast.success).mock.calls.map((call) => call[1]?.id)
    ).toEqual(['system-setting-updated', 'system-setting-updated'])
  })

  test('silent 不发送至 API，不显示内建成功提示，保留官方数字输入', async () => {
    const { result } = setup()
    await act(async () => {
      await result.current.mutateAsync({
        key: 'QuotaPerUnit',
        value: 500000,
        silent: true,
      })
    })
    expect(JSON.parse(transport.mock.calls[0][0].data)).toEqual({
      key: 'QuotaPerUnit',
      value: 500000,
    })
    expect(toast.success).not.toHaveBeenCalled()
  })

  test.each([
    'oidc.display_name',
    'HeaderNavModules',
    'console_setting.api_info',
    'server_address',
  ])('配置 %s 同步失效查询与旧公开状态缓存', async (key) => {
    localStorage.setItem('status', 'stale')
    const { result, invalidate } = setup()
    await act(async () => {
      await result.current.mutateAsync({ key, value: 'updated' })
    })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['system-options'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['status'] })
    expect(localStorage.getItem('status')).toBeNull()
  })

  test('silent 保存的业务失败仍显示错误而不是成功', async () => {
    responseBody = { success: false, message: 'Rejected' }
    const { result, invalidate } = setup()
    await act(async () => {
      await expect(
        result.current.mutateAsync({
          key: 'Notice',
          value: '',
          silent: true,
        })
      ).rejects.toThrow('Rejected')
    })
    expect(toast.error).toHaveBeenCalledExactlyOnceWith('Rejected')
    expect(toast.success).not.toHaveBeenCalled()
    expect(invalidate).not.toHaveBeenCalled()
  })

  test('网络错误保留上下文并只显示一次错误', async () => {
    transport.mockImplementationOnce(async (config) => {
      throw new AxiosError('Disconnected', 'ERR_NETWORK', config)
    })
    const { result } = setup()
    await act(async () => {
      await expect(
        result.current.mutateAsync({ key: 'Notice', value: '', silent: true })
      ).rejects.toThrow('Disconnected')
    })
    expect(toast.error).toHaveBeenCalledExactlyOnceWith('Disconnected')
    expect(toast.success).not.toHaveBeenCalled()
  })
})

describe('真实拦截器下的保存错误归属', () => {
  test('保存请求保留认证、缓存及 JSON headers，不影响其他 API 的错误提示', async () => {
    useAuthStore.setState((state) => ({
      auth: { ...state.auth, accessToken: 'test-only-token' },
    }))
    const { result } = setup()
    await act(async () => {
      await result.current.mutateAsync({ key: 'Notice', value: 'test' })
    })
    const request = transport.mock.calls[0][0]
    expect(request.url).toBe('/api/option/')
    expect(request.method).toBe('put')
    expect(request.withCredentials).toBe(true)
    expect(request.headers.get('Authorization')).toBe('Bearer test-only-token')
    expect(request.headers.get('Cache-Control')).toBe('no-cache, no-store')
    expect(request.headers.get('Content-Type')).toBe('application/json')
    expect(request.headers.get('Accept')).toBe(
      'application/json, text/plain, */*'
    )
    expect(request.skipAuthRefresh).toBeUndefined()
    responseBody = { success: false, message: 'Other request rejected' }
    await api.post('/test-unrelated-api')
    expect(toast.error).toHaveBeenCalledExactlyOnceWith(
      'Other request rejected'
    )
  })

  test('直接保存 API 保留 success:false 原始返回协议', async () => {
    responseBody = { success: false, message: 'Rejected' }
    await expect(
      updateSystemOption({ key: 'Notice', value: '' })
    ).resolves.toEqual(responseBody)
    expect(toast.error).not.toHaveBeenCalled()
  })

  test.each([
    {
      name: 'HTTP 403 拒绝',
      body: { success: false, message: 'Permission denied' },
      expected: 'Permission denied',
    },
    {
      name: 'HTTP 错误码翻译',
      body: {
        success: false,
        message: 'raw server message',
        code: 'AUTH_INTERNAL_ERROR',
      },
      expected: 'Please try again later.',
    },
  ])(
    '$name 保留服务端原因或既有翻译，并仅提示一次',
    async ({ body, expected }) => {
      transport.mockImplementationOnce(async (config) => {
        throw new AxiosError(
          'Request failed with status code 403',
          'ERR_BAD_REQUEST',
          config,
          undefined,
          {
            data: body,
            status: 403,
            statusText: 'Forbidden',
            headers: {},
            config,
          }
        )
      })
      const { result, invalidate } = setup()
      await act(async () => {
        await expect(
          result.current.mutateAsync({ key: 'Notice', value: '', silent: true })
        ).rejects.toMatchObject({ response: { status: 403 } })
      })
      expect(toast.error).toHaveBeenCalledExactlyOnceWith(expected)
      expect(toast.success).not.toHaveBeenCalled()
      expect(invalidate).not.toHaveBeenCalled()
    }
  )

  test.each([
    {
      body: { success: false, message: 'raw', code: 'AUTH_INTERNAL_ERROR' },
      expected: 'Please try again later.',
    },
    {
      body: { success: false, message: '' },
      expected: 'Failed to update setting',
    },
  ])(
    '业务失败保留错误码翻译或空消息 fallback：$expected',
    async ({ body, expected }) => {
      responseBody = body
      const { result } = setup()
      await act(async () => {
        await expect(
          result.current.mutateAsync({ key: 'Notice', value: '' })
        ).rejects.toThrow(expected)
      })
      expect(toast.error).toHaveBeenCalledExactlyOnceWith(expected)
      expect(toast.success).not.toHaveBeenCalled()
    }
  )
})

test('真实 FAQ 页面保存被业务拒绝后保留草稿和保存按钮，成功重试后才标记已保存', async () => {
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  render(
    <QueryClientProvider client={client}>
      <FAQSection enabled data='[]' />
    </QueryClientProvider>
  )
  await user.click(screen.getByRole('button', { name: 'Add FAQ' }))
  const dialog = screen.getByRole('dialog')
  await user.type(
    within(dialog).getByRole('textbox', { name: 'Question' }),
    'Draft question'
  )
  await user.type(
    within(dialog).getByRole('textbox', { name: 'Answer' }),
    'Draft answer'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Add' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  vi.mocked(toast.success).mockClear()
  responseBody = { success: false, message: 'FAQ rejected by server' }
  await user.click(screen.getByRole('button', { name: 'Save Settings' }))
  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledExactlyOnceWith(
      'FAQ rejected by server'
    )
  )
  expect(screen.getByRole('button', { name: 'Save Settings' })).toBeEnabled()
  expect(
    screen.getByRole('cell', { name: 'Draft question' })
  ).toBeInTheDocument()
  expect(screen.getByRole('cell', { name: 'Draft answer' })).toBeInTheDocument()
  expect(toast.success).not.toHaveBeenCalled()
  expect(invalidate).not.toHaveBeenCalled()
  expect(transport).toHaveBeenCalledTimes(1)
  responseBody = { success: true, message: '' }
  await user.click(screen.getByRole('button', { name: 'Save Settings' }))
  await waitFor(() =>
    expect(toast.success).toHaveBeenCalledExactlyOnceWith(
      'FAQ saved successfully'
    )
  )
  expect(screen.getByRole('button', { name: 'Save Settings' })).toBeDisabled()
  expect(transport).toHaveBeenCalledTimes(2)
  expect(transport.mock.calls[1][0].data).toBe(transport.mock.calls[0][0].data)
  const payload = JSON.parse(transport.mock.calls[1][0].data)
  expect(payload.key).toBe('console_setting.faq')
  expect(JSON.parse(payload.value)).toEqual([
    { id: 1, question: 'Draft question', answer: 'Draft answer' },
  ])
})
