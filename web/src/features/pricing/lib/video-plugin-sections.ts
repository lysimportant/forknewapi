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
import { EXCLUDED_GROUPS, FILTER_ALL } from '../constants'
import type { PricingModel } from '../types'

type VideoPlugin = { key: string; name: string }

export type VideoPluginSection = {
  plugin: (VideoPlugin & { title: string }) | null
  models: PricingModel[]
}

function getVideoPlugin(
  model: PricingModel,
  selectedGroup?: string
): VideoPlugin | undefined {
  const plugins = model.video_plugins
  if (!plugins) return undefined

  const groups =
    selectedGroup && selectedGroup !== FILTER_ALL
      ? [selectedGroup]
      : model.enable_groups.filter((group) => !EXCLUDED_GROUPS.includes(group))

  // A shared model belongs to the first eligible plugin in catalog order.
  for (const group of groups) {
    if (!Object.hasOwn(plugins, group)) continue
    const plugin = plugins[group].find(
      (entry) => entry.key && entry.name.trim()
    )
    if (plugin) return plugin
  }
  return undefined
}

export function getVideoPluginSections(
  models: PricingModel[],
  selectedGroup?: string
): VideoPluginSection[] | null {
  const sections = new Map<string, VideoPluginSection>()
  const unassigned: PricingModel[] = []

  for (const model of models) {
    const plugin = getVideoPlugin(model, selectedGroup)
    if (!plugin) {
      unassigned.push(model)
      continue
    }

    const section = sections.get(plugin.key)
    if (section) {
      section.models.push(model)
    } else {
      const name = plugin.name.trim()
      const [initial] = name.toUpperCase()
      sections.set(plugin.key, {
        plugin: {
          key: plugin.key,
          name,
          title: initial,
        },
        models: [model],
      })
    }
  }

  if (sections.size === 0) return null

  const result = [...sections.values()]
  if (unassigned.length > 0) result.push({ plugin: null, models: unassigned })
  return result
}
