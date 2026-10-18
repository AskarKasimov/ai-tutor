import { expect, it } from 'vitest'
import { readPcmStream } from '@/shared/lib/pcm-stream'

const start = {
  type: 'start',
  version: 1,
  sample_rate: 24000,
  channels: 1,
  encoding: 'pcm_s16le',
}
const audio = { type: 'audio', seq: 0, data: 'AACA/w==' }
const end = { type: 'end', chunks: 1, samples: 2 }
const wire = (...events: unknown[]) =>
  events.map((event) => JSON.stringify(event) + '\n').join('')
function body(text: string, fragment = 7) {
  const data = new TextEncoder().encode(text)
  let offset = 0
  return new ReadableStream<Uint8Array>({
    pull(controller) {
      if (offset >= data.length) return controller.close()
      controller.enqueue(data.slice(offset, (offset += fragment)))
    },
  })
}
async function collect(text: string) {
  const result = []
  for await (const event of readPcmStream(
    body(text),
    new AbortController().signal,
  ))
    result.push(event)
  return result
}
it('decodes fragmented NDJSON incrementally and preserves little-endian PCM', async () => {
  const result = await collect(wire(start, audio, end))
  expect(result).toEqual([
    { type: 'start', sampleRate: 24000 },
    { type: 'audio', pcm: new Uint8Array([0, 0, 128, 255]) },
    { type: 'end', samples: 2 },
  ])
})
it.each([
  ['missing start', [audio, end]],
  ['duplicate start', [start, start]],
  ['unknown version', [{ ...start, version: 2 }, audio, end]],
  ['stereo', [{ ...start, channels: 2 }, audio, end]],
  ['too low rate', [{ ...start, sample_rate: 7999 }, audio, end]],
  ['too high rate', [{ ...start, sample_rate: 192001 }, audio, end]],
  ['invalid rate', [{ ...start, sample_rate: 0 }, audio, end]],
  ['wrong encoding', [{ ...start, encoding: 'wav' }, audio, end]],
  ['seq gap', [start, { ...audio, seq: 1 }, end]],
  ['malformed base64', [start, { ...audio, data: '!!!!' }, end]],
  ['noncanonical base64', [start, { ...audio, data: 'AB==' }, end]],
  ['odd PCM', [start, { ...audio, data: 'AA==' }, end]],
  ['empty PCM', [start, { ...audio, data: '' }, end]],
  ['wrong sample count', [start, audio, { ...end, samples: 3 }]],
  ['wrong chunk count', [start, audio, { ...end, chunks: 2 }]],
  ['empty synthesis', [start, { type: 'end', chunks: 0, samples: 0 }]],
  [
    'provider error',
    [start, audio, { type: 'error', code: 'SYNTHESIS_FAILED' }],
  ],
  ['unterminated stream', [start, audio]],
  ['after end', [start, audio, end, audio]],
  ['unknown event', [start, { type: 'other' }]],
])('rejects %s', async (_, events) => {
  await expect(collect(wire(...events))).rejects.toThrow()
})
it('rejects incomplete final line and oversized lines before parsing', async () => {
  await expect(
    collect(wire(start, audio) + JSON.stringify(end)),
  ).rejects.toThrow()
  await expect(collect(' '.repeat(256 * 1024 + 1))).rejects.toThrow()
})
it('caps decoded PCM at 64 MiB without keeping the stream in memory', async () => {
  const block = btoa('\0'.repeat(128 * 1024))
  let index = -1
  const stream = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.enqueue(
        new TextEncoder().encode(
          wire(index < 0 ? start : { type: 'audio', seq: index, data: block }),
        ),
      )
      index++
    },
  })
  let samples = 0
  await expect(
    (async () => {
      for await (const event of readPcmStream(
        stream,
        new AbortController().signal,
      ))
        if (event.type === 'audio') samples += event.pcm.length / 2
    })(),
  ).rejects.toThrow()
  expect(samples).toBe(32 * 1024 * 1024)
})
it('cancels a pending reader on abort and releases its lock', async () => {
  let cancelled = false
  const stream = new ReadableStream<Uint8Array>({
    cancel() {
      cancelled = true
    },
  })
  const controller = new AbortController()
  const next = readPcmStream(stream, controller.signal).next()
  controller.abort()
  await expect(next).rejects.toThrow()
  expect(cancelled).toBe(true)
  expect(stream.locked).toBe(false)
})

it('accepts a legal PCM frame close to the line-size limit in one network read', async () => {
  const bytes = 196000
  const data = btoa('\0'.repeat(bytes))
  let samples = 0
  const text = wire(
    start,
    { type: 'audio', seq: 0, data },
    { type: 'end', chunks: 1, samples: bytes / 2 },
  )
  for await (const event of readPcmStream(
    body(text, text.length),
    new AbortController().signal,
  ))
    if (event.type === 'audio') samples += event.pcm.length / 2
  expect(samples).toBe(98000)
})
