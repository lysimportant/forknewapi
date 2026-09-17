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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'
import { cn } from '@/lib/utils'

import {
  getAPIServiceAgreement,
  getPrivacyPolicy,
  getUserAgreement,
} from './api'

/** 协议种类与现有公开正文接口对应，不在客户端另存协议正文。 */
const documents = {
  'api-service-agreement': {
    title: 'API Service, Privacy and Usage Responsibility Agreement',
    fetch: getAPIServiceAgreement,
  },
  'user-agreement': { title: 'User Agreement', fetch: getUserAgreement },
  'privacy-policy': { title: 'Privacy Policy', fetch: getPrivacyPolicy },
} as const

/** 阅读入口的文档种类和可选样式；打开或关闭均不代表同意协议。 */
interface LegalDocumentDialogProps {
  document: keyof typeof documents
  className?: string
  /** 可选入口文案；省略时使用协议标题，不影响弹窗标题。 */
  children?: ReactNode
}

/** 在当前页面按需加载协议；复用站点 Dialog 的滚动、键盘和焦点恢复行为。 */
export function LegalDocumentDialog(props: LegalDocumentDialogProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const document = documents[props.document]
  const title = t(document.title)
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: [props.document],
    queryFn: document.fetch,
    enabled: open,
    staleTime: 0,
    retry: false,
  })
  const content = data?.data?.trim() ?? ''
  let body: ReactNode
  if (isLoading) {
    body = (
      <div
        role='status'
        aria-label={t('Loading...')}
        className='space-y-3 py-4'
      >
        <Skeleton className='h-5 w-2/3' />
        <Skeleton className='h-4 w-full' />
        <Skeleton className='h-4 w-5/6' />
      </div>
    )
  } else if (error || !data?.success || !content) {
    body = (
      <ErrorState
        title={t('The agreement text is currently unavailable.')}
        onRetry={() => void refetch()}
      />
    )
  } else if (isHttpUrl(content)) {
    body = (
      <iframe
        title={title}
        src={content}
        className='h-[50vh] w-full border-0'
        sandbox='allow-scripts'
        referrerPolicy='no-referrer'
      />
    )
  } else {
    body = (
      <RichContent
        mode={isLikelyHtml(content) ? 'html' : 'markdown'}
        htmlVariant='isolated'
        content={content}
        className='prose-neutral dark:prose-invert max-w-none'
      />
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      title={title}
      contentClassName='sm:max-w-[720px] max-h-[80vh]'
      bodyClassName='break-words'
      trigger={
        <Button
          type='button'
          variant='link'
          className={cn(
            'text-primary inline h-auto whitespace-normal p-0 text-left text-xs leading-[inherit]',
            props.className
          )}
          onClick={(event) => event.stopPropagation()}
        >
          {props.children ?? title}
        </Button>
      }
      footer={
        <Button type='button' variant='outline' onClick={() => setOpen(false)}>
          {t('Close')}
        </Button>
      }
    >
      {body}
    </Dialog>
  )
}
