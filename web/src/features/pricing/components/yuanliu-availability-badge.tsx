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

import { isYuanliuModel } from '../lib/yuanliu-model'
import type {
  PricingModel,
  YuanliuAvailability,
  YuanliuAvailabilityMap,
} from '../types'

/** 目录供给状态对应的展示语义；unknown 不暗示模型可调用。 */
const STATUS_VARIANTS: Record<YuanliuAvailability['status'], StatusVariant> = {
  available: 'success',
  sold_out: 'warning',
  disabled: 'danger',
  unknown: 'neutral',
}

/** 缓存状态超过三分钟后不再作为当前供给情况展示。 */
const MAX_STATUS_AGE_MS = 3 * 60 * 1000

/** 只为源流插件模型显示供给状态；缺失、过期和请求失败均显示 unknown。 */
export function YuanliuAvailabilityBadge(props: {
  model: PricingModel
  availability: YuanliuAvailabilityMap
  isUnavailable?: boolean
  showCheckedAt?: boolean
  className?: string
}) {
  const { t, i18n } = useTranslation()
  if (!isYuanliuModel(props.model, props.availability)) return null

  const entry = props.availability[props.model.model_name]
  const checkedAtMs = entry?.checked_at
    ? Date.parse(entry.checked_at)
    : Number.NaN
  const now = Date.now()
  const isRecent =
    Number.isFinite(checkedAtMs) &&
    checkedAtMs <= now + 60 * 1000 &&
    now - checkedAtMs <= MAX_STATUS_AGE_MS
  const status =
    !props.isUnavailable &&
    isRecent &&
    entry?.status &&
    Object.hasOwn(STATUS_VARIANTS, entry.status)
      ? entry.status
      : 'unknown'
  const label = {
    available: t('Listed in catalog'),
    sold_out: t('Sold out'),
    disabled: t('Unlisted in catalog'),
    unknown: t('Unknown'),
  }[status]
  const checkedAt = Number.isFinite(checkedAtMs)
    ? new Date(checkedAtMs).toLocaleString(toIntlLocale(i18n.language))
    : t('Unknown')
  const checkedAtLabel = t('Catalog checked: {{time}}', { time: checkedAt })

  return (
    <span
      className={cn(
        'flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs',
        props.className
      )}
    >
      <span className='text-muted-foreground'>Yuanliu</span>
      <StatusBadge
        label={label}
        variant={STATUS_VARIANTS[status]}
        type='text'
        copyable={false}
        aria-label={`${t('Yuanliu catalog status')}: ${label}`}
        title={`${t('Based on Yuanliu catalog; generation is not guaranteed.')} ${checkedAtLabel}`}
      />
      {props.showCheckedAt && (
        <span className='text-muted-foreground'>{checkedAtLabel}</span>
      )}
    </span>
  )
}
