import { afterEach, expect, it, vi } from 'vitest'
import { transcribeRecording } from '../data/voice-api'
import { startRecording } from './prototype-audio'

afterEach(() => vi.unstubAllGlobals())

function setup(supportedTypes: string[]) {
  const release = vi.fn()
  const stream = { getTracks: () => [{ stop: release }] }
  const getUserMedia = vi.fn().mockResolvedValue(stream)
  vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } })
  class Recorder {
    static isTypeSupported(type: string) { return supportedTypes.includes(type) }
    readonly mimeType: string
    state = 'inactive'
    ondataavailable?: (event: { data: Blob }) => void
    onstop?: () => void

    constructor(_stream: unknown, options?: MediaRecorderOptions) {
      this.mimeType = options?.mimeType ?? 'audio/ogg;codecs=vorbis'
    }

    start() { this.state = 'recording' }
    stop() {
      this.state = 'inactive'
      this.ondataavailable?.({ data: new Blob(['recorded audio'], { type: this.mimeType }) })
      this.onstop?.()
    }
  }
  vi.stubGlobal('MediaRecorder', Recorder)
  return { release, getUserMedia }
}

it.each([
  ['audio/webm;codecs=opus', 'webm'],
  ['audio/ogg;codecs=opus', 'ogg'],
])('records and uploads %s when it is the supported STT format', async (mimeType, extension) => {
  const { release } = setup([mimeType])
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ id: 'tr-1', text: 'Ответ' }) })
  vi.stubGlobal('fetch', fetchMock)

  const recording = await startRecording()
  const blob = await recording.stop()
  expect(blob.type).toBe(mimeType)
  expect(blob.size).toBeGreaterThan(0)
  expect(release).toHaveBeenCalledOnce()

  await transcribeRecording(blob, new AbortController().signal)
  const file = fetchMock.mock.calls[0][1].body.get('audio')
  expect(file.type).toBe(mimeType)
  expect(file.name).toBe(`answer.${extension}`)
})

it('does not request microphone access when only unsupported codecs are available', async () => {
  const { getUserMedia } = setup(['audio/ogg;codecs=vorbis', 'audio/mp4'])
  await expect(startRecording()).rejects.toThrow('unavailable')
  expect(getUserMedia).not.toHaveBeenCalled()
})

it('releases microphone tracks if recorder initialization fails', async () => {
  const { release } = setup(['audio/webm;codecs=opus'])
  vi.spyOn(MediaRecorder.prototype, 'start').mockImplementation(() => { throw new Error('recording') })
  await expect(startRecording()).rejects.toThrow('recording')
  expect(release).toHaveBeenCalledOnce()
})
