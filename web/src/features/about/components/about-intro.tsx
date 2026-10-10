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
import { Code2, Images, PanelsTopLeft, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { buttonVariants } from '@/components/ui/button'
import { useSystemConfig } from '@/hooks/use-system-config'
import { DEFAULT_LOGO } from '@/lib/constants'
import { cn } from '@/lib/utils'

/** 仅用于识别并隐藏旧运营正文，避免重复展示已过期的联系方式。 */
export const ORIGINAL_ABOUT_CONTENT =
  '充值可以找管理：hkcustom0928 不会接入Codex ChatGPT的也可以找管理远程帮忙; 有问题请联系管理员； Q扣群：811481586'

/** 展示平台定位、能力边界及站点已经公开的支持联系方式。 */
export function AboutIntro() {
  const { t } = useTranslation()
  const { systemName, logo } = useSystemConfig()
  const isDefaultLogo = logo === DEFAULT_LOGO
  const heroImage = isDefaultLogo ? '/mansui-whale.webp' : logo
  const services = [
    {
      icon: Code2,
      title: t('Unified API access'),
      description: t(
        'Connect text, image, and video workflows through one API surface.'
      ),
      color: 'text-sky-700 dark:text-sky-400',
    },
    {
      icon: Images,
      title: t('AI images and video'),
      description: t(
        'Explore image and video creation through adapted H3 and Seed family protocols.'
      ),
      color: 'text-sky-700 dark:text-sky-400',
    },
    {
      icon: PanelsTopLeft,
      title: t('Creative canvas'),
      description: t('Continue visual creation in the connected canvas.'),
      color: 'text-cyan-700 dark:text-cyan-400',
    },
  ]

  return (
    <section className='px-6 pt-24 pb-12 md:pt-28 md:pb-16'>
      <div className='mx-auto max-w-5xl'>
        <div className='grid items-center gap-8 lg:grid-cols-[1fr_18rem]'>
          <div>
            <div className='flex items-center gap-4'>
              <img
                src={logo}
                alt={systemName}
                className='size-14 shrink-0 rounded-2xl object-cover'
              />
              <div className='min-w-0'>
                <p className='text-muted-foreground mb-1 text-sm'>
                  {t('About')}
                </p>
                <h1 className='text-3xl leading-tight font-semibold break-words'>
                  {systemName}
                </h1>
              </div>
            </div>
            <p className='text-muted-foreground mt-6 max-w-2xl text-base leading-7'>
              {t(
                'Connect text, image, and video workflows through one API surface.'
              )}{' '}
              {t('Continue visual creation in the connected canvas.')}
            </p>
            <div className='mt-6 flex flex-wrap gap-3'>
              <Link
                to='/pricing'
                className={cn(
                  buttonVariants(),
                  'bg-sky-700 text-white hover:bg-sky-800 dark:bg-sky-400 dark:text-slate-950 dark:hover:bg-sky-300'
                )}
              >
                {t('Browse available models and pricing')}
              </Link>
              <a
                href='https://love.lolicon.beer'
                target='_blank'
                rel='noopener noreferrer'
                className={cn(buttonVariants({ variant: 'outline' }))}
              >
                {t('Open creative canvas')}
              </a>
            </div>
          </div>
          <div className='mx-auto w-full max-w-72'>
            <img
              src={heroImage}
              alt={
                isDefaultLogo
                  ? t(
                      "Dafeiyu (大肥鱼), a community-created blue-haired whale girl personification of DeepSeek AI, used as this site's avatar"
                    )
                  : systemName
              }
              className='border-border/50 mx-auto aspect-square w-full rounded-[2rem] border object-cover shadow-sm'
            />
            {isDefaultLogo && (
              <p className='text-muted-foreground mt-3 text-center text-xs leading-5'>
                {t(
                  "Dafeiyu (大肥鱼) is a community-created blue-haired whale girl personification of DeepSeek AI and this site's avatar."
                )}
              </p>
            )}
          </div>
        </div>

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

        <p className='text-muted-foreground mt-6 text-sm leading-6'>
          {t(
            'Availability and pricing follow the current model catalog, channel configuration, and upstream permissions.'
          )}
        </p>

        <div className='mt-10'>
          <h2 className='text-lg font-semibold'>
            {t('Questions and support')}
          </h2>
          <p className='text-muted-foreground mt-2 text-sm leading-6'>
            {t(
              'Top-ups, remote setup, and help when you need it. Reach out to the administrator below.'
            )}
          </p>
        </div>
        <div className='mt-6 grid gap-6 md:grid-cols-2 md:gap-8'>
          <div className='flex min-w-0 items-center justify-between gap-4'>
            <div className='min-w-0'>
              <p className='text-muted-foreground text-sm'>
                {t('Administrator')}
              </p>
              <p className='mt-1 font-mono text-xl font-medium break-all'>
                518229879
              </p>
            </div>
            <CopyButton
              value='518229879'
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
                518229879
              </p>
            </div>
            <CopyButton
              value='518229879'
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
