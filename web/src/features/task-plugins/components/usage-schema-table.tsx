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

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from '@/components/ui/collapsible'
import {
  Table,
  TableCaption,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getVideoResolutions,
  getPrimaryVideoResolutions,
  getVideoModeResolutionNote,
  getVideoResolutionField,
  formatTaskSpecificationLabel,
} from '@/features/pricing/lib/video-resolution'
import type { BillingUsageSchema } from '@/features/pricing/types'
import { resolveLocalizedText } from '@/lib/localized-text'

/** 视频详情按原生规格排序展示，默认与额外规格折叠；不修改供应商 schema。 */
type UsageSchemaTableProps = {
  isVideo?: boolean
  modelName?: string
  modelNames?: string[]
  schema: BillingUsageSchema
}

function getUsageTypeLabelKey(
  type: BillingUsageSchema[string]['type']
): string {
  if (type === 'number') return 'Number'
  if (type === 'boolean') return 'Boolean'
  return 'Enum'
}

function formatUsageUnit(
  unit: BillingUsageSchema[string]['unit'],
  t: (key: string) => string
): string {
  if (unit === 'second') return t('Second')
  if (unit === 'count') return t('Count')
  if (unit === 'token') return t('token (unit)')
  if (unit === 'credit') return t('credit')
  return '—'
}

export function UsageSchemaTable(props: UsageSchemaTableProps) {
  const { t, i18n } = useTranslation()
  const entries = Object.entries(props.schema).sort(([left], [right]) =>
    left.localeCompare(right)
  )

  const resolutionField = props.isVideo
    ? getVideoResolutionField(props.schema)
    : undefined
  const resolutions = getVideoResolutions(props.schema)
  const primaryResolutions = getPrimaryVideoResolutions(
    props.schema,
    props.modelName
  )
  const additionalResolutions = resolutions.filter(
    (value) => !primaryResolutions.includes(value)
  )
  const modeNotes = props.isVideo
    ? (props.modelNames ?? []).flatMap((name) => {
        const note = getVideoModeResolutionNote(name)
        return note ? [`${name}: ${note}`] : []
      })
    : []
  return (
    <div className='overflow-x-auto rounded-md border'>
      <Table>
        {modeNotes.length > 0 && (
          <TableCaption className='caption-top text-left'>
            {t('Video resolution')}:{' '}
            {modeNotes.map((note) => (
              <p key={note}>
                <code>{note}</code>
              </p>
            ))}
          </TableCaption>
        )}
        <TableHeader>
          <TableRow>
            <TableHead>{t('Name')}</TableHead>
            <TableHead>{t('Type')}</TableHead>
            <TableHead>{t('Unit')}</TableHead>
            <TableHead>{t('Enum values')}</TableHead>
            <TableHead>{t('Description')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map(([name, definition]) => (
            <TableRow key={name}>
              <TableCell className='font-mono'>{name}</TableCell>
              <TableCell>{t(getUsageTypeLabelKey(definition.type))}</TableCell>
              <TableCell>{formatUsageUnit(definition.unit, t)}</TableCell>
              <TableCell className='font-mono'>
                {name === resolutionField ? (
                  <div className='space-y-2'>
                    <span>{primaryResolutions.join(', ') || '—'}</span>
                    {additionalResolutions.length > 0 && (
                      <Collapsible>
                        <CollapsibleTrigger
                          render={
                            <Button type='button' variant='outline' size='sm' />
                          }
                        >
                          {t('Additional provider specifications')}
                        </CollapsibleTrigger>
                        <CollapsibleContent>
                          {additionalResolutions.join(', ')}
                        </CollapsibleContent>
                      </Collapsible>
                    )}
                    {definition.enum?.includes('unspecified') && (
                      <Collapsible>
                        <CollapsibleTrigger
                          render={
                            <Button type='button' variant='outline' size='sm' />
                          }
                        >
                          {t('Provider default specifications')}
                        </CollapsibleTrigger>
                        <CollapsibleContent>
                          {t('Provider default (not specified)')}
                        </CollapsibleContent>
                      </Collapsible>
                    )}
                  </div>
                ) : (
                  definition.enum
                    ?.map((value) => formatTaskSpecificationLabel(value, t))
                    .join(', ') || '—'
                )}
              </TableCell>
              <TableCell className='min-w-48 whitespace-normal'>
                {resolveLocalizedText(definition.description, i18n.language) ||
                  '—'}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
