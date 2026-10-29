import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import type { CourseGraphView as CourseGraph } from '@/entities/course-graph'
import '@/shared/i18n'
import CourseGraphCanvas from '@/widgets/course-graph'

const graph: CourseGraph = {
  topics: [
    {
      id: 'ml',
      title: 'Машинное обучение',
      question: 'Что такое обучение по данным?',
    },
    {
      id: 'loss',
      title: 'Функция потерь',
      provides: ['Оценка ошибки модели'],
      sources: [
        {
          material_id: 'mat-1',
          block_id: 1,
          start: 0,
          end: 10,
          text: 'Функция потерь',
          week: 2,
          page: 14,
        },
      ],
    },
    {
      id: 'gd',
      title: 'Градиентный спуск',
      question: 'Зачем нужен шаг обучения?',
      text: 'Метод **итеративной** оптимизации.',
      requires: ['Понимание функции потерь'],
    },
  ],
  hierarchy: [
    { source: 'ml', target: 'loss' },
    { source: 'ml', target: 'gd' },
  ],
  prerequisites: [
    { source: 'loss', target: 'gd', reason: 'Спуск минимизирует потери' },
  ],
}

function renderCanvas() {
  return render(
    <Theme>
      <CourseGraphCanvas
        graph={graph}
        materialNames={new Map([['mat-1', 'lecture2.pdf']])}
      />
    </Theme>,
  )
}

function node(title: string) {
  return screen
    .getByRole('region', { name: 'Граф знаний курса' })
    .querySelector(`[aria-label="${title}"]`) as HTMLElement
}

it('renders every topic as a card on the canvas', () => {
  renderCanvas()
  const canvas = screen.getByRole('region', { name: 'Граф знаний курса' })
  for (const topic of graph.topics)
    expect(within(canvas).getAllByText(topic.title).length).toBeGreaterThan(0)
  expect(within(canvas).getByText('Раздел')).toBeInTheDocument()
})

it('opens the details of a clicked topic and moves between linked topics', () => {
  renderCanvas()
  fireEvent.click(node('Градиентный спуск'))

  const details = screen.getByRole('complementary', {
    name: 'Градиентный спуск',
  })
  expect(within(details).getByText('Зачем нужен шаг обучения?')).toBeVisible()
  expect(within(details).getByText('итеративной')).toBeInTheDocument()
  expect(within(details).getByText('Понимание функции потерь')).toBeVisible()
  expect(within(details).getByText('Спуск минимизирует потери')).toBeVisible()

  fireEvent.click(
    within(details).getByRole('button', { name: 'Функция потерь' }),
  )
  const next = screen.getByRole('complementary', { name: 'Функция потерь' })
  expect(within(next).getByText('Оценка ошибки модели')).toBeVisible()
  expect(
    within(next).getByText('lecture2.pdf · неделя 2 · стр. 14'),
  ).toBeVisible()
  expect(
    within(next).getByRole('button', { name: 'Градиентный спуск' }),
  ).toBeInTheDocument()

  fireEvent.click(
    within(next).getByRole('button', { name: 'Закрыть карточку темы' }),
  )
  expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
})

it('opens a focused topic from the keyboard', () => {
  renderCanvas()
  fireEvent.keyDown(node('Функция потерь'), { key: 'Enter' })
  expect(
    screen.getByRole('complementary', { name: 'Функция потерь' }),
  ).toBeInTheDocument()
})

it('searches topics and opens a found one', () => {
  renderCanvas()
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Поиск темы в графе' }),
    {
      target: { value: 'спуск' },
    },
  )
  expect(screen.getByRole('status')).toHaveTextContent('Найдена 1 тема')
  fireEvent.click(screen.getByRole('button', { name: 'Градиентный спуск' }))
  expect(
    screen.getByRole('complementary', { name: 'Градиентный спуск' }),
  ).toBeInTheDocument()

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Поиск темы в графе' }),
    {
      target: { value: 'нейросеть' },
    },
  )
  expect(screen.getByRole('status')).toHaveTextContent('Таких тем нет')
})

const twoSections: CourseGraph = {
  topics: [
    ...graph.topics,
    { id: 'stats', title: 'Статистика' },
    { id: 'mean', title: 'Среднее значение' },
  ],
  hierarchy: [...graph.hierarchy!, { source: 'stats', target: 'mean' }],
  prerequisites: graph.prerequisites,
}

function renderGraph(value: CourseGraph) {
  const canvas = (g: CourseGraph) => (
    <Theme>
      <CourseGraphCanvas graph={g} materialNames={new Map()} />
    </Theme>
  )
  const result = render(canvas(value))
  return { ...result, update: (g: CourseGraph) => result.rerender(canvas(g)) }
}

const state = (title: string) =>
  node(title).querySelector('[data-state]')!.getAttribute('data-state')

it('lets a new search or section win over the topic picked before', () => {
  renderGraph(twoSections)
  fireEvent.click(node('Функция потерь'))
  expect(state('Среднее значение')).toBe('dimmed')

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Поиск темы в графе' }),
    { target: { value: 'Среднее' } },
  )
  expect(state('Среднее значение')).not.toBe('dimmed')
  expect(screen.queryByRole('complementary')).not.toBeInTheDocument()

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Поиск темы в графе' }),
    { target: { value: '' } },
  )
  fireEvent.click(node('Функция потерь'))
  const legend = screen.getByRole('group', { name: 'Разделы курса' })
  fireEvent.click(within(legend).getByRole('button', { name: /Статистика/ }))
  expect(state('Среднее значение')).not.toBe('dimmed')
  expect(state('Функция потерь')).toBe('dimmed')
})

it('drops the filter of a section the rebuilt graph no longer has', () => {
  const { update } = renderGraph(twoSections)
  const legend = screen.getByRole('group', { name: 'Разделы курса' })
  fireEvent.click(within(legend).getByRole('button', { name: /Статистика/ }))
  expect(state('Функция потерь')).toBe('dimmed')

  // The rebuild renamed the section, so its topic id changed.
  update({
    ...twoSections,
    topics: twoSections.topics.map((t) =>
      t.id === 'stats'
        ? { id: 'stats-2', title: 'Описательная статистика' }
        : t,
    ),
    hierarchy: [...graph.hierarchy!, { source: 'stats-2', target: 'mean' }],
  })
  expect(state('Функция потерь')).not.toBe('dimmed')
  expect(state('Среднее значение')).not.toBe('dimmed')
  expect(
    within(screen.getByRole('group', { name: 'Разделы курса' }))
      .getAllByRole('button')
      .some((b) => b.getAttribute('aria-pressed') === 'true'),
  ).toBe(false)
})

it('labels the built-in controls in Russian', () => {
  renderCanvas()
  for (const name of ['Приблизить', 'Отдалить', 'Показать весь граф'])
    expect(screen.getByRole('button', { name })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Zoom|Fit View/ })).toBeNull()
})

it('lets a section chosen after a search win over the search', () => {
  renderGraph(twoSections)
  const search = screen.getByRole('textbox', { name: 'Поиск темы в графе' })
  fireEvent.change(search, { target: { value: 'Среднее' } })
  expect(state('Функция потерь')).toBe('dimmed')

  const legend = screen.getByRole('group', { name: 'Разделы курса' })
  const ml = within(legend).getByRole('button', { name: /Машинное обучение/ })
  fireEvent.click(ml)
  expect(ml).toHaveAttribute('aria-pressed', 'true')
  expect(state('Функция потерь')).not.toBe('dimmed')
  expect(state('Среднее значение')).toBe('dimmed')
  expect(search).toHaveValue('')

  // And a new search releases the section.
  fireEvent.change(search, { target: { value: 'Среднее' } })
  expect(ml).toHaveAttribute('aria-pressed', 'false')
  expect(state('Среднее значение')).not.toBe('dimmed')
})

it('shows every task variant of a topic of a competency map', () => {
  renderGraph({
    topics: [
      {
        id: 'peas',
        title: 'Составляет PEAS',
        question: 'Опишите такси по PEAS.',
        text: 'Эталон: P — безопасность.',
        variants: [
          {
            question: 'Опишите такси по PEAS.',
            content: 'Эталон: P — безопасность.',
            assessment_guidance: 'Балл (макс. 2)',
          },
          {
            question: 'Раскройте назначение компонентов PEAS.',
            content: 'Эталонные элементы: мера задаёт сенсоры.',
            assessment_guidance: 'Пункт рубрики 1 «Назначение»: 2 — полно.',
          },
        ],
      },
    ],
    hierarchy: [],
    prerequisites: [],
  })
  fireEvent.click(node('Составляет PEAS'))
  const details = screen.getByRole('complementary', { name: 'Составляет PEAS' })
  expect(
    within(details).getByText('Варианты задания: экзамен выбирает один'),
  ).toBeVisible()
  expect(within(details).getByText('Вариант 2')).toBeVisible()
  expect(
    within(details).getByText('Раскройте назначение компонентов PEAS.'),
  ).toBeVisible()
  expect(
    within(details).getByText('Эталонные элементы: мера задаёт сенсоры.'),
  ).toBeInTheDocument()
  expect(
    within(details).getByText('Пункт рубрики 1 «Назначение»: 2 — полно.'),
  ).toBeVisible()
})

it('shows the rubric of the only task variant of a topic', () => {
  renderGraph({
    topics: [
      {
        id: 'peas',
        title: 'Составляет PEAS',
        question: 'Опишите такси по PEAS.',
        text: 'Эталон: P — безопасность.',
        variants: [
          {
            question: 'Опишите такси по PEAS.',
            content: 'Эталон: P — безопасность.',
            assessment_guidance: 'Балл (макс. 2): 2 — все четыре компонента.',
          },
        ],
      },
    ],
    hierarchy: [],
    prerequisites: [],
  })
  fireEvent.click(node('Составляет PEAS'))
  const details = screen.getByRole('complementary', { name: 'Составляет PEAS' })
  expect(within(details).getByText('Опишите такси по PEAS.')).toBeVisible()
  expect(within(details).getByText('Рубрика')).toBeVisible()
  expect(
    within(details).getByText('Балл (макс. 2): 2 — все четыре компонента.'),
  ).toBeVisible()
})
