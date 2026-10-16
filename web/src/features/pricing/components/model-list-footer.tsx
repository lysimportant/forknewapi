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
import type { Ref } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

interface ModelListFooterProps {
  count: number
  total: number
  hasMore: boolean
  onLoadMore: () => void
  sentinelRef: Ref<HTMLDivElement>
}

export function ModelListFooter(props: ModelListFooterProps) {
  const { t } = useTranslation()

  return (
    <div
      ref={props.sentinelRef}
      className='text-muted-foreground flex flex-col items-center justify-between gap-3 border-t px-4 py-3 text-sm sm:flex-row'
    >
      <p role='status'>
        {t('Showing {{count}} of {{total}} models', {
          count: props.count,
          total: props.total,
        })}
      </p>
      {props.hasMore ? (
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={props.onLoadMore}
        >
          {t('Load more models')}
        </Button>
      ) : (
        <span>{t('All models loaded')}</span>
      )}
    </div>
  )
}
