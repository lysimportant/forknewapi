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
import type { BillingUsageSchema } from '../types'
import { getTaskNumberFields } from './task-expr'

export type TaskSurchargeOperator = '>' | '>='

/** A usage-fact multiplier applied when a numeric field crosses a threshold. */
export type TaskSurcharge = {
  field: string
  operator: TaskSurchargeOperator
  threshold: number
  multiplier: number
}

export type TaskSurchargeSplit = {
  baseExpr: string
  surcharge: TaskSurcharge | null
}

const SURCHARGE_FACTOR_PATTERN =
  /^u\("([^"]+)"\)\s*(>=|>)\s*(-?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?)\s*\?\s*(-?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?)\s*:\s*1$/

function findOuterParenEnd(expression: string): number {
  let depth = 0
  let quote: '"' | "'" | null = null
  for (let index = 0; index < expression.length; index += 1) {
    const char = expression[index]
    if (quote) {
      if (char === '\\') index += 1
      else if (char === quote) quote = null
      continue
    }
    if (char === '"' || char === "'") {
      quote = char
      continue
    }
    if (char === '(') depth += 1
    else if (char === ')') {
      depth -= 1
      if (depth === 0) return index
    }
  }
  return -1
}

function unwrapOuterParens(expression: string): string {
  let result = expression.trim()
  for (;;) {
    if (!result.startsWith('(') || !result.endsWith(')')) return result
    if (findOuterParenEnd(result) !== result.length - 1) return result
    result = result.slice(1, -1).trim()
  }
}

function splitTopLevelMultiply(expression: string): string[] | null {
  const factors: string[] = []
  let depth = 0
  let quote: '"' | "'" | null = null
  let start = 0
  for (let index = 0; index < expression.length; index += 1) {
    const char = expression[index]
    if (quote) {
      if (char === '\\') index += 1
      else if (char === quote) quote = null
      continue
    }
    if (char === '"' || char === "'") {
      quote = char
      continue
    }
    if (char === '(' || char === '[' || char === '{') {
      depth += 1
      continue
    }
    if (char === ')' || char === ']' || char === '}') {
      depth -= 1
      if (depth < 0) return null
      continue
    }
    if (char === '*' && depth === 0) {
      factors.push(expression.slice(start, index).trim())
      start = index + 1
    }
  }
  if (depth !== 0 || quote) return null
  factors.push(expression.slice(start).trim())
  return factors.every(Boolean) ? factors : null
}

/** Picks the common video duration field when present, otherwise any number field. */
export function defaultTaskSurcharge(
  schema: BillingUsageSchema
): TaskSurcharge | null {
  const fields = getTaskNumberFields(schema).map(([field]) => field)
  if (fields.length === 0) return null
  return {
    field: fields.includes('seconds') ? 'seconds' : fields[0],
    operator: '>',
    threshold: 0,
    multiplier: 2,
  }
}

export function normalizeTaskSurcharge(
  surcharge: TaskSurcharge | null | undefined,
  schema: BillingUsageSchema
): TaskSurcharge | null {
  if (!surcharge) return null
  const fields = new Set(getTaskNumberFields(schema).map(([field]) => field))
  if (!fields.has(surcharge.field)) return null
  if (surcharge.operator !== '>' && surcharge.operator !== '>=') return null
  const threshold = Number(surcharge.threshold)
  const multiplier = Number(surcharge.multiplier)
  if (!Number.isFinite(threshold) || threshold < 0) return null
  if (!Number.isFinite(multiplier) || multiplier <= 0) return null
  return {
    field: surcharge.field,
    operator: surcharge.operator,
    threshold,
    multiplier,
  }
}

function parseTaskSurchargeFactor(
  expression: string,
  schema: BillingUsageSchema
): TaskSurcharge | null {
  const match = SURCHARGE_FACTOR_PATTERN.exec(expression.trim())
  if (!match) return null
  return normalizeTaskSurcharge(
    {
      field: match[1],
      operator: match[2] as TaskSurchargeOperator,
      threshold: Number(match[3]),
      multiplier: Number(match[4]),
    },
    schema
  )
}

function generateTaskSurchargeFactor(
  surcharge: TaskSurcharge | null | undefined,
  schema: BillingUsageSchema
): string {
  const normalized = normalizeTaskSurcharge(surcharge, schema)
  if (!normalized) return ''
  return `(u(${JSON.stringify(normalized.field)}) ${normalized.operator} ${normalized.threshold} ? ${normalized.multiplier} : 1)`
}

/** Wraps a task expression with an optional usage-threshold multiplier. */
export function combineTaskSurcharge(
  baseExpr: string,
  surcharge: TaskSurcharge | null | undefined,
  schema: BillingUsageSchema
): string {
  if (!baseExpr) return ''
  const factor = generateTaskSurchargeFactor(surcharge, schema)
  return factor ? `(${baseExpr}) * ${factor}` : baseExpr
}

/**
 * Splits a stored task expression into its base expression and at most one
 * trailing usage-threshold multiplier. Unsupported shapes keep the expression
 * as the base so the editor falls back to the raw expression.
 */
export function splitTaskSurcharge(
  expression: string | null | undefined,
  schema: BillingUsageSchema
): TaskSurchargeSplit {
  const source = (expression ?? '').trim()
  if (!source) return { baseExpr: '', surcharge: null }
  const factors = splitTopLevelMultiply(source)
  if (!factors) return { baseExpr: source, surcharge: null }

  const baseParts: string[] = []
  let surcharge: TaskSurcharge | null = null
  let matches = 0
  for (const factor of factors) {
    const candidate = parseTaskSurchargeFactor(
      unwrapOuterParens(factor),
      schema
    )
    if (candidate) {
      matches += 1
      surcharge = candidate
      continue
    }
    baseParts.push(factor)
  }
  if (matches === 0 || !surcharge || matches > 1) {
    return { baseExpr: source, surcharge: null }
  }
  if (baseParts.length === 0) return { baseExpr: '', surcharge }
  return {
    baseExpr: unwrapOuterParens(baseParts.join(' * ')),
    surcharge,
  }
}

export function taskSurchargeMatchesSample(
  surcharge: TaskSurcharge | null | undefined,
  sample: Record<string, number | string>
): boolean {
  if (!surcharge) return false
  const value = Number(sample[surcharge.field])
  if (!Number.isFinite(value)) return false
  return surcharge.operator === '>'
    ? value > surcharge.threshold
    : value >= surcharge.threshold
}

export function applyTaskSurcharge(
  total: number,
  surcharge: TaskSurcharge | null | undefined,
  sample: Record<string, number | string>
): number {
  if (!Number.isFinite(total)) return total
  return taskSurchargeMatchesSample(surcharge, sample)
    ? total * (surcharge as TaskSurcharge).multiplier
    : total
}
