import { expect, it } from 'vitest'
import { feedbackExplanation } from '@/pages/trainer/model/feedback-explanation'

it('removes the entire competency heading and keeps the explanation', () => {
  expect(
    feedbackExplanation('**К1 — Модели**\n\nВы различаете **виды моделей**.'),
  ).toBe('Вы различаете **виды моделей**.')
  expect(
    feedbackExplanation('**K6. Метрики**\r\nСравните метрики на примерах.'),
  ).toBe('Сравните метрики на примерах.')
})

it('does not turn a heading without an explanation into an empty list item', () => {
  expect(feedbackExplanation('**К12 — Модели**')).toBe('')
})

it('preserves ordinary explanations, emphasis, and code', () => {
  for (const text of [
    'Вы уверенно различаете модели.',
    '**Повторите** тему. Пример.',
    '**К1** — обозначение в этом примере.',
    '**К1** — вы правильно выбрали **регрессию**.',
    '**К1** — вы правильно выбрали **регрессию**.\nУточните выбор метрики.',
    '```text\n**К1 — пример**\n```',
  ]) {
    expect(feedbackExplanation(text)).toBe(text)
  }
})
