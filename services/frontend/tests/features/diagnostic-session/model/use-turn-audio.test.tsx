import { act, renderHook, waitFor } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import { DiagnosticApiError } from '@/entities/diagnostic-session'
import {
  DiagnosticDependenciesProvider,
  useTurnAudio,
} from '@/features/diagnostic-session'
import type { DiagnosticDependencies } from '@/features/diagnostic-session'
import {
  encodePcm,
  installPcmPlayback,
  pcmBlock,
  pcmBody,
  pcmStart,
  TestAudioContext,
  TestWorkletNode,
} from '../../../support/pcm-playback'
import { validWavBlob } from '../../../support/audio'

class WavAudio {
  static instances: WavAudio[] = []
  onended: (() => void) | null = null
  onerror = null
  constructor() {
    WavAudio.instances.push(this)
  }
  async play() {}
  pause() {}
  removeAttribute() {}
}
function setup(
  stream = vi.fn(async (...args: [string, string, AbortSignal, string?]) => {
    void args
    return pcmBody()
  }),
) {
  installPcmPlayback()
  WavAudio.instances = []
  vi.stubGlobal('Audio', WavAudio)
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:audio')
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
  const fetchWav = vi.fn(async () => validWavBlob())
  const dependencies: DiagnosticDependencies = {
    diagnostic: {
      startDiagnostic: vi.fn(),
      readDiagnostic: vi.fn(),
      listDiagnostics: vi.fn(),
      answerDiagnostic: vi.fn(),
      readDiagnosticResult: vi.fn(),
      transcribeRecording: vi.fn(),
      fetchTurnAudio: fetchWav,
      fetchTurnAudioStream: stream,
    },
  }
  const wrapper = ({ children }: PropsWithChildren) => (
    <DiagnosticDependenciesProvider value={dependencies}>
      {children}
    </DiagnosticDependenciesProvider>
  )
  return {
    ...renderHook(({ sessionId }) => useTurnAudio(sessionId), {
      wrapper,
      initialProps: { sessionId: 'session' },
    }),
    dependencies,
    stream,
    fetchWav,
  }
}
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
it('requests question after receiving reply, plays it after reply, and replays completed cache', async () => {
  const view = setup()
  const started = vi.fn()
  let playback!: Promise<void>
  act(() => {
    playback = view.result.current.play(
      { id: 'turn', reply: 'Ответ.' },
      started,
    )
  })
  await waitFor(() => expect(view.stream).toHaveBeenCalledTimes(2))
  expect(view.stream.mock.calls.map((call) => call[3])).toEqual([
    'reply',
    'question',
  ])
  expect(TestWorkletNode.instances).toHaveLength(1)
  expect(started).not.toHaveBeenCalled()
  act(() => {
    TestWorkletNode.instances[0].render(10000)
  })
  await waitFor(() => expect(TestWorkletNode.instances).toHaveLength(2))
  expect(started).toHaveBeenCalledOnce()
  await act(async () => {
    TestWorkletNode.instances[1].render(10000)
    await playback
  })
  expect(view.result.current.state.status).toBe('idle')
  act(() => {
    playback = view.result.current.play({ id: 'turn', reply: 'Ответ.' })
  })
  await waitFor(() => expect(WavAudio.instances).toHaveLength(1))
  act(() => WavAudio.instances[0].onended?.())
  await waitFor(() => expect(WavAudio.instances).toHaveLength(2))
  await act(async () => {
    WavAudio.instances[1].onended?.()
    await playback
  })
  expect(view.stream).toHaveBeenCalledTimes(2)
  expect(view.fetchWav).not.toHaveBeenCalled()
})
it.each([404, 406, 501])(
  'falls back to WAV for unavailable endpoint %s',
  async (status) => {
    const view = setup(
      vi.fn(async () => {
        throw new DiagnosticApiError(status)
      }),
    )
    let playback!: Promise<void>
    act(() => {
      playback = view.result.current.play({ id: 'turn', reply: '' })
    })
    await waitFor(() => expect(WavAudio.instances).toHaveLength(1))
    await act(async () => {
      WavAudio.instances[0].onended?.()
      await playback
    })
    expect(view.fetchWav).toHaveBeenCalledOnce()
    expect(view.result.current.state.status).toBe('idle')
  },
)
it.each([500, 502, 0])(
  'surfaces stream failure %s without hidden WAV retry',
  async (status) => {
    const view = setup(
      vi.fn(async () => {
        throw new DiagnosticApiError(status)
      }),
    )
    const started = vi.fn()
    await act(() =>
      view.result.current.play({ id: 'turn', reply: '' }, started),
    )
    expect(view.result.current.state.status).toBe('error')
    expect(view.fetchWav).not.toHaveBeenCalled()
    expect(started).toHaveBeenCalledOnce()
  },
)
it('uses WAV when browser streaming APIs are absent', async () => {
  const view = setup()
  vi.stubGlobal('AudioWorkletNode', undefined)
  let playback!: Promise<void>
  act(() => {
    playback = view.result.current.play({ id: 'turn', reply: '' })
  })
  await waitFor(() => expect(WavAudio.instances).toHaveLength(1))
  await act(async () => {
    WavAudio.instances[0].onended?.()
    await playback
  })
  expect(view.stream).not.toHaveBeenCalled()
})
it('releases onStart once and cancels sound and fetch on stop, replacement, and unmount', async () => {
  const signals: AbortSignal[] = []
  let cancelled = 0
  const view = setup(
    vi.fn(async (_session, _turn, signal) => {
      signals.push(signal)
      return new ReadableStream<Uint8Array>({
        cancel() {
          cancelled++
        },
      })
    }),
  )
  const first = vi.fn()
  let playback!: Promise<void>
  act(() => {
    playback = view.result.current.play({ id: 'first', reply: '' }, first)
  })
  await waitFor(() => expect(TestWorkletNode.instances).toHaveLength(1))
  const second = vi.fn()
  let replacement!: Promise<void>
  act(() => {
    replacement = view.result.current.play({ id: 'second', reply: '' }, second)
  })
  await act(() => playback)
  expect(first).toHaveBeenCalledOnce()
  expect(signals[0].aborted).toBe(true)
  await waitFor(() => expect(TestWorkletNode.instances).toHaveLength(2))
  act(() => view.result.current.stop())
  await act(() => replacement)
  expect(second).toHaveBeenCalledOnce()
  expect(view.result.current.state.status).toBe('idle')
  act(() => {
    playback = view.result.current.play({ id: 'third', reply: '' })
  })
  await waitFor(() => expect(TestWorkletNode.instances).toHaveLength(3))
  view.unmount()
  await playback
  expect(cancelled).toBe(3)
  expect(TestAudioContext.instances.every((context) => context.closed)).toBe(
    true,
  )
})
it('leaves manual play available when autoplay is denied without WAV retry', async () => {
  const view = setup()
  TestAudioContext.denied = true
  const started = vi.fn()
  await act(() => view.result.current.play({ id: 'turn', reply: '' }, started))
  expect(view.result.current.state.status).toBe('idle')
  expect(started).toHaveBeenCalledOnce()
  expect(view.fetchWav).not.toHaveBeenCalled()
})
it('never retries or caches speech after the first sound followed by a stream error', async () => {
  let producer!: ReadableStreamDefaultController<Uint8Array>
  const view = setup(
    vi.fn(
      async () =>
        new ReadableStream<Uint8Array>({
          start(controller) {
            producer = controller
          },
        }),
    ),
  )
  const started = vi.fn()
  let playback!: Promise<void>
  act(() => {
    playback = view.result.current.play({ id: 'turn', reply: '' }, started)
  })
  await waitFor(() => expect(producer).toBeDefined())
  producer.enqueue(encodePcm(pcmStart, pcmBlock(0, 7200)))
  await waitFor(() => {
    act(() => {
      TestWorkletNode.instances[0]?.render()
    })
    expect(started).toHaveBeenCalledOnce()
  })
  producer.enqueue(encodePcm({ type: 'error', code: 'SYNTHESIS_FAILED' }))
  producer.close()
  await act(() => playback)
  expect(view.result.current.state.status).toBe('error')
  expect(view.fetchWav).not.toHaveBeenCalled()
  expect(view.stream).toHaveBeenCalledOnce()
  act(() => {
    playback = view.result.current.play({ id: 'turn', reply: '' })
  })
  await waitFor(() => expect(view.stream).toHaveBeenCalledTimes(2))
  act(() => view.result.current.stop())
  await act(() => playback)
})

it('supports an older injected port without streaming capability', async () => {
  const view = setup()
  delete view.dependencies.diagnostic.fetchTurnAudioStream
  let playback!: Promise<void>
  act(() => {
    playback = view.result.current.play({ id: 'turn', reply: '' })
  })
  await waitFor(() => expect(WavAudio.instances).toHaveLength(1))
  await act(async () => {
    WavAudio.instances[0].onended?.()
    await playback
  })
  expect(view.stream).not.toHaveBeenCalled()
  expect(view.fetchWav).toHaveBeenCalledOnce()
})
it('cancels old session playback and resets the available audio state', async () => {
  const view = setup(vi.fn(async () => new ReadableStream<Uint8Array>()))
  const started = vi.fn()
  let playback!: Promise<void>
  act(() => {
    playback = view.result.current.play({ id: 'turn', reply: '' }, started)
  })
  await waitFor(() => expect(TestWorkletNode.instances).toHaveLength(1))
  view.rerender({ sessionId: 'another-session' })
  await act(() => playback)
  expect(TestAudioContext.instances[0].closed).toBe(true)
  expect(started).toHaveBeenCalledOnce()
  expect(view.result.current.state).toEqual({ turnId: null, status: 'idle' })
})
