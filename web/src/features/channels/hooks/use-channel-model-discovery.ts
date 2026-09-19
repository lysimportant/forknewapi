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
import { useCallback, useEffect, useRef, useState } from 'react'

import { createServerError } from '@/lib/server-error-message'

import { fetchModels, fetchUpstreamModels } from '../api'
import type { FetchModelsResponse } from '../types'

/** 区分实时上游目录与插件声明列表。 */
export type ChannelModelDiscoverySource = NonNullable<
  FetchModelsResponse['source']
>

type DiscoveryState = {
  status: 'idle' | 'loading' | 'success' | 'error' | 'stale'
  models: string[]
  source?: ChannelModelDiscoverySource
  unsupportedModels: string[]
  error?: unknown
}

export type ChannelModelDiscoveryRequest =
  | { kind: 'saved'; channelId: number }
  | { kind: 'preview'; data: Parameters<typeof fetchModels>[0] }

type ChannelModelDiscoveryProps = {
  enabled: boolean
  request: ChannelModelDiscoveryRequest
  scopeKey: string
}

/** 读取渠道模型；失效请求不覆盖新配置，失败不修改表单选择。 */
export function useChannelModelDiscovery(props: ChannelModelDiscoveryProps) {
  const [state, setState] = useState<DiscoveryState>({
    status: 'idle',
    models: [],
    unsupportedModels: [],
  })
  const sequence = useRef(0)
  const scopeKey = useRef(props.scopeKey)

  useEffect(() => {
    sequence.current += 1
    const scopeChanged = scopeKey.current !== props.scopeKey
    scopeKey.current = props.scopeKey
    setState((previous) => {
      if (!props.enabled || scopeChanged) {
        return { status: 'idle', models: [], unsupportedModels: [] }
      }
      if (previous.status === 'idle') return previous
      return {
        status: 'stale',
        models: previous.models,
        source: previous.source,
        unsupportedModels: previous.unsupportedModels,
      }
    })
    return () => {
      sequence.current += 1
    }
  }, [props.enabled, props.request, props.scopeKey])

  const fetch = useCallback(async () => {
    if (!props.enabled) return
    const requestSequence = ++sequence.current
    setState((previous) => ({
      status: 'loading',
      models: previous.models,
      source: previous.source,
      unsupportedModels: previous.unsupportedModels,
    }))
    try {
      const response =
        props.request.kind === 'saved'
          ? await fetchUpstreamModels(props.request.channelId)
          : await fetchModels(props.request.data)
      if (requestSequence !== sequence.current) return
      if (!response.success) {
        throw createServerError(response, 'Failed to fetch models')
      }
      setState({
        status: 'success',
        models: response.data ?? [],
        source: response.source,
        unsupportedModels: response.unsupported_models ?? [],
      })
      return response
    } catch (error) {
      if (requestSequence !== sequence.current) return
      setState((previous) => ({
        status: 'error',
        models: previous.models,
        source: previous.source,
        unsupportedModels: previous.unsupportedModels,
        error,
      }))
    }
  }, [props.enabled, props.request])

  return { ...state, fetch }
}
