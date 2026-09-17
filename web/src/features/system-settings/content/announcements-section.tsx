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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Pin, PinOff, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { DateTimePicker } from '@/components/datetime-picker'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { sortAnnouncements } from '@/features/announcements/lib/announcement-sort'
import dayjs from '@/lib/dayjs'

import {
  SettingsSwitchContent,
  SettingsSwitchField,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { SystemOptionsResponse } from '../types'

type Announcement = {
  id: number
  content: string
  publishDate: string
  type: 'default' | 'ongoing' | 'success' | 'warning' | 'error'
  extra?: string
  pinned?: boolean
}

type AnnouncementsSectionProps = {
  enabled: boolean
  data: string
}

const announcementSchema = z.object({
  content: z
    .string()
    .min(1, 'Content is required')
    .max(500, 'Content must be less than 500 characters'),
  publishDate: z.string().min(1, 'Publish date is required'),
  type: z.enum(['default', 'ongoing', 'success', 'warning', 'error']),
  extra: z
    .string()
    .max(100, 'Extra must be less than 100 characters')
    .optional(),
  pinned: z.boolean(),
})

type AnnouncementFormValues = z.infer<typeof announcementSchema>

const ANNOUNCEMENT_FORM_ID = 'announcement-form'

const typeOptions = [
  {
    value: 'default',
    label: 'Default',
    color: 'bg-gray-500',
    badgeVariant: 'neutral' as const,
  },
  {
    value: 'ongoing',
    label: 'Ongoing',
    color: 'bg-blue-500',
    badgeVariant: 'info' as const,
  },
  {
    value: 'success',
    label: 'Success',
    color: 'bg-green-500',
    badgeVariant: 'success' as const,
  },
  {
    value: 'warning',
    label: 'Warning',
    color: 'bg-orange-500',
    badgeVariant: 'warning' as const,
  },
  {
    value: 'error',
    label: 'Error',
    color: 'bg-red-500',
    badgeVariant: 'danger' as const,
  },
]

/** 管理系统公告；每次确认操作立即保存，失败保留输入和已保存列表供重试。 */
export function AnnouncementsSection({
  enabled,
  data,
}: AnnouncementsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const [announcements, setAnnouncements] = useState<Announcement[]>([])
  const [isEnabled, setIsEnabled] = useState(enabled)
  const [isSaving, setIsSaving] = useState(false)
  const savingRef = useRef(false)
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [showDialog, setShowDialog] = useState(false)
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [editingAnnouncement, setEditingAnnouncement] =
    useState<Announcement | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<'single' | 'batch'>('single')

  const form = useForm<AnnouncementFormValues>({
    resolver: zodResolver(announcementSchema),
    defaultValues: {
      content: '',
      publishDate: new Date().toISOString(),
      type: 'default',
      extra: '',
      pinned: false,
    },
  })

  useEffect(() => {
    if (savingRef.current) return
    try {
      const parsed = JSON.parse(data || '[]')
      if (Array.isArray(parsed)) {
        setAnnouncements(
          parsed.map((item, idx) => ({
            ...item,
            // 历史数据没有 pinned 字段，必须按 false 读取
            pinned: item.pinned === true,
            id: item.id || idx + 1,
          }))
        )
      }
    } catch {
      setAnnouncements([])
    }
  }, [data])

  useEffect(() => {
    if (savingRef.current) return
    setIsEnabled(enabled)
  }, [enabled])

  const handleToggleEnabled = async (checked: boolean) => {
    if (savingRef.current) return
    savingRef.current = true
    setIsSaving(true)
    try {
      await updateOption.mutateAsync({
        key: 'console_setting.announcements_enabled',
        value: checked,
      })
      setIsEnabled(checked)
    } catch {
      // 保存失败由公共钩子提示，开关保留原值。
    } finally {
      savingRef.current = false
      setIsSaving(false)
    }
  }

  /** 公告操作只有收到服务端成功后才更新列表，失败保留编辑内容供重试。 */
  const persistAnnouncements = async (
    next: Announcement[]
  ): Promise<boolean> => {
    if (savingRef.current) return false
    savingRef.current = true
    setIsSaving(true)
    const value = JSON.stringify(next)
    try {
      await updateOption.mutateAsync({
        key: 'console_setting.announcements',
        value,
        silent: true,
      })
      // 共享钩子会触发回读；取消仍在途的旧响应，使用刚确认写入的值更新缓存。
      await queryClient.cancelQueries({ queryKey: ['system-options'] })
      queryClient.setQueryData<SystemOptionsResponse>(
        ['system-options'],
        (previous) => {
          if (!previous) return previous
          return {
            ...previous,
            data: [
              ...previous.data.filter(
                (option) => option.key !== 'console_setting.announcements'
              ),
              { key: 'console_setting.announcements', value },
            ],
          }
        }
      )
      setAnnouncements(next)
      toast.success(t('Announcements saved successfully'))
      return true
    } catch {
      // useUpdateOption 已显示服务端错误；此处不清空输入、不宣告保存成功。
      return false
    } finally {
      savingRef.current = false
      setIsSaving(false)
    }
  }

  const handleAdd = () => {
    if (savingRef.current) return
    setEditingAnnouncement(null)
    form.reset({
      content: '',
      publishDate: new Date().toISOString(),
      type: 'default',
      extra: '',
      pinned: false,
    })
    setShowDialog(true)
  }

  const handleEdit = (announcement: Announcement) => {
    if (savingRef.current) return
    setEditingAnnouncement(announcement)
    form.reset({
      content: announcement.content,
      publishDate: announcement.publishDate,
      type: announcement.type,
      extra: announcement.extra || '',
      pinned: announcement.pinned === true,
    })
    setShowDialog(true)
  }

  const handleTogglePin = async (id: number) => {
    await persistAnnouncements(
      announcements.map((item) =>
        item.id === id ? { ...item, pinned: item.pinned !== true } : item
      )
    )
  }

  const handleDelete = (announcement: Announcement) => {
    if (savingRef.current) return
    setEditingAnnouncement(announcement)
    setDeleteTarget('single')
    setShowDeleteDialog(true)
  }

  const handleBatchDelete = () => {
    if (savingRef.current) return
    if (selectedIds.length === 0) {
      toast.error(t('Please select items to delete'))
      return
    }
    setDeleteTarget('batch')
    setShowDeleteDialog(true)
  }

  const confirmDelete = async () => {
    const deletedIds =
      deleteTarget === 'single' ? [editingAnnouncement?.id] : selectedIds
    const next = announcements.filter((item) => !deletedIds.includes(item.id))
    if (await persistAnnouncements(next)) {
      setSelectedIds([])
      setShowDeleteDialog(false)
      setEditingAnnouncement(null)
    }
  }

  const handleSubmitForm = async (values: AnnouncementFormValues) => {
    let next: Announcement[]
    if (editingAnnouncement) {
      next = announcements.map((item) =>
        item.id === editingAnnouncement.id ? { ...item, ...values } : item
      )
    } else {
      const newId = Math.max(...announcements.map((item) => item.id), 0) + 1
      next = [...announcements, { id: newId, ...values }]
    }
    if (await persistAnnouncements(next)) {
      setShowDialog(false)
      setEditingAnnouncement(null)
    }
  }

  const toggleSelectAll = (checked: boolean) => {
    setSelectedIds(checked ? announcements.map((item) => item.id) : [])
  }

  const toggleSelectOne = (id: number, checked: boolean) => {
    setSelectedIds((prev) =>
      checked ? [...prev, id] : prev.filter((item) => item !== id)
    )
  }

  // 列表顺序与公告弹窗、通知列表保持一致：置顶优先 → 发布时间倒序
  const sortedAnnouncements = useMemo(
    () => sortAnnouncements(announcements),
    [announcements]
  )

  const getRelativeTime = (date: string) => {
    const now = new Date()
    const past = new Date(date)
    const diffMs = now.getTime() - past.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)
    const diffDays = Math.floor(diffHours / 24)

    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHours < 24) return `${diffHours}h ago`
    return `${diffDays}d ago`
  }

  return (
    <SettingsSection title={t('Announcements')}>
      <div className='space-y-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <div className='flex flex-wrap items-center gap-2'>
            <Button onClick={handleAdd} size='sm' disabled={isSaving}>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add Announcement')}
            </Button>
            <Button
              onClick={handleBatchDelete}
              size='sm'
              variant='destructive'
              disabled={selectedIds.length === 0 || isSaving}
            >
              <Trash2 className='mr-2 h-4 w-4' />
              {t('Delete (')}
              {selectedIds.length})
            </Button>
          </div>
          <SettingsSwitchField
            checked={isEnabled}
            disabled={isSaving}
            onCheckedChange={handleToggleEnabled}
            label={t('Enabled')}
            className='py-0'
          />
        </div>

        <StaticDataTable
          data={sortedAnnouncements}
          getRowKey={(announcement) => announcement.id}
          emptyContent={t(
            'No announcements yet. Click "Add Announcement" to create one.'
          )}
          columns={[
            {
              id: 'select',
              header: (
                <Checkbox
                  checked={
                    selectedIds.length === announcements.length &&
                    announcements.length > 0
                  }
                  onCheckedChange={toggleSelectAll}
                  disabled={isSaving}
                />
              ),
              className: 'w-12',
              cell: (announcement) => (
                <Checkbox
                  checked={selectedIds.includes(announcement.id)}
                  disabled={isSaving}
                  onCheckedChange={(checked) =>
                    toggleSelectOne(announcement.id, checked as boolean)
                  }
                />
              ),
            },
            {
              id: 'pinned',
              header: t('Pinned'),
              className: 'w-32',
              cell: (announcement) => (
                <div className='flex items-center gap-1.5'>
                  {announcement.pinned ? (
                    <Badge variant='warning'>{t('Pinned')}</Badge>
                  ) : null}
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    aria-label={announcement.pinned ? t('Unpin') : t('Pin')}
                    title={announcement.pinned ? t('Unpin') : t('Pin')}
                    onClick={() => handleTogglePin(announcement.id)}
                    disabled={isSaving}
                  >
                    {announcement.pinned ? <PinOff /> : <Pin />}
                  </Button>
                </div>
              ),
            },
            {
              id: 'content',
              header: t('Content'),
              cellClassName: 'max-w-xs truncate',
              cell: (announcement) => announcement.content,
            },
            {
              id: 'publish-date',
              header: t('Publish Date'),
              cell: (announcement) => (
                <div className='flex flex-col gap-1'>
                  <span className='text-sm font-medium'>
                    {getRelativeTime(announcement.publishDate)}
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {dayjs(announcement.publishDate).format(
                      'YYYY-MM-DD HH:mm:ss'
                    )}
                  </span>
                </div>
              ),
            },
            {
              id: 'type',
              header: t('Type'),
              cell: (announcement) => (
                <StatusBadge
                  label={
                    typeOptions.find((opt) => opt.value === announcement.type)
                      ?.label
                  }
                  variant={
                    typeOptions.find((opt) => opt.value === announcement.type)
                      ?.badgeVariant ?? 'neutral'
                  }
                  copyable={false}
                />
              ),
            },
            {
              id: 'extra',
              header: t('Extra'),
              cellClassName: 'text-muted-foreground max-w-xs truncate',
              cell: (announcement) => announcement.extra || '-',
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (announcement) => (
                <StaticRowActions
                  editLabel={t('Edit')}
                  deleteLabel={t('Delete')}
                  menuLabel={t('Open menu')}
                  onEdit={() => handleEdit(announcement)}
                  onDelete={() => handleDelete(announcement)}
                  editDisabled={isSaving}
                  deleteDisabled={isSaving}
                />
              ),
            },
          ]}
        />
      </div>

      <Dialog
        open={showDialog}
        onOpenChange={(open) => {
          if (!savingRef.current) setShowDialog(open)
        }}
        showCloseButton={!isSaving}
        title={
          editingAnnouncement ? t('Edit Announcement') : t('Add Announcement')
        }
        description={t(
          'Create or update system announcements for the dashboard'
        )}
        contentClassName='max-w-2xl'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setShowDialog(false)}
              disabled={isSaving}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form={ANNOUNCEMENT_FORM_ID}
              disabled={isSaving}
            >
              {isSaving && t('Saving...')}
              {!isSaving && (editingAnnouncement ? t('Update') : t('Add'))}
            </Button>
          </>
        }
      >
        <Form {...form}>
          <form
            id={ANNOUNCEMENT_FORM_ID}
            onSubmit={form.handleSubmit(handleSubmitForm)}
            inert={isSaving}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='content'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Content')}</FormLabel>
                  <FormControl>
                    <Textarea
                      disabled={isSaving}
                      placeholder={t(
                        'Enter announcement content (supports Markdown/HTML)'
                      )}
                      rows={4}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Maximum 500 characters. Supports Markdown and HTML.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='publishDate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Publish Date')}</FormLabel>
                  <FormControl>
                    <DateTimePicker
                      value={field.value ? new Date(field.value) : undefined}
                      onChange={(date) => {
                        if (!savingRef.current) {
                          field.onChange(date ? date.toISOString() : '')
                        }
                      }}
                      placeholder={t('Select publish date')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Date and time when this announcement should be displayed'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='type'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Type')}</FormLabel>
                  <Select
                    disabled={isSaving}
                    items={typeOptions.map((option) => ({
                      value: option.value,
                      label: (
                        <div className='flex items-center gap-2'>
                          <div
                            className={`h-3 w-3 rounded-full ${option.color}`}
                          />
                          {option.label}
                        </div>
                      ),
                    }))}
                    onValueChange={field.onChange}
                    value={field.value}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue
                          placeholder={t('Select announcement type')}
                        />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {typeOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            <div className='flex items-center gap-2'>
                              <div
                                className={`h-3 w-3 rounded-full ${option.color}`}
                              />
                              {option.label}
                            </div>
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='extra'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Extra Notes (Optional)')}</FormLabel>
                  <FormControl>
                    <Input
                      disabled={isSaving}
                      placeholder={t('Additional information')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Optional supplementary information (max 100 characters)'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='pinned'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Pinned')}</FormLabel>
                    <FormDescription>
                      {t('Pinned announcements are displayed first')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      disabled={isSaving}
                      checked={field.value === true}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </form>
        </Form>
      </Dialog>

      <ConfirmDialog
        open={showDeleteDialog}
        onOpenChange={(open) => {
          if (!savingRef.current) setShowDeleteDialog(open)
        }}
        title={t('Are you sure?')}
        desc={
          deleteTarget === 'single'
            ? t('This announcement will be removed from the list.')
            : t('{{count}} announcements will be removed from the list.', {
                count: selectedIds.length,
              })
        }
        destructive
        isLoading={isSaving}
        confirmText={isSaving ? t('Saving...') : t('Delete')}
        handleConfirm={confirmDelete}
      />
    </SettingsSection>
  )
}
