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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import type { AnnouncementEntry } from '../../types'
import { AnnouncementTimelineDialog } from '../announcement-timeline-dialog'

const announcements: AnnouncementEntry[] = [
  { id: 1, content: 'plain newest', publishDate: '2026-09-18T10:00:00Z' },
  {
    id: 2,
    content: 'pinned oldest',
    publishDate: '2026-09-10T10:00:00Z',
    pinned: true,
  },
  { id: 3, content: 'plain oldest', publishDate: '2026-09-01T10:00:00Z' },
]

function renderDialog(overrides?: {
  items?: AnnouncementEntry[]
  onCloseForToday?: () => void
  closeTodayPersists?: boolean
}) {
  const onOpenChange = vi.fn()
  render(
    <AnnouncementTimelineDialog
      open
      onOpenChange={onOpenChange}
      announcements={overrides?.items ?? announcements}
      onCloseForToday={overrides?.onCloseForToday ?? vi.fn()}
      closeTodayPersists={overrides?.closeTodayPersists}
    />
  )
  return { onOpenChange }
}

describe('公告时间轴弹窗', () => {
  it('按置顶优先、发布时间倒序展示公告并标出置顶项', () => {
    renderDialog()

    const entries = screen.getAllByRole('listitem')
    expect(entries).toHaveLength(3)
    expect(entries[0].textContent).toContain('pinned oldest')
    expect(entries[1].textContent).toContain('plain newest')
    expect(entries[2].textContent).toContain('plain oldest')
    expect(screen.getAllByText('Pinned')).toHaveLength(1)
  })

  it('展示全部公告，不做通知列表的前 20 条截断', () => {
    const many: AnnouncementEntry[] = Array.from(
      { length: 25 },
      (_, index) => ({
        id: index + 1,
        content: `announcement ${index + 1}`,
        publishDate: new Date(Date.UTC(2026, 8, 1, 10, index)).toISOString(),
      })
    )

    renderDialog({ items: many })

    const entries = screen.getAllByRole('listitem')
    expect(entries).toHaveLength(25)
    expect(entries[0].textContent).toContain('announcement 25')
    expect(entries[24].textContent).toContain('announcement 1')
  })

  it('底部只提供「关闭公告」与「今日关闭」两个按钮', async () => {
    const user = userEvent.setup()
    const onCloseForToday = vi.fn()
    const { onOpenChange } = renderDialog({ onCloseForToday })

    const footer = document.querySelector('[data-slot="dialog-footer"]')
    expect(footer).not.toBeNull()
    const footerButtons = within(footer as HTMLElement).getAllByRole('button')
    expect(footerButtons.map((button) => button.textContent)).toEqual([
      'Close Announcements',
      'Close Today',
    ])

    await user.click(
      within(footer as HTMLElement).getByRole('button', {
        name: 'Close Announcements',
      })
    )
    expect(onOpenChange).toHaveBeenCalledWith(false)

    await user.click(
      within(footer as HTMLElement).getByRole('button', { name: 'Close Today' })
    )
    expect(onCloseForToday).toHaveBeenCalledTimes(1)
  })

  it('Esc 关闭后把焦点交还给打开弹窗的元素', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [open, setOpen] = useState(false)
      return (
        <>
          <button type='button' onClick={() => setOpen(true)}>
            Open announcements
          </button>
          <AnnouncementTimelineDialog
            open={open}
            onOpenChange={setOpen}
            announcements={announcements}
            onCloseForToday={() => setOpen(false)}
          />
        </>
      )
    }

    render(<Harness />)
    const trigger = screen.getByRole('button', { name: 'Open announcements' })
    await user.click(trigger)
    expect(await screen.findByRole('dialog')).toBeVisible()

    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await waitFor(() => expect(trigger).toHaveFocus())
  })

  it('本地存储不可用时提示「今日关闭」无法跨刷新保存', () => {
    renderDialog({ closeTodayPersists: false })

    expect(
      screen.getByText(
        'Close Today cannot be saved in this browser; announcements will show again after refresh.'
      )
    ).toBeVisible()
  })

  it('没有公告时展示空状态而不是空时间轴', () => {
    renderDialog({ items: [] })

    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    expect(screen.getByText('No system announcements')).toBeVisible()
  })
})
