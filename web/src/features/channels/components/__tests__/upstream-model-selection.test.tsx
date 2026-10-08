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
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { useEffect, useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { fetchModels } from '../../api'
import { mergeDiscoveredModelAliases } from '../../lib/model-mapping-validation'
import {
  channelSchema,
  type Channel,
  type FetchModelsResponse,
} from '../../types'
import { ChannelsProvider, useChannels } from '../channels-provider'
import { FetchModelsDialog } from '../dialogs/fetch-models-dialog'
import { UpstreamModelSelection } from '../upstream-model-selection'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

test.each([
  { mapping: '{', error: 'Model mapping must be valid JSON format' },
  { mapping: '[]', error: 'Model mapping must be a valid JSON object' },
  { mapping: '{"manual":1}', error: 'Model mapping values must be strings' },
])(
  'invalid existing mapping $mapping rejects alias merging without replacing the draft',
  ({ mapping, error }) => {
    expect(() =>
      mergeDiscoveredModelAliases(['Yuan-New'], mapping, {
        'Yuan-New': 'exact-upstream',
      })
    ).toThrow(error)
  }
)

test('alias merging treats __proto__ as an existing model key and preserves its administrator target', () => {
  const merged = mergeDiscoveredModelAliases(
    ['__proto__', 'Yuan-New'],
    '{"__proto__":"administrator-upstream","manual":"manual-upstream"}',
    JSON.parse('{"__proto__":"provider-upstream","Yuan-New":"exact-upstream"}')
  )
  expect(merged.models).toEqual(['__proto__', 'Yuan-New'])
  const mapping = JSON.parse(merged.modelMapping)
  expect(Object.hasOwn(mapping, '__proto__')).toBe(true)
  expect(mapping.__proto__).toBe('administrator-upstream')
  expect(mapping.manual).toBe('manual-upstream')
  expect(mapping['Yuan-New']).toBe('exact-upstream')
})

test('discovery keeps a selected upstream ID when it is already the source of an administrator mapping', () => {
  const existing = '{"exact-upstream":"administrator-upstream"}'
  const merged = mergeDiscoveredModelAliases(['exact-upstream'], existing, {
    'Yuan-New': 'exact-upstream',
  })
  expect(merged.models).toEqual(['exact-upstream'])
  expect(merged.modelMapping).toBe(existing)
})

test.each(['local', 'provider'] as const)(
  '%s mapping errors use their intended display language without changing the form',
  async (origin) => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'zh',
      fallbackLng: 'en',
      resources: { en, zh },
      keySeparator: false,
      interpolation: { escapeValue: false },
    })
    const client = new QueryClient()
    const select = vi.fn()
    const message = 'Model mapping must be valid JSON format'
    const response: FetchModelsResponse =
      origin === 'local'
        ? {
            success: true,
            source: 'upstream',
            data: ['Yuan-New'],
            model_mapping: { 'Yuan-New': 'exact-upstream' },
          }
        : { success: false, message }
    const fetcher = vi
      .fn<() => Promise<FetchModelsResponse>>()
      .mockResolvedValue(response)
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <ChannelsProvider>
            <FetchModelsDialog
              open
              onOpenChange={vi.fn()}
              onModelsSelected={select}
              existingModelsOverride={['manual-alias']}
              existingModelMappingOverride='{'
              customFetcher={fetcher}
            />
          </ChannelsProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )
    const expected = origin === 'local' ? zh.translation[message] : message
    expect(await screen.findByText(expected)).toBeVisible()
    expect(
      screen.queryByRole('button', { name: zh.translation['Save Models'] })
    ).not.toBeInTheDocument()
    expect(select).not.toHaveBeenCalled()
    client.clear()
  }
)

test('local mapping errors are translated when a form mapping becomes invalid before saving', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'zh',
    fallbackLng: 'en',
    resources: { en, zh },
    keySeparator: false,
    interpolation: { escapeValue: false },
  })
  const client = new QueryClient()
  const select = vi.fn()
  const close = vi.fn()
  const showError = vi.spyOn(toast, 'error').mockReturnValue('mapping-error')
  const fetcher = vi
    .fn<() => Promise<FetchModelsResponse>>()
    .mockResolvedValue({
      success: true,
      source: 'upstream',
      data: ['Yuan-New'],
      model_mapping: { 'Yuan-New': 'exact-upstream' },
    })
  const user = userEvent.setup()
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <ChannelsProvider>
          <FetchModelsDialog
            open
            onOpenChange={close}
            onModelsSelected={select}
            existingModelsOverride={['Yuan-New']}
            existingModelMappingOverride='{}'
            customFetcher={fetcher}
          />
        </ChannelsProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(
    await screen.findByRole('checkbox', { name: 'Yuan-New' })
  ).toBeChecked()
  view.rerender(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <ChannelsProvider>
          <FetchModelsDialog
            open
            onOpenChange={close}
            onModelsSelected={select}
            existingModelsOverride={['Yuan-New']}
            existingModelMappingOverride='{'
            customFetcher={fetcher}
          />
        </ChannelsProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
  await user.click(
    screen.getByRole('button', { name: zh.translation['Save Models'] })
  )
  expect(showError).toHaveBeenCalledWith(
    zh.translation['Model mapping must be valid JSON format']
  )
  expect(select).not.toHaveBeenCalled()
  expect(close).not.toHaveBeenCalled()
  client.clear()
})

test('plugin catalogs are identified and merged into the existing selection before saving', async () => {
  const client = new QueryClient()
  const select = vi.fn()
  const fetcher = vi
    .fn<() => Promise<FetchModelsResponse>>()
    .mockResolvedValue({
      success: true,
      source: 'plugin',
      data: ['minimax-h3'],
      unsupported_models: [],
    })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <FetchModelsDialog
          open
          onOpenChange={vi.fn()}
          onModelsSelected={select}
          existingModelsOverride={['manual-alias']}
          customFetcher={fetcher}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  expect(await screen.findByText('Source: Plugin model list')).toBeVisible()
  expect(
    screen.getByRole('dialog', { name: 'Load Plugin Models' })
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Save Models' }))
  expect(select).toHaveBeenCalledWith(['manual-alias', 'minimax-h3'])
  client.clear()
})

test('a failed plugin fetch cannot save an empty selection and retry only offers supported models', async () => {
  const client = new QueryClient()
  const select = vi.fn()
  const fetcher = vi
    .fn<() => Promise<FetchModelsResponse>>()
    .mockResolvedValueOnce({ success: false, message: 'Catalog access denied' })
    .mockResolvedValueOnce({
      success: true,
      source: 'upstream',
      data: ['wan3.0-video'],
      unsupported_models: ['seedance-future'],
    })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <FetchModelsDialog
          open
          onOpenChange={vi.fn()}
          onModelsSelected={select}
          existingModelsOverride={['manual-alias']}
          customFetcher={fetcher}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  expect(await screen.findByText('Catalog access denied')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Save Models' })
  ).not.toBeInTheDocument()
  expect(select).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Source: Upstream catalog')).toBeVisible()
  expect(screen.getByText('seedance-future')).toBeVisible()
  expect(
    screen.queryByRole('checkbox', { name: 'seedance-future' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('checkbox', { name: 'wan3.0-video' }))
  await user.click(screen.getByRole('button', { name: 'Save Models' }))
  expect(select).toHaveBeenCalledWith(['manual-alias', 'wan3.0-video'])
  client.clear()
})

test('filling a form from alias discovery returns selected public models and keeps existing mappings', async () => {
  const client = new QueryClient()
  const select = vi.fn()
  const mappings = {
    'Yuan-Seedance-2.5-Official': 'seedance-2.5-guanfang-anmiao',
    'Yuan-Seedance-2.0-LJ': 'yl_g7zy_seedance_v2_0_std',
  }
  const fetcher = vi
    .fn<() => Promise<FetchModelsResponse>>()
    .mockResolvedValue({
      success: true,
      source: 'upstream',
      data: Object.keys(mappings),
      model_mapping: mappings,
    })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <FetchModelsDialog
          open
          onOpenChange={vi.fn()}
          onModelsSelected={select}
          existingModelsOverride={[
            'manual-alias',
            'seedance-2.5-guanfang-anmiao',
          ]}
          existingModelMappingOverride='{"manual-alias":"manual-upstream"}'
          customFetcher={fetcher}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  expect(
    await screen.findByRole('checkbox', {
      name: 'Yuan-Seedance-2.5-Official',
    })
  ).toBeChecked()
  await user.click(
    screen.getByRole('checkbox', { name: 'Yuan-Seedance-2.0-LJ' })
  )
  await user.click(screen.getByRole('button', { name: 'Save Models' }))
  expect(select).toHaveBeenCalledWith(
    ['manual-alias', ...Object.keys(mappings)],
    expect.any(String)
  )
  expect(JSON.parse(select.mock.calls[0][1])).toEqual({
    'manual-alias': 'manual-upstream',
    ...mappings,
  })
  client.clear()
})

/** 通过渠道列表的已保存入口打开模型获取，使用真实 API 保存路径。 */
function SavedChannelModelDiscovery(props: { channel: Channel }) {
  const { currentRow, setCurrentRow } = useChannels()
  useEffect(() => {
    setCurrentRow(props.channel)
  }, [props.channel, setCurrentRow])
  return <FetchModelsDialog open={Boolean(currentRow)} onOpenChange={vi.fn()} />
}

test('saved channel discovery updates models and selected alias mappings in one request without replacing administrator targets', async () => {
  const client = new QueryClient()
  const channel = channelSchema.parse({
    id: 42,
    name: 'Yuanliu channel',
    type: 61,
    key: '',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'Yuan-Seedance-2.5-Official,manual-alias',
    model_mapping:
      '{"Yuan-Seedance-2.5-Official":"administrator-upstream","manual-alias":"manual-upstream"}',
    group: 'default',
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      source: 'upstream',
      data: ['Yuan-Seedance-2.5-Official', 'Yuan-Seedance-2.0-LJ'],
      model_mapping: {
        'Yuan-Seedance-2.5-Official': 'seedance-2.5-guanfang-anmiao',
        'Yuan-Seedance-2.0-LJ': 'yl_g7zy_seedance_v2_0_std',
      },
      unsupported_models: ['unadapted-upstream-model'],
    },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <SavedChannelModelDiscovery channel={channel} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  const model = await screen.findByRole('checkbox', {
    name: 'Yuan-Seedance-2.0-LJ',
  })
  expect(model).not.toBeChecked()
  expect(
    screen.queryByRole('checkbox', { name: 'unadapted-upstream-model' })
  ).not.toBeInTheDocument()
  await user.click(model)
  await user.click(screen.getByRole('button', { name: 'Save Models' }))
  expect(put).toHaveBeenCalledTimes(1)
  expect(put).toHaveBeenCalledWith(
    '/api/channel/',
    expect.objectContaining({
      id: 42,
      models: 'Yuan-Seedance-2.5-Official,manual-alias,Yuan-Seedance-2.0-LJ',
    }),
    expect.anything()
  )
  const saved = put.mock.calls[0]?.[1] as { model_mapping: string }
  expect(JSON.parse(saved.model_mapping)).toEqual({
    'Yuan-Seedance-2.5-Official': 'administrator-upstream',
    'manual-alias': 'manual-upstream',
    'Yuan-Seedance-2.0-LJ': 'yl_g7zy_seedance_v2_0_std',
  })
  client.clear()
})

test.each([
  {
    result: 'nonempty',
    models: ['gpt-existing', 'gpt-new'],
    removedCount: 1,
    savedModels: ['gpt-existing', 'alias', 'gpt-new'],
  },
  {
    result: 'empty',
    models: [],
    removedCount: 2,
    savedModels: ['alias'],
  },
])(
  'removed models stay available after batch deselection with a $result upstream list and only checked models are saved',
  async ({ models, removedCount, savedModels }) => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: models },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const select = vi.fn()
    const close = vi.fn()
    const user = userEvent.setup()
    const view = render(
      <QueryClientProvider client={client}>
        <ChannelsProvider>
          <FetchModelsDialog
            open
            onOpenChange={close}
            onModelsSelected={select}
            existingModelsOverride={['gpt-existing', 'alias', 'manual-model']}
            redirectSourceModels={['alias']}
            customFetcher={async () =>
              (
                await fetchModels({
                  type: 1,
                  base_url: 'https://example.com',
                  key: 'test-key',
                })
              ).data ?? []
            }
          />
        </ChannelsProvider>
      </QueryClientProvider>
    )
    const removedTab = await screen.findByRole('tab', {
      name: `Removed Models (${removedCount})`,
    })
    if (models.length > 0) {
      await user.click(screen.getByRole('checkbox', { name: 'gpt-new' }))
    }
    await user.click(removedTab)
    expect(
      screen.queryByRole('checkbox', { name: 'alias' })
    ).not.toBeInTheDocument()
    await user.click(
      screen.getByRole('checkbox', { name: 'Select all models in Removed' })
    )
    expect(removedTab).toHaveAttribute('aria-selected', 'true')
    const manualModel = screen.getByRole('checkbox', { name: 'manual-model' })
    expect(manualModel).not.toBeChecked()
    await user.click(manualModel)
    expect(manualModel).toBeChecked()
    await user.click(manualModel)
    expect(manualModel).not.toBeChecked()
    expect(
      screen.getByRole('tab', { name: `Removed Models (${removedCount})` })
    ).toHaveAttribute('aria-selected', 'true')
    expect(select).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Save Models' }))
    expect(select).toHaveBeenCalledWith(savedModels)
    expect(close).toHaveBeenCalledWith(false)
    view.unmount()
    client.clear()
  }
)

function InlineSelection() {
  const [selected, setSelected] = useState(['manual-model'])
  return (
    <>
      <UpstreamModelSelection
        models={['gpt-one', 'gpt-two', 'another-model']}
        selected={selected}
        existingModels={[]}
        showChanges={false}
        onChange={setSelected}
      />
      <output aria-label='Selected models'>{selected.join(',')}</output>
    </>
  )
}

test('category headers keep a transparent background while expanding and collapsing without changing selection', async () => {
  const user = userEvent.setup()
  render(<InlineSelection />)
  const header = screen.getByRole('button', { name: /^OpenAI \(2\)/ })

  expect(header).toHaveAttribute('aria-expanded', 'true')
  expect(header).toHaveClass(
    'aria-expanded:bg-transparent',
    'hover:bg-transparent',
    'dark:hover:bg-transparent'
  )
  await user.click(header)
  expect(header).toHaveAttribute('aria-expanded', 'false')
  expect(
    screen.queryByRole('checkbox', { name: 'gpt-one' })
  ).not.toBeInTheDocument()
  await user.keyboard('{Enter}')
  expect(header).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByRole('checkbox', { name: 'gpt-one' })).toBeVisible()
  expect(header).toHaveFocus()
  expect(screen.getByLabelText('Selected models')).toHaveTextContent(
    'manual-model'
  )
})

test('the matching-model action only appears for a nonblank search and disappears when cleared', async () => {
  const user = userEvent.setup()
  render(<InlineSelection />)
  const search = screen.getByRole('textbox', { name: 'Search models...' })
  expect(
    screen.queryByRole('button', { name: 'Select all matching models' })
  ).not.toBeInTheDocument()

  await user.type(search, '  ')
  expect(
    screen.queryByRole('button', { name: 'Select all matching models' })
  ).not.toBeInTheDocument()

  await user.type(search, 'gpt-')
  expect(
    screen.getByRole('button', { name: 'Select all matching models' })
  ).toBeEnabled()
  await user.clear(search)
  expect(
    screen.queryByRole('button', { name: 'Select all matching models' })
  ).not.toBeInTheDocument()

  await user.type(search, 'missing')
  expect(
    screen.getByRole('button', { name: 'Select all matching models' })
  ).toBeDisabled()
})

test('selecting search results adds only matching models and retains the manual selection', async () => {
  const user = userEvent.setup()
  render(<InlineSelection />)
  await user.type(
    screen.getByRole('textbox', { name: 'Search models...' }),
    'gpt-'
  )
  await user.click(
    screen.getByRole('button', { name: 'Select all matching models' })
  )
  expect(screen.getByLabelText('Selected models')).toHaveTextContent(
    'manual-model,gpt-one,gpt-two'
  )
  expect(
    screen.queryByRole('checkbox', { name: 'another-model' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('checkbox', { name: 'gpt-one' }))
  expect(screen.getByLabelText('Selected models')).toHaveTextContent(
    'manual-model,gpt-two'
  )
})
