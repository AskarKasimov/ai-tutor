import { describe, expect, it } from 'vitest'
import {
  getExpressSteps,
  normalizeText,
  parseGapItem,
  parsePartialItem,
  parseStructuredSummary,
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

  it('parses structured summary with general part, what is good, and what needs work', () => {
    const raw = `Студент показал уверенное понимание базовых определений и уверенно решает задачи. Однако в тонких нюансах метрик возникают неточности. В целом материал усвоен на хорошем базовом уровне. Рекомендуется повторить ключевые формулы перед практикой.

Что хорошо:
Отлично объяснены базовые концепции и постановка задачи обучения с учителем.

Что нужно изучить:
Обратить внимание на различие между Precision и Recall при дисбалансе классов.`

    const parsed = parseStructuredSummary(raw)
    expect(parsed.generalText).toContain('Студент показал уверенное понимание')
    expect(parsed.generalText).toContain(
      'Рекомендуется повторить ключевые формулы',
    )
    expect(parsed.whatIsGood).toBe(
      'Отлично объяснены базовые концепции и постановка задачи обучения с учителем.',
    )
    expect(parsed.whatNeedsWork).toBe(
      'Обратить внимание на различие между Precision и Recall при дисбалансе классов.',
    )
  })

  it('parses structured summary with "Что доработать:" heading', () => {
    const raw = `Студент хорошо ориентируется в базовых концепциях ML. В задачах регрессии и классификации продемонстрированы точные ответы. В оптимизации градиентным спуском есть пробелы.
    
Что хорошо:
Точно определена постановка задачи регрессии и вычисление MSE.

Что доработать:
Повторить аналитическое решение МНК и условия вырожденности матрицы XᵀX.`

    const parsed = parseStructuredSummary(raw)
    expect(parsed.generalText).toContain('Студент хорошо ориентируется')
    expect(parsed.whatIsGood).toBe(
      'Точно определена постановка задачи регрессии и вычисление MSE.',
    )
    expect(parsed.whatNeedsWork).toBe(
      'Повторить аналитическое решение МНК и условия вырожденности матрицы XᵀX.',
    )
  })
})
