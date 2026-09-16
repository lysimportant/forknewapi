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
import { Link } from '@tanstack/react-router'
import { BookOpen, LifeBuoy, Megaphone } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { AnnouncementTimelineDialog } from '@/features/announcements/components/announcement-timeline-dialog'
import type { AnnouncementEntry } from '@/features/announcements/types'
import { useStatus } from '@/hooks/use-status'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { useNotificationStore } from '@/stores/notification-store'

type HelpItem = {
  key: string
  icon: ReactNode
  title: string
  description?: string
  action?: ReactNode
}

/**
 * 帮助区域：已配置的文档入口、系统公告与已配置的支持联系方式。
 * 未配置的条目直接隐藏，不编造客服地址或联系方式。
 */
export function AboutHelp() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const topNavLinks = useTopNavLinks()
  const [announcementsOpen, setAnnouncementsOpen] = useState(false)
  const closeAnnouncementsForToday = useNotificationStore(
    (state) => state.closeAnnouncementsForToday
  )
  const storageAvailable = useNotificationStore(
    (state) => state.storageAvailable
  )

  const docsLink = topNavLinks.find(
    (link) => link.external || link.href === '/docs'
  )
  const announcementsEnabled = Boolean(
    status?.announcements_enabled ?? status?.data?.announcements_enabled
  )
  const announcements = Array.isArray(status?.announcements)
    ? (status.announcements as AnnouncementEntry[])
    : []
  const supportContact =
    typeof status?.support_contact === 'string'
      ? status.support_contact.trim()
      : ''
  const supportContactIsLink = /^(https?:\/\/|mailto:)/i.test(supportContact)

  const items: HelpItem[] = []

  if (docsLink) {
    items.push({
      key: 'docs',
      icon: <BookOpen className='size-5' strokeWidth={1.5} />,
      title: t('Documentation'),
      description: t(
        'Read the access guide, protocols, and configuration examples.'
      ),
      action: (
        <Button
          variant='outline'
          size='sm'
          render={
            docsLink.external ? (
              <a
                href={docsLink.href}
                target='_blank'
                rel='noopener noreferrer'
              />
            ) : (
              <Link to={docsLink.href} />
            )
          }
        >
          {t('View API docs')}
        </Button>
      ),
    })
  }

  if (announcementsEnabled && announcements.length > 0) {
    items.push({
      key: 'announcements',
      icon: <Megaphone className='size-5' strokeWidth={1.5} />,
      title: t('Announcements'),
      description: t('Review maintenance windows and service changes.'),
      action: (
        <Button
          variant='outline'
          size='sm'
          onClick={() => setAnnouncementsOpen(true)}
        >
          {t('View announcements')}
        </Button>
      ),
    })
  }

  if (supportContact) {
    items.push({
      key: 'contact',
      icon: <LifeBuoy className='size-5' strokeWidth={1.5} />,
      title: t('Support contact'),
      description: supportContactIsLink ? supportContact : undefined,
      action: supportContactIsLink ? (
        <Button
          variant='outline'
          size='sm'
          render={
            <a
              href={supportContact}
              target='_blank'
              rel='noopener noreferrer'
            />
          }
        >
          {t('Contact support')}
        </Button>
      ) : (
        <span className='text-muted-foreground text-sm break-words'>
          {supportContact}
        </span>
      ),
    })
  }

  if (items.length === 0) return null

  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-6xl'>
        <h2 className='text-center text-xl font-bold tracking-tight md:text-2xl'>
          {t('Help')}
        </h2>
        <div className='mt-10 grid gap-5 md:grid-cols-3'>
          {items.map((item) => (
            <div
              key={item.key}
              className='border-border/40 bg-muted/10 flex h-full flex-col gap-3 rounded-xl border p-6'
            >
              <div className='text-muted-foreground border-border/50 bg-muted/30 flex size-10 items-center justify-center rounded-xl border'>
                {item.icon}
              </div>
              <h3 className='text-sm font-semibold'>{item.title}</h3>
              {item.description && (
                <p className='text-muted-foreground text-sm leading-relaxed break-words'>
                  {item.description}
                </p>
              )}
              {item.action && <div className='mt-auto pt-2'>{item.action}</div>}
            </div>
          ))}
        </div>
      </div>

      <AnnouncementTimelineDialog
        open={announcementsOpen}
        onOpenChange={setAnnouncementsOpen}
        announcements={announcements}
        onCloseForToday={() => {
          closeAnnouncementsForToday()
          setAnnouncementsOpen(false)
        }}
        closeTodayPersists={storageAvailable}
      />
    </section>
  )
}
