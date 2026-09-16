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
import { act, renderHook, waitFor } from '@testing-library/react'
import { AxiosError, type AxiosAdapter } from 'axios'
import { afterEach, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { useAffiliate } from '../use-affiliate'

/** 网络适配器模拟已入账但响应丢失的情况，测试后恢复真实适配器。 */
const originalAdapter = api.defaults.adapter

afterEach(() => {
  api.defaults.adapter = originalAdapter
})

it('提现响应丢失后重试复用请求标识，成功后下一次提现使用新标识', async () => {
  const keys: string[] = []
  const adapter: AxiosAdapter = async (config) => {
    if (config.url?.endsWith('/aff_transfer')) {
      const request = JSON.parse(String(config.data))
      keys.push(request.idempotency_key)
      if (keys.length === 1) {
        throw new AxiosError('Connection lost', 'ERR_NETWORK', config)
      }
    }
    return {
      config,
      data: { success: true, data: 'referral-code' },
      status: 200,
      statusText: 'OK',
      headers: {},
    }
  }
  api.defaults.adapter = adapter
  const { result } = renderHook(() => useAffiliate())
  await waitFor(() => expect(result.current.loading).toBe(false))

  await act(async () => {
    expect(await result.current.transferQuota(500_000)).toBe(false)
  })
  await act(async () => {
    expect(await result.current.transferQuota(500_000)).toBe(true)
  })
  await act(async () => {
    expect(await result.current.transferQuota(500_000)).toBe(true)
  })

  expect(keys).toHaveLength(3)
  expect(keys[0]).toBeTruthy()
  expect(keys[1]).toBe(keys[0])
  expect(keys[2]).not.toBe(keys[1])
})

it('同一轮连续提交只发起一次提现请求', async () => {
  let transfers = 0
  api.defaults.adapter = async (config) => {
    if (config.url?.endsWith('/aff_transfer')) transfers += 1
    return {
      config,
      data: { success: true, data: 'referral-code' },
      status: 200,
      statusText: 'OK',
      headers: {},
    }
  }
  const { result } = renderHook(() => useAffiliate())
  await waitFor(() => expect(result.current.loading).toBe(false))

  await act(async () => {
    await Promise.all([
      result.current.transferQuota(500_000),
      result.current.transferQuota(500_000),
    ])
  })
  expect(transfers).toBe(1)
})
