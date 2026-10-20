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
import { expect, it } from 'vitest'

import { isYuanliuModel } from '../lib/yuanliu-model'
import type { PricingModel } from '../types'

function model(name: string): PricingModel {
  return {
    id: 1,
    model_name: name,
    quota_type: 1,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
  }
}

it.each([
  ['yl_pp_md_sd25_900', 'Yuan-Seedance-2.5-QN-02'],
  ['yl_pp_vg_sd25_wd', 'Yuan-Seedance-2.5-QN-03'],
  ['yl_pp_md_sd25_30010', 'Yuan-Seedance-2.5-QN-10'],
  ['y_mjna_md_sd25_30010', 'Yuan-Seedance-2.5-VX-01'],
  ['yl_mjna_cd_seedance_25_720p', 'Yuan-Seedance-2.5-VX-02'],
  ['yl_xk_seedance2_0_903', 'Yuan-Seedance-2.0-XK'],
  ['yl_fuyao_wan3_0_video', 'Yuan-Wan-3.0'],
  ['yl_fuyao_wan3_0_video_prime', 'Yuan-Wan-3.0-Prime'],
])(
  'recognizes exact Yuan model %s and its alias when the catalog is unavailable',
  (id, alias) => {
    expect(isYuanliuModel(model(id), {})).toBe(true)
    expect(isYuanliuModel(model(alias), {})).toBe(true)
    expect(isYuanliuModel(model(`${id}-unrelated`), {})).toBe(false)
  }
)
