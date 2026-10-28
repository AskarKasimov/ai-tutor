import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { createQueryClient } from '@/bootstrap/providers'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { replaceSession } from '@/entities/user'
import { LearningRoadmap } from '@/pages/learning-trainer/ui/learning-roadmap'
import { createQueryWrapper } from '../../../support/query-wrapper'

async function setup() {
  const client = createQueryClient()
  await replaceSession(client, {
    id: 'student',
    email: 'student@example.com',
    display_name: null,
    role: 'student',
  })
  const dependencies = createAppDependencies()
  const read = vi.spyOn(dependencies.learningMap, 'readOutline')
  return { read, wrapper: createQueryWrapper(client, dependencies) }
}

it('shows a loading state, then allows retrying a failed outline without starting a diagnostic', async () => {
  const { read, wrapper } = await setup()
  let reject!: (error: Error) => void
  read
    .mockImplementationOnce(
      () =>
        new Promise((_resolve, rejectPromise) => {
          reject = rejectPromise
        }),
    )
    .mockResolvedValue({ competencies: [] })
  render(<LearningRoadmap userId="student" subjectId="course" />, { wrapper })
  expect(screen.getByRole('status')).toHaveTextContent('Загружаем план')
  reject(new Error('Unavailable'))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Не удалось загрузить план обучения',
  )
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(
    await screen.findByText(
      'В карте пока нет блоков, подходящих для диагностики: на компетенцию нужны три готовых TRUE-ОР.',
    ),
  ).toBeVisible()
  expect(read).toHaveBeenCalledTimes(2)
})

it('replaces the outline when switching courses and preserves map order', async () => {
  const { read, wrapper } = await setup()
  read.mockImplementation(async (subjectId) => ({
    competencies: [
      {
        id: 'c1',
        name: `Компетенция ${subjectId}`,
        constituents: [
          {
            id: 't1',
            name: `Тема ${subjectId}`,
            outcomes: [{ id: 'o1', name: `Результат ${subjectId}` }],
          },
        ],
      },
      {
        id: 'c2',
        name: 'Следующая компетенция',
        constituents: [
          {
            id: 't2',
            name: 'Следующая тема',
            outcomes: [{ id: 'o2', name: 'Следующий результат' }],
          },
        ],
      },
    ],
  }))
  const view = render(<LearningRoadmap userId="student" subjectId="one" />, {
    wrapper,
  })
  fireEvent.click(await screen.findByLabelText('Тема one'))
  expect(await screen.findByText('Результат one')).toBeVisible()
  expect(screen.queryByText('Следующий результат')).not.toBeInTheDocument()
  expect(
    screen.queryByText(/Компетенций:.*Тем:.*Результатов:/),
  ).not.toBeInTheDocument()
  expect(
    screen.queryByText(/Блоки карты, выбранные для диагностики/),
  ).not.toBeInTheDocument()
  view.rerender(<LearningRoadmap userId="student" subjectId="two" />)
  fireEvent.click(await screen.findByLabelText('Тема two'))
  expect(await screen.findByText('Результат two')).toBeVisible()
  expect(screen.queryByText('Результат one')).not.toBeInTheDocument()
  expect(read).toHaveBeenLastCalledWith('two', expect.any(AbortSignal))
})
