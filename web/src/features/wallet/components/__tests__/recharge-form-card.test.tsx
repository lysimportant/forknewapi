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
import { useState } from 'react'
import { expect, it, vi } from 'vitest'

import { RechargeFormCard } from '../recharge-form-card'

/** 保留真实受控输入及粘贴事件，付款配置不启用，兑换动作由测试观察。 */
function RedemptionForm({ onRedeem }: { onRedeem: () => void }) {
  const [code, setCode] = useState('')
  return (
    <RechargeFormCard
      topupInfo={null}
      presetAmounts={[]}
      selectedPreset={null}
      onSelectPreset={vi.fn()}
      topupAmount={0}
      onTopupAmountChange={vi.fn()}
      paymentAmount={0}
      calculating={false}
      onPaymentMethodSelect={vi.fn()}
      paymentLoading={null}
      redemptionCode={code}
      onRedemptionCodeChange={setCode}
      onRedeem={onRedeem}
      redeeming={false}
    />
  )
}

it.each([
  ['code-first\ncode-second', 'code-first'],
  ['batch name\t code-first \r\ncode-second', 'code-first'],
  ['batch name\tcode-first', 'code-first'],
])('粘贴结构化兑换码只保留第一行的码：%j', async (text, expected) => {
  const user = userEvent.setup()
  const onRedeem = vi.fn()
  render(<RedemptionForm onRedeem={onRedeem} />)
  const input = screen.getByRole('textbox', { name: 'Have a Code?' })
  await user.click(input)
  await user.paste(text)
  expect(input).toHaveValue(expected)
  expect(onRedeem).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: /^Redeem$/ }))
  expect(onRedeem).toHaveBeenCalledOnce()
})

it('普通单行粘贴仍在光标处插入，保留手工输入内容', async () => {
  const user = userEvent.setup()
  render(<RedemptionForm onRedeem={vi.fn()} />)
  const input = screen.getByRole('textbox', { name: 'Have a Code?' })
  await user.type(input, 'typed-')
  await user.paste('plain-code')
  expect(input).toHaveValue('typed-plain-code')
})

it('店铺默认微信，切换支付宝时同步更新内嵌页和新窗口链接，并保留兑换码', async () => {
  const user = userEvent.setup()
  render(<RedemptionForm onRedeem={vi.fn()} />)
  const wechatTab = screen.getByRole('tab', { name: 'WeChat Pay' })
  const alipayTab = screen.getByRole('tab', { name: 'Alipay' })
  const shopLink = screen.getByRole('button', { name: 'Open in new window' })

  expect(wechatTab).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByTitle('WeChat Pay redemption code shop')).toHaveAttribute(
    'src',
    'https://wzyp.cn/shop/RPE3AZIX'
  )
  expect(shopLink).toHaveAttribute('href', 'https://wzyp.cn/shop/RPE3AZIX')
  expect(shopLink).toHaveAttribute('target', '_blank')
  expect(shopLink).toHaveAttribute('rel', 'noopener noreferrer')

  const code = screen.getByRole('textbox', { name: 'Have a Code?' })
  await user.type(code, 'unredeemed-code')
  await user.click(alipayTab)

  expect(alipayTab).toHaveAttribute('aria-selected', 'true')
  expect(wechatTab).toHaveAttribute('aria-selected', 'false')
  expect(screen.getByTitle('Alipay redemption code shop')).toHaveAttribute(
    'src',
    'https://catfk.com/shop/68AEJFZJ'
  )
  expect(shopLink).toHaveAttribute('href', 'https://catfk.com/shop/68AEJFZJ')
  expect(code).toHaveValue('unredeemed-code')
})

it('店铺标签支持键盘切换回微信并同步新窗口入口', async () => {
  const user = userEvent.setup()
  render(<RedemptionForm onRedeem={vi.fn()} />)
  await user.click(screen.getByRole('tab', { name: 'Alipay' }))
  await user.keyboard('{ArrowLeft}')

  const wechatTab = screen.getByRole('tab', { name: 'WeChat Pay' })
  expect(wechatTab).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(wechatTab).toHaveAttribute('aria-selected', 'true')
  expect(
    screen.getByRole('button', { name: 'Open in new window' })
  ).toHaveAttribute('href', 'https://wzyp.cn/shop/RPE3AZIX')
})
