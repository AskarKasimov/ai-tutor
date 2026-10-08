import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { authenticate, logout, readCurrentUser } from './auth-api'
import type { AuthInput } from './auth-api'
import {
  adoptCurrentUser,
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from './session-lifecycle'
import { queryKeys } from './query-keys'
import type { User } from '../shared/domain'

export type { AuthInput } from './auth-api'

export function useCurrentUserQuery() {
  const cache = useQueryClient()
  // QueryClient-local auth epoch guards shared auth data without changing its public key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useQuery<User | null, Error>({
    queryKey: queryKeys.auth,
    retry: false,
    staleTime: 60_000,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const user = await readCurrentUser(
          AbortSignal.any([signal, controller.signal]),
        )
        if (!isCurrentSession(cache, token))
          return cache.getQueryData(queryKeys.auth) ?? null
        if (!(await adoptCurrentUser(cache, user, token)))
          return cache.getQueryData(queryKeys.auth) ?? null
        return user
      } finally {
        unregister()
      }
    },
  })
}

export function useAuthenticateMutation() {
  const cache = useQueryClient()
  return useMutation({
    mutationKey: queryKeys.authenticate,
    gcTime: 0,
    onMutate: async () => {
      const token = captureSession(cache)
      await cache.cancelQueries({ queryKey: queryKeys.auth })
      return token
    },
    mutationFn: async (input: AuthInput) => {
      const controller = new AbortController()
      const token = captureSession(cache)
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        return await authenticate(input, controller.signal)
      } finally {
        unregister()
      }
    },
    onSuccess: async (user, _input, token) => {
      if (isCurrentSession(cache, token)) await replaceSession(cache, user)
    },
    retry: false,
  })
}

export function useLogoutMutation() {
  const cache = useQueryClient()
  return useMutation({
    mutationKey: queryKeys.logout,
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async () => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        await logout(controller.signal)
      } finally {
        unregister()
      }
    },
    onSuccess: async (_result, _input, token) => {
      if (isCurrentSession(cache, token)) await replaceSession(cache, null)
    },
    retry: false,
  })
}

export function useAuth() {
  const { t } = useTranslation()
  const current = useCurrentUserQuery()
  const authenticateMutation = useAuthenticateMutation()
  const logoutMutation = useLogoutMutation()
  return {
    user: current.data,
    checkingSession: current.isPending,
    sessionError: current.error ? new Error(t('auth.networkError')) : null,
    retrySession: current.refetch,
    authenticate: authenticateMutation,
    logout: logoutMutation,
  }
}
