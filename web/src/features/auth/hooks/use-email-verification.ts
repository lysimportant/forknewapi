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
import { isAxiosError } from 'axios'
import i18next from 'i18next'
import { useState } from 'react'
import { toast } from 'sonner'

import { useCountdown } from '@/hooks/use-countdown'
import { AuthOperationError } from '@/lib/secure-verification'

import { sendEmailVerification } from '../api'
import { EMAIL_VERIFICATION_COUNTDOWN } from '../constants'

/** 邮箱发码所需的人机验证参数；未启用时可省略。 */
interface UseEmailVerificationOptions {
  turnstileToken?: string
  validateTurnstile?: () => boolean
}

/**
 * 管理注册邮箱验证码发送、翻译后的单次提示和成功后的重发倒计时。
 * @returns 发送状态、剩余秒数及发码方法；请求失败时不启动倒计时。
 */
export function useEmailVerification(options?: UseEmailVerificationOptions) {
  const [isSending, setIsSending] = useState(false)
  const {
    secondsLeft,
    isActive,
    start: startCountdown,
  } = useCountdown({ initialSeconds: EMAIL_VERIFICATION_COUNTDOWN })

  /**
   * 向指定邮箱发送验证码，成功返回 true；校验或请求失败时提示原因并返回 false。
   */
  const sendCode = async (email: string) => {
    if (!email) {
      toast.error(i18next.t('Please enter your email first'))
      return false
    }

    // Validate turnstile if validation function is provided
    if (options?.validateTurnstile && !options.validateTurnstile()) {
      return false
    }

    setIsSending(true)
    try {
      const res = await sendEmailVerification(email, options?.turnstileToken)
      if (res?.success) {
        startCountdown()
        toast.success(i18next.t('Verification email sent'))
        return true
      }
      throw new AuthOperationError(
        res?.message || 'Failed to send verification email'
      )
    } catch (error) {
      // 网络异常没有服务端文案，使用可翻译的重试提示。
      const message =
        isAxiosError(error) && !error.response
          ? 'Failed to send verification email'
          : AuthOperationError.from(error, 'Failed to send verification email')
              .message
      toast.error(
        message ===
          "This email address is not allowed by the administrator's email policy."
          ? i18next.t(
              'This email address is not supported. Please use a QQ email address (e.g. 123456@qq.com).'
            )
          : i18next.t(message)
      )
      return false
    } finally {
      setIsSending(false)
    }
  }

  return {
    isSending,
    secondsLeft,
    isActive,
    sendCode,
  }
}
