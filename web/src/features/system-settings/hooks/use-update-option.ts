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
import { isAxiosError } from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'

import { getServerErrorMessageKey } from '@/lib/server-error-message'

import { updateSystemOption } from '../api'
import type { UpdateOptionRequest } from '../types'

/** 连续保存共用提示标识，避免重复显示成功消息。 */
const SETTING_UPDATED_TOAST_ID = 'system-setting-updated'

/** 影响公开状态的官方及定制配置键，保存后刷新页眉等依赖状态的组件。 */
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

/** 保存配置的输入；silent 仅控制本地提示，不发送至 API。 */
export type UpdateOptionVariables = UpdateOptionRequest & {
  /**
   * 批量保存或页面自行提示时设为 true，跳过内建成功提示。
   */
  silent?: boolean
}

/** 保存成功后刷新缓存；业务或传输失败均拒绝 mutateAsync，并由本钩子统一提示错误。 */
export function useUpdateOption() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ silent: _silent, ...data }: UpdateOptionVariables) => {
      // 布尔值转换为字符串以兼容已有保存行为。
      const payload: UpdateOptionRequest = {
        ...data,
        value:
          typeof data.value === 'boolean' ? String(data.value) : data.value,
      }
      const response = await updateSystemOption(payload)
      if (!response.success) {
        const messageKey = getServerErrorMessageKey(response)
        throw new Error(
          messageKey
            ? i18next.t(messageKey)
            : response.message || i18next.t('Failed to update setting')
        )
      }
      return response
    },
    onSuccess: (_response, variables) => {
      // 只有确认业务成功才刷新缓存，失败时保留页面的未保存草稿。
      queryClient.invalidateQueries({ queryKey: ['system-options'] })
      if (STATUS_RELATED_KEYS.has(variables.key)) {
        queryClient.invalidateQueries({ queryKey: ['status'] })
        try {
          window.localStorage.removeItem('status')
        } catch {
          // 浏览器禁用存储时仍通过查询失效刷新状态。
        }
      }
      if (!variables.silent) {
        toast.success(i18next.t('Setting updated successfully'), {
          id: SETTING_UPDATED_TOAST_ID,
        })
      }
    },
    onError: (error: Error) => {
      // 保存 API 关闭全局错误提示，由此处保留服务端原因与翻译且只提示一次。
      const messageKey = getServerErrorMessageKey(error)
      const serverMessage = isAxiosError<{ message?: string }>(error)
        ? error.response?.data?.message
        : undefined
      toast.error(
        messageKey
          ? i18next.t(messageKey)
          : serverMessage ||
              error.message ||
              i18next.t('Failed to update setting')
      )
    },
  })
}
