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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/http-client'

import { useSystemOptions } from '../../hooks/use-system-options'
import { AnnouncementsSection } from '../announcements-section'

/** 仅替换外部传输层；真实保存 API 与请求拦截器均参与测试。 */
const transport = vi.fn<AxiosAdapter>()
const originalAdapter = api.defaults.adapter

let client: QueryClient
let savedAnnouncements: string
let savedEnabled: string
let saveError: string
let pendingWrite: Promise<void> | undefined
let pendingRead: Promise<void> | undefined

/** 从真实设置查询取得公告；HTTP 适配器保存成功后可供重新挂载再次读取。 */
function AnnouncementSettings() {
  const options = useSystemOptions()
  if (options.isLoading) return null
  const data = options.data?.data?.find(
    (option) => option.key === 'console_setting.announcements'
  )?.value
  const enabled = options.data?.data?.find(
    (option) => option.key === 'console_setting.announcements_enabled'
  )?.value
  return (
    <AnnouncementsSection enabled={enabled === 'true'} data={data ?? '[]'} />
  )
}

/** 为每次页面挂载提供独立查询缓存，避免把内存列表误当作持久化结果。 */
function renderSection() {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AnnouncementSettings />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  // 项目提示封装在模块加载时替换 Sonner 方法，监听最终导出以覆盖真实错误路径。
  vi.spyOn(toast, 'success').mockReturnValue('test-success')
  vi.spyOn(toast, 'error').mockReturnValue('test-error')
  transport.mockReset()
  savedAnnouncements = '[]'
  savedEnabled = 'true'
  saveError = ''
  pendingWrite = undefined
  pendingRead = undefined
  transport.mockImplementation(async (config) => {
    expect(config.url).toBe('/api/option/')
    const snapshot = savedAnnouncements
    const enabledSnapshot = savedEnabled
    let data: unknown
    if (config.method === 'put') {
      await pendingWrite
      const payload = JSON.parse(String(config.data))
      expect([
        'console_setting.announcements',
        'console_setting.announcements_enabled',
      ]).toContain(payload.key)
      if (!saveError) {
        if (payload.key === 'console_setting.announcements') {
          savedAnnouncements = payload.value
        } else {
          savedEnabled = payload.value
        }
      }
      data = { success: !saveError, message: saveError }
    } else {
      await pendingRead
      data = {
        success: true,
        data: [
          { key: 'console_setting.announcements', value: snapshot },
          {
            key: 'console_setting.announcements_enabled',
            value: enabledSnapshot,
          },
        ],
      }
    }
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  })
  api.defaults.adapter = transport
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  client.clear()
})

describe('公告管理中的置顶', () => {
  test('置顶和取消置顶立即持久化，无需再次保存设置', async () => {
    const user = userEvent.setup()
    savedAnnouncements = JSON.stringify([
      {
        id: 1,
        content: 'plain announcement',
        publishDate: '2026-09-10T10:00:00Z',
        type: 'default',
      },
    ])
    renderSection()

    await screen.findByText('plain announcement')
    const row = screen.getAllByRole('row')[1]
    expect(within(row).queryByText('Pinned')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Pin' }))

    await waitFor(() =>
      expect(JSON.parse(savedAnnouncements)[0].pinned).toBe(true)
    )
    expect(within(row).getByText('Pinned')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Save Settings' })
    ).not.toBeInTheDocument()
    // 取消置顶后再次保存需要写回 false
    await user.click(screen.getByRole('button', { name: 'Unpin' }))
    await waitFor(() =>
      expect(JSON.parse(savedAnnouncements)[0].pinned).toBe(false)
    )
  })

  test('历史数据缺少 pinned 时按未置顶读取，列表按置顶优先排序', async () => {
    savedAnnouncements = JSON.stringify([
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
    renderSection()

    await screen.findByText('plain newer')
    const rows = screen.getAllByRole('row')
    expect(rows[1].textContent).toContain('pinned older')
    expect(rows[2].textContent).toContain('plain newer')
    // 旧公告没有 pinned 字段，必须按未置顶渲染而不是报错
    expect(screen.getByText('plain newer')).toBeVisible()
    expect(screen.getAllByRole('button', { name: 'Unpin' })).toHaveLength(1)
  })
})

test('添加确认即保存，销毁页面并重新查询后仍能读到公告', async () => {
  const user = userEvent.setup()
  const view = renderSection()
  await user.click(
    await screen.findByRole('button', { name: 'Add Announcement' })
  )
  const dialog = screen.getByRole('dialog')
  await user.type(
    within(dialog).getByRole('textbox', { name: 'Content' }),
    'Persistent announcement'
  )
  await user.click(within(dialog).getByRole('button', { name: 'Add' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(JSON.parse(savedAnnouncements)).toEqual([
    expect.objectContaining({
      content: 'Persistent announcement',
      pinned: false,
    }),
  ])

  view.unmount()
  client.clear()
  renderSection()
  expect(
    await screen.findByRole('cell', { name: 'Persistent announcement' })
  ).toBeVisible()
})

test('编辑保存失败保留输入和旧公告，重试成功后才关闭编辑弹窗', async () => {
  savedAnnouncements = JSON.stringify([
    {
      id: 1,
      content: 'Original announcement',
      publishDate: '2026-09-10T10:00:00Z',
      type: 'default',
    },
  ])
  const user = userEvent.setup()
  renderSection()
  await user.click(await screen.findByRole('button', { name: 'Edit' }))
  const dialog = screen.getByRole('dialog')
  const input = within(dialog).getByRole('textbox', { name: 'Content' })
  await user.clear(input)
  await user.type(input, 'Revised announcement')
  saveError = 'Database write failed'
  await user.click(within(dialog).getByRole('button', { name: 'Update' }))
  await waitFor(() =>
    expect(within(dialog).getByRole('button', { name: 'Update' })).toBeEnabled()
  )
  expect(dialog).toBeVisible()
  expect(input).toHaveValue('Revised announcement')
  expect(toast.error).toHaveBeenCalledWith('Database write failed')
  expect(JSON.parse(savedAnnouncements)[0].content).toBe(
    'Original announcement'
  )

  saveError = ''
  await user.click(within(dialog).getByRole('button', { name: 'Update' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(
    await screen.findByRole('cell', { name: 'Revised announcement' })
  ).toBeVisible()
  expect(JSON.parse(savedAnnouncements)[0].content).toBe('Revised announcement')
})

test('保存等待期间禁止重复与冲突操作，服务端确认前不更改行状态', async () => {
  savedAnnouncements = JSON.stringify([
    {
      id: 1,
      content: 'Pending announcement',
      publishDate: '2026-09-10T10:00:00Z',
      type: 'default',
    },
  ])
  let releaseWrite!: () => void
  pendingWrite = new Promise<void>((resolve) => {
    releaseWrite = resolve
  })
  const user = userEvent.setup()
  renderSection()
  const pin = await screen.findByRole('button', { name: 'Pin' })
  await user.dblClick(pin)
  expect(pin).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Add Announcement' })
  ).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Edit' })).toBeDisabled()
  expect(screen.getByRole('switch')).toHaveAttribute('aria-disabled', 'true')
  expect(
    screen.queryByRole('button', { name: 'Unpin' })
  ).not.toBeInTheDocument()
  expect(
    transport.mock.calls.filter(([config]) => config.method === 'put')
  ).toHaveLength(1)
  await act(async () => releaseWrite())
  expect(await screen.findByRole('button', { name: 'Unpin' })).toBeEnabled()
})

test('添加保存期间不关闭或重复提交，失败后保留正文并恢复操作', async () => {
  const user = userEvent.setup()
  renderSection()
  await user.click(
    await screen.findByRole('button', { name: 'Add Announcement' })
  )
  const dialog = screen.getByRole('dialog')
  const input = within(dialog).getByRole('textbox', { name: 'Content' })
  await user.type(input, 'Retry this announcement')
  let releaseWrite!: () => void
  pendingWrite = new Promise<void>((resolve) => {
    releaseWrite = resolve
  })
  saveError = 'Database write failed'
  await user.dblClick(within(dialog).getByRole('button', { name: 'Add' }))
  expect(
    within(dialog).getByRole('button', { name: 'Saving...' })
  ).toBeDisabled()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  expect(input).toBeDisabled()
  await user.keyboard('{Escape}')
  expect(dialog).toBeVisible()
  expect(
    transport.mock.calls.filter(([config]) => config.method === 'put')
  ).toHaveLength(1)
  await act(async () => releaseWrite())
  expect(
    await within(dialog).findByRole('button', { name: 'Add' })
  ).toBeEnabled()
  expect(input).toHaveValue('Retry this announcement')
  expect(savedAnnouncements).toBe('[]')
})

test('单条删除失败保留确认和数据，重试及批量删除都直接保存', async () => {
  savedAnnouncements = JSON.stringify([
    {
      id: 1,
      content: 'First announcement',
      publishDate: '2026-09-10T10:00:00Z',
      type: 'default',
    },
    {
      id: 2,
      content: 'Second announcement',
      publishDate: '2026-09-11T10:00:00Z',
      type: 'default',
    },
  ])
  const user = userEvent.setup()
  const view = renderSection()
  const row = await screen.findByRole('row', { name: /First announcement/ })
  await user.click(within(row).getByRole('button', { name: 'Open menu' }))
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  const confirmation = screen.getByRole('alertdialog')
  saveError = 'Database write failed'
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }))
  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith('Database write failed')
  )
  expect(confirmation).toBeVisible()
  expect(JSON.parse(savedAnnouncements)).toHaveLength(2)
  saveError = ''
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }))
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(
    JSON.parse(savedAnnouncements).map((item: { id: number }) => item.id)
  ).toEqual([2])

  await user.click(screen.getAllByRole('checkbox')[0])
  await user.click(screen.getByRole('button', { name: 'Delete (1)' }))
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Delete',
    })
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(savedAnnouncements).toBe('[]')
  view.unmount()
  client.clear()
  renderSection()
  expect(
    await screen.findByText(
      'No announcements yet. Click "Add Announcement" to create one.'
    )
  ).toBeVisible()
})

test('保存后再次刷新配置也不会复用旧响应覆盖公告和展示开关', async () => {
  const user = userEvent.setup()
  const view = renderSection()
  await user.click(
    await screen.findByRole('button', { name: 'Add Announcement' })
  )
  const dialog = screen.getByRole('dialog')
  await user.type(
    within(dialog).getByRole('textbox', { name: 'Content' }),
    'Latest persisted announcement'
  )
  let releaseRead!: () => void
  pendingRead = new Promise<void>((resolve) => {
    releaseRead = resolve
  })
  let refresh!: Promise<void>
  await act(async () => {
    refresh = client.invalidateQueries({ queryKey: ['system-options'] })
  })
  await waitFor(() =>
    expect(
      transport.mock.calls.filter(([config]) => config.method === 'get')
    ).toHaveLength(2)
  )
  await user.click(within(dialog).getByRole('button', { name: 'Add' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(
    await screen.findByRole('cell', { name: 'Latest persisted announcement' })
  ).toBeVisible()
  await user.click(screen.getByRole('switch'))
  await waitFor(() => expect(savedEnabled).toBe('false'))
  await waitFor(() =>
    expect(screen.getByRole('switch')).not.toHaveAttribute(
      'aria-disabled',
      'true'
    )
  )
  let nextRefresh!: Promise<void>
  await act(async () => {
    nextRefresh = client.invalidateQueries({ queryKey: ['system-options'] })
  })
  await act(async () => {
    releaseRead()
    await refresh
    await nextRefresh
  })
  view.unmount()
  render(
    <QueryClientProvider client={client}>
      <AnnouncementSettings />
    </QueryClientProvider>
  )
  expect(
    screen.getByRole('cell', { name: 'Latest persisted announcement' })
  ).toBeVisible()
  expect(JSON.parse(savedAnnouncements)[0].content).toBe(
    'Latest persisted announcement'
  )
  expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'false')
  expect(toast.error).not.toHaveBeenCalled()
})

test('展示开关保存失败保持原值，成功后刷新页面仍保持新值', async () => {
  const user = userEvent.setup()
  const view = renderSection()
  const toggle = await screen.findByRole('switch')
  saveError = 'Database write failed'
  await user.click(toggle)
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith(saveError))
  expect(toggle).toHaveAttribute('aria-checked', 'true')
  expect(savedEnabled).toBe('true')
  saveError = ''
  await user.click(toggle)
  await waitFor(() => expect(toggle).toHaveAttribute('aria-checked', 'false'))
  view.unmount()
  client.clear()
  renderSection()
  expect(await screen.findByRole('switch')).toHaveAttribute(
    'aria-checked',
    'false'
  )
})
