import { expect, it } from 'vitest'
import { PcmQueue, PcmResampler } from '@/shared/lib/pcm-playback-core'

it('waits for 200ms, supplies silence on underrun, and drains a short finished stream', () => {
  const queue = new PcmQueue(10)
  queue.push(new Float32Array([0.1]))
  expect(queue.render(new Float32Array(2))).toEqual({
    consumed: 0,
    started: false,
    ended: false,
    underrun: false,
  })
  queue.push(new Float32Array([0.2]))
  const output = new Float32Array(3)
  expect(queue.render(output)).toEqual({
    consumed: 2,
    started: true,
    ended: false,
    underrun: true,
  })
  expect([...output]).toEqual([Math.fround(0.1), Math.fround(0.2), 0])
  const silent = new Float32Array([1, 1])
  expect(queue.render(silent).underrun).toBe(false)
  expect([...silent]).toEqual([0, 0])
  queue.push(new Float32Array([0.3]))
  queue.finish()
  expect(queue.render(output)).toEqual({
    consumed: 1,
    started: false,
    ended: true,
    underrun: false,
  })
  expect([...output]).toEqual([Math.fround(0.3), 0, 0])
  const short = new PcmQueue(10)
  short.push(new Float32Array([0.5]))
  short.finish()
  expect(short.render(output)).toEqual({
    consumed: 1,
    started: true,
    ended: true,
    underrun: false,
  })
})
it('bounds the worklet ring to two seconds and preserves order across wraps', () => {
  const queue = new PcmQueue(10)
  queue.push(Float32Array.from({ length: 20 }, (_, index) => index))
  expect(() => queue.push(new Float32Array([21]))).toThrow()
  const output = new Float32Array(15)
  expect(queue.render(output).consumed).toBe(15)
  queue.push(Float32Array.from({ length: 15 }, (_, index) => index + 20))
  queue.finish()
  const tail = new Float32Array(20)
  expect(queue.render(tail).ended).toBe(true)
  expect([...tail]).toEqual(
    Array.from({ length: 20 }, (_, index) => index + 15),
  )
})
it('resamples PCM across chunk boundaries using continuous interpolation', () => {
  const converter = new PcmResampler(2, 4)
  const first = converter.push(new Uint8Array([0, 0, 0, 64]))
  const second = converter.push(new Uint8Array([0, 128, 0, 0]))
  const tail = converter.finish()
  expect([...first, ...second, ...tail]).toEqual([
    0, 0.25, 0.5, -0.25, -1, -0.5, 0, 0,
  ])
})
it('downsamples with the same phase for split and unsplit PCM', () => {
  const converter = new PcmResampler(4, 2)
  expect([...converter.push(new Uint8Array([0, 0, 0, 32]))]).toEqual([0])
  expect([...converter.push(new Uint8Array([0, 64, 0, 96]))]).toEqual([0.5])
  expect([...converter.finish()]).toEqual([])
})
