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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { RichContent } from '@/components/rich-content'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatAnnouncementRelativeTime } from '@/lib/announcement-time'
import { getAnnouncementColorClass } from '@/lib/colors'
import { formatDateTimeObject } from '@/lib/time'
import { cn } from '@/lib/utils'

import { sortAnnouncements } from '../lib/announcement-sort'
import type { AnnouncementEntry } from '../types'

type AnnouncementTimelineDialogProps = {
  open: boolean
  /** 关闭事件：右上角 ×、Esc、点击遮罩与「关闭公告」都走这里 */
  onOpenChange: (open: boolean) => void
  announcements: AnnouncementEntry[]
  onCloseForToday: () => void
  /** 本地存储不可写时为 false，提示「今日关闭」无法跨刷新保存 */
  closeTodayPersists?: boolean
}

/**
 * 系统公告时间轴弹窗：展示全部已配置公告，仅正文区域滚动
 */
export function AnnouncementTimelineDialog(
  props: AnnouncementTimelineDialogProps
) {
  const { t } = useTranslation()
  // 弹窗必须展示全部公告，并按置顶优先 → 发布时间倒序排列
  const timeline = useMemo(
    () => sortAnnouncements(props.announcements),
    [props.announcements]
  )

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('System Announcements')}
      description={t('Latest platform updates and notices')}
      contentClassName='sm:max-w-[720px] max-h-[80vh]'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Close Announcements')}
          </Button>
          <Button type='button' onClick={props.onCloseForToday}>
            {t('Close Today')}
          </Button>
        </>
      }
    >
      {props.closeTodayPersists === false ? (
        <p className='text-warning text-xs'>
          {t(
            'Close Today cannot be saved in this browser; announcements will show again after refresh.'
          )}
        </p>
      ) : null}

      {timeline.length === 0 ? (
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t('No system announcements')}
        </p>
      ) : (
        <ol className='relative flex flex-col gap-6'>
          <span
            aria-hidden='true'
            className='bg-border absolute top-2 bottom-2 left-[5px] w-px'
          />
          {timeline.map((item, index) => {
            const publishDate = item.publishDate
              ? new Date(item.publishDate)
              : null
            const hasPublishTime =
              publishDate !== null && !Number.isNaN(publishDate.getTime())

            return (
              <li
                key={item.id ?? `announcement-${index}`}
                className='relative pl-6'
              >
                <span
                  aria-hidden='true'
                  className={cn(
                    'ring-background absolute top-1 left-0 size-3 rounded-full ring-4',
                    getAnnouncementColorClass(item.type)
                  )}
                />
                <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
                  {item.pinned ? (
                    <Badge variant='warning'>{t('Pinned')}</Badge>
                  ) : null}
                  {hasPublishTime ? (
                    <>
                      <span className='text-sm font-medium'>
                        {formatAnnouncementRelativeTime(publishDate, t)}
                      </span>
                      <time
                        dateTime={publishDate.toISOString()}
                        className='text-muted-foreground text-xs'
                      >
                        {formatDateTimeObject(publishDate)}
                      </time>
                    </>
                  ) : null}
                </div>

                <div className='mt-2 text-sm'>
                  <RichContent
                    mode='markdown'
                    breaks
                    content={item.content ?? ''}
                  />
                </div>

                {item.extra ? (
                  <div className='text-muted-foreground mt-1.5 text-xs'>
                    <RichContent mode='markdown' breaks content={item.extra} />
                  </div>
                ) : null}
              </li>
            )
          })}
        </ol>
      )}
    </Dialog>
  )
}
