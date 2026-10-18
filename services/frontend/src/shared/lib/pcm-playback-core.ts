// Shared by the AudioWorklet and its deterministic render tests. A fixed ring
// makes the two-second limit independent of message/chunk boundaries.
export class PcmQueue {
  private readonly ring: Float32Array
  private readonly initial: number
  private head = 0
  private size = 0
  private playing = false
  private complete = false
  private starving = false

  constructor(rate: number) {
    this.ring = new Float32Array(Math.ceil(rate * 2))
    this.initial = Math.ceil(rate * 0.2)
  }
  push(samples: Float32Array) {
    if (this.complete || this.size + samples.length > this.ring.length)
      throw new Error('PCM queue overflow')
    for (let index = 0; index < samples.length; index++)
      this.ring[(this.head + this.size + index) % this.ring.length] =
        samples[index]
    this.size += samples.length
  }
  finish() {
    this.complete = true
  }
  render(output: Float32Array) {
    output.fill(0)
    const started =
      !this.playing &&
      this.size > 0 &&
      (this.size >= this.initial || this.complete)
    if (started) this.playing = true
    const consumed = this.playing ? Math.min(this.size, output.length) : 0
    for (let index = 0; index < consumed; index++)
      output[index] = this.ring[(this.head + index) % this.ring.length]
    this.head = (this.head + consumed) % this.ring.length
    this.size -= consumed
    const starving = this.playing && !this.complete && consumed < output.length
    const underrun = starving && !this.starving
    this.starving = starving
    return {
      consumed,
      started,
      ended: this.complete && this.size === 0,
      underrun,
    }
  }
}

// Linear interpolation keeps absolute source position and the previous sample
// across chunks. No per-chunk decodeAudioData or phase reset is involved.
export class PcmResampler {
  private total = 0
  private next = 0
  private previous = 0
  private readonly step: number
  constructor(sourceRate: number, targetRate: number) {
    this.step = sourceRate / targetRate
  }
  push(pcm: Uint8Array): Float32Array<ArrayBuffer> {
    const count = pcm.length / 2
    const view = new DataView(pcm.buffer, pcm.byteOffset, pcm.byteLength)
    const last = this.total + count - 1
    const values: number[] = []
    while (this.next <= last) {
      const left = Math.floor(this.next)
      const fraction = this.next - left
      if (fraction > 0 && left + 1 > last) break
      const local = left - this.total
      const a =
        local < 0 ? this.previous : view.getInt16(local * 2, true) / 32768
      const b =
        fraction === 0 ? a : view.getInt16((local + 1) * 2, true) / 32768
      values.push(a + (b - a) * fraction)
      this.next += this.step
    }
    if (count) this.previous = view.getInt16((count - 1) * 2, true) / 32768
    this.total += count
    return Float32Array.from(values)
  }
  finish(): Float32Array<ArrayBuffer> {
    const values: number[] = []
    while (this.next < this.total) {
      values.push(this.previous)
      this.next += this.step
    }
    return Float32Array.from(values)
  }
}
