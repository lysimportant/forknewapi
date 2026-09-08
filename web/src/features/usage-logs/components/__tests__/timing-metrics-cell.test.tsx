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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { createInstance } from 'i18next'
import type { ComponentProps } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import { TimingMetricsCell } from '../timing-metrics-cell'

/** 测试专用翻译实例，不读取浏览器状态或修改应用全局语言。 */
const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

/** 首字阈值的毫秒输入及预期色系，覆盖每个边界前后。 */
const firstTokenCases = [
  [0, 'emerald'],
  [9999, 'emerald'],
  [10000, 'amber'],
  [29999, 'amber'],
  [30000, 'orange'],
  [59999, 'orange'],
  [60000, 'red'],
] as const

/** 总耗时阈值的秒输入及预期色系，不依赖生产颜色映射推导期望值。 */
const durationCases = [
  [0, 'emerald'],
  [59.999, 'emerald'],
  [60, 'amber'],
  [179.999, 'amber'],
  [180, 'orange'],
  [299.999, 'orange'],
  [300, 'red'],
] as const

/** 静态渲染真实单元格；输入单位沿用组件契约，返回 HTML，无 DOM 或网络副作用。 */
function renderCell(props: ComponentProps<typeof TimingMetricsCell>): string {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <TimingMetricsCell {...props} />
    </I18nextProvider>
  )
}

/** 读取指标标签后的数值 span；缺少对应指标时断言失败，返回类名及显示文字。 */
function metric(html: string, label: 'First token' | 'Duration') {
  const match = html.match(
    new RegExp(`>${label}</span><span class="([^"]*)">([^<]*)</span>`)
  )
  assert.ok(match, `缺少指标 ${label}: ${html}`)
  return { classes: match[1], text: match[2] }
}

/** 提取静态标记中 aria-hidden 状态条或圆点的类名，保留渲染顺序。 */
function indicators(html: string): string[] {
  return Array.from(
    html.matchAll(/<span aria-hidden="true" class="([^"]*)"><\/span>/g),
    (match) => match[1]
  )
}

/** 按完整 CSS 类名断言，避免子串误匹配；缺失类名时报告实际类名。 */
function hasClasses(actual: string, ...expected: string[]): void {
  for (const name of expected) {
    assert.ok(actual.split(/\s+/).includes(name), `缺少 ${name}: ${actual}`)
  }
}

/** 验证纯色状态不含渐变方向及过渡位置，防止无首字时残留渐变。 */
function noGradient(html: string): void {
  assert.doesNotMatch(html, /bg-linear-to-b|from-40%|to-60%/)
}

describe('TimingMetricsCell 延迟健康度', () => {
  for (const [frtMs, color] of firstTokenCases) {
    test(`首字 ${frtMs} ms 的文字与竖条上端使用 ${color} 档`, () => {
      const html = renderCell({
        isStream: true,
        frtMs,
        useTimeSec: 300,
        completionTokens: 100,
      })
      hasClasses(
        metric(html, 'First token').classes,
        `text-${color}-600`,
        `dark:text-${color}-400`
      )
      const bars = indicators(html)
      assert.equal(bars.length, 1)
      hasClasses(
        bars[0],
        `from-${color}-${color === 'amber' ? '400' : '500'}`,
        'to-red-500',
        'bg-linear-to-b',
        'from-40%',
        'to-60%'
      )
    })
  }

  for (const [useTimeSec, color] of durationCases) {
    test(`总耗时 ${useTimeSec} s 的文字与竖条下端使用 ${color} 档`, () => {
      const html = renderCell({
        isStream: true,
        frtMs: 10000,
        useTimeSec,
        completionTokens: 100,
      })
      hasClasses(
        metric(html, 'Duration').classes,
        `text-${color}-600`,
        `dark:text-${color}-400`
      )
      const bars = indicators(html)
      assert.equal(bars.length, 1)
      hasClasses(
        bars[0],
        'from-amber-400',
        `to-${color}-${color === 'amber' ? '400' : '500'}`,
        'bg-linear-to-b',
        'from-40%',
        'to-60%'
      )
    })
    test(`缺失首字时 ${useTimeSec} s 竖条为 ${color} 纯色`, () => {
      const html = renderCell({
        isStream: true,
        useTimeSec,
        completionTokens: 100,
      })
      assert.equal(metric(html, 'First token').text, 'N/A')
      hasClasses(metric(html, 'First token').classes, 'text-muted-foreground')
      const bars = indicators(html)
      assert.equal(bars.length, 1)
      hasClasses(bars[0], `bg-${color}-${color === 'amber' ? '400' : '500'}`)
      noGradient(html)
    })
  }

  test('非流式即使有首字字段也隐藏首字且使用总耗时纯色', () => {
    const html = renderCell({
      isStream: false,
      frtMs: 1000,
      useTimeSec: 180,
      completionTokens: 100,
    })
    assert.doesNotMatch(html, /First token/)
    assert.equal(metric(html, 'Duration').text, '3m 0s')
    assert.equal(indicators(html).length, 1)
    hasClasses(indicators(html)[0], 'bg-orange-500')
    noGradient(html)
  })

  test('0 ms 是有效首字，显示零秒并保留双色渐变', () => {
    const html = renderCell({
      isStream: true,
      frtMs: 0,
      useTimeSec: 300,
      completionTokens: 0,
    })
    assert.equal(metric(html, 'First token').text, '0.0s')
    assert.doesNotMatch(html, /N\/A/)
    assert.equal(metric(html, 'Duration').text, '5m 0s')
    hasClasses(
      indicators(html)[0],
      'from-emerald-500',
      'to-red-500',
      'bg-linear-to-b',
      'from-40%',
      'to-60%'
    )
  })

  for (const completionTokens of [0, 99, 100, 4500, 9000, 900000]) {
    test(`输出 ${completionTokens} tokens 不改变 300 秒总耗时红档`, () => {
      const html = renderCell({
        isStream: true,
        frtMs: 0,
        useTimeSec: 300,
        completionTokens,
      })
      hasClasses(
        metric(html, 'Duration').classes,
        'text-red-600',
        'dark:text-red-400'
      )
      hasClasses(indicators(html)[0], 'to-red-500')
    })
  }

  for (const [frtMs, color] of firstTokenCases) {
    test(`dot 首字 ${frtMs} ms 的圆点和文字使用 ${color} 档且不绘制竖条`, () => {
      const html = renderCell({
        indicator: 'dot',
        isStream: true,
        frtMs,
        useTimeSec: 180,
        completionTokens: 100,
      })
      const dots = indicators(html)
      assert.equal(dots.length, 2)
      hasClasses(
        dots[0],
        'rounded-full',
        `bg-${color}-${color === 'amber' ? '400' : '500'}`
      )
      hasClasses(
        metric(html, 'First token').classes,
        `text-${color}-600`,
        `dark:text-${color}-400`
      )
      hasClasses(dots[1], 'bg-orange-500')
      hasClasses(
        metric(html, 'Duration').classes,
        'text-orange-600',
        'dark:text-orange-400'
      )
      assert.doesNotMatch(html, /\bw-1\b/)
      noGradient(html)
    })
  }

  test('dot 缺失首字保留中性圆点，总耗时仍独立着色', () => {
    const html = renderCell({
      indicator: 'dot',
      isStream: true,
      useTimeSec: 60,
      completionTokens: 0,
    })
    const dots = indicators(html)
    assert.equal(dots.length, 2)
    hasClasses(dots[0], 'bg-neutral')
    assert.equal(metric(html, 'First token').text, 'N/A')
    hasClasses(metric(html, 'First token').classes, 'text-muted-foreground')
    hasClasses(dots[1], 'bg-amber-400')
    noGradient(html)
  })

  test('dot 非流式仅显示总耗时圆点', () => {
    const html = renderCell({
      indicator: 'dot',
      isStream: false,
      useTimeSec: 300,
      completionTokens: 0,
    })
    assert.doesNotMatch(html, /First token/)
    assert.equal(indicators(html).length, 1)
    hasClasses(indicators(html)[0], 'bg-red-500')
    noGradient(html)
  })
})
