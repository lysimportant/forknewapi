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
import type { Table } from '@tanstack/react-table'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { Redemption } from '../../types'
import { DataTableBulkActions } from '../data-table-bulk-actions'
import { RedemptionsProvider } from '../redemptions-provider'

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  delete: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalDelete = apiClient.delete

function redemption(id: number): Redemption {
  return {
    id,
    user_id: 1,
    name: `code-${id}`,
    key: `key-${id}`,
    status: 1,
    quota: 500000,
    created_time: 1,
    redeemed_time: 0,
    expired_time: 0,
    used_user_id: 0,
  }
}

function createTable(items: Redemption[]) {
  const resetRowSelection = vi.fn()
  const rows = items.map((original) => ({ original }))
  const table = {
    getFilteredSelectedRowModel: () => ({ rows }),
    getSelectedRowModel: () => ({ rows }),
    resetRowSelection,
  } as unknown as Table<Redemption>

  return { table, resetRowSelection }
}

function renderBulkActions(items: Redemption[]) {
  const { table, resetRowSelection } = createTable(items)
  render(
    <RedemptionsProvider>
      <DataTableBulkActions table={table} />
    </RedemptionsProvider>
  )
  return { resetRowSelection }
}

afterEach(() => {
  apiClient.delete = originalDelete
})

describe('redemption bulk actions', () => {
  test('places a delete control next to copy for the current selection', () => {
    renderBulkActions([redemption(1), redemption(2)])

    const copyButton = screen.getByRole('button', {
      name: 'Copy selected codes',
    })
    const deleteButton = screen.getByRole('button', {
      name: 'Delete selected redemption codes',
    })

    expect(copyButton.compareDocumentPosition(deleteButton)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING
    )
  })

  test('deletes every selected redemption code after confirmation', async () => {
    const deletedUrls: string[] = []
    apiClient.delete = async (url) => {
      deletedUrls.push(url)
      return { data: { success: true } }
    }

    const { resetRowSelection } = renderBulkActions([
      redemption(11),
      redemption(12),
    ])
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('button', { name: 'Delete selected redemption codes' })
    )
    expect(
      screen.getByRole('heading', { name: 'Delete 2 redemption code(s)?' })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() =>
      expect(deletedUrls).toEqual([
        '/api/redemption/11/',
        '/api/redemption/12/',
      ])
    )
    expect(resetRowSelection).toHaveBeenCalledOnce()
    expect(
      screen.queryByRole('heading', { name: 'Delete 2 redemption code(s)?' })
    ).not.toBeInTheDocument()
  })

  test('keeps the selection when every selected deletion fails', async () => {
    apiClient.delete = async () => ({
      data: { success: false, message: 'still in use' },
    })

    const { resetRowSelection } = renderBulkActions([redemption(21)])
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('button', { name: 'Delete selected redemption codes' })
    )
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled()
    )

    expect(resetRowSelection).not.toHaveBeenCalled()
    expect(
      screen.getByRole('heading', { name: 'Delete 1 redemption code(s)?' })
    ).toBeInTheDocument()
  })
})
