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
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

import {
  DEFAULT_LOGO,
  DEFAULT_SYSTEM_NAME,
  normalizeSystemLogo,
  normalizeSystemName,
} from '@/lib/constants'

export type CurrencyDisplayType = 'USD' | 'CNY' | 'TOKENS' | 'CUSTOM'

export interface CurrencyConfig {
  /** Whether to render quota values as currency instead of raw units */
  displayInCurrency: boolean
  /** Currency presentation strategy configured by the admin */
  quotaDisplayType: CurrencyDisplayType
  /** Number of quota units that equal one USD */
  quotaPerUnit: number
  /** Exchange rate from USD to the configured local currency */
  usdExchangeRate: number
  /** Custom currency symbol configured by the admin (used when type === CUSTOM) */
  customCurrencySymbol: string
  /** Exchange rate from USD to the custom currency (used when type === CUSTOM) */
  customCurrencyExchangeRate: number
}

export interface SystemConfig {
  systemName: string
  logo: string
  footerHtml?: string
  demoSiteEnabled?: boolean
  displayTokenStatEnabled?: boolean
  currency: CurrencyConfig
}

export const DEFAULT_CURRENCY_CONFIG: CurrencyConfig = {
  displayInCurrency: true,
  quotaDisplayType: 'USD',
  quotaPerUnit: 500000,
  usdExchangeRate: 1,
  customCurrencySymbol: '¤',
  customCurrencyExchangeRate: 1,
}

interface SystemConfigState {
  config: SystemConfig
  loading: boolean
  loadedLogoUrl: string
  setConfig: (config: Partial<SystemConfig>) => void
  setLoadedLogoUrl: (url: string) => void
  setLoading: (loading: boolean) => void
}

/** 自动持久化系统名称、图标、页脚、货币显示和加载状态。 */
export const useSystemConfigStore = create<SystemConfigState>()(
  persist(
    (set) => ({
      config: {
        systemName: DEFAULT_SYSTEM_NAME,
        logo: DEFAULT_LOGO,
        currency: { ...DEFAULT_CURRENCY_CONFIG },
      },
      loading: true,
      loadedLogoUrl: DEFAULT_LOGO,
      setConfig: (newConfig) =>
        set((state) => {
          const systemName = normalizeSystemName(
            newConfig.systemName ?? state.config.systemName
          )
          const logo = normalizeSystemLogo(newConfig.logo ?? state.config.logo)
          return {
            config: {
              ...state.config,
              ...newConfig,
              systemName,
              logo,
              currency: {
                ...state.config.currency,
                ...newConfig.currency,
              },
            },
          }
        }),
      setLoadedLogoUrl: (url) => set({ loadedLogoUrl: url }),
      setLoading: (loading) => set({ loading }),
    }),
    {
      name: 'system-config-storage',
      partialize: (state) => ({
        config: state.config,
        loadedLogoUrl: state.loadedLogoUrl,
      }),
      merge: (persisted, current) => {
        const persistedState =
          persisted !== null &&
          typeof persisted === 'object' &&
          !Array.isArray(persisted)
            ? (persisted as Partial<SystemConfigState>)
            : {}
        const persistedConfig = persistedState.config
        const logo = normalizeSystemLogo(
          persistedConfig?.logo ?? current.config.logo
        )
        return {
          ...current,
          config: {
            ...current.config,
            ...persistedConfig,
            systemName: normalizeSystemName(
              persistedConfig?.systemName ?? current.config.systemName
            ),
            logo,
            currency: {
              ...current.config.currency,
              ...persistedConfig?.currency,
            },
          },
          loadedLogoUrl: normalizeSystemLogo(
            persistedState.loadedLogoUrl ?? logo
          ),
        }
      },
    }
  )
)

// Selector helpers for convenience
export const getSystemName = () =>
  useSystemConfigStore.getState().config.systemName

export const getLogo = () => useSystemConfigStore.getState().config.logo

export const getFooterHtml = () =>
  useSystemConfigStore.getState().config.footerHtml
