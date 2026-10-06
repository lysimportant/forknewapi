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
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { subscribeAuthQueryCacheInvalidation } from '@/routes/__root'

import { useAuthStore, type AuthBundle } from '../stores/auth-store'
import {
  applyAuthRotation,
  bootstrapAuthentication,
  clearAuthenticatedClientState,
  createRefreshRunner,
  isAuthBundle,
  type AuthRefreshRuntime,
} from './auth-session'

const bundle: AuthBundle = {
  access_token: 'access-token',
  token_type: 'Bearer',
  access_expires_at: Math.floor(Date.now() / 1000) + 600,
  user: {
    id: 42,
    username: 'test-user',
    role: 1,
  },
  session: {
    sid: 'session-a',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 100,
    last_active_at: 100,
    expires_at: 1000,
  },
}

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('authentication session coordination', () => {
  test('bootstrap distinguishes a completed anonymous check from an active session', async () => {
    useAuthStore.getState().auth.reset('complete')
    expect(await bootstrapAuthentication()).toEqual({ kind: 'anonymous' })

    useAuthStore.getState().auth.setBundle(bundle)
    expect(await bootstrapAuthentication()).toEqual({
      kind: 'authenticated',
      bundle,
    })
  })

  test('a session mismatch clears only local state and retries without the stale SID', async () => {
    let expectedSID: string | undefined = bundle.session.sid
    const requestedSIDs: Array<string | undefined> = []
    const clears: Array<[boolean, string | undefined]> = []
    const accepted: AuthBundle[] = []
    const runtime: AuthRefreshRuntime = {
      request: async (sid) => {
        requestedSIDs.push(sid)
        if (requestedSIDs.length === 1) {
          return {
            status: 409,
            data: { code: 'AUTH_SESSION_MISMATCH' },
          }
        }
        return { status: 200, data: { success: true, data: bundle } }
      },
      getExpectedSID: () => expectedSID,
      parseBundle: (value) => (isAuthBundle(value) ? value : null),
      acceptBundle: (acceptedBundle) => accepted.push(acceptedBundle),
      clear: (synchronizeTabs, bootstrapState) => {
        clears.push([synchronizeTabs, bootstrapState])
        expectedSID = undefined
      },
      markTransient: () => undefined,
      wait: async () => undefined,
    }

    const outcome = await createRefreshRunner(runtime)()

    expect(outcome.kind).toBe('authenticated')
    expect(requestedSIDs).toEqual([bundle.session.sid, undefined])
    expect(clears).toEqual([[false, 'idle']])
    expect(accepted).toEqual([bundle])
  })

  test('a rejected refresh confirms anonymous state and synchronizes sign-out', async () => {
    const clears: Array<[boolean, string | undefined]> = []
    const runtime: AuthRefreshRuntime = {
      request: async () => ({ status: 401 }),
      getExpectedSID: () => bundle.session.sid,
      parseBundle: () => null,
      acceptBundle: () => undefined,
      clear: (synchronizeTabs, bootstrapState) => {
        clears.push([synchronizeTabs, bootstrapState])
      },
      markTransient: () => undefined,
      wait: async () => undefined,
    }

    expect(await createRefreshRunner(runtime)()).toEqual({
      kind: 'anonymous',
    })
    expect(clears).toEqual([[true, undefined]])
  })

  test('a temporary refresh failure remains retryable without clearing the session', async () => {
    let transientCount = 0
    let clearCount = 0
    const runtime: AuthRefreshRuntime = {
      request: async () => ({ status: 503, error: new Error('unavailable') }),
      getExpectedSID: () => bundle.session.sid,
      parseBundle: () => null,
      acceptBundle: () => undefined,
      clear: () => {
        clearCount += 1
      },
      markTransient: () => {
        transientCount += 1
      },
      wait: async () => undefined,
    }

    const outcome = await createRefreshRunner(runtime)()

    expect(outcome.kind).toBe('transient_error')
    expect(clearCount).toBe(0)
    expect(transientCount).toBe(1)
  })

  test('a rate limited refresh remains retryable without clearing the session', async () => {
    let transientCount = 0
    let clearCount = 0
    const runtime: AuthRefreshRuntime = {
      request: async () => ({ status: 429 }),
      getExpectedSID: () => bundle.session.sid,
      parseBundle: () => null,
      acceptBundle: () => undefined,
      clear: () => {
        clearCount += 1
      },
      markTransient: () => {
        transientCount += 1
      },
      wait: async () => undefined,
    }

    const outcome = await createRefreshRunner(runtime)()

    expect(outcome.kind).toBe('transient_error')
    expect(clearCount).toBe(0)
    expect(transientCount).toBe(1)
  })

  test('an exhausted refresh race clears the unusable local session', async () => {
    const requestedDelays: number[] = []
    const clears: Array<[boolean, string | undefined]> = []
    const runtime: AuthRefreshRuntime = {
      request: async () => ({
        status: 409,
        data: { code: 'AUTH_REFRESH_RACE' },
      }),
      getExpectedSID: () => bundle.session.sid,
      parseBundle: () => null,
      acceptBundle: () => undefined,
      clear: (synchronizeTabs, bootstrapState) => {
        clears.push([synchronizeTabs, bootstrapState])
      },
      markTransient: () => undefined,
      wait: async (delay) => {
        requestedDelays.push(delay)
      },
    }

    expect(await createRefreshRunner(runtime)()).toEqual({
      kind: 'out_of_sync',
      code: 'AUTH_REFRESH_RACE',
    })
    expect(requestedDelays).toEqual([80, 200, 500])
    expect(clears).toEqual([[false, undefined]])
  })

  test('an unexpected successful response is treated as out of sync', async () => {
    let cleared = false
    const runtime: AuthRefreshRuntime = {
      request: async () => ({ status: 200, data: { success: true } }),
      getExpectedSID: () => bundle.session.sid,
      parseBundle: () => null,
      acceptBundle: () => undefined,
      clear: () => {
        cleared = true
      },
      markTransient: () => undefined,
      wait: async () => undefined,
    }

    expect(await createRefreshRunner(runtime)()).toEqual({
      kind: 'out_of_sync',
      code: 'AUTH_INVALID_REFRESH_RESPONSE',
    })
    expect(cleared).toBe(true)
  })

  test('a refresh response cannot restore credentials after a newer auth operation', async () => {
    let current = true
    let accepted = false
    const runtime: AuthRefreshRuntime = {
      request: async () => {
        current = false
        return { status: 200, data: { success: true, data: bundle } }
      },
      getExpectedSID: () => bundle.session.sid,
      parseBundle: (value) => (isAuthBundle(value) ? value : null),
      acceptBundle: () => {
        accepted = true
      },
      clear: () => undefined,
      markTransient: () => undefined,
      wait: async () => undefined,
      isCurrent: () => current,
    }

    const outcome = await createRefreshRunner(runtime)()

    expect(outcome.kind).toBe('transient_error')
    expect(accepted).toBe(false)
  })

  test('explicit rotations update only the current session', () => {
    useAuthStore.getState().auth.setBundle(bundle)
    applyAuthRotation({
      access_token: 'rotated-token',
      token_type: 'Bearer',
      access_expires_at: bundle.access_expires_at + 60,
      session: { ...bundle.session, last_active_at: 200 },
    })

    expect(useAuthStore.getState().auth.accessToken).toBe('rotated-token')
    expect(useAuthStore.getState().auth.user).toBe(bundle.user)

    expect(() =>
      applyAuthRotation({
        access_token: 'non-bearer-token',
        token_type: 'Custom',
        access_expires_at: bundle.access_expires_at + 120,
        session: bundle.session,
      })
    ).toThrow(/Invalid authentication rotation response/)
    expect(() =>
      applyAuthRotation({
        access_token: 'non-current-token',
        token_type: 'Bearer',
        access_expires_at: bundle.access_expires_at + 120,
        session: { ...bundle.session, current: false },
      })
    ).toThrow(/Invalid authentication rotation response/)

    expect(() =>
      applyAuthRotation({
        access_token: 'wrong-session-token',
        token_type: 'Bearer',
        access_expires_at: bundle.access_expires_at + 120,
        session: { ...bundle.session, sid: 'session-b' },
      })
    ).toThrow(/session mismatch/)
    expect(useAuthStore.getState().auth.accessToken).toBe('rotated-token')
  })

  test('sign-out clears user-scoped query, mutation, and authentication state', () => {
    const queryClient = new QueryClient()
    queryClient.setQueryData(['account', bundle.user.id], {
      username: bundle.user.username,
    })
    queryClient.getMutationCache().build(queryClient, {
      mutationKey: ['account', bundle.user.id, 'update'],
      mutationFn: async () => undefined,
    })
    useAuthStore.getState().auth.setBundle(bundle)
    useAuthStore.getState().auth.setPendingLoginVerification({
      challenge: {
        require_verification: true,
        flow_token: 'pending-flow',
        expires_at: 9999999999,
        methods: [{ method: '2fa', available: true }],
      },
    })

    clearAuthenticatedClientState(queryClient, false)

    expect(queryClient.getQueryCache().getAll().length).toBe(0)
    expect(queryClient.getMutationCache().getAll().length).toBe(0)
    expect(useAuthStore.getState().auth.user).toBe(null)
    expect(useAuthStore.getState().auth.accessToken).toBe(null)
    expect(useAuthStore.getState().auth.session).toBe(null)
    expect(useAuthStore.getState().auth.pendingLoginVerification).toBe(null)
    expect(useAuthStore.getState().auth.bootstrapState).toBe('complete')

    const nextBundle: AuthBundle = {
      ...bundle,
      access_token: 'next-user-token',
      user: { id: 84, username: 'next-user', role: 1 },
      session: { ...bundle.session, sid: 'session-b' },
    }
    useAuthStore.getState().auth.setBundle(nextBundle)
    expect(queryClient.getQueryData(['account', bundle.user.id])).toBe(
      undefined
    )
  })
})

describe('authentication query cache coordination', () => {
  test('a session change clears cached queries and mutations', () => {
    useAuthStore.getState().auth.setBundle(bundle)
    const queryClient = new QueryClient()
    const unsubscribe = subscribeAuthQueryCacheInvalidation(queryClient)
    queryClient.setQueryData(['pricing'], { groups: ['default'] })
    queryClient.getMutationCache().build(queryClient, {
      mutationKey: ['account', 'update'],
      mutationFn: async () => undefined,
    })

    useAuthStore.getState().auth.setBundle({
      ...bundle,
      session: { ...bundle.session, sid: 'session-b' },
    })

    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0)
    unsubscribe()
    queryClient.clear()
  })

  test.each([
    ['user to administrator', ROLE.USER, ROLE.ADMIN],
    ['administrator to user', ROLE.ADMIN, ROLE.USER],
  ])(
    'a same-session %s transition resets mounted queries and refetches them',
    async (_name, initialRole, nextRole) => {
      useAuthStore.getState().auth.setBundle({
        ...bundle,
        user: { ...bundle.user, role: initialRole },
      })
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      queryClient.setQueryData(['pricing'], { groups: ['stale-group'] })
      const queryFn = vi.fn().mockResolvedValue({ groups: ['fresh-group'] })
      const observer = new QueryObserver(queryClient, {
        queryKey: ['pricing'],
        queryFn,
        staleTime: 5 * 60 * 1000,
      })
      const unsubscribeObserver = observer.subscribe(() => undefined)
      const unsubscribeAuth = subscribeAuthQueryCacheInvalidation(queryClient)

      useAuthStore.getState().auth.setUser({ ...bundle.user, role: nextRole })

      await vi.waitFor(() => {
        expect(queryFn).toHaveBeenCalledOnce()
        expect(observer.getCurrentResult().data).toEqual({
          groups: ['fresh-group'],
        })
      })
      unsubscribeAuth()
      unsubscribeObserver()
      queryClient.clear()
    }
  )

  test('a same-session transition within administrator roles keeps the cache', async () => {
    useAuthStore.getState().auth.setBundle({
      ...bundle,
      user: { ...bundle.user, role: ROLE.ADMIN },
    })
    const queryClient = new QueryClient()
    const unsubscribe = subscribeAuthQueryCacheInvalidation(queryClient)
    queryClient.setQueryData(['pricing'], { groups: ['admin-group'] })
    queryClient.getMutationCache().build(queryClient, {
      mutationKey: ['account', 'update'],
      mutationFn: async () => undefined,
    })

    useAuthStore
      .getState()
      .auth.setUser({ ...bundle.user, role: ROLE.SUPER_ADMIN })
    await Promise.resolve()

    expect(queryClient.getQueryData(['pricing'])).toEqual({
      groups: ['admin-group'],
    })
    expect(queryClient.getMutationCache().getAll()).toHaveLength(1)
    unsubscribe()
    queryClient.clear()
  })
})
