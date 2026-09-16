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
import { describe, expect, test, vi } from 'vitest'

import { createIdempotencyKey, generateAffiliateLink } from '../affiliate'

describe('referral withdrawal request identity', () => {
  test('generates a distinct idempotency key per submission', () => {
    const keys = new Set(
      Array.from({ length: 32 }, () => createIdempotencyKey())
    )

    expect(keys.size).toBe(32)
  })

  test('falls back to a timestamped key when Web Crypto is unavailable', () => {
    const originalCrypto = globalThis.crypto
    // 只有不可控的浏览器 API 边界允许 mock；这里模拟缺少 randomUUID 的环境。
    Object.defineProperty(globalThis, 'crypto', {
      value: undefined,
      configurable: true,
    })
    try {
      const key = createIdempotencyKey()

      expect(key).toMatch(/^aff-\d+-[a-z0-9]+$/)
      expect(createIdempotencyKey()).not.toBe(key)
    } finally {
      Object.defineProperty(globalThis, 'crypto', {
        value: originalCrypto,
        configurable: true,
      })
    }
  })

  test('keeps the referral link stable for a given code', () => {
    expect(generateAffiliateLink('ab12')).toContain('/sign-up?aff=ab12')
  })

  test('does not fail when the window is unavailable', () => {
    const originalWindow = globalThis.window
    vi.stubGlobal('window', undefined)
    try {
      expect(generateAffiliateLink('ab12')).toBe('')
    } finally {
      vi.stubGlobal('window', originalWindow)
    }
  })
})
