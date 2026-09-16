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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { TransferDialog } from '../transfer-dialog'

const baseProps = {
  open: true,
  onOpenChange: vi.fn(),
  onConfirm: vi.fn().mockResolvedValue(true),
  transferring: false,
}

describe('transfer dialog withdrawal contract', () => {
  test('defaults the amount to the full withdrawable quota', async () => {
    render(
      <TransferDialog
        {...baseProps}
        availableQuota={1_500_000}
        frozenQuota={0}
      />
    )

    const input = screen.getByLabelText('Transfer Amount')
    expect(input).toHaveValue(3)
  })

  test('shows the withdrawal destination and the in-site balance notice', () => {
    render(
      <TransferDialog
        {...baseProps}
        availableQuota={1_500_000}
        frozenQuota={500_000}
      />
    )

    expect(screen.getByText('Withdrawal Destination')).toBeInTheDocument()
    expect(screen.getByText('In-site balance')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Rewards are credited to your in-site balance and can be used for API calls.'
      )
    ).toBeInTheDocument()
    expect(screen.getByText(/Frozen Rewards/)).toBeInTheDocument()
  })

  test('disables the confirm button when nothing is withdrawable', () => {
    render(
      <TransferDialog
        {...baseProps}
        availableQuota={0}
        frozenQuota={2_000_000}
      />
    )

    expect(screen.getByRole('button', { name: 'Transfer' })).toBeDisabled()
  })

  test('keeps the dialog open and stays retryable when the server rejects', async () => {
    const onConfirm = vi.fn().mockResolvedValue(false)
    const onOpenChange = vi.fn()
    render(
      <TransferDialog
        {...baseProps}
        onConfirm={onConfirm}
        onOpenChange={onOpenChange}
        availableQuota={1_000_000}
        frozenQuota={0}
      />
    )

    await userEvent.click(screen.getByRole('button', { name: 'Transfer' }))

    expect(onConfirm).toHaveBeenCalledWith(1_000_000)
    expect(onOpenChange).not.toHaveBeenCalled()
  })

  test('prevents duplicate submissions while a withdrawal is in flight', () => {
    render(
      <TransferDialog
        {...baseProps}
        availableQuota={1_000_000}
        frozenQuota={0}
        transferring
      />
    )

    expect(screen.getByRole('button', { name: /Transfer/ })).toBeDisabled()
  })
})
