import { afterEach, expect, it, vi } from 'vitest'
import { apiFetch } from '@/shared/api'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

it('recovers concurrent protected requests with one refresh', async () => {
  let valid = false
  let rotations = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) {
        rotations++
        valid = true
        return Response.json({})
      }
      return valid
        ? Response.json({ restored: true })
        : new Response(null, { status: 401 })
    }),
  )
  const results = await Promise.all([
    apiFetch('/api/v1/auth/me'),
    apiFetch('/api/v1/subjects/subject%3Ademo-a/competency-map'),
    apiFetch('/api/v1/variants', {
      method: 'POST',
      body: JSON.stringify({ subject_id: 'subject:demo-a' }),
    }),
  ])
  expect(results.map((response) => response.status)).toEqual([200, 200, 200])
  expect(rotations).toBe(1)
})

it('rechecks access before rotating after another tab has refreshed', async () => {
  let first = true
  let rotations = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) rotations++
      if (first) {
        first = false
        return new Response(null, { status: 401 })
      }
      return Response.json({ restored: true })
    }),
  )
  expect(
    (await apiFetch('/api/v1/subjects/subject%3Ademo-a/competency-map')).status,
  ).toBe(200)
  expect(rotations).toBe(0)
})

it('serializes refresh across independent tabs sharing cookies', async () => {
  vi.resetModules()
  const otherTab = await import('@/shared/api')
  let queue = Promise.resolve()
  vi.stubGlobal('navigator', {
    locks: {
      request: (
        _name: string,
        _options: unknown,
        action: () => Promise<Response>,
      ) => {
        const result = queue.then(action)
        queue = result.then(
          () => undefined,
          () => undefined,
        )
        return result
      },
    },
  })
  let valid = false
  let rotations = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) {
        rotations++
        // Model the backend's replay detection: a second rotation revokes access.
        if (rotations > 1) {
          valid = false
          return new Response(null, { status: 401 })
        }
        valid = true
        return Response.json({})
      }
      return new Response(null, { status: valid ? 200 : 401 })
    }),
  )
  const results = await Promise.all([
    apiFetch('/api/v1/auth/me'),
    otherTab.apiFetch('/api/v1/auth/me'),
  ])
  expect(results.map((response) => response.status)).toEqual([200, 200])
  expect(rotations).toBe(1)
})

it('preserves the body and credentials of a protected POST on retry', async () => {
  let valid = false
  const fetch = vi.fn(async (url: string, options?: RequestInit) => {
    if (url.endsWith('/auth/refresh')) {
      expect(options).toMatchObject({ method: 'POST', credentials: 'include' })
      valid = true
      return Response.json({})
    }
    if (url.endsWith('/variants') && valid) {
      return Response.json({
        received: options?.body,
        credentials: options?.credentials,
      })
    }
    return new Response(null, { status: 401 })
  })
  vi.stubGlobal('fetch', fetch)
  const response = await apiFetch('/api/v1/variants', {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({ subject_id: 'subject:demo-a' }),
  })
  expect(await response.json()).toEqual({
    received: '{"subject_id":"subject:demo-a"}',
    credentials: 'include',
  })
})

it('does not replay a cancelled request while another caller finishes recovery', async () => {
  let release!: () => void
  let started!: () => void
  const refreshing = new Promise<void>((resolve) => {
    started = resolve
  })
  let valid = false
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) {
        started()
        await new Promise<void>((resolve) => {
          release = resolve
        })
        valid = true
        return Response.json({})
      }
      return new Response(null, { status: valid ? 200 : 401 })
    }),
  )
  const controller = new AbortController()
  const cancelled = apiFetch('/api/v1/variants?subject_id=subject%3Ademo-a', {
    signal: controller.signal,
  })
  const rejection = expect(cancelled).rejects.toMatchObject({
    name: 'AbortError',
  })
  const active = apiFetch('/api/v1/auth/me')
  await refreshing
  controller.abort()
  release()
  await rejection
  expect((await active).status).toBe(200)
})

it('keeps a technical refresh failure distinct from a signed-out session', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async (url: string) =>
        new Response(null, {
          status: url.endsWith('/auth/refresh') ? 503 : 401,
        }),
    ),
  )
  expect((await apiFetch('/api/v1/auth/me')).status).toBe(503)
})

it('returns unauthorized when refresh is invalid without retrying indefinitely', async () => {
  let rotations = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) rotations++
      return new Response(null, { status: 401 })
    }),
  )
  expect((await apiFetch('/api/v1/auth/me')).status).toBe(401)
  expect(rotations).toBe(1)
})

it.each(['login', 'register', 'logout', 'refresh'])(
  'does not recover unauthorized auth/%s requests',
  async (path) => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 401 }))
    vi.stubGlobal('fetch', fetch)
    expect(
      (await apiFetch(`/api/v1/auth/${path}`, { method: 'POST' })).status,
    ).toBe(401)
    expect(fetch).toHaveBeenCalledTimes(1)
  },
)

it('retries a protected request only once even if renewed access is rejected', async () => {
  let rotations = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/refresh')) {
        rotations++
        return Response.json({})
      }
      return new Response(null, { status: 401 })
    }),
  )
  expect(
    (await apiFetch('/api/v1/variants?subject_id=subject%3Ademo-a')).status,
  ).toBe(401)
  expect(rotations).toBe(1)
})

it('keeps concurrent refreshes and portal headers isolated by session portal', async () => {
  let releaseStudent!: (response: Response) => void
  const studentRead = new Promise<Response>((resolve) => {
    releaseStudent = resolve
  })
  const calls: Array<{ url: string; portal: string | null }> = []
  const fetchMock = vi.fn(async (url: string, options?: RequestInit) => {
    const portal = new Headers(options?.headers).get('X-AI-Tutor-Portal')
    calls.push({ url, portal })
    if (url.endsWith('/auth/me'))
      return portal === 'admin' ? Response.json({}) : studentRead
    if (url.endsWith('/auth/refresh')) return Response.json({})
    const attempts = calls.filter((call) => call.url === url).length
    return new Response(null, { status: attempts === 1 ? 401 : 200 })
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState({}, '', '/')
  const student = apiFetch('/api/v1/learn/protected-student')
  await vi.waitFor(() =>
    expect(calls.some((call) => call.url.endsWith('/auth/me'))).toBe(true),
  )
  window.history.replaceState({}, '', '/admin/courses')
  const admin = apiFetch('/api/v1/exam/protected-admin')
  await expect(admin).resolves.toMatchObject({ status: 200 })
  releaseStudent(Response.json({}))
  await expect(student).resolves.toMatchObject({ status: 200 })
  expect(
    calls
      .filter((call) => call.url.endsWith('/auth/me'))
      .map((call) => call.portal)
      .sort(),
  ).toEqual(['admin', null])
})
