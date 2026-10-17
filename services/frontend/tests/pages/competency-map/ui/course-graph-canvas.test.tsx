import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import type { CourseGraphView as CourseGraph } from '@/entities/course-graph'
import '@/shared/i18n'
import CourseGraphCanvas from '@/pages/competency-map/ui/course-graph-canvas'

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
