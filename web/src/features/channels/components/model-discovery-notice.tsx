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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'

import type { ChannelModelDiscoverySource } from '../hooks/use-channel-model-discovery'

/** 展示模型目录来源及不可导入的上游模型，不改变当前选择。 */
export function ModelDiscoveryNotice(props: {
  source?: ChannelModelDiscoverySource
  unsupportedModels: string[]
}) {
  const { t } = useTranslation()
  if (!props.source) return null
  return (
    <Alert>
      <AlertDescription className='space-y-2'>
        <p>
          {props.source === 'plugin'
            ? t('Source: Plugin model list')
            : t('Source: Upstream catalog')}
        </p>
        {props.source === 'plugin' && (
          <p>
            {t(
              'This plugin has no model catalog endpoint. Account access has not been verified.'
            )}
          </p>
        )}
        {props.unsupportedModels.length > 0 && (
          <>
            <p>
              {t('Models not yet supported by this plugin ({{count}})', {
                count: props.unsupportedModels.length,
              })}
            </p>
            <p>
              {t(
                'These models cannot be imported until the plugin supports them.'
              )}
            </p>
            <ul className='max-h-32 list-inside list-disc overflow-y-auto text-xs break-all'>
              {props.unsupportedModels.map((model) => (
                <li key={model}>{model}</li>
              ))}
            </ul>
          </>
        )}
      </AlertDescription>
    </Alert>
  )
}
