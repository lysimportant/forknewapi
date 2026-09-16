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
import { create } from 'zustand'
import {
  createJSONStorage,
  persist,
  type StateStorage,
} from 'zustand/middleware'

/** 本地存储不可用（禁用、隐私模式或配额耗尽）时的内存兜底，保证关闭逻辑不中断渲染。 */
const memoryStorageFallback = new Map<string, string>()

/** 探测本地存储是否可写；不可写时「今日关闭」无法跨刷新保存。 */
function canWriteLocalStorage(): boolean {
  try {
    const probeKey = 'notification-storage-probe'
    window.localStorage.setItem(probeKey, '1')
    window.localStorage.removeItem(probeKey)
    return true
  } catch {
    return false
  }
}

/**
 * 读写全部兜底：localStorage 抛错时降级为内存存储。
 * 关闭状态仍会生效于当前页面会话，只是无法跨刷新保留。
 */
const notificationStorage: StateStorage = {
  getItem: (name) => {
    try {
      return window.localStorage.getItem(name)
    } catch {
      return memoryStorageFallback.get(name) ?? null
    }
  },
  setItem: (name, value) => {
    try {
      window.localStorage.setItem(name, value)
    } catch {
      memoryStorageFallback.set(name, value)
    }
  },
  removeItem: (name) => {
    try {
      window.localStorage.removeItem(name)
    } catch {
      memoryStorageFallback.delete(name)
    }
  },
}

interface NotificationState {
  // Last read Notice content signature (full trimmed message)
  lastReadNotice: string
  // Array of read announcement keys (id or content hash)
  readAnnouncementKeys: string[]
  // Timestamp of last "Close Today" action
  closedUntilDate: string | null
  // 本次页面会话内关闭公告弹窗，不持久化，刷新后允许重新弹出
  sessionClosed: boolean
  // 本地存储是否可写；false 表示「今日关闭」无法跨刷新保存
  storageAvailable: boolean

  // Actions
  markNoticeRead: (noticeContent: string) => void
  markAnnouncementsRead: (keys: string[]) => void
  setClosedUntilDate: (date: string | null) => void
  isAnnouncementRead: (key: string) => boolean
  isNoticeClosed: () => boolean
  closeAnnouncementsForSession: () => void
  closeAnnouncementsForToday: () => void
  canAutoOpenAnnouncements: () => boolean
}

/**
 * Notification store for tracking read status of Notice and Announcements
 * Persists to localStorage to maintain state across sessions
 */
export const useNotificationStore = create<NotificationState>()(
  persist(
    (set, get) => ({
      lastReadNotice: '',
      readAnnouncementKeys: [],
      closedUntilDate: null,
      sessionClosed: false,
      storageAvailable: canWriteLocalStorage(),

      markNoticeRead: (noticeContent: string) => {
        // Persist the full trimmed content so edits beyond 100 chars register
        const normalizedContent = noticeContent.trim()
        set({ lastReadNotice: normalizedContent })
      },

      markAnnouncementsRead: (keys: string[]) => {
        set((state) => ({
          readAnnouncementKeys: [
            ...new Set([...state.readAnnouncementKeys, ...keys]),
          ],
        }))
      },

      setClosedUntilDate: (date: string | null) => {
        set({ closedUntilDate: date })
      },

      isAnnouncementRead: (key: string) => {
        return get().readAnnouncementKeys.includes(key)
      },

      isNoticeClosed: () => {
        const { closedUntilDate } = get()
        if (!closedUntilDate) return false

        const today = new Date().toDateString()
        return closedUntilDate === today
      },

      closeAnnouncementsForSession: () => {
        set({ sessionClosed: true })
      },

      closeAnnouncementsForToday: () => {
        set({
          closedUntilDate: new Date().toDateString(),
          sessionClosed: true,
        })
        // 先触发持久化再探测：配额耗尽或隐私模式下降级为仅本次关闭
        set({ storageAvailable: canWriteLocalStorage() })
      },

      canAutoOpenAnnouncements: () => {
        if (get().sessionClosed) return false
        return !get().isNoticeClosed()
      },
    }),
    {
      name: 'notification-storage',
      storage: createJSONStorage(() => notificationStorage),
      partialize: (state) => ({
        lastReadNotice: state.lastReadNotice,
        readAnnouncementKeys: state.readAnnouncementKeys,
        closedUntilDate: state.closedUntilDate,
      }),
    }
  )
)
