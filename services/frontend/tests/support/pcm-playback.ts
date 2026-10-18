import { vi } from 'vitest'
import { PcmQueue } from '@/shared/lib/pcm-playback-core'

export class TestAudioContext {
  static instances: TestAudioContext[] = []
  static denied = false
  sampleRate = 48000
  state = 'running'
  destination = {}
  closed = false
  get audioWorklet() {
    return { addModule: async (): Promise<void> => undefined }
  }
  constructor() {
    TestAudioContext.instances.push(this)
  }
  async resume() {
    if (TestAudioContext.denied)
      throw new DOMException('Denied', 'NotAllowedError')
  }
  async close() {
    this.closed = true
  }
}
export class TestWorkletNode {
  static instances: TestWorkletNode[] = []
  queue = new PcmQueue(48000)
  disconnected = false
  port = {
    onmessage: null as ((event: { data: unknown }) => void) | null,
    postMessage: (message: { type: string; samples?: Float32Array }) => {
      if (message.type === 'audio') this.queue.push(message.samples!)
      if (message.type === 'end') this.queue.finish()
    },
    close() {},
  }
  onprocessorerror: (() => void) | null = null
  constructor() {
    TestWorkletNode.instances.push(this)
  }
  connect() {}
  disconnect() {
    this.disconnected = true
  }
  render(frames = 4800) {
    const output = new Float32Array(frames)
    const result = this.queue.render(output)
    this.port.onmessage?.({ data: result })
    return output
  }
}
export function installPcmPlayback() {
  TestAudioContext.instances = []
  TestAudioContext.denied = false
  TestWorkletNode.instances = []
  vi.stubGlobal('AudioContext', TestAudioContext)
  vi.stubGlobal('AudioWorkletNode', TestWorkletNode)
}
export const pcmStart = {
  type: 'start',
  version: 1,
  sample_rate: 24000,
  channels: 1,
  encoding: 'pcm_s16le',
}
export function pcmBlock(seq: number, samples = 2400) {
  return { type: 'audio', seq, data: btoa('\0\x40'.repeat(samples)) }
}
export const encodePcm = (...events: unknown[]) =>
  new TextEncoder().encode(
    events.map((value) => JSON.stringify(value) + '\n').join(''),
  )
export function pcmBody(samples = 2400) {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(
        encodePcm(pcmStart, pcmBlock(0, samples), {
          type: 'end',
          chunks: 1,
          samples,
        }),
      )
      controller.close()
    },
  })
}
