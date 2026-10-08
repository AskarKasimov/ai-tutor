import { afterEach, expect, it, vi } from 'vitest'
import { apiFetch, isMockApi } from './api-fetch'

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
    apiFetch('/api/v1/competency-map'),
    apiFetch('/api/v1/variants', { method: 'POST', body: '{}' }),
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
  expect((await apiFetch('/api/v1/competency-map')).status).toBe(200)
  expect(rotations).toBe(0)
})

it('serializes refresh across independent tabs sharing cookies', async () => {
  vi.resetModules()
  const otherTab = await import('./api-fetch')
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
    body: '{"count":2}',
  })
  expect(await response.json()).toEqual({
    received: '{"count":2}',
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
  const cancelled = apiFetch('/api/v1/variants', { signal: controller.signal })
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
  expect((await apiFetch('/api/v1/variants')).status).toBe(401)
  expect(rotations).toBe(1)
})

it('runs every backend interaction in mock mode without making network requests', async () => {
  vi.stubEnv('VITE_API_MODE', 'mock')
  const fetch = vi.fn(() => {
    throw new Error('Network forbidden')
  })
  vi.stubGlobal('fetch', fetch)
  const post = (path: string, body: unknown) =>
    apiFetch(`/api/v1/${path}`, { method: 'POST', body: JSON.stringify(body) })
  const me = await apiFetch('/api/v1/auth/me')
  expect(me.status).toBe(401)
  expect(
    (
      await post('auth/login', {
        email: 'student@example.com',
        password: 'wrong-password',
      })
    ).status,
  ).toBe(401)
  expect(
    (
      await post('auth/login', {
        email: 'other@example.com',
        password: 'demo-student-2026',
      })
    ).status,
  ).toBe(401)
  expect(
    (await post('auth/login', { email: '', password: 'filled' })).status,
  ).toBe(422)
  expect(
    (await post('auth/login', { email: 'student@example.com', password: '' }))
      .status,
  ).toBe(422)
  expect(
    (await post('auth/register', { email: '', password: '' })).status,
  ).toBe(422)
  const login = await post('auth/login', { email: '', password: '' })
  expect((await login.json()).user.role).toBe('student')
  expect((await apiFetch('/api/v1/auth/me')).status).toBe(200)
  const registered = await post('auth/register', {
    email: 'demo@example.com',
    password: 'a-long-demo-password',
    display_name: 'Демо',
  })
  expect(registered.status).toBe(422)
  const form = new FormData()
  form.append('audio', new Blob(['recording']), 'answer.webm')
  const transcription = await (
    await apiFetch('/api/v1/voice/transcriptions', {
      method: 'POST',
      body: form,
    })
  ).json()
  expect(transcription.id).toBeTruthy()
  expect(transcription.text).toContain('Демонстрационный')
  const wav = await (await post('voice/syntheses', { text: 'Вопрос' })).blob()
  expect(wav.type).toBe('audio/wav')
  expect(wav.size).toBeGreaterThan(44)
  const grade = await (
    await post('assessments/evaluate', {
      transcription_id: transcription.id,
      task_id: 'ml_001',
    })
  ).json()
  expect(grade.score).toBe(2)
  expect(grade.feedback).toHaveLength(3)
  expect(
    (
      await apiFetch('/api/v1/assessments/evaluate', {
        method: 'POST',
        body: JSON.stringify({
          transcription_id: 'missing',
          task_id: 'ml_001',
        }),
      })
    ).status,
  ).toBe(404)
  expect((await post('auth/logout', {})).status).toBe(204)
  expect((await apiFetch('/api/v1/auth/me')).status).toBe(401)
  expect((await post('voice/syntheses', { text: 'Вопрос' })).status).toBe(401)
  expect(
    (
      await post('auth/login', {
        email: 'student@example.com',
        password: 'demo-student-2026',
      })
    ).status,
  ).toBe(200)
  expect((await apiFetch('/api/v1/unknown')).status).toBe(404)
  expect(fetch).not.toHaveBeenCalled()
})

it('passes through requests only for the exact real value', async () => {
  vi.stubEnv('VITE_API_MODE', 'real')
  const response = new Response('{}')
  const fetch = vi.fn().mockResolvedValue(response)
  vi.stubGlobal('fetch', fetch)
  const options = { method: 'POST', credentials: 'include' as const }
  expect(await apiFetch('/api/v1/auth/login', options)).toBe(response)
  expect(fetch).toHaveBeenCalledWith('/api/v1/auth/login', options)
})

it.each([undefined, '', 'mock', 'mok', 'dev', 'REAL', ' real', 'real '])(
  'uses mocks without networking for mode %s',
  async (mode) => {
    vi.stubEnv('VITE_API_MODE', mode)
    const fetch = vi.fn(() => {
      throw new Error('Network forbidden')
    })
    vi.stubGlobal('fetch', fetch)
    expect(isMockApi()).toBe(true)
    const response = await apiFetch('/api/v1/unknown')
    expect([401, 404]).toContain(response.status)
    expect(fetch).not.toHaveBeenCalled()
  },
)

it('cancels mock processing without producing a result or fetching', async () => {
  vi.stubEnv('VITE_API_MODE', 'mock')
  vi.stubGlobal('fetch', vi.fn())
  const controller = new AbortController()
  const pending = apiFetch('/api/v1/voice/syntheses', {
    method: 'POST',
    body: JSON.stringify({ text: 'Вопрос' }),
    signal: controller.signal,
  })
  controller.abort()
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
  expect(fetch).not.toHaveBeenCalled()
})
