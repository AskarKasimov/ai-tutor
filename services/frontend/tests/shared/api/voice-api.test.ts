import { afterEach, expect, it, vi } from 'vitest'
import { fetchStoredAudio, transcribeRecording } from '@/shared/api'
import { validExtensibleWavBlob, validWavBlob } from '../../support/audio'

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

it('fetches stored WAV from an internal API path with cookie credentials', async () => {
  const wav = await validWavBlob().arrayBuffer()
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      new Response(wav, { headers: { 'Content-Type': 'audio/wav' } }),
    )
  vi.stubGlobal('fetch', fetchMock)
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).resolves.toBeInstanceOf(Blob)
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/task-audio/audio-1/file')
  expect(fetchMock.mock.calls[0][1].credentials).toBe('include')
  await expect(
    fetchStoredAudio(
      'https://evil.example/audio.wav',
      new AbortController().signal,
    ),
  ).rejects.toThrow()
  await expect(
    fetchStoredAudio('/task-audio/audio-1/other', new AbortController().signal),
  ).rejects.toThrow()
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response('not wav', { headers: { 'Content-Type': 'text/plain' } }),
      ),
  )
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).rejects.toMatchObject({ kind: 'invalid' })
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response('', { headers: { 'Content-Type': 'audio/wav' } }),
      ),
  )
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).rejects.toMatchObject({ kind: 'invalid' })
})

it('rejects a structurally broken WAV even when the response MIME is audio/wav', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response('not a RIFF file', {
        headers: { 'Content-Type': 'audio/wav' },
      }),
    ),
  )
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).rejects.toMatchObject({ kind: 'invalid' })
})

it('accepts a valid WAVE_FORMAT_EXTENSIBLE PCM file like the backend validator', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(await validExtensibleWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      }),
    ),
  )
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).resolves.toBeInstanceOf(Blob)
})

it('preserves HTTP status and API code for stored-audio failures', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ code: 'AUDIO_STORAGE_UNAVAILABLE' }, { status: 503 }),
      ),
  )
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).rejects.toMatchObject({
    kind: 'http',
    status: 503,
    code: 'AUDIO_STORAGE_UNAVAILABLE',
  })
})

it('classifies a truncated stored-audio response as a repairable network failure', async () => {
  const response = new Response(new Uint8Array([1, 2, 3]), {
    headers: { 'Content-Type': 'audio/wav' },
  })
  vi.spyOn(response, 'blob').mockRejectedValue(new TypeError('truncated body'))
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).rejects.toMatchObject({ kind: 'network' })
})

it('preserves an aborted body read instead of classifying it for repair', async () => {
  const response = new Response(new Uint8Array([1, 2, 3]), {
    headers: { 'Content-Type': 'audio/wav' },
  })
  vi.spyOn(response, 'blob').mockRejectedValue(
    new DOMException('stopped', 'AbortError'),
  )
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
  await expect(
    fetchStoredAudio('/task-audio/audio-1/file', new AbortController().signal),
  ).rejects.toMatchObject({ name: 'AbortError' })
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
})
