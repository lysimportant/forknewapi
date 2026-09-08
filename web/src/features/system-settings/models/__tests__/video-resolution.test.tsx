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
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import {
  generateTaskExprFromConfig,
  createDefaultTaskMatrixConfig,
  taskMatrixToTiers,
  tryParseTaskMatrixConfig,
} from '@/features/pricing/lib/task-expr'
import type { BillingUsageSchema } from '@/features/pricing/types'

import { TaskUsagePricingEditor } from '../task-usage-pricing-editor'

/** Grok 的原始顺序用于验证默认 fallback 与价格索引保持不变。 */
const schema: BillingUsageSchema = {
  count: { type: 'number', unit: 'count' },
  resolution: { enum: ['unspecified', '480p', '720p', '1080p', '4k'] },
}

describe('视频清晰度浏览与原始计费隔离', () => {
  test('可灵显示官方模式对应说明，同时保持原生Units定价', () => {
    render(
      <TaskUsagePricingEditor
        isVideo
        modelName='kling-v1'
        billingExpr='tier("base", u("units") * 0.14)'
        requestRuleExpr=''
        usageSchema={{ units: { type: 'number', unit: 'credit' } }}
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(screen.getByText('std → 720P; pro → 1080P')).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(
      screen
        .getAllByRole('spinbutton')
        .some((input) => (input as HTMLInputElement).value === '0.14')
    ).toBe(true)
  })

  test('未确认型号不猜分辨率，已有价格仍可在其他规格中编辑', async () => {
    render(
      <TaskUsagePricingEditor
        isVideo
        modelName='vidu1.5'
        billingExpr='tier("base", u("seconds") * 0.4)'
        requestRuleExpr=''
        usageSchema={{
          seconds: { type: 'number', unit: 'second' },
          resolution: { enum: ['360p', '720p'] },
        }}
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(screen.getByText('Video resolution: Unknown')).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Additional provider specifications' })
    )
    expect(
      screen.getByRole('spinbutton', { name: 'seconds: 360p' })
    ).toHaveValue(0.4)
  })

  test('Grok只主列官方三档，旧4K独立编辑且默认价格保留', async () => {
    const user = userEvent.setup()
    const rows = createDefaultTaskMatrixConfig(schema).rows.map(
      (row, index) => ({ ...row, unitPrices: { count: index + 1 } })
    )
    const expression = generateTaskExprFromConfig(
      { tiers: taskMatrixToTiers({ rows }, schema) },
      schema
    )
    const onChange = vi.fn()
    render(
      <TaskUsagePricingEditor
        isVideo
        modelName='grok-imagine-video-1.5'
        billingExpr={expression}
        requestRuleExpr=''
        usageSchema={schema}
        onBillingExprChange={onChange}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
      '480p',
      '720p',
      '1080p',
    ])
    await user.click(
      screen.getByRole('button', { name: 'Additional provider specifications' })
    )
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('spinbutton', { name: 'count: 4k' })).toHaveValue(5)
    fireEvent.change(screen.getByRole('spinbutton', { name: 'count: 4k' }), {
      target: { value: '9' },
    })
    const saved = tryParseTaskMatrixConfig(onChange.mock.lastCall?.[0], schema)
    expect(saved?.rows).toEqual(
      rows.map((row, index) =>
        index === 4 ? { ...row, unitPrices: { count: 9 } } : row
      )
    )
    onChange.mockClear()
    await user.click(
      screen.getByRole('button', { name: 'Provider default specifications' })
    )
    expect(
      screen.getByRole('spinbutton', {
        name: 'count: Provider default (not specified)',
      })
    ).toHaveValue(1)
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.queryByText(/^unspecified$/)).not.toBeInTheDocument()
  })

  test.each([
    {
      name: 'Hailuo',
      values: ['512P', '768P', '720P', '1080P', '2K'],
      expected: ['512P', '720P', '768P', '1080P', '2K'],
    },
    {
      name: 'Vidu',
      values: ['360p', '540p', '720p', '1080p'],
      expected: ['360p', '540p', '720p', '1080p'],
    },
    {
      name: 'Sora',
      field: 'size',
      values: ['1792x1024', '720x1280', '1280x720', '1024x1792'],
      expected: ['720x1280', '1280x720', '1792x1024', '1024x1792'],
    },
  ])(
    '$name只展示真实规格且按短边升序，不写回定价',
    async ({ values, expected, field = 'resolution' }) => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(
        <TaskUsagePricingEditor
          isVideo
          billingExpr='tier("base", u("seconds") * 0.4)'
          requestRuleExpr=''
          usageSchema={{
            seconds: { type: 'number', unit: 'second' },
            [field]: { enum: values },
          }}
          onBillingExprChange={onChange}
          onRequestRuleExprChange={vi.fn()}
        />
      )
      expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual(
        expected
      )
      for (const value of expected) {
        await user.click(screen.getByRole('tab', { name: value }))
        expect(
          screen.getByRole('spinbutton', { name: `seconds: ${value}` })
        ).toHaveValue(0.4)
      }
      expect(onChange).not.toHaveBeenCalled()
    }
  )

  test.each([
    {
      name: 'Kling',
      schema: { units: { type: 'number', unit: 'credit' } },
      priceLabel: 'units',
    },
    {
      name: 'Jimeng',
      schema: {
        seconds: { type: 'number', unit: 'second' },
        product: { enum: ['v30_720p', 'v30_pro'] },
      },
      priceLabel: 'seconds: v30_720p',
    },
  ] as { name: string; schema: BillingUsageSchema; priceLabel: string }[])(
    '$name无清晰度声明时保留原生价格，不虚构档位',
    ({ schema, priceLabel }) => {
      const field = Object.keys(schema)[0]
      render(
        <TaskUsagePricingEditor
          isVideo
          billingExpr={`tier("base", u("${field}") * 0.4)`}
          requestRuleExpr=''
          usageSchema={schema}
          onBillingExprChange={vi.fn()}
          onRequestRuleExprChange={vi.fn()}
        />
      )
      expect(screen.queryByRole('tab')).not.toBeInTheDocument()
      if (priceLabel.includes(':')) {
        expect(
          screen.getByRole('spinbutton', { name: priceLabel })
        ).toHaveValue(0.4)
      } else {
        expect(
          screen
            .getAllByRole('spinbutton')
            .some((input) => (input as HTMLInputElement).value === '0.4')
        ).toBe(true)
      }
    }
  )
})

test('超过五档仍可编辑额外规格，Enter沿可见行移动且不改价格', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <TaskUsagePricingEditor
      isVideo
      billingExpr='tier("base", u("seconds") * 0.4)'
      requestRuleExpr=''
      usageSchema={{
        seconds: { type: 'number', unit: 'second' },
        resolution: { enum: ['8k', '4k', '2k', '1080p', '720p', '480p'] },
        audio: { enum: ['true', 'false'] },
      }}
      onBillingExprChange={onChange}
      onRequestRuleExprChange={vi.fn()}
    />
  )
  expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
    '480p',
    '720p',
    '1080p',
    '2k',
    '4k',
  ])
  await user.click(
    screen.getByRole('spinbutton', { name: 'seconds: true·480p' })
  )
  await user.keyboard('{Enter}')
  expect(
    screen.getByRole('spinbutton', { name: 'seconds: false·480p' })
  ).toHaveFocus()
  await user.click(
    screen.getByRole('button', { name: 'Additional provider specifications' })
  )
  expect(
    screen.getByRole('spinbutton', { name: 'seconds: true·8k' })
  ).toHaveValue(0.4)
  expect(onChange).not.toHaveBeenCalled()
})

test('预览规格按原生值排序且默认值末尾，选择只改变预览，时长始终可见', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <TaskUsagePricingEditor
      isVideo
      billingExpr='tier("base", u("seconds") * 0.4)'
      requestRuleExpr=''
      usageSchema={{
        seconds: { type: 'number', unit: 'second' },
        resolution: {
          enum: ['unspecified', '768P', '512P', '720P', '2K', '1080P'],
        },
      }}
      onBillingExprChange={onChange}
      onRequestRuleExprChange={vi.fn()}
    />
  )
  const resolution = screen.getByRole('combobox', { name: 'resolution' })
  expect(resolution).toHaveValue('Provider default (not specified)')
  await user.click(resolution)
  expect(
    (await screen.findAllByRole('option')).map((option) => option.textContent)
  ).toEqual([
    '512P',
    '720P',
    '768P',
    '1080P',
    '2K',
    'Provider default (not specified)',
  ])
  await user.click(screen.getByRole('option', { name: /^768P$/ }))
  expect(resolution).toHaveValue('768P')
  expect(screen.getByText(/Hit tier.*768P/)).toBeInTheDocument()
  expect(
    screen
      .getAllByRole('spinbutton')
      .some((input) => (input as HTMLInputElement).value === '5')
  ).toBe(true)
  expect(onChange).not.toHaveBeenCalled()
})
