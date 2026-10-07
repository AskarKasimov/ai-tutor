import { describe, expect, it } from 'vitest'
import { queryKeys } from './query-keys'

describe('query keys', () => {
  it('separates users and current tasks', () => {
    expect(queryKeys.diagnosticProgress('a', 's')).not.toEqual(
      queryKeys.diagnosticProgress('b', 's'),
    )
    expect(queryKeys.diagnosticAudio('a', 's', 't1')).not.toEqual(
      queryKeys.diagnosticAudio('a', 's', 't2'),
    )
  })
})
