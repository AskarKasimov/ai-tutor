import { describe, expect, it } from 'vitest'
import {
  getExpressSteps,
  normalizeText,
  parseGapItem,
  parsePartialItem,
} from '@/entities/assessment/model/feedback'

describe('trainer feedback parsing', () => {
  it('normalizes escaped line breaks and parses gap explanations and topics', () => {
    expect(
      normalizeText(
        'Тема\\nПояснение: причина\\r\\nЧто повторить: основы; практика',
      ),
    ).toBe('Тема\nПояснение: причина\nЧто повторить: основы; практика')
    expect(
      parseGapItem(
        'Тема\nПояснение: причина\nЧто повторить:\n• основы\n• практика',
      ),
    ).toEqual({
      title: 'Тема',
      explanation: 'причина',
      subtopics: ['основы', 'практика'],
    })
  })

  it('parses partial feedback and builds express steps from parsed topics', () => {
    expect(
      parsePartialItem('«Регрессия»\n• Замечание: мало обоснования'),
    ).toEqual({
      title: '«Регрессия»',
      note: 'мало обоснования',
    })
    expect(
      getExpressSteps(
        ['«Классификация»\nЧто повторить: метрики'],
        ['Регрессия'],
        [],
      ),
    ).toHaveLength(3)
  })
})
