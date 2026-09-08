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

import { describe, expect, test } from 'vitest'

import {
  getVideoResolutions,
  getPrimaryVideoResolutions,
  isVideoPricingModel,
} from '../video-resolution'

describe('供应商原生清晰度展示', () => {
  test('按已核对型号过滤主入口，额外原值保留供旧价格编辑', () => {
    const schema = { resolution: { enum: ['480P', '720P', '1080P'] } }
    expect(getPrimaryVideoResolutions(schema, 'wan2.7-t2v')).toEqual([
      '720P',
      '1080P',
    ])
    expect(getPrimaryVideoResolutions(schema, 'wan2.2-i2v-plus')).toEqual([
      '480P',
      '1080P',
    ])
    expect(getPrimaryVideoResolutions(schema, 'wanx2.1-i2v-plus')).toEqual([
      '720P',
    ])
    expect(getVideoResolutions(schema)).toEqual(['480P', '720P', '1080P'])
    expect(getPrimaryVideoResolutions(schema, '__proto__')).toEqual([
      '480P',
      '720P',
      '1080P',
    ])
    expect(
      getPrimaryVideoResolutions(
        { resolution: { enum: ['480p', '720p', '1080p', '4k'] } },
        'doubao-seedance-2-0-mini-260615'
      )
    ).toEqual(['480p', '720p'])
  })
  test('排序返回副本并保留非标准短边、大小写、未知规格与同短边尺寸顺序', () => {
    const values = [
      'custom',
      'unspecified',
      '2k',
      '768P',
      '512P',
      '720P',
      '1080P',
    ]
    expect(getVideoResolutions({ resolution: { enum: values } })).toEqual([
      '512P',
      '720P',
      '768P',
      '1080P',
      '2k',
      'custom',
    ])
    expect(values).toEqual([
      'custom',
      'unspecified',
      '2k',
      '768P',
      '512P',
      '720P',
      '1080P',
    ])
    expect(
      getVideoResolutions({
        size: { enum: ['1792x1024', '1024x1792', '1280x720', '720x1280'] },
      })
    ).toEqual(['1280x720', '720x1280', '1792x1024', '1024x1792'])
  })
  test('无清晰度、仅默认值或产品分级时不生成档位', () => {
    expect(getVideoResolutions({})).toEqual([])
    expect(
      getVideoResolutions({ resolution: { enum: ['unspecified'] } })
    ).toEqual([])
    expect(
      getVideoResolutions({ product: { enum: ['v30_720p', 'v30_pro'] } })
    ).toEqual([])
  })
  test('视频目录元数据优先，缺少目录时支持海螺原生模型名，音频模型不启用入口', () => {
    expect(
      isVideoPricingModel({
        model_name: 'custom',
        output_modalities: ['video'],
      })
    ).toBe(true)
    expect(isVideoPricingModel({ model_name: 'MiniMax-H3' })).toBe(true)
    expect(isVideoPricingModel({ model_name: 'MiniMax-Hailuo-2.3' })).toBe(true)
    expect(isVideoPricingModel({ model_name: 'T2V-01' })).toBe(true)
    expect(isVideoPricingModel({ model_name: 'suno_music' })).toBe(false)
    expect(
      isVideoPricingModel({ model_name: 'sora', output_modalities: ['image'] })
    ).toBe(false)
  })
})
