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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef, useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import type { ModelPricingPluginVariant } from '@/features/model-pricing/api'
import { USD_PRICING_CURRENCY } from '@/features/model-pricing/currency'
import { combineBillingExpr } from '@/features/pricing/lib/billing-expr'
import type { BillingUsageSchema } from '@/features/pricing/types'
import { api } from '@/lib/api'
import { usePricingPreferencesStore } from '@/stores/pricing-preferences-store'

import {
  ModelPricingEditorPanel,
  type ModelPricingEditorPanelHandle,
} from '../model-pricing-sheet'
import { TaskPluginPricingEditor } from '../task-plugin-pricing-editor'

const yuanSchema: BillingUsageSchema = {
  count: { type: 'number', unit: 'count' },
  seconds: { type: 'number', unit: 'second' },
  resolution: { enum: ['480p', '720p', '1080p', '2k'] },
}
const resolutionExpression =
  'u("resolution") == "480p" ? tier("480p", u("seconds") * 0.01)\n' +
  '  : u("resolution") == "720p" ? tier("720p", u("seconds") * 0.02)\n' +
  '  : u("resolution") == "1080p" ? tier("1080p", u("seconds") * 0.03)\n' +
  '  : tier("2k", u("seconds") * 0.04)'
let client: QueryClient | undefined

afterEach(() => {
  client?.clear()
  localStorage.clear()
  vi.restoreAllMocks()
})

function yuanVariant(): ModelPricingPluginVariant {
  return {
    plugin_key: 'yuanliu',
    plugin_name: 'Yuan',
    usage_schema: yuanSchema,
    configured: '',
    effective: '',
    compatible: true,
  }
}

/** 通过真实父级受控草稿观察编辑器的对外更新。 */
function renderProvider(expression?: string, modelExpression = '1') {
  const change = vi.fn<(next: Record<string, string>) => void>()
  function ProviderDraft() {
    const [expressions, setExpressions] = useState<Record<string, string>>(
      expression === undefined ? {} : { yuanliu: expression }
    )
    return (
      <TaskPluginPricingEditor
        variants={[yuanVariant()]}
        expressions={expressions}
        onChange={(update) => {
          const next =
            typeof update === 'function' ? update(expressions) : update
          change(next)
          setExpressions(next)
        }}
        modelExpression={modelExpression}
        currency={USD_PRICING_CURRENCY}
      >
        <span>Model default editor</span>
      </TaskPluginPricingEditor>
    )
  }
  render(<ProviderDraft />)
  return change
}

it('opens saved Yuan call pricing with three billing modes without writing the draft', async () => {
  const original = 'tier( "legacy", u("count") * 0.25 )'
  const rules = '(param("watermark") == true ? 1.5 : 1)'
  const change = renderProvider(combineBillingExpr(original, rules))
  await userEvent.setup().click(screen.getByRole('tab', { name: 'Yuan' }))
  expect(screen.getByRole('tab', { name: 'Per-call' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(screen.getByRole('textbox', { name: 'Price per call' })).toHaveValue(
    '0.25'
  )
  expect(
    screen.getByRole('tab', { name: 'Seconds × resolution' })
  ).toBeVisible()
  expect(screen.getByRole('tab', { name: 'Expression editor' })).toBeVisible()
  expect(
    screen.getByRole('textbox', { name: 'Request rule pricing' })
  ).toHaveValue(rules)
  expect(change).not.toHaveBeenCalled()
})

it('writes Yuan resolution and seconds prices while retaining call and provider drafts', async () => {
  const original = 'tier("base", u("count") * 0.25)'
  const rules = '(param("watermark") == true ? 1.5 : 1)'
  const change = renderProvider(combineBillingExpr(original, rules))
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Yuan' }))
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  const hdPrice = screen.getByRole('textbox', {
    name: '1080p Price per second',
  })
  await user.clear(hdPrice)
  await user.type(hdPrice, '0.08')
  expect(change).toHaveBeenLastCalledWith({
    yuanliu: combineBillingExpr(
      'u("resolution") == "480p" ? tier("480p", u("seconds") * 0)\n' +
        '  : u("resolution") == "720p" ? tier("720p", u("seconds") * 0)\n' +
        '  : u("resolution") == "1080p" ? tier("1080p", u("seconds") * 0.08)\n' +
        '  : tier("2k", u("seconds") * 0)',
      rules
    ),
  })
  await user.click(screen.getByRole('tab', { name: 'Per-call' }))
  expect(screen.getByRole('textbox', { name: 'Price per call' })).toHaveValue(
    '0.25'
  )
  expect(change).toHaveBeenLastCalledWith({
    yuanliu: combineBillingExpr(original, rules),
  })
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  await user.click(screen.getByRole('tab', { name: 'Default' }))
  await user.click(screen.getByRole('tab', { name: 'Yuan' }))
  expect(
    screen.getByRole('textbox', { name: '1080p Price per second' })
  ).toHaveValue('0.08')
})

it('reads all four saved Yuan resolution prices and simulates with its own usage field', async () => {
  const change = renderProvider(resolutionExpression)
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Yuan' }))
  expect(
    screen.getByRole('tab', { name: 'Seconds × resolution' })
  ).toHaveAttribute('aria-selected', 'true')
  for (const [resolution, price] of [
    ['480p', '0.01'],
    ['720p', '0.02'],
    ['1080p', '0.03'],
    ['2K', '0.04'],
  ]) {
    expect(
      screen.getByRole('textbox', {
        name: `${resolution} Price per second`,
      })
    ).toHaveValue(price)
  }
  await user.click(screen.getByRole('button', { name: 'Request simulation' }))
  expect(await screen.findByRole('status')).toHaveTextContent(
    'Simulated request cost: $0.05'
  )
  expect(screen.getByRole('status')).toHaveTextContent('Hit tier: 480p')
  expect(change).not.toHaveBeenCalled()
})

it('preserves legacy Yuan formulas and request rules unless replacement is confirmed', async () => {
  const custom = 'tier("legacy", u("seconds") * 0.04 + 0.2)'
  const rules = '(param("watermark") == true ? 1.5 : 1)'
  const change = renderProvider(combineBillingExpr(custom, rules))
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Yuan' }))
  expect(
    screen.getByRole('tab', { name: 'Expression editor' })
  ).toHaveAttribute('aria-selected', 'true')
  expect(
    screen.getByRole('textbox', { name: 'Billing expression' })
  ).toHaveValue(combineBillingExpr(custom, rules))
  expect(change).not.toHaveBeenCalled()
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  const dialog = screen.getByRole('alertdialog', {
    name: 'Switch billing method?',
  })
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  expect(change).not.toHaveBeenCalled()
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  await user.click(screen.getByRole('button', { name: 'Apply billing method' }))
  expect(change.mock.lastCall?.[0].yuanliu).toContain('u("resolution")')
  expect(change.mock.lastCall?.[0].yuanliu).not.toContain('u("count")')
  expect(change.mock.lastCall?.[0].yuanliu).toContain(rules)
  await user.click(screen.getByRole('tab', { name: 'Expression editor' }))
  expect(change).toHaveBeenLastCalledWith({
    yuanliu: combineBillingExpr(custom, rules),
  })
})

it('starts a new Yuan override with exclusive call pricing for incompatible model pricing', async () => {
  const change = renderProvider(undefined, 'tier("base", u("credits") * 0)')
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Yuan' }))
  await user.click(
    screen.getByRole('switch', { name: 'Set separately for this provider' })
  )
  expect(screen.getByRole('tab', { name: 'Per-call' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(change).toHaveBeenLastCalledWith({
    yuanliu: 'tier("base", u("count") * 0)',
  })
})

it.each([
  { kind: 'unconfigured', expression: '', expectedPrice: '0' },
  {
    kind: 'previously configured',
    expression: 'tier( "legacy", u("count") * 0.35 )',
    expectedPrice: '0.35',
  },
])(
  'opens $kind channel-routed Yuan pricing without rewriting it',
  async (saved) => {
    usePricingPreferencesStore.setState({ currency: 'USD' })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: [], vendors: [] },
    })
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { effective: {} } },
    })
    const save = vi.spyOn(api, 'patch')
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const ref = createRef<ModelPricingEditorPanelHandle>()
    const dirty = vi.fn()
    render(
      <QueryClientProvider client={client}>
        <ModelPricingEditorPanel
          ref={ref}
          embedded
          editData={{
            name: 'arbitrary-yuan-video-id',
            billingMode: 'tiered_expr',
            billingExpr: saved.expression,
          }}
          taskPluginKey='yuanliu'
          usageSchema={yuanSchema}
          onDirtyChange={dirty}
        />
      </QueryClientProvider>
    )
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: 'Per-call' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
    )
    expect(screen.getByRole('textbox', { name: 'Price per call' })).toHaveValue(
      saved.expectedPrice
    )
    expect((await ref.current?.commitDraft())?.billingExpr).toBe(
      saved.expression || 'tier("base", u("count") * 0)'
    )
    expect(dirty).toHaveBeenLastCalledWith(false)
    expect(save).not.toHaveBeenCalled()
  }
)
