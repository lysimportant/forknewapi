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
/** 使用日志延迟档位；与 Sub2API 的绿、黄、橙、红四档一致。 */
export type LatencyVariant = 'success' | 'warning' | 'orange' | 'danger'

/** 首字和总耗时的文字颜色，同时适配浅色与深色主题。 */
export const latencyTextColors = {
  success: 'text-emerald-600 dark:text-emerald-400',
  warning: 'text-amber-600 dark:text-amber-400',
  orange: 'text-orange-600 dark:text-orange-400',
  danger: 'text-red-600 dark:text-red-400',
  neutral: 'text-muted-foreground',
} as const

/** 无首字数据时按总耗时着色，也供移动端状态点复用。 */
export const latencyBarColors = {
  success: 'bg-emerald-500',
  warning: 'bg-amber-400',
  orange: 'bg-orange-500',
  danger: 'bg-red-500',
  neutral: 'bg-neutral',
} as const

/** 竖条上端颜色对应首字延迟。 */
export const latencyBarFromColors = {
  success: 'from-emerald-500',
  warning: 'from-amber-400',
  orange: 'from-orange-500',
  danger: 'from-red-500',
  neutral: 'from-neutral',
} as const

/** 竖条下端颜色对应总耗时，中间 40% 至 60% 渐变。 */
export const latencyBarToColors = {
  success: 'to-emerald-500',
  warning: 'to-amber-400',
  orange: 'to-orange-500',
  danger: 'to-red-500',
} as const
