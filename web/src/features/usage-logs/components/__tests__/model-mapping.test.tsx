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
  getCoreRowModel,
  useReactTable,
  flexRender,
} from '@tanstack/react-table'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { formatModelName } from '../../lib/format'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { DetailsDialog } from '../dialogs/details-dialog'
import { ModelBadge } from '../model-badge'

const requestModel = 'customer-facing-model-with-a-long-routing-alias'
const actualModel = 'provider-production-model-with-a-long-versioned-name'
const responseModel = 'provider-response-model-with-a-different-version-name'
const baseLog: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
  username: 'user',
  token_name: 'token',
  model_name: requestModel,
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 0,
  is_stream: false,
  channel: 1,
  channel_name: '',
  token_id: 1,
  group: 'default',
  ip: '',
  other: '',
  request_id: 'req-1',
  upstream_request_id: '',
}

function makeLog(other: unknown): UsageLog {
  return {
    ...baseLog,
    other: typeof other === 'string' ? other : JSON.stringify(other),
  }
}

describe('model mapping detection', () => {
  test.each([
    ['missing', { upstream_model_name: actualModel }],
    ['false', { is_model_mapped: false, upstream_model_name: actualModel }],
  ])(
    'shows a different upstream model when the legacy flag is %s',
    (_flag, other) => {
      expect(formatModelName(makeLog(other))).toEqual({
        name: requestModel,
        isMapped: true,
        actualModel,
        responseModel: undefined,
        isMismatch: false,
      })
    }
  )

  test('preserves the recorded upstream model name after validating its content', () => {
    const recordedName = `  ${actualModel}  `

    expect(
      formatModelName(makeLog({ upstream_model_name: recordedName }))
    ).toEqual({
      name: requestModel,
      isMapped: true,
      actualModel: recordedName,
      responseModel: undefined,
      isMismatch: false,
    })
  })

  test('reports a server-confirmed difference between sent and response models', () => {
    expect(
      formatModelName(
        makeLog({
          upstream_model_name: actualModel,
          upstream_response_model: responseModel,
          upstream_model_mismatch: true,
        })
      )
    ).toEqual({
      name: requestModel,
      isMapped: true,
      actualModel,
      responseModel,
      isMismatch: true,
    })
  })

  test('reports a response mismatch when the request and sent models match', () => {
    expect(
      formatModelName(
        makeLog({
          upstream_model_name: requestModel,
          upstream_response_model: responseModel,
          upstream_model_mismatch: true,
        })
      )
    ).toEqual({
      name: requestModel,
      isMapped: false,
      actualModel: undefined,
      responseModel,
      isMismatch: true,
    })
  })

  test('does not treat a billing alias as a request-to-sent model mapping', () => {
    const log = makeLog({
      requested_model_name: requestModel,
      upstream_model_name: requestModel,
      upstream_response_model: responseModel,
      upstream_model_mismatch: true,
    })

    expect(
      formatModelName({ ...log, model_name: 'billing-model-alias' })
    ).toEqual({
      name: requestModel,
      isMapped: false,
      actualModel: undefined,
      responseModel,
      isMismatch: true,
    })
  })

  test.each([
    [
      'the mismatch flag is missing',
      {
        upstream_model_name: actualModel,
        upstream_response_model: responseModel,
      },
      responseModel,
    ],
    [
      'the mismatch flag is false',
      {
        upstream_model_name: actualModel,
        upstream_response_model: responseModel,
        upstream_model_mismatch: false,
      },
      responseModel,
    ],
    [
      'the sent model is missing',
      {
        upstream_response_model: responseModel,
        upstream_model_mismatch: true,
      },
      responseModel,
    ],
    [
      'the response model is blank',
      {
        upstream_model_name: actualModel,
        upstream_response_model: '   ',
        upstream_model_mismatch: true,
      },
      undefined,
    ],
  ])(
    'does not infer a response mismatch when %s',
    (_scenario, other, expectedResponse) => {
      expect(formatModelName(makeLog(other))).toMatchObject({
        responseModel: expectedResponse,
        isMismatch: false,
      })
    }
  )

  test.each([
    [
      'the same model name',
      { is_model_mapped: true, upstream_model_name: requestModel },
    ],
    ['an empty model name', { is_model_mapped: true, upstream_model_name: '' }],
    [
      'a whitespace-only model name',
      { is_model_mapped: true, upstream_model_name: '   ' },
    ],
    [
      'a non-string model name',
      { is_model_mapped: true, upstream_model_name: 42 },
    ],
    ['malformed log metadata', '{not-json'],
  ])('does not report a mapping for %s', (_scenario, other) => {
    expect(formatModelName(makeLog(other))).toEqual({
      name: requestModel,
      isMapped: false,
      actualModel: undefined,
      responseModel: undefined,
      isMismatch: false,
    })
  })
})

describe('model mapping badge', () => {
  test('shows all differing models immediately and copies their complete values from the popover', async () => {
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    render(
      <ModelBadge
        modelName={requestModel}
        actualModel={actualModel}
        responseModel={responseModel}
        isMismatch
      />
    )

    expect(screen.getByText(requestModel)).toBeVisible()
    const actualModelButton = screen.getByRole('button', {
      name: `Actual Model: ${actualModel}`,
    })
    expect(actualModelButton).toBeVisible()
    expect(within(actualModelButton).getByText(actualModel)).toBeVisible()
    expect(screen.getByText('Response mismatch:')).toBeVisible()
    const responseModelButton = screen.getByRole('button', {
      name: `Upstream Response Model: ${responseModel}`,
    })
    expect(responseModelButton).toBeVisible()
    expect(within(responseModelButton).getByText(responseModel)).toBeVisible()

    responseModelButton.focus()
    await user.keyboard('{Enter}')
    const popover = await screen.findByRole('dialog', {
      name: 'Model Details',
    })
    expect(within(popover).getByText(requestModel)).toBeVisible()
    expect(within(popover).getByText(actualModel)).toBeVisible()
    expect(within(popover).getByText(responseModel)).toBeVisible()

    const copyButtons = within(popover).getAllByRole('button', {
      name: 'Copy to clipboard',
    })
    expect(copyButtons).toHaveLength(3)
    await user.click(copyButtons[0])
    await user.click(copyButtons[1])
    await user.click(copyButtons[2])
    expect(copy).toHaveBeenNthCalledWith(1, requestModel)
    expect(copy).toHaveBeenNthCalledWith(2, actualModel)
    expect(copy).toHaveBeenNthCalledWith(3, responseModel)

    await user.keyboard('{Escape}')
    await waitFor(() => expect(popover).not.toBeInTheDocument())
    expect(responseModelButton).toHaveFocus()
  })

  test.each([
    ['missing', undefined],
    ['the same as the request', requestModel],
  ])(
    'shows a single model when the upstream model is %s',
    (_scenario, upstreamModel) => {
      render(
        <ModelBadge modelName={requestModel} actualModel={upstreamModel} />
      )

      expect(screen.getByText(requestModel)).toBeVisible()
      expect(
        screen.queryByRole('button', { name: /^Actual Model:/ })
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Response mismatch:')).not.toBeInTheDocument()
    }
  )

  test('shows a confirmed response mismatch without a request mapping', () => {
    render(
      <ModelBadge
        modelName={requestModel}
        responseModel={responseModel}
        isMismatch
      />
    )

    expect(
      screen.queryByRole('button', { name: /^Actual Model:/ })
    ).not.toBeInTheDocument()
    expect(screen.getByText('Response mismatch:')).toBeVisible()
    expect(
      screen.getByRole('button', {
        name: `Upstream Response Model: ${responseModel}`,
      })
    ).toBeVisible()
  })

  test.each([
    ['matched', false],
    ['unknown', undefined],
  ])('does not warn when the response status is %s', (_status, isMismatch) => {
    render(
      <ModelBadge
        modelName={requestModel}
        responseModel={responseModel}
        isMismatch={isMismatch}
      />
    )

    expect(screen.queryByText('Response mismatch:')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /^Upstream Response Model:/ })
    ).not.toBeInTheDocument()
  })

  test('keeps the mobile inspection button while exposing the actual model inline', async () => {
    const user = userEvent.setup()
    const onInspect = vi.fn()
    render(
      <ModelBadge
        modelName={requestModel}
        actualModel={actualModel}
        responseModel={responseModel}
        isMismatch
        reasoningEffort='max'
        onInspect={onInspect}
      />
    )

    const inspectButton = screen.getByRole('button', {
      name: `Model: ${requestModel}`,
    })
    expect(within(inspectButton).getByText(requestModel)).toBeVisible()
    expect(within(inspectButton).getByText(actualModel)).toBeVisible()
    expect(within(inspectButton).getByText(responseModel)).toBeVisible()
    expect(
      within(inspectButton).getByText('Reasoning Effort: max')
    ).toBeVisible()
    await user.click(within(inspectButton).getByText('Reasoning Effort: max'))
    expect(onInspect).toHaveBeenCalledOnce()
  })
})

test('details show different request and actual models without the legacy flag', () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <DetailsDialog
        log={makeLog({
          is_model_mapped: false,
          upstream_model_name: actualModel,
        })}
        isAdmin={false}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )

  const mappingSection = screen.getByText('Model Mapping').parentElement
  expect(mappingSection).not.toBeNull()
  expect(screen.queryByText('Model Details')).not.toBeInTheDocument()
  expect(
    within(mappingSection as HTMLElement).getByText(requestModel)
  ).toBeVisible()
  expect(
    within(mappingSection as HTMLElement).getByText(actualModel)
  ).toBeVisible()
  queryClient.clear()
})

test.each([
  ['confirmed', true],
  ['unknown', undefined],
])(
  'details show the upstream response model when mismatch status is %s',
  (_status, mismatchFlag) => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <DetailsDialog
          log={makeLog({
            upstream_model_name: actualModel,
            upstream_response_model: responseModel,
            upstream_model_mismatch: mismatchFlag,
          })}
          isAdmin={false}
          isRoot={false}
          open
          onOpenChange={() => undefined}
        />
      </QueryClientProvider>
    )

    const detailsSection = screen.getByText('Model Details').parentElement
    expect(detailsSection).not.toBeNull()
    expect(screen.queryByText('Model Mapping')).not.toBeInTheDocument()
    expect(
      within(detailsSection as HTMLElement).getByText(requestModel)
    ).toBeVisible()
    expect(
      within(detailsSection as HTMLElement).getByText(actualModel)
    ).toBeVisible()
    expect(
      within(detailsSection as HTMLElement).getByText(responseModel)
    ).toBeVisible()
    if (mismatchFlag) {
      expect(
        within(detailsSection as HTMLElement).getByText(
          'Upstream model mismatch'
        )
      ).toBeVisible()
    } else {
      expect(
        within(detailsSection as HTMLElement).queryByText(
          'Upstream model mismatch'
        )
      ).not.toBeInTheDocument()
    }
    queryClient.clear()
  }
)

/** 渲染真实日志列以验证元数据接入；只显示模型列，避免依赖其他列的查询。 */
function ModelColumnFixture(props: { log: UsageLog; isAdmin: boolean }) {
  const columns = useCommonLogsColumns(props.isAdmin, false)
  const table = useReactTable({
    data: [props.log],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === 'model_name')
  if (!cell) throw new Error('Model column is missing')
  return <>{flexRender(cell.column.columnDef.cell, cell.getContext())}</>
}

describe('recorded reasoning effort in model column', () => {
  test.each([true, false])(
    'shows recorded effort in admin=%s view without opening details',
    async (isAdmin) => {
      const user = userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      render(
        <ModelColumnFixture
          isAdmin={isAdmin}
          log={makeLog({ reasoning_effort: 'max' })}
        />
      )
      expect(screen.getByText('Reasoning Effort: max')).toBeVisible()
      await user.click(screen.getByText(requestModel))
      expect(copy).toHaveBeenCalledWith(requestModel)
    }
  )

  test.each(['none', 'low', 'high', 'xhigh', 'custom-level', '1024'])(
    'preserves recorded value %s',
    (effort) => {
      render(
        <ModelColumnFixture
          isAdmin={false}
          log={makeLog({ reasoning_effort: effort })}
        />
      )
      expect(screen.getByText(`Reasoning Effort: ${effort}`)).toBeVisible()
    }
  )

  test.each([undefined, '', '   ', null, 0, false, {}])(
    'omits an absent or invalid effort %j instead of guessing from the model',
    (effort) => {
      render(
        <ModelColumnFixture
          isAdmin={false}
          log={{
            ...makeLog({ reasoning_effort: effort }),
            model_name: 'model-high',
          }}
        />
      )
      expect(screen.getByText('model-high')).toBeVisible()
      expect(screen.queryByText(/Reasoning Effort/)).not.toBeInTheDocument()
    }
  )

  test('keeps effort visible alongside mapped and mismatched models', async () => {
    const user = userEvent.setup()
    render(
      <ModelColumnFixture
        isAdmin
        log={makeLog({
          reasoning_effort: ' high ',
          upstream_model_name: actualModel,
          upstream_response_model: responseModel,
          upstream_model_mismatch: true,
        })}
      />
    )
    expect(screen.getByText('Reasoning Effort: high')).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: `Actual Model: ${actualModel}` })
    )
    const dialog = await screen.findByRole('dialog', { name: 'Model Details' })
    expect(within(dialog).getByText(actualModel)).toBeVisible()
    expect(within(dialog).getByText(responseModel)).toBeVisible()
  })
})
