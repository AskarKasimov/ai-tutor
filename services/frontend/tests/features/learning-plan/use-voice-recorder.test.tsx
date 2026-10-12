import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { useVoiceRecorder } from '@/features/learning-plan'

class DeferredRecorder {
  static last: DeferredRecorder | undefined
  static isTypeSupported() {
    return true
  }
  state: RecordingState = 'inactive'
  mimeType = 'audio/webm'
  ondataavailable: ((event: BlobEvent) => void) | null = null
  onstop: ((event: Event) => void) | null = null
  constructor(readonly stream: MediaStream) {
    DeferredRecorder.last = this
  }
  start() {
    this.state = 'recording'
  }
  stop() {
    this.state = 'inactive'
  }
}

function fakeStream() {
  const track = { stop: vi.fn() }
  return {
    value: { getTracks: () => [track] } as unknown as MediaStream,
    track,
  }
}

function installMedia(getUserMedia: () => Promise<MediaStream>) {
  vi.stubGlobal('MediaRecorder', DeferredRecorder)
  Object.defineProperty(navigator, 'mediaDevices', {
    configurable: true,
    value: { getUserMedia },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('stops a permission result that arrives after switching exercises', async () => {
  let resolveMedia!: (stream: MediaStream) => void
  const pending = new Promise<MediaStream>((resolve) => {
    resolveMedia = resolve
  })
  installMedia(() => pending)
  const recorded = vi.fn()
  const view = renderHook(
    ({ identity }) => useVoiceRecorder(identity, recorded),
    { initialProps: { identity: 'session:exercise-a' } },
  )
  let request!: Promise<void>
  act(() => {
    request = view.result.current.start()
  })
  view.rerender({ identity: 'session:exercise-b' })
  const media = fakeStream()
  await act(async () => {
    resolveMedia(media.value)
    await request
  })

  expect(media.track.stop).toHaveBeenCalledOnce()
  expect(view.result.current.stream).toBeUndefined()
  expect(view.result.current.recording).toBe(false)
  expect(view.result.current.requesting).toBe(false)
  expect(recorded).not.toHaveBeenCalled()
})

it('ignores a delayed recorder stop callback from the previous exercise', async () => {
  const media = fakeStream()
  installMedia(async () => media.value)
  const recorded = vi.fn()
  const view = renderHook(
    ({ identity }) => useVoiceRecorder(identity, recorded),
    { initialProps: { identity: 'session:exercise-a' } },
  )
  await act(async () => view.result.current.start())
  const oldRecorder = DeferredRecorder.last
  expect(oldRecorder).toBeDefined()
  expect(view.result.current.recording).toBe(true)
  act(() => view.result.current.stop())
  view.rerender({ identity: 'session:exercise-b' })
  oldRecorder?.ondataavailable?.({ data: new Blob(['old audio']) } as BlobEvent)
  act(() => oldRecorder?.onstop?.(new Event('stop')))

  expect(media.track.stop).toHaveBeenCalled()
  expect(view.result.current.stream).toBeUndefined()
  expect(view.result.current.recording).toBe(false)
  expect(recorded).not.toHaveBeenCalled()
})
