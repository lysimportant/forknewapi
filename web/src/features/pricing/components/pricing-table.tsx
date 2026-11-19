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
import type { Row } from '@tanstack/react-table'
import { Fragment, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTableRow,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { TableCell, TableRow } from '@/components/ui/table'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import { useInfiniteModels } from '../hooks/use-infinite-models'
import {
  getVideoPluginSections,
  type VideoPluginSection,
} from '../lib/video-plugin-sections'
import type {
  PricingModel,
  TokenUnit,
  YuanliuAvailabilityMap,
  SeedanceAvailabilityMap,
} from '../types'
import { ModelListFooter } from './model-list-footer'
import { usePricingColumns } from './pricing-columns'

export interface PricingTableProps {
  models: PricingModel[]
  isLoading?: boolean
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  groupByVideoPlugin?: boolean
  onModelClick?: (modelName: string) => void
  yuanliuAvailability?: YuanliuAvailabilityMap
  isYuanliuAvailabilityUnavailable?: boolean
  seedanceAvailability?: SeedanceAvailabilityMap
  isSeedanceAvailabilityUnavailable?: boolean
}

export function PricingTable(props: PricingTableProps) {
  const { t } = useTranslation()
  const {
    models,
    isLoading = false,
    priceRate = 1,
    usdExchangeRate = 1,
    tokenUnit = DEFAULT_TOKEN_UNIT,
    showRechargePrice = false,
    selectedGroup,
    groupByVideoPlugin,
    yuanliuAvailability,
    isYuanliuAvailabilityUnavailable,
    seedanceAvailability,
    isSeedanceAvailabilityUnavailable,
    onModelClick,
  } = props

  const modelList = useInfiniteModels(models, !isLoading)
  const groupedRows = useMemo(() => {
    if (!groupByVideoPlugin) return null

    const sections = getVideoPluginSections(
      modelList.visibleModels,
      selectedGroup
    )
    if (!sections) return null

    const sectionStarts = new Map<number, VideoPluginSection>()
    let rowIndex = 0
    for (const section of sections) {
      sectionStarts.set(rowIndex, section)
      rowIndex += section.models.length
    }
    return {
      models: sections.flatMap((section) => section.models),
      sectionStarts,
    }
  }, [groupByVideoPlugin, modelList.visibleModels, selectedGroup])

  const columns = usePricingColumns({
    tokenUnit,
    priceRate,
    usdExchangeRate,
    showRechargePrice,
    selectedGroup,
    yuanliuAvailability,
    isYuanliuAvailabilityUnavailable,
    seedanceAvailability,
    isSeedanceAvailabilityUnavailable,
  })

  const { table } = useDataTable({
    data: groupedRows?.models ?? modelList.visibleModels,
    columns,
    manualPagination: false,
    withPaginationRowModel: false,
    withFilteredRowModel: false,
    withSortedRowModel: false,
    withFacetedRowModel: false,
  })

  const handleRowClick = useCallback(
    (model: PricingModel) => {
      onModelClick?.(model.model_name)
    },
    [onModelClick]
  )

  return (
    <div className='space-y-4'>
      <DataTableView
        table={table}
        isLoading={isLoading}
        emptyTitle={t('No Models Found')}
        emptyDescription={t('No models match your current filters.')}
        skeletonKeyPrefix='pricing-skeleton'
        applyHeaderSize
        getColumnClassName={(_columnId, kind) =>
          kind === 'header' ? 'text-muted-foreground font-medium' : undefined
        }
        renderRow={(row: Row<PricingModel>) => {
          const section = groupedRows?.sectionStarts.get(row.index)
          return (
            <Fragment key={groupedRows ? row.original.model_name : row.id}>
              {section && (
                <TableRow className='bg-muted/40 h-9!'>
                  <TableCell colSpan={table.getVisibleLeafColumns().length}>
                    <h2
                      className='font-semibold'
                      aria-label={section.plugin?.name}
                    >
                      {section.plugin?.title ?? t('Other')}
                    </h2>
                  </TableCell>
                </TableRow>
              )}
              <DataTableRow
                row={row}
                cellRenderColumns={columns}
                className='hover:bg-muted/30 cursor-pointer transition-colors'
                onClick={() => handleRowClick(row.original)}
              />
            </Fragment>
          )
        }}
      />

      {!isLoading && models.length > 0 && (
        <ModelListFooter
          count={modelList.visibleCount}
          total={models.length}
          hasMore={modelList.hasMore}
          onLoadMore={modelList.loadMore}
          sentinelRef={modelList.sentinelRef}
        />
      )}
    </div>
  )
}
