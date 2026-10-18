import { afterEach, expect, it, vi } from 'vitest'
import { waitFor } from '@testing-library/react'
import { playPcmStream } from '@/shared/lib/pcm-player'
import {
  encodePcm,
  installPcmPlayback,
  pcmBlock,
  pcmBody,
  pcmStart,
  TestAudioContext,
  TestWorkletNode,
} from '../../support/pcm-playback'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
it('starts before EOF, announces actual render once, and drains a short tail', async () => {
  installPcmPlayback()
  let producer!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      producer = controller
    },
  })
  const started = vi.fn()
  const received = vi.fn()
  const playback = playPcmStream(
    body,
    new AbortController().signal,
    started,
    received,
  )
  producer.enqueue(encodePcm(pcmStart, pcmBlock(0, 7200)))
  await waitFor(() => expect(TestWorkletNode.instances).toHaveLength(1))
  const node = TestWorkletNode.instances[0]
  await waitFor(() => expect(node.render()[0]).toBe(0.5))
  expect(started).toHaveBeenCalledTimes(1)
  expect(received).not.toHaveBeenCalled()
  producer.enqueue(encodePcm({ type: 'end', chunks: 1, samples: 7200 }))
  producer.close()
  await waitFor(() => expect(received).toHaveBeenCalledTimes(1))
  node.render(10000)
  const result = await playback
  expect(result.audio.size).toBe(14444)
  expect(result.metrics.ttfaMs).toBeGreaterThanOrEqual(0)
  expect(started).toHaveBeenCalledTimes(1)
  expect(TestAudioContext.instances[0].closed).toBe(true)
})
it('backpressures network reads at a two-second queue and aborts a blocked writer', async () => {
  installPcmPlayback()
  let pulled = 0
  let cancelled = false
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.enqueue(
        encodePcm(pulled === 0 ? pcmStart : pcmBlock(pulled - 1)),
      )
      pulled++
    },
    cancel() {
      cancelled = true
    },
  })
  const controller = new AbortController()
  const playback = playPcmStream(body, controller.signal, () => undefined)
  await waitFor(() => expect(pulled).toBeGreaterThanOrEqual(21))
  await new Promise((resolve) => setTimeout(resolve, 10))
  expect(pulled).toBeLessThanOrEqual(24)
  controller.abort()
  await expect(playback).rejects.toThrow()
  expect(cancelled).toBe(true)
  expect(TestWorkletNode.instances[0].disconnected).toBe(true)
})
it('allows a stream shorter than initial buffering to play after verified end', async () => {
  installPcmPlayback()
  const received = vi.fn()
  const started = vi.fn()
  const playback = playPcmStream(
    pcmBody(1),
    new AbortController().signal,
    started,
    received,
  )
  await waitFor(() => expect(received).toHaveBeenCalledOnce())
  expect(started).not.toHaveBeenCalled()
  expect(TestWorkletNode.instances[0].render()[0]).toBe(0.5)
  await playback
  expect(started).toHaveBeenCalledOnce()
})
it('closes the response and context on autoplay denial', async () => {
  installPcmPlayback()
  TestAudioContext.denied = true
  let cancelled = false
  const body = new ReadableStream<Uint8Array>({
    cancel() {
      cancelled = true
    },
  })
  await expect(
    playPcmStream(body, new AbortController().signal, () => undefined),
  ).rejects.toMatchObject({ name: 'NotAllowedError' })
  expect(cancelled).toBe(true)
  expect(TestAudioContext.instances[0].closed).toBe(true)
})
it('stops playback after malformed PCM without returning a partial cache', async () => {
  installPcmPlayback()
  let producer!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      producer = controller
    },
  })
  const playback = playPcmStream(
    body,
    new AbortController().signal,
    () => undefined,
  )
  producer.enqueue(encodePcm(pcmStart, pcmBlock(0, 7200)))
  await waitFor(() =>
    expect(TestWorkletNode.instances[0]?.render()[0]).toBe(0.5),
  )
  producer.enqueue(encodePcm({ type: 'error', code: 'SYNTHESIS_FAILED' }))
  producer.close()
  await expect(playback).rejects.toThrow()
  expect(TestWorkletNode.instances[0].disconnected).toBe(true)
})

it('cancels a pending network read if the worklet fails', async () => {
  installPcmPlayback()
  let cancelled = false
  const body = new ReadableStream<Uint8Array>({
    cancel() {
      cancelled = true
    },
  })
  const playback = playPcmStream(
    body,
    new AbortController().signal,
    () => undefined,
  )
  await waitFor(() => expect(TestWorkletNode.instances).toHaveLength(1))
  TestWorkletNode.instances[0].onprocessorerror?.()
  await expect(playback).rejects.toThrow('PCM worklet failed')
  expect(cancelled).toBe(true)
})
it('aborts during worklet module setup without retaining a context or body', async () => {
  installPcmPlayback()
  vi.spyOn(TestAudioContext.prototype, 'audioWorklet', 'get').mockReturnValue({
    addModule: () => new Promise<void>(() => undefined),
  })
  let cancelled = false
  const body = new ReadableStream<Uint8Array>({
    cancel() {
      cancelled = true
    },
  })
  const controller = new AbortController()
  const playback = playPcmStream(body, controller.signal, () => undefined)
  controller.abort()
  await expect(playback).rejects.toThrow()
  expect(cancelled).toBe(true)
  expect(TestAudioContext.instances[0].closed).toBe(true)
})

it('coalesces tiny PCM frames so replay storage metadata stays bounded', async () => {
  installPcmPlayback()
  const NativeBlob = Blob
  let parts = 0
  vi.stubGlobal(
    'Blob',
    class extends NativeBlob {
      constructor(chunks: BlobPart[], options: BlobPropertyBag) {
        super(chunks, options)
        parts = chunks.length
      }
    },
  )
  const frames = Array.from({ length: 1024 }, (_, seq) => pcmBlock(seq, 1))
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(
        encodePcm(pcmStart, ...frames, {
          type: 'end',
          chunks: 1024,
          samples: 1024,
        }),
      )
      controller.close()
    },
  })
  const received = vi.fn()
  const playback = playPcmStream(
    body,
    new AbortController().signal,
    () => undefined,
    received,
  )
  await waitFor(() => expect(received).toHaveBeenCalledOnce())
  TestWorkletNode.instances[0].render(10000)
  expect((await playback).audio.size).toBe(2092)
  expect(parts).toBeLessThanOrEqual(2)
})

it('counts underrun gaps and resumes without discarding subsequent PCM', async () => {
  installPcmPlayback()
  let producer!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      producer = controller
    },
  })
  const received = vi.fn()
  const playback = playPcmStream(
    body,
    new AbortController().signal,
    () => undefined,
    received,
  )
  producer.enqueue(encodePcm(pcmStart, pcmBlock(0, 7200)))
  await waitFor(() =>
    expect(TestWorkletNode.instances[0]?.render(20000)[0]).toBe(0.5),
  )
  expect([...TestWorkletNode.instances[0].render(128)]).toEqual(
    Array(128).fill(0),
  )
  producer.enqueue(
    encodePcm(pcmBlock(1, 2400), { type: 'end', chunks: 2, samples: 9600 }),
  )
  producer.close()
  await waitFor(() => expect(received).toHaveBeenCalledOnce())
  expect(TestWorkletNode.instances[0].render(10000)[0]).toBe(0.5)
  const result = await playback
  expect(result.audio.size).toBe(19244)
  expect(result.metrics.underruns).toBe(1)
})
