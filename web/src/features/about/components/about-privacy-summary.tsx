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
import { Eye, Scale, ShieldCheck, Server } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'

/**
 * 服务边界与隐私摘要：逐条引用登录页协议的既有承诺，并链接到协议全文。
 * 只陈述协议已写明的内容，不新增任何未经确认的隐私承诺。
 */
export function AboutPrivacySummary() {
  const { t } = useTranslation()

  const commitments = [
    {
      key: 'no-sale',
      icon: <ShieldCheck className='size-5' strokeWidth={1.5} />,
      title: t('We do not sell user data'),
      desc: t(
        'Personal information, account details, prompts, and model replies are not sold, rented, or provided to others.'
      ),
    },
    {
      key: 'api-only',
      icon: <Server className='size-5' strokeWidth={1.5} />,
      title: t('API service only'),
      desc: t(
        'The service only covers model API access, calling, and request forwarding. Available models, prices, and limits follow the actual configuration of this site.'
      ),
    },
    {
      key: 'monitoring',
      icon: <Eye className='size-5' strokeWidth={1.5} />,
      title: t('Monitoring boundary'),
      desc: t(
        'Your specific use is not actively tracked, and prompts and model replies are not routinely read or monitored by staff. Security checks and lawful handling stay within the scope the law and service safety require.'
      ),
    },
    {
      key: 'responsibility',
      icon: <Scale className='size-5' strokeWidth={1.5} />,
      title: t('Lawful use and user responsibility'),
      desc: t(
        'You must use the service in compliance with applicable laws and upstream service rules, and you bear the legal responsibility for your own unlawful use.'
      ),
    },
  ]

  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-6xl'>
        <div className='mx-auto max-w-2xl text-center'>
          <h2 className='text-xl font-bold tracking-tight md:text-2xl'>
            {t('Service scope and privacy')}
          </h2>
          <p className='text-muted-foreground mt-3 text-sm leading-relaxed'>
            {t(
              'This site provides API access and request forwarding services. Necessary account, billing, and security records are processed within the scope required to run the service.'
            )}
          </p>
        </div>

        <div className='mt-10 grid gap-5 md:grid-cols-2'>
          {commitments.map((item) => (
            <Card key={item.key} className='h-full'>
              <CardContent className='space-y-3'>
                <div className='text-muted-foreground border-border/50 bg-muted/30 flex size-10 items-center justify-center rounded-xl border'>
                  {item.icon}
                </div>
                <h3 className='text-sm font-semibold'>{item.title}</h3>
                <p className='text-muted-foreground text-sm leading-relaxed'>
                  {item.desc}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>

        <p className='mt-8 text-center text-sm'>
          <Link
            to='/api-service-agreement'
            className='text-primary hover:underline'
          >
            {t('Read the full agreement')}
          </Link>
        </p>
      </div>
    </section>
  )
}
