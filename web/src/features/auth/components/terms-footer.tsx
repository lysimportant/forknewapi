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

import { LegalDocumentDialog } from '@/features/legal/legal-document-dialog'
import { cn } from '@/lib/utils'

import type { SystemStatus } from '../types'

/** 登录或注册页的补充文档入口；阅读入口本身不声明用户已同意。 */
interface TermsFooterProps {
  variant?: 'sign-in' | 'sign-up'
  className?: string
  status?: SystemStatus | null
}

/** 将管理员配置的协议保留为弹窗阅读入口，同意状态由登录表单单独管理。 */
export function TermsFooter(props: TermsFooterProps) {
  const { t } = useTranslation()
  const hasUserAgreement = Boolean(
    props.status?.user_agreement_enabled ??
    props.status?.data?.user_agreement_enabled
  )
  const hasPrivacyPolicy = Boolean(
    props.status?.privacy_policy_enabled ??
    props.status?.data?.privacy_policy_enabled
  )

  if (!hasUserAgreement && !hasPrivacyPolicy) return null

  return (
    <p
      className={cn(
        'text-muted-foreground text-center text-xs',
        props.className
      )}
    >
      {t('Related documents')}
      {': '}
      {hasUserAgreement && <LegalDocumentDialog document='user-agreement' />}
      {hasUserAgreement && hasPrivacyPolicy && ' · '}
      {hasPrivacyPolicy && <LegalDocumentDialog document='privacy-policy' />}
    </p>
  )
}
