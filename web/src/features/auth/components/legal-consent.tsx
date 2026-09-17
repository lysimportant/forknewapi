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

import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { LegalDocumentDialog } from '@/features/legal/legal-document-dialog'
import { cn } from '@/lib/utils'

import { getLegalConsentRequirement } from '../lib/legal-consent'
import type { SystemStatus } from '../types'

interface LegalConsentProps {
  status: SystemStatus | null
  checked: boolean
  onCheckedChange: (nextValue: boolean) => void
  className?: string
  /** 协议要求仍在加载：勾选框可用，但调用方必须保持提交禁用 */
  statusLoading?: boolean
  /** 协议要求加载失败：提示刷新页面，调用方必须保持提交禁用 */
  statusError?: boolean
}

/**
 * 登录前《API 服务、隐私与使用责任协议》的强制勾选区域。
 *
 * 协议正文由站点内置并提供默认内容，因此勾选框始终渲染且默认未勾选；
 * 已配置的用户协议与隐私政策合并到同一勾选框，不再出现第二组 checkbox。
 */
export function LegalConsent(props: LegalConsentProps) {
  const { t } = useTranslation()
  const requirement = getLegalConsentRequirement(props.status)
  const hasUserAgreement = Boolean(
    props.status?.user_agreement_enabled ??
    props.status?.data?.user_agreement_enabled
  )
  const hasPrivacyPolicy = Boolean(
    props.status?.privacy_policy_enabled ??
    props.status?.data?.privacy_policy_enabled
  )
  const hasRelatedDocuments = hasUserAgreement || hasPrivacyPolicy

  const handleChange = (value: boolean) => {
    props.onCheckedChange(value === true)
  }

  let requirementHint: string | null = null
  if (!requirement.known) {
    // 加载中与加载失败给出不同文案，避免把请求失败误报成「正在加载」。
    const stillLoading = Boolean(props.statusLoading) && !props.statusError
    requirementHint = stillLoading
      ? t('Loading the agreement requirement…')
      : t(
          'The agreement requirement could not be loaded. Check your connection and reload the page.'
        )
  }

  return (
    <div
      className={cn(
        'border-border/60 bg-muted/40 flex items-start gap-3 rounded-md border p-3',
        props.className
      )}
    >
      <Checkbox
        id='legal-consent'
        checked={props.checked}
        onCheckedChange={handleChange}
        className='mt-0.5'
      />
      <div className='min-w-0 space-y-2'>
        <Label
          htmlFor='legal-consent'
          className='text-muted-foreground items-start gap-1 text-left text-xs leading-5 font-normal'
        >
          <span>
            {t('I have read and agree to the')}{' '}
            <LegalDocumentDialog document='api-service-agreement' />
            {t(
              ', and I undertake to use this platform’s API service in compliance with the law.'
            )}
          </span>
        </Label>

        {hasRelatedDocuments && (
          <p className='text-muted-foreground/80 text-xs leading-5'>
            {t('Related documents')}
            {': '}
            {hasUserAgreement && (
              <LegalDocumentDialog document='user-agreement' />
            )}
            {hasUserAgreement && hasPrivacyPolicy && ' · '}
            {hasPrivacyPolicy && (
              <LegalDocumentDialog document='privacy-policy' />
            )}
          </p>
        )}

        <p className='text-muted-foreground/80 text-xs leading-5'>
          {t(
            'This site only provides large-model API access services. It does not sell user data or personal information, and does not routinely monitor your specific use. You must use the service in compliance with the law and bear the corresponding legal responsibility for your own unlawful use.'
          )}
        </p>

        {requirementHint && (
          <p className='text-muted-foreground text-xs leading-5'>
            {requirementHint}
          </p>
        )}
      </div>
    </div>
  )
}
