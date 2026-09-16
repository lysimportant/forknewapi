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
import {
  ArrowRight,
  BarChart3,
  BookOpen,
  KeyRound,
  Layers,
  Terminal,
  UserRound,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'

/**
 * 关于页的居中介绍版式：标题区、服务卡片与三步接入流程。
 * 文档与模型入口遵守 Header 导航开关与访问权限，未启用的入口不渲染。
 */
export function AboutIntro() {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()
  const { status } = useStatus()
  const topNavLinks = useTopNavLinks()

  // 文档链接可能是管理员配置的外部地址，也可能是站内 /docs。
  const docsLink = topNavLinks.find(
    (link) => link.external || link.href === '/docs'
  )
  const pricingLink = topNavLinks.find((link) => link.href === '/pricing')
  const registerEnabled =
    (status?.register_enabled ?? status?.data?.register_enabled ?? true) !==
    false

  const serviceCards: { key: string; icon: ReactNode; title: string; desc: string }[] =
    [
      {
        key: 'unified-api',
        icon: <Layers className='size-5' strokeWidth={1.5} />,
        title: t('Unified API access'),
        desc: t(
          'Reach the models this site has enabled through one endpoint. The exact protocols and available scope follow the documentation and the model list.'
        ),
      },
      {
        key: 'account-usage',
        icon: <BarChart3 className='size-5' strokeWidth={1.5} />,
        title: t('Account and usage management'),
        desc: t(
          'Manage API keys, and review balance, request history, and usage statistics.'
        ),
      },
      {
        key: 'onboarding',
        icon: <BookOpen className='size-5' strokeWidth={1.5} />,
        title: t('Clear onboarding guidance'),
        desc: t(
          'Finish configuration with the documentation, and follow announcements for maintenance and service changes.'
        ),
      },
    ]

  const steps = [
    {
      key: 'account',
      icon: <UserRound className='size-5' strokeWidth={1.5} />,
      title: t('Register or sign in'),
      desc: t('Create an account, or sign in to an existing one.'),
    },
    {
      key: 'api-key',
      icon: <KeyRound className='size-5' strokeWidth={1.5} />,
      title: t('Create an API key'),
      desc: t('Generate an API key in the console and keep it secure.'),
    },
    {
      key: 'request',
      icon: <Terminal className='size-5' strokeWidth={1.5} />,
      title: t('Configure and send requests'),
      desc: t(
        'Follow the documentation to configure your client, then start calling the API.'
      ),
    },
  ]

  return (
    <>
      <section className='relative z-10 overflow-hidden px-6 pt-24 pb-12 md:pt-32 md:pb-16'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 -z-10 opacity-20 dark:opacity-[0.12]'
          style={{
            background: [
              'radial-gradient(ellipse 60% 50% at 30% 10%, oklch(0.72 0.18 250 / 70%) 0%, transparent 70%)',
              'radial-gradient(ellipse 45% 40% at 80% 20%, oklch(0.70 0.12 280 / 50%) 0%, transparent 70%)',
            ].join(', '),
          }}
        />
        <div className='mx-auto max-w-3xl text-center'>
          <h1 className='text-[clamp(2rem,4vw,2.75rem)] leading-tight font-bold tracking-tight break-words'>
            {systemName}
          </h1>
          <p className='text-muted-foreground mt-3 text-base md:text-lg'>
            {t('Developer-facing API access service')}
          </p>
          <div className='mt-8 flex flex-wrap items-center justify-center gap-3'>
            {docsLink &&
              (docsLink.external ? (
                <Button
                  className='group h-11 rounded-lg px-5 text-sm font-medium'
                  render={
                    <a
                      href={docsLink.href}
                      target='_blank'
                      rel='noopener noreferrer'
                    />
                  }
                >
                  {t('View API docs')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
              ) : (
                <Button
                  className='group h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to={docsLink.href} />}
                >
                  {t('View API docs')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
              ))}
            {pricingLink &&
              (pricingLink.requiresAuth ? (
                <Button
                  variant='outline'
                  className='h-11 rounded-lg px-5 text-sm font-medium'
                  render={
                    <Link
                      to='/sign-in'
                      search={{ redirect: pricingLink.href }}
                    />
                  }
                >
                  {t('Browse models')}
                </Button>
              ) : (
                <Button
                  variant='outline'
                  className='h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to={pricingLink.href} />}
                >
                  {t('Browse models')}
                </Button>
              ))}
          </div>
        </div>
      </section>

      <section className='relative z-10 px-6 pb-16 md:pb-20'>
        <div className='mx-auto grid max-w-6xl gap-5 md:grid-cols-3'>
          {serviceCards.map((card) => (
            <Card key={card.key} className='h-full'>
              <CardContent className='space-y-3'>
                <div className='text-muted-foreground border-border/50 bg-muted/30 flex size-10 items-center justify-center rounded-xl border'>
                  {card.icon}
                </div>
                <h2 className='text-sm font-semibold'>{card.title}</h2>
                <p className='text-muted-foreground text-sm leading-relaxed'>
                  {card.desc}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      </section>

      <section className='border-border/40 relative z-10 border-t px-6 py-16 md:py-20'>
        <div className='mx-auto max-w-6xl'>
          <h2 className='text-center text-xl font-bold tracking-tight md:text-2xl'>
            {t('Get started in three steps')}
          </h2>
          <div className='mt-10 grid gap-8 md:grid-cols-3 md:gap-12'>
            {steps.map((step, index) => (
              <div
                key={step.key}
                className='relative flex flex-col items-center text-center'
              >
                <div className='relative mb-5'>
                  <div className='text-muted-foreground border-border/50 bg-muted/30 flex size-14 items-center justify-center rounded-2xl border'>
                    {step.icon}
                  </div>
                  <div className='bg-foreground text-background absolute -top-2 -right-2 flex size-6 items-center justify-center rounded-full text-xs font-bold'>
                    {index + 1}
                  </div>
                </div>
                <h3 className='mb-2 text-base font-semibold'>{step.title}</h3>
                <p className='text-muted-foreground max-w-[260px] text-sm leading-relaxed'>
                  {step.desc}
                </p>
              </div>
            ))}
          </div>
          {!registerEnabled && (
            <p className='text-muted-foreground mx-auto mt-10 max-w-2xl text-center text-sm'>
              {t(
                'Registration is currently disabled. Contact the administrator to obtain an account.'
              )}
            </p>
          )}
        </div>
      </section>
    </>
  )
}
