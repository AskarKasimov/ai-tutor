import { expect, it } from 'vitest'
import {
  groupFeedbackDetails,
  type FeedbackTopicItem,
} from '@/pages/trainer/model/group-feedback-details'

const item = (
  topic: string,
  text: string,
  study_scope: FeedbackTopicItem['study_scope'] = 'review',
): FeedbackTopicItem => ({ topic, text, study_scope })

it('shows each topic once with its shared text', () => {
  const text = 'Полезно повторить, как тип задачи связан с целевой переменной.'
  expect(
    groupFeedbackDetails([
      item('Постановка задачи', text, 'full'),
      item('Постановка задачи', text, 'full'),
      item('Признаки', 'Внимания требует масштабирование.'),
    ]),
  ).toEqual([
    { heading: 'Тема для изучения: Постановка задачи.', items: [text] },
    {
      heading: 'Тема для повторения: Признаки.',
      items: ['Внимания требует масштабирование.'],
    },
  ])
})

it('keeps the whole text, including questions quoted from the task', () => {
  const text =
    'Вопрос «Выберите один правильный ответ. Через что агент действует на среду?» требует различать сенсоры и исполнительные механизмы.'
  expect(groupFeedbackDetails([item('Агенты', text)])[0].items).toEqual([text])
})

it('hides map numbering in topic headings and keeps meaningful numbers', () => {
  expect(
    groupFeedbackDetails([
      item('1.3 Умеет выбирать архитектуру агента', 'Текст.'),
      item('6.2 Описывает обучение', 'Текст.', 'skipped'),
      item('2 типа обучения', 'Текст.'),
    ]).map((group) => group.heading),
  ).toEqual([
    'Тема для повторения: выбирать архитектуру агента.',
    'Пропущенный вопрос. Тема для изучения: Описывает обучение.',
    'Тема для повторения: 2 типа обучения.',
  ])
})

it('shows advice without a topic as is and skips empty texts', () => {
  expect(
    groupFeedbackDetails([
      item('', '**Совет** из старого отчёта.'),
      item('Модели', ' '),
    ]),
  ).toEqual([{ heading: '', items: ['**Совет** из старого отчёта.'] }])
})

it('keeps same-named topics of different competencies apart', () => {
  expect(
    groupFeedbackDetails([
      {
        ...item('Основные понятия', 'Полезно изучить тему.', 'full'),
        topic_id: 'topic:0',
      },
      {
        ...item('Основные понятия', 'Полезно повторение.'),
        topic_id: 'topic:1',
      },
      {
        ...item('Основные понятия', 'Полезно изучить тему.', 'full'),
        topic_id: 'topic:0',
      },
    ]),
  ).toEqual([
    {
      heading: 'Тема для изучения: Основные понятия.',
      items: ['Полезно изучить тему.'],
    },
    {
      heading: 'Тема для повторения: Основные понятия.',
      items: ['Полезно повторение.'],
    },
  ])
})
