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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { ModelCard } from '../components/model-card'
import { ModelDetailsContent } from '../components/model-details'
import type { PricingModel } from '../types'

const model: PricingModel = {
  id: 1,
  model_name: 'video-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default'],
  billing_mode: 'tiered_expr',
  billing_expr: 'tier("standard", u("seconds") * 0.4)',
  billing_usage_schema: {
    seconds: {
      type: 'number',
      unit: 'second',
      description: { en: 'Video unit price' },
    },
  },
  billing_usage_examples: [
    { label: 'Five-second video example', facts: { seconds: 5 } },
  ],
}

let queryClient: QueryClient
const originalCurrency = useSystemConfigStore.getState().config.currency

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { data: { groups: [] } } })
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
  vi.restoreAllMocks()
})

it('hides example estimates on cards while preserving the actual unit price', () => {
  render(<ModelCard model={model} onClick={() => {}} />)

  expect(screen.getByText('Video unit price')).toBeVisible()
  expect(screen.getByText('$0.4')).toBeVisible()
  expect(screen.getByText('/ s')).toBeVisible()
  expect(
    screen.queryByText(/Five-second video example/)
  ).not.toBeInTheDocument()
  expect(screen.queryByText(/≈/)).not.toBeInTheDocument()
})

it('hides detail price examples while preserving base and group unit prices', () => {
  render(
    <QueryClientProvider client={queryClient}>
      <ModelDetailsContent
        model={model}
        groupRatio={{ default: 2 }}
        usableGroup={{ default: { desc: '', ratio: 2 } }}
        endpointMap={{}}
        autoGroups={[]}
        priceRate={1}
        usdExchangeRate={1}
        tokenUnit='M'
      />
    </QueryClientProvider>
  )

  expect(screen.getByText('$0.4')).toBeVisible()
  expect(screen.getByText('$0.8')).toBeVisible()
  expect(screen.getByText('2x')).toBeVisible()
  expect(screen.queryByText('Price examples')).not.toBeInTheDocument()
  expect(screen.queryByText('Example price')).not.toBeInTheDocument()
  expect(
    screen.queryByText('Five-second video example')
  ).not.toBeInTheDocument()
  expect(screen.queryByText(/≈/)).not.toBeInTheDocument()
})
