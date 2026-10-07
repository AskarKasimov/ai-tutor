import { apiFetch } from './api-fetch'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { User } from '../shared/domain'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
export type AuthInput = { mode: 'login' | 'register'; email: string; password: string; display_name?: string }

export function useAuth() {
  const { t } = useTranslation()
  const cache = useQueryClient()
  const key = ['auth', 'me']
  async function setSession(user: User | null) {
    // Data and authorization errors from the previous session must not survive a new login.
    const sessionQueries = { predicate: (query: { queryKey: readonly unknown[] }) => query.queryKey[0] !== 'auth' }
    await cache.cancelQueries(sessionQueries)
    cache.removeQueries(sessionQueries)
    cache.setQueryData(key, user)
  }
  const current = useQuery<User | null>({
    queryKey: key,
    retry: false,
    staleTime: 60_000,
    queryFn: async ({ signal }) => {
      const response = await apiFetch(`${apiBase}/auth/me`, { credentials: 'include', signal: AbortSignal.any([signal, AbortSignal.timeout(30_000)]) })
      if (response.status === 401) return null
      if (!response.ok) throw new Error(t('auth.networkError'))
      return response.json()
    },
  })
  const authenticate = useMutation({
    onMutate: () => cache.cancelQueries({ queryKey: key }),
    mutationFn: async ({ mode, ...body }: AuthInput): Promise<User> => {
      let response: Response
      try {
        response = await apiFetch(`${apiBase}/auth/${mode}`, {
          method: 'POST', credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body), signal: AbortSignal.timeout(30_000),
        })
      } catch { throw new Error(t('auth.networkError')) }
      const data = await response.json().catch(() => null)
      if (!response.ok) {
        const message = data?.details?.map((detail: { message: string }) => detail.message).join(' ') || data?.message
        throw new Error(typeof message === 'string' ? message : t('auth.networkError'))
      }
      if (!data?.user?.email) throw new Error(t('auth.networkError'))
      return data.user
    },
    onSuccess: setSession,
  })
  const logout = useMutation({
    mutationFn: async () => {
      const response = await apiFetch(`${apiBase}/auth/logout`, { method: 'POST', credentials: 'include', signal: AbortSignal.timeout(30_000) })
      if (!response.ok) throw new Error(t('auth.networkError'))
    },
    onSuccess: () => setSession(null),
  })
  return { user: current.data, checkingSession: current.isPending, sessionError: current.error, retrySession: current.refetch, authenticate, logout }
}
