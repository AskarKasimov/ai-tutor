import { withAuthRefreshLock } from './auth-refresh-lock'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(
  /\/$/,
  '',
)
const recoveries = new Map<string, Promise<Response>>()
// The admin panel lives under /admin and keeps its own session: its API
// requests carry the portal header, so the backend uses separate cookies and
// the student site in another tab stays signed in.
export function isAdminPortal() {
  return globalThis.location?.pathname.startsWith('/admin') ?? false
}

function withPortal(
  options: RequestInit | undefined,
  adminPortal: boolean,
): RequestInit | undefined {
  if (!adminPortal) return options
  const headers = new Headers(options?.headers)
  headers.set('X-AI-Tutor-Portal', 'admin')
  return { ...options, headers }
}

function recoverSession(adminPortal: boolean): Promise<Response> {
  const portal = adminPortal ? 'admin' : 'student'
  let recovery = recoveries.get(portal)
  if (!recovery) {
    const signal = AbortSignal.timeout(30_000)
    recovery = withAuthRefreshLock(
      `auth-refresh:${apiBase}:${adminPortal ? 'admin' : 'student'}`,
      signal,
      async () => {
        const options = withPortal(
          { credentials: 'include', signal },
          adminPortal,
        )
        const current = await fetch(`${apiBase}/auth/me`, options)
        if (current.status !== 401) return current
        return fetch(`${apiBase}/auth/refresh`, { ...options, method: 'POST' })
      },
    ).finally(() => {
      recoveries.delete(portal)
    })
    recoveries.set(portal, recovery)
  }
  return recovery
}

// Same-origin static files bundled with the app.
export function fetchStaticAsset(path: string): Promise<Response> {
  return fetch(`${import.meta.env.BASE_URL}${path.replace(/^\//, '')}`)
}

export async function apiFetch(
  url: string,
  requestOptions?: RequestInit,
): Promise<Response> {
  const adminPortal = isAdminPortal()
  const options = url.startsWith(`${apiBase}/`)
    ? withPortal(requestOptions, adminPortal)
    : requestOptions
  const response = await fetch(url, options)
  if (
    response.status !== 401 ||
    !url.startsWith(`${apiBase}/`) ||
    /\/auth\/(login|register|teacher|logout|refresh)(?:\?|$)/.test(url)
  )
    return response
  options?.signal?.throwIfAborted()
  const renewed = await recoverSession(adminPortal)
  options?.signal?.throwIfAborted()
  if (!renewed.ok) return renewed.status === 401 ? response : renewed.clone()
  return fetch(url, options)
}
