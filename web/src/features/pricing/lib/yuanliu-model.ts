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
import type { PricingModel, YuanliuAvailabilityMap } from '../types'

/** 插件已适配的上游模型 ID，用于目录暂不可用时识别其来源。 */
const YUANLIU_UPSTREAM_MODELS = new Set([
  'seedance-2.5-guanfang-anmiao',
  'yl_g7zy_seedance_v2_0_std',
  'yl_g7zy_seedance_v2_0_std_full',
  'yl_g7zy_seedance_v2_5',
  'yl_g7zy_seedance_v2_5_full',
  'yl_seedance-2-0_ba0687ff09f2',
  'yl_seedance-2-5_6caffaca7390',
  'yl_seedance-2-5_0fab2f1b1f10',
  'yl_seedance-2-5_750271498003',
  'yl_api_hmstudio_seedance_v2_5_101010_7d58bbb217e6',
  'yl_api_hmstudio_seedance_v2_5_dc729300ff39',
  'yl_video-30_76dbb7993f8e',
  'yl_api_hmstudio_seedance_v2_0_514a65db713b',
])

/** 站内展示的源流模型名称，不用前缀匹配推断未适配型号。 */
const YUANLIU_MODEL_ALIASES = new Set([
  'Yuan-Seedance-2.5-Official',
  'Yuan-Seedance-2.0-LJ',
  'Yuan-Seedance-2.0-LJ-Full',
  'Yuan-Seedance-2.5-LJ',
  'Yuan-Seedance-2.5-LJ-Full',
  'Yuan-Seedance-2.0-HD',
  'Yuan-Seedance-2.5-HD',
  'Yuan-Seedance-2.5-HD-Full',
  'Yuan-Seedance-2.5-HD-PerSecond',
  'Yuan-Seedance-2.5-YS-Full',
  'Yuan-Seedance-2.5-YS',
  'Yuan-Seedance-2.5-YL1',
  'Yuan-Seedance-2.0-YS',
])

/** 根据公开名称、目录键或插件计费标识判断模型是否属于源流；目录请求失败时仍能识别已知型号。 */
export function isYuanliuModel(
  model: PricingModel,
  availability: YuanliuAvailabilityMap
): boolean {
  return (
    Object.hasOwn(availability, model.model_name) ||
    YUANLIU_MODEL_ALIASES.has(model.model_name) ||
    YUANLIU_UPSTREAM_MODELS.has(model.model_name) ||
    Boolean(
      model.billing_plugin_variants?.some(
        (variant) => variant.plugin_key === 'yuanliu'
      )
    )
  )
}
