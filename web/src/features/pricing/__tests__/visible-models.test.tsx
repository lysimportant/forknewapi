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
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useVisibleModels } from '../hooks/use-visible-models'
import { getCatalogAvailabilityStatus } from '../lib/catalog-availability'
import type {
  CatalogAvailability,
  CatalogAvailabilityMap,
  PricingModel,
} from '../types'

const now = Date.parse('2026-10-19T04:00:00Z')
const yuan: PricingModel = {
  id: 1,
  model_name: 'Yuan-Seedance-2.5-QN-02',
  quota_type: 1,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default'],
}
const models = [yuan]

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
})
afterEach(() => vi.useRealTimers())

function availability(
  status: CatalogAvailability['status'],
  age = 0
): CatalogAvailabilityMap {
  return {
    [yuan.model_name]: {
      status,
      checked_at: new Date(now - age).toISOString(),
    },
  }
}

it('automatically restores a hidden Yuan model once the shared three-minute freshness expires', () => {
  const map = availability('sold_out')
  const { result } = renderHook(() => useVisibleModels(models, map, false))
  expect(result.current).toEqual([])
  act(() => vi.advanceTimersByTime(180000))
  expect(result.current).toEqual([])
  act(() => vi.advanceTimersByTime(1))
  expect(result.current).toEqual(models)
  expect(getCatalogAvailabilityStatus(map[yuan.model_name])).toBe('unknown')
})

it('restores a hidden model after failure and hides it again only after a fresh successful negative result', () => {
  const map = availability('disabled')
  const { result, rerender } = renderHook(
    (failed: boolean) => useVisibleModels(models, map, failed),
    { initialProps: false }
  )
  expect(result.current).toEqual([])
  rerender(true)
  expect(result.current).toEqual(models)
  rerender(false)
  expect(result.current).toEqual([])
})

it('restores an available Yuan model while keeping non-Yuan and available Seedance models', () => {
  const normal = { ...yuan, id: 2, model_name: 'normal-model' }
  const seedance = {
    ...yuan,
    id: 3,
    model_name: 'seedance-model',
    billing_plugin_variants: [
      {
        plugin_key: 'seedance-video',
        plugin_name: 'Seedance',
        billing_expr: '',
        billing_usage_schema: {},
      },
    ],
  }
  const all = [yuan, normal, seedance]
  const seedanceCatalog: CatalogAvailabilityMap = {
    [seedance.model_name]: {
      status: 'available',
      checked_at: new Date(now).toISOString(),
    },
  }
  const { result, rerender } = renderHook(
    (map: CatalogAvailabilityMap) =>
      useVisibleModels(all, map, false, seedanceCatalog),
    { initialProps: availability('disabled') }
  )
  expect(result.current).toEqual([normal, seedance])
  rerender(availability('available'))
  expect(result.current).toEqual(all)
})

it.each([
  ['missing', {}],
  ['unknown', availability('unknown')],
  ['expired', availability('sold_out', 180001)],
  ['future beyond one minute', availability('disabled', -60001)],
  [
    'invalid timestamp',
    { [yuan.model_name]: { status: 'sold_out', checked_at: 'invalid' } },
  ],
  [
    'missing timestamp',
    { [yuan.model_name]: { status: 'disabled', checked_at: null } },
  ],
] satisfies [string, CatalogAvailabilityMap][])(
  'keeps a model visible when its Yuan status is %s',
  (_name, map) => {
    const { result } = renderHook(() => useVisibleModels(models, map, false))
    expect(result.current).toEqual(models)
  }
)

it('accepts a fresh negative status with exactly one minute of upstream clock skew', () => {
  const map = availability('sold_out', -60000)
  const { result } = renderHook(() => useVisibleModels(models, map, false))
  expect(result.current).toEqual([])
  expect(getCatalogAvailabilityStatus(map[yuan.model_name])).toBe('sold_out')
})

describe('Seedance visibility', () => {
  const seedance: PricingModel = {
    ...yuan,
    model_name: 'custom-video',
    billing_plugin_variants: [
      {
        plugin_key: 'seedance-video',
        plugin_name: 'Seedance Unified Video',
        billing_expr: '',
        billing_usage_schema: {},
      },
    ],
  }
  const seedanceModels = [seedance]

  function catalog(
    status: CatalogAvailability['status'],
    age = 0
  ): CatalogAvailabilityMap {
    return {
      [seedance.model_name]: {
        status,
        checked_at: new Date(now - age).toISOString(),
      },
    }
  }

  it.each([
    ['missing', {}],
    ['disabled', catalog('disabled')],
    ['sold out', catalog('sold_out')],
    ['unknown', catalog('unknown')],
    ['expired', catalog('available', 180001)],
    [
      'invalid timestamp',
      { 'custom-video': { status: 'available', checked_at: 'invalid' } },
    ],
  ] satisfies [string, CatalogAvailabilityMap][])(
    'hides a Seedance model when its catalog status is %s',
    (_name, map) => {
      const { result } = renderHook(() =>
        useVisibleModels(seedanceModels, {}, false, map, false)
      )
      expect(result.current).toEqual([])
    }
  )

  it('hides a cached available Seedance model after query failure and restores it after recovery', () => {
    const map = catalog('available')
    const { result, rerender } = renderHook(
      (failed: boolean) =>
        useVisibleModels(seedanceModels, {}, false, map, failed),
      { initialProps: false }
    )
    expect(result.current).toEqual(seedanceModels)
    rerender(true)
    expect(result.current).toEqual([])
    rerender(false)
    expect(result.current).toEqual(seedanceModels)
  })

  it('automatically hides an available Seedance model when its catalog result expires', () => {
    const map = catalog('available')
    const { result } = renderHook(() =>
      useVisibleModels(seedanceModels, {}, false, map, false)
    )
    expect(result.current).toEqual(seedanceModels)
    act(() => vi.advanceTimersByTime(180001))
    expect(result.current).toEqual([])
  })
})
