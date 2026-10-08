import { afterEach, expect, it, vi } from 'vitest'
import { synthesizeQuestion, transcribeRecording } from '@/shared/api'

afterEach(() => vi.unstubAllGlobals())

it('uploads recording as multipart file and reads actual transcription', async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ id: 'tr-1', text: 'Мой реальный ответ' }),
  })
  vi.stubGlobal('fetch', fetchMock)
  const signal = new AbortController().signal
  expect(
    await transcribeRecording(
      new Blob(['audio'], { type: 'audio/webm' }),
      signal,
    ),
  ).toEqual({ id: 'tr-1', text: 'Мой реальный ответ' })
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/v1/voice/transcriptions')
  expect(init.body.get('audio').name).toBe('answer.webm')
  expect(init.headers).toBeUndefined()
  expect(init.credentials).toBe('include')
  expect(init.signal).toBeInstanceOf(AbortSignal)
  expect(init.signal.aborted).toBe(false)
})

it('posts text as JSON and returns generated audio', async () => {
  const wav = new Blob(['wav'], { type: 'audio/wav' })
  const fetchMock = vi
    .fn()
    .mockResolvedValue({ ok: true, blob: async () => wav })
  vi.stubGlobal('fetch', fetchMock)
  expect(await synthesizeQuestion('Вопрос', new AbortController().signal)).toBe(
    wav,
  )
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/voice/syntheses')
  expect(fetchMock.mock.calls[0][1].credentials).toBe('include')
  expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
    text: 'Вопрос',
  })
})

it('rejects service failures and empty transcriptions instead of supplying a mock', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 422 }))
  await expect(
    transcribeRecording(new Blob(['a']), new AbortController().signal),
  ).rejects.toThrow('422')
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ id: 'tr-1', text: ' ' }),
    }),
  )
  await expect(
    transcribeRecording(new Blob(['a']), new AbortController().signal),
  ).rejects.toThrow('empty')
})

it('sends production requests through the same origin without build-time env', async () => {
  vi.stubEnv('DEV', false)
  vi.stubEnv('VITE_API_BASE_URL', '')
  vi.resetModules()
  try {
    const api = await import('@/shared/api')
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ id: 'tr-1', text: 'Ответ' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    await expect(
      api.transcribeRecording(
        new Blob(['audio']),
        new AbortController().signal,
      ),
    ).resolves.toEqual({ id: 'tr-1', text: 'Ответ' })
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/voice/transcriptions')
  } finally {
    vi.unstubAllEnvs()
    vi.resetModules()
  }
})

it('preserves unauthorized status for a useful session error', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 401 }))
  await expect(
    transcribeRecording(new Blob(['a']), new AbortController().signal),
  ).rejects.toMatchObject({ status: 401 })
  await expect(
    synthesizeQuestion('Вопрос', new AbortController().signal),
  ).rejects.toMatchObject({ status: 401 })
})
