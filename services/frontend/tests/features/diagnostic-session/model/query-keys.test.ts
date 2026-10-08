import { describe, expect, it } from 'vitest'
import { diagnosticSessionQueryKeys } from '@/features/diagnostic-session/model/query-keys'

describe('query keys', () => {
  it('separates users and current tasks', () => {
    expect(diagnosticSessionQueryKeys.diagnosticProgress('a', 's')).not.toEqual(
      diagnosticSessionQueryKeys.diagnosticProgress('b', 's'),
    )
    expect(
      diagnosticSessionQueryKeys.diagnosticAudio('a', 's', 't1'),
    ).not.toEqual(diagnosticSessionQueryKeys.diagnosticAudio('a', 's', 't2'))
  })
})
