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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/http-client'

import { AnnouncementsSection } from '../announcements-section'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

/** 仅替换外部传输层；真实保存 API 与请求拦截器均参与测试。 */
const transport = vi.fn<AxiosAdapter>()
const originalAdapter = api.defaults.adapter

let client: QueryClient

function renderSection(data: unknown) {
  return render(
    <QueryClientProvider client={client}>
      <AnnouncementsSection enabled data={JSON.stringify(data)} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  transport.mockReset()
  transport.mockImplementation(async (config) => ({
    data: { success: true, message: '' },
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }))
  api.defaults.adapter = transport
  client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  client.clear()
})

describe('公告管理中的置顶', () => {
  test('置顶按钮切换行状态并作为 pinned 字段保存', async () => {
    const user = userEvent.setup()
    renderSection([
      {
        id: 1,
        content: 'plain announcement',
        publishDate: '2026-09-10T10:00:00Z',
        type: 'default',
      },
    ])

    const row = screen.getAllByRole('row')[1]
    expect(within(row).queryByText('Pinned')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Pin' }))

    expect(within(row).getByText('Pinned')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Unpin' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() => expect(transport).toHaveBeenCalledTimes(1))

    const payload = JSON.parse(String(transport.mock.calls[0][0].data))
    expect(payload.key).toBe('console_setting.announcements')
    // 取消置顶后再次保存需要写回 false
    await user.click(screen.getByRole('button', { name: 'Unpin' }))
    await user.click(screen.getByRole('button', { name: 'Save Settings' }))
    await waitFor(() => expect(transport).toHaveBeenCalledTimes(2))
    const secondPayload = JSON.parse(String(transport.mock.calls[1][0].data))
    expect(JSON.parse(secondPayload.value)[0].pinned).toBe(false)
  })

  test('历史数据缺少 pinned 时按未置顶读取，列表按置顶优先排序', () => {
    renderSection([
      {
        id: 1,
        content: 'plain newer',
        publishDate: '2026-09-18T10:00:00Z',
        type: 'default',
      },
      {
        id: 2,
        content: 'pinned older',
        publishDate: '2026-09-10T10:00:00Z',
        type: 'default',
        pinned: true,
      },
    ])

    const rows = screen.getAllByRole('row')
    expect(rows[1].textContent).toContain('pinned older')
    expect(rows[2].textContent).toContain('plain newer')
    // 旧公告没有 pinned 字段，必须按未置顶渲染而不是报错
    expect(screen.getByText('plain newer')).toBeVisible()
    expect(screen.getAllByRole('button', { name: 'Unpin' })).toHaveLength(1)
  })
})
