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
import i18next from 'i18next'
import { useState, useEffect, useCallback, useRef } from 'react'
import { toast } from 'sonner'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getAffiliateCode, transferAffiliateQuota } from '../api'
import { createIdempotencyKey, generateAffiliateLink } from '../lib'

// ============================================================================
// Affiliate Hook
// ============================================================================

export function useAffiliate() {
  const [affiliateCode, setAffiliateCode] = useState<string>('')
  const [affiliateLink, setAffiliateLink] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [transferring, setTransferring] = useState(false)
  const transferInFlight = useRef(false)
  const pendingTransfer = useRef<{
    quota: number
    idempotency_key: string
  } | null>(null)
  const { copyToClipboard } = useCopyToClipboard()

  // Fetch affiliate code
  const fetchAffiliateCode = useCallback(async () => {
    try {
      setLoading(true)
      const response = requireServerSuccess(await getAffiliateCode())

      if (response.success && response.data) {
        setAffiliateCode(response.data)
        const link = generateAffiliateLink(response.data)
        setAffiliateLink(link)
      }
    } catch (error) {
      handleServerError(error)
    } finally {
      setLoading(false)
    }
  }, [])

  // Copy affiliate link
  const copyAffiliateLink = useCallback(() => {
    copyToClipboard(affiliateLink)
  }, [affiliateLink, copyToClipboard])

  // 网络失败不能证明未入账，同金额重试必须沿用标识；收到明确结果后才开始下一笔。
  const transferQuota = useCallback(async (quota: number): Promise<boolean> => {
    if (transferInFlight.current) return false
    transferInFlight.current = true
    try {
      setTransferring(true)
      if (!pendingTransfer.current || pendingTransfer.current.quota !== quota) {
        pendingTransfer.current = {
          quota,
          idempotency_key: createIdempotencyKey(),
        }
      }
      const response = await transferAffiliateQuota(pendingTransfer.current)
      pendingTransfer.current = null

      if (response.success) {
        toast.success(response.message || i18next.t('Transfer successful'))
        return true
      }

      handleServerError(response, i18next.t('Transfer failed'))
      return false
    } catch (_error) {
      handleServerError(_error, i18next.t('Transfer failed'))
      return false
    } finally {
      transferInFlight.current = false
      setTransferring(false)
    }
  }, [])

  useEffect(() => {
    fetchAffiliateCode()
  }, [fetchAffiliateCode])

  return {
    affiliateCode,
    affiliateLink,
    loading,
    transferring,
    copyAffiliateLink,
    transferQuota,
    refetch: fetchAffiliateCode,
  }
}
