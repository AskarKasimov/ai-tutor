import { act, renderHook, waitFor } from '@testing-library/react'
import { createQueryClient } from '@/bootstrap/providers'
import { createQueryWrapper } from '../../../support/query-wrapper'
import { afterEach, expect, it, vi } from 'vitest'
import * as audio from '@/shared/lib'
import type { Recording } from '@/shared/lib'
import type { DiagnosticTask } from '@/entities/diagnostic-session'
import { useDiagnosticVoice } from '@/features/diagnostic-session'
import { validWavBlob } from '../../../support/audio'
import { AudioPlaybackError } from '@/shared/lib'

const task: DiagnosticTask = {
  variant_task_id: 't1',
  source_task_id: 'bank1',
  role: 'main',
  competency_id: 'c1',
  competency_name: 'Модели',
  constituent_id: 's1',
  constituent_name: 'Выбор',
  outcome_id: 'o1',
  outcome_name: 'Выбор модели',
  question: 'Вопрос',
  options: [],
  voice_instruction: 'Объясните выбор.',
}
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function renderDiagnosticVoice(
  callback: () => ReturnType<typeof useDiagnosticVoice>,
) {
  const client = createQueryClient()
  const wrapper = createQueryWrapper(client)
  return renderHook(callback, { wrapper })
}

it('releases a microphone granted after the screen has been closed', async () => {
  let allow!: (recording: Recording) => void
  const dispose = vi.fn()
  vi.spyOn(audio, 'startRecording').mockImplementation(
    () =>
      new Promise((resolve) => {
        allow = resolve
      }),
  )
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  let starting!: Promise<void>
  act(() => {
    starting = view.result.current.start()
  })
  view.unmount()
  await act(async () => {
    allow({ stream: {} as MediaStream, dispose, stop: vi.fn() })
    await starting
  })
  expect(dispose).toHaveBeenCalledTimes(1)
})

it('aborts a pending audio submission and releases recording playback on unmount', async () => {
  const dispose = vi.fn()
  vi.spyOn(audio, 'createAudioUrl').mockReturnValue({
    url: 'blob:answer',
    dispose,
  })
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: async () => new Blob(['audio'], { type: 'audio/webm' }),
  })
  let request!: AbortSignal
  const submit = vi.fn((_input, signal: AbortSignal) => {
    request = signal
    return new Promise<never>((_resolve, reject) =>
      signal.addEventListener('abort', () => reject(signal.reason)),
    )
  })
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, submit, vi.fn()),
  )
  await act(async () => {
    await view.result.current.start()
  })
  let stopping!: Promise<void>
  act(() => {
    stopping = view.result.current.stop()
  })
  await waitFor(() => expect(view.result.current.stage).toBe('processing'))
  await waitFor(() => expect(request).toBeDefined())
  view.unmount()
  await stopping
  expect(request.aborted).toBe(true)
  expect(dispose).toHaveBeenCalledTimes(1)
})

it('cancels instruction generation when recording starts and never plays its late response', async () => {
  let speechRequest!: AbortSignal
  let resolveSpeech!: (response: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn((_url, init: RequestInit) => {
      speechRequest = init.signal!
      return new Promise((resolve) => {
        resolveSpeech = resolve
      })
    }),
  )
  vi.spyOn(audio, 'startRecording').mockResolvedValue({
    stream: {} as MediaStream,
    dispose: vi.fn(),
    stop: vi.fn(),
  })
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  let speaking!: Promise<void>
  act(() => {
    speaking = view.result.current.speak()
  })
  await act(async () => {
    await view.result.current.start()
  })
  await act(async () => {
    resolveSpeech(
      Response.json({
        variant_task_id: task.variant_task_id,
        status: 'ready',
        audio_url: '/task-audio/audio-1/file',
      }),
    )
    await speaking
  })
  expect(speechRequest.aborted).toBe(true)
  expect(play).not.toHaveBeenCalled()
  expect(view.result.current.stage).toBe('recording')
})

it('downloads audio again on replay and releases the finished player', async () => {
  const fetchMock = vi.fn(async (url: string) =>
    Promise.resolve(
      url.includes('/current/audio')
        ? Response.json({
            variant_task_id: 't1',
            status: 'ready',
            audio_url: '/task-audio/audio-1/file',
          })
        : new Response(await validWavBlob().arrayBuffer(), {
            headers: { 'Content-Type': 'audio/wav' },
          }),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  let ended!: () => void
  const dispose = vi.fn()
  vi.spyOn(audio, 'playQuestion').mockImplementation(async (_blob, onEnd) => {
    ended = onEnd
    return dispose
  })
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  act(() => {
    ended()
  })
  expect(view.result.current.speech).toBe('idle')
  await act(async () => {
    await view.result.current.speak()
  })
  expect(
    fetchMock.mock.calls.filter(([url]) => url.endsWith('/file')),
  ).toHaveLength(2)
  act(() => {
    ended()
  })
  view.unmount()
  // playQuestion disposes its own URL/player on ended; the hook must drop its handle.
  expect(dispose).not.toHaveBeenCalled()
})

it('reads metadata and the saved file without posting text for TTS', async () => {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      urls.push(url)
      if (url.includes('/current/audio'))
        return Promise.resolve(
          Response.json({
            variant_task_id: 't1',
            status: 'ready',
            audio_url: '/task-audio/audio-1/file',
          }),
        )
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    }),
  )
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(urls).toEqual([
    '/api/v1/diagnostic-sessions/s1/current/audio?variant_task_id=t1',
    '/api/v1/task-audio/audio-1/file',
  ])
  expect(urls.some((url) => url.includes('/voice/syntheses'))).toBe(false)
  expect(play).toHaveBeenCalledTimes(1)
})

it('repairs a missing stored object once and plays the downloaded replacement', async () => {
  const urls: string[] = []
  let fileReads = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      urls.push(url)
      if (url.includes('/current/audio/regenerate'))
        return Response.json({
          variant_task_id: 't1',
          status: 'ready',
          audio_url: '/task-audio/audio-1/file',
        })
      if (url.includes('/current/audio?'))
        return Response.json({
          variant_task_id: 't1',
          status: 'ready',
          audio_url: '/task-audio/audio-1/file',
        })
      if (url.includes('/task-audio/')) {
        fileReads++
        if (fileReads === 1)
          return Response.json({ code: 'AUDIO_NOT_FOUND' }, { status: 404 })
        return new Response(await validWavBlob().arrayBuffer(), {
          headers: { 'Content-Type': 'audio/wav' },
        })
      }
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(urls.filter((url) => url.includes('/regenerate'))).toHaveLength(1)
  expect(fileReads).toBe(2)
  expect(play).toHaveBeenCalledTimes(1)
  expect(view.result.current.speechError).toBe(false)
})

it('repairs once when the stored audio response body is truncated', async () => {
  let fileReads = 0
  let repairs = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/current/audio/regenerate')) {
        repairs++
        return Response.json({
          variant_task_id: 't1',
          status: 'ready',
          audio_url: '/task-audio/audio-1/file',
        })
      }
      if (url.includes('/current/audio?'))
        return Response.json({
          variant_task_id: 't1',
          status: 'ready',
          audio_url: '/task-audio/audio-1/file',
        })
      if (url.includes('/task-audio/')) {
        fileReads++
        if (fileReads === 1) {
          const response = new Response(new Uint8Array([1, 2, 3]), {
            headers: { 'Content-Type': 'audio/wav' },
          })
          vi.spyOn(response, 'blob').mockRejectedValue(
            new TypeError('truncated body'),
          )
          return response
        }
        return new Response(await validWavBlob().arrayBuffer(), {
          headers: { 'Content-Type': 'audio/wav' },
        })
      }
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(() => {})
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(repairs).toBe(1)
  expect(fileReads).toBe(2)
  expect(play).toHaveBeenCalledTimes(1)
})

it('coalesces media decode errors into one repair and does not loop on a second bad file', async () => {
  let regenerations = 0
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/current/audio/regenerate')) {
      regenerations++
      return Response.json({
        variant_task_id: 't1',
        status: 'ready',
        audio_url: '/task-audio/audio-1/file',
      })
    }
    if (url.includes('/current/audio?'))
      return Response.json({
        variant_task_id: 't1',
        status: 'ready',
        audio_url: '/task-audio/audio-1/file',
      })
    if (url.includes('/task-audio/'))
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    throw new Error(`Unexpected request ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  let mediaError!: (error: AudioPlaybackError) => void
  const play = vi
    .spyOn(audio, 'playQuestion')
    .mockImplementation(async (_blob, _onEnd, onError) => {
      mediaError = onError
      return () => {}
    })
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  await act(async () => {
    mediaError(new AudioPlaybackError('decode'))
  })
  await waitFor(() => expect(regenerations).toBe(1))
  await waitFor(() => expect(play).toHaveBeenCalledTimes(2))
  await act(async () => {
    mediaError(new AudioPlaybackError('decode'))
  })
  expect(regenerations).toBe(1)
  expect(view.result.current.speechError).toBe(true)
})

it('does not repair when browser playback is blocked by autoplay policy', async () => {
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/current/audio?'))
      return Response.json({
        variant_task_id: 't1',
        status: 'ready',
        audio_url: '/task-audio/audio-1/file',
      })
    if (url.includes('/task-audio/'))
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    throw new Error(`Unexpected request ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  vi.spyOn(audio, 'playQuestion').mockRejectedValue(
    new DOMException('blocked', 'NotAllowedError'),
  )
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(
    fetch.mock.calls.some(([url]) => String(url).includes('/regenerate')),
  ).toBe(false)
})

it('attempts repair only once when the replacement download is still invalid', async () => {
  let regenerations = 0
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/current/audio/regenerate')) {
      regenerations++
      return Response.json({
        variant_task_id: 't1',
        status: 'ready',
        audio_url: '/task-audio/audio-1/file',
      })
    }
    if (url.includes('/current/audio?'))
      return Response.json({
        variant_task_id: 't1',
        status: 'ready',
        audio_url: '/task-audio/audio-1/file',
      })
    if (url.includes('/task-audio/'))
      return new Response('broken again', {
        headers: { 'Content-Type': 'audio/wav' },
      })
    throw new Error(`Unexpected request ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(() => {})
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(regenerations).toBe(1)
  expect(
    fetch.mock.calls.filter(([url]) => String(url).includes('/task-audio/')),
  ).toHaveLength(2)
  expect(play).not.toHaveBeenCalled()
  expect(view.result.current.speechError).toBe(true)
})

it('repairs a metadata storage error caused by a stale or malformed audio link', async () => {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      urls.push(url)
      if (url.includes('/current/audio/regenerate'))
        return Response.json({
          variant_task_id: 't1',
          status: 'ready',
          audio_url: '/task-audio/audio-1/file',
        })
      if (url.includes('/current/audio?'))
        return Response.json(
          { code: 'AUDIO_STORAGE_UNAVAILABLE' },
          { status: 503 },
        )
      if (url.includes('/task-audio/'))
        return new Response(await validWavBlob().arrayBuffer(), {
          headers: { 'Content-Type': 'audio/wav' },
        })
      throw new Error(`Unexpected request ${init?.method} ${url}`)
    }),
  )
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(() => {})
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(urls.filter((url) => url.includes('/regenerate'))).toHaveLength(1)
  expect(play).toHaveBeenCalledTimes(1)
})

it('retries failed instruction generation after the TTS service recovers', async () => {
  let repairs = 0
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/current/audio/regenerate')) {
      repairs++
      return repairs === 1
        ? Response.json({ code: 'AUDIO_GENERATION_FAILED' }, { status: 503 })
        : Response.json({
            variant_task_id: 't1',
            status: 'ready',
            audio_url: '/task-audio/audio-1/file',
          })
    }
    if (url.includes('/current/audio?'))
      return Response.json({
        variant_task_id: 't1',
        status: 'failed',
        audio_url: null,
      })
    if (url.includes('/task-audio/'))
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    throw new Error(`Unexpected request ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(view.result.current.speechError).toBe(true)
  await act(async () => {
    await view.result.current.speak()
  })
  expect(repairs).toBe(2)
  expect(play).toHaveBeenCalledTimes(1)
  expect(view.result.current.speechError).toBe(false)
})

it('does not repair a stored-audio authorization failure', async () => {
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/current/audio?'))
      return Response.json({
        variant_task_id: 't1',
        status: 'ready',
        audio_url: '/task-audio/audio-1/file',
      })
    if (url.includes('/task-audio/'))
      return Response.json({ code: 'FORBIDDEN' }, { status: 403 })
    throw new Error(`Unexpected request ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(
    fetch.mock.calls.some(([url]) => String(url).includes('/regenerate')),
  ).toBe(false)
  expect(view.result.current.speechError).toBe(true)
})

it('shows a pending hint and plays only after a later ready read', async () => {
  let reads = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/current/audio')) {
        reads++
        return Promise.resolve(
          Response.json(
            reads === 1
              ? { variant_task_id: 't1', status: 'pending', audio_url: null }
              : {
                  variant_task_id: 't1',
                  status: 'ready',
                  audio_url: '/task-audio/audio-1/file',
                },
          ),
        )
      }
      return new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      })
    }),
  )
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(view.result.current.pendingHint).toBe(true)
  expect(view.result.current.speechError).toBe(false)
  expect(play).not.toHaveBeenCalled()
  await act(async () => {
    await view.result.current.speak()
  })
  expect(view.result.current.pendingHint).toBe(false)
  expect(play).toHaveBeenCalledTimes(1)
})

it('shows a clear hint when the task audio was cancelled after a map replacement', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      Response.json({
        variant_task_id: 't1',
        status: 'cancelled',
        audio_url: null,
      }),
    ),
  )
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    await view.result.current.speak()
  })
  expect(view.result.current.cancelledHint).toBe(true)
  expect(view.result.current.speechError).toBe(false)
  expect(view.result.current.pendingHint).toBe(false)
  expect(play).not.toHaveBeenCalled()
})

it('aborts the saved file request if the current task changes before playback', async () => {
  let resolveFile!: (response: Response) => void
  let fileSignal!: AbortSignal
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init: RequestInit) => {
      if (url.includes('/current/audio'))
        return Promise.resolve(
          Response.json({
            variant_task_id: 't1',
            status: 'ready',
            audio_url: '/task-audio/audio-1/file',
          }),
        )
      fileSignal = init.signal!
      return new Promise<Response>((resolve) => {
        resolveFile = resolve
      })
    }),
  )
  const play = vi.spyOn(audio, 'playQuestion').mockResolvedValue(vi.fn())
  let currentTask: DiagnosticTask = task
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', currentTask, vi.fn(), vi.fn()),
  )
  let speaking!: Promise<void>
  act(() => {
    speaking = view.result.current.speak()
  })
  await waitFor(() => expect(fileSignal).toBeDefined())
  currentTask = { ...task, variant_task_id: 't2' }
  view.rerender()
  await waitFor(() => expect(fileSignal.aborted).toBe(true))
  await act(async () => {
    resolveFile(
      new Response(await validWavBlob().arrayBuffer(), {
        headers: { 'Content-Type': 'audio/wav' },
      }),
    )
    await speaking
  })
  expect(play).not.toHaveBeenCalled()
})

it('cancels a rapidly repeated instruction click instead of launching concurrent playback', async () => {
  const requests: AbortSignal[] = []
  const fetch = vi.fn((_url, init: RequestInit) => {
    requests.push(init.signal!)
    return new Promise<never>((_resolve, reject) =>
      init.signal!.addEventListener('abort', () => reject(init.signal!.reason)),
    )
  })
  vi.stubGlobal('fetch', fetch)
  const view = renderDiagnosticVoice(() =>
    useDiagnosticVoice('u1', 's1', task, vi.fn(), vi.fn()),
  )
  await act(async () => {
    const first = view.result.current.speak()
    await view.result.current.speak()
    await first
  })
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(requests[0].aborted).toBe(true)
  expect(view.result.current.speech).toBe('idle')
})
