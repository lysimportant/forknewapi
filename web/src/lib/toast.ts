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
import { toast as sonnerToast } from 'sonner'

type ToastType = 'success' | 'error' | 'info' | 'warning' | 'message' | 'loading'
type ToastData = Record<string, unknown>

function toastMessageKey(message: unknown): string {
  if (typeof message === 'string' || typeof message === 'number') {
    return String(message)
  }
  if (message == null) {
    return ''
  }
  try {
    return JSON.stringify(message)
  } catch {
    return String(message)
  }
}

function withDedupedId(type: ToastType, message: unknown, data?: ToastData) {
  if (data && Object.hasOwn(data, 'id')) {
    return data
  }
  return {
    ...data,
    id: `${type}:${toastMessageKey(message)}`,
  }
}

function createToast(type: ToastType) {
  const show = sonnerToast[type] as (
    message: unknown,
    data?: ToastData
  ) => string | number
  return (message: unknown, data?: ToastData) =>
    show(message, withDedupedId(type, message, data))
}

export const toast = Object.assign(sonnerToast, {
  success: createToast('success'),
  error: createToast('error'),
  info: createToast('info'),
  warning: createToast('warning'),
  message: createToast('message'),
  loading: createToast('loading'),
})
