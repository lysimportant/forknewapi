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
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { useNotificationStore } from '../notification-store'

const storageKey = 'notification-storage'

/** 模拟页面刷新：只保留 localStorage 中的持久化内容并重新水合 */
async function simulateRefresh() {
  const persisted = window.localStorage.getItem(storageKey)
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  if (persisted === null) {
    window.localStorage.removeItem(storageKey)
  } else {
    window.localStorage.setItem(storageKey, persisted)
  }
  await useNotificationStore.persist.rehydrate()
}

let restoreStorage: (() => void) | null = null

/**
 * 用写入必失败的实现替换 localStorage，模拟隐私模式或配额耗尽
 * @returns 恢复原始 localStorage 的回调
 */
function replaceWithFailingStorage() {
  const original = window.localStorage
  const failing = {
    get length() {
      return 0
    },
    clear: () => undefined,
    getItem: () => null,
    key: () => null,
    removeItem: () => undefined,
    setItem: () => {
      throw new Error('QuotaExceededError')
    },
  } as Storage

  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    writable: true,
    value: failing,
  })

  return () => {
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      writable: true,
      value: original,
    })
  }
}

beforeEach(() => {
  window.localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
})

afterEach(() => {
  restoreStorage?.()
  restoreStorage = null
  window.localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
})

describe('公告弹窗关闭状态', () => {
  it('「关闭本次」只作用于当前页面会话，刷新后重新允许自动弹出', async () => {
    useNotificationStore.getState().closeAnnouncementsForSession()

    expect(useNotificationStore.getState().canAutoOpenAnnouncements()).toBe(
      false
    )
    const persisted = window.localStorage.getItem(storageKey)
    expect(persisted).not.toBeNull()
    // 会话标记不进入持久化结构，兼容既有存储形状
    expect(persisted).not.toContain('sessionClosed')

    await simulateRefresh()
    expect(useNotificationStore.getState().canAutoOpenAnnouncements()).toBe(
      true
    )
  })

  it('「今日关闭」持久化到本地当天，刷新后仍不弹出，跨日恢复', async () => {
    useNotificationStore.getState().closeAnnouncementsForToday()

    const persisted = JSON.parse(
      window.localStorage.getItem(storageKey) ?? '{}'
    ) as { state?: Record<string, unknown> }
    expect(persisted.state?.closedUntilDate).toBe(new Date().toDateString())
    expect(persisted.state?.sessionClosed).toBeUndefined()

    await simulateRefresh()
    expect(useNotificationStore.getState().isNoticeClosed()).toBe(true)
    expect(useNotificationStore.getState().canAutoOpenAnnouncements()).toBe(
      false
    )

    // 跨日：比较的是本地日期字符串，昨天写入的值不再生效
    window.localStorage.setItem(
      storageKey,
      JSON.stringify({
        state: {
          lastReadNotice: '',
          readAnnouncementKeys: [],
          closedUntilDate: 'Mon Jan 01 2024',
        },
        version: 0,
      })
    )
    await useNotificationStore.persist.rehydrate()

    expect(useNotificationStore.getState().isNoticeClosed()).toBe(false)
    expect(useNotificationStore.getState().canAutoOpenAnnouncements()).toBe(
      true
    )
  })

  it('本地存储写入失败时降级为内存关闭且不抛出异常', () => {
    window.localStorage.removeItem(storageKey)
    restoreStorage = replaceWithFailingStorage()

    expect(() =>
      useNotificationStore.getState().closeAnnouncementsForToday()
    ).not.toThrow()

    expect(useNotificationStore.getState().isNoticeClosed()).toBe(true)
    expect(useNotificationStore.getState().canAutoOpenAnnouncements()).toBe(
      false
    )
    expect(useNotificationStore.getState().storageAvailable).toBe(false)

    // 未写入 localStorage：刷新后无法保留「今日关闭」
    restoreStorage()
    restoreStorage = null
    expect(window.localStorage.getItem(storageKey)).toBeNull()
  })

  it('兼容旧持久化结构并保留新增字段的默认值', async () => {
    window.localStorage.setItem(
      storageKey,
      JSON.stringify({
        state: {
          lastReadNotice: 'legacy notice',
          readAnnouncementKeys: ['id:1'],
          closedUntilDate: null,
        },
        version: 0,
      })
    )

    await useNotificationStore.persist.rehydrate()

    const state = useNotificationStore.getState()
    expect(state.lastReadNotice).toBe('legacy notice')
    expect(state.readAnnouncementKeys).toEqual(['id:1'])
    expect(state.isNoticeClosed()).toBe(false)
    expect(state.sessionClosed).toBe(false)
    expect(state.storageAvailable).toBe(true)
    expect(state.canAutoOpenAnnouncements()).toBe(true)
  })
})
