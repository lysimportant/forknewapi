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
import { LifeBuoy, MonitorUp, Wallet, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { useSystemConfig } from '@/hooks/use-system-config'

/** 站点原有运营说明；匹配此正文时直接使用整理后的版式，避免重复展示。 */
export const ORIGINAL_ABOUT_CONTENT =
  '充值可以找管理：hkcustom0928 不会接入Codex ChatGPT的也可以找管理远程帮忙; 有问题请联系管理员； Q扣群：811481586'

/** 以站点提供的充值、远程接入协助及联系方式组织关于页首屏，不推断联系账号所属平台。 */
export function AboutIntro() {
  const { t } = useTranslation()
  const { systemName, logo } = useSystemConfig()
  const services = [
    {
      icon: Wallet,
      title: t('Top-up assistance'),
      description: t('Contact the administrator for account top-ups.'),
      color: 'text-emerald-700 dark:text-emerald-400',
    },
    {
      icon: MonitorUp,
      title: t('Remote setup assistance'),
      description: t(
        'Need help connecting Codex or ChatGPT? Contact the administrator for remote setup assistance.'
      ),
      color: 'text-sky-700 dark:text-sky-400',
    },
    {
      icon: LifeBuoy,
      title: t('Questions and support'),
      description: t(
        'For questions during use, contact the administrator or join the QQ group.'
      ),
      color: 'text-rose-700 dark:text-rose-400',
    },
  ]

  return (
    <section className='px-6 pt-24 pb-12 md:pt-28 md:pb-16'>
      <div className='mx-auto max-w-5xl'>
        <div className='flex items-center gap-4'>
          <img src={logo} alt='' className='size-14 shrink-0 object-contain' />
          <div className='min-w-0'>
            <p className='text-muted-foreground mb-1 text-sm'>{t('About')}</p>
            <h1 className='text-3xl leading-tight font-semibold break-words'>
              {systemName}
            </h1>
          </div>
        </div>
        <p className='text-muted-foreground mt-6 max-w-2xl text-base leading-7'>
          {t(
            'Top-ups, remote setup, and help when you need it. Reach out to the administrator below.'
          )}
        </p>

        <div className='mt-9 grid gap-6 border-y py-7 md:grid-cols-3 md:gap-8'>
          {services.map((service) => (
            <div key={service.title} className='min-w-0'>
              <service.icon
                aria-hidden='true'
                className={`mb-4 size-6 ${service.color}`}
                strokeWidth={1.5}
              />
              <h2 className='text-base font-semibold'>{service.title}</h2>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                {service.description}
              </p>
            </div>
          ))}
        </div>

        <div className='mt-7 grid gap-6 md:grid-cols-2 md:gap-8'>
          <div className='flex min-w-0 items-center justify-between gap-4'>
            <div className='min-w-0'>
              <p className='text-muted-foreground text-sm'>
                {t('Administrator')}
              </p>
              <p className='mt-1 font-mono text-xl font-medium break-all'>
                hkcustom0928
              </p>
            </div>
            <CopyButton
              value='hkcustom0928'
              variant='outline'
              className='size-10'
              tooltip={t('Copy administrator account')}
            />
          </div>
          <div className='flex min-w-0 items-center justify-between gap-4 border-t pt-6 md:border-t-0 md:border-l md:pt-0 md:pl-8'>
            <div className='min-w-0'>
              <p className='text-muted-foreground flex items-center gap-2 text-sm'>
                <Users aria-hidden='true' className='size-4' />
                {t('QQ group')}
              </p>
              <p className='mt-1 font-mono text-xl font-medium break-all'>
                811481586
              </p>
            </div>
            <CopyButton
              value='811481586'
              variant='outline'
              className='size-10'
              tooltip={t('Copy QQ group number')}
            />
          </div>
        </div>
      </div>
    </section>
  )
}
