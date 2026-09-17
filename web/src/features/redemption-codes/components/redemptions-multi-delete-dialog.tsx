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
import { useMutation } from '@tanstack/react-query'
import type { Table } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { toast } from '@/lib/toast'

import { batchDeleteRedemptions } from '../api'
import type { Redemption } from '../types'
import { useRedemptions } from './redemptions-provider'

type RedemptionsMultiDeleteDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  table: Table<Redemption>
  targets: Redemption[]
}

/** 删除打开确认框时选中的兑换码；批量请求失败时保留目标和选择，供用户重试。 */
export function RedemptionsMultiDeleteDialog(
  props: RedemptionsMultiDeleteDialogProps
) {
  const { t } = useTranslation()
  const { triggerRefresh } = useRedemptions()
  const deletion = useMutation({
    mutationFn: async (targets: Redemption[]) => {
      const result = await batchDeleteRedemptions(
        targets.map((code) => code.id)
      )
      if (!result.success) throw createServerError(result)
      return result.data ?? 0
    },
    onSuccess: (count, targets) => {
      toast.success(
        t('Successfully deleted {{count}} redemption codes', { count })
      )
      props.table.setRowSelection((previous) => {
        const next = { ...previous }
        for (const code of targets) delete next[String(code.id)]
        return next
      })
      props.onOpenChange(false)
      triggerRefresh()
    },
    onError: (error, targets) => {
      handleServerError(
        error,
        t('Failed to delete {{count}} redemption codes', {
          count: targets.length,
        })
      )
    },
  })

  return (
    <ConfirmDialog
      destructive
      open={props.open}
      onOpenChange={(open) => {
        if (!deletion.isPending) props.onOpenChange(open)
      }}
      handleConfirm={() => {
        if (props.targets.length && !deletion.isPending) {
          deletion.mutate(props.targets)
        }
      }}
      isLoading={deletion.isPending}
      disabled={props.targets.length === 0}
      className='max-w-md'
      title={t('Delete {{count}} redemption codes?', {
        count: props.targets.length,
      })}
      desc={t('This action cannot be undone.')}
      confirmText={deletion.isPending ? t('Deleting...') : t('Delete')}
    />
  )
}
