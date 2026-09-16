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
import { describe, expect, it } from 'vitest'

import { sortAnnouncements } from '../announcement-sort'

describe('公告统一排序', () => {
  it('置顶优先，其次按发布时间倒序，与配置顺序无关', () => {
    const sorted = sortAnnouncements([
      { id: 1, publishDate: '2026-09-18T10:00:00Z' },
      { id: 2, publishDate: '2026-09-10T10:00:00Z', pinned: true },
      { id: 3, publishDate: '2026-09-16T10:00:00Z', pinned: true },
      { id: 4, publishDate: '2026-09-01T10:00:00Z' },
    ])

    expect(sorted.map((item) => item.id)).toEqual([3, 2, 1, 4])
  })

  it('发布时间相同时保持原有顺序', () => {
    const sorted = sortAnnouncements([
      { id: 'first', publishDate: '2026-09-15T10:00:00Z', pinned: true },
      { id: 'second', publishDate: '2026-09-15T10:00:00Z', pinned: true },
      { id: 'third', publishDate: '2026-09-15T10:00:00Z' },
    ])

    expect(sorted.map((item) => item.id)).toEqual(['first', 'second', 'third'])
  })

  it('缺少或非法的 pinned 与 publishDate 按未置顶、最早处理且不产生 NaN', () => {
    const sorted = sortAnnouncements([
      { id: 'no-date' },
      { id: 'legacy', publishDate: '2026-09-15T10:00:00Z' },
      { id: 'invalid-pinned', publishDate: '2026-09-20T10:00:00Z' },
    ])

    expect(sorted.map((item) => item.id)).toEqual([
      'invalid-pinned',
      'legacy',
      'no-date',
    ])
  })

  it('返回新数组，不修改调用方传入的列表', () => {
    const items = [
      { id: 1, publishDate: '2026-09-15T10:00:00Z' },
      { id: 2, publishDate: '2026-09-16T10:00:00Z', pinned: true },
    ]

    const sorted = sortAnnouncements(items)

    expect(sorted).not.toBe(items)
    expect(items.map((item) => item.id)).toEqual([1, 2])
  })
})
