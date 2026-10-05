import { afterEach, expect, it, vi } from 'vitest'
import WaveSurfer from 'wavesurfer.js'
import RecordPlugin from 'wavesurfer.js/dist/plugins/record.esm.js'
import { createVoiceWaveform } from './live-waveform'

vi.mock('wavesurfer.js', () => ({ default: { create: vi.fn() } }))
vi.mock('wavesurfer.js/dist/plugins/record.esm.js', () => ({ default: { create: vi.fn() } }))
afterEach(() => vi.resetAllMocks())

function setup() {
  const load = vi.fn().mockResolvedValue(undefined)
  const destroy = vi.fn()
  const onDestroy = vi.fn()
  const pluginDestroy = vi.fn()
  const renderMicStream = vi.fn().mockReturnValue({ onDestroy })
  const plugin = { renderMicStream, destroy: pluginDestroy }
  vi.mocked(RecordPlugin.create).mockReturnValue(plugin as unknown as RecordPlugin)
  vi.mocked(WaveSurfer.create).mockReturnValue({ load, registerPlugin: () => plugin, destroy } as unknown as WaveSurfer)
  return { load, destroy, onDestroy, pluginDestroy, renderMicStream }
}

it('keeps the same renderer when recording starts, stops and starts again', () => {
  const { destroy, onDestroy, renderMicStream } = setup()
  const stream = {} as MediaStream
  const waveform = createVoiceWaveform(document.createElement('div'))
  waveform.setStream(stream)
  waveform.setStream(undefined)
  waveform.setStream(stream)
  expect(WaveSurfer.create).toHaveBeenCalledTimes(1)
  expect(destroy).not.toHaveBeenCalled()
  expect(renderMicStream).toHaveBeenNthCalledWith(1, stream)
  expect(renderMicStream).toHaveBeenNthCalledWith(2, stream)
  expect(onDestroy).toHaveBeenCalledTimes(1)
  waveform.dispose()
  expect(onDestroy).toHaveBeenCalledTimes(2)
  expect(destroy).toHaveBeenCalledTimes(1)
})

it('renders a silent signal without starting microphone monitoring', () => {
  const { load, destroy } = setup()
  const waveform = createVoiceWaveform(document.createElement('div'))
  expect(load).toHaveBeenCalledWith('', [expect.any(Float32Array)], 3)
  expect(Array.from(load.mock.calls[0][1][0])).toEqual(Array(300).fill(0))
  expect(RecordPlugin.create).not.toHaveBeenCalled()
  waveform.dispose()
  expect(destroy).toHaveBeenCalledTimes(1)
})

it('releases visualization resources when microphone monitoring fails', () => {
  const { destroy, renderMicStream, pluginDestroy } = setup()
  renderMicStream.mockImplementation(() => { throw new Error('audio context unavailable') })
  const waveform = createVoiceWaveform(document.createElement('div'))
  expect(() => waveform.setStream({} as MediaStream)).toThrow('audio context unavailable')
  expect(pluginDestroy).toHaveBeenCalledTimes(1)
  waveform.dispose()
  expect(destroy).toHaveBeenCalledTimes(1)
})
