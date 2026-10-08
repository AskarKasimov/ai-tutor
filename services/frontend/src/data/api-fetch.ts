import { withAuthRefreshLock } from '../platform/auth-refresh-lock'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
let recovery: Promise<Response> | undefined

export function isMockApi() {
  return import.meta.env.VITE_API_MODE !== 'real'
}

function recoverSession(): Promise<Response> {
  if (!recovery) {
    // A caller's cancellation must not interrupt shared token rotation.
    const signal = AbortSignal.timeout(30_000)
    recovery = withAuthRefreshLock(
      `auth-refresh:${apiBase}`,
      signal,
      async () => {
        const options = { credentials: 'include' as const, signal }
        // A different tab (or an earlier request) may have already renewed access
        // while this request was receiving its 401 or waiting for the lock.
        const current = await fetch(`${apiBase}/auth/me`, options)
        if (current.status !== 401) return current
        return fetch(`${apiBase}/auth/refresh`, { ...options, method: 'POST' })
      },
    ).finally(() => {
      recovery = undefined
    })
  }
  return recovery
}

// All backend requests cross this boundary. Mocks never fall through to fetch.
export async function apiFetch(
  url: string,
  options?: RequestInit,
): Promise<Response> {
  if (isMockApi()) {
    const { mockApiFetch } = await import('../mocks/api')
    return mockApiFetch(url, options)
  }
  const response = await fetch(url, options)
  if (
    response.status !== 401 ||
    !url.startsWith(`${apiBase}/`) ||
    /\/auth\/(login|register|logout|refresh)(?:\?|$)/.test(url)
  )
    return response

  options?.signal?.throwIfAborted()
  const renewed = await recoverSession()
  options?.signal?.throwIfAborted()
  if (!renewed.ok) return renewed.status === 401 ? response : renewed.clone()
  // Bypass apiFetch to bound retries, including non-idempotent protected calls:
  // backend authentication rejected the first request before its handler ran.
  return fetch(url, options)
}
