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
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { getLogStats, getUserLogStats } from '../api'
import { LOG_TYPES, LOG_TYPE_ENUM } from '../constants'
import { buildApiParams, fetchLogsByCategory } from '../lib/utils'

afterEach(() => {
  vi.restoreAllMocks()
})

it('keeps model mismatch out of persisted log types', () => {
  expect(Object.values(LOG_TYPE_ENUM)).toEqual([0, 1, 2, 3, 4, 5, 6, 7])
  expect(LOG_TYPES.map((type) => type.value)).toEqual([0, 1, 2, 3, 4, 5, 6, 7])
})

it.each([true, false])(
  'sends type=-1 and the intersecting group to common log list and stats when isAdmin=%s',
  async (isAdmin) => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: {} },
    })
    const config = {
      logCategory: 'common' as const,
      isAdmin,
      page: 1,
      pageSize: 20,
      searchParams: {
        type: ['-1'],
        group: 'premium',
        startTime: 1700000000000,
        endTime: 1700003600000,
      },
      columnFilters: [],
    }
    await fetchLogsByCategory(config)
    const params = buildApiParams(config)
    await (isAdmin ? getLogStats(params) : getUserLogStats(params))
    const prefix = isAdmin ? '/api/log' : '/api/log/self'
    const requests = vi
      .mocked(api.get)
      .mock.calls.map(([url]) => new URL(url, 'http://localhost'))
    expect(requests.map((url) => url.pathname)).toEqual([
      prefix,
      `${prefix}/stat`,
    ])
    for (const request of requests) {
      expect(Object.fromEntries(request.searchParams)).toEqual({
        type: '-1',
        group: 'premium',
        p: '1',
        page_size: '20',
        start_timestamp: '1700000000',
        end_timestamp: '1700003600',
      })
    }
  }
)

it.each([
  { searchParams: { type: '-1' }, columnFilters: [] },
  { searchParams: { type: ['-1'] }, columnFilters: [] },
  {
    searchParams: { type: ['2'] },
    columnFilters: [{ id: 'type', value: ['-1'] }],
  },
])(
  'preserves model mismatch in scalar, array and table filter inputs: %j',
  (filters) => {
    expect(
      buildApiParams({
        page: 3,
        pageSize: 20,
        isAdmin: false,
        ...filters,
        searchParams: { group: 'premium', ...filters.searchParams },
      })
    ).toMatchObject({ type: -1, group: 'premium', p: 3, page_size: 20 })
  }
)
