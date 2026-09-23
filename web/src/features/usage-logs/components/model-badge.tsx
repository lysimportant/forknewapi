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
import { AlertTriangle, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { getReasoningEffortVariant } from '../lib/format'

/** 日志模型展示参数；实际模型来自已记录的上游请求，不代表底层模型身份验证。 */
interface ModelBadgeProps {
  /** 客户端请求的完整模型名称。 */
  modelName: string
  /** 可选上游模型名称；与请求模型相同时不重复展示。 */
  actualModel?: string
  /** 上游响应原始声明的模型，未观测到时省略。 */
  responseModel?: string
  /** 日志记录的推理强度；缺失时不推测上游默认值。 */
  reasoningEffort?: string
  /** 服务端核对调用模型与响应模型后的不一致标记。 */
  isMismatch?: boolean
  className?: string
  /** 在受限宽度内换行，名称最多显示两行，完整值可在详情中查看。 */
  wrapText?: boolean
  /** 卡片详情入口；提供时由调用方打开详情，徽标不触发复制。 */
  onInspect?: () => void
}

interface ModelProvider {
  icon: string
  label: string
}

function resolveModelProvider(modelName: string): ModelProvider | null {
  const model = modelName.toLowerCase()
  const hasAny = (keywords: string[]) =>
    keywords.some((keyword) => model.includes(keyword))

  if (
    hasAny([
      'gpt-',
      'chatgpt-',
      'text-embedding-',
      'omni-moderation',
      'dall-e',
      'whisper',
      'tts-',
    ]) ||
    /\bo[134](?:-|$)/.test(model)
  ) {
    return { icon: 'OpenAI.Color', label: 'OpenAI' }
  }
  if (hasAny(['claude-', 'anthropic'])) {
    return { icon: 'Claude.Color', label: 'Claude' }
  }
  if (hasAny(['gemini-', 'learnlm-'])) {
    return { icon: 'Gemini.Color', label: 'Gemini' }
  }
  if (hasAny(['grok-', 'xai-'])) {
    return { icon: 'Grok.Color', label: 'Grok' }
  }
  if (hasAny(['deepseek-'])) {
    return { icon: 'DeepSeek.Color', label: 'DeepSeek' }
  }
  if (hasAny(['qwen', 'qwq-'])) {
    return { icon: 'Qwen.Color', label: 'Qwen' }
  }
  if (hasAny(['doubao-', 'volcengine'])) {
    return { icon: 'Doubao.Color', label: 'Doubao' }
  }
  if (hasAny(['moonshot-', 'kimi-'])) {
    return { icon: 'Moonshot.Color', label: 'Moonshot' }
  }
  if (hasAny(['minimax', 'abab'])) {
    return { icon: 'Minimax.Color', label: 'MiniMax' }
  }
  if (hasAny(['glm-', 'chatglm', 'cogview', 'cogvideo'])) {
    return { icon: 'Zhipu.Color', label: 'Zhipu' }
  }
  if (hasAny(['mimo-'])) {
    return { icon: 'XiaomiMiMo', label: 'MiMo' }
  }
  if (hasAny(['ernie'])) {
    return { icon: 'Wenxin.Color', label: 'Baidu' }
  }
  if (hasAny(['spark'])) {
    return { icon: 'Spark.Color', label: 'iFlyTek' }
  }
  if (hasAny(['hunyuan'])) {
    return { icon: 'Hunyuan.Color', label: 'Tencent' }
  }
  if (hasAny(['baichuan'])) {
    return { icon: 'Baichuan.Color', label: 'Baichuan' }
  }
  if (hasAny(['internlm'])) {
    return { icon: 'InternLM.Color', label: 'InternLM' }
  }
  if (hasAny(['step-'])) {
    return { icon: 'Stepfun.Color', label: 'StepFun' }
  }
  if (hasAny(['yi-'])) {
    return { icon: 'Yi.Color', label: 'Yi' }
  }
  if (hasAny(['mistral-', 'mixtral-'])) {
    return { icon: 'Mistral.Color', label: 'Mistral' }
  }
  if (hasAny(['llama-', 'meta-'])) {
    return { icon: 'Meta.Color', label: 'Meta' }
  }
  if (hasAny(['command-', 'cohere-'])) {
    return { icon: 'Cohere.Color', label: 'Cohere' }
  }

  return null
}

/** 展示请求模型及供应商图标；没有详情回调时沿用徽标复制行为。 */
function ModelBadgeContent(props: ModelBadgeProps) {
  const provider = resolveModelProvider(props.modelName)

  return (
    <StatusBadge
      copyText={props.modelName}
      copyable={!props.onInspect}
      size='sm'
      showDot={!provider}
      autoColor={provider ? undefined : props.modelName}
      className={cn(
        'border-border/60 bg-muted/30 h-6 max-w-none gap-1.5 rounded-md border px-2 [font-family:var(--font-body)]',
        provider && 'text-foreground',
        props.wrapText && 'h-auto min-h-6 max-w-full py-0.5 whitespace-normal',
        props.className
      )}
    >
      <span
        className={cn(
          'flex items-center gap-1.5',
          props.wrapText ? 'max-w-full min-w-0' : 'max-w-none'
        )}
      >
        {provider && (
          <span
            className='flex h-[18px] w-[18px] shrink-0 items-center justify-center'
            title={provider.label}
            aria-label={provider.label}
          >
            {getLobeIcon(provider.icon, 18)}
          </span>
        )}
        <span
          className={
            props.wrapText
              ? 'line-clamp-2 [overflow-wrap:anywhere]'
              : 'whitespace-nowrap'
          }
        >
          {props.modelName}
        </span>
      </span>
    </StatusBadge>
  )
}

/** 展示请求模型、已记录的推理强度及模型差异，保留完整名称的查看和复制入口。 */
export function ModelBadge(props: ModelBadgeProps) {
  const { t } = useTranslation()
  const reasoningEffort =
    typeof props.reasoningEffort === 'string'
      ? props.reasoningEffort.trim()
      : ''
  const reasoningEffortContent = reasoningEffort ? (
    <StatusBadge
      label={`${t('Reasoning Effort')}: ${reasoningEffort}`}
      variant={getReasoningEffortVariant(reasoningEffort)}
      size='sm'
      copyable={false}
      className='h-auto min-h-5 max-w-full whitespace-normal [&>span]:overflow-visible [&>span]:[overflow-wrap:anywhere] [&>span]:whitespace-normal'
    />
  ) : null
  const actualModel =
    typeof props.actualModel === 'string' &&
    props.actualModel.trim() !== '' &&
    props.actualModel !== props.modelName
      ? props.actualModel
      : undefined
  const actualModelContent = actualModel ? (
    <span className='flex max-w-full min-w-0 items-start gap-1 text-xs'>
      <Route className='mt-0.5 size-3 shrink-0' aria-hidden='true' />
      <span className='shrink-0'>{t('Actual Model:')}</span>
      <span className='line-clamp-2 min-w-0 font-mono [overflow-wrap:anywhere]'>
        {actualModel}
      </span>
    </span>
  ) : null
  const responseModel =
    typeof props.responseModel === 'string' && props.responseModel.trim() !== ''
      ? props.responseModel
      : undefined
  const isMismatch = props.isMismatch === true && responseModel !== undefined
  const responseModelContent = isMismatch ? (
    <span className='text-warning flex max-w-full min-w-0 items-start gap-1 text-xs'>
      <AlertTriangle className='mt-0.5 size-3 shrink-0' aria-hidden='true' />
      <span className='shrink-0'>{t('Response mismatch:')}</span>
      <span className='line-clamp-2 min-w-0 font-mono [overflow-wrap:anywhere]'>
        {responseModel}
      </span>
    </span>
  ) : null

  if (props.onInspect) {
    return (
      <Button
        variant='ghost'
        aria-label={`${t('Model')}: ${props.modelName}`}
        aria-haspopup='dialog'
        onClick={props.onInspect}
        className='h-auto min-h-8 max-w-full min-w-0 flex-col items-start justify-start gap-1 px-0 py-0 text-left font-normal whitespace-normal'
      >
        <ModelBadgeContent {...props} />
        {reasoningEffortContent}
        {actualModelContent}
        {responseModelContent}
      </Button>
    )
  }

  if (!actualModel && !isMismatch) {
    if (!reasoningEffort) return <ModelBadgeContent {...props} />
    return (
      <div className='flex max-w-80 min-w-0 flex-col items-start gap-1'>
        <ModelBadgeContent {...props} wrapText />
        {reasoningEffortContent}
      </div>
    )
  }

  return (
    <div className='flex max-w-80 min-w-0 flex-col items-start gap-1'>
      <ModelBadgeContent {...props} wrapText />
      {reasoningEffortContent}
      <Popover>
        {actualModel && (
          <PopoverTrigger
            render={
              <Button
                variant='ghost'
                aria-label={`${t('Actual Model')}: ${actualModel}`}
                className='text-muted-foreground h-auto min-h-6 max-w-full min-w-0 justify-start px-1 py-0.5 text-left font-normal whitespace-normal'
              />
            }
          >
            {actualModelContent}
          </PopoverTrigger>
        )}
        {isMismatch && (
          <PopoverTrigger
            render={
              <Button
                variant='ghost'
                aria-label={`${t('Upstream Response Model')}: ${responseModel}`}
                className='h-auto min-h-6 max-w-full min-w-0 justify-start px-1 py-0.5 text-left font-normal whitespace-normal'
              />
            }
          >
            {responseModelContent}
          </PopoverTrigger>
        )}
        <PopoverContent
          aria-label={t('Model Details')}
          className='w-80 max-w-[calc(100vw-2rem)]'
        >
          <div className='space-y-3'>
            <div className='space-y-1'>
              <p className='text-muted-foreground text-xs'>
                {t('Request Model:')}
              </p>
              <div className='flex items-start gap-2'>
                <span className='min-w-0 flex-1 font-mono text-xs font-medium [overflow-wrap:anywhere] whitespace-pre-wrap'>
                  {props.modelName}
                </span>
                <CopyButton
                  value={props.modelName}
                  className='size-6'
                  iconClassName='size-3'
                />
              </div>
            </div>
            <div className='space-y-1'>
              <p className='text-muted-foreground text-xs'>
                {t('Actual Model:')}
              </p>
              <div className='flex items-start gap-2'>
                <span className='min-w-0 flex-1 font-mono text-xs font-medium [overflow-wrap:anywhere] whitespace-pre-wrap'>
                  {actualModel ?? props.modelName}
                </span>
                <CopyButton
                  value={actualModel ?? props.modelName}
                  className='size-6'
                  iconClassName='size-3'
                />
              </div>
            </div>
            {responseModel && (
              <div className='space-y-1'>
                <p className='text-muted-foreground text-xs'>
                  {t('Upstream Response Model')}
                </p>
                <div className='flex items-start gap-2'>
                  <span className='min-w-0 flex-1 font-mono text-xs font-medium [overflow-wrap:anywhere] whitespace-pre-wrap'>
                    {responseModel}
                  </span>
                  <CopyButton
                    value={responseModel}
                    className='size-6'
                    iconClassName='size-3'
                  />
                </div>
                {isMismatch && (
                  <StatusBadge
                    label={t('Upstream model mismatch')}
                    variant='warning'
                    copyable={false}
                  />
                )}
              </div>
            )}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  )
}
