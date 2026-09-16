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
import { describe, expect, it } from 'vitest'

import { DEFAULT_DOCS_LINK } from '@/lib/constants'

import { docsLinkSchema, getDocsLinkError } from '../maintenance/docs-link'

describe('文档链接校验', () => {
  it('允许留空或完整的 http(s) 地址', () => {
    for (const value of [
      '',
      '   ',
      DEFAULT_DOCS_LINK,
      'https://docs.example.com/guide?from=header#top',
      'http://127.0.0.1:8080/docs',
    ]) {
      expect(getDocsLinkError(value)).toBeNull()
    }
  })

  it('拒绝相对地址与非 http(s) 协议', () => {
    for (const value of [
      'docs.example.com',
      '/docs',
      'https://',
      'ftp://docs.example.com',
      'javascript:alert(1)',
      'data:text/html,<script>alert(1)</script>',
    ]) {
      expect(getDocsLinkError(value)).toBe(
        'Enter a valid http:// or https:// URL'
      )
    }
  })

  it('拒绝携带用户名或密码的地址', () => {
    for (const value of [
      'https://user@docs.example.com',
      'https://user:secret@docs.example.com',
    ]) {
      expect(getDocsLinkError(value)).toBe(
        'Enter a URL without a username or password'
      )
    }
  })

  it('Zod schema 复用同一套规则并返回可翻译的错误键', () => {
    expect(
      docsLinkSchema.safeParse({
        general_setting: { docs_link: 'https://docs.example.com' },
      }).success
    ).toBe(true)
    expect(
      docsLinkSchema.safeParse({ general_setting: { docs_link: '' } }).success
    ).toBe(true)

    const invalid = docsLinkSchema.safeParse({
      general_setting: { docs_link: 'javascript:alert(1)' },
    })
    expect(invalid.success).toBe(false)
    expect(invalid.error?.issues[0]?.message).toBe(
      'Enter a valid http:// or https:// URL'
    )
  })
})
