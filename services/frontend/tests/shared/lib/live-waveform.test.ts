import { afterEach, expect, it, vi } from 'vitest'
import WaveSurfer from 'wavesurfer.js'
import { createVoiceWaveform } from '@/shared/lib'
import { monitorVoiceStream } from '@/shared/lib/monitor-voice-stream'

vi.mock('wavesurfer.js', () => ({ default: { create: vi.fn() } }))
vi.mock('@/shared/lib/monitor-voice-stream', () => ({
  monitorVoiceStream: vi.fn(),
}))
afterEach(() => vi.resetAllMocks())

function setup() {
  const load = vi.fn().mockResolvedValue(undefined)
  const loadBlob = vi.fn().mockResolvedValue(undefined)
  const setOptions = vi.fn()
  const destroy = vi.fn()
  const stopMonitor = vi.fn()
  vi.mocked(monitorVoiceStream).mockReturnValue(stopMonitor)
  vi.mocked(WaveSurfer.create).mockReturnValue({
    load,
    loadBlob,
    setOptions,
    destroy,
  } as unknown as WaveSurfer)
  return {
    load,
    loadBlob,
    setOptions,
    destroy,
    stopMonitor,
  }
}

it('keeps the same renderer when recording starts, stops and starts again', () => {
  const { destroy, stopMonitor } = setup()
  const stream = {} as MediaStream
  const waveform = createVoiceWaveform(document.createElement('div'))
  waveform.setStream(stream)
  waveform.setStream(undefined)
  waveform.setStream(stream)
  expect(WaveSurfer.create).toHaveBeenCalledTimes(1)
  expect(destroy).not.toHaveBeenCalled()
  expect(monitorVoiceStream).toHaveBeenNthCalledWith(
    1,
    stream,
    expect.any(Function),
  )
  expect(monitorVoiceStream).toHaveBeenNthCalledWith(
    2,
    stream,
    expect.any(Function),
  )
  expect(stopMonitor).toHaveBeenCalledTimes(1)
  waveform.dispose()
  expect(stopMonitor).toHaveBeenCalledTimes(2)
  expect(destroy).toHaveBeenCalledTimes(1)
})

it('renders a silent signal without starting microphone monitoring', () => {
  const { load, destroy } = setup()
  const waveform = createVoiceWaveform(document.createElement('div'))
  expect(load).toHaveBeenCalledWith('', [expect.any(Float32Array)], 3)
  expect(Array.from(load.mock.calls[0][1][0])).toEqual(Array(300).fill(0))
  expect(monitorVoiceStream).not.toHaveBeenCalled()
  waveform.dispose()
  expect(destroy).toHaveBeenCalledTimes(1)
})

it('keeps disposal safe when microphone monitoring fails', () => {
  const { destroy } = setup()
  vi.mocked(monitorVoiceStream).mockImplementation(() => {
    throw new Error('audio context unavailable')
  })
  const waveform = createVoiceWaveform(document.createElement('div'))
  expect(() => waveform.setStream({} as MediaStream)).toThrow(
    'audio context unavailable',
  )
  waveform.dispose()
  expect(destroy).toHaveBeenCalledTimes(1)
})

it('amplifies live microphone bars and stretches a finished recording', () => {
  const { load, loadBlob, setOptions } = setup()
  const waveform = createVoiceWaveform(document.createElement('div'), 48, 2.5)
  expect(WaveSurfer.create).toHaveBeenCalledWith(
    expect.objectContaining({ height: 48, barHeight: 2.5 }),
  )
  waveform.setStream({} as MediaStream)
  expect(setOptions).toHaveBeenLastCalledWith({
    normalize: false,
    barHeight: 2.5,
  })
  const peaks = new Float32Array([0, 0.1, 0.25])
  vi.mocked(monitorVoiceStream).mock.calls[0][1](peaks)
  expect(load).toHaveBeenLastCalledWith('', [peaks], 3)
  // A recording is not redrawn while the microphone is still live.
  const audio = new Blob(['voice'])
  waveform.setAudio(audio)
  expect(loadBlob).not.toHaveBeenCalled()
  waveform.setStream(undefined)
  waveform.setAudio(audio)
  expect(setOptions).toHaveBeenLastCalledWith({ normalize: true, barHeight: 1 })
  expect(loadBlob).toHaveBeenCalledWith(audio)
  waveform.dispose()
})
