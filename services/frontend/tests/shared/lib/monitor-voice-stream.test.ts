import { afterEach, expect, it, vi } from 'vitest'
import { monitorVoiceStream } from '@/shared/lib/monitor-voice-stream'

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function setup() {
  vi.useFakeTimers()
  let amplitude = 0.1
  let phase = 0
  const source = { connect: vi.fn(), disconnect: vi.fn() }
  const analyser = {
    fftSize: 2048,
    disconnect: vi.fn(),
    getFloatTimeDomainData: (samples: Float32Array) => {
      for (let i = 0; i < samples.length; i++)
        samples[i] =
          amplitude * Math.sin((2 * Math.PI * 100 * i) / 48000 + phase)
      phase += 0.7
    },
  }
  const context = {
    state: 'running',
    createMediaStreamSource: vi.fn().mockReturnValue(source),
    createAnalyser: () => analyser,
    resume: vi.fn().mockResolvedValue(undefined),
    close: vi.fn().mockResolvedValue(undefined),
  }
  vi.stubGlobal(
    'AudioContext',
    vi.fn(function () {
      return context
    }),
  )
  const track = { stop: vi.fn() }
  const stream = { getTracks: () => [track] } as unknown as MediaStream
  const onPeaks = vi.fn<(peaks: Float32Array) => void>()
  return {
    context,
    source,
    analyser,
    track,
    stream,
    onPeaks,
    setAmplitude: (value: number) => {
      amplitude = value
    },
  }
}

it('keeps a constant low-frequency signal visible for fifteen seconds regardless of phase', () => {
  const { stream, onPeaks } = setup()
  const stop = monitorVoiceStream(stream, onPeaks)
  vi.advanceTimersByTime(15000)
  expect(onPeaks.mock.calls.length).toBeGreaterThan(300)
  for (const [peaks] of onPeaks.mock.calls)
    expect(peaks[peaks.length - 1]).toBeCloseTo(0.1, 4)
  const [last] = onPeaks.mock.calls.at(-1)!
  expect(last.every((peak) => Math.abs(peak - 0.1) < 0.0001)).toBe(true)
  stop()
})

it('clears speech from the window after three seconds of silence without changing past snapshots', () => {
  const { stream, onPeaks, setAmplitude } = setup()
  const stop = monitorVoiceStream(stream, onPeaks)
  vi.advanceTimersByTime(1000)
  const [speech] = onPeaks.mock.calls.at(-1)!
  setAmplitude(0)
  vi.advanceTimersByTime(3000)
  const [silence] = onPeaks.mock.calls.at(-1)!
  expect(silence.every((peak) => peak === 0)).toBe(true)
  expect(speech[speech.length - 1]).toBeCloseTo(0.1, 4)
  stop()
})

it('stops its timer and audio graph while keeping the recorder microphone tracks alive', () => {
  const { context, source, analyser, track, stream, onPeaks } = setup()
  const stop = monitorVoiceStream(stream, onPeaks)
  vi.advanceTimersByTime(100)
  stop()
  stop()
  const count = onPeaks.mock.calls.length
  vi.advanceTimersByTime(1000)
  expect(onPeaks).toHaveBeenCalledTimes(count)
  expect(source.disconnect).toHaveBeenCalledOnce()
  expect(analyser.disconnect).toHaveBeenCalledOnce()
  expect(context.close).toHaveBeenCalledOnce()
  expect(track.stop).not.toHaveBeenCalled()
})

it('releases the audio context if initialization fails', () => {
  const { context, stream } = setup()
  context.createMediaStreamSource.mockImplementation(() => {
    throw new Error('unavailable')
  })
  expect(() => monitorVoiceStream(stream, vi.fn())).toThrow('unavailable')
  expect(context.close).toHaveBeenCalledOnce()
  expect(vi.getTimerCount()).toBe(0)
})

it('resumes an initially suspended context', () => {
  const { context, stream, onPeaks } = setup()
  context.state = 'suspended'
  const stop = monitorVoiceStream(stream, onPeaks)
  expect(context.resume).toHaveBeenCalledOnce()
  stop()
})
