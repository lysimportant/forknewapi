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
import { describe, expect, it } from 'vitest'

import { getCardExamplePrice } from '../lib/dynamic-price'
import { withPluginPricing } from '../lib/plugin-pricing'
import type { PricingModel } from '../types'

function seedanceModel(expression: string): PricingModel {
  return {
    id: 1,
    model_name: 'custom-video',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    billing_plugin_variants: [
      {
        plugin_key: 'seedance-video',
        plugin_name: 'Seedance Unified Video',
        billing_expr: expression,
        billing_usage_schema: {
          count: { type: 'number', unit: 'count' },
          seconds: { type: 'number', unit: 'second' },
          video_resolution: { enum: ['480p'] },
        },
        billing_usage_examples: [
          {
            label: '480p · 5s',
            facts: { count: 1, seconds: 5, video_resolution: '480p' },
          },
        ],
      },
    ],
  }
}

describe('Seedance square price examples', () => {
  it('shows resolution without a duration for per-call pricing, including older plugin labels', () => {
    const result = getCardExamplePrice(
      seedanceModel('tier("base", u("count") * 2)'),
      { tokenUnit: 'M' }
    )
    expect(result?.label).toBe('480p')
    expect(result?.formatted).toContain('2')
  })

  it('omits a duration-dependent estimate rather than presenting it as a price for any duration', () => {
    expect(
      getCardExamplePrice(seedanceModel('tier("base", u("seconds") * 2)'), {
        tokenUnit: 'M',
      })
    ).toBeNull()
  })

  it('applies the same labels to detail examples without changing the measured facts', () => {
    const model = seedanceModel('tier("base", u("count") * 2)')
    const variant = model.billing_plugin_variants?.[0]
    if (!variant) throw new Error('Missing test plugin variant')
    const example = withPluginPricing(model, variant)
      .billing_usage_examples?.[0]
    expect(example?.label).toBe('480p')
    expect(example?.facts.seconds).toBe(5)
    expect(variant.billing_usage_examples?.[0]?.label).toBe('480p · 5s')
  })

  it('keeps duration labels for other plugins', () => {
    const model = seedanceModel('tier("base", u("seconds") * 2)')
    const variant = model.billing_plugin_variants?.[0]
    if (!variant) throw new Error('Missing test plugin variant')
    variant.plugin_key = 'other-video'
    expect(getCardExamplePrice(model, { tokenUnit: 'M' })?.label).toBe(
      '480p · 5s'
    )
  })
})
