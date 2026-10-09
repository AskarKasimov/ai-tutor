import { withAuthRefreshLock } from './auth-refresh-lock'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
let recovery: Promise<Response> | undefined
let mockHandler:
  ((url: string, options?: RequestInit) => Promise<Response>) | undefined

export function configureMockApiHandler(
  handler: (url: string, options?: RequestInit) => Promise<Response>,
) {
  mockHandler = handler
}

export function isMockApi() {
  return import.meta.env.VITE_API_MODE !== 'real'
}

function recoverSession(): Promise<Response> {
  if (!recovery) {
    const signal = AbortSignal.timeout(30_000)
    recovery = withAuthRefreshLock(
      `auth-refresh:${apiBase}`,
      signal,
      async () => {
        const options = { credentials: 'include' as const, signal }
        const current = await fetch(`${apiBase}/auth/me`, options)
        if (current.status !== 401) return current
        return fetch(`${apiBase}/auth/refresh`, { ...options, method: 'POST' })
      },
    ).finally(() => {
      recovery = undefined
    })
  }
  return recovery!
}

export async function apiFetch(
  url: string,
  options?: RequestInit,
): Promise<Response> {
  if (isMockApi()) {
    if (!mockHandler) throw new Error('Demo API handler is not configured')
    return mockHandler(url, options)
  }
  const response = await fetch(url, options)
  if (
    response.status !== 401 ||
    !url.startsWith(`${apiBase}/`) ||
    /\/auth\/(login|register|teacher|logout|refresh)(?:\?|$)/.test(url)
  )
    return response
  options?.signal?.throwIfAborted()
  const renewed = await recoverSession()
  options?.signal?.throwIfAborted()
  if (!renewed.ok) return renewed.status === 401 ? response : renewed.clone()
  return fetch(url, options)
}
