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
import { useQuery } from '@tanstack/react-query'
import { memo, useMemo } from 'react'

import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import { useInfiniteModels } from '../hooks/use-infinite-models'
import type {
  PricingModel,
  TokenUnit,
  YuanliuAvailabilityMap,
  SeedanceAvailabilityMap,
} from '../types'
import { ModelCard } from './model-card'
import { ModelListFooter } from './model-list-footer'
import type { ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardGridProps {
  models: PricingModel[]
  onModelClick: (modelName: string) => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  yuanliuAvailability?: YuanliuAvailabilityMap
  isYuanliuAvailabilityUnavailable?: boolean
  seedanceAvailability?: SeedanceAvailabilityMap
  isSeedanceAvailabilityUnavailable?: boolean
}

export const ModelCardGrid = memo(function ModelCardGrid(
  props: ModelCardGridProps
) {
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const modelList = useInfiniteModels(props.models)

  const recentModels = modelList.visibleModels.map((model) => model.model_name || '')
  const perfQuery = useQuery({
    queryKey: ['perf-metrics-summary', 24, recentModels],
    queryFn: async () =>
      requireServerSuccess(await getPerfMetricsSummary(24, recentModels)),
    enabled: recentModels.length > 0,
    staleTime: 60 * 1000,
    retry: false,
  })

  const perfMap = useMemo(() => {
    const map = new Map<string, ModelPerfBadgeData>()
    for (const model of perfQuery.data?.data?.models ?? []) {
      map.set(model.model_name, {
        ...model,
        window_start: perfQuery.data?.data.window_start,
        window_end: perfQuery.data?.data.window_end,
      })
    }
    for (const [name, series] of Object.entries(
      perfQuery.data?.data?.recent_success_series ?? {}
    )) {
      map.set(name, {
        avg_latency_ms: Number.NaN,
        avg_tps: Number.NaN,
        success_rate: Number.NaN,
        ...map.get(name),
        recent_success_series: series,
      })
    }
    return map
  }, [perfQuery.data])

  if (props.models.length === 0) {
    return null
  }

  return (
    <div className='flex flex-col gap-4 sm:gap-5'>
      <div className='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 xl:grid-cols-3'>
        {modelList.visibleModels.map((model) => (
          <ModelCard
            key={model.id ?? model.model_name}
            model={model}
            tokenUnit={tokenUnit}
            priceRate={props.priceRate}
            usdExchangeRate={props.usdExchangeRate}
            showRechargePrice={props.showRechargePrice}
            selectedGroup={props.selectedGroup}
            yuanliuAvailability={props.yuanliuAvailability}
            seedanceAvailability={props.seedanceAvailability}
            isSeedanceAvailabilityUnavailable={
              props.isSeedanceAvailabilityUnavailable
            }
            isYuanliuAvailabilityUnavailable={
              props.isYuanliuAvailabilityUnavailable
            }
            perf={perfMap.get(model.model_name || '')}
            onClick={props.onModelClick}
          />
        ))}
      </div>

      <ModelListFooter
        count={modelList.visibleCount}
        total={props.models.length}
        hasMore={modelList.hasMore}
        onLoadMore={modelList.loadMore}
        sentinelRef={modelList.sentinelRef}
      />
    </div>
  )
})
