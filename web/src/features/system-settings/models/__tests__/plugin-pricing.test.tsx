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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import type {
  ModelPricingConfig,
  ModelPricingPluginVariant,
} from '@/features/model-pricing/api'
import { USD_PRICING_CURRENCY } from '@/features/model-pricing/currency'
import { ModelPricingPanel } from '@/features/model-pricing/model-pricing-panel'
import { pricingOptions } from '@/features/model-pricing/pricing'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { usePricingPreferencesStore } from '@/stores/pricing-preferences-store'

import {
  ModelPricingEditorPanel,
  type ModelPricingEditorPanelHandle,
  type ModelRatioData,
} from '../model-pricing-sheet'
import { TaskPluginPricingEditor } from '../task-plugin-pricing-editor'

let client: QueryClient | undefined

afterEach(() => {
  client?.clear()
  useAuthStore.getState().auth.setUser(null)
  localStorage.clear()
  vi.restoreAllMocks()
})

it('keeps provider drafts across tabs and saves nested expressions with the model version', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'administrator', role: 100 })
  usePricingPreferencesStore.setState({ currency: 'USD' })
  const expression = 'tier("base", u("seconds") * 0.4)'
  const values = {
    'billing_setting.billing_mode': 'tiered_expr',
    'billing_setting.billing_expr': expression,
  }
  const snapshot: ModelPricingConfig = {
    entries: [
      {
        model_name: 'shared',
        version: 'version-with-providers',
        configured: values,
        effective: values,
        usage_schema: {
          seconds: {
            type: 'number',
            unit: 'second',
            description: 'Video unit price',
          },
        },
        plugin_variants: [
          {
            plugin_key: 'alpha',
            plugin_name: 'Alpha',
            usage_schema: {
              seconds: {
                type: 'number',
                unit: 'second',
                description: 'Video unit price',
              },
            },
            configured: '',
            effective: expression,
            compatible: true,
          },
          {
            plugin_key: 'beta',
            plugin_name: 'Beta',
            usage_schema: {
              credits: {
                type: 'number',
                unit: 'credit',
                description: 'Credit unit price',
              },
            },
            configured: '',
            effective: expression,
            compatible: false,
          },
        ],
      },
    ],
    options: pricingOptions({}),
    empty_version: 'empty',
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/option/model_pricing') {
      return { data: { success: true, data: snapshot } }
    }
    if (url === '/api/pricing') {
      return { data: { success: true, data: [], vendors: [] } }
    }
    return { data: { success: true, data: {} } }
  })
  let finishSave: (() => void) | undefined
  const save = vi.spyOn(api, 'patch').mockImplementation(
    () =>
      new Promise((resolve) => {
        finishSave = () => resolve({ data: { success: true } })
      })
  )
  const dirty = vi.fn()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ModelPricingPanel modelName='shared' onDirtyChange={dirty} />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  const defaultTab = await screen.findByRole('tab', { name: 'Default' })
  expect(defaultTab).toHaveAttribute('aria-selected', 'true')
  expect(
    await screen.findByText(
      'The model-level expression cannot be evaluated by: Beta'
    )
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Beta' }))
  const betaPanel = screen.getByRole('tabpanel', { name: 'Beta' })
  expect(
    within(betaPanel).getByText('Not configured for this provider')
  ).toBeVisible()
  const separate = within(betaPanel).getByRole('switch', {
    name: 'Set separately for this provider',
  })
  expect(separate).not.toBeChecked()
  await user.click(separate)
  expect(separate).toBeChecked()
  await user.click(separate)
  expect(separate).not.toBeChecked()
  expect(
    within(betaPanel).getByText('Use model-level expression')
  ).toBeVisible()
  expect(
    within(betaPanel).queryByRole('textbox', { name: 'credits' })
  ).not.toBeInTheDocument()
  await user.click(separate)
  const price = within(betaPanel).getByRole('textbox', { name: 'credits' })
  await user.clear(price)
  await user.type(price, '1.5')
  expect(dirty).toHaveBeenLastCalledWith(true)
  await user.click(defaultTab)
  expect(
    screen.queryByText(
      'The model-level expression cannot be evaluated by: Beta'
    )
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Beta' }))
  expect(price).toHaveValue('1.5')
  const saveButton = screen.getByRole('button', { name: 'Save model prices' })
  await user.click(saveButton)
  await waitFor(() => expect(save).toHaveBeenCalledOnce())
  expect(saveButton).toBeDisabled()
  const changes = save.mock.calls[0][1] as {
    changes: { expected_version: string; pricing: Record<string, unknown> }[]
  }
  expect(changes.changes[0].expected_version).toBe('version-with-providers')
  expect(changes.changes[0].pricing['billing_setting.billing_expr']).toBe(
    expression
  )
  expect(
    changes.changes[0].pricing['billing_setting.plugin_billing_expr']
  ).toEqual({ beta: 'tier("base", u("credits") * 1.5)' })
  await act(async () => {
    finishSave?.()
  })
  await waitFor(() => expect(saveButton).toBeEnabled())
})

it.each([
  { kind: 'unconfigured', expression: '' },
  {
    kind: 'previously configured',
    expression: 'tier( "legacy", u("count") * 0.25 )',
  },
])(
  'shows and saves $kind channel-routed Seedance pricing without provider variants',
  async ({ expression }) => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'administrator', role: 100 })
    usePricingPreferencesStore.setState({ currency: 'USD' })
    const values = expression
      ? {
          'billing_setting.billing_mode': 'tiered_expr',
          'billing_setting.billing_expr': expression,
        }
      : {}
    const snapshot: ModelPricingConfig = {
      entries: [
        {
          model_name: 'LLseadanco2.5',
          version: 'channel-routed-model',
          configured: values,
          effective: values,
          task_plugin_key: 'seedance-video',
          usage_schema: {
            count: { type: 'number', unit: 'count' },
            seconds: { type: 'number', unit: 'second' },
            video_resolution: { enum: ['480p', '720p', '1080p', '2k'] },
          },
        },
      ],
      options: pricingOptions({}),
      empty_version: 'empty',
    }
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/option/model_pricing') {
        return { data: { success: true, data: snapshot } }
      }
      return { data: { success: true, data: [], vendors: [] } }
    })
    const save = vi.spyOn(api, 'patch').mockResolvedValue({
      data: { success: true },
    })
    client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    render(
      <QueryClientProvider client={client}>
        <ModelPricingPanel modelName='LLseadanco2.5' />
      </QueryClientProvider>
    )

    const perCallTab = await screen.findByRole('tab', { name: 'Per-call' })
    expect(perCallTab).toHaveAttribute('aria-selected', 'true')
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
    for (const resolution of ['480p', '720p', '1080p', '2K']) {
      expect(
        screen.getByRole('textbox', {
          name: `${resolution} Price per second`,
        })
      ).toBeVisible()
    }
    await user.click(perCallTab)
    await user.click(screen.getByRole('button', { name: 'Save model prices' }))
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith('/api/option/model_pricing', {
        changes: [
          expect.objectContaining({
            model_name: 'LLseadanco2.5',
            expected_version: 'channel-routed-model',
            pricing: expect.objectContaining({
              'billing_setting.billing_mode': 'tiered_expr',
              'billing_setting.billing_expr':
                expression || 'tier("base", u("count") * 0)',
            }),
          }),
        ],
      })
    )
  }
)

it.each(['another-video', 'seedance-video-preview', undefined])(
  'keeps generic task pricing and its saved expression for plugin key %s',
  async (pluginKey) => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'administrator', role: 100 })
    usePricingPreferencesStore.setState({ currency: 'USD' })
    const expression =
      'tier( "legacy", u("count") * 0.25 + u("seconds") * 0.04 )'
    const values = {
      'billing_setting.billing_mode': 'tiered_expr',
      'billing_setting.billing_expr': expression,
    }
    const snapshot: ModelPricingConfig = {
      entries: [
        {
          model_name: 'seedance-looking-model-name',
          version: 'generic-model',
          configured: values,
          effective: values,
          task_plugin_key: pluginKey,
          usage_schema: seedanceVariant().usage_schema,
        },
      ],
      options: pricingOptions({}),
      empty_version: 'empty',
    }
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/option/model_pricing') {
        return { data: { success: true, data: snapshot } }
      }
      return { data: { success: true, data: [], vendors: [] } }
    })
    const save = vi.spyOn(api, 'patch').mockResolvedValue({
      data: { success: true },
    })
    client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    render(
      <QueryClientProvider client={client}>
        <ModelPricingPanel modelName='seedance-looking-model-name' />
      </QueryClientProvider>
    )

    expect(
      await screen.findByRole('combobox', { name: 'Editor mode' })
    ).toBeVisible()
    expect(
      screen.queryByRole('tab', { name: 'Seconds × resolution' })
    ).not.toBeInTheDocument()
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: 'Save model prices' }))
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith('/api/option/model_pricing', {
        changes: [
          expect.objectContaining({
            model_name: 'seedance-looking-model-name',
            expected_version: 'generic-model',
            pricing: expect.objectContaining(values),
          }),
        ],
      })
    )
  }
)

it('keeps long provider names accessible while allowing the tabs to wrap', () => {
  const longName =
    'Provider with a long international service name and a regional deployment designation'
  render(
    <TaskPluginPricingEditor
      variants={[
        {
          plugin_key: 'alpha',
          plugin_name: 'Alpha',
          usage_schema: {},
          configured: '',
          effective: '1',
          compatible: true,
        },
        {
          plugin_key: 'beta',
          plugin_name: longName,
          usage_schema: {},
          configured: '',
          effective: '1',
          compatible: true,
        },
      ]}
      expressions={{}}
      onChange={vi.fn()}
      modelExpression='1'
      currency={USD_PRICING_CURRENCY}
    >
      <span>Default editor</span>
    </TaskPluginPricingEditor>
  )
  expect(screen.getByRole('tablist', { name: 'Provider' })).toHaveClass(
    'flex-wrap'
  )
  expect(screen.getByRole('tab', { name: longName })).toHaveClass(
    'max-w-full',
    'min-w-0'
  )
  expect(screen.getByText(longName)).toHaveAttribute('title', longName)
})

it('lets an administrator remove an unavailable provider’s saved price', async () => {
  const expression = 'u("images")'
  const { ref } = renderEditor(
    {
      name: 'shared',
      billingMode: 'tiered_expr',
      billingExpr: expression,
      pluginBillingExpr: { retired: expression },
    },
    [
      {
        plugin_key: 'retired',
        plugin_name: 'Retired',
        stale: true,
        usage_schema: {},
        configured: expression,
        effective: '',
        compatible: false,
      },
    ]
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Retired' }))
  expect(
    screen.getByText('This provider is unavailable for this model.')
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Remove saved price' }))
  expect(await ref.current?.commitDraft()).toMatchObject({
    pluginBillingExpr: {},
    billingExpr: expression,
  })
  expect(screen.getByRole('tab', { name: 'Default' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
})

it('shows pending server validation for an expression the browser cannot check', async () => {
  render(
    <TaskPluginPricingEditor
      variants={[
        {
          plugin_key: 'alpha',
          plugin_name: 'Alpha',
          usage_schema: {},
          configured: '',
          effective: '1',
          compatible: true,
        },
      ]}
      expressions={{}}
      onChange={vi.fn()}
      modelExpression='futureFunction()'
      currency={USD_PRICING_CURRENCY}
    >
      <span>Default editor</span>
    </TaskPluginPricingEditor>
  )
  await userEvent.setup().click(screen.getByRole('tab', { name: 'Alpha' }))
  expect(
    screen.getByText('Expression compatibility will be checked when saving.')
  ).toBeVisible()
})

function renderEditor(
  editData: ModelRatioData,
  variants: ModelPricingPluginVariant[],
  dirty = vi.fn(),
  taskPluginKey?: string
) {
  usePricingPreferencesStore.setState({ currency: 'USD' })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [], vendors: [] },
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { effective: {} } },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const ref = createRef<ModelPricingEditorPanelHandle>()
  render(
    <QueryClientProvider client={client}>
      <ModelPricingEditorPanel
        ref={ref}
        embedded
        editData={editData}
        pluginVariants={variants}
        taskPluginKey={taskPluginKey}
        onDirtyChange={dirty}
      />
    </QueryClientProvider>
  )
  return { ref, dirty }
}

it('shows and removes a single provider override while retaining the per-call default', async () => {
  const expression = 'tier("base", u("seconds") * 0.4)'
  const { ref } = renderEditor(
    {
      name: 'shared',
      billingMode: 'per-request',
      price: '0.25',
      pluginBillingExpr: { alpha: expression },
    },
    [
      {
        plugin_key: 'alpha',
        plugin_name: 'Alpha',
        usage_schema: { seconds: { type: 'number', unit: 'second' } },
        configured: expression,
        effective: expression,
        compatible: true,
      },
    ]
  )
  expect(
    screen.getByRole('tab', { name: 'Per-request (deprecated)' })
  ).toHaveAttribute('aria-selected', 'true')
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Alpha' }))
  await user.click(
    screen.getByRole('switch', { name: 'Set separately for this provider' })
  )
  expect(screen.getByText('Use model-level pricing')).toBeVisible()
  expect(await ref.current?.commitDraft()).toMatchObject({
    billingMode: 'per-request',
    price: '0.25',
    pluginBillingExpr: {},
  })
})

it('clears dirty state after restoring an override in a different key order', async () => {
  const expression = 'tier("base", u("seconds") * 0.4)'
  const { dirty } = renderEditor(
    {
      name: 'shared',
      billingMode: 'tiered_expr',
      billingExpr: expression,
      pluginBillingExpr: { alpha: expression, beta: expression },
    },
    ['alpha', 'beta'].map((key) => ({
      plugin_key: key,
      plugin_name: key,
      usage_schema: { seconds: { type: 'number', unit: 'second' } },
      configured: expression,
      effective: expression,
      compatible: true,
    }))
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'alpha' }))
  const toggle = screen.getByRole('switch', {
    name: 'Set separately for this provider',
  })
  await user.click(toggle)
  expect(dirty).toHaveBeenLastCalledWith(true)
  await user.click(toggle)
  expect(dirty).toHaveBeenLastCalledWith(false)
})

it('opens task expression pricing when only provider metadata supplies a schema', async () => {
  renderEditor(
    { name: 'shared', billingMode: 'per-token' },
    ['alpha', 'beta'].map((key) => ({
      plugin_key: key,
      plugin_name: key,
      usage_schema: { seconds: { type: 'number', unit: 'second' } },
      configured: '',
      effective: '',
      compatible: false,
    }))
  )
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Expression' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  )
  expect(screen.getByRole('textbox', { name: 'seconds' })).toBeVisible()
})

/** Seedance 渠道的定价元数据，模型名称可以是渠道配置的任意精确 ID。 */
function seedanceVariant(configured = ''): ModelPricingPluginVariant {
  return {
    plugin_key: 'seedance-video',
    plugin_name: 'Seedance Video',
    usage_schema: {
      count: { type: 'number', unit: 'count' },
      seconds: { type: 'number', unit: 'second' },
      video_resolution: {
        enum: ['480p', '720p', '1080p', '2k'],
      },
    },
    configured,
    effective: configured,
    compatible: true,
  }
}

/** NSKeven/V6K 等插件视频渠道的定价元数据，清晰度档位由插件 schema 决定。 */
function unifiedVideoVariant(
  pluginKey: string,
  pluginName: string,
  configured = ''
): ModelPricingPluginVariant {
  return {
    plugin_key: pluginKey,
    plugin_name: pluginName,
    usage_schema: {
      count: { type: 'number', unit: 'count' },
      seconds: { type: 'number', unit: 'second' },
      video_resolution: {
        enum: ['480p', '720p', '1080p', '4k'],
      },
    },
    configured,
    effective: configured,
    compatible: true,
  }
}

it('keeps Seedance call and resolution prices separate until the draft is committed', async () => {
  const original = 'tier( "original", u("count") * 0.25 )'
  const rules = 'header("x-priority") == "high" ? 2 : 1'
  const { ref, dirty } = renderEditor(
    {
      name: 'arbitrary-channel-video-id',
      billingMode: 'tiered_expr',
      billingExpr: original,
      requestRuleExpr: rules,
    },
    [seedanceVariant()]
  )
  const save = vi.spyOn(api, 'patch')
  const user = userEvent.setup()
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Per-call' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  )
  expect(dirty).toHaveBeenLastCalledWith(false)
  expect(await ref.current?.commitDraft()).toMatchObject({
    billingExpr: original,
    requestRuleExpr: rules,
  })

  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  const hdPrice = screen.getByRole('textbox', {
    name: '1080p Price per second',
  })
  await user.clear(hdPrice)
  await user.type(hdPrice, '0.08')
  const draft = await ref.current?.commitDraft()
  expect(draft?.billingExpr).toBe(
    'u("video_resolution") == "480p" ? tier("480p", u("seconds") * 0)\n' +
      '  : u("video_resolution") == "720p" ? tier("720p", u("seconds") * 0)\n' +
      '  : u("video_resolution") == "1080p" ? tier("1080p", u("seconds") * 0.08)\n' +
      '  : tier("2k", u("seconds") * 0)'
  )
  expect(draft?.requestRuleExpr).toBe(rules)
  expect(draft?.billingExpr).not.toContain('u("count")')

  await user.click(screen.getByRole('tab', { name: 'Per-call' }))
  expect(screen.getByRole('textbox', { name: 'Price per call' })).toHaveValue(
    '0.25'
  )
  expect((await ref.current?.commitDraft())?.billingExpr).toBe(original)
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  expect(
    screen.getByRole('textbox', { name: '1080p Price per second' })
  ).toHaveValue('0.08')
  await user.click(screen.getByRole('tab', { name: 'Expression editor' }))
  await user.click(screen.getByRole('tab', { name: 'Per-call' }))
  expect((await ref.current?.commitDraft())?.billingExpr).toBe(original)
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  expect(
    screen.getByRole('textbox', { name: '1080p Price per second' })
  ).toHaveValue('0.08')
  expect(save).not.toHaveBeenCalled()
})

it.each([
  {
    kind: 'per-call without a tier',
    expression: 'u("count") * 0.35',
    mode: 'Per-call',
    priceLabel: 'Price per call',
    price: '0.35',
  },
  {
    kind: 'the saved token sentinel',
    expression: 'tier("base", p * 0 + c * 0)',
    mode: 'Expression editor',
    priceLabel: 'Billing expression',
    price: 'tier("base", p * 0 + c * 0)',
  },
  {
    kind: 'equal resolution prices',
    expression:
      'u("video_resolution") == "480p" ? tier("480p", u("seconds") * 0.1)\n' +
      '  : u("video_resolution") == "720p" ? tier("720p", u("seconds") * 0.1)\n' +
      '  : u("video_resolution") == "1080p" ? tier("1080p", u("seconds") * 0.1)\n' +
      '  : tier("2k", u("seconds") * 0.1)',
    mode: 'Seconds × resolution',
    priceLabel: '2K Price per second',
    price: '0.1',
  },
])('opens Seedance $kind without rewriting saved pricing', async (saved) => {
  const { ref, dirty } = renderEditor(
    {
      name: 'saved-channel-model',
      billingMode: 'tiered_expr',
      billingExpr: saved.expression,
    },
    [seedanceVariant()]
  )
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: saved.mode })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  )
  expect(screen.getByRole('textbox', { name: saved.priceLabel })).toHaveValue(
    saved.price
  )
  expect((await ref.current?.commitDraft())?.billingExpr).toBe(saved.expression)
  expect(dirty).toHaveBeenLastCalledWith(false)
})

it('starts an unconfigured sole Seedance provider with exclusive per-call pricing', async () => {
  const { ref } = renderEditor(
    { name: 'channel-configured-model', billingMode: 'tiered_expr' },
    [seedanceVariant()]
  )
  await waitFor(() =>
    expect(
      screen.getByRole('textbox', { name: 'Price per call' })
    ).toBeVisible()
  )
  expect((await ref.current?.commitDraft())?.billingExpr).toBe(
    'tier("base", u("count") * 0)'
  )
})

it('preserves custom Seedance pricing and rules until replacement is confirmed', async () => {
  const custom = 'tier("custom", u("count") * 0.2 + u("seconds") * 0.04)'
  const rules = 'param("watermark") == true ? 1.5 : 1'
  const { ref, dirty } = renderEditor(
    {
      name: 'custom-channel-model',
      billingMode: 'tiered_expr',
      billingExpr: custom,
      requestRuleExpr: rules,
    },
    [seedanceVariant()]
  )
  const user = userEvent.setup()
  await waitFor(() =>
    expect(
      screen.getByRole('tab', { name: 'Expression editor' })
    ).toHaveAttribute('aria-selected', 'true')
  )
  expect(dirty).toHaveBeenLastCalledWith(false)
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  const dialog = screen.getByRole('alertdialog', {
    name: 'Switch billing method?',
  })
  expect(await ref.current?.commitDraft()).toMatchObject({
    billingExpr: custom,
    requestRuleExpr: rules,
  })
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  expect(
    screen.getByRole('tab', { name: 'Expression editor' })
  ).toHaveAttribute('aria-selected', 'true')
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  await user.click(screen.getByRole('button', { name: 'Apply billing method' }))
  const draft = await ref.current?.commitDraft()
  expect(draft?.requestRuleExpr).toBe(rules)
  expect(draft?.billingExpr?.match(/u\("seconds"\) \* 0/g)).toHaveLength(4)
  await user.click(screen.getByRole('tab', { name: 'Expression editor' }))
  expect(await ref.current?.commitDraft()).toMatchObject({
    billingExpr: custom,
    requestRuleExpr: rules,
  })
})

it('uses the Seedance editor only for its provider key on a shared model', async () => {
  const generic = 'tier("base", u("seconds") * 0.4)'
  const { ref } = renderEditor(
    {
      name: 'seedance-looking-model-name',
      billingMode: 'tiered_expr',
      billingExpr: generic,
      pluginBillingExpr: { 'seedance-video': 'tier("base", u("count") * 0.3)' },
    },
    [
      {
        plugin_key: 'another-video',
        plugin_name: 'Another Video',
        usage_schema: { seconds: { type: 'number', unit: 'second' } },
        configured: '',
        effective: generic,
        compatible: true,
      },
      seedanceVariant('tier("base", u("count") * 0.3)'),
    ]
  )
  const user = userEvent.setup()
  expect(screen.queryByRole('tab', { name: 'Seconds × resolution' })).toBeNull()
  await user.click(screen.getByRole('tab', { name: 'Seedance Video' }))
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  const input = screen.getByRole('textbox', { name: '2K Price per second' })
  await user.clear(input)
  await user.type(input, '0.12')
  await user.click(screen.getByRole('tab', { name: 'Default' }))
  await user.click(screen.getByRole('tab', { name: 'Seedance Video' }))
  expect(input).toHaveValue('0.12')
  const draft = await ref.current?.commitDraft()
  expect(draft?.billingExpr).toBe(generic)
  expect(draft?.pluginBillingExpr?.['seedance-video']).toContain(
    'tier("2k", u("seconds") * 0.12)'
  )
  expect(draft?.pluginBillingExpr?.['seedance-video']).not.toContain(
    'u("count")'
  )
})

it('derives NSKeven resolution tiers from the plugin usage schema', async () => {
  const { ref } = renderEditor(
    { name: 'nskeven-any-video-model', billingMode: 'tiered_expr' },
    [unifiedVideoVariant('nskeven-video', 'NSKeven 视频')]
  )
  const user = userEvent.setup()
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Per-call' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  )
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  expect(
    screen.getByRole('textbox', { name: '4K Price per second' })
  ).toBeVisible()
  expect(
    screen.queryByRole('textbox', { name: '2K Price per second' })
  ).toBeNull()
  const price = screen.getByRole('textbox', {
    name: '1080p Price per second',
  })
  await user.clear(price)
  await user.type(price, '0.06')
  expect((await ref.current?.commitDraft())?.billingExpr).toBe(
    'u("video_resolution") == "480p" ? tier("480p", u("seconds") * 0)\n' +
      '  : u("video_resolution") == "720p" ? tier("720p", u("seconds") * 0)\n' +
      '  : u("video_resolution") == "1080p" ? tier("1080p", u("seconds") * 0.06)\n' +
      '  : tier("4k", u("seconds") * 0)'
  )
})

it('lets a channel-mapped model use the unified video tabs for NSKeven', async () => {
  const generic = 'tier("base", u("seconds") * 0.4)'
  const { ref } = renderEditor(
    {
      name: 'shared-nskeven-model',
      billingMode: 'tiered_expr',
    },
    [
      unifiedVideoVariant('nskeven-video', 'NSKeven 视频'),
      {
        plugin_key: 'another-video',
        plugin_name: 'Another Video',
        usage_schema: {
          seconds: { type: 'number', unit: 'second' },
        },
        configured: '',
        effective: generic,
        compatible: true,
      },
    ],
    vi.fn(),
    'nskeven-video'
  )
  await waitFor(() =>
    expect(
      screen.getByRole('textbox', { name: 'Price per call' })
    ).toBeVisible()
  )
  expect(await ref.current?.commitDraft()).toMatchObject({
    billingExpr: 'tier("base", u("count") * 0)',
  })
})

it('offers the unified video tabs on a V6K provider override', async () => {
  const generic = 'tier("base", u("seconds") * 0.4)'
  const { ref } = renderEditor(
    {
      name: 'shared-v6k-model',
      billingMode: 'tiered_expr',
      billingExpr: generic,
    },
    [
      unifiedVideoVariant('v6k-video', 'V6K 视频'),
      {
        plugin_key: 'another-video',
        plugin_name: 'Another Video',
        usage_schema: {
          seconds: { type: 'number', unit: 'second' },
        },
        configured: '',
        effective: generic,
        compatible: true,
      },
    ]
  )
  const user = userEvent.setup()
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'V6K 视频' })).toBeVisible()
  )
  await user.click(screen.getByRole('tab', { name: 'V6K 视频' }))
  await user.click(
    screen.getByRole('switch', { name: 'Set separately for this provider' })
  )
  await user.click(screen.getByRole('tab', { name: 'Seconds × resolution' }))
  const dialog = screen.getByRole('alertdialog', {
    name: 'Switch billing method?',
  })
  await user.click(
    within(dialog).getByRole('button', { name: 'Apply billing method' })
  )
  const price = screen.getByRole('textbox', { name: '720p Price per second' })
  await user.clear(price)
  await user.type(price, '0.05')
  const draft = await ref.current?.commitDraft()
  expect(draft?.pluginBillingExpr?.['v6k-video']).toContain(
    'u("video_resolution") == "720p" ? tier("720p", u("seconds") * 0.05)'
  )
  expect(draft?.pluginBillingExpr?.['v6k-video']).toContain(
    'tier("4k", u("seconds") * 0)'
  )
  expect(draft?.pluginBillingExpr?.['v6k-video']).not.toContain('u("count")')
})
