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
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import { getCatalogAvailabilityStatus } from '../lib/catalog-availability'
import { isSeedanceModel } from '../lib/seedance-model'
import { isYuanliuModel } from '../lib/yuanliu-model'
import type {
  CatalogAvailabilityMap,
  PricingModel,
  YuanliuAvailability,
} from '../types'

/** 目录供给状态对应的展示语义；unknown 不暗示模型可调用。 */
const STATUS_VARIANTS: Record<YuanliuAvailability['status'], StatusVariant> = {
  available: 'success',
  sold_out: 'warning',
  disabled: 'danger',
  unknown: 'neutral',
}

type AvailabilityBadgeProps = {
  model: PricingModel
  availability: CatalogAvailabilityMap
  isUnavailable?: boolean
  showCheckedAt?: boolean
  className?: string
}

export function YuanliuAvailabilityBadge(props: AvailabilityBadgeProps) {
  if (!isYuanliuModel(props.model, props.availability)) return null
  return <CatalogAvailabilityBadge {...props} provider='Yuanliu' />
}

export function SeedanceAvailabilityBadge(props: AvailabilityBadgeProps) {
  if (!isSeedanceModel(props.model, props.availability)) return null
  return <CatalogAvailabilityBadge {...props} provider='Seedance' />
}

/** 缺少查询结果、状态过期或请求失败均显示 unknown。 */
function CatalogAvailabilityBadge(
  props: AvailabilityBadgeProps & {
    provider: 'Yuanliu' | 'Seedance'
  }
) {
  const { t, i18n } = useTranslation()

  const entry = props.availability[props.model.model_name]
  const checkedAtMs = entry?.checked_at
    ? Date.parse(entry.checked_at)
    : Number.NaN
  const status = getCatalogAvailabilityStatus(entry, props.isUnavailable)
  const label = {
    available: t('Listed in catalog'),
    sold_out: t('Sold out'),
    disabled: t('Upstream temporarily unavailable'),
    unknown: t('Unknown'),
  }[status]
  const checkedAt = Number.isFinite(checkedAtMs)
    ? new Date(checkedAtMs).toLocaleString(toIntlLocale(i18n.language))
    : t('Unknown')
  const checkedAtLabel = t('Catalog checked: {{time}}', { time: checkedAt })
  const catalogLabel =
    props.provider === 'Seedance'
      ? t('Seedance catalog status')
      : t('Yuanliu catalog status')
  const catalogHint =
    props.provider === 'Seedance'
      ? t('Based on Seedance catalog; generation is not guaranteed.')
      : t('Based on Yuanliu catalog; generation is not guaranteed.')

  return (
    <span
      className={cn(
        'flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs',
        props.className
      )}
    >
      <span className='text-muted-foreground'>{props.provider}</span>
      <StatusBadge
        label={label}
        variant={STATUS_VARIANTS[status]}
        type='text'
        copyable={false}
        aria-label={`${catalogLabel}: ${label}`}
        title={`${catalogHint} ${checkedAtLabel}`}
      />
      {props.showCheckedAt && (
        <span className='text-muted-foreground'>{checkedAtLabel}</span>
      )}
    </span>
  )
}
