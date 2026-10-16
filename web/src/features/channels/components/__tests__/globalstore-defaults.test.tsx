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
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema, type Channel } from '../../types'
import { ChannelsProvider } from '../channels-provider'
import { ChannelMutateDrawer } from '../drawers/channel-mutate-drawer'

const baseUrl = 'https://saas.globalstore.ai/kyyReactApiServer'
const imageModels = [
  'banana2',
  'bananapro',
  'banana_c1',
  'banana_c2',
  'globalstore/gpt-image-2',
  'gpt-image-2.5',
  'seedream-5.0',
  'seedream_5.0Pro',
]
const videoModels = [
  'sd_2.0_discount',
  'sd_2.0_fast_discount',
  'sd_2.0_mini_discount',
  'sd_2.5_discount',
  'sd_2.0_special',
  'sd_2.0_fast_special',
  'sd_2.0_mini_special',
  'sd_2.5_special',
  'seedance_2.0_nsfw_c1',
  'grok_video',
  'grok_1.5',
  'omni_720P',
  'veo3.1-fast',
  'wan3.0',
  'wan3.0-Prime',
]
const originalAuth = useAuthStore.getState().auth
let client: QueryClient
let catalog: { success: boolean; data?: Record<number, string[]> }
let saved: Channel

function Harness(props: { editing?: boolean }) {
  return (
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelMutateDrawer
          open
          onOpenChange={() => {}}
          currentRow={props.editing ? saved : undefined}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  catalog = {
    success: true,
    data: { 62: imageModels, 1: ['unrelated-text-model'] },
  }
  saved = channelSchema.parse({
    id: 42,
    name: 'Saved image channel',
    type: 62,
    key: '',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'custom-image',
    group: 'default',
    base_url: 'https://saved.example/media',
  })
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/models') return { data: catalog }
    if (url === '/api/channel/42') {
      return { data: { success: true, data: saved } }
    }
    if (url === '/api/task_plugin_options') {
      return {
        data: {
          success: true,
          data: [
            {
              key: 'globalstore',
              name: 'GlobalStore Model Center',
              baseUrl,
              models: videoModels,
            },
          ],
        },
      }
    }
    if (url === '/api/channel/models') {
      return { data: { success: true, data: [{ id: 'custom-image' }] } }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === '/api/prefill_group') {
      return { data: { success: true, data: [] } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState({ auth: originalAuth })
  vi.restoreAllMocks()
})

test('selecting GlobalStore Images fills its address and eight image models without requesting upstream discovery', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  render(<Harness />)
  await user.click(screen.getByRole('option', { name: /GlobalStore Images/ }))
  expect(screen.getByLabelText('Base URL')).toHaveValue(baseUrl)
  expect(
    await screen.findByRole('button', { name: 'seedream_5.0Pro' })
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Fetch from Upstream' })
  ).not.toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('API Key *'), {
    target: { value: 'fixture-key' },
  })
  await user.click(screen.getByRole('button', { name: 'Create Channel' }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post.mock.calls[0][0]).toBe('/api/channel')
  expect(post.mock.calls[0][1]).toMatchObject({
    channel: { type: 62, base_url: baseUrl, models: imageModels.join(',') },
  })
  expect(post).toHaveBeenCalledTimes(1)
})

test('selecting GlobalStore Video fills the plugin address and all fifteen video models', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  render(<Harness />)
  await user.click(
    await screen.findByRole('option', { name: /GlobalStore Video/ })
  )
  expect(screen.getByLabelText('Base URL *')).toHaveValue(baseUrl)
  fireEvent.change(screen.getByLabelText('API Key *'), {
    target: { value: 'fixture-key' },
  })
  await user.click(screen.getByRole('button', { name: 'Create Channel' }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post.mock.calls[0][1]).toMatchObject({
    channel: { type: 61, base_url: baseUrl, models: videoModels.join(',') },
  })
  expect(api.get).not.toHaveBeenCalledWith('/api/models')
})

test('switching GlobalStore media providers replaces their model list while preserving a manually entered address and key', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  render(<Harness />)
  await user.click(screen.getByRole('option', { name: /GlobalStore Images/ }))
  await screen.findByRole('button', { name: 'seedream_5.0Pro' })
  fireEvent.change(screen.getByLabelText('Base URL'), {
    target: { value: 'https://my-proxy.example' },
  })
  fireEvent.change(screen.getByLabelText('API Key *'), {
    target: { value: 'fixture-key' },
  })
  await user.click(screen.getByRole('button', { name: 'Change provider' }))
  await user.click(screen.getByRole('option', { name: /GlobalStore Video/ }))
  expect(
    screen.queryByRole('button', { name: 'seedream_5.0Pro' })
  ).not.toBeInTheDocument()
  expect(screen.getByLabelText('Base URL *')).toHaveValue(
    'https://my-proxy.example'
  )
  await user.click(screen.getByRole('button', { name: 'Change provider' }))
  await user.click(screen.getByRole('option', { name: /GlobalStore Images/ }))
  await screen.findByRole('button', { name: 'seedream_5.0Pro' })
  expect(screen.getByLabelText('Base URL')).toHaveValue(
    'https://my-proxy.example'
  )
  expect(screen.getByLabelText('API Key *')).toHaveValue('fixture-key')
  await user.click(screen.getByRole('button', { name: 'Create Channel' }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post.mock.calls[0][1]).toMatchObject({
    channel: { type: 62, models: imageModels.join(',') },
  })
})

test('opening a saved GlobalStore image channel preserves its custom address and models', async () => {
  render(<Harness editing />)
  await screen.findByDisplayValue('Saved image channel')
  expect(screen.getByLabelText('Base URL')).toHaveValue(saved.base_url)
  expect(screen.getByRole('button', { name: 'custom-image' })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'seedream_5.0Pro' })
  ).not.toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalledWith('/api/models')
})

test('a failed image catalog shows retry and preserves manually entered models when retry succeeds', async () => {
  catalog = { success: false }
  const user = userEvent.setup()
  render(<Harness />)
  await user.click(screen.getByRole('option', { name: /GlobalStore Images/ }))
  expect(await screen.findByText('Failed to load models')).toBeVisible()
  await user.type(
    screen.getByRole('combobox', { name: 'Select models or add custom ones' }),
    'custom-image,'
  )
  await user.keyboard('{Escape}')
  catalog = { success: true, data: { 62: imageModels } }
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() =>
    expect(screen.queryByText('Failed to load models')).not.toBeInTheDocument()
  )
  expect(screen.getByRole('button', { name: 'custom-image' })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'seedream_5.0Pro' })
  ).not.toBeInTheDocument()
})

test('a late image catalog cannot overwrite the selected video provider models', async () => {
  let finish!: (value: { data: typeof catalog }) => void
  const pending = new Promise<{ data: typeof catalog }>((resolve) => {
    finish = resolve
  })
  const get = vi.mocked(api.get).getMockImplementation()
  if (!get) throw new Error('Missing API fixture')
  vi.mocked(api.get).mockImplementation((url, ...args) =>
    url === '/api/models' ? pending : get(url, ...args)
  )
  const user = userEvent.setup()
  render(<Harness />)
  await user.click(screen.getByRole('option', { name: /GlobalStore Images/ }))
  await user.click(screen.getByRole('button', { name: 'Change provider' }))
  await user.click(
    await screen.findByRole('option', { name: /GlobalStore Video/ })
  )
  await act(async () => {
    finish({ data: catalog })
    await pending
  })
  expect(screen.getByRole('button', { name: 'sd_2.0_discount' })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'seedream_5.0Pro' })
  ).not.toBeInTheDocument()
})

test('an empty built-in image catalog exposes retry without inserting unrelated models', async () => {
  catalog = { success: true, data: { 62: [], 1: ['unrelated-text-model'] } }
  const user = userEvent.setup()
  render(<Harness />)
  await user.click(screen.getByRole('option', { name: /GlobalStore Images/ }))
  expect(await screen.findByText('Failed to load models')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  expect(
    screen.queryByRole('button', { name: 'unrelated-text-model' })
  ).not.toBeInTheDocument()
  expect(screen.getByLabelText('Base URL')).toHaveValue(baseUrl)
})
