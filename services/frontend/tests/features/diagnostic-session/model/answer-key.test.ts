import { expect, it } from 'vitest'
import { createAnswerKeys } from '@/features/diagnostic-session'

it('repeats the key for the exact same reply and changes it for any other one', () => {
  let n = 0
  const keyFor = createAnswerKeys(() => `id-${++n}`)
  const text = keyFor('t1', { kind: 'text', text: 'Ответ' })
  expect(keyFor('t1', { kind: 'text', text: 'Ответ' })).toBe(text)
  expect(keyFor('t1', { kind: 'text', text: 'Ответ!' })).not.toBe(text)
  expect(keyFor('t2', { kind: 'text', text: 'Ответ' })).not.toBe(text)
  // Equal-length texts that a 32-bit hash collides on stay distinct.
  expect(keyFor('t1', { kind: 'text', text: 'Aa' })).not.toBe(
    keyFor('t1', { kind: 'text', text: 'B@' }),
  )
  expect(keyFor('t1', { kind: 'dont_know' })).not.toBe(
    keyFor('t1', { kind: 'voice', transcriptionId: '' }),
  )
})
