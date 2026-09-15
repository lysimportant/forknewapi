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
import { afterEach, describe, expect, test, vi } from 'vitest'

const showError = vi.fn()

vi.mock('sonner', () => ({
  toast: {
    error: showError,
    success: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
    message: vi.fn(),
    loading: vi.fn(),
  },
}))

describe('deduped toast ids', () => {
  afterEach(() => {
    showError.mockClear()
  })

  test('identical error messages reuse one toast id', async () => {
    const { toast } = await import('../toast')
    toast.error('兑换失败，请稍后重试')
    toast.error('兑换失败，请稍后重试')
    expect(showError).toHaveBeenCalledTimes(2)
    expect(showError.mock.calls[0][1]).toEqual({
      id: 'error:兑换失败，请稍后重试',
    })
    expect(showError.mock.calls[1][1]).toEqual({
      id: 'error:兑换失败，请稍后重试',
    })
  })

  test('explicit toast ids are preserved', async () => {
    const { toast } = await import('../toast')
    toast.error('Rejected', { id: 'custom-id' })
    expect(showError).toHaveBeenCalledExactlyOnceWith('Rejected', {
      id: 'custom-id',
    })
  })
})
