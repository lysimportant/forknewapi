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
export type SortableAnnouncement = {
  pinned?: boolean
  publishDate?: string | Date | null
}

/** 解析发布时间，缺失或非法时按最小值处理，避免排序出现 NaN */
function publishTimestamp(value: string | Date | null | undefined): number {
  if (!value) return 0

  const timestamp = new Date(value).getTime()
  return Number.isNaN(timestamp) ? 0 : timestamp
}

/**
 * 公告统一排序：置顶优先 → 发布时间倒序 → 时间相同时保持原有顺序
 * 弹窗、通知列表与仪表盘必须共用该顺序
 */
export function sortAnnouncements<T extends SortableAnnouncement>(
  items: readonly T[]
): T[] {
  return [...items].sort((a, b) => {
    const pinnedDiff = Number(b.pinned === true) - Number(a.pinned === true)
    if (pinnedDiff !== 0) return pinnedDiff

    return publishTimestamp(b.publishDate) - publishTimestamp(a.publishDate)
  })
}
