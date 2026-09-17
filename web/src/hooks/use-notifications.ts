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
import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'

import { sortAnnouncements } from '@/features/announcements/lib/announcement-sort'
import type { AnnouncementEntry } from '@/features/announcements/types'
import { useStatus } from '@/hooks/use-status'
import { getNotice } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useNotificationStore } from '@/stores/notification-store'

function hashString(input: string): string {
  let hash = 0
  if (!input) return '0'

  for (let i = 0; i < input.length; i += 1) {
    const chr = input.charCodeAt(i)
    hash = (hash << 5) - hash + chr
    hash |= 0
  }

  return hash.toString(36)
}

/**
 * Generate a unique key for an announcement
 * Prefer backend id, fall back to a content hash so edits register
 */
function getAnnouncementKey(item: AnnouncementEntry): string {
  if (!item) return ''

  if (item.id !== undefined && item.id !== null) {
    return `id:${item.id}`
  }

  // 历史公告可能带有 title / link 字段，指纹必须与旧版本保持一致
  const legacy = item as AnnouncementEntry & { title?: string; link?: string }
  const fingerprint = JSON.stringify({
    publishDate: item.publishDate || '',
    content: (item.content || '').trim(),
    extra: (item.extra || '').trim(),
    type: item.type || '',
    title: (legacy.title || '').trim(),
    link: (legacy.link || '').trim(),
  })
  return `hash:${hashString(fingerprint)}`
}

/**
 * Hook to manage notifications (Notice + Announcements)
 * Provides unread counts and read status management
 */
export function useNotifications() {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [timelineOpen, setTimelineOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'notice' | 'announcements'>(
    'notice'
  )
  // 自动弹出每次页面会话只判定一次，路由切换不重复打扰
  const autoOpenHandledRef = useRef(false)

  // Fetch Notice from API
  const {
    data: noticeResponse,
    isLoading: noticeLoading,
    refetch: refetchNotice,
  } = useQuery({
    queryKey: ['notice'],
    queryFn: async () => requireServerSuccess(await getNotice()),
    staleTime: 1000 * 60 * 5, // 5 minutes
  })

  // Fetch Announcements from status
  const { status, loading: statusLoading } = useStatus()
  const announcementsEnabled = status?.announcements_enabled ?? false
  const statusAnnouncements = status?.announcements as
    | AnnouncementEntry[]
    | undefined
  // 时间轴弹窗展示全部公告，通知列表沿用前 20 条截断；两者共用置顶优先排序
  const allAnnouncements = useMemo(
    () =>
      announcementsEnabled ? sortAnnouncements(statusAnnouncements ?? []) : [],
    [announcementsEnabled, statusAnnouncements]
  )
  const announcements = useMemo(
    () => allAnnouncements.slice(0, 20),
    [allAnnouncements]
  )
  // 旧版站点公告独立于时间轴展示开关，不能因未配置时间轴而漏掉。
  const noticeContent = noticeResponse?.success
    ? (noticeResponse.data || '').trim()
    : ''

  // Notification store
  const {
    lastReadNotice,
    markNoticeRead,
    markAnnouncementsRead,
    isAnnouncementRead,
    storageAvailable,
    closeAnnouncementsForSession,
    closeAnnouncementsForToday,
    canAutoOpenAnnouncements,
  } = useNotificationStore()

  // 公共页面与控制台首次进入时只展示系统公告时间轴；旧 Notice 仍在通知面板显示。
  useEffect(() => {
    if (autoOpenHandledRef.current || statusLoading) return
    if (allAnnouncements.length === 0) return

    autoOpenHandledRef.current = true
    // 「今日关闭」与「关闭本次」都不强制重新弹出
    if (!canAutoOpenAnnouncements()) return

    setTimelineOpen(true)
  }, [statusLoading, allAnnouncements.length, canAutoOpenAnnouncements])

  // Calculate unread counts
  const unreadCounts = useMemo(() => {
    const noticeUnread =
      noticeContent && noticeContent !== lastReadNotice ? 1 : 0

    const announcementsUnread = announcements.filter((item) => {
      const key = getAnnouncementKey(item)
      return !isAnnouncementRead(key)
    }).length

    return {
      notice: noticeUnread,
      announcements: announcementsUnread,
      total: noticeUnread + announcementsUnread,
    }
  }, [noticeContent, lastReadNotice, announcements, isAnnouncementRead])

  const markAnnouncementsAsRead = () => {
    if (announcements.length > 0) {
      const allKeys = announcements.map((item) => getAnnouncementKey(item))
      markAnnouncementsRead(allKeys)
    }
  }

  // Handle popover open
  const handleOpenPopover = (tab?: 'notice' | 'announcements') => {
    const nextTab = tab || activeTab

    // Mark currently visible content as read when opening the notification center
    if (noticeContent) {
      markNoticeRead(noticeContent)
    }
    if (nextTab === 'announcements') {
      markAnnouncementsAsRead()
    }

    setActiveTab(nextTab)
    setPopoverOpen(true)
  }

  const handlePopoverOpenChange = (open: boolean) => {
    if (open) {
      handleOpenPopover(activeTab)
      return
    }

    setPopoverOpen(false)
  }

  // Handle tab change - mark announcements as read when switching to that tab
  const handleTabChange = (tab: 'notice' | 'announcements') => {
    setActiveTab(tab)

    if (tab === 'announcements') {
      markAnnouncementsAsRead()
    }
  }

  // 手动查看始终可用：不受「今日关闭」限制
  const openTimeline = () => {
    setTimelineOpen(true)
  }

  // ×、Esc、点击遮罩与「关闭公告」都只关闭本次页面会话
  const handleTimelineOpenChange = (open: boolean) => {
    setTimelineOpen(open)

    if (!open) {
      closeAnnouncementsForSession()
    }
  }

  const closeTimelineForToday = () => {
    closeAnnouncementsForToday()
    setTimelineOpen(false)
  }

  return {
    // Data
    notice: noticeContent,
    announcements,
    allAnnouncements,
    loading: noticeLoading || statusLoading,

    // Unread counts
    unreadCount: unreadCounts.total,
    unreadNoticeCount: unreadCounts.notice,
    unreadAnnouncementsCount: unreadCounts.announcements,

    // Popover state
    popoverOpen,
    setPopoverOpen: handlePopoverOpenChange,
    activeTab,
    setActiveTab: handleTabChange,

    // Timeline dialog state
    timelineOpen,
    setTimelineOpen: handleTimelineOpenChange,
    openTimeline,
    closeTimelineForToday,
    closeTodayPersists: storageAvailable,

    // Actions
    openPopover: handleOpenPopover,
    closePopover: () => setPopoverOpen(false),
    refetchNotice,
  }
}
