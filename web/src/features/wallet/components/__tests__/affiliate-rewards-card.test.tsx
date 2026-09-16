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
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import type { UserWalletData } from '../../types'
import { AffiliateRewardsCard } from '../affiliate-rewards-card'

/** 构造仅含两天后可提现奖励的用户，不依赖系统时钟。 */
const frozenUser = {
  id: 1,
  username: 'referral-user',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  aff_quota: 500_000,
  aff_history_quota: 500_000,
  aff_count: 1,
  group: 'default',
  referral_rewards: {
    aff_quota: 500_000,
    aff_history_quota: 500_000,
    aff_count: 1,
    frozen_quota: 500_000,
    withdrawable_quota: 0,
    withdrawn_quota: 0,
    next_available_at: 1_800_172_800,
    minimum_quota: 500_000,
    freeze_seconds: 172_800,
    server_time: 1_800_000_000,
    rewards: [],
  },
} satisfies UserWalletData

it('全部奖励冻结时保留禁用的提现按钮和可提时间，成熟后才允许手动提交', async () => {
  const onTransfer = vi.fn()
  const props = {
    user: frozenUser,
    affiliateLink: 'https://example.com/sign-up?aff=referral',
    onTransfer,
    onShowRewards: vi.fn(),
  }
  const { rerender } = render(<AffiliateRewardsCard {...props} />)

  expect(screen.getByRole('button', { name: 'Withdraw' })).toBeDisabled()
  expect(screen.getByText(/Next reward becomes withdrawable at/)).toBeVisible()

  rerender(
    <AffiliateRewardsCard
      {...props}
      user={{
        ...frozenUser,
        referral_rewards: {
          ...frozenUser.referral_rewards,
          frozen_quota: 0,
          withdrawable_quota: 500_000,
          next_available_at: 0,
        },
      }}
    />
  )

  expect(onTransfer).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Withdraw' }))
  expect(onTransfer).toHaveBeenCalledOnce()
})
