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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { useSeedanceAvailability } from '../hooks/use-seedance-availability'
import { useYuanliuAvailability } from '../hooks/use-yuanliu-availability'
import type {
  CatalogAvailability,
  SeedanceAvailabilityResponse,
} from '../types'

const modelName = 'shared-video-model'

function response(status: CatalogAvailability['status']): {
  data: SeedanceAvailabilityResponse
} {
  return {
    data: {
      success: true,
      data: {
        models: {
          [modelName]: { status, checked_at: new Date().toISOString() },
        },
      },
    },
  }
}

let queryClient: QueryClient
beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
})
afterEach(() => {
  queryClient.clear()
  vi.useRealTimers()
})

function wrapper(props: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      {props.children}
    </QueryClientProvider>
  )
}

describe('Seedance catalog polling', () => {
  it('checks every minute and reports available, missing, and recovered states', async () => {
    vi.useFakeTimers()
    const request = vi
      .spyOn(api, 'get')
      .mockImplementationOnce(async () => response('available'))
      .mockImplementationOnce(async () => response('disabled'))
      .mockImplementationOnce(async () => response('available'))
    const { result } = renderHook(() => useSeedanceAvailability(), { wrapper })

    await act(() => vi.advanceTimersByTimeAsync(0))
    expect(result.current.data?.data.models[modelName].status).toBe('available')
    expect(request).toHaveBeenCalledWith('/api/pricing/seedance-availability')
    await act(() => vi.advanceTimersByTimeAsync(59999))
    expect(request).toHaveBeenCalledTimes(1)

    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(request).toHaveBeenCalledTimes(2)
    // React Query delivers the completed request on the next timer turn.
    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(result.current.data?.data.models[modelName].status).toBe('disabled')

    await act(() => vi.advanceTimersByTimeAsync(59999))
    expect(request).toHaveBeenCalledTimes(3)
    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(result.current.data?.data.models[modelName].status).toBe('available')
  })

  it('exposes a request error while retaining the last catalog identity for the UI fallback', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce(response('available'))
      .mockRejectedValueOnce(new Error('catalog unavailable'))
    const { result } = renderHook(() => useSeedanceAvailability(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    await act(async () => {
      await result.current.refetch()
    })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data?.data.models[modelName].status).toBe('available')
  })

  it('treats a business failure response as a failed catalog query', async () => {
    const request = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: false,
        message: 'catalog unavailable',
        data: { models: {} },
      },
    })
    const { result } = renderHook(() => useSeedanceAvailability(), { wrapper })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('uses separate queries for Seedance and Yuanliu even when model names match', async () => {
    const request = vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/pricing/seedance-availability') {
        return response('disabled')
      }
      if (url === '/api/pricing/yuanliu-availability') {
        return response('available')
      }
      throw new Error('Unexpected catalog request')
    })
    const { result } = renderHook(
      () => ({
        seedance: useSeedanceAvailability(),
        yuanliu: useYuanliuAvailability(),
      }),
      { wrapper }
    )
    await waitFor(() => {
      expect(result.current.seedance.isSuccess).toBe(true)
      expect(result.current.yuanliu.isSuccess).toBe(true)
    })
    expect(result.current.seedance.data?.data.models[modelName].status).toBe(
      'disabled'
    )
    expect(result.current.yuanliu.data?.data.models[modelName].status).toBe(
      'available'
    )

    request.mockRejectedValueOnce(new Error('Seedance temporarily unavailable'))
    await act(async () => {
      await result.current.seedance.refetch()
    })
    await waitFor(() => expect(result.current.seedance.isError).toBe(true))
    expect(result.current.yuanliu.isSuccess).toBe(true)
    expect(result.current.yuanliu.data?.data.models[modelName].status).toBe(
      'available'
    )
  })
})
