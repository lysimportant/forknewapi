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
import { useQuery } from '@tanstack/react-query'

import { requireServerSuccess } from '@/lib/server-error-message'

import { getYuanliuAvailability } from '../api'

/** 定期读取网关发布的源流状态；请求失败由界面显示 unknown，不重复弹出错误提示。 */
export function useYuanliuAvailability() {
  return useQuery({
    queryKey: ['pricing', 'yuanliu-availability'],
    queryFn: async () => requireServerSuccess(await getYuanliuAvailability()),
    refetchInterval: 60 * 1000,
    refetchOnWindowFocus: 'always',
    retry: false,
    meta: { errorToast: false },
  })
}
