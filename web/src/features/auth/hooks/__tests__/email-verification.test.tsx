import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type AxiosAdapter } from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { SignUpForm } from '../../sign-up/components/sign-up-form'
import { useEmailVerification } from '../use-email-verification'

/** 保存请求适配器，测试只替换网络边界并在结束时恢复。 */
const originalAdapter = api.defaults.adapter

beforeEach(async () => {
  i18next.addResourceBundle('zhCN', 'translation', zh.translation, true, true)
  await i18next.changeLanguage('zhCN')
})

afterEach(async () => {
  api.defaults.adapter = originalAdapter
  await i18next.changeLanguage('en')
  vi.restoreAllMocks()
})

it.each([
  [
    'reader@example.com',
    '',
    '当前邮箱不支持，请使用 QQ 邮箱（例如：123456@qq.com）。',
  ],
  [
    'reader@example.com',
    "This email address is not allowed by the administrator's email policy.",
    '当前邮箱不支持，请使用 QQ 邮箱（例如：123456@qq.com）。',
  ],
])(
  '邮箱 %s 被拒绝时只显示一次中文提示且不开始倒计时',
  async (email, message, expected) => {
    const errorToast = vi.spyOn(toast, 'error')
    api.defaults.adapter = async (config) => ({
      config,
      data: { success: false, code: 'EMAIL_ADDRESS_REJECTED', message },
      status: 200,
      statusText: 'OK',
      headers: {},
    })
    const { result } = renderHook(() => useEmailVerification())
    await act(async () => {
      expect(await result.current.sendCode(email)).toBe(false)
    })
    expect(errorToast).toHaveBeenCalledExactlyOnceWith(expected)
    expect(result.current.isActive).toBe(false)
    expect(result.current.isSending).toBe(false)
  }
)

it('网络失败时显示中文重试提示且恢复发送状态', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  api.defaults.adapter = async (config) => {
    throw new AxiosError('Network Error', 'ERR_NETWORK', config)
  }
  const { result } = renderHook(() => useEmailVerification())
  await act(async () => {
    expect(await result.current.sendCode('123456@qq.com')).toBe(false)
  })
  expect(errorToast).toHaveBeenCalledExactlyOnceWith('发送验证邮件失败')
  expect(result.current.isActive).toBe(false)
  expect(result.current.isSending).toBe(false)
})

it('QQ 邮箱发送成功后显示中文提示并开始倒计时', async () => {
  const successToast = vi.spyOn(toast, 'success')
  const adapter = vi.fn<AxiosAdapter>(async (config) => ({
    config,
    data: { success: true, message: '' },
    status: 200,
    statusText: 'OK',
    headers: {},
  }))
  api.defaults.adapter = adapter
  const { result } = renderHook(() => useEmailVerification())
  await act(async () => {
    expect(await result.current.sendCode('123456@qq.com')).toBe(true)
  })
  expect(successToast).toHaveBeenCalledExactlyOnceWith('验证邮件已发送')
  expect(result.current.isActive).toBe(true)
  expect(adapter.mock.calls[0][0].params.email).toBe('123456@qq.com')
})

it('注册页提示使用 QQ 邮箱，输入其他邮箱后点击发码会显示中文原因', async () => {
  const errorToast = vi.spyOn(toast, 'error')
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {
    email_verification: true,
    oauth_register_enabled: false,
  })
  api.defaults.adapter = async (config) => ({
    config,
    data: {
      success: false,
      code: 'EMAIL_ADDRESS_REJECTED',
      message:
        "This email address is not allowed by the administrator's email policy.",
    },
    status: 200,
    statusText: 'OK',
    headers: {},
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: SignUpForm }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  try {
    const user = userEvent.setup()
    const email = await screen.findByRole('textbox', {
      name: i18next.t('Email (required for verification)'),
    })
    expect(email).toHaveAttribute(
      'placeholder',
      '请使用 QQ 邮箱（例如：123456@qq.com）'
    )
    await user.type(email, 'reader@example.com')
    await user.click(screen.getByRole('button', { name: '发送验证码' }))
    expect(errorToast).toHaveBeenCalledExactlyOnceWith(
      '当前邮箱不支持，请使用 QQ 邮箱（例如：123456@qq.com）。'
    )
  } finally {
    view.unmount()
    queryClient.clear()
  }
})
