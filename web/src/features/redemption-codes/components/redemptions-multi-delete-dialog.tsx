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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { toast } from '@/lib/toast'

import { deleteRedemption } from '../api'
import { ERROR_MESSAGES } from '../constants'
import type { Redemption } from '../types'
import { useRedemptions } from './redemptions-provider'

type RedemptionsMultiDeleteDialogProps<TData> = {
  open: boolean
  onOpenChange: (open: boolean) => void
  table: Table<TData>
}

export function RedemptionsMultiDeleteDialog<TData>({
  open,
  onOpenChange,
  table,
}: RedemptionsMultiDeleteDialogProps<TData>) {
  const { t } = useTranslation()
  const { triggerRefresh } = useRedemptions()
  const [isDeleting, setIsDeleting] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows

  const handleConfirm = async () => {
    if (selectedRows.length === 0 || isDeleting) return

    setIsDeleting(true)
    try {
      let successCount = 0
      for (const row of selectedRows) {
        const redemption = row.original as Redemption
        try {
          const result = await deleteRedemption(redemption.id, {
            skipBusinessError: true,
            skipErrorHandler: true,
          })
          if (result.success) {
            successCount += 1
          }
        } catch {
          // 单条失败不影响其余选中项，结束时统一提示。
        }
      }

      const failedCount = selectedRows.length - successCount
      if (successCount > 0) {
        toast.success(
          t('Successfully deleted {{count}} redemption code(s)', {
            count: successCount,
          })
        )
        table.resetRowSelection()
        triggerRefresh()
        onOpenChange(false)
      }
      if (failedCount > 0) {
        toast.error(t(ERROR_MESSAGES.BATCH_DELETE_FAILED))
      }
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <ConfirmDialog
      destructive
      open={open}
      onOpenChange={onOpenChange}
      handleConfirm={handleConfirm}
      isLoading={isDeleting}
      className='max-w-md'
      title={t('Delete {{count}} redemption code(s)?', {
        count: selectedRows.length,
      })}
      desc={
        <>
          {t('You are about to delete {{count}} redemption code(s).', {
            count: selectedRows.length,
          })}{' '}
          <br />
          {t('This action cannot be undone.')}
        </>
      }
      confirmText={t('Delete')}
    />
  )
}
