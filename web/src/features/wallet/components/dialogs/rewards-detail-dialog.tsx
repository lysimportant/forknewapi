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

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatQuota } from '@/lib/format'
import { formatDateTimeObject } from '@/lib/time'
import { cn } from '@/lib/utils'

import type { InviteRewardItem, InviteRewardSummary } from '../../types'

interface RewardsDetailDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  summary: InviteRewardSummary | null
}

/**
 * Map a reward status to the badge style used in the timeline.
 * Withdrawn entries stay visible for auditing, so they are labelled rather
 * than hidden.
 */
function rewardBadgeVariant(
  status: InviteRewardItem['status']
): 'default' | 'secondary' | 'warning' {
  if (status === 'available') return 'default'
  if (status === 'frozen') return 'warning'
  return 'secondary'
}

/** 展示服务端返回的奖励金额、冻结到期时间和提现状态，历史奖励按各自到期时间显示。 */
export function RewardsDetailDialog(props: RewardsDetailDialogProps) {
  const { t } = useTranslation()
  const rewards = props.summary?.rewards ?? []

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Reward Details')}
      description={t(
        'Each commission is frozen for 48 hours before withdrawal to your in-site balance.'
      )}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-2xl'
      titleClassName='text-xl font-semibold'
      contentHeight='auto'
      footer={
        <Button variant='outline' onClick={() => props.onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      {rewards.length === 0 ? (
        <EmptyState
          title={t('No rewards yet')}
          description={t(
            'Rewards appear after invited users top up or redeem a code.'
          )}
        />
      ) : (
        <ol className='space-y-3'>
          {rewards.map((reward: InviteRewardItem) => {
            const pending = reward.remaining_quota > 0
            return (
              <li key={reward.id} className='flex gap-3'>
                <div
                  className={cn(
                    'mt-1.5 size-2 shrink-0 rounded-full',
                    reward.status === 'available' && 'bg-primary',
                    reward.status === 'frozen' && 'bg-warning',
                    reward.status === 'withdrawn' && 'bg-muted-foreground/40'
                  )}
                  aria-hidden='true'
                />
                <div className='border-border/60 min-w-0 flex-1 border-l pb-3 pl-3'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <span className='text-sm font-semibold tabular-nums'>
                      {formatQuota(reward.quota)}
                    </span>
                    <Badge variant={rewardBadgeVariant(reward.status)}>
                      {reward.status === 'available' && t('Withdrawable')}
                      {reward.status === 'frozen' && t('Frozen')}
                      {reward.status === 'withdrawn' && t('Withdrawn')}
                    </Badge>
                  </div>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('Earned at')}{' '}
                    {formatDateTimeObject(new Date(reward.created_at * 1000))}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {reward.status === 'frozen'
                      ? `${t('Withdrawable after')} ${formatDateTimeObject(new Date(reward.next_available_at * 1000))}`
                      : `${t('Withdrawable since')} ${formatDateTimeObject(new Date(reward.next_available_at * 1000))}`}
                  </p>
                  {pending && reward.remaining_quota !== reward.quota ? (
                    <p className='text-muted-foreground text-xs'>
                      {t('Remaining')}: {formatQuota(reward.remaining_quota)}
                    </p>
                  ) : null}
                </div>
              </li>
            )
          })}
        </ol>
      )}
    </Dialog>
  )
}
