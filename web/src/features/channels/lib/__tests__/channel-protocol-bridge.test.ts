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
import { describe, expect, test } from 'vitest'

import { channelSchema } from '../../types'
import {
  buildSettingJSON,
  channelFormSchema,
  transformChannelToFormDefaults,
} from '../channel-form'

/** 构造已有渠道，使用非敏感字段验证设置经过编辑表单后不会丢失。 */
function existingChannel(setting: Record<string, unknown>) {
  return channelSchema.parse({
    id: 42,
    name: 'Existing OpenAI channel',
    type: 1,
    key: '',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'text-model',
    group: 'default',
    setting: JSON.stringify(setting),
  })
}

describe('legacy channel OpenAI protocol bridge', () => {
  test.each(['chat', 'responses'])(
    'preserves %s when another setting is edited and read back',
    (protocol) => {
      const channel = existingChannel({
        openai_protocol_bridge: protocol,
        force_format: true,
        pass_through_body_enabled: false,
        system_prompt: 'Keep this prompt',
        unrelated_unknown_field: 'not part of the form contract',
      })
      const defaults = transformChannelToFormDefaults(channel)
      const values = channelFormSchema.parse({
        ...defaults,
        name: 'Renamed channel',
        proxy: 'http://proxy.example',
      })
      const saved = buildSettingJSON(values)
      expect(JSON.parse(saved)).toMatchObject({
        openai_protocol_bridge: protocol,
        force_format: true,
        pass_through_body_enabled: false,
        system_prompt: 'Keep this prompt',
        proxy: 'http://proxy.example',
      })
      expect(JSON.parse(saved)).not.toHaveProperty('unrelated_unknown_field')
      expect(
        transformChannelToFormDefaults({ ...channel, setting: saved })
      ).toHaveProperty('openai_protocol_bridge', protocol)
    }
  )

  test.each([{}, { openai_protocol_bridge: '' }])(
    'keeps legacy behavior for an unset protocol: %j',
    (setting) => {
      const values = channelFormSchema.parse(
        transformChannelToFormDefaults(existingChannel(setting))
      )
      expect(JSON.parse(buildSettingJSON(values))).not.toHaveProperty(
        'openai_protocol_bridge'
      )
    }
  )

  test.each(
    ['unknown', null, false, 0, {}, []].map((protocol) => ({ protocol }))
  )(
    'rejects an invalid saved protocol instead of silently dropping it: $protocol',
    ({ protocol }) => {
      const values = transformChannelToFormDefaults(
        existingChannel({ openai_protocol_bridge: protocol })
      )
      const result = channelFormSchema.safeParse(values)
      expect(result.success).toBe(false)
      if (!result.success) {
        expect(result.error.issues).toEqual(
          expect.arrayContaining([
            expect.objectContaining({ path: ['openai_protocol_bridge'] }),
          ])
        )
      }
      expect(() => buildSettingJSON(values)).toThrow()
    }
  )
  test('preserves explicitly configured body passthrough instead of silently disabling it', () => {
    const values = channelFormSchema.parse(
      transformChannelToFormDefaults(
        existingChannel({
          openai_protocol_bridge: 'chat',
          pass_through_body_enabled: true,
        })
      )
    )
    expect(JSON.parse(buildSettingJSON(values))).toMatchObject({
      openai_protocol_bridge: 'chat',
      pass_through_body_enabled: true,
    })
  })
})
