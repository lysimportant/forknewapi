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
import type { CatalogAvailability } from '../types'

/** 目录状态在三分钟内有效，允许上游时钟最多快一分钟。 */
export const MAX_CATALOG_STATUS_AGE_MS = 3 * 60 * 1000

/** 展示和可见性共用降级语义：缺失、失败、过期或异常状态均为 unknown。 */
export function getCatalogAvailabilityStatus(
  entry: CatalogAvailability | undefined,
  isUnavailable = false,
  now = Date.now()
): CatalogAvailability['status'] {
  const checkedAt = entry?.checked_at
    ? Date.parse(entry.checked_at)
    : Number.NaN
  if (
    isUnavailable ||
    !Number.isFinite(checkedAt) ||
    checkedAt > now + 60 * 1000 ||
    now - checkedAt > MAX_CATALOG_STATUS_AGE_MS
  ) {
    return 'unknown'
  }
  switch (entry?.status) {
    case 'available':
    case 'sold_out':
    case 'disabled':
      return entry.status
    default:
      return 'unknown'
  }
}
