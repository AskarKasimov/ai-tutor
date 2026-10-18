import { PcmQueue } from './pcm-playback-core'

declare const sampleRate: number
declare class AudioWorkletProcessor {
  readonly port: MessagePort
}
declare function registerProcessor(
  name: string,
  processor: typeof AudioWorkletProcessor,
): void

class PcmProcessor extends AudioWorkletProcessor {
  private readonly queue = new PcmQueue(sampleRate)
  constructor() {
    super()
    this.port.onmessage = (
      event: MessageEvent<{ type: string; samples?: Float32Array }>,
    ) => {
      if (event.data.type === 'audio') this.queue.push(event.data.samples!)
      else if (event.data.type === 'end') this.queue.finish()
    }
  }
  process(_inputs: Float32Array[][], outputs: Float32Array[][]) {
    const output = outputs[0]?.[0]
    if (!output) return true
    const result = this.queue.render(output)
    if (result.consumed || result.started || result.ended || result.underrun)
      this.port.postMessage(result)
    return !result.ended
  }
}
registerProcessor('tutor-pcm', PcmProcessor)
