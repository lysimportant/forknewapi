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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { DEFAULT_PRICING_PAGE_SIZE } from '../constants'
import type { PricingModel } from '../types'

export function useInfiniteModels(models: PricingModel[], enabled = true) {
  const listKey = useMemo(
    () => JSON.stringify(models.map((model) => [model.id, model.model_name])),
    [models]
  )
  const [window, setWindow] = useState({
    listKey,
    limit: DEFAULT_PRICING_PAGE_SIZE,
  })
  const sentinelRef = useRef<HTMLDivElement>(null)

  // Reset for a new result set, while preserving progress on price/status refresh.
  if (window.listKey !== listKey) {
    setWindow({ listKey, limit: DEFAULT_PRICING_PAGE_SIZE })
  }
  const limit =
    window.listKey === listKey ? window.limit : DEFAULT_PRICING_PAGE_SIZE
  const visibleCount = Math.min(limit, models.length)
  const hasMore = visibleCount < models.length
  const visibleModels = useMemo(
    () => models.slice(0, visibleCount),
    [models, visibleCount]
  )

  const loadMore = useCallback(() => {
    if (!enabled || !hasMore) return
    setWindow({ listKey, limit: visibleCount + DEFAULT_PRICING_PAGE_SIZE })
  }, [enabled, hasMore, listKey, visibleCount])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (
      !enabled ||
      !hasMore ||
      !sentinel ||
      typeof IntersectionObserver === 'undefined'
    ) {
      return
    }

    let active = true
    const observer = new IntersectionObserver(
      (entries) => {
        if (!active || !entries.some((entry) => entry.isIntersecting)) return
        active = false
        observer.disconnect()
        loadMore()
      },
      { rootMargin: '0px 0px 160px 0px' }
    )
    observer.observe(sentinel)
    return () => {
      active = false
      observer.disconnect()
    }
  }, [enabled, hasMore, loadMore])

  return { visibleModels, visibleCount, hasMore, loadMore, sentinelRef }
}
