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
import * as z from 'zod'

/**
 * 校验文档链接：允许留空；否则必须是完整的 http/https 地址，
 * 拒绝 javascript:、data: 等协议以及携带用户名/密码的地址
 * @returns 错误信息的 i18n 键，合法时返回 null
 */
export function getDocsLinkError(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed === '') return null

  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return 'Enter a valid http:// or https:// URL'
  }

  const isHttp =
    (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
    parsed.hostname.length > 0
  if (!isHttp) {
    return 'Enter a valid http:// or https:// URL'
  }

  if (parsed.username !== '' || parsed.password !== '') {
    return 'Enter a URL without a username or password'
  }

  return null
}

export const docsLinkSchema = z.object({
  general_setting: z.object({
    docs_link: z.string().superRefine((value, ctx) => {
      const message = getDocsLinkError(value)
      if (!message) return

      ctx.addIssue({ code: z.ZodIssueCode.custom, message })
    }),
  }),
})

export type DocsLinkFormValues = z.infer<typeof docsLinkSchema>
