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
import { expect, test } from 'vitest'

import { channelSchema } from '../../types'
import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
} from '../channel-form'

const modelTypes = {
  'Nano-banana-Pro': 'image',
  '无限制-Seedream-5-Pro-2K': 'image',
  'future-video-v99': 'video',
} as const

test('new plugin models retain exact upstream IDs and metadata in the saved setting', () => {
  const form = channelFormSchema.parse({
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'Mixed media plugin',
    type: 61,
    task_plugin_key: 'mixed-media',
    base_url: 'https://mixed-media.example',
    models: Object.keys(modelTypes).join(','),
    task_plugin_model_types: modelTypes,
  })

  expect(JSON.parse(buildSettingJSON(form)).task_plugin_model_types).toEqual(
    modelTypes
  )
})

test('editing a plugin preserves model types and unknown administrator settings', () => {
  const channel = channelSchema.parse({
    id: 42,
    name: 'Mixed media plugin',
    type: 61,
    key: '',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: Object.keys(modelTypes).join(','),
    group: 'default',
    setting: JSON.stringify({
      task_plugin_key: 'mixed-media',
      task_plugin_model_types: modelTypes,
      administrator_option: { enabled: true },
      http_protocol: 'http1',
    }),
  })
  const form = transformChannelToFormDefaults(channel)
  form.models = 'Nano-banana-Pro'
  form.http_protocol = 'auto'
  const setting = JSON.parse(buildSettingJSON(form))

  expect(setting.task_plugin_model_types).toEqual(modelTypes)
  expect(setting.administrator_option).toEqual({ enabled: true })
  expect(setting).not.toHaveProperty('http_protocol')
})

test('switching from a plugin to a built-in channel removes saved plugin type metadata', () => {
  const setting = JSON.parse(
    buildSettingJSON({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: 1,
      setting: JSON.stringify({
        task_plugin_key: 'mixed-media',
        task_plugin_model_types: modelTypes,
        administrator_option: true,
      }),
    })
  )

  expect(setting).not.toHaveProperty('task_plugin_model_types')
  expect(setting).not.toHaveProperty('task_plugin_key')
  expect(setting.administrator_option).toBe(true)
})
