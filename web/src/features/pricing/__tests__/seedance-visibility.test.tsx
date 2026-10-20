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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { Pricing } from '../index'
import type { CatalogAvailabilityMap, PricingModel } from '../types'

const originalAdapter = api.defaults.adapter
const seedanceName = 'custom-seedance-video'
const now = new Date('2026-10-20T04:00:00Z')
let queryClient: QueryClient
let upstream: CatalogAvailabilityMap
let requestFails: boolean

const seedanceModel: PricingModel = {
  id: 1,
  model_name: seedanceName,
  quota_type: 1,
  model_ratio: 1,
  completion_ratio: 1,
  model_price: 0.2,
  enable_groups: ['default'],
  vendor_id: 1,
  tags: 'Seedance-only-tag',
  billing_plugin_variants: [
    {
      plugin_key: 'seedance-video',
      plugin_name: 'Seedance Unified Video',
      billing_expr: 'tier("base", u("count") * 0.2)',
      billing_usage_schema: { count: { type: 'number', unit: 'count' } },
    },
  ],
}

function catalog(status: 'available' | 'disabled'): CatalogAvailabilityMap {
  return { [seedanceName]: { status, checked_at: now.toISOString() } }
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(now)
  localStorage.clear()
  requestFails = false
  queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: Infinity },
    },
  })
  queryClient.setQueryData(['status'], { system_name: 'Test Gateway' })
  queryClient.setQueryData(['notice'], { success: true, data: '' })
  queryClient.setQueryData(['perf-metrics-summary', 24], {
    success: true,
    data: { models: [] },
  })
  api.defaults.adapter = vi.fn<AxiosAdapter>(async (config) => {
    const isSeedance = config.url === '/api/pricing/seedance-availability'
    if (isSeedance && requestFails) throw new Error('Synthetic catalog failure')
    return {
      config,
      data: {
        success: true,
        data: { models: isSeedance ? upstream : [], groups: [] },
      },
      status: 200,
      statusText: 'OK',
      headers: {},
    }
  })
})

afterEach(() => {
  queryClient.clear()
  api.defaults.adapter = originalAdapter
  vi.useRealTimers()
  localStorage.clear()
})

async function renderPricing(
  availability: CatalogAvailabilityMap,
  view = 'card'
) {
  upstream = availability
  queryClient.setQueryData(['pricing'], {
    success: true,
    data: [
      seedanceModel,
      {
        ...seedanceModel,
        id: 2,
        model_name: 'normal-model',
        vendor_id: undefined,
        tags: '',
        billing_plugin_variants: undefined,
      },
    ],
    vendors: [{ id: 1, name: 'Seedance-only vendor' }],
    group_ratio: { default: 1 },
    usable_group: { default: { desc: '', ratio: 1 } },
    supported_endpoint: {},
    auto_groups: [],
  })
  queryClient.setQueryData(['pricing', 'seedance-availability'], {
    success: true,
    data: { models: availability },
  })
  queryClient.setQueryData(['pricing', 'yuanliu-availability'], {
    success: true,
    data: { models: {} },
  })
  const root = createRootRoute()
  const router = createRouter({
    routeTree: root.addChildren([
      createRoute({
        getParentRoute: () => root,
        path: '/pricing/',
        component: Pricing,
      }),
      ...['/', '/dashboard', '/rankings', '/about', '/sign-in', '/sign-up'].map(
        (path) =>
          createRoute({
            getParentRoute: () => root,
            path,
            component: () => null,
          })
      ),
    ]),
    history: createMemoryHistory({
      initialEntries: [`/pricing/?view=${view}`],
    }),
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('heading', { name: 'Model Square' })
}

async function reload(availability: CatalogAvailabilityMap) {
  upstream = availability
  await act(async () => {
    await queryClient.refetchQueries({
      queryKey: ['pricing', 'seedance-availability'],
      exact: true,
    })
  })
}

describe.each(['card', 'table'])('%s Seedance visibility', (view) => {
  it('hides unavailable Seedance models, counts and filters and restores them on refresh', async () => {
    await renderPricing(catalog('disabled'), view)
    expect(screen.queryByText(seedanceName)).toBeNull()
    expect(screen.getByText('normal-model')).toBeVisible()
    expect(
      screen.getByText('This site currently has 1 models enabled')
    ).toBeVisible()
    expect(
      within(screen.getByTitle('All Vendors')).getByText('1')
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: /Seedance-only vendor/ })
    ).toBeNull()
    expect(
      screen.queryByRole('button', { name: /Seedance-only-tag/ })
    ).toBeNull()
    expect(
      screen.queryByLabelText(
        'Seedance catalog status: Upstream temporarily unavailable'
      )
    ).toBeNull()
    upstream = catalog('available')
    await userEvent.setup().click(
      screen.getByRole('button', {
        name: 'Reload upstream status from gateway',
      })
    )
    expect(await screen.findByText(seedanceName)).toBeVisible()
    expect(
      screen.getByText('This site currently has 2 models enabled')
    ).toBeVisible()
  })
})

it('hides Seedance with no catalog result or a failed query until fresh availability recovers', async () => {
  await renderPricing({})
  expect(screen.queryByText(seedanceName)).toBeNull()
  await reload(catalog('available'))
  expect(await screen.findByText(seedanceName)).toBeVisible()
  requestFails = true
  await reload(upstream)
  await waitFor(() => expect(screen.queryByText(seedanceName)).toBeNull())
  requestFails = false
  await reload(catalog('available'))
  expect(await screen.findByText(seedanceName)).toBeVisible()
})

it('closes selected Seedance details when unavailable without reopening them after recovery', async () => {
  await renderPricing(catalog('available'))
  const card = screen
    .getByRole('heading', { name: seedanceName })
    .closest('[data-slot="card"]')
  if (!(card instanceof HTMLElement)) {
    throw new Error('Missing Seedance model card')
  }
  await userEvent
    .setup()
    .click(within(card).getByRole('button', { name: 'Details' }))
  expect(
    await screen.findByRole('dialog', { name: seedanceName })
  ).toBeVisible()
  await reload(catalog('disabled'))
  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: seedanceName })).toBeNull()
  )
  expect(screen.queryByText(seedanceName)).toBeNull()
  await reload(catalog('available'))
  expect(await screen.findByText(seedanceName)).toBeVisible()
  expect(screen.queryByRole('dialog', { name: seedanceName })).toBeNull()
})
