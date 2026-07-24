/*
Copyright (C) 2025 QuantumNous

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

import { useMutation, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'
import { updateSystemOption } from '../api'
import type { SystemOption } from '../types'

/** Collapse rapid successive success toasts into one visible message. */
const SETTING_UPDATED_TOAST_ID = 'system-setting-updated'

// Status keys that affect the public /api/status endpoint.
// When any of these change, invalidate the status query so header/footer
// components refetch without a full page reload.
const STATUS_RELATED_KEYS = new Set([
  'system_name',
  'logo',
  'footer_html',
  'quota_display_type',
  'custom_currency_symbol',
  'usd_exchange_rate',
  'server_address',
  'chats',
  'console_setting.announcements',
  'console_setting.faq',
  'console_setting.api_info',
  'console_setting.uptime_kuma_groups',
  'console_setting.announcements_enabled',
  'console_setting.faq_enabled',
  'console_setting.api_info_enabled',
  'console_setting.uptime_kuma_enabled',
])

export type UpdateOptionVariables = SystemOption & {
  /**
   * When true, skip the built-in success toast (for batch updates that show
   * a single toast after all requests finish, or pages with their own toast).
   */
  silent?: boolean
}

export function useUpdateOption() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ silent: _silent, ...data }: UpdateOptionVariables) => {
      // Convert boolean to string for API compatibility
      const payload: SystemOption = {
        ...data,
        value:
          typeof data.value === 'boolean' ? String(data.value) : data.value,
      }
      return updateSystemOption(payload)
    },
    onSuccess: (response, variables) => {
      if (response.success) {
        // Invalidate system options to refetch
        queryClient.invalidateQueries({ queryKey: ['system-options'] })
        // Also refresh public status when a status-related key changes
        if (STATUS_RELATED_KEYS.has(variables.key)) {
          queryClient.invalidateQueries({ queryKey: ['status'] })
        }
        if (!variables.silent) {
          toast.success(i18next.t('Setting updated successfully'), {
            id: SETTING_UPDATED_TOAST_ID,
          })
        }
      } else {
        // API returned HTTP 200 but business-level failure
        toast.error(response.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })
}
