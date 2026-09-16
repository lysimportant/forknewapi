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
import { act, renderHook, waitFor } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useNotificationStore } from '@/stores/notification-store'

import { useNotifications } from '../use-notifications'

const announcements = Array.from({ length: 25 }, (_, index) => ({
  id: index + 1,
  content: `announcement ${index + 1}`,
  publishDate: new Date(Date.UTC(2026, 8, 1, 10, index)).toISOString(),
  ...(index === 24 ? { pinned: true } : {}),
}))

let client: QueryClient
let statusBody: Record<string, unknown>

function setup() {
  /** 为被测钩子提供独立缓存，状态请求由模拟传输层返回 */
  function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
  return renderHook(() => useNotifications(), { wrapper: Wrapper })
}

beforeEach(() => {
  window.localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  statusBody = { announcements_enabled: true, announcements }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return { data: { data: statusBody } }
    }
    if (url === '/api/notice') {
      return { data: { success: true, message: '', data: '' } }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  client.clear()
  window.localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
})

describe('公告弹窗自动弹出', () => {
  it('进入控制台后自动弹出一次，并保留通知列表的前 20 条截断', async () => {
    const { result } = setup()

    await waitFor(() => expect(result.current.timelineOpen).toBe(true))
    expect(result.current.allAnnouncements).toHaveLength(25)
    expect(result.current.announcements).toHaveLength(20)
    // 通知列表同样置顶优先
    expect(result.current.announcements[0].id).toBe(25)
  })

  it('展示开关关闭或无公告时不弹出空弹窗', async () => {
    statusBody = { announcements_enabled: false, announcements }
    const disabled = setup()
    await waitFor(() => expect(disabled.result.current.loading).toBe(false))
    expect(disabled.result.current.timelineOpen).toBe(false)

    statusBody = { announcements_enabled: true, announcements: [] }
    const empty = setup()
    await waitFor(() => expect(empty.result.current.loading).toBe(false))
    expect(empty.result.current.timelineOpen).toBe(false)
  })

  it('尊重「今日关闭」不强制弹出，但手动入口始终可用', async () => {
    useNotificationStore.getState().closeAnnouncementsForToday()
    const { result } = setup()

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.timelineOpen).toBe(false)

    act(() => result.current.openTimeline())
    expect(result.current.timelineOpen).toBe(true)
  })

  it('关闭本次后路由切换不再弹出，重新挂载（刷新）后恢复', async () => {
    const first = setup()
    await waitFor(() => expect(first.result.current.timelineOpen).toBe(true))

    act(() => first.result.current.setTimelineOpen(false))
    expect(first.result.current.timelineOpen).toBe(false)
    expect(useNotificationStore.getState().canAutoOpenAnnouncements()).toBe(
      false
    )

    await act(async () => {
      useNotificationStore.setState(
        useNotificationStore.getInitialState(),
        true
      )
    })
    const second = setup()
    await waitFor(() => expect(second.result.current.timelineOpen).toBe(true))
  })
})
