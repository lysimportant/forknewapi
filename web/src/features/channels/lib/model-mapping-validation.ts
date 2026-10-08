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
// ============================================================================
// Model Mapping Validation Utilities
// ============================================================================

/**
 * Parse models string to array
 */
export function parseModelsString(modelsStr: string): string[] {
  return modelsStr
    ? modelsStr
        .split(',')
        .map((m) => m.trim())
        .filter(Boolean)
    : []
}

/**
 * Format models array to string
 */
export function formatModelsArray(models: string[]): string {
  return [...new Set(models)].join(',')
}

/**
 * Normalize model name
 */
export function normalizeModelName(model: string): string {
  return typeof model === 'string' ? model.trim() : ''
}

/**
 * Extract source keys from model_mapping JSON
 * (the keys of the mapping object — models being remapped FROM)
 */
export function extractMappingSourceModels(modelMapping: string): string[] {
  if (typeof modelMapping !== 'string') return []
  const trimmed = modelMapping.trim()
  if (!trimmed) return []

  try {
    const parsed = JSON.parse(trimmed)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }

    const keys = Object.keys(parsed)
      .map((key) => key.trim())
      .filter(Boolean)

    return [...new Set(keys)]
  } catch {
    return []
  }
}

/**
 * Extract redirect models from model_mapping JSON
 */
export function extractRedirectModels(modelMapping: string): string[] {
  const mapping = modelMapping
  if (typeof mapping !== 'string') return []
  const trimmed = mapping.trim()
  if (!trimmed) return []

  try {
    const parsed = JSON.parse(trimmed)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }

    const values = Object.values(parsed)
      .map((value) => (typeof value === 'string' ? value.trim() : undefined))
      .filter((value): value is string => Boolean(value))

    return [...new Set(values)]
  } catch {
    return []
  }
}

/**
 * Check if model configuration has changed
 */
export function hasModelConfigChanged(
  currentModels: string[],
  currentModelMapping: string,
  initialModels: string[],
  initialModelMapping: string
): boolean {
  // Always return true if not editing (new channel)
  if (initialModels.length === 0 && !initialModelMapping) {
    return true
  }

  // Check if models array changed
  if (currentModels.length !== initialModels.length) {
    return true
  }
  for (let i = 0; i < currentModels.length; i++) {
    if (currentModels[i] !== initialModels[i]) {
      return true
    }
  }

  // Check if model_mapping changed
  const normalizedCurrent = (currentModelMapping || '').trim()
  const normalizedInitial = (initialModelMapping || '').trim()

  return normalizedCurrent !== normalizedInitial
}

/**
 * Find models in model_mapping that are missing from the models list
 */
export function findMissingModelsInMapping(
  modelMapping: string,
  currentModels: string[]
): string[] {
  if (!modelMapping || modelMapping.trim() === '') {
    return []
  }

  let parsedMapping: Record<string, unknown>
  try {
    parsedMapping = JSON.parse(modelMapping)
    if (
      !parsedMapping ||
      typeof parsedMapping !== 'object' ||
      Array.isArray(parsedMapping)
    ) {
      return []
    }
  } catch {
    return []
  }

  const modelSet = new Set(currentModels.map((m) => normalizeModelName(m)))
  const missingModels = Object.keys(parsedMapping)
    .map((key) => normalizeModelName(key))
    .filter((key) => key && !modelSet.has(key))

  return [...new Set(missingModels)]
}

/**
 * Validate model mapping JSON format
 */
export function validateModelMappingJson(modelMapping: string): {
  valid: boolean
  error?: string
} {
  if (!modelMapping || modelMapping.trim() === '') {
    return { valid: true }
  }

  try {
    const parsed = JSON.parse(modelMapping)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {
        valid: false,
        error: 'Model mapping must be a valid JSON object',
      }
    }
    if (Object.values(parsed).some((value) => typeof value !== 'string')) {
      return {
        valid: false,
        error: 'Model mapping values must be strings',
      }
    }
    return { valid: true }
  } catch {
    return {
      valid: false,
      error: 'Model mapping must be valid JSON format',
    }
  }
}

/** 本地模型映射校验错误；message 为已有翻译键，用于与供应商原始错误区分。 */
export class ModelMappingValidationError extends Error {}

/**
 * 将已选的目录模型转换为公开别名，并补入对应映射，不覆盖管理员已有映射。
 * @param models 当前模型选择；已选的精确上游 ID 可转换为此次目录建议的别名。
 * @param modelMapping 当前渠道映射 JSON；没有新增映射时保留原文。
 * @param suggestions 成功目录返回的别名映射；只写入已选别名，不删除未选的旧映射。
 * @returns 转换后的模型选择和合并后的映射 JSON。
 * @throws {ModelMappingValidationError} 当前映射不是合法字符串映射对象时抛错，调用方应保留草稿并翻译本地原因。
 */
export function mergeDiscoveredModelAliases(
  models: string[],
  modelMapping: string,
  suggestions?: Record<string, string>
): { models: string[]; modelMapping: string } {
  if (!suggestions || Object.keys(suggestions).length === 0) {
    return { models, modelMapping }
  }
  const validation = validateModelMappingJson(modelMapping)
  if (!validation.valid) {
    throw new ModelMappingValidationError(
      validation.error || 'Invalid model mapping format'
    )
  }

  const existing: Record<string, string> = modelMapping.trim()
    ? JSON.parse(modelMapping)
    : {}
  const upstreamAliases = new Map<string, string[]>()
  for (const [alias, upstream] of Object.entries(suggestions)) {
    if (Object.hasOwn(existing, alias) && existing[alias] !== upstream) continue
    const aliases = upstreamAliases.get(upstream) ?? []
    aliases.push(alias)
    upstreamAliases.set(upstream, aliases)
  }
  const selected = [
    ...new Set(
      models.flatMap((model) =>
        Object.hasOwn(existing, model) || Object.hasOwn(suggestions, model)
          ? [model]
          : (upstreamAliases.get(model) ?? [model])
      )
    ),
  ]
  const additions = selected
    .filter(
      (model) =>
        Object.hasOwn(suggestions, model) && !Object.hasOwn(existing, model)
    )
    .map((model) => [model, suggestions[model]])

  return {
    models: selected,
    modelMapping:
      additions.length === 0
        ? modelMapping
        : JSON.stringify(
            Object.fromEntries([...Object.entries(existing), ...additions]),
            null,
            2
          ),
  }
}

/**
 * Get redirect models that are also in the models list
 * (These should be removed from models list to keep /v1/models clean)
 */
export function findExposedTargetModels(
  modelMapping: string,
  currentModels: string[]
): string[] {
  const redirectModels = extractRedirectModels(modelMapping)
  if (redirectModels.length === 0) return []

  const normalizedModels = currentModels.map((m) => normalizeModelName(m))
  const modelSet = new Set(normalizedModels)

  return redirectModels.filter((model) =>
    modelSet.has(normalizeModelName(model))
  )
}

/**
 * Categorize models into different sets for UI display
 */
export function categorizeModelsWithRedirect(
  currentModels: string[],
  redirectModels: string[]
): {
  normalizedCurrentModels: Set<string>
  normalizedRedirectModels: Set<string>
  classificationSet: Set<string>
  redirectOnlySet: Set<string>
} {
  const normalizedCurrentModels = new Set(
    currentModels.map((m) => normalizeModelName(m)).filter(Boolean)
  )

  const normalizedRedirectModels = new Set(
    redirectModels.map((m) => normalizeModelName(m)).filter(Boolean)
  )

  const classificationSet = new Set([
    ...normalizedCurrentModels,
    ...normalizedRedirectModels,
  ])

  const redirectOnlySet = new Set(
    [...normalizedRedirectModels].filter((m) => !normalizedCurrentModels.has(m))
  )

  return {
    normalizedCurrentModels,
    normalizedRedirectModels,
    classificationSet,
    redirectOnlySet,
  }
}
