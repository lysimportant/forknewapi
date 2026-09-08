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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { TimingMetricsCell } from '../timing-metrics-cell'

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

describe('TimingMetricsCell 延迟健康度', () => {
  test.each(firstTokenCases)(
    '首字 %i ms 的文字与竖条上端使用 %s 档',
    (frtMs, color) => {
      const { container } = render(
        <TimingMetricsCell
          isStream
          frtMs={frtMs}
          useTimeSec={300}
          completionTokens={100}
        />
      )
      expect(screen.getByText('First token').nextElementSibling).toHaveClass(
        `text-${color}-600`,
        `dark:text-${color}-400`
      )
      expect(container.querySelector('[aria-hidden]')).toHaveClass(
        `from-${color}-${color === 'amber' ? '400' : '500'}`,
        'to-red-500',
        'bg-linear-to-b',
        'from-40%',
        'to-60%'
      )
    }
  )

  test.each(durationCases)(
    '总耗时 %s s 的文字与竖条下端使用 %s 档',
    (useTimeSec, color) => {
      const { container } = render(
        <TimingMetricsCell
          isStream
          frtMs={10000}
          useTimeSec={useTimeSec}
          completionTokens={100}
        />
      )
      expect(screen.getByText('Duration').nextElementSibling).toHaveClass(
        `text-${color}-600`,
        `dark:text-${color}-400`
      )
      expect(container.querySelector('[aria-hidden]')).toHaveClass(
        'from-amber-400',
        `to-${color}-${color === 'amber' ? '400' : '500'}`,
        'bg-linear-to-b',
        'from-40%',
        'to-60%'
      )
    }
  )

  test.each(durationCases)(
    '缺失首字时 %s s 竖条仅使用 %s 总耗时纯色',
    (useTimeSec, color) => {
      const { container } = render(
        <TimingMetricsCell
          isStream
          useTimeSec={useTimeSec}
          completionTokens={100}
        />
      )
      expect(screen.getByText('N/A')).toHaveClass('text-muted-foreground')
      const bar = container.querySelector('[aria-hidden]')
      expect(bar).toHaveClass(
        `bg-${color}-${color === 'amber' ? '400' : '500'}`
      )
      expect(bar).not.toHaveClass('bg-linear-to-b')
      expect(bar).not.toHaveClass('from-40%')
      expect(bar).not.toHaveClass('to-60%')
    }
  )

  test('非流式即使有首字字段也隐藏首字且使用总耗时纯色', () => {
    const { container } = render(
      <TimingMetricsCell
        isStream={false}
        frtMs={1000}
        useTimeSec={180}
        completionTokens={100}
      />
    )
    expect(screen.queryByText('First token')).not.toBeInTheDocument()
    expect(screen.getByText('Duration').nextElementSibling).toHaveTextContent(
      '3m 0s'
    )
    expect(container.querySelector('[aria-hidden]')).toHaveClass(
      'bg-orange-500'
    )
    expect(container.querySelector('[aria-hidden]')).not.toHaveClass(
      'bg-linear-to-b'
    )
  })

  test('0 ms 是有效首字，显示零秒并保留双色渐变', () => {
    const { container } = render(
      <TimingMetricsCell
        isStream
        frtMs={0}
        useTimeSec={300}
        completionTokens={0}
      />
    )
    expect(
      screen.getByText('First token').nextElementSibling
    ).toHaveTextContent('0.0s')
    expect(screen.queryByText('N/A')).not.toBeInTheDocument()
    expect(screen.getByText('Duration').nextElementSibling).toHaveTextContent(
      '5m 0s'
    )
    expect(container.querySelector('[aria-hidden]')).toHaveClass(
      'from-emerald-500',
      'to-red-500',
      'bg-linear-to-b',
      'from-40%',
      'to-60%'
    )
  })

  test.each([0, 99, 100, 4500, 9000, 900000])(
    '输出 %i tokens 不改变 300 秒总耗时红档',
    (completionTokens) => {
      const { container } = render(
        <TimingMetricsCell
          isStream
          frtMs={0}
          useTimeSec={300}
          completionTokens={completionTokens}
        />
      )
      expect(screen.getByText('Duration').nextElementSibling).toHaveClass(
        'text-red-600'
      )
      expect(container.querySelector('[aria-hidden]')).toHaveClass('to-red-500')
    }
  )

  test.each(firstTokenCases)(
    'dot 首字 %i ms 的圆点和文字使用 %s 档且不绘制竖条',
    (frtMs, color) => {
      const { container } = render(
        <TimingMetricsCell
          indicator='dot'
          isStream
          frtMs={frtMs}
          useTimeSec={180}
          completionTokens={100}
        />
      )
      const dots = container.querySelectorAll('[aria-hidden]')
      expect(dots).toHaveLength(2)
      expect(dots[0]).toHaveClass(
        'rounded-full',
        `bg-${color}-${color === 'amber' ? '400' : '500'}`
      )
      expect(screen.getByText('First token').nextElementSibling).toHaveClass(
        `text-${color}-600`
      )
      expect(dots[1]).toHaveClass('bg-orange-500')
      expect(screen.getByText('Duration').nextElementSibling).toHaveClass(
        'text-orange-600'
      )
      expect(container.querySelector('.w-1')).not.toBeInTheDocument()
      expect(container.querySelector('.bg-linear-to-b')).not.toBeInTheDocument()
    }
  )

  test('dot 缺失首字保留中性圆点，总耗时仍独立着色', () => {
    const { container } = render(
      <TimingMetricsCell
        indicator='dot'
        isStream
        useTimeSec={60}
        completionTokens={0}
      />
    )
    const dots = container.querySelectorAll('[aria-hidden]')
    expect(dots).toHaveLength(2)
    expect(dots[0]).toHaveClass('bg-neutral')
    expect(screen.getByText('N/A')).toHaveClass('text-muted-foreground')
    expect(dots[1]).toHaveClass('bg-amber-400')
  })

  test('dot 非流式仅显示总耗时圆点', () => {
    const { container } = render(
      <TimingMetricsCell
        indicator='dot'
        isStream={false}
        useTimeSec={300}
        completionTokens={0}
      />
    )
    expect(screen.queryByText('First token')).not.toBeInTheDocument()
    expect(container.querySelectorAll('[aria-hidden]')).toHaveLength(1)
    expect(container.querySelector('[aria-hidden]')).toHaveClass('bg-red-500')
  })
})
