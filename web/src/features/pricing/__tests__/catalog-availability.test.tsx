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
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ModelCard } from '../components/model-card'
import { PricingTable } from '../components/pricing-table'
import type { CatalogAvailabilityMap, PricingModel } from '../types'

const model: PricingModel = {
  id: 1,
  model_name: 'custom-seedance-video',
  quota_type: 1,
  model_ratio: 1,
  completion_ratio: 1,
  model_price: 0.2,
  enable_groups: ['default'],
  billing_plugin_variants: [
    {
      plugin_key: 'seedance-video',
      plugin_name: 'Seedance Unified Video',
      billing_expr: 'tier("base", u("count") * 0.2)',
      billing_usage_schema: { count: { type: 'number', unit: 'count' } },
    },
  ],
}
const models = [model]

let queryClient: QueryClient
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-10-16T12:00:00.000Z'))
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  queryClient.setQueryData(['perf-metrics', model.model_name], {
    success: true,
    data: { groups: [] },
  })
})
afterEach(() => {
  queryClient.clear()
  vi.useRealTimers()
})

function availability(
  status: 'available' | 'disabled',
  checkedAt = new Date().toISOString()
): CatalogAvailabilityMap {
  return { [model.model_name]: { status, checked_at: checkedAt } }
}

type CatalogViewProps = {
  seedanceAvailability: CatalogAvailabilityMap
  yuanliuAvailability?: CatalogAvailabilityMap
  isSeedanceAvailabilityUnavailable?: boolean
}

describe.each(['card', 'table'] as const)(
  '%s Seedance catalog availability',
  (view) => {
    function renderView(initialProps: CatalogViewProps) {
      function content(props: CatalogViewProps) {
        let component
        if (view === 'table') {
          component = <PricingTable models={models} {...props} />
        } else {
          component = <ModelCard model={model} onClick={vi.fn()} {...props} />
        }
        return (
          <QueryClientProvider client={queryClient}>
            {component}
          </QueryClientProvider>
        )
      }
      const result = render(content(initialProps))
      return {
        ...result,
        rerenderAvailability: (props: CatalogViewProps) =>
          result.rerender(content(props)),
      }
    }

    it('keeps a model visible as it becomes unavailable and recovers', () => {
      const result = renderView({
        seedanceAvailability: availability('available'),
      })
      expect(
        screen.getByLabelText('Seedance catalog status: Listed in catalog')
      ).toBeVisible()

      result.rerenderAvailability({
        seedanceAvailability: availability('disabled'),
      })
      expect(
        screen.getByLabelText(
          'Seedance catalog status: Upstream temporarily unavailable'
        )
      ).toBeVisible()
      expect(screen.getAllByText(model.model_name)[0]).toBeVisible()

      result.rerenderAvailability({
        seedanceAvailability: availability('available'),
      })
      expect(
        screen.getByLabelText('Seedance catalog status: Listed in catalog')
      ).toBeVisible()
      expect(
        screen.queryByLabelText(
          'Seedance catalog status: Upstream temporarily unavailable'
        )
      ).toBeNull()
    })

    it('shows unknown instead of a cached success when the latest query failed', () => {
      renderView({
        seedanceAvailability: availability('available'),
        isSeedanceAvailabilityUnavailable: true,
      })
      expect(
        screen.getByLabelText('Seedance catalog status: Unknown')
      ).toBeVisible()
      expect(
        screen.queryByLabelText('Seedance catalog status: Listed in catalog')
      ).toBeNull()
    })

    it('shows unknown when a known plugin model has no catalog result', () => {
      renderView({ seedanceAvailability: {} })
      expect(
        screen.getByLabelText('Seedance catalog status: Unknown')
      ).toBeVisible()
      expect(screen.getAllByText(model.model_name)[0]).toBeVisible()
    })

    it('expires a formerly available result after three minutes', () => {
      const props = { seedanceAvailability: availability('available') }
      const result = renderView(props)
      expect(
        screen.getByLabelText('Seedance catalog status: Listed in catalog')
      ).toBeVisible()
      vi.setSystemTime(new Date('2026-10-16T12:03:01.000Z'))
      result.rerenderAvailability(props)
      expect(
        screen.getByLabelText('Seedance catalog status: Unknown')
      ).toBeVisible()
    })

    it('keeps the Yuanliu status separate when Seedance for the same model is unavailable', () => {
      const props = {
        seedanceAvailability: availability('disabled'),
        yuanliuAvailability: availability('available'),
      }
      const result = renderView(props)
      expect(
        screen.getByLabelText(
          'Seedance catalog status: Upstream temporarily unavailable'
        )
      ).toBeVisible()
      expect(
        screen.getByLabelText('Yuanliu catalog status: Listed in catalog')
      ).toBeVisible()

      result.rerenderAvailability({
        ...props,
        isSeedanceAvailabilityUnavailable: true,
      })
      expect(
        screen.getByLabelText('Seedance catalog status: Unknown')
      ).toBeVisible()
      expect(
        screen.getByLabelText('Yuanliu catalog status: Listed in catalog')
      ).toBeVisible()
    })
  }
)

/** Readonly catalog status must remain visible without hiding the model. */
describe('model catalog availability', () => {
  it('keeps a missing Seedance model visible and labels it temporarily unavailable', () => {
    render(
      <ModelCard
        model={model}
        onClick={vi.fn()}
        seedanceAvailability={{
          [model.model_name]: {
            status: 'disabled',
            checked_at: new Date().toISOString(),
          },
        }}
      />
    )
    expect(screen.getByText(model.model_name)).toBeVisible()
    expect(
      screen.getByLabelText(
        'Seedance catalog status: Upstream temporarily unavailable'
      )
    ).toBeVisible()
  })

  it('uses the same temporarily unavailable wording for missing Yuanliu models', () => {
    render(
      <ModelCard
        model={{
          ...model,
          billing_plugin_variants: model.billing_plugin_variants?.map(
            (variant) => ({
              ...variant,
              plugin_key: 'yuanliu',
              plugin_name: 'Yuanliu Video',
            })
          ),
        }}
        onClick={vi.fn()}
        yuanliuAvailability={{
          [model.model_name]: {
            status: 'disabled',
            checked_at: new Date().toISOString(),
          },
        }}
      />
    )
    expect(
      screen.getByLabelText(
        'Yuanliu catalog status: Upstream temporarily unavailable'
      )
    ).toBeVisible()
  })
})
