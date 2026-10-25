import { expect, it } from 'vitest'
import { groupFeedbackDetails } from '@/pages/learning-trainer/model/group-feedback-details'

it('shows the full-study constituent once and retains every outcome and advice', () => {
  const heading =
    'Необходимо изучить полностью составляющую «Постановка задачи».'
  expect(
    groupFeedbackDetails([
      `${heading} Проверенный результат: «Определяет тип». Повторите регрессию.`,
      `${heading} Проверенный результат: «Выделяет признаки». Укажите признаки.`,
      `${heading} Проверенный результат: «Находит ошибки». Проверьте утечки.`,
    ]),
  ).toEqual([
    {
      heading: 'Изучите тему: Постановка задачи.',
      items: [
        'Определяет тип. Повторите регрессию.',
        'Выделяет признаки. Укажите признаки.',
        'Находит ошибки. Проверьте утечки.',
      ],
    },
  ])
})

it('keeps different constituents and study instructions separate', () => {
  expect(
    groupFeedbackDetails([
      'Дополнительно изучить. Составляющая «Модели», результат «Регрессия».',
      'Дополнительно изучить. Составляющая «Модели», результат «Метрики».',
      'Необходимо изучить полностью составляющую «Признаки». Проверенный результат: «Масштаб».',
    ]),
  ).toEqual([
    {
      heading: 'Повторите тему: Модели.',
      items: ['Регрессия.', 'Метрики.'],
    },
    {
      heading: 'Изучите тему: Признаки.',
      items: ['Масштаб.'],
    },
  ])
})

it('preserves arbitrary advice and Markdown without guessing its structure', () => {
  expect(
    groupFeedbackDetails(['**Повторите** тему. Пример.', 'Другой совет.']),
  ).toEqual([
    { heading: '', items: ['**Повторите** тему. Пример.', 'Другой совет.'] },
  ])
})
