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
const yuanName = 'Yuan-Seedance-2.5-QN-02'
const disabledName = 'Yuan-Wan-3.0'
const seedanceName = 'seedance-custom-video'
const now = new Date('2026-10-19T04:00:00Z')
let queryClient: QueryClient
let upstream: CatalogAvailabilityMap
let requestFails: boolean

function model(name: string, id = 1): PricingModel {
  return {
    id,
    model_name: name,
    quota_type: 1,
    model_ratio: 1,
    completion_ratio: 1,
    model_price: 0.2,
    enable_groups: ['default'],
  }
}

function entry(
  status: 'available' | 'sold_out' | 'disabled' | 'unknown',
  checkedAt = now.toISOString()
) {
  return { status, checked_at: checkedAt }
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(now)
  localStorage.clear()
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
  upstream = {}
  requestFails = false
  api.defaults.adapter = vi.fn<AxiosAdapter>(async (config) => {
    if (config.url === '/api/pricing/yuanliu-availability' && requestFails) {
      throw new Error('Synthetic catalog failure')
    }
    const models =
      config.url === '/api/pricing/yuanliu-availability' ? upstream : []
    return {
      config,
      data: { success: true, data: { models, groups: [] } },
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
  models: PricingModel[],
  availability: CatalogAvailabilityMap,
  view = 'card'
) {
  upstream = availability
  queryClient.setQueryData(['pricing'], {
    success: true,
    data: models,
    vendors: [{ id: 1, name: 'Yuan only vendor' }],
    group_ratio: { default: 1 },
    usable_group: { default: { desc: '', ratio: 1 } },
    supported_endpoint: {},
    auto_groups: [],
  })
  queryClient.setQueryData(['pricing', 'yuanliu-availability'], {
    success: true,
    data: { models: availability },
  })
  queryClient.setQueryData(['pricing', 'seedance-availability'], {
    success: true,
    data: { models: { [seedanceName]: entry('available') } },
  })
  const root = createRootRoute()
  const routeTree = root.addChildren([
    createRoute({
      getParentRoute: () => root,
      path: '/pricing/',
      component: Pricing,
    }),
    ...['/', '/dashboard', '/rankings', '/about', '/sign-in', '/sign-up'].map(
      (path) =>
        createRoute({ getParentRoute: () => root, path, component: () => null })
    ),
  ])
  const router = createRouter({
    routeTree,
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
      queryKey: ['pricing', 'yuanliu-availability'],
      exact: true,
    })
  })
}

describe.each(['card', 'table'])('%s Yuan visibility', (view) => {
  it('hides fresh sold-out and disabled Yuan models with matching counts and filters while keeping available Seedance', async () => {
    await renderPricing(
      [
        { ...model(yuanName), vendor_id: 1, tags: 'Yuan-only-tag' },
        model(disabledName, 2),
        model('normal-model', 3),
        model(seedanceName, 4),
      ],
      { [yuanName]: entry('sold_out'), [disabledName]: entry('disabled') },
      view
    )

    expect(screen.queryByText(yuanName)).toBeNull()
    expect(screen.queryByText(disabledName)).toBeNull()
    expect(screen.getByText(seedanceName)).toBeVisible()
    expect(
      screen.getByText('This site currently has 2 models enabled')
    ).toBeVisible()
    const allVendors = screen.getByTitle('All Vendors')
    expect(allVendors).toBeVisible()
    expect(within(allVendors).getByText('2')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: /Yuan only vendor/ })
    ).toBeNull()
    expect(screen.queryByRole('button', { name: /Yuan-only-tag/ })).toBeNull()
    const main = screen.getByRole('main')
    expect(
      within(main).getByText('models').previousElementSibling
    ).toHaveTextContent('2')
    expect(within(main).queryByText('/ 4')).toBeNull()
  })

  it('keeps refresh available when every Yuan model is hidden and restores available models', async () => {
    await renderPricing(
      [model(yuanName)],
      { [yuanName]: entry('sold_out') },
      view
    )
    expect(screen.queryByText(yuanName)).toBeNull()
    expect(
      screen.getByText('This site currently has 0 models enabled')
    ).toBeVisible()
    const user = userEvent.setup()
    upstream = { [yuanName]: entry('available') }
    await user.click(
      screen.getByRole('button', {
        name: 'Reload upstream status from gateway',
      })
    )
    expect(await screen.findByText(yuanName)).toBeVisible()
    expect(
      screen.getByText('This site currently has 1 models enabled')
    ).toBeVisible()
  })
})

it('keeps missing, unknown, stale and implausibly future Yuan statuses visible', async () => {
  const names = [
    yuanName,
    disabledName,
    'Yuan-Wan-3.0-Prime',
    'Yuan-Seedance-2.0-XK',
  ]
  await renderPricing(
    names.map((name, index) => model(name, index + 1)),
    {
      [disabledName]: entry('unknown'),
      [names[2]]: entry(
        'sold_out',
        new Date(now.getTime() - 180001).toISOString()
      ),
      [names[3]]: entry(
        'disabled',
        new Date(now.getTime() + 60001).toISOString()
      ),
    }
  )
  for (const name of names) expect(screen.getByText(name)).toBeVisible()
})

it('shows cached sold-out Yuan models as unknown when the latest request fails', async () => {
  await renderPricing([model(yuanName)], { [yuanName]: entry('sold_out') })
  requestFails = true
  await reload(upstream)
  expect(await screen.findByText(yuanName)).toBeVisible()
  expect(screen.getByLabelText('Yuanliu catalog status: Unknown')).toBeVisible()
})

it('closes an open model detail when its fresh Yuan status becomes disabled', async () => {
  await renderPricing([model(yuanName)], { [yuanName]: entry('available') })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Details' }))
  expect(await screen.findByRole('dialog', { name: yuanName })).toBeVisible()
  await reload({ [yuanName]: entry('disabled') })
  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: yuanName })).toBeNull()
  )
  expect(screen.queryByText(yuanName)).toBeNull()
  await reload({ [yuanName]: entry('available') })
  expect(await screen.findByText(yuanName)).toBeVisible()
  expect(screen.queryByRole('dialog', { name: yuanName })).toBeNull()
})
