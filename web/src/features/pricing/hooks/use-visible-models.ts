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
import { useEffect, useMemo, useReducer } from 'react'

import {
  getCatalogAvailabilityStatus,
  MAX_CATALOG_STATUS_AGE_MS,
} from '../lib/catalog-availability'
import { isSeedanceModel } from '../lib/seedance-model'
import { isYuanliuModel } from '../lib/yuanliu-model'
import type {
  PricingModel,
  SeedanceAvailabilityMap,
  YuanliuAvailabilityMap,
} from '../types'

/** Seedance 仅展示新鲜可用模型；Yuan 保留只隐藏已确认负向状态的行为。 */
export function useVisibleModels(
  models: PricingModel[],
  availability: YuanliuAvailabilityMap,
  isUnavailable: boolean,
  seedanceAvailability: SeedanceAvailabilityMap = {},
  isSeedanceUnavailable = false
): PricingModel[] {
  const [, refresh] = useReducer((version: number) => version + 1, 0)
  const now = Date.now()
  const visibleModels = useMemo(
    () =>
      models.filter((model) => {
        if (
          isSeedanceModel(model, seedanceAvailability) &&
          getCatalogAvailabilityStatus(
            seedanceAvailability[model.model_name],
            isSeedanceUnavailable,
            now
          ) !== 'available'
        ) {
          return false
        }
        if (!isYuanliuModel(model, availability)) return true
        const status = getCatalogAvailabilityStatus(
          availability[model.model_name],
          isUnavailable,
          now
        )
        return status !== 'sold_out' && status !== 'disabled'
      }),
    [
      models,
      availability,
      isUnavailable,
      seedanceAvailability,
      isSeedanceUnavailable,
      now,
    ]
  )

  // 没有网络或用户交互时，也及时按目录过期后的策略更新可见性。
  useEffect(() => {
    let nextExpiry = Infinity
    for (const model of models) {
      const entry = availability[model.model_name]
      const status = getCatalogAvailabilityStatus(entry, isUnavailable, now)
      if (status === 'sold_out' || status === 'disabled') {
        nextExpiry = Math.min(
          nextExpiry,
          Date.parse(entry.checked_at ?? '') + MAX_CATALOG_STATUS_AGE_MS + 1
        )
      }
      const seedanceEntry = seedanceAvailability[model.model_name]
      if (
        getCatalogAvailabilityStatus(
          seedanceEntry,
          isSeedanceUnavailable,
          now
        ) === 'available'
      ) {
        nextExpiry = Math.min(
          nextExpiry,
          Date.parse(seedanceEntry.checked_at ?? '') +
            MAX_CATALOG_STATUS_AGE_MS +
            1
        )
      }
    }
    if (!Number.isFinite(nextExpiry)) return
    const timer = window.setTimeout(refresh, Math.max(0, nextExpiry - now))
    return () => window.clearTimeout(timer)
  }, [
    models,
    availability,
    isUnavailable,
    seedanceAvailability,
    isSeedanceUnavailable,
    now,
  ])

  return visibleModels
}
