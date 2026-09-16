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
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { RichContent } from '@/components/rich-content'
import { Skeleton } from '@/components/ui/skeleton'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'

import { getAboutContent } from './api'
import { AboutHelp } from './components/about-help'
import { AboutIntro } from './components/about-intro'
import { AboutPrivacySummary } from './components/about-privacy-summary'

function AboutSkeleton() {
  return (
    <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
      <Skeleton className='h-8 w-[45%]' />
      <Skeleton className='h-4 w-full' />
      <Skeleton className='h-4 w-[90%]' />
      <Skeleton className='h-4 w-[80%]' />
    </div>
  )
}

/**
 * 管理员配置的站点介绍。普通文本与 Markdown 都作为「站点介绍」正文渲染，
 * 因此单行内容也有完整版式；未配置时明确区分于请求失败。
 */
function SiteIntroduction(props: { content: string }) {
  const { t } = useTranslation()

  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-4xl'>
        <h2 className='mb-6 text-xl font-bold tracking-tight md:text-2xl'>
          {t('Site introduction')}
        </h2>
        {props.content ? (
          <div className='break-words'>
            <RichContent
              mode='markdown'
              content={props.content}
              className='prose-neutral dark:prose-invert max-w-none'
            />
          </div>
        ) : (
          <EmptyState
            icon={Info}
            bordered
            className='min-h-0'
            title={t('No site introduction yet')}
            description={t(
              'The administrator has not configured a site introduction yet. It can be set in the site settings.'
            )}
          />
        )}
      </div>
    </section>
  )
}

export function About() {
  const { t } = useTranslation()
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['about-content'],
    queryFn: getAboutContent,
  })

  const rawContent = data?.data?.trim() ?? ''
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isHttpUrl(rawContent)
  const contentIsHtml = hasContent && isLikelyHtml(rawContent)
  // 请求失败与管理员未配置必须分开：前者可重试，后者是正常空状态。
  const loadFailed = Boolean(error) || data?.success === false

  if (isLoading) {
    return (
      <PublicLayout showMainContainer={false}>
        <div className='px-6 pt-20'>
          <AboutSkeleton />
        </div>
      </PublicLayout>
    )
  }

  if (loadFailed) {
    return (
      <PublicLayout showMainContainer={false}>
        <div className='px-6 pt-20'>
          <ErrorState
            title={t('Failed to load the about page')}
            description={t(
              'The about content could not be loaded. Check your connection and retry. If it keeps failing, contact the administrator.'
            )}
            onRetry={() => {
              void refetch()
            }}
          />
        </div>
        <Footer />
      </PublicLayout>
    )
  }

  if (isUrl) {
    return (
      <PublicLayout showMainContainer={false}>
        <iframe
          src={rawContent}
          className='h-[calc(100vh-3.5rem)] w-full border-0'
          title={t('About')}
          sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
        />
        <Footer />
      </PublicLayout>
    )
  }

  if (contentIsHtml) {
    return (
      <PublicLayout showMainContainer={false}>
        <RichContent
          mode='html'
          htmlVariant='isolated'
          content={rawContent}
          className='prose-neutral dark:prose-invert max-w-none'
        />
        <Footer />
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <AboutIntro />
      <SiteIntroduction content={rawContent} />
      <AboutPrivacySummary />
      <AboutHelp />
      <Footer />
    </PublicLayout>
  )
}
