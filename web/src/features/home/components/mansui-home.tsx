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
import {
  ApiIcon,
  ArrowRight02Icon,
  Coins01Icon,
  Image02Icon,
  InformationCircleIcon,
  Layers01Icon,
  LinkSquare01Icon,
  MagicWand01Icon,
  PaintBoardIcon,
  PauseIcon,
  PlayIcon,
  Video01Icon,
  WorkflowCircle06Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

/** 默认主页输入，登录状态仅用于调整创作引导文案。 */
interface MansuiHomeProps {
  isAuthenticated: boolean
}

/** 模型展示资料；id 保留精确调用名称，展示本身不代表当前渠道可用。 */
interface ModelFact {
  name: string
  id: string
  fact: string
  kind: 'image' | 'text' | 'video'
}

/** 已有适配器中的模型示例，实际开放范围由模型目录决定。 */
const MODEL_FACTS: ModelFact[] = [
  {
    name: 'DeepSeek Chat',
    id: 'deepseek-chat',
    fact: 'Chat and text generation for general-purpose AI workflows.',
    kind: 'text',
  },
  {
    name: 'GPT-4.1',
    id: 'gpt-4.1',
    fact: 'Text and code generation for instruction-driven workflows.',
    kind: 'text',
  },
  {
    name: 'MiniMax H3',
    id: 'MiniMax-H3',
    fact: 'Create videos from text, images, and multimodal references.',
    kind: 'video',
  },
  {
    name: 'Seedream 4.0',
    id: 'doubao-seedream-4-0-250828',
    fact: 'Create and edit images through the image generation API.',
    kind: 'image',
  },
  {
    name: 'Seedance 2.5',
    id: 'doubao-seedance-2-5-260628',
    fact: 'Create videos from text, images, and video references.',
    kind: 'video',
  },
]

/** 平台接入、Token 管理和漫剧创作的展示内容。 */
const CAPABILITIES = [
  {
    icon: ApiIcon,
    eyebrow: 'One endpoint',
    title: 'Unified AI access',
    description:
      'Connect text, image, and video workflows through a consistent API surface.',
  },
  {
    icon: Coins01Icon,
    eyebrow: 'Clear control',
    title: 'Token market',
    description:
      'Compare model choices and manage creative workloads from one account.',
  },
  {
    icon: PaintBoardIcon,
    eyebrow: 'Creator first',
    title: 'Comics creation',
    description:
      'Turn scripts, panels, references, and motion ideas into connected workflows.',
  },
]

/** 画布创作示例中的故事、图像和动态生成步骤。 */
const WORKFLOW_STEPS = [
  {
    index: '01',
    title: 'Write the story beat',
    description:
      'Start with a scene, character intent, or panel-by-panel outline.',
  },
  {
    index: '02',
    title: 'Shape the visual language',
    description:
      'Use image models and references to explore composition and style.',
  },
  {
    index: '03',
    title: 'Bring the sequence to life',
    description:
      'Connect video generation and iterate without leaving the canvas.',
  },
]

/** 面向新用户的接入、选模和迭代指引。 */
const START_STEPS = [
  {
    number: '01',
    title: 'Choose your entrance',
    description:
      'Open API pricing for integration, or enter the canvas for creation.',
  },
  {
    number: '02',
    title: 'Select the right model',
    description:
      'Match text, image, or video capabilities to the task you want to run.',
  },
  {
    number: '03',
    title: 'Build and refine',
    description:
      'Keep prompts, references, outputs, and revisions in one flow.',
  },
]

/** 首页常见问题，明确模型目录与外部画布的使用边界。 */
const FAQ_ITEMS = [
  {
    value: 'availability',
    question: 'Are all displayed models always available?',
    answer: 'Model availability and pricing follow the current catalog.',
  },
  {
    value: 'api',
    question: 'Where can I view API access and pricing?',
    answer:
      'Open the API portal to review the current integration entry points, account options, and pricing information.',
  },
  {
    value: 'canvas',
    question: 'What is the creative canvas for?',
    answer:
      'The canvas is an external visual workspace for arranging creative steps, references, and generated media into a connected workflow.',
  },
]

/** 渲染统一样式的外部链接，并保留原生链接语义和新窗口安全属性。 */
function ExternalLink(props: {
  href: string
  label: string
  variant?: 'default' | 'outline'
  className?: string
}) {
  return (
    <a
      className={cn(
        buttonVariants({ variant: props.variant }),
        props.className
      )}
      href={props.href}
      target='_blank'
      rel='noopener noreferrer'
    >
      <span>{props.label}</span>
      <HugeiconsIcon
        icon={LinkSquare01Icon}
        data-icon='inline-end'
        strokeWidth={1.8}
        aria-hidden='true'
      />
    </a>
  )
}

/** 渲染模型标识；主循环支持键盘说明，复制循环仅支持悬停并避开辅助导航。 */
function ModelBadge(props: { model: ModelFact; interactive: boolean }) {
  const { t } = useTranslation()
  let modelIcon = ApiIcon
  if (props.model.kind === 'image') {
    modelIcon = Image02Icon
  } else if (props.model.kind === 'video') {
    modelIcon = Video01Icon
  }

  const content = (
    <span className='mansui-model-badge'>
      <HugeiconsIcon icon={modelIcon} strokeWidth={1.7} aria-hidden='true' />
      <span>{props.model.name}</span>
      <span className='mansui-model-id'>{props.model.id}</span>
      <HugeiconsIcon
        icon={InformationCircleIcon}
        className='mansui-model-info'
        strokeWidth={1.7}
        aria-hidden='true'
      />
    </span>
  )

  if (!props.interactive) {
    return (
      <Tooltip>
        <TooltipTrigger
          className='mansui-model-trigger'
          render={<span aria-hidden='true' />}
        >
          {content}
        </TooltipTrigger>
        <TooltipContent
          role='tooltip'
          aria-hidden='true'
          className='mansui-model-tooltip'
        >
          <strong>{props.model.name}</strong>
          <span>{t(props.model.fact)}</span>
        </TooltipContent>
      </Tooltip>
    )
  }

  const factId = `mansui-model-fact-${props.model.id}`

  return (
    <>
      <Tooltip>
        <TooltipTrigger
          className='mansui-model-trigger'
          aria-label={t('{{model}} model information', {
            model: `${props.model.name} ${props.model.id}`,
          })}
          aria-describedby={factId}
        >
          {content}
        </TooltipTrigger>
        <TooltipContent role='tooltip' className='mansui-model-tooltip'>
          <strong>{props.model.name}</strong>
          <span>{t(props.model.fact)}</span>
        </TooltipContent>
      </Tooltip>
      <span id={factId} className='sr-only'>
        {t(props.model.fact)}
      </span>
    </>
  )
}

/** 展示可用模型能力，并支持悬停、键盘聚焦和手动暂停滚动。 */
export function ModelMarquee() {
  const { t } = useTranslation()
  const [isHovered, setIsHovered] = useState(false)
  const [isFocusWithin, setIsFocusWithin] = useState(false)
  const [isManuallyPaused, setIsManuallyPaused] = useState(false)
  const isPaused = isHovered || isFocusWithin || isManuallyPaused

  return (
    <TooltipProvider delay={120}>
      <section
        className='mansui-model-section'
        aria-labelledby='mansui-model-title'
      >
        <div className='mansui-shell mansui-model-header'>
          <h2 id='mansui-model-title'>
            {t('A creative stack that keeps moving')}
          </h2>
          <div className='mansui-model-meta'>
            <p className='mansui-model-note'>
              {t('Model availability and pricing follow the current catalog.')}
            </p>
            <Button
              type='button'
              variant='outline'
              size='sm'
              className='mansui-marquee-toggle'
              aria-pressed={isManuallyPaused}
              onClick={() => setIsManuallyPaused((isPaused) => !isPaused)}
            >
              <HugeiconsIcon
                icon={isManuallyPaused ? PlayIcon : PauseIcon}
                data-icon='inline-start'
                strokeWidth={1.8}
                aria-hidden='true'
              />
              <span>{isManuallyPaused ? t('Resume') : t('Pause')}</span>
            </Button>
          </div>
        </div>

        <div
          className='mansui-marquee'
          data-testid='model-marquee'
          data-paused={isPaused ? 'true' : 'false'}
          onPointerEnter={() => setIsHovered(true)}
          onPointerLeave={() => setIsHovered(false)}
          onFocus={() => setIsFocusWithin(true)}
          onBlur={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget)) {
              setIsFocusWithin(false)
            }
          }}
        >
          <div className='mansui-marquee-track'>
            <div className='mansui-marquee-group'>
              {MODEL_FACTS.map((model) => (
                <ModelBadge key={model.id} model={model} interactive />
              ))}
            </div>
            <div className='mansui-marquee-group' aria-hidden='true'>
              {MODEL_FACTS.map((model) => (
                <ModelBadge
                  key={`duplicate-${model.id}`}
                  model={model}
                  interactive={false}
                />
              ))}
            </div>
          </div>
        </div>
      </section>
    </TooltipProvider>
  )
}

/** 渲染品牌首屏、双入口行动按钮与鲸鱼主题视觉。 */
function HeroSection() {
  const { t } = useTranslation()

  return (
    <section className='mansui-hero' aria-labelledby='mansui-home-title'>
      <div className='mansui-ripple-field' aria-hidden='true'>
        <span />
        <span />
        <span />
      </div>
      <div className='mansui-shell mansui-hero-grid'>
        <div className='mansui-hero-copy'>
          <div className='mansui-brand-mark'>
            <img src='/mansui-icon.png' alt='' width='44' height='44' />
            <span>{t('ManSuiAI creative intelligence')}</span>
          </div>
          <p className='mansui-kicker'>
            {t('AI creation, connected end to end')}
          </p>
          <h1
            id='mansui-home-title'
            aria-label={`ManSuiAI - ${t('AI aggregation platform')}`}
          >
            <span>ManSuiAI -</span>
            <span>{t('AI aggregation platform')}</span>
          </h1>
          <p className='mansui-hero-lede'>
            {t(
              'Bring AI empowerment, the Token market, and comics creation into one ocean-blue creative portal.'
            )}
          </p>
          <div className='mansui-hero-actions'>
            <ExternalLink
              href='https://api.lolicon.beer/pricing'
              label={t('Explore API and pricing')}
              className='mansui-primary-button'
            />
            <ExternalLink
              href='https://love.lolicon.beer'
              label={t('Open creative canvas')}
              variant='outline'
              className='mansui-secondary-button'
            />
          </div>
        </div>

        <div className='mansui-hero-visual'>
          <div className='mansui-globe' aria-hidden='true'>
            <span className='mansui-globe-ring mansui-globe-ring-one' />
            <span className='mansui-globe-ring mansui-globe-ring-two' />
            <span className='mansui-globe-ring mansui-globe-ring-three' />
            <span className='mansui-globe-core' />
          </div>
          <div className='mansui-portrait-frame'>
            <div className='mansui-portrait-aura' aria-hidden='true' />
            <img
              src='/mansui-whale.webp'
              alt={t('Blue whale-inspired ManSuiAI guide')}
              width='768'
              height='768'
              fetchPriority='high'
            />
          </div>
        </div>
      </div>
    </section>
  )
}

/** 展示从故事构思到图像、视频输出的画布工作流。 */
function CanvasSection() {
  const { t } = useTranslation()

  return (
    <section className='mansui-section mansui-canvas-section'>
      <div className='mansui-shell mansui-canvas-grid'>
        <div className='mansui-canvas-copy'>
          <h2>{t('Let the story flow across the canvas')}</h2>
          <p>
            {t(
              'Arrange prompts, references, image exploration, and motion generation as one readable creative sequence.'
            )}
          </p>
          <ExternalLink
            href='https://love.lolicon.beer'
            label={t('Open creative canvas')}
            className='mansui-primary-button'
          />
        </div>

        <div
          className='mansui-workflow-board'
          aria-label={t('Canvas workflow example')}
        >
          <div className='mansui-workflow-orbit' aria-hidden='true' />
          {WORKFLOW_STEPS.map((step, index) => (
            <article
              key={step.index}
              className={`mansui-workflow-card mansui-workflow-card-${index + 1}`}
            >
              <span>{step.index}</span>
              <div>
                <h3>{t(step.title)}</h3>
                <p>{t(step.description)}</p>
              </div>
            </article>
          ))}
          <div className='mansui-workflow-center'>
            <HugeiconsIcon icon={WorkflowCircle06Icon} strokeWidth={1.5} />
            <span>{t('Connected creation')}</span>
          </div>
        </div>
      </div>
    </section>
  )
}

/** 展示统一接入、Token 管理和漫画创作三类核心能力。 */
function CapabilitiesSection() {
  const { t } = useTranslation()

  return (
    <section className='mansui-section mansui-capabilities-section'>
      <div className='mansui-shell'>
        <div className='mansui-section-heading'>
          <h2>{t('From infrastructure to imagination')}</h2>
          <p>
            {t(
              'A focused portal for people who build with APIs and people who create with pictures, panels, and motion.'
            )}
          </p>
        </div>

        <div className='mansui-capability-list'>
          {CAPABILITIES.map((capability) => (
            <article key={capability.title}>
              <div className='mansui-capability-icon'>
                <HugeiconsIcon icon={capability.icon} strokeWidth={1.6} />
              </div>
              <p>{t(capability.eyebrow)}</p>
              <h3>{t(capability.title)}</h3>
              <span>{t(capability.description)}</span>
            </article>
          ))}
        </div>
      </div>
    </section>
  )
}

/** 展示上手步骤和常见问题，说明模型可用性的实际边界。 */
function StartAndFaqSection() {
  const { t } = useTranslation()

  return (
    <section className='mansui-section mansui-start-section'>
      <div className='mansui-shell mansui-start-grid'>
        <div>
          <h2>{t('Three steps from idea to output')}</h2>
          <div className='mansui-start-steps'>
            {START_STEPS.map((step) => (
              <article key={step.number}>
                <span>{step.number}</span>
                <div>
                  <h3>{t(step.title)}</h3>
                  <p>{t(step.description)}</p>
                </div>
              </article>
            ))}
          </div>
        </div>

        <div className='mansui-faq-panel'>
          <div className='mansui-faq-heading'>
            <HugeiconsIcon icon={MagicWand01Icon} strokeWidth={1.7} />
            <h2>{t('A few useful answers')}</h2>
          </div>
          <Accordion className='mansui-faq-list'>
            {FAQ_ITEMS.map((item) => (
              <AccordionItem key={item.value} value={item.value}>
                <AccordionTrigger className='mansui-faq-trigger hover:no-underline'>
                  {t(item.question)}
                </AccordionTrigger>
                <AccordionContent className='mansui-faq-content'>
                  {t(item.answer)}
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </div>
      </div>
    </section>
  )
}

/** 渲染主页收束行动区，并按登录状态调整引导语。 */
function FinalCtaSection(props: MansuiHomeProps) {
  const { t } = useTranslation()

  return (
    <section className='mansui-final-cta'>
      <div className='mansui-shell mansui-final-cta-inner'>
        <div>
          <HugeiconsIcon
            icon={Layers01Icon}
            strokeWidth={1.5}
            aria-hidden='true'
          />
          <h2>
            {props.isAuthenticated
              ? t('Return with a new idea. Leave with a connected workflow.')
              : t('Start with one idea. Connect it to the right model.')}
          </h2>
        </div>
        <div className='mansui-final-actions'>
          <ExternalLink
            href='https://api.lolicon.beer/pricing'
            label={t('Explore API and pricing')}
            className='mansui-primary-button'
          />
          <ExternalLink
            href='https://love.lolicon.beer'
            label={t('Open creative canvas')}
            variant='outline'
            className='mansui-secondary-button'
          />
        </div>
        <HugeiconsIcon
          icon={ArrowRight02Icon}
          className='mansui-final-arrow'
          strokeWidth={1.2}
          aria-hidden='true'
        />
      </div>
    </section>
  )
}

/** 组合 ManSuiAI 默认主页；管理员自定义主页分支由上层入口继续处理。 */
export function MansuiHome(props: MansuiHomeProps) {
  return (
    <main className='mansui-home'>
      <HeroSection />
      <ModelMarquee />
      <CanvasSection />
      <CapabilitiesSection />
      <StartAndFaqSection />
      <FinalCtaSection isAuthenticated={props.isAuthenticated} />
    </main>
  )
}
