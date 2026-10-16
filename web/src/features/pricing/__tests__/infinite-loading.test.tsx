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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ModelCardGrid } from '../components/model-card-grid'
import { PricingTable } from '../components/pricing-table'
import type { PricingModel } from '../types'

class IntersectionObserverMock {
  static instances: IntersectionObserverMock[] = []
  readonly callback: IntersectionObserverCallback
  target: Element | null = null
  disconnected = false

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback
    IntersectionObserverMock.instances.push(this)
  }

  observe(target: Element): void {
    this.target = target
  }

  disconnect(): void {
    this.disconnected = true
  }

  emit(isIntersecting = true): void {
    this.callback(
      [{ isIntersecting, target: this.target } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver
    )
  }
}

function makeModels(count = 45): PricingModel[] {
  return Array.from({ length: count }, (_, index) => ({
    id: index + 1,
    model_name: `model-${index + 1}`,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: [],
  }))
}

function activeObserver(): IntersectionObserverMock {
  const observer = [...IntersectionObserverMock.instances]
    .reverse()
    .find((instance) => !instance.disconnected)
  if (!observer) throw new Error('Expected an active model list observer')
  return observer
}

let queryClient: QueryClient
beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  queryClient.setQueryData(['perf-metrics-summary', 24], {
    success: true,
    data: { models: [] },
  })
  IntersectionObserverMock.instances = []
  vi.stubGlobal('IntersectionObserver', IntersectionObserverMock)
})
afterEach(() => {
  queryClient.clear()
  vi.unstubAllGlobals()
})

describe.each(['cards', 'table'] as const)(
  '%s infinite model loading',
  (view) => {
    function renderList(models: PricingModel[]) {
      function list(nextModels: PricingModel[]) {
        return (
          <QueryClientProvider client={queryClient}>
            {view === 'cards' ? (
              <ModelCardGrid models={nextModels} onModelClick={vi.fn()} />
            ) : (
              <PricingTable models={nextModels} />
            )}
          </QueryClientProvider>
        )
      }
      const result = render(list(models))
      return {
        ...result,
        rerenderModels: (nextModels: PricingModel[]) =>
          result.rerender(list(nextModels)),
      }
    }

    it('appends the next batch at the bottom and preserves earlier models', () => {
      renderList(makeModels())
      expect(screen.getByText('model-20')).toBeVisible()
      expect(screen.queryByText('model-21')).not.toBeInTheDocument()

      const observer = activeObserver()
      act(() => observer.emit(false))
      expect(screen.queryByText('model-21')).not.toBeInTheDocument()
      act(() => {
        observer.emit()
        observer.emit()
      })

      expect(screen.getByText('model-1')).toBeVisible()
      expect(screen.getByText('model-40')).toBeVisible()
      expect(screen.queryByText('model-41')).not.toBeInTheDocument()
    })

    it('stops at the final batch and ignores notifications from an old observer', () => {
      renderList(makeModels(21))
      const observer = activeObserver()
      act(() => observer.emit())
      expect(screen.getByText('model-21')).toBeVisible()
      expect(screen.getByText('All models loaded')).toBeVisible()
      expect(
        screen.queryByRole('button', { name: 'Load more models' })
      ).toBeNull()
      expect(observer.disconnected).toBe(true)
      act(() => observer.emit())
      expect(screen.getAllByText('model-21')).toHaveLength(1)
    })

    it('resets to the first batch when filtering or sorting changes the model list', () => {
      const models = makeModels()
      const result = renderList(models)
      act(() => activeObserver().emit())
      result.rerenderModels([...models].reverse())
      expect(screen.getByText('model-45')).toBeVisible()
      expect(screen.getByText('model-26')).toBeVisible()
      expect(screen.queryByText('model-25')).not.toBeInTheDocument()

      result.rerenderModels(models.slice(10))
      expect(screen.getByText('model-11')).toBeVisible()
      expect(screen.getByText('model-30')).toBeVisible()
      expect(screen.queryByText('model-31')).not.toBeInTheDocument()
    })

    it('preserves loaded models when refreshed records keep the same order', () => {
      const models = makeModels()
      const result = renderList(models)
      act(() => activeObserver().emit())
      result.rerenderModels(
        models.map((model) => ({ ...model, model_ratio: 2 }))
      )
      expect(screen.getByText('model-40')).toBeVisible()
      expect(screen.queryByText('model-41')).not.toBeInTheDocument()
    })

    it('allows keyboard loading when IntersectionObserver is unavailable', async () => {
      vi.stubGlobal('IntersectionObserver', undefined)
      const user = userEvent.setup()
      renderList(makeModels(21))
      screen.getByRole('button', { name: 'Load more models' }).focus()
      await user.keyboard('{Enter}')
      expect(screen.getByText('model-1')).toBeVisible()
      expect(screen.getByText('model-21')).toBeVisible()
    })

    it('disconnects observations when the list unmounts', () => {
      const result = renderList(makeModels())
      const observer = activeObserver()
      result.unmount()
      expect(observer.disconnected).toBe(true)
    })

    it('does not offer more models for an empty or single-model result', () => {
      const result = renderList([])
      expect(
        screen.queryByRole('button', { name: 'Load more models' })
      ).toBeNull()
      result.rerenderModels(makeModels(1))
      expect(screen.getByText('model-1')).toBeVisible()
      expect(
        screen.queryByRole('button', { name: 'Load more models' })
      ).toBeNull()
    })
  }
)
