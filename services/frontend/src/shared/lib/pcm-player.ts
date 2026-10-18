import workletUrl from './pcm-worklet.ts?worker&url'
import { PcmResampler } from './pcm-playback-core'
import { readPcmStream } from './pcm-stream'

export function supportsPcmStreaming() {
  return (
    typeof ReadableStream !== 'undefined' &&
    typeof AudioContext !== 'undefined' &&
    typeof AudioWorkletNode !== 'undefined' &&
    'audioWorklet' in AudioContext.prototype
  )
}

type PlaybackMetrics = { ttfaMs: number; underruns: number }

// Pass the action timestamp captured before fetch so TTFA includes headers,
// transport and player setup. Direct stream callers default to invocation time.
export async function playPcmStream(
  body: ReadableStream<Uint8Array>,
  signal: AbortSignal,
  onStart: () => void,
  onReceived?: () => void,
  playStartedAt = performance.now(),
): Promise<{ audio: Blob; metrics: PlaybackMetrics }> {
  const transport = new AbortController()
  const readSignal = AbortSignal.any([signal, transport.signal])
  const context = new AudioContext()
  let node: AudioWorkletNode | undefined
  let queued = 0
  let ended = false
  let failure: Error | undefined
  let notify: (() => void) | undefined
  let started = false
  const metrics = { ttfaMs: 0, underruns: 0 }
  const check = () => {
    signal.throwIfAborted()
    if (failure) throw failure
  }
  const wake = () => {
    notify?.()
    notify = undefined
  }
  signal.addEventListener('abort', wake)
  const wait = async () => {
    check()
    await new Promise<void>((resolve) => {
      notify = resolve
    })
    check()
  }
  // AudioContext setup is asynchronous too: stop must not wait for module
  // loading or a suspended context before closing the old player.
  const duringSetup = <T>(promise: Promise<T>): Promise<T> =>
    new Promise((resolve, reject) => {
      const abort = () => reject(signal.reason)
      signal.addEventListener('abort', abort, { once: true })
      promise
        .then(resolve, reject)
        .finally(() => signal.removeEventListener('abort', abort))
      if (signal.aborted) abort()
    })
  try {
    check()
    // Detect autoplay denial before waiting for module loading.
    await duringSetup(context.resume())
    if (context.state === 'suspended')
      throw new DOMException('Autoplay denied', 'NotAllowedError')
    await duringSetup(context.audioWorklet.addModule(workletUrl))
    check()
    node = new AudioWorkletNode(context, 'tutor-pcm', {
      numberOfInputs: 0,
      numberOfOutputs: 1,
      outputChannelCount: [1],
    })
    node.onprocessorerror = () => {
      failure = new Error('PCM worklet failed')
      transport.abort(failure)
      wake()
    }
    node.port.onmessage = (
      event: MessageEvent<{
        consumed: number
        started: boolean
        ended: boolean
        underrun: boolean
      }>,
    ) => {
      if (signal.aborted) return
      const value = event.data
      queued -= value.consumed
      if (value.started && !started) {
        started = true
        metrics.ttfaMs = performance.now() - playStartedAt
        onStart()
      }
      if (value.underrun) metrics.underruns++
      if (value.ended) ended = true
      wake()
    }
    node.connect(context.destination)
    const limit = Math.ceil(context.sampleRate * 2)
    const send = async (samples: Float32Array<ArrayBuffer>) => {
      let offset = 0
      while (offset < samples.length) {
        check()
        if (queued >= limit) {
          await wait()
          continue
        }
        const count = Math.min(limit - queued, samples.length - offset)
        const block = samples.slice(offset, offset + count)
        queued += count
        node!.port.postMessage({ type: 'audio', samples: block }, [
          block.buffer,
        ])
        offset += count
      }
    }
    let rate = 0
    let converter: PcmResampler | undefined
    let bytes = 0
    const cache: Uint8Array<ArrayBuffer>[] = []
    let cacheTail = new Uint8Array(64 * 1024)
    let tailSize = 0
    const retain = (pcm: Uint8Array) => {
      let offset = 0
      while (offset < pcm.length) {
        const count = Math.min(cacheTail.length - tailSize, pcm.length - offset)
        cacheTail.set(pcm.subarray(offset, offset + count), tailSize)
        tailSize += count
        offset += count
        if (tailSize === cacheTail.length) {
          cache.push(cacheTail)
          cacheTail = new Uint8Array(64 * 1024)
          tailSize = 0
        }
      }
    }
    for await (const event of readPcmStream(body, readSignal)) {
      check()
      if (event.type === 'start') {
        rate = event.sampleRate
        converter = new PcmResampler(rate, context.sampleRate)
      } else if (event.type === 'audio') {
        retain(event.pcm)
        bytes += event.pcm.length
        await send(converter!.push(event.pcm))
      } else {
        await send(converter!.finish())
      }
    }
    check()
    // Only a complete, validated stream becomes replayable. Cache chunks
    // once; Blob assembly avoids repeatedly concatenating growing buffers.
    if (tailSize) cache.push(cacheTail.subarray(0, tailSize))
    const audio = pcmWav(cache, bytes, rate)
    onReceived?.()
    node.port.postMessage({ type: 'end' })
    while (!ended) await wait()
    check()
    return { audio, metrics }
  } finally {
    signal.removeEventListener('abort', wake)
    node?.disconnect()
    node?.port.close()
    if (!body.locked) await body.cancel().catch(() => undefined)
    await context.close().catch(() => undefined)
  }
}

function pcmWav(
  chunks: Uint8Array<ArrayBuffer>[],
  bytes: number,
  rate: number,
) {
  const header = new Uint8Array(44)
  const view = new DataView(header.buffer)
  const text = (offset: number, value: string) => {
    for (let index = 0; index < value.length; index++)
      header[offset + index] = value.charCodeAt(index)
  }
  text(0, 'RIFF')
  view.setUint32(4, bytes + 36, true)
  text(8, 'WAVEfmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, rate, true)
  view.setUint32(28, rate * 2, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  text(36, 'data')
  view.setUint32(40, bytes, true)
  return new Blob([header, ...chunks], { type: 'audio/wav' })
}
