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

import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { taskPriceLabel } from '../lib/task-price-display'
import type { TaskSurcharge } from '../lib/task-surcharge'
import type { BillingUsageSchema } from '../types'

export type TaskSurchargeNoteProps = {
  surcharge: TaskSurcharge
  usageSchema?: BillingUsageSchema
  className?: string
}

export function TaskSurchargeNote(props: TaskSurchargeNoteProps) {
  const { t, i18n } = useTranslation()
  const locales = toIntlLocale(i18n.language)
  const definition = props.usageSchema?.[props.surcharge.field]
  const fieldLabel = taskPriceLabel(
    definition?.description,
    props.surcharge.field,
    i18n.language
  )
  const operator =
    props.surcharge.operator === '>='
      ? t('Greater than or equal to')
      : t('Greater than')

  return (
    <p
      className={cn(
        'text-muted-foreground text-xs break-words whitespace-normal',
        props.className
      )}
    >
      {t(
        'Usage surcharge: ×{{multiplier}} when {{field}} {{operator}} {{threshold}}',
        {
          multiplier: formatNumber(props.surcharge.multiplier, locales),
          field: fieldLabel,
          operator,
          threshold: formatNumber(props.surcharge.threshold, locales),
        }
      )}
    </p>
  )
}
