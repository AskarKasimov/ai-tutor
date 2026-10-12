import { vi } from 'vitest'

export function mockChatVoice() {
  const stream = { getTracks: () => [{ stop: vi.fn() }] }
  const getUserMedia = vi.fn().mockResolvedValue(stream)
  vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } })

  class Recorder {
    static isTypeSupported() {
      return true
    }
    readonly mimeType = 'audio/webm'
    state: RecordingState = 'inactive'
    ondataavailable?: (event: { data: Blob }) => void
    onstop?: () => void

    start() {
      this.state = 'recording'
    }
    stop() {
      this.state = 'inactive'
      this.ondataavailable?.({
        data: new Blob(['recorded voice'], { type: this.mimeType }),
      })
      this.onstop?.()
    }
  }
  vi.stubGlobal('MediaRecorder', Recorder)
  return { getUserMedia }
}
