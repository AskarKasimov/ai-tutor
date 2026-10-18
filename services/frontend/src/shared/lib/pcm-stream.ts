export type PcmEvent =
  | { type: 'start'; sampleRate: number }
  | { type: 'audio'; pcm: Uint8Array<ArrayBuffer> }
  | { type: 'end'; samples: number }

const MAX_LINE = 256 * 1024
const MAX_PCM = 64 * 1024 * 1024
const invalid = () => new Error('Invalid PCM stream')

// This generator retains one bounded line, never the response body. Awaiting
// the consumer between yields propagates playback backpressure to fetch.
export async function* readPcmStream(
  body: ReadableStream<Uint8Array>,
  signal: AbortSignal,
): AsyncGenerator<PcmEvent> {
  const reader = body.getReader()
  const abort = () => {
    void reader.cancel(signal.reason).catch(() => undefined)
  }
  signal.addEventListener('abort', abort, { once: true })
  const line = new Uint8Array(MAX_LINE)
  const decoder = new TextDecoder('utf-8', { fatal: true })
  let length = 0
  let rate = 0
  let chunks = 0
  let bytes = 0
  let ended = false
  let eof = false
  try {
    signal.throwIfAborted()
    while (true) {
      const next = await reader.read()
      signal.throwIfAborted()
      if (next.done) {
        eof = true
        break
      }
      let offset = 0
      while (offset < next.value.length) {
        const newline = next.value.indexOf(10, offset)
        const limit = newline < 0 ? next.value.length : newline
        const count = limit - offset
        if (length + count > MAX_LINE) throw invalid()
        line.set(next.value.subarray(offset, limit), length)
        length += count
        offset = limit + 1
        if (newline < 0) continue
        if (ended || !length) throw invalid()
        const event: unknown = JSON.parse(
          decoder.decode(line.subarray(0, length)),
        )
        length = 0
        if (!event || typeof event !== 'object') throw invalid()
        const value = event as Record<string, unknown>
        if (value.type === 'start') {
          if (
            rate ||
            value.version !== 1 ||
            value.channels !== 1 ||
            value.encoding !== 'pcm_s16le' ||
            !Number.isSafeInteger(value.sample_rate) ||
            Number(value.sample_rate) < 8000 ||
            Number(value.sample_rate) > 192000
          )
            throw invalid()
          rate = Number(value.sample_rate)
          yield { type: 'start', sampleRate: rate }
        } else if (value.type === 'audio') {
          if (
            !rate ||
            value.seq !== chunks ||
            typeof value.data !== 'string' ||
            !value.data ||
            !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(
              value.data,
            )
          )
            throw invalid()
          const decoded = atob(value.data)
          if (
            btoa(decoded) !== value.data ||
            !decoded.length ||
            decoded.length % 2 ||
            bytes + decoded.length > MAX_PCM
          )
            throw invalid()
          const pcm = Uint8Array.from(decoded, (char) => char.charCodeAt(0))
          bytes += pcm.length
          chunks++
          yield { type: 'audio', pcm }
        } else if (value.type === 'end') {
          if (
            !rate ||
            !bytes ||
            value.chunks !== chunks ||
            value.samples !== bytes / 2
          )
            throw invalid()
          ended = true
          yield { type: 'end', samples: bytes / 2 }
        } else {
          // Error events, unknown events and invalid ordering are terminal.
          throw invalid()
        }
      }
    }
    if (length || !ended) throw invalid()
  } finally {
    signal.removeEventListener('abort', abort)
    if (!eof) await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
