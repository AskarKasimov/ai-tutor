import { expect, it } from 'vitest'
import { answerKey } from '@/features/diagnostic-session'

it('repeats the key for the same reply and changes it for another one', () => {
  const text = answerKey('t1', { kind: 'text', text: 'Ответ' })
  expect(answerKey('t1', { kind: 'text', text: 'Ответ' })).toBe(text)
  expect(answerKey('t1', { kind: 'text', text: 'Ответ!' })).not.toBe(text)
  expect(answerKey('t2', { kind: 'text', text: 'Ответ' })).not.toBe(text)
  expect(answerKey('t1', { kind: 'dont_know' })).not.toBe(
    answerKey('t1', { kind: 'voice', transcriptionId: '' }),
  )
})
