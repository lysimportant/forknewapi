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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useMemo, useState, type ComponentProps } from 'react'
import {
  afterEach,
  assert,
  beforeEach,
  describe,
  expect,
  test,
  vi,
  type MockInstance,
} from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { RatioSettingsCard } from '../ratio-settings-card'
import { parseGroupOpenAIProtocolBridge } from '../utils'

/** 协议设置与计费配置共存的最小分组样本。 */
const groupDefaults = {
  GroupRatio: '{"openclaw":1.5,"special":2}',
  TopupGroupRatio: '{"openclaw":0.8,"special":0.9}',
  UserUsableGroups: '{"openclaw":"OpenClaw users","special":"Special users"}',
  GroupGroupRatio: '{"special":{"openclaw":1.2}}',
  AutoGroups: '["openclaw","special"]',
  MaxTokenAutoGroups: 5,
  DefaultUseAutoGroup: false,
  GroupSpecialUsableGroup: '{"special":{"openclaw":"Allowed"}}',
}

/** 仅展示分组页，模型价格不参与本次读写。 */
const modelDefaults: ComponentProps<typeof RatioSettingsCard>['modelDefaults'] =
  {
    ModelPrice: '{}',
    ModelRatio: '{}',
    CacheRatio: '{}',
    CreateCacheRatio: '{}',
    CompletionRatio: '{}',
    ImageRatio: '{}',
    AudioRatio: '{}',
    AudioCompletionRatio: '{}',
    ExposeRatioEnabled: false,
    BillingMode: '{}',
    BillingExpr: '{}',
    PluginBillingExpr: '{}',
  }

let client: QueryClient
let put: MockInstance<typeof api.put>

/** 使用真实表单、选择器和保存钩子，只有 HTTP 边界被替换。 */
function GroupSettingsFixture(props: { protocols?: string }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  const defaults = useMemo(
    () => ({
      ...groupDefaults,
      GroupOpenAIProtocolBridge: props.protocols ?? '{}',
    }),
    [props.protocols]
  )
  return (
    <QueryClientProvider client={client}>
      <div ref={setActions} />
      <SettingsPageProvider actionsContainer={actions}>
        <RatioSettingsCard
          modelDefaults={modelDefaults}
          groupDefaults={defaults}
          toolPricesDefault='{}'
          visibleTabs={['groups']}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}

/** 通过可见组名取得所在行，避免依赖内部行标识。 */
function groupRow(name: string) {
  const row = screen.getByDisplayValue(name).closest('tr')
  assert(row)
  return within(row)
}

/** 读取经过真实保存入口提交的配置，不复制生产序列化逻辑。 */
function savedOptions(): Record<string, string> {
  return Object.fromEntries(
    put.mock.calls.map((call) => {
      const value = call[1] as { key: string; value: string }
      expect(call[0]).toBe('/api/option/')
      return [value.key, value.value]
    })
  )
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

afterEach(() => {
  client.clear()
  vi.restoreAllMocks()
})

describe('group upstream text protocol', () => {
  test('selects an upstream, saves only the protocol option, and reads it back without changing other groups or pricing', async () => {
    const user = userEvent.setup()
    const view = render(
      <GroupSettingsFixture protocols='{"special":"responses","archived":"chat"}' />
    )
    const openclaw = groupRow('openclaw')
    const choice = openclaw.getByRole('combobox', {
      name: 'Upstream text protocol',
    })
    expect(choice).toHaveValue('Keep existing settings')
    choice.focus()
    await user.keyboard('{ArrowDown}')
    await user.click(
      await screen.findByRole('option', {
        name: 'Use Chat Completions upstream',
      })
    )
    expect(choice).toHaveValue('Use Chat Completions upstream')
    expect(
      groupRow('special').getByRole('combobox', {
        name: 'Upstream text protocol',
      })
    ).toHaveValue('Use Responses upstream')
    expect(openclaw.getByRole('spinbutton', { name: 'Ratio' })).toHaveValue(1.5)
    expect(
      openclaw.getByRole('spinbutton', { name: 'Top-up ratio' })
    ).toHaveValue(0.8)
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    const saved = savedOptions()
    expect(Object.keys(saved)).toEqual(['global.group_openai_protocol_bridge'])
    expect(JSON.parse(saved['global.group_openai_protocol_bridge'])).toEqual({
      special: 'responses',
      archived: 'chat',
      openclaw: 'chat',
    })
    view.rerender(
      <GroupSettingsFixture
        protocols={saved['global.group_openai_protocol_bridge']}
      />
    )
    expect(
      groupRow('openclaw').getByRole('combobox', {
        name: 'Upstream text protocol',
      })
    ).toHaveValue('Use Chat Completions upstream')
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))
    expect(put).toHaveBeenCalledTimes(1)
  })

  test('restoring inheritance removes only the selected group override', async () => {
    const user = userEvent.setup()
    render(
      <GroupSettingsFixture protocols='{"openclaw":"chat","special":"responses"}' />
    )
    await user.click(
      groupRow('openclaw').getByRole('combobox', {
        name: 'Upstream text protocol',
      })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Keep existing settings' })
    )
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(
      JSON.parse(savedOptions()['global.group_openai_protocol_bridge'])
    ).toEqual({ special: 'responses' })
  })

  test('renaming a group retains its protocol and billing fields through an empty name draft', async () => {
    const user = userEvent.setup()
    render(
      <GroupSettingsFixture protocols='{"openclaw":"chat","special":"responses"}' />
    )
    const name = groupRow('openclaw').getByRole('textbox', {
      name: 'Group name',
    })
    await user.clear(name)
    expect(
      screen.getByRole('button', { name: 'Save group ratios' })
    ).toBeDisabled()
    await user.type(name, 'renamed')
    await user.tab()
    await user.keyboard('{Escape}')
    expect(
      groupRow('renamed').getByRole('combobox', {
        name: 'Upstream text protocol',
      })
    ).toHaveValue('Use Chat Completions upstream')
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))
    await waitFor(() => expect(Object.keys(savedOptions())).toHaveLength(4))
    expect(
      JSON.parse(savedOptions()['global.group_openai_protocol_bridge'])
    ).toEqual({ renamed: 'chat', special: 'responses' })
    expect(JSON.parse(savedOptions().GroupRatio)).toEqual({
      renamed: 1.5,
      special: 2,
    })
    expect(JSON.parse(savedOptions().TopupGroupRatio)).toEqual({
      renamed: 0.8,
      special: 0.9,
    })
    expect(JSON.parse(savedOptions().UserUsableGroups)).toEqual({
      renamed: 'OpenClaw users',
      special: 'Special users',
    })
    expect(savedOptions()).not.toHaveProperty('GroupGroupRatio')
    expect(savedOptions()).not.toHaveProperty('AutoGroups')
    expect(savedOptions()).not.toHaveProperty(
      'group_ratio_setting.group_special_usable_group'
    )
  })

  test('deleting a group deletes only its protocol and table entries', async () => {
    const user = userEvent.setup()
    render(
      <GroupSettingsFixture protocols='{"openclaw":"chat","special":"responses","archived":"chat"}' />
    )
    await user.click(
      groupRow('openclaw').getByRole('button', { name: 'Delete' })
    )
    expect(screen.queryByDisplayValue('openclaw')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))
    await waitFor(() => expect(Object.keys(savedOptions())).toHaveLength(4))
    expect(
      JSON.parse(savedOptions()['global.group_openai_protocol_bridge'])
    ).toEqual({ special: 'responses', archived: 'chat' })
    expect(JSON.parse(savedOptions().GroupRatio)).toEqual({ special: 2 })
    expect(JSON.parse(savedOptions().TopupGroupRatio)).toEqual({ special: 0.9 })
    expect(JSON.parse(savedOptions().UserUsableGroups)).toEqual({
      special: 'Special users',
    })
    expect(savedOptions()).not.toHaveProperty('GroupGroupRatio')
  })

  test.each(['special', 'archived', 'auto'])(
    'blocks renaming over a reserved or configured group: %s',
    async (target) => {
      const user = userEvent.setup()
      render(
        <GroupSettingsFixture protocols='{"openclaw":"chat","special":"responses","archived":"chat"}' />
      )
      const name = groupRow('openclaw').getByRole('textbox', {
        name: 'Group name',
      })
      fireEvent.change(name, { target: { value: target } })
      fireEvent.blur(name)
      expect(
        screen.getByRole('button', { name: 'Save group ratios' })
      ).toBeDisabled()
      expect(name).toHaveAttribute('aria-invalid', 'true')
      fireEvent.change(name, { target: { value: 'openclaw' } })
      fireEvent.blur(name)
      await user.click(
        screen.getByRole('button', { name: 'Save group ratios' })
      )
      expect(put).not.toHaveBeenCalled()
    }
  )

  test.each([undefined, '{}', '{"openclaw":""}'])(
    'keeps existing channel behavior without adding an override for %s',
    async (protocols) => {
      const user = userEvent.setup()
      render(<GroupSettingsFixture protocols={protocols} />)
      expect(
        groupRow('openclaw').getByRole('combobox', {
          name: 'Upstream text protocol',
        })
      ).toHaveValue('Keep existing settings')
      await user.click(
        screen.getByRole('button', { name: 'Save group ratios' })
      )
      expect(put).not.toHaveBeenCalled()
    }
  )

  test('retains invalid saved configuration, blocks visual edits and saving, and allows an explicit JSON correction', async () => {
    const user = userEvent.setup()
    render(<GroupSettingsFixture protocols='{"openclaw":null}' />)
    expect(
      screen.getByText(
        'Invalid upstream text protocol configuration. Switch to JSON to correct it before saving.'
      )
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Save group ratios' })
    ).toBeDisabled()
    expect(
      groupRow('openclaw').getByRole('combobox', {
        name: 'Upstream text protocol',
      })
    ).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
    const editor = screen.getByRole('textbox', {
      name: 'Upstream text protocol',
    })
    expect(JSON.parse((editor as HTMLTextAreaElement).value)).toEqual({
      openclaw: null,
    })
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))
    expect(
      await screen.findByText(
        'Use a JSON object with non-empty, trimmed group names other than auto and values "", "chat", or "responses".'
      )
    ).toBeVisible()
    expect(put).not.toHaveBeenCalled()
    fireEvent.input(editor, { target: { value: '{"openclaw":"responses"}' } })
    await user.click(screen.getByRole('button', { name: 'Save group ratios' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(
      JSON.parse(savedOptions()['global.group_openai_protocol_bridge'])
    ).toEqual({ openclaw: 'responses' })
    await user.click(screen.getByRole('button', { name: 'Switch to Visual' }))
    expect(
      groupRow('openclaw').getByRole('combobox', {
        name: 'Upstream text protocol',
      })
    ).toHaveValue('Use Responses upstream')
  })

  test.each(['', '   '])(
    'blocks an empty JSON draft instead of submitting an invalid option: %j',
    async (draft) => {
      const user = userEvent.setup()
      render(<GroupSettingsFixture />)
      await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
      fireEvent.input(
        screen.getByRole('textbox', { name: 'Upstream text protocol' }),
        { target: { value: draft } }
      )
      await user.click(
        screen.getByRole('button', { name: 'Save group ratios' })
      )
      expect(await screen.findByText('Value is required')).toBeVisible()
      expect(put).not.toHaveBeenCalled()
      await user.click(screen.getByRole('button', { name: 'Switch to Visual' }))
      expect(
        screen.getByRole('button', { name: 'Save group ratios' })
      ).toBeDisabled()
    }
  )

  test.each([
    '',
    '   ',
    'null',
    '[]',
    '"chat"',
    'false',
    '{',
    '{"":"chat"}',
    '{" openclaw":"chat"}',
    '{"openclaw ":"chat"}',
    '{"auto":""}',
    '{"auto":"chat"}',
    '{"openclaw":null}',
    '{"openclaw":false}',
    '{"openclaw":0}',
    '{"openclaw":{}}',
    '{"openclaw":[]}',
    '{"openclaw":"unknown"}',
    '{"openclaw":" Chat "}',
  ])(
    'rejects malformed protocol configuration without normalizing it away: %s',
    (value) => {
      expect(parseGroupOpenAIProtocolBridge(value)).toBeNull()
    }
  )
})
