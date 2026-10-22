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
import i18next from 'i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ModelCard } from '../components/model-card'
import { ModelDetailsContent } from '../components/model-details'
import { ModelPriceCell } from '../components/model-price-cell'
import { parseTaskTiersFromExpr } from '../lib/billing-expr'
import type { BillingUsageSchema, PricingModel } from '../types'

const imageSchema: BillingUsageSchema = {
  count: {
    type: 'number',
    unit: 'count',
    unitLabel: { en: 'image', zh: '张' },
    description: { en: 'Image generation unit price', zh: '图片生成单价' },
  },
}
const legacyExpression = 'tier("base", u("count")*3.5 + u("seconds")*0)'
const imageModel: PricingModel = {
  id: 1,
  model_name: 'image-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default'],
  billing_mode: 'tiered_expr',
  billing_expr: legacyExpression,
  billing_usage_schema: imageSchema,
}
let client: QueryClient | undefined

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

afterEach(() => {
  cleanup()
  client?.clear()
  client = undefined
  vi.restoreAllMocks()
})

it('reads image prices when a legacy expression includes an undeclared zero-price video unit', () => {
  expect(parseTaskTiersFromExpr(legacyExpression, imageSchema)).toEqual([
    { label: 'base', conditions: [], constant: 0, unitPrices: { count: 3.5 } },
  ])
})

it.each(['card', 'table'] as const)(
  'shows the image price without video pricing in the %s for a legacy zero video term',
  (surface) => {
    if (surface === 'card') {
      render(<ModelCard model={imageModel} onClick={() => {}} />)
      expect(screen.getByText('$3.5')).toBeVisible()
      expect(screen.getByText('/ image')).toBeVisible()
      expect(screen.getByText('Image generation unit price')).toBeVisible()
    } else {
      render(
        <table>
          <tbody>
            <tr>
              <td>
                <ModelPriceCell model={imageModel} />
              </td>
            </tr>
          </tbody>
        </table>
      )
      expect(screen.getByText('3.5/image')).toBeVisible()
      expect(screen.getByText('count')).toBeVisible()
    }
    expect(
      screen.queryByText('Video generation unit price')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('seconds')).not.toBeInTheDocument()
    expect(screen.queryByText(/\/\s*second/)).not.toBeInTheDocument()
    expect(
      screen.queryByText('Special billing expression')
    ).not.toBeInTheDocument()
  }
)

it('shows base and group image prices without video rows in details for a legacy zero video term', () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { data: { groups: [] } } })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ModelDetailsContent
        model={imageModel}
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
  expect(screen.getByText('$3.5')).toBeVisible()
  expect(screen.getByText('$7')).toBeVisible()
  expect(screen.getAllByText('/ image')).toHaveLength(2)
  expect(screen.getAllByText('Image generation unit price')).toHaveLength(2)
  expect(
    screen.queryByText('Video generation unit price')
  ).not.toBeInTheDocument()
  expect(screen.queryByText('seconds')).not.toBeInTheDocument()
  expect(screen.queryByText(/\/\s*second/)).not.toBeInTheDocument()
  expect(
    screen.queryByText('Special billing expression')
  ).not.toBeInTheDocument()
})

it('projects image pricing onto a narrowed profile while keeping the declared prices exact', () => {
  // RC.46 projects saved expressions onto the current plugin profile. Dropped
  // numeric facts are assumed to remain zero in the plugin's measured usage.
  const expression = 'tier("base", u("count")*3.5 + u("seconds")*0.1)'
  expect(
    parseTaskTiersFromExpr(expression, {
      ...imageSchema,
      seconds: { type: 'number', unit: 'second' },
    })
  ).toEqual([
    {
      label: 'base',
      conditions: [],
      constant: 0,
      unitPrices: { count: 3.5, seconds: 0.1 },
    },
  ])
  expect(parseTaskTiersFromExpr(expression, imageSchema)).toEqual([
    { label: 'base', conditions: [], constant: 0, unitPrices: { count: 3.5 } },
  ])
})

it.each([
  ['computed zero price', 'tier("base", u("count")*3.5 + u("seconds")*(1-1))'],
  ['no schema-backed price', 'tier("base", u("seconds")*0)'],
  ['reversed product', 'tier("base", u("count")*3.5 + 0*u("seconds"))'],
  ['unsupported scale', 'tier("base", u("count")*3.5 + u("seconds")*0/1000)'],
])(
  'rejects %s instead of extracting a partial image price',
  (_case, expression) => {
    expect(parseTaskTiersFromExpr(expression, imageSchema)).toEqual([])
  }
)

it('keeps declared zero-price units in the parsed prices', () => {
  const schema: BillingUsageSchema = {
    ...imageSchema,
    seconds: { type: 'number', unit: 'second' },
  }
  expect(
    parseTaskTiersFromExpr(legacyExpression, schema)[0]?.unitPrices
  ).toEqual({
    count: 3.5,
    seconds: 0,
  })
})

it('rejects declared invalid usage fields even when their price is zero', () => {
  const schema: BillingUsageSchema = {
    ...imageSchema,
    seconds: { type: 'boolean' },
  }
  expect(parseTaskTiersFromExpr(legacyExpression, schema)).toEqual([])
})
