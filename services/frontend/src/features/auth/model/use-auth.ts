import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useAuthDependencies } from './dependencies-context'
import type { AuthInput } from '@/entities/user'
import {
  adoptCurrentUser,
  captureSession,
  isCurrentSession,
  registerSessionRequest,
  replaceSession,
} from '@/entities/user'
import { userQueryKeys } from '@/entities/user'
import { isAdminPortal } from '@/shared/api'
import type { User } from '@/entities/user'

export type { AuthInput } from '@/entities/user'

export function useCurrentUserQuery() {
  const { auth } = useAuthDependencies()
  const cache = useQueryClient()
  const authKey = userQueryKeys.authForPortal()
  // QueryClient-local auth epoch guards shared auth data without changing its public key.
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  return useQuery<User | null, Error>({
    queryKey: authKey,
    retry: false,
    staleTime: 60_000,
    queryFn: async ({ signal }) => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        const user = await auth.readCurrentUser(
          AbortSignal.any([signal, controller.signal]),
        )
        if (!isCurrentSession(cache, token))
          return cache.getQueryData(authKey) ?? null
        if (!(await adoptCurrentUser(cache, user, token)))
          return cache.getQueryData(authKey) ?? null
        return user
      } finally {
        unregister()
      }
    },
  })
}

export function useAuthenticateMutation() {
  const { auth } = useAuthDependencies()
  const cache = useQueryClient()
  const adminPortal = isAdminPortal()
  const authKey = userQueryKeys.authForPortal(adminPortal)
  return useMutation({
    mutationKey: [
      ...userQueryKeys.authenticate,
      adminPortal ? 'admin' : 'student',
    ],
    gcTime: 0,
    onMutate: async () => {
      const token = captureSession(cache)
      await cache.cancelQueries({ queryKey: authKey })
      return token
    },
    mutationFn: async (input: AuthInput) => {
      const controller = new AbortController()
      const token = captureSession(cache)
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        return await auth.authenticate(input, controller.signal)
      } finally {
        unregister()
      }
    },
    onSuccess: async (user, _input, token) => {
      if (isCurrentSession(cache, token))
        await replaceSession(cache, user, token.portal)
    },
    retry: false,
  })
}

export function useLogoutMutation() {
  const { auth } = useAuthDependencies()
  const cache = useQueryClient()
  const adminPortal = isAdminPortal()
  return useMutation({
    mutationKey: [...userQueryKeys.logout, adminPortal ? 'admin' : 'student'],
    gcTime: 0,
    onMutate: () => captureSession(cache),
    mutationFn: async () => {
      const token = captureSession(cache)
      const controller = new AbortController()
      const unregister = registerSessionRequest(cache, token, controller)
      try {
        await auth.logout(controller.signal)
      } finally {
        unregister()
      }
    },
    onSuccess: async (_result, _input, token) => {
      if (isCurrentSession(cache, token))
        await replaceSession(cache, null, token.portal)
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
