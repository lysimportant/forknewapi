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
import { CircleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatUseTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getFirstResponseTimeColor, getResponseTimeColor } from '../lib/format'
import {
  latencyTextColors,
  latencyBarColors,
  latencyBarFromColors,
  latencyBarToColors,
  type LatencyVariant,
} from '../lib/latency-health'
import type { LogOtherData } from '../types'

/** 使用日志耗时单元格的输入；总耗时为秒，首字为毫秒。 */
interface TimingMetricsCellProps {
  useTimeSec: number
  completionTokens: number
  frtMs?: number
  isStream: boolean
  className?: string
  /**
   * `bar` (default) draws a full-height color segment beside the labels,
   * matching the dense desktop table. `dot` swaps that segment for small
   * status dots inline with each label, matching the lighter-weight status
   * indicator used elsewhere on the mobile card.
   */
  indicator?: 'bar' | 'dot'
}

/** 展示首字与总耗时；文字、竖条和移动端状态点使用同一延迟档位。 */
export function TimingMetricsCell(props: TimingMetricsCellProps) {
  const { t } = useTranslation()
  const indicator = props.indicator ?? 'bar'
  const showFirstToken = props.isStream
  const firstTokenSeconds =
    props.frtMs != null && props.frtMs >= 0 ? props.frtMs / 1000 : null
  const firstTokenVariant: LatencyVariant | 'neutral' =
    firstTokenSeconds == null
      ? 'neutral'
      : getFirstResponseTimeColor(firstTokenSeconds)
  const totalTimeVariant = getResponseTimeColor(props.useTimeSec)
  const firstTokenLabel =
    firstTokenSeconds == null ? t('N/A') : formatUseTime(firstTokenSeconds)
  const totalTimeLabel = formatUseTime(props.useTimeSec)

  const labels = (
    <div className='flex min-h-8 min-w-0 flex-col justify-center gap-0.5 text-xs leading-tight'>
      {showFirstToken && (
        <div className='flex items-baseline gap-1.5'>
          {indicator === 'dot' && (
            <span
              aria-hidden
              className={cn(
                'size-1.5 shrink-0 rounded-full',
                latencyBarColors[firstTokenVariant]
              )}
            />
          )}
          <span className='text-muted-foreground shrink-0'>
            {t('First token')}
          </span>
          <span
            className={cn('tabular-nums', latencyTextColors[firstTokenVariant])}
          >
            {firstTokenLabel}
          </span>
        </div>
      )}
      <div className='flex items-baseline gap-1.5'>
        {indicator === 'dot' && (
          <span
            aria-hidden
            className={cn(
              'size-1.5 shrink-0 rounded-full',
              latencyBarColors[totalTimeVariant]
            )}
          />
        )}
        <span className='text-muted-foreground shrink-0'>{t('Duration')}</span>
        <span
          className={cn('tabular-nums', latencyTextColors[totalTimeVariant])}
        >
          {totalTimeLabel}
        </span>
      </div>
    </div>
  )

  if (indicator === 'dot') {
    return (
      <div className={cn('flex items-stretch', props.className)}>{labels}</div>
    )
  }

  return (
    <div className={cn('flex items-stretch gap-2', props.className)}>
      <span
        aria-hidden
        className={cn(
          'w-1 shrink-0 rounded-full',
          showFirstToken && firstTokenSeconds != null
            ? [
                'bg-linear-to-b from-40% to-60%',
                latencyBarFromColors[firstTokenVariant],
                latencyBarToColors[totalTimeVariant],
              ]
            : latencyBarColors[totalTimeVariant]
        )}
      />
      {labels}
    </div>
  )
}

interface StreamTpsCellProps {
  isStream: boolean
  /** Task logs are asynchronous jobs; stream vs non-stream does not apply. */
  isTask?: boolean
  tokensPerSecond?: number | null
  streamStatus?: LogOtherData['stream_status']
  className?: string
}

export function StreamTpsCell(props: StreamTpsCellProps) {
  const { t } = useTranslation()
  const showStreamError =
    props.isStream && props.streamStatus && props.streamStatus.status !== 'ok'
  const tpsLabel =
    props.tokensPerSecond != null
      ? `${Math.round(props.tokensPerSecond)} t/s`
      : '—'
  let streamLabel = props.isStream ? t('Stream') : t('Non-stream')
  if (props.isTask) {
    streamLabel = t('Async')
  }

  return (
    <div
      className={cn(
        'flex shrink-0 flex-col items-start justify-center gap-0.5 text-xs leading-tight',
        props.className
      )}
    >
      <span
        className={cn(
          'inline-flex items-center gap-1 font-medium',
          props.isStream ? 'text-info' : 'text-muted-foreground'
        )}
      >
        {streamLabel}
        {showStreamError && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger
                render={<CircleAlert className='text-destructive size-3' />}
              />
              <TooltipContent>
                <div className='space-y-0.5 text-xs'>
                  <p>
                    {t('Stream Status')}: {t('Error')}
                  </p>
                  <p>{props.streamStatus?.end_reason || 'unknown'}</p>
                  {(props.streamStatus?.error_count ?? 0) > 0 && (
                    <p>
                      {t('Soft Errors')}: {props.streamStatus?.error_count}
                    </p>
                  )}
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
      </span>
      <span className='text-muted-foreground/60 px-0.5 tabular-nums'>
        {tpsLabel}
      </span>
    </div>
  )
}
