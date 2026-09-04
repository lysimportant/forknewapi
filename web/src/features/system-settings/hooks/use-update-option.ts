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
import type { UpdateOptionRequest } from '../types'

/** 合并连续的成功提示，避免批量保存时重复展示。 */
const SETTING_UPDATED_TOAST_ID = 'system-setting-updated'

/** 会影响公开状态接口缓存的配置项。 */
const STATUS_RELATED_KEYS = new Set([
  'HeaderNavModules',
  'SidebarModulesAdmin',
  'Notice',
  'LogConsumeEnabled',
  'QuotaPerUnit',
  'USDExchangeRate',
  'DisplayInCurrencyEnabled',
  'DisplayTokenStatEnabled',
  'general_setting.quota_display_type',
  'general_setting.custom_currency_symbol',
  'general_setting.custom_currency_exchange_rate',
  'oidc.display_name',
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

export type UpdateOptionVariables = UpdateOptionRequest & {
  /**
   * 为 true 时跳过内置成功提示，用于批量保存或页面自行展示提示的场景。
   */
  silent?: boolean
}

export function useUpdateOption() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ silent: _silent, ...request }: UpdateOptionVariables) =>
      updateSystemOption(request),
    onSuccess: (response, variables) => {
      if (response.success) {
        // 更新系统配置后刷新查询缓存。
        queryClient.invalidateQueries({ queryKey: ['system-options'] })

        if (STATUS_RELATED_KEYS.has(variables.key)) {
          queryClient.invalidateQueries({ queryKey: ['status'] })
          try {
            window.localStorage.removeItem('status')
          } catch {
            // 本地存储不可用时，查询缓存失效仍可保证后续刷新。
          }
        }
        if (!variables.silent) {
          toast.success(i18next.t('Setting updated successfully'), {
            id: SETTING_UPDATED_TOAST_ID,
          })
        }
      } else {
        // HTTP 成功不代表业务保存成功。
        toast.error(response.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })
}
