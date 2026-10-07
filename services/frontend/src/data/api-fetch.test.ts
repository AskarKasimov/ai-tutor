import { afterEach, expect, it, vi } from 'vitest'
import { apiFetch, isMockApi } from './api-fetch'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
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
