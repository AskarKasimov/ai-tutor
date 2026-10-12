import { afterEach, expect, it, vi } from 'vitest'
import { prepareSpeech } from '@/shared/lib'

afterEach(() => vi.unstubAllGlobals())

function stubDecoder(duration: number) {
  const rendered = new Float32Array(Math.ceil(duration * 16_000)).fill(0.5)
  class Context {
    constructor(
      readonly channels: number,
      readonly length: number,
      readonly sampleRate: number,
    ) {}
    decodeAudioData = vi.fn(async () => ({ duration }))
    createBufferSource = () => ({
      buffer: null,
      connect: vi.fn(),
      start: vi.fn(),
    })
    destination = {}
    startRendering = async () => ({ getChannelData: () => rendered })
  }
  vi.stubGlobal('OfflineAudioContext', Context)
}

it('sends a short recording unchanged', async () => {
  stubDecoder(12)
  const blob = new Blob(['voice'], { type: 'audio/webm' })
  await expect(prepareSpeech(blob)).resolves.toBe(blob)
})

it('turns a long recording into 16 kHz mono WAV the backend can cut', async () => {
  stubDecoder(30)
  const wav = await prepareSpeech(new Blob(['voice'], { type: 'audio/webm' }))
  expect(wav.type).toBe('audio/wav')
  const view = new DataView(await wav.arrayBuffer())
  const text = (offset: number) =>
    String.fromCharCode(
      ...new Uint8Array(view.buffer.slice(offset, offset + 4)),
    )
  expect(text(0)).toBe('RIFF')
  expect(text(8)).toBe('WAVE')
  expect(view.getUint32(24, true)).toBe(16_000)
  expect(view.getUint16(22, true)).toBe(1)
  expect(view.getUint32(40, true)).toBe(30 * 16_000 * 2)
  expect(view.getInt16(44, true)).toBe(Math.trunc(0.5 * 0x7fff))
})

it('keeps the recording when the browser cannot decode it', async () => {
  vi.stubGlobal(
    'OfflineAudioContext',
    class {
      decodeAudioData = () => Promise.reject(new Error('decode'))
    },
  )
  const blob = new Blob(['voice'], { type: 'audio/ogg' })
  await expect(prepareSpeech(blob)).resolves.toBe(blob)
})
