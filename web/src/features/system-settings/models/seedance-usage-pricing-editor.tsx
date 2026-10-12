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
import { useId, useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { InputGroup, InputGroupAddon } from '@/components/ui/input-group'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { USD_PRICING_CURRENCY } from '@/features/model-pricing/currency'
import { PricingAmountInput } from '@/features/model-pricing/pricing-amount-input'
import { combineBillingExpr } from '@/features/pricing/lib/billing-expr'
import { compileBillingExpression } from '@/features/pricing/lib/billing-expression/parser'
import type { ExpressionNode } from '@/features/pricing/lib/billing-expression/types'

import { RequestSimulation } from './request-simulation'
import { TaskUsagePricingEditor } from './task-usage-pricing-editor'

/** 新建 Seedance 定价默认按次计费，零价仍读取用量事实。 */
export const DEFAULT_SEEDANCE_BILLING_EXPR = 'tier("base", u("count") * 0)'

/** 清晰度标识与插件事实一致；四档价格不会合并成通用兜底档。 */
const RESOLUTIONS = ['480p', '720p', '1080p', '2k'] as const
/** 简易模式互斥，自定义模式沿用任务表达式编辑器。 */
type SeedancePricingMode = 'per-call' | 'seconds-resolution' | 'custom'
/** 单价以 USD 保存，保留按次及各清晰度的独立草稿。 */
type SeedancePrices = Record<'count' | (typeof RESOLUTIONS)[number], string>
/** 模式切换保留各自表达式及自定义草稿，不在初始化时写回。 */
type SeedancePricingDraft = {
  mode: SeedancePricingMode
  prices: SeedancePrices
  expressions: Record<Exclude<SeedancePricingMode, 'custom'>, string>
  custom: string | null
}

/** 识别单个用量乘单价，可带 tier 包装；附加收费及复杂运算留在自定义编辑器。 */
function readSeedanceUnitPrice(
  node: ExpressionNode,
  field: string
): string | null {
  let body = node
  if (node.kind === 'call' && node.name === 'tier') {
    const [name, value] = node.args
    if (name?.kind !== 'literal' || typeof name.value !== 'string' || !value) {
      return null
    }
    body = value
  }
  if (body?.kind !== 'binary' || body.operator !== '*') return null
  if (body.left.kind !== 'call' || body.left.name !== 'u') return null
  const usage = body.left.args[0]
  if (usage?.kind !== 'literal' || usage.value !== field) return null
  if (body.right.kind !== 'literal' || typeof body.right.value !== 'number') {
    return null
  }
  if (!Number.isFinite(body.right.value) || body.right.value < 0) return null
  return String(body.right.value)
}

/** 保守识别按次或完整四档按秒模式；无法无损表达的公式返回 null。 */
function readSeedancePricing(expression: string): {
  mode: Exclude<SeedancePricingMode, 'custom'>
  prices: SeedancePrices
} | null {
  const compiled = compileBillingExpression(expression)
  if (compiled.status !== 'ready') return null
  const prices: SeedancePrices = {
    count: '0',
    '480p': '0',
    '720p': '0',
    '1080p': '0',
    '2k': '0',
  }
  const count = readSeedanceUnitPrice(compiled.ast, 'count')
  if (count !== null) return { mode: 'per-call', prices: { ...prices, count } }
  let node = compiled.ast
  for (const resolution of RESOLUTIONS.slice(0, -1)) {
    if (node.kind !== 'conditional') return null
    const condition = node.condition
    if (condition.kind !== 'binary' || condition.operator !== '==') return null
    if (condition.left.kind !== 'call' || condition.left.name !== 'u') {
      return null
    }
    const usage = condition.left.args[0]
    if (usage?.kind !== 'literal' || usage.value !== 'video_resolution') {
      return null
    }
    if (
      condition.right.kind !== 'literal' ||
      condition.right.value !== resolution
    ) {
      return null
    }
    const price = readSeedanceUnitPrice(node.yes, 'seconds')
    if (price === null) return null
    prices[resolution] = price
    node = node.no
  }
  const finalPrice = readSeedanceUnitPrice(node, 'seconds')
  if (finalPrice === null) return null
  prices['2k'] = finalPrice
  return { mode: 'seconds-resolution', prices }
}

/** 生成互斥计费公式；保留零价 seconds 项，让缺少时长或清晰度的请求验证失败。 */
function seedanceBillingExpression(
  mode: Exclude<SeedancePricingMode, 'custom'>,
  prices: SeedancePrices
): string {
  if (mode === 'per-call') {
    return `tier("base", u("count") * ${prices.count || '0'})`
  }
  return [
    `u("video_resolution") == "480p" ? tier("480p", u("seconds") * ${prices['480p'] || '0'})`,
    `u("video_resolution") == "720p" ? tier("720p", u("seconds") * ${prices['720p'] || '0'})`,
    `u("video_resolution") == "1080p" ? tier("1080p", u("seconds") * ${prices['1080p'] || '0'})`,
    `tier("2k", u("seconds") * ${prices['2k'] || '0'})`,
  ].join('\n  : ')
}

/** Seedance 专用互斥价格表；只更新父级草稿，保存由现有模型定价流程完成。 */
export function SeedanceUsagePricingEditor(
  props: ComponentProps<typeof TaskUsagePricingEditor>
) {
  const { t } = useTranslation()
  const id = useId()
  const currency = props.currency ?? USD_PRICING_CURRENCY
  const [draft, setDraft] = useState<SeedancePricingDraft>(() => {
    const parsed = readSeedancePricing(props.billingExpr)
    const prices = parsed?.prices ?? {
      count: '0',
      '480p': '0',
      '720p': '0',
      '1080p': '0',
      '2k': '0',
    }
    return {
      mode: parsed?.mode ?? 'custom',
      prices,
      expressions: {
        'per-call':
          parsed?.mode === 'per-call'
            ? props.billingExpr
            : seedanceBillingExpression('per-call', prices),
        'seconds-resolution':
          parsed?.mode === 'seconds-resolution'
            ? props.billingExpr
            : seedanceBillingExpression('seconds-resolution', prices),
      },
      custom: parsed ? null : props.billingExpr,
    }
  })
  const [pendingMode, setPendingMode] = useState<Exclude<
    SeedancePricingMode,
    'custom'
  > | null>(null)

  const applyMode = (mode: SeedancePricingMode): void => {
    if (mode === 'custom') {
      setDraft({ ...draft, mode })
      props.onBillingExprChange(draft.custom ?? props.billingExpr)
      return
    }
    const parsed =
      draft.mode === 'custom' ? readSeedancePricing(props.billingExpr) : null
    const prices = { ...draft.prices }
    if (parsed?.mode === 'per-call') prices.count = parsed.prices.count
    else if (parsed) {
      for (const resolution of RESOLUTIONS) {
        prices[resolution] = parsed.prices[resolution]
      }
    }
    const expressions = { ...draft.expressions }
    if (parsed) expressions[parsed.mode] = props.billingExpr
    setDraft({ ...draft, mode, prices, expressions })
    props.onBillingExprChange(expressions[mode])
  }
  const changeMode = (mode: SeedancePricingMode): void => {
    if (mode === draft.mode) return
    if (
      mode !== 'custom' &&
      draft.mode === 'custom' &&
      !readSeedancePricing(props.billingExpr)
    ) {
      setPendingMode(mode)
      return
    }
    applyMode(mode)
  }
  const changePrice = (field: keyof SeedancePrices, value: string): void => {
    if (draft.mode === 'custom') return
    const prices = { ...draft.prices, [field]: value }
    const expression = seedanceBillingExpression(draft.mode, prices)
    setDraft({
      ...draft,
      prices,
      expressions: { ...draft.expressions, [draft.mode]: expression },
    })
    props.onBillingExprChange(expression)
  }

  return (
    <div className='space-y-4'>
      <Tabs
        value={draft.mode}
        onValueChange={(value) => changeMode(value as SeedancePricingMode)}
      >
        <TabsList
          aria-label={t('Pricing mode')}
          className='h-auto! max-w-full flex-wrap'
        >
          <TabsTrigger value='per-call'>{t('Per-call')}</TabsTrigger>
          <TabsTrigger value='seconds-resolution'>
            {t('Seconds × resolution')}
          </TabsTrigger>
          <TabsTrigger value='custom'>{t('Expression editor')}</TabsTrigger>
        </TabsList>
        <TabsContent value='per-call'>
          <Field className='gap-2'>
            <FieldLabel htmlFor={`${id}-count`}>
              {t('Price per call')}
            </FieldLabel>
            <InputGroup>
              <PricingAmountInput
                id={`${id}-count`}
                aria-label={t('Price per call')}
                grouped
                currency={currency}
                value={draft.prices.count}
                onChange={(value) => changePrice('count', value)}
              />
              <InputGroupAddon align='inline-end'>
                {currency.symbol}/{t('Request')}
              </InputGroupAddon>
            </InputGroup>
          </Field>
        </TabsContent>
        <TabsContent
          value='seconds-resolution'
          className='grid gap-3 sm:grid-cols-2'
        >
          {RESOLUTIONS.map((resolution) => (
            <Field key={resolution} className='gap-2'>
              <FieldLabel htmlFor={`${id}-${resolution}`}>
                {resolution === '2k' ? '2K' : resolution}{' '}
                {t('Price per second')}
              </FieldLabel>
              <InputGroup>
                <PricingAmountInput
                  id={`${id}-${resolution}`}
                  aria-label={`${resolution === '2k' ? '2K' : resolution} ${t('Price per second')}`}
                  grouped
                  currency={currency}
                  value={draft.prices[resolution]}
                  onChange={(value) => changePrice(resolution, value)}
                />
                <InputGroupAddon align='inline-end'>
                  {currency.symbol}/{t('second')}
                </InputGroupAddon>
              </InputGroup>
            </Field>
          ))}
        </TabsContent>
        <TabsContent value='custom'>
          <TaskUsagePricingEditor
            {...props}
            expressionOnly
            onBillingExprChange={(value) => {
              setDraft((current) => ({ ...current, custom: value }))
              props.onBillingExprChange(value)
            }}
          />
        </TabsContent>
      </Tabs>
      {draft.mode !== 'custom' && (
        <>
          <Field className='gap-2'>
            <FieldLabel htmlFor={`${id}-rules`}>
              {t('Request rule pricing')}
            </FieldLabel>
            <Textarea
              id={`${id}-rules`}
              value={props.requestRuleExpr}
              onChange={(event) =>
                props.onRequestRuleExprChange(event.target.value)
              }
              rows={3}
              className='font-mono text-xs'
              spellCheck={false}
            />
            <FieldDescription>
              {t(
                'Optional request-rule multiplier expression. Leave empty when no request rule applies.'
              )}
            </FieldDescription>
          </Field>
          <code className='bg-muted/30 block max-h-40 overflow-auto rounded-md border p-3 text-xs break-all whitespace-pre-wrap'>
            {props.billingExpr}
          </code>
          <RequestSimulation
            expression={combineBillingExpr(
              props.billingExpr,
              props.requestRuleExpr
            )}
            usage={
              props.usageExamples?.[0]?.facts ?? {
                count: 1,
                seconds: 5,
                video_resolution: '480p',
              }
            }
            usageSchema={props.usageSchema}
            currency={currency}
            mode='task'
          />
        </>
      )}
      <ConfirmDialog
        open={pendingMode !== null}
        onOpenChange={(open) => !open && setPendingMode(null)}
        title={t('Switch billing method?')}
        desc={t(
          "The custom billing expression will be replaced by this method's prices. Request rules are retained. Changes apply only after saving."
        )}
        confirmText={t('Apply billing method')}
        handleConfirm={() => {
          if (pendingMode) applyMode(pendingMode)
          setPendingMode(null)
        }}
      />
    </div>
  )
}
