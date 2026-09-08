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
import type { BillingUsageSchema, PricingModel } from '../types'

/** 官方文档已明确的型号规格；仅约束主展示入口，不删除旧价格。来源与型号差异见 verification/rc35/video-brand-pricing.md。 */
const documentedModelResolutions: Record<string, readonly string[]> = {
  'grok-imagine-video-1.5': ['480p', '720p', '1080p'],
  'wan2.7-t2v': ['720P', '1080P'],
  'wan2.7-i2v': ['720P', '1080P'],
  'wan2.2-i2v-plus': ['480P', '1080P'],
  'wanx2.1-i2v-turbo': ['480P', '720P'],
  'wanx2.1-i2v-plus': ['720P'],
  'doubao-seedance-2-0-fast-260128': ['480p', '720p'],
  'doubao-seedance-2-0-mini-260615': ['480p', '720p'],
  'doubao-seedance-2-5-260628': ['480p', '720p', '1080p'],
  'doubao-seedance-1-5-pro-251215': ['480p', '720p', '1080p'],
  'doubao-seedance-1-0-pro-250528': ['480p', '720p', '1080p'],
  'veo-3.0-generate-001': ['720p', '1080p'],
  'veo-3.0-fast-generate-001': ['720p', '1080p'],
  // Gemini 与 Vertex 的同名模型共用定价入口；高档能力资料不一致时保留在其他规格，不删除旧配置。
  'veo-3.1-generate-preview': ['720p', '1080p'],
  'veo-3.1-fast-generate-preview': ['720p', '1080p'],
  'MiniMax-H3': ['768P', '2K'],
  'MiniMax-Hailuo-2.3': ['768P', '1080P'],
  'MiniMax-Hailuo-2.3-Fast': ['768P', '1080P'],
  'MiniMax-Hailuo-02': ['512P', '768P', '1080P'],
  'T2V-01': ['720P'],
  'T2V-01-Director': ['720P'],
  'I2V-01': ['720P'],
  'I2V-01-Director': ['720P'],
  'I2V-01-live': ['720P'],
  viduq1: ['1080p'],
  viduq2: ['540p', '720p', '1080p'],
  'vidu2.0': ['360p', '720p', '1080p'],
  // 当前文档未验证这些旧型号的输出规格，原 schema 全部保留在其他规格区。
  'vidu1.5': [],
  'S2V-01': [],
  'doubao-seedance-1-0-lite-t2v': [],
  'doubao-seedance-1-0-lite-i2v': [],
}

/** 官方已确认的模式/产品与清晰度对应；只作说明，不从 units 反推模式，也不合并同分辨率产品的价格。 */
export function getVideoModeResolutionNote(
  modelName?: string
): string | undefined {
  if (modelName === 'kling-v1' || modelName === 'kling-v1-6') {
    return 'std → 720P; pro → 1080P'
  }
  if (modelName === 'kling-v2-master') return 'pro → 1080P'
  if (modelName === 'jimeng_vgfm_t2v_l20') {
    return 's2_pro / v30_720p → 720P; v30_1080p / v30_pro → 1080P'
  }
  return undefined
}

/** 仅用于展示排序的短边像素数；未知规格排在已知规格之后，不推断上游能力。 */
function resolutionShortEdge(value: string): number {
  const pixels = /^(\d+)p$/i.exec(value)
  if (pixels) return Number(pixels[1])
  const kilo = /^(\d+(?:\.\d+)?)k$/i.exec(value)
  if (kilo) {
    const k = Number(kilo[1])
    if (k === 2) return 1440
    return k * 540
  }
  const dimensions = /^(\d+)[x*](\d+)$/i.exec(value)
  if (dimensions) return Math.min(Number(dimensions[1]), Number(dimensions[2]))
  return Number.POSITIVE_INFINITY
}

/** 按品牌原生规格的短边升序返回副本；保留原始字符串、同短边顺序，默认值单独展示。 */
export function getVideoResolutions(schema: BillingUsageSchema): string[] {
  const field = getVideoResolutionField(schema)
  return (field ? [...(schema[field].enum ?? [])] : [])
    .filter((value) => value !== 'unspecified')
    .sort(
      (left, right) => resolutionShortEdge(left) - resolutionShortEdge(right)
    )
}

/** 主入口最多展示五项；已核对型号只主列文档确认档位，其余已有规格保留在原 schema 与独立编辑区。 */
export function getPrimaryVideoResolutions(
  schema: BillingUsageSchema,
  modelName?: string
): string[] {
  const resolutions = getVideoResolutions(schema)
  const documented =
    modelName && Object.hasOwn(documentedModelResolutions, modelName)
      ? documentedModelResolutions[modelName]
      : undefined
  if (documented) {
    return resolutions.filter((value) => documented.includes(value)).slice(0, 5)
  }
  return resolutions.slice(0, 5)
}

/** 只识别原生清晰度/尺寸字段；产品分级和 units 不推断清晰度。 */
export function getVideoResolutionField(
  schema: BillingUsageSchema
): string | undefined {
  return ['resolution', 'size'].find((field) => schema[field]?.enum?.length)
}

/** 从目录元数据或既有视频模型名称识别展示入口，不推断供应商具体档位能力。 */
export function isVideoPricingModel(model: Partial<PricingModel>): boolean {
  if (model.output_modalities?.length) {
    return model.output_modalities.includes('video')
  }
  return Boolean(
    model.supported_endpoint_types?.some((endpoint) =>
      /video/i.test(endpoint)
    ) ||
    /^(grok.*video|sora|veo|kling|jimeng|vidu|hailuo|minimax-(?:h3|hailuo)|[tis]2v-01|wan[\d.-]|doubao-seedance|seedance)/i.test(
      model.model_name ?? ''
    )
  )
}

/** 将默认规格转换成可翻译的展示标签；仅作用于显示，不修改表达式中的 tier 名称或条件。 */
export function formatTaskSpecificationLabel(
  value: string,
  t: (key: string) => string
): string {
  return value.replaceAll(/\bunspecified\b/g, () =>
    t('Provider default (not specified)')
  )
}
