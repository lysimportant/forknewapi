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
import { resolveModelProvider } from '@/lib/model-provider'
import { cn } from '@/lib/utils'

import { getReasoningEffortVariant } from '../lib/format'
/** 日志模型展示参数；实际模型来自已记录的上游请求，不代表底层模型身份验证。 */
import { isResponseModelMismatch } from '../lib/response-model'
import type { LogOtherData } from '../types'
import { DetailRow } from './dialogs/log-detail-layout'

interface ModelBadgeProps {
  /** 客户端请求的完整模型名称。 */
  modelName: string
  /** 可选上游模型名称；与请求模型相同时不重复展示。 */
  actualModel?: string
  /** 上游响应原始声明的模型，未观测到时省略。 */
  responseModel?: string | LogOtherData['response_model']
  /** 日志记录的推理强度；缺失时不推测上游默认值。 */
  reasoningEffort?: string
  /** 服务端核对调用模型与响应模型后的不一致标记。 */
  isMismatch?: boolean
  className?: string
  /** 在受限宽度内换行，名称最多显示两行，完整值可在详情中查看。 */
  wrapText?: boolean
  /** 卡片详情入口；存在模型映射或差异时打开详情，否则复制模型名。 */
  onInspect?: () => void
}

function ModelBadgeContent(props: ModelBadgeProps & { copyable?: boolean }) {
  const provider = resolveModelProvider(props.modelName)

  return (
    <StatusBadge
      copyText={props.modelName}
      copyable={props.copyable ?? !props.onInspect}
      size='sm'
      showDot={!provider?.icon}
      autoColor={provider?.icon ? undefined : props.modelName}
      className={cn(
        'border-border/60 bg-muted/30 h-6 max-w-none gap-1.5 rounded-md border px-2 [font-family:var(--font-body)]',
        provider?.icon && 'text-foreground',
        props.wrapText && 'h-auto min-h-6 max-w-full py-px whitespace-normal',
        props.className
      )}
    >
      <span
        className={cn(
          'flex items-center gap-1.5',
          props.wrapText ? 'max-w-full min-w-0' : 'max-w-none'
        )}
      >
        {provider?.icon && (
          <span
            className='flex h-[18px] w-[18px] shrink-0 items-center justify-center'
            title={provider.label ?? provider.name}
            aria-label={provider.label ?? provider.name}
          >
            {getLobeIcon(provider.icon, 18)}
          </span>
        )}
        <span
          className={
            props.wrapText
              ? 'line-clamp-2 leading-5 [overflow-wrap:anywhere]'
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
  if (typeof props.responseModel === 'object' && props.responseModel !== null) {
    return <ObservedModelBadge {...props} responseModel={props.responseModel} />
  }
  return <LegacyModelBadge {...props} />
}

function LegacyModelBadge(props: ModelBadgeProps) {
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
    if (!actualModel && !isMismatch) {
      return (
        <CopyButton
          value={props.modelName}
          aria-label={`${t('Model')}: ${props.modelName}`}
          size='sm'
          iconClassName='hidden'
          className='h-auto min-h-8 max-w-full min-w-0 flex-col items-start justify-start gap-1 px-0 py-0 text-left font-normal whitespace-normal'
        >
          <ModelBadgeContent {...props} copyable={false} />
          {reasoningEffortContent}
        </CopyButton>
      )
    }
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

function ObservedModelBadge(
  props: ModelBadgeProps & {
    responseModel: NonNullable<LogOtherData['response_model']>
  }
) {
  const { t } = useTranslation()
  const mismatch = isResponseModelMismatch(props.responseModel)
  const responseModelLabel =
    mismatch && props.responseModel
      ? t('Response model: {{model}}', {
          model: props.responseModel.returned_model,
        })
      : ''
  const modelLabel = `${t('Model')}: ${props.modelName}${responseModelLabel ? `, ${responseModelLabel}` : ''}`
  const hasDetails =
    !!props.actualModel ||
    !!(
      props.responseModel &&
      (mismatch ||
        props.responseModel.returned_model !==
          props.responseModel.requested_model ||
        (props.responseModel.upstream_model &&
          props.responseModel.upstream_model !==
            props.responseModel.requested_model))
    )

  if (!hasDetails) {
    if (props.onInspect) {
      return (
        <CopyButton
          value={props.modelName}
          aria-label={modelLabel}
          size='sm'
          iconClassName='hidden'
          className='h-auto min-h-8 max-w-full min-w-0 justify-start px-0 py-0 text-left font-normal whitespace-normal'
        >
          <ModelBadgeContent {...props} copyable={false} />
        </CopyButton>
      )
    }
    return <ModelBadgeContent {...props} copyable />
  }

  const content = (
    <>
      {props.reasoningEffort && (
        <StatusBadge
          label={`${t('Reasoning Effort')}: ${props.reasoningEffort}`}
          variant={getReasoningEffortVariant(props.reasoningEffort)}
          size='sm'
          copyable={false}
        />
      )}
      <ModelBadgeContent {...props} copyable={false} />
      {mismatch && (
        <StatusBadge
          icon={AlertTriangle}
          label={responseModelLabel}
          variant='warning'
          copyable={false}
        />
      )}
      {!mismatch && props.actualModel && (
        <Route
          className='text-muted-foreground size-3 shrink-0'
          aria-hidden='true'
        />
      )}
    </>
  )

  if (props.onInspect) {
    return (
      <Button
        variant='ghost'
        aria-label={modelLabel}
        aria-haspopup='dialog'
        onClick={props.onInspect}
        className='h-auto min-h-8 max-w-full min-w-0 flex-wrap justify-start gap-1 px-0 py-0 text-left font-normal whitespace-normal'
      >
        {content}
      </Button>
    )
  }

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant='ghost'
            aria-label={modelLabel}
            className='h-auto max-w-full min-w-0 flex-wrap justify-start gap-1 p-0 font-normal'
          />
        }
      >
        {content}
      </PopoverTrigger>
      <PopoverContent className='w-96 max-w-[calc(100vw-2rem)]'>
        {props.responseModel ? (
          <ResponseModelDetails observation={props.responseModel} />
        ) : (
          <div className='space-y-2'>
            <div className='flex items-start justify-between gap-3'>
              <span className='text-muted-foreground text-xs'>
                {t('Request Model:')}
              </span>
              <span className='truncate font-mono text-xs font-medium'>
                {props.modelName}
              </span>
            </div>
            <div className='flex items-start justify-between gap-3'>
              <span className='text-muted-foreground text-xs'>
                {t('Actual Model:')}
              </span>
              <span className='truncate font-mono text-xs font-medium'>
                {props.actualModel}
              </span>
            </div>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

export function ResponseModelDetails(props: {
  observation: NonNullable<LogOtherData['response_model']>
}) {
  const { t } = useTranslation()
  const mismatch = isResponseModelMismatch(props.observation)

  return (
    <div className='min-w-0 space-y-2'>
      {mismatch && (
        <StatusBadge
          icon={AlertTriangle}
          label={t('Response model: {{model}}', {
            model: props.observation.returned_model,
          })}
          variant='warning'
          copyable={false}
          className='h-auto whitespace-normal'
        />
      )}
      <DetailRow
        label={t('Request Model')}
        value={props.observation.requested_model}
        mono
      />
      <DetailRow
        label={t('Upstream Model')}
        value={
          props.observation.upstream_model || props.observation.requested_model
        }
        mono
      />
      <DetailRow
        label={t('Response Model')}
        value={props.observation.returned_model}
        mono
      />
      {mismatch && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'The upstream returned a model name different from both the requested and upstream models. Aliases or dated versions may also cause this; this warning alone does not prove model substitution.'
          )}
        </p>
      )}
    </div>
  )
}
